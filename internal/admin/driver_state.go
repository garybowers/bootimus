package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"bootimus/internal/models"
	"bootimus/internal/wim"
)

type driverBuildState struct {
	Status     string    `json:"status"`
	Error      string    `json:"error,omitempty"`
	Revision   string    `json:"revision,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
	CanApply   bool      `json:"can_apply"`
	Reason     string    `json:"reason,omitempty"`
	SMBLoading bool      `json:"smb_loading"`
}

func (h *Handler) driverImage(id uint) (*models.Image, error) {
	images, err := h.storage.ListImages()
	if err != nil {
		return nil, err
	}
	for _, image := range images {
		if image.ID == id {
			if !models.IsWindowsDistro(image.Distro) {
				return nil, fmt.Errorf("select a Windows image")
			}
			if filepath.Base(image.Filename) != image.Filename {
				return nil, fmt.Errorf("invalid image filename")
			}
			return image, nil
		}
	}
	return nil, fmt.Errorf("image not found")
}

func driverRevision(packs []*models.DriverPack) string {
	ids := make([]uint, 0, len(packs))
	for _, pack := range packs {
		ids = append(ids, pack.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	sum := sha256.Sum256([]byte(fmt.Sprint(ids)))
	return hex.EncodeToString(sum[:])
}

func (h *Handler) driverStatePath(id uint) string {
	return filepath.Join(h.dataDir, "driver-builds", strconv.FormatUint(uint64(id), 10)+".json")
}

func (h *Handler) saveDriverState(id uint, state driverBuildState) error {
	filename := h.driverStatePath(id)
	if err := os.MkdirAll(filepath.Dir(filename), 0750); err != nil {
		return err
	}
	state.UpdatedAt = time.Now().UTC()
	content, err := json.Marshal(state)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".driver-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err = temporary.Write(content); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporary.Name(), filename)
}

func (h *Handler) driverBuildRunning(id uint) bool {
	h.driverBuildsMu.Lock()
	defer h.driverBuildsMu.Unlock()
	return h.driverBuilds[id]
}

func (h *Handler) reserveDriverBuild(id uint) bool {
	h.driverBuildsMu.Lock()
	defer h.driverBuildsMu.Unlock()
	if h.driverBuilds[id] {
		return false
	}
	if h.driverBuilds == nil {
		h.driverBuilds = make(map[uint]bool)
	}
	h.driverBuilds[id] = true
	return true
}

func (h *Handler) releaseDriverBuild(id uint) {
	h.driverBuildsMu.Lock()
	delete(h.driverBuilds, id)
	h.driverBuildsMu.Unlock()
}

func (h *Handler) driverBuildReadiness(image *models.Image) string {
	if !image.Extracted {
		return "Extract this Windows image before applying drivers."
	}
	if h.extractedBootWimPath(image.Filename) == "" {
		return "Driver management needs an extracted sources/boot.wim. Extract the image first; custom WIM layouts are not supported."
	}
	if !wim.IsAvailable() {
		return "Install wimtools on the server before applying drivers."
	}
	if image.SMBInstallEnabled {
		if h.smbManager == nil || h.smbManager.Err() != nil {
			return "Start the Windows SMB service before rebuilding this patched image."
		}
		if _, err := os.Stat(h.extractedBootWimPath(image.Filename) + ".orig"); err != nil {
			return "Re-extract this image to create a clean boot.wim backup before applying drivers."
		}
	}
	return ""
}

func (h *Handler) driverBackupAvailable(image *models.Image) bool {
	filename := h.extractedBootWimPath(image.Filename)
	if filename == "" {
		return false
	}
	if _, err := os.Stat(filename + ".orig"); err == nil {
		return true
	}
	if !image.SMBInstallEnabled {
		if _, err := os.Stat(filename + ".backup"); err == nil {
			return true
		}
	}
	return false
}

func (h *Handler) DriverBuildStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseUint(r.URL.Query().Get("imageId"), 10, 32)
	if err != nil || id == 0 {
		h.sendJSON(w, http.StatusBadRequest, Response{Error: "Invalid image ID"})
		return
	}
	image, err := h.driverImage(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Error: err.Error()})
		return
	}
	packs, err := h.storage.ListDriverPacksByImage(image.ID)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Error: err.Error()})
		return
	}
	state := driverBuildState{Status: "idle"}
	content, err := os.ReadFile(h.driverStatePath(image.ID))
	if err == nil {
		err = json.Unmarshal(content, &state)
	}
	if err != nil && !os.IsNotExist(err) {
		h.sendJSON(w, http.StatusInternalServerError, Response{Error: "Cannot read driver build status"})
		return
	}
	revision := driverRevision(packs)
	if (state.Revision != "" && state.Revision != revision) || (state.Revision == "" && len(packs) > 0) {
		state.Status, state.Error = "pending", ""
	}
	if h.driverBuildRunning(image.ID) {
		state.Status, state.Error = "applying", ""
	} else if state.Status == "applying" {
		state.Status, state.Error = "failed", "The previous rebuild was interrupted. Apply the changes again."
	}
	state.Reason = h.driverBuildReadiness(image)
	state.CanApply = state.Reason == "" && state.Status != "applying"
	state.SMBLoading = image.SMBInstallEnabled
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: state})
}
