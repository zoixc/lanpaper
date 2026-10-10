# Disaster-recovery runbook

This runbook is for operators restoring Lanpaper after host loss, accidental
delete, damaged metadata, a full disk, or a bad upgrade. Practice it before an
incident. Commands assume `/srv/lanpaper/data`; adapt the path and service name.

## Recovery objectives and ownership

Choose and record an RPO (maximum acceptable data loss) and RTO (maximum
acceptable downtime). A reasonable small installation starts with daily
backups, 30 days retention and a quarterly restore drill. Upload frequency may
require much shorter intervals.

One writer owns a data directory. Never start a second Lanpaper writer against
the live volume during backup, restore, repair or rollback.

## What must be backed up

Back up the **entire `data/` tree as one unit**, including:

- JSON or SQLite metadata and SQLite sidecar files;
- `media/`, `previews/`, `history/` and `items/`;
- `sessions.json`, migration checkpoints, repair journals and quarantine data.

Also preserve outside the data backup, in a secret manager:

- the exact image digest or binary version;
- `ADMIN_PASSWORD_HASH`, publish keys and TLS private keys;
- non-secret configuration and reverse-proxy configuration.

A panel link-list export is not a backup: it contains neither media nor all
operational state. Do not back up only `wallpapers.json` or only a SQLite file.

## Create and verify a backup

A stopped backup is the simplest guaranteed-consistent procedure:

```sh
docker compose stop -t 35 lanpaper
sudo tar --xattrs --acls --numeric-owner \
  -C /srv/lanpaper -czf /backups/lanpaper-$(date -u +%Y%m%dT%H%M%SZ).tar.gz data
sha256sum /backups/lanpaper-*.tar.gz > /backups/SHA256SUMS.tmp
mv /backups/SHA256SUMS.tmp /backups/SHA256SUMS
docker compose up -d lanpaper
curl -fsS http://127.0.0.1:8080/health/ready
```

For near-zero downtime, use an atomic filesystem/volume snapshot and archive
the snapshot, not the changing live directory. Merely tarring a busy volume is
not equivalent. Encrypt off-host backups and test that retention cannot be
modified with the application's credentials.

## Restore drill and real restore

Restore into an empty staging directory first. Never unpack over live data.

```sh
sha256sum -c /backups/SHA256SUMS
sudo install -d -m 0700 -o 100 -g 101 /srv/lanpaper-restore
sudo tar --numeric-owner -C /srv/lanpaper-restore -xzf /backups/lanpaper-TIMESTAMP.tar.gz
sudo chown -R 100:101 /srv/lanpaper-restore/data
```

Start the **same immutable image digest** against the restored directory on a
non-public port. Keep automation and publish keys disabled during validation.
Then check:

```sh
curl -fsS http://127.0.0.1:18080/health/ready
/path/to/lanpaper audit --root /srv/lanpaper-restore/data --json > restore-audit.json
```

Review the audit, open representative public/token/auth links, verify history
and playlist media, and compare record/media counts with the source or the last
drill. Exit status 0 means the audit is clean; 1 means findings exist; 2 means
the audit itself could not complete.

For cutover: stop the live service, rename (do not delete) its current data
directory, atomically rename the validated restore into place, check uid
100/gid 101 ownership, start, and test `/health/ready` plus representative
links. Keep the displaced directory until acceptance and the next backup.
Record elapsed time and achieved RPO/RTO after every drill.

## Damaged or inconsistent data

1. Stop the writer and make a forensic copy/snapshot before changing anything.
2. Run `lanpaper audit --root DIR --json`; preserve its output.
3. Run `lanpaper repair --dry-run` and review every proposed operation.
4. Apply only the reviewed plan with `lanpaper repair --apply`.
5. Preserve `data/quarantine/`, metadata backups and `repair-*.jsonl` journals.
6. Re-run audit and perform a service smoke test.

Repair quarantines rather than silently deleting. Missing live media or
structurally invalid metadata generally requires restore from a known-good
backup. For SQLite, also use the documented integrity/backup tooling and follow
[`SQLITE_MIGRATION.md`](SQLITE_MIGRATION.md); never edit database pages or copy
only the main file while WAL sidecars may be active.

## Disk-full response

A full disk can prevent metadata commits, session revocation, uploads and
SQLite checkpoints.

1. Stop uploads/automation and, if writes continue, stop Lanpaper.
2. Capture logs and `df -h`/`df -i`; determine whether bytes or inodes ran out.
3. Free space outside `data/` first (old unrelated logs/images). Do not manually
   delete media, metadata, history, quarantine or SQLite sidecars.
4. Snapshot the data directory, run audit, then use supported retention/delete
   operations or restore to a larger volume.
5. Restart and confirm readiness, a durable test update, and a clean audit.

Alert before exhaustion; leave headroom for an upload, preview, metadata atomic
replacement, history move and backup at the same time.

## Credential and session rotation after compromise

- Replace `ADMIN_PASSWORD_HASH` and all `PUBLISH_KEYS` in the secret manager.
- Rotate TLS keys if their host or backup confidentiality is in doubt.
- Restart Lanpaper with the new environment. Changing the administrator
  credential fingerprint invalidates persisted browser sessions; also use
  “Sign out on all devices” when the old service remains trustworthy.
- Revoke old proxy/cloud credentials separately and inspect access logs.
- Never restore compromised active credentials merely because they were in an
  older backup.

## Bad release and version rollback

Pin production to verified image digests. Before upgrading, retain the old
digest and take a verified backup. If rollback is needed:

1. stop the new version;
2. preserve its data and logs;
3. read release and migration notes—do not run an older binary against a data
   format it cannot understand;
4. restore the pre-upgrade backup when downgrade compatibility is not explicit;
5. start the old verified digest and repeat readiness, audit and link tests.

Never point two versions at the same writable directory. JSON-to-SQLite staging
is non-destructive, but its downgrade/rollback procedure remains the authority
for that migration.

## Drill record

For each quarterly drill record: backup identifier and checksum, source and
restore image digests, start/end times, achieved RPO/RTO, audit result, sampled
links, record/media counts, credential handling, failures found, and an owner
and due date for every corrective action.
