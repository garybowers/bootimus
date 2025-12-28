package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"bootimus/bootloaders"
	"bootimus/internal/admin"
	"bootimus/internal/auth"
	"bootimus/internal/database"
	"bootimus/internal/models"
	"bootimus/web"

	"github.com/pin/tftp/v3"
)

type Config struct {
	TFTPPort   int
	HTTPPort   int
	AdminPort  int
	BootDir    string
	DataDir    string
	ServerAddr string
	DB         *database.DB
	Auth       *auth.Manager
}

type Server struct {
	config      *Config
	httpServer  *http.Server
	adminServer *http.Server
	tftpServer  *tftp.Server
	wg          sync.WaitGroup
}

type ISOImage struct {
	Name     string
	Filename string
	Size     int64
	SizeStr  string
}

func New(cfg *Config) *Server {
	return &Server{
		config: cfg,
	}
}

func (s *Server) Start() error {
	log.Printf("Starting Bootimus - PXE/HTTP Boot Server")
	log.Printf("Boot directory: %s", s.config.BootDir)
	log.Printf("Data directory: %s", s.config.DataDir)
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
		if s.config.DB != nil {
			isoFiles := make([]struct{ Name, Filename string; Size int64 }, len(isos))
			for i, iso := range isos {
				isoFiles[i] = struct{ Name, Filename string; Size int64 }{
					Name:     iso.Name,
					Filename: iso.Filename,
					Size:     iso.Size,
				}
			}
			if err := s.config.DB.SyncImages(isoFiles); err != nil {
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

	s.wg.Wait()
	log.Println("All servers stopped")
	return nil
}

func (s *Server) scanISOs() ([]ISOImage, error) {
	var isos []ISOImage

	entries, err := os.ReadDir(s.config.DataDir)
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
			log.Printf("HTTP: Failed to decode filename %s: %v", filename, err)
			http.Error(w, "Invalid filename", http.StatusBadRequest)
			return
		}

		log.Printf("HTTP: ISO request %s %s (decoded: %s) from %s", r.Method, filename, decodedFilename, r.RemoteAddr)

		// Build full path to ISO
		fullPath := filepath.Join(s.config.DataDir, decodedFilename)

		// Security check: ensure the path is within DataDir
		cleanPath := filepath.Clean(fullPath)
		if !strings.HasPrefix(cleanPath, filepath.Clean(s.config.DataDir)) {
			log.Printf("HTTP: Path traversal attempt: %s", decodedFilename)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Check if file exists
		fileInfo, err := os.Stat(fullPath)
		if err != nil {
			log.Printf("HTTP: File not found: %s (error: %v)", fullPath, err)
			http.NotFound(w, r)
			return
		}

		if fileInfo.IsDir() {
			log.Printf("HTTP: Requested path is a directory: %s", fullPath)
			http.Error(w, "Not a file", http.StatusBadRequest)
			return
		}

		log.Printf("HTTP: Serving ISO %s (%d bytes)", decodedFilename, fileInfo.Size())
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, fullPath)
	})

	// Boot files server endpoint (kernel/initrd)
	mux.HandleFunc("/boot/", func(w http.ResponseWriter, r *http.Request) {
		// Strip /boot/ prefix and decode the path
		urlPath := strings.TrimPrefix(r.URL.Path, "/boot/")
		decodedPath, err := url.PathUnescape(urlPath)
		if err != nil {
			log.Printf("HTTP: Failed to decode boot path %s: %v", urlPath, err)
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		log.Printf("HTTP: Boot file request %s (decoded: %s) from %s", urlPath, decodedPath, r.RemoteAddr)

		// Build full path to boot file (in cache directory)
		cacheDir := filepath.Join(s.config.DataDir, ".cache")
		fullPath := filepath.Join(cacheDir, decodedPath)

		// Security check: ensure the path is within cache directory
		cleanPath := filepath.Clean(fullPath)
		if !strings.HasPrefix(cleanPath, filepath.Clean(cacheDir)) {
			log.Printf("HTTP: Path traversal attempt: %s", decodedPath)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Check if file exists
		fileInfo, err := os.Stat(fullPath)
		if err != nil {
			log.Printf("HTTP: Boot file not found: %s (error: %v)", fullPath, err)
			http.NotFound(w, r)
			return
		}

		if fileInfo.IsDir() {
			log.Printf("HTTP: Requested path is a directory: %s", fullPath)
			http.Error(w, "Not a file", http.StatusBadRequest)
			return
		}

		log.Printf("HTTP: Serving boot file %s (%d bytes)", decodedPath, fileInfo.Size())
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
		Handler: mux,
	}

	if err := s.adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("Admin server failed: %w", err)
	}

	return nil
}

func (s *Server) setupAdminInterface(mux *http.ServeMux) {
	log.Println("Setting up admin interface")

	// Create admin handler
	adminHandler := admin.NewHandler(s.config.DB, s.config.DataDir, s.config.BootDir)

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
	mux.HandleFunc("/api/clients/assign", authWrap(adminHandler.AssignImages))
	mux.HandleFunc("/api/images/upload", authWrap(adminHandler.UploadImage))

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

	log.Printf("Generating iPXE menu for MAC: %s", macAddress)

	var images []models.Image
	var err error

	// Get images based on MAC address permissions
	if s.config.DB != nil {
		images, err = s.config.DB.GetImagesForClient(macAddress)
		if err != nil {
			log.Printf("Failed to get images from database: %v", err)
			// Fall back to scanning filesystem
			isos, _ := s.scanISOs()
			images = convertISOsToImages(isos)
		}
	} else {
		// No database, use filesystem
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
kernel http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/vmlinuz {{$img.BootParams}}iso-url=http://{{$.ServerAddr}}:{{$.HTTPPort}}/isos/{{$img.EncodedFilename}} ip=dhcp
initrd http://{{$.ServerAddr}}:{{$.HTTPPort}}/boot/{{$img.CacheDir}}/initrd
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
		Name            string
		Filename        string
		EncodedFilename string
		SizeStr         string
		BootMethod      string
		Extracted       bool
		BootParams      string
		CacheDir        string
	}

	imageData := make([]ImageData, len(images))
	for i, img := range images {
		cacheDir := strings.TrimSuffix(img.Filename, filepath.Ext(img.Filename))
		imageData[i] = ImageData{
			Name:            img.Name,
			Filename:        img.Filename,
			EncodedFilename: url.PathEscape(img.Filename),
			SizeStr:         formatBytes(img.Size),
			BootMethod:      img.BootMethod,
			Extracted:       img.Extracted,
			BootParams:      img.BootParams,
			CacheDir:        url.PathEscape(cacheDir),
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

	if s.config.DB != nil {
		images, err = s.config.DB.GetImagesForClient(macAddress)
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
