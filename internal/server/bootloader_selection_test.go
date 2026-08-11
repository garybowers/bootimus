package server

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"bootimus/internal/models"
	"bootimus/internal/storage"
)

func newBootloaderSelectionTestServer(t *testing.T) (*Server, *storage.SQLiteStore) {
	t.Helper()
	store, err := storage.NewSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	if err := store.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	bootDir := t.TempDir()
	setDir := filepath.Join(bootDir, "se350")
	if err := os.MkdirAll(setDir, 0o755); err != nil {
		t.Fatalf("create bootloader set: %v", err)
	}
	manifest := `{
  "name": "se350",
  "bootfiles": {
    "bios": "undionly-se350.kpxe",
    "uefi": "bootimus-se350.efi",
    "arm64": "bootimus-se350-arm64.efi"
  }
}`
	if err := os.WriteFile(filepath.Join(setDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	return &Server{config: &Config{Storage: store, BootDir: bootDir}}, store
}

func mustParseMAC(t *testing.T, value string) net.HardwareAddr {
	t.Helper()
	mac, err := net.ParseMAC(value)
	if err != nil {
		t.Fatalf("ParseMAC(%q): %v", value, err)
	}
	return mac
}

func TestEffectiveBootloaderSetPrecedence(t *testing.T) {
	server, store := newBootloaderSelectionTestServer(t)
	server.SetActiveBootloaderSet("default")

	group := &models.ClientGroup{Name: "rack-a", BootloaderSet: "secureboot"}
	if err := store.CreateClientGroup(group); err != nil {
		t.Fatalf("CreateClientGroup: %v", err)
	}

	clients := []*models.Client{
		{MACAddress: "02:00:00:00:00:01", BootloaderSet: "se350", ClientGroupID: &group.ID},
		{MACAddress: "02:00:00:00:00:02", ClientGroupID: &group.ID},
		{MACAddress: "02:00:00:00:00:03"},
	}
	for _, client := range clients {
		if err := store.CreateClient(client); err != nil {
			t.Fatalf("CreateClient(%s): %v", client.MACAddress, err)
		}
	}

	tests := []struct {
		name       string
		mac        string
		wantSet    string
		wantDirect bool
	}{
		{name: "client overrides group", mac: clients[0].MACAddress, wantSet: "se350", wantDirect: true},
		{name: "group overrides global", mac: clients[1].MACAddress, wantSet: "secureboot", wantDirect: true},
		{name: "global fallback", mac: clients[2].MACAddress, wantSet: "default", wantDirect: false},
		{name: "unknown client", mac: "02:00:00:00:00:ff", wantSet: "default", wantDirect: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSet, gotDirect := server.effectiveBootloaderSet(mustParseMAC(t, tt.mac))
			if gotSet != tt.wantSet || gotDirect != tt.wantDirect {
				t.Fatalf("effectiveBootloaderSet = (%q, %v), want (%q, %v)", gotSet, gotDirect, tt.wantSet, tt.wantDirect)
			}
		})
	}
}

func TestProxyDHCPBootfilesForClientQualifiesOverrides(t *testing.T) {
	server, store := newBootloaderSelectionTestServer(t)
	server.SetActiveBootloaderSet("default")

	direct := &models.Client{MACAddress: "02:00:00:00:01:01", BootloaderSet: "se350"}
	inherited := &models.Client{MACAddress: "02:00:00:00:01:02"}
	secureBoot := &models.Client{MACAddress: "02:00:00:00:01:03", BootloaderSet: "secureboot"}
	defaultOverride := &models.Client{MACAddress: "02:00:00:00:01:04", BootloaderSet: "default"}
	for _, client := range []*models.Client{direct, inherited, secureBoot, defaultOverride} {
		if err := store.CreateClient(client); err != nil {
			t.Fatalf("CreateClient(%s): %v", client.MACAddress, err)
		}
	}

	bios, uefi, arm64 := server.proxyDHCPBootfilesForClient(mustParseMAC(t, direct.MACAddress))
	if bios != "bootloader-sets/se350/undionly-se350.kpxe" {
		t.Errorf("BIOS bootfile = %q", bios)
	}
	if uefi != "bootloader-sets/se350/bootimus-se350.efi" {
		t.Errorf("UEFI bootfile = %q", uefi)
	}
	if arm64 != "bootloader-sets/se350/bootimus-se350-arm64.efi" {
		t.Errorf("ARM64 bootfile = %q", arm64)
	}

	bios, uefi, arm64 = server.proxyDHCPBootfilesForClient(mustParseMAC(t, inherited.MACAddress))
	if bios != "undionly.kpxe" || uefi != "bootimus.efi" || arm64 != "bootimus-arm64.efi" {
		t.Fatalf("global bootfiles = (%q, %q, %q), want unqualified defaults", bios, uefi, arm64)
	}

	bios, uefi, arm64 = server.proxyDHCPBootfilesForClient(mustParseMAC(t, secureBoot.MACAddress))
	if bios != "bootloader-sets/secureboot/undionly.kpxe" ||
		uefi != "bootloader-sets/secureboot/ipxe-shimx64.efi" ||
		arm64 != "bootloader-sets/secureboot/ipxe-shimaa64.efi" {
		t.Fatalf("secure boot files = (%q, %q, %q), want qualified manifest files", bios, uefi, arm64)
	}

	server.SetActiveBootloaderSet("se350")
	bios, uefi, arm64 = server.proxyDHCPBootfilesForClient(mustParseMAC(t, inherited.MACAddress))
	if bios != "undionly-se350.kpxe" || uefi != "bootimus-se350.efi" || arm64 != "bootimus-se350-arm64.efi" {
		t.Fatalf("global custom files = (%q, %q, %q), want unqualified manifest files", bios, uefi, arm64)
	}
	bios, uefi, arm64 = server.proxyDHCPBootfilesForClient(mustParseMAC(t, defaultOverride.MACAddress))
	if bios != "bootloader-sets/default/undionly.kpxe" ||
		uefi != "bootloader-sets/default/bootimus.efi" ||
		arm64 != "bootloader-sets/default/bootimus-arm64.efi" {
		t.Fatalf("default override files = (%q, %q, %q), want qualified default files", bios, uefi, arm64)
	}
}

func TestResolveBootloaderRequest(t *testing.T) {
	server := &Server{config: &Config{}}
	server.SetActiveBootloaderSet("default")

	tests := []struct {
		name         string
		request      string
		wantSet      string
		wantFilename string
		wantError    bool
	}{
		{name: "global request", request: "bootimus.efi", wantSet: "default", wantFilename: "bootimus.efi"},
		{name: "qualified request", request: "bootloader-sets/se350/bootimus.efi", wantSet: "se350", wantFilename: "bootimus.efi"},
		{name: "qualified nested file", request: "bootloader-sets/se350/efi/ipxe.efi", wantSet: "se350", wantFilename: "efi/ipxe.efi"},
		{name: "traversal", request: "../secret", wantError: true},
		{name: "windows traversal", request: `..\secret`, wantError: true},
		{name: "missing set filename", request: "bootloader-sets/se350", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSet, gotFilename, err := server.resolveBootloaderRequest(tt.request)
			if tt.wantError {
				if err == nil {
					t.Fatalf("resolveBootloaderRequest(%q) unexpectedly succeeded", tt.request)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveBootloaderRequest(%q): %v", tt.request, err)
			}
			if gotSet != tt.wantSet || gotFilename != tt.wantFilename {
				t.Fatalf("resolveBootloaderRequest = (%q, %q), want (%q, %q)", gotSet, gotFilename, tt.wantSet, tt.wantFilename)
			}
		})
	}
}

func TestResolveBootloaderFileStaysWithinSelectedSet(t *testing.T) {
	server, _ := newBootloaderSelectionTestServer(t)
	setDir := filepath.Join(server.config.BootDir, "se350")
	bootfile := filepath.Join(setDir, "bootimus-se350.efi")
	if err := os.WriteFile(bootfile, []byte("test"), 0o644); err != nil {
		t.Fatalf("write bootfile: %v", err)
	}

	if got := server.resolveBootloaderFile("se350", "bootimus-se350.efi"); got != bootfile {
		t.Fatalf("resolveBootloaderFile = %q, want %q", got, bootfile)
	}
	if got := server.resolveBootloaderFile("se350", "../manifest.json"); got != "" {
		t.Fatalf("traversal resolved outside selected set: %q", got)
	}
}
