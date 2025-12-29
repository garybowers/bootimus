package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"bootimus/internal/database"
	"bootimus/internal/extractor"
	"bootimus/internal/models"
	"bootimus/internal/storage"
	"bootimus/internal/sysstats"
)

type Handler struct {
	db          *database.DB
	sqliteStore *storage.SQLiteStore
	dataDir     string // Base data directory (/data) - for SQLite database
	isoDir      string // ISO directory (/data/isos) - for ISO files
	bootDir     string
	version     string
}

func NewHandler(db *database.DB, sqliteStore *storage.SQLiteStore, dataDir string, isoDir string, bootDir string, version string) *Handler {
	return &Handler{
		db:          db,
		sqliteStore: sqliteStore,
		dataDir:     dataDir,
		isoDir:      isoDir,
		bootDir:     bootDir,
		version:     version,
	}
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

	// Use SQLite if database is disabled
	if h.db == nil {
		clients, err := h.sqliteStore.ListClients()
		if err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: clients})
		return
	}

	var clients []models.Client
	if err := h.db.Preload("Images").Find(&clients).Error; err != nil {
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

	// Use SQLite if database is disabled
	if h.db == nil {
		client, err := h.sqliteStore.GetClient(mac)
		if err != nil {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Client not found"})
			return
		}
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: client})
		return
	}

	var client models.Client
	if err := h.db.Preload("Images").Where("mac_address = ?", mac).First(&client).Error; err != nil {
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

	// Use SQLite if database is disabled
	if h.db == nil {
		client.Enabled = true // Default
		if err := h.sqliteStore.CreateClient(&client); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
		log.Printf("Client created (SQLite mode): %s (%s)", client.MACAddress, client.Name)
		h.sendJSON(w, http.StatusCreated, Response{Success: true, Message: "Client created", Data: client})
		return
	}

	if err := h.db.Create(&client).Error; err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Client created (DB mode): %s (%s)", client.MACAddress, client.Name)
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

	// Use SQLite if database is disabled
	if h.db == nil {
		client, err := h.sqliteStore.GetClient(mac)
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

		if err := h.sqliteStore.UpdateClient(mac, client); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}

		h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Client updated", Data: client})
		return
	}

	var client models.Client
	if err := h.db.Where("mac_address = ?", mac).First(&client).Error; err != nil {
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

	if err := h.db.Save(&client).Error; err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

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

	// Use SQLite if database is disabled
	if h.db == nil {
		if err := h.sqliteStore.DeleteClient(mac); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
		log.Printf("Client deleted (SQLite mode): %s", mac)
		h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Client deleted"})
		return
	}

	if err := h.db.Where("mac_address = ?", mac).Delete(&models.Client{}).Error; err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Client deleted (DB mode): %s", mac)
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
		var exists bool
		if h.db == nil {
			// Check SQLite
			_, err := h.sqliteStore.GetImage(entry.Name())
			exists = (err == nil)
		} else {
			// Check PostgreSQL
			var existing models.Image
			err := h.db.Where("filename = ?", entry.Name()).First(&existing).Error
			exists = (err == nil)
		}

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

			if h.db == nil {
				if err := h.sqliteStore.CreateImage(image); err != nil {
					log.Printf("Failed to auto-add image from filesystem (SQLite): %s - %v", entry.Name(), err)
				} else {
					log.Printf("Auto-added image from filesystem (SQLite): %s", entry.Name())
				}
			} else {
				if err := h.db.Create(image).Error; err != nil {
					log.Printf("Failed to auto-add image from filesystem (DB): %s - %v", entry.Name(), err)
				} else {
					log.Printf("Auto-added image from filesystem (DB): %s", entry.Name())
				}
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

	// Use SQLite if database is disabled
	if h.db == nil {
		images, err := h.sqliteStore.ListImages()
		if err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
		log.Printf("ListImages returning %d images from SQLite", len(images))
		for i, img := range images {
			log.Printf("  [%d] %s (filename: %s, size: %d)", i, img.Name, img.Filename, img.Size)
		}
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: images})
		return
	}

	var images []models.Image
	if err := h.db.Find(&images).Error; err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("ListImages returning %d images from PostgreSQL", len(images))
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

	// Use SQLite if database is disabled
	if h.db == nil {
		image, err := h.sqliteStore.GetImage(filename)
		if err != nil {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
			return
		}
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: image})
		return
	}

	var image models.Image
	if err := h.db.Where("filename = ?", filename).First(&image).Error; err != nil {
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

	// Use SQLite if database is disabled
	if h.db == nil {
		image, err := h.sqliteStore.GetImage(filename)
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

		if err := h.sqliteStore.UpdateImage(filename, image); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}

		log.Printf("Image updated: %s (enabled=%v, public=%v)", filename, image.Enabled, image.Public)
		h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Image updated", Data: image})
		return
	}

	var image models.Image
	if err := h.db.Where("filename = ?", filename).First(&image).Error; err != nil {
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

	if err := h.db.Save(&image).Error; err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

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

	// Use SQLite if database is disabled
	if h.db == nil {
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

		// Delete from state
		if err := h.sqliteStore.DeleteImage(filename); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}

		log.Printf("Image deleted (SQLite mode): %s", filename)
		h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Image deleted"})
		return
	}

	var image models.Image
	if err := h.db.Where("filename = ?", filename).First(&image).Error; err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
		return
	}

	// Delete from filesystem if requested
	if deleteFile && image.Filename != "" {
		filePath := filepath.Join(h.isoDir, image.Filename)
		if err := os.Remove(filePath); err != nil {
			log.Printf("Failed to delete file %s: %v", filePath, err)
		}

		// Also clean up extracted kernel directory if it exists
		isoBase := strings.TrimSuffix(image.Filename, filepath.Ext(image.Filename))
		extractedDir := filepath.Join(h.isoDir, isoBase)
		if _, err := os.Stat(extractedDir); err == nil {
			if err := os.RemoveAll(extractedDir); err != nil {
				log.Printf("Failed to delete extracted directory %s: %v", extractedDir, err)
			} else {
				log.Printf("Cleaned up extracted kernel directory: %s", extractedDir)
			}
		}
	}

	// Delete from database (hard delete to avoid unique constraint issues on re-upload)
	if err := h.db.Unscoped().Delete(&image).Error; err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Image deleted (PostgreSQL mode): %s", filename)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Image deleted"})
}

func (h *Handler) UploadImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	// Parse multipart form (max 10GB)
	if err := r.ParseMultipartForm(10 << 30); err != nil {
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

	log.Printf("Starting ISO upload: %s (size: %d bytes)", header.Filename, header.Size)

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
	log.Printf("Upload complete: %s (%d MB)", header.Filename, size/(1024*1024))

	// Create entry
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

	// Use SQLite if database is disabled
	if h.db == nil {
		if err := h.sqliteStore.CreateImage(&image); err != nil {
			// Clean up uploaded file on database error
			os.Remove(filePath)
			log.Printf("Failed to create image record, file removed: %s - %v", header.Filename, err)
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to create image record"})
			return
		}
		log.Printf("Image uploaded (SQLite mode): %s (%d bytes)", image.Filename, image.Size)
		h.sendJSON(w, http.StatusCreated, Response{Success: true, Message: "Image uploaded", Data: image})
		return
	}

	if err := h.db.Create(&image).Error; err != nil {
		// Clean up uploaded file on database error
		os.Remove(filePath)
		log.Printf("Failed to create image record, file removed: %s - %v", header.Filename, err)
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to create image record"})
		return
	}

	log.Printf("Image uploaded (DB mode): %s (%d bytes)", image.Filename, image.Size)
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

	// Use SQLite if database is disabled
	if h.db == nil {
		if req.MACAddress == "" {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing mac_address"})
			return
		}

		if err := h.sqliteStore.AssignImagesToClient(req.MACAddress, req.ImageFilenames); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}

		log.Printf("Images assigned to client (SQLite mode): %s -> %v", req.MACAddress, req.ImageFilenames)
		h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Images assigned to client"})
		return
	}

	var client models.Client
	if err := h.db.First(&client, req.ClientID).Error; err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Client not found"})
		return
	}

	var images []models.Image
	if err := h.db.Find(&images, req.ImageIDs).Error; err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	// Replace associations
	if err := h.db.Model(&client).Association("Images").Replace(&images); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

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

	// Get the image from database (works with both PostgreSQL and SQLite)
	var image *models.Image
	var err error

	if h.db == nil {
		// SQLite mode
		image, err = h.sqliteStore.GetImage(filename)
		if err != nil {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
			return
		}
	} else {
		// PostgreSQL mode
		var dbImage models.Image
		if err := h.db.Where("filename = ?", filename).First(&dbImage).Error; err != nil {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
			return
		}
		image = &dbImage
	}

	// Check if already extracted
	if image.Extracted && image.BootMethod == "kernel" {
		h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Image already extracted", Data: image})
		return
	}

	// Import extractor package
	log.Printf("Extracting kernel/initrd from ISO: %s", filename)

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

		if h.db == nil {
			h.sqliteStore.UpdateImage(filename, image)
		} else {
			h.db.Save(image)
		}

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

	// Update database with extraction info
	now := time.Now()
	image.Extracted = true
	image.Distro = bootFiles.Distro
	image.BootMethod = "kernel"
	image.KernelPath = bootFiles.Kernel
	image.InitrdPath = bootFiles.Initrd
	image.BootParams = bootFiles.BootParams + " "
	image.ExtractionError = ""
	image.ExtractedAt = &now

	if h.db == nil {
		// SQLite mode
		if err := h.sqliteStore.UpdateImage(filename, image); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
	} else {
		// PostgreSQL mode
		if err := h.db.Save(image).Error; err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
	}

	log.Printf("Successfully extracted %s: distro=%s, kernel=%s, initrd=%s",
		filename, bootFiles.Distro, bootFiles.Kernel, bootFiles.Initrd)

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: fmt.Sprintf("Successfully extracted %s boot files", bootFiles.Distro),
		Data:    image,
	})
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

	// Get the image (works with both PostgreSQL and SQLite)
	var image *models.Image
	var err error

	if h.db == nil {
		// SQLite mode
		image, err = h.sqliteStore.GetImage(req.Filename)
		if err != nil {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
			return
		}
	} else {
		// PostgreSQL mode
		var dbImage models.Image
		if err := h.db.Where("filename = ?", req.Filename).First(&dbImage).Error; err != nil {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
			return
		}
		image = &dbImage
	}

	// If switching to kernel method, ensure extraction has been done
	if req.BootMethod == "kernel" && !image.Extracted {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Cannot use kernel boot method: image not extracted. Please extract first.",
		})
		return
	}

	// Update boot method
	image.BootMethod = req.BootMethod

	if h.db == nil {
		// SQLite mode
		if err := h.sqliteStore.UpdateImage(req.Filename, image); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
	} else {
		// PostgreSQL mode
		if err := h.db.Save(image).Error; err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
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

	var stats struct {
		TotalClients  int64 `json:"total_clients"`
		ActiveClients int64 `json:"active_clients"`
		TotalImages   int64 `json:"total_images"`
		EnabledImages int64 `json:"enabled_images"`
		TotalBoots    int64 `json:"total_boots"`
	}

	// Use SQLite if database is disabled
	if h.db == nil {
		sqliteStats, err := h.sqliteStore.GetStats()
		if err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
		stats.TotalClients = sqliteStats["total_clients"]
		stats.ActiveClients = sqliteStats["active_clients"]
		stats.TotalImages = sqliteStats["total_images"]
		stats.EnabledImages = sqliteStats["enabled_images"]
		stats.TotalBoots = sqliteStats["total_boots"]
		log.Printf("Stats retrieved (SQLite mode): %d clients, %d images, %d boots", stats.TotalClients, stats.TotalImages, stats.TotalBoots)
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: stats})
		return
	}

	h.db.Model(&models.Client{}).Count(&stats.TotalClients)
	h.db.Model(&models.Client{}).Where("enabled = ?", true).Count(&stats.ActiveClients)
	h.db.Model(&models.Image{}).Count(&stats.TotalImages)
	h.db.Model(&models.Image{}).Where("enabled = ?", true).Count(&stats.EnabledImages)
	h.db.Model(&models.BootLog{}).Count(&stats.TotalBoots)

	log.Printf("Stats retrieved (DB mode): %d clients, %d images, %d boots", stats.TotalClients, stats.TotalImages, stats.TotalBoots)
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

	// Use SQLite if database is disabled
	if h.db == nil {
		logs, err := h.sqliteStore.GetBootLogs(limit)
		if err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
		log.Printf("Boot logs retrieved (SQLite mode): %d entries", len(logs))
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: logs})
		return
	}

	var logs []models.BootLog
	if err := h.db.Preload("Client").Preload("Image").
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Boot logs retrieved (DB mode): %d entries", len(logs))
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

	// Use SQLite if database is disabled
	if h.db == nil {
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
			existing, err := h.sqliteStore.GetImage(entry.Name())
			if err != nil { // Not found, create new
				displayName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
				image := &models.Image{
					Name:     displayName,
					Filename: entry.Name(),
					Size:     info.Size(),
					Enabled:  true,
					Public:   true,
				}

				if err := h.sqliteStore.CreateImage(image); err == nil {
					newImages = append(newImages, entry.Name())
					log.Printf("Added new image to database: %s", entry.Name())
				} else {
					log.Printf("Failed to add image to database: %s - %v", entry.Name(), err)
				}
			} else {
				// Image exists, update size if changed
				if existing.Size != info.Size() {
					oldSize := existing.Size
					existing.Size = info.Size()
					if err := h.sqliteStore.UpdateImage(existing.Filename, existing); err == nil {
						log.Printf("Updated image size: %s (%d -> %d bytes)", existing.Filename, oldSize, info.Size())
					}
				}
			}
		}

		// Remove images that no longer exist on disk
		allImages, err := h.sqliteStore.ListImages()
		if err == nil {
			log.Printf("Checking %d database images against %d filesystem ISOs", len(allImages), len(existingFiles))
			for _, image := range allImages {
				if !existingFiles[image.Filename] {
					// ISO file no longer exists, delete from database
					log.Printf("Deleting missing image from database: %s (ID: %d)", image.Filename, image.ID)
					if err := h.sqliteStore.DeleteImage(image.Filename); err == nil {
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
		h.sendJSON(w, http.StatusOK, Response{
			Success: true,
			Message: msg,
			Data: map[string]interface{}{
				"new":     newImages,
				"deleted": deletedImages,
			},
		})
		return
	}

	// PostgreSQL mode
	// Add new images and update existing ones
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".iso") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		var existing models.Image
		err = h.db.Where("filename = ?", entry.Name()).First(&existing).Error

		if err != nil { // Not found, create new
			displayName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			image := models.Image{
				Name:     displayName,
				Filename: entry.Name(),
				Size:     info.Size(),
				Enabled:  true,
				Public:   true,
			}

			if err := h.db.Create(&image).Error; err == nil {
				newImages = append(newImages, entry.Name())
				log.Printf("Added new image to database: %s", entry.Name())
			} else {
				log.Printf("Failed to add image to database: %s - %v", entry.Name(), err)
			}
		} else {
			// Image exists, update size if changed
			if existing.Size != info.Size() {
				if err := h.db.Model(&existing).Update("size", info.Size()).Error; err == nil {
					log.Printf("Updated image size: %s (%d -> %d bytes)", entry.Name(), existing.Size, info.Size())
				}
			}
		}
	}

	// Remove images that no longer exist on disk
	var allImages []models.Image
	if err := h.db.Find(&allImages).Error; err == nil {
		log.Printf("Checking %d database images against %d filesystem ISOs", len(allImages), len(existingFiles))
		for _, image := range allImages {
			if !existingFiles[image.Filename] {
				// ISO file no longer exists, delete from database
				log.Printf("Deleting missing image from database: %s (ID: %d)", image.Filename, image.ID)
				if err := h.db.Delete(&image).Error; err == nil {
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
				if h.db != nil {
					return "PostgreSQL"
				} else if h.sqliteStore != nil {
					return "SQLite"
				}
				return "Disabled"
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
	if h.db == nil {
		// SQLite mode
		users, err := h.sqliteStore.ListUsers()
		if err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: users})
		return
	}

	// PostgreSQL mode
	var users []models.User
	if err := h.db.Find(&users).Error; err != nil {
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

	if h.db == nil {
		// SQLite mode - check if user exists
		if _, err := h.sqliteStore.GetUser(req.Username); err == nil {
			h.sendJSON(w, http.StatusConflict, Response{Success: false, Error: "User already exists"})
			return
		}

		if err := h.sqliteStore.CreateUser(&user); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
	} else {
		// PostgreSQL mode - check if user exists
		var existing models.User
		if err := h.db.Where("username = ?", req.Username).First(&existing).Error; err == nil {
			h.sendJSON(w, http.StatusConflict, Response{Success: false, Error: "User already exists"})
			return
		}

		if err := h.db.Create(&user).Error; err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
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

	if h.db == nil {
		// SQLite mode
		user, err := h.sqliteStore.GetUser(username)
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

		if err := h.sqliteStore.UpdateUser(username, user); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}

		log.Printf("User updated: %s (admin=%v, enabled=%v)", user.Username, user.IsAdmin, user.Enabled)
		h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "User updated", Data: user})
		return
	}

	// PostgreSQL mode
	var user models.User
	if err := h.db.Where("username = ?", username).First(&user).Error; err != nil {
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

	if err := h.db.Save(&user).Error; err != nil {
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

	if h.db == nil {
		// SQLite mode
		if err := h.sqliteStore.DeleteUser(username); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
	} else {
		// PostgreSQL mode
		if err := h.db.Where("username = ?", username).Delete(&models.User{}).Error; err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
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

	if h.db == nil {
		// SQLite mode
		user, err := h.sqliteStore.GetUser(req.Username)
		if err != nil {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "User not found"})
			return
		}

		if err := user.SetPassword(req.NewPassword); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to hash password"})
			return
		}

		if err := h.sqliteStore.UpdateUser(req.Username, user); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}

		log.Printf("Password reset for user: %s", user.Username)
		h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Password reset successfully"})
		return
	}

	// PostgreSQL mode
	var user models.User
	if err := h.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "User not found"})
		return
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to hash password"})
		return
	}

	if err := h.db.Save(&user).Error; err != nil {
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
	if h.db != nil {
		isoFiles := []struct{ Name, Filename string; Size int64 }{
			{Name: strings.TrimSuffix(filename, filepath.Ext(filename)), Filename: filename, Size: downloaded},
		}
		if err := h.db.SyncImages(isoFiles); err != nil {
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

	if h.db == nil {
		// SQLite mode
		image, err = h.sqliteStore.GetImage(filename)
		if err != nil {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
			return
		}
	} else {
		// PostgreSQL mode
		var dbImage models.Image
		if err := h.db.Where("filename = ?", filename).First(&dbImage).Error; err != nil {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
			return
		}
		image = &dbImage
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

	var image *models.Image
	var err error

	if h.db == nil {
		// SQLite mode
		image, err = h.sqliteStore.GetImage(filename)
		if err != nil {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
			return
		}

		image.AutoInstallScript = req.Script
		image.AutoInstallEnabled = req.Enabled
		image.AutoInstallScriptType = req.ScriptType

		if err := h.sqliteStore.UpdateImage(filename, image); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
	} else {
		// PostgreSQL mode
		var dbImage models.Image
		if err := h.db.Where("filename = ?", filename).First(&dbImage).Error; err != nil {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
			return
		}

		dbImage.AutoInstallScript = req.Script
		dbImage.AutoInstallEnabled = req.Enabled
		dbImage.AutoInstallScriptType = req.ScriptType

		if err := h.db.Save(&dbImage).Error; err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
		image = &dbImage
	}

	log.Printf("Auto-install script updated for %s: enabled=%v, type=%s, size=%d bytes",
		filename, image.AutoInstallEnabled, image.AutoInstallScriptType, len(image.AutoInstallScript))

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "Auto-install script updated",
		Data:    image,
	})
}

