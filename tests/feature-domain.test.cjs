// SPDX-License-Identifier: MIT
'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');
const moduleURL = pathToFileURL(path.join(__dirname, '..', 'static', 'js', 'feature-domain.js')).href;

test('access and history models expose stable display metadata', async () => {
  const { accessMeta, entryMeta, entryTitle } = await import(moduleURL);
  const bytes = value => `${value} B`;
  assert.deepEqual(accessMeta('token'), { icon: 'key', key: 'access_token' });
  assert.deepEqual(accessMeta('unknown'), { icon: 'globe', key: 'access_public' });
  assert.equal(entryMeta({ ext: 'png', sizeBytes: 12 }, bytes), 'PNG · 12 B');
  assert.equal(entryTitle({ sizeBytes: 7 }, bytes), '7 B');
});

test('upload model serializes append mode without changing replace contracts', async () => {
  const { uploadForm } = await import(moduleURL);
  const appended = uploadForm('wall', 'append', 'url', 'https://example.test/a.png');
  assert.equal(appended.get('linkName'), 'wall');
  assert.equal(appended.get('mode'), 'append');
  assert.equal(appended.get('url'), 'https://example.test/a.png');
  const replaced = uploadForm('wall', 'replace', 'url', '/gallery/a.png');
  assert.equal(replaced.has('mode'), false);
});

test('settings/export snapshot cannot mutate top-level panel preferences', async () => {
  const { panelSnapshot } = await import(moduleURL);
  const state = { lang: 'de', theme: 'dark', view: 'list', sort: 'name_asc', links: [{ linkName: 'wall' }] };
  const snapshot = panelSnapshot(state);
  assert.equal(Object.isFrozen(snapshot), true);
  snapshot.links[0].linkName = 'copy';
  assert.equal(state.links[0].linkName, 'wall');
});
