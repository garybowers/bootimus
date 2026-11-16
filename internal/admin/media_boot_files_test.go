package admin

import (
	"os"
	"path/filepath"
	"testing"

	"bootimus/internal/models"
)

func TestMediaBootFiles(t *testing.T) {
	for _, scenario := range []struct {
		name             string
		onDisk           []string
		bcd, sdi         string
		wantBCD, wantSDI string
	}{
		{name: "paths recorded in the case on disk", onDisk: []string{"boot/bcd", "boot/boot.sdi"}, bcd: "boot/bcd", sdi: "boot/boot.sdi", wantBCD: "boot/bcd", wantSDI: "boot/boot.sdi"},
		{name: "deployment kit media extracted in mixed case", onDisk: []string{"Boot/BCD", "Boot/boot.sdi"}, bcd: "boot/bcd", sdi: "boot/boot.sdi", wantBCD: "Boot/BCD", wantSDI: "Boot/boot.sdi"},
		{name: "upper-case media", onDisk: []string{"BOOT/BCD", "BOOT/BOOT.SDI"}, bcd: "boot/bcd", sdi: "boot/boot.sdi", wantBCD: "BOOT/BCD", wantSDI: "BOOT/BOOT.SDI"},
		{name: "firmware BCD", onDisk: []string{"EFI/Microsoft/Boot/BCD", "boot/boot.sdi"}, bcd: "efi/microsoft/boot/bcd", sdi: "boot/boot.sdi", wantBCD: "EFI/Microsoft/Boot/BCD", wantSDI: "boot/boot.sdi"},
		{name: "loader recorded in place of a BCD", onDisk: []string{"efi/boot/bootx64.efi", "boot/boot.sdi"}, bcd: "efi/boot/bootx64.efi", sdi: "boot/boot.sdi"},
		{name: "boot.sdi missing from the media", onDisk: []string{"boot/bcd"}, bcd: "boot/bcd", sdi: "boot/boot.sdi"},
		{name: "path outside the extracted media", onDisk: []string{"boot/boot.sdi"}, bcd: "../../other/bcd", sdi: "boot/boot.sdi"},
	} {
		extracted := filepath.Join(t.TempDir(), "winpe", "iso")
		for _, name := range scenario.onDisk {
			target := filepath.Join(extracted, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte(name), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		bcd, sdi := mediaBootFiles(extracted, filepath.Join(extracted, filepath.FromSlash(scenario.bcd)), filepath.Join(extracted, filepath.FromSlash(scenario.sdi)))
		if bcd != scenario.wantBCD || sdi != scenario.wantSDI {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", scenario.name, bcd, sdi, scenario.wantBCD, scenario.wantSDI)
		}
	}
	if bcd, sdi := mediaBootFiles(t.TempDir(), "", ""); bcd != "" || sdi != "" {
		t.Errorf("nothing recorded: got (%q, %q)", bcd, sdi)
	}
}

func TestRefreshMediaBootFilesClearsStaleValues(t *testing.T) {
	handler := &Handler{isoDir: t.TempDir()}
	for _, image := range []*models.Image{
		{Filename: "debian.iso", Distro: "debian", BCDPath: "Boot/BCD", BootSDIPath: "Boot/boot.sdi"},
		{Filename: "winpe.iso", Distro: "winpe", BCDPath: "Boot/BCD", BootSDIPath: "Boot/boot.sdi"},
	} {
		handler.refreshMediaBootFiles(image)
		if image.BCDPath != "" || image.BootSDIPath != "" {
			t.Errorf("%s kept stale media boot files: %q, %q", image.Filename, image.BCDPath, image.BootSDIPath)
		}
	}
}
