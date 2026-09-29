package storage

import (
	"testing"

	"bootimus/internal/models"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	if err := store.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return store
}

func TestDeleteClientHardDeletesAndAllowsRecreate(t *testing.T) {
	store := newTestStore(t)
	mac := "aa:bb:cc:dd:ee:ff"

	if err := store.CreateClient(&models.Client{MACAddress: mac, Name: "first"}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if err := store.DeleteClient(mac); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}

	var count int64
	store.db.Unscoped().Model(&models.Client{}).Where("mac_address = ?", mac).Count(&count)
	if count != 0 {
		t.Fatalf("expected client row to be gone, found %d rows", count)
	}

	if err := store.CreateClient(&models.Client{MACAddress: mac, Name: "second"}); err != nil {
		t.Fatalf("CreateClient after delete: %v", err)
	}
	client, err := store.GetClient(mac)
	if err != nil {
		t.Fatalf("GetClient after recreate: %v", err)
	}
	if client.Name != "second" {
		t.Fatalf("expected recreated client, got %q", client.Name)
	}
}

func TestDeleteClientMissingMACIsNoop(t *testing.T) {
	store := newTestStore(t)
	if err := store.DeleteClient("00:00:00:00:00:00"); err != nil {
		t.Fatalf("DeleteClient on missing MAC: %v", err)
	}
}

func TestGetClientBootloaderSetsSelectsOnlyRequiredColumns(t *testing.T) {
	store := newTestStore(t)
	group := &models.ClientGroup{Name: "rack-a", BootloaderSet: "secureboot-official"}
	if err := store.CreateClientGroup(group); err != nil {
		t.Fatalf("CreateClientGroup: %v", err)
	}

	clients := []*models.Client{
		{MACAddress: "02:00:00:00:10:01", BootloaderSet: "se350", ClientGroupID: &group.ID},
		{MACAddress: "02:00:00:00:10:02", ClientGroupID: &group.ID},
		{MACAddress: "02:00:00:00:10:03"},
	}
	for _, client := range clients {
		if err := store.CreateClient(client); err != nil {
			t.Fatalf("CreateClient(%s): %v", client.MACAddress, err)
		}
	}

	tests := []struct {
		mac        string
		wantClient string
		wantGroup  string
	}{
		{mac: clients[0].MACAddress, wantClient: "se350", wantGroup: "secureboot-official"},
		{mac: clients[1].MACAddress, wantGroup: "secureboot-official"},
		{mac: clients[2].MACAddress},
	}
	for _, tt := range tests {
		clientSet, groupSet, err := store.GetClientBootloaderSets(tt.mac)
		if err != nil {
			t.Fatalf("GetClientBootloaderSets(%s): %v", tt.mac, err)
		}
		if clientSet != tt.wantClient || groupSet != tt.wantGroup {
			t.Fatalf("GetClientBootloaderSets(%s) = (%q, %q), want (%q, %q)", tt.mac, clientSet, groupSet, tt.wantClient, tt.wantGroup)
		}
	}
}

func TestDeleteClientDetachesBootLogsAndInventory(t *testing.T) {
	store := newTestStore(t)
	mac := "aa:bb:cc:dd:ee:01"

	if err := store.CreateClient(&models.Client{MACAddress: mac}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	client, err := store.GetClient(mac)
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if err := store.db.Create(&models.BootLog{ClientID: &client.ID, MACAddress: mac}).Error; err != nil {
		t.Fatalf("create boot log: %v", err)
	}
	if err := store.db.Create(&models.HardwareInventory{ClientID: &client.ID, MACAddress: mac}).Error; err != nil {
		t.Fatalf("create inventory: %v", err)
	}

	if err := store.DeleteClient(mac); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}

	var bootLog models.BootLog
	if err := store.db.Where("mac_address = ?", mac).First(&bootLog).Error; err != nil {
		t.Fatalf("boot log should survive client deletion: %v", err)
	}
	if bootLog.ClientID != nil {
		t.Fatalf("expected boot log client_id to be nulled, got %d", *bootLog.ClientID)
	}

	var inv models.HardwareInventory
	if err := store.db.Where("mac_address = ?", mac).First(&inv).Error; err != nil {
		t.Fatalf("inventory should survive client deletion: %v", err)
	}
	if inv.ClientID != nil {
		t.Fatalf("expected inventory client_id to be nulled, got %d", *inv.ClientID)
	}
}

func TestAutoMigratePurgesLegacySoftDeletedClients(t *testing.T) {
	store := newTestStore(t)
	mac := "aa:bb:cc:dd:ee:02"

	if err := store.CreateClient(&models.Client{MACAddress: mac}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if err := store.db.Where("mac_address = ?", mac).Delete(&models.Client{}).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	var count int64
	store.db.Unscoped().Model(&models.Client{}).Where("mac_address = ?", mac).Count(&count)
	if count != 1 {
		t.Fatalf("expected soft-deleted row to exist before migration, found %d", count)
	}

	if err := store.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	store.db.Unscoped().Model(&models.Client{}).Where("mac_address = ?", mac).Count(&count)
	if count != 0 {
		t.Fatalf("expected soft-deleted row to be purged, found %d", count)
	}

	if err := store.CreateClient(&models.Client{MACAddress: mac}); err != nil {
		t.Fatalf("CreateClient after purge: %v", err)
	}
}

func TestUpdateClientPersistsAutoInstallFile(t *testing.T) {
	store := newTestStore(t)
	mac := "aa:bb:cc:dd:ee:03"

	if err := store.CreateClient(&models.Client{MACAddress: mac, Name: "node"}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	if err := store.UpdateClient(mac, &models.Client{Name: "node", AutoInstallFile: "ubuntu/user-data.yaml"}); err != nil {
		t.Fatalf("UpdateClient: %v", err)
	}
	client, err := store.GetClient(mac)
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if client.AutoInstallFile != "ubuntu/user-data.yaml" {
		t.Fatalf("expected auto install file to persist, got %q", client.AutoInstallFile)
	}

	if err := store.UpdateClient(mac, &models.Client{Name: "node"}); err != nil {
		t.Fatalf("UpdateClient clear: %v", err)
	}
	client, err = store.GetClient(mac)
	if err != nil {
		t.Fatalf("GetClient after clear: %v", err)
	}
	if client.AutoInstallFile != "" {
		t.Fatalf("expected auto install file to clear, got %q", client.AutoInstallFile)
	}
}

func TestUpdateClientGroupPersistsAutoInstallFile(t *testing.T) {
	store := newTestStore(t)

	group := &models.ClientGroup{Name: "fleet"}
	if err := store.CreateClientGroup(group); err != nil {
		t.Fatalf("CreateClientGroup: %v", err)
	}

	if err := store.UpdateClientGroup(group.ID, &models.ClientGroup{Name: "fleet", AutoInstallFile: "debian/preseed.cfg"}); err != nil {
		t.Fatalf("UpdateClientGroup: %v", err)
	}
	got, err := store.GetClientGroup(group.ID)
	if err != nil {
		t.Fatalf("GetClientGroup: %v", err)
	}
	if got.AutoInstallFile != "debian/preseed.cfg" {
		t.Fatalf("expected group auto install file to persist, got %q", got.AutoInstallFile)
	}

	if err := store.UpdateClientGroup(group.ID, &models.ClientGroup{Name: "fleet"}); err != nil {
		t.Fatalf("UpdateClientGroup clear: %v", err)
	}
	got, err = store.GetClientGroup(group.ID)
	if err != nil {
		t.Fatalf("GetClientGroup after clear: %v", err)
	}
	if got.AutoInstallFile != "" {
		t.Fatalf("expected group auto install file to clear, got %q", got.AutoInstallFile)
	}
}
