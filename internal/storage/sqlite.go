package storage

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

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
	if err := s.db.AutoMigrate(&models.User{}, &models.Client{}, &models.ImageGroup{}, &models.Image{}, &models.BootLog{}, &models.CustomFile{}, &models.DriverPack{}, &models.MenuTheme{}, &models.BootTool{}, &models.HardwareInventory{}, &models.DistroProfile{}); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Clean up soft-deleted custom files
	if err := s.cleanupSoftDeletedFiles(); err != nil {
		return fmt.Errorf("failed to cleanup soft-deleted files: %w", err)
	}

	return nil
}

func (s *SQLiteStore) cleanupSoftDeletedFiles() error {
	result := s.db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&models.CustomFile{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		fmt.Printf("Cleaned up %d soft-deleted custom files from database\n", result.RowsAffected)
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
	return s.db.Model(&models.Client{}).Where("mac_address = ?", mac).
		Select("Name", "Description", "Enabled", "ShowPublicImages", "BootloaderSet", "Static", "UpdatedAt").
		Updates(client).Error
}

func (s *SQLiteStore) DeleteClient(mac string) error {
	return s.db.Where("mac_address = ?", mac).Delete(&models.Client{}).Error
}

func (s *SQLiteStore) ListImages() ([]*models.Image, error) {
	var images []*models.Image
	if err := s.db.Preload("Group").Find(&images).Error; err != nil {
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
	var image models.Image
	if err := s.db.Where("filename = ?", filename).First(&image).Error; err != nil {
		return err
	}
	s.db.Exec("DELETE FROM client_images WHERE image_id = ?", image.ID)
	s.db.Unscoped().Where("image_id = ?", image.ID).Delete(&models.CustomFile{})
	s.db.Unscoped().Where("image_id = ?", image.ID).Delete(&models.BootLog{})
	return s.db.Unscoped().Delete(&image).Error
}

func (s *SQLiteStore) AssignImagesToClient(mac string, imageFilenames []string) error {
	var client models.Client
	if err := s.db.Where("mac_address = ?", mac).First(&client).Error; err != nil {
		return err
	}

	allowed := models.StringSlice(imageFilenames)
	return s.db.Model(&client).Select("AllowedImages").Updates(map[string]interface{}{
		"allowed_images": allowed,
	}).Error
}

func (s *SQLiteStore) SetNextBootImage(mac string, imageFilename string) error {
	return s.db.Model(&models.Client{}).Where("mac_address = ?", mac).
		Update("next_boot_image", imageFilename).Error
}

func (s *SQLiteStore) ClearNextBootImage(mac string) error {
	return s.db.Model(&models.Client{}).Where("mac_address = ?", mac).
		Update("next_boot_image", "").Error
}

func (s *SQLiteStore) GetClientImages(mac string) ([]string, error) {
	var client models.Client
	if err := s.db.Where("mac_address = ?", mac).First(&client).Error; err != nil {
		return nil, err
	}
	return client.AllowedImages, nil
}

func (s *SQLiteStore) SaveHardwareInventory(inv *models.HardwareInventory) error {
	if inv.MACAddress != "" {
		var client models.Client
		if err := s.db.Where("mac_address = ?", inv.MACAddress).First(&client).Error; err == nil {
			inv.ClientID = &client.ID
		} else {
			// Check for soft-deleted client and restore it
			var deleted models.Client
			if err := s.db.Unscoped().Where("mac_address = ? AND deleted_at IS NOT NULL", inv.MACAddress).First(&deleted).Error; err == nil {
				deleted.DeletedAt = gorm.DeletedAt{}
				deleted.Enabled = true
				deleted.ShowPublicImages = true
				deleted.Static = false
				s.db.Unscoped().Save(&deleted)
				inv.ClientID = &deleted.ID
				log.Printf("Storage: Restored soft-deleted client for MAC %s", inv.MACAddress)
			} else {
				// Auto-create a dynamic (discovered) client
				client = models.Client{
					MACAddress:       inv.MACAddress,
					Enabled:          true,
					ShowPublicImages: true,
					Static:           false,
				}
				if err := s.db.Create(&client).Error; err == nil {
					inv.ClientID = &client.ID
					log.Printf("Storage: Auto-created dynamic client for MAC %s", inv.MACAddress)
				}
			}
		}
	}
	return s.db.Create(inv).Error
}

func (s *SQLiteStore) GetLatestHardwareInventory(mac string) (*models.HardwareInventory, error) {
	var inv models.HardwareInventory
	if err := s.db.Where("mac_address = ?", mac).Order("created_at DESC").First(&inv).Error; err != nil {
		return nil, err
	}
	return &inv, nil
}

func (s *SQLiteStore) GetHardwareInventoryHistory(mac string, limit int) ([]models.HardwareInventory, error) {
	var history []models.HardwareInventory
	if err := s.db.Where("mac_address = ?", mac).Order("created_at DESC").Limit(limit).Find(&history).Error; err != nil {
		return nil, err
	}
	return history, nil
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

func (s *SQLiteStore) GetCustomFileByFilenameAndImage(filename string, imageID *uint, public bool) (*models.CustomFile, error) {
	var files []models.CustomFile

	// Find ALL records with this filename, regardless of public/imageID/deleted status
	// This ensures we catch any record that would violate the unique constraint
	if err := s.db.Unscoped().Where("filename = ?", filename).Find(&files).Error; err != nil {
		return nil, err
	}

	// Delete all found records to avoid conflicts
	if len(files) > 0 {
		for _, f := range files {
			s.db.Unscoped().Delete(&models.CustomFile{}, f.ID)
		}
		// Return the first one so the caller knows a file existed
		return &files[0], nil
	}

	return nil, fmt.Errorf("record not found")
}

func (s *SQLiteStore) CreateCustomFile(file *models.CustomFile) error {
	return s.db.Create(file).Error
}

func (s *SQLiteStore) UpdateCustomFile(id uint, file *models.CustomFile) error {
	return s.db.Model(&models.CustomFile{}).Where("id = ?", id).Save(file).Error
}

func (s *SQLiteStore) DeleteCustomFile(id uint) error {
	return s.db.Unscoped().Delete(&models.CustomFile{}, id).Error
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
	// Check for a soft-deleted group with the same name and parent — undelete it
	var existing models.ImageGroup
	q := s.db.Unscoped().Where("name = ?", group.Name)
	if group.ParentID != nil {
		q = q.Where("parent_id = ?", *group.ParentID)
	} else {
		q = q.Where("parent_id IS NULL")
	}
	if err := q.Where("deleted_at IS NOT NULL").First(&existing).Error; err == nil {
		existing.DeletedAt = gorm.DeletedAt{}
		existing.Description = group.Description
		existing.Order = group.Order
		existing.Enabled = group.Enabled
		if err := s.db.Unscoped().Save(&existing).Error; err != nil {
			return err
		}
		group.ID = existing.ID
		return nil
	}
	return s.db.Create(group).Error
}

func (s *SQLiteStore) UpdateImageGroup(id uint, group *models.ImageGroup) error {
	return s.db.Model(&models.ImageGroup{}).Where("id = ?", id).Save(group).Error
}

func (s *SQLiteStore) DeleteImageGroup(id uint) error {
	return s.db.Unscoped().Delete(&models.ImageGroup{}, id).Error
}

func (s *SQLiteStore) ListImagesByGroup(groupID uint) ([]*models.Image, error) {
	var images []*models.Image
	if err := s.db.Preload("Group").Where("group_id = ? AND enabled = ?", groupID, true).Order("`order` ASC, name ASC").Find(&images).Error; err != nil {
		return nil, err
	}
	return images, nil
}

func (s *SQLiteStore) GetImagesForClient(macAddress string) ([]models.Image, error) {
	var client models.Client
	if err := s.db.Where("mac_address = ? AND enabled = ?", macAddress, true).First(&client).Error; err == nil {
		log.Printf("GetImagesForClient: client=%s, AllowedImages=%v, ShowPublicImages=%v", macAddress, client.AllowedImages, client.ShowPublicImages)
		var assigned []models.Image
		if len(client.AllowedImages) > 0 {
			s.db.Where("filename IN (?) AND enabled = ?", []string(client.AllowedImages), true).Find(&assigned)
			if len(assigned) == 0 {
				// Debug: list all image filenames to find the mismatch
				var allImages []models.Image
				s.db.Select("filename").Find(&allImages)
				var fnames []string
				for _, img := range allImages {
					fnames = append(fnames, img.Filename)
				}
				log.Printf("GetImagesForClient: 0 matches for %v. DB filenames: %v", client.AllowedImages, fnames)
			} else {
				log.Printf("GetImagesForClient: found %d assigned images for %v", len(assigned), client.AllowedImages)
			}
		}

		// If client has ShowPublicImages enabled, also include public images
		if client.ShowPublicImages {
			var publicImages []models.Image
			s.db.Where("enabled = ? AND public = ?", true, true).Find(&publicImages)

			// Merge, avoiding duplicates
			seen := make(map[string]bool)
			for _, img := range assigned {
				seen[img.Filename] = true
			}
			for _, img := range publicImages {
				if !seen[img.Filename] {
					assigned = append(assigned, img)
				}
			}
		}

		if len(assigned) > 0 {
			return assigned, nil
		}

		// Client exists but has no assigned images and public images are off
		if !client.ShowPublicImages {
			return []models.Image{}, nil
		}
	}

	// Unknown client — show all public images
	var images []models.Image
	if err := s.db.Where("enabled = ? AND public = ?", true, true).Find(&images).Error; err != nil {
		return nil, err
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

func (s *SQLiteStore) SyncImages(isoFiles []models.SyncFile) error {
	groupCache := make(map[string]*uint) // groupPath -> groupID

	for _, iso := range isoFiles {
		var groupID *uint
		if iso.GroupPath != "" {
			gid, err := s.resolveGroupPath(iso.GroupPath, groupCache)
			if err != nil {
				return fmt.Errorf("failed to resolve group for %s: %w", iso.GroupPath, err)
			}
			groupID = gid
		}

		var image models.Image
		err := s.db.Where("filename = ?", iso.Filename).First(&image).Error

		if err == gorm.ErrRecordNotFound {
			image = models.Image{
				Name:     iso.Name,
				Filename: iso.Filename,
				Size:     iso.Size,
				Enabled:  true,
				Public:   true,
				GroupID:  groupID,
			}
			if err := s.db.Create(&image).Error; err != nil {
				return fmt.Errorf("failed to create image %s: %w", iso.Name, err)
			}
		} else if err == nil {
			updates := map[string]interface{}{}
			if image.Size != iso.Size {
				updates["size"] = iso.Size
			}
			if groupID != nil {
				updates["group_id"] = *groupID
			}
			if len(updates) > 0 {
				s.db.Model(&image).Updates(updates)
			}
		} else {
			return err
		}
	}

	return nil
}

// resolveGroupPath creates ImageGroups for each segment of a path (e.g. "linux/debian")
// and returns the ID of the leaf group. Results are cached in groupCache.
func (s *SQLiteStore) resolveGroupPath(groupPath string, cache map[string]*uint) (*uint, error) {
	if id, ok := cache[groupPath]; ok {
		return id, nil
	}

	segments := strings.Split(groupPath, string(filepath.Separator))
	var parentID *uint
	builtPath := ""

	for _, seg := range segments {
		if builtPath == "" {
			builtPath = seg
		} else {
			builtPath = builtPath + string(filepath.Separator) + seg
		}

		if id, ok := cache[builtPath]; ok {
			parentID = id
			continue
		}

		var group models.ImageGroup
		query := s.db.Where("name = ?", seg)
		if parentID != nil {
			query = query.Where("parent_id = ?", *parentID)
		} else {
			query = query.Where("parent_id IS NULL")
		}

		err := query.First(&group).Error
		if err == gorm.ErrRecordNotFound {
			group = models.ImageGroup{
				Name:     seg,
				ParentID: parentID,
				Enabled:  true,
			}
			if err := s.db.Create(&group).Error; err != nil {
				return nil, fmt.Errorf("failed to create group %s: %w", seg, err)
			}
		} else if err != nil {
			return nil, err
		}

		id := group.ID
		cache[builtPath] = &id
		parentID = &id
	}

	return parentID, nil
}

func (s *SQLiteStore) GetMenuTheme() (*models.MenuTheme, error) {
	var theme models.MenuTheme
	err := s.db.First(&theme).Error
	if err == gorm.ErrRecordNotFound {
		theme = models.MenuTheme{ID: 1}
		if err := s.db.Create(&theme).Error; err != nil {
			return nil, err
		}
		return &theme, nil
	}
	if err != nil {
		return nil, err
	}
	return &theme, nil
}

func (s *SQLiteStore) UpdateMenuTheme(theme *models.MenuTheme) error {
	theme.ID = 1
	return s.db.Save(theme).Error
}

func (s *SQLiteStore) ListBootTools() ([]*models.BootTool, error) {
	var tools []*models.BootTool
	if err := s.db.Order("`order` ASC, name ASC").Find(&tools).Error; err != nil {
		return nil, err
	}
	return tools, nil
}

func (s *SQLiteStore) GetBootTool(name string) (*models.BootTool, error) {
	var tool models.BootTool
	if err := s.db.Where("name = ?", name).First(&tool).Error; err != nil {
		return nil, err
	}
	return &tool, nil
}

func (s *SQLiteStore) SaveBootTool(tool *models.BootTool) error {
	return s.db.Save(tool).Error
}

func (s *SQLiteStore) ListDistroProfiles() ([]*models.DistroProfile, error) {
	var profiles []*models.DistroProfile
	if err := s.db.Order("display_name ASC").Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

func (s *SQLiteStore) GetDistroProfile(profileID string) (*models.DistroProfile, error) {
	var profile models.DistroProfile
	if err := s.db.Where("profile_id = ?", profileID).First(&profile).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}

func (s *SQLiteStore) SaveDistroProfile(profile *models.DistroProfile) error {
	return s.db.Save(profile).Error
}

func (s *SQLiteStore) DeleteDistroProfile(profileID string) error {
	return s.db.Unscoped().Where("profile_id = ?", profileID).Delete(&models.DistroProfile{}).Error
}

func (s *SQLiteStore) DeleteBootTool(name string) error {
	return s.db.Unscoped().Where("name = ?", name).Delete(&models.BootTool{}).Error
}

func (s *SQLiteStore) Close() error {
	db, err := s.db.DB()
	if err != nil {
		return err
	}
	return db.Close()
}
