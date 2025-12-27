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

	"bootimus/internal/database"
	"bootimus/internal/models"
	"bootimus/internal/storage"
)

type Handler struct {
	db          *database.DB
	sqliteStore *storage.SQLiteStore
	dataDir     string
	bootDir     string
}

func NewHandler(db *database.DB, dataDir string, bootDir string) *Handler {
	var sqliteStore *storage.SQLiteStore
	if db == nil {
		// Only initialise SQLite if PostgreSQL is disabled
		var err error
		sqliteStore, err = storage.NewSQLiteStore(dataDir)
		if err != nil {
			log.Printf("Failed to initialise SQLite store: %v", err)
		} else {
			log.Printf("SQLite database initialised: %s/bootimus.db", dataDir)
		}
	}

	return &Handler{
		db:          db,
		sqliteStore: sqliteStore,
		dataDir:     dataDir,
		bootDir:     bootDir,
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
	entries, err := os.ReadDir(h.dataDir)
	if err != nil {
		log.Printf("Failed to read data directory for sync: %v", err)
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

	var updates models.Image
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

		// Update fields - merge updates into existing image
		if updates.Name != "" {
			image.Name = updates.Name
		}
		if updates.Description != "" {
			image.Description = updates.Description
		}
		// For boolean fields, always update from request
		image.Enabled = updates.Enabled
		image.Public = updates.Public

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

	// Update fields
	if updates.Name != "" {
		image.Name = updates.Name
	}
	if updates.Description != "" {
		image.Description = updates.Description
	}
	image.Enabled = updates.Enabled
	image.Public = updates.Public

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
			filePath := filepath.Join(h.dataDir, filename)
			if err := os.Remove(filePath); err != nil {
				log.Printf("Failed to delete file %s: %v", filePath, err)
			} else {
				log.Printf("Deleted ISO file: %s", filename)
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
		filePath := filepath.Join(h.dataDir, image.Filename)
		if err := os.Remove(filePath); err != nil {
			log.Printf("Failed to delete file %s: %v", filePath, err)
		}
	}

	// Delete from database
	if err := h.db.Delete(&image).Error; err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

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
	filePath := filepath.Join(h.dataDir, header.Filename)
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
	image := models.Image{
		Name:     displayName,
		Filename: header.Filename,
		Size:     size,
		Enabled:  true,
		Public:   r.FormValue("public") == "true",
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

	entries, err := os.ReadDir(h.dataDir)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	var newImages []string

	// Use SQLite if database is disabled
	if h.db == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".iso") {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}

			// Check if already exists
			_, err = h.sqliteStore.GetImage(entry.Name())
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
				}
			}
		}

		h.sendJSON(w, http.StatusOK, Response{
			Success: true,
			Message: fmt.Sprintf("Scan complete. Found %d new images.", len(newImages)),
			Data:    newImages,
		})
		return
	}

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
			}
		}
	}

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: fmt.Sprintf("Scan complete. Found %d new images.", len(newImages)),
		Data:    newImages,
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

	info := map[string]interface{}{
		"configuration": map[string]string{
			"data_directory": h.dataDir,
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
	}

	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: info})
}
