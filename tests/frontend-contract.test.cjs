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
const scripts = fs.readdirSync(path.join(ROOT, 'static/js'))
  .filter((name) => name.endsWith('.js'))
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
  const used = new Set([
    ...matches(scriptSource, /getElementById\(\s*'([^']+)'\s*\)/g),
    ...matches(scriptSource, /querySelector\(\s*'#([^']+)'\s*\)/g),
  ]);
  const defined = new Set([
    ...matches(adminHtml, /\bid="([^"]+)"/g),
    ...matches(scriptSource, /\.id\s*=\s*'([^']+)'/g),
  ]);
  const missing = [...used].filter((id) => !defined.has(id));
  assert.deepEqual(missing, [], `ids used by the scripts but missing from admin.html: ${missing.join(', ')}`);
  assert.ok(used.size > 20, `only ${used.size} ids found; the selector patterns probably stopped matching`);
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
