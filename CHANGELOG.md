# Changelog

Notable changes to Lanpaper. Docker images are published as
`ptabi/lanpaper:<version>` and `ptabi/lanpaper:latest`.

## [Unreleased]

Security hardening from an internal audit. Behaviour changes are listed under
**Changed**; upgrading is a container restart.

### Security

- **Client address comes only from the rightmost `X-Forwarded-For` entry.**
  `X-Real-IP` is no longer read at all. Before, a client could send
  `X-Real-IP` through a trusted proxy that does not overwrite it and appear as
  any address, which bypassed the `local` access level and the per-client rate
  limit and login lockout.
- **Publish keys cannot replace existing media.** `POST /api/upload` with a
  publish key and `mode=replace` (the default) on a link that already has media
  now returns `403`. Keys can still create links, upload a first file and append
  playlist items. Admin uploads are unchanged.
- **MP4 detection checks the brand.** A file is stored as MP4 only if its `ftyp`
  box declares a known video brand as major or compatible brand. Before, any
  file with `ftyp` at offset 4 was accepted and served as `video/mp4`.
- **Runtime image requires zlib 1.3.2-r1** (CVE-2026-85091). The Alpine base
  ships 1.3.2-r0.

### Changed

- `GET /health` no longer returns the `version` field.
- Access-token generation fails the request instead of ignoring a failed read
  from the system random source.
- Static assets referenced by the admin page carry a content hash (`?v=`) and
  are cached for a year. Other URLs still revalidate.
- The external gallery listing is cached for 10 seconds.
- Preview regeneration runs two previews at a time.
- Proxy setup: configure `X-Forwarded-For` (append or overwrite) instead of
  `X-Real-IP`. The nginx example is updated; the Caddy example no longer needs
  `header_up X-Real-IP`.

### Performance

- `GET /api/wallpapers` no longer copies every record on each request.
- Metadata writes no longer block readers while the file is written.
- The rate limiter's counters are split across 64 locks.
- Upload forms are streamed to disk instead of being parsed into memory first.

## [0.15.0] – 2026-10-09

A visual release for the 2.0 panel: a monochrome default with the colour kept
in the accent palettes, a new minimalist logo, and a tidier header, list and
card layout. No data or URL behaviour changes; the palette a user has already
chosen is kept. Users who never picked one see the new Mono default. Upgrading
is a container restart.

### Changed

- **New minimalist logo**: a network mark (one node linked to four) in the
  favicon, the header, the logo files and the PWA icons.
- **The panel is monochrome by default.** The accent and the status colours
  are neutral; colour comes only from the Accent palettes in Settings. Mono is
  the default, and Indigo, Sage, Clay, Graphite and Ocean keep their colours.
  Destructive actions stay red.
- **Softer shadows** on cards, the header and the buttons.
- **Header controls share one height**: the grid/list switch, the theme and
  settings buttons and "New link" sit on the same line.
- **The list/grid switch animates**: a sliding highlight under the active view
  and a short fade of the cards.
- **The card menu (⋯) is always visible** as a bordered button at the end of
  the card's title row.
- **List rows are aligned in columns**: metadata, tags, access and the menu sit
  on the same vertical lines in every row.

### Fixed

- **The search field on phones is full width**, so its placeholder is no longer
  cut off; filters sit on the line below and still scroll.
- **The metadata line on phones and in tiles** no longer cuts a value in the
  middle: the type and size stay whole, the resolution is hidden in the phone
  tile, and the date is hidden on phones as before.
- **The "1 in playlist" badge on a thumbnail** shrinks with an ellipsis instead
  of overflowing the frame.

## [0.14.1] – 2026-10-08

A fix release for the 2.0 panel: the strings it looked up but never had, the
phone layout, the header, the typography and one reverse-proxy setting — plus a
pass over the whole application that repaired dragging files onto the panel,
bulk pinning, the version links, the access counters and the design stand. Two
query parameters of `GET /api/wallpapers` are validated instead of ignored and
the manifest is served with its own media type; no data or URL behaviour
changes, upgrading is a container restart.

### Fixed

- **Twenty-two interface strings no longer show as raw keys.** The 2.0 panel
  asks for `sc_search`, `sc_new`, `sc_theme`, `sc_view`, `sc_select`,
  `sc_close` (settings sheet), `access_public_hint`, `access_local_hint`,
  `access_token_hint`, `access_auth_hint` (access levels), `filter_all`,
  `filter_playlist`, `filter_pinned` (filter chips), `pin`, `unpin`,
  `bulk_pin`, `bulk_unpin`, `pinned_toast`, `unpinned_toast`,
  `upload_success`, `append_success` and `search_placeholder_short`, none of
  which existed in `static/i18n/*.json`. Without a dictionary entry `t()`
  falls back to the key itself, so the sheet, the chips, the card menu and the
  toasts printed identifiers instead of words. All six languages now carry
  all 219 keys, and the phone-width placeholder is two characters shorter on
  purpose.
- **`tests/i18n.test.cjs` finds this class of defect again.** It matched only
  `t('key')` and `data-i18n` attributes, while the panel builds its labels
  from arrays, ternaries and lookup tables (`['t', 'sc_theme']`,
  `t(pinned ? 'unpin' : 'pin')`, `t(SORT_LABELS[…])`, `t('palette_' + name)`).
  The test now inventories every snake_case literal in `admin.html`,
  `static/js/*.js` and `static/sw.js`, checks the argument list of every
  `t(…)` call, walks the domain of keys assembled from a constant
  (`PALETTES`), and verifies the narrow-screen `*_short` variants. Keys that
  deliberately are not translations are listed with a reason.
- **The accent swatch draws its selection ring around the swatch.** The
  hidden radio was `position: absolute; inset: 0` without explicit sizes; an
  `input` is a replaced element, so it stayed at its intrinsic ~15 px in the
  top-left corner and the ring was drawn there instead of around the 34 px
  circle.
- **The phone layout no longer scrolls sideways.** Four segment tabs
  (media/versions/playlist/access) and the three theme options could not
  shrink below their text width and pushed the page wider than the screen:
  the rows scroll on their own now (`max-width: 560px`), and `html` clips
  accidental horizontal overflow (`overflow-x: clip`, which — unlike
  `hidden` — keeps the sticky header working). The header itself lost the
  app name and the 44 px tap targets below 400 px, where five icon buttons
  and the wordmark no longer fit in one line, and the drag-and-drop frame
  caps its width and breaks long file names.
- **Header buttons have no tooltips any more.** A `[data-tip]::after` above a
  sticky header is pushed off the top edge and, at the right edge, widened the
  document — the source of the sideways scroll on desktop. `aria-label`
  stays, so the buttons still have accessible names.
- **The settings and theme icons are the 0.12.1 ones again.** The hand-drawn
  gear (`ICONS.gear`) was a twelve-pointed star with uneven radii that read as
  crooked at 17 px; it and the moon (outer and inner arc of different radii,
  8.5 and 8.6) are the Feather outlines the 0.12.1 panel used.

- **Dragging files onto the panel uploads them again.** `static/js/app.js`
  declared `createLinksFromFiles` twice: the second, optimistic definition — no
  request at all, `hasImage: true`, an empty `imageUrl` — won by hoisting, so a
  dropped file produced a phantom tile named after the first path segment, no
  `/api/upload` call, and a `404` after the reload. The dead copy is gone; the
  surviving one is what the drop handler, the empty state and `#filePicker`
  reach.
- **Bulk pin and unpin change the links the button names.** The filter was
  `l.pinned !== !anyUnpinned`, which selected the links that were *already* in
  the wanted state: «Закрепить» pinned the pinned ones and left the rest
  untouched, and «Открепить» (a fully pinned selection) did nothing at all.
  Only the selected links whose `pinned` still differs from the label are
  toggled now.
- **The Open button of an archived version opens that version.** Every history
  row called `openMedia(link)` and therefore served the current file; the row
  opens `/{name}?v=N` now, while the live row keeps the plain URL.
- **Access counters survive a rename.** `storage.RenameStats` moves the
  counters with the link (and merges them when the new name still had counters
  from a deleted link), so a renamed link no longer reads as «Обращений пока не
  было» and a new link under the old name cannot inherit another link's
  traffic. Covered by the new `storage/stats_test.go`.
- **The counter map's bound works again.** `RecordHit` read `LoadOrStore`'s
  second result as "created" while it actually reports "already existed", so
  `trackedLinks` grew only on the race path while `ForgetStats` decremented it
  on every deletion: the number drifted negative and the 10 000-name cap never
  triggered.
- **A link without media no longer shows 1 January 1970.** `modTime` is `0` for
  those records and `formatRelative(0)` fell through to the date formatter.
  `formatDate` and `formatRelative` return «—» for a zero stamp — the same
  marker `formatBytes` uses.
- **The rotation interval is formatted in the reader's language.**
  `toFixed(1).replace('.', ',')` put a comma into the English panel («1,5 h»);
  the value goes through `Intl.NumberFormat` now.
- **The panel tells image, PNG and video apart by `mimeType`, not by
  `category`.** `GET /api/wallpapers` reports the stored file extension in
  `mimeType` (`png`, `mp4`) and the *user* category in `category`
  (`tech`/`life`/`work`/`other` — and `other` is what the panel's own create
  dialog stores), but the panel compared `mimeType` with `image/png` and read
  the media kind out of `category`. A link with a file therefore printed «—»
  instead of `PNG` or `MP4`, a transparent PNG was cropped (`cover`) and got no
  checkerboard background, the «Видео» chip and the play badge stayed empty for
  every video made in the panel, and such a tile pulled the video itself into
  an `<img>` through `/api/preview/{name}` — the server keeps no poster for a
  video and answers with the file, which could only fail. The kind, the
  checkerboard, the chips and the frame now all come from `mimeType`, and a
  video frame is never requested.
- **`design/v2` has its fonts back.** The stand still pointed at the deleted
  `manrope-*`/`unbounded-*` files, rendered in a system font and made
  `tools/build-standalone.py` exit with `FileNotFoundError`; it uses Golos Text
  again and `standalone.html` is rebuilt (816 KB, no external requests).
- **The theme button and the gear move exactly the way 0.12.1 moved them.**
  The button keeps both glyphs stacked (`svg.theme-icon`, 17 px, absolutely
  positioned) and the visible one is chosen by CSS from `html[data-theme]`,
  which `prepaint.js` sets before the first paint; the leaving glyph goes to
  `scale(0.5) rotate(-90deg)` while the arriving one returns to
  `scale(1) rotate(0)` (0.2 s opacity, 0.25 s transform). The gear turns 90°
  while the settings sheet is open, driven by `body.settings-open`, which
  `openOverlay()`/`closeOverlay()` toggle from the `data-sheet` attribute. An
  earlier attempt in this series swapped `data-icon` at runtime and pulsed it
  with an `is-swap` keyframe; both are gone. The sun is still the 0.12.1
  (Feather) outline — circle r=4.5 and eight rays from r=8.5 to r=10.5 — and
  the settings sheet labels its three modes with a sun, a moon and a monitor.
- **The remaining icons answer the pointer.** Every icon button lifts its glyph
  slightly on hover and presses it down on click; `+` folds over, the refresh
  arrows half-turn, the trash tips, the copy glyph hops after a successful
  copy, the selection check grows out of the centre and the video play mark
  breathes. Every animation is short, sits in the stylesheet (the panel runs
  under a CSP without inline styles) and is switched off by
  `prefers-reduced-motion`. The gear, the sun and the moon deliberately keep
  the 0.12.1 motion instead of a hover gesture.
- **Browser chrome, document language and shortcuts follow the panel.** Three
  more defects from a second audit pass. (1) `<meta name="theme-color">` had
  only `media`-attributed variants, so a manually chosen dark panel kept a
  light status bar on phones; `applyTheme()` — and `prepaint.js`, before the
  first paint — now writes the effective colour into both tags. (2) `<html
  lang>` stayed `en` for a Russian or German UI because only `setLang()` wrote
  it; `applyTranslations()` owns the attribute now. (3) The keyboard shortcuts
  fired behind an open dialog: pressing `n` while the create form was open
  called `openCreateDialog()` again, `openOverlay()` closed the previous
  dialog, and everything typed into it was lost. The handler returns early
  while an overlay is up (Esc and Tab belong to the overlay). The service
  worker precache generation moved to `lanpaper-static-v9` so offline copies of
  the changed stylesheet and scripts are replaced on the next visit.
- **The settings gear announces whether its sheet is open.** The button carries
  `aria-expanded`/`aria-controls`, both kept in step with the sheet, so a
  screen reader can hear the state it toggles.
- **A large file is no longer cut off on a slow link.** Every response ran
  under the server's 120-second write timeout, which is sized for an API call:
  a 50 MB video at 256 KiB/s needs longer, and the connection was closed
  mid-file. Media responses — the public file, its `?v=`/`?i=` variants, the
  panel preview and the gallery file — now extend their own deadline to fit the
  size they have to send (at 256 KiB/s, capped at 15 minutes), so a client that
  reads slowly still receives the whole file, while one that stops reading
  loses the connection exactly as before. Covered by
  `handlers/public_test.go:TestMediaWriteBudgetFollowsTheFileSize`.
- **HEIF and AVIF photos are no longer stored as videos.** The `ftyp` shortcut
  that accepts camera MP4 brands (`isom`, `iso2`, `avc1`, `M4V`…) also accepted
  the ISO-BMFF image brands: a photo from an iPhone or an image downloaded as
  `.avif` was stored as `.mp4` and served as `video/mp4` — a tile no browser can
  play, while the build before that shortcut simply refused it. The image
  brands (`avif`, `avis`, `heic`, `heix`, `heim`, `heis`, `hevc`, `hevx`,
  `hevm`, `hevs`, `mif1`, `msf1`, `msix`, `mshf`) are named and rejected again with `400 Invalid
  or unsupported media file`; MP4 brands are untouched. Covered by
  `handlers/upload_test.go:TestInspectMediaFileAcceptsAllMP4Brands`.
- **The panel no longer claims a success it does not have.** `execCommand("copy")`
  returns a boolean and its result was ignored, so a refused copy still said
  «Скопировано»; the bulk button called that per link and then toasted a total,
  printing «Скопировано» twenty-one times for a twenty-link copy. `copyText()`
  now returns the real result, the bulk copy prints one toast and only when the
  clipboard agreed. The upload toast lived 1600 ms — shorter than any real
  upload, which made the panel look idle and invited a second send — and is held
  until the request finishes. Bulk delete dropped every selected name from the
  list whatever the server answered; a refused `DELETE` left a link that still
  exists on disk hidden until a reload. Covered by two contract tests
  («copying reports the truth…», «a link the server refused to delete stays on
  screen»).

### Changed

- **The interface is set in Golos Text.** Manrope (latin + cyrillic) and
  Unbounded (logo only) — 39 KB + 9 KB — are replaced by one variable
  Golos Text 400–900 in the same two subsets, 60 KB, SIL OFL 1.1, fetched
  from `@fontsource-variable/golos-text`. The display face is the same family
  set tighter, so `--font-display` no longer loads a second file; headings and
  the wordmark carry a little more weight and less tracking instead. The
  service worker cache generation moved with the assets
  (`lanpaper-static-v8` here, `lanpaper-static-v9` after the icon and theme
  work below), and `tests/sw.test.cjs` now derives the fonts it expects from
  the stylesheet's `@font-face` rules instead of naming them.
- **A trusted proxy that forwards no `X-Forwarded-Proto` no longer turns the
  panel into a wall of `403`s.** Such a proxy leaves the external scheme
  unknown (the browser sees `https://host`, the connection to the server is
  plain HTTP), and the CSRF check compared the Origin's scheme with `http`.
  When the proxy is listed in `TRUSTED_PROXY` and the browser itself marks the
  request as same-origin (`Sec-Fetch-Site: same-origin`), a matching host and a
  port that fits the other scheme are accepted now; cross-site, same-site,
  other-host, other-port and header-less requests are rejected exactly as
  before, and a forwarded `X-Forwarded-Proto` still wins over the guess.
  `config.ApplyTrustedProxy` is public so a test (or a future settings reload)
  can refresh the parsed list.
- **`TRUSTED_PROXY` accepts a comma-separated list of IPs and CIDRs.** One
  address was not enough as soon as a proxy reached the container through more
  than one hop: a Docker container sees the bridge gateway, not the proxy's LAN
  address, so `TRUSTED_PROXY="192.168.20.1,172.24.0.1"` is what makes rate
  limiting, the login lockout and the `local` access level see the real client
  IP. Entries that do not parse are dropped with a warning naming the entry,
  the remaining ones stay in effect, and `IsTrustedProxy` matches against any
  of them. The CSRF rejection line now prints `X-Forwarded-Host` as well and
  names the address to add (`set TRUSTED_PROXY=172.24.0.1`), because that
  `403` is almost always a proxy that rewrites `Host` rather than an attack.
- **`GET /api/wallpapers` validates its filters.** `has_image=1` used to mean
  `false` (`want := hasImg == "true"`, and every other value read as false) and
  an unknown `sort=…` silently sorted by creation date. `has_image` accepts
  `1/0/true/false` and `sort` accepts `created/updated`; anything else is a
  `400` (`Invalid has_image`, `Invalid sort`).
- **The manifest is served as `application/manifest+json`** — the `.json`
  extension gave it `application/json`, which Chrome warns about — and its
  `theme_color` and `background_color` are the panel's `#f5f6f9` instead of
  `#f3f2f7`.
- **The concurrent-upload `429` carries `Retry-After: 5`**, as `docs/API.md`
  already promised, and a non-`POST` probe of `/api/upload` or
  `/api/regenerate-previews` no longer spends the upload rate budget: the
  method is checked before the limiter.
- **`POST /api/link/{name}` answers `405` with `Allow: PATCH, DELETE`** instead
  of `404` — the path is routed, only the method is wrong.
- **One Basic-auth realm.** The panel asked for `realm="Admin"` and an
  auth-level link for `realm="Link"`, so a browser could ask twice for the same
  credentials; both send `realm="Admin"` now.
- **The import dialog counts what will be created** — the links missing on the
  server, not every link in the file — says so when nothing is missing, and the
  success toast reports how many links were created; a partial failure says how
  many were not.
- **Selecting several files for one link is no longer silent.** In the playlist
  (`append`) every selected file becomes an item (one request each, in order);
  for a plain replace the first file is used and the toast says how many were
  skipped. The picker keeps `multiple`, because the empty state creates one
  link per file.
- **Dead style from the stand no longer ships.** `style.css` carried rules for
  a history-usage meter, an icon variant of a picker thumbnail and a footer
  spacer that the panel never renders (the versions tab shows a text hint,
  because the server does not publish the history budget), plus the
  `card__frame--failed` hook that no stylesheet ever described. The rules stay
  in the `design/v2` snapshot, which does use the meter.

## [0.14.0] – 2026-10-08

The admin panel is rebuilt on the `design/v2` stand: the same capabilities as
0.12.1 in a quieter, denser interface, plus replacing media from a URL or from
the files already on the server, version rollback without leaving the panel,
five muted accents and six fully translated languages. The Go side carries the
two changes that were already waiting in `main`: WebP encoding in pure Go (no
CGO, no C toolchain in the image) and a rollback for links that `autoCreate`
made before an upload failed.

### Changed

- **The admin panel is rebuilt on the `design/v2` layout.** `admin.html`,
  `static/css/style.css`, `static/js/app.js`, `static/js/export-import.js` and
  `static/js/settings-menu.js` are the port of the second stand: a sticky
  two-line header, filter chips with counts, a 12-tile grid with "Show more", a
  permanent link panel with media/versions/playlist/access tabs, five muted
  accents, light and dark themes and a touch-sized phone layout. Every request
  goes to the same API as before — `GET /api/wallpapers`, `POST /api/link`,
  `PATCH /api/link/{id}` (`newLinkName`, `accessLevel`, `rotate`, `rotateToken`,
  `removeItem`), `DELETE /api/link/{id}`, `/pin`, `/rollback`,
  `/history/{v}`, `POST /api/upload` (device file, remote URL or a file already
  on the server), `/api/external-images`, `/api/preview/{id}`,
  `/api/compression-config`, `/api/regenerate-previews`. The six
  `static/i18n/*.json` files now carry the panel's 197 keys — the same set in
  every language, checked by `tests/i18n.test.cjs`.
- **The panel still lives under the strict CSP.** The inline pre-paint script
  became `static/js/prepaint.js`, and no markup carries a `style` attribute any
  more: `script-src 'self'; style-src 'self'` keeps working without
  `'unsafe-inline'`, and dynamic values (meter fill, accent samples) are set
  through the CSSOM. `static/sw.js` moves to `lanpaper-static-v7` and precaches
  the new file; caches of older versions are still deleted on activation.
- **Three details of the mock-up are done differently, because the API does not
  carry the data.** The version tab shows the link's own archive size and
  `HISTORY_LIMIT` instead of a share of the global `HISTORY_MAX_MB` budget (the
  server does not publish it); the tile metadata shows a frame's real pixel size
  once its preview has loaded, and no video duration (neither is in
  `/api/wallpapers`); the server-file gallery lists names with previews rather
  than dimensions and file sizes (`/api/external-images` returns names only).
- **WebP encoding is pure Go.** `github.com/chai2010/webp` (CGO, a vendored
  libwebp 1.4.0) is replaced by `github.com/SeriousBug/webp-go-pure` v1.2.0
  (MIT, no C toolchain, no shared library). Go 1.26+ can build the project
  without a C compiler, the Docker image needs no `gcc`/`musl-dev` and sets
  `CGO_ENABLED=0`. Memory at the 36 MP limit drops from 350 MB to 174 MB peak
  (283 MB → 100 MB resident) and uploads finish ~28 % faster; thumbnails are
  about 14 % larger and re-encoded media about 9 % larger at the same quality
  setting.
- **A failed upload no longer leaves an empty link behind.** `autoCreate`
  created the link before the media was validated, so a rejected file (or an
  append to a link that has no media yet) kept the new link in the panel until
  it was deleted by hand. The link is now rolled back when the request fails.

### Added

- **A second admin redesign stand: `design/v2/`.** A clickable prototype of the
  panel rebuilt from its tasks rather than patched from the previous mock-up: a
  sticky header with search and filter chips (type, pinned, access, sorting), a
  compact media tile with two controls instead of six, a permanent link panel
  with media/versions/playlist/access tabs, five muted accent palettes, light and
  dark themes and a touch-sized mobile layout. `design/v2/screens.html` shows the
  same screen in live 1440 / 834 / 390 px frames, and
  `design/06-redesign-v2.md` holds the audit of 0.12.1, the contrast table and
  the file-by-file migration plan. `design/v2/standalone.html` (799 KB,
  `tools/build-standalone.py`) is the same mock-up as one file with the styles,
  scripts, fonts and demo frames inlined, for viewers that show a single file.
  The Go code and the panel's own files stay untouched by the stand itself: the
  port to `admin.html` and `static/` is the change listed above, on top of the
  same audit. Contrast of every palette pair is checked by
  `node design/v2/tools/check.mjs`.
- **The prototype also answers what 0.12.1 does not do.** The library is drawn in
  chunks of 12 with a "Show more" button and a "showing 12 of 20" counter instead
  of rendering every tile at once; a failed `/api/wallpapers` request gets its own
  error state with a "Retry" button instead of an empty library; media can be
  replaced from a URL (validated: `https?://`, 2048 characters, known
  extensions) or from the files already on the server, not only by uploading;
  every replacement archives the previous file into the version history up to
  `HISTORY_LIMIT`; the playlist stops at `PLAYLIST_MAX` with the limit spelled
  out before the click; the version tab shows the history budget and disables
  deletion when there is nothing to delete; the link id format is reported as
  you type and the id is checked for availability 250 ms later;
  filter chips carry counts; hotkeys gained `t`, `g` and `s` next to the existing
  ones; and a newly created, renamed or uploaded link is revealed and highlighted
  instead of landing past the rendered window; the sort chip is labelled with
  the chosen order instead of the raw state key; and picking one of the four
  languages the mock-up does not translate yet says so out loud instead of
  silently keeping Russian. Bulk access changes were considered and dropped:
  the API has no batch endpoint, and one button over many links can open more
  than intended. The list of behaviour changes — and what deliberately stays as
  it is — is section 11-бис of `design/06-redesign-v2.md`.
- **The mock-up has some depth back.** Cards were flat: a hairline border and
  nothing else, which read as stickers on the page. Each surface now has its own
  height — the tile rests on a soft shadow along its bottom edge, lifts 2 px and
  widens it under the cursor, the loading skeleton wears the same shadow so the
  grid does not "sink" while it loads, and the sticky header casts a soft shadow
  downwards so the content reads as passing under it rather than being cut by a
  line. In the dark theme a shadow is nearly invisible, so height there is shown
  by a 1 px light lip (`inset 0 1px 0 rgba(255,255,255,.05)`) instead. Accent
  buttons keep a soft shadow of their own colour; nothing glows, and no shadow
  is animated except the tile lift, which is off on touch screens where hover
  sticks after a tap. Tokens: `--shadow-card`, `--shadow-card-hover`,
  `--shadow-bar`, overridden per theme like the rest.
- **The stand was re-read line by line before any of it moves into the app, and
  eleven bugs came out of it.** Eight would have shipped invisibly: the panel
  renderer closed *any* overlay instead of its own (so opening settings while the
  panel was open silently killed the panel, and switching language in the
  settings closed the panel it lived in); translation placeholders were
  substituted with `String.replace`, so a link name containing `$&` or `$'` would
  print a fragment of the template; `Intl` formatters were built on every call —
  320 constructions per 20 list redraws, now zero, cached per language; size units
  and the type/size/changed/version labels were Russian literals shown even in the
  English interface; the chip row was rebuilt wholesale, dropping keyboard focus
  and `scrollLeft`; renaming a link lost its selection and desynchronised the bulk
  counter; an evicted toast kept its timer and touched a removed node; and
  `dismiss` could run twice. The last two came from the harness itself. The same
  pass measured the result — 4–15 ms, typically ~8, of work per 12-card redraw, zero node growth
  over 30 open/close cycles — and confirmed the application needs no security
  rework: SSRF is closed by `ResolvePublicURL` plus a pinned transport, paths go
  through `os.OpenRoot`, public media carry `nosniff` / `X-Frame-Options: DENY` /
  `Referrer-Policy: no-referrer`, tokens are compared in constant time and never
  exported. Two things must change in the app during the port: `formatBytes`
  (`static/js/app.js:2339`) hard-codes `KB/MB/GB` regardless of language, and
  `static/i18n/*.json` has no keys for the panel's Type / Size / Changed /
  Version labels. Details in section 11-тер of `design/06-redesign-v2.md`.
- **Three bugs the mock-up showed only on screen.** Wrapper elements
  (`.choice__body`, `.row__body`, `.vrow__body`) were flex children without a
  `display`, so the `span`s inside stayed inline and the bold title ran straight
  into its description — "PublicAvailable to anyone with the link" — in the
  access options, the "Replace media" rows and the version rows. The panel's
  tabs switch `aria-selected`, but only `aria-pressed` and `aria-checked` were
  styled, so the active tab was never highlighted. And the tab strip had no
  surface or edge of its own while the scrollable body started flush against it,
  so on scroll the content was cut off right under the pill, full-bleed next to
  it. All three are `ui.css` only: wrappers became columns, the selector gained
  `aria-selected`, and the strip got its own bottom border, an 8 px gap and the
  header shadow, with the line under the panel title removed via
  `.sheet__head:has(+ .sheet__tabs)`. A sweep of every container built in
  `app.js` (looking for wrappers whose CSS sets no `display`) and of every
  attribute the script writes found no other case.
- **The mock-up no longer needs `:has()`, and twelve dead rules are gone.**
  It turned out the selected access level and its focus ring hung on
  `:has(input:checked)` — while the application's own stylesheet does not use
  `:has()` even once. Ported as it was, that would have smuggled a new browser
  requirement into the panel, silently: an unsupported selector just shows
  nothing. The state now lives in the `is-checked` class the script was already
  writing (the list re-renders on every change, so the class tracks the radio),
  and the focus ring moved to the sibling selector
  `input:focus-visible + .choice__mark`, the same trick the mock-up's other
  native fields use. The tab strip's `:has(+ .sheet__tabs)` went the same way —
  the strip now covers the header's hairline with its own surface
  (`margin-top: -1px`). A runtime class audit — the sweep opens every state and
  collects the classes actually present, 192 of them — found twelve rules from
  earlier iterations with no markup behind them: five `.badge--*` variants
  (`badge--accent` is the one in use), `.btn--ghost`, the `.dropzone` block with
  its captions, `.foot__hint`, `.is-active` and `.is-over`; all removed, along
  with the dead half of `.icon-btn.is-active`. The audit is repeatable: it also
  catches names built by concatenation, so it cannot mistake a live rule for a
  dead one. Harness grown to match: layout rules jsdom cannot compute (40),
  build checks (30), state checks (33), flow checks (25), all run over the page
  and the single-file build. Measured after the changes: ~8 ms per 12-card
  redraw, zero `Intl` constructions, no node growth over 30 open/close cycles.
- **The mock-up survives a narrow screen and a wide list view.** On a phone the
  header keeps the application name (it used to vanish, leaving a row of icons
  with no clue what panel this is) and the link counter moves to the page title
  instead of disappearing, while the floating "New link" button no longer covers
  the last row — the content reserves 96 px for it. The list view is a single
  column at every width now: the rule was `.grid--list`, one class, and the
  `.grid` column rules in the media queries below it won on specificity, so on a
  wide desktop "list" stayed a grid with the cards merely turned into rows. It
  is `.grid.grid--list` and a real row now — thumbnail on the left, name, meta
  and badges on one line, and the access switch pinned to the right edge at
  176 px instead of stretching across the row. That switch is drawn only where
  the row is wide (≥ 721 px) and replaces the access badge there rather than
  sitting next to it, so the same fact is no longer stated twice; on a phone the
  badge speaks and the switch is gone. A frame whose file fails to load is shown
  as "no file" instead of a torn-image icon.

## [0.12.1] – 2026-10-04

A rollback release. The admin redesign published as 0.13.0 broke the panel
layout in normal use, so the presentation returns to exactly the 0.12.0 state.
Nothing else moved: no Go code changed, and the API, the stored data, the
access levels and every existing URL behave as they did in 0.12.0.

### Changed

- **The admin panel is back to the 0.12.0 interface.** The tile grid, the
  permanent card footer (`Change ▾ · Panel · ⋯`), the previews fitted into a
  16:9 frame, the five accent palettes and the animated icon set are reverted.
  `admin.html`, `static/css/style.css`, `static/js/app.js`,
  `static/js/settings-menu.js`, `static/sw.js` and the six `static/i18n/*.json`
  files are byte-identical to 0.12.0 again; hover actions on the preview, the
  access row in the card and the gallery/list toggle all work as before.
- **The service worker cache generation is `lanpaper-static-v6` again.** An
  installed PWA that picked up `v7` from 0.13.0 deletes that cache when the
  rolled-back `sw.js` activates — `activate` purges every `lanpaper-*` cache
  except the current one — so no stale stylesheet survives the downgrade.
- **0.13.0 is superseded.** `ptabi/lanpaper:0.13.0` still contains the
  redesign; `:latest`, `:0.12` and `:0.12.1` do not. Update to 0.12.1 (or
  re-pull `latest`) to get the working panel back.

### Added

- `design/` stays in the repository: the UI/UX audit, the design tokens and the
  clickable prototypes that the redesign was built from, so a second attempt
  starts from them rather than from scratch. Nothing under `design/` is served
  by the application, and the directory is excluded from the Docker build
  context.
- `tests/i18n.test.cjs` keeps the six translations in step: the same key set in
  every language, no empty or untranslated values, every key referenced by
  `admin.html` and the scripts present in all six files, and `{{placeholder}}`
  names preserved. The checked key list matches the reverted interface.

## [0.12.0] – 2026-10-03

Everything in this release is off by default or backwards compatible: an
existing `data/` directory, an existing `wallpapers.json` and every existing URL
keep working unchanged, and a link that uses none of the new features is stored
and served exactly as before.

### Added

- **Version history and rollback.** Replacing a file archives the one it
  replaced in `data/history/{link}/{version}.{ext}`. Archived versions stay
  reachable as `/{name}?v=2`, are listed by `GET /api/link/{name}/history`, can
  be restored with `POST /api/link/{name}/rollback` and dropped one by one with
  `DELETE /api/link/{name}/history/{version}`. A rollback archives the file it
  displaces, so restoring version 2 of a link at version 4 makes the live file
  version 5 and keeps version 4 — a rollback can always be rolled back. The
  thumbnail is regenerated from the restored file.
  - Defaults: `HISTORY_LIMIT=3` versions per link and `HISTORY_MAX_MB=512` as a
    budget shared by all links. `HISTORY_LIMIT=0` restores the previous
    replace-only behaviour and writes no archive at all.
  - When the budget is exceeded, the oldest archives across every link are
    dropped first, in the background pass that already pruned media. The pass
    recomputes the total from the metadata before deciding, so a missed counter
    update heals itself.
  - Renaming a link moves its archive; deleting a link deletes it.
- **Playlists and rotation.** One URL can now serve several files.
  `POST /api/upload` with `mode=append` stores a file as a playlist item in
  `data/items/{link}/{id}.{ext}` and leaves the live file untouched, so every
  existing embed keeps showing what it showed. Items are addressable as
  `/{name}?i=2` (`?i=0` is always the live file) and removable with
  `PATCH /api/link/{name}` and `{"removeItem":2}`. `PLAYLIST_MAX` (default 8,
  maximum 64) caps the list and answers `409` when it is full.
  - `PATCH /api/link/{name}` also accepts
    `{"rotate":{"enabled":true,"interval":300,"order":"random"}}`. Fields are
    merged, so toggling `enabled` keeps the interval and the order;
    `interval: 0` restores the default of 60 s, and any other value must be
    5–86400 s.
  - Rotation is **stateless**: the position is derived from
    `floor(now / interval)` (sequential) or a stable FNV hash of the link name
    and that window (random). No goroutine, no persisted cursor, no extra
    request-time I/O, the image cannot flicker inside one window, and two
    replicas reading the same `data/` directory always agree.
  - Playlist items get no thumbnail on purpose: a WebP per item would multiply
    upload CPU and disk for metadata nobody looks at.
- **Publishing without the admin login.** `PUBLISH_KEYS` (comma-separated,
  16+ characters, at most 32 keys) authorizes `POST /api/upload` and
  `POST /api/link` only, via `X-Api-Key` or `Authorization: Bearer`. Renaming,
  re-scoping, pinning, listing, rolling back and deleting still need Basic Auth,
  so a leaked key can publish but cannot read or destroy the library.
- **`autoCreate=1` on upload** creates the link and pushes its first file in one
  request — what a webhook or a cron job needs. Optional `category` and
  `accessLevel` fields are validated before the link is created. Without the
  flag an unknown name is still `400`, so a typo cannot silently create a link.
- **CORS for public media.** `CORS_ORIGINS` (comma-separated, `*` for any
  origin) adds `Access-Control-Allow-Origin`, `Vary: Origin` and
  `Access-Control-Expose-Headers` (`ETag`, `Last-Modified`, `Content-Length`,
  `Content-Range`, `Content-Type`) and answers `OPTIONS` preflights with `204`,
  `Access-Control-Allow-Methods: GET, HEAD, OPTIONS`, `Max-Age: 600` and the
  requested headers filtered to an allowlist. A browser canvas or `fetch()` on
  another origin can finally read a public link.
- **`ALLOW_EMBED=true`** drops `X-Frame-Options` and the CSP `sandbox` for
  public media only, so a kiosk page or a dashboard can frame `/{name}`.
  `/admin` and `/api/*` stay unframable whatever this flag says.
- **URL aliases.** `/{name}.jpg`, `/{name}.png`, `/{name}/latest` and
  combinations of the two resolve to `/{name}`, for clients that insist on a
  file extension. Only extensions from the supported media list are aliases, so
  `/manifest.json`, `/favicon.ico`, `/robots.txt` and `/sw.js` keep answering
  `404`. The stored extension still decides `Content-Type` and
  `Content-Disposition`, never the alias.
- **Per-link access counters.** `stats` in the link object reports requests,
  bytes and the last hit since the process started, in memory only: no visitor
  identity, no storage, no cost after a restart. `304` and `HEAD` count as a hit
  with zero bytes; refused requests are not counted.
- **Admin panel:** a per-link *Versions* dialog lists archived versions
  (open / restore / delete), playlist items (open / remove), rotation settings
  and the access counters. The upload menu gained *Add to playlist*, and a card
  shows `v5 · 2 in playlist · rotating · 42 hits` only when the link uses those
  features.

### Changed

- `PATCH /api/link/{name}` with `newLinkName` moves `data/history/{name}/` and
  `data/items/{name}/` along with the media, and rolls the renames back if the
  metadata save fails. `DELETE /api/link/{name}` removes both directories and
  the link's counters.
- `data/items/` is created at startup, `data/history/` only when history is
  enabled. Both are also created on demand, so a failure to create them is a
  warning and not a reason to refuse to start.
- The service worker cache is bumped to `lanpaper-static-v5` so clients pick up
  the new panel assets.

### Security

- Publish keys are compared as SHA-256 digests in constant time, are never
  written to `config.json` and appear in logs only as an 8-hex-character
  fingerprint. A wrong key shares the brute-force budget with admin logins
  (10 failures per client per 15 minutes, then `429` with `Retry-After`), and
  the lockout is not an oracle: the right key is refused while it lasts. On a
  server without `PUBLISH_KEYS` a stray `X-Api-Key` header is ignored entirely
  and cannot lock anybody out.
- `CORS_ORIGINS` entries are parsed as URLs: a scheme of `http`/`https`, no path,
  no credentials and no wildcard subdomains. An origin that is not listed gets
  no CORS headers at all, and `OPTIONS` keeps answering `405` until CORS is
  configured, so the default deployment behaves exactly as before.
- An unusable `?v=` or `?i=` selector is a `404` instead of a silent fallback to
  other bytes, and the access check of the link always runs before any selector
  is resolved, so an alias or a version cannot bypass `token`, `local` or `auth`.
- Archived versions and playlist items are stored under validated link names and
  validated extensions only, in the same `data/` tree as the live media: nothing
  new is reachable from the static web root.

### Performance

- Archiving is a **rename** of the backup that an upload already created, so a
  replacement costs no extra copy and no extra disk read.
- Rotation and version selection are pure arithmetic on data that is already in
  memory; a request for `/{name}` does the same single `ServeContent` call as
  before, and media still uses `sendfile` (the stats counter wraps the response
  writer with a `ReadFrom` delegate).
- Playlist appends skip thumbnail generation, and the item cap is checked before
  any media is downloaded or decoded.
- The history budget runs in the existing prune worker instead of adding a
  timer, and the byte counter is a single atomic.

### Production readiness

- **A panicking handler costs one request, not the process.** A recovery layer
  between the gzip middleware and the router logs the method, the path and the
  stack, and answers a clean `500` when the response has not started yet.
  Nothing from the panic reaches the client, and the query string is never
  logged because token links carry their secret there. `http.ErrAbortHandler`
  is re-raised untouched, so a deliberately abandoned response still behaves
  the way `net/http` documents. The two background goroutines nobody restarts
  (the prune worker and the rate-limit cleaner) recover as well.
- **Optional built-in TLS.** `TLS_CERT_FILE` + `TLS_KEY_FILE` serve HTTPS from
  the process itself with a TLS 1.2 floor; Go's current cipher suite and curve
  defaults apply, and HTTP/2 is negotiated automatically. Setting only one of
  the two is a startup error: refusing to start beats silently serving admin
  credentials and token URLs in plaintext.
- **`/robots.txt`** answers `Disallow: /` instead of `404`, so crawlers stop
  walking mutable media URLs (and stop retrying a path that will never exist).
- **The probes reject write methods** with `405` instead of answering a `POST`
  to `/health` as if it were a `GET`.
- **Startup warnings** for `INSECURE_SKIP_VERIFY=true`, `CORS_ORIGINS=*` and an
  `ADMIN_PASS` shorter than 12 characters, so an internet-facing mistake is
  visible in the log on day one.
- **Package documentation** for `main`, `config`, `storage`, `handlers`,
  `middleware` and `utils`, spelling out the invariants each package relies on
  (copy-on-write snapshots, lock ordering, transparent response writers, no
  global reads on the request path).
- **Licence hygiene:** every first-party source file carries an
  `SPDX-License-Identifier: MIT` header, and
  [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md) lists all bundled
  components (BSD-3-Clause, MIT, SIL OFL 1.1 — no copyleft) with the texts a
  redistribution must reproduce.
- **Release automation:** pushing a version tag runs the full test matrix,
  publishes `linux/amd64` + `linux/arm64` images under the semver tags and
  creates the GitHub release, after checking that the tag matches `VERSION`.
- **[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)** documents the production
  topology, a hardened systemd unit, log lines worth alerting on, capacity
  planning, backups and a pre-launch checklist.

## [0.11.0] – 2026-09-29

### Changed

- **New admin panel design.** An image-first gallery look: large 16:9
  previews in framed tiles, a sticky glass toolbar, rounded pill controls and
  a sunset gradient accent, with matching light and dark themes.
- **New app icon, favicon and logo**: a sunset between two peaks with signal
  arcs. SVG favicon and logo; PNG icons at 192 and 512 px, a maskable 512 px
  icon and a 180 px Apple touch icon.
- **Fonts:** Manrope for the interface (Latin and Cyrillic) and Unbounded for
  link IDs, as WOFF2 subsets with their OFL licences: 48 KB instead of 528 KB
  of TTF. Rubik and the Disket Mono fonts, one of which was never used, are
  removed. The service worker now precaches 211 KB instead of 713 KB.
- **Mobile toolbar:** the search field has its own row; the result counter
  and the sort menu share the next row instead of squeezing into the search
  field.

### Added

- **Previews for icons and unusual aspect ratios.** Small images, and square
  images in formats with transparency (PNG, WebP, GIF), are treated as icons:
  they are shown at their own size on a blurred copy of themselves instead of
  being cropped and enlarged. Portrait, square and panoramic images are shown
  whole, and portrait videos are letterboxed. The server gallery uses the same
  rules.
- **gzip compression** for text responses of at least 1 KiB (HTML, CSS,
  JavaScript, JSON, SVG). The admin page downloads about three times less, and
  a 1,000-link API response shrinks from about 295 KB to 26 KB. Media, range
  requests and `HEAD` responses are never compressed, so media keeps the
  `sendfile` path.

### Performance

- **Uploads use far less memory.** Images are resized without a full-size
  intermediate buffer, and memory is returned to the system as soon as no
  image is being processed. The process drops back to about 10 MB right after
  an upload instead of staying at its peak. Peak memory, before → after:

  | Upload | Default | `COMPRESSION_SCALE=50` |
  | --- | --- | --- |
  | 4K JPEG | 65 → 23 MB | 179 → 31 MB |
  | 24 MP JPEG | 114 → 48 MB | 470 → 71 MB |
  | 36 MP JPEG | 144 → 65 MB | 689 → 99 MB |
  | Two 24 MP JPEGs at once | 218 → 86 MB | 932 → 132 MB |

  Uploads take 5–60 % longer; memory was the priority.
- **Browser caching.** Admin previews and `public`/`local` media are
  revalidated (`Cache-Control: private, no-cache` with `ETag` and
  `Last-Modified`) instead of downloaded again. The access check still runs
  on every request, before any `304`. A repeat visit to a 13-link panel
  transfers about 12 KB instead of 890 KB. `token` and `auth` media stays
  `no-store`.
- **Video previews play only while visible** (at least a quarter on screen,
  tab visible) and never with reduced motion. Offscreen videos are not
  downloaded: the first visit on a phone transfers about 106 KB instead of
  1.36 MB.

### Fixed

- The theme and view toggle icons animate again; the stylesheet targeted
  button IDs that did not exist.
- Long link names end with an ellipsis instead of being cut off.
- "Copy URL" falls back to the legacy clipboard method when the browser
  refuses `navigator.clipboard` (permission policy, unfocused page).
- Server gallery tiles no longer fade in before the file has loaded.

## [0.10.0] – 2026-09-28

### Security

- **Brute-force protection now works.** Before, the limit only changed the
  status of *wrong* guesses to 429, and a correct password was still accepted.
  An attacker could keep guessing, and admin-protected public links were an
  unthrottled password oracle. Now:
  - After 10 failed logins per client in 15 minutes, credentials are no longer
    evaluated until the window ends (`429` + `Retry-After`).
  - The admin API and `token`/`auth` links share the counter.
  - Requests without credentials are not counted.
- **Dependency CVE fixed.** `golang.org/x/image` updated to v0.46.0, which
  fixes CVE-2026-46603 / GO-2026-6222: excessive memory allocation in the
  VP8L (lossless WebP) decoder, reachable through uploads.
- Rate limits and the lockout group IPv6 clients by `/64`, so rotating
  addresses no longer escapes them. Every `429` carries `Retry-After`.
- More special-purpose ranges are blocked for URL downloads: IPv4-compatible
  IPv6, the whole `2001::/23` block, discard-only `100::/64`, site-local
  `fec0::/10` and the 6to4 relay anycast range.
- A 10 s `ReadHeaderTimeout` against slow-header clients.
- HSTS is sent on all responses to HTTPS requests, including public media.
- Unneeded `blob:` sources removed from the admin CSP (`media-src`,
  `worker-src`). Obsolete `X-Download-Options` header removed.
- Static assets are opened through `os.Root` with a check against file swaps
  between `Lstat` and `Open`.
- Browser exports no longer contain access tokens.
- Container:
  - The unprivileged user is pinned to uid 100 / gid 101 (unchanged values).
  - Application files are no longer writable by the service.
  - `-trimpath` build.
  - The Compose example drops all capabilities and sets `no-new-privileges`.
- CI:
  - Least-privilege `GITHUB_TOKEN`.
  - Actions pinned to commit SHAs.
  - `govulncheck`, `go mod verify` and a gofmt check.
  - Dependabot for Go modules, Actions and Docker images.
- Docs: the Caddy reverse-proxy example now overwrites `X-Real-IP`. Before,
  clients could spoof it and appear to be on the LAN.

### Fixed

- **Graceful shutdown.** The process used to exit as soon as shutdown began,
  cutting off running uploads. It now waits up to 30 s for in-flight requests.
- **Slow uploads.** Large uploads over slow connections were aborted by the
  30 s read timeout. After authentication, uploads now get a deadline sized to
  the maximum upload, and preview regeneration a 30-minute deadline.
- **Durability.** Metadata and media are flushed (`fsync`) before the atomic
  rename, and the metadata directory is synced, so a power loss cannot leave
  a truncated file.
- **Browser JPEG pre-processing:**
  - It used to cap JPEG uploads at 1920×1080. 4K images uploaded through the
    panel were silently downscaled, unlike URL or gallery uploads, and with
    `COMPRESSION_SCALE` < 100 they were scaled twice.
  - Dimensions are now always left to the server.
  - Pre-processing is skipped when the settings can't be loaded (so lossless
    mode is never bypassed) and for images too large for a browser canvas.
- **Copy URL** copied a stale URL after the access level changed or the token
  was rotated.
- **Mobile/tablet layout:** on token links and with long link names, the card
  text and token buttons overflowed the card.
- **Server error messages** from the API were never translated, because of
  the trailing newline in plain-text errors.
- **Create button:** its icon disappeared once translations loaded.
- **Gallery picker** shows videos (MP4/WebM) as video thumbnails instead of
  broken images. Confirming the picker now restores focus and ARIA state.
- The access-level `<label>` was not associated with its `<select>` for link
  names starting with a digit.
- The "Create new link" PWA shortcut (`/admin?action=create`) now focuses the
  input.
- Searching for a file type (e.g. `png`, `.mp4`) finds matching links.
- The configured `EXTERNAL_IMAGE_DIR` is created at startup instead of the
  hard-coded default path.
- The Docker `HEALTHCHECK` honours a custom `PORT`.

### Changed

- Go 1.26 or newer is required to build (Go 1.25 is end of life). The Docker
  build uses Go 1.27.
- Invalid numeric or boolean environment variables are logged instead of
  ignored silently.
- Admin thumbnails are encoded at a fixed WebP quality of 80. They are
  smaller in lossless mode, and stored media is unaffected.
- The gallery listing walks the directory once inside an `os.Root`, instead
  of opening a new root for every file.
- Code cleanup:
  - duplicated helpers merged: credential checks, resize, access defaults,
    static file opening;
  - Go modernizations applied with `go fix`;
  - unused i18n keys and dead frontend code removed.

### Documentation

- README, SECURITY.md and docs/API.md rewritten to match the code. This
  includes the bind-mount permissions, the lockout and the full endpoint
  reference.
- The outdated `docs/security.md` was merged into SECURITY.md. Among other
  things, it wrongly claimed that missing credentials disable authentication.
- Historical audit notes (`AUDIT.md`, `IMPROVEMENTS.md`, `UI_IMPROVEMENTS.md`)
  were replaced by this changelog. ROADMAP.md now lists only open items.

## [0.9.9] – 2026-09-27

A security-hardening series (PRs #16–#21).

- **Access and authentication.**
  - Per-link access levels (`public`, `local`, `token`, `auth`) with token
    rotation.
  - Missing admin credentials fail closed (503).
  - `DISABLE_AUTH` added as an explicit opt-out.
- **Media isolation.**
  - Media and thumbnails moved from `static/images/` to `data/media/` and
    `data/previews/` (automatic migration).
  - `/static/` serves an allowlist only, and previews are admin-only.
- **Downloads.** SSRF-safe URL downloads: every DNS answer and redirect is
  checked, the IP is pinned even through HTTP, HTTPS and SOCKS5 proxies, and
  downloads are streamed to bounded temporary files.
- **Uploads.** Content-based type detection, full decoding before
  publication, pixel and memory budgets, a streaming pure-Go WebP decoder, and
  MP4 files with any `ftyp` brand accepted.
- **Storage.** Atomic, copy-on-write metadata store with per-link locks and
  rollback. Corrupt metadata stops startup.
- **Browser protection.** CSRF protection, strict CSP and security headers,
  and a service worker limited to static assets.
- **Admin UI.**
  - Resilient initialisation on old browsers and plain-HTTP LAN access.
  - Pinning, inline rename, drag-and-drop link creation.
  - Refreshed design and PWA installability.

## [0.9.7] – 2026-02-28

See the [GitHub release](https://github.com/zoixc/lanpaper/releases/tag/0.9.7).
