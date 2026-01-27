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
	"bootimus/internal/models"
	"bootimus/internal/nbd"
	"bootimus/internal/storage"
	"bootimus/web"

	"github.com/pin/tftp/v3"
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
	TFTPPort       int
	TFTPSinglePort bool
	HTTPPort       int
	AdminPort      int
	BootDir        string
	DataDir        string
	ISODir         string
	ServerAddr     string
	Storage        storage.Storage
	Auth           *auth.Manager
	NBDEnabled     bool
	NBDPort        int
}

type Server struct {
	config         *Config
	httpServer     *http.Server
	adminServer    *http.Server
	tftpServer     *tftp.Server
	wg             sync.WaitGroup
	activeSessions *ActiveSessions
	logBroadcaster *LogBroadcaster
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
	Name     string
	Filename string
	Size     int64
	SizeStr  string
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

	return &Server{
		config: cfg,
		activeSessions: &ActiveSessions{
			sessions: make(map[string]*ActiveSession),
		},
		logBroadcaster: lb,
	}
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

	isos, err := s.scanISOs()
	if err != nil {
		log.Printf("Warning: Failed to scan ISOs: %v", err)
	} else {
		log.Printf("Found %d ISO image(s)", len(isos))
		for _, iso := range isos {
			log.Printf("  - %s (%s)", iso.Name, iso.SizeStr)
		}

		if s.config.Storage != nil {
			isoFiles := make([]struct {
				Name, Filename string
				Size           int64
			}, len(isos))
			for i, iso := range isos {
				isoFiles[i] = struct {
					Name, Filename string
					Size           int64
				}{
					Name:     iso.Name,
					Filename: iso.Filename,
					Size:     iso.Size,
				}
			}

			if err := s.config.Storage.SyncImages(isoFiles); err != nil {
				log.Printf("Warning: Failed to sync images with database: %v", err)
			}
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

	return nil
}

func (s *Server) Wait() {
	s.wg.Wait()
}

func (s *Server) Shutdown() error {
	log.Println("Initiating graceful shutdown...")

	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		} else {
			log.Println("HTTP server stopped")
		}
	}

	if s.adminServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.adminServer.Shutdown(ctx); err != nil {
			log.Printf("Admin server shutdown error: %v", err)
		} else {
			log.Println("Admin server stopped")
		}
	}

	if s.tftpServer != nil {
		log.Println("TFTP server will stop after current transfers complete")
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All servers stopped gracefully")
	case <-time.After(10 * time.Second):
		log.Println("Shutdown timeout reached (10s) - forcing shutdown")
		log.Println("Some goroutines may not have completed cleanly")
	}

	return nil
}

func (s *Server) scanISOs() ([]ISOImage, error) {
	var isos []ISOImage

	entries, err := os.ReadDir(s.config.ISODir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".iso") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			log.Printf("Warning: Failed to get info for %s: %v", name, err)
			continue
		}

		displayName := strings.TrimSuffix(name, filepath.Ext(name))

		isos = append(isos, ISOImage{
			Name:     displayName,
			Filename: name,
			Size:     info.Size(),
			SizeStr:  formatBytes(info.Size()),
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

func (s *Server) startTFTPServer() error {
	log.Printf("Starting TFTP server on port %d...", s.config.TFTPPort)

	server := tftp.NewServer(
		func(filename string, rf io.ReaderFrom) error {
			cleanPath := filepath.Clean(filename)
			if filepath.IsAbs(cleanPath) {
				cleanPath = filepath.Base(cleanPath)
			}

			log.Printf("TFTP: Client requesting file: %s", filename)

			if cleanPath == "autoexec.ipxe" {
				script := fmt.Sprintf(`#!ipxe

# Auto-detect server IP and chain to dynamic menu
dhcp
chain http://${next-server}:%d/menu.ipxe?mac=${net0/mac} || goto failed

:failed
echo Failed to load boot menu
echo Server: ${next-server}
echo MAC: ${net0/mac}
echo Press any key to retry...
prompt
goto dhcp
`, s.config.HTTPPort)
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

			if s.config.BootDir != "" {
				fullPath := filepath.Join(s.config.BootDir, cleanPath)
				log.Printf("TFTP: Trying boot directory: %s", fullPath)

				file, err := os.Open(fullPath)
				if err != nil {
					log.Printf("TFTP: Failed to open file %s: %v", fullPath, err)
					return err
				}
				defer file.Close()

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

		data, err := bootloaders.Bootloaders.ReadFile(cleanPath)
		if err == nil {
			log.Printf("HTTP: Serving embedded bootloader: %s", cleanPath)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(data)
			return
		}

		if s.config.BootDir != "" {
			fullPath := filepath.Join(s.config.BootDir, cleanPath)
			if _, err := os.Stat(fullPath); err == nil {
				log.Printf("HTTP: Serving from boot directory: %s", fullPath)
				ext := filepath.Ext(r.URL.Path)
				if ext == ".efi" || ext == ".img" || ext == ".iso" {
					w.Header().Set("Content-Type", "application/octet-stream")
				}
				http.ServeFile(w, r, fullPath)
				return
			}
		}

		http.Error(w, "Not found", http.StatusNotFound)
	})

	mux.HandleFunc("/menu.ipxe", s.handleIPXEMenu)

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

	adminHandler := admin.NewHandler(s.config.Storage, s.config.DataDir, s.config.ISODir, s.config.BootDir, Version)

	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		log.Printf("Failed to setup static files: %v", err)
		return
	}

	useAuth := s.config.Auth != nil

	authWrap := func(handler http.HandlerFunc) http.HandlerFunc {
		if useAuth {
			return s.config.Auth.BasicAuthMiddleware(handler)
		}
		return handler
	}

	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// Logout endpoint - returns 401 to clear Basic Auth
	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="Bootimus"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Logged out successfully. Please close your browser or clear credentials."))
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
			if id != "" {
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

	mux.HandleFunc("/api/bootloaders", authWrap(adminHandler.ListBootloaders))
	mux.HandleFunc("/api/bootloaders/upload", authWrap(adminHandler.UploadBootloader))
	mux.HandleFunc("/api/bootloaders/delete", authWrap(adminHandler.DeleteBootloader))

	mux.HandleFunc("/api/images/extract", authWrap(adminHandler.ExtractImage))
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

	log.Printf("autoexec.ipxe requested, chaining to menu.ipxe")

	script := fmt.Sprintf(`#!ipxe
chain http://%s:%d/menu.ipxe?mac=%s
`, s.config.ServerAddr, s.config.HTTPPort, macAddress)

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

	menu := s.generateIPXEMenuWithGroups(images, macAddress)
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
{{if eq $img.BootMethod "memdisk"}}
echo Using Thin OS memdisk loader...
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/thinos-kernel
initrd http://{{$.ServerAddr}}:{{$.HTTPPort}}/thinos-initrd.gz
imgargs thinos-kernel ISO_NAME={{$img.EncodedFilename}} BOOTIMUS_SERVER={{$.ServerAddr}} console=tty0 console=ttyS0,115200n8 earlyprintk=vga,keep debug loglevel=8 rdinit=/init
boot || goto failed
{{else if eq $img.BootMethod "kernel"}}
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
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz {{$img.AutoInstallParam}}{{$img.BootParams}}ip=dhcp
{{else if or (eq $img.Distro "fedora") (eq $img.Distro "centos")}}
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz {{$img.AutoInstallParam}}root=live:http://{{$.ServerAddr}}:{{$.HTTPPort}}/isos/{{$img.EncodedFilename}} rd.live.image inst.repo=http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/iso/ inst.stage2=http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/iso/ rd.neednet=1 ip=dhcp
{{else if eq $img.Distro "debian"}}
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz {{$img.AutoInstallParam}}{{$img.BootParams}}initrd=initrd ip=dhcp priority=critical
{{else if eq $img.Distro "ubuntu"}}
{{if $img.NetbootAvailable}}
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz {{$img.AutoInstallParam}}{{$img.BootParams}}initrd=initrd ip=dhcp
{{else}}
{{if $img.SquashfsPath}}
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz {{$img.AutoInstallParam}}{{$img.BootParams}}initrd=initrd ip=dhcp fetch=http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/{{$img.SquashfsPath}}
{{else}}
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz {{$img.AutoInstallParam}}{{$img.BootParams}}initrd=initrd ip=dhcp url=http://{{$.ServerAddr}}:{{$.HTTPPort}}/isos/{{$img.EncodedFilename}}
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
			autoInstallURL = fmt.Sprintf("http://%s:%d/autoinstall/%s", s.config.ServerAddr, s.config.HTTPPort, url.PathEscape(img.Filename))

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

	var image *models.Image
	var err error

	if s.config.Storage != nil {
		image, err = s.config.Storage.GetImage(path)
		if err != nil || image == nil {
			http.Error(w, "Image not found", http.StatusNotFound)
			return
		}
	} else {
		http.Error(w, "Auto-install requires database", http.StatusInternalServerError)
		return
	}

	if !image.AutoInstallEnabled || image.AutoInstallScript == "" {
		http.Error(w, "Auto-install not configured for this image", http.StatusNotFound)
		return
	}

	var customFiles []*models.CustomFile
	if s.config.Storage != nil {
		customFiles, _ = s.config.Storage.ListCustomFilesByImage(image.ID)
	}

	script := image.AutoInstallScript
	if len(customFiles) > 0 && image.Distro == "arch" {
		script = s.injectArchFileDownloads(script, customFiles)
	}

	contentType := "text/plain"
	switch image.AutoInstallScriptType {
	case "preseed":
		contentType = "text/plain; charset=utf-8"
	case "kickstart":
		contentType = "text/plain; charset=utf-8"
	case "autounattend":
		contentType = "application/xml; charset=utf-8"
	case "autoinstall":
		contentType = "text/yaml; charset=utf-8"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(script)))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))

	log.Printf("Served auto-install script for %s (type: %s, size: %d bytes, files: %d)",
		image.Filename, image.AutoInstallScriptType, len(script), len(customFiles))
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
