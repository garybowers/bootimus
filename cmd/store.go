package cmd

import (
	"log"
	"os"

	"bootimus/internal/storage"

	"github.com/spf13/viper"
)

// openStore opens the storage backend for one-shot CLI subcommands (migrate,
// profiles, etc.). It mirrors the backend-selection logic used by `serve`:
// PostgreSQL when db.host is configured, otherwise a local SQLite database
// under data_dir. Migrations are applied so the command works against a fresh
// database. The caller is responsible for calling Close on the returned store.
func openStore() storage.Storage {
	dataDir := viper.GetString("data_dir")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory %s: %v", dataDir, err)
	}

	var store storage.Storage
	var err error

	if pgHost := viper.GetString("db.host"); pgHost != "" {
		dbCfg := &storage.Config{
			Host:     pgHost,
			Port:     viper.GetInt("db.port"),
			User:     viper.GetString("db.user"),
			Password: viper.GetString("db.password"),
			DBName:   viper.GetString("db.name"),
			SSLMode:  viper.GetString("db.sslmode"),
		}
		store, err = storage.NewPostgresStore(dbCfg)
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
	} else {
		store, err = storage.NewSQLiteStore(dataDir)
		if err != nil {
			log.Fatalf("Failed to initialize SQLite store: %v", err)
		}
	}

	if err := store.AutoMigrate(); err != nil {
		log.Fatalf("Failed to run database migrations: %v", err)
	}

	return store
}
