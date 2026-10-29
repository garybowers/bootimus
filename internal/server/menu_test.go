package server

import (
	"strings"
	"testing"

	"bootimus/internal/models"
	"bootimus/internal/tools"
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
	if !strings.Contains(out, ":start\nmenu ") {
		t.Fatalf("expected a return menu after next-boot failure or cancellation, got:\n%s", out)
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
		{"autoyast", "autoyast=http://10.0.0.1:8080/autoinstall/test.iso?mac=aa:bb:cc:dd:ee:ff"},
		{"agama", "inst.auto=http://10.0.0.1:8080/autoinstall/test.iso?mac=aa:bb:cc:dd:ee:ff"},
		{"ignition", "ignition.config.url=http://10.0.0.1:8080/autoinstall/test.iso?mac=aa:bb:cc:dd:ee:ff ignition.firstboot ignition.platform.id=metal"},
	}

	for _, c := range cases {
		mb := testMenuBuilder(map[uint]string{7: c.scriptType})
		out := mb.buildKernelBootSection(img, "test.iso", "test")
		if !strings.Contains(out, c.want) {
			t.Errorf("type %s: expected kernel line to contain %q, got:\n%s", c.scriptType, c.want, out)
		}
	}
}

func TestBuildKernelBootSectionFlatcarIgnitionParams(t *testing.T) {
	img := &models.Image{ID: 7, Filename: "flatcar.iso", Enabled: true, BootMethod: "kernel", Distro: "flatcar", BootParams: "initrd=initrd"}
	mb := testMenuBuilder(map[uint]string{7: "ignition"})

	out := mb.buildKernelBootSection(img, "flatcar.iso", "flatcar")
	if !strings.Contains(out, "flatcar.first_boot=1 ignition.config.url=http://10.0.0.1:8080/autoinstall/flatcar.iso?mac=aa:bb:cc:dd:ee:ff") {
		t.Errorf("expected flatcar ignition params, got:\n%s", out)
	}
	if strings.Contains(out, "ignition.platform.id") {
		t.Errorf("flatcar must not receive the fedora coreos platform id, got:\n%s", out)
	}
}

func TestBuildKernelBootSectionNoAutoInstall(t *testing.T) {
	img := &models.Image{ID: 7, Filename: "test.iso", Enabled: true, BootMethod: "kernel", BootParams: "ip=dhcp"}

	for _, types := range []map[uint]string{nil, {7: "autounattend"}, {7: "generic"}, {7: "proxmox"}} {
		mb := testMenuBuilder(types)
		out := mb.buildKernelBootSection(img, "test.iso", "test")
		if strings.Contains(out, "autoinstall") || strings.Contains(out, "inst.ks") {
			t.Errorf("expected no auto-install params for types=%v, got:\n%s", types, out)
		}
	}
}

func TestBuildKernelBootSectionProxmoxLoadsISOAsSecondInitrd(t *testing.T) {
	img := &models.Image{ID: 7, Filename: "proxmox-ve_9.1-1.iso", Enabled: true, BootMethod: "kernel", Distro: "proxmox", BootParams: "initrd=initrd ramdisk_size=16777216 rw quiet"}
	mb := testMenuBuilder(map[uint]string{7: "proxmox"})

	out := mb.buildKernelBootSection(img, "proxmox-ve_9.1-1.iso", "proxmox-ve_9.1-1")
	if !strings.Contains(out, "initrd http://10.0.0.1:8080/boot/proxmox-ve_9.1-1/initrd\n") {
		t.Errorf("expected the installer initrd line, got:\n%s", out)
	}
	if !strings.Contains(out, "initrd http://10.0.0.1:8080/isos/proxmox-ve_9.1-1.iso?mac=aa:bb:cc:dd:ee:ff proxmox.iso\n") {
		t.Errorf("expected the ISO to be loaded as proxmox.iso, got:\n%s", out)
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

func TestBuildKernelBootSectionShimChain(t *testing.T) {
	img := &models.Image{
		ID:         7,
		Filename:   "fedora.iso",
		Enabled:    true,
		BootMethod: "kernel",
		Distro:     "fedora",
		BootParams: "ip=dhcp",
		ShimPath:   "iso/EFI/BOOT/BOOTX64.EFI",
	}
	mb := testMenuBuilder(nil)

	out := mb.buildKernelBootSection(img, "fedora.iso", "fedora")
	want := "iseq ${platform} efi && shim http://10.0.0.1:8080/boot/fedora/iso/EFI/BOOT/BOOTX64.EFI ||\nboot || goto failed\n"
	if !strings.Contains(out, want) {
		t.Errorf("expected shim chain line before boot, got:\n%s", out)
	}

	img.ShimPath = ""
	out = mb.buildKernelBootSection(img, "fedora.iso", "fedora")
	if strings.Contains(out, "shim ") {
		t.Errorf("expected no shim line without a detected shim, got:\n%s", out)
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

func TestBuildWindowsBootSectionDropsLinuxParams(t *testing.T) {
	mb := testMenuBuilder(nil)
	img := &models.Image{
		ID:         9,
		Filename:   "Win11_25H2.iso",
		Enabled:    true,
		BootMethod: "kernel",
		Distro:     "windows",
		BootParams: "iso-url={{BASE_URL}}/isos/{{FILENAME}} ip=dhcp gui index=2 pause=quiet",
	}

	out := mb.buildKernelBootSection(img, "Win11_25H2.iso", "Win11_25H2")
	if !strings.Contains(out, "kernel http://10.0.0.1:8080/wimboot gui index=2 pause=quiet\n") {
		t.Errorf("expected only wimboot arguments on the kernel line, got:\n%s", out)
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

func TestBuildFooterToolKernelGetsInitrdName(t *testing.T) {
	mb := testMenuBuilder(nil)
	mb.enabledTools = []tools.EnabledTool{
		{
			Name:        "clonezilla",
			DisplayName: "Clonezilla",
			KernelURL:   "http://10.0.0.1:8080/tools/clonezilla/live/vmlinuz",
			InitrdURL:   "http://10.0.0.1:8080/tools/clonezilla/live/initrd.img",
			BootParams:  "boot=live fetch=http://10.0.0.1:8080/tools/clonezilla/live/filesystem.squashfs",
			BootMethod:  "kernel",
		},
		{
			Name:        "memtest86plus",
			DisplayName: "Memtest86+",
			KernelURL:   "http://10.0.0.1:8080/tools/memtest86plus/memtest64.bin",
			BootMethod:  "kernel",
		},
	}

	out := mb.buildFooter()
	if !strings.Contains(out, "kernel http://10.0.0.1:8080/tools/clonezilla/live/vmlinuz initrd=initrd boot=live ") {
		t.Errorf("expected initrd=initrd on the tool kernel line, got:\n%s", out)
	}
	if !strings.Contains(out, "initrd --name initrd http://10.0.0.1:8080/tools/clonezilla/live/initrd.img\n") {
		t.Errorf("expected the tool initrd to be named initrd, got:\n%s", out)
	}
	if strings.Contains(out, "memtest64.bin initrd=") {
		t.Errorf("tools without an initrd must not get initrd=, got:\n%s", out)
	}
}

func TestBuildImageBootSectionsUKI(t *testing.T) {
	mb := testMenuBuilder(nil)
	mb.images = []models.Image{
		{ID: 21, Name: "Hello World", Filename: "hello-world.efi", Enabled: true, BootMethod: "uki", BootParams: "audit=0"},
		{ID: 22, Name: "Bare", Filename: "lab/bare.efi", Enabled: true, BootMethod: "uki"},
	}

	out := mb.buildImageBootSections()
	for _, want := range []string{
		":iso21\n",
		"iseq ${platform} efi || goto uki_needs_uefi\n",
		"kernel http://10.0.0.1:8080/isos/hello-world.efi?mac=aa:bb:cc:dd:ee:ff audit=0\n",
		"kernel http://10.0.0.1:8080/isos/lab/bare.efi?mac=aa:bb:cc:dd:ee:ff\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "iso-url=") || strings.Contains(out, "sanboot") {
		t.Errorf("UKI sections must not fall back to ISO parameters or sanboot:\n%s", out)
	}
	if !strings.Contains(mb.buildFooter(), ":uki_needs_uefi\n") {
		t.Error("footer must define the uki_needs_uefi label")
	}
}

func TestBuildKernelBootSectionBSDChainsTFTPHook(t *testing.T) {
	mb := testMenuBuilder(nil)
	for _, distro := range []string{"openbsd", "netbsd"} {
		img := &models.Image{ID: 31, Name: "BSD", Filename: "lab/install77.iso", Enabled: true, BootMethod: "kernel", Distro: distro, BootParams: "stty com0 115200"}
		out := mb.buildKernelBootSection(img, "lab/install77.iso", "lab/install77")
		want := "chain http://10.0.0.1:8080/bsd/lab/install77/boot.ipxe?mac=aa:bb:cc:dd:ee:ff&platform=${platform}&buildarch=${buildarch} || goto failed\n"
		if !strings.Contains(out, want) {
			t.Errorf("%s: expected %q in:\n%s", distro, want, out)
		}
		if strings.Contains(out, "vmlinuz") || strings.Contains(out, "initrd") || strings.Contains(out, "stty") {
			t.Errorf("%s: BSD entries must not emit Linux kernel lines or raw boot params:\n%s", distro, out)
		}
	}
}

func TestBuildWinPEBootSectionUsesSelectedWIM(t *testing.T) {
	mb := testMenuBuilder(nil)
	img := &models.Image{
		ID:         12,
		Filename:   "WinPE_Strelec.iso",
		Enabled:    true,
		BootMethod: "kernel",
		Distro:     "winpe",
		WIMPath:    "SSTR/strelec10x64 Eng.wim",
		WIMIndex:   2,
	}

	out := mb.buildKernelBootSection(img, "WinPE_Strelec.iso", "WinPE_Strelec")
	if !strings.Contains(out, "kernel http://10.0.0.1:8080/wimboot index=2\n") {
		t.Errorf("expected the WIM index on the wimboot line, got:\n%s", out)
	}
	if !strings.Contains(out, "initrd http://10.0.0.1:8080/boot/WinPE_Strelec/iso/SSTR/strelec10x64%20Eng.wim boot.wim\n") {
		t.Errorf("expected the selected WIM as the initrd, got:\n%s", out)
	}
	if strings.Contains(out, "iso-url=") || strings.Contains(out, "sources/boot.wim") {
		t.Errorf("WinPE section must not fall back to Linux or installer defaults:\n%s", out)
	}
}

func TestBuildWinPEBootSectionKeepsExplicitIndex(t *testing.T) {
	mb := testMenuBuilder(nil)
	img := &models.Image{
		ID:         13,
		Filename:   "WinPE_Strelec.iso",
		Enabled:    true,
		BootMethod: "kernel",
		Distro:     "winpe",
		BootParams: "index=3 gui",
		WIMPath:    "boot/rescue.wim",
		WIMIndex:   2,
	}

	out := mb.buildKernelBootSection(img, "WinPE_Strelec.iso", "WinPE_Strelec")
	if !strings.Contains(out, "kernel http://10.0.0.1:8080/wimboot index=3 gui\n") {
		t.Errorf("explicit index parameter should win, got:\n%s", out)
	}
}

func TestBuildWindowsBootSectionAutoUnattendInitrd(t *testing.T) {
	img := &models.Image{
		ID:         9,
		Filename:   "Win11_25H2.iso",
		Enabled:    true,
		BootMethod: "kernel",
		Distro:     "windows",
	}
	unattendLine := "initrd http://10.0.0.1:8080/autoinstall/Win11_25H2.iso?mac=aa:bb:cc:dd:ee:ff AutoUnattend.xml\n"

	cases := []struct {
		name  string
		types map[uint]string
		want  bool
	}{
		{"no auto-install", nil, false},
		{"autounattend", map[uint]string{9: "autounattend"}, true},
		{"other image autounattend", map[uint]string{3: "autounattend"}, false},
		{"non-windows type", map[uint]string{9: "kickstart"}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mb := testMenuBuilder(c.types)
			out := mb.buildKernelBootSection(img, "Win11_25H2.iso", "Win11_25H2")
			if got := strings.Contains(out, unattendLine); got != c.want {
				t.Errorf("AutoUnattend initrd present=%v, want %v:\n%s", got, c.want, out)
			}
			if !c.want {
				return
			}
			kernelAt := strings.Index(out, "kernel http://10.0.0.1:8080/wimboot\n")
			unattendAt := strings.Index(out, unattendLine)
			wimAt := strings.Index(out, "initrd http://10.0.0.1:8080/boot/Win11_25H2/iso/sources/boot.wim boot.wim")
			if !(kernelAt >= 0 && kernelAt < unattendAt && unattendAt < wimAt) {
				t.Errorf("expected wimboot, then AutoUnattend.xml, then boot.wim:\n%s", out)
			}
		})
	}
}
