package models

import (
	"database/sql/driver"
	"encoding/json"
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

type User struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Username  string    `gorm:"uniqueIndex;not null" json:"username"`
	Password  string    `gorm:"not null" json:"-"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	IsAdmin   bool      `gorm:"default:false" json:"is_admin"`
	LastLogin *time.Time `json:"last_login,omitempty"`
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

	// Redfish (BMC) power control. Empty fields = inherit from the client
	// group, if any. ipmi_host is the BMC/iLO/iDRAC/XCC address — separate
	// from the client's main interface because BMCs have their own NIC.
	IPMIHost     string `json:"ipmi_host,omitempty"`
	IPMIPort     int    `json:"ipmi_port,omitempty"`
	IPMIUsername string `json:"ipmi_username,omitempty"`
	IPMIPassword string `json:"ipmi_password,omitempty"`
	IPMIInsecure bool   `gorm:"default:false" json:"ipmi_insecure,omitempty"`
}

// ScheduledTask is a recurring action bootimus runs on a cron schedule
// against a client group. Action types: "wake" (WOL all members),
// "power" (Redfish against all members, ActionParam=On/ForceOff/etc),
// "next-boot" (set next-boot image on all members, ActionParam=filename),
// "next-boot-clear" (clear next-boot on all members).
type ScheduledTask struct {
	ID             uint           `gorm:"primarykey" json:"id"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Name           string         `gorm:"not null" json:"name"`
	Enabled        bool           `gorm:"default:true" json:"enabled"`
	CronExpr       string         `gorm:"not null" json:"cron_expr"`
	ClientGroupID  uint           `gorm:"not null;index" json:"client_group_id"`
	ClientGroup    *ClientGroup   `gorm:"foreignKey:ClientGroupID" json:"client_group,omitempty"`
	ActionType     string         `gorm:"not null" json:"action_type"`
	ActionParam    string         `json:"action_param,omitempty"`
	LastRun        *time.Time     `json:"last_run,omitempty"`
	LastStatus     string         `json:"last_status,omitempty"`
	LastError      string         `json:"last_error,omitempty"`
	RunCount       int            `gorm:"default:0" json:"run_count"`
}

// WebhookConfig is a singleton (ID=1) holding the outbound webhook URL and
// per-event toggles. Fire-and-forget HTTP POSTs with a JSON payload.
type WebhookConfig struct {
	ID                    uint      `gorm:"primarykey" json:"id"`
	UpdatedAt             time.Time `json:"updated_at"`
	URL                   string    `json:"url"`
	Enabled               bool      `gorm:"default:false" json:"enabled"`
	OnBootStarted         bool      `gorm:"default:true" json:"on_boot_started"`
	OnClientDiscovered    bool      `gorm:"default:true" json:"on_client_discovered"`
	OnInventoryUpdated    bool      `gorm:"default:false" json:"on_inventory_updated"`
}

// ClientGroup represents a set of clients that share common settings and can
// be targeted by bulk actions (wake all, set next-boot on all members, etc.).
// Group-level AllowedImages are unioned with each member's client-level
// AllowedImages when resolving what that client can boot.
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

	// Redfish (BMC) defaults inherited by member clients unless overridden.
	// IPMIHost is almost always per-client (each BMC has its own IP) so it's
	// not group-level; the group only carries shared credentials + port.
	IPMIPort     int    `json:"ipmi_port,omitempty"`
	IPMIUsername string `json:"ipmi_username,omitempty"`
	IPMIPassword string `json:"ipmi_password,omitempty"`
	IPMIInsecure bool   `gorm:"default:false" json:"ipmi_insecure,omitempty"`
}

type SyncFile struct {
	Name      string
	Filename  string
	Size      int64
	GroupPath string // relative directory path from isoDir, empty for root
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
	ID          uint           `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Name        string         `gorm:"not null" json:"name"`
	Filename    string         `gorm:"uniqueIndex;not null" json:"filename"`
	Description string         `json:"description"`
	Size        int64          `json:"size"`
	Enabled     bool           `gorm:"default:true" json:"enabled"`
	Public      bool           `gorm:"default:false" json:"public"`
	BootCount   int            `gorm:"default:0" json:"boot_count"`
	LastBooted  *time.Time     `json:"last_booted,omitempty"`
	Clients     []Client       `gorm:"many2many:client_images;" json:"clients,omitempty"`
	GroupID     *uint          `gorm:"index" json:"group_id,omitempty"`
	Group       *ImageGroup    `gorm:"foreignKey:GroupID" json:"group,omitempty"`
	Order       int            `gorm:"default:0" json:"order"`
	Extracted         bool       `gorm:"default:false" json:"extracted"`
	Distro            string     `json:"distro,omitempty"`
	BootMethod        string     `gorm:"default:sanboot" json:"boot_method"`
	KernelPath        string     `json:"kernel_path,omitempty"`
	InitrdPath        string     `json:"initrd_path,omitempty"`
	BootParams        string     `json:"boot_params,omitempty"`
	SquashfsPath      string     `json:"squashfs_path,omitempty"`
	ExtractionError   string     `json:"extraction_error,omitempty"`
	ExtractedAt       *time.Time `json:"extracted_at,omitempty"`
	SanbootCompatible bool       `gorm:"default:true" json:"sanboot_compatible"`
	SanbootHint       string     `json:"sanboot_hint,omitempty"`
	NetbootRequired   bool       `gorm:"default:false" json:"netboot_required"`
	NetbootAvailable  bool       `gorm:"default:false" json:"netboot_available"`
	NetbootURL        string     `json:"netboot_url,omitempty"`
	AutoInstallScript     string `gorm:"type:text" json:"auto_install_script,omitempty"`
	AutoInstallEnabled    bool   `gorm:"default:false" json:"auto_install_enabled"`
	AutoInstallScriptType string `json:"auto_install_script_type,omitempty"`
	InstallWimPath        string `json:"install_wim_path,omitempty"`
	SMBInstallEnabled     bool   `gorm:"default:false" json:"smb_install_enabled"`
}

type BootLog struct {
	ID         uint       `gorm:"primarykey" json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	ClientID   *uint      `json:"client_id,omitempty"`
	Client     *Client    `gorm:"foreignKey:ClientID" json:"client,omitempty"`
	ImageID    *uint      `json:"image_id,omitempty"`
	Image      *Image     `gorm:"foreignKey:ImageID" json:"image,omitempty"`
	MACAddress string     `gorm:"index" json:"mac_address"`
	ImageName  string     `json:"image_name"`
	Success    bool       `json:"success"`
	ErrorMsg   string     `json:"error_msg,omitempty"`
	IPAddress  string     `json:"ip_address,omitempty"`
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
	ID                    uint      `gorm:"primarykey" json:"id"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	ProfileID             string    `gorm:"uniqueIndex;not null" json:"profile_id"`
	DisplayName           string    `json:"display_name"`
	Family                string    `json:"family"`
	FilenamePatterns      StringSlice `gorm:"type:text" json:"filename_patterns"`
	KernelPaths           StringSlice `gorm:"type:text" json:"kernel_paths"`
	InitrdPaths           StringSlice `gorm:"type:text" json:"initrd_paths"`
	SquashfsPaths         StringSlice `gorm:"type:text" json:"squashfs_paths"`
	DefaultBootParams     string    `json:"default_boot_params"`
	BootParamsWithSquashfs string   `json:"boot_params_with_squashfs,omitempty"`
	AutoInstallType       string    `json:"auto_install_type,omitempty"`
	BootMethod            string    `json:"boot_method,omitempty"`
	Custom                bool      `gorm:"default:false" json:"custom"`
	Version               string    `json:"version,omitempty"`
}

type MenuTheme struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Title        string `gorm:"default:Bootimus - Boot Menu" json:"title"`
	MenuTimeout  int    `gorm:"default:30" json:"menu_timeout"`    // seconds, 0 = no timeout (wait forever)
}

type BootTool struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Name        string `gorm:"uniqueIndex;not null" json:"name"`        // e.g. "gparted"
	DisplayName string `json:"display_name"`                            // e.g. "GParted Live"
	Description string `json:"description"`                             // short description
	Version     string `json:"version"`                                 // e.g. "1.8.1-2"
	Enabled     bool   `gorm:"default:false" json:"enabled"`            // show in boot menu
	Downloaded  bool   `gorm:"default:false" json:"downloaded"`         // files are on disk
	Order       int    `gorm:"default:0" json:"order"`                  // menu order
	DownloadURL string `json:"download_url"`                            // user-overridable URL
	Custom      bool   `gorm:"default:false" json:"custom"`             // user-created tool
	KernelPath  string `json:"kernel_path,omitempty"`                   // path within tool dir
	InitrdPath  string `json:"initrd_path,omitempty"`                   // path within tool dir
	BootParams  string `json:"boot_params,omitempty"`                   // kernel parameters ({{HTTP_URL}} replaced)
	BootMethod  string `json:"boot_method,omitempty"`                   // "kernel", "memdisk", or "chain"
	ArchiveType string `json:"archive_type,omitempty"`                  // "zip", "bin", or "iso"
}
