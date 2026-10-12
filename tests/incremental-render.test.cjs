// SPDX-License-Identifier: MIT
'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { pathToFileURL } = require('node:url');
const root = path.join(__dirname, '..');
const source = fs.readFileSync(path.join(root, 'static/js/app.js'), 'utf8');

// Execute the actual render functions with a small DOM adapter. No copied
// pagination implementation, no browser dependencies in the Node test job.
for (const size of [0, 8, 12, 13, 25]) {
  test(`render and load more preserve icons and chunk boundaries (${size} links)`, async () => {
    const stateModule = await import(pathToFileURL(path.join(root, 'static/js/state.js')).href);
    const state = stateModule.createAppState({ loading: false,
      links: Array.from({ length: size }, (_, i) => ({ linkName: `link-${i}` })) });
    const nodes = new Map();
    function node() {
      return {
        children: [], hidden: false, textContent: '', classes: new Set(),
        classList: { toggle(name, on) { this.owner.classes[on ? 'add' : 'delete'](name); } },
        setAttribute() {},
        set innerHTML(value) { assert.equal(value, ''); this.children = []; },
        append(child) { this.children.push(...(child.fragment ? child.children : [child])); },
        querySelector(selector) { return $(selector); }
      };
    }
    function $(selector) {
      if (!nodes.has(selector)) {
        const el = node(); el.classList.owner = el; nodes.set(selector, el);
      }
      return nodes.get(selector);
    }
    const context = { state, $, document: { body: $('body'), createDocumentFragment() {
      return Object.assign(node(), { fragment: true });
    } }, visibleLinks: () => state.links, applyReveal() {}, jumpToRevealed() {},
      updateBulkbar() {}, cardFor: link => ({ name: link.linkName, hydrated: false }),
      hydrateIcons: grid => grid.children.forEach(card => { card.hydrated = true; }),
      t: (key, values) => JSON.stringify({ key, ...values }) };
    // Bind only names actually imported from state.js: deleting the import
    // must fail too, rather than being hidden by a hand-injected constant.
    const imports = source.match(/import \{([^}]+)\} from '\.\/state\.js';/)[1];
    for (const name of imports.split(',').map(s => s.trim())) {
      if (/^\w+$/.test(name)) context[name] = stateModule[name];
    }
    vm.createContext(context);
    const functions = ['render', 'renderLoadMore', 'showMore'].map(name => {
      const match = source.match(new RegExp(`    function ${name}\\([^)]*\\) \\{[\\s\\S]*?\\n    \\}`));
      assert.ok(match, `${name} must be exercised`);
      return match[0];
    }).join('\n');
    vm.runInContext(functions, context);
    context.render();
    const chunk = stateModule.RENDER_CHUNK;
    function check(count) {
      assert.equal($('#grid').children.length, count);
      assert.ok($('#grid').children.every(card => card.hydrated));
      assert.equal($('#errorState').classes.has('is-hidden'), true);
      assert.equal($('#loadMore').classes.has('is-hidden'), count === size);
      assert.equal(new Set($('#grid').children.map(card => card.name)).size, count);
      if (count < size) {
        assert.equal(JSON.parse($('#loadMoreBtn').textContent).count, Math.min(chunk, size - count));
      }
    }
    check(Math.min(chunk, size));
    while (state.shown < size) { context.showMore(); check(Math.min(state.shown, size)); }
    context.render(); check(size);
  });
}
