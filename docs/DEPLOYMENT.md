# Deploying Lanpaper in production

This is the operational companion to [SECURITY.md](../SECURITY.md) (threat
model, trust boundaries, reverse-proxy examples) and [API.md](API.md). It
covers what to decide before the first request, how to run the process, and
what to watch afterwards.

Lanpaper is a single static binary with no database and no external services.
That makes the deployment small, but it also means the operator owns three
things the application cannot do alone: **TLS in front of it, backups of
`data/`, and the network policy that decides who can reach it.**

---

## 1. Decide the topology

| Topology | When to use it | What Lanpaper does |
| --- | --- | --- |
| Reverse proxy terminates TLS (**recommended**) | Any internet-facing or multi-tenant deployment | Set `TRUSTED_PROXY` to the proxy's IP/CIDR so forwarded headers are believed; leave `TLS_*` unset |
| Lanpaper terminates TLS | Single host, no proxy available, internal CA | Set `TLS_CERT_FILE` + `TLS_KEY_FILE` (both or neither — a partial configuration refuses to start) |
| Plain HTTP | LAN only, or a proxy that already enforces auth | Keep the default and never expose the port to the internet |

Only one process should write to a given `data/` directory. Replicas can *read*
the same directory (rotation is stateless, and metadata is renamed atomically,
so a reader never sees a half-written file), but two writers would overwrite
each other's `wallpapers.json` — the last rename wins and the other update is
lost. Scale by putting a cache in front, not by running several writers.

## 2. Configuration checklist

Minimum for an internet-facing instance:

```sh
ADMIN_USER=ops
ADMIN_PASS="$(openssl rand -base64 24)"     # a short password only warns; the lockout is the real defence
TRUSTED_PROXY=10.0.0.1                      # or the proxy's CIDR
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

- **Secrets come from the environment**, never from `config.json`.
  `ADMIN_PASS` and `PUBLISH_KEYS` are deliberately not serialized; the TLS
  *paths* may live in `config.json`, the key file itself must not.
- `DISABLE_AUTH=true` only makes sense behind a proxy that authenticates every
  request. With it enabled, `auth`-level links are refused (403) rather than
  served, because Lanpaper can no longer tell who is asking.
- `INSECURE_SKIP_VERIFY=true` disables certificate validation for URL
  downloads. It is logged loudly at startup and should not survive a review.
- Every out-of-range value is clamped and logged, so a typo degrades one
  setting instead of the service.

## 3. Run it

### Docker

```sh
docker run -d --name lanpaper \
  -p 127.0.0.1:8080:8080 \
  -v /srv/lanpaper/data:/app/data \
  -e ADMIN_USER -e ADMIN_PASS -e TRUSTED_PROXY=172.17.0.1 \
  --read-only --tmpfs /tmp \
  --security-opt no-new-privileges:true \
  --restart unless-stopped \
  ptabi/lanpaper:latest
```

The image runs as uid 100 / gid 101 and only `/app/data` needs to be writable.
A `HEALTHCHECK` against `/health` is built in. See
[docker-compose-example.yml](../docker-compose-example.yml) for the compose
variant, including a proxy.

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

`MemoryDenyWriteExecute` is safe here: the WebP encoder is compiled C, not a
JIT. If a future dependency needs writable-executable memory, drop that one
line rather than the rest.

### Shutdown and upgrades

`SIGTERM`/`SIGINT` stops the listener and waits up to 30 s for in-flight
uploads and metadata writes, so a rolling upgrade does not truncate a file.
Upgrade = stop, replace the binary or image, start; `data/` is forward
compatible (see *Backups and upgrades* in the [README](../README.md)). Keep one
copy of the previous image tag around: downgrading is safe as long as you did
not start using history or playlists, and those directories are simply ignored
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
- `Security: rejected cross-origin …` — CSRF attempt, or a missing
  `TRUSTED_PROXY` after a proxy change.
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
  `POST /api/regenerate-previews` (whole library, rate limited like uploads).
  Serving media is `sendfile`: cheap, and range requests cost one syscall path.
- **Writes**: every metadata mutation rewrites `data/wallpapers.json`
  atomically (temp file → `fsync` → `rename` → directory `fsync`). That is
  O(links) per mutation and the reason the file stays human-readable; it is
  fine into the tens of thousands of links, but do not script thousands of
  mutations per second against one instance.

## 6. Back up and restore

```sh
# Consistent enough for a live instance: metadata is renamed atomically,
# media files are never modified in place.
tar -C /srv/lanpaper -czf "lanpaper-$(date +%F).tar.gz" data
```

Restore = stop the service, unpack `data/`, start. Verify with
`GET /health/ready` and one known link. If you cannot afford any window at all,
snapshot the volume instead of tarring it.

Keep the backup **outside** the container's writable layer and test a restore
once per quarter: an untested backup is a rumour.

## 7. Pre-launch checklist

- [ ] TLS terminates somewhere, and `TRUSTED_PROXY` matches that hop
- [ ] `ADMIN_PASS` is long and random; credentials are in a secret store, not in git
- [ ] `data/` is on a volume that survives container replacement, and is backed up
- [ ] `MAX_UPLOAD_MB`, `MAX_IMAGES`, `HISTORY_MAX_MB`, `PLAYLIST_MAX` reflect the intended use
- [ ] `DISABLE_AUTH` is `false` unless the proxy authenticates every request
- [ ] `INSECURE_SKIP_VERIFY` is `false`
- [ ] `CORS_ORIGINS`/`ALLOW_EMBED` are set only if a real consumer needs them
- [ ] Publish keys are long, random, and rotatable (add the new key, migrate
      clients, remove the old one — keys are compared by digest, so rotating
      needs no restart order)
- [ ] `/health` and `/health/ready` are wired into the orchestrator
- [ ] The startup log has been read once: every `Warning:` is understood
- [ ] Log output is collected, and the alerts in §4 exist

## 8. Licensing

Lanpaper is MIT licensed — see [LICENSE](../LICENSE) and
[THIRD-PARTY-NOTICES.md](../THIRD-PARTY-NOTICES.md) for the (permissive-only)
dependencies it ships with. Commercial and SaaS use, modification and
redistribution are permitted; the license grants no trademark rights and comes
without warranty.
