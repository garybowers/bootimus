package proxydhcp

import (
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	return &Server{cfg: Config{
		ServerIP:      net.ParseIP("192.168.203.111").To4(),
		BootfileBIOS:  DefaultBootfileBIOS,
		BootfileUEFI:  DefaultBootfileUEFI,
		BootfileARM64: DefaultBootfileARM64,
	}}
}

func discover(t *testing.T, mac string, vci string, opts ...dhcpv4.Modifier) *dhcpv4.DHCPv4 {
	t.Helper()
	hw, err := net.ParseMAC(mac)
	if err != nil {
		t.Fatal(err)
	}
	modifiers := append([]dhcpv4.Modifier{
		dhcpv4.WithMessageType(dhcpv4.MessageTypeDiscover),
		dhcpv4.WithOption(dhcpv4.OptClassIdentifier(vci)),
	}, opts...)
	req, err := dhcpv4.New(modifiers...)
	if err != nil {
		t.Fatal(err)
	}
	req.ClientHWAddr = hw
	return req
}

func TestARM64ClientViaOption93(t *testing.T) {
	s := testServer(t)
	req := discover(t, "dc:a6:32:12:34:56", "PXEClient:Arch:00011:UNDI:003000",
		dhcpv4.WithOption(dhcpv4.OptClientArch(iana.EFI_ARM64)))

	resp, bootfile, ok := s.buildReply(req)
	if !ok {
		t.Fatal("expected a reply")
	}
	if bootfile != DefaultBootfileARM64 {
		t.Errorf("bootfile = %q, want %q", bootfile, DefaultBootfileARM64)
	}
	if resp.BootFileName != DefaultBootfileARM64 {
		t.Errorf("BootFileName = %q, want %q", resp.BootFileName, DefaultBootfileARM64)
	}
}

func TestARM64ClientWithoutOption93FallsBackToVCI(t *testing.T) {
	s := testServer(t)
	req := discover(t, "dc:a6:32:12:34:56", "PXEClient:Arch:00011:UNDI:003000")

	_, bootfile, ok := s.buildReply(req)
	if !ok {
		t.Fatal("expected a reply")
	}
	if bootfile != DefaultBootfileARM64 {
		t.Errorf("bootfile = %q, want %q (arch must come from the VCI when option 93 is absent)", bootfile, DefaultBootfileARM64)
	}
}

func TestX64UEFIClient(t *testing.T) {
	s := testServer(t)
	for _, arch := range []iana.Arch{iana.EFI_X86_64, iana.EFI_BC, iana.EFI_IA32} {
		req := discover(t, "00:11:22:33:44:55", "PXEClient:Arch:00007:UNDI:003016",
			dhcpv4.WithOption(dhcpv4.OptClientArch(arch)))
		_, bootfile, ok := s.buildReply(req)
		if !ok || bootfile != DefaultBootfileUEFI {
			t.Errorf("arch %d: bootfile = %q, want %q", arch, bootfile, DefaultBootfileUEFI)
		}
	}
}

func TestBIOSClientGetsBIOSFileAndPXEOpts(t *testing.T) {
	s := testServer(t)
	req := discover(t, "00:11:22:33:44:55", "PXEClient:Arch:00000:UNDI:002001",
		dhcpv4.WithOption(dhcpv4.OptClientArch(iana.INTEL_X86PC)))

	resp, bootfile, ok := s.buildReply(req)
	if !ok || bootfile != DefaultBootfileBIOS {
		t.Fatalf("bootfile = %q, want %q", bootfile, DefaultBootfileBIOS)
	}
	if string(resp.GetOneOption(dhcpv4.OptionVendorSpecificInformation)) == "Raspberry Pi Boot" {
		t.Error("non-Pi BIOS client must not receive the Raspberry Pi vendor option")
	}
}

func TestPiFirmwareStageGetsPiVendorOption(t *testing.T) {
	s := testServer(t)
	req := discover(t, "dc:a6:32:12:34:56", "PXEClient:Arch:00000:UNDI:002001",
		dhcpv4.WithOption(dhcpv4.OptClientArch(iana.INTEL_X86PC)))

	resp, _, ok := s.buildReply(req)
	if !ok {
		t.Fatal("expected a reply")
	}
	if string(resp.GetOneOption(dhcpv4.OptionVendorSpecificInformation)) != "Raspberry Pi Boot" {
		t.Error("Pi firmware stage must receive the Raspberry Pi Boot vendor option")
	}
}

func TestPiUEFIStageKeepsStandardPXEOpts(t *testing.T) {
	s := testServer(t)
	req := discover(t, "dc:a6:32:12:34:56", "PXEClient:Arch:00011:UNDI:003000",
		dhcpv4.WithOption(dhcpv4.OptClientArch(iana.EFI_ARM64)))

	resp, bootfile, ok := s.buildReply(req)
	if !ok {
		t.Fatal("expected a reply")
	}
	if bootfile != DefaultBootfileARM64 {
		t.Errorf("bootfile = %q, want %q", bootfile, DefaultBootfileARM64)
	}
	if string(resp.GetOneOption(dhcpv4.OptionVendorSpecificInformation)) == "Raspberry Pi Boot" {
		t.Error("Pi UEFI stage must receive standard PXE vendor options, not the firmware-stage string")
	}
}

func TestNonPXEClientIgnored(t *testing.T) {
	s := testServer(t)
	req := discover(t, "00:11:22:33:44:55", "MSFT 5.0")
	if _, _, ok := s.buildReply(req); ok {
		t.Error("expected non-PXE client to be ignored")
	}
}

func TestArchFromVCI(t *testing.T) {
	cases := []struct {
		vci  string
		want iana.Arch
		ok   bool
	}{
		{"PXEClient:Arch:00011:UNDI:003000", iana.EFI_ARM64, true},
		{"PXEClient:Arch:00000:UNDI:002001", iana.INTEL_X86PC, true},
		{"PXEClient:Arch:00007:UNDI:003016", iana.EFI_X86_64, true},
		{"PXEClient", 0, false},
		{"PXEClient:Arch:bogus:UNDI:003000", 0, false},
	}
	for _, c := range cases {
		got, ok := archFromVCI(c.vci)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("archFromVCI(%q) = %d,%v want %d,%v", c.vci, got, ok, c.want, c.ok)
		}
	}
}
