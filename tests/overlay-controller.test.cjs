// SPDX-License-Identifier: MIT
'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { pathToFileURL } = require('node:url');
const path = require('node:path');
const moduleURL = pathToFileURL(path.join(__dirname, '..', 'static', 'js', 'overlay-controller.js')).href;

function element(label = '') {
  const attrs = new Map();
  return {
    isConnected: true, inert: false, removed: false, focused: false, offsetParent: {},
    getAttribute(name) { return name === 'aria-label' ? label : (attrs.get(name) || ''); },
    setAttribute(name, value) { attrs.set(name, value); },
    removeAttribute(name) { attrs.delete(name); },
    querySelector() { return null; }, querySelectorAll() { return []; },
    contains(other) { return other === this; },
    remove() { this.removed = true; this.isConnected = false; },
    focus() { this.focused = true; }
  };
}

test('overlay controller stacks nested dialogs and restores the parent', async () => {
  const { createOverlayController } = await import(moduleURL);
  const origin = element();
  const changes = [];
  const host = { append() {} };
  const doc = { activeElement: origin, body: element() };
  const controller = createOverlayController({
    document: doc, host: () => host, createScrim: () => element(),
    onChange: (active, depth) => changes.push([active && active.kind, depth])
  });
  const panel = element('Panel');
  controller.open(panel, { kind: 'panel' });
  const dialog = element('Confirm');
  controller.open(dialog, { kind: 'dialog', stack: true });
  assert.equal(controller.depth(), 2);
  assert.equal(panel.inert, true);
  assert.equal(panel.getAttribute('aria-hidden'), 'true');
  controller.close();
  assert.equal(controller.current().node, panel);
  assert.equal(panel.inert, false);
  assert.equal(panel.getAttribute('aria-hidden'), '');
  assert.deepEqual(changes.at(-1), ['panel', 1]);
});

test('Escape dismisses the active overlay and returns focus', async () => {
  const { createOverlayController } = await import(moduleURL);
  const origin = element();
  const doc = { activeElement: origin, body: element() };
  let dismissed = 0; let prevented = false;
  const controller = createOverlayController({
    document: doc, host: () => ({ append() {} }), createScrim: () => element(), onChange() {}
  });
  controller.open(element(), { onDismiss: () => { dismissed += 1; } });
  controller.keydown({ key: 'Escape', preventDefault() { prevented = true; } });
  assert.equal(controller.depth(), 0);
  assert.equal(dismissed, 1);
  assert.equal(origin.focused, true);
  assert.equal(prevented, true);
});
