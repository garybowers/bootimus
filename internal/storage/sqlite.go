package storage

import (
	"fmt"
	"path/filepath"

	"bootimus/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

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

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) AutoMigrate() error {
	if err := s.db.AutoMigrate(&models.User{}, &models.Client{}, &models.ImageGroup{}, &models.Image{}, &models.BootLog{}, &models.CustomFile{}, &models.DriverPack{}); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}

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
	return s.db.Unscoped().Where("filename = ?", filename).Delete(&models.Image{}).Error
}

func (s *SQLiteStore) AssignImagesToClient(mac string, imageFilenames []string) error {
	var client models.Client
	if err := s.db.Where("mac_address = ?", mac).First(&client).Error; err != nil {
		return err
	}

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

func (s *SQLiteStore) EnsureAdminUser() (username, password string, created bool, err error) {
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

func (s *SQLiteStore) ListCustomFiles() ([]*models.CustomFile, error) {
	var files []*models.CustomFile
	if err := s.db.Preload("Image").Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

func (s *SQLiteStore) GetCustomFileByFilename(filename string) (*models.CustomFile, error) {
	var file models.CustomFile
	if err := s.db.Preload("Image").Where("filename = ?", filename).First(&file).Error; err != nil {
		return nil, err
	}
	return &file, nil
}

func (s *SQLiteStore) GetCustomFileByID(id uint) (*models.CustomFile, error) {
	var file models.CustomFile
	if err := s.db.Preload("Image").First(&file, id).Error; err != nil {
		return nil, err
	}
	return &file, nil
}

func (s *SQLiteStore) CreateCustomFile(file *models.CustomFile) error {
	return s.db.Create(file).Error
}

func (s *SQLiteStore) UpdateCustomFile(id uint, file *models.CustomFile) error {
	return s.db.Model(&models.CustomFile{}).Where("id = ?", id).Save(file).Error
}

func (s *SQLiteStore) DeleteCustomFile(id uint) error {
	return s.db.Delete(&models.CustomFile{}, id).Error
}

func (s *SQLiteStore) IncrementFileDownloadCount(id uint) error {
	return s.db.Model(&models.CustomFile{}).Where("id = ?", id).Updates(map[string]interface{}{
		"download_count": gorm.Expr("download_count + 1"),
		"last_download":  gorm.Expr("CURRENT_TIMESTAMP"),
	}).Error
}

func (s *SQLiteStore) ListCustomFilesByImage(imageID uint) ([]*models.CustomFile, error) {
	var files []*models.CustomFile
	if err := s.db.Preload("Image").Where("image_id = ?", imageID).Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

func (s *SQLiteStore) ListDriverPacks() ([]*models.DriverPack, error) {
	var packs []*models.DriverPack
	if err := s.db.Preload("Image").Find(&packs).Error; err != nil {
		return nil, err
	}
	return packs, nil
}

func (s *SQLiteStore) GetDriverPack(id uint) (*models.DriverPack, error) {
	var pack models.DriverPack
	if err := s.db.Preload("Image").First(&pack, id).Error; err != nil {
		return nil, err
	}
	return &pack, nil
}

func (s *SQLiteStore) CreateDriverPack(pack *models.DriverPack) error {
	return s.db.Create(pack).Error
}

func (s *SQLiteStore) UpdateDriverPack(id uint, pack *models.DriverPack) error {
	return s.db.Model(&models.DriverPack{}).Where("id = ?", id).Save(pack).Error
}

func (s *SQLiteStore) DeleteDriverPack(id uint) error {
	return s.db.Delete(&models.DriverPack{}, id).Error
}

func (s *SQLiteStore) ListDriverPacksByImage(imageID uint) ([]*models.DriverPack, error) {
	var packs []*models.DriverPack
	if err := s.db.Preload("Image").Where("image_id = ? AND enabled = ?", imageID, true).Find(&packs).Error; err != nil {
		return nil, err
	}
	return packs, nil
}

func (s *SQLiteStore) ListImageGroups() ([]*models.ImageGroup, error) {
	var groups []*models.ImageGroup
	if err := s.db.Preload("Parent").Order("`order` ASC, name ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *SQLiteStore) GetImageGroup(id uint) (*models.ImageGroup, error) {
	var group models.ImageGroup
	if err := s.db.Preload("Parent").First(&group, id).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func (s *SQLiteStore) GetImageGroupByName(name string) (*models.ImageGroup, error) {
	var group models.ImageGroup
	if err := s.db.Preload("Parent").Where("name = ?", name).First(&group).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func (s *SQLiteStore) CreateImageGroup(group *models.ImageGroup) error {
	return s.db.Create(group).Error
}

func (s *SQLiteStore) UpdateImageGroup(id uint, group *models.ImageGroup) error {
	return s.db.Model(&models.ImageGroup{}).Where("id = ?", id).Save(group).Error
}

func (s *SQLiteStore) DeleteImageGroup(id uint) error {
	return s.db.Delete(&models.ImageGroup{}, id).Error
}

func (s *SQLiteStore) ListImagesByGroup(groupID uint) ([]*models.Image, error) {
	var images []*models.Image
	if err := s.db.Preload("Group").Where("group_id = ? AND enabled = ?", groupID, true).Order("`order` ASC, name ASC").Find(&images).Error; err != nil {
		return nil, err
	}
	return images, nil
}

func (s *SQLiteStore) GetImagesForClient(macAddress string) ([]models.Image, error) {
	var images []models.Image

	if err := s.db.Where("enabled = ? AND public = ?", true, true).Find(&images).Error; err != nil {
		return nil, err
	}

	var client models.Client
	if err := s.db.Where("mac_address = ? AND enabled = ?", macAddress, true).First(&client).Error; err == nil {
		if len(client.AllowedImages) > 0 {
			var clientImages []models.Image
			if err := s.db.Where("filename IN ? AND enabled = ?", client.AllowedImages, true).Find(&clientImages).Error; err == nil {
				images = append(images, clientImages...)
			}
		}
	}

	return images, nil
}

func (s *SQLiteStore) LogBootAttempt(macAddress, imageName, ipAddress string, success bool, errorMsg string) error {
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

func (s *SQLiteStore) UpdateClientBootStats(macAddress string) error {
	return s.db.Model(&models.Client{}).
		Where("mac_address = ?", macAddress).
		Updates(map[string]interface{}{
			"last_boot":  gorm.Expr("CURRENT_TIMESTAMP"),
			"boot_count": gorm.Expr("boot_count + 1"),
		}).Error
}

func (s *SQLiteStore) UpdateImageBootStats(imageName string) error {
	return s.db.Model(&models.Image{}).
		Where("name = ?", imageName).
		Updates(map[string]interface{}{
			"last_booted": gorm.Expr("CURRENT_TIMESTAMP"),
			"boot_count":  gorm.Expr("boot_count + 1"),
		}).Error
}

func (s *SQLiteStore) SyncImages(isoFiles []struct{ Name, Filename string; Size int64 }) error {
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

func (s *SQLiteStore) Close() error {
	db, err := s.db.DB()
	if err != nil {
		return err
	}
	return db.Close()
}
