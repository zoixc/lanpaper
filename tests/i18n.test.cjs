// SPDX-License-Identifier: MIT
// Translation completeness. The admin panel ships six full translations; a
// key that exists in one file but not another shows up as English (or as a
// raw key) in the middle of a translated interface, which is exactly the
// kind of defect that is invisible in review but obvious to a user.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const LANGS = ['en', 'ru', 'de', 'fr', 'it', 'es'];
const ROOT = path.join(__dirname, '..');

const translations = {};
for (const lang of LANGS) {
  translations[lang] = JSON.parse(fs.readFileSync(path.join(ROOT, `static/i18n/${lang}.json`), 'utf8'));
}

const adminHtml = fs.readFileSync(path.join(ROOT, 'admin.html'), 'utf8');
const scripts = ['app.js', 'compressor.js', 'export-import.js', 'settings-menu.js']
  .map(name => fs.readFileSync(path.join(ROOT, 'static/js', name), 'utf8'))
  .join('\n');

function referencedKeys() {
  const keys = new Set();
  // Data attributes: data-i18n, data-i18n-placeholder, data-i18n-aria, data-i18n-title.
  for (const match of adminHtml.matchAll(/data-i18n(?:-placeholder|-aria|-title)?="([^"]+)"/g)) {
    keys.add(match[1]);
  }
  // Runtime lookups: t('key') and t("key").
  for (const match of scripts.matchAll(/\bt\(\s*['"]([a-z0-9_]*[a-z0-9])['"]/g)) {
    keys.add(match[1]);
  }
  // Keys passed through a variable: t('cat_' + category, ...) and friends.
  keys.add('cat_image');
  keys.add('cat_video');
  keys.add('cat_other');
  keys.add('access_public');
  keys.add('access_local');
  keys.add('access_token');
  keys.add('access_auth');
  keys.add('palette_sunset');
  keys.add('palette_indigo');
  keys.add('palette_terra');
  keys.add('palette_sage');
  keys.add('palette_graphite');
  return keys;
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
  const mustDiffer = ['create_btn', 'delete_btn', 'settings_language', 'pult_btn', 'change_media'];
  for (const lang of LANGS.filter(l => l !== 'en')) {
    const identical = mustDiffer.filter(key => translations[lang][key] === translations.en[key]);
    // A word may legitimately coincide (Italian "Video", French "Photo"), so
    // only fail when the whole sample is unchanged.
    assert.ok(identical.length < mustDiffer.length,
      `${lang}.json looks untranslated: ${identical.join(', ')}`);
  }
});
