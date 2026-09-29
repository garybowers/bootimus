package proxydhcp

import (
	"net"
	"testing"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
)

func TestBootfileForPassesClientHardwareAddress(t *testing.T) {
	mac, err := net.ParseMAC("02:00:00:00:00:01")
	if err != nil {
		t.Fatalf("ParseMAC: %v", err)
	}
	request, err := dhcpv4.New(
		dhcpv4.WithHwAddr(mac),
		dhcpv4.WithOption(dhcpv4.OptClientArch(iana.EFI_X86_64)),
	)
	if err != nil {
		t.Fatalf("New DHCP request: %v", err)
	}

	server := &Server{cfg: Config{
		BootfileBIOS:  DefaultBootfileBIOS,
		BootfileUEFI:  DefaultBootfileUEFI,
		BootfileARM64: DefaultBootfileARM64,
		Bootfiles: func(clientHWAddr net.HardwareAddr) (string, string, string) {
			if clientHWAddr.String() != mac.String() {
				t.Fatalf("callback MAC = %q, want %q", clientHWAddr, mac)
			}
			return "custom/legacy.kpxe", "custom/client.efi", "custom/client-arm64.efi"
		},
	}}

	if got := server.bootfileFor(request); got != "custom/client.efi" {
		t.Fatalf("bootfileFor = %q, want custom/client.efi", got)
	}
}
