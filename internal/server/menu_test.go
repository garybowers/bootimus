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

func TestGroupedNextBootStartsInContainingMenuAndSelectsImage(t *testing.T) {
	rootID := uint(10)
	childID := uint(20)
	root := &models.ImageGroup{ID: rootID, Name: "Linux", Enabled: true}
	child := &models.ImageGroup{ID: childID, Name: "Alma", ParentID: &rootID, Parent: root, Enabled: true}
	mb := testMenuBuilder(nil)
	mb.groups = []*models.ImageGroup{root, child}
	mb.images = []models.Image{{
		ID:       42,
		Name:     "AlmaLinux",
		Filename: "linux/alma/AlmaLinux.iso",
		Enabled:  true,
		GroupID:  &childID,
	}}
	mb.nextBootImageID = 42

	out := mb.Build()
	if !strings.HasPrefix(out, "#!ipxe\n\ngoto group20\n\n:start\n") {
		t.Fatalf("expected grouped next boot to enter its containing menu first:\n%s", out)
	}
	if !strings.Contains(out, ":group20\nmenu Bootimus - Boot Menu - Alma") {
		t.Fatalf("expected target group menu to be present:\n%s", out)
	}
	if !strings.Contains(out, "choose --default iso42 --timeout 30000 selected || goto group20") {
		t.Fatalf("expected grouped image to be the timed default in its menu:\n%s", out)
	}
}

func TestGroupedNextBootUsesTimeoutWhenMenusNormallyWaitForever(t *testing.T) {
	groupID := uint(10)
	mb := testMenuBuilder(nil)
	mb.theme = &models.MenuTheme{MenuTimeout: 0}
	mb.groups = []*models.ImageGroup{{ID: groupID, Name: "Alma", Enabled: true}}
	mb.images = []models.Image{{ID: 42, Name: "AlmaLinux", Filename: "alma/AlmaLinux.iso", Enabled: true, GroupID: &groupID}}
	mb.nextBootImageID = 42

	out := mb.Build()
	if !strings.Contains(out, "choose --default iso42 --timeout 10000 selected || goto group10") {
		t.Fatalf("expected one-shot grouped image to receive the 10-second timeout override:\n%s", out)
	}
}

func TestUngroupedNextBootRemainsSelectedOnRootMenu(t *testing.T) {
	mb := testMenuBuilder(nil)
	mb.images = []models.Image{{ID: 42, Name: "AlmaLinux", Filename: "AlmaLinux.iso", Enabled: true}}
	mb.nextBootImageID = 42

	out := mb.Build()
	if strings.HasPrefix(out, "#!ipxe\n\ngoto group") {
		t.Fatalf("did not expect an ungrouped image to enter a group menu:\n%s", out)
	}
	if !strings.Contains(out, "choose --default iso42 --timeout 30000 selected || goto start") {
		t.Fatalf("expected ungrouped image to remain the root-menu default:\n%s", out)
	}
}
