// SPDX-License-Identifier: MIT
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');

const moduleURL = pathToFileURL(path.join(__dirname, '..', 'static', 'js', 'api.js')).href;

function response(status, body = '', contentType = 'text/plain', code = '') {
  return {
    status,
    ok: status >= 200 && status < 300,
    headers: { get(name) { if (name === 'content-type') return contentType; if (name === 'x-error-code') return code; return ''; } },
    async text() { return body; },
    async json() { return JSON.parse(body); },
  };
}

test('API module serializes JSON and decodes success', async () => {
  const { request } = await import(moduleURL);
  let received;
  global.fetch = async (url, options) => { received = { url, options }; return response(200, '{"ok":true}', 'application/json'); };
  assert.deepEqual(await request('/api/link', { method: 'POST', body: { linkName: 'wall' } }), { ok: true });
  assert.equal(received.options.credentials, 'same-origin');
  assert.equal(received.options.body, '{"linkName":"wall"}');
});

test('API module returns typed authentication and machine-code errors', async () => {
  const { request, ApiError } = await import(moduleURL);
  global.fetch = async () => response(401);
  await assert.rejects(request('/api/wallpapers'), (error) => error instanceof ApiError && error.code === 'session_expired');
  global.fetch = async () => response(429, 'slow down', 'text/plain', 'rate_limited');
  await assert.rejects(request('/api/upload'), (error) => error instanceof ApiError && error.code === 'rate_limited' && error.retryable);
});

test('API module distinguishes cancellation from retryable network failure', async () => {
  const { request } = await import(moduleURL);
  global.fetch = async () => { const error = new Error('cancel'); error.name = 'AbortError'; throw error; };
  await assert.rejects(request('/api/x'), (error) => error.kind === 'cancelled' && !error.retryable);
  global.fetch = async () => { throw new Error('offline'); };
  await assert.rejects(request('/api/x'), (error) => error.kind === 'network' && error.retryable);
});
