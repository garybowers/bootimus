package storage

import (
	"encoding/json"
	"strings"
	"testing"

	"bootimus/internal/models"
)

func TestBootPasswordUpgradeDefaultsToDisabled(test *testing.T) {
	store := newTestStore(test)
	image := &models.Image{Name: "Existing image", Filename: "existing.iso", Enabled: true, Public: true}
	if err := store.CreateImage(image); err != nil {
		test.Fatal(err)
	}
	theme, err := store.GetMenuTheme()
	if err != nil {
		test.Fatal(err)
	}
	theme.Title, theme.MenuTimeout = "Existing menu", 17
	if err := store.UpdateMenuTheme(theme); err != nil {
		test.Fatal(err)
	}
	for _, model := range []any{&models.Image{}, &models.MenuTheme{}} {
		for _, column := range []string{"BootPasswordHash", "BootPasswordRequired", "BootPasswordRevision", "BootPasswordSourceAt"} {
			if err := store.db.Migrator().DropColumn(model, column); err != nil {
				test.Fatal(err)
			}
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
	image, err = store.GetImage("existing.iso")
	if err != nil {
		test.Fatal(err)
	}
	theme, err = store.GetMenuTheme()
	if err != nil {
		test.Fatal(err)
	}
	if image.Name != "Existing image" || !image.Enabled || !image.Public || theme.Title != "Existing menu" || theme.MenuTimeout != 17 {
		test.Fatal("upgrade changed existing image or menu settings")
	}
	if image.BootPasswordRequired || image.BootPasswordHash != "" || image.BootPasswordRevision != "" || !image.BootPasswordSourceAt.IsZero() || theme.BootPasswordRequired || theme.BootPasswordHash != "" || theme.BootPasswordRevision != "" || !theme.BootPasswordSourceAt.IsZero() {
		test.Fatal("upgrade unexpectedly enabled password protection")
	}
}

func TestBootPasswordMigrationAndPersistence(test *testing.T) {
	store := newTestStore(test)
	theme, err := store.GetMenuTheme()
	if err != nil {
		test.Fatal(err)
	}
	if theme.BootPasswordRequired || theme.BootPasswordHash != "" {
		test.Fatal("password protection must default to disabled")
	}
	theme.BootPasswordRequired, theme.BootPasswordHash = true, "test-menu-hash"
	if err := store.UpdateMenuTheme(theme); err != nil {
		test.Fatal(err)
	}
	image := &models.Image{Name: "Test", Filename: "test.iso", Enabled: true, BootPasswordRequired: true, BootPasswordHash: "test-image-hash"}
	if err := store.CreateImage(image); err != nil {
		test.Fatal(err)
	}
	if err := store.AutoMigrate(); err != nil {
		test.Fatal(err)
	}
	theme, err = store.GetMenuTheme()
	if err != nil {
		test.Fatal(err)
	}
	image, err = store.GetImage("test.iso")
	if err != nil {
		test.Fatal(err)
	}
	if !theme.BootPasswordRequired || theme.BootPasswordHash != "test-menu-hash" || !image.BootPasswordRequired || image.BootPasswordHash != "test-image-hash" {
		test.Fatal("migration lost protection")
	}
	for _, value := range []any{theme, image} {
		encoded, err := json.Marshal(value)
		if err != nil {
			test.Fatal(err)
		}
		if strings.Contains(string(encoded), "-hash") || strings.Contains(string(encoded), "boot_password_hash") {
			test.Fatal("JSON exposes the hash")
		}
	}
	image.BootPasswordRequired, image.BootPasswordHash = false, ""
	if err := store.UpdateImage(image.Filename, image); err != nil {
		test.Fatal(err)
	}
	image, err = store.GetImage(image.Filename)
	if err != nil || image.BootPasswordRequired || image.BootPasswordHash != "" {
		test.Fatal("removing protection did not persist")
	}
}
