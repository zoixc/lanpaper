// SPDX-License-Identifier: MIT
// Translation completeness. The admin panel ships six full translations; a
// key that exists in one file but not another shows up as English (or as a
// raw key) in the middle of a translated interface, which is exactly the
// kind of defect that is invisible in review but obvious to a user.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const ROOT = path.join(__dirname, '..');

// The panel's language list has exactly one definition, in settings-menu.js.
// Parse it instead of keeping a second copy here that can drift.
const menuSource = fs.readFileSync(path.join(ROOT, 'static/js/settings-menu.js'), 'utf8');
const declaration = menuSource.match(/window\.LANPAPER_LANGS\s*=\s*(\[[^\]]*\]);/);
assert.ok(declaration, 'settings-menu.js no longer declares window.LANPAPER_LANGS');
// The declaration is a JS array literal with single quotes; the codes are
// plain ASCII, so quoting them for JSON is enough to parse it.
const LANGS = JSON.parse(declaration[1].replace(/'/g, '"'));

const translations = {};
for (const lang of LANGS) {
  translations[lang] = JSON.parse(fs.readFileSync(path.join(ROOT, `static/i18n/${lang}.json`), 'utf8'));
}

const adminHtml = fs.readFileSync(path.join(ROOT, 'admin.html'), 'utf8');
const scripts = ['app.js', 'compressor.js', 'export-import.js', 'settings-menu.js', 'prepaint.js']
  .map(name => fs.readFileSync(path.join(ROOT, 'static/js', name), 'utf8'))
  .join('\n');
const workerSource = fs.readFileSync(path.join(ROOT, 'static/sw.js'), 'utf8');

// Literals that look like a translation key but are not one. Each entry needs
// a reason, because the point of the inventory below is that a stray
// snake_case literal in the panel is either a real key or a deliberate
// decision — never an oversight.
const NOT_TRANSLATION_KEYS = new Set([
  // Sort state values (see SORT_KEYS/SORTS in app.js). They are stored in the
  // preferences and travel in the API query; the visible label comes from
  // SORT_LABELS, where date_desc maps to 'date_new'.
  'date_asc', 'date_desc'
]);

function referencedKeys() {
  const keys = new Set();
  // Data attributes: data-i18n, data-i18n-placeholder, data-i18n-aria, data-i18n-title.
  for (const match of adminHtml.matchAll(/data-i18n(?:-placeholder|-aria|-title)?="([^"]+)"/g)) {
    keys.add(match[1]);
  }
  // Literal inventory. Any literal of the form word_word anywhere in the panel
  // is a translation key reference unless it is listed above. Keys spelled out
  // in arrays, ternaries and lookup tables (`['t', 'sc_theme']`,
  // `t(cond ? 'a' : 'b')`, `SORT_LABELS[...]`) never reach the t('key') regex
  // this test used to rely on, and that is how twenty-one keys once shipped
  // missing while the whole suite stayed green.
  const sources = [adminHtml, scripts, workerSource];
  for (const source of sources) {
    for (const match of source.matchAll(/['"]([a-z][a-z0-9]*(?:_[a-z0-9]+)+)['"]/g)) {
      if (!NOT_TRANSLATION_KEYS.has(match[1])) keys.add(match[1]);
    }
  }
  // The argument list of every t(...) call, as a balanced slice of the source.
  // This catches single-word keys ('cancel') and any literal the inventory
  // pattern above would miss.
  for (const argument of translationCallArguments(scripts)) {
    for (const match of argument.matchAll(/["']([a-z0-9_]+)["']/g)) {
      const literal = match[1];
      // Fragments of a key assembled at runtime ('access_' + level,
      // key + '_short') are not keys on their own.
      if (!literal || literal.endsWith('_') || literal.startsWith('_')) continue;
      if (!/^[a-z][a-z0-9]*(_[a-z0-9]+)*$/.test(literal)) continue;
      // A literal that is compared (`t(scope === 'all' ? ...)`) or indexed
      // (`t(SORT_LABELS['x'])`) is data for the call, not the key it asks for.
      const before = argument.slice(0, match.index);
      const after = argument.slice(match.index + match[0].length);
      if (/^\s*(===|!==|==|!=|<=|>=|<|>|\+|\?|&&|\|\|)/.test(after)) continue;
      if (before.trimEnd().endsWith('[')) continue;
      keys.add(literal);
    }
  }
  // Keys assembled at runtime: `'access_' + level` in app.js.
  keys.add('access_public');
  keys.add('access_local');
  keys.add('access_token');
  keys.add('access_auth');
  keys.add('access_auth_hint');
  return keys;
}

// Returns the source text between the parentheses of every t( ... ) call.
function translationCallArguments(source) {
  const args = [];
  for (const match of source.matchAll(/\bt\(/g)) {
    let depth = 0;
    let quote = null;
    for (let i = match.index + match[0].length - 1; i < source.length; i += 1) {
      const char = source[i];
      if (quote) {
        if (char === '\\') i += 1;
        else if (char === quote) quote = null;
        continue;
      }
      if (char === "'" || char === '"' || char === '`') quote = char;
      else if (char === '(') depth += 1;
      else if (char === ')') {
        depth -= 1;
        if (depth === 0) {
          args.push(source.slice(match.index, i + 1));
          break;
        }
      }
    }
  }
  return args;
}

test('every language has exactly the same keys', () => {
  const base = Object.keys(translations.en).sort();
  for (const lang of LANGS) {
    assert.deepEqual(Object.keys(translations[lang]).sort(), base, `${lang}.json keys differ from en.json`);
  }
});

test('no translation is empty and none leaks a raw key', () => {
  for (const lang of LANGS) {
    for (const [key, value] of Object.entries(translations[lang])) {
      assert.equal(typeof value, 'string', `${lang}: ${key} is not a string`);
      assert.ok(value.trim().length > 0, `${lang}: ${key} is empty`);
      // A value that is just its own key means the translation file was
      // generated without content.
      assert.notEqual(value, key, `${lang}: ${key} is not translated`);
    }
  }
});

test('every key used by the markup or the scripts exists in all six languages', () => {
  for (const key of referencedKeys()) {
    for (const lang of LANGS) {
      assert.ok(translations[lang][key] !== undefined, `${lang}.json is missing "${key}"`);
    }
  }
});

// t('prefix_' + name) builds the key at runtime, so the inventory above cannot
// see it. Each prefix declares where its domain comes from; the test reads that
// list out of the source instead of keeping a hand-written copy of it.
const RUNTIME_KEY_DOMAINS = [
  // Accent swatches in the settings sheet: t('palette_' + name).
  { prefix: 'palette_', constants: ['PALETTES'] }
];

test('keys assembled from a list at runtime are translated for every value', () => {
  for (const { prefix, constants } of RUNTIME_KEY_DOMAINS) {
    assert.ok(scripts.includes(`t('${prefix}' +`),
      `nothing assembles "${prefix}" keys any more; drop the rule`);
    const values = [];
    for (const name of constants) {
      const declaration = scripts.match(new RegExp(`const ${name} = \\[([^\\]]*)\\]`));
      assert.ok(declaration, `${name} is no longer declared in the panel scripts`);
      for (const value of declaration[1].matchAll(/'([^']+)'/g)) values.push(value[1]);
    }
    assert.ok(values.length > 0, `${constants.join(', ')} is empty`);
    for (const value of values) {
      for (const lang of LANGS) {
        assert.ok(translations[lang][prefix + value] !== undefined,
          `${lang}.json is missing "${prefix}${value}" (assembled at runtime)`);
      }
    }
  }
});

test('the narrow-screen variants the panel asks for exist', () => {
  // applyTranslations() looks up `<key>_short` for placeholders on phones and
  // silently falls back to the long text when it is missing, so a missing
  // variant is invisible until the layout breaks.
  for (const match of adminHtml.matchAll(/data-i18n-placeholder="([^"]+)"/g)) {
    const variant = `${match[1]}_short`;
    const declared = Object.values(translations).some(dict => dict[variant] !== undefined);
    if (!declared) continue;
    for (const lang of LANGS) {
      assert.ok(translations[lang][variant] !== undefined, `${lang}.json is missing "${variant}"`);
    }
  }
  for (const lang of LANGS) {
    assert.ok(translations[lang].search_placeholder_short !== undefined,
      `${lang}.json is missing "search_placeholder_short"`);
  }
});

test('placeholders such as {{n}} survive translation', () => {
  const placeholders = value => (value.match(/\{\{\s*[a-z]+\s*\}\}/g) || []).sort();
  const base = Object.fromEntries(Object.entries(translations.en).map(([k, v]) => [k, placeholders(v)]));
  for (const lang of LANGS) {
    for (const [key, value] of Object.entries(translations[lang])) {
      if (!base[key]) continue;
      assert.deepEqual(placeholders(value), base[key],
        `${lang}.json: ${key} has different placeholders than en.json`);
    }
  }
});

test('each non-English language actually translates the interface', () => {
  // A handful of strings that must not stay English in a translated panel.
  const mustDiffer = ['create_submit', 'delete', 'settings_shortcuts', 'upload_file', 'search_placeholder'];
  for (const lang of LANGS.filter(l => l !== 'en')) {
    const identical = mustDiffer.filter(key => translations[lang][key] === translations.en[key]);
    // A word may legitimately coincide (Italian "Video", French "Photo"), so
    // only fail when the whole sample is unchanged.
    assert.ok(identical.length < mustDiffer.length,
      `${lang}.json looks untranslated: ${identical.join(', ')}`);
  }
});
