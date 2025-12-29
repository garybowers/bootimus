package storage

import (
	"crypto/rand"
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
	if err := db.AutoMigrate(&models.User{}, &models.Client{}, &models.Image{}, &models.BootLog{}); err != nil {
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
	return s.db.Model(&models.Client{}).Where("mac_address = ?", mac).Save(client).Error
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
	return s.db.Model(&models.Image{}).Where("filename = ?", filename).Save(image).Error
}

func (s *SQLiteStore) DeleteImage(filename string) error {
	// Use Unscoped to perform hard delete (avoid unique constraint issues on re-upload)
	return s.db.Unscoped().Where("filename = ?", filename).Delete(&models.Image{}).Error
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

// User management operations
func (s *SQLiteStore) EnsureAdminUser() (username, password string, created bool, err error) {
	var admin models.User
	err = s.db.Where("username = ?", "admin").First(&admin).Error

	if err == gorm.ErrRecordNotFound {
		// Create admin user with random password
		password = generateRandomPassword(16)
		admin = models.User{
			Username: "admin",
			Enabled:  true,
			IsAdmin:  true,
		}
		if err := admin.SetPassword(password); err != nil {
			return "", "", false, err
		}
		if err := s.db.Create(&admin).Error; err != nil {
			return "", "", false, err
		}
		return "admin", password, true, nil
	}

	return "admin", "", false, err
}

func (s *SQLiteStore) ResetAdminPassword() (string, error) {
	var admin models.User
	if err := s.db.Where("username = ?", "admin").First(&admin).Error; err != nil {
		return "", err
	}

	password := generateRandomPassword(16)
	if err := admin.SetPassword(password); err != nil {
		return "", err
	}

	if err := s.db.Save(&admin).Error; err != nil {
		return "", err
	}

	return password, nil
}

func (s *SQLiteStore) GetUser(username string) (*models.User, error) {
	var user models.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *SQLiteStore) UpdateUserLastLogin(username string) error {
	return s.db.Model(&models.User{}).Where("username = ?", username).Update("last_login", gorm.Expr("CURRENT_TIMESTAMP")).Error
}

func (s *SQLiteStore) ListUsers() ([]*models.User, error) {
	var users []*models.User
	if err := s.db.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (s *SQLiteStore) CreateUser(user *models.User) error {
	return s.db.Create(user).Error
}

func (s *SQLiteStore) UpdateUser(username string, user *models.User) error {
	return s.db.Model(&models.User{}).Where("username = ?", username).Save(user).Error
}

func (s *SQLiteStore) DeleteUser(username string) error {
	return s.db.Where("username = ?", username).Delete(&models.User{}).Error
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	db, err := s.db.DB()
	if err != nil {
		return err
	}
	return db.Close()
}

// Helper functions for password generation
func generateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[randInt(len(charset))]
	}
	return string(b)
}

func randInt(max int) int {
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	return int(b[0]) % max
}
