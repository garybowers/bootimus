package wim

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func captureWIM(t *testing.T, files ...string) string {
	t.Helper()
	tree := filepath.Join(t.TempDir(), "tree")
	for _, name := range append([]string{"setup.exe"}, files...) {
		path := filepath.Join(tree, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(t.TempDir(), "boot.wim")
	if output, err := exec.Command("wimlib-imagex", "capture", tree, target, "--boot").CombinedOutput(); err != nil {
		t.Fatalf("capture WIM: %v: %s", err, output)
	}
	return target
}

func TestCarriesBootFiles(t *testing.T) {
	manager, err := NewManager()
	if err != nil {
		t.Skip("wimlib-imagex required to inspect real WIM files")
	}
	for _, scenario := range []struct {
		name  string
		files []string
		want  bool
	}{
		{name: "installer layout", files: []string{"Windows/Boot/DVD/PCAT/BCD", "Windows/Boot/DVD/PCAT/boot.sdi"}, want: true},
		{name: "lower-case paths", files: []string{"windows/boot/dvd/pcat/bcd", "windows/boot/dvd/pcat/boot.sdi"}, want: true},
		{name: "configuration manager sdi", files: []string{"Windows/Boot/DVD/PCAT/BCD", "sms/boot/boot.sdi"}, want: true},
		{name: "deployment kit layout", files: []string{"Windows/Boot/PXE/bootmgr.exe"}, want: false},
		{name: "BCD without sdi", files: []string{"Windows/Boot/DVD/PCAT/BCD"}, want: false},
		{name: "sdi without BCD", files: []string{"Windows/Boot/DVD/PCAT/boot.sdi"}, want: false},
	} {
		wimPath := captureWIM(t, scenario.files...)
		for _, index := range []int{0, 1} {
			got, err := manager.CarriesBootFiles(wimPath, index)
			if err != nil {
				t.Errorf("%s (index %d): %v", scenario.name, index, err)
				continue
			}
			if got != scenario.want {
				t.Errorf("%s (index %d): got %v, want %v", scenario.name, index, got, scenario.want)
			}
		}
	}
}

func TestCarriesBootFilesReportsUnreadableWIM(t *testing.T) {
	manager, err := NewManager()
	if err != nil {
		t.Skip("wimlib-imagex required to inspect real WIM files")
	}
	missing := filepath.Join(t.TempDir(), "missing.wim")
	if _, err := manager.CarriesBootFiles(missing, 0); err == nil {
		t.Error("expected an error for a missing WIM with the boot index")
	}
	if _, err := manager.CarriesBootFiles(missing, 1); err == nil {
		t.Error("expected an error for a missing WIM with an explicit index")
	}
	if _, err := manager.CarriesBootFiles(captureWIM(t), 7); err == nil {
		t.Error("expected an error for an image index the WIM does not have")
	}
}
