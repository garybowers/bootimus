package storage

import (
	"log"

	"bootimus/internal/models"

	"gorm.io/gorm"
)

type clientBootloaderSelection struct {
	ClientBootloaderSet string
	GroupBootloaderSet  string
}

func getClientBootloaderSets(db *gorm.DB, mac string) (clientSet, groupSet string, err error) {
	var selection clientBootloaderSelection
	err = db.Table("clients").
		Select("clients.bootloader_set AS client_bootloader_set, client_groups.bootloader_set AS group_bootloader_set").
		Joins("LEFT JOIN client_groups ON client_groups.id = clients.client_group_id AND client_groups.deleted_at IS NULL").
		Where("clients.mac_address = ? AND clients.deleted_at IS NULL", mac).
		Take(&selection).Error
	if err != nil {
		return "", "", err
	}
	return selection.ClientBootloaderSet, selection.GroupBootloaderSet, nil
}

func hardDeleteClient(db *gorm.DB, client *models.Client) error {
	if err := db.Exec("DELETE FROM client_images WHERE client_id = ?", client.ID).Error; err != nil {
		return err
	}
	if err := db.Model(&models.BootLog{}).Where("client_id = ?", client.ID).Update("client_id", nil).Error; err != nil {
		return err
	}
	if err := db.Model(&models.HardwareInventory{}).Where("client_id = ?", client.ID).Update("client_id", nil).Error; err != nil {
		return err
	}
	return db.Unscoped().Delete(client).Error
}

func deleteClientByMAC(db *gorm.DB, mac string) error {
	var client models.Client
	if err := db.Where("mac_address = ?", mac).First(&client).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	return hardDeleteClient(db, &client)
}

func cleanupSoftDeletedClients(db *gorm.DB) error {
	var clients []models.Client
	if err := db.Unscoped().Where("deleted_at IS NOT NULL").Find(&clients).Error; err != nil {
		return err
	}
	for i := range clients {
		if err := hardDeleteClient(db, &clients[i]); err != nil {
			return err
		}
	}
	if len(clients) > 0 {
		log.Printf("Cleaned up %d soft-deleted clients from database", len(clients))
	}
	return nil
}
