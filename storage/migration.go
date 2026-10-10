// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"lanpaper/internal/atomicfile"
	"lanpaper/utils"
)

type SQLiteMigrationOptions struct {
	Source      string
	Destination string
	Checkpoint  string
	Backup      string
	BatchSize   int
	DryRun      bool
	Resume      bool
}

type SQLiteMigrationReport struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Checksum    string `json:"checksum"`
	Backup      string `json:"backup,omitempty"`
	Total       int    `json:"total"`
	Migrated    int    `json:"migrated"`
	ResumedAt   int    `json:"resumedAt"`
	DryRun      bool   `json:"dryRun"`
	Complete    bool   `json:"complete"`
}

type sqliteMigrationCheckpoint struct {
	Version     int    `json:"version"`
	Checksum    string `json:"checksum"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Backup      string `json:"backup"`
	Next        int    `json:"next"`
	Total       int    `json:"total"`
}

func migrationDefaults(options SQLiteMigrationOptions) SQLiteMigrationOptions {
	if options.Source == "" {
		options.Source = dataFile
	}
	if options.Destination == "" {
		options.Destination = filepath.Join(filepath.Dir(options.Source), "wallpapers.db")
	}
	if options.Checkpoint == "" {
		options.Checkpoint = options.Destination + ".migration.json"
	}
	if options.BatchSize <= 0 {
		options.BatchSize = 250
	}
	if options.BatchSize > 1000 {
		options.BatchSize = 1000
	}
	return options
}

func readMigrationSource(path string) ([]byte, map[string]*Wallpaper, []string, string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, "", err
	}
	sum := sha256.Sum256(body)
	checksum := hex.EncodeToString(sum[:])
	wallpapers := make(map[string]*Wallpaper)
	if err := json.Unmarshal(body, &wallpapers); err != nil {
		return nil, nil, nil, "", err
	}
	if wallpapers == nil {
		return nil, nil, nil, "", errors.New("source is not a wallpaper object")
	}
	names := make([]string, 0, len(wallpapers))
	for name, wallpaper := range wallpapers {
		if wallpaper == nil || !utils.IsValidLinkName(name) || (wallpaper.HasImage && !validStoredMediaExt(wallpaper.MIMEType)) {
			return nil, nil, nil, "", fmt.Errorf("invalid wallpaper entry %q", name)
		}
		wallpaper.ID, wallpaper.LinkName = name, name
		sanitizeMediaLists(wallpaper)
		derivePaths(wallpaper)
		names = append(names, name)
	}
	sort.Strings(names)
	return body, wallpapers, names, checksum, nil
}

func durableCopy(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(destination)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func writeMigrationCheckpoint(path string, checkpoint sqliteMigrationCheckpoint) error {
	body, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return atomicfile.Write(atomicfile.OSFS{}, path, ".migration-*.json", body, 0o600)
}

func readMigrationCheckpoint(path string) (*sqliteMigrationCheckpoint, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var checkpoint sqliteMigrationCheckpoint
	if err := json.Unmarshal(body, &checkpoint); err != nil {
		return nil, err
	}
	return &checkpoint, nil
}

func MigrateJSONToSQLite(ctx context.Context, options SQLiteMigrationOptions) (SQLiteMigrationReport, error) {
	options = migrationDefaults(options)
	_, wallpapers, names, checksum, err := readMigrationSource(options.Source)
	report := SQLiteMigrationReport{Source: options.Source, Destination: options.Destination, Checksum: checksum, Total: len(names), DryRun: options.DryRun}
	if err != nil {
		return report, err
	}
	if options.DryRun {
		report.Complete = true
		return report, nil
	}

	checkpoint := &sqliteMigrationCheckpoint{Version: 1, Checksum: checksum, Source: options.Source, Destination: options.Destination, Total: len(names)}
	if options.Resume {
		existing, readErr := readMigrationCheckpoint(options.Checkpoint)
		if readErr != nil {
			return report, fmt.Errorf("resume checkpoint: %w", readErr)
		}
		if existing.Checksum != checksum || existing.Source != options.Source || existing.Destination != options.Destination || existing.Total != len(names) {
			return report, errors.New("migration checkpoint does not match source or destination")
		}
		checkpoint = existing
	} else {
		if _, err := os.Stat(options.Checkpoint); err == nil {
			return report, errors.New("migration checkpoint already exists; use resume or rollback")
		}
		if _, err := os.Stat(options.Destination); err == nil {
			return report, errors.New("destination already exists")
		}
		backup := options.Backup
		if backup == "" {
			backup = fmt.Sprintf("%s.pre-sqlite-%s.bak", options.Source, time.Now().UTC().Format("20060102T150405Z"))
		}
		if err := durableCopy(options.Source, backup); err != nil {
			return report, fmt.Errorf("backup source: %w", err)
		}
		_, _, _, backupChecksum, err := readMigrationSource(backup)
		if err != nil || backupChecksum != checksum {
			return report, errors.New("backup checksum verification failed")
		}
		checkpoint.Backup = backup
		if err := writeMigrationCheckpoint(options.Checkpoint, *checkpoint); err != nil {
			return report, err
		}
	}
	report.Backup, report.ResumedAt = checkpoint.Backup, checkpoint.Next

	store, err := OpenSQLiteWallpaperStore(ctx, options.Destination)
	if err != nil {
		return report, err
	}
	defer store.Close()
	for checkpoint.Next < len(names) {
		if err := contextError(ctx); err != nil {
			return report, err
		}
		end := checkpoint.Next + options.BatchSize
		if end > len(names) {
			end = len(names)
		}
		batch := names[checkpoint.Next:end]
		err := store.Transact(ctx, func(tx WallpaperTransaction) error {
			for _, name := range batch {
				if _, exists := tx.Get(name); exists {
					continue
				}
				if err := tx.Create(wallpapers[name]); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return report, err
		}
		checkpoint.Next = end
		report.Migrated = end - report.ResumedAt
		if err := writeMigrationCheckpoint(options.Checkpoint, *checkpoint); err != nil {
			return report, err
		}
	}
	if err := store.IntegrityCheck(ctx); err != nil {
		return report, err
	}
	report.Complete = true
	if err := os.Remove(options.Checkpoint); err != nil && !os.IsNotExist(err) {
		return report, err
	}
	return report, nil
}

func RollbackSQLiteMigration(options SQLiteMigrationOptions) error {
	options = migrationDefaults(options)
	checkpoint, err := readMigrationCheckpoint(options.Checkpoint)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if checkpoint != nil && checkpoint.Destination != options.Destination {
		return errors.New("checkpoint destination mismatch")
	}
	for _, path := range []string{options.Destination, options.Destination + "-wal", options.Destination + "-shm", options.Checkpoint} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
