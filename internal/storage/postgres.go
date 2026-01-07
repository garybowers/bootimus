package storage

import (
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

type PostgresStore struct {
	db *gorm.DB
}

func NewPostgresStore(cfg *Config) (*PostgresStore, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) AutoMigrate() error {
	log.Println("Running PostgreSQL database migrations...")

	if err := s.db.AutoMigrate(
		&models.User{},
		&models.Client{},
		&models.ImageGroup{},
		&models.Image{},
		&models.BootLog{},
		&models.CustomFile{},
		&models.DriverPack{},
	); err != nil {
		return err
	}

	if err := s.migrateCustomFileUniqueIndex(); err != nil {
		log.Printf("Warning: CustomFile index migration failed (may already be migrated): %v", err)
	}

	return nil
}

func (s *PostgresStore) migrateCustomFileUniqueIndex() error {
	var indexExists bool
	err := s.db.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE indexname = 'idx_custom_files_filename'
		)
	`).Scan(&indexExists).Error

	if err != nil {
		return fmt.Errorf("failed to check index: %w", err)
	}

	if !indexExists {
		log.Println("CustomFile index already migrated")
		return nil
	}

	log.Println("Migrating CustomFile unique index...")

	if err := s.db.Exec("DROP INDEX IF EXISTS idx_custom_files_filename").Error; err != nil {
		return fmt.Errorf("failed to drop old index: %w", err)
	}

	if err := s.db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_filename_image
		ON custom_files (filename, public, image_id)
	`).Error; err != nil {
		return fmt.Errorf("failed to create new index: %w", err)
	}

	log.Println("CustomFile index migration completed successfully")
	return nil
}

func (s *PostgresStore) Close() error {
	return nil
}


func (s *PostgresStore) ListClients() ([]*models.Client, error) {
	var clients []*models.Client
	if err := s.db.Preload("Images").Find(&clients).Error; err != nil {
		return nil, err
	}
	return clients, nil
}

func (s *PostgresStore) GetClient(mac string) (*models.Client, error) {
	var client models.Client
	if err := s.db.Preload("Images").Where("mac_address = ?", mac).First(&client).Error; err != nil {
		return nil, err
	}
	return &client, nil
}

func (s *PostgresStore) CreateClient(client *models.Client) error {
	return s.db.Create(client).Error
}

func (s *PostgresStore) UpdateClient(mac string, client *models.Client) error {
	return s.db.Model(&models.Client{}).Where("mac_address = ?", mac).Updates(client).Error
}

func (s *PostgresStore) DeleteClient(mac string) error {
	return s.db.Where("mac_address = ?", mac).Delete(&models.Client{}).Error
}


func (s *PostgresStore) ListImages() ([]*models.Image, error) {
	var images []*models.Image
	if err := s.db.Find(&images).Error; err != nil {
		return nil, err
	}
	return images, nil
}

func (s *PostgresStore) GetImage(filename string) (*models.Image, error) {
	var image models.Image
	if err := s.db.Where("filename = ?", filename).First(&image).Error; err != nil {
		return nil, err
	}
	return &image, nil
}

func (s *PostgresStore) CreateImage(image *models.Image) error {
	return s.db.Create(image).Error
}

func (s *PostgresStore) UpdateImage(filename string, image *models.Image) error {
	return s.db.Model(&models.Image{}).Where("filename = ?", filename).Updates(image).Error
}

func (s *PostgresStore) DeleteImage(filename string) error {
	return s.db.Unscoped().Where("filename = ?", filename).Delete(&models.Image{}).Error
}

func (s *PostgresStore) SyncImages(isoFiles []struct{ Name, Filename string; Size int64 }) error {
	for _, iso := range isoFiles {
		var image models.Image
		err := s.db.Where("filename = ?", iso.Filename).First(&image).Error

		if err == gorm.ErrRecordNotFound {
			image = models.Image{
				Name:     iso.Name,
				Filename: iso.Filename,
				Size:     iso.Size,
				Enabled:  true,
				Public:   true,
			}
			if err := s.db.Create(&image).Error; err != nil {
				return fmt.Errorf("failed to create image %s: %w", iso.Name, err)
			}
		} else if err == nil {
			if image.Size != iso.Size {
				s.db.Model(&image).Update("size", iso.Size)
			}
		} else {
			return err
		}
	}

	return nil
}


func (s *PostgresStore) AssignImagesToClient(mac string, imageFilenames []string) error {
	var client models.Client
	if err := s.db.Where("mac_address = ?", mac).First(&client).Error; err != nil {
		return err
	}

	var images []models.Image
	if err := s.db.Where("filename IN ?", imageFilenames).Find(&images).Error; err != nil {
		return err
	}

	return s.db.Model(&client).Association("Images").Replace(images)
}

func (s *PostgresStore) GetClientImages(mac string) ([]string, error) {
	var client models.Client
	if err := s.db.Preload("Images").Where("mac_address = ?", mac).First(&client).Error; err != nil {
		return nil, err
	}

	filenames := make([]string, len(client.Images))
	for i, img := range client.Images {
		filenames[i] = img.Filename
	}
	return filenames, nil
}

func (s *PostgresStore) GetImagesForClient(macAddress string) ([]models.Image, error) {
	var images []models.Image

	if err := s.db.Where("enabled = ? AND public = ?", true, true).Find(&images).Error; err != nil {
		return nil, err
	}

	var client models.Client
	if err := s.db.Where("mac_address = ? AND enabled = ?", macAddress, true).
		Preload("Images", "enabled = ?", true).
		First(&client).Error; err == nil {
		images = append(images, client.Images...)
	}

	return images, nil
}


func (s *PostgresStore) EnsureAdminUser() (username, password string, created bool, err error) {
	var admin models.User
	err = s.db.Where("username = ?", "admin").First(&admin).Error

	if err == gorm.ErrRecordNotFound {
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

func (s *PostgresStore) ResetAdminPassword() (string, error) {
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

func (s *PostgresStore) GetUser(username string) (*models.User, error) {
	var user models.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *PostgresStore) UpdateUserLastLogin(username string) error {
	now := time.Now()
	return s.db.Model(&models.User{}).Where("username = ?", username).Update("last_login", now).Error
}

func (s *PostgresStore) ListUsers() ([]*models.User, error) {
	var users []*models.User
	if err := s.db.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (s *PostgresStore) CreateUser(user *models.User) error {
	return s.db.Create(user).Error
}

func (s *PostgresStore) UpdateUser(username string, user *models.User) error {
	return s.db.Model(&models.User{}).Where("username = ?", username).Updates(user).Error
}

func (s *PostgresStore) DeleteUser(username string) error {
	return s.db.Where("username = ?", username).Delete(&models.User{}).Error
}


func (s *PostgresStore) ListCustomFiles() ([]*models.CustomFile, error) {
	var files []*models.CustomFile
	if err := s.db.Preload("Image").Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

func (s *PostgresStore) GetCustomFileByFilename(filename string) (*models.CustomFile, error) {
	var file models.CustomFile
	if err := s.db.Preload("Image").Where("filename = ?", filename).First(&file).Error; err != nil {
		return nil, err
	}
	return &file, nil
}

func (s *PostgresStore) GetCustomFileByID(id uint) (*models.CustomFile, error) {
	var file models.CustomFile
	if err := s.db.Preload("Image").First(&file, id).Error; err != nil {
		return nil, err
	}
	return &file, nil
}

func (s *PostgresStore) CreateCustomFile(file *models.CustomFile) error {
	return s.db.Create(file).Error
}

func (s *PostgresStore) UpdateCustomFile(id uint, file *models.CustomFile) error {
	return s.db.Model(&models.CustomFile{}).Where("id = ?", id).Updates(file).Error
}

func (s *PostgresStore) DeleteCustomFile(id uint) error {
	return s.db.Delete(&models.CustomFile{}, id).Error
}

func (s *PostgresStore) IncrementFileDownloadCount(id uint) error {
	now := time.Now()
	return s.db.Model(&models.CustomFile{}).Where("id = ?", id).Updates(map[string]interface{}{
		"download_count": gorm.Expr("download_count + 1"),
		"last_download":  now,
	}).Error
}

func (s *PostgresStore) ListCustomFilesByImage(imageID uint) ([]*models.CustomFile, error) {
	var files []*models.CustomFile
	if err := s.db.Preload("Image").Where("image_id = ?", imageID).Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

func (s *PostgresStore) ListDriverPacks() ([]*models.DriverPack, error) {
	var packs []*models.DriverPack
	if err := s.db.Preload("Image").Find(&packs).Error; err != nil {
		return nil, err
	}
	return packs, nil
}

func (s *PostgresStore) GetDriverPack(id uint) (*models.DriverPack, error) {
	var pack models.DriverPack
	if err := s.db.Preload("Image").First(&pack, id).Error; err != nil {
		return nil, err
	}
	return &pack, nil
}

func (s *PostgresStore) CreateDriverPack(pack *models.DriverPack) error {
	return s.db.Create(pack).Error
}

func (s *PostgresStore) UpdateDriverPack(id uint, pack *models.DriverPack) error {
	return s.db.Model(&models.DriverPack{}).Where("id = ?", id).Save(pack).Error
}

func (s *PostgresStore) DeleteDriverPack(id uint) error {
	return s.db.Delete(&models.DriverPack{}, id).Error
}

func (s *PostgresStore) ListDriverPacksByImage(imageID uint) ([]*models.DriverPack, error) {
	var packs []*models.DriverPack
	if err := s.db.Preload("Image").Where("image_id = ? AND enabled = ?", imageID, true).Find(&packs).Error; err != nil {
		return nil, err
	}
	return packs, nil
}

func (s *PostgresStore) ListImageGroups() ([]*models.ImageGroup, error) {
	var groups []*models.ImageGroup
	if err := s.db.Preload("Parent").Order("\"order\" ASC, name ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *PostgresStore) GetImageGroup(id uint) (*models.ImageGroup, error) {
	var group models.ImageGroup
	if err := s.db.Preload("Parent").First(&group, id).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func (s *PostgresStore) GetImageGroupByName(name string) (*models.ImageGroup, error) {
	var group models.ImageGroup
	if err := s.db.Preload("Parent").Where("name = ?", name).First(&group).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func (s *PostgresStore) CreateImageGroup(group *models.ImageGroup) error {
	return s.db.Create(group).Error
}

func (s *PostgresStore) UpdateImageGroup(id uint, group *models.ImageGroup) error {
	return s.db.Model(&models.ImageGroup{}).Where("id = ?", id).Save(group).Error
}

func (s *PostgresStore) DeleteImageGroup(id uint) error {
	return s.db.Delete(&models.ImageGroup{}, id).Error
}

func (s *PostgresStore) ListImagesByGroup(groupID uint) ([]*models.Image, error) {
	var images []*models.Image
	if err := s.db.Preload("Group").Where("group_id = ? AND enabled = ?", groupID, true).Order("\"order\" ASC, name ASC").Find(&images).Error; err != nil {
		return nil, err
	}
	return images, nil
}


func (s *PostgresStore) LogBootAttempt(macAddress, imageName, ipAddress string, success bool, errorMsg string) error {
	bootLog := models.BootLog{
		MACAddress: macAddress,
		ImageName:  imageName,
		IPAddress:  ipAddress,
		Success:    success,
		ErrorMsg:   errorMsg,
	}

	var client models.Client
	if err := s.db.Where("mac_address = ?", macAddress).First(&client).Error; err == nil {
		bootLog.ClientID = &client.ID
	}

	var image models.Image
	if err := s.db.Where("name = ?", imageName).First(&image).Error; err == nil {
		bootLog.ImageID = &image.ID
	}

	return s.db.Create(&bootLog).Error
}

func (s *PostgresStore) UpdateClientBootStats(macAddress string) error {
	now := time.Now()
	return s.db.Model(&models.Client{}).
		Where("mac_address = ?", macAddress).
		Updates(map[string]interface{}{
			"last_boot":  now,
			"boot_count": gorm.Expr("boot_count + 1"),
		}).Error
}

func (s *PostgresStore) UpdateImageBootStats(imageName string) error {
	now := time.Now()
	return s.db.Model(&models.Image{}).
		Where("name = ?", imageName).
		Updates(map[string]interface{}{
			"last_booted": now,
			"boot_count":  gorm.Expr("boot_count + 1"),
		}).Error
}

func (s *PostgresStore) GetBootLogs(limit int) ([]models.BootLog, error) {
	var logs []models.BootLog
	if err := s.db.Preload("Client").Preload("Image").
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}


func (s *PostgresStore) GetStats() (map[string]int64, error) {
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
