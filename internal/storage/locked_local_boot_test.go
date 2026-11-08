package storage

import (
	"testing"

	"bootimus/internal/models"
)

func TestLockedMenuUpgradeAndPersistence(test *testing.T) {
	store := newTestStore(test)
	theme, err := store.GetMenuTheme()
	if err != nil {
		test.Fatal(err)
	}
	theme.Title, theme.MenuTimeout = "Existing menu", 17
	if err := store.UpdateMenuTheme(theme); err != nil {
		test.Fatal(err)
	}
	for _, column := range []string{"LockedMenuLocalBoot", "LockedMenuTimeout"} {
		if err := store.db.Migrator().DropColumn(&models.MenuTheme{}, column); err != nil {
			test.Fatal(err)
		}
	}
	if err := store.AutoMigrate(); err != nil {
		test.Fatal(err)
	}
	if store.postgres() {
		fresh, err := NewPostgresStore(store.cfg)
		if err != nil {
			test.Fatal(err)
		}
		test.Cleanup(func() { fresh.Close() })
		store.Storage, store.db = fresh, fresh.db
	}
	theme, err = store.GetMenuTheme()
	if err != nil || theme.LockedMenuLocalBoot || theme.LockedMenuTimeout != 0 || theme.Title != "Existing menu" || theme.MenuTimeout != 17 {
		test.Fatal("upgrade changed existing boot behaviour")
	}
	for _, enabled := range []bool{true, false} {
		theme.LockedMenuLocalBoot, theme.LockedMenuTimeout = enabled, 15
		if err := store.UpdateMenuTheme(theme); err != nil {
			test.Fatal(err)
		}
		theme, err = store.GetMenuTheme()
		if err != nil || theme.LockedMenuLocalBoot != enabled || theme.LockedMenuTimeout != 15 {
			test.Fatal("local-boot settings were not persisted")
		}
	}
}
