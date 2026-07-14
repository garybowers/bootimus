package cmd

import (
	"log"

	"bootimus/internal/profiles"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "Manage built-in distro profiles",
	Long:  `Manage the catalogue of built-in distro profiles.`,
}

var profilesUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Refresh built-in distro profiles from the remote catalogue",
	Long: `Fetch the latest built-in distro profiles from the Bootimus repository and
apply them to the local database. Custom profiles are never modified.

This is an on-demand action. Bootimus never contacts the remote catalogue on
its own; the profiles baked into the binary are used until you run this command
(or click "Check for Updates" in the admin UI under Boot > Distro Profiles).

Remote URL contacted:
  ` + profiles.RemoteProfilesURL + `

Respects --disable-remote-profiles, which blocks all remote profile fetches.`,
	Run: runProfilesUpdate,
}

func init() {
	rootCmd.AddCommand(profilesCmd)
	profilesCmd.AddCommand(profilesUpdateCmd)
}

func runProfilesUpdate(cmd *cobra.Command, args []string) {
	if viper.GetBool("disable_remote_profiles") {
		log.Fatal("Remote distro profile updates are disabled (--disable-remote-profiles); nothing to do")
	}

	store := openStore()
	defer store.Close()

	mgr := profiles.NewManager(store)

	// Ensure the embedded baseline is present so a fresh database reports
	// meaningful added/updated counts against the remote catalogue.
	if err := mgr.SeedProfiles(); err != nil {
		log.Printf("Warning: failed to seed embedded profiles: %v", err)
	}

	added, updated, version, err := mgr.UpdateFromRemote()
	if err != nil {
		log.Fatalf("Profile update failed: %v", err)
	}

	log.Printf("Distro profiles updated to version %s (%d added, %d updated)", version, added, updated)
}
