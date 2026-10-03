<!-- SPDX-License-Identifier: MIT -->
# Contributing

Thanks for considering a change to Lanpaper. The bar is deliberately high: this
is a service people point e-ink frames, dashboards and dashboards-of-dashboards
at, and a URL that starts answering different bytes is worse than a missing
feature.

## Before you open a pull request

- **Read [SECURITY.md](SECURITY.md) first.** It lists the controls that already
  exist; a change that weakens one of them (a header, an access check, a
  validation) will not be merged as-is.
- **Keep the invariants.** They are documented per package in the `doc.go`
  files. The four that come up most often:
  1. A response writer that wraps another one must delegate `ReadFrom` and
     `Unwrap`, or media loses `sendfile` and uploads lose their deadlines.
  2. Never mutate a slice you got from `Store.Get`/`GetAll` — snapshots are
     shared. Build a new slice inside an `Update` closure.
  3. Never read `config.Current` from a background goroutine; pass the value in.
  4. Validate before doing work, and authorise before resolving a selector.
- **Backwards compatibility is a requirement.** An existing `data/` directory,
  an existing `wallpapers.json` and every existing URL keep working. New
  persisted fields are `omitempty` (and pointers, if a struct — `encoding/json`
  never omits an empty struct), and a new default must not change what an
  upgraded installation does on disk.
- **Watch the resource cost.** Prefer a rename over a copy, arithmetic over a
  goroutine, an existing background pass over a new timer. If a feature needs a
  budget, give it one (`HISTORY_MAX_MB` and `PLAYLIST_MAX` are the models).

## Development

```sh
gofmt -l .                  # must print nothing
go vet ./...
go test -race ./...         # -race is not optional: config and counters are global
node --check static/sw.js && node --test tests/*.test.cjs
docker build -t lanpaper .
```

CI runs all of that on Go `oldstable` and `stable`, then builds and smoke-tests
the image. A pull request is merged when every check is green.

If you change `static/js/app.js` or `static/css/style.css`, bump `STATIC_CACHE`
in `static/sw.js` in the same commit — installed clients cache those assets.

Conventions worth following: table-driven tests next to the code they cover, an
end-to-end test in `features_test.go` for anything reachable through the router,
error strings that say what to do next, and comments that explain *why* a
decision was made (the "what" is in the code).

## Reporting a vulnerability

Do not open a public issue. Follow the private channel described in
[SECURITY.md](SECURITY.md#reporting-a-vulnerability).

## Licensing of contributions

Lanpaper is MIT licensed (see [LICENSE](LICENSE) and
[THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md)). By submitting a change you
agree to license it under the same MIT terms. If the project ever moves to a
dual or open-core licence, contributions made under MIT stay MIT — which is why
maintainers may ask for a contributor agreement before a relicensing effort
starts.
