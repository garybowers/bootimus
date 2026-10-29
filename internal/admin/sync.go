package admin

import (
	"context"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bootimus/internal/models"
	"bootimus/internal/replica"

	"golang.org/x/crypto/bcrypt"
)

type ReplicaController interface {
	Status() replica.Status
	RunNow(ctx context.Context) replica.Status
}

type syncKind struct {
	Count     int    `json:"count"`
	UpdatedAt string `json:"updated_at"`
}

type syncBootVerifier struct {
	Filename  string    `json:"filename,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	Required  bool      `json:"required"`
	Hash      string    `json:"hash"`
	Revision  string    `json:"revision"`
}

func validBootVerifier(required bool, hash, revision string) bool {
	if revision != "" {
		if len(revision) != 32 {
			return false
		}
		if _, err := hex.DecodeString(revision); err != nil {
			return false
		}
	}
	if !required {
		return hash == ""
	}
	if len(hash) != 60 {
		return false
	}
	cost, err := bcrypt.Cost([]byte(hash))
	return err == nil && cost <= 14
}

func (h *Handler) SyncBootVerifiers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	if r.TLS == nil {
		h.sendJSON(w, http.StatusForbidden, Response{Success: false, Error: "Boot verifiers require HTTPS"})
		return
	}
	images, err := h.storage.ListImages()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Could not list images"})
		return
	}
	theme, err := h.storage.GetMenuTheme()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Could not load theme"})
		return
	}
	verifiers := make([]syncBootVerifier, 0, len(images))
	for _, image := range images {
		if !validBootVerifier(image.BootPasswordRequired, image.BootPasswordHash, image.BootPasswordRevision) {
			h.sendJSON(w, http.StatusConflict, Response{Success: false, Error: "Invalid image boot password state"})
			return
		}
		verifiers = append(verifiers, syncBootVerifier{Filename: image.Filename, UpdatedAt: image.UpdatedAt, Required: image.BootPasswordRequired, Hash: image.BootPasswordHash, Revision: image.BootPasswordRevision})
	}
	if !validBootVerifier(theme.BootPasswordRequired, theme.BootPasswordHash, theme.BootPasswordRevision) {
		h.sendJSON(w, http.StatusConflict, Response{Success: false, Error: "Invalid menu boot password state"})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: map[string]interface{}{
		"version": 1,
		"images":  verifiers,
		"theme":   syncBootVerifier{UpdatedAt: theme.UpdatedAt, Required: theme.BootPasswordRequired, Hash: theme.BootPasswordHash, Revision: theme.BootPasswordRevision},
	}})
}

func latest(times ...time.Time) string {
	var max time.Time
	for _, t := range times {
		if t.After(max) {
			max = t
		}
	}
	if max.IsZero() {
		return ""
	}
	return max.UTC().Format(time.RFC3339Nano)
}

func (h *Handler) SyncManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	kinds := map[string]syncKind{}

	clients, _ := h.storage.ListClients()
	var ts []time.Time
	for _, c := range clients {
		ts = append(ts, c.UpdatedAt)
	}
	kinds["clients"] = syncKind{Count: len(clients), UpdatedAt: latest(ts...)}

	images, _ := h.storage.ListImages()
	ts = ts[:0]
	for _, i := range images {
		ts = append(ts, i.UpdatedAt)
	}
	kinds["images"] = syncKind{Count: len(images), UpdatedAt: latest(ts...)}

	clientGroups, _ := h.storage.ListClientGroups()
	ts = ts[:0]
	for _, g := range clientGroups {
		ts = append(ts, g.UpdatedAt)
	}
	kinds["client_groups"] = syncKind{Count: len(clientGroups), UpdatedAt: latest(ts...)}

	imageGroups, _ := h.storage.ListImageGroups()
	ts = ts[:0]
	for _, g := range imageGroups {
		ts = append(ts, g.UpdatedAt)
	}
	kinds["image_groups"] = syncKind{Count: len(imageGroups), UpdatedAt: latest(ts...)}

	sanTargets, _ := h.storage.ListSANTargets()
	ts = ts[:0]
	for _, t := range sanTargets {
		ts = append(ts, t.UpdatedAt)
	}
	kinds["san_targets"] = syncKind{Count: len(sanTargets), UpdatedAt: latest(ts...)}

	profiles, _ := h.storage.ListDistroProfiles()
	ts = ts[:0]
	custom := 0
	for _, p := range profiles {
		if p.Custom {
			custom++
			ts = append(ts, p.UpdatedAt)
		}
	}
	kinds["profiles"] = syncKind{Count: custom, UpdatedAt: latest(ts...)}

	files, _ := h.storage.ListCustomFiles()
	ts = ts[:0]
	for _, f := range files {
		ts = append(ts, f.UpdatedAt)
	}
	kinds["custom_files"] = syncKind{Count: len(files), UpdatedAt: latest(ts...)}

	activeSet := h.bootloaderSelector.GetActiveBootloaderSet()
	if activeSet == "" {
		activeSet = "built-in"
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: map[string]interface{}{
		"version":        h.version,
		"boot_base_url":  fmt.Sprintf("http://%s:%d", h.serverAddr, h.httpPort),
		"bootloader_set": activeSet,
		"server_time":    time.Now().UTC().Format(time.RFC3339),
		"kinds":          kinds,
	}})
}

type syncToolFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func (h *Handler) SyncToolFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid tool name"})
		return
	}
	if _, err := h.storage.GetBootTool(name); err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Tool not found"})
		return
	}
	root := h.toolsManager.ToolDir(name)
	files := []syncToolFile{}
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, syncToolFile{Path: filepath.ToSlash(rel), Size: info.Size()})
		return nil
	})
	if walkErr != nil && !os.IsNotExist(walkErr) {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: walkErr.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: files})
}

func containedPath(root, rel string) (string, bool) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "", false
	}
	full := filepath.Join(root, clean)
	if !strings.HasPrefix(full, filepath.Clean(root)+string(os.PathSeparator)) {
		return "", false
	}
	return full, true
}

func (h *Handler) SyncAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	q := r.URL.Query()
	var path string
	switch q.Get("kind") {
	case "iso":
		full, ok := containedPath(h.isoDir, q.Get("filename"))
		if !ok {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid filename"})
			return
		}
		path = full
	case "tool":
		name := q.Get("name")
		if name == "" || strings.ContainsAny(name, `/\`) {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid tool name"})
			return
		}
		full, ok := containedPath(h.toolsManager.ToolDir(name), q.Get("path"))
		if !ok {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid path"})
			return
		}
		path = full
	case "file":
		id, err := strconv.ParseUint(q.Get("id"), 10, 32)
		if err != nil {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid file ID"})
			return
		}
		file, err := h.storage.GetCustomFileByID(uint(id))
		if err != nil {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "File not found"})
			return
		}
		path = h.customFilePath(file)
		if path == "" {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "File has no storage path"})
			return
		}
	default:
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "kind must be iso, tool or file"})
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Asset not found"})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, path)
}

func (h *Handler) customFilePath(file *models.CustomFile) string {
	if file.Public {
		return filepath.Join(h.dataDir, "files", file.Filename)
	}
	if file.ImageID != nil {
		image := file.Image
		if image == nil {
			images, _ := h.storage.ListImages()
			for _, img := range images {
				if img.ID == *file.ImageID {
					image = img
				}
			}
		}
		if image != nil {
			imageName := strings.TrimSuffix(image.Filename, filepath.Ext(image.Filename))
			return filepath.Join(h.isoDir, imageName, "files", file.Filename)
		}
	}
	return ""
}

func (h *Handler) ReplicaStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	if h.Replica == nil {
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: map[string]interface{}{"enabled": false}})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: map[string]interface{}{"enabled": true, "status": h.Replica.Status()}})
}

func (h *Handler) ReplicaSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	if h.Replica == nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "This instance is not a replica"})
		return
	}
	status := h.Replica.RunNow(r.Context())
	h.sendJSON(w, http.StatusOK, Response{Success: status.LastError == "", Message: "Sync finished", Error: status.LastError, Data: status})
}
