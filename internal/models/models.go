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
	ID            uint           `gorm:"primarykey" json:"id"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	MACAddress    string         `gorm:"uniqueIndex:idx_mac_not_deleted;not null" json:"mac_address"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Enabled       bool           `gorm:"default:true" json:"enabled"`
	LastBoot      *time.Time     `json:"last_boot,omitempty"`
	BootCount     int            `gorm:"default:0" json:"boot_count"`
	Images        []Image        `gorm:"many2many:client_images;" json:"images,omitempty"`
	AllowedImages StringSlice    `gorm:"type:text" json:"allowed_images,omitempty"`
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
