package storage

import (
	"testing"

	"bootimus/internal/models"
)

func TestImageExceptionUpgradeAndPersistence(test *testing.T) {
	store := newTestStore(test)
	image := &models.Image{Name: "Existing", Filename: "existing.iso", Enabled: true, Public: true, BootPasswordRequired: true, BootPasswordHash: "existing-verifier"}
	if err := store.CreateImage(image); err != nil {
		test.Fatal(err)
	}
	if err := store.db.Migrator().DropColumn(&models.Image{}, "BootMenuExempt"); err != nil {
		test.Fatal(err)
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
	image, err := store.GetImage(image.Filename)
	if err != nil || image.BootMenuExempt || !image.BootPasswordRequired || image.BootPasswordHash != "existing-verifier" {
		test.Fatal("upgrade changed existing protection")
	}
	for _, exempt := range []bool{true, false} {
		image.BootMenuExempt = exempt
		if err := store.UpdateImage(image.Filename, image); err != nil {
			test.Fatal(err)
		}
		image, err = store.GetImage(image.Filename)
		if err != nil || image.BootMenuExempt != exempt {
			test.Fatal("exception was not persisted")
		}
	}
}
