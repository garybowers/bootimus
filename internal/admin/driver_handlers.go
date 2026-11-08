package admin

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"bootimus/internal/models"
)

func (h *Handler) UploadDriverPack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxDriverUpload+(1<<20))
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Error: "Cannot read upload; the ZIP limit is 500 MiB"})
		return
	}
	defer r.MultipartForm.RemoveAll()
	id, err := strconv.ParseUint(r.FormValue("imageId"), 10, 32)
	if err != nil || id == 0 {
		h.sendJSON(w, http.StatusBadRequest, Response{Error: "Invalid image ID"})
		return
	}
	image, err := h.driverImage(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Error: err.Error()})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Error: "Select a ZIP driver pack"})
		return
	}
	defer file.Close()
	name := filepath.Base(strings.ReplaceAll(header.Filename, `\`, "/"))
	if !strings.EqualFold(filepath.Ext(name), ".zip") {
		h.sendJSON(w, http.StatusBadRequest, Response{Error: "Upload a ZIP containing the INF and its supporting files"})
		return
	}
	mu := h.bootWimLock(image.Filename)
	if !mu.TryLock() {
		h.sendJSON(w, http.StatusConflict, Response{Error: "Another operation is changing this image; try again when it finishes"})
		return
	}
	defer mu.Unlock()
	if h.driverBuildRunning(image.ID) {
		h.sendJSON(w, http.StatusConflict, Response{Error: "Wait for the driver rebuild to finish"})
		return
	}
	directory := filepath.Join(h.isoDir, strings.TrimSuffix(image.Filename, filepath.Ext(image.Filename)), "drivers")
	if err := os.MkdirAll(directory, 0755); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Error: err.Error()})
		return
	}
	output, err := os.CreateTemp(directory, "pack-*.zip")
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Error: err.Error()})
		return
	}
	keep := false
	defer func() {
		output.Close()
		if !keep {
			os.Remove(output.Name())
		}
	}()
	checksum := sha256.New()
	written, err := io.Copy(io.MultiWriter(output, checksum), io.LimitReader(file, maxDriverUpload+1))
	if err != nil || written > maxDriverUpload {
		h.sendJSON(w, http.StatusBadRequest, Response{Error: "Cannot save driver pack; the ZIP limit is 500 MiB"})
		return
	}
	if err := output.Close(); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Error: err.Error()})
		return
	}
	if err := unpackDriverArchive(output.Name(), ""); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Error: err.Error()})
		return
	}
	packs, err := h.storage.ListDriverPacksByImage(image.ID)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Error: err.Error()})
		return
	}
	for _, existing := range packs {
		if filepath.Base(existing.Filename) != existing.Filename {
			h.sendJSON(w, http.StatusInternalServerError, Response{Error: "Invalid stored driver pack filename"})
			return
		}
		matches, err := driverArchiveMatches(filepath.Join(directory, existing.Filename), written, checksum.Sum(nil))
		if err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Error: fmt.Sprintf("Cannot check existing driver pack %q: %v", existing.OriginalName, err)})
			return
		}
		if matches {
			h.sendJSON(w, http.StatusConflict, Response{Error: fmt.Sprintf("This ZIP is already uploaded to this image as %q", existing.OriginalName)})
			return
		}
	}
	if err := h.saveDriverState(image.ID, driverBuildState{Status: "pending"}); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Error: err.Error()})
		return
	}
	pack := &models.DriverPack{Filename: filepath.Base(output.Name()), OriginalName: name, Description: r.FormValue("description"), ImageID: image.ID, Size: written, Enabled: true}
	if err := h.storage.CreateDriverPack(pack); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Error: err.Error()})
		return
	}
	keep = true
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Driver pack uploaded. Apply changes to update boot.wim.", Data: pack})
}

func (h *Handler) DeleteDriverPack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
	if err != nil || id == 0 {
		h.sendJSON(w, http.StatusBadRequest, Response{Error: "Invalid driver pack ID"})
		return
	}
	pack, err := h.storage.GetDriverPack(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Error: "Driver pack not found"})
		return
	}
	image, err := h.driverImage(pack.ImageID)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Error: err.Error()})
		return
	}
	mu := h.bootWimLock(image.Filename)
	if !mu.TryLock() {
		h.sendJSON(w, http.StatusConflict, Response{Error: "Another operation is changing this image; try again when it finishes"})
		return
	}
	defer mu.Unlock()
	if h.driverBuildRunning(image.ID) {
		h.sendJSON(w, http.StatusConflict, Response{Error: "Wait for the driver rebuild to finish"})
		return
	}
	if pack.LastApplied != nil && !h.driverBackupAvailable(image) {
		h.sendJSON(w, http.StatusPreconditionFailed, Response{Error: "Re-extract this image before removing applied drivers: its original boot.wim backup is missing"})
		return
	}
	if err := h.saveDriverState(image.ID, driverBuildState{Status: "pending"}); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Error: err.Error()})
		return
	}
	if err := h.storage.DeleteDriverPack(pack.ID); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Error: err.Error()})
		return
	}
	// Legacy uploads could share a filename. Do not remove a still-referenced ZIP.
	packs, err := h.storage.ListDriverPacks()
	if err == nil && filepath.Base(pack.Filename) == pack.Filename {
		referenced := false
		for _, other := range packs {
			if other.ImageID == image.ID && other.Filename == pack.Filename {
				referenced = true
			}
		}
		if !referenced {
			filename := filepath.Join(h.isoDir, strings.TrimSuffix(image.Filename, filepath.Ext(image.Filename)), "drivers", pack.Filename)
			if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
				log.Printf("Cannot remove driver pack archive: %v", err)
			}
		}
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Driver pack removed. Apply changes to update boot.wim."})
}

func (h *Handler) RebuildImageBootWim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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
	if reason := h.driverBuildReadiness(image); reason != "" {
		h.sendJSON(w, http.StatusPreconditionFailed, Response{Error: reason})
		return
	}
	if !h.reserveDriverBuild(image.ID) {
		h.sendJSON(w, http.StatusConflict, Response{Error: "A driver rebuild is already running"})
		return
	}
	go func() {
		defer h.releaseDriverBuild(image.ID)
		if err := h.runDriverBuild(image.ID); err != nil {
			log.Printf("Driver rebuild for %s failed: %v", image.Filename, err)
		}
	}()
	h.sendJSON(w, http.StatusAccepted, Response{Success: true, Message: "Applying driver changes", Data: map[string]string{"status": "applying", "status_url": fmt.Sprintf("/api/drivers/status?imageId=%d", image.ID)}})
}
