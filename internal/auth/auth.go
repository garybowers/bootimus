package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"bootimus/internal/database"

	"github.com/golang-jwt/jwt/v5"
)

type Manager struct {
	userStore  database.UserStore
	jwtSecret  []byte
	ldapConfig *LDAPConfig
}

type Claims struct {
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

type LoginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	AuthMethod string `json:"auth_method"`
}

type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
}

func NewManager(userStore database.UserStore, ldapConfig ...*LDAPConfig) (*Manager, error) {
	if userStore == nil {
		return nil, fmt.Errorf("userStore is required for authentication")
	}

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("failed to generate JWT secret: %w", err)
	}

	m := &Manager{
		userStore: userStore,
		jwtSecret: secret,
	}

	if len(ldapConfig) > 0 && ldapConfig[0] != nil && ldapConfig[0].IsConfigured() {
		m.ldapConfig = ldapConfig[0]
		log.Printf("LDAP authentication enabled (server: %s)", m.ldapConfig.serverURL())
	}

	username, password, created, err := userStore.EnsureAdminUser()
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
		log.Println("Admin authentication enabled")
	}

	return m, nil
}

func (m *Manager) GenerateToken(username string, isAdmin bool) (string, error) {
	claims := &Claims{
		Username: username,
		IsAdmin:  isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        generateTokenID(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.jwtSecret)
}

func (m *Manager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

func (m *Manager) ValidateCredentials(username, password string) bool {
	user, err := m.userStore.GetUser(username)
	if err != nil {
		return false
	}

	if !user.Enabled {
		return false
	}

	if !user.CheckPassword(password) {
		return false
	}

	_ = m.userStore.UpdateUserLastLogin(username)
	return true
}

func (m *Manager) GetUserAdmin(username string) bool {
	user, err := m.userStore.GetUser(username)
	if err != nil {
		return false
	}
	return user.IsAdmin
}

func (m *Manager) HandleAuthInfo(w http.ResponseWriter, r *http.Request) {
	backends := []map[string]string{
		{"id": "local", "name": "Local"},
	}
	if m.ldapConfig != nil {
		name := "LDAP"
		if m.ldapConfig.Host != "" {
			name = "LDAP (" + m.ldapConfig.Host + ")"
		}
		backends = append(backends, map[string]string{"id": "ldap", "name": name})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": backends})
}

func (m *Manager) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid request body"})
		return
	}

	var authenticated bool
	var isAdmin bool

	method := req.AuthMethod
	if method == "" {
		method = "local"
	}

	switch method {
	case "ldap":
		if m.ldapConfig == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "LDAP is not configured"})
			return
		}
		ldapOk, ldapAdmin, err := ldapAuthenticate(m.ldapConfig, req.Username, req.Password)
		if err != nil {
			log.Printf("LDAP: Error authenticating %s: %v", req.Username, err)
		}
		if ldapOk {
			authenticated = true
			isAdmin = ldapAdmin
		}
	default:
		if m.ValidateCredentials(req.Username, req.Password) {
			authenticated = true
			isAdmin = m.GetUserAdmin(req.Username)
		}
	}

	if !authenticated {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid username or password"})
		return
	}

	token, err := m.GenerateToken(req.Username, isAdmin)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to generate token"})
		return
	}

	log.Printf("Auth: User '%s' logged in", req.Username)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": LoginResponse{
			Token:    token,
			Username: req.Username,
			IsAdmin:  isAdmin,
		},
	})
}

func (m *Manager) JWTMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Authentication required"})
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := m.ValidateToken(tokenString)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid or expired token"})
			return
		}

		user, err := m.userStore.GetUser(claims.Username)
		if err != nil || !user.Enabled {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "User account disabled"})
			return
		}

		next(w, r)
	}
}

func (m *Manager) LDAPEnabled() bool {
	return m.ldapConfig != nil
}

func (m *Manager) LDAPHost() string {
	if m.ldapConfig != nil {
		return m.ldapConfig.Host
	}
	return ""
}

func generateTokenID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
