// SPDX-License-Identifier: MIT
'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');
const moduleURL = pathToFileURL(path.join(__dirname, '..', 'static', 'js', 'upload-feature.js')).href;

test('upload feature suppresses a duplicate submission for the same link', async () => {
  const { createUploadFeature } = await import(moduleURL);
  let release; let requests = 0;
  const blocked = new Promise(resolve => { release = resolve; });
  const feature = createUploadFeature({
    compressor: () => null, t: key => key, formatBytes: String,
    toast: () => ({ dismissToast() {} }),
    request: async () => { requests += 1; await blocked; return { linkName: 'wall' }; },
    applyUpdate() {}
  });
  const source = new Blob(['png'], { type: 'image/png' });
  const first = feature.file({ linkName: 'wall' }, source, 'replace');
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.equal(await feature.url({ linkName: 'wall' }, 'https://example.test/wall.png', 'replace'), false);
  assert.equal(requests, 1);
  release();
  assert.equal(await first, true);
});
