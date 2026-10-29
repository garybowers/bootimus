package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	if len(s) == 0 {
		return "[]", nil
	}
	return json.Marshal(s)
}

func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = []string{}
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		str, ok := value.(string)
		if !ok {
			return nil
		}
		bytes = []byte(str)
	}

	return json.Unmarshal(bytes, s)
}

type StringMap map[string]string

func (m StringMap) Value() (driver.Value, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	return json.Marshal(m)
}

func (m *StringMap) Scan(value interface{}) error {
	if value == nil {
		*m = StringMap{}
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		str, ok := value.(string)
		if !ok {
			return nil
		}
		bytes = []byte(str)
	}

	return json.Unmarshal(bytes, m)
}

type User struct {
	ID        uint       `gorm:"primarykey" json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Username  string     `gorm:"uniqueIndex;not null" json:"username"`
	Password  string     `gorm:"not null" json:"-"`
	Enabled   bool       `gorm:"default:true" json:"enabled"`
	IsAdmin   bool       `gorm:"default:false" json:"is_admin"`
	Source    string     `gorm:"default:local" json:"source"`
	LastLogin *time.Time `json:"last_login,omitempty"`

	TOTPSecret    string `json:"-"`
	TOTPEnabled   bool   `gorm:"default:false" json:"totp_enabled"`
	RecoveryCodes string `gorm:"type:text" json:"-"`
}

const (
	UserSourceLocal = "local"
	UserSourceLDAP  = "ldap"
	UserSourceOIDC  = "oidc"
)

func (u *User) AuthSource() string {
	if u.Source == "" {
		return UserSourceLocal
	}
	return u.Source
}

func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hash)
	return nil
}

func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	return err == nil
}

type Client struct {
	ID               uint           `gorm:"primarykey" json:"id"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	MACAddress       string         `gorm:"uniqueIndex:idx_mac_not_deleted;not null" json:"mac_address"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	Enabled          bool           `gorm:"default:true" json:"enabled"`
	ShowPublicImages bool           `gorm:"default:true" json:"show_public_images"`
	BootloaderSet    string         `json:"bootloader_set,omitempty"`
	LastBoot         *time.Time     `json:"last_boot,omitempty"`
	BootCount        int            `gorm:"default:0" json:"boot_count"`
	Images           []Image        `gorm:"many2many:client_images;" json:"images,omitempty"`
	AllowedImages    StringSlice    `gorm:"type:text" json:"allowed_images,omitempty"`
	NextBootImage    string         `json:"next_boot_image,omitempty"`
	Static           bool           `gorm:"default:false" json:"static"`
	ClientGroupID    *uint          `gorm:"index" json:"client_group_id,omitempty"`
	ClientGroup      *ClientGroup   `gorm:"foreignKey:ClientGroupID" json:"client_group,omitempty"`
	ProfileID        *uint          `gorm:"index" json:"profile_id,omitempty"`
	EnrolledByRuleID *uint          `gorm:"index" json:"enrolled_by_rule_id,omitempty"`
	EnrolledAt       *time.Time     `json:"enrolled_at,omitempty"`

	BMCProtocol  string `json:"bmc_protocol,omitempty"`
	IPMIHost     string `json:"ipmi_host,omitempty"`
	IPMIPort     int    `json:"ipmi_port,omitempty"`
	IPMIUsername string `json:"ipmi_username,omitempty"`
	IPMIPassword string `json:"ipmi_password,omitempty"`
	IPMIInsecure bool   `gorm:"default:false" json:"ipmi_insecure,omitempty"`

	AutoInstallFile string    `json:"auto_install_file,omitempty"`
	Variables       StringMap `gorm:"type:text" json:"variables,omitempty"`

	InstallState     string     `json:"install_state,omitempty"`
	InstallImage     string     `json:"install_image,omitempty"`
	InstallUpdatedAt *time.Time `json:"install_updated_at,omitempty"`
	SANTargetID      *uint      `gorm:"index" json:"san_target_id,omitempty"`
	BootFromSAN      bool       `gorm:"default:false" json:"boot_from_san"`

	ProxyDHCPNoVendorOption bool `gorm:"default:false" json:"proxy_dhcp_no_vendor_option"`

	EnrollmentState string     `gorm:"default:'';index" json:"enrollment_state,omitempty"`
	ClusterID       *uint      `gorm:"index" json:"cluster_id,omitempty"`
	Cluster         *Cluster   `gorm:"foreignKey:ClusterID" json:"cluster,omitempty"`
	ClusterRole     string     `json:"cluster_role,omitempty"`
	InstallDisk     string     `json:"install_disk,omitempty"`
	ConfigPatch     string     `gorm:"type:text" json:"config_patch,omitempty"`
	EnrollmentToken string     `json:"-"`
	TokenExpiresAt  *time.Time `json:"token_expires_at,omitempty"`
	ApprovedAt      *time.Time `json:"approved_at,omitempty"`
}

const (
	EnrollmentStateUnmanaged  = ""
	EnrollmentStatePending    = "pending"
	EnrollmentStateApproved   = "approved"
	EnrollmentStateInstalling = "installing"
	EnrollmentStateInstalled  = "installed"
	EnrollmentStateRejected   = "rejected"
)

const (
	ClusterRoleControlPlane = "controlplane"
	ClusterRoleWorker       = "worker"
)

const (
	ProviderTalos   = "talos"
	ProviderFlatcar = "flatcar"
)

type NodeImage struct {
	ID         uint           `gorm:"primarykey" json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Name       string         `gorm:"uniqueIndex;not null" json:"name"`
	Provider   string         `gorm:"not null" json:"provider"`
	Version    string         `json:"version"`
	ImageRef   string         `json:"image_ref"`
	Downloaded bool           `gorm:"default:false" json:"downloaded"`
}

type Cluster struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Name        string         `gorm:"uniqueIndex;not null" json:"name"`
	Description string         `json:"description"`

	NodeImageID *uint      `gorm:"index" json:"node_image_id,omitempty"`
	NodeImage   *NodeImage `gorm:"foreignKey:NodeImageID" json:"node_image,omitempty"`

	KernelParams       string `json:"kernel_params,omitempty"`
	DefaultInstallDisk string `json:"default_install_disk"`

	ControlPlaneConfig string `gorm:"type:text" json:"-"`
	WorkerConfig       string `gorm:"type:text" json:"-"`
	ConfigSource       string `json:"config_source,omitempty"`
}

type DeploymentProfile struct {
	ID               uint      `gorm:"primarykey" json:"id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Name             string    `gorm:"uniqueIndex;not null" json:"name"`
	Description      string    `json:"description"`
	ImageFilename    string    `json:"image_filename,omitempty"`
	AutoInstallFile  string    `json:"auto_install_file,omitempty"`
	SANTargetID      *uint     `gorm:"index" json:"san_target_id,omitempty"`
	BootFromSAN      bool      `gorm:"default:false" json:"boot_from_san"`
	BootloaderSet    string    `json:"bootloader_set,omitempty"`
	DiskImageID      *uint     `gorm:"index" json:"disk_image_id,omitempty"`
	FirstBootAction  string    `json:"first_boot_action,omitempty"`
	ShowPublicImages *bool     `json:"show_public_images,omitempty"`
}

type EnrolmentRule struct {
	ID                   uint       `gorm:"primarykey" json:"id"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	Name                 string     `gorm:"not null" json:"name"`
	Priority             int        `gorm:"index" json:"priority"`
	Enabled              bool       `json:"enabled"`
	MACPrefix            string     `json:"mac_prefix,omitempty"`
	ManufacturerContains string     `json:"manufacturer_contains,omitempty"`
	ProductContains      string     `json:"product_contains,omitempty"`
	SerialPattern        string     `json:"serial_pattern,omitempty"`
	UUIDPrefix           string     `json:"uuid_prefix,omitempty"`
	Firmware             string     `json:"firmware,omitempty"`
	ClientGroupID        *uint      `gorm:"index" json:"client_group_id,omitempty"`
	ProfileID            *uint      `gorm:"index" json:"profile_id,omitempty"`
	MakeStatic           bool       `gorm:"default:false" json:"make_static"`
	NamePattern          string     `json:"name_pattern,omitempty"`
	MatchCount           int        `gorm:"default:0" json:"match_count"`
	LastMatchedAt        *time.Time `json:"last_matched_at,omitempty"`
}

const (
	FirstBootActionNone    = ""
	FirstBootActionInstall = "install"
	FirstBootActionDeploy  = "deploy"
)

type ScheduledTask struct {
	ID            uint           `gorm:"primarykey" json:"id"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Name          string         `gorm:"not null" json:"name"`
	Enabled       bool           `gorm:"default:true" json:"enabled"`
	CronExpr      string         `gorm:"not null" json:"cron_expr"`
	ClientGroupID uint           `gorm:"not null;index" json:"client_group_id"`
	ClientGroup   *ClientGroup   `gorm:"foreignKey:ClientGroupID" json:"client_group,omitempty"`
	ActionType    string         `gorm:"not null" json:"action_type"`
	ActionParam   string         `json:"action_param,omitempty"`
	LastRun       *time.Time     `json:"last_run,omitempty"`
	LastStatus    string         `json:"last_status,omitempty"`
	LastError     string         `json:"last_error,omitempty"`
	RunCount      int            `gorm:"default:0" json:"run_count"`
}

type WebhookConfig struct {
	ID                 uint      `gorm:"primarykey" json:"id"`
	UpdatedAt          time.Time `json:"updated_at"`
	URL                string    `json:"url"`
	Enabled            bool      `gorm:"default:false" json:"enabled"`
	OnBootStarted      bool      `gorm:"default:true" json:"on_boot_started"`
	OnClientDiscovered bool      `gorm:"default:true" json:"on_client_discovered"`
	OnInventoryUpdated bool      `gorm:"default:false" json:"on_inventory_updated"`
	OnEnrollment       bool      `gorm:"default:true" json:"on_enrollment"`
	OnAdminAction      bool      `gorm:"default:false" json:"on_admin_action"`
	OnDiskImaging      bool      `gorm:"default:true" json:"on_disk_imaging"`
	OnInstall          bool      `gorm:"default:true" json:"on_install"`
	OnAutoEnrol        bool      `gorm:"default:true" json:"on_auto_enrol"`
}

const (
	InstallStateBooted     = "booted"
	InstallStateInstalling = "installing"
	InstallStateCompleted  = "completed"
	InstallStateFailed     = "failed"
)

type InstallRecord struct {
	ID            uint       `gorm:"primarykey" json:"id"`
	CreatedAt     time.Time  `json:"created_at"`
	MACAddress    string     `gorm:"index" json:"mac_address"`
	ImageFilename string     `json:"image_filename"`
	ImageName     string     `json:"image_name"`
	Token         string     `gorm:"uniqueIndex;size:64" json:"-"`
	State         string     `gorm:"index" json:"state"`
	BootedAt      time.Time  `json:"booted_at"`
	InstallingAt  *time.Time `json:"installing_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	Message       string     `json:"message,omitempty"`
	ReporterIP    string     `json:"reporter_ip,omitempty"`
}

func (r *InstallRecord) Open() bool {
	return r.State == InstallStateBooted || r.State == InstallStateInstalling
}

type APIToken struct {
	ID         uint       `gorm:"primarykey" json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	Name       string     `gorm:"not null" json:"name"`
	Prefix     string     `gorm:"not null" json:"prefix"`
	Hash       string     `gorm:"uniqueIndex;not null" json:"-"`
	Scope      string     `gorm:"not null" json:"scope"`
	CreatedBy  string     `json:"created_by"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	Revoked    bool       `gorm:"default:false" json:"revoked"`
}

const (
	APITokenScopeRead  = "read"
	APITokenScopeAdmin = "admin"
)

func (t *APIToken) Expired(now time.Time) bool {
	return t.ExpiresAt != nil && !t.ExpiresAt.After(now)
}

func (t *APIToken) Usable(now time.Time) bool {
	return !t.Revoked && !t.Expired(now)
}

type AuditLog struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	Username  string    `gorm:"index" json:"username"`
	Method    string    `json:"method"`
	Path      string    `gorm:"index" json:"path"`
	Query     string    `json:"query,omitempty"`
	Status    int       `json:"status"`
	RemoteIP  string    `json:"remote_ip,omitempty"`
	Summary   string    `json:"summary"`
}

type ReplicaState struct {
	ID        uint       `gorm:"primarykey" json:"id"`
	UpdatedAt time.Time  `json:"updated_at"`
	LastSync  *time.Time `json:"last_sync,omitempty"`
	Report    string     `gorm:"type:text" json:"report,omitempty"`
}

type HousekeepingSettings struct {
	ID                   uint       `gorm:"primarykey" json:"id"`
	UpdatedAt            time.Time  `json:"updated_at"`
	BootLogDays          int        `json:"boot_log_days"`
	InventoryHistoryDays int        `json:"inventory_history_days"`
	DiscoveredClientDays int        `json:"discovered_client_days"`
	AuditLogDays         int        `json:"audit_log_days"`
	CatalogueCheckOff    bool       `json:"catalogue_check_off"`
	LastRun              *time.Time `json:"last_run,omitempty"`
	LastStatus           string     `json:"last_status,omitempty"`
	LastReport           string     `gorm:"type:text" json:"last_report,omitempty"`
}

type ClientGroup struct {
	ID                 uint           `gorm:"primarykey" json:"id"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Name               string         `gorm:"uniqueIndex;not null" json:"name"`
	Description        string         `json:"description"`
	Enabled            bool           `gorm:"default:true" json:"enabled"`
	AllowedImages      StringSlice    `gorm:"type:text" json:"allowed_images,omitempty"`
	BootloaderSet      string         `json:"bootloader_set,omitempty"`
	WOLBroadcastAddr   string         `json:"wol_broadcast_addr,omitempty"`
	StaggerDelayMillis int            `gorm:"default:0" json:"stagger_delay_millis"`
	Clients            []Client       `gorm:"foreignKey:ClientGroupID" json:"clients,omitempty"`

	BMCProtocol  string `json:"bmc_protocol,omitempty"`
	IPMIPort     int    `json:"ipmi_port,omitempty"`
	IPMIUsername string `json:"ipmi_username,omitempty"`
	IPMIPassword string `json:"ipmi_password,omitempty"`
	IPMIInsecure bool   `gorm:"default:false" json:"ipmi_insecure,omitempty"`

	AutoInstallFile string    `json:"auto_install_file,omitempty"`
	Variables       StringMap `gorm:"type:text" json:"variables,omitempty"`

	SANTargetID *uint `gorm:"index" json:"san_target_id,omitempty"`
	BootFromSAN bool  `gorm:"default:false" json:"boot_from_san"`
	ProfileID   *uint `gorm:"index" json:"profile_id,omitempty"`
}

type SANTarget struct {
	ID                  uint      `gorm:"primarykey" json:"id"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	Name                string    `gorm:"uniqueIndex;not null" json:"name"`
	Description         string    `json:"description"`
	URI                 string    `gorm:"not null" json:"uri"`
	InitiatorIQN        string    `json:"initiator_iqn,omitempty"`
	CHAPUsername        string    `json:"chap_username,omitempty"`
	CHAPPassword        string    `json:"chap_password,omitempty"`
	ReverseCHAPUsername string    `json:"reverse_chap_username,omitempty"`
	ReverseCHAPPassword string    `json:"reverse_chap_password,omitempty"`
}

const NextBootSANPrefix = "san:"

func (t *SANTarget) NextBootValue() string {
	return fmt.Sprintf("%s%d", NextBootSANPrefix, t.ID)
}

func ParseNextBootSAN(value string) (uint, bool) {
	if !strings.HasPrefix(value, NextBootSANPrefix) {
		return 0, false
	}
	id, err := strconv.ParseUint(strings.TrimPrefix(value, NextBootSANPrefix), 10, 32)
	if err != nil || id == 0 {
		return 0, false
	}
	return uint(id), true
}

func (t *SANTarget) ResolveInitiatorIQN(mac string) string {
	return strings.ReplaceAll(t.InitiatorIQN, "{{MAC}}", strings.ReplaceAll(mac, ":", ""))
}

type SyncFile struct {
	Name       string
	Filename   string
	Size       int64
	GroupPath  string // relative directory path from isoDir, empty for root
	BootMethod string
	BootParams string
}

type ImageGroup struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Name        string         `gorm:"uniqueIndex:idx_group_name_parent;not null" json:"name"`
	Description string         `json:"description"`
	ParentID    *uint          `gorm:"uniqueIndex:idx_group_name_parent;index" json:"parent_id,omitempty"`
	Parent      *ImageGroup    `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Order       int            `gorm:"default:0" json:"order"`
	Enabled     bool           `gorm:"default:true" json:"enabled"`
}

type Image struct {
	ID                    uint           `gorm:"primarykey" json:"id"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Name                  string         `gorm:"not null" json:"name"`
	Filename              string         `gorm:"uniqueIndex;not null" json:"filename"`
	Description           string         `json:"description"`
	Size                  int64          `json:"size"`
	Enabled               bool           `gorm:"default:true" json:"enabled"`
	Public                bool           `gorm:"default:false" json:"public"`
	BootPasswordRequired  bool           `gorm:"default:false" json:"boot_password_required"`
	BootPasswordHash      string         `json:"-"`
	BootPasswordRevision  string         `json:"boot_password_revision,omitempty"`
	BootPasswordSourceAt  time.Time      `json:"-"`
	BootCount             int            `gorm:"default:0" json:"boot_count"`
	LastBooted            *time.Time     `json:"last_booted,omitempty"`
	Clients               []Client       `gorm:"many2many:client_images;" json:"clients,omitempty"`
	GroupID               *uint          `gorm:"index" json:"group_id,omitempty"`
	Group                 *ImageGroup    `gorm:"foreignKey:GroupID" json:"group,omitempty"`
	Order                 int            `gorm:"default:0" json:"order"`
	Extracted             bool           `gorm:"default:false" json:"extracted"`
	Distro                string         `json:"distro,omitempty"`
	BootMethod            string         `gorm:"default:sanboot" json:"boot_method"`
	KernelPath            string         `json:"kernel_path,omitempty"`
	InitrdPath            string         `json:"initrd_path,omitempty"`
	BootParams            string         `json:"boot_params,omitempty"`
	SquashfsPath          string         `json:"squashfs_path,omitempty"`
	ShimPath              string         `json:"shim_path,omitempty"`
	KernelOverride        string         `json:"kernel_override,omitempty"`
	InitrdOverride        string         `json:"initrd_override,omitempty"`
	ExtractionError       string         `json:"extraction_error,omitempty"`
	ExtractedAt           *time.Time     `json:"extracted_at,omitempty"`
	SanbootCompatible     bool           `gorm:"default:true" json:"sanboot_compatible"`
	SanbootHint           string         `json:"sanboot_hint,omitempty"`
	NetbootRequired       bool           `gorm:"default:false" json:"netboot_required"`
	NetbootAvailable      bool           `gorm:"default:false" json:"netboot_available"`
	NetbootURL            string         `json:"netboot_url,omitempty"`
	AutoInstallScript     string         `gorm:"type:text" json:"auto_install_script,omitempty"`
	AutoInstallEnabled    bool           `gorm:"default:false" json:"auto_install_enabled"`
	AutoInstallScriptType string         `json:"auto_install_script_type,omitempty"`
	InstallWimPath        string         `json:"install_wim_path,omitempty"`
	SMBInstallEnabled     bool           `gorm:"default:false" json:"smb_install_enabled"`
	SMBPatchFingerprint   string         `json:"smb_patch_fingerprint,omitempty"`
	SMBNeedsRepatch       bool           `gorm:"-" json:"smb_needs_repatch"`
	SMBUnpatchAvailable   bool           `gorm:"-" json:"smb_unpatch_available"`
	Checksum              string         `json:"checksum,omitempty"`
	ChecksumVerified      bool           `gorm:"default:false" json:"checksum_verified"`
	ChecksumCheckedAt     *time.Time     `json:"checksum_checked_at,omitempty"`
	CatalogueID           string         `json:"catalogue_id,omitempty"`
	CatalogueRelease      string         `json:"catalogue_release,omitempty"`
	UpdateAvailable       bool           `gorm:"default:false" json:"update_available"`
	UpdateVersion         string         `json:"update_version,omitempty"`
	UpdateFilename        string         `json:"update_filename,omitempty"`
	UpdateURL             string         `json:"update_url,omitempty"`
	WIMFiles              []WIMFile      `gorm:"serializer:json" json:"wim_files,omitempty"`
	WIMPath               string         `json:"wim_path,omitempty"`
	WIMIndex              int            `gorm:"default:0" json:"wim_index,omitempty"`

	AutoInstallFile string `json:"auto_install_file,omitempty"`
}

type WIMFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

const (
	DistroWindows  = "windows"
	DistroWindows7 = "windows7"
	DistroWinPE    = "winpe"
)

func IsWindowsDistro(distro string) bool {
	switch distro {
	case DistroWindows, DistroWindows7, DistroWinPE:
		return true
	}
	return false
}

type BootLog struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	ClientID   *uint     `json:"client_id,omitempty"`
	Client     *Client   `gorm:"foreignKey:ClientID" json:"client,omitempty"`
	ImageID    *uint     `json:"image_id,omitempty"`
	Image      *Image    `gorm:"foreignKey:ImageID" json:"image,omitempty"`
	MACAddress string    `gorm:"index" json:"mac_address"`
	ImageName  string    `json:"image_name"`
	Success    bool      `json:"success"`
	ErrorMsg   string    `json:"error_msg,omitempty"`
	IPAddress  string    `json:"ip_address,omitempty"`
}

type HardwareInventory struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	ClientID     *uint     `gorm:"index" json:"client_id,omitempty"`
	MACAddress   string    `gorm:"index;not null" json:"mac_address"`
	IPAddress    string    `json:"ip_address,omitempty"`
	Manufacturer string    `json:"manufacturer,omitempty"`
	Product      string    `json:"product,omitempty"`
	Serial       string    `json:"serial,omitempty"`
	UUID         string    `json:"uuid,omitempty"`
	CPU          string    `json:"cpu,omitempty"`
	Memory       int64     `json:"memory,omitempty"`
	Platform     string    `json:"platform,omitempty"`
	BuildArch    string    `json:"buildarch,omitempty"`
	Asset        string    `json:"asset,omitempty"`
	NICChip      string    `json:"nic_chip,omitempty"`
}

type CustomFile struct {
	ID              uint           `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Filename        string         `gorm:"uniqueIndex:idx_filename_image;not null" json:"filename"`
	OriginalName    string         `gorm:"not null" json:"original_name"`
	Description     string         `json:"description"`
	Size            int64          `json:"size"`
	ContentType     string         `json:"content_type"`
	Public          bool           `gorm:"uniqueIndex:idx_filename_image;default:false" json:"public"`
	ImageID         *uint          `gorm:"uniqueIndex:idx_filename_image;index" json:"image_id,omitempty"`
	Image           *Image         `gorm:"foreignKey:ImageID" json:"image,omitempty"`
	DownloadCount   int            `gorm:"default:0" json:"download_count"`
	LastDownload    *time.Time     `json:"last_download,omitempty"`
	DestinationPath string         `json:"destination_path,omitempty"`
	AutoInstall     bool           `gorm:"default:true" json:"auto_install"`
}

type DriverPack struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Filename     string         `gorm:"not null" json:"filename"`
	OriginalName string         `gorm:"not null" json:"original_name"`
	Description  string         `json:"description"`
	Size         int64          `json:"size"`
	ImageID      uint           `gorm:"index;not null" json:"image_id"`
	Image        *Image         `gorm:"foreignKey:ImageID" json:"image,omitempty"`
	Enabled      bool           `gorm:"default:true" json:"enabled"`
	LastApplied  *time.Time     `json:"last_applied,omitempty"`
}

type DistroProfile struct {
	ID                     uint        `gorm:"primarykey" json:"id"`
	CreatedAt              time.Time   `json:"created_at"`
	UpdatedAt              time.Time   `json:"updated_at"`
	ProfileID              string      `gorm:"uniqueIndex;not null" json:"profile_id"`
	DisplayName            string      `json:"display_name"`
	Family                 string      `json:"family"`
	FilenamePatterns       StringSlice `gorm:"type:text" json:"filename_patterns"`
	KernelPaths            StringSlice `gorm:"type:text" json:"kernel_paths"`
	InitrdPaths            StringSlice `gorm:"type:text" json:"initrd_paths"`
	SquashfsPaths          StringSlice `gorm:"type:text" json:"squashfs_paths"`
	DefaultBootParams      string      `json:"default_boot_params"`
	BootParamsWithSquashfs string      `json:"boot_params_with_squashfs,omitempty"`
	AutoInstallType        string      `json:"auto_install_type,omitempty"`
	BootMethod             string      `json:"boot_method,omitempty"`
	Custom                 bool        `gorm:"default:false" json:"custom"`
	Version                string      `json:"version,omitempty"`
}

type MenuTheme struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Title                string    `gorm:"default:Bootimus - Boot Menu" json:"title"`
	MenuTimeout          int       `gorm:"default:30" json:"menu_timeout"` // seconds, 0 = no timeout (wait forever)
	DefaultMenuItem      string    `gorm:"default:local" json:"default_menu_item"`
	BootPasswordRequired bool      `gorm:"default:false" json:"boot_password_required"`
	BootPasswordHash     string    `json:"-"`
	BootPasswordRevision string    `json:"boot_password_revision,omitempty"`
	BootPasswordSourceAt time.Time `json:"-"`
}

type BootTool struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Name            string `gorm:"uniqueIndex;not null" json:"name"` // e.g. "gparted"
	DisplayName     string `json:"display_name"`                     // e.g. "GParted Live"
	Description     string `json:"description"`                      // short description
	Version         string `json:"version"`                          // e.g. "1.8.1-2"
	Enabled         bool   `gorm:"default:false" json:"enabled"`     // show in boot menu
	Downloaded      bool   `gorm:"default:false" json:"downloaded"`  // files are on disk
	Order           int    `gorm:"default:0" json:"order"`           // menu order
	DownloadURL     string `json:"download_url"`                     // user-overridable URL
	Custom          bool   `gorm:"default:false" json:"custom"`      // user-created tool
	KernelPath      string `json:"kernel_path,omitempty"`            // path within tool dir
	InitrdPath      string `json:"initrd_path,omitempty"`            // path within tool dir
	BootParams      string `json:"boot_params,omitempty"`            // kernel parameters ({{HTTP_URL}} replaced)
	BootMethod      string `json:"boot_method,omitempty"`            // "kernel", "memdisk", or "chain"
	ArchiveType     string `json:"archive_type,omitempty"`           // "zip", "bin", or "iso"
	DownloadURLBIOS string `json:"download_url_bios,omitempty"`      // optional BIOS variant URL (chain tools)
	DownloadedURL   string `json:"downloaded_url,omitempty"`
	UpdateAvailable bool   `gorm:"default:false" json:"update_available"`
	UpdateURL       string `json:"update_url,omitempty"`
	KernelPathBIOS  string `json:"kernel_path_bios,omitempty"` // optional BIOS variant local path
}

const (
	DiskImageStateCapturing = "capturing"
	DiskImageStateReady     = "ready"
	DiskImageStateFailed    = "failed"

	DiskJobKindCapture = "capture"
	DiskJobKindDeploy  = "deploy"

	DiskJobStatePending   = "pending"
	DiskJobStateRunning   = "running"
	DiskJobStateCompleted = "completed"
	DiskJobStateFailed    = "failed"
	DiskJobStateCancelled = "cancelled"
)

type DiskImage struct {
	ID               uint      `gorm:"primarykey" json:"id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Name             string    `gorm:"uniqueIndex;not null" json:"name"`
	Description      string    `json:"description"`
	SourceMAC        string    `json:"source_mac,omitempty"`
	SourceClientName string    `json:"source_client_name,omitempty"`
	Disk             string    `json:"disk"`
	SizeBytes        int64     `json:"size_bytes"`
	State            string    `gorm:"index" json:"state"`
}

type DiskJob struct {
	ID          uint       `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClientMAC   string     `gorm:"index" json:"client_mac"`
	ClientName  string     `json:"client_name,omitempty"`
	Kind        string     `gorm:"not null" json:"kind"`
	DiskImageID *uint      `gorm:"index" json:"disk_image_id,omitempty"`
	DiskImage   *DiskImage `gorm:"foreignKey:DiskImageID;constraint:OnDelete:SET NULL" json:"disk_image,omitempty"`
	Disk        string     `json:"disk"`
	State       string     `gorm:"index" json:"state"`
	Token       string     `gorm:"uniqueIndex;not null" json:"-"`
	Message     string     `json:"message,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

func (j *DiskJob) Active() bool {
	return j.State == DiskJobStatePending || j.State == DiskJobStateRunning
}
