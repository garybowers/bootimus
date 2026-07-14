package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bootimus/internal/models"
)

type fakeUserStore struct {
	users map[string]*models.User
}

func (f *fakeUserStore) EnsureAdminUser() (string, string, bool, error) { return "admin", "", false, nil }
func (f *fakeUserStore) ResetAdminPassword() (string, error)            { return "", nil }
func (f *fakeUserStore) UpdateUserLastLogin(string) error               { return nil }
func (f *fakeUserStore) GetUser(username string) (*models.User, error) {
	u, ok := f.users[username]
	if !ok {
		return nil, http.ErrNoLocation
	}
	return u, nil
}

func TestAdminMiddlewareRequiresAdmin(t *testing.T) {
	store := &fakeUserStore{users: map[string]*models.User{
		"alice": {Username: "alice", Enabled: true, IsAdmin: true},
		"bob":   {Username: "bob", Enabled: true, IsAdmin: false},
	}}
	m := &Manager{userStore: store, jwtSecret: []byte("test-secret-0123456789")}

	tok := func(user string, isAdmin bool) string {
		s, err := m.GenerateToken(user, isAdmin)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	call := func(authHeader string) int {
		h := m.AdminMiddleware(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodPost, "/api/users", nil)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec.Code
	}

	if got := call(""); got != http.StatusUnauthorized {
		t.Fatalf("no token: want 401, got %d", got)
	}
	if got := call("Bearer " + tok("alice", true)); got != http.StatusOK {
		t.Fatalf("admin: want 200, got %d", got)
	}
	if got := call("Bearer " + tok("bob", false)); got != http.StatusForbidden {
		t.Fatalf("non-admin: want 403, got %d", got)
	}
	if got := call("Bearer " + tok("bob", true)); got != http.StatusForbidden {
		t.Fatalf("forged admin claim: want 403, got %d", got)
	}
	store.users["alice"].IsAdmin = false
	if got := call("Bearer " + tok("alice", true)); got != http.StatusForbidden {
		t.Fatalf("demoted admin: want 403, got %d", got)
	}
}
