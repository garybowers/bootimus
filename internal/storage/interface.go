package storage

import (
	"io"
	"time"

	"bootimus/internal/models"
)

type Snapshotter interface {
	Snapshot(w io.Writer) (filename string, err error)
}

type Storage interface {
	AutoMigrate() error
	Close() error
	Snapshotter

	ListClients() ([]*models.Client, error)
	GetClient(mac string) (*models.Client, error)
	GetClientBootloaderSets(mac string) (clientSet, groupSet string, err error)
	CreateClient(client *models.Client) error
	UpdateClient(mac string, client *models.Client) error
	RecordNativeTFTPBoot(filename, remoteAddr string) (bool, error)
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
	ListAPITokens() ([]*models.APIToken, error)
	CreateAPIToken(token *models.APIToken) error
	GetAPITokenByHash(hash string) (*models.APIToken, error)
	GetAPIToken(id uint) (*models.APIToken, error)
	RevokeAPIToken(id uint) error
	TouchAPIToken(id uint, at time.Time) error
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

	ListSANTargets() ([]*models.SANTarget, error)
	GetSANTarget(id uint) (*models.SANTarget, error)
	CreateSANTarget(target *models.SANTarget) error
	UpdateSANTarget(id uint, target *models.SANTarget) error
	DeleteSANTarget(id uint) error
	CountSANTargetUsers(id uint) (int64, int64, error)
	ListDeploymentProfiles() ([]*models.DeploymentProfile, error)
	GetDeploymentProfile(id uint) (*models.DeploymentProfile, error)
	CreateDeploymentProfile(profile *models.DeploymentProfile) error
	UpdateDeploymentProfile(id uint, profile *models.DeploymentProfile) error
	DeleteDeploymentProfile(id uint) error
	CountDeploymentProfileUsers(id uint) (int64, int64, error)
	ListEnrolmentRules() ([]*models.EnrolmentRule, error)
	GetEnrolmentRule(id uint) (*models.EnrolmentRule, error)
	CreateEnrolmentRule(rule *models.EnrolmentRule) error
	UpdateEnrolmentRule(id uint, rule *models.EnrolmentRule) error
	DeleteEnrolmentRule(id uint) error
	ApplyEnrolment(mac string, update EnrolmentUpdate) error
	ListUnenrolledClients() ([]*models.Client, error)

	GetWebhookConfig() (*models.WebhookConfig, error)
	UpdateWebhookConfig(cfg *models.WebhookConfig) error

	GetHousekeepingSettings() (*models.HousekeepingSettings, error)
	UpdateHousekeepingSettings(settings *models.HousekeepingSettings) error
	RecordHousekeepingRun(status, report string) error
	DeleteBootLogsBefore(cutoff time.Time) (int64, error)
	DeleteInventoryHistoryBefore(cutoff time.Time) (int64, error)
	DeleteDiscoveredClientsNotSeenSince(cutoff time.Time) (int64, error)
	CreateInstallRecord(rec *models.InstallRecord) error
	UpdateInstallRecord(rec *models.InstallRecord) error
	GetInstallRecordByToken(token string) (*models.InstallRecord, error)
	LatestOpenInstallRecord(mac, imageFilename string) (*models.InstallRecord, error)
	ListInstallRecords(mac string, limit int) ([]models.InstallRecord, error)
	ListOpenInstallRecordsBefore(cutoff time.Time) ([]models.InstallRecord, error)
	SetClientInstallStatus(mac, state, image string, at time.Time) error

	GetReplicaState() (*models.ReplicaState, error)
	SaveReplicaState(state *models.ReplicaState) error

	CreateAuditLog(entry *models.AuditLog) error
	ListAuditLogs(filter AuditLogFilter) ([]models.AuditLog, int64, error)
	DeleteAuditLogsBefore(cutoff time.Time) (int64, error)

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

	ListNodeImages() ([]*models.NodeImage, error)
	GetNodeImage(id uint) (*models.NodeImage, error)
	CreateNodeImage(image *models.NodeImage) error
	SetNodeImageDownloaded(id uint, downloaded bool) error
	DeleteNodeImage(id uint) error

	ListClusters() ([]*models.Cluster, error)
	GetCluster(id uint) (*models.Cluster, error)
	CreateCluster(cluster *models.Cluster) error
	UpdateCluster(id uint, cluster *models.Cluster) error
	SetClusterConfigs(id uint, controlPlane, worker, source string) error
	DeleteCluster(id uint) error
	ListClusterNodes(clusterID uint) ([]*models.Client, error)
	AssignNodeToCluster(mac string, clusterID uint, role, installDisk, configPatch, token string, expires time.Time) error
	SetClientEnrollmentState(mac, state string) error
	ResetClientEnrollment(mac string) error

	GetStats() (map[string]int64, error)

	CreateDiskImage(image *models.DiskImage) error
	GetDiskImage(id uint) (*models.DiskImage, error)
	GetDiskImageByName(name string) (*models.DiskImage, error)
	ListDiskImages() ([]*models.DiskImage, error)
	UpdateDiskImage(image *models.DiskImage) error
	DeleteDiskImage(id uint, removeFiles func() error) error
	CreateDiskJob(job *models.DiskJob) error
	GetDiskJob(id uint) (*models.DiskJob, error)
	GetDiskJobByToken(token string) (*models.DiskJob, error)
	ListDiskJobs(mac string, limit int) ([]*models.DiskJob, error)
	ActiveDiskJobForClient(mac string) (*models.DiskJob, error)
	UpdateDiskJob(job *models.DiskJob) error
}
