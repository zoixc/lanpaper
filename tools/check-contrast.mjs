#!/usr/bin/env node
/* SPDX-License-Identifier: MIT
 * ============================================================
 * Проверка контраста палитр панели (без зависимостей).
 *
 *   node tools/check-contrast.mjs
 *
 * Скрипт читает static/css/style.css, собирает значения переменных
 * для каждой палитры в обеих темах и считает WCAG-контраст пар,
 * которые реально встречаются в интерфейсе. Ненулевой код выхода —
 * значит какая-то пара ниже порога.
 * ============================================================ */
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const raw = readFileSync(join(here, '..', 'static', 'css', 'style.css'), 'utf8') +
    (process.argv.includes('--studio') ? readFileSync(join(here, '..', 'static', 'css', 'studio.css'), 'utf8') : '');

/* Блоки @media вырезаем целиком: внутри них те же :root, но со
   «усиленным» контрастом — это отдельный режим, а не база. */
function stripAtRules(src, name) {
    let out = '';
    let i = 0;
    while (i < src.length) {
        if (src.startsWith(name, i)) {
            let depth = 0;
            let k = src.indexOf('{', i);
            if (k === -1) break;
            for (; k < src.length; k++) {
                if (src[k] === '{') depth++;
                else if (src[k] === '}') { depth--; if (depth === 0) break; }
            }
            i = k + 1;
        } else { out += src[i]; i++; }
    }
    return out;
}
const css = stripAtRules(raw.replace(/\/\*[\s\S]*?\*\//g, ''), '@media');

/* --- разбор CSS: все объявления в порядке появления --- */
function declarations(source) {
    const out = [];
    /* Комментарии убираем до разбора: переменая после комментария
       в той же строке иначе не находится. */
    const blocks = source.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{}]+)\{([^{}]*)\}/g);
    for (const [, selector, body] of blocks) {
        const sel = selector.trim();
        const vars = {};
        for (const line of body.split(';')) {
            const m = line.match(/^\s*(--[\w-]+)\s*:\s*([^;]+)$/);
            if (m) vars[m[1]] = m[2].trim();
        }
        if (Object.keys(vars).length) out.push({ sel, vars });
    }
    return out;
}

const all = declarations(css);
const palettes = ['mono', 'indigo', 'sage', 'clay', 'graphite', 'ocean'];

function themeVars(palette, theme) {
    // Specificity first, source order second: a combined palette/theme selector
    // beats a later generic theme rule, just as it does in the browser.
    const rules = all.flatMap(({ sel, vars }, order) => sel.split(',').map(selector => ({
        sel: selector.trim(), vars, order,
        specificity: (selector.match(/\[|:root|\.studio/g) || []).length
    }))).filter(({ sel }) => {
        sel = sel.replace(/^:root/, 'html').replace(/^\.studio/, 'html').replace(/\.studio/g, '');
        if (sel === 'html') return true;
        if (!/^html(?:\[data-(?:theme|palette)="[a-z]+"\])+$/.test(sel)) return false;
        return [...sel.matchAll(/\[data-(theme|palette)="([a-z]+)"\]/g)]
            .every(([, key, value]) => value === (key === 'theme' ? theme : palette));
    }).sort((a, b) => a.specificity - b.specificity || a.order - b.order);
    return Object.assign({}, ...rules.map(rule => rule.vars));
}

/* --- WCAG --- */
const hex = (h) => {
    h = h.replace('#', '');
    if (h.length === 3) h = [...h].map((c) => c + c).join('');
    return [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16));
};
const lin = (c) => { c /= 255; return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4; };
const lum = (h) => { const [r, g, b] = hex(h); return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b); };
const ratio = (a, b) => { const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p); return (x + 0.05) / (y + 0.05); };

/* Пары «что на чём» и порог. 4.5 — обычный текст, 3.0 — крупный и
   элементы управления (значки, акцентные заливки). 1.1 для декоративной
   границы — только дизайн-порог, не требование WCAG к контролам. */
const CHECKS = [
    ['--text', '--bg', 7, 'основной текст на фоне'],
    ['--text', '--surface', 7, 'основной текст на карточке'],
    ['--text-2', '--surface', 4.5, 'вторичный текст на карточке'],
    ['--text-2', '--bg', 4.5, 'вторичный текст на фоне'],
    ['--text-3', '--surface', 4.5, 'подписи и мета на карточке'],
    ['--text-3', '--bg', 4.5, 'подписи и мета на фоне'],
    ['--accent-text', '--surface', 4.5, 'акцентный текст на карточке'],
    ['--accent', '--surface', 3, 'акцентный элемент на карточке (границы, значки)'],
    ['--border-2', '--surface', 1.1, 'границы различимы на карточке'],
    ['--text', '--surface-2', 4.5, 'текст на вложенной поверхности'],
    ['--text-2', '--surface-2', 4.5, 'вторичный текст на вложенной поверхности'],
    ['--text-3', '--surface-2', 4.5, 'мета на вложенной поверхности'],
    ['--text-3', '--surface-hover', 4.5, 'мета при наведении']
];
const ON_ACCENT = ['--text-on-accent', '--accent', 4.5, 'текст на акцентной кнопке'];
const DANGER = ['--danger', '--surface', 4.5, 'текст удаления на карточке'];

// The select chevron is an encoded SVG, not a CSS text colour. Audit its
// actual stroke too, so a future edit cannot silently restore the old arrow.
function selectArrow(theme) {
    const selector = theme === 'dark' ? 'html[data-theme="dark"] .select' : '.select';
    const blocks = [...css.matchAll(/([^{}]+)\{([^{}]*)\}/g)];
    const body = blocks.find(([, sel]) => sel.trim() === selector)?.[2] || '';
    const stroke = body.match(/stroke='%23([\da-f]{6})'/i);
    return stroke ? '#' + stroke[1] : undefined;
}

const table = process.argv.includes('--table');
let fails = 0;
let checks = 0;
const rows = [];
for (const palette of palettes) {
    for (const theme of ['light', 'dark']) {
        const vars = themeVars(palette, theme);
        vars['--select-arrow'] = selectArrow(theme);
        const list = [...CHECKS,
            ['--select-arrow', '--surface', 3, 'стрелка select'],
            ['--select-arrow', '--surface-2', 3, 'стрелка select на вложенной поверхности'], theme === 'light' ? ON_ACCENT : ['--text-on-accent', '--accent', 4.5, 'тёмный текст на светлой кнопке'], DANGER];
        let worst = { value: 99, label: '' };
        for (const [fg, bg, min, label] of list) {
            const a = vars[fg];
            const b = vars[bg];
            if (!a || !b) { console.log(`  ? нет переменной ${!a ? fg : bg} (${palette}/${theme})`); fails++; continue; }
            if (![a, b].every(c => /^#(?:[\da-f]{3}|[\da-f]{6})$/i.test(c))) {
                console.error(`  ✗ unsupported colour: ${fg}=${a}, ${bg}=${b}`);
                fails++;
                continue;
            }
            const value = ratio(a, b);
            checks++;
            if (value < min) {
                fails++;
                console.log(`  ✗ ${palette}/${theme}: ${label} — ${value.toFixed(2)} < ${min}`);
            }
            if (value < worst.value) worst = { value, label };
            if (table) console.log(`  ${palette}/${theme} ${label}: ${value.toFixed(2)}:1`);
        }
        rows.push(`  ${palette.padEnd(9)} ${theme.padEnd(5)} минимум ${worst.value.toFixed(2)}:1 — ${worst.label}`);
    }
}
console.log('Контраст по палитрам (худшая пара из проверяемых):');
rows.forEach((r) => console.log(r));
console.log(`\nПроверок: ${checks}, провалов: ${fails}`);
process.exit(fails ? 1 : 0);
