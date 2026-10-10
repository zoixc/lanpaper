// SPDX-License-Identifier: MIT
'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');
const moduleURL = pathToFileURL(path.join(__dirname, '..', 'static', 'js', 'operation-state.js')).href;

test('operation state rejects duplicate submissions until completion', async () => {
  const { createOperationState } = await import(moduleURL);
  const state = createOperationState();
  assert.equal(state.begin('upload:wall'), true);
  assert.equal(state.begin('upload:wall'), false);
  assert.equal(state.size(), 1);
  state.end('upload:wall');
  assert.equal(state.begin('upload:wall'), true);
});

test('connectivity observer reports transitions and can unsubscribe', async () => {
  const { observeConnectivity } = await import(moduleURL);
  const listeners = new Map();
  const target = {
    navigator: { onLine: false },
    addEventListener(name, fn) { listeners.set(name, fn); },
    removeEventListener(name) { listeners.delete(name); }
  };
  const states = [];
  const stop = observeConnectivity(target, value => states.push(value));
  target.navigator.onLine = true;
  listeners.get('online')();
  assert.deepEqual(states, [false, true]);
  stop();
  assert.equal(listeners.size, 0);
});
