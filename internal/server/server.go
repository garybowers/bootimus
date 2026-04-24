package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"bootimus/bootloaders"
	"bootimus/internal/admin"
	"bootimus/internal/auth"
	"bootimus/internal/metrics"
	"bootimus/internal/models"
	"bootimus/internal/nbd"
	"bootimus/internal/profiles"
	"bootimus/internal/proxydhcp"
	"bootimus/internal/redfish"
	"bootimus/internal/autoinstall"
	"bootimus/internal/scheduler"
	"bootimus/internal/smb"
	"bootimus/internal/storage"
	"bootimus/internal/tools"
	"bootimus/internal/webhook"
	"bootimus/internal/wol"
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
	TFTPPort          int
	TFTPSinglePort    bool
	HTTPPort          int
	AdminPort         int
	BootDir           string
	DataDir           string
	ISODir            string
	ServerAddr        string
	Storage           storage.Storage
	Auth              *auth.Manager
	NBDEnabled        bool
	NBDPort           int
	WOLBroadcastAddr  string
	ProfileManager    *profiles.Manager

	// ProxyDHCPEnabled turns on an in-process proxyDHCP server that answers
	// PXE boot requests without handing out IPs. Requires root or
	// CAP_NET_BIND_SERVICE to bind UDP/67. Off by default so bootimus
	// doesn't collide with an existing dnsmasq on the same network.
	ProxyDHCPEnabled      bool
	ProxyDHCPBootfileBIOS string
	ProxyDHCPBootfileUEFI string
	ProxyDHCPBootfileARM  string

	// WindowsSMBEnabled opts into running an embedded smbd that exposes
	// extracted Windows installation media as guest read-only shares so
	// WinPE can auto-mount them and launch setup.exe. Off by default;
	// requires smbd in PATH (samba package). A missing smbd is logged but
	// does not abort startup — the feature degrades to disabled.
	WindowsSMBEnabled bool
	WindowsSMBPort    int
}

type Server struct {
	config             *Config
	httpServer         *http.Server
	adminServer        *http.Server
	tftpServer         *tftp.Server
	proxyDHCPServer    *proxydhcp.Server
	webhookNotifier    *webhook.Notifier
	scheduler          *scheduler.Scheduler
	bootLogDedup       map[string]time.Time
	bootLogDedupMu     sync.Mutex
	wg                 sync.WaitGroup
	activeSessions     *ActiveSessions
	logBroadcaster     *LogBroadcaster
	activeBootloaderSet   string // name of active set folder, empty = built-in
	activeBootloaderSetMu sync.RWMutex
	toolsManager          *tools.Manager
	smbManager            *smb.Manager
	autoInstallLib        *autoinstall.Library
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
	if err := tm.SeedTools(); err != nil {
		log.Printf("Tools: Failed to seed tools: %v", err)
	}

	s := &Server{
		config: cfg,
		activeSessions: &ActiveSessions{
			sessions: make(map[string]*ActiveSession),
		},
		logBroadcaster:  lb,
		toolsManager:    tm,
		bootLogDedup:    make(map[string]time.Time),
		webhookNotifier: webhook.New(cfg.Storage),
	}
	s.scheduler = scheduler.New(cfg.Storage, s.executeScheduledTask)
	s.loadBootloaderConfig()
	return s
}

// Bootloader set config - stores which set folder is active
func (s *Server) bootloaderConfigPath() string {
	return filepath.Join(s.config.DataDir, "bootloader-config.json")
}

type bootloaderConfigFile struct {
	ActiveSet string `json:"active_set"` // folder name or "" for built-in
}

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
	defer s.activeBootloaderSetMu.Unlock()
	s.activeBootloaderSet = name
}

// resolveBootloaderFile checks the active set folder for a file, returns "" if not found
func (s *Server) resolveBootloaderFile(filename string) string {
	setName := s.GetActiveBootloaderSet()
	if setName == "" || s.config.BootDir == "" {
		return "" // use embedded
	}
	fullPath := filepath.Join(s.config.BootDir, setName, filename)
	if _, err := os.Stat(fullPath); err == nil {
		return fullPath
	}
	return "" // not in set, fall back to embedded
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
	log.Printf("Starting Bootimus - PXE/HTTP Boot Server")
	log.Printf("Boot directory: %s", s.config.BootDir)
	log.Printf("Data directory: %s", s.config.DataDir)
	log.Printf("ISO directory: %s", s.config.ISODir)
	log.Printf("TFTP Port: %d", s.config.TFTPPort)
	log.Printf("HTTP Port: %d", s.config.HTTPPort)
	log.Printf("Admin Port: %d", s.config.AdminPort)
	log.Printf("Server Address: %s", s.config.ServerAddr)

	if mgr, err := autoinstall.New(s.config.DataDir); err != nil {
		log.Printf("Warning: could not initialise autoinstall manager: %v", err)
	} else {
		s.autoInstallLib = mgr
		log.Printf("Auto-install files directory: %s", mgr.Root())
	}

	isos, err := s.scanISOs()
	if err != nil {
		log.Printf("Warning: Failed to scan ISOs: %v", err)
	} else {
		log.Printf("Found %d ISO image(s)", len(isos))
		for _, iso := range isos {
			log.Printf("  - %s (%s)", iso.Name, iso.SizeStr)
		}

		if s.config.Storage != nil {
			isoFiles := make([]models.SyncFile, len(isos))
			for i, iso := range isos {
				isoFiles[i] = models.SyncFile{
					Name:      iso.Name,
					Filename:  iso.Filename,
					Size:      iso.Size,
					GroupPath: iso.GroupPath,
				}
			}

			if err := s.config.Storage.SyncImages(isoFiles); err != nil {
				log.Printf("Warning: Failed to sync images with database: %v", err)
			}
		}
	}

	// Start SMB before the admin server so admin.NewHandler captures a live
	// *smb.Manager rather than a nil pointer. Shares for already-extracted
	// Windows images are populated into the manager's map *before* smbd
	// launches so the initial smb.conf already lists them — avoids a reload
	// race where SIGHUP/reload-config lands before smbd has installed its
	// signal/messaging handlers and kills the daemon.
	if s.config.WindowsSMBEnabled {
		mgr := smb.NewManager(s.config.DataDir, s.config.WindowsSMBPort)
		s.preloadSMBShares(mgr)
		if err := mgr.Start(); err != nil {
			log.Printf("Windows SMB: requested but could not start: %v - feature disabled for this run", err)
		} else {
			s.smbManager = mgr
		}
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.startTFTPServer(); err != nil {
			log.Printf("TFTP server error: %v", err)
		}
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.startHTTPServer(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.startAdminServer(); err != nil {
			log.Printf("Admin server error: %v", err)
		}
	}()

	// Periodically clean up stale active sessions
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
			nbdServer := nbd.NewServer(s.config.ISODir, s.config.NBDPort)
			if err := nbdServer.Start(); err != nil {
				log.Printf("NBD server error: %v", err)
			}
		}()
	}

	if s.scheduler != nil {
		s.scheduler.Start()
	}

	if s.config.ProxyDHCPEnabled {
		pd, err := proxydhcp.NewServer(proxydhcp.Config{
			ServerIP:      net.ParseIP(s.config.ServerAddr),
			BootfileBIOS:  s.config.ProxyDHCPBootfileBIOS,
			BootfileUEFI:  s.config.ProxyDHCPBootfileUEFI,
			BootfileARM64: s.config.ProxyDHCPBootfileARM,
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

// preloadSMBShares seeds the manager's in-memory share map with all
// already-extracted Windows images that have a successful SMB patch, so
// that the initial smb.conf written by Start() already lists them. Called
// *before* smbd launches — no reload/SIGHUP needed at startup.
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
		mgr.AddShare(smb.SanitizeShareName(isoBase), sharePath)
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

	if s.smbManager != nil {
		s.smbManager.Stop()
		log.Println("SMB server stopped")
	}

	log.Println("Shutdown complete")

	return nil
}

func (s *Server) scanISOs() ([]ISOImage, error) {
	var isos []ISOImage

	err := filepath.WalkDir(s.config.ISODir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".iso") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			log.Printf("Warning: Failed to get info for %s: %v", path, err)
			return nil
		}

		relPath, _ := filepath.Rel(s.config.ISODir, path)
		groupPath := filepath.Dir(relPath)
		if groupPath == "." {
			groupPath = ""
		}

		displayName := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))

		isos = append(isos, ISOImage{
			Name:      displayName,
			Filename:  relPath,
			Size:      info.Size(),
			SizeStr:   formatBytes(info.Size()),
			GroupPath: groupPath,
		})

		return nil
	})
	if err != nil {
		return nil, err
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

func (s *Server) startTFTPServer() error {
	log.Printf("Starting TFTP server on port %d...", s.config.TFTPPort)

	server := tftp.NewServer(
		func(filename string, rf io.ReaderFrom) error {
			cleanPath := filepath.Clean(filename)
			if filepath.IsAbs(cleanPath) {
				cleanPath = filepath.Base(cleanPath)
			}

			log.Printf("TFTP: Client requesting file: %s", filename)
			metrics.TFTPRequests.WithLabelValues(cleanPath).Inc()

			if cleanPath == "autoexec.ipxe" {
				serverAddr := "${next-server}"
				if s.config.ServerAddr != "" {
					serverAddr = s.config.ServerAddr
				}
				script := fmt.Sprintf(`#!ipxe

# Auto-detect server IP and chain to dynamic menu
dhcp
chain http://%s:%d/inventory?mac=${net0/mac}&cpu=${cpuid/0}&memsize=${memsize}&platform=${platform}&buildarch=${buildarch}&product=${product}&manufacturer=${manufacturer}&serial=${serial}&asset=${asset}&uuid=${uuid}&nic_chip=${net0/chip} || chain http://%s:%d/menu.ipxe?mac=${net0/mac} || goto failed

:failed
echo Failed to load boot menu
echo Server: ${next-server}
echo MAC: ${net0/mac}
echo Press any key to retry...
prompt
goto dhcp
`, serverAddr, s.config.HTTPPort, serverAddr, s.config.HTTPPort)
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

			// Check active bootloader set
			if customPath := s.resolveBootloaderFile(cleanPath); customPath != "" {
				file, err := os.Open(customPath)
				if err == nil {
					defer file.Close()
					log.Printf("TFTP: Serving from set '%s': %s", s.GetActiveBootloaderSet(), cleanPath)

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

			// Default: serve embedded bootloader
			data, err := bootloaders.Bootloaders.ReadFile(cleanPath)
			if err == nil {
				log.Printf("TFTP: Serving embedded bootloader: %s", cleanPath)

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
		},
		nil,
	)

	server.SetTimeout(5 * time.Second)
	if s.config.TFTPSinglePort {
		log.Print("Enabling single port mode for TFTP server")
		server.EnableSinglePort()
	}

	addr := fmt.Sprintf(":%d", s.config.TFTPPort)
	if err := server.ListenAndServe(addr); err != nil {
		return fmt.Errorf("TFTP server failed: %w", err)
	}

	return nil
}

func (s *Server) startHTTPServer() error {
	log.Printf("Starting HTTP server on port %d...", s.config.HTTPPort)

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("HTTP: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		cleanPath := strings.TrimPrefix(r.URL.Path, "/")
		if cleanPath == "" {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		// Check active bootloader set
		if customPath := s.resolveBootloaderFile(cleanPath); customPath != "" {
			log.Printf("HTTP: Serving from set '%s': %s", s.GetActiveBootloaderSet(), cleanPath)
			ext := filepath.Ext(cleanPath)
			if ext == ".efi" || ext == ".img" || ext == ".iso" || ext == ".kpxe" || ext == ".usb" {
				w.Header().Set("Content-Type", "application/octet-stream")
			}
			http.ServeFile(w, r, customPath)
			return
		}

		// Default: serve embedded bootloader
		data, err := bootloaders.Bootloaders.ReadFile(cleanPath)
		if err == nil {
			log.Printf("HTTP: Serving embedded bootloader: %s", cleanPath)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(data)
			return
		}

		http.Error(w, "Not found", http.StatusNotFound)
	})

	mux.HandleFunc("/inventory", s.handleInventoryReport)
	mux.HandleFunc("/menu.ipxe", s.handleIPXEMenu)

	// Serve tool files (GParted, Clonezilla, etc.)
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

		// Clean up session when handler exits (client disconnect or completion)
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

	mux.HandleFunc("/files/", s.handleCustomFile)

	mux.HandleFunc("/bootenv/", func(w http.ResponseWriter, r *http.Request) {
		urlPath := strings.TrimPrefix(r.URL.Path, "/bootenv/")
		filePath := filepath.Join("bootenv", urlPath)
		log.Printf("HTTP: Bootenv request - %s (always embedded)", urlPath)

		data, err := bootloaders.Bootloaders.ReadFile(filePath)
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

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server failed: %w", err)
	}

	return nil
}

func (s *Server) startAdminServer() error {
	log.Printf("Starting Admin server on port %d...", s.config.AdminPort)

	mux := http.NewServeMux()

	s.setupAdminInterface(mux)

	// Prometheus metrics endpoint — unauthenticated, consistent with how
	// most monitoring scrapers expect /metrics to behave. Bind to admin
	// port so it's not exposed on the public HTTP port.
	mux.Handle("/metrics", promhttp.Handler())
	go s.refreshMetricsGauges()

	addr := fmt.Sprintf(":%d", s.config.AdminPort)
	s.adminServer = &http.Server{
		Addr:    addr,
		Handler: panicRecoveryMiddleware(mux),
	}

	if err := s.adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("Admin server failed: %w", err)
	}

	return nil
}

func (s *Server) setupAdminInterface(mux *http.ServeMux) {
	log.Println("Setting up admin interface")

	adminHandler := admin.NewHandler(s.config.Storage, s.config.DataDir, s.config.ISODir, s.config.BootDir, Version, s, s.toolsManager, s.config.WOLBroadcastAddr, s.config.ProfileManager, s.config.ProxyDHCPEnabled, s.config.HTTPPort, s.config.ServerAddr, s.config.WindowsSMBPort, s.smbManager, s.config.WindowsSMBEnabled, s.autoInstallLib)
	if s.scheduler != nil {
		adminHandler.SchedulerReload = s.scheduler.Reload
		adminHandler.SchedulerRunNow = s.scheduler.RunNow
	}

	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		log.Printf("Failed to setup static files: %v", err)
		return
	}

	useAuth := s.config.Auth != nil

	authWrap := func(handler http.HandlerFunc) http.HandlerFunc {
		if useAuth {
			return s.config.Auth.JWTMiddleware(handler)
		}
		return handler
	}

	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// Logout - redirect to login page
	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	})

	// Auth info endpoint - returns available backends (no auth required)
	mux.HandleFunc("/api/auth-info", func(w http.ResponseWriter, r *http.Request) {
		if s.config.Auth != nil {
			s.config.Auth.HandleAuthInfo(w, r)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success":true,"data":[{"id":"local","name":"Local"}]}`))
		}
	})

	// Login endpoint - returns JWT token
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if s.config.Auth != nil {
			s.config.Auth.HandleLogin(w, r)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success":true,"data":{"token":"","username":"admin","is_admin":true}}`))
		}
	})

	mux.HandleFunc("/api/server-info", authWrap(adminHandler.GetServerInfo))
	mux.HandleFunc("/api/stats", authWrap(adminHandler.GetStats))
	mux.HandleFunc("/api/logs", authWrap(adminHandler.GetBootLogs))
	mux.HandleFunc("/api/scan", authWrap(adminHandler.ScanImages))
	mux.HandleFunc("/api/images/upload", authWrap(adminHandler.UploadImage))
	mux.HandleFunc("/api/assign-images", authWrap(adminHandler.AssignImages))

	mux.HandleFunc("/api/clients", authWrap(func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("/api/images", authWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			id := r.URL.Query().Get("id")
			if id != "" {
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

	mux.HandleFunc("/api/clients/wake", authWrap(adminHandler.WakeClient))
	mux.HandleFunc("/api/clients/next-boot", authWrap(adminHandler.SetNextBootImage))
	mux.HandleFunc("/api/clients/promote", authWrap(adminHandler.PromoteClient))
	mux.HandleFunc("/api/clients/inventory", authWrap(adminHandler.GetClientInventory))
	mux.HandleFunc("/api/clients/inventory/history", authWrap(adminHandler.GetClientInventoryHistory))

	mux.HandleFunc("/api/bootloaders", authWrap(adminHandler.ListBootloaders))
	mux.HandleFunc("/api/bootloaders/create", authWrap(adminHandler.CreateBootloaderSet))
	mux.HandleFunc("/api/bootloaders/upload", authWrap(adminHandler.UploadBootloader))
	mux.HandleFunc("/api/bootloaders/delete", authWrap(adminHandler.DeleteBootloader))
	mux.HandleFunc("/api/bootloaders/select", authWrap(adminHandler.SelectBootloader))

	// Tools
	mux.HandleFunc("/api/tools", authWrap(adminHandler.ListTools))
	mux.HandleFunc("/api/tools/toggle", authWrap(adminHandler.ToggleTool))
	mux.HandleFunc("/api/tools/download", authWrap(adminHandler.DownloadTool))
	mux.HandleFunc("/api/tools/delete", authWrap(adminHandler.DeleteTool))
	mux.HandleFunc("/api/tools/progress", authWrap(adminHandler.ToolProgress))
	mux.HandleFunc("/api/tools/url", authWrap(adminHandler.UpdateToolURL))
	mux.HandleFunc("/api/tools/custom", authWrap(adminHandler.CreateCustomTool))
	mux.HandleFunc("/api/tools/custom/delete", authWrap(adminHandler.DeleteCustomTool))
	mux.HandleFunc("/api/tools/update", authWrap(adminHandler.UpdateTools))

	mux.HandleFunc("/api/images/extract", authWrap(adminHandler.ExtractImage))
	mux.HandleFunc("/api/images/extract-progress", authWrap(adminHandler.ExtractProgress))
	mux.HandleFunc("/api/images/redetect", authWrap(adminHandler.RedetectImage))
	mux.HandleFunc("/api/images/patch-smb", authWrap(adminHandler.PatchImageSMB))
	mux.HandleFunc("/api/autoinstall-files", authWrap(adminHandler.ListAutoInstallFiles))
	mux.HandleFunc("/api/autoinstall-files/get", authWrap(adminHandler.GetAutoInstallFile))
	mux.HandleFunc("/api/autoinstall-files/save", authWrap(adminHandler.SaveAutoInstallFile))
	mux.HandleFunc("/api/autoinstall-files/upload", authWrap(adminHandler.UploadAutoInstallFile))
	mux.HandleFunc("/api/autoinstall-files/download", authWrap(adminHandler.DownloadAutoInstallFile))
	mux.HandleFunc("/api/autoinstall-files/delete", authWrap(adminHandler.DeleteAutoInstallFile))

	mux.HandleFunc("/api/profiles", authWrap(adminHandler.ListDistroProfiles))
	mux.HandleFunc("/api/profiles/save", authWrap(adminHandler.SaveDistroProfile))
	mux.HandleFunc("/api/profiles/delete", authWrap(adminHandler.DeleteDistroProfile))
	mux.HandleFunc("/api/profiles/update", authWrap(adminHandler.UpdateDistroProfiles))
	mux.HandleFunc("/api/images/boot-method", authWrap(adminHandler.SetBootMethod))

	mux.HandleFunc("/api/active-sessions", authWrap(s.handleActiveSessions))

	mux.HandleFunc("/api/logs/stream", authWrap(s.handleLogsStream))
	mux.HandleFunc("/api/logs/buffer", authWrap(s.handleLogsBuffer))

	mux.HandleFunc("/api/users", authWrap(func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/api/users/reset-password", authWrap(adminHandler.ResetUserPassword))

	mux.HandleFunc("/api/images/download", authWrap(adminHandler.DownloadISO))
	mux.HandleFunc("/api/downloads", authWrap(adminHandler.ListDownloads))
	mux.HandleFunc("/api/downloads/progress", authWrap(adminHandler.GetDownloadProgress))

	mux.HandleFunc("/api/images/netboot/download", authWrap(adminHandler.DownloadNetboot))

	mux.HandleFunc("/api/images/autoinstall", authWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.GetAutoInstallScript(w, r)
		case http.MethodPut:
			adminHandler.UpdateAutoInstallScript(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/files", authWrap(func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/api/files/upload", authWrap(adminHandler.UploadCustomFile))
	mux.HandleFunc("/api/files/update", authWrap(adminHandler.UpdateCustomFile))
	mux.HandleFunc("/api/files/delete", authWrap(adminHandler.DeleteCustomFile))

	// Driver pack routes
	mux.HandleFunc("/api/drivers", authWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.ListDriverPacks(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/drivers/upload", authWrap(adminHandler.UploadDriverPack))
	mux.HandleFunc("/api/drivers/delete", authWrap(adminHandler.DeleteDriverPack))
	mux.HandleFunc("/api/drivers/rebuild", authWrap(adminHandler.RebuildImageBootWim))

	mux.HandleFunc("/api/groups", authWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.ListImageGroups(w, r)
		case http.MethodPost:
			adminHandler.CreateImageGroup(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/groups/update", authWrap(adminHandler.UpdateImageGroup))
	mux.HandleFunc("/api/groups/delete", authWrap(adminHandler.DeleteImageGroup))

	mux.HandleFunc("/api/clients/import", authWrap(adminHandler.ImportClientsCSV))
	mux.HandleFunc("/api/backup/export", authWrap(adminHandler.ExportBackup))

	mux.HandleFunc("/api/webhook", authWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.GetWebhookConfig(w, r)
		case http.MethodPut, http.MethodPost:
			adminHandler.UpdateWebhookConfig(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/webhook/test", authWrap(adminHandler.TestWebhook))

	mux.HandleFunc("/api/client-groups", authWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.ListClientGroups(w, r)
		case http.MethodPost:
			adminHandler.CreateClientGroup(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/client-groups/get", authWrap(adminHandler.GetClientGroup))
	mux.HandleFunc("/api/client-groups/update", authWrap(adminHandler.UpdateClientGroup))
	mux.HandleFunc("/api/client-groups/delete", authWrap(adminHandler.DeleteClientGroup))
	mux.HandleFunc("/api/client-groups/membership", authWrap(adminHandler.SetClientGroupMembership))
	mux.HandleFunc("/api/client-groups/wake", authWrap(adminHandler.WakeClientGroup))
	mux.HandleFunc("/api/client-groups/next-boot", authWrap(adminHandler.SetNextBootForClientGroup))
	mux.HandleFunc("/api/client-groups/power", authWrap(adminHandler.PowerClientGroup))

	mux.HandleFunc("/api/scheduled-tasks", authWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.ListScheduledTasks(w, r)
		case http.MethodPost:
			adminHandler.CreateScheduledTask(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/scheduled-tasks/update", authWrap(adminHandler.UpdateScheduledTask))
	mux.HandleFunc("/api/scheduled-tasks/delete", authWrap(adminHandler.DeleteScheduledTask))
	mux.HandleFunc("/api/scheduled-tasks/run", authWrap(adminHandler.RunScheduledTask))

	mux.HandleFunc("/api/clients/power", authWrap(adminHandler.PowerClient))
	mux.HandleFunc("/api/clients/power/status", authWrap(adminHandler.PowerStatusClient))

	mux.HandleFunc("/api/theme", authWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.GetMenuTheme(w, r)
		case http.MethodPut:
			adminHandler.UpdateMenuTheme(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/usb", authWrap(adminHandler.ListUSBImages))
	mux.HandleFunc("/api/usb/download", authWrap(adminHandler.DownloadUSBImage))

	mux.HandleFunc("/api/images/files", authWrap(adminHandler.ListImageFiles))
	mux.HandleFunc("/api/images/files/delete", authWrap(adminHandler.DeleteImageFile))
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
`, s.config.ServerAddr, s.config.HTTPPort, macAddress, s.config.ServerAddr, s.config.HTTPPort, macAddress)

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(script))
}

// executeScheduledTask is the Runner bound to the scheduler package. It maps
// a ScheduledTask's action_type to the underlying bulk primitive and returns
// a status + error for the run-history row.
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
			host, port, user, pass, insecure := resolveRedfishForClient(c, group)
			if host == "" || user == "" || pass == "" {
				continue
			}
			dispatched++
			go func(mac, host string, port int, user, pass string, insecure bool) {
				rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				client := redfish.New(host, port, user, pass, insecure)
				if err := client.SetPower(rctx, redfish.PowerAction(action)); err != nil {
					log.Printf("scheduler power %s on %s failed: %v", action, mac, err)
				}
			}(c.MACAddress, host, port, user, pass, insecure)
			if stagger > 0 {
				time.Sleep(stagger)
			}
		}
		return "ok", fmt.Sprintf("dispatched to %d/%d", dispatched, len(members))

	default:
		return "failed", "unknown action_type: " + t.ActionType
	}
}

// resolveRedfishForClient merges per-client Redfish settings with its group
// defaults. Duplicates the same logic as the admin handler but kept here to
// avoid a circular import between server/admin.
func resolveRedfishForClient(c *models.Client, g *models.ClientGroup) (host string, port int, user string, pass string, insecure bool) {
	host = c.IPMIHost
	port = c.IPMIPort
	user = c.IPMIUsername
	pass = c.IPMIPassword
	insecure = c.IPMIInsecure
	if g != nil {
		if port == 0 {
			port = g.IPMIPort
		}
		if user == "" {
			user = g.IPMIUsername
		}
		if pass == "" {
			pass = g.IPMIPassword
		}
		if !insecure {
			insecure = g.IPMIInsecure
		}
	}
	return
}

// refreshMetricsGauges updates the "totals" gauges every 30s. Counters are
// incremented at event time elsewhere; gauges just mirror DB state.
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

// recordBootIfNew logs a boot attempt once per (MAC, image) within a short
// dedup window. A single client boot generates many /boot/ requests (kernel,
// initrd, squashfs, ucode, etc.) — we only want one BootLog row per boot.
func (s *Server) recordBootIfNew(mac, path, remoteAddr string) {
	if s.config.Storage == nil || mac == "" || mac == "unknown" {
		return
	}
	// First path segment is the image cache directory (filename minus .iso).
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
	// Occasional GC — keep the map from growing unbounded.
	if len(s.bootLogDedup) > 1024 {
		for k, t := range s.bootLogDedup {
			if now.Sub(t) > 10*time.Minute {
				delete(s.bootLogDedup, k)
			}
		}
	}
	s.bootLogDedupMu.Unlock()

	imageName := imageDir
	if img, err := s.config.Storage.GetImage(imageDir + ".iso"); err == nil {
		imageName = img.Name
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
	if c, err := s.config.Storage.GetClient(mac); err == nil {
		clientName = c.Name
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
		// No valid MAC — just redirect to menu
		http.Redirect(w, r, fmt.Sprintf("/menu.ipxe?mac=unknown"), http.StatusTemporaryRedirect)
		return
	}

	// Parse memory (iPXE memsize is in bytes, may be hex or decimal)
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

	// Detect whether this MAC is new so we can emit discovered vs updated.
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

	// Return iPXE script that chains to the boot menu
	script := fmt.Sprintf("#!ipxe\nchain http://%s:%d/menu.ipxe?mac=%s\n", s.config.ServerAddr, s.config.HTTPPort, mac)
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(script))
}

func (s *Server) handleIPXEMenu(w http.ResponseWriter, r *http.Request) {
	macAddress := r.URL.Query().Get("mac")
	if macAddress == "" {
		macAddress = "unknown"
	}

	macAddress = strings.ToLower(strings.ReplaceAll(macAddress, "-", ":"))

	s.logAndBroadcast("Client Connected: MAC %s (IP: %s) requesting boot menu", macAddress, r.RemoteAddr)

	// Check for one-time next boot image
	var nextBootImageID uint
	if s.config.Storage != nil {
		client, err := s.config.Storage.GetClient(macAddress)
		if err == nil && client.NextBootImage != "" {
			img, imgErr := s.config.Storage.GetImage(client.NextBootImage)
			if imgErr == nil && img.Enabled {
				s.logAndBroadcast("Client %s: next boot action set - pre-selecting %s", macAddress, img.Name)
				nextBootImageID = img.ID
				s.config.Storage.ClearNextBootImage(macAddress)
			} else {
				s.config.Storage.ClearNextBootImage(macAddress)
			}
		}
	}

	var images []models.Image
	var err error

	if s.config.Storage != nil {
		images, err = s.config.Storage.GetImagesForClient(macAddress)
		if err != nil {
			log.Printf("Failed to get images from database: %v", err)
			isos, _ := s.scanISOs()
			images = convertISOsToImages(isos)
		}
	} else {
		isos, _ := s.scanISOs()
		images = convertISOsToImages(isos)
	}

	menu := s.generateIPXEMenuWithGroups(images, macAddress, nextBootImageID)
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(menu))
}

func (s *Server) generateIPXEMenu(images []models.Image, macAddress string) string {
	tmpl := `#!ipxe

:start
menu Bootimus - Boot Menu
item --gap -- Available Images:
{{range $index, $img := .Images}}
item iso{{$index}} {{$img.Name}} ({{$img.SizeStr}}){{if $img.Extracted}} [kernel]{{end}}
{{end}}
item --gap -- Options:
item shell Drop to iPXE shell
item reboot Reboot
choose --default iso0 --timeout 30000 selected || goto start
goto ${selected}

{{range $index, $img := .Images}}
:iso{{$index}}
echo Booting {{$img.Name}}...
{{if eq $img.BootMethod "kernel"}}
echo Loading kernel and initrd...
{{if $img.AutoInstallEnabled}}
echo Auto-install enabled for this image
{{end}}
{{if eq $img.Distro "windows"}}
echo Loading Windows boot files via wimboot...
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/wimboot
initrd http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/bootmgfw.efi bootmgfw.efi
initrd http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/bootx64.efi bootx64.efi
initrd http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/bcd BCD
initrd http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/boot.sdi boot.sdi
initrd http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/boot.wim boot.wim
{{if $img.InstallWimPath}}initrd --name {{$img.InstallBasename}} http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/{{$img.InstallBasename}}
{{end}}boot || goto failed
{{else if eq $img.Distro "arch"}}
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz {{$img.AutoInstallParam}}{{$img.BootParams}}archiso_http_srv=http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/iso/ ip=dhcp
{{else if eq $img.Distro "nixos"}}
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz {{$img.AutoInstallParam}}{{$img.BootParams}} ip=dhcp
{{else if or (eq $img.Distro "fedora") (eq $img.Distro "centos")}}
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz {{$img.AutoInstallParam}}root=live:http://{{$.ServerAddr}}:{{$.HTTPPort}}/isos/{{$img.EncodedFilename}} rd.live.image inst.repo=http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/iso/ inst.stage2=http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/iso/ rd.neednet=1 ip=dhcp
{{else if eq $img.Distro "debian"}}
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz {{$img.AutoInstallParam}}{{$img.BootParams}} initrd=initrd ip=dhcp priority=critical
{{else if eq $img.Distro "ubuntu"}}
{{if $img.NetbootAvailable}}
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz {{$img.AutoInstallParam}}{{$img.BootParams}} initrd=initrd ip=dhcp
{{else}}
{{if $img.SquashfsPath}}
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz {{$img.AutoInstallParam}}{{$img.BootParams}} initrd=initrd ip=dhcp fetch=http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/{{$img.SquashfsPath}}
{{else}}
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz {{$img.AutoInstallParam}}{{$img.BootParams}} initrd=initrd ip=dhcp url=http://{{$.ServerAddr}}:{{$.HTTPPort}}/isos/{{$img.EncodedFilename}}
{{end}}
{{end}}
{{else if eq $img.Distro "freebsd"}}
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz vfs.root.mountfrom=cd9660:/dev/md0 kernelname=/boot/kernel/kernel
{{else}}
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz {{$img.AutoInstallParam}}{{$img.BootParams}}iso-url=http://{{$.ServerAddr}}:{{$.HTTPPort}}/isos/{{$img.EncodedFilename}} ip=dhcp
{{end}}
{{if ne $img.Distro "windows"}}
initrd http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/initrd
{{end}}
boot || goto failed
{{else}}
sanboot --no-describe --drive 0x80 http://{{$.ServerAddr}}:{{$.HTTPPort}}/isos/{{$img.EncodedFilename}}?mac={{$.MAC}}
{{end}}
goto start
{{end}}

:failed
echo Boot failed! Press any key to return to menu...
prompt
goto start

:shell
echo Type 'exit' to return to menu
shell
goto start

:reboot
reboot
`

	t, _ := template.New("menu").Parse(tmpl)

	type ImageData struct {
		Name               string
		Filename           string
		EncodedFilename    string
		SizeStr            string
		BootMethod         string
		Extracted          bool
		BootParams         string
		CacheDir           string
		Distro             string
		AutoInstallEnabled bool
		AutoInstallURL     string
		AutoInstallParam   string
		SquashfsPath       string
		NetbootAvailable   bool
		InstallWimPath     string
		InstallBasename    string
	}

	imageData := make([]ImageData, len(images))
	for i, img := range images {
		cacheDir := strings.TrimSuffix(img.Filename, filepath.Ext(img.Filename))

		autoInstallURL := ""
		autoInstallParam := ""
		if img.AutoInstallEnabled && img.AutoInstallScript != "" {
			// ${net0/mac} is expanded by iPXE before the kernel is booted, so the
			// resulting kernel cmdline carries the literal MAC and the autoinstall
			// endpoint can resolve the client for template variable substitution.
			autoInstallURL = fmt.Sprintf("http://%s:%d/autoinstall/%s?mac=${net0/mac}", s.config.ServerAddr, s.config.HTTPPort, url.PathEscape(img.Filename))

			switch img.AutoInstallScriptType {
			case "preseed":
				autoInstallParam = fmt.Sprintf("auto=true priority=critical url=%s ", autoInstallURL)
			case "kickstart":
				autoInstallParam = fmt.Sprintf("inst.ks=%s ", autoInstallURL)
			case "autoinstall":
				autoInstallParam = fmt.Sprintf("autoinstall ds=nocloud-net;s=%s/ ", autoInstallURL)
			case "autounattend":
				autoInstallParam = ""
			default:
				autoInstallParam = fmt.Sprintf("autoinstall=%s ", autoInstallURL)
			}
		}

		// Determine install image basename (install.wim or install.esd)
		installBasename := "install.wim"
		if img.InstallWimPath != "" && strings.Contains(strings.ToLower(img.InstallWimPath), ".esd") {
			installBasename = "install.esd"
		}

		imageData[i] = ImageData{
			Name:               img.Name,
			Filename:           img.Filename,
			EncodedFilename:    url.PathEscape(img.Filename),
			SizeStr:            formatBytes(img.Size),
			BootMethod:         img.BootMethod,
			Extracted:          img.Extracted,
			BootParams:         img.BootParams,
			CacheDir:           url.PathEscape(cacheDir),
			Distro:             img.Distro,
			AutoInstallEnabled: img.AutoInstallEnabled,
			AutoInstallURL:     autoInstallURL,
			AutoInstallParam:   autoInstallParam,
			SquashfsPath:       img.SquashfsPath,
			NetbootAvailable:   img.NetbootAvailable,
			InstallWimPath:     img.InstallWimPath,
			InstallBasename:    installBasename,
		}
	}

	data := struct {
		Images     []ImageData
		ServerAddr string
		HTTPPort   int
		MAC        string
	}{
		Images:     imageData,
		ServerAddr: s.config.ServerAddr,
		HTTPPort:   s.config.HTTPPort,
		MAC:        macAddress,
	}

	var buf bytes.Buffer
	t.Execute(&buf, data)
	return buf.String()
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

	image, err := s.config.Storage.GetImage(path)
	if err != nil || image == nil {
		http.Error(w, "Image not found", http.StatusNotFound)
		return
	}

	mac := strings.ToLower(strings.ReplaceAll(r.URL.Query().Get("mac"), "-", ":"))
	var client *models.Client
	if mac != "" {
		if c, err := s.config.Storage.GetClient(mac); err == nil {
			client = c
		}
	}

	script, scriptType, source, err := s.resolveAutoInstallScript(image, client)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	clientName := ""
	if client != nil {
		clientName = client.Name
	}
	clientIP := r.RemoteAddr
	if i := strings.LastIndex(clientIP, ":"); i > 0 {
		clientIP = clientIP[:i]
	}
	substitutions := map[string]string{
		"{{MAC}}":            mac,
		"{{CLIENT_NAME}}":    clientName,
		"{{HOSTNAME}}":       clientName,
		"{{IP}}":             clientIP,
		"{{SERVER_ADDR}}":    s.config.ServerAddr,
		"{{IMAGE_NAME}}":     image.Name,
		"{{IMAGE_FILENAME}}": image.Filename,
	}
	for k, v := range substitutions {
		script = strings.ReplaceAll(script, k, v)
	}

	if image.Distro == "arch" {
		if files, _ := s.config.Storage.ListCustomFilesByImage(image.ID); len(files) > 0 {
			script = s.injectArchFileDownloads(script, files)
		}
	}

	contentType := "text/plain; charset=utf-8"
	switch scriptType {
	case "autounattend":
		contentType = "application/xml; charset=utf-8"
	case "autoinstall":
		contentType = "text/yaml; charset=utf-8"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(script)))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))

	log.Printf("Served auto-install script for %s (source: %s, type: %s, size: %d bytes)",
		image.Filename, source, scriptType, len(script))
}

// resolveAutoInstallScript walks the hierarchy: per-client file > group file >
// image file > image inline legacy. Returns the script body, type, and a tag
// for logging.
func (s *Server) resolveAutoInstallScript(image *models.Image, client *models.Client) (string, string, string, error) {
	if s.autoInstallLib != nil {
		tryFile := func(rel, src string) (string, string, string, error) {
			content, err := s.autoInstallLib.ReadPath(rel)
			if err != nil {
				return "", "", "", err
			}
			return content, scriptTypeForPath(rel), src, nil
		}

		if client != nil && client.AutoInstallFile != "" {
			if c, t, src, err := tryFile(client.AutoInstallFile, "client:"+client.MACAddress); err == nil {
				return c, t, src, nil
			}
		}
		if client != nil && client.ClientGroupID != nil {
			if g, err := s.config.Storage.GetClientGroup(*client.ClientGroupID); err == nil && g.AutoInstallFile != "" {
				if c, t, src, err := tryFile(g.AutoInstallFile, "group:"+g.Name); err == nil {
					return c, t, src, nil
				}
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
			t = "generic"
		}
		return image.AutoInstallScript, t, "inline:" + image.Filename, nil
	}

	return "", "", "", fmt.Errorf("no auto-install configuration for this image/client")
}

func scriptTypeForPath(rel string) string {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".xml":
		return "autounattend"
	case ".cfg":
		return "preseed"
	case ".ks":
		return "kickstart"
	case ".yaml", ".yml":
		return "autoinstall"
	default:
		return "generic"
	}
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
