// SPDX-License-Identifier: MIT
// Frontend contract: the panel's HTML, its scripts and the Go routes have to
// agree. These checks are static (no browser), but they catch the failures
// that a syntax check and a unit test both miss: an element id renamed in
// admin.html while a selector still looks for the old one, a script calling an
// endpoint that is no longer routed, and a static asset referenced but not
// shipped.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const ROOT = path.join(__dirname, '..');
const read = (file) => fs.readFileSync(path.join(ROOT, file), 'utf8');
const exists = (url) => fs.existsSync(path.join(ROOT, url.replace(/^\//, '')));

const adminHtml = read('admin.html');
// login.js belongs to login.html, not to the admin panel, so the panel-facing
// checks skip it. It has its own checks below.
const PANEL_SCRIPTS = (name) => name.endsWith('.js') && name !== 'login.js';
const scripts = fs.readdirSync(path.join(ROOT, 'static/js'))
  .filter(PANEL_SCRIPTS)
  .sort()
  .map((name) => read(`static/js/${name}`));
const scriptSource = scripts.join('\n');

function matches(source, re, group = 1) {
  return [...source.matchAll(re)].map((match) => match[group]);
}

// A template literal path is only checkable up to its first substitution:
// /api/link/${name}/history becomes the prefix /api/link/, which must still be
// backed by a route.
function normalizeTemplatePath(url) {
  return url.split('${')[0];
}

test('every element id the scripts use exists in the panel', () => {
  // The panel looks elements up through the $() helper, which takes a plain
  // CSS selector ("#id", optionally with a scope), so match that convention
  // as well as the direct DOM calls.
  const used = new Set([
    ...matches(scriptSource, /getElementById\(\s*'([^']+)'\s*\)/g),
    ...matches(scriptSource, /querySelector\(\s*'#([^']+)'\s*\)/g),
    ...matches(scriptSource, /\$\(\s*'#([a-zA-Z0-9_-]+)'\s*(?=[,)])/g),
  ]);
  const defined = new Set([
    ...matches(adminHtml, /\bid="([^"]+)"/g),
    ...matches(scriptSource, /\.id\s*=\s*'([^']+)'/g),
    // Elements the panel builds itself declare their id in the attribute
    // object passed to h(), e.g. h('div', { id: 'bulkbar' }).
    ...matches(scriptSource, /\bid:\s*'([^']+)'/g),
  ]);
  const missing = [...used].filter((id) => !defined.has(id));
  assert.deepEqual(missing, [], `ids used by the scripts but missing from admin.html: ${missing.join(', ')}`);
  assert.ok(used.size > 20, `only ${used.size} ids found; the selector patterns probably stopped matching`);
});

test('sign out redirects only after durable server success', () => {
  assert.match(scriptSource,
    /async function signOut\(\)[\s\S]*?catch \(_\) \{[\s\S]*?return;[\s\S]*?window\.location\.replace\('\/admin'\)/,
    'a failed session revocation must remain retryable instead of redirecting');
});

test('every API path the scripts call is routed in main.go', () => {
  // The catch-all "/" matches every path by prefix, so it is not evidence that
  // a specific API path is routed.
  const routes = matches(read('main.go'), /HandleFunc\("([^"]+)"/g).filter((route) => route !== '/');
  const called = new Set([
    ...matches(scriptSource, /['"`](\/api\/[^'"`\s?]*)['"`]/g),
    ...matches(adminHtml, /['"](\/api\/[^'"\s?]*)['"]/g),
  ]);
  assert.ok(called.size > 5, `only ${called.size} API paths found; the fetch patterns probably stopped matching`);

  const unmatched = [];
  for (const raw of called) {
    const url = normalizeTemplatePath(raw);
    const ok = routes.some((route) => route === url || (route.endsWith('/') && url.startsWith(route)));
    if (!ok) unmatched.push(raw);
  }
  assert.deepEqual(unmatched, [], `paths without a route: ${unmatched.join(', ')}`);
});

test('every static asset the panel references is on disk', () => {
  const referenced = new Set([
    ...matches(adminHtml, /(?:src|href)="(\/static\/[^"]+)"/g),
    ...matches(scriptSource, /['"`](\/static\/[^'"`]*)['"`]/g),
  ]);
  assert.ok(referenced.size > 5, `only ${referenced.size} static paths found; the patterns probably stopped matching`);
  const missing = [...referenced].filter((url) => !url.includes('${') && !exists(url));
  assert.deepEqual(missing, [], `static files referenced but absent: ${missing.join(', ')}`);
});

test('every helper is declared exactly once per script', () => {
  // A second declaration of the same name wins by hoisting and turns the first
  // one into dead code. That is how a createLinksFromFiles() which uploaded
  // nothing took over from the real one: the drop handler and the file picker
  // called it and the panel drew a tile with no file behind it.
  const files = fs.readdirSync(path.join(ROOT, 'static/js')).filter(PANEL_SCRIPTS);
  const duplicated = [];
  let declarations = 0;
  for (const name of files) {
    const seen = new Set();
    // Top-level helpers sit at one indent level inside the file's IIFE; a
    // nested function of the same name lives in its own scope and is fine.
    for (const match of read(`static/js/${name}`).matchAll(/^    (?:async )?function ([A-Za-z0-9_$]+)\s*\(/gm)) {
      declarations += 1;
      if (seen.has(match[1])) duplicated.push(`${name}: ${match[1]}`);
      seen.add(match[1]);
    }
  }
  assert.ok(declarations > 50, `only ${declarations} declarations found; the pattern probably stopped matching`);
  assert.deepEqual(duplicated, [], `helpers declared twice: ${duplicated.join(', ')}`);
});

test('bulk pin selects the links the button promises to change', () => {
  // anyUnpinned is both the label ("Закрепить" while a selected link is
  // unpinned) and the wanted state, so the filter has to keep the links whose
  // pinned flag still differs from it. The negated form selected the links that
  // were already in the wanted state and toggled exactly those.
  const anchor = 'const targets = selectedLinks().filter(';
  const start = scriptSource.indexOf(anchor);
  assert.ok(start >= 0, 'the bulk-pin target filter disappeared');
  // The argument contains a nested call (Boolean(...)), so slice it by its
  // balanced parentheses instead of stopping at the first ")".
  let depth = 0;
  let end = start + anchor.length - 1;
  for (; end < scriptSource.length; end += 1) {
    if (scriptSource[end] === '(') depth += 1;
    else if (scriptSource[end] === ')' && --depth === 0) break;
  }
  const expression = scriptSource.slice(start + anchor.length, end).replace(/\s+/g, '');
  assert.match(expression, /anyUnpinned/, `the filter no longer uses the wanted state: ${expression}`);
  assert.doesNotMatch(expression, /!anyUnpinned/,
    `the bulk-pin filter inverted the wanted state: ${expression}`);
});

test('an archived version is opened with its own ?v= parameter', () => {
  const opener = scriptSource.match(/function openMediaVersion\(link, version, isCurrent\) \{([\s\S]*?)\n    \}/);
  assert.ok(opener, 'openMediaVersion() disappeared');
  assert.match(opener[1], /\?v=/, `the version opener no longer builds a ?v= URL: ${opener[1]}`);
  assert.ok(scriptSource.includes('openMediaVersion(link, version, isCurrent)'),
    'the history row no longer opens the version it stands for');
});

test('the media kind comes from mimeType, never from the user category', () => {
  // GET /api/wallpapers reports the stored extension in mimeType ("png", "mp4")
  // and the user category in category (tech/life/work/other — and "other" is
  // what the panel's own create dialog stores). Comparing mimeType with
  // "image/png" or looking for "video" in category therefore never matched a
  // link the panel created itself: the type badge read "—", a transparent PNG
  // was cropped without its checkerboard, and a video tile asked
  // /api/preview/{name} — which answers with the video file — inside an <img>.
  assert.doesNotMatch(scriptSource,
    /category === 'video'|category !== 'video'|category === 'gif'|mimeType === 'image\/|mimeType\.split\('\/'\)/,
    'the media kind is read out of category, or mimeType is treated as a MIME string again');
  assert.match(scriptSource, /mediaExt[^\n]*mimeType/,
    'mediaExt() disappeared or no longer reads mimeType');
  assert.match(scriptSource, /isVideoMedia[^\n]*mp4[^\n]*webm/,
    'isVideoMedia() disappeared or no longer recognizes mp4/webm');
  const hasFrame = scriptSource.match(/function hasFrame\(link\) \{([\s\S]*?)\n    \}/);
  assert.ok(hasFrame && /isVideoMedia\(link\)/.test(hasFrame[1]),
    'hasFrame() no longer excludes videos, so a tile pulls the movie into an <img>');
  const guards = scriptSource.match(/if \(!hasFrame\(link\)\) \{/g) || [];
  assert.ok(guards.length >= 2, `only ${guards.length} preview sites skip a missing frame`);
});

test('dropping files creates the links and uploads the files', () => {
  // A second, optimistic createLinksFromFiles() won by hoisting and only drew a
  // tile: no POST /api/link, no upload, no validLinkName — a file called
  // admin.png produced a phantom tile under a reserved name. There is only one
  // definition now, and it has to talk to the server (see also the duplicate
  // declaration check above).
  const helper = scriptSource.match(/async function createLinksFromFiles\(files\) \{([\s\S]*?)\n    \}/);
  assert.ok(helper, 'the async createLinksFromFiles() disappeared');
  assert.match(helper[1], /validLinkName\(name\)/, 'the reserved-name check is gone');
  assert.match(helper[1], /'\/api\/link'/, `the link is not created over the API: ${helper[1].slice(0, 160)}`);
  assert.match(helper[1], /uploadFileTo\(/, 'the file is never uploaded');
});

test('every icon the panel asks for has a drawing', () => {
  // icon() and data-icon="" name the same sprite. A name without a path renders
  // nothing at all, so a typo (or a new glyph the panel forgot to define) is an
  // invisible button, not a visible error.
  const names = new Set([
    ...matches(adminHtml, /data-icon="([a-zA-Z0-9_-]+)"/g),
    ...matches(scriptSource, /data-icon="([a-zA-Z0-9_-]+)"/g),
    ...matches(scriptSource, /icon\(\s*'([a-zA-Z0-9_-]+)'/g),
  ]);
  assert.ok(names.size > 20, `only ${names.size} icon names found; the pattern probably stopped matching`);
  const sprite = scriptSource.slice(scriptSource.indexOf('const ICONS = {'), scriptSource.indexOf('const FILLED'));
  assert.ok(sprite.length > 500, 'the ICONS sprite was not found');
  const missing = [...names].filter((name) => !new RegExp(`\\b${name}:\\s*'`).test(sprite));
  assert.deepEqual(missing, [], `icon names without a drawing: ${missing.join(', ')}`);
});

test('the theme switch and the gear move the way 0.12.1 moved them', () => {
  // 0.12.1 kept both theme glyphs in the button, stacked, and cross-faded them:
  // the leaving one goes to scale(0.5) rotate(-90deg) while the other returns
  // to scale(1) rotate(0). The gear turned 90 degrees while the settings panel
  // was open. The panel keeps those numbers; the state comes from
  // html[data-theme], which prepaint.js sets before the first paint, so the
  // right glyph is there without a flash (and without a JS icon swap).
  const button = adminHtml.match(/<button[^>]*id="themeBtn"[\s\S]*?<\/button>/);
  assert.ok(button, 'the theme button disappeared');
  for (const glyph of ['sun', 'moon']) {
    assert.match(button[0], new RegExp(`class="theme-icon" data-icon="${glyph}"`),
      `the ${glyph} is not one of the stacked theme icons`);
  }
  const css = read('static/css/style.css');
  // Test declarations, not line breaks, indentation or selector-list layout.
  // These contracts concern ordinary flat rules, not the whole CSS grammar.
  function declarations(selector, source = css) {
    const result = {};
    const clean = source.replace(/\/\*[\s\S]*?\*\//g, '');
    for (const [, selectors, body] of clean.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
      if (!selectors.split(',').some(s => s.trim().replace(/\s+/g, ' ') === selector)) continue;
      for (const decl of body.split(';')) {
        const colon = decl.indexOf(':');
        if (colon !== -1) result[decl.slice(0, colon).trim()] = decl.slice(colon + 1).replace(/\s+/g, ' ').trim();
      }
    }
    return result;
  }
  const themeIcon = declarations('#themeBtn .theme-icon');
  assert.deepEqual(declarations('#themeBtn .theme-icon', css.replace(/\s+/g, ' ')), themeIcon);
  assert.equal(themeIcon.position, 'absolute', 'the theme icons are not stacked');
  assert.equal(themeIcon.transition, 'opacity 0.2s ease, transform 0.25s ease');
  assert.equal(themeIcon.transform, 'scale(0.5) rotate(-90deg)');
  for (const [theme, icon] of [['light', 'sun'], ['dark', 'moon']]) {
    const active = declarations(`html[data-theme="${theme}"] #themeBtn .theme-icon[data-icon="${icon}"]`);
    assert.equal(active.opacity, '1');
    assert.equal(active.transform, 'scale(1) rotate(0deg)');
  }
  assert.equal(declarations('#settingsBtn svg').transition, 'transform 0.25s ease');
  assert.equal(declarations('body.settings-open #settingsBtn svg').transform, 'rotate(90deg)');
  assert.match(scriptSource, /classList\.toggle\('settings-open'/,
    'nothing sets the settings-open state the gear turns on');
  assert.match(css, /prefers-reduced-motion/, 'the "less motion" switch disappeared');
});

test('browser chrome and the document language follow the panel, not the system', () => {
  // Two defects found in the second audit pass. (1) <meta name="theme-color">
  // only had media-attributed variants: a manual theme choice moved the panel
  // to dark while the phone's status bar stayed light. Both tags now receive
  // the colour of the effective theme (CSSOM, so the CSP stays intact), and
  // prepaint.js does it before the first paint. (2) <html lang="en"> stayed
  // English for a Russian or German UI because only setLang() wrote the
  // attribute; applyTranslations() owns it now, so the first render is
  // already right for screen readers and spellcheck.
  const applyTheme = scriptSource.match(/function applyTheme\(\) \{([\s\S]*?)\n    \}/);
  assert.ok(applyTheme, 'applyTheme() disappeared');
  assert.match(applyTheme[1], /meta\[name="theme-color"\]/,
    'applyTheme no longer repaints the browser chrome');
  const prepaint = read('static/js/prepaint.js');
  assert.match(prepaint, /meta\[name="theme-color"\]/,
    'prepaint.js no longer sets the ring before the first paint');
  for (const color of ['#F5F6F9', '#14161B']) {
    assert.ok(prepaint.includes(color), `${color} missing from prepaint.js`);
    assert.ok(scriptSource.includes(color), `${color} missing from app.js`);
  }
  const translations = scriptSource.match(/function applyTranslations\(root\) \{([\s\S]*?)\n    \}/);
  assert.ok(translations, 'applyTranslations() disappeared');
  assert.match(translations[1], /documentElement\.lang = state\.lang/,
    'the panel no longer keeps <html lang> in step with the chosen language');
});

test('shortcuts stay out of an open dialog, and the gear reports its state', () => {
  // Pressing "n" while a dialog was open called openCreateDialog() again:
  // openOverlay() closes what is already there, so the form was replaced and
  // everything typed into it was lost. The handler bails out while an overlay
  // is up (Esc and Tab belong to the overlay handler). The gear also carries
  // aria-expanded now — it opens a modal sheet, and a screen reader had no way
  // to hear whether that sheet was open.
  const shortcuts = scriptSource.match(/function initShortcuts\(\) \{([\s\S]*?)\n    \}\n/);
  assert.ok(shortcuts, 'initShortcuts() disappeared');
  assert.match(shortcuts[1], /if \(currentOverlay\) return;/,
    'shortcuts act behind an open dialog again');
  const button = adminHtml.match(/<button[^>]*id="settingsBtn"[^>]*>/);
  assert.ok(button, 'the settings button disappeared');
  assert.match(button[0], /aria-expanded="false"/, 'the gear lost aria-expanded');
  assert.match(button[0], /aria-controls="settingsSheet"/, 'the gear lost aria-controls');
  assert.match(adminHtml, /id="settingsSheet"/, 'the settings sheet lost its id');
  assert.match(scriptSource, /setSettingsExpanded\(/, 'nothing updates the gear state');
  const code = read('static/sw.js');
  assert.match(code, /lanpaper-static-v22/,
    'the precache generation must be bumped whenever the panel assets change');
});

test('the boot screen covers the unassembled panel and is always released', () => {
  // Разметка admin.html — не тот вид, который получится: initializeStudio()
  // переносит шапку в рельсу, hydrateIcons() рисует значки,
  // applyTranslations() ставит подписи. Пока этого не произошло, панель
  // выглядит иначе, поэтому её закрывает экран загрузки. Три вещи обязаны
  // выполняться вместе: метку ставит prepaint.js (единственный скрипт до
  // первой отрисовки), app.js снимает её после сборки, и она снимается сама,
  // если app.js не выполнится, — иначе панель останется пустой навсегда.
  const prepaint = read('static/js/prepaint.js');
  assert.match(adminHtml, /id="bootScreen"/, 'the boot screen disappeared from admin.html');
  assert.match(prepaint, /dataset\.boot = 'pending'/,
    'prepaint.js no longer arms the boot screen before the first paint');
  assert.match(prepaint, /dataset\.layout !== 'studio'/,
    'nothing keeps the sign-in page, which no script assembles, out of the boot screen');
  assert.match(prepaint, /window\.LANPAPER_BOOT = \{ reveal: reveal \}/,
    'prepaint.js no longer publishes the release app.js has to call');
  assert.match(prepaint, /setTimeout\(reveal, \d+\)/,
    'nothing releases the boot screen when app.js never runs');
  // Ресурс, который не загрузился, события не всплывает: без capture скрипт
  // панели молча остаётся за экраном до таймаута.
  const listener = prepaint.match(
    /addEventListener\('error', function \(event\) \{([\s\S]*?)\n        \}, true\);/);
  assert.ok(listener, 'the boot screen no longer listens for a panel script that failed');
  assert.match(listener[1], /tagName !== 'SCRIPT'/,
    'a failed image or stylesheet now releases the boot screen as if app.js had failed');
  const init = scriptSource.match(/async function init\(\) \{([\s\S]*?)\n    \}/);
  assert.ok(init, 'init() disappeared');
  assert.match(init[1], /window\.LANPAPER_BOOT\?\.reveal\(\)/,
    'app.js no longer releases the boot screen');
  for (const step of ['initializeStudio(t)', 'applyTranslations()', 'hydrateIcons(document)', 'render();']) {
    assert.ok(init[1].indexOf(step) < init[1].indexOf('LANPAPER_BOOT'),
      `the boot screen is released before ${step}, so the panel flashes half-built`);
  }
  // Без метки экран должен оставаться выключенным: prepaint.js — отдельный
  // файл, и если он не загрузится, панель грузится как раньше.
  const studioCss = read('static/css/studio.css');
  const boot = studioCss.slice(studioCss.indexOf('\n.boot {'));
  assert.match(boot, /\.boot \{\s*display: none;/,
    'the boot screen shows even when prepaint.js never armed it');
  assert.match(boot, /:root\[data-boot\] \.boot[\s\S]*?display: flex;/,
    'nothing turns the boot screen on once it is armed');
  assert.match(boot, /:root\[data-boot="done"\] \.boot[\s\S]*?opacity: 0;/,
    'the boot screen no longer fades out when it is released');
});

test('the panel preloads the typeface subset its language needs', () => {
  // Интерфейс — Manrope, а в разметке предзагружен только латинский поднабор:
  // его хватает на имена ссылок при любом языке. Если интерфейс русский,
  // кириллический поднабор тоже нужен до первой отрисовки, иначе бренд,
  // заголовок и названия карточек рисуются подменой и перещёлкиваются.
  // prepaint.js знает сохранённый язык и добавляет предзагрузку сам.
  const prepaint = read('static/js/prepaint.js');
  assert.match(adminHtml, /rel="preload"[^>]*manrope-latin-wght-normal\.woff2/,
    'the latin subset is no longer preloaded in the markup');
  assert.match(prepaint, /manrope-cyrillic-wght-normal\.woff2/,
    'prepaint.js no longer preloads the subset a Russian panel needs');
  assert.match(prepaint, /navigator\.language/,
    'the preload no longer falls back to the browser language, as langFromPrefs() does');
  // Шрифты загружаются в CORS-режиме: без crossorigin предзагрузка не
  // совпадёт с настоящим запросом, и файл придёт дважды.
  assert.match(prepaint, /crossOrigin = 'anonymous'/, 'the font preload lost crossorigin');
  assert.match(prepaint, /link\.as = 'font'/, 'the font preload is no longer announced as a font');
  for (const subset of ['latin', 'cyrillic']) {
    assert.ok(exists(`/static/fonts/manrope-${subset}-wght-normal.woff2`),
      `the ${subset} subset is gone from static/fonts`);
  }
});

test('the manifest colours agree with the background the panel paints', () => {
  // Android рисует экран запуска цветом background_color и кладёт на него
  // иконку. Иконка — тёмная и непрозрачная, поэтому фон обязан оставаться
  // светлым и совпадать с реальным --bg Studio, иначе установленное
  // приложение открывается чужим цветом (а в тёмной теме — светлой вспышкой).
  const manifest = JSON.parse(read('static/manifest.json'));
  const studio = read('static/css/studio.css');
  for (const key of ['background_color', 'theme_color']) {
    assert.equal(manifest[key], '#fafafa', `${key} is no longer the Studio light background`);
  }
  assert.match(studio, /--bg:\s*#fafafa/,
    'the Studio background moved; the manifest has to move with it');
});

test('copying reports the truth, and the upload toast stays until the upload ends', () => {
  // execCommand("copy") returns a boolean, and the old code ignored it: a
  // refused copy still said «Скопировано». The bulk button called copyText()
  // for the whole selection and toasted on its own, so a twenty-link copy
  // printed «Скопировано» twenty times and then «Скопировано ссылок: 20». And
  // the upload toast lived 1600 ms, which is shorter than any real upload:
  // the панель looked idle while the request was still running.
  const fn = scriptSource.match(/function copyText\(text, trigger, opts\) \{([\s\S]*?)\n    \}/);
  assert.ok(fn, 'copyText() lost its options argument');
  assert.match(fn[1], /opts && opts\.quiet/, 'copyText() no longer supports a quiet copy');
  assert.match(fn[1], /return Promise\.resolve\(ok\)/, 'the fallback path stopped reporting success');
  assert.match(fn[1], /if \(!quiet\) toast\(t\('copy_error'\)/, 'a failed copy is silent now');
  assert.ok(!/copyText\(text\)\s*;/.test(scriptSource), 'a call site still ignores the copy result');

  const bulk = scriptSource.match(/const count = state\.selected\.size;([\s\S]*?)\n            \}/);
  assert.ok(bulk, 'the bulk copy block disappeared');
  assert.match(bulk[1], /quiet: true/, 'the bulk copy still prints one toast per link');
  assert.match(bulk[1], /bulk_copied/, 'the bulk copy no longer reports what it copied');
  assert.match(bulk[1], /copy_error/, 'the bulk copy claims success when the clipboard refused');

  const toasts = scriptSource.match(/function toast\(message, opts\) \{([\s\S]*?)\n    \}/);
  assert.ok(toasts, 'toast() disappeared');
  assert.match(toasts[1], /duration === 0 \? null : setTimeout/, 'a toast can no longer stay on screen');
  assert.match(toasts[1], /return box;/, 'toast() does not hand the node back for dismissal');
  assert.ok(!/toast\(t\('uploading'\), \{ type: 'info', duration: 1[0-9]{3} \}\)/.test(scriptSource),
    'an upload toast is still dismissed on a fixed short timer');
  const uploads = scriptSource.match(/(?:const )?busy = [^\n]*(?:deps\.)?toast\((?:deps\.)?t\('uploading'\), \{ type: 'info', duration: 0 \}\)/g) || [];
  assert.equal(uploads.length, 2, `expected both upload paths to hold their toast, found ${uploads.length}`);
  assert.equal((scriptSource.match(/busy\.dismissToast\(\)/g) || []).length, 2,
    'an upload path never dismisses its toast');
});

test('a link the server refused to delete stays on screen', () => {
  // The bulk delete collected every selected name and then dropped all of them
  // from the list no matter what the server answered — a refused DELETE left a
  // link that existed on disk but had vanished from the panel until a reload.
  const block = scriptSource.match(/const names = Array\.from\(state\.selected\);([\s\S]*?)state\.links = state\.links\.filter/);
  assert.ok(block, 'the bulk delete block disappeared');
  assert.match(block[1], /removed\.push\(name\)/, 'the bulk delete no longer tracks what was deleted');
  assert.ok(!/state\.links = state\.links\.filter\(l => names\.indexOf/.test(scriptSource),
    'the panel still hides links the server never deleted');
});

test('Studio mobile layout avoids coarse-pointer distortions and button sheet grips', () => {
  const studioCss = read('static/css/studio.css');
  assert.ok(!/@media\s*\([^)]*pointer:\s*coarse/.test(studioCss),
    'studio.css must not distort control sizes on coarse pointers');
  assert.ok(!/@media\s*\([^)]*hover:\s*none/.test(studioCss),
    'studio.css must not override control geometry on touch devices');
  assert.ok(!/<button[^>]+class="sheet__grip"/.test(adminHtml),
    'sheet grips must remain decorative divs so they do not steal focus or add a 44px header gap');
  assert.equal((adminHtml.match(/<div class="sheet__grip"><\/div>/g) || []).length, 3,
    'each sheet template must keep its decorative grip div');
  assert.match(adminHtml, /class="section section--shortcuts"/,
    'settings shortcuts section must carry section--shortcuts class');
  assert.match(studioCss, /\.studio\s+\.section--shortcuts[\s\S]*?display:\s*none/,
    'keyboard shortcuts section must be hidden on mobile viewports');
});

// Innermost declaration blocks, comments and @media/@keyframes wrappers
// skipped. Enough to see which selector paints which literal colour; the
// cascade itself is not modelled here — the two CSS files are read as text.
function colorDeclarations(file) {
  const css = read(file).replace(/\/\*[\s\S]*?\*\//g, '');
  const found = [];
  const block = /([^{}]*?)\{([^{}]*)\}/g;
  let match;
  while ((match = block.exec(css)) !== null) {
    const selector = match[1].trim().replace(/\s+/g, ' ');
    if (!selector || selector.startsWith('@') || /^(from|to|\d+%)$/.test(selector)) continue;
    for (const [, prop, value] of match[2].matchAll(/([-\w]+)\s*:\s*([^;]+);/g)) {
      found.push({ selector, prop, value: value.trim(), file,
        line: css.slice(0, match.index).split('\n').length });
    }
  }
  return found;
}

test('the brand mark and the sign-in wordmark follow the theme they are drawn on', () => {
  // Both are one bitmap for two themes: a dark tile with a light glyph. Left
  // alone, the tile (#1B1C1E) melts into the dark rail (#171717) at 1.06:1 —
  // the mark stops reading as a mark. The sign-in wordmark already inverted;
  // the rail mark did not.
  const style = read('static/css/style.css');
  for (const selector of ['.login-card__logo', '.brand__mark img']) {
    const rule = new RegExp(
      `html\\[data-theme="dark"\\]\\s+${selector.replace(/[.[\]]/g, '\\$&')}\\s*\\{([^}]*)\\}`);
    const body = style.match(rule);
    assert.ok(body, `${selector} is not repainted for the dark theme`);
    assert.match(body[1], /filter:\s*invert\(1\)/, `${selector} keeps its light-theme colours in the dark theme`);
  }
});

test('an autofilled field keeps the panel colours, not the browser ones', () => {
  // Chrome and Safari paint their own background on an autofilled input and
  // ignore background-color: only an inset shadow covers it. Without this the
  // saved username and password arrive pale blue (light theme) or blue-grey
  // (dark) in a panel that never chose those colours.
  const style = read('static/css/style.css');
  const panel = style.match(/\.field input:-webkit-autofill,\s*\.input:-webkit-autofill\s*\{([^}]*)\}/);
  assert.ok(panel, 'panel fields have no autofill rule');
  assert.match(panel[1], /-webkit-text-fill-color:\s*var\(--text\)/, 'autofilled text keeps the browser colour');
  assert.match(panel[1], /box-shadow:\s*inset 0 0 0 1000px var\(--surface\)/, 'autofill background is not repainted');
  assert.match(style, /\.input:-webkit-autofill:focus\s*\{[^}]*var\(--shadow-focus\)/,
    'an autofilled field loses its focus ring');

  const login = style.match(/\.login-card__field input:-webkit-autofill\s*\{([^}]*)\}/);
  assert.ok(login, 'the sign-in fields have no autofill rule');
  assert.match(login[1], /box-shadow:\s*inset 0 0 0 1000px var\(--surface-2\)/,
    'the sign-in fields are filled with --surface-2, so autofill must match it');
  assert.match(style, /\.login-card__field input:-webkit-autofill:focus-visible\s*\{[^}]*var\(--shadow-focus\)/,
    'an autofilled sign-in field loses its focus ring');
});

test('every colour outside a media overlay comes from a theme token', () => {
  // A literal cannot follow the theme: it is one colour in both. The only
  // ones allowed are the badges, play and select buttons painted on top of a
  // photograph or a frame, where the backing is dark in both themes by
  // design (--shade and the stage). Everything else — chrome, controls,
  // shadows — has to read a token, or it carries one shade into the theme
  // where it does not belong.
  const allowed = new Set([
    // Всё, что нарисовано поверх кадра: подложка там всегда тёмная, какой
    // бы ни была тема интерфейса вокруг.
    '.on-media|background',
    '.on-media|box-shadow',
    '.on-media|color',
    '.card__open|color',
    '.card__open:hover|background',
    '.card__play|color',
    '.card__play|box-shadow',
    '.card__check|border',
    '.studio .card__frame--empty .card__play svg|color'
  ]);
  const literal = /(?:#[0-9a-f]{3,8}\b|\brgba?\([^)]*\)|\bhsla?\([^)]*\))/i;
  const offenders = [];
  for (const file of ['static/css/style.css', 'static/css/studio.css']) {
    for (const decl of colorDeclarations(file)) {
      if (decl.prop.startsWith('--')) continue;      // сами токены — значения тем
      if (!literal.test(decl.value) || /\bvar\(/.test(decl.value)) continue;
      if (allowed.has(`${decl.selector}|${decl.prop}`)) continue;
      offenders.push(`${file}:${decl.line} ${decl.selector} { ${decl.prop}: ${decl.value} }`);
    }
  }
  assert.deepEqual(offenders, [],
    'a hardcoded colour cannot follow the theme — add a token instead');
});

test('a phone keeps its rail open: the filters and the action buttons live in it', () => {
  // .studio.sidebar-collapsed .catalog-rail is display: none — collapsing
  // removes the whole band, not only its width. On a phone the band carries
  // the filters, the new-link button and the only copy of the theme and
  // settings buttons, so folding it would leave nowhere to reach them.
  // The saved preference is therefore honoured only where the rail is a
  // sidebar, and the toggle is not offered where it cannot be used.
  const studio = read('static/css/studio.css');
  assert.match(studio, /\.studio\.sidebar-collapsed \.catalog-rail \{\s*display: none;/,
    'collapsing no longer removes the whole rail, so the rule below is stale');

  const layout = read('static/js/studio-layout.js');
  const apply = layout.match(/function apply\(\) \{([\s\S]*?)\n    \}/);
  assert.ok(apply, 'apply() disappeared');
  assert.match(apply[1], /classList\.toggle\('sidebar-collapsed', folded\(\)\)/,
    'the rail is collapsed from its saved state without asking whether it can collapse');
  assert.match(apply[1], /toggle\.hidden = !collapsible\(\)/,
    'a phone is still offered a control that can only hide its filters and buttons');
  assert.match(layout, /const collapsible = \(\) => !mobile\.matches;/,
    'the phone breakpoint no longer decides whether the rail can collapse');
  assert.match(layout, /mobile\.addEventListener\('change', apply\)/,
    'a rotation between phone and desktop no longer re-applies the collapsed state');

  // Before the first paint the class comes from prepaint.js, so a phone would
  // flash an empty band until studio-layout.js put it back.
  const prepaint = read('static/js/prepaint.js');
  const saved = prepaint.match(
    /if \(localStorage\.getItem\('lp-studio-nav-collapsed'\) === 'true'[\s\S]{0,160}?\{\s*root\.classList\.add\('sidebar-collapsed'\);/);
  assert.ok(saved, 'the saved collapse state is no longer applied before the first paint');
  assert.match(saved[0], /matchMedia\('\(max-width: 760px\)'\)\.matches/,
    'prepaint.js collapses the rail on a phone, where nothing can expand it back');
});

test('the theme and settings buttons have one home', () => {
  // Они переезжали: раскрытая рельса держала их внизу, свёрнутая — в шапке.
  // Узел один (id один, фокус один), а дубль ломает и то и другое, значит
  // дом у кнопок один: шапка на широком экране, строка действий в полосе
  // рельсы на телефоне.
  const layout = read('static/js/studio-layout.js');
  const studio = read('static/css/studio.css');
  assert.ok(!/studio-utilities/.test(layout + studio),
    'the rail-bottom utilities are back: the buttons jump between the rail and the appbar again');
  const place = layout.match(/function placeActions\(\) \{([\s\S]*?)\n    \}/);
  assert.ok(place, 'placeActions() disappeared');
  assert.match(place[1], /actionRow\.append\(theme, settings\)/,
    'the theme and settings buttons no longer live in the appbar');
  assert.ok(!/utilities\.append/.test(layout), 'something still moves the buttons into the rail');
});

test('the rail head and the appbar share one top line', () => {
  // Блок марки по высоте равен строке шапки, поэтому знак, название и
  // заголовок «Ссылки» стоят на одной центральной линии. А верх «Новой
  // ссылки» совпадёт с верхом сетки, только если отступ сетки сверху равен
  // отступу марки снизу: до марки отступов нет ни сверху, ни сбоку.
  const studio = read('static/css/studio.css');
  assert.match(studio, /--studio-head-height: 66px/, 'the head height token disappeared');
  assert.match(studio, /\.studio \.appbar__row--top \{\s*min-height: var\(--studio-head-height\)/,
    'the appbar row no longer takes its height from the shared token');
  assert.match(studio, /\.studio \.catalog-rail \.brand \{\s*height: var\(--studio-head-height\)/,
    'the brand block is no longer as tall as the appbar row, so the name and the title drift apart');
  const brand = studio.match(/\.studio \.catalog-rail \.brand \{\s*margin: 0 7px (\d+)px;/);
  const content = studio.match(/\.studio \.content \{[^}]*padding: (\d+)px/);
  assert.ok(brand && content, 'the brand margin or the content padding is gone');
  assert.equal(content[1], brand[1],
    'the grid no longer starts level with the new-link button');
});
