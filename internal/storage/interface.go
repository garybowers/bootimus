package storage

import "bootimus/internal/models"

type Storage interface {
	AutoMigrate() error
	Close() error

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
	SyncImages(isoFiles []struct{ Name, Filename string; Size int64 }) error

	AssignImagesToClient(mac string, imageFilenames []string) error
	GetClientImages(mac string) ([]string, error)
	GetImagesForClient(macAddress string) ([]models.Image, error)

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

	LogBootAttempt(macAddress, imageName, ipAddress string, success bool, errorMsg string) error
	UpdateClientBootStats(macAddress string) error
	UpdateImageBootStats(imageName string) error
	GetBootLogs(limit int) ([]models.BootLog, error)

	GetStats() (map[string]int64, error)
}
