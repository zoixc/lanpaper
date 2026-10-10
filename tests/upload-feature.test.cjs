// SPDX-License-Identifier: MIT
'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');
const moduleURL = pathToFileURL(path.join(__dirname, '..', 'static', 'js', 'upload-feature.js')).href;

test('upload feature owns request serialization and applies server state', async () => {
  const { createUploadFeature } = await import(moduleURL);
  let received; let updated;
  const feature = createUploadFeature({
    compressor: () => null, t: key => key, formatBytes: String,
    toast: () => ({ dismissToast() {} }),
    request: async (url, method, form, isForm) => {
      received = { url, method, form, isForm };
      return { linkName: 'wall', hasImage: true };
    },
    applyUpdate: value => { updated = value; }
  });
  const source = new Blob(['png'], { type: 'image/png' });
  assert.equal(await feature.file({ linkName: 'wall' }, source, 'append'), true);
  assert.deepEqual({ url: received.url, method: received.method, isForm: received.isForm },
    { url: '/api/upload', method: 'POST', isForm: true });
  assert.equal(received.form.get('mode'), 'append');
  assert.equal(updated.hasImage, true);
});

test('upload feature rejects unsupported media before a request', async () => {
  const { createUploadFeature } = await import(moduleURL);
  let requests = 0;
  const feature = createUploadFeature({
    compressor: () => null, t: key => key, formatBytes: String,
    toast: () => ({ dismissToast() {} }), request: async () => { requests += 1; }, applyUpdate() {}
  });
  assert.equal(await feature.file({ linkName: 'wall' }, new Blob(['x'], { type: 'text/plain' }), 'replace'), false);
  assert.equal(requests, 0);
});
