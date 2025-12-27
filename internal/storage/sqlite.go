package storage

import (
	"fmt"
	"path/filepath"

	"bootimus/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SQLiteStore provides SQLite-based storage for local-only mode
type SQLiteStore struct {
	db *gorm.DB
}

func NewSQLiteStore(dataDir string) (*SQLiteStore, error) {
	dbPath := filepath.Join(dataDir, "bootimus.db")

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Run migrations
	if err := db.AutoMigrate(&models.Client{}, &models.Image{}, &models.BootLog{}); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Client operations
func (s *SQLiteStore) ListClients() ([]*models.Client, error) {
	var clients []*models.Client
	if err := s.db.Find(&clients).Error; err != nil {
		return nil, err
	}
	return clients, nil
}

func (s *SQLiteStore) GetClient(mac string) (*models.Client, error) {
	var client models.Client
	if err := s.db.Where("mac_address = ?", mac).First(&client).Error; err != nil {
		return nil, err
	}
	return &client, nil
}

func (s *SQLiteStore) CreateClient(client *models.Client) error {
	return s.db.Create(client).Error
}

func (s *SQLiteStore) UpdateClient(mac string, client *models.Client) error {
	return s.db.Model(&models.Client{}).Where("mac_address = ?", mac).Updates(client).Error
}

func (s *SQLiteStore) DeleteClient(mac string) error {
	return s.db.Where("mac_address = ?", mac).Delete(&models.Client{}).Error
}

// Image operations
func (s *SQLiteStore) ListImages() ([]*models.Image, error) {
	var images []*models.Image
	if err := s.db.Find(&images).Error; err != nil {
		return nil, err
	}
	return images, nil
}

func (s *SQLiteStore) GetImage(filename string) (*models.Image, error) {
	var image models.Image
	if err := s.db.Where("filename = ?", filename).First(&image).Error; err != nil {
		return nil, err
	}
	return &image, nil
}

func (s *SQLiteStore) CreateImage(image *models.Image) error {
	return s.db.Create(image).Error
}

func (s *SQLiteStore) UpdateImage(filename string, image *models.Image) error {
	return s.db.Model(&models.Image{}).Where("filename = ?", filename).Updates(image).Error
}

func (s *SQLiteStore) DeleteImage(filename string) error {
	return s.db.Where("filename = ?", filename).Delete(&models.Image{}).Error
}

// Client-Image associations
func (s *SQLiteStore) AssignImagesToClient(mac string, imageFilenames []string) error {
	var client models.Client
	if err := s.db.Where("mac_address = ?", mac).First(&client).Error; err != nil {
		return err
	}

	// Store as JSON array in AllowedImages field
	client.AllowedImages = imageFilenames
	return s.db.Save(&client).Error
}

func (s *SQLiteStore) GetClientImages(mac string) ([]string, error) {
	var client models.Client
	if err := s.db.Where("mac_address = ?", mac).First(&client).Error; err != nil {
		return nil, err
	}
	return client.AllowedImages, nil
}

// GetStats returns statistics about clients, images, and boot logs
func (s *SQLiteStore) GetStats() (map[string]int64, error) {
	stats := make(map[string]int64)

	var totalClients, activeClients, totalImages, enabledImages, totalBoots int64

	s.db.Model(&models.Client{}).Count(&totalClients)
	s.db.Model(&models.Client{}).Where("enabled = ?", true).Count(&activeClients)
	s.db.Model(&models.Image{}).Count(&totalImages)
	s.db.Model(&models.Image{}).Where("enabled = ?", true).Count(&enabledImages)
	s.db.Model(&models.BootLog{}).Count(&totalBoots)

	stats["total_clients"] = totalClients
	stats["active_clients"] = activeClients
	stats["total_images"] = totalImages
	stats["enabled_images"] = enabledImages
	stats["total_boots"] = totalBoots

	return stats, nil
}

// GetBootLogs returns boot logs with optional limit
func (s *SQLiteStore) GetBootLogs(limit int) ([]models.BootLog, error) {
	var logs []models.BootLog
	if err := s.db.Preload("Client").Preload("Image").
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	db, err := s.db.DB()
	if err != nil {
		return err
	}
	return db.Close()
}
