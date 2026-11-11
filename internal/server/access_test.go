package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"bootimus/internal/auth"
	"bootimus/internal/models"
	"bootimus/internal/storage"
	"bootimus/internal/storage/storagetest"
)

var viewerReadRoutes = map[string]bool{
	"/api/server-info":               true,
	"/api/stats":                     true,
	"/api/logs":                      true,
	"/api/clients":                   true,
	"/api/images":                    true,
	"/api/clusters":                  true,
	"/api/clusters/nodes":            true,
	"/api/node-images":               true,
	"/api/kubernetes/providers":      true,
	"/api/clients/inventory":         true,
	"/api/clients/inventory/history": true,
	"/api/bootloaders":               true,
	"/api/tools":                     true,
	"/api/tools/progress":            true,
	"/api/images/extract-progress":   true,
	"/api/images/boot-candidates":    true,
	"/api/profiles":                  true,
	"/api/iso-catalog":               true,
	"/api/active-sessions":           true,
	"/api/downloads":                 true,
	"/api/downloads/progress":        true,
	"/api/files":                     true,
	"/api/drivers":                   true,
	"/api/drivers/status":            true,
	"/api/groups":                    true,
	"/api/client-groups":             true,
	"/api/client-groups/get":         true,
	"/api/scheduled-tasks":           true,
	"/api/clients/power/status":      true,
	"/api/clients/menu-preview":      true,
	"/api/disk-images":               true,
	"/api/disk-images/jobs":          true,
	"/api/clients/installs":          true,
	"/api/clients/timeline":          true,
	"/api/installs":                  true,
	"/api/san-targets":               true,
	"/api/deployment-profiles":       true,
	"/api/clients/effective":         true,
	"/api/enrolment-rules":           true,
	"/api/theme":                     true,
	"/api/usb":                       true,
	"/api/usb/download":              true,
	"/api/images/files":              true,
	"/api/sync/manifest":             true,
	"/api/sync/tool-files":           true,
	"/api/sync/asset":                true,
	"/api/replica/status":            true,
	"/api/housekeeping":              true,
	"/api/proxydhcp/events":          true,
	"/api/proxydhcp/stream":          true,
}

var selfServiceRoutes = map[string]bool{
	"/api/me/totp/setup":   true,
	"/api/me/totp/enable":  true,
	"/api/me/totp/disable": true,
}

var publicRoutes = map[string]bool{
	"/api/isos":          true,
	"/api/auth-info":     true,
	"/api/login":         true,
	"/api/me":            true,
	"/api/oidc/login":    true,
	"/api/oidc/callback": true,
	"/api/openapi.json":  true,
}

func registeredAPIRoutes(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	var routes []string
	for _, m := range regexp.MustCompile(`mux\.HandleFunc\("(/api/[^"]+)"`).FindAllStringSubmatch(string(src), -1) {
		routes = append(routes, m[1])
	}
	if len(routes) < 50 {
		t.Fatalf("only found %d API routes in server.go; has route registration moved?", len(routes))
	}
	return routes
}

type accessEnv struct {
	handler http.Handler
	server  *Server
	store   storage.Storage
	admin   string
	viewer  string
}

func newAccessEnv(t *testing.T) *accessEnv {
	t.Helper()
	store := storagetest.New(t)
	mgr, err := auth.NewManager(store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	viewer := &models.User{Username: "viewer", Enabled: true, Source: models.UserSourceLocal}
	if err := viewer.SetPassword("viewer-password"); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateUser(viewer); err != nil {
		t.Fatal(err)
	}

	srv := &Server{config: &Config{Storage: store, Auth: mgr, ServerAddr: "10.0.0.1", HTTPPort: 8080, DataDir: t.TempDir()}}
	mux := http.NewServeMux()
	srv.setupAdminInterface(mux)
	t.Cleanup(srv.audit.wait)

	adminTok, _ := mgr.GenerateToken("admin", true)
	viewerTok, _ := mgr.GenerateToken("viewer", false)
	return &accessEnv{handler: mux, server: srv, store: store, admin: adminTok, viewer: viewerTok}
}

func (e *accessEnv) do(method, path, token string) (code int, body string) {
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	func() {
		defer func() {
			if r := recover(); r != nil {
				rec.Code = -1
				rec.Body.WriteString(fmt.Sprint(r))
			}
		}()
		e.handler.ServeHTTP(rec, req)
	}()
	return rec.Code, rec.Body.String()
}

func TestViewerAccessToEveryAPIRoute(t *testing.T) {
	env := newAccessEnv(t)
	for _, route := range registeredAPIRoutes(t) {
		if publicRoutes[route] {
			continue
		}
		if code, _ := env.do(http.MethodGet, route, ""); code != http.StatusUnauthorized {
			t.Errorf("GET %s without a token: want 401, got %d", route, code)
		}
		if selfServiceRoutes[route] {
			continue
		}
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			if code, _ := env.do(method, route, env.viewer); code != http.StatusForbidden {
				t.Errorf("%s %s as viewer: want 403, got %d", method, route, code)
			}
		}
		code, body := env.do(http.MethodGet, route, env.viewer)
		if viewerReadRoutes[route] {
			if code == http.StatusUnauthorized || code == http.StatusForbidden {
				t.Errorf("GET %s as viewer: want access, got %d %s", route, code, body)
			}
		} else if code != http.StatusForbidden {
			t.Errorf("GET %s as viewer: want 403 (route is not on the read-only list), got %d", route, code)
		}
	}
	for route := range viewerReadRoutes {
		found := false
		for _, r := range registeredAPIRoutes(t) {
			found = found || r == route
		}
		if !found {
			t.Errorf("read-only route %s is no longer registered", route)
		}
	}
}

func TestViewerDoesNotSeeIPMIPasswords(t *testing.T) {
	env := newAccessEnv(t)
	group := &models.ClientGroup{Name: "rack-1", IPMIPassword: "group-secret"}
	if err := env.store.CreateClientGroup(group); err != nil {
		t.Fatal(err)
	}
	client := &models.Client{
		MACAddress:    "aa:bb:cc:dd:ee:01",
		Name:          "node-1",
		IPMIHost:      "10.0.0.50",
		IPMIPassword:  "client-secret",
		ConfigPatch:   "machine:\n  token: patch-secret",
		ClientGroupID: &group.ID,
	}
	if err := env.store.CreateClient(client); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/api/clients", "/api/clients?mac=aa:bb:cc:dd:ee:01", "/api/client-groups", fmt.Sprintf("/api/client-groups/get?id=%d", group.ID)} {
		code, body := env.do(http.MethodGet, path, env.viewer)
		if code != http.StatusOK {
			t.Fatalf("GET %s as viewer: %d %s", path, code, body)
		}
		for _, secret := range []string{"client-secret", "group-secret", "patch-secret"} {
			if strings.Contains(body, secret) {
				t.Errorf("GET %s as viewer leaked %q", path, secret)
			}
		}
	}

	if _, body := env.do(http.MethodGet, "/api/clients?mac=aa:bb:cc:dd:ee:01", env.admin); !strings.Contains(body, "client-secret") {
		t.Errorf("admin no longer sees the IPMI password needed by the edit form: %s", body)
	}
	if _, body := env.do(http.MethodGet, "/api/clients", env.viewer); !strings.Contains(body, "10.0.0.50") {
		t.Errorf("viewer lost non-secret fields: %s", body)
	}
}
