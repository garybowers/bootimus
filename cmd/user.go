package cmd

import (
	"fmt"
	"os"

	"bootimus/internal/storage"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var userForce bool

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

func init() {
	rootCmd.AddCommand(userCmd)
	userCmd.AddCommand(userListCmd)
	userCmd.AddCommand(userEnableCmd)
	userCmd.AddCommand(userDisableCmd)
	userCmd.AddCommand(userSetAdminCmd)
	userCmd.AddCommand(userUnsetAdminCmd)

	for _, c := range []*cobra.Command{userDisableCmd, userUnsetAdminCmd} {
		c.Flags().BoolVar(&userForce, "force", false, "Bypass the last-active-admin guard")
	}
}

// openStoreOrExit opens the configured storage backend (Postgres or SQLite)
// and runs migrations. Exits the process on failure — callers are CLI ops.
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

// setUserFlags applies enabled and/or isAdmin updates to a user. nil means
// "leave unchanged". Refuses to demote/disable the last active admin unless
// --force is passed.
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
