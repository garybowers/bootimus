package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"bootimus/internal/auth"
	"bootimus/internal/database"
	"bootimus/internal/server"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	colorReset      = "\033[0m"
	colorLightGreen = "\033[92m"
	colorYellow     = "\033[33m"
)

var version = "dev" // Overridden at build time

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the PXE/HTTP boot server",
	Long:  `Start the TFTP and HTTP servers to provide network boot services`,
	Run:   runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func printBanner() {
	banner := `
   ____              __  _
  / __ )____  ____  / /_(_)___ ___  __  _______
 / __  / __ \/ __ \/ __/ / __ '__ \/ / / / ___/
/ /_/ / /_/ / /_/ / /_/ / / / / / / /_/ (__  )
\____/\____/\____/\__/_/_/ /_/ /_/\__,_/____/
`
	fmt.Printf("%s%s%s", colorLightGreen, banner, colorReset)
	fmt.Printf("%sVersion: %s%s\n", colorYellow, version, colorReset)
	fmt.Printf("%sPXE/HTTP Boot Server%s\n\n", colorLightGreen, colorReset)
}

func runServe(cmd *cobra.Command, args []string) {
	// Print banner
	printBanner()

	// Get directories - don't auto-create them
	bootDir := viper.GetString("boot_dir")
	dataDir := viper.GetString("data_dir")

	// Boot directory is now optional (bootloaders are embedded)
	if bootDir != "" {
		if _, err := os.Stat(bootDir); os.IsNotExist(err) {
			log.Printf("Info: Boot directory does not exist (using embedded bootloaders): %s", bootDir)
		}
	} else {
		log.Println("Using embedded iPXE bootloaders (no boot directory configured)")
	}

	// Data directory check
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		log.Printf("Warning: Data directory does not exist: %s", dataDir)
		log.Printf("Create it with: mkdir -p %s", dataDir)
	}

	// Auto-detect server address if not specified
	serverAddr := viper.GetString("server_addr")
	if serverAddr == "" {
		serverAddr = server.GetOutboundIP()
		log.Printf("Auto-detected server IP: %s", serverAddr)
	}

	// Setup database if not disabled
	var db *database.DB
	var err error

	if !viper.GetBool("db.disable") {
		dbCfg := &database.Config{
			Host:     viper.GetString("db.host"),
			Port:     viper.GetInt("db.port"),
			User:     viper.GetString("db.user"),
			Password: viper.GetString("db.password"),
			DBName:   viper.GetString("db.name"),
			SSLMode:  viper.GetString("db.sslmode"),
		}

		// Retry database connection with exponential backoff
		maxRetries := 10
		for i := 0; i < maxRetries; i++ {
			db, err = database.New(dbCfg)
			if err == nil {
				break
			}

			if i < maxRetries-1 {
				waitTime := (1 << uint(i)) // Exponential: 1s, 2s, 4s, 8s...
				log.Printf("Database connection failed (attempt %d/%d): %v", i+1, maxRetries, err)
				log.Printf("Retrying in %d seconds...", waitTime)
				time.Sleep(time.Duration(waitTime) * time.Second)
			}
		}

		if err != nil {
			log.Fatalf("Failed to connect to database after %d attempts: %v", maxRetries, err)
		}

		if err := db.AutoMigrate(); err != nil {
			log.Fatalf("Failed to run database migrations: %v", err)
		}

		log.Println("Database connected and migrations completed")
	} else {
		log.Println("Running in SQLite mode (PostgreSQL disabled)")
	}

	// Initialise authentication manager
	// Use current working directory for config if no config file is set
	configDir := "."
	if viper.ConfigFileUsed() != "" {
		configDir = filepath.Dir(viper.ConfigFileUsed())
	}

	authMgr, err := auth.NewManager(configDir)
	if err != nil {
		log.Fatalf("Failed to initialise authentication: %v", err)
	}

	// Create server config
	cfg := &server.Config{
		TFTPPort:   viper.GetInt("tftp_port"),
		HTTPPort:   viper.GetInt("http_port"),
		AdminPort:  viper.GetInt("admin_port"),
		BootDir:    bootDir,
		DataDir:    dataDir,
		ServerAddr: serverAddr,
		DB:         db,
		Auth:       authMgr,
	}

	// Create and start server
	srv := server.New(cfg)
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Received shutdown signal...")
	if err := srv.Shutdown(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}
