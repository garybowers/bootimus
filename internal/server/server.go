package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"bootimus/bootloaders"
	"bootimus/internal/admin"
	"bootimus/internal/auth"
	"bootimus/internal/autoinstall"
	"bootimus/internal/bmc"
	_ "bootimus/internal/bmc/amt"
	_ "bootimus/internal/bmc/redfish"
	"bootimus/internal/catalogue"
	"bootimus/internal/deployment"
	"bootimus/internal/diskimage"
	"bootimus/internal/housekeeping"
	"bootimus/internal/imagescan"
	"bootimus/internal/metrics"
	"bootimus/internal/models"
	"bootimus/internal/nbd"
	"bootimus/internal/netbind"
	"bootimus/internal/nfs"
	"bootimus/internal/profiles"
	"bootimus/internal/proxydhcp"
	"bootimus/internal/replica"
	"bootimus/internal/scheduler"
	"bootimus/internal/smb"
	"bootimus/internal/storage"
	"bootimus/internal/tlscert"
	"bootimus/internal/tools"
	"bootimus/internal/webhook"
	"bootimus/internal/wol"
	"bootimus/raspberrypi"
	"bootimus/web"

	"github.com/pin/tftp/v3"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var Version = "dev"

func panicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC RECOVERED: %v", err)
				log.Printf("Request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				log.Printf("Memory Stats at panic:")
				log.Printf("  Alloc = %d MB (currently allocated)", m.Alloc/1024/1024)
				log.Printf("  TotalAlloc = %d MB (total allocated over time)", m.TotalAlloc/1024/1024)
				log.Printf("  Sys = %d MB (obtained from system)", m.Sys/1024/1024)
				log.Printf("  NumGC = %d (number of GC runs)", m.NumGC)

				log.Printf("Stack trace:\n%s", debug.Stack())

				http.Error(w, "Internal Server Error - the request caused a panic. Check server logs for details.", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type Config struct {
	TFTPPort            int
	TFTPSinglePort      bool
	TFTPBlockSize       int
	HTTPPort            int
	AdminPort           int
	BootDir             string
	DataDir             string
	ISODir              string
	ServerAddr          string
	BindInterface       string
	Storage             storage.Storage
	Auth                *auth.Manager
	NBDEnabled          bool
	NBDPort             int
	NFSEnabled          bool
	InstallTimeoutHours int
	NFSPort             int
	WOLBroadcastAddr    string
	ProfileManager      *profiles.Manager

	ProxyDHCPEnabled          bool
	ProxyDHCPBootfileBIOS     string
	ProxyDHCPBootfileUEFI     string
	ProxyDHCPBootfileARM      string
	ProxyDHCPNoBootfileOption bool
	ProxyDHCPNoVendorOption   bool

	WindowsSMBEnabled bool
	WindowsSMBPort    int

	AdminTLS AdminTLSConfig
	Replica  *replica.Config
}

type AdminTLSConfig struct {
	Enabled      bool
	CertFile     string
	KeyFile      string
	SelfSigned   bool
	RedirectPort int
}

func (c AdminTLSConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if (c.CertFile == "") != (c.KeyFile == "") {
		return fmt.Errorf("admin TLS needs both a certificate file and a key file, or neither for a self-signed certificate")
	}
	if c.RedirectPort < 0 || c.RedirectPort > 65535 {
		return fmt.Errorf("admin TLS redirect port %d is out of range", c.RedirectPort)
	}
	return nil
}

func (c AdminTLSConfig) usesSelfSigned() bool {
	return c.Enabled && (c.SelfSigned || c.CertFile == "")
}

type Server struct {
	config                *Config
	httpServer            *http.Server
	adminServer           *http.Server
	adminRedirectServer   *http.Server
	adminTLS              *tlscert.Reloader
	adminTLSMode          string
	bootAuth              bootAuthentication
	tftpServer            *tftp.Server
	proxyDHCPServer       *proxydhcp.Server
	proxyDHCPRecorder     *proxydhcp.Recorder
	bsdSelections         *bsdSelections
	webhookNotifier       *webhook.Notifier
	scheduler             *scheduler.Scheduler
	housekeeping          *housekeeping.Runner
	replica               *replica.Syncer
	catalogue             *catalogue.Checker
	bootLogDedup          map[string]time.Time
	bootLogDedupMu        sync.Mutex
	installSweepStop      chan struct{}
	installSweepOnce      sync.Once
	installSweepWG        sync.WaitGroup
	wg                    sync.WaitGroup
	activeSessions        *ActiveSessions
	logBroadcaster        *LogBroadcaster
	audit                 *auditRecorder
	consoles              consoleSessions
	openConsole           func(ctx context.Context, t bmc.Target) (bmc.Console, error)
	activeBootloaderSet   string // name of active set folder, empty = built-in
	activeBootloaderSetMu sync.RWMutex
	toolsManager          *tools.Manager
	smbManager            *smb.Manager
	autoInstallLib        *autoinstall.Library
	diskImages            *diskimage.Manager
}

type ActiveSession struct {
	IP         string    `json:"ip"`
	Filename   string    `json:"filename"`
	StartedAt  time.Time `json:"started_at"`
	BytesRead  int64     `json:"bytes_read"`
	TotalBytes int64     `json:"total_bytes"`
	Activity   string    `json:"activity"`
}

type ActiveSessions struct {
	mu       sync.RWMutex
	sessions map[string]*ActiveSession
}

type LogBroadcaster struct {
	mu        sync.RWMutex
	clients   map[chan string]bool
	logBuffer []string
	maxBuffer int
}

var globalLogBuffer struct {
	mu     sync.RWMutex
	buffer []string
}

var globalLogBroadcaster *LogBroadcaster

type LogWriter struct{}

func (lw *LogWriter) Write(p []byte) (n int, err error) {
	msg := string(bytes.TrimRight(p, "\n"))

	globalLogBuffer.mu.Lock()
	globalLogBuffer.buffer = append(globalLogBuffer.buffer, msg)
	if len(globalLogBuffer.buffer) > 100 {
		globalLogBuffer.buffer = globalLogBuffer.buffer[1:]
	}
	globalLogBuffer.mu.Unlock()

	if globalLogBroadcaster != nil {
		globalLogBroadcaster.Broadcast(msg)
	}

	return os.Stdout.Write(p)
}

func InitGlobalLogger() {
	log.SetOutput(&LogWriter{})
	log.SetFlags(log.Ldate | log.Ltime)
}

func NewLogBroadcaster() *LogBroadcaster {
	lb := &LogBroadcaster{
		clients:   make(map[chan string]bool),
		logBuffer: make([]string, 0, 100),
		maxBuffer: 100,
	}

	globalLogBuffer.mu.RLock()
	lb.logBuffer = make([]string, len(globalLogBuffer.buffer))
	copy(lb.logBuffer, globalLogBuffer.buffer)
	globalLogBuffer.mu.RUnlock()

	return lb
}

func (lb *LogBroadcaster) Subscribe() chan string {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	ch := make(chan string, 10)
	lb.clients[ch] = true

	for _, msg := range lb.logBuffer {
		select {
		case ch <- msg:
		default:
		}
	}

	return ch
}

func (lb *LogBroadcaster) Unsubscribe(ch chan string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	delete(lb.clients, ch)
	close(ch)
}

func (lb *LogBroadcaster) Broadcast(msg string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.logBuffer = append(lb.logBuffer, msg)
	if len(lb.logBuffer) > lb.maxBuffer {
		lb.logBuffer = lb.logBuffer[1:]
	}

	globalLogBuffer.mu.Lock()
	globalLogBuffer.buffer = append(globalLogBuffer.buffer, msg)
	if len(globalLogBuffer.buffer) > 100 {
		globalLogBuffer.buffer = globalLogBuffer.buffer[1:]
	}
	globalLogBuffer.mu.Unlock()

	for ch := range lb.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (lb *LogBroadcaster) GetLogs() []string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	logs := make([]string, len(lb.logBuffer))
	copy(logs, lb.logBuffer)
	return logs
}

func (s *Server) logAndBroadcast(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
}

type ISOImage struct {
	Name      string
	Filename  string
	Size      int64
	SizeStr   string
	GroupPath string // relative directory path from isoDir, empty for root
}

type completionLogger struct {
	http.ResponseWriter
	filename       string
	remoteAddr     string
	fileSize       int64
	startTime      time.Time
	written        int64
	logged         bool
	activeSessions *ActiveSessions
}

func (w *completionLogger) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.written += int64(n)

	if w.activeSessions != nil {
		w.activeSessions.Update(w.remoteAddr, w.written)
	}

	if !w.logged && w.written >= w.fileSize {
		duration := time.Since(w.startTime)
		msg := fmt.Sprintf("ISO: Client %s finished downloading %s (%d MB) in %v",
			w.remoteAddr, w.filename, w.fileSize/1024/1024, duration.Round(time.Second))
		log.Print(msg)
		w.logged = true

		if w.activeSessions != nil {
			w.activeSessions.Remove(w.remoteAddr)
		}
	}

	return n, err
}

func New(cfg *Config) *Server {
	lb := NewLogBroadcaster()

	globalLogBroadcaster = lb

	tm := tools.NewManager(cfg.Storage, cfg.DataDir)
	tm.DisableRemoteCheck = cfg.ProfileManager != nil && cfg.ProfileManager.DisableRemoteCheck
	if err := tm.SeedTools(); err != nil {
		log.Printf("Tools: Failed to seed tools: %v", err)
	}

	s := &Server{
		config: cfg,
		activeSessions: &ActiveSessions{
			sessions: make(map[string]*ActiveSession),
		},
		logBroadcaster:    lb,
		toolsManager:      tm,
		bootLogDedup:      make(map[string]time.Time),
		webhookNotifier:   webhook.New(cfg.Storage),
		proxyDHCPRecorder: proxydhcp.NewRecorder(proxydhcp.DefaultRecorderSize),
		bsdSelections:     newBSDSelections(),
	}
	s.scheduler = scheduler.New(cfg.Storage, s.executeScheduledTask)
	if cfg.Storage != nil {
		s.housekeeping = housekeeping.New(cfg.Storage)
		s.diskImages = diskimage.New(cfg.Storage, diskimage.Config{
			DataDir:    cfg.DataDir,
			ServerAddr: cfg.ServerAddr,
			HTTPPort:   cfg.HTTPPort,
			NFSPort:    cfg.NFSPort,
			NFSEnabled: cfg.NFSEnabled,
		}, tm)
		s.diskImages.OnEvent = s.fireDiskImagingEvent
		s.catalogue = catalogue.NewChecker(cfg.Storage, tm.DisableRemoteCheck)
		s.housekeeping.Register(s.catalogue)
	}
	s.loadBootloaderConfig()
	return s
}

func (s *Server) bootloaderConfigPath() string {
	return filepath.Join(s.config.DataDir, "bootloader-config.json")
}

type bootloaderConfigFile struct {
	ActiveSet string `json:"active_set"`
}

const bootloaderSetRequestPrefix = "bootloader-sets/"

func (s *Server) loadBootloaderConfig() {
	data, err := os.ReadFile(s.bootloaderConfigPath())
	if err != nil {
		return
	}
	var cfg bootloaderConfigFile
	if json.Unmarshal(data, &cfg) == nil {
		s.activeBootloaderSetMu.Lock()
		s.activeBootloaderSet = cfg.ActiveSet
		s.activeBootloaderSetMu.Unlock()
	}
}

func (s *Server) SaveBootloaderConfig() error {
	s.activeBootloaderSetMu.RLock()
	cfg := bootloaderConfigFile{ActiveSet: s.activeBootloaderSet}
	s.activeBootloaderSetMu.RUnlock()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.bootloaderConfigPath(), data, 0644)
}

func (s *Server) GetActiveBootloaderSet() string {
	s.activeBootloaderSetMu.RLock()
	defer s.activeBootloaderSetMu.RUnlock()
	return s.activeBootloaderSet
}

func (s *Server) SetActiveBootloaderSet(name string) {
	s.activeBootloaderSetMu.Lock()
	s.activeBootloaderSet = name
	s.activeBootloaderSetMu.Unlock()

	display := name
	if display == "" {
		display = bootloaders.DefaultSet
	}
	bios, uefi, arm64 := s.proxyDHCPBootfiles()
	log.Printf("Bootloader set %q active: PXE bootfiles BIOS=%s UEFI=%s ARM64=%s", display, bios, uefi, arm64)
}

func normaliseBootloaderSetName(setName string) string {
	setName = strings.TrimSpace(setName)
	if setName == "" || strings.ContainsAny(setName, `/\`) || setName != path.Base(setName) || setName == "." || setName == ".." {
		return ""
	}
	return setName
}

func (s *Server) activeBootloaderSetName() string {
	if setName := normaliseBootloaderSetName(s.GetActiveBootloaderSet()); setName != "" {
		return setName
	}
	return bootloaders.DefaultSet
}

func (s *Server) bootloaderSetManifest(setName string) *bootloaders.Manifest {
	setName = normaliseBootloaderSetName(setName)
	if setName == "" {
		return nil
	}
	if s.config.BootDir != "" {
		diskPath := filepath.Join(s.config.BootDir, setName, "manifest.json")
		if data, err := os.ReadFile(diskPath); err == nil {
			if m, err := bootloaders.ParseManifest(data); err == nil {
				return m
			}
		}
	}
	m, err := bootloaders.LoadManifest(setName)
	if err != nil {
		return nil
	}
	return m
}

func (s *Server) proxyDHCPOmitVendorOption(mac net.HardwareAddr) bool {
	if s.config.Storage == nil || len(mac) == 0 {
		return false
	}
	client, err := s.config.Storage.GetClient(strings.ToLower(mac.String()))
	return err == nil && client != nil && client.ProxyDHCPNoVendorOption
}

// proxyDHCPBootfiles returns the bootfile names proxyDHCP should advertise.
// Precedence: explicitly configured value (differs from the compiled default)
// > active bootloader set manifest > compiled default. Evaluated per DHCP
// request, so switching sets takes effect without a restart.
func (s *Server) proxyDHCPBootfiles() (bios, uefi, arm64 string) {
	bios, uefi, arm64 = s.bootfilesForSet(s.activeBootloaderSetName())
	return configuredBootfile(s.config.ProxyDHCPBootfileBIOS, proxydhcp.DefaultBootfileBIOS, bios),
		configuredBootfile(s.config.ProxyDHCPBootfileUEFI, proxydhcp.DefaultBootfileUEFI, uefi),
		configuredBootfile(s.config.ProxyDHCPBootfileARM, proxydhcp.DefaultBootfileARM64, arm64)
}

func configuredBootfile(configured, defaultName, setName string) string {
	if configured != "" && configured != defaultName {
		return configured
	}
	return setName
}

func (s *Server) bootfilesForSet(setName string) (bios, uefi, arm64 string) {
	bios = proxydhcp.DefaultBootfileBIOS
	uefi = proxydhcp.DefaultBootfileUEFI
	arm64 = proxydhcp.DefaultBootfileARM64
	if manifest := s.bootloaderSetManifest(setName); manifest != nil {
		if manifest.Bootfiles.BIOS != "" {
			bios = manifest.Bootfiles.BIOS
		}
		if manifest.Bootfiles.UEFI != "" {
			uefi = manifest.Bootfiles.UEFI
		}
		if manifest.Bootfiles.ARM64 != "" {
			arm64 = manifest.Bootfiles.ARM64
		}
	}
	return bios, uefi, arm64
}

func (s *Server) effectiveBootloaderSet(clientHWAddr net.HardwareAddr) (string, bool) {
	globalSet := s.activeBootloaderSetName()
	if len(clientHWAddr) == 0 || s.config.Storage == nil {
		return globalSet, false
	}

	mac := strings.ToLower(clientHWAddr.String())
	clientSet, groupSet, err := s.config.Storage.GetClientBootloaderSets(mac)
	if err != nil {
		return globalSet, false
	}
	if setName := normaliseBootloaderSetName(clientSet); setName != "" {
		return setName, true
	}
	if setName := normaliseBootloaderSetName(s.profileBootloaderSet(mac)); setName != "" {
		return setName, true
	}
	if setName := normaliseBootloaderSetName(groupSet); setName != "" {
		return setName, true
	}
	return globalSet, false
}

func (s *Server) profileBootloaderSet(mac string) string {
	client, err := s.config.Storage.GetClient(mac)
	if err != nil {
		return ""
	}
	if client.ProfileID == nil && client.ClientGroupID == nil {
		return ""
	}
	eff, _ := s.effectiveFor(client)
	switch eff.Sources["bootloader_set"] {
	case deployment.SourceProfile, deployment.SourceGroupProfile:
		return eff.BootloaderSet
	}
	return ""
}

func cleanBootloaderPath(filename string) (string, bool) {
	if strings.Contains(filename, `\`) {
		return "", false
	}
	filename = path.Clean(filename)
	if filename == "." || filename == ".." || path.IsAbs(filename) || strings.HasPrefix(filename, "../") {
		return "", false
	}
	return filename, true
}

func qualifyBootloaderSetRequest(setName, filename string) string {
	setName = normaliseBootloaderSetName(setName)
	filename, valid := cleanBootloaderPath(filename)
	if setName == "" || !valid {
		return ""
	}
	return path.Join(bootloaderSetRequestPrefix, setName, filename)
}

func (s *Server) proxyDHCPBootfilesForClient(clientHWAddr net.HardwareAddr) (bios, uefi, arm64 string) {
	setName, overridden := s.effectiveBootloaderSet(clientHWAddr)
	if !overridden {
		return s.proxyDHCPBootfiles()
	}

	bios, uefi, arm64 = s.bootfilesForSet(setName)
	return configuredOrQualifiedBootfile(s.config.ProxyDHCPBootfileBIOS, proxydhcp.DefaultBootfileBIOS, setName, bios),
		configuredOrQualifiedBootfile(s.config.ProxyDHCPBootfileUEFI, proxydhcp.DefaultBootfileUEFI, setName, uefi),
		configuredOrQualifiedBootfile(s.config.ProxyDHCPBootfileARM, proxydhcp.DefaultBootfileARM64, setName, arm64)
}

func configuredOrQualifiedBootfile(configured, defaultName, setName, setFilename string) string {
	if configured != "" && configured != defaultName {
		return configured
	}
	return qualifyBootloaderSetRequest(setName, setFilename)
}

func (s *Server) resolveBootloaderRequest(requestPath string) (setName, filename string, err error) {
	cleanPath, valid := cleanBootloaderPath(requestPath)
	if !valid {
		return "", "", fmt.Errorf("invalid bootloader path: %s", requestPath)
	}
	if !strings.HasPrefix(cleanPath, bootloaderSetRequestPrefix) {
		return s.activeBootloaderSetName(), cleanPath, nil
	}

	relativePath := strings.TrimPrefix(cleanPath, bootloaderSetRequestPrefix)
	separator := strings.IndexByte(relativePath, '/')
	if separator <= 0 || separator == len(relativePath)-1 {
		return "", "", fmt.Errorf("invalid qualified bootloader path: %s", requestPath)
	}
	setName = normaliseBootloaderSetName(relativePath[:separator])
	filename, valid = cleanBootloaderPath(relativePath[separator+1:])
	if setName == "" || !valid {
		return "", "", fmt.Errorf("invalid qualified bootloader path: %s", requestPath)
	}
	return setName, filename, nil
}

func (s *Server) resolveRaspberryPiOverride(filename string) string {
	if s.config.DataDir == "" || strings.Contains(filename, "..") {
		return ""
	}
	base := filepath.Join(s.config.DataDir, "raspberrypi")
	for _, candidate := range []string{filename, raspberrypi.StripSerialPrefix(filename)} {
		fullPath := filepath.Join(base, candidate)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			return fullPath
		}
	}
	return ""
}

func (s *Server) resolveBootloaderFile(setName, filename string) string {
	setName = normaliseBootloaderSetName(setName)
	filename, valid := cleanBootloaderPath(filename)
	if setName == "" || !valid || s.config.BootDir == "" {
		return ""
	}
	setDir := filepath.Join(s.config.BootDir, setName)
	fullPath := filepath.Join(setDir, filepath.FromSlash(filename))
	if _, err := os.Stat(fullPath); err == nil {
		return fullPath
	}
	return ""
}

func (as *ActiveSessions) Add(ip, filename string, totalBytes int64, activity string) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.sessions[ip] = &ActiveSession{
		IP:         ip,
		Filename:   filename,
		StartedAt:  time.Now(),
		BytesRead:  0,
		TotalBytes: totalBytes,
		Activity:   activity,
	}
}

func (as *ActiveSessions) Update(ip string, bytesRead int64) {
	as.mu.Lock()
	defer as.mu.Unlock()
	if session, ok := as.sessions[ip]; ok {
		session.BytesRead = bytesRead
	}
}

func (as *ActiveSessions) Remove(ip string) {
	as.mu.Lock()
	defer as.mu.Unlock()
	delete(as.sessions, ip)
}

func (as *ActiveSessions) CleanupStale(maxAge time.Duration) {
	as.mu.Lock()
	defer as.mu.Unlock()
	now := time.Now()
	for ip, session := range as.sessions {
		if now.Sub(session.StartedAt) > maxAge {
			log.Printf("Cleaning up stale session: %s downloading %s (started %v ago)", ip, session.Filename, now.Sub(session.StartedAt).Round(time.Second))
			delete(as.sessions, ip)
		}
	}
}

func (as *ActiveSessions) GetAll() []ActiveSession {
	as.mu.RLock()
	defer as.mu.RUnlock()
	sessions := make([]ActiveSession, 0, len(as.sessions))
	for _, s := range as.sessions {
		sessions = append(sessions, *s)
	}
	return sessions
}

func (s *Server) Start() error {
	if err := s.prepareAdminTLS(); err != nil {
		return fmt.Errorf("admin TLS: %w", err)
	}
	tftpConn, err := netbind.ListenUDP(s.config.BindInterface, "udp", fmt.Sprintf(":%d", s.config.TFTPPort))
	if err != nil {
		return fmt.Errorf("listen TFTP: %w", err)
	}
	httpListener, err := netbind.Listen(s.config.BindInterface, "tcp", fmt.Sprintf(":%d", s.config.HTTPPort))
	if err != nil {
		tftpConn.Close()
		return fmt.Errorf("listen boot HTTP: %w", err)
	}
	adminListener, err := netbind.Listen(s.config.BindInterface, "tcp", fmt.Sprintf(":%d", s.config.AdminPort))
	if err != nil {
		httpListener.Close()
		tftpConn.Close()
		return fmt.Errorf("listen admin HTTP: %w", err)
	}
	s.tftpServer = s.newTFTPServer()

	log.Printf("Starting Bootimus - PXE/HTTP Boot Server")
	log.Printf("Boot directory: %s", s.config.BootDir)
	log.Printf("Data directory: %s", s.config.DataDir)
	log.Printf("ISO directory: %s", s.config.ISODir)
	log.Printf("TFTP Port: %d", s.config.TFTPPort)
	log.Printf("HTTP Port: %d", s.config.HTTPPort)
	log.Printf("Admin Port: %d", s.config.AdminPort)
	log.Printf("Server Address: %s", s.config.ServerAddr)
	if s.config.BindInterface != "" {
		log.Printf("Binding all listeners to interface: %s", s.config.BindInterface)
	}

	if mgr, err := autoinstall.New(s.config.DataDir); err != nil {
		log.Printf("Warning: could not initialise autoinstall manager: %v", err)
	} else {
		s.autoInstallLib = mgr
		log.Printf("Auto-install files directory: %s", mgr.Root())
	}

	isoFiles, err := imagescan.Scan(s.config.ISODir)
	if err != nil {
		log.Printf("Warning: Failed to scan ISOs: %v", err)
	} else {
		log.Printf("Found %d image(s)", len(isoFiles))
		for _, f := range isoFiles {
			log.Printf("  - %s (%s)", f.Name, formatBytes(f.Size))
		}

		if s.config.Storage != nil {
			if err := s.config.Storage.SyncImages(isoFiles); err != nil {
				log.Printf("Warning: Failed to sync images with database: %v", err)
			}
		}
	}

	if s.config.WindowsSMBEnabled {
		mgr := smb.NewManager(s.config.DataDir, s.config.ServerAddr, s.config.BindInterface, s.config.WindowsSMBPort)
		s.preloadSMBShares(mgr)
		if err := mgr.Start(); err != nil {
			log.Printf("Windows SMB: requested but could not start: %v - feature disabled for this run", err)
		}
		s.smbManager = mgr
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.startTFTPServer(tftpConn); err != nil {
			log.Printf("TFTP server error: %v", err)
		}
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.startHTTPServer(httpListener); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.startAdminServer(adminListener); err != nil {
			log.Printf("Admin server error: %v", err)
		}
	}()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			s.activeSessions.CleanupStale(30 * time.Minute)
		}
	}()

	if s.config.NBDEnabled {
		log.Printf("NBD Port: %d", s.config.NBDPort)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			nbdServer := nbd.NewServer(s.config.ISODir, s.config.NBDPort, s.config.BindInterface)
			if err := nbdServer.Start(); err != nil {
				log.Printf("NBD server error: %v", err)
			}
		}()
	}

	if s.config.NFSEnabled {
		log.Printf("NFS Port: %d", s.config.NFSPort)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			nfsServer := nfs.NewServer(s.config.ISODir, s.config.NFSPort, s.config.BindInterface)
			if s.diskImages != nil {
				nfsServer.DiskImageDir = s.diskImages.Root()
			}
			if err := nfsServer.Start(); err != nil {
				log.Printf("NFS server error: %v", err)
			}
		}()
	}

	if s.scheduler != nil {
		s.scheduler.Start()
	}

	if s.housekeeping != nil {
		s.housekeeping.Start()
	}

	s.startInstallSweeper()

	if s.config.ProxyDHCPEnabled {
		pd, err := proxydhcp.NewServer(proxydhcp.Config{
			ServerIP:         net.ParseIP(s.config.ServerAddr),
			ServerHTTPPort:   s.config.HTTPPort,
			Interface:        s.config.BindInterface,
			BootfileBIOS:     s.config.ProxyDHCPBootfileBIOS,
			BootfileUEFI:     s.config.ProxyDHCPBootfileUEFI,
			BootfileARM64:    s.config.ProxyDHCPBootfileARM,
			NoBootfileption:  s.config.ProxyDHCPNoBootfileOption,
			NoVendorOption:   s.config.ProxyDHCPNoVendorOption,
			OmitVendorOption: s.proxyDHCPOmitVendorOption,
			Bootfiles:        s.proxyDHCPBootfilesForClient,
			Observer:         s.proxyDHCPRecorder.Record,
		})
		if err != nil {
			log.Printf("proxyDHCP: failed to construct server: %v", err)
		} else if err := pd.Start(); err != nil {
			log.Printf("proxyDHCP: failed to start: %v", err)
		} else {
			s.proxyDHCPServer = pd
		}
	}

	return nil
}

func (s *Server) preloadSMBShares(mgr *smb.Manager) {
	if mgr == nil || s.config.Storage == nil {
		return
	}
	images, err := s.config.Storage.ListImages()
	if err != nil {
		log.Printf("Windows SMB: failed to list images for share preload: %v", err)
		return
	}
	added := 0
	for _, img := range images {
		if img.Distro != "windows" || !img.SMBInstallEnabled {
			continue
		}
		isoBase := strings.TrimSuffix(img.Filename, filepath.Ext(img.Filename))
		sharePath := filepath.Join(s.config.ISODir, isoBase, "iso")
		if _, err := os.Stat(sharePath); err != nil {
			continue
		}
		if err := mgr.AddShare(smb.ShareName(img.Filename), sharePath); err != nil {
			log.Printf("windows smb: cannot preload share for %s: %v", img.Filename, err)
			continue
		}
		added++
	}
	if added > 0 {
		log.Printf("Windows SMB: preloaded %d share(s) for smbd startup", added)
	}
}

func (s *Server) Wait() {
	s.wg.Wait()
}

func (s *Server) Shutdown() error {
	log.Println("Initiating graceful shutdown...")

	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		} else {
			log.Println("HTTP server stopped")
		}
	}

	if s.adminServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if err := s.adminServer.Shutdown(ctx); err != nil {
			log.Printf("Admin server shutdown error: %v", err)
		} else {
			log.Println("Admin server stopped")
		}
	}

	if s.adminRedirectServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if err := s.adminRedirectServer.Shutdown(ctx); err != nil {
			log.Printf("Admin redirect server shutdown error: %v", err)
		}
	}

	if s.tftpServer != nil {
		s.tftpServer.Shutdown()
		log.Println("TFTP server stopped")
	}

	if s.proxyDHCPServer != nil {
		if err := s.proxyDHCPServer.Shutdown(); err != nil {
			log.Printf("proxyDHCP server shutdown error: %v", err)
		} else {
			log.Println("proxyDHCP server stopped")
		}
	}

	if s.scheduler != nil {
		s.scheduler.Stop()
		log.Println("Scheduler stopped")
	}

	if s.housekeeping != nil {
		s.housekeeping.Stop()
		log.Println("Housekeeping stopped")
	}

	if s.replica != nil {
		s.replica.Stop()
		log.Println("Replica sync stopped")
	}

	s.stopInstallSweeper()

	if s.smbManager != nil {
		s.smbManager.Stop()
		log.Println("SMB server stopped")
	}

	log.Println("Shutdown complete")

	return nil
}

func (s *Server) scanISOs() ([]ISOImage, error) {
	files, err := imagescan.Scan(s.config.ISODir)
	if err != nil {
		return nil, err
	}

	isos := make([]ISOImage, 0, len(files))
	for _, f := range files {
		isos = append(isos, ISOImage{
			Name:      f.Name,
			Filename:  f.Filename,
			Size:      f.Size,
			SizeStr:   formatBytes(f.Size),
			GroupPath: f.GroupPath,
		})
	}

	sort.Slice(isos, func(i, j int) bool {
		return isos[i].Name < isos[j].Name
	})

	return isos, nil
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

type tftpRemoteAddrer interface {
	RemoteAddr() net.UDPAddr
}

type tftpDebugHook struct{}

func (tftpDebugHook) OnSuccess(stats tftp.TransferStats) {
	log.Printf("TFTP DEBUG: ✓ %s %s sent=%d acked=%d duration=%s mode=%s",
		stats.RemoteAddr, stats.Filename,
		stats.DatagramsSent, stats.DatagramsAcked,
		stats.Duration, stats.Mode)
}

func (tftpDebugHook) OnFailure(stats tftp.TransferStats, err error) {
	log.Printf("TFTP DEBUG: ✗ %s %s sent=%d acked=%d duration=%s err=%v",
		stats.RemoteAddr, stats.Filename,
		stats.DatagramsSent, stats.DatagramsAcked,
		stats.Duration, err)
}

func tftpRemote(rf io.ReaderFrom) string {
	if ra, ok := rf.(tftpRemoteAddrer); ok {
		addr := ra.RemoteAddr()
		return addr.String()
	}
	return "?"
}

func sanitiseTFTPFilename(name string) string {
	if i := strings.IndexFunc(name, func(r rune) bool {
		return r == utf8.RuneError || r < 0x20 || r == 0x7f
	}); i >= 0 {
		name = name[:i]
	}
	return name
}

func (s *Server) serveTFTPFile(filename string, rf io.ReaderFrom) error {
	if clean := sanitiseTFTPFilename(filename); clean != filename {
		log.Printf("TFTP: Truncated malformed filename %q to %q", filename, clean)
		filename = clean
	}
	cleanPath := filepath.Clean(filename)
	if filepath.IsAbs(cleanPath) {
		cleanPath = filepath.Base(cleanPath)
	}
	cleanPath = filepath.ToSlash(cleanPath)

	remote := tftpRemote(rf)
	start := time.Now()
	defer func() {
		log.Printf("TFTP DEBUG: handler exit %s file=%s elapsed=%s", remote, filename, time.Since(start))
	}()
	log.Printf("TFTP DEBUG: handler entry %s file=%s", remote, filename)
	log.Printf("TFTP: Client requesting file: %s", filename)
	metrics.TFTPRequests.WithLabelValues(cleanPath).Inc()

	if handled, err := s.serveBSDTFTP(cleanPath, remote, rf); handled {
		if err != nil {
			log.Printf("TFTP: %v", err)
		}
		return err
	}

	setName, bootloaderFilename, err := s.resolveBootloaderRequest(cleanPath)
	if err != nil {
		return err
	}

	if bootloaderFilename == "autoexec.ipxe" {
		serverAddr := "${next-server}"
		if s.config.ServerAddr != "" {
			serverAddr = s.config.ServerAddr
		}
		script := fmt.Sprintf(`#!ipxe

# Auto-detect server IP and chain to dynamic menu
:dhcp
dhcp
chain http://%s:%d/inventory?mac=${net0/mac}&cpu=${cpuid/0}&memsize=${memsize}&platform=${platform}&buildarch=${buildarch}&product=${product}&manufacturer=${manufacturer}&serial=${serial}&asset=${asset}&uuid=${uuid}&nic_chip=${net0/chip} || chain http://%s:%d/menu.ipxe?mac=${net0/mac} || goto failed
%s
:failed
echo Failed to load boot menu
echo Server: ${next-server}
echo MAC: ${net0/mac}
echo Press any key to retry...
prompt
goto dhcp
%s`, serverAddr, s.config.HTTPPort, serverAddr, s.config.HTTPPort, ipxeLocalBootCheck, ipxeLocalBootTail)
		data := []byte(script)
		log.Printf("TFTP: Serving dynamic autoexec.ipxe (HTTP port: %d)", s.config.HTTPPort)

		if rfs, ok := rf.(interface{ SetSize(int64) error }); ok {
			rfs.SetSize(int64(len(data)))
		}

		n, err := rf.ReadFrom(bytes.NewReader(data))
		if err != nil {
			log.Printf("TFTP: Transfer error for %s: %v", filename, err)
			return err
		}

		log.Printf("TFTP: Successfully sent %s (%d bytes)", filename, n)
		return nil
	}

	if customPath := s.resolveBootloaderFile(setName, bootloaderFilename); customPath != "" {
		file, err := os.Open(customPath)
		if err == nil {
			defer file.Close()
			log.Printf("TFTP: Serving from set '%s': %s", setName, bootloaderFilename)

			fileInfo, err := file.Stat()
			if err != nil {
				return err
			}

			if rfs, ok := rf.(interface{ SetSize(int64) error }); ok {
				rfs.SetSize(fileInfo.Size())
			}

			n, err := rf.ReadFrom(file)
			if err != nil {
				log.Printf("TFTP: Transfer error for %s: %v", filename, err)
				return err
			}

			log.Printf("TFTP: Successfully sent %s (%d bytes)", filename, n)
			return nil
		}
	}

	data, resolvedSet, err := bootloaders.Resolve(setName, bootloaderFilename)
	if err == nil {
		log.Printf("TFTP: Serving embedded bootloader from set '%s': %s", resolvedSet, bootloaderFilename)

		if rfs, ok := rf.(interface{ SetSize(int64) error }); ok {
			rfs.SetSize(int64(len(data)))
		}

		n, err := rf.ReadFrom(bytes.NewReader(data))
		if err != nil {
			log.Printf("TFTP: Transfer error for %s: %v", filename, err)
			return err
		}

		log.Printf("TFTP: Successfully sent %s (%d bytes)", filename, n)
		return nil
	}

	if diskPath := s.resolveRaspberryPiOverride(cleanPath); diskPath != "" {
		file, err := os.Open(diskPath)
		if err == nil {
			defer file.Close()
			log.Printf("TFTP: Serving Raspberry Pi override from disk: %s", cleanPath)

			fileInfo, err := file.Stat()
			if err != nil {
				return err
			}
			if rfs, ok := rf.(interface{ SetSize(int64) error }); ok {
				rfs.SetSize(fileInfo.Size())
			}
			n, err := rf.ReadFrom(file)
			if err != nil {
				log.Printf("TFTP: Transfer error for %s: %v", filename, err)
				return err
			}
			log.Printf("TFTP: Successfully sent %s (%d bytes)", filename, n)
			return nil
		}
	}

	if data, ok := raspberrypi.Resolve(cleanPath); ok {
		log.Printf("TFTP: Serving embedded Raspberry Pi boot asset: %s", cleanPath)

		if rfs, ok := rf.(interface{ SetSize(int64) error }); ok {
			rfs.SetSize(int64(len(data)))
		}
		n, err := rf.ReadFrom(bytes.NewReader(data))
		if err != nil {
			log.Printf("TFTP: Transfer error for %s: %v", filename, err)
			return err
		}
		log.Printf("TFTP: Successfully sent %s (%d bytes)", filename, n)
		return nil
	}

	return fmt.Errorf("file not found: %s", filename)
}

func (s *Server) newTFTPServer() *tftp.Server {
	server := tftp.NewServer(s.serveTFTPFile, nil)

	server.SetTimeout(5 * time.Second)
	blockSize := s.config.TFTPBlockSize
	if blockSize <= 0 {
		blockSize = 1456
	}
	server.SetBlockSize(blockSize)
	log.Printf("TFTP block size: %d bytes (clients may negotiate up to this via blksize option)", blockSize)
	server.SetHook(tftpDebugHook{})
	if s.config.TFTPSinglePort {
		log.Print("Enabling single port mode for TFTP server")
		server.EnableSinglePort()
	}

	return server
}

func (s *Server) startTFTPServer(conn *net.UDPConn) error {
	log.Printf("starting TFTP server on port %d", s.config.TFTPPort)
	defer conn.Close()
	if err := s.tftpServer.Serve(conn); err != nil {
		return fmt.Errorf("TFTP server failed: %w", err)
	}

	return nil
}

func (s *Server) startHTTPServer(listener net.Listener) error {
	defer listener.Close()
	log.Printf("Starting HTTP server on port %d...", s.config.HTTPPort)

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("HTTP: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		cleanPath := strings.TrimPrefix(r.URL.Path, "/")
		if cleanPath == "" {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		setName, bootloaderFilename, err := s.resolveBootloaderRequest(cleanPath)
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		if customPath := s.resolveBootloaderFile(setName, bootloaderFilename); customPath != "" {
			log.Printf("HTTP: Serving from set '%s': %s", setName, bootloaderFilename)
			ext := filepath.Ext(bootloaderFilename)
			if ext == ".efi" || ext == ".img" || ext == ".iso" || ext == ".kpxe" || ext == ".usb" {
				w.Header().Set("Content-Type", "application/octet-stream")
			}
			http.ServeFile(w, r, customPath)
			return
		}

		data, resolvedSet, err := bootloaders.Resolve(setName, bootloaderFilename)
		if err == nil {
			log.Printf("HTTP: Serving embedded bootloader from set '%s': %s", resolvedSet, bootloaderFilename)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(data)
			return
		}

		http.Error(w, "Not found", http.StatusNotFound)
	})

	mux.HandleFunc("/inventory", s.handleInventoryReport)
	mux.HandleFunc("/menu.ipxe", s.handleIPXEMenu)

	toolsDir := filepath.Join(s.config.DataDir, "tools")
	mux.Handle("/tools/", http.StripPrefix("/tools/", http.FileServer(http.Dir(toolsDir))))

	mux.HandleFunc("/autoexec.ipxe", s.handleAutoexec)

	mux.HandleFunc("/isos/", func(w http.ResponseWriter, r *http.Request) {
		filename := strings.TrimPrefix(r.URL.Path, "/isos/")
		decodedFilename, err := url.PathUnescape(filename)
		if err != nil {
			log.Printf("ISO: Failed to decode filename %s: %v", filename, err)
			http.Error(w, "Invalid filename", http.StatusBadRequest)
			return
		}

		macAddress := r.URL.Query().Get("mac")
		if macAddress == "" {
			macAddress = "unknown"
		} else {
			macAddress = strings.ToLower(strings.ReplaceAll(macAddress, "-", ":"))
		}

		fullPath := filepath.Join(s.config.ISODir, decodedFilename)

		cleanPath := filepath.Clean(fullPath)
		if !strings.HasPrefix(cleanPath, filepath.Clean(s.config.ISODir)) {
			s.logAndBroadcast("ISO: Path traversal attempt from MAC %s (IP: %s): %s", macAddress, r.RemoteAddr, decodedFilename)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		fileInfo, err := os.Stat(fullPath)
		if err != nil {
			s.logAndBroadcast("ISO: File not found (MAC: %s, IP: %s): %s", macAddress, r.RemoteAddr, decodedFilename)
			http.NotFound(w, r)
			return
		}

		if fileInfo.IsDir() {
			log.Printf("ISO: Path is a directory: %s", fullPath)
			http.Error(w, "Not a file", http.StatusBadRequest)
			return
		}

		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			s.logAndBroadcast("ISO Download: Client MAC %s (IP: %s) started downloading %s (%d MB)", macAddress, r.RemoteAddr, decodedFilename, fileInfo.Size()/1024/1024)
			s.activeSessions.Add(r.RemoteAddr, decodedFilename, fileInfo.Size(), "downloading")
		} else {
			log.Printf("ISO: Range request from MAC %s (IP: %s) for %s - Range: %s", macAddress, r.RemoteAddr, decodedFilename, rangeHeader)
		}

		wrappedWriter := &completionLogger{
			ResponseWriter: w,
			filename:       decodedFilename,
			remoteAddr:     r.RemoteAddr,
			fileSize:       fileInfo.Size(),
			startTime:      time.Now(),
			activeSessions: s.activeSessions,
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(wrappedWriter, r, fullPath)

		if rangeHeader == "" {
			s.activeSessions.Remove(r.RemoteAddr)
		}
	})

	mux.HandleFunc("/boot/", func(w http.ResponseWriter, r *http.Request) {
		urlPath := strings.TrimPrefix(r.URL.Path, "/boot/")
		decodedPath, err := url.PathUnescape(urlPath)
		if err != nil {
			log.Printf("Boot: Failed to decode path %s: %v", urlPath, err)
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		macAddress := r.URL.Query().Get("mac")
		if macAddress == "" {
			macAddress = "unknown"
		} else {
			macAddress = strings.ToLower(strings.ReplaceAll(macAddress, "-", ":"))
		}

		fullPath := filepath.Join(s.config.ISODir, decodedPath)

		cleanPath := filepath.Clean(fullPath)
		if !strings.HasPrefix(cleanPath, filepath.Clean(s.config.ISODir)) {
			s.logAndBroadcast("Boot: Path traversal attempt from MAC %s (IP: %s): %s", macAddress, r.RemoteAddr, decodedPath)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		fileInfo, err := os.Stat(fullPath)
		if err != nil {
			s.logAndBroadcast("Boot: File not found (MAC: %s, IP: %s): %s", macAddress, r.RemoteAddr, decodedPath)
			http.NotFound(w, r)
			return
		}

		if fileInfo.IsDir() {
			log.Printf("Boot: Path is a directory: %s", fullPath)
			http.Error(w, "Not a file", http.StatusBadRequest)
			return
		}

		if r.Header.Get("Range") == "" {
			s.logAndBroadcast("Boot File: Serving %s (%d MB) to MAC %s (IP: %s)", decodedPath, fileInfo.Size()/1024/1024, macAddress, r.RemoteAddr)
			s.recordBootIfNew(macAddress, decodedPath, r.RemoteAddr)
			metrics.HTTPBootRequests.Inc()
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, fullPath)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK\n")
	})

	mux.HandleFunc("/api/isos", s.handleListISOs)

	mux.HandleFunc("/autoinstall/", s.handleAutoInstallScript)
	mux.HandleFunc("/install/callback", s.handleInstallCallback)

	mux.HandleFunc("/machineconfig", s.handleMachineConfig)

	mux.Handle("/node-images/", http.StripPrefix("/node-images/", http.FileServer(http.Dir(s.provisionAssetDir()))))

	mux.HandleFunc("/files/", s.handleCustomFile)
	mux.HandleFunc("/diskimage/job", s.handleDiskImageJob)
	mux.HandleFunc("/diskimage/callback", s.handleDiskImageCallback)

	mux.HandleFunc("/bsd/", s.handleBSDBoot)

	mux.HandleFunc("/bootenv/", func(w http.ResponseWriter, r *http.Request) {
		urlPath := strings.TrimPrefix(r.URL.Path, "/bootenv/")
		filePath := path.Join("bootenv", urlPath)
		log.Printf("HTTP: Bootenv request - %s (always embedded)", urlPath)

		data, _, err := bootloaders.Resolve(bootloaders.DefaultSet, filePath)
		if err != nil {
			log.Printf("HTTP: Error reading embedded bootenv file %s: %v", filePath, err)
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		log.Printf("HTTP: Successfully read embedded bootenv file %s (%d bytes)", filePath, len(data))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(data)
	})

	addr := fmt.Sprintf(":%d", s.config.HTTPPort)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server failed: %w", err)
	}

	return nil
}

func (s *Server) prepareAdminTLS() error {
	cfg := s.config.AdminTLS
	if !cfg.Enabled {
		return nil
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	var load func() (tls.Certificate, error)
	if cfg.usesSelfSigned() {
		dir := filepath.Join(s.config.DataDir, "tls")
		hosts := []string{s.config.ServerAddr}
		load = func() (tls.Certificate, error) {
			cert, created, err := tlscert.LoadOrCreateSelfSigned(dir, hosts)
			if created {
				log.Printf("Admin TLS: generated a self-signed certificate in %s", dir)
			}
			return cert, err
		}
		s.adminTLSMode = "self-signed"
	} else {
		certFile, keyFile := cfg.CertFile, cfg.KeyFile
		load = func() (tls.Certificate, error) { return tlscert.Load(certFile, keyFile) }
		s.adminTLSMode = "certificate"
	}
	reloader, err := tlscert.NewReloader(load)
	if err != nil {
		return err
	}
	s.adminTLS = reloader
	log.Printf("Admin TLS: enabled (%s), SHA-256 fingerprint %s", s.adminTLSMode, tlscert.Fingerprint(reloader.Certificate()))
	return nil
}

func (s *Server) ReloadTLS() error {
	if s.adminTLS == nil {
		return nil
	}
	if err := s.adminTLS.Reload(); err != nil {
		return err
	}
	log.Printf("Admin TLS: certificate reloaded, SHA-256 fingerprint %s", tlscert.Fingerprint(s.adminTLS.Certificate()))
	return nil
}

func (s *Server) AdminTLSStatus() string {
	if s.adminTLS == nil {
		return "Disabled (plain HTTP)"
	}
	return fmt.Sprintf("Enabled (%s, SHA-256 %s)", s.adminTLSMode, tlscert.Fingerprint(s.adminTLS.Certificate()))
}

func (s *Server) AdminTLSEnabled() bool {
	return s.adminTLS != nil
}

func httpsRedirectHandler(httpsPort int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		target := url.URL{Scheme: "https", Host: net.JoinHostPort(host, strconv.Itoa(httpsPort)), Path: r.URL.Path, RawQuery: r.URL.RawQuery}
		http.Redirect(w, r, target.String(), http.StatusMovedPermanently)
	})
}

func (s *Server) startAdminRedirectServer() {
	port := s.config.AdminTLS.RedirectPort
	if s.adminTLS == nil || port == 0 {
		return
	}
	addr := fmt.Sprintf(":%d", port)
	s.adminRedirectServer = &http.Server{Addr: addr, Handler: httpsRedirectHandler(s.config.AdminPort)}
	ln, err := netbind.Listen(s.config.BindInterface, "tcp", addr)
	if err != nil {
		log.Printf("Admin TLS: redirect listener on port %d failed: %v", port, err)
		return
	}
	log.Printf("Admin TLS: redirecting plain HTTP on port %d to HTTPS", port)
	go func() {
		if err := s.adminRedirectServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("Admin TLS: redirect server error: %v", err)
		}
	}()
}

func (s *Server) startAdminServer(listener net.Listener) error {
	defer listener.Close()
	if s.adminTLS != nil {
		log.Printf("Starting Admin server on port %d (HTTPS)...", s.config.AdminPort)
	} else {
		log.Printf("Starting Admin server on port %d...", s.config.AdminPort)
	}

	mux := http.NewServeMux()

	s.setupAdminInterface(mux)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK\n")
	})

	mux.Handle("/metrics", promhttp.Handler())
	go s.refreshMetricsGauges()

	addr := fmt.Sprintf(":%d", s.config.AdminPort)
	s.adminServer = &http.Server{
		Addr:    addr,
		Handler: panicRecoveryMiddleware(mux),
	}

	if s.adminTLS != nil {
		s.adminServer.TLSConfig = s.adminTLS.Config()
		s.startAdminRedirectServer()
		if err := s.adminServer.ServeTLS(listener, "", ""); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("Admin server failed: %w", err)
		}
		return nil
	}
	if err := s.adminServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("Admin server failed: %w", err)
	}

	return nil
}

func (s *Server) setupAdminInterface(mux *http.ServeMux) {
	log.Println("Setting up admin interface")

	adminHandler := admin.NewHandler(s.config.Storage, s.config.DataDir, s.config.ISODir, s.config.BootDir, Version, s, s.toolsManager, s.config.WOLBroadcastAddr, s.config.ProfileManager, s.config.ProxyDHCPEnabled, s.config.HTTPPort, s.config.ServerAddr, s.config.WindowsSMBPort, s.smbManager, s.config.WindowsSMBEnabled, s.autoInstallLib)
	adminHandler.AdminTLSStatus = s.AdminTLSStatus
	adminHandler.BootAuthReady = s.AdminTLSEnabled
	mux.HandleFunc("/boot-auth", s.handleBootAuthentication)
	if s.scheduler != nil {
		adminHandler.SchedulerReload = s.scheduler.Reload
		adminHandler.SchedulerRunNow = s.scheduler.RunNow
	}
	adminHandler.WebhookNotifier = s.webhookNotifier
	adminHandler.Housekeeping = s.housekeeping
	adminHandler.DiskImaging = s.diskImages
	adminHandler.Catalogue = s.catalogue

	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		log.Printf("Failed to setup static files: %v", err)
		return
	}

	useAuth := s.config.Auth != nil
	if s.audit == nil {
		s.audit = &auditRecorder{store: s.config.Storage, notifier: s.webhookNotifier}
	}

	adminWrap := func(handler http.HandlerFunc) http.HandlerFunc {
		handler = s.audit.wrap(s.replicaGuard(handler))
		if useAuth {
			return s.config.Auth.AdminMiddleware(handler)
		}
		return handler
	}

	readWrap := func(handler http.HandlerFunc) http.HandlerFunc {
		handler = s.audit.wrap(s.replicaGuard(handler))
		if useAuth {
			return s.config.Auth.ReadMiddleware(handler)
		}
		return handler
	}

	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	})

	mux.HandleFunc("/api/openapi.json", adminHandler.OpenAPI(Version))

	mux.HandleFunc("/api/auth-info", func(w http.ResponseWriter, r *http.Request) {
		if s.config.Auth != nil {
			s.config.Auth.HandleAuthInfo(w, r)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success":true,"data":[{"id":"local","name":"Local"}]}`))
		}
	})

	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if s.config.Auth != nil {
			s.config.Auth.HandleLogin(w, r)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success":true,"data":{"token":"","username":"admin","is_admin":true}}`))
		}
	})

	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		if s.config.Auth != nil {
			s.config.Auth.HandleMe(w, r)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success":true,"data":{"username":"admin","is_admin":true,"role":"admin"}}`))
		}
	})

	mux.HandleFunc("/api/oidc/login", func(w http.ResponseWriter, r *http.Request) {
		if s.config.Auth != nil {
			s.config.Auth.HandleOIDCLogin(w, r)
		} else {
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("/api/oidc/callback", func(w http.ResponseWriter, r *http.Request) {
		if s.config.Auth != nil {
			s.config.Auth.HandleOIDCCallback(w, r)
		} else {
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("/api/server-info", readWrap(adminHandler.GetServerInfo))
	mux.HandleFunc("/api/stats", readWrap(adminHandler.GetStats))
	mux.HandleFunc("/api/logs", readWrap(adminHandler.GetBootLogs))
	mux.HandleFunc("/api/scan", adminWrap(adminHandler.ScanImages))
	mux.HandleFunc("/api/images/upload", adminWrap(adminHandler.UploadImage))
	mux.HandleFunc("/api/assign-images", adminWrap(adminHandler.AssignImages))

	mux.HandleFunc("/api/clients", readWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			id := r.URL.Query().Get("id")
			mac := r.URL.Query().Get("mac")
			if id != "" || mac != "" {
				adminHandler.GetClient(w, r)
			} else {
				adminHandler.ListClients(w, r)
			}
		case http.MethodPost:
			adminHandler.CreateClient(w, r)
		case http.MethodPut:
			adminHandler.UpdateClient(w, r)
		case http.MethodDelete:
			adminHandler.DeleteClient(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/images", readWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			filename := r.URL.Query().Get("filename")
			if filename != "" {
				adminHandler.GetImage(w, r)
			} else {
				adminHandler.ListImages(w, r)
			}
		case http.MethodPut:
			adminHandler.UpdateImage(w, r)
		case http.MethodDelete:
			adminHandler.DeleteImage(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/clusters", readWrap(adminHandler.Clusters))
	mux.HandleFunc("/api/clusters/update", adminWrap(adminHandler.UpdateCluster))
	mux.HandleFunc("/api/clusters/delete", adminWrap(adminHandler.DeleteCluster))
	mux.HandleFunc("/api/clusters/config", adminWrap(adminHandler.ClusterConfig))
	mux.HandleFunc("/api/clusters/config/template", adminWrap(adminHandler.ConfigTemplate))
	mux.HandleFunc("/api/clusters/nodes", readWrap(adminHandler.ClusterNodes))
	mux.HandleFunc("/api/node-images", readWrap(adminHandler.NodeImages))
	mux.HandleFunc("/api/node-images/delete", adminWrap(adminHandler.DeleteNodeImage))
	mux.HandleFunc("/api/node-images/download", adminWrap(adminHandler.DownloadNodeImage))
	mux.HandleFunc("/api/kubernetes/providers", readWrap(adminHandler.KubernetesProviders))
	mux.HandleFunc("/api/clusters/nodes/assign", adminWrap(adminHandler.AssignNode))
	mux.HandleFunc("/api/clusters/nodes/unassign", adminWrap(adminHandler.UnassignNode))
	mux.HandleFunc("/api/clusters/nodes/reinstall", adminWrap(adminHandler.ReinstallNode))
	mux.HandleFunc("/api/clusters/nodes/mark-installed", adminWrap(adminHandler.MarkNodeInstalled))

	mux.HandleFunc("/api/clients/wake", adminWrap(adminHandler.WakeClient))
	mux.HandleFunc("/api/clients/next-boot", adminWrap(adminHandler.SetNextBootImage))
	mux.HandleFunc("/api/clients/promote", adminWrap(adminHandler.PromoteClient))
	mux.HandleFunc("/api/clients/inventory", readWrap(adminHandler.GetClientInventory))
	mux.HandleFunc("/api/clients/inventory/history", readWrap(adminHandler.GetClientInventoryHistory))
	mux.HandleFunc("/api/clients/installs", readWrap(s.handleClientInstalls))
	mux.HandleFunc("/api/clients/timeline", readWrap(s.handleClientTimeline))
	mux.HandleFunc("/api/installs", readWrap(s.handleRecentInstalls))

	mux.HandleFunc("/api/bootloaders", readWrap(adminHandler.ListBootloaders))
	mux.HandleFunc("/api/bootloaders/create", adminWrap(adminHandler.CreateBootloaderSet))
	mux.HandleFunc("/api/bootloaders/upload", adminWrap(adminHandler.UploadBootloader))
	mux.HandleFunc("/api/bootloaders/delete", adminWrap(adminHandler.DeleteBootloader))
	mux.HandleFunc("/api/bootloaders/select", adminWrap(adminHandler.SelectBootloader))

	mux.HandleFunc("/api/tools", readWrap(adminHandler.ListTools))
	mux.HandleFunc("/api/tools/toggle", adminWrap(adminHandler.ToggleTool))
	mux.HandleFunc("/api/tools/download", adminWrap(adminHandler.DownloadTool))
	mux.HandleFunc("/api/tools/delete", adminWrap(adminHandler.DeleteTool))
	mux.HandleFunc("/api/tools/progress", readWrap(adminHandler.ToolProgress))
	mux.HandleFunc("/api/tools/url", adminWrap(adminHandler.UpdateToolURL))
	mux.HandleFunc("/api/tools/custom", adminWrap(adminHandler.CreateCustomTool))
	mux.HandleFunc("/api/tools/custom/delete", adminWrap(adminHandler.DeleteCustomTool))
	mux.HandleFunc("/api/tools/update", adminWrap(adminHandler.UpdateTools))

	mux.HandleFunc("/api/images/extract", adminWrap(adminHandler.ExtractImage))
	mux.HandleFunc("/api/images/extract-progress", readWrap(adminHandler.ExtractProgress))
	mux.HandleFunc("/api/images/redetect", adminWrap(adminHandler.RedetectImage))
	mux.HandleFunc("/api/images/boot-candidates", readWrap(adminHandler.BootFileCandidates))
	mux.HandleFunc("/api/images/patch-smb", adminWrap(adminHandler.PatchImageSMB))
	mux.HandleFunc("/api/images/unpatch-smb", adminWrap(adminHandler.UnpatchImageSMB))
	mux.HandleFunc("/api/autoinstall-files", adminWrap(adminHandler.ListAutoInstallFiles))
	mux.HandleFunc("/api/autoinstall-files/get", adminWrap(adminHandler.GetAutoInstallFile))
	mux.HandleFunc("/api/autoinstall-files/save", adminWrap(adminHandler.SaveAutoInstallFile))
	mux.HandleFunc("/api/autoinstall-files/upload", adminWrap(adminHandler.UploadAutoInstallFile))
	mux.HandleFunc("/api/autoinstall-files/download", adminWrap(adminHandler.DownloadAutoInstallFile))
	mux.HandleFunc("/api/autoinstall-files/delete", adminWrap(adminHandler.DeleteAutoInstallFile))
	mux.HandleFunc("/api/autoinstall-files/render", adminWrap(adminHandler.RenderAutoInstallFile))

	mux.HandleFunc("/api/profiles", readWrap(adminHandler.ListDistroProfiles))
	mux.HandleFunc("/api/profiles/save", adminWrap(adminHandler.SaveDistroProfile))
	mux.HandleFunc("/api/profiles/delete", adminWrap(adminHandler.DeleteDistroProfile))
	mux.HandleFunc("/api/profiles/update", adminWrap(adminHandler.UpdateDistroProfiles))
	mux.HandleFunc("/api/iso-catalog", readWrap(adminHandler.GetISOCatalogue))
	mux.HandleFunc("/api/images/boot-method", adminWrap(adminHandler.SetBootMethod))
	mux.HandleFunc("/api/images/winpe", adminWrap(adminHandler.SetWinPEWIM))

	mux.HandleFunc("/api/active-sessions", readWrap(s.handleActiveSessions))

	mux.HandleFunc("/api/logs/stream", adminWrap(s.handleLogsStream))
	mux.HandleFunc("/api/logs/buffer", adminWrap(s.handleLogsBuffer))
	mux.HandleFunc("/api/proxydhcp/events", readWrap(s.handleProxyDHCPEvents))
	mux.HandleFunc("/api/proxydhcp/stream", readWrap(s.handleProxyDHCPStream))
	mux.HandleFunc("/api/proxydhcp/events/clear", adminWrap(s.handleProxyDHCPEventsClear))

	selfService := func(handler func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if s.config.Auth == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"success":false,"error":"Authentication is disabled on this server"}`))
				return
			}
			handler(w, r)
		}
	}
	mux.HandleFunc("/api/me/totp/setup", selfService(func(w http.ResponseWriter, r *http.Request) { s.config.Auth.HandleTOTPSetup(w, r) }))
	mux.HandleFunc("/api/me/totp/enable", selfService(func(w http.ResponseWriter, r *http.Request) { s.config.Auth.HandleTOTPEnable(w, r) }))
	mux.HandleFunc("/api/me/totp/disable", selfService(func(w http.ResponseWriter, r *http.Request) { s.config.Auth.HandleTOTPDisable(w, r) }))

	mux.HandleFunc("/api/users", adminWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.ListUsers(w, r)
		case http.MethodPost:
			adminHandler.CreateUser(w, r)
		case http.MethodPut:
			adminHandler.UpdateUser(w, r)
		case http.MethodDelete:
			adminHandler.DeleteUser(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/users/reset-password", adminWrap(adminHandler.ResetUserPassword))
	mux.HandleFunc("/api/tokens", adminWrap(adminHandler.APITokens))
	mux.HandleFunc("/api/users/totp/reset", adminWrap(adminHandler.ResetUserTOTP))

	mux.HandleFunc("/api/images/download", adminWrap(adminHandler.DownloadISO))
	mux.HandleFunc("/api/images/verify", adminWrap(adminHandler.VerifyImage))
	mux.HandleFunc("/api/downloads", readWrap(adminHandler.ListDownloads))
	mux.HandleFunc("/api/downloads/progress", readWrap(adminHandler.GetDownloadProgress))

	mux.HandleFunc("/api/images/netboot/download", adminWrap(adminHandler.DownloadNetboot))

	mux.HandleFunc("/api/images/autoinstall", adminWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.GetAutoInstallScript(w, r)
		case http.MethodPut:
			adminHandler.UpdateAutoInstallScript(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/files", readWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Query().Get("id") != "" {
				adminHandler.GetCustomFile(w, r)
			} else {
				adminHandler.ListCustomFiles(w, r)
			}
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/files/upload", adminWrap(adminHandler.UploadCustomFile))
	mux.HandleFunc("/api/files/update", adminWrap(adminHandler.UpdateCustomFile))
	mux.HandleFunc("/api/files/delete", adminWrap(adminHandler.DeleteCustomFile))

	mux.HandleFunc("/api/drivers", readWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.ListDriverPacks(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/drivers/upload", adminWrap(adminHandler.UploadDriverPack))
	mux.HandleFunc("/api/drivers/delete", adminWrap(adminHandler.DeleteDriverPack))
	mux.HandleFunc("/api/drivers/rebuild", adminWrap(adminHandler.RebuildImageBootWim))

	mux.HandleFunc("/api/groups", readWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.ListImageGroups(w, r)
		case http.MethodPost:
			adminHandler.CreateImageGroup(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/groups/update", adminWrap(adminHandler.UpdateImageGroup))
	mux.HandleFunc("/api/groups/delete", adminWrap(adminHandler.DeleteImageGroup))

	mux.HandleFunc("/api/clients/import", adminWrap(adminHandler.ImportClientsCSV))
	mux.HandleFunc("/api/backup/export", adminWrap(adminHandler.ExportBackup))
	mux.HandleFunc("/api/audit", adminWrap(s.handleAuditList))
	mux.HandleFunc("/api/backup/import", adminWrap(adminHandler.ImportBackup))

	mux.HandleFunc("/api/webhook", adminWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.GetWebhookConfig(w, r)
		case http.MethodPut, http.MethodPost:
			adminHandler.UpdateWebhookConfig(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/webhook/test", adminWrap(adminHandler.TestWebhook))

	mux.HandleFunc("/api/sync/manifest", readWrap(adminHandler.SyncManifest))
	mux.HandleFunc("/api/sync/boot-verifiers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !useAuth {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		adminWrap(adminHandler.SyncBootVerifiers)(w, r)
	})
	mux.HandleFunc("/api/sync/tool-files", readWrap(adminHandler.SyncToolFiles))
	mux.HandleFunc("/api/sync/asset", readWrap(adminHandler.SyncAsset))
	mux.HandleFunc("/api/replica/status", readWrap(adminHandler.ReplicaStatus))
	mux.HandleFunc("/api/replica/sync", adminWrap(adminHandler.ReplicaSync))
	s.startReplica(adminHandler)

	mux.HandleFunc("/api/housekeeping", readWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.GetHousekeeping(w, r)
		case http.MethodPut, http.MethodPost:
			adminHandler.UpdateHousekeeping(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/housekeeping/run", adminWrap(adminHandler.RunHousekeeping))
	mux.HandleFunc("/api/catalogue/check", adminWrap(adminHandler.CheckCatalogue))

	mux.HandleFunc("/api/client-groups", readWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.ListClientGroups(w, r)
		case http.MethodPost:
			adminHandler.CreateClientGroup(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/client-groups/get", readWrap(adminHandler.GetClientGroup))
	mux.HandleFunc("/api/client-groups/update", adminWrap(adminHandler.UpdateClientGroup))
	mux.HandleFunc("/api/client-groups/delete", adminWrap(adminHandler.DeleteClientGroup))
	mux.HandleFunc("/api/client-groups/membership", adminWrap(adminHandler.SetClientGroupMembership))
	mux.HandleFunc("/api/client-groups/wake", adminWrap(adminHandler.WakeClientGroup))
	mux.HandleFunc("/api/client-groups/next-boot", adminWrap(adminHandler.SetNextBootForClientGroup))
	mux.HandleFunc("/api/client-groups/power", adminWrap(adminHandler.PowerClientGroup))

	mux.HandleFunc("/api/scheduled-tasks", readWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.ListScheduledTasks(w, r)
		case http.MethodPost:
			adminHandler.CreateScheduledTask(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/scheduled-tasks/update", adminWrap(adminHandler.UpdateScheduledTask))
	mux.HandleFunc("/api/scheduled-tasks/delete", adminWrap(adminHandler.DeleteScheduledTask))
	mux.HandleFunc("/api/scheduled-tasks/run", adminWrap(adminHandler.RunScheduledTask))

	mux.HandleFunc("/api/clients/power", adminWrap(adminHandler.PowerClient))
	mux.HandleFunc("/api/clients/power/status", readWrap(adminHandler.PowerStatusClient))
	mux.HandleFunc("/api/clients/menu-preview", readWrap(s.handleMenuPreview))
	mux.HandleFunc("/api/disk-images", readWrap(adminHandler.DiskImages))
	mux.HandleFunc("/api/disk-images/capture", adminWrap(adminHandler.CaptureDisk))
	mux.HandleFunc("/api/disk-images/deploy", adminWrap(adminHandler.DeployDiskImage))
	mux.HandleFunc("/api/disk-images/jobs", readWrap(adminHandler.DiskJobs))
	mux.HandleFunc("/api/clients/console", s.handleClientConsole)
	mux.HandleFunc("/api/san-targets", readWrap(adminHandler.SANTargets))
	mux.HandleFunc("/api/deployment-profiles", readWrap(adminHandler.DeploymentProfiles))
	mux.HandleFunc("/api/clients/effective", readWrap(s.handleClientEffective))
	mux.HandleFunc("/api/enrolment-rules", readWrap(adminHandler.EnrolmentRules))
	mux.HandleFunc("/api/enrolment-rules/test", adminWrap(s.handleEnrolmentRuleTest))
	mux.HandleFunc("/api/enrolment-rules/apply", adminWrap(s.handleEnrolmentRulesApply))

	mux.HandleFunc("/api/theme", readWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.GetMenuTheme(w, r)
		case http.MethodPut:
			adminHandler.UpdateMenuTheme(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/usb", readWrap(adminHandler.ListUSBImages))
	mux.HandleFunc("/api/usb/download", readWrap(adminHandler.DownloadUSBImage))

	mux.HandleFunc("/api/images/files", readWrap(adminHandler.ListImageFiles))
	mux.HandleFunc("/api/images/files/delete", adminWrap(adminHandler.DeleteImageFile))
}

func (s *Server) handleActiveSessions(w http.ResponseWriter, r *http.Request) {
	sessions := s.activeSessions.GetAll()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sessions); err != nil {
		log.Printf("Failed to encode active sessions: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) handleLogsBuffer(w http.ResponseWriter, r *http.Request) {
	logs := s.logBroadcaster.GetLogs()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"logs":    logs,
	}); err != nil {
		log.Printf("Failed to encode logs: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) handleLogsStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	logChan := s.logBroadcaster.Subscribe()
	defer s.logBroadcaster.Unsubscribe(logChan)

	ctx := r.Context()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-logChan:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: {\"type\":\"log\",\"message\":%q}\n\n", msg)
			flusher.Flush()
		}
	}
}

func (s *Server) handleAutoexec(w http.ResponseWriter, r *http.Request) {
	macAddress := r.URL.Query().Get("mac")
	if macAddress == "" {
		macAddress = "${net0/mac}"
	}

	log.Printf("autoexec.ipxe requested, chaining to inventory then menu.ipxe")

	script := fmt.Sprintf(`#!ipxe
dhcp
chain http://%s:%d/inventory?mac=%s&cpu=${cpuid/0}&memsize=${memsize}&platform=${platform}&buildarch=${buildarch}&product=${product}&manufacturer=${manufacturer}&serial=${serial}&asset=${asset}&uuid=${uuid}&nic_chip=${net0/chip} || chain http://%s:%d/menu.ipxe?mac=%s
%s%s`, s.config.ServerAddr, s.config.HTTPPort, macAddress, s.config.ServerAddr, s.config.HTTPPort, macAddress, ipxeLocalBootCheck, ipxeLocalBootTail)

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(script))
}

func (s *Server) executeScheduledTask(ctx context.Context, t *models.ScheduledTask) (string, string) {
	if s.config.Storage == nil {
		return "failed", "storage unavailable"
	}
	members, err := s.config.Storage.ListClientsInGroup(t.ClientGroupID)
	if err != nil {
		return "failed", err.Error()
	}
	group, _ := s.config.Storage.GetClientGroup(t.ClientGroupID)

	switch t.ActionType {
	case "wake":
		broadcast := s.config.WOLBroadcastAddr
		if group != nil && group.WOLBroadcastAddr != "" {
			broadcast = group.WOLBroadcastAddr
		}
		sent := 0
		stagger := time.Duration(0)
		if group != nil {
			stagger = time.Duration(group.StaggerDelayMillis) * time.Millisecond
		}
		for i, c := range members {
			if !c.Enabled {
				continue
			}
			if i > 0 && stagger > 0 {
				time.Sleep(stagger)
			}
			if err := wol.SendMagicPacket(c.MACAddress, broadcast); err != nil {
				log.Printf("scheduler wake: %s failed: %v", c.MACAddress, err)
				continue
			}
			sent++
		}
		return "ok", fmt.Sprintf("woke %d/%d", sent, len(members))

	case "next-boot":
		image := t.ActionParam
		applied := 0
		for _, c := range members {
			if !c.Enabled {
				continue
			}
			if err := s.config.Storage.SetNextBootImage(c.MACAddress, image); err != nil {
				continue
			}
			applied++
		}
		return "ok", fmt.Sprintf("set on %d/%d", applied, len(members))

	case "next-boot-clear":
		cleared := 0
		for _, c := range members {
			if err := s.config.Storage.ClearNextBootImage(c.MACAddress); err == nil {
				cleared++
			}
		}
		return "ok", fmt.Sprintf("cleared %d/%d", cleared, len(members))

	case "power":
		action := t.ActionParam
		if action == "" {
			return "failed", "power action requires action_param (On/ForceOff/ForceRestart/etc)"
		}
		dispatched := 0
		stagger := time.Duration(0)
		if group != nil {
			stagger = time.Duration(group.StaggerDelayMillis) * time.Millisecond
		}
		for _, c := range members {
			if !c.Enabled {
				continue
			}
			target, ok := bmc.Resolve(c, group)
			if !ok {
				continue
			}
			ctrl, err := bmc.New(target)
			if err != nil {
				log.Printf("scheduler power %s on %s skipped: %v", action, c.MACAddress, err)
				continue
			}
			dispatched++
			go func(mac string, ctrl bmc.Controller) {
				rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				if err := ctrl.SetPower(rctx, bmc.Action(action)); err != nil {
					log.Printf("scheduler power %s on %s failed: %v", action, mac, err)
				}
			}(c.MACAddress, ctrl)
			if stagger > 0 {
				time.Sleep(stagger)
			}
		}
		return "ok", fmt.Sprintf("dispatched to %d/%d", dispatched, len(members))

	default:
		return "failed", "unknown action_type: " + t.ActionType
	}
}

func (s *Server) refreshMetricsGauges() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if s.config.Storage != nil {
			if stats, err := s.config.Storage.GetStats(); err == nil {
				if n, ok := stats["clients"]; ok {
					metrics.ClientsTotal.Set(float64(n))
				}
				if n, ok := stats["images"]; ok {
					metrics.ImagesTotal.Set(float64(n))
				}
			}
		}
		metrics.ActiveSessions.Set(float64(len(s.activeSessions.GetAll())))
		<-ticker.C
	}
}

func (s *Server) recordBootIfNew(mac, path, remoteAddr string) {
	if s.config.Storage == nil || mac == "" || mac == "unknown" {
		return
	}
	slash := strings.Index(path, "/")
	if slash <= 0 {
		return
	}
	imageDir := path[:slash]

	key := mac + "|" + imageDir
	s.bootLogDedupMu.Lock()
	now := time.Now()
	if last, ok := s.bootLogDedup[key]; ok && now.Sub(last) < 30*time.Second {
		s.bootLogDedupMu.Unlock()
		return
	}
	s.bootLogDedup[key] = now
	if len(s.bootLogDedup) > 1024 {
		for k, t := range s.bootLogDedup {
			if now.Sub(t) > 10*time.Minute {
				delete(s.bootLogDedup, k)
			}
		}
	}
	s.bootLogDedupMu.Unlock()

	imageName := imageDir
	var bootImage *models.Image
	if images, err := s.config.Storage.ListImages(); err == nil {
		for _, img := range images {
			if strings.TrimSuffix(img.Filename, filepath.Ext(img.Filename)) == imageDir {
				imageName = img.Name
				bootImage = img
				break
			}
		}
	}
	metrics.BootAttempts.WithLabelValues(imageName).Inc()
	go func() {
		if err := s.config.Storage.LogBootAttempt(mac, imageName, remoteAddr, true, ""); err != nil {
			log.Printf("Boot log: failed to write for %s: %v", mac, err)
		}
		s.config.Storage.UpdateClientBootStats(mac)
		s.config.Storage.UpdateImageBootStats(imageName)
	}()
	clientName := ""
	var bootClient *models.Client
	if c, err := s.config.Storage.GetClient(mac); err == nil {
		clientName = c.Name
		bootClient = c
	}
	if bootImage != nil {
		s.noteImageBoot(mac, bootImage, bootClient)
	}
	ip := remoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	s.webhookNotifier.Fire(webhook.Event{
		Event:      webhook.EventBootStarted,
		MAC:        mac,
		ClientName: clientName,
		Image:      imageName,
		IP:         ip,
	})
}

func (s *Server) handleInventoryReport(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	mac := strings.ToLower(strings.ReplaceAll(r.FormValue("mac"), "-", ":"))
	if mac == "" || mac == "${net0/mac}" {
		http.Redirect(w, r, fmt.Sprintf("/menu.ipxe?mac=unknown"), http.StatusTemporaryRedirect)
		return
	}

	var memBytes int64
	if ms := r.FormValue("memsize"); ms != "" {
		if strings.HasPrefix(ms, "0x") || strings.HasPrefix(ms, "0X") {
			fmt.Sscanf(ms, "%v", &memBytes)
		} else {
			fmt.Sscanf(ms, "%d", &memBytes)
		}
	}

	inv := &models.HardwareInventory{
		MACAddress:   mac,
		IPAddress:    r.RemoteAddr,
		CPU:          r.FormValue("cpu"),
		Memory:       memBytes,
		Platform:     r.FormValue("platform"),
		BuildArch:    r.FormValue("buildarch"),
		Product:      r.FormValue("product"),
		Manufacturer: r.FormValue("manufacturer"),
		Serial:       r.FormValue("serial"),
		Asset:        r.FormValue("asset"),
		UUID:         r.FormValue("uuid"),
		NICChip:      r.FormValue("nic_chip"),
	}

	isNewClient := false
	if s.config.Storage != nil {
		if _, err := s.config.Storage.GetClient(mac); err != nil {
			isNewClient = true
		}
	}

	if s.config.Storage != nil {
		if err := s.config.Storage.SaveHardwareInventory(inv); err != nil {
			log.Printf("Inventory: Failed to save for %s: %v", mac, err)
		} else {
			log.Printf("Inventory: Saved hardware info for %s (product: %s, manufacturer: %s, memory: %d)", mac, inv.Product, inv.Manufacturer, inv.Memory)
		}
	}

	clientName := ""
	if c, err := s.config.Storage.GetClient(mac); err == nil {
		s.enrolClient(c, inv, r.RemoteAddr)
		clientName = c.Name
	}
	ip := r.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	if isNewClient {
		s.webhookNotifier.Fire(webhook.Event{
			Event:      webhook.EventClientDiscovered,
			MAC:        mac,
			ClientName: clientName,
			IP:         ip,
			Metadata: map[string]string{
				"product":      inv.Product,
				"manufacturer": inv.Manufacturer,
			},
		})
	} else {
		s.webhookNotifier.Fire(webhook.Event{
			Event:      webhook.EventInventoryUpdated,
			MAC:        mac,
			ClientName: clientName,
			IP:         ip,
		})
	}

	script := fmt.Sprintf("#!ipxe\nchain http://%s:%d/menu.ipxe?mac=%s\n", s.config.ServerAddr, s.config.HTTPPort, mac)
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(script))
}

type bootMenuRender struct {
	MAC           string `json:"mac"`
	KnownClient   bool   `json:"known_client"`
	ClientName    string `json:"client_name,omitempty"`
	NextBootImage string `json:"next_boot_image,omitempty"`
	DiskJob       string `json:"disk_job,omitempty"`
	Profile       string `json:"profile,omitempty"`
	Kind          string `json:"kind"`
	Script        string `json:"script"`
}

func normaliseMenuMAC(macAddress string) string {
	if macAddress == "" {
		macAddress = "unknown"
	}
	return strings.ToLower(strings.ReplaceAll(macAddress, "-", ":"))
}

func (s *Server) renderBootMenu(macAddress, remoteAddr string, commit bool) bootMenuRender {
	return s.renderBootMenuWithAccess(macAddress, remoteAddr, commit, bootMenuAccess{})
}

func (s *Server) renderBootMenuWithAccess(macAddress, remoteAddr string, commit bool, access bootMenuAccess) bootMenuRender {
	result := bootMenuRender{MAC: macAddress, Kind: "menu"}
	if s.config.Storage != nil {
		theme, err := s.config.Storage.GetMenuTheme()
		if err != nil {
			result.Kind, result.Script = "unavailable", bootProtectionUnavailable()
			return result
		}
		if (theme.BootPasswordRequired || theme.BootPasswordHash != "") && !access.global {
			if theme.BootPasswordHash == "" {
				result.Kind, result.Script = "unavailable", bootProtectionUnavailable()
				return result
			}
			result.Kind, result.Script = "locked", s.bootPasswordMenu(macAddress)
			return result
		}
	}
	var profileClient *models.Client
	var profileGroup *models.ClientGroup
	var eff deployment.Effective
	if s.config.Storage != nil {
		if c, err := s.config.Storage.GetClient(macAddress); err == nil {
			profileClient = c
			eff, profileGroup = s.effectiveFor(c)
			if p := eff.ActiveProfile(); p != nil {
				result.Profile = p.Name
			}
			if commit {
				s.ensureFirstBootDeploy(c, eff)
			}
		}
	}
	if s.diskImages != nil {
		if job := s.diskImages.ActiveJob(macAddress); job != nil && job.DiskImage != nil {
			result.Kind = "imaging"
			result.DiskJob = fmt.Sprintf("%s %s (disk %s)", job.Kind, job.DiskImage.Name, job.Disk)
			if commit {
				s.logAndBroadcast("Client %s: disk imaging job %d pending - booting Clonezilla to %s %s", macAddress, job.ID, job.Kind, job.DiskImage.Name)
			}
		}
	}

	var nextBootImageID uint
	var nextBootSANID uint
	if s.config.Storage != nil {
		client, err := s.config.Storage.GetClient(macAddress)
		if err == nil {
			result.KnownClient = true
			result.ClientName = client.Name
			switch client.EnrollmentState {
			case models.EnrollmentStateApproved:
				if script, ok := s.provisionInstallScriptFor(client); ok {
					if commit {
						s.logAndBroadcast("Client %s: approved cluster node - serving install boot", macAddress)
					}
					result.Kind = "install"
					result.Script = script
					return result
				}
			case models.EnrollmentStateInstalling:
				if !commit {
					break
				}
				if stateErr := s.config.Storage.SetClientEnrollmentState(macAddress, models.EnrollmentStateInstalled); stateErr == nil {
					s.logAndBroadcast("Client %s: install complete - node will default to local boot", macAddress)
					s.webhookNotifier.Fire(webhook.Event{
						Event:      webhook.EventInstallCompleted,
						MAC:        macAddress,
						ClientName: client.Name,
						IP:         remoteAddr,
					})
				}
			}
		}
		if err == nil && client.NextBootImage != "" {
			if sanID, ok := models.ParseNextBootSAN(client.NextBootImage); ok {
				if target, tErr := s.config.Storage.GetSANTarget(sanID); tErr == nil {
					if commit {
						s.logAndBroadcast("Client %s: next boot action set - booting from SAN target %s", macAddress, target.Name)
					}
					nextBootSANID = sanID
					result.NextBootImage = "SAN: " + target.Name
				}
				if commit {
					s.config.Storage.ClearNextBootImage(macAddress)
				}
			} else {
				img, imgErr := s.config.Storage.GetImage(client.NextBootImage)
				if imgErr == nil && img.Enabled {
					if commit {
						s.logAndBroadcast("Client %s: next boot action set - pre-selecting %s", macAddress, img.Name)
					}
					nextBootImageID = img.ID
					result.NextBootImage = img.Name
				}
				if commit && (imgErr != nil || img == nil || (!img.BootPasswordRequired && img.BootPasswordHash == "")) {
					s.config.Storage.ClearNextBootImage(macAddress)
				}
			}
		}
	}

	var images []models.Image
	var err error

	if s.config.Storage != nil {
		images, err = s.config.Storage.GetImagesForClient(macAddress)
		if err != nil {
			log.Printf("Failed to get images from database: %v", err)
			result.Kind, result.Script = "unavailable", bootProtectionUnavailable()
			return result
		}
	} else {
		isos, _ := s.scanISOs()
		images = convertISOsToImages(isos)
	}

	images = s.applyProfileImages(images, profileClient, profileGroup, eff)
	result.Script = s.generateIPXEMenuWithAccess(images, macAddress, nextBootImageID, nextBootSANID, access)
	return result
}

func (s *Server) handleIPXEMenu(w http.ResponseWriter, r *http.Request) {
	macAddress := normaliseMenuMAC(r.URL.Query().Get("mac"))

	s.logAndBroadcast("Client Connected: MAC %s (IP: %s) requesting boot menu", macAddress, r.RemoteAddr)

	rendered := s.renderBootMenu(macAddress, r.RemoteAddr, true)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(rendered.Script))
}

func (s *Server) handleMenuPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	macAddress := normaliseMenuMAC(r.URL.Query().Get("mac"))
	rendered := s.renderBootMenu(macAddress, r.RemoteAddr, false)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": rendered})
}

func (s *Server) handleListISOs(w http.ResponseWriter, r *http.Request) {
	macAddress := r.URL.Query().Get("mac")
	if macAddress == "" {
		macAddress = "unknown"
	}

	var images []models.Image
	var err error

	if s.config.Storage != nil {
		images, err = s.config.Storage.GetImagesForClient(macAddress)
		if err != nil {
			http.Error(w, "Failed to fetch images", http.StatusInternalServerError)
			return
		}
	} else {
		isos, _ := s.scanISOs()
		images = convertISOsToImages(isos)
	}

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "Available ISO images:\n")
	for _, img := range images {
		fmt.Fprintf(w, "  - %s (%s)\n", img.Name, formatBytes(img.Size))
	}
}

func (s *Server) handleCustomFile(w http.ResponseWriter, r *http.Request) {
	filename := strings.TrimPrefix(r.URL.Path, "/files/")
	if filename == "" {
		http.Error(w, "Missing filename in path", http.StatusBadRequest)
		return
	}

	decodedFilename, err := url.PathUnescape(filename)
	if err != nil {
		log.Printf("CustomFile: Failed to decode filename %s: %v", filename, err)
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	cleanFilename := filepath.Clean(decodedFilename)
	if cleanFilename == "." || cleanFilename == ".." || strings.Contains(cleanFilename, "..") {
		log.Printf("CustomFile: Path traversal attempt: %s from %s", decodedFilename, r.RemoteAddr)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var file *models.CustomFile
	if s.config.Storage != nil {
		file, err = s.config.Storage.GetCustomFileByFilename(cleanFilename)
		if err != nil || file == nil {
			log.Printf("CustomFile: File not found in database: %s", cleanFilename)
			http.NotFound(w, r)
			return
		}
	} else {
		log.Printf("CustomFile: No database configured")
		http.Error(w, "Custom files require database", http.StatusInternalServerError)
		return
	}

	var fullPath string
	if file.Public {
		fullPath = filepath.Join(s.config.DataDir, "files", cleanFilename)
	} else if file.ImageID != nil && file.Image != nil {
		imageName := strings.TrimSuffix(file.Image.Filename, filepath.Ext(file.Image.Filename))
		fullPath = filepath.Join(s.config.ISODir, imageName, "files", cleanFilename)
	} else {
		log.Printf("CustomFile: Invalid file configuration for %s", cleanFilename)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	cleanPath := filepath.Clean(fullPath)
	dataDir := filepath.Clean(s.config.DataDir)
	if !strings.HasPrefix(cleanPath, dataDir) {
		log.Printf("CustomFile: Path traversal attempt: %s from %s", cleanFilename, r.RemoteAddr)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		log.Printf("CustomFile: File not found on disk: %s", fullPath)
		http.NotFound(w, r)
		return
	}

	if fileInfo.IsDir() {
		log.Printf("CustomFile: Path is a directory: %s", fullPath)
		http.Error(w, "Not a file", http.StatusBadRequest)
		return
	}

	go func() {
		if s.config.Storage != nil {
			s.config.Storage.IncrementFileDownloadCount(file.ID)
		}
	}()

	contentType := file.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)

	log.Printf("CustomFile: Serving %s to %s (size: %d bytes, public: %v, image: %v)",
		cleanFilename, r.RemoteAddr, fileInfo.Size(), file.Public,
		func() string {
			if file.Image != nil {
				return file.Image.Name
			}
			return "none"
		}())
	http.ServeFile(w, r, fullPath)
}

func (s *Server) handleAutoInstallScript(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/autoinstall/")
	if path == "" {
		http.Error(w, "Missing image filename in path", http.StatusBadRequest)
		return
	}
	if s.config.Storage == nil {
		http.Error(w, "Auto-install requires database", http.StatusInternalServerError)
		return
	}

	doc := "user-data"
	mac := ""
	if idx := strings.LastIndex(path, "/mac/"); idx >= 0 {
		rest := strings.TrimSuffix(path[idx+len("/mac/"):], "/")
		path = path[:idx]
		parts := strings.SplitN(rest, "/", 2)
		mac = parts[0]
		if len(parts) == 2 && parts[1] != "" {
			doc = parts[1]
		}
	}
	if mac == "" {
		mac = r.URL.Query().Get("mac")
	}
	mac = autoinstall.NormaliseMAC(mac)

	var postedMACs []string
	if r.Method == http.MethodPost && mac == "" {
		macs, err := autoinstall.ProxmoxInterfaceMACs(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		postedMACs = macs
	}

	image, err := s.config.Storage.GetImage(path)
	if err != nil || image == nil {
		http.Error(w, "Image not found", http.StatusNotFound)
		return
	}

	var client *models.Client
	var group *models.ClientGroup
	if len(postedMACs) > 0 {
		mac, client = s.matchClientByMACs(postedMACs)
		group = s.clientGroupOf(client)
	} else if mac != "" {
		if c, err := s.config.Storage.GetClient(mac); err == nil {
			client = c
			group = s.clientGroupOf(client)
		}
	}

	switch doc {
	case "meta-data":
		instanceID := strings.ReplaceAll(mac, ":", "-")
		if instanceID == "" {
			instanceID = "unknown"
		}
		md := fmt.Sprintf("instance-id: iid-%s\n", instanceID)
		if client != nil && client.Name != "" {
			md += fmt.Sprintf("local-hostname: %s\n", client.Name)
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Write([]byte(md))
		return
	case "vendor-data":
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		return
	case "user-data":
	default:
		http.Error(w, "Unknown seed document", http.StatusNotFound)
		return
	}

	script, scriptType, source, err := s.resolveAutoInstallScript(image, client)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	clientIP := r.RemoteAddr
	if i := strings.LastIndex(clientIP, ":"); i > 0 {
		clientIP = clientIP[:i]
	}
	installToken := s.noteAutoInstallFetch(mac, image)
	vars := autoinstall.Context{
		MAC:        mac,
		ClientIP:   clientIP,
		ServerAddr: s.config.ServerAddr,
		ServerPort: s.config.HTTPPort,
		Image:      image,
		Client:     client,
		Group:      group,
	}.Variables()
	vars["CALLBACK_URL"] = s.installCallbackURL(installToken)
	vars["INSTALL_TOKEN"] = installToken
	script, unresolved := autoinstall.Render(script, vars)
	if len(unresolved) > 0 {
		log.Printf("auto-install script for %s has unresolved placeholders: %s", image.Filename, strings.Join(unresolved, ", "))
	}

	if image.Distro == "arch" {
		if files, _ := s.config.Storage.ListCustomFilesByImage(image.ID); len(files) > 0 {
			script = s.injectArchFileDownloads(script, files)
		}
	}

	w.Header().Set("Content-Type", autoinstall.ContentType(scriptType))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(script)))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))

	log.Printf("Served auto-install script for %s (source: %s, type: %s, size: %d bytes)",
		image.Filename, source, scriptType, len(script))
}

func (s *Server) clientGroupOf(client *models.Client) *models.ClientGroup {
	if client == nil || client.ClientGroupID == nil || s.config.Storage == nil {
		return nil
	}
	group, err := s.config.Storage.GetClientGroup(*client.ClientGroupID)
	if err != nil {
		return nil
	}
	return group
}

func (s *Server) resolveAutoInstallScript(image *models.Image, client *models.Client) (string, string, string, error) {
	eff, group := s.effectiveFor(client)
	return resolveAutoInstallWithProfiles(s.autoInstallLib, image, client, group, eff.AutoInstallCandidates())
}

func resolveAutoInstallSource(lib *autoinstall.Library, image *models.Image, client *models.Client, group *models.ClientGroup) (string, string, string, error) {
	return resolveAutoInstallWithProfiles(lib, image, client, group, nil)
}

func resolveAutoInstallWithProfiles(lib *autoinstall.Library, image *models.Image, client *models.Client, group *models.ClientGroup, profileFiles []deployment.Candidate) (string, string, string, error) {
	if lib != nil {
		tryFile := func(rel, src string) (string, string, string, error) {
			content, err := lib.ReadPath(rel)
			if err != nil {
				return "", "", "", err
			}
			return content, autoinstall.PathType(rel), src, nil
		}

		if client != nil && client.AutoInstallFile != "" {
			if c, t, src, err := tryFile(client.AutoInstallFile, "client:"+client.MACAddress); err == nil {
				return c, t, src, nil
			}
		}
		for _, candidate := range profileFiles {
			if c, t, src, err := tryFile(candidate.Path, candidate.Source); err == nil {
				return c, t, src, nil
			}
		}
		if group != nil && group.AutoInstallFile != "" {
			if c, t, src, err := tryFile(group.AutoInstallFile, "group:"+group.Name); err == nil {
				return c, t, src, nil
			}
		}
		if image.AutoInstallFile != "" {
			if c, t, src, err := tryFile(image.AutoInstallFile, "image:"+image.Filename); err == nil {
				return c, t, src, nil
			}
		}
	}

	if image.AutoInstallEnabled && image.AutoInstallScript != "" {
		t := image.AutoInstallScriptType
		if t == "" {
			t = autoinstall.TypeGeneric
		}
		return image.AutoInstallScript, t, "inline:" + image.Filename, nil
	}

	return "", "", "", fmt.Errorf("no auto-install configuration for this image/client")
}

func (s *Server) resolveAutoInstallType(image *models.Image, client *models.Client) string {
	_, scriptType, _, err := s.resolveAutoInstallScript(image, client)
	if err != nil {
		return ""
	}
	return scriptType
}

func (s *Server) matchClientByMACs(macs []string) (string, *models.Client) {
	for _, mac := range macs {
		if c, err := s.config.Storage.GetClient(mac); err == nil && c != nil {
			return mac, c
		}
	}
	return macs[0], nil
}

func (s *Server) injectArchFileDownloads(script string, files []*models.CustomFile) string {
	if len(files) == 0 {
		return script
	}

	serverIP := GetOutboundIP()
	serverPort := "8080"

	var downloadCommands strings.Builder
	downloadCommands.WriteString("\n\n# Download custom files from Bootimus\n")

	for _, file := range files {
		destPath := file.DestinationPath
		if destPath == "" {
			destPath = "/root/" + file.Filename
		}

		destDir := filepath.Dir(destPath)
		if destDir != "/" && destDir != "." {
			downloadCommands.WriteString(fmt.Sprintf("arch-chroot /mnt mkdir -p %s\n", destDir))
		}

		downloadCommands.WriteString(fmt.Sprintf(
			"arch-chroot /mnt wget -q http://%s:%s/files/%s -O %s\n",
			serverIP, serverPort, file.Filename, destPath,
		))

		if strings.HasSuffix(file.Filename, ".sh") {
			downloadCommands.WriteString(fmt.Sprintf("arch-chroot /mnt chmod +x %s\n", destPath))
		}
	}

	return script + downloadCommands.String()
}

func convertISOsToImages(isos []ISOImage) []models.Image {
	images := make([]models.Image, len(isos))
	for i, iso := range isos {
		images[i] = models.Image{
			Name:     iso.Name,
			Filename: iso.Filename,
			Size:     iso.Size,
			Enabled:  true,
			Public:   true,
			ID:       uint(i + 1),
		}
	}
	return images
}

func GetOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

var replicaWritablePaths = map[string]bool{
	"/api/replica/sync":           true,
	"/api/users":                  true,
	"/api/users/reset-password":   true,
	"/api/users/totp/reset":       true,
	"/api/tokens":                 true,
	"/api/proxydhcp/events/clear": true,
}

func (s *Server) replicaGuard(next http.HandlerFunc) http.HandlerFunc {
	if !s.config.Replica.Enabled() {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if !replicaWritablePaths[r.URL.Path] && !strings.HasPrefix(r.URL.Path, "/api/me/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"error":   fmt.Sprintf("This instance is a replica of %s; make changes there", s.config.Replica.DisplayURL()),
					"replica": s.config.Replica.DisplayURL(),
				})
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) bootloaderSetExists(name string) bool {
	if name == "" {
		return true
	}
	if bootloaders.IsBuiltIn(name) {
		return true
	}
	info, err := os.Stat(filepath.Join(s.config.BootDir, name))
	return err == nil && info.IsDir()
}

func (s *Server) startReplica(adminHandler *admin.Handler) {
	if !s.config.Replica.Enabled() || s.config.Storage == nil {
		return
	}
	if s.replica == nil {
		s.replica = replica.New(*s.config.Replica, replica.Deps{
			Store:       s.config.Storage,
			DataDir:     s.config.DataDir,
			ISODir:      s.config.ISODir,
			AutoInstall: s.autoInstallLib,
			Extract:     adminHandler.ExtractImageByFilename,
			SelectBootloaderSet: func(name string) error {
				if s.GetActiveBootloaderSet() == name {
					return nil
				}
				s.SetActiveBootloaderSet(name)
				return s.SaveBootloaderConfig()
			},
			BootloaderSetExists: s.bootloaderSetExists,
			ToolDir:             s.toolsManager.ToolDir,
		})
		if s.config.Auth != nil {
			s.config.Auth.MeExtras = s.replicaMeExtras
		}
		s.replica.Start()
		log.Printf("Replica mode: syncing from %s every %s", s.config.Replica.DisplayURL(), s.config.Replica.Interval)
	}
	adminHandler.Replica = s.replica
}

func (s *Server) replicaMeExtras() map[string]interface{} {
	if s.replica == nil {
		return nil
	}
	status := s.replica.Status()
	return map[string]interface{}{
		"replica": map[string]interface{}{
			"primary_url": status.PrimaryURL,
			"last_sync":   status.LastSuccess,
			"last_run":    status.LastRun,
			"last_error":  status.LastError,
			"sync_isos":   status.SyncISOs,
		},
	}
}

func (s *Server) remoteImages(images []models.Image) (string, map[uint]bool) {
	if s.replica == nil {
		return "", nil
	}
	base := s.replica.BootBaseURL()
	if base == "" {
		return "", nil
	}
	remote := map[uint]bool{}
	for _, img := range images {
		if !s.replica.ImageIsLocal(img.Filename) {
			remote[img.ID] = true
		}
	}
	return base, remote
}
