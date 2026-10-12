#!/usr/bin/env node
/* SPDX-License-Identifier: MIT
 * Focused structural guard, not a complete CSS validator. Uses PostCSS so
 * comments, strings, data URLs and nested media rules are parsed correctly.
 * No rewriting or property sorting: fallbacks and cascade order matter.
 */
import postcss from 'postcss';
import valueParser from 'postcss-value-parser';
import { readFileSync } from 'node:fs';
import { fileURLToPath, pathToFileURL } from 'node:url';

export function checkCSS(source) {
    const root = postcss.parse(source);
    const errors = [];
    const tokens = new Set();
    root.walkDecls(/^--/, decl => tokens.add(decl.prop));
    root.walk(node => {
        if (node.type !== 'rule' && node.type !== 'atrule') return;
        const seen = new Map();
        for (const decl of node.nodes || []) {
            if (decl.type !== 'decl') continue;
            const name = decl.prop.startsWith('--') ? decl.prop : decl.prop.toLowerCase();
            const previous = seen.get(name);
            if (previous && (name.startsWith('--') ||
                (previous.value === decl.value && previous.important === decl.important))) {
                errors.push(`${decl.source.start.line}: duplicate declaration ${name}`);
            }
            seen.set(name, decl);
        }
    });
    root.walkDecls(decl => {
        if (decl.parent.type === 'root') errors.push(`${decl.source.start.line}: declaration outside a rule`);
        valueParser(decl.value).walk(node => {
            if (node.type !== 'function' || node.value !== 'var') return;
            const name = node.nodes.find(part => part.type === 'word')?.value;
            if (!name || !name.startsWith('--')) {
                errors.push(`${decl.source.start.line}: invalid var() reference`);
            } else if (!tokens.has(name)) {
                errors.push(`${decl.source.start.line}: unknown token ${name}`);
            }
        });
    });
    return errors;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
    const file = fileURLToPath(new URL('../static/css/style.css', import.meta.url));
    const errors = checkCSS(readFileSync(file, 'utf8'));
    if (errors.length) {
        console.error(errors.join('\n'));
        process.exitCode = 1;
    } else console.log('CSS structure, duplicate declarations and token references: OK');
}
