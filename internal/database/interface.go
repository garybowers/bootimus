package database

import "bootimus/internal/models"

// UserStore defines the interface for user management operations
type UserStore interface {
	EnsureAdminUser() (username, password string, created bool, err error)
	ResetAdminPassword() (string, error)
	GetUser(username string) (*models.User, error)
	UpdateUserLastLogin(username string) error
}
