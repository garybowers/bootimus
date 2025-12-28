package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
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

var resetAdminPassword bool

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the PXE/HTTP boot server",
	Long:  `Start the TFTP and HTTP servers to provide network boot services`,
	Run:   runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().BoolVar(&resetAdminPassword, "reset-admin-password", false, "Reset admin password to a new random value")
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
	// Initialize global logger to capture all logs
	server.InitGlobalLogger()

	// Print banner
	printBanner()

	// Get base data directory
	dataDir := viper.GetString("data_dir")

	// Create directory structure if it doesn't exist
	isoDir := dataDir + "/isos"
	bootloadersDir := dataDir + "/bootloaders"

	for _, dir := range []string{dataDir, isoDir, bootloadersDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	log.Printf("Data directory structure initialized at: %s", dataDir)
	log.Printf("  - ISOs: %s", isoDir)
	log.Printf("  - Bootloaders: %s", bootloadersDir)

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

	// Handle admin password reset if requested
	if resetAdminPassword {
		if db == nil {
			log.Fatalf("Database is required for password reset. Please enable database in config.")
		}

		password, err := db.ResetAdminPassword()
		if err != nil {
			log.Fatalf("Failed to reset admin password: %v", err)
		}

		log.Println("╔════════════════════════════════════════════════════════════════╗")
		log.Println("║                  ADMIN PASSWORD RESET                          ║")
		log.Println("╠════════════════════════════════════════════════════════════════╣")
		log.Printf("║  Username: %-50s ║\n", "admin")
		log.Printf("║  New Password: %-46s ║\n", password)
		log.Println("╠════════════════════════════════════════════════════════════════╣")
		log.Println("║  This password will NOT be shown again!                        ║")
		log.Println("║  Save it now before continuing.                                ║")
		log.Println("╚════════════════════════════════════════════════════════════════╝")
		log.Println("\nContinuing to start server...")
	}

	// Initialise authentication manager
	authMgr, err := auth.NewManager(db)
	if err != nil {
		log.Fatalf("Failed to initialise authentication: %v", err)
	}

	// Set version in server package
	server.Version = version

	// Create server config
	cfg := &server.Config{
		TFTPPort:   viper.GetInt("tftp_port"),
		HTTPPort:   viper.GetInt("http_port"),
		AdminPort:  viper.GetInt("admin_port"),
		BootDir:    bootloadersDir,
		DataDir:    isoDir,
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
