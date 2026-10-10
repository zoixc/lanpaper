// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"lanpaper/storage"
)

func runSQLiteMigration(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("migrate-sqlite", flag.ContinueOnError)
	flags.SetOutput(stderr)
	source := flags.String("source", "data/wallpapers.json", "source JSON metadata")
	destination := flags.String("destination", "data/wallpapers.db", "destination SQLite database")
	checkpoint := flags.String("checkpoint", "", "checkpoint path (defaults beside destination)")
	backup := flags.String("backup", "", "backup path (default is timestamped beside source)")
	batchSize := flags.Int("batch-size", 250, "records per durable transaction (1-1000)")
	dryRun := flags.Bool("dry-run", false, "validate and checksum without writing")
	resume := flags.Bool("resume", false, "resume a matching checkpoint")
	rollback := flags.Bool("rollback", false, "remove the destination and checkpoint; source/backup remain")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	options := storage.SQLiteMigrationOptions{
		Source: *source, Destination: *destination, Checkpoint: *checkpoint,
		Backup: *backup, BatchSize: *batchSize, DryRun: *dryRun, Resume: *resume,
	}
	if *rollback {
		if *dryRun || *resume {
			fmt.Fprintln(stderr, "migrate-sqlite: rollback cannot be combined with dry-run or resume")
			return 2
		}
		if err := storage.RollbackSQLiteMigration(options); err != nil {
			fmt.Fprintf(stderr, "migrate-sqlite rollback: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "SQLite migration rolled back; JSON source and backup were not changed.")
		return 0
	}
	report, err := storage.MigrateJSONToSQLite(context.Background(), options)
	if err != nil {
		_ = json.NewEncoder(stdout).Encode(report)
		fmt.Fprintf(stderr, "migrate-sqlite: %v\n", err)
		return 1
	}
	_ = json.NewEncoder(stdout).Encode(report)
	return 0
}
