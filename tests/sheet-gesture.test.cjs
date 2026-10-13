// SPDX-License-Identifier: MIT
const test = require('node:test');
const assert = require('node:assert/strict');
async function harness(allowed = true) {
  const { attachSheetGesture } = await import('../static/js/sheet-gesture.js');
  const handlers = new Map();
  let dismissed = 0;
  const grip = { addEventListener: (n, f) => handlers.set(n, f), removeEventListener: n => handlers.delete(n), setPointerCapture() {} };
  const cleanup = attachSheetGesture(grip, { canDismiss: () => allowed, dismiss: () => dismissed++ });
  const send = (type, values = {}) => handlers.get(type)?.({ isPrimary: true, pointerType: 'touch', pointerId: 1, clientX: 0, clientY: 0, timeStamp: 0, preventDefault() {}, ...values });
  return { send, cleanup, handlers, count: () => dismissed };
}
test('grip closes only on a deliberate downward touch swipe; subsequent click is suppressed', async () => {
  const h = await harness();
  h.send('pointerdown'); h.send('pointerup', { clientY: 80, timeStamp: 200 }); h.send('click');
  assert.equal(h.count(), 1);
});
for (const [name, end] of Object.entries({ short: { clientY: 40 }, up: { clientY: -90 }, horizontal: { clientX: 90, clientY: 80 }, slow: { clientY: 80, timeStamp: 1500 }, otherPointer: { pointerId: 2, clientY: 80 } })) {
  test(`grip ignores ${name} gestures`, async () => {
    const h = await harness(); h.send('pointerdown'); h.send('pointerup', end); assert.equal(h.count(), 0);
  });
}
test('persistent/covered sheets cannot be dismissed by touch or click', async () => {
  const h = await harness(false); h.send('pointerdown'); h.send('pointerup', { clientY: 100 }); h.send('click'); assert.equal(h.count(), 0);
});
test('pointer cancellation, non-primary and mouse pointers never swipe-dismiss', async () => {
  const h = await harness();
  h.send('pointerdown'); h.send('pointercancel'); h.send('pointerup', { clientY: 100 });
  h.send('pointerdown', { pointerType: 'mouse' }); h.send('pointerup', { clientY: 100 });
  h.send('pointerdown', { isPrimary: false }); h.send('pointerup', { clientY: 100 });
  assert.equal(h.count(), 0);
});
test('native button click/keyboard activation closes; disposal removes every listener', async () => {
  const h = await harness(); h.send('click'); assert.equal(h.count(), 1);
  h.cleanup(); assert.equal(h.handlers.size, 0); h.send('click'); assert.equal(h.count(), 1);
});
