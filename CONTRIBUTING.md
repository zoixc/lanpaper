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
  an existing `wallpapers.json` (imported into SQLite on first start) and every
  existing URL keep working. Do not add new writes to the JSON file: the
  database is the only writer. New
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
node tools/check-contrast.mjs
docker build -t lanpaper .
```

CI runs all of that on Go `oldstable` and `stable`, then builds and smoke-tests
the image. A pull request is merged when every check is green.

If you change `static/js/app.js` or `static/css/style.css`, bump `STATIC_CACHE`
in `static/sw.js` in the same commit — installed clients cache those assets.
Update the cache-generation assertion in `tests/frontend-contract.test.cjs` too.
Restart the server after changing static files: asset version hashes are computed
at startup, and versioned URLs are served with immutable caching.

### Editing the panel design

`static/css/style.css` is the production source of truth. `design/v2/` is a
historical prototype, not a second stylesheet to keep editing in parallel.
Start with the `:root` tokens, then the dark-theme and palette overrides. Common
spacing and component paddings have dedicated tokens; optical adjustments and
control geometry intentionally keep literal values. See [the CSS editing guide](docs/STYLE_GUIDE.md)
for the token map and responsive exceptions. Preserve reduced-motion rules,
focus indicators, mobile scrolling, and the login wordmark's aspect ratio.

Run `node tools/check-contrast.mjs --table` to inspect the production token pairs
in all six palettes (including mono) and both themes. It also reads the actual
select-arrow SVG strokes. A failed or unsupported colour exits nonzero; the old
`node design/v2/tools/check.mjs` command delegates to this same check. This is a
focused static audit of opaque hex colours in the existing root/theme/palette
rules, **not** a complete accessibility audit: alpha compositing, media-query
contrast overrides and arbitrary CSS selectors need browser testing. The 1.1:1
decorative-border threshold is a design guard, not WCAG control conformance.

Run the Node tests above and the browser suite in `tests/e2e` before merging.
Animation-value and wordmark contracts also live in
`tests/frontend-contract.test.cjs` and `session_test.go`; changing their design
intentionally requires reviewing those tests, not just deleting assertions.
Animation checks tolerate formatting changes.

Development-only CSS tooling is pinned in `tools/package-lock.json`:

```sh
npm ci --prefix tools --ignore-scripts
npm run format --prefix tools  # formats only static/css/style.css
npm run check --prefix tools   # formatting, structural/token guards, tool tests
```

CI runs the same checks. Do not auto-sort properties or merge media queries:
source order and fallback declarations are intentional. No CSS build step is
needed to run or deploy the application.

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
