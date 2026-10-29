package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"bootimus/internal/models"
	"bootimus/internal/replica"
	"bootimus/internal/storage/storagetest"
)

func TestReplicaBootPasswordsThroughLiveHandlers(test *testing.T) {
	primary := newAuditEnv(test)
	client := &models.Client{MACAddress: bootTestMAC, Enabled: true, ShowPublicImages: true}
	if err := primary.store.CreateClient(client); err != nil {
		test.Fatal(err)
	}
	primaryImage := &models.Image{Name: "Protected", Filename: "protected.iso", Enabled: true, Public: true, BootMethod: "kernel"}
	if err := primary.store.CreateImage(primaryImage); err != nil {
		test.Fatal(err)
	}
	primary.srv.config.AdminTLS = AdminTLSConfig{Enabled: true}
	primary.srv.config.AdminPort = 8443
	if err := primary.srv.prepareAdminTLS(); err != nil {
		test.Fatal(err)
	}
	setPrimaryPassword := func(path, password string) {
		test.Helper()
		request := httptest.NewRequest(http.MethodPut, "https://primary.example"+path, strings.NewReader(`{"boot_password":"`+password+`"}`))
		request.Header.Set("Authorization", "Bearer "+primary.admin)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		primary.handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			test.Fatalf("set primary password at %s: %d %s", path, recorder.Code, recorder.Body.String())
		}
	}
	setPrimaryPassword("/api/images?filename=protected.iso", "image-one")
	setPrimaryPassword("/api/theme", "menu-one")
	code, tokenResponse := createToken(test, primary, primary.admin, `{"name":"branch","scope":"admin"}`)
	if code != http.StatusCreated {
		test.Fatalf("create replica token: %d", code)
	}
	secret := tokenResponse["data"].(map[string]interface{})["secret"].(string)
	primaryHTTP := httptest.NewTLSServer(primary.handler)
	defer primaryHTTP.Close()

	replicaStore := storagetest.New(test)
	if err := replicaStore.CreateImage(&models.Image{Name: "Old local image", Filename: "old.iso", Enabled: true}); err != nil {
		test.Fatal(err)
	}
	replicaConfig := &replica.Config{PrimaryURL: primaryHTTP.URL, Token: secret, Interval: time.Minute, SyncISOs: false}
	replicaDir := test.TempDir()
	replicaServer := &Server{config: &Config{Storage: replicaStore, DataDir: replicaDir, ServerAddr: "192.0.2.10", HTTPPort: 8080, AdminPort: 8443, AdminTLS: AdminTLSConfig{Enabled: true}, Replica: replicaConfig}}
	if err := replicaServer.prepareAdminTLS(); err != nil {
		test.Fatal(err)
	}
	replicaServer.replica = replica.New(*replicaConfig, replica.Deps{Store: replicaStore, DataDir: replicaDir, HTTPClient: primaryHTTP.Client()})
	mux := http.NewServeMux()
	replicaServer.setupAdminInterface(mux)
	test.Cleanup(replicaServer.audit.wait)
	if status := replicaServer.replica.RunNow(context.Background()); status.LastError != "" {
		test.Fatalf("initial replica sync: %s", status.LastError)
	}
	localImage, err := replicaStore.GetImage("protected.iso")
	if err != nil || localImage.ID == primaryImage.ID {
		test.Fatalf("replica image ID mapping: image=%+v error=%v", localImage, err)
	}
	sourceAddress := "192.0.2.20:12345"
	boot := func(fields url.Values) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "https://192.0.2.10:8443/boot-auth", strings.NewReader(fields.Encode()))
		request.RemoteAddr = sourceAddress
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder
	}
	unlock := func(menuPassword, imagePassword string) {
		test.Helper()
		menu := boot(url.Values{"action": {"menu"}, "mac": {bootTestMAC}, "password": {menuPassword}})
		if menu.Code != http.StatusOK {
			test.Fatalf("menu unlock: %d %s", menu.Code, menu.Body.String())
		}
		match := regexp.MustCompile(`set bootimus_session ([0-9]+\.[a-f0-9]+)`).FindStringSubmatch(menu.Body.String())
		if len(match) != 2 {
			test.Fatal("menu unlock returned no session")
		}
		image := boot(url.Values{"action": {"image"}, "mac": {bootTestMAC}, "image": {strconvImage(localImage)}, "session": {match[1]}, "password": {imagePassword}})
		if image.Code != http.StatusOK || !strings.Contains(image.Body.String(), "/boot/protected/") {
			test.Fatalf("image unlock: %d %s", image.Code, image.Body.String())
		}
	}
	unlock("menu-one", "image-one")
	setPrimaryPassword("/api/images?filename=protected.iso", "image-two")
	setPrimaryPassword("/api/theme", "menu-two")
	if status := replicaServer.replica.RunNow(context.Background()); status.LastError != "" {
		test.Fatalf("rotation replica sync: %s", status.LastError)
	}
	if response := boot(url.Values{"action": {"menu"}, "mac": {bootTestMAC}, "password": {"menu-one"}}); response.Code != http.StatusForbidden {
		test.Fatalf("old menu password accepted: %d", response.Code)
	}
	sourceAddress = "192.0.2.21:12345"
	unlock("menu-two", "image-two")
	primaryHTTP.Close()
	if status := replicaServer.replica.RunNow(context.Background()); status.LastError == "" {
		test.Fatal("offline sync reported success")
	}
	sourceAddress = "192.0.2.22:12345"
	unlock("menu-two", "image-two")
}
