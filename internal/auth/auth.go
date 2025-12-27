package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	passwordFile = ".admin_password"
	passwordLen  = 32
)

type Manager struct {
	passwordHash string
	configDir    string
}

func NewManager(configDir string) (*Manager, error) {
	m := &Manager{
		configDir: configDir,
	}

	passwordPath := filepath.Join(configDir, passwordFile)

	// Check if password file exists
	if _, err := os.Stat(passwordPath); os.IsNotExist(err) {
		// Generate new password on first run
		password, err := m.generateAndSavePassword(passwordPath)
		if err != nil {
			return nil, err
		}

		log.Println("╔════════════════════════════════════════════════════════════════╗")
		log.Println("║                    ADMIN PASSWORD GENERATED                    ║")
		log.Println("╠════════════════════════════════════════════════════════════════╣")
		log.Printf("║  Password: %-50s ║\n", password)
		log.Println("╠════════════════════════════════════════════════════════════════╣")
		log.Printf("║  Saved to: %-50s ║\n", passwordPath)
		log.Println("║  This password will NOT be shown again!                        ║")
		log.Println("║  Save it now or retrieve it from the file above.               ║")
		log.Println("╚════════════════════════════════════════════════════════════════╝")

		m.passwordHash = hashPassword(password)
	} else {
		// Load existing password hash
		data, err := os.ReadFile(passwordPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read password file: %w", err)
		}
		m.passwordHash = strings.TrimSpace(string(data))
		log.Printf("Admin authentication enabled (password file: %s)", passwordPath)
	}

	return m, nil
}

func (m *Manager) generateAndSavePassword(path string) (string, error) {
	// Generate random password
	bytes := make([]byte, passwordLen)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random password: %w", err)
	}

	password := base64.URLEncoding.EncodeToString(bytes)[:passwordLen]

	// Hash and save
	hash := hashPassword(password)

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	// Save hash to file with restrictive permissions
	if err := os.WriteFile(path, []byte(hash), 0600); err != nil {
		return "", fmt.Errorf("failed to write password file: %w", err)
	}

	return password, nil
}

func (m *Manager) ValidatePassword(password string) bool {
	return m.passwordHash == hashPassword(password)
}

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// BasicAuthMiddleware provides HTTP basic authentication
func (m *Manager) BasicAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()

		if !ok || username != "admin" || !m.ValidatePassword(password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="Bootimus Admin"`)
			http.Error(w, "Unauthorised", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}
