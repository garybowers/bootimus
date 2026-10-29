package replica

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bootimus/internal/models"

	"golang.org/x/crypto/bcrypt"
)

func testBootHash(test *testing.T, password string) string {
	test.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		test.Fatal(err)
	}
	return string(hash)
}

func setProtectedPrimary(environment *env, imageHash, menuHash string, revision time.Time) {
	environment.primary.mu.Lock()
	environment.primary.data["/api/theme"] = models.MenuTheme{Title: "Protected", UpdatedAt: revision, BootPasswordRequired: menuHash != "", BootPasswordHash: menuHash}
	environment.primary.data["/api/images"] = []models.Image{{ID: 7, Name: "Protected", Filename: "protected.iso", UpdatedAt: revision, Enabled: true, Public: true, BootPasswordRequired: imageHash != "", BootPasswordHash: imageHash}}
	environment.primary.mu.Unlock()
}

func replicaHashes(test *testing.T, environment *env) (string, string) {
	test.Helper()
	theme, err := environment.store.GetMenuTheme()
	if err != nil {
		test.Fatal(err)
	}
	image, err := environment.store.GetImage("protected.iso")
	if err != nil {
		test.Fatal(err)
	}
	return image.BootPasswordHash, theme.BootPasswordHash
}

func TestReplicaSyncsAndRotatesBootVerifiers(test *testing.T) {
	environment := newEnv(test, false)
	firstImage := testBootHash(test, "image-one")
	firstMenu := testBootHash(test, "menu-one")
	revision := time.Now().UTC().Add(-time.Minute)
	setProtectedPrimary(environment, firstImage, firstMenu, revision)
	if status := environment.syncer.RunNow(context.Background()); status.LastError != "" {
		test.Fatalf("initial sync: %s", status.LastError)
	}
	if imageHash, menuHash := replicaHashes(test, environment); imageHash != firstImage || menuHash != firstMenu {
		test.Fatal("password verifiers did not sync")
	}
	restarted := New(environment.syncer.cfg, environment.syncer.deps)
	restarted.primary.http = environment.syncer.primary.http
	if status := restarted.RunNow(context.Background()); status.LastError != "" {
		test.Fatalf("restart sync: %s", status.LastError)
	}
	secondImage := testBootHash(test, "image-two")
	setProtectedPrimary(environment, secondImage, firstMenu, revision.Add(time.Second))
	if status := environment.syncer.RunNow(context.Background()); status.LastError != "" {
		test.Fatalf("rotation sync: %s", status.LastError)
	}
	if imageHash, menuHash := replicaHashes(test, environment); imageHash != secondImage || menuHash != firstMenu {
		test.Fatal("image rotation affected menu verifier")
	}
	setProtectedPrimary(environment, "", "", revision.Add(2*time.Second))
	if status := environment.syncer.RunNow(context.Background()); status.LastError != "" {
		test.Fatalf("clear sync: %s", status.LastError)
	}
	if imageHash, menuHash := replicaHashes(test, environment); imageHash != "" || menuHash != "" {
		test.Fatal("password removal did not sync")
	}
}

func TestReplicaKeepsLastGoodVerifiersOnFeedFailure(test *testing.T) {
	environment := newEnv(test, false)
	imageHash := testBootHash(test, "image-one")
	menuHash := testBootHash(test, "menu-one")
	revision := time.Now().UTC().Add(-time.Minute)
	setProtectedPrimary(environment, imageHash, menuHash, revision)
	if status := environment.syncer.RunNow(context.Background()); status.LastError != "" {
		test.Fatalf("initial sync: %s", status.LastError)
	}
	environment.primary.mu.Lock()
	environment.primary.status["/api/sync/boot-verifiers"] = http.StatusNotFound
	environment.primary.mu.Unlock()
	status := environment.syncer.RunNow(context.Background())
	if status.LastError == "" || !strings.Contains(status.LastError, "boot-verifiers") {
		test.Fatalf("missing endpoint did not fail sync: %+v", status)
	}
	if gotImage, gotMenu := replicaHashes(test, environment); gotImage != imageHash || gotMenu != menuHash {
		test.Fatal("transient feed failure cleared matching last good verifiers")
	}
	setProtectedPrimary(environment, testBootHash(test, "image-two"), testBootHash(test, "menu-two"), revision.Add(time.Second))
	status = environment.syncer.RunNow(context.Background())
	if status.LastError == "" {
		test.Fatalf("changed policy did not fail: %+v", status)
	}
	if gotImage, gotMenu := replicaHashes(test, environment); gotImage != "" || gotMenu != "" {
		test.Fatal("changed revisions retained old verifiers")
	}
	environment.primary.mu.Lock()
	delete(environment.primary.status, "/api/sync/boot-verifiers")
	environment.primary.admin = false
	environment.primary.mu.Unlock()
	status = environment.syncer.RunNow(context.Background())
	if status.LastError == "" || !strings.Contains(status.LastError, "higher scope") {
		test.Fatalf("read token did not fail verifier sync: %+v", status)
	}
	if gotImage, gotMenu := replicaHashes(test, environment); gotImage != "" || gotMenu != "" {
		test.Fatal("read token changed protected policy")
	}
}

func TestReplicaBlocksNewProtectionWithoutVerifier(test *testing.T) {
	environment := newEnv(test, false)
	revision := time.Now().UTC().Add(-time.Minute)
	setProtectedPrimary(environment, "", "", revision)
	if status := environment.syncer.RunNow(context.Background()); status.LastError != "" {
		test.Fatalf("initial sync: %s", status.LastError)
	}
	setProtectedPrimary(environment, testBootHash(test, "image"), testBootHash(test, "menu"), revision.Add(time.Second))
	environment.primary.mu.Lock()
	environment.primary.admin = false
	environment.primary.mu.Unlock()
	status := environment.syncer.RunNow(context.Background())
	if status.LastError == "" {
		test.Fatalf("new protection did not fail safely: %+v", status)
	}
	if imageHash, menuHash := replicaHashes(test, environment); imageHash != "" || menuHash != "" {
		test.Fatal("new image protection kept a verifier")
	}
	image, err := environment.store.GetImage("protected.iso")
	if err != nil || !image.BootPasswordRequired {
		test.Fatal("new image protection remained bootable")
	}
	theme, err := environment.store.GetMenuTheme()
	if err != nil || !theme.BootPasswordRequired {
		test.Fatal("new menu protection remained bootable")
	}
	environment.primary.mu.Lock()
	environment.primary.admin = true
	environment.primary.status["/api/sync/boot-verifiers"] = http.StatusNotFound
	environment.primary.mu.Unlock()
	status = environment.syncer.RunNow(context.Background())
	if status.LastError == "" {
		test.Fatal("missing endpoint accepted new protection")
	}
	environment.primary.mu.Lock()
	delete(environment.primary.status, "/api/sync/boot-verifiers")
	environment.primary.mu.Unlock()
	if status := environment.syncer.RunNow(context.Background()); status.LastError != "" {
		test.Fatalf("recovery sync: %s", status.LastError)
	}
	if imageHash, menuHash := replicaHashes(test, environment); imageHash == "" || menuHash == "" {
		test.Fatal("recovery did not apply verifiers")
	}
}

func TestReplicaDefersMenuRemovalWhenImageVerifierFails(test *testing.T) {
	environment := newEnv(test, false)
	menuHash := testBootHash(test, "menu")
	revision := time.Now().UTC().Add(-time.Minute)
	setProtectedPrimary(environment, "", menuHash, revision)
	if status := environment.syncer.RunNow(context.Background()); status.LastError != "" {
		test.Fatalf("initial sync: %s", status.LastError)
	}
	setProtectedPrimary(environment, testBootHash(test, "image"), "", revision.Add(time.Second))
	environment.primary.mu.Lock()
	environment.primary.status["/api/sync/boot-verifiers"] = http.StatusNotFound
	environment.primary.mu.Unlock()
	status := environment.syncer.RunNow(context.Background())
	if status.LastError == "" || !kind(status, "theme").Skipped {
		test.Fatalf("menu removal was not deferred: %+v", status)
	}
	if imageHash, retainedMenu := replicaHashes(test, environment); imageHash != "" || retainedMenu != menuHash {
		test.Fatal("failed move exposed the menu or image")
	}
}

func TestReplicaBlocksOnMismatchedBootVerifierFeed(test *testing.T) {
	environment := newEnv(test, false)
	revision := time.Now().UTC().Add(-time.Minute)
	setProtectedPrimary(environment, "", "", revision)
	if status := environment.syncer.RunNow(context.Background()); status.LastError != "" {
		test.Fatalf("initial sync: %s", status.LastError)
	}
	setProtectedPrimary(environment, testBootHash(test, "image"), testBootHash(test, "menu"), revision.Add(time.Second))
	environment.primary.mu.Lock()
	environment.primary.data["/api/sync/boot-verifiers"] = bootVerifierFeed{Version: 1, Images: []bootVerifier{{Filename: "protected.iso", UpdatedAt: revision, Required: true, Hash: testBootHash(test, "wrong")}}, Theme: bootVerifier{UpdatedAt: revision, Required: true, Hash: testBootHash(test, "wrong")}}
	environment.primary.mu.Unlock()
	status := environment.syncer.RunNow(context.Background())
	if status.LastError == "" {
		test.Fatal("mismatched feed accepted")
	}
	if imageHash, menuHash := replicaHashes(test, environment); imageHash != "" || menuHash != "" {
		test.Fatal("mismatched feed left an unlockable protected boot")
	}
	image, _ := environment.store.GetImage("protected.iso")
	theme, _ := environment.store.GetMenuTheme()
	if !image.BootPasswordRequired || !theme.BootPasswordRequired {
		test.Fatal("mismatched feed did not block newly protected boot")
	}
}

func TestReplicaRejectsUntrustedBootVerifierTransport(test *testing.T) {
	environment := newEnv(test, false)
	revision := time.Now().UTC().Add(-time.Minute)
	setProtectedPrimary(environment, testBootHash(test, "image"), "", revision)
	environment.syncer.primary.base = "http://primary.invalid"
	if _, err := environment.syncer.bootVerifiers(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		test.Fatalf("HTTP verifier sync error = %v", err)
	}
	untrusted := newPrimary(environment.syncer.cfg)
	if err := untrusted.getBootVerifiers(context.Background(), &bootVerifierFeed{}); err == nil {
		test.Fatal("accepted an untrusted primary certificate")
	}
}

func TestReplicaBlocksNewProtectionFromHTTPPrimary(test *testing.T) {
	environment := newEnv(test, false)
	revision := time.Now().UTC().Add(-time.Minute)
	setProtectedPrimary(environment, "", "", revision)
	if status := environment.syncer.RunNow(context.Background()); status.LastError != "" {
		test.Fatalf("initial sync: %s", status.LastError)
	}
	imageHash, menuHash := testBootHash(test, "image"), testBootHash(test, "menu")
	setProtectedPrimary(environment, imageHash, menuHash, revision.Add(time.Second))
	httpPrimary := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		environment.primary.mu.Lock()
		payload := environment.primary.data[request.URL.Path]
		environment.primary.mu.Unlock()
		json.NewEncoder(writer).Encode(map[string]interface{}{"success": true, "data": payload})
	}))
	defer httpPrimary.Close()
	environment.syncer.primary.base = httpPrimary.URL
	if _, err := environment.syncer.syncImages(context.Background(), &syncRun{imageGroups: map[uint]uint{}, imageIDs: map[uint]string{}}); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		test.Fatalf("image sync over HTTP = %v", err)
	}
	if _, err := environment.syncer.syncTheme(context.Background(), &syncRun{}); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		test.Fatalf("menu sync over HTTP = %v", err)
	}
	if imageHash, menuHash := replicaHashes(test, environment); imageHash != "" || menuHash != "" {
		test.Fatal("HTTP primary left a newly protected boot unlockable")
	}
	image, _ := environment.store.GetImage("protected.iso")
	theme, _ := environment.store.GetMenuTheme()
	if !image.BootPasswordRequired || !theme.BootPasswordRequired {
		test.Fatal("HTTP primary did not block observed protection")
	}
}

func TestReplicaVerifierFeedDoesNotFollowHTTPRedirect(test *testing.T) {
	var targetHits atomic.Int32
	httpTarget := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		targetHits.Add(1)
		writer.WriteHeader(http.StatusOK)
	}))
	defer httpTarget.Close()
	tlsPrimary := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, httpTarget.URL, http.StatusFound)
	}))
	defer tlsPrimary.Close()
	primary := newPrimary(Config{PrimaryURL: tlsPrimary.URL, Token: "btk_test_secret"})
	primary.http = tlsPrimary.Client()
	if err := primary.getBootVerifiers(context.Background(), &bootVerifierFeed{}); err == nil {
		test.Fatal("accepted a redirected verifier feed")
	}
	if targetHits.Load() != 0 {
		test.Fatal("verifier request followed an HTTP redirect")
	}
}

func TestBootVerifierFeedRejectsMismatchedRevision(test *testing.T) {
	revision := time.Now().UTC()
	hash := testBootHash(test, "image")
	feed := bootVerifierFeed{Images: []bootVerifier{{Filename: "test.iso", UpdatedAt: revision.Add(time.Second), Required: true, Hash: hash}}}
	images := []models.Image{{Filename: "test.iso", UpdatedAt: revision, BootPasswordRequired: true}}
	if _, err := feed.imageHashes(images); err == nil {
		test.Fatal("accepted mismatched image revision")
	}
	feed.Images[0].UpdatedAt = revision
	feed.Images[0].Hash = ""
	if _, err := feed.imageHashes(images); err == nil {
		test.Fatal("accepted missing verifier")
	}
	feed.Images[0].Hash = hash
	feed.Images[0].Revision = strings.Repeat("a", 32)
	if _, err := feed.imageHashes(images); err == nil {
		test.Fatal("accepted mismatched password revision")
	}
}

func TestReplicaPasswordRevisionControlsFailureGate(test *testing.T) {
	environment := newEnv(test, false)
	initialHash := testBootHash(test, "first")
	revision := time.Now().UTC().Add(-time.Minute)
	firstPolicy := strings.Repeat("1", 32)
	setProtectedPrimary(environment, initialHash, "", revision)
	environment.primary.mu.Lock()
	images := environment.primary.data["/api/images"].([]models.Image)
	images[0].BootPasswordRevision = firstPolicy
	environment.primary.data["/api/images"] = images
	environment.primary.mu.Unlock()
	if status := environment.syncer.RunNow(context.Background()); status.LastError != "" {
		test.Fatalf("initial sync: %s", status.LastError)
	}
	setProtectedPrimary(environment, initialHash, "", revision.Add(time.Second))
	environment.primary.mu.Lock()
	images = environment.primary.data["/api/images"].([]models.Image)
	images[0].BootPasswordRevision = firstPolicy
	environment.primary.data["/api/images"] = images
	environment.primary.status["/api/sync/boot-verifiers"] = http.StatusNotFound
	environment.primary.mu.Unlock()
	if status := environment.syncer.RunNow(context.Background()); status.LastError == "" {
		test.Fatal("missing feed reported success")
	}
	if hash, _ := replicaHashes(test, environment); hash != initialHash {
		test.Fatal("unrelated metadata change invalidated the same password revision")
	}
	setProtectedPrimary(environment, testBootHash(test, "second"), "", revision.Add(time.Second))
	environment.primary.mu.Lock()
	images = environment.primary.data["/api/images"].([]models.Image)
	images[0].BootPasswordRevision = strings.Repeat("2", 32)
	environment.primary.data["/api/images"] = images
	environment.primary.mu.Unlock()
	if status := environment.syncer.RunNow(context.Background()); status.LastError == "" {
		test.Fatal("same-time password rotation reported success without feed")
	}
	if hash, _ := replicaHashes(test, environment); hash != "" {
		test.Fatal("same-time password rotation retained stale verifier")
	}
}
