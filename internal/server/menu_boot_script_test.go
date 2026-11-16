package server

import (
	"strings"
	"testing"

	"bootimus/internal/models"
)

const customWinPEScript = `kernel {{BASE_URL}}/wimboot
initrd {{BASE_URL}}/boot/{{CACHE_DIR}}/iso/Boot/BCD bcd
initrd {{BASE_URL}}/boot/{{CACHE_DIR}}/iso/Boot/boot.sdi boot.sdi
initrd {{BASE_URL}}/boot/{{CACHE_DIR}}/iso/sources/boot.wim boot.wim
boot || goto failed`

func TestBuildCustomBootScriptReplacesGeneratedCommands(t *testing.T) {
	mb := testMenuBuilder(nil)
	mb.images = []models.Image{{
		ID:         17,
		Name:       "Windows PE",
		Filename:   "windows pe.iso",
		Enabled:    true,
		BootMethod: "kernel",
		Distro:     "winpe",
		BootParams: "index=2",
		WIMPath:    "sources/boot.wim",
		BootScript: customWinPEScript,
	}}

	out := mb.buildImageBootSections()
	want := `:iso17
echo Booting Windows PE...
chain --autofree http://10.0.0.1:8080/boot-event?mac=aa:bb:cc:dd:ee:ff&image=17 ||
kernel http://10.0.0.1:8080/wimboot
initrd http://10.0.0.1:8080/boot/windows%20pe/iso/Boot/BCD bcd
initrd http://10.0.0.1:8080/boot/windows%20pe/iso/Boot/boot.sdi boot.sdi
initrd http://10.0.0.1:8080/boot/windows%20pe/iso/sources/boot.wim boot.wim
boot || goto failed
goto start
`
	if out != want {
		t.Fatalf("custom boot script section mismatch\ngot:\n%s\nwant:\n%s", out, want)
	}
}

func TestBuildCustomBootScriptExpandsClientPlaceholders(t *testing.T) {
	mb := testMenuBuilder(nil)
	mb.images = []models.Image{{
		ID:         4,
		Name:       "Custom",
		Filename:   "custom.iso",
		Enabled:    true,
		BootMethod: "sanboot",
		BootScript: "kernel {{BASE_URL}}/isos/{{FILENAME}} server={{SERVER_ADDR}} mac={{MAC}} answers={{AUTOINSTALL_URL}}\nboot || goto failed",
	}}

	out := mb.buildImageBootSections()
	want := "kernel http://10.0.0.1:8080/isos/custom.iso server=10.0.0.1 mac=aa:bb:cc:dd:ee:ff answers=http://10.0.0.1:8080/autoinstall/custom.iso?mac=aa:bb:cc:dd:ee:ff\nboot || goto failed\n"
	if !strings.Contains(out, want) {
		t.Fatalf("expected placeholders to be expanded, got:\n%s", out)
	}
	if strings.Contains(out, "sanboot") {
		t.Fatalf("custom boot script must replace the boot method's commands, got:\n%s", out)
	}
}

func TestBuildCustomBootScriptReturnsToGroupAndUnlockedCaller(t *testing.T) {
	groupID := uint(3)
	image := models.Image{ID: 5, Name: "Grouped", Filename: "grouped.iso", Enabled: true, GroupID: &groupID, BootScript: "chain http://example.test/boot.ipxe || goto failed"}

	mb := testMenuBuilder(nil)
	mb.images = []models.Image{image}
	if out := mb.buildImageBootSections(); !strings.HasSuffix(out, "chain http://example.test/boot.ipxe || goto failed\ngoto group3\n") {
		t.Fatalf("expected a grouped image to return to its group, got:\n%s", out)
	}

	mb = testMenuBuilder(nil)
	mb.images = []models.Image{image}
	mb.bootAccess = bootMenuAccess{imageID: image.ID}
	if out := mb.buildImageBootSections(); !strings.HasSuffix(out, "chain http://example.test/boot.ipxe || goto failed\nexit\n") {
		t.Fatalf("expected an unlocked single-image section to exit, got:\n%s", out)
	}
}

func TestBuildCustomBootScriptStaysBehindImagePassword(t *testing.T) {
	mb := testMenuBuilder(nil)
	mb.bootAuthURL = "https://10.0.0.1:8443/boot-auth"
	mb.images = []models.Image{{
		ID:                   6,
		Name:                 "Protected",
		Filename:             "protected.iso",
		Enabled:              true,
		BootPasswordRequired: true,
		BootPasswordHash:     "verifier",
		BootScript:           "chain http://example.test/secret.ipxe || goto failed",
	}}

	out := mb.buildImageBootSections()
	if strings.Contains(out, "secret.ipxe") {
		t.Fatalf("custom boot script leaked past the image password gate:\n%s", out)
	}
}

func TestBuildWindowsBootSectionSendsMediaBootFiles(t *testing.T) {
	mb := testMenuBuilder(nil)
	image := &models.Image{
		ID:          18,
		Filename:    "windows-pe-amd64.iso",
		Enabled:     true,
		BootMethod:  "kernel",
		Distro:      "winpe",
		WIMPath:     "sources/boot.wim",
		BCDPath:     "Boot/BCD",
		BootSDIPath: "Boot/boot.sdi",
	}

	out := mb.buildKernelBootSection(image, "windows-pe-amd64.iso", "windows-pe-amd64")
	want := `kernel http://10.0.0.1:8080/wimboot || goto failed
initrd --name BCD http://10.0.0.1:8080/boot/windows-pe-amd64/iso/Boot/BCD BCD || goto failed
initrd --name boot.sdi http://10.0.0.1:8080/boot/windows-pe-amd64/iso/Boot/boot.sdi boot.sdi || goto failed
initrd --name boot.wim http://10.0.0.1:8080/boot/windows-pe-amd64/iso/sources/boot.wim boot.wim
boot || goto failed
`
	if !strings.HasSuffix(out, want) {
		t.Fatalf("expected the media BCD and boot.sdi ahead of the WIM, got:\n%s", out)
	}

	image.BCDPath = ""
	image.BootSDIPath = ""
	out = mb.buildKernelBootSection(image, "windows-pe-amd64.iso", "windows-pe-amd64")
	if strings.Contains(out, " BCD") || strings.Contains(out, "boot.sdi") {
		t.Fatalf("a WIM that carries its own boot files must be sent alone, got:\n%s", out)
	}
}
