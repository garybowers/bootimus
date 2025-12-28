package auth

import (
	"fmt"
	"log"
	"net/http"

	"bootimus/internal/database"
)

type Manager struct {
	db *database.DB
}

func NewManager(db *database.DB) (*Manager, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required for authentication")
	}

	m := &Manager{
		db: db,
	}

	// Ensure admin user exists
	username, password, created, err := db.EnsureAdminUser()
	if err != nil {
		return nil, fmt.Errorf("failed to ensure admin user: %w", err)
	}

	if created {
		log.Println("╔════════════════════════════════════════════════════════════════╗")
		log.Println("║                    ADMIN PASSWORD GENERATED                    ║")
		log.Println("╠════════════════════════════════════════════════════════════════╣")
		log.Printf("║  Username: %-50s ║\n", username)
		log.Printf("║  Password: %-50s ║\n", password)
		log.Println("╠════════════════════════════════════════════════════════════════╣")
		log.Println("║  This password will NOT be shown again!                        ║")
		log.Println("║  Save it now or reset it using --reset-admin-password flag    ║")
		log.Println("╚════════════════════════════════════════════════════════════════╝")
	} else {
		log.Println("Admin authentication enabled (using database)")
	}

	return m, nil
}

// ValidateCredentials validates username and password against the database
func (m *Manager) ValidateCredentials(username, password string) bool {
	user, err := m.db.GetUser(username)
	if err != nil {
		return false
	}

	if !user.Enabled {
		return false
	}

	if !user.CheckPassword(password) {
		return false
	}

	// Update last login
	_ = m.db.UpdateUserLastLogin(username)

	return true
}

// BasicAuthMiddleware provides HTTP basic authentication
func (m *Manager) BasicAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()

		if !ok || !m.ValidateCredentials(username, password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="Bootimus Admin"`)
			http.Error(w, "Unauthorised", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}
