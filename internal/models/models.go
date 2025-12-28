package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// StringSlice is a custom type for storing string slices in SQLite
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

// User represents an admin user
type User struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Username  string    `gorm:"uniqueIndex;not null" json:"username"`
	Password  string    `gorm:"not null" json:"-"` // Never send password in JSON
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	IsAdmin   bool      `gorm:"default:false" json:"is_admin"`
	LastLogin *time.Time `json:"last_login,omitempty"`
}

// SetPassword hashes and sets the user's password
func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hash)
	return nil
}

// CheckPassword verifies the password
func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	return err == nil
}

// Client represents a network boot client identified by MAC address
type Client struct {
	ID            uint           `gorm:"primarykey" json:"id"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	MACAddress    string         `gorm:"uniqueIndex;not null" json:"mac_address"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Enabled       bool           `gorm:"default:true" json:"enabled"`
	LastBoot      *time.Time     `json:"last_boot,omitempty"`
	BootCount     int            `gorm:"default:0" json:"boot_count"`
	Images        []Image        `gorm:"many2many:client_images;" json:"images,omitempty"`
	AllowedImages StringSlice    `gorm:"type:text" json:"allowed_images,omitempty"` // For SQLite storage
}

// Image represents an ISO image available for network booting
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
	Public      bool           `gorm:"default:false" json:"public"` // If true, available to all clients
	BootCount   int            `gorm:"default:0" json:"boot_count"`
	LastBooted  *time.Time     `json:"last_booted,omitempty"`
	Clients     []Client       `gorm:"many2many:client_images;" json:"clients,omitempty"`
	// Kernel/Initrd extraction fields
	Extracted       bool       `gorm:"default:false" json:"extracted"`
	Distro          string     `json:"distro,omitempty"`
	BootMethod      string     `gorm:"default:sanboot" json:"boot_method"` // "sanboot" or "kernel"
	KernelPath      string     `json:"kernel_path,omitempty"`
	InitrdPath      string     `json:"initrd_path,omitempty"`
	BootParams      string     `json:"boot_params,omitempty"`
	ExtractionError string     `json:"extraction_error,omitempty"`
	ExtractedAt     *time.Time `json:"extracted_at,omitempty"`
}

// BootLog represents a log entry for boot attempts
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
