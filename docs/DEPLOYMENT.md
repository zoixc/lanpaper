# Deploying Lanpaper in production

This is the operational companion to [SECURITY.md](../SECURITY.md) (threat
model, trust boundaries, reverse-proxy examples) and [API.md](API.md). It
covers what to decide before the first request, how to run the process, and
what to watch afterwards.

Lanpaper is a single static binary with local metadata storage and no required
external services. That makes the deployment small, but it also means the operator owns three
things the application cannot do alone: **TLS in front of it, backups of
`data/`, and the network policy that decides who can reach it.**

---

## 1. Decide the topology

| Topology | When to use it | What Lanpaper does |
| --- | --- | --- |
| Reverse proxy terminates TLS (**recommended**) | Any internet-facing or multi-tenant deployment | Set `TRUSTED_PROXY` to the address the proxy connects from (IP/CIDR, or a comma-separated list). The proxy must set `X-Forwarded-For` (see [SECURITY.md](../SECURITY.md#reverse-proxy)); Lanpaper reads only its rightmost entry and ignores `X-Real-IP`. Leave `TLS_*` unset |
| Lanpaper terminates TLS | Single host, no proxy available, internal CA | Set `TLS_CERT_FILE` + `TLS_KEY_FILE` (both or neither — a partial configuration refuses to start) |
| Plain HTTP | LAN only, or a proxy that already enforces auth | Keep the default and never expose the port to the internet |

Only one process should write to a given `data/` directory. Replicas can *read*
the same directory (rotation is stateless, and metadata is renamed atomically,
so a reader never sees a half-written file), but two writers would overwrite
each other's changes: each process keeps its own in-memory copy of the library
and writes it back, so the last commit wins and the other update is lost. Scale
by putting a cache in front, not by running several writers.

## 2. Configuration checklist

Minimum for an internet-facing instance:

```sh
ADMIN_USER=ops
# Generate offline with: printf '%s' 'a unique password' | ./lanpaper hash-password
ADMIN_PASSWORD_HASH='$argon2id$v=19$…'      # store the complete PHC value in a secret manager
TRUSTED_PROXY=10.0.0.1                      # or the proxy's CIDR; a list is allowed: "10.0.0.1,172.18.0.1"
MAX_UPLOAD_MB=50                            # what a client may push, 1–512
HISTORY_LIMIT=3                             # 0 = no archives, i.e. pre-0.12 behaviour
HISTORY_MAX_MB=512                          # global disk budget for archives
PLAYLIST_MAX=8
RATE_PUBLIC_PER_MIN=120
RATE_UPLOAD_PER_MIN=20
PUBLISH_KEYS="$(openssl rand -hex 24)"      # only if automation publishes
# CORS_ORIGINS=https://dash.example.com     # only if a site must read media from JavaScript
# ALLOW_EMBED=true                          # only if media must be framable
```

Notes that matter in production:

- **Secrets come from a secret manager or a mode-0600 environment file**, never
  from source control or `config.json`. Prefer `ADMIN_PASSWORD_HASH`; deprecated
  plaintext `ADMIN_PASS` exists only for migration. `PUBLISH_KEYS` is also not
  serialized. TLS *paths* may live in `config.json`, but key material must not.
- `DISABLE_AUTH=true` only makes sense behind a proxy that authenticates every
  request. With it enabled, `auth`-level links are refused (403) rather than
  served, because Lanpaper can no longer tell who is asking.
- `REMOTE_INSECURE_SKIP_VERIFY=true` disables certificate validation for URL
  downloads. It is logged loudly at startup and should not survive a review.
- Every out-of-range value is clamped and logged, so a typo degrades one
  setting instead of the service.

## 3. Run it

### Docker

```sh
docker run -d --name lanpaper \
  -p 127.0.0.1:8080:8080 \
  -v /srv/lanpaper/data:/app/data \
  -e ADMIN_USER -e ADMIN_PASSWORD_HASH -e TRUSTED_PROXY=172.17.0.1 \
  -e GOMEMLIMIT=768MiB \
  --read-only --tmpfs /tmp:rw,noexec,nosuid,size=64m,mode=1777 \
  --cap-drop ALL --security-opt no-new-privileges:true \
  --pids-limit 128 --memory 1g --cpus 2 \
  --stop-timeout 35 --restart unless-stopped \
  ptabi/lanpaper:latest
```

The image runs as uid 100 / gid 101. `/app/data` is the sole required writable
persistent path; create it with that ownership and back it up as one unit.
`/tmp` is bounded and non-executable. The limits above are a starting point:
size memory and CPU for your configured upload/decode limits, then monitor
pressure rather than removing the bounds. A built-in `HEALTHCHECK` probes
`/health`; stop timeout 35 seconds gives the server's 30-second graceful drain
room to complete before Docker kills it.

The maintained [docker-compose-example.yml](../docker-compose-example.yml)
binds to loopback for a host reverse proxy, requires an Argon2id hash, and
encodes the same filesystem, privilege, PID, resource and shutdown boundaries.
For a proxy in the same Compose project, remove `ports`, attach both services to
an internal network and use `expose: ["8080"]`; only the proxy should publish
host ports. Do not set `DISABLE_AUTH` merely because a reverse proxy terminates
TLS—it is valid only when that proxy authenticates every request.

### systemd (binary)

```ini
[Unit]
Description=Lanpaper
After=network-online.target

[Service]
Type=simple
User=lanpaper
Group=lanpaper
WorkingDirectory=/srv/lanpaper
EnvironmentFile=/etc/lanpaper/lanpaper.env
ExecStart=/usr/local/bin/lanpaper
Restart=always
RestartSec=3

# Hardening: the process needs its working directory and nothing else.
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/srv/lanpaper/data
ProtectKernelTunables=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
MemoryDenyWriteExecute=true
SystemCallArchitectures=native
LimitNOFILE=8192

[Install]
WantedBy=multi-user.target
```

`MemoryDenyWriteExecute` is safe here: the binary is pure Go — no JIT, no cgo,
no shared libraries — so it never asks for writable-executable memory. If a
future dependency needs it, drop that one line rather than the rest.

### Shutdown and upgrades

`SIGTERM`/`SIGINT` stops the listener and waits up to 30 s for in-flight
uploads and metadata writes, so a rolling upgrade does not truncate a file.
Upgrade = stop, replace the binary or image, start; `data/` is forward
compatible (see *Backups and upgrades* in the [README](../README.md)). Rebuild
the image from the current base regularly so Alpine security fixes reach it.
Keep one copy of the previous image tag around: downgrading is safe as long as
you did not start using history or playlists, and those directories are simply ignored
by older versions.

## 4. Observe it

| Signal | Where | Meaning |
| --- | --- | --- |
| `GET /health` | unauthenticated, no disk I/O | process is alive |
| `GET /health/ready` | unauthenticated | `data/` and `data/media/` reachable, ≥1 GB free; 503 otherwise |
| stdout/stderr | `log.Printf`, one line per event | uploads, rejections, lockouts, pruning, archive trimming, panics |
| `stats` in `/api/wallpapers` | in memory since start | hits and bytes per link; resets on restart |

Point your log collector at the container/journal output. Lines worth an alert:

- `Security: … locked out after N failed login attempts` — brute force, or a
  misconfigured client retrying with a wrong password.
- `Security: rejected cross-origin …` — CSRF attempt, or a proxy that
  rewrites `Host` / is missing from `TRUSTED_PROXY`. The line prints
  `RemoteAddr` and both host names, so the value to trust can be copied
  straight out of it.
- `Panic serving …` — a bug; the request was answered with 500 and the stack is
  in the same log line. The service keeps running.
- `Critical: background prune recovered from a panic` — maintenance pass
  aborted; it is retried on the next upload.
- `History budget: dropped N archived version(s)` — archives are being trimmed,
  i.e. `HISTORY_MAX_MB` is smaller than the churn; raise it or lower
  `HISTORY_LIMIT`.
- `Warning: …` at startup — a configuration value was ignored or clamped.

There is no metrics endpoint on purpose: per-link counters are in memory and
exposed through the authenticated API, so nothing about traffic is readable
without credentials.

## 5. Capacity

- **Disk** ≈ live media + previews (≈30 KB per image link) + archives
  (bounded by `HISTORY_MAX_MB`) + playlist items (bounded by `PLAYLIST_MAX` ×
  link count). `MAX_IMAGES` and the history budget are the two knobs that keep
  a shared volume from filling up; `/health/ready` fails below 1 GB free.
- **Memory** is dominated by image decoding, not by the link count. A global
  budget (`MaxDecodedPixelsInFlight`) caps concurrent decodes and
  `MAX_CONCURRENT_UPLOADS` (1–8) caps parallel uploads; a request that arrives
  while the budget is busy gets `429` with `Retry-After: 5`. Metadata is one
  small struct per link.
- **CPU** spikes on upload (decode + scale + encode + thumbnail) and on
  `POST /api/regenerate-previews` (whole library, two previews at a time,
  rate limited like uploads).
  Serving media is `sendfile`: cheap, and range requests cost one syscall path.
- **Writes**: every metadata mutation is one SQLite transaction in
  `data/wallpapers.db` that writes only the changed link and its history,
  playlist and rotation rows (WAL mode, `synchronous=FULL`). Writers are
  queued, but readers keep serving from memory while the write is in progress.
  The cost of one change no longer grows with the library; the whole library is
  still held in memory for reads. Do not script thousands of mutations per
  second against one instance.

## Storage audit

Run `./lanpaper audit` with the application stopped or against a filesystem
snapshot. Exit status 0 means clean, 1 means issues were found, and 2 means the
audit could not complete. Use `--json` for automation and `--root DIR` when the
Lanpaper data directory is mounted elsewhere. The command never mutates files.

For recoverable findings, stop Lanpaper and review `./lanpaper repair
--dry-run`. Apply exactly that plan with `./lanpaper repair --apply`. Files are
moved to timestamped `data/quarantine/` directories rather than deleted;
metadata is backed up and every operation is recorded in a fsynced
`data/repair-*.jsonl` journal. Keep both until a subsequent audit and service
smoke test pass. Missing live media and structurally invalid records require
manual restore from backup.

## 6. Back up and restore

Back up the complete `data/` tree after a graceful stop, or archive an atomic
volume snapshot when downtime is unacceptable. A plain tar of a changing live
volume is not guaranteed to keep metadata, media and SQLite sidecars from the
same point in time. Keep encrypted backups outside the container's writable
layer and perform a quarterly restore drill.

The step-by-step [disaster-recovery runbook](DISASTER_RECOVERY.md) covers backup
scope and checksums, staged restore and audit, corruption/repair, disk-full
response, credential rotation and version rollback.

## 7. Pre-launch checklist

- [ ] TLS terminates somewhere, and `TRUSTED_PROXY` matches that hop
- [ ] `ADMIN_PASSWORD_HASH` contains a generated Argon2id PHC value; credentials are in a secret store, not in git
- [ ] Deprecated plaintext `ADMIN_PASS`/`adminPass` has been removed after migration
- [ ] `data/` is on a volume that survives container replacement, and is backed up
- [ ] `MAX_UPLOAD_MB`, `MAX_IMAGES`, `HISTORY_MAX_MB`, `PLAYLIST_MAX` reflect the intended use
- [ ] `DISABLE_AUTH` is `false` unless the proxy authenticates every request
- [ ] `REMOTE_INSECURE_SKIP_VERIFY` and `PROXY_INSECURE_SKIP_VERIFY` are `false`
- [ ] `CORS_ORIGINS`/`ALLOW_EMBED` are set only if a real consumer needs them
- [ ] Publish keys are long and random. Rotating one means restarting with a
      new `PUBLISH_KEYS`; per-key revocation is on the [roadmap](../ROADMAP.md)
- [ ] `/health` and `/health/ready` are wired into the orchestrator
- [ ] The startup log has been read once: every `Warning:` is understood
- [ ] Log output is collected, and the alerts in §4 exist

## 8. Licensing

Lanpaper is MIT licensed — see [LICENSE](../LICENSE) and
[THIRD-PARTY-NOTICES.md](../THIRD-PARTY-NOTICES.md) for the (permissive-only)
dependencies it ships with. Commercial and SaaS use, modification and
redistribution are permitted; the license grants no trademark rights and comes
without warranty.
