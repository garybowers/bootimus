package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "bootimus",
	Short: "A PXE and HTTP boot server with MAC address access control",
	Long: `Bootimus is a network boot server that provides:
- TFTP server for PXE boot
- HTTP server for iPXE and ISO serving
- PostgreSQL-backed MAC address and image access control
- Auto-generated boot menus based on client permissions`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./bootimus.yaml)")

	// Server flags
	rootCmd.PersistentFlags().Int("tftp-port", 69, "TFTP server port")
	rootCmd.PersistentFlags().Int("http-port", 8080, "HTTP server port")
	rootCmd.PersistentFlags().Int("admin-port", 8081, "Admin interface port")
	rootCmd.PersistentFlags().String("boot-dir", "", "Optional directory for additional boot files (iPXE bootloaders are embedded)")
	rootCmd.PersistentFlags().String("data-dir", "./data", "Directory containing ISO images")
	rootCmd.PersistentFlags().String("server-addr", "", "Server IP address (auto-detected if not specified)")

	// Database flags
	rootCmd.PersistentFlags().String("db-host", "localhost", "PostgreSQL host")
	rootCmd.PersistentFlags().Int("db-port", 5432, "PostgreSQL port")
	rootCmd.PersistentFlags().String("db-user", "bootimus", "PostgreSQL user")
	rootCmd.PersistentFlags().String("db-password", "", "PostgreSQL password")
	rootCmd.PersistentFlags().String("db-name", "bootimus", "PostgreSQL database name")
	rootCmd.PersistentFlags().String("db-sslmode", "disable", "PostgreSQL SSL mode")
	rootCmd.PersistentFlags().Bool("db-disable", false, "Disable PostgreSQL (use embedded SQLite database)")

	// Bind flags to viper
	viper.BindPFlag("tftp_port", rootCmd.PersistentFlags().Lookup("tftp-port"))
	viper.BindPFlag("http_port", rootCmd.PersistentFlags().Lookup("http-port"))
	viper.BindPFlag("admin_port", rootCmd.PersistentFlags().Lookup("admin-port"))
	viper.BindPFlag("boot_dir", rootCmd.PersistentFlags().Lookup("boot-dir"))
	viper.BindPFlag("data_dir", rootCmd.PersistentFlags().Lookup("data-dir"))
	viper.BindPFlag("server_addr", rootCmd.PersistentFlags().Lookup("server-addr"))
	viper.BindPFlag("db.host", rootCmd.PersistentFlags().Lookup("db-host"))
	viper.BindPFlag("db.port", rootCmd.PersistentFlags().Lookup("db-port"))
	viper.BindPFlag("db.user", rootCmd.PersistentFlags().Lookup("db-user"))
	viper.BindPFlag("db.password", rootCmd.PersistentFlags().Lookup("db-password"))
	viper.BindPFlag("db.name", rootCmd.PersistentFlags().Lookup("db-name"))
	viper.BindPFlag("db.sslmode", rootCmd.PersistentFlags().Lookup("db-sslmode"))
	viper.BindPFlag("db.disable", rootCmd.PersistentFlags().Lookup("db-disable"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/bootimus/")
		viper.SetConfigType("yaml")
		viper.SetConfigName("bootimus")
	}

	viper.SetEnvPrefix("BOOTIMUS")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
