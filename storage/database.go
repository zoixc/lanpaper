// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// importLegacyMetadata brings an installation that still has only
// wallpapers.json into the SQLite database. It is a no-op once the database
// exists and no import is pending.
func importLegacyMetadata(ctx context.Context) error {
	return importLegacyMetadataAt(ctx, databaseFile, dataFile)
}

func importLegacyMetadataAt(ctx context.Context, dbPath, jsonPath string) error {
	checkpoint := dbPath + ".migration.json"
	dbExists := fileExists(dbPath)
	jsonExists := fileExists(jsonPath)
	switch {
	case fileExists(checkpoint):
		// An earlier import stopped midway. Resume it; the checkpoint already
		// pins the source checksum, so a changed JSON file is refused.
		if !jsonExists {
			return fmt.Errorf("import checkpoint %s exists but %s is missing", checkpoint, jsonPath)
		}
		report, err := MigrateJSONToSQLite(ctx, SQLiteMigrationOptions{Source: jsonPath, Destination: dbPath, Resume: true, Checkpoint: checkpoint})
		if err != nil {
			return fmt.Errorf("resume metadata import: %w", err)
		}
		log.Printf("Metadata import resumed: %d records, backup %s", report.Total, report.Backup)
		return nil
	case dbExists || !jsonExists:
		return nil
	}
	_, wallpapers, _, _, err := readMigrationSource(jsonPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", jsonPath, err)
	}
	// Validate before writing anything, so a bad legacy file stops the start
	// without creating a half-populated database.
	if _, err := prepareRecords(wallpapers, jsonPath); err != nil {
		return err
	}
	report, err := MigrateJSONToSQLite(ctx, SQLiteMigrationOptions{Source: jsonPath, Destination: dbPath})
	if err != nil {
		return fmt.Errorf("import %s: %w", jsonPath, err)
	}
	log.Printf("Imported %d wallpaper records from %s into %s; original kept as %s", report.Total, jsonPath, dbPath, report.Backup)
	return nil
}

// ReadMetadataRecords returns the persisted records under a Lanpaper working
// directory without starting the server. It prefers the database and falls
// back to the legacy JSON file. Records are returned as stored, without the
// startup validation, so the recovery tools can report what is wrong with them.
func ReadMetadataRecords(root string) (map[string]*Wallpaper, error) {
	dbPath := filepath.Join(root, databaseFile)
	if fileExists(dbPath) {
		db, err := OpenSQLiteWallpaperStore(context.Background(), dbPath)
		if err != nil {
			return nil, err
		}
		defer db.Close()
		return db.loadAll(context.Background())
	}
	return readLegacyMetadata(filepath.Join(root, dataFile))
}

// MetadataDatabaseExists reports whether a working directory already uses the
// SQLite metadata backend.
func MetadataDatabaseExists(root string) bool {
	return fileExists(filepath.Join(root, databaseFile))
}

// RepairMetadataDatabase replaces the stored records with records. backupPath
// receives a consistent copy of the database first, so a bad repair can be
// undone by hand. Only changed rows are written, in one transaction.
func RepairMetadataDatabase(root string, records map[string]*Wallpaper, backupPath string) error {
	ctx := context.Background()
	dbPath := filepath.Join(root, databaseFile)
	db, err := OpenSQLiteWallpaperStore(ctx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o700); err != nil {
		return err
	}
	if err := db.Backup(ctx, backupPath); err != nil {
		return fmt.Errorf("backup database: %w", err)
	}
	prev, err := db.loadAll(ctx)
	if err != nil {
		return err
	}
	return db.syncRecords(ctx, prev, records)
}

func readLegacyMetadata(path string) (map[string]*Wallpaper, error) {
	_, wallpapers, _, _, err := readMigrationSource(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]*Wallpaper{}, nil
		}
		return nil, err
	}
	return wallpapers, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
