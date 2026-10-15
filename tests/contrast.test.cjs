// SPDX-License-Identifier: MIT
'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const root = path.join(__dirname, '..');
const css = fs.readFileSync(path.join(root, 'static/css/style.css'), 'utf8');

function audit(t, source) {
  // Mutate a disposable fixture, never the working tree's production CSS.
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'lanpaper-contrast-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  fs.mkdirSync(path.join(dir, 'tools'));
  fs.mkdirSync(path.join(dir, 'static/css'), { recursive: true });
  fs.copyFileSync(path.join(root, 'tools/check-contrast.mjs'), path.join(dir, 'tools/check-contrast.mjs'));
  fs.writeFileSync(path.join(dir, 'static/css/style.css'), source);
  return spawnSync(process.execPath, [path.join(dir, 'tools/check-contrast.mjs')], { encoding: 'utf8' });
}

test('production checker covers mono and five colour palettes in both themes', t => {
  const result = audit(t, css);
  assert.equal(result.status, 0, result.stdout + result.stderr);
  for (const palette of ['mono', 'indigo', 'sage', 'clay', 'graphite', 'ocean']) {
    for (const theme of ['light', 'dark']) assert.match(result.stdout, new RegExp(`${palette} +${theme}`));
  }
  // 204 пары палитр плюс шесть пар про плитку в шапке (та же матрица из
  // шести палитр в двух темах): число растёт только вместе с новыми парами.
  assert.match(result.stdout, /Проверок: 276, провалов: 0/);
});

test('checker rejects the original dark nested-surface contrast regression', t => {
  const result = audit(t, css.replace(/--text-3:\s*#8d9095;/i, '--text-3: #898C91;'));
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.match(result.stdout, /mono\/dark: мета на вложенной поверхности/);
});

test('checker fails closed on unsupported colour syntax', t => {
  const result = audit(t, css + '\n:root { --text: var(--missing); }');
  assert.equal(result.status, 1);
  assert.match(result.stderr, /unsupported colour/);
});

test('checker respects combined-selector specificity and ignores media overrides', t => {
  const result = audit(t, css + '\nhtml[data-theme="dark"] { --bg: #14161B; }\n' +
    '@media (prefers-contrast: more) { :root { --text: #fff; } }');
  assert.equal(result.status, 0, result.stdout + result.stderr);
});

test('mono-specific low contrast is not hidden by the coloured palettes', t => {
  const result = audit(t, css + '\nhtml[data-palette="mono"] { --text: #fff; }');
  assert.equal(result.status, 1);
  assert.match(result.stdout, /mono\/light/);
});


test('checker rejects the old dark select arrow', t => {
  const result = audit(t, css.replace('%2393969B', '%236B6E73'));
  assert.equal(result.status, 1);
  assert.match(result.stdout, /mono\/dark: стрелка select на вложенной поверхности/);
});
