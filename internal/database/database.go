package database

import (
	"crypto/rand"
	"fmt"
	"log"
	"time"

	"bootimus/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type DB struct {
	*gorm.DB
}

// New creates a new database connection
func New(cfg *Config) (*DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &DB{db}, nil
}

// AutoMigrate runs database migrations
func (db *DB) AutoMigrate() error {
	log.Println("Running database migrations...")
	return db.DB.AutoMigrate(
		&models.User{},
		&models.Client{},
		&models.Image{},
		&models.BootLog{},
	)
}

// GetImagesForClient returns images accessible to a specific MAC address
func (db *DB) GetImagesForClient(macAddress string) ([]models.Image, error) {
	var images []models.Image

	// First, get public images
	if err := db.Where("enabled = ? AND public = ?", true, true).Find(&images).Error; err != nil {
		return nil, err
	}

	// Then, get client-specific images
	var client models.Client
	if err := db.Where("mac_address = ? AND enabled = ?", macAddress, true).
		Preload("Images", "enabled = ?", true).
		First(&client).Error; err == nil {
		// Client found, append their specific images
		images = append(images, client.Images...)
	}

	return images, nil
}

// LogBootAttempt logs a boot attempt
func (db *DB) LogBootAttempt(macAddress, imageName, ipAddress string, success bool, errorMsg string) error {
	bootLog := models.BootLog{
		MACAddress: macAddress,
		ImageName:  imageName,
		IPAddress:  ipAddress,
		Success:    success,
		ErrorMsg:   errorMsg,
	}

	// Try to link to existing client and image
	var client models.Client
	if err := db.Where("mac_address = ?", macAddress).First(&client).Error; err == nil {
		bootLog.ClientID = &client.ID
	}

	var image models.Image
	if err := db.Where("name = ?", imageName).First(&image).Error; err == nil {
		bootLog.ImageID = &image.ID
	}

	return db.Create(&bootLog).Error
}

// UpdateClientBootStats updates client boot statistics
func (db *DB) UpdateClientBootStats(macAddress string) error {
	now := time.Now()
	return db.Model(&models.Client{}).
		Where("mac_address = ?", macAddress).
		Updates(map[string]interface{}{
			"last_boot":  now,
			"boot_count": gorm.Expr("boot_count + 1"),
		}).Error
}

// UpdateImageBootStats updates image boot statistics
func (db *DB) UpdateImageBootStats(imageName string) error {
	now := time.Now()
	return db.Model(&models.Image{}).
		Where("name = ?", imageName).
		Updates(map[string]interface{}{
			"last_booted": now,
			"boot_count":  gorm.Expr("boot_count + 1"),
		}).Error
}

// SyncImages syncs filesystem ISOs with database
func (db *DB) SyncImages(isoFiles []struct{ Name, Filename string; Size int64 }) error {
	for _, iso := range isoFiles {
		var image models.Image
		err := db.Where("filename = ?", iso.Filename).First(&image).Error

		if err == gorm.ErrRecordNotFound {
			// Create new image
			image = models.Image{
				Name:     iso.Name,
				Filename: iso.Filename,
				Size:     iso.Size,
				Enabled:  true,
				Public:   true, // Default to public
			}
			if err := db.Create(&image).Error; err != nil {
				log.Printf("Failed to create image %s: %v", iso.Name, err)
			} else {
				log.Printf("Added new image: %s", iso.Name)
			}
		} else if err == nil {
			// Update existing image size if changed
			if image.Size != iso.Size {
				db.Model(&image).Update("size", iso.Size)
			}
		}
	}

	return nil
}

// User management functions

// EnsureAdminUser ensures an admin user exists, creates one if not
func (db *DB) EnsureAdminUser() (username, password string, created bool, err error) {
	var admin models.User
	err = db.Where("username = ?", "admin").First(&admin).Error

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
		if err := db.Create(&admin).Error; err != nil {
			return "", "", false, err
		}
		return "admin", password, true, nil
	}

	return "admin", "", false, err
}

// ResetAdminPassword resets the admin password to a new random one
func (db *DB) ResetAdminPassword() (string, error) {
	var admin models.User
	if err := db.Where("username = ?", "admin").First(&admin).Error; err != nil {
		return "", err
	}

	password := generateRandomPassword(16)
	if err := admin.SetPassword(password); err != nil {
		return "", err
	}

	if err := db.Save(&admin).Error; err != nil {
		return "", err
	}

	return password, nil
}

// GetUser retrieves a user by username
func (db *DB) GetUser(username string) (*models.User, error) {
	var user models.User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateUserLastLogin updates the user's last login time
func (db *DB) UpdateUserLastLogin(username string) error {
	now := time.Now()
	return db.Model(&models.User{}).Where("username = ?", username).Update("last_login", now).Error
}

// generateRandomPassword generates a random password
func generateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[randInt(len(charset))]
	}
	return string(b)
}

func randInt(max int) int {
	// Simple random int for password generation
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	return int(b[0]) % max
}
