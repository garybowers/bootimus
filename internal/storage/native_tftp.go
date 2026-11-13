package storage

import (
	"errors"
	"net"
	"strings"
	"time"

	"bootimus/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNativeTFTPBootFileExists = errors.New("native TFTP boot file is already assigned to another client")

func nativeTFTPWriteError(err error) error {
	// Both SQLite and PostgreSQL identify the unique column/index in this error.
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") && strings.Contains(err.Error(), "native_tftp_boot_file") {
		return ErrNativeTFTPBootFileExists
	}
	return err
}

// A completed transfer is evidence of a boot attempt, not successful OS startup.
// Persist the 30-second retry window so concurrent transfers and server restarts
// cannot inflate the count. The guarded UPDATE takes the write lock before any
// reads, and the log and counter commit together.
func recordNativeTFTPBoot(db *gorm.DB, filename, remoteAddr string, now time.Time) (bool, error) {
	if filename == "" || models.ValidateNativeTFTPBootFile(filename) != nil {
		return false, nil
	}
	if ip, _, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteAddr = ip
	}
	recorded := false
	err := db.Transaction(func(tx *gorm.DB) error {
		var client models.Client
		result := tx.Model(&client).
			Clauses(clause.Returning{Columns: []clause.Column{{Name: "id"}, {Name: "mac_address"}}}).
			Where("native_tftp_boot_file = ?", filename).
			Where("native_tftp_last_boot IS NULL OR native_tftp_last_boot <= ?", now.Add(-30*time.Second)).
			Updates(map[string]interface{}{
				"native_tftp_last_boot": now,
				"last_boot":             now,
				"boot_count":            gorm.Expr("boot_count + 1"),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		entry := models.BootLog{
			Source:    models.BootSourceNativeTFTP,
			CreatedAt: now, ClientID: &client.ID, MACAddress: client.MACAddress,
			ImageName: "Native TFTP transfer: " + filename, IPAddress: remoteAddr, Success: true,
		}
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		recorded = true
		return nil
	})
	return recorded && err == nil, err
}
