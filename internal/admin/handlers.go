package admin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"bootimus/bootloaders"
	"bootimus/internal/autoinstall"
	"bootimus/internal/extractor"
	"bootimus/internal/models"
	"bootimus/internal/profiles"
	"bootimus/internal/redfish"
	"bootimus/internal/smb"
	"bootimus/internal/storage"
	"bootimus/internal/sysstats"
	"bootimus/internal/tools"
	"bootimus/internal/wim"
	"bootimus/internal/wol"
)

type BootloaderSelector interface {
	GetActiveBootloaderSet() string
	SetActiveBootloaderSet(name string)
	SaveBootloaderConfig() error
}

type Handler struct {
	storage            storage.Storage
	dataDir            string
	isoDir             string
	bootDir            string
	version            string
	bootloaderSelector BootloaderSelector
	toolsManager       *tools.Manager
	wolBroadcastAddr   string
	profileManager     *profiles.Manager
	proxyDHCPEnabled   bool
	httpPort           int
	serverAddr         string
	smbPort            int
	smbManager         *smb.Manager
	smbRequested       bool
	autoInstallLib     *autoinstall.Library
	extractionMu       sync.RWMutex
	extractionStates   map[string]*extractionState
	SchedulerReload    func() error
	SchedulerRunNow    func(id uint) error
}

type extractionState struct {
	reporter *extractor.ProgressReporter
	status   string
	errMsg   string
}

func NewHandler(store storage.Storage, dataDir string, isoDir string, bootDir string, version string, blSelector BootloaderSelector, tm *tools.Manager, wolBroadcastAddr string, pm *profiles.Manager, proxyDHCPEnabled bool, httpPort int, serverAddr string, smbPort int, smbManager *smb.Manager, smbRequested bool, autoInstallLib *autoinstall.Library) *Handler {
	return &Handler{
		storage:            store,
		dataDir:            dataDir,
		isoDir:             isoDir,
		bootDir:            bootDir,
		version:            version,
		bootloaderSelector: blSelector,
		toolsManager:       tm,
		profileManager:     pm,
		wolBroadcastAddr:   wolBroadcastAddr,
		proxyDHCPEnabled:   proxyDHCPEnabled,
		httpPort:           httpPort,
		serverAddr:         serverAddr,
		smbPort:            smbPort,
		smbManager:         smbManager,
		smbRequested:       smbRequested,
		autoInstallLib:     autoInstallLib,
		extractionStates:   make(map[string]*extractionState),
	}
}

func (h *Handler) computeSMBPatchFingerprint(img *models.Image) string {
	if img == nil {
		return ""
	}
	autoInstallContent := ""
	if img.AutoInstallEnabled {
		autoInstallContent = h.resolveImageAutoInstallContent(img)
	}
	hasher := sha256.New()
	fmt.Fprintf(hasher, "addr=%s\nfile=%s\nai_enabled=%v\nai_file=%s\nai_content=", h.serverAddr, img.Filename, img.AutoInstallEnabled, img.AutoInstallFile)
	hasher.Write([]byte(autoInstallContent))
	return hex.EncodeToString(hasher.Sum(nil))
}

func (h *Handler) patchWindowsBootWim(isoFilename string) bool {
	if h.smbManager == nil {
		return false
	}
	if h.serverAddr == "" {
		log.Printf("Windows SMB: skipping boot.wim patch - server address not configured")
		return false
	}
	if !wim.IsAvailable() {
		log.Printf("Windows SMB: skipping boot.wim patch - wimlib-imagex not available")
		return false
	}

	isoBase := strings.TrimSuffix(isoFilename, filepath.Ext(isoFilename))
	sharePath := filepath.Join(h.isoDir, isoBase, "iso")

	bootWimPath := findExtractedBootWim(sharePath)
	if bootWimPath == "" {
		log.Printf("Windows SMB: skipping boot.wim patch - %s has no extracted sources/boot.wim", isoFilename)
		return false
	}

	wimMgr, err := wim.NewManager()
	if err != nil {
		log.Printf("Windows SMB: skipping boot.wim patch - %v", err)
		return false
	}

	shareName := smb.SanitizeShareName(isoBase)
	autoInstall := false
	if img, err := h.storage.GetImage(isoFilename); err == nil && img != nil && img.AutoInstallEnabled {
		if content := h.resolveImageAutoInstallContent(img); content != "" {
			xmlPath := filepath.Join(sharePath, "AutoUnattend.xml")
			if err := os.WriteFile(xmlPath, []byte(content), 0644); err != nil {
				log.Printf("Windows SMB: failed to stage autounattend.xml on share: %v", err)
			} else {
				autoInstall = true
				log.Printf("Windows SMB: staged autounattend.xml on share for %s (%d bytes)", isoFilename, len(content))
			}
		}
	}
	script := buildStartnetScript(h.serverAddr, shareName, h.smbPort, h.httpPort, isoFilename, autoInstall)
	if err := wimMgr.PatchStartnetCmd(bootWimPath, script); err != nil {
		log.Printf("Windows SMB: failed to patch boot.wim for %s: %v", isoFilename, err)
		return false
	}

	h.smbManager.AddShare(shareName, sharePath)
	if err := h.smbManager.Reload(); err != nil {
		log.Printf("Windows SMB: failed to reload smbd after adding share %q: %v", shareName, err)
	}
	log.Printf("Windows SMB: boot.wim patched for %s (share: %s)", isoFilename, shareName)
	return true
}

func (h *Handler) resolveImageAutoInstallContent(img *models.Image) string {
	if h.autoInstallLib != nil && img.AutoInstallFile != "" {
		if c, err := h.autoInstallLib.ReadPath(img.AutoInstallFile); err == nil {
			return c
		}
	}
	return img.AutoInstallScript
}

func findExtractedBootWim(extractedISODir string) string {
	for _, rel := range []string{"sources/boot.wim", "SOURCES/BOOT.WIM"} {
		p := filepath.Join(extractedISODir, rel)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func buildStartnetScript(serverAddr, shareName string, smbPort, httpPort int, isoFilename string, autoInstall bool) string {
	if smbPort == 0 {
		smbPort = 445
	}
	if httpPort == 0 {
		httpPort = 8080
	}

	base := fmt.Sprintf(`@echo off
wpeinit
echo Acquiring DHCP lease...
ipconfig /renew >nul 2>&1

echo Waiting for network...
set /a TRIES=0
:waitnet
ping -n 1 -w 1000 %s >nul 2>&1
if not errorlevel 1 goto netready
set /a TRIES+=1
if %%TRIES%% geq 60 goto netfail
ping 127.0.0.1 -n 2 >nul 2>&1
goto waitnet
:netfail
echo ERROR: Could not reach %s after 60 seconds.
echo Dropping to shell. Try: ipconfig, ping %s
echo Type 'exit' to reboot.
cmd.exe
exit /b 1
:netready

echo Connecting to bootimus installation source...
rem Kick the SMB client stack — net use otherwise triggers lazy init and races wpeinit.
net start Workstation >nul 2>&1
set /a TRIES=0
:mapshare
net use Z: \\%s\%s /persistent:no >nul
if not errorlevel 1 goto mapped
set /a TRIES+=1
if %%TRIES%% geq 30 goto mapfail
ping 127.0.0.1 -n 4 >nul 2>&1
goto mapshare
:mapfail
echo.
echo ERROR: Failed to connect to \\%s\%s after 90 seconds (SMB port %d)
echo Dropping to shell for debugging. Try: net use Z: \\%s\%s
echo Type 'exit' to reboot.
cmd.exe
exit /b 1
:mapped

if not exist Z:\setup.exe (
	echo.
	echo ERROR: Z:\setup.exe not found on the share.
	dir Z:\
	echo Type 'exit' to reboot.
	cmd.exe
	exit /b 1
)
`, serverAddr, serverAddr, serverAddr, serverAddr, shareName, serverAddr, shareName, smbPort, serverAddr, shareName)

	launch := "echo Starting Windows Setup...\r\nZ:\\setup.exe\r\n"
	if autoInstall {
		launch = `copy /Y Z:\AutoUnattend.xml X:\AutoUnattend.xml >nul
if not exist X:\AutoUnattend.xml (
	echo WARNING: AutoUnattend.xml not on share, running interactive setup.
	Z:\setup.exe
	exit /b 0
)
echo Starting Windows Setup (unattended)...
Z:\setup.exe /unattend:X:\AutoUnattend.xml
`
	}

	return base + launch
}

func isRunningInDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	data, err := os.ReadFile("/proc/1/cgroup")
	if err == nil {
		content := string(data)
		if strings.Contains(content, "docker") || strings.Contains(content, "containerd") {
			return true
		}
	}

	if os.Getpid() == 1 {
		entries, err := os.ReadDir("/proc")
		if err == nil && len(entries) < 50 {
			return true
		}
	}

	return false
}

type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func (h *Handler) sendJSON(w http.ResponseWriter, status int, resp Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}

	if !resp.Success {
		log.Printf("Admin API error (status %d): %s", status, resp.Error)
	}
}

func (h *Handler) ListClients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	clients, err := h.storage.ListClients()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: clients})
}

func (h *Handler) GetClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	mac := r.URL.Query().Get("mac")
	if mac == "" {
		idStr := r.URL.Query().Get("id")
		if idStr != "" {
			id, err := strconv.ParseUint(idStr, 10, 64)
			if err != nil {
				h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid id"})
				return
			}
			clients, _ := h.storage.ListClients()
			for _, c := range clients {
				if c.ID == uint(id) {
					h.sendJSON(w, http.StatusOK, Response{Success: true, Data: c})
					return
				}
			}
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Client not found"})
			return
		}
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing mac or id parameter"})
		return
	}

	client, err := h.storage.GetClient(mac)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Client not found"})
		return
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: client})
}

func (h *Handler) CreateClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var client models.Client
	if err := json.NewDecoder(r.Body).Decode(&client); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	client.MACAddress = strings.ToLower(strings.ReplaceAll(client.MACAddress, "-", ":"))

	client.Enabled = true
	client.Static = true

	if err := h.storage.CreateClient(&client); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Client created - MAC: %s, Name: %s", client.MACAddress, client.Name)
	h.sendJSON(w, http.StatusCreated, Response{Success: true, Message: "Client created", Data: client})
}

func (h *Handler) UpdateClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	mac := r.URL.Query().Get("mac")
	if mac == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing mac parameter"})
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	client, err := h.storage.GetClient(mac)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Client not found"})
		return
	}

	if name, ok := updates["name"].(string); ok {
		client.Name = name
	}
	if desc, ok := updates["description"].(string); ok {
		client.Description = desc
	}
	if enabled, ok := updates["enabled"].(bool); ok {
		client.Enabled = enabled
	}
	if showPub, ok := updates["show_public_images"].(bool); ok {
		client.ShowPublicImages = showPub
	}
	if bls, ok := updates["bootloader_set"].(string); ok {
		client.BootloaderSet = bls
	}
	if aif, ok := updates["auto_install_file"].(string); ok {
		client.AutoInstallFile = aif
	}
	if groupID, ok := updates["client_group_id"]; ok {
		if groupID == nil {
			client.ClientGroupID = nil
		} else if gid, ok := groupID.(float64); ok {
			groupIDUint := uint(gid)
			client.ClientGroupID = &groupIDUint
		}
	}

	if err := h.storage.UpdateClient(mac, client); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Client updated - MAC: %s, Name: %s, Enabled: %v, ShowPublicImages: %v, BootloaderSet: %s", client.MACAddress, client.Name, client.Enabled, client.ShowPublicImages, client.BootloaderSet)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Client updated", Data: client})
}

func (h *Handler) DeleteClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	mac := r.URL.Query().Get("mac")
	if mac == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing mac parameter"})
		return
	}

	if err := h.storage.DeleteClient(mac); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Client deleted - MAC: %s", mac)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Client deleted"})
}

func (h *Handler) WakeClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	mac := r.URL.Query().Get("mac")
	if mac == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing mac parameter"})
		return
	}

	if _, err := h.storage.GetClient(mac); err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Client not found"})
		return
	}

	broadcastAddr := h.wolBroadcastAddr
	var req struct {
		BroadcastAddr string `json:"broadcast_addr"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
		if req.BroadcastAddr != "" {
			broadcastAddr = req.BroadcastAddr
		}
	}

	if err := wol.SendMagicPacket(mac, broadcastAddr); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: fmt.Sprintf("Failed to send WOL packet: %v", err)})
		return
	}

	log.Printf("Admin: Wake-on-LAN sent to %s (broadcast: %s)", mac, broadcastAddr)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: fmt.Sprintf("Wake-on-LAN packet sent to %s", mac)})
}

func (h *Handler) SetNextBootImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var req struct {
		MACAddress    string `json:"mac_address"`
		ImageFilename string `json:"image_filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	if req.MACAddress == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing mac_address"})
		return
	}

	if req.ImageFilename == "" {
		if err := h.storage.ClearNextBootImage(req.MACAddress); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
		log.Printf("Admin: Cleared next boot image for %s", req.MACAddress)
		h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Next boot action cleared"})
		return
	}

	if err := h.storage.SetNextBootImage(req.MACAddress, req.ImageFilename); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Set next boot image for %s to %s", req.MACAddress, req.ImageFilename)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: fmt.Sprintf("Next boot set to %s", req.ImageFilename)})
}

func (h *Handler) PromoteClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	mac := r.URL.Query().Get("mac")
	if mac == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing mac parameter"})
		return
	}

	client, err := h.storage.GetClient(mac)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Client not found"})
		return
	}

	client.Static = true
	if err := h.storage.UpdateClient(mac, client); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Client promoted to static - MAC: %s", mac)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Client promoted to static"})
}

func (h *Handler) GetClientInventory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	mac := r.URL.Query().Get("mac")
	if mac == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing mac parameter"})
		return
	}

	inv, err := h.storage.GetLatestHardwareInventory(mac)
	if err != nil {
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: nil})
		return
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: inv})
}

func (h *Handler) GetClientInventoryHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	mac := r.URL.Query().Get("mac")
	if mac == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing mac parameter"})
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	history, err := h.storage.GetHardwareInventoryHistory(mac, limit)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: history})
}

func (h *Handler) syncFilesystemToDatabase() {
	var isoFiles []models.SyncFile

	err := filepath.WalkDir(h.isoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".iso") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			log.Printf("Failed to get file info for %s: %v", path, err)
			return nil
		}

		relPath, _ := filepath.Rel(h.isoDir, path)
		groupPath := filepath.Dir(relPath)
		if groupPath == "." {
			groupPath = ""
		}

		isoFiles = append(isoFiles, models.SyncFile{
			Name:      strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())),
			Filename:  relPath,
			Size:      info.Size(),
			GroupPath: groupPath,
		})

		return nil
	})
	if err != nil {
		log.Printf("Failed to walk ISO directory for sync: %v", err)
		return
	}

	if err := h.storage.SyncImages(isoFiles); err != nil {
		log.Printf("Failed to sync images with database: %v", err)
	}

	h.detectManualExtractions()
}

func (h *Handler) detectManualExtractions() {
	images, err := h.storage.ListImages()
	if err != nil {
		return
	}

	for _, image := range images {
		if image.Extracted {
			continue
		}

		isoBase := strings.TrimSuffix(filepath.Base(image.Filename), filepath.Ext(image.Filename))
		bootDir := filepath.Join(h.isoDir, filepath.Dir(image.Filename), isoBase)

		kernelPath := filepath.Join(bootDir, "vmlinuz")
		initrdPath := filepath.Join(bootDir, "initrd")

		kernelExists := fileExistsOnDisk(kernelPath)
		initrdExists := fileExistsOnDisk(initrdPath)

		if !kernelExists || !initrdExists {
			continue
		}

		now := time.Now()
		image.Extracted = true
		image.KernelPath = kernelPath
		image.InitrdPath = initrdPath
		image.BootMethod = "kernel"
		image.ExtractedAt = &now

		if image.Distro == "" {
			image.Distro = detectDistroFromFilename(image.Filename)
		}

		if err := h.storage.UpdateImage(image.Filename, image); err != nil {
			log.Printf("Failed to update manually extracted image %s: %v", image.Filename, err)
		} else {
			log.Printf("Detected manual extraction for %s", image.Filename)
		}
	}
}

func fileExistsOnDisk(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func detectDistroFromFilename(filename string) string {
	lower := strings.ToLower(filename)
	patterns := map[string]string{
		"ubuntu": "ubuntu", "debian": "debian", "arch": "arch",
		"fedora": "fedora", "centos": "centos", "rocky": "centos",
		"alma": "centos", "opensuse": "opensuse", "nixos": "nixos",
		"proxmox": "debian", "truenas": "debian", "pop-os": "ubuntu",
		"pop_os": "ubuntu", "mint": "ubuntu", "kali": "debian",
		"windows": "windows", "freebsd": "freebsd",
	}
	for pattern, distro := range patterns {
		if strings.Contains(lower, pattern) {
			return distro
		}
	}
	return ""
}

func (h *Handler) detectAndSetDistro(image *models.Image) {
	if h.profileManager == nil || image == nil || image.Distro != "" {
		return
	}
	if profile, err := h.profileManager.MatchProfile(image.Filename); err == nil {
		image.Distro = profile.ProfileID
		log.Printf("Admin: Detected distro %s for %s", profile.ProfileID, image.Filename)
	}
}

func (h *Handler) ListImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	h.syncFilesystemToDatabase()

	images, err := h.storage.ListImages()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	for _, img := range images {
		if img.SMBInstallEnabled && img.SMBPatchFingerprint != "" {
			img.SMBNeedsRepatch = h.computeSMBPatchFingerprint(img) != img.SMBPatchFingerprint
		}
	}

	log.Printf("ListImages returning %d images", len(images))
	for i, img := range images {
		log.Printf("  [%d] %s (filename: %s, size: %d)", i, img.Name, img.Filename, img.Size)
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: images})
}

func (h *Handler) GetImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	filename := r.URL.Query().Get("filename")
	if filename == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing filename parameter"})
		return
	}

	image, err := h.storage.GetImage(filename)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
		return
	}

	if image.SMBInstallEnabled && image.SMBPatchFingerprint != "" {
		image.SMBNeedsRepatch = h.computeSMBPatchFingerprint(image) != image.SMBPatchFingerprint
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: image})
}

func (h *Handler) UpdateImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	filename := r.URL.Query().Get("filename")
	if filename == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing filename parameter"})
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	image, err := h.storage.GetImage(filename)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
		return
	}

	if name, ok := updates["name"].(string); ok && name != "" {
		image.Name = name
	}
	if desc, ok := updates["description"].(string); ok {
		image.Description = desc
	}
	if enabled, ok := updates["enabled"].(bool); ok {
		image.Enabled = enabled
	}
	if public, ok := updates["public"].(bool); ok {
		image.Public = public
	}
	if groupID, ok := updates["group_id"]; ok {
		if groupID == nil {
			image.GroupID = nil
		} else if gid, ok := groupID.(float64); ok {
			groupIDUint := uint(gid)
			image.GroupID = &groupIDUint
		}
	}
	if order, ok := updates["order"].(float64); ok {
		image.Order = int(order)
	}
	if bootMethod, ok := updates["boot_method"].(string); ok {
		image.BootMethod = bootMethod
	}
	if distro, ok := updates["distro"].(string); ok {
		image.Distro = distro
	}
	if bootParams, ok := updates["boot_params"].(string); ok {
		image.BootParams = bootParams
	}
	if aiFile, ok := updates["auto_install_file"].(string); ok {
		image.AutoInstallFile = aiFile
		image.AutoInstallEnabled = aiFile != "" || image.AutoInstallScript != ""
	}

	if err := h.storage.UpdateImage(filename, image); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Image updated: %s (enabled=%v, public=%v)", filename, image.Enabled, image.Public)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Image updated", Data: image})
}

func (h *Handler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	filename := r.URL.Query().Get("filename")
	deleteFile := r.URL.Query().Get("delete_file") == "true"

	if filename == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing filename parameter"})
		return
	}

	if deleteFile {
		filePath := filepath.Join(h.isoDir, filename)
		if err := os.Remove(filePath); err != nil {
			log.Printf("Failed to delete file %s: %v", filePath, err)
		} else {
			log.Printf("Deleted ISO file: %s", filename)
		}

		isoBase := strings.TrimSuffix(filename, filepath.Ext(filename))
		extractedDir := filepath.Join(h.isoDir, isoBase)
		if _, err := os.Stat(extractedDir); err == nil {
			if err := os.RemoveAll(extractedDir); err != nil {
				log.Printf("Failed to delete extracted directory %s: %v", extractedDir, err)
			} else {
				log.Printf("Cleaned up extracted kernel directory: %s", extractedDir)
			}
		}
	}

	if err := h.storage.DeleteImage(filename); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	if h.smbManager != nil {
		isoBase := strings.TrimSuffix(filename, filepath.Ext(filename))
		shareName := smb.SanitizeShareName(isoBase)
		if h.smbManager.HasShare(shareName) {
			h.smbManager.RemoveShare(shareName)
			if err := h.smbManager.Reload(); err != nil {
				log.Printf("Windows SMB: failed to reload after removing share %q: %v", shareName, err)
			}
		}
	}

	log.Printf("Admin: Image deleted - %s", filename)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Image deleted"})
}

func (h *Handler) UploadImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	reader, err := r.MultipartReader()
	if err != nil {
		log.Printf("Failed to read multipart body: %v", err)
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid multipart body"})
		return
	}

	var (
		filename    string
		filePath    string
		size        int64
		fileSaved   bool
		publicValue string
		description string
	)

	cleanup := func() {
		if fileSaved {
			os.Remove(filePath)
		}
	}

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			cleanup()
			log.Printf("Failed to read multipart part: %v", err)
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid multipart body"})
			return
		}

		switch part.FormName() {
		case "file":
			filename = filepath.Base(part.FileName())
			if !strings.HasSuffix(strings.ToLower(filename), ".iso") {
				part.Close()
				log.Printf("Upload rejected: invalid file type: %s", filename)
				h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Only .iso files are allowed"})
				return
			}

			filePath = filepath.Join(h.isoDir, filename)
			if _, err := os.Stat(filePath); err == nil {
				part.Close()
				log.Printf("Upload rejected: file already exists on filesystem: %s", filename)
				h.sendJSON(w, http.StatusConflict, Response{Success: false, Error: "An image with this filename already exists"})
				return
			}

			dst, err := os.Create(filePath)
			if err != nil {
				part.Close()
				log.Printf("Failed to create file %s: %v", filePath, err)
				h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to create file"})
				return
			}

			log.Printf("Starting ISO upload: %s", filename)
			size, err = io.Copy(dst, &progressReader{r: part, name: filename})
			closeErr := dst.Close()
			part.Close()
			if err == nil {
				err = closeErr
			}
			if err != nil {
				os.Remove(filePath)
				log.Printf("Failed to save file %s: %v", filename, err)
				h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to save file"})
				return
			}
			fileSaved = true
			log.Printf("Upload complete: %s (%d MB)", filename, size/(1024*1024))

		case "public":
			b, _ := io.ReadAll(io.LimitReader(part, 64))
			publicValue = string(b)
			part.Close()

		case "description":
			b, _ := io.ReadAll(io.LimitReader(part, 4096))
			description = string(b)
			part.Close()

		default:
			part.Close()
		}
	}

	if !fileSaved {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "No file provided"})
		return
	}

	existingImage, err := h.storage.GetImage(filename)
	if err == nil && existingImage != nil {
		existingImage.Size = size
		existingImage.Enabled = true
		if publicValue == "on" || publicValue == "true" || publicValue == "false" {
			existingImage.Public = publicValue == "on" || publicValue == "true"
		}
		if description != "" {
			existingImage.Description = description
		}

		h.detectAndSetDistro(existingImage)

		if err := h.storage.UpdateImage(filename, existingImage); err != nil {
			cleanup()
			log.Printf("Failed to update image record, file removed: %s - %v", filename, err)
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to update image record"})
			return
		}

		log.Printf("Admin: Image re-uploaded and database updated - %s (%d MB)", existingImage.Filename, existingImage.Size/1024/1024)
		h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Image re-uploaded successfully", Data: existingImage})
		return
	}

	displayName := strings.TrimSuffix(filename, filepath.Ext(filename))
	isPublic := publicValue == "on" || publicValue == "true"

	image := models.Image{
		Name:        displayName,
		Filename:    filename,
		Size:        size,
		Enabled:     true,
		Public:      isPublic,
		Description: description,
	}

	h.detectAndSetDistro(&image)

	if err := h.storage.CreateImage(&image); err != nil {
		cleanup()
		log.Printf("Failed to create image record, file removed: %s - %v", filename, err)
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to create image record"})
		return
	}

	log.Printf("Admin: Image uploaded successfully - %s (%d MB)", image.Filename, image.Size/1024/1024)
	h.sendJSON(w, http.StatusCreated, Response{Success: true, Message: "Image uploaded", Data: image})
}

type progressReader struct {
	r       io.Reader
	name    string
	read    int64
	lastLog int64
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.read += int64(n)
		if p.read-p.lastLog >= 100*1024*1024 {
			log.Printf("Upload progress: %s - %d MB read", p.name, p.read/(1024*1024))
			p.lastLog = p.read
		}
	}
	return n, err
}

func (h *Handler) AssignImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var req struct {
		MACAddress     string   `json:"mac_address"`
		ImageFilenames []string `json:"image_filenames"`
		ClientID       uint     `json:"client_id"`
		ImageIDs       []uint   `json:"image_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	if req.MACAddress == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing mac_address"})
		return
	}

	if err := h.storage.AssignImagesToClient(req.MACAddress, req.ImageFilenames); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Images assigned to client: %s -> %v", req.MACAddress, req.ImageFilenames)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Images assigned to client"})
}

func (h *Handler) ExtractImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	filename := r.URL.Query().Get("filename")
	if filename == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing filename parameter"})
		return
	}

	image, err := h.storage.GetImage(filename)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
		return
	}

	log.Printf("Admin: Starting kernel/initrd extraction - %s (re-extract: %v)", filename, image.Extracted)

	ext, err := extractor.New(h.isoDir)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: fmt.Sprintf("Failed to create extractor: %v", err)})
		return
	}

	isoPath := filepath.Join(h.isoDir, filename)

	reporter := extractor.NewProgressReporter()
	reporter.SetStage("Scanning ISO...")
	if info, statErr := os.Stat(isoPath); statErr == nil {
		reporter.SetTotalBytes(info.Size())
	}
	ext.SetProgress(reporter)

	state := &extractionState{reporter: reporter, status: "running"}
	h.extractionMu.Lock()
	h.extractionStates[filename] = state
	h.extractionMu.Unlock()
	defer func() {
		go func() {
			time.Sleep(5 * time.Second)
			h.extractionMu.Lock()
			delete(h.extractionStates, filename)
			h.extractionMu.Unlock()
		}()
	}()

	reporter.SetStage("Extracting boot files...")
	bootFiles, err := ext.Extract(isoPath)
	if err != nil {
		h.extractionMu.Lock()
		state.status = "error"
		state.errMsg = err.Error()
		h.extractionMu.Unlock()

		image.ExtractionError = err.Error()
		h.storage.UpdateImage(filename, image)

		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to extract boot files: %v", err),
		})
		return
	}
	reporter.SetStage("Saving metadata...")

	if err := ext.SaveMetadata(filename, bootFiles); err != nil {
		log.Printf("Failed to save extraction metadata: %v", err)
	}

	sanbootCompatible, sanbootHint := checkSanbootCompatibility(bootFiles.Distro, image.Filename)

	now := time.Now()
	image.Extracted = true
	image.Distro = bootFiles.Distro
	image.BootMethod = "kernel"
	image.KernelPath = bootFiles.Kernel
	image.InitrdPath = bootFiles.Initrd
	image.SquashfsPath = bootFiles.SquashfsPath

	if h.profileManager != nil && bootFiles.Distro != "" {
		hasSquashfs := bootFiles.SquashfsPath != ""
		profileParams := h.profileManager.GetBootParams(bootFiles.Distro, hasSquashfs)
		if profileParams != "" {
			image.BootParams = profileParams
		} else {
			image.BootParams = strings.TrimSpace(bootFiles.BootParams)
		}
	} else {
		image.BootParams = strings.TrimSpace(bootFiles.BootParams)
	}
	image.ExtractionError = ""
	image.ExtractedAt = &now
	image.SanbootCompatible = sanbootCompatible
	image.SanbootHint = sanbootHint
	image.NetbootRequired = bootFiles.NetbootRequired
	image.NetbootURL = bootFiles.NetbootURL
	image.NetbootAvailable = false
	image.InstallWimPath = bootFiles.InstallWim

	if bootFiles.Distro == "windows" {
		image.SMBInstallEnabled = h.patchWindowsBootWim(filename)
		if image.SMBInstallEnabled {
			image.SMBPatchFingerprint = h.computeSMBPatchFingerprint(image)
		}
	}

	log.Printf("Setting boot_method to 'kernel' for image ID=%d, filename=%s", image.ID, image.Filename)

	if err := h.storage.UpdateImage(filename, image); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Image extraction completed - %s (distro: %s, kernel: %s, initrd: %s)",
		filename, bootFiles.Distro, bootFiles.Kernel, bootFiles.Initrd)

	reporter.SetStage("Complete")
	h.extractionMu.Lock()
	state.status = "done"
	h.extractionMu.Unlock()

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: fmt.Sprintf("Successfully extracted %s boot files", bootFiles.Distro),
		Data:    image,
	})
}

func (h *Handler) ExtractProgress(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("filename")
	if filename == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing filename parameter"})
		return
	}

	h.extractionMu.RLock()
	state, ok := h.extractionStates[filename]
	h.extractionMu.RUnlock()

	if !ok {
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: map[string]any{"status": "idle"}})
		return
	}

	snap := state.reporter.Snapshot()
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: map[string]any{
		"status":  state.status,
		"stage":   snap.Stage,
		"percent": snap.Percent,
		"error":   state.errMsg,
	}})
}

func (h *Handler) PatchImageSMB(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	filename := r.URL.Query().Get("filename")
	if filename == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing filename parameter"})
		return
	}

	if h.smbManager == nil {
		h.sendJSON(w, http.StatusPreconditionFailed, Response{Success: false, Error: "Windows SMB is not enabled on this server"})
		return
	}
	if !wim.IsAvailable() {
		h.sendJSON(w, http.StatusPreconditionFailed, Response{Success: false, Error: "wimlib-imagex not installed on the server. Install wimtools (e.g. apt install wimtools) to enable boot.wim patching."})
		return
	}

	image, err := h.storage.GetImage(filename)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
		return
	}
	if image.Distro != "windows" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Not a Windows image"})
		return
	}
	if !image.Extracted {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Image must be extracted first"})
		return
	}

	ok := h.patchWindowsBootWim(filename)
	image.SMBInstallEnabled = ok
	if ok {
		image.SMBPatchFingerprint = h.computeSMBPatchFingerprint(image)
	}
	if err := h.storage.UpdateImage(filename, image); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	if !ok {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Patch failed — see server logs"})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "boot.wim patched for SMB auto-install", Data: image})
}

func (h *Handler) RedetectImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	filename := r.URL.Query().Get("filename")
	if filename == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing filename parameter"})
		return
	}

	image, err := h.storage.GetImage(filename)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
		return
	}

	if !image.Extracted {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Image must be extracted first"})
		return
	}

	if h.profileManager != nil {
		if profile, err := h.profileManager.MatchProfile(filename); err == nil {
			image.Distro = profile.ProfileID
		}
		if image.Distro != "" {
			if _, err := h.storage.GetDistroProfile(image.Distro); err != nil {
				if profile, err := h.profileManager.MatchProfile(image.Distro); err == nil {
					image.Distro = profile.ProfileID
				}
			}
		}
	}

	isoBase := strings.TrimSuffix(filename, filepath.Ext(filename))
	extractedDir := filepath.Join(h.isoDir, isoBase, "iso")
	var squashfsPath string
	filepath.Walk(extractedDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(info.Name()), ".squashfs") {
			rel, _ := filepath.Rel(filepath.Join(h.isoDir, isoBase), path)
			squashfsPath = rel
			return filepath.SkipAll
		}
		return nil
	})
	image.SquashfsPath = squashfsPath

	if h.profileManager != nil && image.Distro != "" {
		hasSquashfs := squashfsPath != ""
		image.BootParams = h.profileManager.GetBootParams(image.Distro, hasSquashfs)
	} else {
		image.BootParams = ""
	}

	sanbootCompatible, sanbootHint := checkSanbootCompatibility(image.Distro, image.Filename)
	image.SanbootCompatible = sanbootCompatible
	image.SanbootHint = sanbootHint

	if err := h.storage.UpdateImage(filename, image); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Re-detected image %s (distro: %s, squashfs: %s)", filename, image.Distro, squashfsPath)
	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: fmt.Sprintf("Re-detected: distro=%s, squashfs=%s", image.Distro, squashfsPath),
		Data:    image,
	})
}

func (h *Handler) ListDistroProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	profs, err := h.storage.ListDistroProfiles()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: profs})
}

func (h *Handler) SaveDistroProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var profile models.DistroProfile
	if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	if profile.ProfileID == "" || profile.DisplayName == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Profile ID and display name are required"})
		return
	}

	existing, err := h.storage.GetDistroProfile(profile.ProfileID)
	if err == nil {
		profile.ID = existing.ID
		profile.CreatedAt = existing.CreatedAt
	}

	profile.Custom = true
	if err := h.storage.SaveDistroProfile(&profile); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Distro profile saved - %s (%s)", profile.DisplayName, profile.ProfileID)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Profile saved", Data: profile})
}

func (h *Handler) DeleteDistroProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	profileID := r.URL.Query().Get("id")
	if profileID == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing id parameter"})
		return
	}

	profile, err := h.storage.GetDistroProfile(profileID)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Profile not found"})
		return
	}

	if !profile.Custom {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Cannot delete built-in profiles. They will be restored on next update."})
		return
	}

	if err := h.storage.DeleteDistroProfile(profileID); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Distro profile deleted - %s", profileID)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Profile deleted"})
}

func (h *Handler) GetISOCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	catalog, err := profiles.LoadISOCatalog()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: catalog})
}

func (h *Handler) UpdateDistroProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	if h.profileManager == nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Profile manager not available"})
		return
	}

	added, updated, version, err := h.profileManager.UpdateFromRemote()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Distro profiles updated from remote (version: %s, added: %d, updated: %d)", version, added, updated)
	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: fmt.Sprintf("Updated to version %s (%d added, %d updated)", version, added, updated),
	})
}

func (h *Handler) UpdateTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	if h.toolsManager == nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Tools manager not available"})
		return
	}

	added, updated, version, err := h.toolsManager.UpdateFromRemote()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Tools manifest updated from remote (version: %s, added: %d, updated: %d)", version, added, updated)
	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: fmt.Sprintf("Updated to version %s (%d added, %d updated)", version, added, updated),
	})
}

func checkSanbootCompatibility(distro, filename string) (bool, string) {
	filenameLower := strings.ToLower(filename)

	if strings.Contains(filenameLower, "winpe") ||
		strings.Contains(filenameLower, "windows pe") ||
		strings.Contains(filenameLower, "memtest") ||
		strings.Contains(filenameLower, "gparted") && strings.Contains(filenameLower, "live") {
		return true, ""
	}

	incompatibleDistros := map[string]string{
		"windows":  "Windows requires boot file extraction. Use 'Extract Kernel/Initrd' to extract boot files for wimboot support.",
		"ubuntu":   "Ubuntu requires kernel extraction. Use 'Extract Kernel/Initrd' for network boot support.",
		"debian":   "Debian requires kernel extraction. Use 'Extract Kernel/Initrd' for network boot support.",
		"fedora":   "Fedora requires kernel extraction. Use 'Extract Kernel/Initrd' for network boot support.",
		"centos":   "CentOS requires kernel extraction. Use 'Extract Kernel/Initrd' for network boot support.",
		"arch":     "Arch Linux requires kernel extraction. Use 'Extract Kernel/Initrd' for network boot support.",
		"opensuse": "openSUSE requires kernel extraction. Use 'Extract Kernel/Initrd' for network boot support.",
		"nixos":    "NixOS requires kernel extraction. Use 'Extract Kernel/Initrd' for network boot support.",
		"mint":     "Linux Mint requires kernel extraction. Use 'Extract Kernel/Initrd' for network boot support.",
		"manjaro":  "Manjaro requires kernel extraction. Use 'Extract Kernel/Initrd' for network boot support.",
		"popos":    "Pop!_OS requires kernel extraction. Use 'Extract Kernel/Initrd' for network boot support.",
		"kali":     "Kali Linux requires kernel extraction. Use 'Extract Kernel/Initrd' for network boot support.",
		"rocky":    "Rocky Linux requires kernel extraction. Use 'Extract Kernel/Initrd' for network boot support.",
		"alma":     "AlmaLinux requires kernel extraction. Use 'Extract Kernel/Initrd' for network boot support.",
	}

	if hint, found := incompatibleDistros[distro]; found {
		return false, hint
	}

	if distro != "" {
		return false, "This Linux distribution likely requires kernel extraction. Use 'Extract Kernel/Initrd' for reliable network boot support."
	}

	return true, ""
}

func (h *Handler) SetBootMethod(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var req struct {
		Filename   string `json:"filename"`
		BootMethod string `json:"boot_method"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	if req.Filename == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing filename"})
		return
	}

	if req.BootMethod != "sanboot" && req.BootMethod != "kernel" && req.BootMethod != "nbd" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid boot method (must be 'sanboot', 'kernel', or 'nbd')"})
		return
	}

	image, err := h.storage.GetImage(req.Filename)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
		return
	}

	if req.BootMethod == "kernel" && !image.Extracted {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Cannot use kernel boot method: image not extracted. Please extract first.",
		})
		return
	}

	if req.BootMethod == "sanboot" && !image.SanbootCompatible {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   fmt.Sprintf("Sanboot not recommended for this ISO. %s", image.SanbootHint),
		})
		return
	}

	image.BootMethod = req.BootMethod

	if err := h.storage.UpdateImage(req.Filename, image); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Boot method changed for %s: %s", req.Filename, req.BootMethod)
	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: fmt.Sprintf("Boot method set to %s", req.BootMethod),
		Data:    image,
	})
}

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	statsMap, err := h.storage.GetStats()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	stats := struct {
		TotalClients  int64 `json:"total_clients"`
		ActiveClients int64 `json:"active_clients"`
		TotalImages   int64 `json:"total_images"`
		EnabledImages int64 `json:"enabled_images"`
		TotalBoots    int64 `json:"total_boots"`
	}{
		TotalClients:  statsMap["total_clients"],
		ActiveClients: statsMap["active_clients"],
		TotalImages:   statsMap["total_images"],
		EnabledImages: statsMap["enabled_images"],
		TotalBoots:    statsMap["total_boots"],
	}

	log.Printf("Stats retrieved: %d clients, %d images, %d boots", stats.TotalClients, stats.TotalImages, stats.TotalBoots)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: stats})
}

func (h *Handler) GetBootLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	var (
		logs []models.BootLog
		err  error
	)
	if mac := r.URL.Query().Get("mac"); mac != "" {
		mac = strings.ToLower(strings.ReplaceAll(mac, "-", ":"))
		logs, err = h.storage.GetBootLogsByMAC(mac, limit)
	} else {
		logs, err = h.storage.GetBootLogs(limit)
	}
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: logs})
}

func (h *Handler) ScanImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	existingFiles := make(map[string]bool)
	var isoFiles []models.SyncFile

	err := filepath.WalkDir(h.isoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".iso") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(h.isoDir, path)
		groupPath := filepath.Dir(relPath)
		if groupPath == "." {
			groupPath = ""
		}

		existingFiles[relPath] = true
		isoFiles = append(isoFiles, models.SyncFile{
			Name:      strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())),
			Filename:  relPath,
			Size:      info.Size(),
			GroupPath: groupPath,
		})

		return nil
	})
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	var newImages []string
	allImagesBefore, _ := h.storage.ListImages()
	existingFilenames := make(map[string]bool)
	for _, img := range allImagesBefore {
		existingFilenames[img.Filename] = true
	}

	if err := h.storage.SyncImages(isoFiles); err != nil {
		log.Printf("Failed to sync images during scan: %v", err)
	}

	for _, iso := range isoFiles {
		if !existingFilenames[iso.Filename] {
			newImages = append(newImages, iso.Filename)
			log.Printf("Admin: Image scan found new ISO - %s", iso.Filename)
		}
	}

	var deletedImages []string
	allImages, err := h.storage.ListImages()
	if err == nil {
		log.Printf("Checking %d database images against %d filesystem ISOs", len(allImages), len(existingFiles))
		for _, image := range allImages {
			if !existingFiles[image.Filename] {
				log.Printf("Deleting missing image from database: %s (ID: %d)", image.Filename, image.ID)
				if err := h.storage.DeleteImage(image.Filename); err == nil {
					deletedImages = append(deletedImages, image.Filename)
					log.Printf("Successfully removed missing image from database: %s", image.Filename)

					isoBase := strings.TrimSuffix(image.Filename, filepath.Ext(image.Filename))
					bootFilesDir := filepath.Join(h.isoDir, isoBase)
					if _, err := os.Stat(bootFilesDir); err == nil {
						if err := os.RemoveAll(bootFilesDir); err == nil {
							log.Printf("Cleaned up boot files directory: %s", bootFilesDir)
						}
					}
				} else {
					log.Printf("Failed to delete missing image from database: %s - %v", image.Filename, err)
				}
			}
		}
	}

	msg := fmt.Sprintf("Scan complete. Found %d new images, removed %d missing images.", len(newImages), len(deletedImages))
	log.Printf("Admin: ISO scan completed - %d new, %d removed", len(newImages), len(deletedImages))
	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: msg,
		Data: map[string]interface{}{
			"new":     newImages,
			"deleted": deletedImages,
		},
	})
}

type BootloaderFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type BootloaderSet struct {
	Name    string           `json:"name"`
	Files   []BootloaderFile `json:"files"`
	BuiltIn bool             `json:"built_in"`
}

func (h *Handler) CreateBootloaderSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	if h.bootDir == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Boot directory not configured"})
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	setName := strings.TrimSpace(req.Name)
	if setName == "" || setName == "built-in" || bootloaders.IsBuiltIn(setName) {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid set name (cannot collide with built-in)"})
		return
	}
	setName = filepath.Base(setName)

	setDir := filepath.Join(h.bootDir, setName)
	if _, err := os.Stat(setDir); err == nil {
		h.sendJSON(w, http.StatusConflict, Response{Success: false, Error: "Set already exists"})
		return
	}

	if err := os.MkdirAll(setDir, 0755); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: fmt.Sprintf("Failed to create set: %v", err)})
		return
	}

	log.Printf("Admin: Created bootloader set: %s", setName)
	h.sendJSON(w, http.StatusCreated, Response{Success: true, Message: fmt.Sprintf("Set '%s' created", setName)})
}

func (h *Handler) ListBootloaders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var sets []BootloaderSet

	embeddedSets, _ := bootloaders.ListSets()
	for _, setName := range embeddedSets {
		entries, err := bootloaders.ListFiles(setName)
		if err != nil {
			continue
		}
		var files []BootloaderFile
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			info, _ := entry.Info()
			if info == nil {
				continue
			}
			files = append(files, BootloaderFile{Name: entry.Name(), Size: info.Size()})
		}
		sets = append(sets, BootloaderSet{Name: setName, Files: files, BuiltIn: true})
	}

	if h.bootDir != "" {
		_ = os.MkdirAll(h.bootDir, 0755)
		entries, err := os.ReadDir(h.bootDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				setName := entry.Name()
				setPath := filepath.Join(h.bootDir, setName)
				fileEntries, err := os.ReadDir(setPath)
				if err != nil {
					continue
				}
				var files []BootloaderFile
				for _, fe := range fileEntries {
					if fe.IsDir() {
						continue
					}
					info, _ := fe.Info()
					if info == nil {
						continue
					}
					files = append(files, BootloaderFile{Name: fe.Name(), Size: info.Size()})
				}
				sets = append(sets, BootloaderSet{Name: setName, Files: files, BuiltIn: bootloaders.IsBuiltIn(setName)})
			}
		}
	}

	activeSet := h.bootloaderSelector.GetActiveBootloaderSet()
	if activeSet == "" {
		activeSet = "built-in"
	}

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Data: map[string]interface{}{
			"sets":      sets,
			"active":    activeSet,
		},
	})
}

func (h *Handler) UploadBootloader(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	if h.bootDir == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Boot directory not configured. Set boot_dir in config to enable custom bootloader uploads.",
		})
		return
	}

	if err := r.ParseMultipartForm(100 << 20); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to parse form: %v", err),
		})
		return
	}

	setName := r.FormValue("set")
	if setName == "" || setName == "built-in" || bootloaders.IsBuiltIn(setName) {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Cannot upload to built-in set"})
		return
	}
	setName = filepath.Base(setName)

	setDir := filepath.Join(h.bootDir, setName)
	if _, err := os.Stat(setDir); os.IsNotExist(err) {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Set does not exist. Create it first."})
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		files = r.MultipartForm.File["file"]
	}
	if len(files) == 0 {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "No files provided"})
		return
	}

	var uploaded []BootloaderFile
	for _, header := range files {
		filename := filepath.Base(header.Filename)
		if filename == "" || filename == "." || filename == ".." {
			continue
		}

		file, err := header.Open()
		if err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: fmt.Sprintf("Failed to read file %s: %v", filename, err)})
			return
		}

		destPath := filepath.Join(setDir, filename)
		dest, err := os.Create(destPath)
		if err != nil {
			file.Close()
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: fmt.Sprintf("Failed to create file %s: %v", filename, err)})
			return
		}

		written, err := io.Copy(dest, file)
		dest.Close()
		file.Close()

		if err != nil {
			os.Remove(destPath)
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: fmt.Sprintf("Failed to write file %s: %v", filename, err)})
			return
		}

		log.Printf("Uploaded bootloader: %s/%s (%d bytes)", setName, filename, written)
		uploaded = append(uploaded, BootloaderFile{Name: filename, Size: written})
	}

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: fmt.Sprintf("Uploaded %d file(s) to set '%s'", len(uploaded), setName),
		Data:    uploaded,
	})
}

func (h *Handler) DeleteBootloader(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	if h.bootDir == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Boot directory not configured"})
		return
	}

	setName := r.URL.Query().Get("set")
	if setName == "" || setName == "built-in" || bootloaders.IsBuiltIn(setName) {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Cannot delete built-in set"})
		return
	}
	setName = filepath.Base(setName)

	filename := r.URL.Query().Get("name")

	if filename == "" {
		setPath := filepath.Join(h.bootDir, setName)
		if err := os.RemoveAll(setPath); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: fmt.Sprintf("Failed to delete set: %v", err)})
			return
		}

		if h.bootloaderSelector.GetActiveBootloaderSet() == setName {
			h.bootloaderSelector.SetActiveBootloaderSet("")
			h.bootloaderSelector.SaveBootloaderConfig()
		}

		log.Printf("Deleted bootloader set: %s", setName)
		h.sendJSON(w, http.StatusOK, Response{Success: true, Message: fmt.Sprintf("Set deleted: %s", setName)})
		return
	}

	filename = filepath.Base(filename)
	filePath := filepath.Join(h.bootDir, setName, filename)
	if err := os.Remove(filePath); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: fmt.Sprintf("Failed to delete file: %v", err)})
		return
	}

	log.Printf("Deleted bootloader %s from set %s", filename, setName)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: fmt.Sprintf("Deleted %s from %s", filename, setName)})
}

func (h *Handler) SelectBootloader(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		activeSet := h.bootloaderSelector.GetActiveBootloaderSet()
		if activeSet == "" {
			activeSet = "built-in"
		}
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: activeSet})
		return
	}

	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var req struct {
		Set string `json:"set"` // "built-in" or set folder name
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request"})
		return
	}

	setName := req.Set
	if setName == "built-in" {
		setName = ""
	}

	h.bootloaderSelector.SetActiveBootloaderSet(setName)
	if err := h.bootloaderSelector.SaveBootloaderConfig(); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: fmt.Sprintf("Failed to save config: %v", err)})
		return
	}

	displayName := req.Set
	if displayName == "" {
		displayName = "built-in"
	}
	log.Printf("Active bootloader set changed to: %s", displayName)
	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: fmt.Sprintf("Active bootloader set: %s", displayName),
	})
}

func (h *Handler) ListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	toolsList, err := h.storage.ListBootTools()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	for _, t := range toolsList {
		t.Downloaded = h.toolsManager.IsDownloaded(t.Name)
		if t.DownloadURL == "" {
			if def := tools.GetDefinition(t.Name); def != nil {
				t.DownloadURL = def.DownloadURL
			}
		}
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: toolsList})
}

func (h *Handler) ToggleTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var req struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request"})
		return
	}

	tool, err := h.storage.GetBootTool(req.Name)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Tool not found"})
		return
	}

	if req.Enabled && !h.toolsManager.IsDownloaded(req.Name) {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Tool must be downloaded before enabling"})
		return
	}

	tool.Enabled = req.Enabled
	if err := h.storage.SaveBootTool(tool); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	status := "disabled"
	if req.Enabled {
		status = "enabled"
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: fmt.Sprintf("%s %s", tool.DisplayName, status)})
}

func (h *Handler) DownloadTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Tool name required"})
		return
	}

	def := tools.GetDefinition(name)
	if def == nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Unknown tool"})
		return
	}

	go func() {
		if err := h.toolsManager.Download(name, nil); err != nil {
			log.Printf("Tool download failed for %s: %v", name, err)
		}
	}()

	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: fmt.Sprintf("Downloading %s...", def.DisplayName)})
}

func (h *Handler) DeleteTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Tool name required"})
		return
	}

	if err := h.toolsManager.Delete(name); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Tool files deleted"})
}

func (h *Handler) UpdateToolURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var req struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request"})
		return
	}

	tool, err := h.storage.GetBootTool(req.Name)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Tool not found"})
		return
	}

	tool.DownloadURL = req.URL
	if err := h.storage.SaveBootTool(tool); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Download URL updated"})
}

func (h *Handler) CreateCustomTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var req struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Description string `json:"description"`
		Version     string `json:"version"`
		DownloadURL string `json:"download_url"`
		KernelPath  string `json:"kernel_path"`
		InitrdPath  string `json:"initrd_path"`
		BootParams  string `json:"boot_params"`
		BootMethod  string `json:"boot_method"`
		ArchiveType string `json:"archive_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	if req.Name == "" || req.DisplayName == "" || req.DownloadURL == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Name, display name, and download URL are required"})
		return
	}

	if req.BootMethod == "" {
		req.BootMethod = "kernel"
	}
	if req.ArchiveType == "" {
		req.ArchiveType = "bin"
	}

	tool := &models.BootTool{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Version:     req.Version,
		DownloadURL: req.DownloadURL,
		KernelPath:  req.KernelPath,
		InitrdPath:  req.InitrdPath,
		BootParams:  req.BootParams,
		BootMethod:  req.BootMethod,
		ArchiveType: req.ArchiveType,
		Custom:      true,
		Enabled:     false,
		Downloaded:  false,
	}

	if err := h.storage.SaveBootTool(tool); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Custom tool created - %s (%s)", req.DisplayName, req.Name)
	h.sendJSON(w, http.StatusCreated, Response{Success: true, Message: "Custom tool created", Data: tool})
}

func (h *Handler) DeleteCustomTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Tool name required"})
		return
	}

	tool, err := h.storage.GetBootTool(name)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Tool not found"})
		return
	}

	if !tool.Custom {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Cannot delete built-in tools"})
		return
	}

	h.toolsManager.Delete(name)

	if err := h.storage.DeleteBootTool(name); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Custom tool deleted - %s", name)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Custom tool deleted"})
}

func (h *Handler) ToolProgress(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Tool name required"})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: h.toolsManager.GetProgress(name)})
}

func (h *Handler) GetServerInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	monitoredPaths := sysstats.GetMonitoredPaths(h.dataDir)
	sysStats, err := sysstats.GetStats(monitoredPaths)
	if err != nil {
		log.Printf("Failed to get system stats: %v", err)
	}

	info := map[string]interface{}{
		"version": h.version,
		"configuration": map[string]string{
			"data_directory": h.dataDir,
			"iso_directory":  h.isoDir,
			"boot_directory": h.bootDir,
			"database_mode": func() string {
				if h.storage != nil {
					return "Enabled"
				}
				return "Disabled"
			}(),
			"runtime_mode": func() string {
				if isRunningInDocker() {
					return "Docker"
				}
				return "Native"
			}(),
			"ldap_enabled": func() string {
				if os.Getenv("BOOTIMUS_LDAP_HOST") != "" {
					return os.Getenv("BOOTIMUS_LDAP_HOST")
				}
				return "Disabled"
			}(),
			"proxy_dhcp": func() string {
				if h.proxyDHCPEnabled {
					return "Enabled (standalone — no external DHCP PXE config needed)"
				}
				return "Disabled (external DHCP must set next-server/bootfile)"
			}(),
			"windows_smb": func() string {
				if !h.smbRequested {
					return "Disabled"
				}
				if h.smbManager == nil {
					return "Requested but unavailable (install samba and ensure smbd is in PATH)"
				}
				return fmt.Sprintf("Enabled (%d share(s) active, port %d)", h.smbManager.ShareCount(), h.smbManager.Port())
			}(),
			"windows_smb_patcher": func() string {
				if wim.IsAvailable() {
					return "Available"
				}
				return "Unavailable (install wimtools / wimlib-imagex to enable boot.wim patching)"
			}(),
			"http_port": fmt.Sprintf("%d", h.httpPort),
		},
		"environment": map[string]string{
			"BOOTIMUS_TFTP_PORT":        os.Getenv("BOOTIMUS_TFTP_PORT"),
			"BOOTIMUS_TFTP_SINGLE_PORT": os.Getenv("BOOTIMUS_TFTP_SINGLE_PORT"),
			"BOOTIMUS_HTTP_PORT":        os.Getenv("BOOTIMUS_HTTP_PORT"),
			"BOOTIMUS_ADMIN_PORT":       os.Getenv("BOOTIMUS_ADMIN_PORT"),
			"BOOTIMUS_DATA_DIR":         os.Getenv("BOOTIMUS_DATA_DIR"),
			"BOOTIMUS_DB_HOST":          os.Getenv("BOOTIMUS_DB_HOST"),
			"BOOTIMUS_DB_PORT":          os.Getenv("BOOTIMUS_DB_PORT"),
			"BOOTIMUS_DB_USER":          os.Getenv("BOOTIMUS_DB_USER"),
			"BOOTIMUS_DB_NAME":          os.Getenv("BOOTIMUS_DB_NAME"),
			"BOOTIMUS_DB_SSLMODE":       os.Getenv("BOOTIMUS_DB_SSLMODE"),
			"BOOTIMUS_DB_DISABLE":       os.Getenv("BOOTIMUS_DB_DISABLE"),
			"BOOTIMUS_SERVER_ADDR":      os.Getenv("BOOTIMUS_SERVER_ADDR"),
			"BOOTIMUS_LDAP_HOST":        os.Getenv("BOOTIMUS_LDAP_HOST"),
			"BOOTIMUS_LDAP_BASE_DN":     os.Getenv("BOOTIMUS_LDAP_BASE_DN"),
		},
		"system_stats": sysStats,
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: info})
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.storage.ListUsers()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: users})
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"is_admin"`
		Enabled  bool   `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request"})
		return
	}

	if req.Username == "" || req.Password == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Username and password are required"})
		return
	}

	user := models.User{
		Username: req.Username,
		IsAdmin:  req.IsAdmin,
		Enabled:  req.Enabled,
	}

	if err := user.SetPassword(req.Password); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to hash password"})
		return
	}

	if _, err := h.storage.GetUser(req.Username); err == nil {
		h.sendJSON(w, http.StatusConflict, Response{Success: false, Error: "User already exists"})
		return
	}

	if err := h.storage.CreateUser(&user); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("User created: %s (admin=%v, enabled=%v)", user.Username, user.IsAdmin, user.Enabled)
	h.sendJSON(w, http.StatusCreated, Response{Success: true, Message: "User created", Data: user})
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Username required"})
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	user, err := h.storage.GetUser(username)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "User not found"})
		return
	}

	willBeAdmin := user.IsAdmin
	if isAdmin, ok := updates["is_admin"].(bool); ok {
		willBeAdmin = isAdmin
	}
	willBeEnabled := user.Enabled
	if enabled, ok := updates["enabled"].(bool); ok {
		willBeEnabled = enabled
	}

	if user.IsAdmin && user.Enabled && (!willBeAdmin || !willBeEnabled) {
		all, err := h.storage.ListUsers()
		if err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
		others := 0
		for _, u := range all {
			if u.Username != username && u.IsAdmin && u.Enabled {
				others++
			}
		}
		if others == 0 {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Cannot remove admin rights or disable the only active admin user"})
			return
		}
	}

	user.IsAdmin = willBeAdmin
	user.Enabled = willBeEnabled

	if err := h.storage.UpdateUser(username, user); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("User updated: %s (admin=%v, enabled=%v)", user.Username, user.IsAdmin, user.Enabled)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "User updated", Data: user})
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Username required"})
		return
	}

	if username == "admin" {
		h.sendJSON(w, http.StatusForbidden, Response{Success: false, Error: "Cannot delete admin user"})
		return
	}

	if err := h.storage.DeleteUser(username); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("User deleted: %s", username)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "User deleted"})
}

func (h *Handler) ResetUserPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		NewPassword string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request"})
		return
	}

	if req.Username == "" || req.NewPassword == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Username and new password are required"})
		return
	}

	user, err := h.storage.GetUser(req.Username)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "User not found"})
		return
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to hash password"})
		return
	}

	if err := h.storage.UpdateUser(req.Username, user); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Password reset for user: %s", user.Username)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Password reset successfully"})
}

type DownloadProgress struct {
	URL             string    `json:"url"`
	Filename        string    `json:"filename"`
	TotalBytes      int64     `json:"total_bytes"`
	DownloadedBytes int64     `json:"downloaded_bytes"`
	Percentage      float64   `json:"percentage"`
	Speed           string    `json:"speed"`
	Status          string    `json:"status"`
	Error           string    `json:"error,omitempty"`
	StartTime       time.Time `json:"start_time"`
}

type DownloadManager struct {
	mu        sync.RWMutex
	downloads map[string]*DownloadProgress
}

var downloadMgr = &DownloadManager{
	downloads: make(map[string]*DownloadProgress),
}

func (dm *DownloadManager) Add(url, filename string, totalBytes int64) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.downloads[filename] = &DownloadProgress{
		URL:        url,
		Filename:   filename,
		TotalBytes: totalBytes,
		Status:     "downloading",
		StartTime:  time.Now(),
	}
}

func (dm *DownloadManager) Update(filename string, downloadedBytes int64) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	if progress, ok := dm.downloads[filename]; ok {
		progress.DownloadedBytes = downloadedBytes
		if progress.TotalBytes > 0 {
			progress.Percentage = float64(downloadedBytes) / float64(progress.TotalBytes) * 100
		}

		elapsed := time.Since(progress.StartTime).Seconds()
		if elapsed > 0 {
			bytesPerSec := float64(downloadedBytes) / elapsed
			progress.Speed = formatBytes(int64(bytesPerSec)) + "/s"
		}
	}
}

func (dm *DownloadManager) Complete(filename string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	if progress, ok := dm.downloads[filename]; ok {
		progress.Status = "completed"
		progress.Percentage = 100
	}
}

func (dm *DownloadManager) Error(filename, errMsg string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	if progress, ok := dm.downloads[filename]; ok {
		progress.Status = "error"
		progress.Error = errMsg
	}
}

func (dm *DownloadManager) Get(filename string) *DownloadProgress {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.downloads[filename]
}

func (dm *DownloadManager) GetAll() []*DownloadProgress {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	result := make([]*DownloadProgress, 0, len(dm.downloads))
	for _, p := range dm.downloads {
		result = append(result, p)
	}
	return result
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func (h *Handler) DownloadISO(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL         string `json:"url"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request"})
		return
	}

	if req.URL == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "URL is required"})
		return
	}

	filename := filepath.Base(req.URL)
	if !strings.HasSuffix(strings.ToLower(filename), ".iso") {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "URL must point to an .iso file"})
		return
	}

	destPath := filepath.Join(h.isoDir, filename)
	if _, err := os.Stat(destPath); err == nil {
		h.sendJSON(w, http.StatusConflict, Response{Success: false, Error: "File already exists"})
		return
	}

	go h.downloadISO(req.URL, filename, destPath, req.Description)

	h.sendJSON(w, http.StatusAccepted, Response{
		Success: true,
		Message: "Download started",
		Data: map[string]string{
			"filename": filename,
			"url":      req.URL,
		},
	})
}

func (h *Handler) downloadISO(url, filename, destPath, description string) {
	log.Printf("Starting ISO download: %s from %s", filename, url)

	downloadMgr.Add(url, filename, 0)

	client := &http.Client{
		Timeout: 0,
	}

	resp, err := client.Get(url)
	if err != nil {
		log.Printf("Failed to download ISO %s: %v", filename, err)
		downloadMgr.Error(filename, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status)
		log.Printf("Failed to download ISO %s: %s", filename, errMsg)
		downloadMgr.Error(filename, errMsg)
		return
	}

	downloadMgr.Add(url, filename, resp.ContentLength)

	out, err := os.Create(destPath)
	if err != nil {
		log.Printf("Failed to create file %s: %v", destPath, err)
		downloadMgr.Error(filename, err.Error())
		return
	}
	defer out.Close()

	buffer := make([]byte, 32*1024)
	var downloaded int64

	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			_, writeErr := out.Write(buffer[:n])
			if writeErr != nil {
				log.Printf("Failed to write to file %s: %v", destPath, writeErr)
				downloadMgr.Error(filename, writeErr.Error())
				os.Remove(destPath)
				return
			}
			downloaded += int64(n)
			downloadMgr.Update(filename, downloaded)
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Failed to download ISO %s: %v", filename, err)
			downloadMgr.Error(filename, err.Error())
			os.Remove(destPath)
			return
		}
	}

	downloadMgr.Complete(filename)
	log.Printf("Completed ISO download: %s (%d bytes)", filename, downloaded)

	if h.storage != nil {
		isoFiles := []models.SyncFile{
			{Name: strings.TrimSuffix(filename, filepath.Ext(filename)), Filename: filename, Size: downloaded},
		}

		if err := h.storage.SyncImages(isoFiles); err != nil {
			log.Printf("Failed to sync downloaded ISO to database: %v", err)
		}

		if img, err := h.storage.GetImage(filename); err == nil {
			h.detectAndSetDistro(img)
			if img.Distro != "" {
				if err := h.storage.UpdateImage(filename, img); err != nil {
					log.Printf("Failed to save detected distro for %s: %v", filename, err)
				}
			}
		}
	}
}

func (h *Handler) GetDownloadProgress(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("filename")
	if filename == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Filename required"})
		return
	}

	progress := downloadMgr.Get(filename)
	if progress == nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Download not found"})
		return
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: progress})
}

func (h *Handler) ListDownloads(w http.ResponseWriter, r *http.Request) {
	downloads := downloadMgr.GetAll()
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: downloads})
}

func (h *Handler) GetAutoInstallScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	filename := r.URL.Query().Get("filename")
	if filename == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing filename parameter"})
		return
	}

	var image *models.Image
	var err error

	image, err = h.storage.GetImage(filename)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
		return
	}

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Data: map[string]interface{}{
			"script":      image.AutoInstallScript,
			"enabled":     image.AutoInstallEnabled,
			"script_type": image.AutoInstallScriptType,
		},
	})
}

func (h *Handler) UpdateAutoInstallScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	filename := r.URL.Query().Get("filename")
	if filename == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing filename parameter"})
		return
	}

	var req struct {
		Script     string `json:"script"`
		Enabled    bool   `json:"enabled"`
		ScriptType string `json:"script_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	validTypes := map[string]bool{
		"preseed":      true,
		"kickstart":    true,
		"autounattend": true,
		"autoinstall":  true,
	}

	if req.ScriptType != "" && !validTypes[req.ScriptType] {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Invalid script_type. Must be one of: preseed, kickstart, autounattend, autoinstall",
		})
		return
	}

	image, err := h.storage.GetImage(filename)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
		return
	}

	image.AutoInstallScript = req.Script
	image.AutoInstallEnabled = req.Enabled
	image.AutoInstallScriptType = req.ScriptType

	if err := h.storage.UpdateImage(filename, image); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Auto-install script updated for %s: enabled=%v, type=%s, size=%d bytes",
		filename, image.AutoInstallEnabled, image.AutoInstallScriptType, len(image.AutoInstallScript))

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "Auto-install script updated",
		Data:    image,
	})
}

func (h *Handler) ListAutoInstallFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	if h.autoInstallLib == nil {
		h.sendJSON(w, http.StatusServiceUnavailable, Response{Success: false, Error: "Auto-install manager unavailable"})
		return
	}
	files, err := h.autoInstallLib.List()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: files})
}

func (h *Handler) GetAutoInstallFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	if h.autoInstallLib == nil {
		h.sendJSON(w, http.StatusServiceUnavailable, Response{Success: false, Error: "Auto-install manager unavailable"})
		return
	}
	distro := r.URL.Query().Get("distro")
	filename := r.URL.Query().Get("filename")
	content, err := h.autoInstallLib.Read(distro, filename)
	if err != nil {
		if err == autoinstall.ErrNotFound {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "File not found"})
			return
		}
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: map[string]string{
		"distro":   distro,
		"filename": filename,
		"content":  content,
	}})
}

func (h *Handler) SaveAutoInstallFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	if h.autoInstallLib == nil {
		h.sendJSON(w, http.StatusServiceUnavailable, Response{Success: false, Error: "Auto-install manager unavailable"})
		return
	}
	var req struct {
		Distro   string `json:"distro"`
		Filename string `json:"filename"`
		Content  string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid body"})
		return
	}
	if h.profileManager != nil {
		if _, err := h.storage.GetDistroProfile(req.Distro); err != nil {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: fmt.Sprintf("Unknown distro profile: %s", req.Distro)})
			return
		}
	}
	if err := h.autoInstallLib.Write(req.Distro, req.Filename, req.Content); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Saved"})
}

func (h *Handler) UploadAutoInstallFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	if h.autoInstallLib == nil {
		h.sendJSON(w, http.StatusServiceUnavailable, Response{Success: false, Error: "Auto-install manager unavailable"})
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid multipart body"})
		return
	}
	distro := r.FormValue("distro")
	if _, err := h.storage.GetDistroProfile(distro); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: fmt.Sprintf("Unknown distro profile: %s", distro)})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing file"})
		return
	}
	defer file.Close()
	filename := r.FormValue("filename")
	if filename == "" {
		filename = header.Filename
	}
	buf, err := io.ReadAll(file)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Read failed"})
		return
	}
	if err := h.autoInstallLib.Write(distro, filename, string(buf)); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Uploaded", Data: map[string]string{
		"distro":   distro,
		"filename": filename,
	}})
}

func (h *Handler) DownloadAutoInstallFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	if h.autoInstallLib == nil {
		http.Error(w, "Auto-install manager unavailable", http.StatusServiceUnavailable)
		return
	}
	distro := r.URL.Query().Get("distro")
	filename := r.URL.Query().Get("filename")
	content, err := h.autoInstallLib.Read(distro, filename)
	if err != nil {
		if err == autoinstall.ErrNotFound {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	w.Write([]byte(content))
}

func (h *Handler) DeleteAutoInstallFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	if h.autoInstallLib == nil {
		h.sendJSON(w, http.StatusServiceUnavailable, Response{Success: false, Error: "Auto-install manager unavailable"})
		return
	}
	distro := r.URL.Query().Get("distro")
	filename := r.URL.Query().Get("filename")
	if err := h.autoInstallLib.Delete(distro, filename); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Deleted"})
}

func (h *Handler) ListCustomFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var files []*models.CustomFile
	var err error

	files, err = h.storage.ListCustomFiles()

	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: files})
}

func (h *Handler) GetCustomFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "File ID required"})
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid file ID"})
		return
	}

	file, err := h.storage.GetCustomFileByID(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "File not found"})
		return
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: file})
}

func (h *Handler) UploadCustomFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	if err := r.ParseMultipartForm(500 << 20); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to parse form: %v", err),
		})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "No file provided",
		})
		return
	}
	defer file.Close()

	description := r.FormValue("description")
	destinationPath := r.FormValue("destinationPath")
	autoInstallStr := r.FormValue("autoInstall")
	publicStr := r.FormValue("public")
	imageIDStr := r.FormValue("imageId")

	isPublic := publicStr == "true"
	autoInstall := autoInstallStr == "true"

	var imageID *uint
	if imageIDStr != "" && imageIDStr != "null" && imageIDStr != "0" {
		id, err := strconv.ParseUint(imageIDStr, 10, 32)
		if err == nil {
			uid := uint(id)
			imageID = &uid
		}
	}

	originalFilename := filepath.Base(header.Filename)
	if originalFilename == "" || originalFilename == "." || originalFilename == ".." {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Invalid filename",
		})
		return
	}

	cleanFilename := filepath.Clean(originalFilename)

	var destDir string
	if isPublic {
		destDir = filepath.Join(h.dataDir, "files")
	} else if imageID != nil {
		var imageName string
		var images []*models.Image
		images, _ = h.storage.ListImages()
		for _, i := range images {
			if i.ID == *imageID {
				imageName = strings.TrimSuffix(i.Filename, filepath.Ext(i.Filename))
				break
			}
		}

		if imageName == "" {
			h.sendJSON(w, http.StatusBadRequest, Response{
				Success: false,
				Error:   "Image not found for image-specific file",
			})
			return
		}

		destDir = filepath.Join(h.isoDir, imageName, "autoinstall")
	} else {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "File must be either public or assigned to an image",
		})
		return
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to create directory: %v", err),
		})
		return
	}

	destPath := filepath.Join(destDir, cleanFilename)
	dest, err := os.Create(destPath)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to create file: %v", err),
		})
		return
	}
	defer dest.Close()

	written, err := io.Copy(dest, file)
	if err != nil {
		os.Remove(destPath)
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to write file: %v", err),
		})
		return
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	log.Printf("Checking for existing file: filename=%s, imageID=%v, public=%v", cleanFilename, imageID, isPublic)
	existingFile, err := h.storage.GetCustomFileByFilenameAndImage(cleanFilename, imageID, isPublic)
	if err != nil {
		log.Printf("Query result: %v", err)
	}
	if existingFile != nil {
		log.Printf("File %s already exists (ID: %d), deleting old record", cleanFilename, existingFile.ID)
		if err := h.storage.DeleteCustomFile(existingFile.ID); err != nil {
			log.Printf("ERROR: Failed to delete existing file record: %v", err)
			os.Remove(destPath)
			h.sendJSON(w, http.StatusInternalServerError, Response{
				Success: false,
				Error:   fmt.Sprintf("Failed to delete existing file: %v", err),
			})
			return
		}
		log.Printf("Successfully deleted old file record ID: %d", existingFile.ID)
	} else {
		log.Printf("No existing file found, creating new record")
	}

	customFile := &models.CustomFile{
		Filename:        cleanFilename,
		OriginalName:    originalFilename,
		Description:     description,
		Size:            written,
		ContentType:     contentType,
		Public:          isPublic,
		ImageID:         imageID,
		DestinationPath: destinationPath,
		AutoInstall:     autoInstall,
	}

	log.Printf("Attempting to create file record: %+v", customFile)
	if err = h.storage.CreateCustomFile(customFile); err != nil {
		log.Printf("ERROR: Failed to create file record: %v", err)
		os.Remove(destPath)
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to save file metadata: %v", err),
		})
		return
	}

	log.Printf("Uploaded custom file: %s (%d bytes, public=%v, imageID=%v)",
		cleanFilename, written, isPublic, imageID)

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "File uploaded successfully",
		Data:    customFile,
	})
}

func (h *Handler) UpdateCustomFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "File ID required"})
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid file ID"})
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	file, err := h.storage.GetCustomFileByID(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "File not found"})
		return
	}

	if desc, ok := updates["description"].(string); ok {
		file.Description = desc
	}

	if err = h.storage.UpdateCustomFile(uint(id), file); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Updated custom file: %s (ID: %d)", file.Filename, file.ID)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "File updated", Data: file})
}

func (h *Handler) DeleteCustomFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "File ID required"})
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid file ID"})
		return
	}

	file, err := h.storage.GetCustomFileByID(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "File not found"})
		return
	}

	var filePath string
	if file.Public {
		filePath = filepath.Join(h.dataDir, "files", file.Filename)
	} else if file.ImageID != nil && file.Image != nil {
		imageName := strings.TrimSuffix(file.Image.Filename, filepath.Ext(file.Image.Filename))
		filePath = filepath.Join(h.isoDir, imageName, "files", file.Filename)
	}

	if err = h.storage.DeleteCustomFile(uint(id)); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	if filePath != "" {
		if err := os.Remove(filePath); err != nil {
			log.Printf("Warning: Failed to delete file %s: %v", filePath, err)
		}
	}

	log.Printf("Deleted custom file: %s (ID: %d)", file.Filename, file.ID)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "File deleted"})
}


func (h *Handler) ListDriverPacks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	imageIDStr := r.URL.Query().Get("imageId")
	var packs []*models.DriverPack
	var err error

	if imageIDStr != "" {
		imageID, parseErr := strconv.ParseUint(imageIDStr, 10, 32)
		if parseErr != nil {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid image ID"})
			return
		}
		packs, err = h.storage.ListDriverPacksByImage(uint(imageID))
	} else {
		packs, err = h.storage.ListDriverPacks()
	}

	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: packs})
}

func (h *Handler) UploadDriverPack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	if err := r.ParseMultipartForm(500 << 20); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to parse form: %v", err),
		})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "No file provided",
		})
		return
	}
	defer file.Close()

	description := r.FormValue("description")
	imageIDStr := r.FormValue("imageId")

	if imageIDStr == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Image ID is required",
		})
		return
	}

	imageID, err := strconv.ParseUint(imageIDStr, 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Invalid image ID",
		})
		return
	}

	var images []*models.Image
	images, _ = h.storage.ListImages()
	var imageName string
	for _, img := range images {
		if img.ID == uint(imageID) {
			imageName = strings.TrimSuffix(img.Filename, filepath.Ext(img.Filename))
			break
		}
	}

	if imageName == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Image not found",
		})
		return
	}

	originalFilename := filepath.Base(header.Filename)
	if originalFilename == "" || originalFilename == "." || originalFilename == ".." {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Invalid filename",
		})
		return
	}

	if !strings.HasSuffix(strings.ToLower(originalFilename), ".zip") {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Only ZIP files are allowed",
		})
		return
	}

	cleanFilename := filepath.Clean(originalFilename)
	destDir := filepath.Join(h.isoDir, imageName, "drivers")

	if err := os.MkdirAll(destDir, 0755); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to create directory: %v", err),
		})
		return
	}

	destPath := filepath.Join(destDir, cleanFilename)
	dest, err := os.Create(destPath)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to create file: %v", err),
		})
		return
	}
	defer dest.Close()

	written, err := io.Copy(dest, file)
	if err != nil {
		os.Remove(destPath)
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to write file: %v", err),
		})
		return
	}

	driverPack := &models.DriverPack{
		Filename:     cleanFilename,
		OriginalName: originalFilename,
		Description:  description,
		Size:         written,
		ImageID:      uint(imageID),
		Enabled:      true,
	}

	if err = h.storage.CreateDriverPack(driverPack); err != nil {
		os.Remove(destPath)
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to save driver pack metadata: %v", err),
		})
		return
	}

	log.Printf("Uploaded driver pack: %s (%d bytes) for image %s", cleanFilename, written, imageName)

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "Driver pack uploaded successfully",
		Data:    driverPack,
	})
}

func (h *Handler) DeleteDriverPack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "ID required"})
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid ID"})
		return
	}

	pack, err := h.storage.GetDriverPack(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Driver pack not found"})
		return
	}

	var images []*models.Image
	images, _ = h.storage.ListImages()
	var imageName string
	for _, img := range images {
		if img.ID == pack.ImageID {
			imageName = strings.TrimSuffix(img.Filename, filepath.Ext(img.Filename))
			break
		}
	}

	if err = h.storage.DeleteDriverPack(uint(id)); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	if imageName != "" {
		filePath := filepath.Join(h.isoDir, imageName, "drivers", pack.Filename)
		if err := os.Remove(filePath); err != nil {
			log.Printf("Warning: Failed to delete driver pack file %s: %v", filePath, err)
		}
	}

	log.Printf("Deleted driver pack: %s (ID: %d)", pack.Filename, pack.ID)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Driver pack deleted"})
}

func (h *Handler) RebuildImageBootWim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	imageIDStr := r.URL.Query().Get("imageId")
	if imageIDStr == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Image ID required"})
		return
	}

	imageID, err := strconv.ParseUint(imageIDStr, 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid image ID"})
		return
	}

	log.Printf("Rebuilding boot.wim for image ID %d...", imageID)

	go func() {
		if err := h.RebuildBootWim(uint(imageID)); err != nil {
			log.Printf("ERROR: Failed to rebuild boot.wim for image %d: %v", imageID, err)
		} else {
			log.Printf("Successfully rebuilt boot.wim for image %d", imageID)
		}
	}()

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "Boot.wim rebuild started in background. Check logs for progress.",
	})
}

func (h *Handler) ListImageGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	groups, err := h.storage.ListImageGroups()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: groups})
}

func (h *Handler) CreateImageGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var group models.ImageGroup
	if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	if group.Name == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Group name is required"})
		return
	}

	if err := h.storage.CreateImageGroup(&group); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Created image group: %s (ID: %d)", group.Name, group.ID)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Group created", Data: group})
}

func (h *Handler) UpdateImageGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Group ID required"})
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid group ID"})
		return
	}

	var group models.ImageGroup
	if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	group.ID = uint(id)

	if err := h.storage.UpdateImageGroup(uint(id), &group); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Updated image group: %s (ID: %d)", group.Name, group.ID)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Group updated", Data: group})
}

func (h *Handler) DeleteImageGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Group ID required"})
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid group ID"})
		return
	}

	group, err := h.storage.GetImageGroup(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Group not found"})
		return
	}

	if err := h.storage.DeleteImageGroup(uint(id)); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Deleted image group: %s (ID: %d)", group.Name, group.ID)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Group deleted"})
}

func (h *Handler) resolveRedfish(c *models.Client) (string, int, string, string, bool, bool) {
	host := c.IPMIHost
	port := c.IPMIPort
	user := c.IPMIUsername
	pass := c.IPMIPassword
	insecure := c.IPMIInsecure

	if c.ClientGroupID != nil {
		if g, err := h.storage.GetClientGroup(*c.ClientGroupID); err == nil && g != nil {
			if port == 0 {
				port = g.IPMIPort
			}
			if user == "" {
				user = g.IPMIUsername
			}
			if pass == "" {
				pass = g.IPMIPassword
			}
			if !insecure {
				insecure = g.IPMIInsecure
			}
		}
	}

	if host == "" || user == "" || pass == "" {
		return "", 0, "", "", false, false
	}
	return host, port, user, pass, insecure, true
}

func (h *Handler) PowerClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	mac := strings.ToLower(strings.ReplaceAll(r.URL.Query().Get("mac"), "-", ":"))
	action := r.URL.Query().Get("action")
	if mac == "" || action == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "mac and action are required"})
		return
	}
	c, err := h.storage.GetClient(mac)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Client not found"})
		return
	}
	host, port, user, pass, insecure, ok := h.resolveRedfish(c)
	if !ok {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Client has no BMC host + credentials (set on client or group)"})
		return
	}
	client := redfish.New(host, port, user, pass, insecure)
	if err := client.SetPower(r.Context(), redfish.PowerAction(action)); err != nil {
		log.Printf("Redfish %s on %s (%s) failed: %v", action, mac, host, err)
		h.sendJSON(w, http.StatusOK, Response{Success: false, Error: err.Error()})
		return
	}
	log.Printf("Redfish %s on %s (%s) succeeded", action, mac, host)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: fmt.Sprintf("Power %s sent to %s", action, mac)})
}

func (h *Handler) PowerStatusClient(w http.ResponseWriter, r *http.Request) {
	mac := strings.ToLower(strings.ReplaceAll(r.URL.Query().Get("mac"), "-", ":"))
	if mac == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "mac required"})
		return
	}
	c, err := h.storage.GetClient(mac)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Client not found"})
		return
	}
	host, port, user, pass, insecure, ok := h.resolveRedfish(c)
	if !ok {
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: map[string]string{"state": "unconfigured"}})
		return
	}
	client := redfish.New(host, port, user, pass, insecure)
	state, err := client.PowerState(r.Context())
	if err != nil {
		h.sendJSON(w, http.StatusOK, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: map[string]string{"state": state}})
}

func (h *Handler) PowerClientGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid group ID"})
		return
	}
	action := r.URL.Query().Get("action")
	if action == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "action required"})
		return
	}
	group, err := h.storage.GetClientGroup(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Group not found"})
		return
	}
	members, err := h.storage.ListClientsInGroup(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	stagger := time.Duration(group.StaggerDelayMillis) * time.Millisecond
	dispatched := 0
	for _, c := range members {
		if !c.Enabled {
			continue
		}
		host, port, user, pass, insecure, ok := h.resolveRedfish(c)
		if !ok {
			continue
		}
		dispatched++
		go func(mac, host string, port int, user, pass string, insecure bool) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			client := redfish.New(host, port, user, pass, insecure)
			if err := client.SetPower(ctx, redfish.PowerAction(action)); err != nil {
				log.Printf("Redfish bulk %s on %s (%s) failed: %v", action, mac, host, err)
			} else {
				log.Printf("Redfish bulk %s on %s (%s) ok", action, mac, host)
			}
		}(c.MACAddress, host, port, user, pass, insecure)
		if stagger > 0 {
			time.Sleep(stagger)
		}
	}
	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: fmt.Sprintf("Power %s dispatched to %d member(s) of %s", action, dispatched, group.Name),
	})
}


func (h *Handler) ListScheduledTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	var tasks []*models.ScheduledTask
	var err error
	if gid := r.URL.Query().Get("group_id"); gid != "" {
		id, perr := strconv.ParseUint(gid, 10, 32)
		if perr != nil {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid group_id"})
			return
		}
		tasks, err = h.storage.ListScheduledTasksByGroup(uint(id))
	} else {
		tasks, err = h.storage.ListScheduledTasks()
	}
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: tasks})
}

func (h *Handler) CreateScheduledTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	var t models.ScheduledTask
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid body"})
		return
	}
	if t.Name == "" || t.CronExpr == "" || t.ActionType == "" || t.ClientGroupID == 0 {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "name, cron_expr, action_type, client_group_id are required"})
		return
	}
	if err := h.storage.CreateScheduledTask(&t); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	if h.SchedulerReload != nil {
		h.SchedulerReload()
	}
	log.Printf("Scheduled task created: %s (id=%d)", t.Name, t.ID)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: t})
}

func (h *Handler) UpdateScheduledTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid id"})
		return
	}
	var t models.ScheduledTask
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid body"})
		return
	}
	t.ID = uint(id)
	if err := h.storage.UpdateScheduledTask(uint(id), &t); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	if h.SchedulerReload != nil {
		h.SchedulerReload()
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: t})
}

func (h *Handler) DeleteScheduledTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid id"})
		return
	}
	if err := h.storage.DeleteScheduledTask(uint(id)); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	if h.SchedulerReload != nil {
		h.SchedulerReload()
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Scheduled task deleted"})
}

func (h *Handler) RunScheduledTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid id"})
		return
	}
	if h.SchedulerRunNow == nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Scheduler not wired"})
		return
	}
	if err := h.SchedulerRunNow(uint(id)); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Task dispatched"})
}

func (h *Handler) GetWebhookConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	cfg, err := h.storage.GetWebhookConfig()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: cfg})
}

func (h *Handler) UpdateWebhookConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	var cfg models.WebhookConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid body"})
		return
	}
	if err := h.storage.UpdateWebhookConfig(&cfg); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Webhook config saved", Data: cfg})
}

func (h *Handler) TestWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	cfg, err := h.storage.GetWebhookConfig()
	if err != nil || cfg == nil || cfg.URL == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "No webhook URL configured"})
		return
	}
	body, _ := json.Marshal(map[string]interface{}{
		"event":     "test",
		"timestamp": time.Now().UTC(),
		"message":   "Bootimus webhook test",
	})
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", cfg.URL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "bootimus-webhook/1 (test)")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		h.sendJSON(w, http.StatusOK, Response{Success: false, Error: err.Error()})
		return
	}
	defer resp.Body.Close()
	h.sendJSON(w, http.StatusOK, Response{
		Success: resp.StatusCode < 300,
		Message: fmt.Sprintf("Downstream HTTP %d", resp.StatusCode),
	})
}

func (h *Handler) ExportBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	dataDir := filepath.Clean(h.dataDir)
	if dataDir == "" {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Data directory not configured"})
		return
	}

	var dbBuf bytes.Buffer
	dbName, err := h.storage.Snapshot(&dbBuf)
	if err != nil {
		log.Printf("Backup snapshot failed: %v", err)
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Database snapshot failed: " + err.Error()})
		return
	}

	ts := time.Now().UTC().Format("20060102-150405")
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="bootimus-backup-%s.tar.gz"`, ts))

	gz := gzip.NewWriter(w)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	dbHdr := &tar.Header{
		Name:    dbName,
		Mode:    0o600,
		Size:    int64(dbBuf.Len()),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(dbHdr); err != nil {
		log.Printf("Backup export failed writing db header: %v", err)
		return
	}
	if _, err := io.Copy(tw, &dbBuf); err != nil {
		log.Printf("Backup export failed writing db body: %v", err)
		return
	}

	skipDirs := map[string]bool{
		"isos":  true,
		"tools": true,
	}
	skipFiles := map[string]bool{
		"bootimus.db":         true,
		"bootimus.db-wal":     true,
		"bootimus.db-shm":     true,
		"bootimus.db-journal": true,
	}

	err = filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dataDir, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		topLevel := rel
		if i := strings.Index(topLevel, string(os.PathSeparator)); i >= 0 {
			topLevel = topLevel[:i]
		}
		if skipDirs[topLevel] {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() && skipFiles[filepath.Base(rel)] {
			return nil
		}

		hdr, hdrErr := tar.FileInfoHeader(info, "")
		if hdrErr != nil {
			return hdrErr
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}
		f, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		defer f.Close()
		_, copyErr := io.Copy(tw, f)
		return copyErr
	})
	if err != nil {
		log.Printf("Backup export failed mid-stream: %v", err)
		return
	}
	log.Printf("Backup exported (%s) — db: %s (%d bytes)", ts, dbName, dbBuf.Len())
}

func (h *Handler) ImportClientsCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Upload too large or malformed"})
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "file field missing"})
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true
	header, err := reader.Read()
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "CSV header row missing"})
		return
	}
	idx := make(map[string]int, len(header))
	for i, name := range header {
		idx[strings.ToLower(strings.TrimSpace(name))] = i
	}
	if _, ok := idx["mac_address"]; !ok {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "CSV must include a mac_address column"})
		return
	}

	groupsByName := map[string]uint{}
	if groups, err := h.storage.ListClientGroups(); err == nil {
		for _, g := range groups {
			groupsByName[strings.ToLower(g.Name)] = g.ID
		}
	}

	get := func(row []string, key string) string {
		if i, ok := idx[key]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	toBool := func(s string) bool {
		switch strings.ToLower(s) {
		case "1", "true", "yes", "y", "on":
			return true
		}
		return false
	}

	var created, updated, skipped int
	var errors []string

	for rowNum := 2; ; rowNum++ {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errors = append(errors, fmt.Sprintf("row %d: %v", rowNum, err))
			continue
		}
		mac := strings.ToLower(strings.ReplaceAll(get(row, "mac_address"), "-", ":"))
		if mac == "" {
			skipped++
			continue
		}

		client := &models.Client{
			MACAddress:       mac,
			Name:             get(row, "name"),
			Description:      get(row, "description"),
			BootloaderSet:    get(row, "bootloader_set"),
			NextBootImage:    get(row, "next_boot_image"),
			Enabled:          true,
			ShowPublicImages: true,
		}
		if v := get(row, "enabled"); v != "" {
			client.Enabled = toBool(v)
		}
		if v := get(row, "show_public_images"); v != "" {
			client.ShowPublicImages = toBool(v)
		}
		if v := get(row, "static"); v != "" {
			client.Static = toBool(v)
		}
		if allowed := get(row, "allowed_images"); allowed != "" {
			var list []string
			for _, f := range strings.Split(allowed, "|") {
				f = strings.TrimSpace(f)
				if f != "" {
					list = append(list, f)
				}
			}
			client.AllowedImages = list
		}
		if groupName := get(row, "client_group"); groupName != "" {
			if id, ok := groupsByName[strings.ToLower(groupName)]; ok {
				idCopy := id
				client.ClientGroupID = &idCopy
			}
		}

		if _, err := h.storage.GetClient(mac); err == nil {
			if err := h.storage.UpdateClient(mac, client); err != nil {
				errors = append(errors, fmt.Sprintf("row %d (%s): update failed: %v", rowNum, mac, err))
				continue
			}
			updated++
		} else {
			if err := h.storage.CreateClient(client); err != nil {
				errors = append(errors, fmt.Sprintf("row %d (%s): create failed: %v", rowNum, mac, err))
				continue
			}
			created++
		}
	}

	log.Printf("Clients CSV import: %d created, %d updated, %d skipped, %d errors", created, updated, skipped, len(errors))
	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Data: map[string]interface{}{
			"created": created,
			"updated": updated,
			"skipped": skipped,
			"errors":  errors,
		},
	})
}


func (h *Handler) ListClientGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	groups, err := h.storage.ListClientGroups()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	type groupWithCount struct {
		*models.ClientGroup
		MemberCount int `json:"member_count"`
	}
	out := make([]groupWithCount, 0, len(groups))
	for _, g := range groups {
		members, _ := h.storage.ListClientsInGroup(g.ID)
		out = append(out, groupWithCount{ClientGroup: g, MemberCount: len(members)})
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: out})
}

func (h *Handler) GetClientGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid group ID"})
		return
	}
	group, err := h.storage.GetClientGroup(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Group not found"})
		return
	}
	members, _ := h.storage.ListClientsInGroup(group.ID)
	group.Clients = make([]models.Client, 0, len(members))
	for _, m := range members {
		group.Clients = append(group.Clients, *m)
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: group})
}

func (h *Handler) CreateClientGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	var group models.ClientGroup
	if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}
	if group.Name == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Group name is required"})
		return
	}
	if err := h.storage.CreateClientGroup(&group); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	log.Printf("Created client group: %s (ID: %d)", group.Name, group.ID)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Client group created", Data: group})
}

func (h *Handler) UpdateClientGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid group ID"})
		return
	}
	var group models.ClientGroup
	if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}
	group.ID = uint(id)
	type groupWithMembers struct {
		Members []string `json:"members"`
	}
	var withMembers groupWithMembers
	_ = withMembers

	if err := h.storage.UpdateClientGroup(uint(id), &group); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	log.Printf("Updated client group: %s (ID: %d)", group.Name, group.ID)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Client group updated", Data: group})
}

func (h *Handler) DeleteClientGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid group ID"})
		return
	}
	group, err := h.storage.GetClientGroup(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Group not found"})
		return
	}
	if err := h.storage.DeleteClientGroup(uint(id)); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	log.Printf("Deleted client group: %s (ID: %d)", group.Name, group.ID)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Client group deleted"})
}

func (h *Handler) SetClientGroupMembership(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	var req struct {
		MACAddress string `json:"mac_address"`
		GroupID    *uint  `json:"group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}
	if req.MACAddress == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "mac_address is required"})
		return
	}
	if err := h.storage.SetClientGroup(req.MACAddress, req.GroupID); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Client group membership updated"})
}

func (h *Handler) WakeClientGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid group ID"})
		return
	}
	group, err := h.storage.GetClientGroup(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Group not found"})
		return
	}
	members, err := h.storage.ListClientsInGroup(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	broadcastAddr := h.wolBroadcastAddr
	if group.WOLBroadcastAddr != "" {
		broadcastAddr = group.WOLBroadcastAddr
	}
	stagger := time.Duration(group.StaggerDelayMillis) * time.Millisecond
	sent := 0
	for _, c := range members {
		if !c.Enabled {
			continue
		}
		sent++
	}
	go func(macs []string, bcast string, gap time.Duration) {
		for i, mac := range macs {
			if i > 0 && gap > 0 {
				time.Sleep(gap)
			}
			if err := wol.SendMagicPacket(mac, bcast); err != nil {
				log.Printf("ClientGroup wake: failed for %s: %v", mac, err)
			} else {
				log.Printf("ClientGroup wake: sent to %s (broadcast: %s)", mac, bcast)
			}
		}
	}(collectEnabledMACs(members), broadcastAddr, stagger)

	log.Printf("Admin: Wake-on-LAN bulk sent to group %s (%d enabled members, stagger %s)",
		group.Name, sent, stagger)
	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: fmt.Sprintf("Wake-on-LAN sent to %d member(s) of %s", sent, group.Name),
	})
}

func (h *Handler) SetNextBootForClientGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid group ID"})
		return
	}
	var req struct {
		ImageFilename string `json:"image_filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}
	group, err := h.storage.GetClientGroup(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Group not found"})
		return
	}
	members, err := h.storage.ListClientsInGroup(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	applied := 0
	for _, c := range members {
		if !c.Enabled {
			continue
		}
		if req.ImageFilename == "" {
			if err := h.storage.ClearNextBootImage(c.MACAddress); err != nil {
				log.Printf("ClientGroup next-boot clear failed for %s: %v", c.MACAddress, err)
				continue
			}
		} else {
			if err := h.storage.SetNextBootImage(c.MACAddress, req.ImageFilename); err != nil {
				log.Printf("ClientGroup next-boot set failed for %s: %v", c.MACAddress, err)
				continue
			}
		}
		applied++
	}
	msg := fmt.Sprintf("Cleared next-boot for %d member(s) of %s", applied, group.Name)
	if req.ImageFilename != "" {
		msg = fmt.Sprintf("Set next-boot=%s for %d member(s) of %s", req.ImageFilename, applied, group.Name)
	}
	log.Printf("Admin: %s", msg)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: msg})
}

func collectEnabledMACs(clients []*models.Client) []string {
	out := make([]string, 0, len(clients))
	for _, c := range clients {
		if c.Enabled {
			out = append(out, c.MACAddress)
		}
	}
	return out
}

func (h *Handler) GetMenuTheme(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	theme, err := h.storage.GetMenuTheme()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: theme})
}

func (h *Handler) UpdateMenuTheme(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	var theme models.MenuTheme
	if err := json.NewDecoder(r.Body).Decode(&theme); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}
	if err := h.storage.UpdateMenuTheme(&theme); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	log.Printf("Admin: Updated menu theme settings")
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Theme updated", Data: theme})
}

func (h *Handler) ListUSBImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	entries, err := bootloaders.ListFiles(bootloaders.DefaultSet)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	var images []map[string]interface{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".usb") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		images = append(images, map[string]interface{}{
			"name": entry.Name(),
			"size": info.Size(),
		})
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: images})
}

func (h *Handler) DownloadUSBImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "name parameter required"})
		return
	}

	name = filepath.Base(name)
	if !strings.HasSuffix(name, ".usb") {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid file type"})
		return
	}

	data, _, err := bootloaders.Resolve(bootloaders.DefaultSet, name)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "USB image not found"})
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Write(data)
}

func (h *Handler) ListImageFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	filename := r.URL.Query().Get("filename")
	if filename == "" {
		log.Printf("ListImageFiles: missing filename parameter")
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "filename parameter required"})
		return
	}

	log.Printf("ListImageFiles: requested for image: %s", filename)

	baseDir := strings.TrimSuffix(filename, filepath.Ext(filename))
	bootDir := filepath.Join(h.isoDir, baseDir)

	log.Printf("ListImageFiles: boot directory: %s", bootDir)

	type FileInfo struct {
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
		Size  int64  `json:"size"`
	}

	var files []FileInfo

	if _, err := os.Stat(bootDir); err == nil {
		log.Printf("ListImageFiles: directory exists, walking files...")
		err := filepath.Walk(bootDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Printf("ListImageFiles: error accessing %s: %v", path, err)
				return nil
			}

			relPath, err := filepath.Rel(bootDir, path)
			if err != nil {
				log.Printf("ListImageFiles: error getting relative path for %s: %v", path, err)
				return nil
			}

			if relPath == "." {
				return nil
			}

			files = append(files, FileInfo{
				Path:  relPath,
				IsDir: info.IsDir(),
				Size:  info.Size(),
			})

			return nil
		})

		if err != nil {
			log.Printf("ListImageFiles: error walking directory %s: %v", bootDir, err)
		}

		log.Printf("ListImageFiles: found %d files/folders", len(files))
	} else {
		log.Printf("ListImageFiles: boot directory does not exist: %s (error: %v)", bootDir, err)
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: map[string]interface{}{"files": files}})
}

func (h *Handler) DeleteImageFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var req struct {
		Filename string `json:"filename"`
		BaseDir  string `json:"base_dir"`
		Path     string `json:"path"`
		IsDir    bool   `json:"is_dir"`
		IsIso    bool   `json:"is_iso"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("DeleteImageFile: invalid request body: %v", err)
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	log.Printf("DeleteImageFile: request - filename=%s, base_dir=%s, path=%s, is_dir=%v, is_iso=%v",
		req.Filename, req.BaseDir, req.Path, req.IsDir, req.IsIso)

	if req.Filename == "" {
		log.Printf("DeleteImageFile: missing filename")
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "filename is required"})
		return
	}

	if req.IsIso {
		isoPath := filepath.Join(h.isoDir, req.Filename)
		if _, err := os.Stat(isoPath); err != nil {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "ISO file not found"})
			return
		}

		if err := os.Remove(isoPath); err != nil {
			log.Printf("Error deleting ISO file %s: %v", isoPath, err)
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to delete ISO file"})
			return
		}

		log.Printf("Deleted ISO file: %s", isoPath)
		h.sendJSON(w, http.StatusOK, Response{Success: true})
		return
	}

	if req.Path != "" && !req.IsDir {
		bootDir := filepath.Join(h.isoDir, req.BaseDir)
		filePath := filepath.Join(bootDir, req.Path)

		cleanTarget, err := filepath.Abs(filePath)
		if err != nil {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid path"})
			return
		}
		cleanBootDir, _ := filepath.Abs(bootDir)
		if !strings.HasPrefix(cleanTarget, cleanBootDir) {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Path outside boot directory"})
			return
		}

		if _, err := os.Stat(filePath); err != nil {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "File not found"})
			return
		}

		if err := os.Remove(filePath); err != nil {
			log.Printf("Error deleting file %s: %v", filePath, err)
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to delete file"})
			return
		}

		log.Printf("Deleted file: %s", filePath)
		h.sendJSON(w, http.StatusOK, Response{Success: true})
		return
	}

	bootDir := filepath.Join(h.isoDir, req.BaseDir)
	if _, err := os.Stat(bootDir); err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Boot directory not found"})
		return
	}

	entries, err := os.ReadDir(bootDir)
	if err != nil {
		log.Printf("Error reading boot directory %s: %v", bootDir, err)
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to read boot directory"})
		return
	}

	for _, entry := range entries {
		if entry.Name() == "autoinstall" {
			log.Printf("Preserving autoinstall folder: %s/autoinstall", bootDir)
			continue
		}

		entryPath := filepath.Join(bootDir, entry.Name())
		if err := os.RemoveAll(entryPath); err != nil {
			log.Printf("Error deleting %s: %v", entryPath, err)
		} else {
			log.Printf("Deleted: %s", entryPath)
		}
	}

	log.Printf("Deleted boot directory contents (preserved autoinstall): %s", bootDir)

	image, err := h.storage.GetImage(req.Filename)
	if err == nil && image != nil {
		image.Extracted = false
		image.BootMethod = "sanboot"
		image.KernelPath = ""
		image.InitrdPath = ""
		image.SquashfsPath = ""
		image.Distro = ""
		image.NetbootAvailable = false
		image.NetbootRequired = false
		image.ExtractedAt = nil
		image.ExtractionError = ""

		if err := h.storage.UpdateImage(req.Filename, image); err != nil {
			log.Printf("Warning: Failed to reset image metadata after deleting boot folder: %v", err)
		} else {
			log.Printf("Reset image %s to sanboot mode after deleting boot folder", req.Filename)
		}
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true})
}
