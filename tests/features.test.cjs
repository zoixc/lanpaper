// SPDX-License-Identifier: MIT
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');
const moduleURL = pathToFileURL(path.join(__dirname, '..', 'static', 'js', 'features.js')).href;

test('feature registry exposes immutable narrow capabilities', async () => {
  const registry = await import(moduleURL);
  registry.registerPanelFacade({ snapshot: () => ({ links: [] }), request() {} });
  registry.registerFeature('link-list', { exportData() {} });
  const facade = registry.getPanelFacade();
  const feature = registry.getFeature('link-list');
  assert.equal(Object.isFrozen(facade), true);
  assert.equal(Object.isFrozen(feature), true);
  assert.deepEqual(Object.keys(facade).sort(), ['request', 'snapshot']);
  assert.deepEqual(Object.keys(feature), ['exportData']);
  assert.equal(registry.getFeature('missing'), null);
});
