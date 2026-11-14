package extractor

import (
	"path/filepath"
	"strings"
	"testing"
)

func trueNASFiles() map[string]string {
	return map[string]string{
		"/vmlinuz":                  "truenas-kernel",
		"/initrd.img":               "truenas-initrd",
		"/live/filesystem.squashfs": "truenas-live",
		"/TrueNAS-SCALE.update":     "truenas-installation-payload",
	}
}

func TestExtractTrueNASScale(t *testing.T) {
	for _, name := range []string{"TrueNAS-SCALE-25.10.2.1.iso", "storage-installer.iso", "debian-renamed.iso"} {
		t.Run(name, func(t *testing.T) {
			files, cache := extractTestISO(t, name, trueNASFiles())
			if files.Distro != "truenas-scale" || files.NetbootRequired {
				t.Fatalf("wrong profile or netboot requirement: %+v", files)
			}
			if got := readFileString(t, files.Kernel); got != "truenas-kernel" {
				t.Errorf("kernel = %q", got)
			}
			if got := readFileString(t, files.Initrd); got != "truenas-initrd" {
				t.Errorf("initrd = %q", got)
			}
			base := strings.TrimSuffix(name, filepath.Ext(name))
			if got := readFileString(t, filepath.Join(cache, base, files.SquashfsPath)); got != "truenas-live" {
				t.Errorf("live filesystem = %q", got)
			}
		})
	}
}

func TestTrueNASCoreIsNotDebian(t *testing.T) {
	files, _ := extractTestISO(t, "TrueNAS-13.0-U6.2.iso", map[string]string{
		"/boot/kernel/kernel": "freebsd-kernel", "/boot/mfsroot.gz": "freebsd-root",
	})
	if files.Distro != "freebsd" {
		t.Fatalf("CORE distro = %q, want freebsd", files.Distro)
	}
}

func TestDebianRootLayoutIsNotTrueNAS(t *testing.T) {
	files := trueNASFiles()
	delete(files, "/TrueNAS-SCALE.update")
	got, _ := extractTestISO(t, "debian-live.iso", files)
	if got.Distro != "debian" {
		t.Fatalf("Debian distro = %q", got.Distro)
	}
}

func TestTrueNASBsdtarFallback(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "renamed.iso")
	buildJolietOnlyISO(t, name, trueNASFiles())
	ext, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ext.extractViaBsdtar(name)
	if err != nil {
		t.Fatal(err)
	}
	if got.Distro != "truenas-scale" || !strings.EqualFold(got.SquashfsPath, "iso/live/filesystem.squashfs") {
		t.Fatalf("incorrect fallback detection: %+v", got)
	}
}
