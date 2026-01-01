package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"bootimus/internal/extractor"
	"bootimus/internal/models"
	"bootimus/internal/storage"
	"bootimus/internal/sysstats"
)

type Handler struct {
	storage storage.Storage // Unified storage interface (PostgreSQL or SQLite)
	dataDir string          // Base data directory (/data) - for SQLite database
	isoDir  string          // ISO directory (/data/isos) - for ISO files
	bootDir string
	version string
}

func NewHandler(store storage.Storage, dataDir string, isoDir string, bootDir string, version string) *Handler {
	return &Handler{
		storage: store,
		dataDir: dataDir,
		isoDir:  isoDir,
		bootDir: bootDir,
		version: version,
	}
}

// isRunningInDocker detects if the application is running inside a Docker container
func isRunningInDocker() bool {
	// Check for /.dockerenv file (most reliable)
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// Check /proc/1/cgroup for docker or containerd
	data, err := os.ReadFile("/proc/1/cgroup")
	if err == nil {
		content := string(data)
		if strings.Contains(content, "docker") || strings.Contains(content, "containerd") {
			return true
		}
	}

	// Check if running as PID 1 with limited process count (common in containers)
	if os.Getpid() == 1 {
		// Additional check: containers typically have very few processes
		entries, err := os.ReadDir("/proc")
		if err == nil && len(entries) < 50 {
			return true
		}
	}

	return false
}

// Response helpers
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

	// Log request result
	if !resp.Success {
		log.Printf("Admin API error (status %d): %s", status, resp.Error)
	}
}

// ============================================================================
// Client Management
// ============================================================================

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
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing mac parameter"})
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

	// Normalise MAC address
	client.MACAddress = strings.ToLower(strings.ReplaceAll(client.MACAddress, "-", ":"))

	// Set default enabled state
	client.Enabled = true

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

	var updates models.Client
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	client, err := h.storage.GetClient(mac)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Client not found"})
		return
	}

	// Update fields
	if updates.Name != "" {
		client.Name = updates.Name
	}
	if updates.Description != "" {
		client.Description = updates.Description
	}
	client.Enabled = updates.Enabled

	if err := h.storage.UpdateClient(mac, client); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Client updated - MAC: %s, Name: %s, Enabled: %v", client.MACAddress, client.Name, client.Enabled)
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

// ============================================================================
// Image Management
// ============================================================================

// syncFilesystemToDatabase ensures all ISO files on disk are represented in the database
func (h *Handler) syncFilesystemToDatabase() {
	entries, err := os.ReadDir(h.isoDir)
	if err != nil {
		log.Printf("Failed to read ISO directory for sync: %v", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .iso files
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".iso") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			log.Printf("Failed to get file info for %s: %v", entry.Name(), err)
			continue
		}

		// Check if image exists in database
		_, err = h.storage.GetImage(entry.Name())
		exists := (err == nil)

		// If not in database, add it
		if !exists {
			displayName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			image := &models.Image{
				Name:     displayName,
				Filename: entry.Name(),
				Size:     info.Size(),
				Enabled:  true,
				Public:   true,
			}

			if err := h.storage.CreateImage(image); err != nil {
				log.Printf("Failed to auto-add image from filesystem: %s - %v", entry.Name(), err)
			} else {
				log.Printf("Auto-added image from filesystem: %s", entry.Name())
			}
		}
	}
}

func (h *Handler) ListImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	// Sync filesystem to database before listing
	h.syncFilesystemToDatabase()

	images, err := h.storage.ListImages()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
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

	// Decode into a map to detect which fields are actually present
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

	// Update only the fields that are present in the request
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

	// Delete from filesystem if requested
	if deleteFile {
		filePath := filepath.Join(h.isoDir, filename)
		if err := os.Remove(filePath); err != nil {
			log.Printf("Failed to delete file %s: %v", filePath, err)
		} else {
			log.Printf("Deleted ISO file: %s", filename)
		}

		// Also clean up extracted kernel directory if it exists
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

	// Delete from database
	if err := h.storage.DeleteImage(filename); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Image deleted - %s", filename)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Image deleted"})
}

func (h *Handler) UploadImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	// Use streaming multipart reader - don't load entire file into memory
	// Only allocate 32MB for form fields, files are streamed directly
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		log.Printf("Failed to parse upload form: %v", err)
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Failed to parse form"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		log.Printf("No file provided in upload request: %v", err)
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "No file provided"})
		return
	}
	defer file.Close()

	// Validate file extension
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".iso") {
		log.Printf("Upload rejected: invalid file type: %s", header.Filename)
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Only .iso files are allowed"})
		return
	}

	// Log initial memory state
	var startMem runtime.MemStats
	runtime.ReadMemStats(&startMem)
	log.Printf("Starting ISO upload: %s (size: %d bytes) - Memory: %d MB allocated",
		header.Filename, header.Size, startMem.Alloc/1024/1024)

	// Check if file already exists on filesystem (filesystem is source of truth)
	filePath := filepath.Join(h.isoDir, header.Filename)
	if _, err := os.Stat(filePath); err == nil {
		log.Printf("Upload rejected: file already exists on filesystem: %s", header.Filename)
		h.sendJSON(w, http.StatusConflict, Response{Success: false, Error: "An image with this filename already exists"})
		return
	}

	// Save file
	dst, err := os.Create(filePath)
	if err != nil {
		log.Printf("Failed to create file %s: %v", filePath, err)
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to create file"})
		return
	}
	defer dst.Close()

	// Copy file with progress logging
	buf := make([]byte, 32*1024*1024) // 32MB buffer for faster copying
	var written int64
	lastLog := int64(0)
	logInterval := int64(100 * 1024 * 1024) // Log every 100MB

	for {
		nr, er := file.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}

			// Log progress every 100MB
			if written-lastLog >= logInterval {
				log.Printf("Upload progress: %s - %d MB written", header.Filename, written/(1024*1024))
				lastLog = written
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}

	if err != nil {
		os.Remove(filePath)
		log.Printf("Failed to save file %s: %v", header.Filename, err)
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to save file"})
		return
	}

	size := written

	// Log final memory state
	var endMem runtime.MemStats
	runtime.ReadMemStats(&endMem)
	runtime.GC() // Force garbage collection to free upload buffers
	var afterGC runtime.MemStats
	runtime.ReadMemStats(&afterGC)

	log.Printf("Upload complete: %s (%d MB)", header.Filename, size/(1024*1024))
	log.Printf("Memory usage - Start: %d MB, End: %d MB, After GC: %d MB",
		startMem.Alloc/1024/1024, endMem.Alloc/1024/1024, afterGC.Alloc/1024/1024)

	// Check if database record already exists (file may have been deleted but DB record remains)
	existingImage, err := h.storage.GetImage(header.Filename)
	if err == nil && existingImage != nil {
		// Update existing record
		existingImage.Size = size
		existingImage.Enabled = true
		// Preserve existing settings unless explicitly overridden
		publicValue := r.FormValue("public")
		if publicValue == "on" || publicValue == "true" || publicValue == "false" {
			existingImage.Public = publicValue == "on" || publicValue == "true"
		}
		if r.FormValue("description") != "" {
			existingImage.Description = r.FormValue("description")
		}

		if err := h.storage.UpdateImage(header.Filename, existingImage); err != nil {
			os.Remove(filePath)
			log.Printf("Failed to update image record, file removed: %s - %v", header.Filename, err)
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to update image record"})
			return
		}

		log.Printf("Admin: Image re-uploaded and database updated - %s (%d MB)", existingImage.Filename, existingImage.Size/1024/1024)
		h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Image re-uploaded successfully", Data: existingImage})
		return
	}

	// Create new entry
	displayName := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	publicValue := r.FormValue("public")
	isPublic := publicValue == "on" || publicValue == "true"

	image := models.Image{
		Name:     displayName,
		Filename: header.Filename,
		Size:     size,
		Enabled:  true,
		Public:   isPublic,
	}

	if r.FormValue("description") != "" {
		image.Description = r.FormValue("description")
	}

	if err := h.storage.CreateImage(&image); err != nil {
		// Clean up uploaded file on database error
		os.Remove(filePath)
		log.Printf("Failed to create image record, file removed: %s - %v", header.Filename, err)
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to create image record"})
		return
	}

	log.Printf("Admin: Image uploaded successfully - %s (%d MB)", image.Filename, image.Size/1024/1024)
	h.sendJSON(w, http.StatusCreated, Response{Success: true, Message: "Image uploaded", Data: image})
}

// ============================================================================
// Client-Image Association
// ============================================================================

func (h *Handler) AssignImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var req struct {
		MACAddress      string   `json:"mac_address"`
		ImageFilenames  []string `json:"image_filenames"`
		ClientID        uint     `json:"client_id"` // For DB mode
		ImageIDs        []uint   `json:"image_ids"` // For DB mode
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

// ============================================================================
// ISO Kernel/Initrd Extraction
// ============================================================================

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

	// Get the image from database
	image, err := h.storage.GetImage(filename)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
		return
	}

	// Check if already extracted
	if image.Extracted && image.BootMethod == "kernel" {
		h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Image already extracted", Data: image})
		return
	}

	// Import extractor package
	log.Printf("Admin: Starting kernel/initrd extraction - %s", filename)

	// Create extractor
	ext, err := extractor.New(h.isoDir)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: fmt.Sprintf("Failed to create extractor: %v", err)})
		return
	}

	// Extract boot files
	isoPath := filepath.Join(h.isoDir, filename)
	bootFiles, err := ext.Extract(isoPath)
	if err != nil {
		// Save error to database
		image.ExtractionError = err.Error()
		h.storage.UpdateImage(filename, image)

		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to extract boot files: %v", err),
		})
		return
	}

	// Save metadata
	if err := ext.SaveMetadata(filename, bootFiles); err != nil {
		log.Printf("Failed to save extraction metadata: %v", err)
	}

	// Check sanboot compatibility and generate hints
	sanbootCompatible, sanbootHint := checkSanbootCompatibility(bootFiles.Distro, image.Filename)

	// Update database with extraction info
	now := time.Now()
	image.Extracted = true
	image.Distro = bootFiles.Distro
	image.BootMethod = "kernel"
	image.KernelPath = bootFiles.Kernel
	image.InitrdPath = bootFiles.Initrd
	image.BootParams = bootFiles.BootParams + " "
	image.SquashfsPath = bootFiles.SquashfsPath
	image.ExtractionError = ""
	image.ExtractedAt = &now
	image.SanbootCompatible = sanbootCompatible
	image.SanbootHint = sanbootHint
	image.NetbootRequired = bootFiles.NetbootRequired
	image.NetbootURL = bootFiles.NetbootURL
	image.NetbootAvailable = false // Not yet downloaded

	log.Printf("Setting boot_method to 'kernel' for image ID=%d, filename=%s", image.ID, image.Filename)

	if err := h.storage.UpdateImage(filename, image); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Image extraction completed - %s (distro: %s, kernel: %s, initrd: %s)",
		filename, bootFiles.Distro, bootFiles.Kernel, bootFiles.Initrd)

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: fmt.Sprintf("Successfully extracted %s boot files", bootFiles.Distro),
		Data:    image,
	})
}

// checkSanbootCompatibility determines if an ISO is compatible with sanboot and provides hints
func checkSanbootCompatibility(distro, filename string) (bool, string) {
	filenameLower := strings.ToLower(filename)

	// Windows PE and diagnostic ISOs are sanboot compatible
	if strings.Contains(filenameLower, "winpe") ||
	   strings.Contains(filenameLower, "windows pe") ||
	   strings.Contains(filenameLower, "memtest") ||
	   strings.Contains(filenameLower, "gparted") && strings.Contains(filenameLower, "live") {
		return true, ""
	}

	// Most Linux distributions and Windows are NOT sanboot compatible
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

	// If distro detected but not in our list, assume incompatible for safety
	if distro != "" {
		return false, "This Linux distribution likely requires kernel extraction. Use 'Extract Kernel/Initrd' for reliable network boot support."
	}

	// Unknown ISO type - allow sanboot but warn
	return true, ""
}

func (h *Handler) SetBootMethod(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var req struct {
		Filename   string `json:"filename"`
		BootMethod string `json:"boot_method"` // "sanboot" or "kernel"
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	if req.Filename == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing filename"})
		return
	}

	if req.BootMethod != "sanboot" && req.BootMethod != "kernel" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid boot method (must be 'sanboot' or 'kernel')"})
		return
	}

	// Get the image
	image, err := h.storage.GetImage(req.Filename)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
		return
	}

	// If switching to kernel method, ensure extraction has been done
	if req.BootMethod == "kernel" && !image.Extracted {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Cannot use kernel boot method: image not extracted. Please extract first.",
		})
		return
	}

	// If switching to sanboot, check compatibility and warn user
	if req.BootMethod == "sanboot" && !image.SanbootCompatible {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   fmt.Sprintf("Sanboot not recommended for this ISO. %s", image.SanbootHint),
		})
		return
	}

	// Update boot method
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

// ============================================================================
// Statistics and Logs
// ============================================================================

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

	logs, err := h.storage.GetBootLogs(limit)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Boot logs retrieved: %d entries", len(logs))
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: logs})
}

// ============================================================================
// System Operations
// ============================================================================

func (h *Handler) ScanImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	entries, err := os.ReadDir(h.isoDir)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	// Build map of existing ISO files
	existingFiles := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".iso") {
			existingFiles[entry.Name()] = true
		}
	}

	var newImages []string
	var deletedImages []string

	// Add new images and update existing ones
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".iso") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Check if already exists
		existing, err := h.storage.GetImage(entry.Name())
		if err != nil { // Not found, create new
			displayName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			image := &models.Image{
				Name:     displayName,
				Filename: entry.Name(),
				Size:     info.Size(),
				Enabled:  true,
				Public:   true,
			}

			if err := h.storage.CreateImage(image); err == nil {
				newImages = append(newImages, entry.Name())
				log.Printf("Admin: Image scan found new ISO - %s (%d MB)", entry.Name(), info.Size()/1024/1024)
			} else {
				log.Printf("Failed to add image to database: %s - %v", entry.Name(), err)
			}
		} else {
			// Image exists, update size if changed
			if existing.Size != info.Size() {
				oldSize := existing.Size
				existing.Size = info.Size()
				if err := h.storage.UpdateImage(existing.Filename, existing); err == nil {
					log.Printf("Updated image size: %s (%d -> %d bytes)", existing.Filename, oldSize, info.Size())
				}
			}
		}
	}

	// Remove images that no longer exist on disk
	allImages, err := h.storage.ListImages()
	if err == nil {
		log.Printf("Checking %d database images against %d filesystem ISOs", len(allImages), len(existingFiles))
		for _, image := range allImages {
			if !existingFiles[image.Filename] {
				// ISO file no longer exists, delete from database
				log.Printf("Deleting missing image from database: %s (ID: %d)", image.Filename, image.ID)
				if err := h.storage.DeleteImage(image.Filename); err == nil {
					deletedImages = append(deletedImages, image.Filename)
					log.Printf("Successfully removed missing image from database: %s", image.Filename)

					// Also clean up extracted boot files directory if it exists
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

// ============================================================================
// Bootloader Management
// ============================================================================

type Bootloader struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func (h *Handler) ListBootloaders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	var bootloaders []Bootloader

	// Check if boot directory exists
	if h.bootDir == "" {
		h.sendJSON(w, http.StatusOK, Response{
			Success: true,
			Message: "No boot directory configured (using embedded bootloaders)",
			Data:    bootloaders,
		})
		return
	}

	// Create boot directory if it doesn't exist
	if err := os.MkdirAll(h.bootDir, 0755); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to create boot directory: %v", err),
		})
		return
	}

	entries, err := os.ReadDir(h.bootDir)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to read boot directory: %v", err),
		})
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		bootloaders = append(bootloaders, Bootloader{
			Name: entry.Name(),
			Size: info.Size(),
		})
	}

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Data:    bootloaders,
	})
}

func (h *Handler) UploadBootloader(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	// Check if boot directory is configured
	if h.bootDir == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Boot directory not configured. Set boot_dir in config to enable custom bootloader uploads.",
		})
		return
	}

	// Create boot directory if it doesn't exist
	if err := os.MkdirAll(h.bootDir, 0755); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to create boot directory: %v", err),
		})
		return
	}

	// Parse multipart form (max 10MB for bootloaders)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
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

	// Validate file name
	filename := filepath.Base(header.Filename)
	if filename == "" || filename == "." || filename == ".." {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Invalid filename",
		})
		return
	}

	// Create destination file
	destPath := filepath.Join(h.bootDir, filename)
	dest, err := os.Create(destPath)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to create file: %v", err),
		})
		return
	}
	defer dest.Close()

	// Copy file
	written, err := io.Copy(dest, file)
	if err != nil {
		os.Remove(destPath)
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to write file: %v", err),
		})
		return
	}

	log.Printf("Uploaded bootloader: %s (%d bytes)", filename, written)

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: fmt.Sprintf("Bootloader uploaded successfully: %s (%d bytes)", filename, written),
		Data: Bootloader{
			Name: filename,
			Size: written,
		},
	})
}

func (h *Handler) DeleteBootloader(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	if h.bootDir == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Boot directory not configured",
		})
		return
	}

	filename := r.URL.Query().Get("name")
	if filename == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "No filename provided",
		})
		return
	}

	// Validate filename
	filename = filepath.Base(filename)
	if filename == "" || filename == "." || filename == ".." {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Invalid filename",
		})
		return
	}

	filePath := filepath.Join(h.bootDir, filename)
	if err := os.Remove(filePath); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to delete bootloader: %v", err),
		})
		return
	}

	log.Printf("Deleted bootloader: %s", filename)

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: fmt.Sprintf("Bootloader deleted: %s", filename),
	})
}

// ============================================================================
// Server Information
// ============================================================================

func (h *Handler) GetServerInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	// Get system statistics
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
			"database_mode":  func() string {
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
		},
		"environment": map[string]string{
			"BOOTIMUS_TFTP_PORT":   os.Getenv("BOOTIMUS_TFTP_PORT"),
			"BOOTIMUS_HTTP_PORT":   os.Getenv("BOOTIMUS_HTTP_PORT"),
			"BOOTIMUS_ADMIN_PORT":  os.Getenv("BOOTIMUS_ADMIN_PORT"),
			"BOOTIMUS_DATA_DIR":    os.Getenv("BOOTIMUS_DATA_DIR"),
			"BOOTIMUS_DB_HOST":     os.Getenv("BOOTIMUS_DB_HOST"),
			"BOOTIMUS_DB_PORT":     os.Getenv("BOOTIMUS_DB_PORT"),
			"BOOTIMUS_DB_USER":     os.Getenv("BOOTIMUS_DB_USER"),
			"BOOTIMUS_DB_NAME":     os.Getenv("BOOTIMUS_DB_NAME"),
			"BOOTIMUS_DB_SSLMODE":  os.Getenv("BOOTIMUS_DB_SSLMODE"),
			"BOOTIMUS_DB_DISABLE":  os.Getenv("BOOTIMUS_DB_DISABLE"),
			"BOOTIMUS_SERVER_ADDR": os.Getenv("BOOTIMUS_SERVER_ADDR"),
		},
		"system_stats": sysStats,
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: info})
}

// User Management Endpoints

// ListUsers returns all users
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.storage.ListUsers()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: users})
}

// CreateUser creates a new user
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

	// Check if user exists
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

// UpdateUser updates an existing user
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Username required"})
		return
	}

	// Decode into a map to detect which fields are actually present
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

	// Update only the fields that are present
	if enabled, ok := updates["enabled"].(bool); ok {
		user.Enabled = enabled
	}
	if isAdmin, ok := updates["is_admin"].(bool); ok {
		user.IsAdmin = isAdmin
	}

	if err := h.storage.UpdateUser(username, user); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("User updated: %s (admin=%v, enabled=%v)", user.Username, user.IsAdmin, user.Enabled)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "User updated", Data: user})
}

// DeleteUser deletes a user
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Username required"})
		return
	}

	// Prevent deleting the admin user
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

// ResetUserPassword resets a user's password
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

// ISO Download Management

type DownloadProgress struct {
	URL          string  `json:"url"`
	Filename     string  `json:"filename"`
	TotalBytes   int64   `json:"total_bytes"`
	DownloadedBytes int64   `json:"downloaded_bytes"`
	Percentage   float64 `json:"percentage"`
	Speed        string  `json:"speed"`
	Status       string  `json:"status"` // "downloading", "completed", "error"
	Error        string  `json:"error,omitempty"`
	StartTime    time.Time `json:"start_time"`
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

// DownloadISO downloads an ISO from a URL
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

	// Extract filename from URL
	filename := filepath.Base(req.URL)
	if !strings.HasSuffix(strings.ToLower(filename), ".iso") {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "URL must point to an .iso file"})
		return
	}

	// Check if file already exists
	destPath := filepath.Join(h.isoDir, filename)
	if _, err := os.Stat(destPath); err == nil {
		h.sendJSON(w, http.StatusConflict, Response{Success: false, Error: "File already exists"})
		return
	}

	// Start download in background
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

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 0, // No timeout for large downloads
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

	// Add to download manager
	totalBytes := resp.ContentLength
	downloadMgr.Add(url, filename, totalBytes)

	// Create destination file
	out, err := os.Create(destPath)
	if err != nil {
		log.Printf("Failed to create file %s: %v", destPath, err)
		downloadMgr.Error(filename, err.Error())
		return
	}
	defer out.Close()

	// Download with progress tracking
	buffer := make([]byte, 32*1024) // 32KB buffer
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

	// Sync to database if available
	if h.storage != nil {
		isoFiles := []struct{ Name, Filename string; Size int64 }{
			{Name: strings.TrimSuffix(filename, filepath.Ext(filename)), Filename: filename, Size: downloaded},
		}

		if err := h.storage.SyncImages(isoFiles); err != nil {
			log.Printf("Failed to sync downloaded ISO to database: %v", err)
		}
	}
}

// GetDownloadProgress returns the progress of a specific download
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

// ListDownloads returns all active/recent downloads
func (h *Handler) ListDownloads(w http.ResponseWriter, r *http.Request) {
	downloads := downloadMgr.GetAll()
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: downloads})
}

// ============================================================================
// Auto-Install Script Management
// ============================================================================

// GetAutoInstallScript returns the auto-install script for an image
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
			"script":        image.AutoInstallScript,
			"enabled":       image.AutoInstallEnabled,
			"script_type":   image.AutoInstallScriptType,
		},
	})
}

// UpdateAutoInstallScript updates the auto-install script for an image
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
		ScriptType string `json:"script_type"` // "preseed", "kickstart", "autounattend"
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid request body"})
		return
	}

	// Validate script type
	validTypes := map[string]bool{
		"preseed":      true,
		"kickstart":    true,
		"autounattend": true,
		"autoinstall":  true, // Ubuntu autoinstall (cloud-init)
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

// ============================================================================
// Custom File Management
// ============================================================================

// ListCustomFiles lists all custom files
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

// GetCustomFile gets a single custom file by ID
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

// UploadCustomFile handles custom file uploads
func (h *Handler) UploadCustomFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	// Parse multipart form (max 100MB for custom files)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
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

	// Get form parameters
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

	// Validate filename
	originalFilename := filepath.Base(header.Filename)
	if originalFilename == "" || originalFilename == "." || originalFilename == ".." {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Invalid filename",
		})
		return
	}

	// Clean filename for storage (use original name as stored filename)
	cleanFilename := filepath.Clean(originalFilename)

	// Determine storage path based on public/image-specific
	var destDir string
	if isPublic {
		// Public files go in /data/files/
		destDir = filepath.Join(h.dataDir, "files")
	} else if imageID != nil {
		// Image-specific files go in /data/isos/{image-name}/files/
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

		destDir = filepath.Join(h.isoDir, imageName, "files")
	} else {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "File must be either public or assigned to an image",
		})
		return
	}

	// Create destination directory if it doesn't exist
	if err := os.MkdirAll(destDir, 0755); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to create directory: %v", err),
		})
		return
	}

	// Create destination file
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

	// Copy file
	written, err := io.Copy(dest, file)
	if err != nil {
		os.Remove(destPath)
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to write file: %v", err),
		})
		return
	}

	// Detect content type
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Create database record
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

	if err = h.storage.CreateCustomFile(customFile); err != nil {
		// Clean up file if database insert fails
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

// UpdateCustomFile updates custom file metadata
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

	// Get existing file
	file, err := h.storage.GetCustomFileByID(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "File not found"})
		return
	}

	// Update allowed fields
	if desc, ok := updates["description"].(string); ok {
		file.Description = desc
	}

	// Note: Changing public/imageID requires moving the file, which we'll implement later
	// For now, just update the description

	if err = h.storage.UpdateCustomFile(uint(id), file); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Updated custom file: %s (ID: %d)", file.Filename, file.ID)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "File updated", Data: file})
}

// DeleteCustomFile deletes a custom file
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

	// Get file to determine path before deleting
	file, err := h.storage.GetCustomFileByID(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "File not found"})
		return
	}

	// Determine file path
	var filePath string
	if file.Public {
		filePath = filepath.Join(h.dataDir, "files", file.Filename)
	} else if file.ImageID != nil && file.Image != nil {
		imageName := strings.TrimSuffix(file.Image.Filename, filepath.Ext(file.Image.Filename))
		filePath = filepath.Join(h.isoDir, imageName, "files", file.Filename)
	}

	// Delete from database first
	if err = h.storage.DeleteCustomFile(uint(id)); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	// Delete physical file
	if filePath != "" {
		if err := os.Remove(filePath); err != nil {
			log.Printf("Warning: Failed to delete file %s: %v", filePath, err)
		}
	}

	log.Printf("Deleted custom file: %s (ID: %d)", file.Filename, file.ID)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "File deleted"})
}

