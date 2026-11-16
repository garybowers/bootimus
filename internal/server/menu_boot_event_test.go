package server

import (
	"strings"
	"testing"

	"bootimus/internal/diskimage"
	"bootimus/internal/models"
	"bootimus/internal/tools"
)

func bootEventMenuBuilder(mac string) *MenuBuilder {
	mb := testMenuBuilder(nil)
	mb.macAddress = mac
	mb.images = []models.Image{
		{ID: 7, Name: "Alpine", Filename: "alpine.iso", Enabled: true, BootMethod: "kernel", Distro: "alpine"},
		{ID: 8, Name: "Rescue", Filename: "rescue.iso", Enabled: true, BootMethod: "sanboot"},
	}
	mb.enabledTools = []tools.EnabledTool{{Name: "my tool", DisplayName: "My Tool", KernelURL: "http://10.0.0.1:8080/tools/my/vmlinuz"}}
	mb.sanTarget = &models.SANTarget{ID: 3, Name: "lab", URI: "iscsi:10.0.0.9::::iqn.2026-01.test:lab"}
	mb.diskBoot = &diskimage.Boot{KernelURL: "http://10.0.0.1:8080/tools/clonezilla/vmlinuz", InitrdURL: "http://10.0.0.1:8080/tools/clonezilla/initrd", Label: "Capturing disk"}
	return mb
}

func TestBuildReportsEveryBootToTheServer(t *testing.T) {
	out := bootEventMenuBuilder("aa:bb:cc:dd:ee:ff").Build()
	const event = "chain --autofree http://10.0.0.1:8080/boot-event?mac=aa:bb:cc:dd:ee:ff&"
	for _, scenario := range []struct {
		name, before, after string
	}{
		{"kernel image", "echo Booting Alpine...\n" + event + "image=7 ||\n", "kernel http://10.0.0.1:8080/boot/alpine/vmlinuz"},
		{"sanboot image", "echo Booting Rescue...\n" + event + "image=8 ||\n", "sanboot --no-describe --drive 0x80 http://10.0.0.1:8080/isos/rescue.iso"},
		{"tool", "echo Booting My Tool...\n" + event + "tool=my+tool ||\n", "kernel http://10.0.0.1:8080/tools/my/vmlinuz"},
		{"SAN target", "echo Booting from SAN target lab...\n" + event + "san=3 ||\n", "sanboot --no-describe --drive 0x80 iscsi:"},
		{"disk imaging", "echo Capturing disk...\n" + event + "diskimage=1 ||\n", "kernel http://10.0.0.1:8080/tools/clonezilla/vmlinuz"},
	} {
		reported := strings.Index(out, scenario.before)
		if reported < 0 {
			t.Errorf("%s: boot is not reported straight after its banner:\n%s", scenario.name, out)
			continue
		}
		if booted := strings.Index(out[reported:], scenario.after); booted < 0 {
			t.Errorf("%s: boot commands do not follow the report:\n%s", scenario.name, out[reported:])
		}
	}
	if count := strings.Count(out, "/boot-event?"); count != 5 {
		t.Errorf("expected one report per boot section, found %d:\n%s", count, out)
	}
}

func TestBuildSkipsBootReportsWithoutAClientMAC(t *testing.T) {
	for _, mac := range []string{"", "unknown"} {
		if out := bootEventMenuBuilder(mac).Build(); strings.Contains(out, "boot-event") {
			t.Errorf("mac %q: a client that cannot be identified must not report boots:\n%s", mac, out)
		}
	}
}
