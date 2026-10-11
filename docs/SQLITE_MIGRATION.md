# JSON-to-SQLite migration

The server now uses SQLite for metadata. On its first start with an existing
`data/wallpapers.json` and no `data/wallpapers.db`, it imports the JSON file
automatically: it verifies the checksum, makes a durable backup named
`wallpapers.json.pre-sqlite-<UTC time>.bak`, writes the database in batches and
keeps the original JSON file. An interrupted import is resumed on the next start
from its checkpoint.

Upgrading to 0.17.0 also moves sign-in sessions into the same database. The
first start imports `data/sessions.json` once and keeps the file as a backup.

This page describes the manual `migrate-sqlite` command, which is still useful
for dry runs, for checking an installation before upgrading, and for staging a
database somewhere other than the default location. The command is explicit and
offline and never deletes `wallpapers.json`.

## Before migrating

1. Stop the Lanpaper server so metadata cannot change during migration. The
   command takes `data/.lanpaper.lock` for every run except `--dry-run`, and
   it refuses to write while a server is running.
2. Back up the entire `data/` directory, including media, history and playlist
   files. The migration only changes metadata storage.
3. Run a validation pass:

```sh
lanpaper migrate-sqlite --dry-run \
  --source data/wallpapers.json \
  --destination data/wallpapers.db
```

The command prints a JSON report with the source SHA-256 and record count. A dry
run creates no database, checkpoint or backup.

## Migrate

```sh
lanpaper migrate-sqlite \
  --source data/wallpapers.json \
  --destination data/wallpapers.db \
  --batch-size 250
```

Before opening SQLite, the command creates and fsyncs a timestamped byte-for-byte
backup and verifies its checksum. It then imports deterministic name-sorted
batches into normalized SQLite transactions. `wallpapers.json` remains
untouched for downgrade and independent recovery.

A checkpoint beside the database records source checksum, backup, destination
and the next batch. If the process is interrupted, do not edit the JSON source;
resume with:

```sh
lanpaper migrate-sqlite --resume \
  --source data/wallpapers.json \
  --destination data/wallpapers.db
```

Resume refuses a changed source, destination or record count. A batch committed
just before interruption is safe to replay: existing records are recognized,
and the checkpoint advances only after a durable transaction.

After all batches, the command runs `PRAGMA integrity_check` and removes the
checkpoint. Media bytes are never copied or moved.

## Rollback and downgrade

**Partial staging output.** If you ran the manual command and want to discard
its output before starting the server on it:

```sh
lanpaper migrate-sqlite --rollback \
  --source data/wallpapers.json \
  --destination data/wallpapers.db
```

Rollback removes only the staged database, its WAL/SHM files and the checkpoint.
It does not touch the JSON source or its pre-migration backup.

**Going back to a release before 0.17.0.** Those releases read only
`data/wallpapers.json` and `data/sessions.json`. After the upgrade neither file
changes: metadata is written only to `data/wallpapers.db`, and sessions only to
its `sessions` table. An older release therefore starts from the state at the
time of the upgrade. Choose one of:

1. Restore the whole `data/` directory from a backup taken before the upgrade.
   This is the only option that keeps the old state exactly.
2. Keep the newer state by hand: list the links through the API before you go
   back and re-create the changes in the older release.

Users signed in after the upgrade will have to sign in again, because the older
release does not see the `sessions` table.

Do not run an old binary against a `data/` directory that has changed since the
upgrade, and never run two versions against the same directory at once.
