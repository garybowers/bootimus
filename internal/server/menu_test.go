package server

import (
	"strings"
	"testing"

	"bootimus/internal/models"
)

func testMenuBuilder(types map[uint]string) *MenuBuilder {
	return &MenuBuilder{
		macAddress:       "aa:bb:cc:dd:ee:ff",
		serverAddr:       "10.0.0.1",
		httpPort:         8080,
		autoInstallTypes: types,
	}
}

func TestBuildNextBootBypassesMenuForGroupedImage(t *testing.T) {
	groupID := uint(3)
	mb := testMenuBuilder(map[uint]string{7: "kickstart"})
	mb.nextBootImageID = 7
	mb.images = []models.Image{
		{
			ID:         7,
			Name:       "AlmaLinux",
			Filename:   "almalinux.iso",
			Enabled:    true,
			BootMethod: "kernel",
			Distro:     "alma",
			GroupID:    &groupID,
		},
	}

	out := mb.Build()
	if !strings.HasPrefix(out, "#!ipxe\n\ngoto iso7\n\n") {
		t.Fatalf("expected next boot to jump directly to the image, got:\n%s", out)
	}
	if !strings.Contains(out, ":iso7\n") {
		t.Fatalf("expected next boot image section to be emitted, got:\n%s", out)
	}
	if strings.Contains(out, ":start\nmenu ") {
		t.Fatalf("expected next boot to bypass the interactive menu, got:\n%s", out)
	}
}

func TestBuildMissingNextBootFallsBackToMenu(t *testing.T) {
	mb := testMenuBuilder(nil)
	mb.nextBootImageID = 99

	out := mb.Build()
	if !strings.Contains(out, ":start\nmenu ") {
		t.Fatalf("expected an unavailable next boot image to fall back to the menu, got:\n%s", out)
	}
}

func TestBuildKernelBootSectionAutoInstallParams(t *testing.T) {
	img := &models.Image{
		ID:         7,
		Name:       "Test Distro",
		Filename:   "test.iso",
		Enabled:    true,
		BootMethod: "kernel",
		Distro:     "ubuntu",
		BootParams: "ip=dhcp",
	}

	cases := []struct {
		scriptType string
		want       string
	}{
		{"kickstart", "inst.ks=http://10.0.0.1:8080/autoinstall/test.iso?mac=aa:bb:cc:dd:ee:ff"},
		{"preseed", "auto=true priority=critical url=http://10.0.0.1:8080/autoinstall/test.iso?mac=aa:bb:cc:dd:ee:ff"},
		{"autoinstall", "autoinstall ds=nocloud-net;s=http://10.0.0.1:8080/autoinstall/test.iso/mac/aa:bb:cc:dd:ee:ff/"},
		{"generic", "autoinstall=http://10.0.0.1:8080/autoinstall/test.iso?mac=aa:bb:cc:dd:ee:ff"},
	}

	for _, c := range cases {
		mb := testMenuBuilder(map[uint]string{7: c.scriptType})
		out := mb.buildKernelBootSection(img, "test.iso", "test")
		if !strings.Contains(out, c.want) {
			t.Errorf("type %s: expected kernel line to contain %q, got:\n%s", c.scriptType, c.want, out)
		}
	}
}

func TestBuildKernelBootSectionNoAutoInstall(t *testing.T) {
	img := &models.Image{ID: 7, Filename: "test.iso", Enabled: true, BootMethod: "kernel", BootParams: "ip=dhcp"}

	for _, types := range []map[uint]string{nil, {7: "autounattend"}} {
		mb := testMenuBuilder(types)
		out := mb.buildKernelBootSection(img, "test.iso", "test")
		if strings.Contains(out, "autoinstall") || strings.Contains(out, "inst.ks") {
			t.Errorf("expected no auto-install params for types=%v, got:\n%s", types, out)
		}
	}
}

func TestBuildKernelBootSectionStripsBareNocloudParam(t *testing.T) {
	img := &models.Image{
		ID:         7,
		Filename:   "ubuntu.iso",
		Enabled:    true,
		BootMethod: "kernel",
		Distro:     "ubuntu",
		BootParams: "boot=casper initrd=initrd ds=nocloud ip=dhcp",
	}
	mb := testMenuBuilder(map[uint]string{7: "autoinstall"})

	out := mb.buildKernelBootSection(img, "ubuntu.iso", "ubuntu")
	if !strings.Contains(out, "ds=nocloud-net;s=") {
		t.Fatalf("expected nocloud-net seed param:\n%s", out)
	}
	if strings.Contains(out, " ds=nocloud ") || strings.HasSuffix(strings.TrimSpace(out), "ds=nocloud") {
		t.Errorf("expected bare ds=nocloud to be stripped from boot params:\n%s", out)
	}
	if !strings.Contains(out, "boot=casper") || !strings.Contains(out, "ip=dhcp") {
		t.Errorf("expected remaining boot params to survive:\n%s", out)
	}
}

func TestResolveBootParamsPlaceholders(t *testing.T) {
	img := &models.Image{
		BootParams: "url={{BASE_URL}} host={{SERVER_ADDR}} file={{IMAGE_FILENAME}} legacy={{FILENAME}} cache={{CACHE_DIR}} mac={{MAC}}",
	}
	mb := testMenuBuilder(nil)

	got := mb.resolveBootParams(img, "http://10.0.0.1:8080", "test.iso", "test")
	want := "url=http://10.0.0.1:8080 host=10.0.0.1 file=test.iso legacy=test.iso cache=test mac=aa:bb:cc:dd:ee:ff"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestBuildWindowsBootSectionDropsPathTokens(t *testing.T) {
	mb := testMenuBuilder(nil)
	img := &models.Image{
		ID:         9,
		Filename:   "Win11_25H2.iso",
		Enabled:    true,
		BootMethod: "kernel",
		Distro:     "windows",
		BootParams: "rawbcd /opt/bootimus/data/isos/Win11_25H2/iso/sources/boot.wim quiet",
	}

	out := mb.buildKernelBootSection(img, "Win11_25H2.iso", "Win11_25H2")
	if !strings.Contains(out, "kernel http://10.0.0.1:8080/wimboot rawbcd quiet\n") {
		t.Errorf("expected path tokens stripped from the wimboot line, got:\n%s", out)
	}
	if strings.Contains(out, "/opt/bootimus") {
		t.Errorf("server filesystem path leaked into the menu:\n%s", out)
	}
	if !strings.Contains(out, "initrd http://10.0.0.1:8080/boot/Win11_25H2/iso/sources/boot.wim boot.wim") {
		t.Errorf("expected boot.wim initrd line, got:\n%s", out)
	}
}

func TestBuildWindowsBootSectionBareWimboot(t *testing.T) {
	mb := testMenuBuilder(nil)
	img := &models.Image{
		ID:         9,
		Filename:   "Win11_25H2.iso",
		Enabled:    true,
		BootMethod: "kernel",
		Distro:     "windows",
	}

	out := mb.buildKernelBootSection(img, "Win11_25H2.iso", "Win11_25H2")
	if !strings.Contains(out, "kernel http://10.0.0.1:8080/wimboot\n") {
		t.Errorf("expected a bare wimboot kernel line, got:\n%s", out)
	}
}
