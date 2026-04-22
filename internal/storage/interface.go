package storage

import (
	"io"

	"bootimus/internal/models"
)

// Snapshot writes a portable, restore-ready dump of the database to w and
// returns the suggested filename to use inside a backup tarball.
//
// SQLite returns "bootimus.db" (a clean copy produced via VACUUM INTO,
// safe under WAL/concurrent writes). PostgreSQL returns "bootimus.sql"
// (a pg_dump in plain SQL format).
type Snapshotter interface {
	Snapshot(w io.Writer) (filename string, err error)
}

type Storage interface {
	AutoMigrate() error
	Close() error
	Snapshotter

	ListClients() ([]*models.Client, error)
	GetClient(mac string) (*models.Client, error)
	CreateClient(client *models.Client) error
	UpdateClient(mac string, client *models.Client) error
	DeleteClient(mac string) error

	ListImages() ([]*models.Image, error)
	GetImage(filename string) (*models.Image, error)
	CreateImage(image *models.Image) error
	UpdateImage(filename string, image *models.Image) error
	DeleteImage(filename string) error
	SyncImages(isoFiles []models.SyncFile) error

	AssignImagesToClient(mac string, imageFilenames []string) error
	GetClientImages(mac string) ([]string, error)
	GetImagesForClient(macAddress string) ([]models.Image, error)
	SetNextBootImage(mac string, imageFilename string) error
	ClearNextBootImage(mac string) error

	EnsureAdminUser() (username, password string, created bool, err error)
	ResetAdminPassword() (string, error)
	GetUser(username string) (*models.User, error)
	UpdateUserLastLogin(username string) error
	ListUsers() ([]*models.User, error)
	CreateUser(user *models.User) error
	UpdateUser(username string, user *models.User) error
	DeleteUser(username string) error

	ListCustomFiles() ([]*models.CustomFile, error)
	GetCustomFileByFilename(filename string) (*models.CustomFile, error)
	GetCustomFileByID(id uint) (*models.CustomFile, error)
	GetCustomFileByFilenameAndImage(filename string, imageID *uint, public bool) (*models.CustomFile, error)
	CreateCustomFile(file *models.CustomFile) error
	UpdateCustomFile(id uint, file *models.CustomFile) error
	DeleteCustomFile(id uint) error
	IncrementFileDownloadCount(id uint) error
	ListCustomFilesByImage(imageID uint) ([]*models.CustomFile, error)

	ListDriverPacks() ([]*models.DriverPack, error)
	GetDriverPack(id uint) (*models.DriverPack, error)
	CreateDriverPack(pack *models.DriverPack) error
	UpdateDriverPack(id uint, pack *models.DriverPack) error
	DeleteDriverPack(id uint) error
	ListDriverPacksByImage(imageID uint) ([]*models.DriverPack, error)

	ListImageGroups() ([]*models.ImageGroup, error)
	GetImageGroup(id uint) (*models.ImageGroup, error)
	GetImageGroupByName(name string) (*models.ImageGroup, error)
	CreateImageGroup(group *models.ImageGroup) error
	UpdateImageGroup(id uint, group *models.ImageGroup) error
	DeleteImageGroup(id uint) error
	ListImagesByGroup(groupID uint) ([]*models.Image, error)

	ListClientGroups() ([]*models.ClientGroup, error)
	GetClientGroup(id uint) (*models.ClientGroup, error)
	GetClientGroupByName(name string) (*models.ClientGroup, error)
	CreateClientGroup(group *models.ClientGroup) error
	UpdateClientGroup(id uint, group *models.ClientGroup) error
	DeleteClientGroup(id uint) error
	ListClientsInGroup(groupID uint) ([]*models.Client, error)
	SetClientGroup(mac string, groupID *uint) error

	GetMenuTheme() (*models.MenuTheme, error)
	UpdateMenuTheme(theme *models.MenuTheme) error

	GetWebhookConfig() (*models.WebhookConfig, error)
	UpdateWebhookConfig(cfg *models.WebhookConfig) error

	ListScheduledTasks() ([]*models.ScheduledTask, error)
	ListScheduledTasksByGroup(groupID uint) ([]*models.ScheduledTask, error)
	GetScheduledTask(id uint) (*models.ScheduledTask, error)
	CreateScheduledTask(task *models.ScheduledTask) error
	UpdateScheduledTask(id uint, task *models.ScheduledTask) error
	DeleteScheduledTask(id uint) error
	RecordScheduledTaskRun(id uint, status, errorMsg string) error

	ListDistroProfiles() ([]*models.DistroProfile, error)
	GetDistroProfile(profileID string) (*models.DistroProfile, error)
	SaveDistroProfile(profile *models.DistroProfile) error
	DeleteDistroProfile(profileID string) error

	ListBootTools() ([]*models.BootTool, error)
	GetBootTool(name string) (*models.BootTool, error)
	SaveBootTool(tool *models.BootTool) error
	DeleteBootTool(name string) error

	LogBootAttempt(macAddress, imageName, ipAddress string, success bool, errorMsg string) error
	UpdateClientBootStats(macAddress string) error
	UpdateImageBootStats(imageName string) error
	GetBootLogs(limit int) ([]models.BootLog, error)
	GetBootLogsByMAC(macAddress string, limit int) ([]models.BootLog, error)

	SaveHardwareInventory(inventory *models.HardwareInventory) error
	GetLatestHardwareInventory(mac string) (*models.HardwareInventory, error)
	GetHardwareInventoryHistory(mac string, limit int) ([]models.HardwareInventory, error)

	GetStats() (map[string]int64, error)
}
