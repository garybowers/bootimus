package cmd

import (
	"fmt"
	"os"
	"strings"

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
- Database-backed MAC address and image access control (SQLite or PostgreSQL)
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

	rootCmd.PersistentFlags().Int("tftp-port", 69, "TFTP server port")
	rootCmd.PersistentFlags().Bool("tftp-single-port", false, "Enable TFTP single port")
	rootCmd.PersistentFlags().Int("http-port", 8080, "HTTP server port")
	rootCmd.PersistentFlags().Int("admin-port", 8081, "Admin interface port")
	rootCmd.PersistentFlags().Bool("nbd-enabled", true, "Enable NBD server for network block device ISO mounting")
	rootCmd.PersistentFlags().Int("nbd-port", 10809, "NBD server port")
	rootCmd.PersistentFlags().String("data-dir", "./data", "Base data directory (subdirs: isos/, bootloaders/)")
	rootCmd.PersistentFlags().String("server-addr", "", "Server IP address (auto-detected if not specified)")

	rootCmd.PersistentFlags().String("db-host", "", "PostgreSQL host (if empty, uses SQLite)")
	rootCmd.PersistentFlags().Int("db-port", 5432, "PostgreSQL port")
	rootCmd.PersistentFlags().String("db-user", "bootimus", "PostgreSQL user")
	rootCmd.PersistentFlags().String("db-password", "", "PostgreSQL password")
	rootCmd.PersistentFlags().String("db-name", "bootimus", "PostgreSQL database name")
	rootCmd.PersistentFlags().String("db-sslmode", "disable", "PostgreSQL SSL mode")

	rootCmd.PersistentFlags().String("ldap-host", "", "LDAP server hostname (enables LDAP auth)")
	rootCmd.PersistentFlags().Int("ldap-port", 389, "LDAP server port")
	rootCmd.PersistentFlags().Bool("ldap-tls", false, "Use LDAPS (TLS)")
	rootCmd.PersistentFlags().Bool("ldap-starttls", false, "Use StartTLS")
	rootCmd.PersistentFlags().Bool("ldap-skip-verify", false, "Skip TLS certificate verification")
	rootCmd.PersistentFlags().String("ldap-bind-dn", "", "LDAP bind DN for search")
	rootCmd.PersistentFlags().String("ldap-bind-password", "", "LDAP bind password")
	rootCmd.PersistentFlags().String("ldap-base-dn", "", "LDAP base DN for user search")
	rootCmd.PersistentFlags().String("ldap-user-filter", "(sAMAccountName=%s)", "LDAP user search filter (%s = username)")
	rootCmd.PersistentFlags().String("ldap-group-filter", "", "LDAP group filter for admin access (optional)")
	rootCmd.PersistentFlags().String("ldap-group-base-dn", "", "LDAP base DN for group search")

	rootCmd.PersistentFlags().Bool("disable-remote-profiles", false, "Disable remote distro profile updates")

	rootCmd.PersistentFlags().Bool("proxy-dhcp", false, "Enable in-process proxyDHCP server (answers PXE requests without handing out IPs; requires root or CAP_NET_BIND_SERVICE)")
	rootCmd.PersistentFlags().String("proxy-dhcp-bootfile-bios", "undionly.kpxe", "Bootfile advertised to legacy BIOS PXE clients")
	rootCmd.PersistentFlags().String("proxy-dhcp-bootfile-uefi", "bootimus.efi", "Bootfile advertised to UEFI x64 PXE clients")
	rootCmd.PersistentFlags().String("proxy-dhcp-bootfile-arm64", "bootimus-arm64.efi", "Bootfile advertised to UEFI ARM64 PXE clients")

	rootCmd.PersistentFlags().Bool("windows-smb", false, "Enable Samba share for unattended Windows PXE installs (requires smbd in PATH)")
	rootCmd.PersistentFlags().Int("windows-smb-port", 445, "SMB port (Windows 'net use' always uses 445; override only for testing)")

	viper.BindPFlag("tftp_port", rootCmd.PersistentFlags().Lookup("tftp-port"))
	viper.BindPFlag("tftp_single_port", rootCmd.PersistentFlags().Lookup("tftp-single-port"))
	viper.BindPFlag("http_port", rootCmd.PersistentFlags().Lookup("http-port"))
	viper.BindPFlag("admin_port", rootCmd.PersistentFlags().Lookup("admin-port"))
	viper.BindPFlag("nbd_enabled", rootCmd.PersistentFlags().Lookup("nbd-enabled"))
	viper.BindPFlag("nbd_port", rootCmd.PersistentFlags().Lookup("nbd-port"))
	viper.BindPFlag("data_dir", rootCmd.PersistentFlags().Lookup("data-dir"))
	viper.BindPFlag("server_addr", rootCmd.PersistentFlags().Lookup("server-addr"))
	viper.BindPFlag("db.host", rootCmd.PersistentFlags().Lookup("db-host"))
	viper.BindPFlag("db.port", rootCmd.PersistentFlags().Lookup("db-port"))
	viper.BindPFlag("db.user", rootCmd.PersistentFlags().Lookup("db-user"))
	viper.BindPFlag("db.password", rootCmd.PersistentFlags().Lookup("db-password"))
	viper.BindPFlag("db.name", rootCmd.PersistentFlags().Lookup("db-name"))
	viper.BindPFlag("db.sslmode", rootCmd.PersistentFlags().Lookup("db-sslmode"))

	viper.BindPFlag("ldap.host", rootCmd.PersistentFlags().Lookup("ldap-host"))
	viper.BindPFlag("ldap.port", rootCmd.PersistentFlags().Lookup("ldap-port"))
	viper.BindPFlag("ldap.tls", rootCmd.PersistentFlags().Lookup("ldap-tls"))
	viper.BindPFlag("ldap.starttls", rootCmd.PersistentFlags().Lookup("ldap-starttls"))
	viper.BindPFlag("ldap.skip_verify", rootCmd.PersistentFlags().Lookup("ldap-skip-verify"))
	viper.BindPFlag("ldap.bind_dn", rootCmd.PersistentFlags().Lookup("ldap-bind-dn"))
	viper.BindPFlag("ldap.bind_password", rootCmd.PersistentFlags().Lookup("ldap-bind-password"))
	viper.BindPFlag("ldap.base_dn", rootCmd.PersistentFlags().Lookup("ldap-base-dn"))
	viper.BindPFlag("ldap.user_filter", rootCmd.PersistentFlags().Lookup("ldap-user-filter"))
	viper.BindPFlag("ldap.group_filter", rootCmd.PersistentFlags().Lookup("ldap-group-filter"))
	viper.BindPFlag("ldap.group_base_dn", rootCmd.PersistentFlags().Lookup("ldap-group-base-dn"))

	viper.BindPFlag("disable_remote_profiles", rootCmd.PersistentFlags().Lookup("disable-remote-profiles"))

	viper.BindPFlag("proxy_dhcp.enabled", rootCmd.PersistentFlags().Lookup("proxy-dhcp"))
	viper.BindPFlag("proxy_dhcp.bootfile_bios", rootCmd.PersistentFlags().Lookup("proxy-dhcp-bootfile-bios"))
	viper.BindPFlag("proxy_dhcp.bootfile_uefi", rootCmd.PersistentFlags().Lookup("proxy-dhcp-bootfile-uefi"))
	viper.BindPFlag("proxy_dhcp.bootfile_arm64", rootCmd.PersistentFlags().Lookup("proxy-dhcp-bootfile-arm64"))

	viper.BindPFlag("windows_smb.enabled", rootCmd.PersistentFlags().Lookup("windows-smb"))
	viper.BindPFlag("windows_smb.port", rootCmd.PersistentFlags().Lookup("windows-smb-port"))
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
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
