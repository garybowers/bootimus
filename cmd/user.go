package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"bootimus/internal/storage"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

var userForce bool
var userPassword string

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage local users from the CLI",
	Long: `Enable, disable, and toggle admin rights on local users.
Useful for emergency recovery if you've locked yourself out of the admin UI
(e.g. demoted the only admin or disabled their account).`,
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all local users",
	Run: func(cmd *cobra.Command, args []string) {
		store := openStoreOrExit()
		users, err := store.ListUsers()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list users: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("%-20s %-8s %-8s\n", "USERNAME", "ENABLED", "ADMIN")
		for _, u := range users {
			fmt.Printf("%-20s %-8v %-8v\n", u.Username, u.Enabled, u.IsAdmin)
		}
	},
}

var userEnableCmd = &cobra.Command{
	Use:   "enable <username>",
	Short: "Enable a user account",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		t := true
		setUserFlags(args[0], &t, nil)
	},
}

var userDisableCmd = &cobra.Command{
	Use:   "disable <username>",
	Short: "Disable a user account",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		f := false
		setUserFlags(args[0], &f, nil)
	},
}

var userSetAdminCmd = &cobra.Command{
	Use:   "set-admin <username>",
	Short: "Grant admin rights to a user",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		t := true
		setUserFlags(args[0], nil, &t)
	},
}

var userUnsetAdminCmd = &cobra.Command{
	Use:   "unset-admin <username>",
	Short: "Revoke admin rights from a user",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		f := false
		setUserFlags(args[0], nil, &f)
	},
}

var userSetPasswordCmd = &cobra.Command{
	Use:   "set-password <username>",
	Short: "Set a user's password",
	Long: `Set a local user's password directly in the database.

If --password is omitted you'll be prompted interactively (input is hidden
and confirmed), avoiding exposure in your shell history. Provide --password
for non-interactive/scripted use.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		setUserPassword(args[0])
	},
}

func init() {
	rootCmd.AddCommand(userCmd)
	userCmd.AddCommand(userListCmd)
	userCmd.AddCommand(userEnableCmd)
	userCmd.AddCommand(userDisableCmd)
	userCmd.AddCommand(userSetAdminCmd)
	userCmd.AddCommand(userUnsetAdminCmd)
	userCmd.AddCommand(userSetPasswordCmd)

	for _, c := range []*cobra.Command{userDisableCmd, userUnsetAdminCmd} {
		c.Flags().BoolVar(&userForce, "force", false, "Bypass the last-active-admin guard")
	}

	userSetPasswordCmd.Flags().StringVar(&userPassword, "password", "", "New password (omit to be prompted interactively)")
}

func openStoreOrExit() storage.Storage {
	dataDir := viper.GetString("data_dir")
	pgHost := viper.GetString("db.host")
	var store storage.Storage
	var err error
	if pgHost != "" {
		store, err = storage.NewPostgresStore(&storage.Config{
			Host:     pgHost,
			Port:     viper.GetInt("db.port"),
			User:     viper.GetString("db.user"),
			Password: viper.GetString("db.password"),
			DBName:   viper.GetString("db.name"),
			SSLMode:  viper.GetString("db.sslmode"),
		})
	} else {
		store, err = storage.NewSQLiteStore(dataDir)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open storage: %v\n", err)
		os.Exit(1)
	}
	if err := store.AutoMigrate(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to run migrations: %v\n", err)
		os.Exit(1)
	}
	return store
}

func setUserFlags(username string, enabled, isAdmin *bool) {
	store := openStoreOrExit()
	user, err := store.GetUser(username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "User not found: %s\n", username)
		os.Exit(1)
	}

	willBeEnabled := user.Enabled
	if enabled != nil {
		willBeEnabled = *enabled
	}
	willBeAdmin := user.IsAdmin
	if isAdmin != nil {
		willBeAdmin = *isAdmin
	}

	if user.IsAdmin && user.Enabled && (!willBeAdmin || !willBeEnabled) && !userForce {
		all, err := store.ListUsers()
		if err == nil {
			others := 0
			for _, u := range all {
				if u.Username != username && u.IsAdmin && u.Enabled {
					others++
				}
			}
			if others == 0 {
				fmt.Fprintf(os.Stderr, "Refusing: %s is the only active admin. Pass --force to override.\n", username)
				os.Exit(1)
			}
		}
	}

	user.Enabled = willBeEnabled
	user.IsAdmin = willBeAdmin
	if err := store.UpdateUser(username, user); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to update user: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Updated %s: enabled=%v admin=%v\n", username, user.Enabled, user.IsAdmin)
}

func setUserPassword(username string) {
	store := openStoreOrExit()
	user, err := store.GetUser(username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "User not found: %s\n", username)
		os.Exit(1)
	}

	password := userPassword
	if password == "" {
		password, err = promptForPassword()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}
	if password == "" {
		fmt.Fprintln(os.Stderr, "Password cannot be empty")
		os.Exit(1)
	}

	if err := user.SetPassword(password); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to hash password: %v\n", err)
		os.Exit(1)
	}
	if err := store.UpdateUser(username, user); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to update user: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Password updated for %s\n", username)
}

func promptForPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("--password is required when stdin is not a terminal")
	}

	fmt.Print("New password: ")
	first, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}

	fmt.Print("Confirm password: ")
	second, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}

	if strings.TrimRight(string(first), "\r\n") != strings.TrimRight(string(second), "\r\n") {
		return "", errors.New("passwords do not match")
	}
	return strings.TrimRight(string(first), "\r\n"), nil
}
