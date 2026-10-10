// SPDX-License-Identifier: MIT
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');
const moduleURL = pathToFileURL(path.join(__dirname, '..', 'static', 'js', 'state.js')).href;

test('state selectors compose query, access, filters and pinned ordering', async () => {
  const { createAppState, normalizeLink, selectVisibleLinks } = await import(moduleURL);
  const state = createAppState({
    query: 'png', scope: 'name', filter: 'image', access: 'public', sort: 'name_asc',
    links: [
      normalizeLink({ linkName: 'z-png', mimeType: 'png', hasImage: true, accessLevel: 'public' }),
      normalizeLink({ linkName: 'a-png', mimeType: 'png', hasImage: true, accessLevel: 'public', pinned: true }),
      normalizeLink({ linkName: 'private-png', mimeType: 'png', hasImage: true, accessLevel: 'auth' }),
      normalizeLink({ linkName: 'clip-png', mimeType: 'mp4', hasImage: true, accessLevel: 'public' })
    ]
  });
  assert.deepEqual(selectVisibleLinks(state).map(link => link.linkName), ['a-png', 'z-png']);
});

test('incremental rendering resets after a state transition', async () => {
  const { createAppState, resetIncrementalRender, RENDER_CHUNK } = await import(moduleURL);
  const state = createAppState({ shown: 96, reveal: new Set(['wall']), scrolledReveal: 'wall' });
  resetIncrementalRender(state);
  assert.equal(state.shown, RENDER_CHUNK);
  assert.equal(state.reveal, null);
  assert.equal(state.scrolledReveal, null);
});

test('normalization creates independent safe collection defaults', async () => {
  const { normalizeLink } = await import(moduleURL);
  const first = normalizeLink({ id: 'first' });
  const second = normalizeLink({ id: 'second', currentVersion: 0 });
  first.items.push('changed');
  assert.deepEqual(second.items, []);
  assert.equal(second.currentVersion, 1);
});
