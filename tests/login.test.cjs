// SPDX-License-Identifier: MIT
// Checks the sign-in page against its script: every element the script looks
// up exists, and the page has no inline script or style (the CSP forbids them).
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const ROOT = path.join(__dirname, '..');
const html = fs.readFileSync(path.join(ROOT, 'login.html'), 'utf8');
const script = fs.readFileSync(path.join(ROOT, 'static/js/login.js'), 'utf8');

test('login.js only looks up elements that login.html defines', () => {
  const lookups = [...script.matchAll(/getElementById\('([^']+)'\)/g)].map((m) => m[1]);
  assert.ok(lookups.length >= 3, 'expected the script to look up its elements');
  for (const id of lookups) {
    assert.match(html, new RegExp(`id="${id}"`), `login.html has no #${id}`);
  }
});

test('login.js posts to the session endpoint', () => {
  assert.match(script, /fetch\('\/api\/session'/);
});

test('login.html has no inline script or style', () => {
  assert.doesNotMatch(html, /<script(?![^>]*\ssrc=)[^>]*>/i);
  assert.doesNotMatch(html, /<style[\s>]/i);
  assert.doesNotMatch(html, /\sstyle="/i);
});
