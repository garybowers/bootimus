package replica

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"bootimus/internal/autoinstall"
	"bootimus/internal/models"
	"bootimus/internal/storage"
)

type fakePrimary struct {
	mu       sync.Mutex
	data     map[string]interface{}
	status   map[string]int
	isoBytes []byte
	token    string
	admin    bool
	hits     map[string]int
}

func newFakePrimary(t *testing.T) (*fakePrimary, *httptest.Server) {
	t.Helper()
	group := uint(7)
	san := uint(3)
	f := &fakePrimary{
		token:    "btk_test",
		admin:    true,
		isoBytes: []byte("not really an iso but long enough to count"),
		status:   map[string]int{},
		hits:     map[string]int{},
		data: map[string]interface{}{
			"/api/san-targets": []models.SANTarget{{ID: san, Name: "lun0", URI: "iscsi:10.0.0.9::::iqn.2024-01.lan:disk0", CHAPUsername: "u", CHAPPassword: "p"}},
			"/api/groups":      []models.ImageGroup{{ID: 7, Name: "linux", Enabled: true, Order: 2}, {ID: 8, Name: "servers", ParentID: &group, Enabled: true}},
			"/api/images": []models.Image{
				{ID: 11, Name: "Debian 12", Filename: "debian-12.iso", Size: 42, Enabled: true, Public: true, GroupID: &group, BootMethod: "sanboot", Description: "stable"},
				{ID: 12, Name: "Disabled", Filename: "old.iso", Size: 5, Enabled: false, BootMethod: "sanboot"},
			},
			"/api/client-groups": []models.ClientGroup{{ID: 5, Name: "rack-1", Enabled: true, SANTargetID: &san, IPMIPassword: "grp"}},
			"/api/clients": []models.Client{
				{MACAddress: "AA:BB:CC:DD:EE:01", Name: "node-1", Enabled: true, Static: true, ShowPublicImages: true, AllowedImages: models.StringSlice{"debian-12.iso"}, IPMIHost: "10.0.0.50", IPMIPassword: "secret"},
				{MACAddress: "aa:bb:cc:dd:ee:02", Name: "node-2", Enabled: true, Static: true, SANTargetID: &san, BootFromSAN: true},
			},
			"/api/theme":                 models.MenuTheme{Title: "HQ menu", MenuTimeout: 12, DefaultMenuItem: "local"},
			"/api/profiles":              []models.DistroProfile{{ProfileID: "custom-os", DisplayName: "Custom OS", Family: "debian", Custom: true, FilenamePatterns: models.StringSlice{"custom"}}, {ProfileID: "ubuntu", DisplayName: "Ubuntu", Custom: false}},
			"/api/tools":                 []models.BootTool{{Name: "memtest", DisplayName: "Memtest86+", Enabled: true, Downloaded: true, Order: 1}, {Name: "mytool", DisplayName: "My Tool", Custom: true, Enabled: true, Downloaded: false, KernelPath: "k", BootMethod: "kernel"}},
			"/api/autoinstall-files":     []autoinstall.File{{Distro: "debian", Filename: "preseed.cfg", Path: "debian/preseed.cfg"}},
			"/api/autoinstall-files/get": map[string]string{"content": "d-i debian-installer/locale string en_GB"},
			"/api/files":                 []models.CustomFile{{ID: 21, Filename: "post.sh", OriginalName: "post.sh", Public: true, Size: 4, ContentType: "text/x-sh"}},
			"/api/sync/tool-files":       []toolFile{{Path: "memtest.efi", Size: 6}},
			"/api/sync/manifest": map[string]interface{}{
				"boot_base_url":  "http://10.0.0.1:8080",
				"bootloader_set": "built-in",
				"kinds":          map[string]interface{}{},
			},
		},
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.hits[r.URL.Path]++
		if r.Header.Get("Authorization") != "Bearer "+f.token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if (r.URL.Path == "/api/autoinstall-files" || r.URL.Path == "/api/sync/boot-verifiers") && !f.admin {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if code, ok := f.status[r.URL.Path]; ok {
			w.WriteHeader(code)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "boom"})
			return
		}
		if r.URL.Path == "/api/sync/asset" {
			switch r.URL.Query().Get("kind") {
			case "iso":
				w.Write(f.isoBytes)
			case "tool":
				w.Write([]byte("efi!!!"))
			case "file":
				w.Write([]byte("#!sh"))
			}
			return
		}
		if r.URL.Path == "/api/sync/boot-verifiers" {
			if payload, overridden := f.data[r.URL.Path]; overridden {
				json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": payload})
				return
			}
			images := f.data["/api/images"].([]models.Image)
			theme := f.data["/api/theme"].(models.MenuTheme)
			verifiers := make([]bootVerifier, 0, len(images))
			for _, image := range images {
				verifiers = append(verifiers, bootVerifier{Filename: image.Filename, UpdatedAt: image.UpdatedAt, Required: image.BootPasswordRequired, Hash: image.BootPasswordHash, Revision: image.BootPasswordRevision})
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": bootVerifierFeed{Version: 1, Images: verifiers, Theme: bootVerifier{UpdatedAt: theme.UpdatedAt, Required: theme.BootPasswordRequired, Hash: theme.BootPasswordHash, Revision: theme.BootPasswordRevision}}})
			return
		}
		payload, ok := f.data[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "not found"})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": payload})
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

type env struct {
	store   *storage.SQLiteStore
	dataDir string
	lib     *autoinstall.Library
	syncer  *Syncer
	primary *fakePrimary
	extract []string
	selects []string
}

func newEnv(t *testing.T, syncISOs bool) *env {
	t.Helper()
	dataDir := t.TempDir()
	store, err := storage.NewSQLiteStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	lib, err := autoinstall.New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	f, srv := newFakePrimary(t)
	e := &env{store: store, dataDir: dataDir, lib: lib, primary: f}
	cfg := Config{PrimaryURL: srv.URL, Token: "btk_test", Interval: time.Minute, SyncISOs: syncISOs}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	e.syncer = New(cfg, Deps{
		Store:       store,
		HTTPClient:  srv.Client(),
		DataDir:     dataDir,
		ISODir:      filepath.Join(dataDir, "isos"),
		AutoInstall: lib,
		Extract: func(filename string) error {
			e.extract = append(e.extract, filename)
			return nil
		},
		SelectBootloaderSet: func(name string) error {
			e.selects = append(e.selects, name)
			return nil
		},
		BootloaderSetExists: func(name string) bool { return name == "" || name == "ipxe-custom" },
		ToolDir:             func(name string) string { return filepath.Join(dataDir, "tools", name) },
	})
	return e
}

func kind(status Status, name string) KindResult {
	for _, k := range status.Kinds {
		if k.Name == name {
			return k
		}
	}
	return KindResult{}
}

func TestSyncMirrorsEveryKind(t *testing.T) {
	e := newEnv(t, true)
	status := e.syncer.RunNow(context.Background())
	if status.LastError != "" {
		t.Fatalf("sync error: %s", status.LastError)
	}

	targets, _ := e.store.ListSANTargets()
	if len(targets) != 1 || targets[0].CHAPPassword != "p" {
		t.Fatalf("san targets = %+v", targets)
	}
	groups, _ := e.store.ListImageGroups()
	if len(groups) != 2 {
		t.Fatalf("image groups = %d, want 2", len(groups))
	}
	var child *models.ImageGroup
	for _, g := range groups {
		if g.Name == "servers" {
			child = g
		}
	}
	if child == nil || child.ParentID == nil {
		t.Fatalf("nested group not linked to its parent: %+v", child)
	}
	img, err := e.store.GetImage("debian-12.iso")
	if err != nil || img.Description != "stable" || img.GroupID == nil {
		t.Fatalf("image not mirrored: %+v %v", img, err)
	}
	if data, err := os.ReadFile(filepath.Join(e.dataDir, "isos", "debian-12.iso")); err != nil || string(data) != string(e.primary.isoBytes) {
		t.Fatalf("iso not downloaded: %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.dataDir, "isos", "old.iso")); err == nil {
		t.Fatal("disabled image should not be downloaded")
	}
	cg, err := e.store.GetClientGroupByName("rack-1")
	if err != nil || cg.SANTargetID == nil || *cg.SANTargetID != targets[0].ID || cg.IPMIPassword != "grp" {
		t.Fatalf("client group not mirrored: %+v %v", cg, err)
	}
	c1, err := e.store.GetClient("aa:bb:cc:dd:ee:01")
	if err != nil || c1.Name != "node-1" || c1.IPMIPassword != "secret" {
		t.Fatalf("client 1 not mirrored: %+v %v", c1, err)
	}
	if allowed, _ := e.store.GetClientImages("aa:bb:cc:dd:ee:01"); len(allowed) != 1 || allowed[0] != "debian-12.iso" {
		t.Fatalf("allowed images = %v", allowed)
	}
	c2, _ := e.store.GetClient("aa:bb:cc:dd:ee:02")
	if c2 == nil || c2.SANTargetID == nil || !c2.BootFromSAN {
		t.Fatalf("client 2 SAN mapping missing: %+v", c2)
	}
	theme, _ := e.store.GetMenuTheme()
	if theme.Title != "HQ menu" || theme.MenuTimeout != 12 {
		t.Fatalf("theme = %+v", theme)
	}
	if p, err := e.store.GetDistroProfile("custom-os"); err != nil || !p.Custom {
		t.Fatalf("custom profile missing: %v", err)
	}
	if _, err := e.store.GetDistroProfile("ubuntu-from-primary"); err == nil {
		t.Fatal("built-in profiles must not be copied")
	}
	tool, err := e.store.GetBootTool("mytool")
	if err != nil || !tool.Custom || !tool.Enabled {
		t.Fatalf("custom tool missing: %+v %v", tool, err)
	}
	if data, err := os.ReadFile(filepath.Join(e.dataDir, "tools", "memtest", "memtest.efi")); err != nil || string(data) != "efi!!!" {
		t.Fatalf("tool file not downloaded: %v", err)
	}
	if content, err := e.lib.Read("debian", "preseed.cfg"); err != nil || !strings.Contains(content, "en_GB") {
		t.Fatalf("autoinstall file not mirrored: %q %v", content, err)
	}
	files, _ := e.store.ListCustomFiles()
	if len(files) != 1 || files[0].Filename != "post.sh" {
		t.Fatalf("custom files = %+v", files)
	}
	if data, err := os.ReadFile(filepath.Join(e.dataDir, "files", "post.sh")); err != nil || string(data) != "#!sh" {
		t.Fatalf("custom file content: %v", err)
	}
	if len(e.selects) != 1 || e.selects[0] != "" {
		t.Fatalf("bootloader selection calls = %v", e.selects)
	}
	if e.syncer.BootBaseURL() != "http://10.0.0.1:8080" {
		t.Fatalf("boot base = %q", e.syncer.BootBaseURL())
	}
	state, _ := e.store.GetReplicaState()
	if state.LastSync == nil || state.Report == "" {
		t.Fatalf("state not persisted: %+v", state)
	}
}

func TestSyncRemovesVanishedRowsAndKeepsDiscoveredClients(t *testing.T) {
	e := newEnv(t, true)
	if err := e.store.CreateClient(&models.Client{MACAddress: "de:ad:be:ef:00:01", Name: "branch-discovered", Static: false, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	e.syncer.RunNow(context.Background())

	e.primary.mu.Lock()
	e.primary.data["/api/clients"] = []models.Client{{MACAddress: "aa:bb:cc:dd:ee:02", Name: "node-2", Enabled: true, Static: true}}
	e.primary.data["/api/images"] = []models.Image{}
	e.primary.data["/api/san-targets"] = []models.SANTarget{}
	e.primary.data["/api/client-groups"] = []models.ClientGroup{}
	e.primary.mu.Unlock()

	status := e.syncer.RunNow(context.Background())
	if status.LastError != "" {
		t.Fatalf("sync error: %s", status.LastError)
	}
	if _, err := e.store.GetClient("aa:bb:cc:dd:ee:01"); err == nil {
		t.Fatal("static client removed on the primary should be deleted")
	}
	if _, err := e.store.GetClient("de:ad:be:ef:00:01"); err != nil {
		t.Fatal("locally discovered client must survive")
	}
	if kind(status, "clients").Removed != 1 {
		t.Fatalf("clients result = %+v", kind(status, "clients"))
	}
	if _, err := e.store.GetImage("debian-12.iso"); err == nil {
		t.Fatal("vanished image row should be deleted")
	}
	if _, err := os.Stat(filepath.Join(e.dataDir, "isos", "debian-12.iso")); err == nil {
		t.Fatal("vanished image file should be removed when ISOs are synced")
	}
	if targets, _ := e.store.ListSANTargets(); len(targets) != 0 {
		t.Fatalf("san targets = %d, want 0", len(targets))
	}
}

func TestSyncIsolatesErrorsPerKind(t *testing.T) {
	e := newEnv(t, true)
	e.primary.mu.Lock()
	e.primary.status["/api/groups"] = http.StatusInternalServerError
	e.primary.mu.Unlock()

	status := e.syncer.RunNow(context.Background())
	if status.LastError == "" || !strings.Contains(status.LastError, "image_groups") {
		t.Fatalf("expected an image_groups error, got %q", status.LastError)
	}
	if kind(status, "image_groups").Error == "" {
		t.Fatal("image_groups should report its error")
	}
	if kind(status, "clients").Error != "" || kind(status, "theme").Error != "" {
		t.Fatalf("other kinds should still sync: %+v", status.Kinds)
	}
	if _, err := e.store.GetClient("aa:bb:cc:dd:ee:01"); err != nil {
		t.Fatal("clients should have synced despite the group failure")
	}
	if status.LastSuccess != nil {
		t.Fatal("a run with errors must not count as a success")
	}
}

func TestSyncSkipsAutoInstallWithReadToken(t *testing.T) {
	e := newEnv(t, false)
	e.primary.mu.Lock()
	e.primary.admin = false
	e.primary.mu.Unlock()

	status := e.syncer.RunNow(context.Background())
	if status.LastError != "" {
		t.Fatalf("sync error: %s", status.LastError)
	}
	k := kind(status, "autoinstall_files")
	if !k.Skipped || !strings.Contains(k.Note, "admin") {
		t.Fatalf("autoinstall result = %+v", k)
	}
	if _, err := os.Stat(filepath.Join(e.dataDir, "isos", "debian-12.iso")); err == nil {
		t.Fatal("ISOs must not be downloaded when sync_isos is off")
	}
	img, err := e.store.GetImage("debian-12.iso")
	if err != nil || !img.Enabled {
		t.Fatalf("image metadata should still be mirrored: %+v %v", img, err)
	}
	if e.syncer.ImageIsLocal("debian-12.iso") {
		t.Fatal("image should be reported as remote")
	}
	if len(e.extract) != 0 {
		t.Fatalf("no extraction expected without local files, got %v", e.extract)
	}
}

func TestSyncRedownloadsWhenSizeDiffers(t *testing.T) {
	e := newEnv(t, true)
	e.syncer.RunNow(context.Background())
	isoPath := filepath.Join(e.dataDir, "isos", "debian-12.iso")
	if err := os.WriteFile(isoPath, []byte("truncated"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.primary.mu.Lock()
	e.primary.hits["/api/sync/asset"] = 0
	e.primary.mu.Unlock()

	e.syncer.RunNow(context.Background())
	data, _ := os.ReadFile(isoPath)
	if string(data) != string(e.primary.isoBytes) {
		t.Fatalf("iso should be re-downloaded, got %q", data)
	}
	e.syncer.RunNow(context.Background())
	e.primary.mu.Lock()
	hits := e.primary.hits["/api/sync/asset"]
	e.primary.mu.Unlock()
	if hits != 1 {
		t.Fatalf("asset downloads after a stable run = %d, want 1", hits)
	}
}

func TestSyncReportsUnreachablePrimary(t *testing.T) {
	e := newEnv(t, true)
	cfg := e.syncer.cfg
	cfg.PrimaryURL = "http://127.0.0.1:1"
	s := New(cfg, e.syncer.deps)
	status := s.RunNow(context.Background())
	if status.LastError == "" || status.LastSuccess != nil {
		t.Fatalf("expected a failure, got %+v", status)
	}
	skipped := 0
	for _, k := range status.Kinds {
		if k.Skipped {
			skipped++
		}
	}
	if skipped == 0 {
		t.Fatal("later kinds should be skipped once the primary is unreachable")
	}
}

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		cfg  Config
		want bool
	}{
		{Config{}, true},
		{Config{PrimaryURL: "https://hq:8081", Token: "btk_x"}, true},
		{Config{PrimaryURL: "hq:8081", Token: "btk_x"}, false},
		{Config{PrimaryURL: "https://hq:8081"}, false},
		{Config{PrimaryURL: "https://hq:8081", Token: "btk_x", Interval: time.Second}, false},
	}
	for i, c := range cases {
		err := c.cfg.Validate()
		if (err == nil) != c.want {
			t.Errorf("case %d: err = %v, want ok=%v", i, err, c.want)
		}
	}
	cfg := Config{PrimaryURL: "https://user:pw@hq:8081/", Token: "btk_x"}
	if cfg.DisplayURL() != "https://hq:8081" {
		t.Fatalf("display url = %q", cfg.DisplayURL())
	}
}
