package storage

import "bootimus/internal/models"

// Storage defines the interface for all database operations
// Both PostgreSQL and SQLite implementations must satisfy this interface
type Storage interface {
	// Database lifecycle
	AutoMigrate() error
	Close() error

	// Client operations
	ListClients() ([]*models.Client, error)
	GetClient(mac string) (*models.Client, error)
	CreateClient(client *models.Client) error
	UpdateClient(mac string, client *models.Client) error
	DeleteClient(mac string) error

	// Image operations
	ListImages() ([]*models.Image, error)
	GetImage(filename string) (*models.Image, error)
	CreateImage(image *models.Image) error
	UpdateImage(filename string, image *models.Image) error
	DeleteImage(filename string) error
	SyncImages(isoFiles []struct{ Name, Filename string; Size int64 }) error

	// Client-Image relationships
	// Note: PostgreSQL uses many2many, SQLite uses JSON arrays internally
	// Both provide identical external behavior
	AssignImagesToClient(mac string, imageFilenames []string) error
	GetClientImages(mac string) ([]string, error)
	GetImagesForClient(macAddress string) ([]models.Image, error)

	// User operations
	EnsureAdminUser() (username, password string, created bool, err error)
	ResetAdminPassword() (string, error)
	GetUser(username string) (*models.User, error)
	UpdateUserLastLogin(username string) error
	ListUsers() ([]*models.User, error)
	CreateUser(user *models.User) error
	UpdateUser(username string, user *models.User) error
	DeleteUser(username string) error

	// CustomFile operations
	ListCustomFiles() ([]*models.CustomFile, error)
	GetCustomFileByFilename(filename string) (*models.CustomFile, error)
	GetCustomFileByID(id uint) (*models.CustomFile, error)
	CreateCustomFile(file *models.CustomFile) error
	UpdateCustomFile(id uint, file *models.CustomFile) error
	DeleteCustomFile(id uint) error
	IncrementFileDownloadCount(id uint) error
	ListCustomFilesByImage(imageID uint) ([]*models.CustomFile, error)

	// Boot operations
	LogBootAttempt(macAddress, imageName, ipAddress string, success bool, errorMsg string) error
	UpdateClientBootStats(macAddress string) error
	UpdateImageBootStats(imageName string) error
	GetBootLogs(limit int) ([]models.BootLog, error)

	// Statistics
	GetStats() (map[string]int64, error)
}
