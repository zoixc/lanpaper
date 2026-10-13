// SPDX-License-Identifier: MIT
const test = require('node:test');
const assert = require('node:assert/strict');

function documentFor(errorName) {
  return { createElement(tag) {
    const attrs = new Map(), listeners = new Map();
    return {
      tag, children: [],
      setAttribute(k, v) { attrs.set(k, v); },
      hasAttribute(k) { return attrs.has(k); },
      removeAttribute(k) { attrs.delete(k); },
      set src(v) { attrs.set('src', v); },
      append(el) { this.children.push(el); },
      contains(el) { return this.children.includes(el); },
      replaceChildren(...els) { this.children = els; },
      addEventListener(k, fn) { listeners.set(k, fn); },
      removeEventListener(k) { listeners.delete(k); },
      fire(k) { listeners.get(k)?.(); },
      querySelectorAll(tag) { return this.children.filter(el => el.tag === tag); },
      play() { return Promise.reject(Object.assign(new Error(), { name: errorName })); },
      pause() { this.paused = true; }, load() {}
    };
  } };
}

for (const name of ['NotSupportedError', 'NotAllowedError', 'AbortError']) {
  test(`video handles ${name} without an unhandled rejection`, async () => {
    const { createVideoPreview } = await import('../static/js/media-preview.js');
    const box = createVideoPreview(documentFor(name), { src: '/api/preview/demo', label: 'Play', error: 'Cannot play' });
    assert.equal(box.children[0].tag, 'button');
    box.children[0].fire('click');
    const video = box.children[0];
    await new Promise(resolve => setImmediate(resolve));
    if (name === 'NotSupportedError') {
      assert.equal(box.children[0].textContent, 'Cannot play');
      assert.equal(video.paused, true);
      assert.equal(video.hasAttribute('src'), false);
    } else {
      assert.equal(box.children[0], video);
      assert.equal(video.controls, true);
    }
  });
}

test('late unsupported-playback rejection does not resurrect disposed media', async () => {
  const { createVideoPreview, stopMedia } = await import('../static/js/media-preview.js');
  const box = createVideoPreview(documentFor('NotSupportedError'), { src: '/api/preview/demo' });
  box.children[0].fire('click');
  const video = box.children[0];
  stopMedia(box);
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(box.children[0], video);
  assert.equal(video.hasAttribute('src'), false);
});
