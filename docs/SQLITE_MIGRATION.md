# JSON-to-SQLite migration

The server now uses SQLite for metadata. On its first start with an existing
`data/wallpapers.json` and no `data/wallpapers.db`, it imports the JSON file
automatically: it verifies the checksum, makes a durable backup named
`wallpapers.json.pre-sqlite-<UTC time>.bak`, writes the database in batches and
keeps the original JSON file. An interrupted import is resumed on the next start
from its checkpoint.

This page describes the manual `migrate-sqlite` command, which is still useful
for dry runs, for checking an installation before upgrading, and for staging a
database somewhere other than the default location. The command is explicit and
offline and never deletes `wallpapers.json`.

## Before migrating

1. Stop the Lanpaper server so metadata cannot change during migration.
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

Before application configuration is switched to SQLite, rollback partial output:

```sh
lanpaper migrate-sqlite --rollback \
  --source data/wallpapers.json \
  --destination data/wallpapers.db
```

Rollback removes only the SQLite database, WAL/SHM files and checkpoint. It does
not remove or rewrite the JSON source or its pre-migration backup.

This command stages and validates SQLite metadata; it does not switch the
running server's backend. Downgrade at this roadmap stage therefore means
removing the staged SQLite files and continuing with the untouched JSON source.
Do not start independent writers against both metadata files. Media files stay
compatible because both backends use the same filesystem layout.

## Going back to a JSON-only version

Versions before the SQLite backend read only `data/wallpapers.json`. After the
upgrade that file is a snapshot from the moment of import, so those versions
would show stale data and would not see later changes. To go back, stop the
server, then either restore the whole `data/` directory from a backup taken
before the upgrade, or copy the newest state out of the database and into
`wallpapers.json` with `recovery` tooling. Do not run an old binary against a
directory that has changed since the upgrade.
