package proxydhcp

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"

	"bootimus/internal/metrics"
	"bootimus/internal/netbind"
	"bootimus/raspberrypi"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
)

const (
	DefaultBootfileBIOS  = "undionly.kpxe"
	DefaultBootfileUEFI  = "bootimus.efi"
	DefaultBootfileARM64 = "bootimus-arm64.efi"
)

type Config struct {
	ServerIP        net.IP
	Interface       string
	BootfileBIOS    string
	BootfileUEFI    string
	BootfileARM64   string
	NoBootfileption bool
	// Bootfiles, when set, is consulted on every request; any non-empty value
	// it returns overrides the static Bootfile* fields. This lets the server
	// switch bootloader sets at runtime without restarting proxyDHCP.
	Bootfiles func() (bios, uefi, arm64 string)
}

type Server struct {
	cfg      Config
	conn     *net.UDPConn
	conn4011 *net.UDPConn
	wg       sync.WaitGroup
	done     chan struct{}
}

func NewServer(cfg Config) (*Server, error) {
	if cfg.ServerIP == nil {
		if cfg.Interface != "" {
			ip, err := netbind.InterfaceIPv4(cfg.Interface)
			if err != nil {
				return nil, fmt.Errorf("determine server IP: %w", err)
			}
			cfg.ServerIP = ip
		} else {
			ip, err := defaultServerIP()
			if err != nil {
				return nil, fmt.Errorf("determine server IP: %w", err)
			}
			cfg.ServerIP = ip
		}
	}
	if cfg.BootfileBIOS == "" {
		cfg.BootfileBIOS = DefaultBootfileBIOS
	}
	if cfg.BootfileUEFI == "" {
		cfg.BootfileUEFI = DefaultBootfileUEFI
	}
	if cfg.BootfileARM64 == "" {
		cfg.BootfileARM64 = DefaultBootfileARM64
	}
	return &Server{cfg: cfg, done: make(chan struct{})}, nil
}

func (s *Server) Start() error {
	conn, err := netbind.ListenUDP(s.cfg.Interface, "udp4", "0.0.0.0:67")
	if err != nil {
		return fmt.Errorf("listen UDP/67: %w (needs root or CAP_NET_BIND_SERVICE)", err)
	}
	if err := enableBroadcast(conn); err != nil {
		conn.Close()
		return fmt.Errorf("enable broadcast on UDP/67: %w", err)
	}
	s.conn = conn

	conn4011, err := netbind.ListenUDP(s.cfg.Interface, "udp4", "0.0.0.0:4011")
	if err != nil {
		conn.Close()
		return fmt.Errorf("listen UDP/4011: %w", err)
	}
	s.conn4011 = conn4011

	bios, uefi, arm64 := s.effectiveBootfiles()
	log.Printf("proxyDHCP: listening on UDP/67 + UDP/4011, advertising next-server=%s (BIOS=%s, UEFI=%s, ARM64=%s)",
		s.cfg.ServerIP, bios, uefi, arm64)

	s.wg.Add(2)
	go s.loop(conn, true)
	go s.loop(conn4011, false)
	return nil
}

func (s *Server) Shutdown() error {
	close(s.done)
	if s.conn != nil {
		s.conn.Close()
	}
	if s.conn4011 != nil {
		s.conn4011.Close()
	}
	s.wg.Wait()
	return nil
}

func (s *Server) loop(conn *net.UDPConn, bootp bool) {
	defer s.wg.Done()
	buf := make([]byte, 1500)
	for {
		select {
		case <-s.done:
			return
		default:
		}
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-s.done:
				return
			default:
			}
			log.Printf("proxyDHCP: read error: %v", err)
			continue
		}
		req, err := dhcpv4.FromBytes(buf[:n])
		if err != nil {
			log.Printf("proxyDHCP: parse error: %v", err)
			continue
		}
		s.handle(conn, src, req, bootp)
	}
}

func (s *Server) handle(conn *net.UDPConn, src *net.UDPAddr, req *dhcpv4.DHCPv4, bootp bool) {
	resp, bootfile, ok := s.buildReply(req)
	if !ok {
		return
	}

	dst := &net.UDPAddr{IP: net.IPv4bcast, Port: 68}
	if !bootp {
		dst = src
	}

	if _, err := conn.WriteToUDP(resp.ToBytes(), dst); err != nil {
		log.Printf("proxyDHCP: send reply: %v", err)
		return
	}
	metrics.ProxyDHCPOffers.WithLabelValues(strconv.Itoa(int(clientArch(req)))).Inc()
	log.Printf("proxyDHCP: %s -> %s arch=%d bootfile=%s",
		req.MessageType(), req.ClientHWAddr, clientArch(req), bootfile)
}

func (s *Server) buildReply(req *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, string, bool) {
	vci := req.ClassIdentifier()
	if len(vci) < 9 || vci[:9] != "PXEClient" {
		return nil, "", false
	}

	var respType dhcpv4.MessageType
	switch req.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		respType = dhcpv4.MessageTypeOffer
	case dhcpv4.MessageTypeRequest, dhcpv4.MessageTypeInform:
		respType = dhcpv4.MessageTypeAck
	default:
		return nil, "", false
	}

	bootfile := s.bootfileFor(req)
	vendorOpts := pxeVendorOptions()
	if clientArch(req) == iana.INTEL_X86PC && raspberrypi.IsRaspberryPiMAC(req.ClientHWAddr) {
		vendorOpts = []byte("Raspberry Pi Boot")
	}
	modifiers := []dhcpv4.Modifier{
		dhcpv4.WithMessageType(respType),
		dhcpv4.WithServerIP(s.cfg.ServerIP),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(s.cfg.ServerIP)),
		dhcpv4.WithOption(dhcpv4.OptClassIdentifier("PXEClient")),
		dhcpv4.WithOption(dhcpv4.OptTFTPServerName(s.cfg.ServerIP.String())),
		dhcpv4.WithOption(dhcpv4.OptGeneric(dhcpv4.OptionVendorSpecificInformation, vendorOpts)),
	}
	if !s.cfg.NoBootfileption {
		modifiers = append(modifiers, dhcpv4.WithOption(dhcpv4.OptBootFileName(bootfile)))
	}
	resp, err := dhcpv4.NewReplyFromRequest(req, modifiers...)
	if err != nil {
		log.Printf("proxyDHCP: build reply: %v", err)
		return nil, "", false
	}
	resp.YourIPAddr = net.IPv4zero
	if guid := req.GetOneOption(dhcpv4.OptionClientMachineIdentifier); guid != nil {
		resp.UpdateOption(dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, guid))
	}
	resp.BootFileName = bootfile
	return resp, bootfile, true
}

func pxeVendorOptions() []byte {
	return []byte{
		0x06, 0x01, 0x08,
		0xff,
	}
}

func (s *Server) effectiveBootfiles() (bios, uefi, arm64 string) {
	bios, uefi, arm64 = s.cfg.BootfileBIOS, s.cfg.BootfileUEFI, s.cfg.BootfileARM64
	if s.cfg.Bootfiles != nil {
		overrideBIOS, overrideUEFI, overrideARM64 := s.cfg.Bootfiles()
		if overrideBIOS != "" {
			bios = overrideBIOS
		}
		if overrideUEFI != "" {
			uefi = overrideUEFI
		}
		if overrideARM64 != "" {
			arm64 = overrideARM64
		}
	}
	return bios, uefi, arm64
}

func (s *Server) bootfileFor(req *dhcpv4.DHCPv4) string {
	bios, uefi, arm64 := s.effectiveBootfiles()
	switch clientArch(req) {
	case iana.EFI_IA32, iana.EFI_X86_64, iana.EFI_BC,
		iana.EFI_X86_HTTP, iana.EFI_X86_64_HTTP, iana.EFI_BC_HTTP:
		return uefi
	case iana.EFI_ARM64, iana.EFI_ARM64_HTTP:
		return arm64
	default:
		return bios
	}
}

func clientArch(req *dhcpv4.DHCPv4) iana.Arch {
	archs := req.ClientArch()
	if len(archs) > 0 {
		return archs[0]
	}
	if arch, ok := archFromVCI(req.ClassIdentifier()); ok {
		return arch
	}
	return iana.INTEL_X86PC
}

func archFromVCI(vci string) (iana.Arch, bool) {
	const marker = ":Arch:"
	i := strings.Index(vci, marker)
	if i < 0 {
		return 0, false
	}
	digits := vci[i+len(marker):]
	if j := strings.IndexByte(digits, ':'); j >= 0 {
		digits = digits[:j]
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n < 0 || n > 0xffff {
		return 0, false
	}
	return iana.Arch(n), true
}

func defaultServerIP() (net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 != nil && !ip4.IsLoopback() {
				return ip4, nil
			}
		}
	}
	return nil, fmt.Errorf("no suitable IPv4 address found")
}
