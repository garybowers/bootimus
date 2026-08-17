package extractor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kdomanski/iso9660"
)

func writeTestISO(t *testing.T, isoPath string, files map[string]string) {
	t.Helper()

	w, err := iso9660.NewWriter()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Cleanup()

	for path, content := range files {
		if err := w.AddFile(bytes.NewReader([]byte(content)), path); err != nil {
			t.Fatalf("AddFile %s: %v", path, err)
		}
	}

	f, err := os.Create(isoPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := w.WriteTo(f, "TESTISO"); err != nil {
		t.Fatal(err)
	}
}

func extractTestISO(t *testing.T, isoName string, files map[string]string) (*BootFiles, string) {
	t.Helper()

	dataDir := t.TempDir()
	isoPath := filepath.Join(dataDir, isoName)
	writeTestISO(t, isoPath, files)

	ext, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	ext.SetProgress(NewProgressReporter())

	bootFiles, err := ext.Extract(isoPath)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	return bootFiles, dataDir
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestExtractKaliLiveLayout(t *testing.T) {
	files := map[string]string{
		"/live/vmlinuz":             "live-kernel",
		"/live/initrd.img":          "live-initrd",
		"/live/filesystem.squashfs": "live-squashfs",
		"/install.amd/vmlinuz":      "installer-kernel",
		"/install.amd/initrd.gz":    "installer-initrd",
		"/.disk/info":               "Kali GNU/Linux Rolling",
		"/isolinux/isolinux.cfg":    "default live",
	}

	bootFiles, dataDir := extractTestISO(t, "kali-linux-2099.1-live-amd64.iso", files)

	if bootFiles.Distro != "kali" {
		t.Errorf("distro = %q, want kali", bootFiles.Distro)
	}
	if bootFiles.NetbootRequired {
		t.Error("live image must not require netboot files")
	}
	if got := readFileString(t, bootFiles.Kernel); got != "live-kernel" {
		t.Errorf("extracted kernel content = %q, want live kernel (installer kernel won)", got)
	}
	if got := readFileString(t, bootFiles.Initrd); got != "live-initrd" {
		t.Errorf("extracted initrd content = %q, want live initrd (installer initrd won)", got)
	}
	if !strings.EqualFold(bootFiles.SquashfsPath, "iso/live/filesystem.squashfs") {
		t.Errorf("squashfs path = %q, want iso/live/filesystem.squashfs", bootFiles.SquashfsPath)
	}

	cacheDir := filepath.Join(dataDir, "kali-linux-2099.1-live-amd64")
	squashfsOnDisk := filepath.Join(cacheDir, filepath.FromSlash(bootFiles.SquashfsPath))
	if got := readFileString(t, squashfsOnDisk); got != "live-squashfs" {
		t.Errorf("squashfs content = %q, want live-squashfs", got)
	}
}

func TestExtractDebianNetinstLayout(t *testing.T) {
	files := map[string]string{
		"/install.amd/vmlinuz":   "installer-kernel",
		"/install.amd/initrd.gz": "installer-initrd",
		"/.disk/info":            "Debian GNU/Linux 13",
	}

	bootFiles, _ := extractTestISO(t, "debian-13.0.0-amd64-netinst.iso", files)

	if bootFiles.Distro != "debian" {
		t.Errorf("distro = %q, want debian", bootFiles.Distro)
	}
	if !bootFiles.NetbootRequired {
		t.Error("installer-only image should require netboot files")
	}
	if got := readFileString(t, bootFiles.Kernel); got != "installer-kernel" {
		t.Errorf("extracted kernel content = %q, want installer kernel", got)
	}
	if bootFiles.SquashfsPath != "" {
		t.Errorf("squashfs path = %q, want empty", bootFiles.SquashfsPath)
	}
}

func TestExtractUbuntuCasperLayout(t *testing.T) {
	files := map[string]string{
		"/casper/vmlinuz":             "casper-kernel",
		"/casper/initrd":              "casper-initrd",
		"/casper/filesystem.squashfs": "casper-squashfs",
		"/.disk/info":                 "Ubuntu 26.04 LTS",
	}

	bootFiles, _ := extractTestISO(t, "ubuntu-26.04-desktop-amd64.iso", files)

	if bootFiles.Distro != "ubuntu" {
		t.Errorf("distro = %q, want ubuntu", bootFiles.Distro)
	}
	if bootFiles.NetbootRequired {
		t.Error("casper live image must not require netboot files")
	}
	if got := readFileString(t, bootFiles.Kernel); got != "casper-kernel" {
		t.Errorf("extracted kernel content = %q, want casper kernel", got)
	}
	if !strings.EqualFold(bootFiles.SquashfsPath, "iso/casper/filesystem.squashfs") {
		t.Errorf("squashfs path = %q, want iso/casper/filesystem.squashfs", bootFiles.SquashfsPath)
	}
}

func TestExtractDetectsSecureBootShim(t *testing.T) {
	files := map[string]string{
		"/casper/vmlinuz":             "casper-kernel",
		"/casper/initrd":              "casper-initrd",
		"/casper/filesystem.squashfs": "casper-squashfs",
		"/EFI/BOOT/BOOTX64.EFI":       "distro-shim",
		"/.disk/info":                 "Ubuntu 26.04 LTS",
	}

	bootFiles, dataDir := extractTestISO(t, "ubuntu-26.04-desktop-amd64.iso", files)

	if !strings.EqualFold(bootFiles.ShimPath, "iso/EFI/BOOT/BOOTX64.EFI") {
		t.Errorf("shim path = %q, want iso/EFI/BOOT/BOOTX64.EFI", bootFiles.ShimPath)
	}

	cacheDir := filepath.Join(dataDir, "ubuntu-26.04-desktop-amd64")
	if got := readFileString(t, filepath.Join(cacheDir, filepath.FromSlash(bootFiles.ShimPath))); got != "distro-shim" {
		t.Errorf("shim content = %q, want distro-shim", got)
	}
}

func TestExtractNoShimLeavesPathEmpty(t *testing.T) {
	files := map[string]string{
		"/casper/vmlinuz": "casper-kernel",
		"/casper/initrd":  "casper-initrd",
		"/.disk/info":     "Ubuntu 26.04 LTS",
	}

	bootFiles, _ := extractTestISO(t, "ubuntu-26.04-live-server-amd64.iso", files)

	if bootFiles.ShimPath != "" {
		t.Errorf("shim path = %q, want empty", bootFiles.ShimPath)
	}
}

func TestDetectShimPrefersX64AndFallsBackToARM64(t *testing.T) {
	dir := t.TempDir()
	efiBoot := filepath.Join(dir, "iso", "efi", "boot")
	if err := os.MkdirAll(efiBoot, 0755); err != nil {
		t.Fatal(err)
	}

	if got := DetectShim(dir); got != "" {
		t.Errorf("empty tree: shim path = %q, want empty", got)
	}

	if err := os.WriteFile(filepath.Join(efiBoot, "bootaa64.efi"), []byte("arm"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectShim(dir); got != "iso/efi/boot/bootaa64.efi" {
		t.Errorf("arm64 only: shim path = %q, want iso/efi/boot/bootaa64.efi", got)
	}

	if err := os.WriteFile(filepath.Join(efiBoot, "BOOTX64.EFI"), []byte("x64"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectShim(dir); got != "iso/efi/boot/BOOTX64.EFI" {
		t.Errorf("both arches: shim path = %q, want iso/efi/boot/BOOTX64.EFI", got)
	}
}

func TestExtractFedoraLayout(t *testing.T) {
	files := map[string]string{
		"/images/pxeboot/vmlinuz":    "fedora-kernel",
		"/images/pxeboot/initrd.img": "fedora-initrd",
		"/.disk/info":                "Fedora 43",
	}

	bootFiles, _ := extractTestISO(t, "Fedora-Workstation-Live-x86_64-43.iso", files)

	if bootFiles.Distro != "fedora" {
		t.Errorf("distro = %q, want fedora", bootFiles.Distro)
	}
	if got := readFileString(t, bootFiles.Kernel); got != "fedora-kernel" {
		t.Errorf("extracted kernel content = %q, want fedora kernel", got)
	}
}
