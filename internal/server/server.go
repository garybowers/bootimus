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
	"bootimus/internal/storage"
	"bootimus/web"

	"github.com/pin/tftp/v3"
)

var Version = "dev" // Overridden at build time

// panicRecoveryMiddleware catches panics and logs them with full stack traces
func panicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Log the panic with full details
				log.Printf("PANIC RECOVERED: %v", err)
				log.Printf("Request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

				// Get memory stats
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				log.Printf("Memory Stats at panic:")
				log.Printf("  Alloc = %d MB (currently allocated)", m.Alloc/1024/1024)
				log.Printf("  TotalAlloc = %d MB (total allocated over time)", m.TotalAlloc/1024/1024)
				log.Printf("  Sys = %d MB (obtained from system)", m.Sys/1024/1024)
				log.Printf("  NumGC = %d (number of GC runs)", m.NumGC)

				// Print stack trace
				log.Printf("Stack trace:\n%s", debug.Stack())

				// Return 500 error to client
				http.Error(w, "Internal Server Error - the request caused a panic. Check server logs for details.", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type Config struct {
	TFTPPort   int
	HTTPPort   int
	AdminPort  int
	BootDir    string        // Directory for custom bootloaders (/data/bootloaders)
	DataDir    string        // Base data directory (/data)
	ISODir     string        // ISO directory (/data/isos)
	ServerAddr string
	Storage    storage.Storage // Unified storage interface (PostgreSQL or SQLite)
	Auth       *auth.Manager
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
	Activity   string    `json:"activity"` // "downloading", "booting", etc
}

type ActiveSessions struct {
	mu       sync.RWMutex
	sessions map[string]*ActiveSession // key: IP address
}

type LogBroadcaster struct {
	mu        sync.RWMutex
	clients   map[chan string]bool
	logBuffer []string
	maxBuffer int
}

// Global shared log buffer for capturing logs from application start
var globalLogBuffer struct {
	mu     sync.RWMutex
	buffer []string
}

// Global log broadcaster reference for real-time log streaming
var globalLogBroadcaster *LogBroadcaster

// LogWriter is a custom writer that captures logs and writes to stdout
type LogWriter struct{}

func (lw *LogWriter) Write(p []byte) (n int, err error) {
	msg := string(bytes.TrimRight(p, "\n"))

	// Add to global buffer
	globalLogBuffer.mu.Lock()
	globalLogBuffer.buffer = append(globalLogBuffer.buffer, msg)
	if len(globalLogBuffer.buffer) > 100 {
		globalLogBuffer.buffer = globalLogBuffer.buffer[1:]
	}
	globalLogBuffer.mu.Unlock()

	// Broadcast to live log viewers if broadcaster is initialized
	if globalLogBroadcaster != nil {
		globalLogBroadcaster.Broadcast(msg)
	}

	// Write to stdout
	return os.Stdout.Write(p)
}

// InitGlobalLogger sets up the global logger to capture all logs
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

	// Copy global buffer to this broadcaster's buffer
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

	// Send buffered logs to new subscriber
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

	// Add to buffer
	lb.logBuffer = append(lb.logBuffer, msg)
	if len(lb.logBuffer) > lb.maxBuffer {
		lb.logBuffer = lb.logBuffer[1:]
	}

	// Also add to global buffer
	globalLogBuffer.mu.Lock()
	globalLogBuffer.buffer = append(globalLogBuffer.buffer, msg)
	if len(globalLogBuffer.buffer) > 100 {
		globalLogBuffer.buffer = globalLogBuffer.buffer[1:]
	}
	globalLogBuffer.mu.Unlock()

	// Send to all subscribers
	for ch := range lb.clients {
		select {
		case ch <- msg:
		default:
			// Client is slow, skip
		}
	}
}

func (lb *LogBroadcaster) GetLogs() []string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	// Return a copy of the log buffer
	logs := make([]string, len(lb.logBuffer))
	copy(logs, lb.logBuffer)
	return logs
}

// Helper method for Server to log and broadcast
func (s *Server) logAndBroadcast(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	// Just use log.Print - it will be captured by our global logger
	log.Print(msg)
}

type ISOImage struct {
	Name     string
	Filename string
	Size     int64
	SizeStr  string
}

// completionLogger wraps http.ResponseWriter to log when file transfer completes
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

	// Update session progress
	if w.activeSessions != nil {
		w.activeSessions.Update(w.remoteAddr, w.written)
	}

	// Log completion when all bytes written (only log once)
	if !w.logged && w.written >= w.fileSize {
		duration := time.Since(w.startTime)
		msg := fmt.Sprintf("ISO: Client %s finished downloading %s (%d MB) in %v",
			w.remoteAddr, w.filename, w.fileSize/1024/1024, duration.Round(time.Second))
		// Just use log.Print - it will be captured by our global logger
		log.Print(msg)
		w.logged = true

		// Remove from active sessions
		if w.activeSessions != nil {
			w.activeSessions.Remove(w.remoteAddr)
		}
	}

	return n, err
}

func New(cfg *Config) *Server {
	lb := NewLogBroadcaster()

	// Set global broadcaster so LogWriter can broadcast all logs in real-time
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

	// Scan for ISOs
	isos, err := s.scanISOs()
	if err != nil {
		log.Printf("Warning: Failed to scan ISOs: %v", err)
	} else {
		log.Printf("Found %d ISO image(s)", len(isos))
		for _, iso := range isos {
			log.Printf("  - %s (%s)", iso.Name, iso.SizeStr)
		}

		// Sync ISOs with database
		if s.config.Storage != nil {
			isoFiles := make([]struct{ Name, Filename string; Size int64 }, len(isos))
			for i, iso := range isos {
				isoFiles[i] = struct{ Name, Filename string; Size int64 }{
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

	// Start TFTP server
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.startTFTPServer(); err != nil {
			log.Printf("TFTP server error: %v", err)
		}
	}()

	// Start HTTP server
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.startHTTPServer(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start Admin server
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.startAdminServer(); err != nil {
			log.Printf("Admin server error: %v", err)
		}
	}()

	return nil
}

func (s *Server) Wait() {
	s.wg.Wait()
}

func (s *Server) Shutdown() error {
	log.Println("Initiating graceful shutdown...")

	// Shutdown HTTP server
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		} else {
			log.Println("HTTP server stopped")
		}
	}

	// Shutdown Admin server
	if s.adminServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.adminServer.Shutdown(ctx); err != nil {
			log.Printf("Admin server shutdown error: %v", err)
		} else {
			log.Println("Admin server stopped")
		}
	}

	// TFTP server doesn't support graceful shutdown, so we just log
	if s.tftpServer != nil {
		log.Println("TFTP server will stop after current transfers complete")
	}

	// Wait for goroutines with 10 second timeout
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

			// Try embedded bootloaders first
			data, err := bootloaders.Bootloaders.ReadFile(cleanPath)
			if err == nil {
				// Serving from embedded bootloaders
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

			// Fallback to boot directory if configured
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

	addr := fmt.Sprintf(":%d", s.config.TFTPPort)
	if err := server.ListenAndServe(addr); err != nil {
		return fmt.Errorf("TFTP server failed: %w", err)
	}

	return nil
}

func (s *Server) startHTTPServer() error {
	log.Printf("Starting HTTP server on port %d...", s.config.HTTPPort)

	mux := http.NewServeMux()

	// Main file server for boot files - serve from embedded bootloaders
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("HTTP: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		// Clean the path
		cleanPath := strings.TrimPrefix(r.URL.Path, "/")
		if cleanPath == "" {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		// Try embedded bootloaders first
		data, err := bootloaders.Bootloaders.ReadFile(cleanPath)
		if err == nil {
			log.Printf("HTTP: Serving embedded bootloader: %s", cleanPath)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(data)
			return
		}

		// Fallback to boot directory if configured
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

	// Dynamic iPXE menu generation
	mux.HandleFunc("/menu.ipxe", s.handleIPXEMenu)

	// autoexec.ipxe - chainload to menu.ipxe
	mux.HandleFunc("/autoexec.ipxe", s.handleAutoexec)

	// ISO file server endpoint
	mux.HandleFunc("/isos/", func(w http.ResponseWriter, r *http.Request) {
		// Strip /isos/ prefix and decode the filename
		filename := strings.TrimPrefix(r.URL.Path, "/isos/")
		decodedFilename, err := url.PathUnescape(filename)
		if err != nil {
			log.Printf("ISO: Failed to decode filename %s: %v", filename, err)
			http.Error(w, "Invalid filename", http.StatusBadRequest)
			return
		}

		// Extract MAC address from query parameter if provided
		macAddress := r.URL.Query().Get("mac")
		if macAddress == "" {
			macAddress = "unknown"
		} else {
			macAddress = strings.ToLower(strings.ReplaceAll(macAddress, "-", ":"))
		}

		// Build full path to ISO
		fullPath := filepath.Join(s.config.ISODir, decodedFilename)

		// Security check: ensure the path is within ISODir
		cleanPath := filepath.Clean(fullPath)
		if !strings.HasPrefix(cleanPath, filepath.Clean(s.config.ISODir)) {
			s.logAndBroadcast("ISO: Path traversal attempt from MAC %s (IP: %s): %s", macAddress, r.RemoteAddr, decodedFilename)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Check if file exists
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

		// Log download requests
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			s.logAndBroadcast("ISO Download: Client MAC %s (IP: %s) started downloading %s (%d MB)", macAddress, r.RemoteAddr, decodedFilename, fileInfo.Size()/1024/1024)
			// Add to active sessions
			s.activeSessions.Add(r.RemoteAddr, decodedFilename, fileInfo.Size(), "downloading")
		} else {
			// Log range requests for debugging
			log.Printf("ISO: Range request from MAC %s (IP: %s) for %s - Range: %s", macAddress, r.RemoteAddr, decodedFilename, rangeHeader)
		}

		// Wrap ResponseWriter to detect when transfer completes
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

	// Boot files server endpoint (kernel/initrd)
	mux.HandleFunc("/boot/", func(w http.ResponseWriter, r *http.Request) {
		// Strip /boot/ prefix and decode the path
		urlPath := strings.TrimPrefix(r.URL.Path, "/boot/")
		decodedPath, err := url.PathUnescape(urlPath)
		if err != nil {
			log.Printf("Boot: Failed to decode path %s: %v", urlPath, err)
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		// Extract MAC address from query parameter if provided
		macAddress := r.URL.Query().Get("mac")
		if macAddress == "" {
			macAddress = "unknown"
		} else {
			macAddress = strings.ToLower(strings.ReplaceAll(macAddress, "-", ":"))
		}

		// Build full path to boot file (in isos directory subdirs)
		fullPath := filepath.Join(s.config.ISODir, decodedPath)

		// Security check: ensure the path is within ISO directory
		cleanPath := filepath.Clean(fullPath)
		if !strings.HasPrefix(cleanPath, filepath.Clean(s.config.ISODir)) {
			s.logAndBroadcast("Boot: Path traversal attempt from MAC %s (IP: %s): %s", macAddress, r.RemoteAddr, decodedPath)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Check if file exists
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

		// Only log kernel/initrd fetches, not range requests
		if r.Header.Get("Range") == "" {
			s.logAndBroadcast("Boot File: Serving %s (%d MB) to MAC %s (IP: %s)", decodedPath, fileInfo.Size()/1024/1024, macAddress, r.RemoteAddr)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, fullPath)
	})

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK\n")
	})

	// API endpoint to list ISOs
	mux.HandleFunc("/api/isos", s.handleListISOs)

	// Auto-install script serving endpoint (public, no auth)
	mux.HandleFunc("/autoinstall/", s.handleAutoInstallScript)

	// Custom files serving endpoint (public, with access control)
	mux.HandleFunc("/files/", s.handleCustomFile)

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

	// Setup admin interface
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

	// Create admin handler with unified storage
	adminHandler := admin.NewHandler(s.config.Storage, s.config.DataDir, s.config.ISODir, s.config.BootDir, Version)

	// Serve embedded static files
	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		log.Printf("Failed to setup static files: %v", err)
		return
	}

	// Determine if we should use authentication
	useAuth := s.config.Auth != nil

	// Helper function to optionally wrap with auth
	authWrap := func(handler http.HandlerFunc) http.HandlerFunc {
		if useAuth {
			return s.config.Auth.BasicAuthMiddleware(handler)
		}
		return handler
	}

	// Admin UI - serve at root of admin server
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// Admin API endpoints with REST routing and optional authentication
	mux.HandleFunc("/api/server-info", authWrap(adminHandler.GetServerInfo))
	mux.HandleFunc("/api/stats", authWrap(adminHandler.GetStats))
	mux.HandleFunc("/api/logs", authWrap(adminHandler.GetBootLogs))
	mux.HandleFunc("/api/scan", authWrap(adminHandler.ScanImages))
	mux.HandleFunc("/api/images/upload", authWrap(adminHandler.UploadImage))
	mux.HandleFunc("/api/assign-images", authWrap(adminHandler.AssignImages))

	// RESTful client endpoints
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

	// RESTful image endpoints
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

	// Bootloader endpoints
	mux.HandleFunc("/api/bootloaders", authWrap(adminHandler.ListBootloaders))
	mux.HandleFunc("/api/bootloaders/upload", authWrap(adminHandler.UploadBootloader))
	mux.HandleFunc("/api/bootloaders/delete", authWrap(adminHandler.DeleteBootloader))

	// Extraction endpoints
	mux.HandleFunc("/api/images/extract", authWrap(adminHandler.ExtractImage))
	mux.HandleFunc("/api/images/boot-method", authWrap(adminHandler.SetBootMethod))

	// Active sessions endpoint
	mux.HandleFunc("/api/active-sessions", authWrap(s.handleActiveSessions))

	// Live logs endpoints
	mux.HandleFunc("/api/logs/stream", authWrap(s.handleLogsStream))
	mux.HandleFunc("/api/logs/buffer", authWrap(s.handleLogsBuffer))

	// User management endpoints
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

	// ISO download endpoints
	mux.HandleFunc("/api/images/download", authWrap(adminHandler.DownloadISO))
	mux.HandleFunc("/api/downloads", authWrap(adminHandler.ListDownloads))
	mux.HandleFunc("/api/downloads/progress", authWrap(adminHandler.GetDownloadProgress))

	// Netboot download endpoints
	mux.HandleFunc("/api/images/netboot/download", authWrap(adminHandler.DownloadNetboot))

	// Auto-install script endpoints
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

	// Custom file management endpoints
	mux.HandleFunc("/api/files", authWrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// Check if ID parameter is provided for single file retrieval
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
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Subscribe to log broadcaster
	logChan := s.logBroadcaster.Subscribe()
	defer s.logBroadcaster.Unsubscribe(logChan)

	// Get context for client disconnect detection
	ctx := r.Context()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send initial connection message
	fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	// Stream logs to client
	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			return
		case msg, ok := <-logChan:
			if !ok {
				return
			}
			// Send log message as JSON
			fmt.Fprintf(w, "data: {\"type\":\"log\",\"message\":%q}\n\n", msg)
			flusher.Flush()
		}
	}
}

func (s *Server) handleAutoexec(w http.ResponseWriter, r *http.Request) {
	// autoexec.ipxe chains to menu.ipxe with MAC address
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
	// Extract MAC address from query parameter or use default
	macAddress := r.URL.Query().Get("mac")
	if macAddress == "" {
		// Try to extract from X-Forwarded-For or other headers if needed
		macAddress = "unknown"
	}

	// Normalise MAC address
	macAddress = strings.ToLower(strings.ReplaceAll(macAddress, "-", ":"))

	s.logAndBroadcast("Client Connected: MAC %s (IP: %s) requesting boot menu", macAddress, r.RemoteAddr)

	var images []models.Image
	var err error

	// Get images based on MAC address permissions
	if s.config.Storage != nil {
		images, err = s.config.Storage.GetImagesForClient(macAddress)
		if err != nil {
			log.Printf("Failed to get images from database: %v", err)
			// Fall back to scanning filesystem
			isos, _ := s.scanISOs()
			images = convertISOsToImages(isos)
		}
	} else {
		// No database configured, use filesystem
		isos, _ := s.scanISOs()
		images = convertISOsToImages(isos)
	}

	menu := s.generateIPXEMenu(images, macAddress)
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
initrd http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/bcd BCD
initrd http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/boot.sdi boot.sdi
initrd http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/boot.wim boot.wim
boot || goto failed
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
sanboot --no-describe --drive 0x80 http://{{$.ServerAddr}}:{{$.HTTPPort}}/isos/{{$img.EncodedFilename}}?mac={{$.MAC}} || goto failed
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
		Name                string
		Filename            string
		EncodedFilename     string
		SizeStr             string
		BootMethod          string
		Extracted           bool
		BootParams          string
		CacheDir            string
		Distro              string
		AutoInstallEnabled  bool
		AutoInstallURL      string
		AutoInstallParam    string
		SquashfsPath        string
		NetbootAvailable    bool
	}

	imageData := make([]ImageData, len(images))
	for i, img := range images {
		cacheDir := strings.TrimSuffix(img.Filename, filepath.Ext(img.Filename))

		// Build auto-install URL and parameter if enabled
		autoInstallURL := ""
		autoInstallParam := ""
		if img.AutoInstallEnabled && img.AutoInstallScript != "" {
			autoInstallURL = fmt.Sprintf("http://%s:%d/autoinstall/%s", s.config.ServerAddr, s.config.HTTPPort, url.PathEscape(img.Filename))

			// Set appropriate boot parameter based on script type and distro
			switch img.AutoInstallScriptType {
			case "preseed":
				// Debian/Ubuntu preseed
				autoInstallParam = fmt.Sprintf("auto=true priority=critical url=%s ", autoInstallURL)
			case "kickstart":
				// Red Hat/CentOS/Fedora kickstart
				autoInstallParam = fmt.Sprintf("inst.ks=%s ", autoInstallURL)
			case "autoinstall":
				// Ubuntu autoinstall (cloud-init)
				autoInstallParam = fmt.Sprintf("autoinstall ds=nocloud-net;s=%s/ ", autoInstallURL)
			case "autounattend":
				// Windows autounattend.xml - not typically used via kernel params
				autoInstallParam = ""
			default:
				autoInstallParam = fmt.Sprintf("autoinstall=%s ", autoInstallURL)
			}
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
		// No database, use filesystem
		isos, _ := s.scanISOs()
		images = convertISOsToImages(isos)
	}

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "Available ISO images:\n")
	for _, img := range images {
		fmt.Fprintf(w, "  - %s (%s)\n", img.Name, formatBytes(img.Size))
	}
}

// handleCustomFile serves custom files to clients
// URL format: /files/{filename}
// Public files are accessible to all clients
// Image-specific files are accessible to any client (they're just organized by image)
func (s *Server) handleCustomFile(w http.ResponseWriter, r *http.Request) {
	// Extract filename from path
	filename := strings.TrimPrefix(r.URL.Path, "/files/")
	if filename == "" {
		http.Error(w, "Missing filename in path", http.StatusBadRequest)
		return
	}

	// Decode filename
	decodedFilename, err := url.PathUnescape(filename)
	if err != nil {
		log.Printf("CustomFile: Failed to decode filename %s: %v", filename, err)
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	// Clean filename for security
	cleanFilename := filepath.Clean(decodedFilename)
	if cleanFilename == "." || cleanFilename == ".." || strings.Contains(cleanFilename, "..") {
		log.Printf("CustomFile: Path traversal attempt: %s from %s", decodedFilename, r.RemoteAddr)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Get file metadata from database
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

	// Determine file path based on whether it's public or image-specific
	var fullPath string
	if file.Public {
		// Public files are stored in /data/files/
		fullPath = filepath.Join(s.config.DataDir, "files", cleanFilename)
	} else if file.ImageID != nil && file.Image != nil {
		// Image-specific files are stored in /data/isos/{image-name}/files/
		imageName := strings.TrimSuffix(file.Image.Filename, filepath.Ext(file.Image.Filename))
		fullPath = filepath.Join(s.config.ISODir, imageName, "files", cleanFilename)
	} else {
		log.Printf("CustomFile: Invalid file configuration for %s", cleanFilename)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Security check: ensure the path is within allowed directories
	cleanPath := filepath.Clean(fullPath)
	dataDir := filepath.Clean(s.config.DataDir)
	if !strings.HasPrefix(cleanPath, dataDir) {
		log.Printf("CustomFile: Path traversal attempt: %s from %s", cleanFilename, r.RemoteAddr)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Check if file exists
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

	// Increment download count in background
	go func() {
		if s.config.Storage != nil {
			s.config.Storage.IncrementFileDownloadCount(file.ID)
		}
	}()

	// Set content type
	contentType := file.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)

	// Serve file
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

// handleAutoInstallScript serves auto-install scripts for unattended installations
// URL format: /autoinstall/{filename}
// Example: /autoinstall/ubuntu-22.04.iso returns the preseed/autoinstall script for that image

func (s *Server) handleAutoInstallScript(w http.ResponseWriter, r *http.Request) {
	// Extract filename from path
	path := strings.TrimPrefix(r.URL.Path, "/autoinstall/")
	if path == "" {
		http.Error(w, "Missing image filename in path", http.StatusBadRequest)
		return
	}

	// Get image from database
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

	// Check if auto-install is enabled and script exists
	if !image.AutoInstallEnabled || image.AutoInstallScript == "" {
		http.Error(w, "Auto-install not configured for this image", http.StatusNotFound)
		return
	}

	// Get custom files for this image
	var customFiles []*models.CustomFile
	if s.config.Storage != nil {
		customFiles, _ = s.config.Storage.ListCustomFilesByImage(image.ID)
	}

	// Inject file download commands into the script
	script := image.AutoInstallScript
	if len(customFiles) > 0 && image.Distro == "arch" {
		script = s.injectArchFileDownloads(script, customFiles)
	}

	// Set appropriate content type based on script type
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

// injectArchFileDownloads injects file download commands into Arch Linux autoinstall scripts
func (s *Server) injectArchFileDownloads(script string, files []*models.CustomFile) string {
	if len(files) == 0 {
		return script
	}

	// Get server IP
	serverIP := GetOutboundIP()
	serverPort := "8080" // Main server port

	// Build download commands for Arch installation
	var downloadCommands strings.Builder
	downloadCommands.WriteString("\n\n# Download custom files from Bootimus\n")

	for _, file := range files {
		destPath := file.DestinationPath
		if destPath == "" {
			// Default to /root/ if no destination specified
			destPath = "/root/" + file.Filename
		}

		// Create directory if needed
		destDir := filepath.Dir(destPath)
		if destDir != "/" && destDir != "." {
			downloadCommands.WriteString(fmt.Sprintf("arch-chroot /mnt mkdir -p %s\n", destDir))
		}

		// Download file using wget
		downloadCommands.WriteString(fmt.Sprintf(
			"arch-chroot /mnt wget -q http://%s:%s/files/%s -O %s\n",
			serverIP, serverPort, file.Filename, destPath,
		))

		// Make executable if it's a script
		if strings.HasSuffix(file.Filename, ".sh") {
			downloadCommands.WriteString(fmt.Sprintf("arch-chroot /mnt chmod +x %s\n", destPath))
		}
	}

	// Append download commands to the script
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
