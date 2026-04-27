// Package proxydhcp implements a PXE proxyDHCP responder (RFC 4578).
//
// It answers only the PXE-specific DHCP options (next-server, bootfile,
// vendor class) and never offers an IP address, so it coexists with any
// existing DHCP server on the same broadcast domain without conflict.
package proxydhcp

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"

	"bootimus/internal/metrics"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
)

type Config struct {
	// ServerIP is advertised as next-server. If zero, defaultServerIP picks one.
	ServerIP      net.IP
	BootfileBIOS  string
	BootfileUEFI  string
	BootfileARM64 string
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
		ip, err := defaultServerIP()
		if err != nil {
			return nil, fmt.Errorf("determine server IP: %w", err)
		}
		cfg.ServerIP = ip
	}
	if cfg.BootfileBIOS == "" {
		cfg.BootfileBIOS = "undionly.kpxe"
	}
	if cfg.BootfileUEFI == "" {
		cfg.BootfileUEFI = "bootimus.efi"
	}
	if cfg.BootfileARM64 == "" {
		cfg.BootfileARM64 = "bootimus-arm64.efi"
	}
	return &Server{cfg: cfg, done: make(chan struct{})}, nil
}

func (s *Server) Start() error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 67})
	if err != nil {
		return fmt.Errorf("listen UDP/67: %w (needs root or CAP_NET_BIND_SERVICE)", err)
	}
	// SO_BROADCAST lets us reply from source port 67 directly, which many PXE
	// ROMs require — they drop DHCP replies that don't come from port 67.
	if err := enableBroadcast(conn); err != nil {
		conn.Close()
		return fmt.Errorf("enable broadcast on UDP/67: %w", err)
	}
	s.conn = conn

	conn4011, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 4011})
	if err != nil {
		conn.Close()
		return fmt.Errorf("listen UDP/4011: %w", err)
	}
	s.conn4011 = conn4011

	log.Printf("proxyDHCP: listening on UDP/67 + UDP/4011, advertising next-server=%s (BIOS=%s, UEFI=%s, ARM64=%s)",
		s.cfg.ServerIP, s.cfg.BootfileBIOS, s.cfg.BootfileUEFI, s.cfg.BootfileARM64)

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
	vci := req.ClassIdentifier()
	if len(vci) < 9 || vci[:9] != "PXEClient" {
		return
	}

	var respType dhcpv4.MessageType
	switch req.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		respType = dhcpv4.MessageTypeOffer
	case dhcpv4.MessageTypeRequest, dhcpv4.MessageTypeInform:
		respType = dhcpv4.MessageTypeAck
	default:
		return
	}

	bootfile := s.bootfileFor(req)
	resp, err := dhcpv4.NewReplyFromRequest(req,
		dhcpv4.WithMessageType(respType),
		dhcpv4.WithServerIP(s.cfg.ServerIP),
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(s.cfg.ServerIP)),
		dhcpv4.WithOption(dhcpv4.OptClassIdentifier("PXEClient")),
		dhcpv4.WithOption(dhcpv4.OptTFTPServerName(s.cfg.ServerIP.String())),
		dhcpv4.WithOption(dhcpv4.OptBootFileName(bootfile)),
		dhcpv4.WithOption(dhcpv4.OptGeneric(dhcpv4.OptionVendorSpecificInformation, pxeVendorOptions())),
	)
	if err != nil {
		log.Printf("proxyDHCP: build reply: %v", err)
		return
	}
	// yiaddr must be zero — we are a proxy, not a DHCP server.
	resp.YourIPAddr = net.IPv4zero
	if guid := req.GetOneOption(dhcpv4.OptionClientMachineIdentifier); guid != nil {
		resp.UpdateOption(dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, guid))
	}
	// Older PXE ROMs read the bootp `file` header, not option 67.
	resp.BootFileName = bootfile

	dst := &net.UDPAddr{IP: net.IPv4bcast, Port: 68}
	if !bootp {
		// PXE boot-server REQUEST on 4011: reply unicast to the client source.
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

// pxeVendorOptions builds the DHCP option 43 (vendor-specific) payload for
// PXE. Sub-option 6 (PXE_DISCOVERY_CONTROL) with bit 3 set tells the client
// to use the BootFileName directly and skip boot-server discovery, which is
// what AMI/Supermicro UEFI PXE ROMs expect before they'll TFTP the bootfile.
func pxeVendorOptions() []byte {
	return []byte{
		0x06, 0x01, 0x08, // PXE_DISCOVERY_CONTROL = USE_BOOT_FILE
		0xff, // end
	}
}

func (s *Server) bootfileFor(req *dhcpv4.DHCPv4) string {
	switch clientArch(req) {
	case iana.EFI_IA32, iana.EFI_X86_64, iana.EFI_BC:
		return s.cfg.BootfileUEFI
	case iana.EFI_ARM64:
		return s.cfg.BootfileARM64
	default:
		return s.cfg.BootfileBIOS
	}
}

func clientArch(req *dhcpv4.DHCPv4) iana.Arch {
	archs := req.ClientArch()
	if len(archs) == 0 {
		return iana.INTEL_X86PC
	}
	return archs[0]
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