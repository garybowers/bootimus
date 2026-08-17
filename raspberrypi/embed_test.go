package raspberrypi

import (
	"net"
	"testing"
)

func TestResolveStripsSerialPrefix(t *testing.T) {
	direct, ok := Resolve("start4.elf")
	if !ok || len(direct) == 0 {
		t.Fatal("expected start4.elf to resolve from embedded assets")
	}
	prefixed, ok := Resolve("7efcf632/start4.elf")
	if !ok || len(prefixed) != len(direct) {
		t.Fatal("expected serial-prefixed path to resolve to the same file")
	}
	if _, ok := Resolve("7efcf632/config.txt"); !ok {
		t.Fatal("expected serial-prefixed config.txt to resolve")
	}
	if _, ok := Resolve("overlays/miniuart-bt.dtbo"); !ok {
		t.Fatal("expected overlay subpath to resolve")
	}
}

func TestResolveRejectsUnknownAndTraversal(t *testing.T) {
	if _, ok := Resolve("no-such-file.bin"); ok {
		t.Fatal("expected unknown file to fail")
	}
	if _, ok := Resolve("../embed.go"); ok {
		t.Fatal("expected traversal to fail")
	}
	if _, ok := Resolve("7efcf632/../embed.go"); ok {
		t.Fatal("expected prefixed traversal to fail")
	}
	if _, ok := Resolve("deadbeef11/start4.elf"); ok {
		t.Fatal("expected a ten-character prefix to not be treated as a serial")
	}
}

func TestBothFamiliesPresent(t *testing.T) {
	for _, f := range []string{"bootcode.bin", "start.elf", "RPI_EFI_3.fd", "start4.elf", "RPI_EFI_4.fd", "config.txt", "LICENCE.broadcom.txt"} {
		if _, ok := Resolve(f); !ok {
			t.Errorf("expected %s in embedded assets", f)
		}
	}
}

func TestIsRaspberryPiMAC(t *testing.T) {
	pi, _ := net.ParseMAC("dc:a6:32:12:34:56")
	if !IsRaspberryPiMAC(pi) {
		t.Error("expected dc:a6:32 OUI to be recognised as a Pi")
	}
	other, _ := net.ParseMAC("00:11:22:33:44:55")
	if IsRaspberryPiMAC(other) {
		t.Error("expected non-Pi OUI to be rejected")
	}
	if IsRaspberryPiMAC(nil) {
		t.Error("expected nil MAC to be rejected")
	}
}
