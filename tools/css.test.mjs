// SPDX-License-Identifier: MIT
import test from 'node:test';
import assert from 'node:assert/strict';
import { checkCSS } from './check-css.mjs';

test('formatted and compact CSS have the same structural checks', () => {
    const a = ':root { --s: 8px; }\n@media (max-width: 720px) { .card { padding: var(--s); } }';
    assert.deepEqual(checkCSS(a), []);
    assert.deepEqual(checkCSS(a.replace(/\s+/g, ' ')), []);
});
test('duplicate declarations are reported without rejecting viewport fallbacks', () => {
    assert.deepEqual(checkCSS('.page { min-height: 100vh; min-height: 100dvh; }'), []);
    assert.match(checkCSS('.card { padding: 8px; padding: 8px; }')[0], /duplicate declaration padding/);
    assert.match(checkCSS(':root { --s: 8px; --s: 9px; }')[0], /duplicate declaration --s/);
});
test('unknown tokens are reported, including misspelled fallback references', () => {
    assert.match(checkCSS('.card { padding: var(--typo, 8px); }')[0], /unknown token --typo/);
});
test('comments and data URLs do not become declarations', () => {
    assert.deepEqual(checkCSS('.a { content: "var(--not-a-token)"; }'), []);
    assert.deepEqual(checkCSS('/* --unused: var(--missing); */ .a { background: url("data:image/svg+xml;a:b"); }'), []);
});
test('broken block syntax and top-level declarations fail', () => {
    assert.throws(() => checkCSS('.a { padding: 8px;'), /Unclosed block/);
    assert.match(checkCSS('padding: 8px;')[0], /outside a rule/);
});
