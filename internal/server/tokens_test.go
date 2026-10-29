package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bootimus/internal/models"
	"bootimus/internal/storage/storagetest"

	"golang.org/x/crypto/bcrypt"
)

func createToken(t *testing.T, env *auditEnv, bearer, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)
	env.srv.audit.wait()
	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

func doAs(env *auditEnv, method, path, bearer string) int {
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)
	env.srv.audit.wait()
	return rec.Code
}

func TestAPITokenLifecycleThroughRoutes(t *testing.T) {
	env := newAuditEnv(t)

	code, resp := createToken(t, env, env.admin, `{"name":"ci","scope":"read"}`)
	if code != http.StatusCreated {
		t.Fatalf("create read token: %d %+v", code, resp)
	}
	data := resp["data"].(map[string]interface{})
	readSecret := data["secret"].(string)
	if !strings.HasPrefix(readSecret, "btk_") {
		t.Fatalf("secret %q lacks the btk_ prefix", readSecret)
	}
	if _, present := data["token"].(map[string]interface{})["hash"]; present {
		t.Fatal("token hash must never be returned")
	}

	code, resp = createToken(t, env, env.admin, `{"name":"deploy","scope":"admin","expires_in_days":30}`)
	if code != http.StatusCreated {
		t.Fatalf("create admin token: %d %+v", code, resp)
	}
	adminSecret := resp["data"].(map[string]interface{})["secret"].(string)
	adminID := resp["data"].(map[string]interface{})["token"].(map[string]interface{})["id"].(float64)

	if code := doAs(env, http.MethodGet, "/api/clients", readSecret); code != http.StatusOK {
		t.Errorf("read token GET /api/clients: %d", code)
	}
	if code := doAs(env, http.MethodGet, "/api/users", readSecret); code != http.StatusForbidden {
		t.Errorf("read token GET /api/users: want 403, got %d", code)
	}
	if code := doAs(env, http.MethodPut, "/api/theme", readSecret); code != http.StatusForbidden {
		t.Errorf("read token PUT /api/theme: want 403, got %d", code)
	}
	if code := doAs(env, http.MethodPut, "/api/theme", adminSecret); code != http.StatusOK {
		t.Errorf("admin token PUT /api/theme: want 200, got %d", code)
	}
	if code, _ := createToken(t, env, adminSecret, `{"name":"escalate","scope":"admin"}`); code != http.StatusForbidden {
		t.Errorf("admin token creating a token: want 403, got %d", code)
	}

	entries := env.entries(t)
	var seen bool
	for _, e := range entries {
		if e.Path == "/api/theme" && e.Username == "token:deploy" {
			seen = true
		}
	}
	if !seen {
		t.Errorf("audit log should attribute the change to token:deploy, got %+v", entries)
	}

	if code := doAs(env, http.MethodDelete, "/api/tokens?id="+strings.TrimSuffix(strings.TrimSuffix(jsonNumber(adminID), ".0"), ".000000"), env.admin); code != http.StatusOK {
		t.Fatalf("revoke: %d", code)
	}
	if code := doAs(env, http.MethodPut, "/api/theme", adminSecret); code != http.StatusUnauthorized {
		t.Errorf("revoked token: want 401, got %d", code)
	}
	tokens, err := env.store.ListAPITokens()
	if err != nil || len(tokens) != 2 {
		t.Fatalf("ListAPITokens: %v %d", err, len(tokens))
	}
	for _, tok := range tokens {
		if tok.Name == "deploy" && (!tok.Revoked || tok.ExpiresAt == nil || tok.LastUsedAt == nil) {
			t.Errorf("revoked admin token state wrong: %+v", tok)
		}
		if tok.Scope != models.APITokenScopeRead && tok.Scope != models.APITokenScopeAdmin {
			t.Errorf("bad scope %+v", tok)
		}
	}
}

func jsonNumber(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}

func TestBootVerifierSyncRequiresAdminTokenAndHTTPS(t *testing.T) {
	env := newAuditEnv(t)
	hashBytes, err := bcrypt.GenerateFromPassword([]byte("private"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	hash := string(hashBytes)
	image := &models.Image{Name: "Protected", Filename: "protected.iso", Enabled: true, BootPasswordRequired: true, BootPasswordHash: hash}
	if err := env.store.CreateImage(image); err != nil {
		t.Fatal(err)
	}
	theme, err := env.store.GetMenuTheme()
	if err != nil {
		t.Fatal(err)
	}
	theme.BootPasswordRequired, theme.BootPasswordHash = true, hash
	if err := env.store.UpdateMenuTheme(theme); err != nil {
		t.Fatal(err)
	}
	_, readData := createToken(t, env, env.admin, `{"name":"reader","scope":"read"}`)
	_, adminData := createToken(t, env, env.admin, `{"name":"replica","scope":"admin"}`)
	readToken := readData["data"].(map[string]interface{})["secret"].(string)
	adminToken := adminData["data"].(map[string]interface{})["secret"].(string)
	call := func(scheme, path, token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, scheme+"://bootimus"+path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		env.handler.ServeHTTP(recorder, request)
		return recorder
	}
	if response := call("https", "/api/sync/boot-verifiers", readToken); response.Code != http.StatusForbidden {
		t.Fatalf("read token status = %d", response.Code)
	}
	if response := call("http", "/api/sync/boot-verifiers", adminToken); response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), hash) {
		t.Fatalf("HTTP verifier response = %d", response.Code)
	}
	response := call("https", "/api/sync/boot-verifiers", adminToken)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), hash) {
		t.Fatalf("HTTPS verifier response = %d %s", response.Code, response.Body.String())
	}
	for _, path := range []string{"/api/images", "/api/theme", "/api/sync/manifest"} {
		response := call("https", path, readToken)
		if strings.Contains(response.Body.String(), hash) {
			t.Errorf("read endpoint %s exposed a verifier", path)
		}
	}
	tlsServer := httptest.NewTLSServer(env.handler)
	defer tlsServer.Close()
	request, err := http.NewRequest(http.MethodGet, tlsServer.URL+"/api/sync/boot-verifiers", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+adminToken)
	liveResponse, err := tlsServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer liveResponse.Body.Close()
	body, err := io.ReadAll(liveResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	if liveResponse.StatusCode != http.StatusOK || !strings.Contains(string(body), hash) {
		t.Fatalf("live TLS verifier response = %d %s", liveResponse.StatusCode, body)
	}
}

func TestBootVerifierSyncDeniedWithoutAuthentication(t *testing.T) {
	server := &Server{config: &Config{Storage: storagetest.New(t), DataDir: t.TempDir()}}
	mux := http.NewServeMux()
	server.setupAdminInterface(mux)
	t.Cleanup(server.audit.wait)
	request := httptest.NewRequest(http.MethodGet, "https://bootimus/api/sync/boot-verifiers", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unauthenticated server verifier response = %d", recorder.Code)
	}
}
