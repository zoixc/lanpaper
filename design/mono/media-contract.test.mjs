// SPDX-License-Identifier: MIT
// Run after npm ci --prefix tools: node --test design/mono/media-contract.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
const require = createRequire(new URL('../../tools/package.json', import.meta.url));
const postcss = require('postcss');
const parse = path => postcss.parse(readFileSync(new URL(path, import.meta.url), 'utf8'));
const base = parse('../../static/css/style.css');
const studio = parse('../../static/css/studio.css');
const compact = text => text.replace(/\s+/g, '');
function declarations(css, selector, props) {
    const result = [];
    css.walkRules(selector, rule => {
        const media = rule.parent.type === 'atrule' ? compact(rule.parent.params) : '';
        for (const decl of rule.nodes) {
            if (decl.type === 'decl' && props.includes(decl.prop)) result.push([media, decl.prop, compact(decl.value)]);
        }
    });
    return result;
}
test('Studio grid uses all production sizing breakpoints and gaps in the same order', () => {
    const props = ['grid-template-columns', 'gap'];
    assert.deepEqual(declarations(studio, '.studio .grid:not(.grid--list)', props), declarations(base, '.grid', props));
});
test('Studio does not override production image fitting, zoom or list thumbnail widths', () => {
    assert.deepEqual(declarations(studio, '.studio .card__media', ['object-fit']), []);
    assert.deepEqual(declarations(studio, '.studio .grid--list .card__media', ['object-fit']), []);
    assert.deepEqual(declarations(studio, '.studio .card:hover .card__media', ['transform']), []);
    assert.deepEqual(declarations(studio, '.studio .grid--list .card__frame', ['width', 'min-height', 'aspect-ratio']), []);
    assert.deepEqual(declarations(studio, '.studio .grid:not(.grid--list) .card:only-child', ['max-width']), []);
});
test('all grid frames, including the first, use production 16:9', () => {
    const frames = [];
    studio.walkRules(rule => {
        if (!rule.selector.includes('.studio .grid:not(.grid--list)') || !rule.selector.includes('.card__frame')) return;
        rule.walkDecls('aspect-ratio', decl => frames.push(compact(decl.value)));
    });
    assert.deepEqual(frames, ['16/9']);
    assert.deepEqual(declarations(base, '.card__frame', ['aspect-ratio']), [['', 'aspect-ratio', '16/9']]);
});
test('toggle rotates clockwise on mobile in both states, desktop keeps its direction', () => {
    assert.deepEqual(declarations(studio, '.studio .studio-nav-toggle svg', ['transform']), [
        ['', 'transform', 'none'], ['(max-width:760px)', 'transform', 'rotate(90deg)']
    ]);
    assert.deepEqual(declarations(studio, '.studio.sidebar-collapsed .studio-nav-toggle svg', ['transform']), [
        ['', 'transform', 'scaleX(-1)'], ['(max-width:760px)', 'transform', 'rotate(90deg)scaleX(-1)']
    ]);
});
