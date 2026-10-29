package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"bootimus/internal/models"
	"bootimus/internal/storage"
	"bootimus/internal/storage/storagetest"

	"golang.org/x/crypto/bcrypt"
)

const bootTestMAC = "52:54:00:12:34:56"

func TestBootPasswordScriptsFreeCompletedResponses(test *testing.T) {
	server, _, image := newBootAuthServer(test, true)
	builder := testMenuBuilder(nil)
	builder.bootAuthURL = server.bootAuthenticationURL()
	scripts := map[string]string{
		"menu":       server.bootPasswordMenu(bootTestMAC),
		"image gate": builder.buildImagePasswordGate(image.ID),
		"image form": server.imagePasswordForm(bootTestMAC, image.ID),
	}
	for name, script := range scripts {
		test.Run(name, func(test *testing.T) {
			if !strings.Contains(script, "chain --autofree "+server.bootAuthenticationURL()+"##params=bootimus") {
				test.Fatal("completed authentication scripts must not remain in the image list")
			}
		})
	}
}

func TestBootPasswordRejectsLegacyAmbiguity(test *testing.T) {
	for _, scenario := range []struct {
		name   string
		stored string
		body   string
	}{
		{"literal plus", "a b", "password=a+b"},
		{"encoded space", "a b", "password=a%20b"},
		{"encoded plus", "a+b", "password=a%2Bb"},
		{"encoded semicolon", "a;b", "password=a%3Bb"},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			server := &Server{}
			request := httptest.NewRequest(http.MethodPost, "https://bootimus/boot-auth", strings.NewReader(scenario.body))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if err := request.ParseForm(); err != nil {
				test.Fatal(err)
			}
			response := httptest.NewRecorder()
			if server.checkBootPassword(response, request, bootTestHash(test, scenario.stored)) || response.Code != http.StatusForbidden {
				test.Fatal("legacy incompatible password accepted; it must be reset")
			}
		})
	}
}

func bootTestHash(test *testing.T, password string) string {
	test.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		test.Fatal(err)
	}
	return string(hash)
}

func newBootAuthServer(test *testing.T, global bool) (*Server, storage.Storage, *models.Image) {
	test.Helper()
	store := storagetest.New(test)
	server := &Server{config: &Config{Storage: store, DataDir: test.TempDir(), ServerAddr: "192.0.2.10", HTTPPort: 8080, AdminPort: 8443, AdminTLS: AdminTLSConfig{Enabled: true}}}
	if err := server.prepareAdminTLS(); err != nil {
		test.Fatal(err)
	}
	client := &models.Client{MACAddress: bootTestMAC, Enabled: true, ShowPublicImages: true}
	if err := store.CreateClient(client); err != nil {
		test.Fatal(err)
	}
	image := &models.Image{Name: "Protected image", Filename: "protected.iso", Enabled: true, Public: true, BootMethod: "kernel", BootPasswordRequired: true, BootPasswordHash: bootTestHash(test, "image&=%")}
	if err := store.CreateImage(image); err != nil {
		test.Fatal(err)
	}
	if global {
		theme, err := store.GetMenuTheme()
		if err != nil {
			test.Fatal(err)
		}
		theme.BootPasswordRequired, theme.BootPasswordHash = true, bootTestHash(test, "menu&=%")
		if err := store.UpdateMenuTheme(theme); err != nil {
			test.Fatal(err)
		}
	}
	return server, store, image
}

func bootAuthRequest(server *Server, fields url.Values) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "https://192.0.2.10:8443/boot-auth", strings.NewReader(fields.Encode()))
	request.RemoteAddr = "192.0.2.20:12345"
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server.handleBootAuthentication(recorder, request)
	return recorder
}

func unlockTestMenu(test *testing.T, server *Server) string {
	test.Helper()
	response := bootAuthRequest(server, url.Values{"action": {"menu"}, "mac": {bootTestMAC}, "password": {"menu&=%"}})
	if response.Code != http.StatusOK {
		test.Fatalf("menu authentication: %d", response.Code)
	}
	match := regexp.MustCompile(`set bootimus_session ([0-9]+\.[a-f0-9]+)`).FindStringSubmatch(response.Body.String())
	if len(match) != 2 {
		test.Fatal("authenticated menu has no session")
	}
	if !strings.Contains(response.Body.String(), "item lock Lock network-boot menu") || !strings.Contains(response.Body.String(), ":lock\nclear bootimus_password\nclear bootimus_session\nexit") {
		test.Fatal("authenticated menu has no way to lock and authenticate again")
	}
	if strings.Contains(response.Body.String(), "/boot/protected/") || strings.Contains(response.Body.String(), "menu&=%") {
		test.Fatal("menu exposed the protected image commands or password")
	}
	return match[1]
}

func TestBootPasswordsRequireSeparateChecks(test *testing.T) {
	for _, global := range []bool{false, true} {
		test.Run(fmt.Sprintf("global=%v", global), func(test *testing.T) {
			server, store, image := newBootAuthServer(test, global)
			if err := store.SetNextBootImage(bootTestMAC, image.Filename); err != nil {
				test.Fatal(err)
			}
			preview := server.renderBootMenu(bootTestMAC, "192.0.2.20:12345", true)
			if strings.Contains(preview.Script, "/boot/protected/") || strings.Contains(preview.Script, image.BootPasswordHash) {
				test.Fatal("unauthenticated menu exposes protected image commands or hash")
			}
			if global && (!strings.Contains(preview.Script, "Boot from Local Disk") || strings.Contains(preview.Script, "Protected image")) {
				test.Fatal("global protection must hide the menu and retain local boot")
			}
			fields := url.Values{"action": {"image"}, "mac": {bootTestMAC}, "image": {strconvImage(image)}}
			if global {
				fields.Set("password", "image&=%")
				if response := bootAuthRequest(server, fields); response.Code != http.StatusForbidden {
					test.Fatal("image password bypassed the global password")
				}
				fields.Del("password")
				fields.Set("session", unlockTestMenu(test, server))
			}
			response := bootAuthRequest(server, fields)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "item --secret bootimus_password") {
				test.Fatalf("image must prompt separately: %d", response.Code)
			}
			fields.Set("password", "wrong")
			if response = bootAuthRequest(server, fields); response.Code != http.StatusForbidden {
				test.Fatal("wrong password accepted")
			}
			client, err := store.GetClient(bootTestMAC)
			if err != nil || client.NextBootImage != image.Filename {
				test.Fatal("prompt or failure consumed the pending next boot")
			}
			fields.Set("password", "image&=%")
			response = bootAuthRequest(server, fields)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "kernel http://192.0.2.10:8080/boot/protected/vmlinuz") {
				test.Fatalf("correct image password failed: %d", response.Code)
			}
			if strings.Contains(response.Body.String(), "form ") || strings.Contains(response.Body.String(), "menu Bootimus") || strings.Contains(response.Body.String(), "image&=%") {
				test.Fatal("unlocked image script must not carry passwords or a reusable unlocked menu")
			}
			client, err = store.GetClient(bootTestMAC)
			if err != nil || client.NextBootImage != "" {
				test.Fatal("successful authentication did not consume next boot")
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				test.Fatal("authenticated script is cacheable")
			}
		})
	}
}

func strconvImage(image *models.Image) string {
	return fmt.Sprint(image.ID)
}

func TestBootSessionScopeExpiryAndRotation(test *testing.T) {
	now := time.Now()
	authentication := bootAuthentication{now: func() time.Time { return now }}
	token := authentication.session(bootTestMAC, "192.0.2.20", "first-hash")
	if !authentication.accepts(token, bootTestMAC, "192.0.2.20", "first-hash") {
		test.Fatal("fresh token rejected")
	}
	for _, invalid := range []struct{ mac, address, hash, token string }{
		{"52:54:00:12:34:57", "192.0.2.20", "first-hash", token},
		{bootTestMAC, "192.0.2.21", "first-hash", token},
		{bootTestMAC, "192.0.2.20", "rotated-hash", token},
		{bootTestMAC, "192.0.2.20", "first-hash", token + "x"},
		{bootTestMAC, "192.0.2.20", "", token},
	} {
		if authentication.accepts(invalid.token, invalid.mac, invalid.address, invalid.hash) {
			test.Fatal("invalid token accepted")
		}
	}
	now = now.Add(bootSessionLifetime)
	if authentication.accepts(token, bootTestMAC, "192.0.2.20", "first-hash") {
		test.Fatal("expired token accepted")
	}
}

func TestBootAuthenticationTransportAndInput(test *testing.T) {
	server, _, _ := newBootAuthServer(test, true)
	cases := []struct {
		name, method, endpoint, contentType, body string
		status                                    int
	}{
		{"HTTP", "POST", "http://192.0.2.10/boot-auth", "application/x-www-form-urlencoded", "action=menu&password=menu", 503},
		{"GET", "GET", "https://192.0.2.10/boot-auth", "", "", 405},
		{"query credentials", "POST", "https://192.0.2.10/boot-auth?password=menu", "application/x-www-form-urlencoded", "action=menu", 400},
		{"JSON", "POST", "https://192.0.2.10/boot-auth", "application/json", `{}`, 400},
		{"unescaped semicolon", "POST", "https://192.0.2.10/boot-auth", "application/x-www-form-urlencoded", "action=menu&password=a;b", 400},
		{"oversize", "POST", "https://192.0.2.10/boot-auth", "application/x-www-form-urlencoded", "password=" + strings.Repeat("a", 4096), 400},
		{"MAC injection", "POST", "https://192.0.2.10/boot-auth", "application/x-www-form-urlencoded", "mac=%0Aboot&action=menu", 400},
		{"unknown action", "POST", "https://192.0.2.10/boot-auth", "application/x-www-form-urlencoded", "action=other", 400},
	}
	for _, scenario := range cases {
		test.Run(scenario.name, func(test *testing.T) {
			request := httptest.NewRequest(scenario.method, scenario.endpoint, strings.NewReader(scenario.body))
			request.Header.Set("Content-Type", scenario.contentType)
			request.Header.Set("X-Forwarded-Proto", "https")
			recorder := httptest.NewRecorder()
			server.handleBootAuthentication(recorder, request)
			if recorder.Code != scenario.status {
				test.Fatalf("status = %d, want %d", recorder.Code, scenario.status)
			}
		})
	}
}

func TestBootPasswordRateLimit(test *testing.T) {
	server, _, image := newBootAuthServer(test, false)
	fields := url.Values{"action": {"image"}, "mac": {bootTestMAC}, "image": {strconvImage(image)}, "password": {"wrong"}}
	for attempt := 0; attempt < 5; attempt++ {
		if response := bootAuthRequest(server, fields); response.Code != http.StatusForbidden {
			test.Fatalf("attempt %d: status %d", attempt, response.Code)
		}
	}
	fields.Set("mac", "52:54:00:12:34:57")
	response := bootAuthRequest(server, fields)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "60" {
		test.Fatal("changing the claimed MAC bypassed the source-address rate limit")
	}
}

func TestBootAttemptLimitConcurrentAndBounded(test *testing.T) {
	now := time.Now()
	authentication := bootAuthentication{now: func() time.Time { return now }}
	var workers sync.WaitGroup
	accepted := make(chan bool, 30)
	for attempt := 0; attempt < 30; attempt++ {
		workers.Add(1)
		go func() { defer workers.Done(); accepted <- authentication.attempt("192.0.2.20") }()
	}
	workers.Wait()
	close(accepted)
	count := 0
	for allowed := range accepted {
		if allowed {
			count++
		}
	}
	if count != 5 {
		test.Fatalf("accepted %d concurrent attempts", count)
	}
	now = now.Add(time.Minute)
	if !authentication.attempt("192.0.2.20") {
		test.Fatal("window did not expire")
	}
	for identifier := 0; identifier < 4096; identifier++ {
		authentication.attempt(fmt.Sprint(identifier))
	}
	if len(authentication.attempts) != 4096 || authentication.attempt("overflow") {
		test.Fatal("rate limiter is unbounded")
	}
}

func TestProtectedImageAutomaticPaths(test *testing.T) {
	for _, selection := range []string{"menu", "next boot", "profile", "default", "group"} {
		test.Run(selection, func(test *testing.T) {
			builder := testMenuBuilder(nil)
			builder.bootAuthURL = "https://192.0.2.10:8443/boot-auth"
			builder.images = []models.Image{{ID: 7, Name: "Protected", Filename: "protected.iso", Enabled: true, BootMethod: "kernel", BootPasswordRequired: true}}
			switch selection {
			case "next boot":
				builder.nextBootImageID = 7
			case "profile":
				builder.profileImageID = 7
			case "default":
				builder.theme = &models.MenuTheme{DefaultMenuItem: ""}
			case "group":
				groupID := uint(3)
				builder.images[0].GroupID = &groupID
				builder.groups = []*models.ImageGroup{{ID: groupID, Name: "Group", Enabled: true}}
			}
			script := builder.Build()
			if strings.Contains(script, "\nkernel ") || !strings.Contains(script, "param --params bootimus image 7") || !strings.Contains(script, ":start\nmenu ") {
				test.Fatal("automatic selection bypasses protection or lacks a cancellation menu")
			}
		})
	}
}

func TestBootAuthenticationRejectsUnavailableImage(test *testing.T) {
	server, store, image := newBootAuthServer(test, false)
	image.Public = false
	if err := store.UpdateImage(image.Filename, image); err != nil {
		test.Fatal(err)
	}
	fields := url.Values{"action": {"image"}, "mac": {bootTestMAC}, "image": {strconvImage(image)}, "password": {"image&=%"}}
	if response := bootAuthRequest(server, fields); response.Code != http.StatusForbidden {
		test.Fatal("unassigned image unlocked")
	}
	image.Public, image.Enabled = true, false
	if err := store.UpdateImage(image.Filename, image); err != nil {
		test.Fatal(err)
	}
	if response := bootAuthRequest(server, fields); response.Code != http.StatusForbidden {
		test.Fatal("disabled image unlocked")
	}
}

func TestBootProtectionStorageFailureAndMissingTLS(test *testing.T) {
	server, store, _ := newBootAuthServer(test, true)
	server.adminTLS = nil
	if script := server.renderBootMenu(bootTestMAC, "", true).Script; strings.Contains(script, "kernel ") || !strings.Contains(script, "Boot from Local Disk") {
		test.Fatal("missing TLS did not fail closed")
	}
	if err := store.Close(); err != nil {
		test.Fatal(err)
	}
	if script := server.renderBootMenu(bootTestMAC, "", true).Script; strings.Contains(script, "kernel ") || !strings.Contains(script, "Protected boot is unavailable") {
		test.Fatal("database failure did not fail closed")
	}
}

func TestBootProtectionWithoutReplicaHashes(test *testing.T) {
	server, store, image := newBootAuthServer(test, false)
	image.BootPasswordHash = ""
	if err := store.UpdateImage(image.Filename, image); err != nil {
		test.Fatal(err)
	}
	fields := url.Values{"action": {"image"}, "mac": {bootTestMAC}, "image": {strconvImage(image)}, "password": {"image&=%"}}
	if response := bootAuthRequest(server, fields); response.Code != http.StatusServiceUnavailable {
		test.Fatal("an image without its verification hash must remain blocked")
	}
	theme, err := store.GetMenuTheme()
	if err != nil {
		test.Fatal(err)
	}
	theme.BootPasswordRequired = true
	if err := store.UpdateMenuTheme(theme); err != nil {
		test.Fatal(err)
	}
	if response := bootAuthRequest(server, url.Values{"action": {"menu"}, "mac": {bootTestMAC}}); response.Code != http.StatusServiceUnavailable {
		test.Fatal("a menu without its verification hash must remain blocked")
	}
	if rendered := server.renderBootMenu(bootTestMAC, "", true); rendered.Kind != "unavailable" || strings.Contains(rendered.Script, "Protected image") {
		test.Fatal("a replica exposed the protected menu")
	}
}

func TestBootPasswordDeploymentProfile(test *testing.T) {
	server, store, image := newBootAuthServer(test, false)
	image.Public = false
	if err := store.UpdateImage(image.Filename, image); err != nil {
		test.Fatal(err)
	}
	profile := &models.DeploymentProfile{Name: "protected install", ImageFilename: image.Filename, FirstBootAction: models.FirstBootActionInstall}
	if err := store.CreateDeploymentProfile(profile); err != nil {
		test.Fatal(err)
	}
	client, err := store.GetClient(bootTestMAC)
	if err != nil {
		test.Fatal(err)
	}
	client.ProfileID = &profile.ID
	if err := store.UpdateClient(bootTestMAC, client); err != nil {
		test.Fatal(err)
	}
	script := server.renderBootMenu(bootTestMAC, "192.0.2.20:12345", true).Script
	if !strings.Contains(script, "choose --default iso"+strconvImage(image)+" ") || strings.Contains(script, "\nkernel ") {
		test.Fatal("profile default must select the password gate")
	}
	fields := url.Values{"action": {"image"}, "mac": {bootTestMAC}, "image": {strconvImage(image)}}
	if response := bootAuthRequest(server, fields); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "form Image password") {
		test.Fatal("profile image must request its password")
	}
	fields.Set("password", "image&=%")
	if response := bootAuthRequest(server, fields); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "\nkernel ") {
		test.Fatal("correct password did not unlock the profile image")
	}
}

func TestGlobalBootPasswordPrecedesAutomaticActions(test *testing.T) {
	for _, action := range []string{"unprotected.iso", "san:123"} {
		test.Run(action, func(test *testing.T) {
			server, store, _ := newBootAuthServer(test, true)
			if err := store.SetNextBootImage(bootTestMAC, action); err != nil {
				test.Fatal(err)
			}
			client, err := store.GetClient(bootTestMAC)
			if err != nil {
				test.Fatal(err)
			}
			if err := store.SetClientEnrollmentState(bootTestMAC, models.EnrollmentStateInstalling); err != nil {
				test.Fatal(err)
			}
			if rendered := server.renderBootMenu(bootTestMAC, "", true); rendered.Kind != "locked" {
				test.Fatal("automatic action bypassed global password")
			}
			client, err = store.GetClient(bootTestMAC)
			if err != nil || client.NextBootImage != action || client.EnrollmentState != models.EnrollmentStateInstalling {
				test.Fatal("locked menu changed automatic boot state")
			}
		})
	}
}

func TestBootPasswordRotationRejectsMenuSession(test *testing.T) {
	server, store, image := newBootAuthServer(test, true)
	session := unlockTestMenu(test, server)
	theme, err := store.GetMenuTheme()
	if err != nil {
		test.Fatal(err)
	}
	theme.BootPasswordHash = bootTestHash(test, "changed-password")
	if err := store.UpdateMenuTheme(theme); err != nil {
		test.Fatal(err)
	}
	fields := url.Values{"action": {"image"}, "mac": {bootTestMAC}, "image": {strconvImage(image)}, "password": {"image&=%"}, "session": {session}}
	if response := bootAuthRequest(server, fields); response.Code != http.StatusForbidden {
		test.Fatal("password rotation did not invalidate the menu session")
	}
}
