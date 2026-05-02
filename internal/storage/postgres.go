package storage

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
	db  *gorm.DB
	cfg Config
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

	return &PostgresStore{db: db, cfg: *cfg}, nil
}

func (s *PostgresStore) Snapshot(w io.Writer) (string, error) {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		return "", fmt.Errorf("pg_dump not found on PATH: %w (install postgresql-client in your image)", err)
	}

	cmd := exec.Command("pg_dump",
		"--host="+s.cfg.Host,
		"--port="+strconv.Itoa(s.cfg.Port),
		"--username="+s.cfg.User,
		"--dbname="+s.cfg.DBName,
		"--no-owner",
		"--no-privileges",
		"--clean",
		"--if-exists",
		"--format=plain",
	)
	cmd.Env = append(cmd.Environ(),
		"PGPASSWORD="+s.cfg.Password,
		"PGSSLMODE="+s.cfg.SSLMode,
	)
	cmd.Stdout = w
	stderr := &strings.Builder{}
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pg_dump failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return "bootimus.sql", nil
}

func (s *PostgresStore) AutoMigrate() error {
	log.Println("Running PostgreSQL database migrations...")

	if err := s.db.AutoMigrate(
		&models.User{},
		&models.ClientGroup{},
		&models.Client{},
		&models.ImageGroup{},
		&models.Image{},
		&models.BootLog{},
		&models.CustomFile{},
		&models.DriverPack{},
		&models.MenuTheme{},
		&models.BootTool{},
		&models.HardwareInventory{},
		&models.DistroProfile{},
		&models.WebhookConfig{},
		&models.ScheduledTask{},
	); err != nil {
		return err
	}

	if err := s.migrateCustomFileUniqueIndex(); err != nil {
		log.Printf("Warning: CustomFile index migration failed (may already be migrated): %v", err)
	}

	if err := s.cleanupSoftDeletedFiles(); err != nil {
		log.Printf("Warning: Failed to cleanup soft-deleted files: %v", err)
	}

	return nil
}

func (s *PostgresStore) cleanupSoftDeletedFiles() error {
	result := s.db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&models.CustomFile{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		log.Printf("Cleaned up %d soft-deleted custom files from database", result.RowsAffected)
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

func (s *PostgresStore) ListBootTools() ([]*models.BootTool, error) {
	var tools []*models.BootTool
	if err := s.db.Order("\"order\" ASC, name ASC").Find(&tools).Error; err != nil {
		return nil, err
	}
	return tools, nil
}

func (s *PostgresStore) GetBootTool(name string) (*models.BootTool, error) {
	var tool models.BootTool
	if err := s.db.Where("name = ?", name).First(&tool).Error; err != nil {
		return nil, err
	}
	return &tool, nil
}

func (s *PostgresStore) SaveBootTool(tool *models.BootTool) error {
	return s.db.Save(tool).Error
}

func (s *PostgresStore) ListDistroProfiles() ([]*models.DistroProfile, error) {
	var profiles []*models.DistroProfile
	if err := s.db.Order("display_name ASC").Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

func (s *PostgresStore) GetDistroProfile(profileID string) (*models.DistroProfile, error) {
	var profile models.DistroProfile
	if err := s.db.Where("profile_id = ?", profileID).First(&profile).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}

func (s *PostgresStore) SaveDistroProfile(profile *models.DistroProfile) error {
	return s.db.Save(profile).Error
}

func (s *PostgresStore) DeleteDistroProfile(profileID string) error {
	return s.db.Unscoped().Where("profile_id = ?", profileID).Delete(&models.DistroProfile{}).Error
}

func (s *PostgresStore) DeleteBootTool(name string) error {
	return s.db.Unscoped().Where("name = ?", name).Delete(&models.BootTool{}).Error
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
	return s.db.Model(&models.Client{}).Where("mac_address = ?", mac).
		Select("Name", "Description", "Enabled", "ShowPublicImages", "BootloaderSet", "Static", "ClientGroupID",
			"IPMIHost", "IPMIPort", "IPMIUsername", "IPMIPassword", "IPMIInsecure", "UpdatedAt").
		Updates(client).Error
}

func (s *PostgresStore) DeleteClient(mac string) error {
	return s.db.Where("mac_address = ?", mac).Delete(&models.Client{}).Error
}


func (s *PostgresStore) ListImages() ([]*models.Image, error) {
	var images []*models.Image
	if err := s.db.Preload("Group").Find(&images).Error; err != nil {
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
	return s.db.Model(&models.Image{}).Where("filename = ?", filename).Save(image).Error
}

func (s *PostgresStore) DeleteImage(filename string) error {
	var image models.Image
	if err := s.db.Where("filename = ?", filename).First(&image).Error; err != nil {
		return err
	}
	s.db.Exec("DELETE FROM client_images WHERE image_id = ?", image.ID)
	s.db.Unscoped().Where("image_id = ?", image.ID).Delete(&models.CustomFile{})
	s.db.Unscoped().Where("image_id = ?", image.ID).Delete(&models.BootLog{})
	return s.db.Unscoped().Delete(&image).Error
}

func (s *PostgresStore) SyncImages(isoFiles []models.SyncFile) error {
	groupCache := make(map[string]*uint)

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

func (s *PostgresStore) resolveGroupPath(groupPath string, cache map[string]*uint) (*uint, error) {
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


func (s *PostgresStore) AssignImagesToClient(mac string, imageFilenames []string) error {
	var client models.Client
	if err := s.db.Where("mac_address = ?", mac).First(&client).Error; err != nil {
		return err
	}

	allowed := models.StringSlice(imageFilenames)
	return s.db.Model(&client).Select("AllowedImages").Updates(map[string]interface{}{
		"allowed_images": allowed,
	}).Error
}

func (s *PostgresStore) SetNextBootImage(mac string, imageFilename string) error {
	return s.db.Model(&models.Client{}).Where("mac_address = ?", mac).
		Update("next_boot_image", imageFilename).Error
}

func (s *PostgresStore) ClearNextBootImage(mac string) error {
	return s.db.Model(&models.Client{}).Where("mac_address = ?", mac).
		Update("next_boot_image", "").Error
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
	var client models.Client
	if err := s.db.Where("mac_address = ? AND enabled = ?", macAddress, true).First(&client).Error; err == nil {
		allowed := append([]string{}, client.AllowedImages...)
		if client.ClientGroupID != nil {
			var group models.ClientGroup
			if err := s.db.Where("id = ? AND enabled = ?", *client.ClientGroupID, true).First(&group).Error; err == nil {
				seen := make(map[string]bool, len(allowed))
				for _, f := range allowed {
					seen[f] = true
				}
				for _, f := range group.AllowedImages {
					if !seen[f] {
						allowed = append(allowed, f)
						seen[f] = true
					}
				}
			}
		}
		log.Printf("GetImagesForClient: client=%s, AllowedImages=%v, ShowPublicImages=%v", macAddress, allowed, client.ShowPublicImages)
		var assigned []models.Image
		if len(allowed) > 0 {
			s.db.Where("filename IN (?) AND enabled = ?", allowed, true).Find(&assigned)
			log.Printf("GetImagesForClient: found %d assigned images for %v", len(assigned), allowed)
		}

		if client.ShowPublicImages {
			var publicImages []models.Image
			s.db.Where("enabled = ? AND public = ?", true, true).Find(&publicImages)

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

		if !client.ShowPublicImages {
			return []models.Image{}, nil
		}
	}

	var images []models.Image
	if err := s.db.Where("enabled = ? AND public = ?", true, true).Find(&images).Error; err != nil {
		return nil, err
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

func (s *PostgresStore) GetCustomFileByFilenameAndImage(filename string, imageID *uint, public bool) (*models.CustomFile, error) {
	var files []models.CustomFile

	if err := s.db.Unscoped().Where("filename = ?", filename).Find(&files).Error; err != nil {
		return nil, err
	}

	if len(files) > 0 {
		for _, f := range files {
			s.db.Unscoped().Delete(&models.CustomFile{}, f.ID)
		}
		return &files[0], nil
	}

	return nil, fmt.Errorf("record not found")
}

func (s *PostgresStore) CreateCustomFile(file *models.CustomFile) error {
	return s.db.Create(file).Error
}

func (s *PostgresStore) UpdateCustomFile(id uint, file *models.CustomFile) error {
	return s.db.Model(&models.CustomFile{}).Where("id = ?", id).Updates(file).Error
}

func (s *PostgresStore) DeleteCustomFile(id uint) error {
	return s.db.Unscoped().Delete(&models.CustomFile{}, id).Error
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

func (s *PostgresStore) UpdateImageGroup(id uint, group *models.ImageGroup) error {
	return s.db.Model(&models.ImageGroup{}).Where("id = ?", id).Save(group).Error
}

func (s *PostgresStore) DeleteImageGroup(id uint) error {
	return s.db.Unscoped().Delete(&models.ImageGroup{}, id).Error
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

func (s *PostgresStore) GetBootLogsByMAC(macAddress string, limit int) ([]models.BootLog, error) {
	var logs []models.BootLog
	if err := s.db.Preload("Client").Preload("Image").
		Where("mac_address = ?", macAddress).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}


func (s *PostgresStore) SaveHardwareInventory(inv *models.HardwareInventory) error {
	if inv.MACAddress != "" {
		var client models.Client
		if err := s.db.Where("mac_address = ?", inv.MACAddress).First(&client).Error; err == nil {
			inv.ClientID = &client.ID
		} else {
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

func (s *PostgresStore) GetLatestHardwareInventory(mac string) (*models.HardwareInventory, error) {
	var inv models.HardwareInventory
	if err := s.db.Where("mac_address = ?", mac).Order("created_at DESC").First(&inv).Error; err != nil {
		return nil, err
	}
	return &inv, nil
}

func (s *PostgresStore) GetHardwareInventoryHistory(mac string, limit int) ([]models.HardwareInventory, error) {
	var history []models.HardwareInventory
	if err := s.db.Where("mac_address = ?", mac).Order("created_at DESC").Limit(limit).Find(&history).Error; err != nil {
		return nil, err
	}
	return history, nil
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

func (s *PostgresStore) GetMenuTheme() (*models.MenuTheme, error) {
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

func (s *PostgresStore) UpdateMenuTheme(theme *models.MenuTheme) error {
	theme.ID = 1
	return s.db.Save(theme).Error
}

func (s *PostgresStore) ListScheduledTasks() ([]*models.ScheduledTask, error) {
	var tasks []*models.ScheduledTask
	if err := s.db.Preload("ClientGroup").Order("name ASC").Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *PostgresStore) ListScheduledTasksByGroup(groupID uint) ([]*models.ScheduledTask, error) {
	var tasks []*models.ScheduledTask
	if err := s.db.Where("client_group_id = ?", groupID).Order("name ASC").Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *PostgresStore) GetScheduledTask(id uint) (*models.ScheduledTask, error) {
	var t models.ScheduledTask
	if err := s.db.Preload("ClientGroup").First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *PostgresStore) CreateScheduledTask(task *models.ScheduledTask) error {
	return s.db.Create(task).Error
}

func (s *PostgresStore) UpdateScheduledTask(id uint, task *models.ScheduledTask) error {
	return s.db.Model(&models.ScheduledTask{}).Where("id = ?", id).
		Select("Name", "Enabled", "CronExpr", "ClientGroupID", "ActionType", "ActionParam", "UpdatedAt").
		Updates(task).Error
}

func (s *PostgresStore) DeleteScheduledTask(id uint) error {
	return s.db.Delete(&models.ScheduledTask{}, id).Error
}

func (s *PostgresStore) RecordScheduledTaskRun(id uint, status, errorMsg string) error {
	now := time.Now()
	return s.db.Model(&models.ScheduledTask{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_run":    now,
		"last_status": status,
		"last_error":  errorMsg,
		"run_count":   gorm.Expr("run_count + 1"),
	}).Error
}

func (s *PostgresStore) GetWebhookConfig() (*models.WebhookConfig, error) {
	var cfg models.WebhookConfig
	if err := s.db.First(&cfg, 1).Error; err != nil {
		return &models.WebhookConfig{ID: 1, OnBootStarted: true, OnClientDiscovered: true}, nil
	}
	return &cfg, nil
}

func (s *PostgresStore) UpdateWebhookConfig(cfg *models.WebhookConfig) error {
	cfg.ID = 1
	return s.db.Save(cfg).Error
}

func (s *PostgresStore) ListClientGroups() ([]*models.ClientGroup, error) {
	var groups []*models.ClientGroup
	if err := s.db.Order("name ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *PostgresStore) GetClientGroup(id uint) (*models.ClientGroup, error) {
	var group models.ClientGroup
	if err := s.db.First(&group, id).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func (s *PostgresStore) GetClientGroupByName(name string) (*models.ClientGroup, error) {
	var group models.ClientGroup
	if err := s.db.Where("name = ?", name).First(&group).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func (s *PostgresStore) CreateClientGroup(group *models.ClientGroup) error {
	var existing models.ClientGroup
	if err := s.db.Unscoped().Where("name = ? AND deleted_at IS NOT NULL", group.Name).First(&existing).Error; err == nil {
		existing.DeletedAt = gorm.DeletedAt{}
		existing.Description = group.Description
		existing.Enabled = group.Enabled
		existing.AllowedImages = group.AllowedImages
		existing.BootloaderSet = group.BootloaderSet
		existing.WOLBroadcastAddr = group.WOLBroadcastAddr
		existing.StaggerDelayMillis = group.StaggerDelayMillis
		if err := s.db.Unscoped().Save(&existing).Error; err != nil {
			return err
		}
		group.ID = existing.ID
		return nil
	}
	return s.db.Create(group).Error
}

func (s *PostgresStore) UpdateClientGroup(id uint, group *models.ClientGroup) error {
	return s.db.Model(&models.ClientGroup{}).Where("id = ?", id).
		Select("Name", "Description", "Enabled", "AllowedImages", "BootloaderSet", "WOLBroadcastAddr", "StaggerDelayMillis",
			"IPMIPort", "IPMIUsername", "IPMIPassword", "IPMIInsecure", "UpdatedAt").
		Updates(group).Error
}

func (s *PostgresStore) DeleteClientGroup(id uint) error {
	if err := s.db.Model(&models.Client{}).Where("client_group_id = ?", id).Update("client_group_id", nil).Error; err != nil {
		return err
	}
	return s.db.Delete(&models.ClientGroup{}, id).Error
}

func (s *PostgresStore) ListClientsInGroup(groupID uint) ([]*models.Client, error) {
	var clients []*models.Client
	if err := s.db.Where("client_group_id = ?", groupID).Order("name ASC").Find(&clients).Error; err != nil {
		return nil, err
	}
	return clients, nil
}

func (s *PostgresStore) SetClientGroup(mac string, groupID *uint) error {
	return s.db.Model(&models.Client{}).Where("mac_address = ?", mac).
		Update("client_group_id", groupID).Error
}
