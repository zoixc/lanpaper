/* SPDX-License-Identifier: MIT */
/* ============================================================
   LANPAPER 2.0 — логика панели.

   Порт макета design/v2/app.js: разметка, классы и состояния те же,
   источник данных — настоящее API. Ручки, формат ответов и правила
   проверки здесь ровно те же, что у сервера: панель не должна
   предлагать имя, которое сервер отвергнет, и не должна показывать
   состояние, которого нет.
   ============================================================ */
import { request, ApiError } from './api.js';
import { registerPanelFacade, getFeature } from './features.js';
import { accessMeta, entryMeta as formatEntryMeta, entryTitle as formatEntryTitle,
    panelSnapshot } from './feature-domain.js';
import { createUploadFeature } from './upload-feature.js';
import { createOverlayController } from './overlay-controller.js';
import { observeConnectivity } from './operation-state.js';
import { createAppState, normalizeLink, mediaExt, isVideoMedia, matchesQuery as linkMatchesQuery,
    countFilteredLinks, resetIncrementalRender, selectVisibleLinks } from './state.js';

(function () {
    'use strict';

    /* ========================================================
       1. МЕЛОЧИ: выборка, создание узлов, формат значений
       ======================================================== */
    const $ = (sel, root) => (root || document).querySelector(sel);
    const $$ = (sel, root) => Array.from((root || document).querySelectorAll(sel));

    function h(tag, props) {
        const el = document.createElement(tag);
        const attrs = props || {};
        for (const key of Object.keys(attrs)) {
            const val = attrs[key];
            if (val === null || val === undefined || val === false) continue;
            if (key === 'class') el.className = val;
            else if (key === 'text') el.textContent = val;
            else if (key === 'html') el.innerHTML = val;
            /* CSSOM, а не атрибут style: панель живёт под CSP без
               'unsafe-inline', и атрибут браузер бы проигнорировал. */
            else if (key === 'style') el.style.cssText = val;
            else if (key.startsWith('on')) el.addEventListener(key.slice(2).toLowerCase(), val);
            else el.setAttribute(key, val === true ? '' : val);
        }
        for (let i = 2; i < arguments.length; i++) {
            const kids = [].concat(arguments[i]);
            for (const kid of kids) {
                if (kid === null || kid === undefined || kid === false) continue;
                el.append(kid instanceof Node ? kid : document.createTextNode(String(kid)));
            }
        }
        return el;
    }

    /* Иконки: один спрайт в JS вместо сотни строк разметки */
    const ICONS = {
        plus: '<line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>',
        search: '<circle cx="11" cy="11" r="7"/><line x1="16.4" y1="16.4" x2="21" y2="21"/>',
        x: '<line x1="6.5" y1="6.5" x2="17.5" y2="17.5"/><line x1="17.5" y1="6.5" x2="6.5" y2="17.5"/>',
        check: '<polyline points="20 6.5 9.5 17 4 11.5"/>',
        grid: '<rect x="3" y="3" width="7.5" height="7.5" rx="1.8"/><rect x="13.5" y="3" width="7.5" height="7.5" rx="1.8"/><rect x="3" y="13.5" width="7.5" height="7.5" rx="1.8"/><rect x="13.5" y="13.5" width="7.5" height="7.5" rx="1.8"/>',
        list: '<line x1="8.5" y1="6" x2="21" y2="6"/><line x1="8.5" y1="12" x2="21" y2="12"/><line x1="8.5" y1="18" x2="21" y2="18"/><line x1="3.6" y1="6" x2="3.61" y2="6"/><line x1="3.6" y1="12" x2="3.61" y2="12"/><line x1="3.6" y1="18" x2="3.61" y2="18"/>',
        /* Солнце — контур из панели 0.12.1 (Feather): круг r=4.5 и восемь
           лучей от r=8.5 до r=10.5. Лучи одинаковой длины и начинаются на
           одном радиусе, поэтому знак не «разъезжается» на 17 px. */
        sun: '<circle cx="12" cy="12" r="4.5"/><line x1="12" y1="1.5" x2="12" y2="3.5"/><line x1="12" y1="20.5" x2="12" y2="22.5"/><line x1="4.6" y1="4.6" x2="6" y2="6"/><line x1="18" y1="18" x2="19.4" y2="19.4"/><line x1="1.5" y1="12" x2="3.5" y2="12"/><line x1="20.5" y1="12" x2="22.5" y2="12"/><line x1="4.6" y1="19.4" x2="6" y2="18"/><line x1="18" y1="6" x2="19.4" y2="4.6"/>',
        /* Луна — тоже из набора 0.12.1 (Feather). У прежнего контура
           внешняя и внутренняя дуги были разного радиуса (8.5 и 8.6),
           поэтому месяц выходил слегка перекошенным. */
        moon: '<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>',
        /* Шестерёнка — та же, что в панели 0.12.1 (Feather, 24×24): у
           нарисованной здесь «звезды» лучи разной длины, и на 17 px она
           выглядела кривой. */
        gear: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>',
        select: '<path d="M9 4.5H6.5A2 2 0 0 0 4.5 6.5V9"/><path d="M15 4.5h2.5a2 2 0 0 1 2 2V9"/><path d="M9 19.5H6.5a2 2 0 0 1-2-2V15"/><path d="M15 19.5h2.5a2 2 0 0 0 2-2V15"/><polyline points="9 12 11 14 15.5 9.5"/>',
        more: '<circle cx="5.5" cy="12" r="1.7"/><circle cx="12" cy="12" r="1.7"/><circle cx="18.5" cy="12" r="1.7"/>',
        copy: '<rect x="9" y="9" width="12" height="12" rx="2.6"/><path d="M15 5.6A2.6 2.6 0 0 0 12.4 3H5.6A2.6 2.6 0 0 0 3 5.6v6.8A2.6 2.6 0 0 0 5.6 15"/>',
        external: '<path d="M18.5 13.6V19a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7.5a2 2 0 0 1 2-2h5.4"/><polyline points="14.5 3 21 3 21 9.5"/><line x1="10" y1="14" x2="20.5" y2="3.5"/>',
        pin: '<path d="M19 21.5l-7-5-7 5V5.5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z"/>',
        trash: '<polyline points="3.5 6.5 5.5 6.5 20.5 6.5"/><path d="M18.5 6.5l-1 13a2 2 0 0 1-2 1.9H8.5a2 2 0 0 1-2-1.9l-1-13"/><path d="M10 11v6M14 11v6"/><path d="M9 6.5V4.8A1.3 1.3 0 0 1 10.3 3.5h3.4A1.3 1.3 0 0 1 15 4.8v1.7"/>',
        image: '<rect x="3" y="3" width="18" height="18" rx="3.4"/><circle cx="8.8" cy="9.2" r="1.7"/><path d="M20.5 15.2l-4.3-4.2L6.5 20.8"/>',
        imageOff: '<rect x="3" y="3" width="18" height="18" rx="3.4"/><line x1="4.5" y1="19.5" x2="19.5" y2="4.5"/>',
        film: '<rect x="2.5" y="4" width="19" height="16" rx="3"/><line x1="7.5" y1="4" x2="7.5" y2="20"/><line x1="16.5" y1="4" x2="16.5" y2="20"/><line x1="2.5" y1="12" x2="21.5" y2="12"/>',
        playlist: '<line x1="3.5" y1="7" x2="15" y2="7"/><line x1="3.5" y1="12" x2="15" y2="12"/><line x1="3.5" y1="17" x2="11" y2="17"/><polygon points="17 13.5 22 16.5 17 19.5"/>',
        folder: '<path d="M3 7.5A2.5 2.5 0 0 1 5.5 5h3.3a2 2 0 0 1 1.6.8l1 1.4h7.1A2.5 2.5 0 0 1 21 9.7V17a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 17z"/>',
        globe: '<circle cx="12" cy="12" r="9"/><path d="M3.2 12h17.6"/><path d="M12 3a14 14 0 0 1 0 18 14 14 0 0 1 0-18z"/>',
        logout: '<path d="M15 4.5h2.5a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H15"/><polyline points="10 8 6 12 10 16"/><line x1="6" y1="12" x2="15" y2="12"/>',
        lock: '<rect x="4.5" y="10.5" width="15" height="10" rx="2.6"/><path d="M8 10.5V7.6a4 4 0 0 1 8 0v2.9"/>',
        key: '<circle cx="8" cy="15.5" r="3.6"/><path d="M10.7 12.8L20 3.5"/><path d="M17 4.5l2.5 2.5"/><path d="M14.5 7l2.5 2.5"/>',
        /* «Как в системе» в переключателе тем — монитор (Feather). */
        monitor: '<rect x="2.5" y="3.5" width="19" height="13" rx="2.4"/><line x1="8" y1="20.5" x2="16" y2="20.5"/><line x1="12" y1="16.5" x2="12" y2="20.5"/>',
        user: '<circle cx="12" cy="8.2" r="3.9"/><path d="M4.8 20.2a7.4 7.4 0 0 1 14.4 0"/>',
        refresh: '<polyline points="22 4.5 22 10 16.5 10"/><path d="M20.2 15a8.5 8.5 0 1 1-2-8.8L22 10"/>',
        rotate: '<polyline points="22 4.5 22 10 16.5 10"/><polyline points="2 19.5 2 14 7.5 14"/><path d="M4.6 9.4a8.5 8.5 0 0 1 14-3.2L22 10"/><path d="M2 14l3.4 3.8A8.5 8.5 0 0 0 19.4 14.6"/>',
        download: '<path d="M21 15.5V19a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-3.5"/><polyline points="7.5 10.5 12 15 16.5 10.5"/><line x1="12" y1="15" x2="12" y2="3"/>',
        upload: '<path d="M21 15.5V19a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-3.5"/><polyline points="16.5 8 12 3.5 7.5 8"/><line x1="12" y1="3.5" x2="12" y2="15"/>',
        clock: '<circle cx="12" cy="12" r="9"/><polyline points="12 7 12 12.3 15.4 14.3"/>',
        pencil: '<path d="M4 20.2h4.2L19.5 8.8a2.9 2.9 0 0 0-4.1-4.1L4 16z"/><line x1="14.2" y1="6.5" x2="17.6" y2="9.9"/>',
        sliders: '<line x1="4" y1="8.5" x2="20" y2="8.5"/><line x1="4" y1="15.5" x2="20" y2="15.5"/><circle cx="9" cy="8.5" r="2.3"/><circle cx="15" cy="15.5" r="2.3"/>',
        info: '<circle cx="12" cy="12" r="9"/><line x1="12" y1="11" x2="12" y2="16.5"/><line x1="12" y1="7.6" x2="12.01" y2="7.6"/>',
        alert: '<circle cx="12" cy="12" r="9"/><line x1="12" y1="7.6" x2="12" y2="13.2"/><line x1="12" y1="16.4" x2="12.01" y2="16.4"/>',
        play: '<path d="M7.5 4.8l11.5 7.2-11.5 7.2z"/>',
        palette: '<path d="M12 3.2c5 0 8.8 3.4 8.8 7.6 0 2.6-2 3.8-3.6 3.8h-1.6a1.9 1.9 0 0 0-1.4 3.2c.5.6.2 1.6-.7 2a6 6 0 0 1-2.3.4c-4.6 0-8.4-3.8-8.4-8.4S7.4 3.2 12 3.2z"/><circle cx="8.4" cy="10.2" r="1.2"/><circle cx="12" cy="8" r="1.2"/><circle cx="15.6" cy="10.2" r="1.2"/>',
        github: '<path d="M12 2C6.5 2 2 6.6 2 12.2c0 4.5 2.9 8.3 6.8 9.6.5.1.7-.2.7-.5v-1.9c-2.8.6-3.4-1.2-3.4-1.2-.5-1.2-1.1-1.5-1.1-1.5-.9-.6.1-.6.1-.6 1 .1 1.5 1 1.5 1 .9 1.5 2.3 1.1 2.9.8.1-.7.4-1.1.6-1.4-2.2-.3-4.6-1.1-4.6-5 0-1.1.4-2 1-2.7-.1-.3-.4-1.3.1-2.7 0 0 .8-.3 2.7 1a9.3 9.3 0 0 1 5 0c1.9-1.3 2.7-1 2.7-1 .5 1.4.2 2.4.1 2.7.6.7 1 1.6 1 2.7 0 3.9-2.4 4.7-4.6 5 .4.3.7.9.7 1.9v2.8c0 .3.2.6.7.5A10.2 10.2 0 0 0 22 12.2C22 6.6 17.5 2 12 2z"/>'
    };
    const FILLED = new Set(['play', 'more', 'github', 'pin']);

    function hydrateIcons(root) {
        $$('svg[data-icon]', root || document).forEach(function (svg) {
            const name = svg.dataset.icon;
            /* Помним, ЧТО нарисовали, а не «нарисовали ли вообще»: панель
               зовёт hydrateIcons на уже готовых узлах (тосты, шторки), и
               флаг без имени перерисовывал бы значки заново. */
            if (svg.dataset.hydrated === name) return;
            const path = ICONS[name];
            if (!path) return;
            svg.setAttribute('viewBox', '0 0 24 24');
            if (FILLED.has(name)) {
                svg.setAttribute('fill', 'currentColor');
            } else {
                svg.setAttribute('fill', 'none');
                svg.setAttribute('stroke', 'currentColor');
                svg.setAttribute('stroke-width', '1.7');
                svg.setAttribute('stroke-linecap', 'round');
                svg.setAttribute('stroke-linejoin', 'round');
            }
            svg.setAttribute('aria-hidden', 'true');
            svg.setAttribute('focusable', 'false');
            svg.innerHTML = path;
            svg.dataset.hydrated = name;
        });
    }

    /* Короткая анимация значка как ответ на событие (после копирования).
       Класс снимается сам: жест можно повторять сколько угодно раз. */
    function pulseIcon(svg, cls) {
        if (!svg) return;
        const name = cls || 'is-pulse';
        svg.classList.remove(name);
        void svg.getBoundingClientRect();   /* перезапуск анимации */
        svg.classList.add(name);
        svg.addEventListener('animationend', function handler() {
            svg.classList.remove(name);
            svg.removeEventListener('animationend', handler);
        });
    }
    const icon = (name, cls) => {
        const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
        svg.dataset.icon = name;
        if (cls) svg.setAttribute('class', cls);
        return svg;
    };

    /* Банк значений, которые не переживают перезагрузку: сейчас это
       компрессор изображений (создаётся один раз, после ответа
       /api/compression-config). */
    const STATE = { compressor: null };

    /* Форматирование */
    const UNIT_KEYS = ['unit_b', 'unit_kb', 'unit_mb', 'unit_gb'];
    function formatBytes(bytes) {
        if (!bytes) return '—';
        let i = 0;
        let v = bytes;
        while (v >= 1024 && i < UNIT_KEYS.length - 1) { v /= 1024; i++; }
        const digits = v < 10 && i > 0 ? 1 : 0;
        /* Разделитель разрядов — по языку: в русском запятая, в английском
           точка. Число форматируем Intl, единицу берём из словаря. */
        const value = i === 0 ? String(Math.round(v)) : fmt(state.lang).num.format(Number(v.toFixed(digits)));
        return value + ' ' + t(UNIT_KEYS[i]);
    }
    function formatDuration(sec) {
        if (!sec) return '';
        const m = Math.floor(sec / 60);
        const s = sec % 60;
        return m + ':' + String(s).padStart(2, '0');
    }
    /* Интервал ротации в человеческом виде: секунды — до минуты,
       минуты — до часа, дальше часы. Сырые 900 с ничего не говорят. */
    function formatInterval(sec) {
        const s = Math.max(5, Math.round(sec));
        if (s < 60) return s + ' ' + t('unit_sec');
        if (s < 3600) return Math.round(s / 60) + ' ' + t('unit_min');
        const h = s / 3600;
        /* Разделитель дробной части — по языку: с жёсткой запятой в
           английском интерфейсе выходило «1,5 h». */
        const hours = h < 10 ? Number(h.toFixed(1)) : Math.round(h);
        return num(hours) + ' ' + t('unit_hour');
    }
    /* Форматтеры Intl дорого СОЗДАВАТЬ, а не использовать: на 12 карточках
       их набиралось 16 штук на каждую перерисовку (замер: 320 созданий на
       20 перерисовок). Держим по одному на язык — ключ кеша и есть язык,
       поэтому сбрасывать при смене языка ничего не нужно. */
    const FMT = {};
    function fmt(lang) {
        if (!FMT[lang]) {
            const ru = lang === 'ru';
            FMT[lang] = {
                date: new Intl.DateTimeFormat(ru ? 'ru-RU' : 'en-GB', { day: 'numeric', month: 'short' }),
                rtf: new Intl.RelativeTimeFormat(ru ? 'ru' : 'en', { numeric: 'auto' }),
                num: new Intl.NumberFormat(ru ? 'ru-RU' : 'en-US')
            };
        }
        return FMT[lang];
    }
    /* Нулевая метка времени — это «времени нет», а не 1 января 1970 года:
       у ссылки без файла modTime равен нулю, и «1 Jan» в карточке выглядел
       как чужой файл. Прочерк — тот же знак отсутствия, что и в formatBytes. */
    function formatDate(ts) {
        if (!ts) return '—';
        return fmt(state.lang).date.format(new Date(ts * 1000));
    }
    function formatRelative(ts) {
        if (!ts) return '—';
        const diff = Math.floor(Date.now() / 1000 - ts);
        const rtf = fmt(state.lang).rtf;
        if (diff < 90) return rtf.format(-Math.max(1, Math.round(diff / 60)), 'minute');
        if (diff < 3600 * 22) return rtf.format(-Math.round(diff / 3600), 'hour');
        if (diff < 86400 * 6) return rtf.format(-Math.round(diff / 86400), 'day');
        return formatDate(ts);
    }
    const num = (n) => fmt(state.lang).num.format(n);

    /* Узкий экран: на телефоне строка списка слишком коротка для отдельного
       переключателя доступа — там о доступе говорит метка на карточке. */
    function isPhone() { return window.matchMedia('(max-width: 720px)').matches; }

    function sanitizeId(name) {
        return name.replace(/\.[a-z0-9]+$/i, '')
            .toLowerCase()
            .replace(/[^a-z0-9_-]+/g, '-')
            .replace(/^-+|-+$/g, '')
            .slice(0, 64) || 'link';
    }

    /* ========================================================
       2. СОСТОЯНИЕ
       ======================================================== */
    const STORE_KEY = 'lp-ui';
    const LANGS = Array.isArray(window.LANPAPER_LANGS) && window.LANPAPER_LANGS.length
        ? window.LANPAPER_LANGS.slice()
        : ['en'];
    const THEME_MODES = ['light', 'dark', 'auto'];
    /* Ключи состояния и ключи перевода не совпадают по имени ('date_desc' —
       это 'date_new'), поэтому соответствие живёт в одном месте: список
       ключей выводится из него, а не наоборот. Панель сортирует загруженный
       список сама: серверная ручка принимает sort=created|updated (см.
       handlers/admin.go), а панель сортирует по имени, дате и размеру. */
    const SORT_LABELS = {
        name_asc: 'name_asc', name_desc: 'name_desc',
        date_desc: 'date_new', date_asc: 'date_old', size_desc: 'size_desc'
    };
    const SORT_KEYS = Object.keys(SORT_LABELS);
    /* Имена, которые сервер считает занятыми всегда (см. utils/link.go):
       панель обязана проверять то же самое, иначе предложит имя, на
       котором запрос упадёт. */
    const RESERVED_NAMES = ['api', 'admin', 'static', 'external', 'data', 'health',
        'sw.js', 'favicon.ico', 'robots.txt', 'sitemap.xml', 'manifest.json',
        'manifest.webmanifest'];
    /* Правило сервера: латиница или цифра в начале, дальше цифры, латиница,
       дефис и подчёркивание; длина 1–64. Составляется из той же строки, что
       в utils/link.go, чтобы правила не разъехались. */
    const LINK_NAME_RE = /^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$/;
    function validLinkName(name) {
        return LINK_NAME_RE.test(name) && RESERVED_NAMES.indexOf(name.toLowerCase()) < 0;
    }
    const state = createAppState({ config: {
        maxUploadMB: 50, playlistMax: 8, historyLimit: 3,
        historyBudgetBytes: 0, langs: LANGS
    }});

    /* Ключи настроек до 2.0 (theme, viewMode, sortBy, lang) читаются один
       раз как запасной вариант: панель, обновлённая с 0.12.x, не должна
       потерять выбранный вид, тему и язык. */
    function loadPrefs() {
        let saved = {};
        try { saved = JSON.parse(localStorage.getItem(STORE_KEY) || '{}'); } catch (e) { saved = {}; }
        let legacy = {};
        try {
            legacy = {
                theme: localStorage.getItem('theme') || '',
                view: localStorage.getItem('viewMode') || '',
                sort: localStorage.getItem('sortBy') || ''
            };
        } catch (e) { legacy = {}; }
        const theme = saved.theme || (legacy.theme === 'dark' ? 'dark'
            : (legacy.theme === 'light' ? 'light' : 'auto'));
        const sort = saved.sort || (SORT_KEYS.indexOf(legacy.sort) >= 0 ? legacy.sort : 'date_desc');
        state.theme = THEME_MODES.indexOf(theme) >= 0 ? theme : 'auto';
        state.palette = PALETTES.indexOf(saved.palette) >= 0 ? saved.palette : 'mono';
        state.lang = langFromPrefs();
        state.view = saved.view === 'list' || legacy.view === 'list' ? 'list' : 'grid';
        state.sort = sort;
        state.scope = saved.scope === 'all' ? 'all' : 'name';
    }
    function langFromPrefs() {
        let stored = '';
        try { stored = localStorage.getItem('lang') || ''; } catch (e) { stored = ''; }
        const guess = String(stored || (navigator.language || 'en')).slice(0, 2).toLowerCase();
        return LANGS.indexOf(guess) >= 0 ? guess : 'en';
    }
    function savePrefs() {
        try {
            localStorage.setItem(STORE_KEY, JSON.stringify({
                theme: state.theme, palette: state.palette, lang: state.lang,
                view: state.view, sort: state.sort, scope: state.scope
            }));
        } catch (e) { /* приватный режим — настройки просто не запомнятся */ }
    }

    /* ========================================================
       3. ПЕРЕВОДЫ
       ======================================================== */
    function t(key, vars) {
        const dict = state.dict || {};
        /* Английский — запасной словарь: если строки ещё не загрузились
           (или ключ появился позже), панель покажет английский текст,
           а не сырое имя ключа. */
        let str = dict[key] || EN_DICT[key] || key;
        if (vars) {
            for (const k of Object.keys(vars)) {
                /* Замена через функцию: иначе имя ссылки, содержащее $& или
                   $', подставилось бы по правилам String.replace и текст
                   поехал бы. Имена приходят из имён файлов. */
                const value = String(vars[k]);
                str = str.replace(new RegExp('\\{\\{' + k + '\\}\\}', 'g'), () => value);
            }
        }
        return str;
    }
    /* На телефоне полное «Поиск по имени или файлу» не помещается */
    const narrow = () => window.matchMedia('(max-width: 560px)').matches;
    function applyTranslations(root) {
        /* <html lang> в разметке — «en», а панель может говорить по-русски,
           по-немецки и т. д. Без этой строки html остаётся англоязычным:
           экранный диктор читает русский текст с английской фонетикой,
           браузер предлагает перевести страницу, а проверка орфографии
           подчёркивает каждое слово. prepaint.js ставит язык только для
           сохранённого выбора — здесь он ставится всегда. */
        document.documentElement.lang = state.lang;
        $$('[data-i18n]', root || document).forEach(function (el) {
            el.textContent = t(el.dataset.i18n);
        });
        $$('[data-i18n-placeholder]', root || document).forEach(function (el) {
            const key = el.dataset.i18nPlaceholder;
            el.setAttribute('placeholder', narrow() && t(key + '_short') !== key + '_short' ? t(key + '_short') : t(key));
        });
        $$('[data-i18n-aria]', root || document).forEach(function (el) {
            el.setAttribute('aria-label', t(el.dataset.i18nAria));
        });
        $$('[data-tip-key]', root || document).forEach(function (el) {
            el.dataset.tip = t(el.dataset.tipKey);
        });
        /* Образцы акцента собираются в скрипте, поэтому их подписи — и в
           подсказке, и в aria-label — обновляем здесь же, при смене языка. */
        $$('#paletteRow .swatch').forEach(function (label) {
            const name = label.dataset.palette;
            if (!name) return;
            const text = t('palette_' + name);
            label.dataset.tip = text;
            const input = $('input', label);
            if (input) input.setAttribute('aria-label', text);
        });
    }

    /* ========================================================
       4. ТЕМА, АКЦЕНТ, ЯЗЫК
       ======================================================== */
    const prefersDark = () => matchMedia('(prefers-color-scheme: dark)').matches;
    /* Цвет полосы браузера; те же значения стоят в <meta name="theme-color">
       в admin.html. */
    const THEME_COLOR = { light: '#F5F6F9', dark: '#14161B' };
    function effectiveTheme() {
        return state.theme === 'auto' ? (prefersDark() ? 'dark' : 'light') : state.theme;
    }
    function applyTheme() {
        const dark = effectiveTheme() === 'dark';
        document.documentElement.dataset.theme = dark ? 'dark' : 'light';
        document.documentElement.dataset.themeMode = state.theme;
        /* Знак кнопки темы переключает CSS: оба значка лежат друг на друге,
           виден тот, что подходит текущей теме (html[data-theme] ставит
           prepaint.js до первой отрисовки). Явно трогать data-icon здесь
           больше не нужно — раньше это и не работало: hydrateIcons считал
           значок уже нарисованным, и солнце не появлялось вовсе. */
        $$('[data-theme-opt]').forEach(input => { input.checked = input.value === state.theme; });
        /* Полоса браузера (адресная строка, статус-бар на телефоне) должна
           совпадать с выбранной темой, а не только с системной: у <meta
           name="theme-color"> есть лишь media-варианты, и ручной выбор темы
           они не видят — полоса оставалась чужого цвета. Обе метки получают
           цвет действующей темы, какая бы media ни совпала. */
        const ring = dark ? THEME_COLOR.dark : THEME_COLOR.light;
        $$('meta[name="theme-color"]').forEach(function (meta) { meta.setAttribute('content', ring); });
        /* Тёмная тема меняет оттенок образцов акцента: цвет ставим заново,
           иначе на тёмном фоне останутся светлые кружки. */
        paintSwatches();
    }

    /* Цвет образца зависит от темы, поэтому живёт в скрипте (CSSOM), а не в
       атрибуте style="" — тот панель под CSP не применяет. */
    function paintSwatches(root) {
        $$('.swatch__dot[data-palette]', root || document).forEach(function (dot) {
            dot.style.background = swatchColor(dot.dataset.palette);
        });
    }
    function applyPalette() {
        document.documentElement.dataset.palette = state.palette;
        $$('#paletteRow .swatch input').forEach(function (input) {
            input.checked = input.value === state.palette;
        });
        const name = $('#paletteName');
        if (name) name.textContent = t('palette_' + state.palette);
    }
    function applyView() {
        document.documentElement.dataset.view = state.view;
        const grid = $('#grid');
        grid.classList.toggle('grid--list', state.view === 'list');
        /* Подложка переключателя едет к активной кнопке (см. style.css) */
        $('#viewGridBtn').parentElement.dataset.view = state.view;
        $('#viewGridBtn').setAttribute('aria-pressed', String(state.view === 'grid'));
        $('#viewListBtn').setAttribute('aria-pressed', String(state.view === 'list'));
    }
    /* Короткое проявление карточек после смены вида. Класс ставится уже на
       свежие карточки (render перерисовал выдачу), поэтому анимация идёт
       у них, а не у ушедших из DOM. Снимаем класс, чтобы он не мешал
       подъёму карточки при наведении. */
    let viewSwitchTimer = null;
    function playViewSwitch() {
        const grid = $('#grid');
        grid.classList.remove('is-switching');
        void grid.offsetWidth;
        grid.classList.add('is-switching');
        clearTimeout(viewSwitchTimer);
        viewSwitchTimer = setTimeout(function () { grid.classList.remove('is-switching'); }, 700);
    }
    /* Словари кешируются по языку: смена языка не ходит в сеть дважды, а
       английский остаётся запасным для ключей, которых нет в переводе. */
    const DICT_CACHE = {};
    let EN_DICT = {};
    async function loadDict() {
        const load = async function (code) {
            if (DICT_CACHE[code]) return DICT_CACHE[code];
            try {
                const res = await fetch('/static/i18n/' + code + '.json', { credentials: 'same-origin' });
                DICT_CACHE[code] = res.ok ? await res.json() : {};
            } catch (e) {
                DICT_CACHE[code] = {};
            }
            return DICT_CACHE[code];
        };
        const current = await load(state.lang);
        EN_DICT = state.lang === 'en' ? current : await load('en');
        state.dict = current;
    }
    async function setLang(lang) {
        if (LANGS.indexOf(lang) < 0) return;
        state.lang = lang;
        try { localStorage.setItem('lang', lang); } catch (e) { /* приватный режим */ }
        await loadDict();
        document.documentElement.lang = lang;
        applyTranslations();
        applyPalette();
        renderChips();
        render();
        /* Открытое окно — тоже на новом языке: у пульта перерисовываем
           содержимое, у остальных диалогов достаточно переводов разметки. */
        if (currentOverlay) {
            applyTranslations(currentOverlay.node);
            renderPanel();
        }
        const sel = $('#langSelect');
        if (sel) sel.value = lang;
        savePrefs();
    }

    /* ========================================================
       5. ЗАПРОСЫ К СЕРВЕРУ
       ======================================================== */
    /* Один вход для всех ручек: ошибку сервера превращаем в текст на языке
       панели, 401/403 — в объяснение, а не в «Failed to fetch». */
    async function apiCall(url, method, body, isForm, signal, retry) {
        try {
            return await request(url, { method: method || 'GET', body, isForm, signal });
        } catch (error) {
            if (error instanceof ApiError && error.kind === 'authentication') {
                /* Session expired or signed out elsewhere: the login form takes over. */
                window.location.reload();
            } else if (error instanceof ApiError && error.kind === 'network') {
                toast(t('network_error'), {
                    type: 'error',
                    action: typeof retry === 'function' ? t('retry') : '',
                    onAction: typeof retry === 'function' ? retry : null
                });
            } else if (!(error instanceof ApiError && error.kind === 'cancelled')) {
                toast(translateServerError(error), { type: 'error' });
            }
            throw error;
        }
    }

    /* Выход считается завершённым только после того, как сервер надёжно
       сохранил отзыв сессии. При ошибке остаёмся в панели: cookie и сессия ещё
       действуют, поэтому пользователь может исправить storage и повторить. */
    async function signOut() {
        try {
            await apiCall('/api/session', 'DELETE');
        } catch (_) {
            return; /* apiCall уже показал локализованную ошибку */
        }
        window.location.replace('/admin');
    }

    async function signOutAll() {
        const confirmed = await openConfirm({
            title: t('sign_out_all_confirm_title'),
            text: t('sign_out_all_confirm')
        });
        if (!confirmed) return;
        try {
            await apiCall('/api/sessions', 'DELETE');
        } catch (_) {
            return; /* сессии восстановлены сервером; операцию можно повторить */
        }
        window.location.replace('/admin');
    }

    /* Тексты сервера короткие и английские; показываем понятное на языке
       панели, а неизвестное отдаём как есть — лучше точная цитата, чем
       выдуманный перевод. */
    function translateServerError(err) {
        const status = err && err.status;
        if (status === 401) return t('auth_required');
        if (status === 403) return t('forbidden');
        if (status === 413) return t('upload_too_big', { mb: state.config.maxUploadMB });
        const text = String((err && err.message) || '');
        /* Ручки «дай настройки» у сервера нет, зато предел плейлиста назван
           прямо в отказе: запоминаем его и в следующий раз показываем лимит
           до нажатия, а не после. */
        const full = /playlist is full \(max (\d+)/i.exec(text);
        if (full) state.config.playlistMax = Number(full[1]);
        if (/could not sign out all sessions/i.test(text)) return t('sign_out_all_error');
        if (/could not sign out/i.test(text)) return t('sign_out_error');
        if (/link already exists/i.test(text)) return t('link_taken');
        if (/link does not exist|link not found/i.test(text)) return t('link_gone');
        if (/invalid link name|invalid id/i.test(text)) return t('invalid_id');
        if (/file too large/i.test(text)) return t('upload_too_big', { mb: state.config.maxUploadMB });
        if (/no file provided|invalid or unsupported media/i.test(text)) return t('invalid_image');
        if (/playlist is full|playlist full/i.test(text)) return t('playlist_full', { max: state.config.playlistMax });
        if (/no media to add|has no media/i.test(text)) return t('append_needs_media');
        if (/url too long/i.test(text)) return t('url_too_long');
        if (/failed to load media|invalid local media path|download/i.test(text)) return t('url_not_media');
        if (/method not allowed/i.test(text)) return t('action_failed');
        return text ? t('action_failed') + ': ' + text : t('action_failed');
    }

    async function fetchLinks() {
        const pageSize = 200;
        const list = [];
        for (let page = 1; ; page += 1) {
            const res = await apiCall('/api/wallpapers?page=' + page + '&page_size=' + pageSize);
            const data = Array.isArray(res) ? res : (res && Array.isArray(res.data) ? res.data : []);
            list.push(...data);
            if (Array.isArray(res) || !res || page >= res.totalPages || data.length === 0) break;
            // Yield between pages so large libraries do not monopolize the UI.
            await new Promise(resolve => setTimeout(resolve, 0));
        }
        state.links = list.map(normalizeLink);
        return state.links;
    }

    /* Обновление одной ссылки по ответу сервера: подменяем запись, а карточку
       и открытый пульт перерисовываем — данные всегда те, что на диске. */
    function applyLinkUpdate(updated) {
        if (!updated || !updated.linkName) return null;
        const fresh = normalizeLink(updated);
        const idx = state.links.findIndex(l => l.linkName === fresh.linkName);
        if (idx >= 0) state.links[idx] = fresh;
        else state.links.push(fresh);
        render();
        if (currentOverlay && currentOverlay.kind === 'panel'
            && currentOverlay.node.dataset.name === fresh.linkName) {
            const scroller = $('.sheet__body', currentOverlay.node);
            const top = scroller ? scroller.scrollTop : 0;
            renderPanel();
            const after = $('.sheet__body', currentOverlay.node);
            if (after) after.scrollTop = top;
        }
        return fresh;
    }

    const findLink = (name) => state.links.find(l => l.linkName === name);

    /* ========================================================
       6. ВЫБОРКА И СЕТКА
       ======================================================== */
    const selectorHelpers = () => ({
        accessText: level => t(accessMeta(level).key),
        formatBytes
    });
    function matchesQuery(link) { return linkMatchesQuery(state, link, selectorHelpers()); }
    function filterCount(key) { return countFilteredLinks(state, key, selectorHelpers()); }
    function resetShown() { resetIncrementalRender(state); }
    function visibleLinks() { return selectVisibleLinks(state, selectorHelpers()); }

    const ratio = (l) => (l.width && l.height ? l.width / l.height : 0);
    function fitMode(link) {
        if (!link.hasImage) return 'empty';
        const r = ratio(link);
        if (isVideoMedia(link)) return r > 1.4 || !r ? 'cover' : 'contain';
        if (mediaExt(link) === 'png') return 'contain';        /* иконки и логотипы с альфой */
        if (!r) return 'cover';
        if (r < 1.25) return 'contain';                        /* вертикаль и квадрат */
        return 'cover';
    }



    /* Кадр ссылки берём у /api/preview (он отдаётся и для ссылок под
       авторизацией), а версию добавляем в адрес: после замены файла браузер
       не должен показывать прежний кадр из кеша. */
    function previewSrc(link) {
        const base = link.preview || '/api/preview/' + encodeURIComponent(link.linkName);
        const path = base.charAt(0) === '/' ? base : '/' + base;
        const version = link.currentVersion > 1 ? '?v=' + link.currentVersion : '';
        return path + version;
    }
    /* Кадр есть только у картинок. Для видео превью сервер не делает, а
       /api/preview/{имя} отдал бы сам видеофайл: браузер тянул его как
       картинку, ловил ошибку и всё равно показывал «нет кадра» — то есть
       трафик впустую. Поэтому у видео кадра нет сразу. */
    function hasFrame(link) {
        return !!(link.hasImage && !isVideoMedia(link));
    }

    function cardFor(link) {
        const node = $('#tplCard').content.firstElementChild.cloneNode(true);
        node.dataset.name = link.linkName;
        node.dataset.mime = link.mimeType;
        node.classList.toggle('is-pinned', !!link.pinned);
        node.classList.toggle('is-selecting', state.selecting);
        node.classList.toggle('is-selected', state.selected.has(link.linkName));

        const frame = $('.card__frame', node);
        const img = $('.card__media', node);
        const fit = fitMode(link);
        /* Размеры кадра сервер не отдаёт, зато их видно у загруженного
           превью: показываем их, когда картинка дошла, — и не показываем
           выдуманных чисел, если она не загрузилась. */
        const dimsSpan = h('span', { class: 'is-hidden' });
        const dims = link.width && link.height ? link.width + '×' + link.height : null;

        if (!hasFrame(link)) {
            frame.classList.add('card__frame--empty');
            img.remove();
            frame.append(icon('imageOff'));
        } else {
            img.src = previewSrc(link);
            img.alt = '';
            if (fit === 'contain') {
                img.classList.add('card__media--contain');
                if (mediaExt(link) === 'png') frame.classList.add('card__frame--checker');
            }
            /* У загруженного превью видно и пропорции, и настоящий размер
               файла: вертикальное и квадратное показываем целиком, а не
               обрезком, а строку метаданных дополняем размерами. */
            img.addEventListener('load', function () {
                if (!img.naturalWidth) return;
                if (!dims && !isVideoMedia(link)) {
                    dimsSpan.textContent = img.naturalWidth + '×' + img.naturalHeight;
                    dimsSpan.classList.remove('is-hidden');
                }
                if (!isVideoMedia(link) && img.naturalHeight > img.naturalWidth * 1.25) {
                    img.classList.add('card__media--contain');
                    if (mediaExt(link) === 'png') frame.classList.add('card__frame--checker');
                }
            });
            /* Файл не пришёл — показываем тот же знак «нет кадра», что и у
               ссылки без медиа: пустая рамка честнее битой картинки. */
            img.addEventListener('error', function () { showEmptyFrame(frame, img); });
        }

        if (isVideoMedia(link)) {
            const play = $('.card__play', node);
            play.hidden = false;
            if (link.durationSec) {
                const dur = h('span', { class: 'on-media on-media--bottom' },
                    formatDuration(link.durationSec));
                frame.append(dur);
            }
        }

        /* Значки на кадре: только то, что не видно из картинки */
        const left = $('.card__badges-left', node);
        if (link.currentVersion > 1) left.append(h('span', { class: 'on-media num', text: 'v' + link.currentVersion }));
        if (link.items.length) {
            left.append(h('span', { class: 'on-media' }, icon('playlist'),
                h('span', { text: t('meta_items', { n: link.items.length }) })));
        } else if (mediaExt(link) === 'gif') {
            left.append(h('span', { class: 'on-media', text: t('animation') }));
        }

        /* Имя и метаданные */
        const nameBtn = $('.card__name', node);
        nameBtn.textContent = link.linkName;
        nameBtn.setAttribute('aria-label', t('open_panel') + ': ' + link.linkName);
        const check = $('.card__check', node);
        check.setAttribute('aria-pressed', String(state.selected.has(link.linkName)));
        if (state.selecting) {
            check.hidden = false;
            check.removeAttribute('tabindex');
            check.setAttribute('aria-label', t('select_mode') + ': ' + link.linkName);
        }
        $('.card__open', node).setAttribute('aria-label', t('open_media'));

        const meta = $('.card__meta', node);
        const parts = [
            (mediaExt(link) || '—').toUpperCase(),
            dims || dimsSpan,
            isVideoMedia(link) && link.durationSec ? formatDuration(link.durationSec) : null,
            link.hasImage ? formatBytes(link.sizeBytes) : t('no_image'),
            formatRelative(link.modTime)
        ].filter(Boolean);
        parts.forEach(function (part, i) {
            /* Разделители рисует CSS: если часть скрыта на узком экране,
               точка-разделитель исчезает вместе с ней. */
            if (typeof part !== 'string') {
                part.classList.toggle('meta-hide-sm', i === parts.length - 1);
                meta.append(part);
                return;
            }
            meta.append(h('span', { class: i === parts.length - 1 ? 'meta-hide-sm' : '', text: part }));
        });

        /* В широкой строке списка уровень доступа показывает переключатель
           справа — метка в теле была бы вторым ответом на тот же вопрос. */
        const rowSelect = state.view === 'list' && !isPhone();

        /* Теги: доступ (если не публичный) и ротация */
        const tags = $('.card__tags', node);
        if (link.accessLevel !== 'public' && !rowSelect) {
            const acc = accessMeta(link.accessLevel);
            tags.append(h('span', { class: 'badge' }, icon(acc.icon), h('span', { text: t(acc.key) })));
        }
        if (link.rotate && link.rotate.enabled) {
            tags.append(h('span', { class: 'badge badge--accent' }, icon('rotate'),
                h('span', { text: t('meta_rotating') + ' ' + formatInterval(link.rotate.interval) })));
        }
        if (!tags.children.length) tags.remove();

        /* В режиме списка строка широкая, и справа остаётся место — там
           помещается переключатель доступа. В приложении 0.12.1 он был в
           каждой карточке, из-за чего карточка росла в высоту; здесь он
           появляется только там, где место действительно есть. */
        if (rowSelect) {
            const select = h('select', {
                class: 'select select--mini card__access', 'aria-label': t('access_label') + ': ' + link.linkName,
                onchange: function () { setAccessLevel(link, select.value); }
            });
            ['public', 'local', 'token', 'auth'].forEach(function (level) {
                select.append(h('option', {
                    value: level, text: t(accessMeta(level).key), selected: link.accessLevel === level
                }));
            });
            $('.card__body', node).append(select);
        }

        return node;
    }

    /* Кадр, который не загрузился, приводим к виду «нет файла» */
    function showEmptyFrame(frame, img) {
        if (img && img.isConnected) img.remove();
        frame.classList.add('card__frame--empty');
        /* Своя проверка на конкретный значок: в кадре уже есть svg кнопок
           («выбрать», «открыть в новой вкладке», «играть»). */
        if (!$('svg[data-icon="imageOff"]', frame)) {
            frame.append(icon('imageOff'));
            hydrateIcons(frame);
        }
    }

    function render() {
        const grid = $('#grid');
        const list = visibleLinks();
        const total = state.links.length;
        const narrowed = !!state.query.trim() || state.filter !== 'all' || state.access !== 'any';

        applyReveal(list);
        grid.setAttribute('aria-busy', String(state.loading));
        grid.innerHTML = '';

        if (state.loading) {
            for (let i = 0; i < 8; i++) {
                grid.append(h('div', { class: 'skeleton' },
                    h('div', { class: 'skeleton__frame' }),
                    h('div', { class: 'skeleton__line' }),
                    h('div', { class: 'skeleton__line skeleton__line--short' })));
            }
        } else {
            const frag = document.createDocumentFragment();
            list.slice(0, state.shown).forEach(link => frag.append(cardFor(link)));
            grid.append(frag);
        }

        document.body.classList.toggle('is-selecting', state.selecting);
        $('#noResults').classList.toggle('is-hidden', !(!state.loading && !list.length && total > 0 && narrowed));
        $('#emptyState').classList.toggle('is-hidden', !(!state.loading && !state.loadError && total === 0));
        $('#errorState').classList.toggle('is-hidden', !(!state.loading && state.loadError));
        $('#pageTitle').textContent = t('links');
        $('#pageCount').textContent = narrowed
            ? t('found', { shown: list.length, total: total })
            : String(total);
        $('#brandCount').textContent = String(total);
        $('#selectAllBtn').hidden = !state.selecting;
        renderLoadMore(list);
        updateBulkbar();
        hydrateIcons(grid);
        jumpToRevealed();
    }

    /* «Показать ещё»: длинная библиотека дорисовывается порциями. Кнопка
       говорит, сколько именно осталось, вместо безымянного «ещё», а рядом
       видно, сколько из найденного уже на экране. */
    function renderLoadMore(list) {
        const box = $('#loadMore');
        const left = Math.max(0, list.length - state.shown);
        box.classList.toggle('is-hidden', state.loading || left === 0);
        if (!left) return;
        $('#loadMoreBtn').textContent = t('loading_more', { count: Math.min(CHUNK, left) });
        box.querySelector('[data-left]').textContent = t('shown_of', { shown: state.shown, total: list.length });
    }

    /* Только что созданная ссылка должна быть видна: иначе тост сказал
       «создана», а найти её в длинной библиотеке нельзя. Запоминаем имя и
       раскрываем порцию ровно на время одного рендера — не раньше, чтобы
       позиция считалась по актуальному порядку. */
    function revealLinks(names) {
        const keep = state.reveal || [];
        state.reveal = keep.concat(names.filter(n => !keep.includes(n)));
    }
    function applyReveal(list) {
        if (!state.reveal || !state.reveal.length) return;
        let need = state.shown;
        let last = null;
        state.reveal.forEach(function (name) {
            const idx = list.findIndex(l => l.linkName === name);
            if (idx < 0) return;
            last = name;
            if (idx >= need) need = idx + 1;
        });
        if (need > state.shown) state.shown = need;
        /* Прокручиваем один раз — к последней созданной, а не к каждой. */
        if (last && last !== state.scrolledReveal) {
            state.pendingScroll = last;
            state.scrolledReveal = last;
        }
    }

    /* Прокрутка к только что созданной ссылке: подсвечиваем и подводим
       экран — иначе в длинном списке непонятно, где она. */
    function jumpToRevealed() {
        if (!state.pendingScroll) return;
        const name = state.pendingScroll;
        state.pendingScroll = null;
        const card = $('#grid .card[data-name="' + name + '"]');
        if (!card) return;
        card.classList.add('is-new');
        card.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }

    function showMore() {
        const list = visibleLinks();
        const before = state.shown;
        state.shown = Math.min(list.length, state.shown + CHUNK);
        const grid = $('#grid');
        const frag = document.createDocumentFragment();
        list.slice(before, state.shown).forEach(link => frag.append(cardFor(link)));
        grid.append(frag);
        hydrateIcons(grid);
        renderLoadMore(list);
    }

    /* ========================================================
       7. ЧИПЫ ФИЛЬТРОВ И СОРТИРОВКА
       ======================================================== */
    function chip(label, count, opts) {
        const o = opts || {};
        const b = h('button', {
            class: 'chip' + (o.ghost ? ' chip--ghost' : ''),
            type: 'button',
            'aria-pressed': String(!!o.pressed),
            onclick: o.onclick
        }, o.icon ? icon(o.icon) : null, h('span', { text: label }));
        if (count !== undefined) b.append(h('span', { class: 'chip__count', text: String(count) }));
        if (o.key) b.dataset.key = o.key;
        if (o.tip) b.dataset.tip = o.tip;
        if (o.haspopup) b.setAttribute('aria-haspopup', 'menu');
        return b;
    }

    /* Счётчики на чипах зависят от запроса, поэтому обновляются при каждом
       нажатии. Пересобирать всю строку чипов ради шести чисел незачем —
       меняем только текст, DOM остаётся на месте. */
    function updateChipCounts() {
        $$('#filterRow .chip[data-key]').forEach(function (b) {
            const count = b.querySelector('.chip__count');
            if (count) count.textContent = String(filterCount(b.dataset.key));
        });
    }

    function renderChips() {
        const row = $('#filterRow');
        /* Чипы пересобираются целиком, а вместе с ними теряются фокус
           клавиатуры и прокрутка строки: на телефоне нажатие чипа
           возвращало список к началу, а с клавиатуры фокус улетал в body.
           Запоминаем «кто в фокусе» по устойчивой примете и возвращаем. */
        const focus = document.activeElement;
        const focusKey = (focus && row.contains(focus))
            ? (focus.dataset.key || focus.dataset.tip || (focus.getAttribute('aria-haspopup') ? 'popup:' + (focus.textContent || '').trim() : ''))
            : null;
        const scrollLeft = row.scrollLeft;
        row.innerHTML = '';

        /* Где искать. По умолчанию — имя и тип файла, как в приложении;
           «Везде» добавляет уровень доступа, вес, версию и размеры, чтобы
           можно было найти ссылку по примете, а не по имени. */
        row.append(chip(t('search_scope') + ': ' + t(state.scope === 'all' ? 'search_in_all' : 'search_in_name'),
            undefined, {
                ghost: true, icon: 'search', haspopup: true, tip: t('search_scope'),
                pressed: state.scope === 'all',
                onclick: function (e) { openScopeMenu(e.currentTarget); }
            }));
        row.append(h('span', { class: 'chip-row__sep' }));

        [['all', 'filter_all'], ['image', 'filter_photo'], ['video', 'filter_video'],
         ['playlist', 'filter_playlist'], ['pinned', 'filter_pinned']].forEach(function (pair) {
            row.append(chip(t(pair[1]), filterCount(pair[0]), {
                key: pair[0],
                pressed: state.filter === pair[0],
                onclick: function () { state.filter = pair[0]; renderChips(); resetShown(); render(); }
            }));
        });

        row.append(h('span', { class: 'chip-row__sep' }));

        const accMeta = state.access === 'any'
            ? { icon: 'globe', label: t('filter_access') }
            : { icon: accessMeta(state.access).icon, label: t(accessMeta(state.access).key) };
        row.append(chip(accMeta.label, undefined, {
            ghost: true, icon: accMeta.icon, haspopup: true,
            pressed: state.access !== 'any',
            onclick: function (e) { openAccessMenu(e.currentTarget); }
        }));

        row.append(chip(t('sort_by') + ': ' + t(SORT_LABELS[state.sort]), undefined, {
            ghost: true, icon: 'sliders', haspopup: true,
            pressed: state.sort !== 'date_desc',
            onclick: function (e) { openSortMenu(e.currentTarget); }
        }));
        hydrateIcons(row);

        row.scrollLeft = scrollLeft;
        if (focusKey) {
            const again = $$('.chip', row).find(function (b) {
                const key = b.dataset.key || b.dataset.tip ||
                    (b.getAttribute('aria-haspopup') ? 'popup:' + (b.textContent || '').trim() : '');
                return key === focusKey;
            });
            if (again) again.focus({ preventScroll: true });
        }
    }

    /* ========================================================
       8. МЕНЮ
       ======================================================== */
    let openMenuEl = null;
    function closeMenu() {
        if (openMenuEl) { openMenuEl.remove(); openMenuEl = null; }
    }
    function showMenu(anchor, items, opts) {
        closeMenu();
        const menu = h('div', { class: 'menu', role: 'menu' });
        items.forEach(function (item) {
            if (item.sep) { menu.append(h('div', { class: 'menu__sep' })); return; }
            if (item.label) { menu.append(h('div', { class: 'menu__label', text: item.label })); return; }
            menu.append(h('button', {
                class: 'menu__item' + (item.danger ? ' menu__item--danger' : ''),
                type: 'button',
                role: item.checked === undefined ? 'menuitem' : 'menuitemradio',
                'aria-checked': item.checked === undefined ? null : String(!!item.checked),
                onclick: function (ev) {
                    closeMenu();
                    /* Событие идёт дальше: действию нужен значок той кнопки,
                       по которой щёлкнули (короткая анимация ответа). */
                    if (item.onclick) item.onclick(ev);
                }
            }, icon(item.icon), h('span', { class: 'grow', text: item.text }),
               item.hint ? h('span', { class: 'menu__item__hint', text: item.hint }) : null,
               item.checked ? icon('check') : null));
        });
        document.body.append(menu);
        openMenuEl = menu;

        const r = anchor.getBoundingClientRect();
        const mw = menu.offsetWidth;
        const mh = menu.offsetHeight;
        const align = (opts && opts.align) || 'start';
        let left = align === 'end' ? r.right - mw : r.left;
        left = Math.max(10, Math.min(left, window.innerWidth - mw - 10));
        let top = r.bottom + 6;
        if (top + mh > window.innerHeight - 10) top = Math.max(10, r.top - mh - 6);
        menu.style.left = left + 'px';
        menu.style.top = top + 'px';
        hydrateIcons(menu);
        if (opts && opts.focusFirst !== false) $('.menu__item', menu)?.focus();
        return menu;
    }

    /* Ключи состояния и ключи перевода не совпадают по имени — держим
       соответствие в одном месте, чтобы чип не показывал 'date_desc'.
       Таблица описана выше рядом с SORT_KEYS. */

    function openSortMenu(anchor) {
        showMenu(anchor, [
            { label: t('sort_by') },
            { text: t('name_asc'), checked: state.sort === 'name_asc', onclick: () => setSort('name_asc') },
            { text: t('name_desc'), checked: state.sort === 'name_desc', onclick: () => setSort('name_desc') },
            { text: t('date_new'), checked: state.sort === 'date_desc', onclick: () => setSort('date_desc') },
            { text: t('date_old'), checked: state.sort === 'date_asc', onclick: () => setSort('date_asc') },
            { sep: true },
            { text: t('size_desc'), checked: state.sort === 'size_desc', onclick: () => setSort('size_desc') }
        ]);
    }
    function setSort(sort) {
        resetShown();
        state.sort = sort;
        savePrefs();
        renderChips();
        render();
    }
    function openScopeMenu(anchor) {
        showMenu(anchor, [
            {
                icon: 'search', text: t('search_in_name'), checked: state.scope === 'name',
                onclick: function () { state.scope = 'name'; savePrefs(); renderChips(); resetShown(); render(); }
            },
            {
                icon: 'sliders', text: t('search_in_all'), checked: state.scope === 'all',
                onclick: function () { state.scope = 'all'; savePrefs(); renderChips(); resetShown(); render(); }
            }
        ], { align: 'start' });
    }

    function openAccessMenu(anchor) {
        const items = [{ label: t('access_label') }, { text: t('filter_access_any'), checked: state.access === 'any', onclick: () => setAccess('any') }];
        ['public', 'local', 'token', 'auth'].forEach(function (level) {
            items.push({
                icon: accessMeta(level).icon,
                text: t(accessMeta(level).key),
                checked: state.access === level,
                onclick: () => setAccess(level)
            });
        });
        showMenu(anchor, items);
    }
    function setAccess(level) {
        resetShown();
        state.access = level;
        renderChips();
        render();
    }

    /* ========================================================
       9. ОВЕРЛЕИ: панель, диалоги, фокус
       ======================================================== */
    const host = () => $('#overlayHost');
    let currentOverlay = null;
    const overlayController = createOverlayController({
        document,
        visualViewport: window.visualViewport,
        host,
        createScrim: onClick => h('div', { class: 'scrim', onclick: onClick }),
        announce: message => {
            const live = $('#overlayAnnouncements');
            if (live) live.textContent = message;
        },
        onChange: function (active, depth, overlays) {
            currentOverlay = active;
            document.body.style.overflow = depth ? 'hidden' : '';
            const settingsOpen = overlays.some(item => item.node.matches('[data-sheet="settings"]'));
            document.body.classList.toggle('settings-open', settingsOpen);
            setSettingsExpanded(settingsOpen);
            if (!depth) closeMenu();
        }
    });

    function openOverlay(node, opts) {
        applyTranslations(node);
        hydrateIcons(node);
        return overlayController.open(node, opts || {});
    }
    function setSettingsExpanded(open) {
        const gear = $('#settingsBtn');
        if (gear) gear.setAttribute('aria-expanded', open ? 'true' : 'false');
    }
    function closeOverlay(keepFocus) { overlayController.close(keepFocus); }
    document.addEventListener('keydown', function (e) {
        if (e.key === 'Escape' && openMenuEl) { closeMenu(); return; }
        overlayController.keydown(e);
    });
    document.addEventListener('click', function (e) {
        if (openMenuEl && !openMenuEl.contains(e.target) && !e.target.closest('[aria-haspopup]')) closeMenu();
    });
    window.addEventListener('resize', closeMenu);
    window.addEventListener('scroll', closeMenu, true);
    /* От ширины зависит состав строки списка: на телефоне нет переключателя
       доступа, на десктопе есть. Пересекли границу — перерисовываем. */
    let wasPhone = isPhone();
    window.addEventListener('resize', function () {
        const now = isPhone();
        if (now !== wasPhone) { wasPhone = now; render(); }
    });

    /* ========================================================
       10. ТОСТЫ
       ======================================================== */
    /* Больше четырёх сообщений на экране не читаются: пятое вытесняет
       самое старое. */
    const MAX_TOASTS = 4;
    function toast(message, opts) {
        const o = opts || {};
        const box = h('div', { class: 'toast toast--' + (o.type || 'success'), role: 'status' },
            h('span', { class: 'toast__icon' }, icon(o.type === 'error' ? 'alert' : (o.type === 'info' ? 'info' : 'check'))),
            h('span', { class: 'toast__text', text: message }));
        if (o.action) {
            box.append(h('button', {
                class: 'toast__action', type: 'button', text: o.action,
                onclick: function () { dismiss(); if (o.onAction) o.onAction(); }
            }));
        }
        box.append(h('button', {
            class: 'toast__close', type: 'button', 'aria-label': t('close'),
            onclick: () => dismiss()
        }, icon('x')));
        const container = $('#toasts');
        container.append(box);
        hydrateIcons(box);
        /* Старшие тосты уходят сразу: таймер снимаем, иначе он ещё четыре
           секунды держит ссылку на удалённый узел. */
        while (container.children.length >= MAX_TOASTS) {
            const oldest = container.firstElementChild;
            if (oldest.dismissToast) oldest.dismissToast();
            oldest.remove();
        }
        /* duration: 0 — «висит, пока его не снимут»: так показывается
           загрузка, которая может идти минутами. */
        const timer = o.duration === 0 ? null : setTimeout(dismiss, o.duration || 4200);
        box.dismissToast = dismiss;
        function dismiss() {
            if (timer) clearTimeout(timer);
            if (!box.isConnected || box.classList.contains('is-out')) return;
            box.classList.add('is-out');
            setTimeout(() => box.remove(), 200);
        }
        return box;
    }

    /* ========================================================
       11. ДЕЙСТВИЯ СО ССЫЛКОЙ
       ======================================================== */
    const hostName = () => location.hostname && location.protocol !== 'file:' ? location.hostname : 'lanpaper.local';
    const linkUrl = (link, extra) => location.origin.replace(/\/$/, '') + '/' + link.linkName + (extra || '');

    /* Копирование возвращает промис с результатом: массовое копирование
       показывает один общий тост и должно знать, удалось ли скопировать, а
       раньше «Скопировано» всплывало отдельно на каждую ссылку. quiet=true
       глушит этот отдельный тост. */
    function copyText(text, trigger, opts) {
        const quiet = !!(opts && opts.quiet);
        /* Значок кнопки, которая скопировала, отвечает коротким «плюсом»:
           иначе на быстрый клик виден только тост в углу. */
        const done = () => {
            if (!quiet) toast(t('copied'), { type: 'success', duration: 2000 });
            pulseIcon(trigger && $('svg', trigger));
        };
        if (navigator.clipboard && window.isSecureContext) {
            return navigator.clipboard.writeText(text).then(
                function () { done(); return true; },
                function () { if (!quiet) toast(t('copy_error'), { type: 'error' }); return false; }
            );
        }
        const ta = h('textarea', { class: 'clipboard-proxy' });
        ta.value = text;
        document.body.append(ta);
        ta.select();
        let ok = false;
        try { ok = document.execCommand('copy'); } catch (e) { ok = false; }
        ta.remove();
        if (ok) done(); else if (!quiet) toast(t('copy_error'), { type: 'error' });
        return Promise.resolve(ok);
    }
    function copyLink(link, trigger) {
        const extra = link.accessLevel === 'token' && link.accessToken ? '?token=' + link.accessToken : '';
        copyText(linkUrl(link, extra), trigger);
    }
    function openMedia(link) {
        if (!link.hasImage) { toast(t('no_image'), { type: 'info' }); return; }
        window.open(linkUrl(link), '_blank', 'noopener');
    }
    /* Открыть конкретную версию из истории: без ?v=N сервер отдаёт
       текущий файл, и кнопка «открыть» у строки v1 показывала v3. */
    function openMediaVersion(link, version, isCurrent) {
        if (!link.hasImage) { toast(t('no_image'), { type: 'info' }); return; }
        const extra = isCurrent ? '' : '?v=' + encodeURIComponent(version.version);
        window.open(linkUrl(link, extra), '_blank', 'noopener');
    }
    /* Закрепление, уровень доступа и удаление меняют файлы на сервере, а
       показанное берём из ответа: панель никогда не рисует состояние,
       которого нет на диске. */
    async function togglePin(link) {
        if (state.busy) return;
        state.busy = true;
        try {
            const updated = await apiCall('/api/link/' + encodeURIComponent(link.linkName) + '/pin', 'POST');
            const fresh = applyLinkUpdate(updated);
            toast(t((fresh && fresh.pinned ? 'pinned_toast' : 'unpinned_toast')), { type: 'info', duration: 2200 });
        } catch (_) { /* текст ошибки уже показан */ }
        finally { state.busy = false; }
    }
    async function setAccessLevel(link, level) {
        if (state.busy) return;
        if (link.accessLevel === level && !(level === 'token' && !link.accessToken)) return;
        state.busy = true;
        try {
            const updated = await apiCall('/api/link/' + encodeURIComponent(link.linkName),
                'PATCH', { accessLevel: level });
            applyLinkUpdate(updated);
            toast(t('access_updated'));
            /* Пульт перерисовывает applyLinkUpdate, сохраняя прокрутку:
               иначе переключение уровня отбрасывает в начало панели. */
        } catch (_) {
            render();
            renderPanel();
        }
        finally { state.busy = false; }
    }
    function deleteLink(link, skipConfirm) {
        const remove = async function () {
            try {
                await apiCall('/api/link/' + encodeURIComponent(link.linkName), 'DELETE');
            } catch (_) {
                return;   /* текст ошибки уже показан */
            }
            const index = state.links.indexOf(link);
            if (index >= 0) state.links.splice(index, 1);
            state.selected.delete(link.linkName);
            if (currentOverlay && currentOverlay.kind === 'panel'
                && currentOverlay.node.dataset.name === link.linkName) closeOverlay();
            renderChips();
            render();
            /* Возврата нет: сервер удаляет файл вместе с версиями и
               плейлистом, поэтому тост говорит о результате, а не обещает
               отмену, которой не существует. */
            toast(t('deleted', { name: link.linkName }), { type: 'info', duration: 4000 });
        };
        if (skipConfirm) { remove(); return; }
        openConfirm({
            title: t('delete_title'),
            text: t('delete_msg', { name: link.linkName }),
            onConfirm: remove
        });
    }
    function renameLink(link) {
        openCreateDialog({ rename: link });
    }

    /* Меню «⋯» в карточке */
    function openCardMenu(link, anchor) {
        anchor.setAttribute('aria-expanded', 'true');
        const menu = showMenu(anchor, [
            { icon: 'sliders', text: t('open_panel'), onclick: () => openPanel(link) },
            { icon: 'external', text: t('open_media'), onclick: () => openMedia(link) },
            { icon: 'copy', text: t('copy_url'), onclick: (ev) => copyLink(link, ev.currentTarget) },
            { sep: true },
            { icon: 'pencil', text: t('rename'), onclick: () => renameLink(link) },
            {
                icon: 'pin',
                text: t(link.pinned ? 'unpin' : 'pin'),
                checked: !!link.pinned,
                onclick: () => togglePin(link)
            },
            { icon: 'lock', text: t('access_label'), onclick: () => openPanel(link, 'access') },
            { sep: true },
            { icon: 'trash', text: t('delete'), danger: true, onclick: () => deleteLink(link) }
        ], { align: 'end' });
        const clear = function () { anchor.setAttribute('aria-expanded', 'false'); };
        const observer = new MutationObserver(function () {
            if (!menu.isConnected) { clear(); observer.disconnect(); }
        });
        observer.observe(document.body, { childList: true });
    }

    /* ========================================================
       12. РЕЖИМ ВЫБОРА И ПАКЕТНЫЕ ДЕЙСТВИЯ
       ======================================================== */
    function setSelecting(on) {
        state.selecting = on;
        if (!on) state.selected.clear();
        $('#selectBtn').setAttribute('aria-pressed', String(on));
        render();
    }
    function toggleSelected(link) {
        if (state.selected.has(link.linkName)) state.selected.delete(link.linkName);
        else state.selected.add(link.linkName);
        render();
    }
    function selectedLinks() {
        return state.links.filter(l => state.selected.has(l.linkName));
    }
    function updateBulkbar() {
        let bar = $('#bulkbar');
        document.body.classList.toggle('has-bulk', !!(state.selecting && state.selected.size));
        if (!state.selecting || !state.selected.size) {
            if (bar) bar.remove();
            $('#fabBtn').classList.remove('is-hidden');
            return;
        }
        if (!bar) {
            bar = h('div', { class: 'bulkbar', id: 'bulkbar', role: 'toolbar', 'aria-label': t('select_mode') });
            document.body.append(bar);
        }
        bar.innerHTML = '';
        bar.append(h('span', { class: 'bulkbar__count', text: t('selected', { count: state.selected.size }) }));
        bar.append(h('span', { class: 'bulkbar__sep' }));
        const anyUnpinned = selectedLinks().some(l => !l.pinned);
        bar.append(h('button', {
            class: 'btn btn--sm', type: 'button', onclick: async function () {
                /* Пакетной ручки закрепления нет: это N запросов по одному
                   намерению. Массовой смены ДОСТУПА здесь намеренно нет —
                   у ссылок разные уровни, и одним нажатием можно открыть
                   наружу больше, чем хотелось (см. design/06-redesign-v2.md). */
                /* anyUnpinned — это и подпись кнопки, и желаемое состояние:
                   при «Закрепить» нужны незакреплённые, при «Открепить» —
                   закреплённые. Лишний «!» в прежнем условии отбирал ровно
                   наоборот, поэтому закреплялись уже закреплённые, а
                   остальные не менялись; /pin, к тому же, переключает. */
                const targets = selectedLinks().filter(l => Boolean(l.pinned) !== anyUnpinned);
                for (const link of targets) {
                    try {
                        const updated = await apiCall('/api/link/'
                            + encodeURIComponent(link.linkName) + '/pin', 'POST');
                        const idx = state.links.findIndex(l => l.linkName === link.linkName);
                        if (idx >= 0 && updated) state.links[idx] = normalizeLink(updated);
                    } catch (_) { /* по одной ссылке — уже показано */ }
                }
                toast(t('bulk_pinned', { count: targets.length }));
                render();
            }
        }, icon('pin'), h('span', { text: t(anyUnpinned ? 'bulk_pin' : 'bulk_unpin') })));
        bar.append(h('button', {
            class: 'btn btn--sm', type: 'button', onclick: function () {
                const count = state.selected.size;
                const text = selectedLinks()
                    .map(l => linkUrl(l, l.accessLevel === 'token' ? '?token=' + l.accessToken : '')).join('\n');
                copyText(text, null, { quiet: true }).then(function (ok) {
                    toast(ok ? t('bulk_copied', { count: count }) : t('copy_error'),
                        { type: ok ? 'success' : 'error' });
                });
            }
        }, icon('copy'), h('span', { text: t('bulk_copy') })));
        bar.append(h('button', {
            class: 'btn btn--sm btn--danger', type: 'button', onclick: function () {
                const count = state.selected.size;
                openConfirm({
                    title: t('bulk_delete_title', { count: count }),
                    text: t('bulk_delete_msg'),
                    onConfirm: async function () {
                        const names = Array.from(state.selected);
                        /* С экрана уходят только те ссылки, которые сервер
                           действительно удалил: при частичном сбое остальные
                           остаются на месте, а не «исчезают до перезагрузки». */
                        const removed = [];
                        for (const name of names) {
                            try {
                                await apiCall('/api/link/' + encodeURIComponent(name), 'DELETE');
                                removed.push(name);
                            } catch (_) { /* по одной ссылке — уже показано */ }
                        }
                        const done = removed.length;
                        state.links = state.links.filter(l => removed.indexOf(l.linkName) < 0);
                        state.selected.clear();
                        renderChips();
                        render();
                        toast(t('bulk_deleted', { count: done }), { type: 'info' });
                    }
                });
            }
        }, icon('trash'), h('span', { text: t('bulk_delete') })));
        bar.append(h('button', {
            class: 'icon-btn icon-btn--sm', type: 'button', 'aria-label': t('clear_selection'),
            onclick: function () { state.selected.clear(); render(); }
        }, icon('x')));
        hydrateIcons(bar);
        $('#fabBtn').classList.add('is-hidden');
    }

    /* ========================================================
       13. ПАНЕЛЬ ССЫЛКИ («пульт»)
       ======================================================== */
    function openPanel(link, tab) {
        state.panelName = link.linkName;
        state.panelTab = tab || 'media';
        const node = $('#tplPanel').content.firstElementChild.cloneNode(true);
        node.dataset.name = link.linkName;

        $$('[data-tab]', node).forEach(function (btn) {
            btn.addEventListener('click', function () {
                state.panelTab = btn.dataset.tab;
                renderPanel();
            });
        });
        $$('[data-close]', node).forEach(b => b.addEventListener('click', () => closeOverlay()));
        $('[data-delete]', node).addEventListener('click', function () {
            deleteLink(link);
        });

        openOverlay(node, { kind: 'panel', focus: node.querySelector('[data-tab][aria-selected="true"]') });
        renderPanel();
    }

    function renderPanel() {
        /* Только для пульта ссылки: диалог создания и настройки — тоже
           оверлеи, и перерисовка «панелью» закрывала бы их (так смена языка
           закрывала настройки). */
        if (!currentOverlay || currentOverlay.kind !== 'panel') return;
        const node = currentOverlay.node;
        const link = findLink(node.dataset.name);
        if (!link) { closeOverlay(); return; }

        $$('[data-tab]', node).forEach(function (btn) {
            const on = btn.dataset.tab === state.panelTab;
            btn.setAttribute('aria-selected', String(on));
            if (on) $('[data-body]', node).setAttribute('aria-labelledby', btn.id);
        });

        $('#panelTitle', node).textContent = link.linkName;
        const sub = $('[data-sub]', node);
        sub.innerHTML = '';
        if (link.stats && link.stats.hits) {
            sub.append(
                h('span', { text: t('stat_hits') + ': ' + num(link.stats.hits) }),
                h('span', { class: 'sep', text: '·' }),
                h('span', { text: formatBytes(link.stats.bytes) }),
                h('span', { class: 'sep', text: '·' }),
                h('span', { text: formatRelative(link.stats.lastHit || 0) })
            );
        } else {
            sub.append(h('span', { text: t('stat_none') }));
        }

        const body = $('[data-body]', node);
        body.innerHTML = '';
        const builders = { media: panelMedia, versions: panelVersions, playlist: panelPlaylist, access: panelAccess };
        body.append((builders[state.panelTab] || panelMedia)(link));
        hydrateIcons(body);
    }

    function panelMedia(link) {
        const wrap = h('div', { class: 'stack' });

        /* URL — главная ценность ссылки, поэтому она наверху */
        wrap.append(h('div', { class: 'url-line' },
            h('span', { class: 'url-line__host', text: hostName() + '/' }),
            h('span', { class: 'url-line__id grow', text: link.linkName }),
            h('button', {
                class: 'icon-btn icon-btn--sm', type: 'button', 'aria-label': t('copy_url'), 'data-tip': t('copy_url'),
                onclick: (ev) => copyLink(link, ev.currentTarget)
            }, icon('copy'))));

        /* Превью текущего файла */
        const fit = fitMode(link);
        const frame = h('div', { class: 'card__frame panel-preview' });
        if (!hasFrame(link)) {
            frame.classList.add('card__frame--empty');
            frame.append(icon('imageOff'));
        } else {
            const img = h('img', {
                class: 'card__media' + (fit === 'contain' ? ' card__media--contain' : ''),
                src: previewSrc(link), alt: ''
            });
            if (fit === 'contain' && mediaExt(link) === 'png') frame.classList.add('card__frame--checker');
            img.addEventListener('error', function () { showEmptyFrame(frame, img); });
            frame.append(img);
        }
        wrap.append(frame);

        /* Сведения о файле */
        const info = h('div', { class: 'card-block card-block--soft stack stack--tight' });
        const rows = [
            [t('file_type'), (link.mimeType || '—') + (link.width ? ' · ' + link.width + '×' + link.height : '')],
            [t('file_size'), link.hasImage ? formatBytes(link.sizeBytes) : t('no_image')],
            [t('file_changed'), formatRelative(link.modTime)],
            [t('file_version'), 'v' + link.currentVersion + (link.history.length
                ? ' · ' + t('in_archive', { count: link.history.length }) : '')]
        ];
        rows.forEach(function (row) {
            info.append(h('div', { class: 'split' },
                h('span', { class: 'label', text: row[0] }),
                h('span', { class: 'num num--sm', text: row[1] })));
        });
        wrap.append(h('section', { class: 'section' },
            h('div', { class: 'section__head' }, h('span', { class: 'section__title', text: t('change_media') })),
            info));

        /* Способы заменить медиа */
        wrap.append(h('div', { class: 'stack stack--tight' },
            h('button', { class: 'row', type: 'button', onclick: () => pickFileFor(link) },
                h('span', { class: 'row__thumb row__thumb--icon' }, icon('upload')),
                h('span', { class: 'row__body' },
                    h('span', { class: 'row__title', text: t('upload_file') }),
                    h('span', { class: 'row__sub', text: t('dropzone_hint') })),
                h('span', { class: 'row__aside' }, icon('external'))),
            h('button', { class: 'row', type: 'button', onclick: () => openUrlDialog(link) },
                h('span', { class: 'row__thumb row__thumb--icon' }, icon('globe')),
                h('span', { class: 'row__body' },
                    h('span', { class: 'row__title', text: t('upload_url') }),
                    h('span', { class: 'row__sub', text: t('url_hint') })),
                h('span', { class: 'row__aside' }, icon('external'))),
            h('button', { class: 'row', type: 'button', onclick: () => openServerPicker(link) },
                h('span', { class: 'row__thumb row__thumb--icon' }, icon('folder')),
                h('span', { class: 'row__body' },
                    h('span', { class: 'row__title', text: t('upload_server') }),
                    h('span', { class: 'row__sub', text: t('server_hint') })),
                h('span', { class: 'row__aside' }, icon('external'))),
            (function () {
                /* Лимит сервера (PLAYLIST_MAX) виден заранее: в приложении
                   панель узнаёт о нём только из отказа. */
                const max = state.config.playlistMax;
                const full = link.items.length >= max;
                const btn = h('button', {
                    class: 'row', type: 'button',
                    onclick: () => pickFileFor(link, 'append')
                },
                    h('span', { class: 'row__thumb row__thumb--icon' }, icon('playlist')),
                    h('span', { class: 'row__body' },
                        h('span', { class: 'row__title', text: t('upload_append') }),
                        h('span', { class: 'row__sub', text: full
                            ? t('playlist_full', { max: max })
                            : (link.items.length ? t('playlist_count', { count: link.items.length }) : t('playlist_empty')) })),
                    h('span', { class: 'row__aside' }, icon(full ? 'x' : 'external')));
                if (full) btn.disabled = true;
                return btn;
            })()));

        /* Статистика: раздел на месте всегда. Пустой блок честнее
           исчезающего — видно, что счётчики есть, но обращений не было. */
        const hits = link.stats && link.stats.hits ? link.stats.hits : 0;
        const held = link.stats && link.stats.bytes ? link.stats.bytes : 0;
        const last = link.stats && link.stats.lastHit ? formatRelative(link.stats.lastHit) : '';
        wrap.append(h('section', { class: 'section' },
            h('div', { class: 'section__head' },
                h('span', { class: 'section__title', text: t('stats_title') })),
            h('div', { class: 'card-block card-block--soft stack stack--tight' },
                h('div', { class: 'split' },
                    h('span', { class: 'num num--lg', text: num(hits) }),
                    h('span', { class: 'label', text: formatBytes(held) })),
                h('span', { class: 'field-hint', text: hits
                    ? t('stats_last', { when: last })
                    : t('stats_none_hint') })),
            h('p', { class: 'field-hint', text: t('stats_reset_hint') })));
        return wrap;
    }

    /* Лимит версий и их бюджет знает только сервер: /api/link/{name}/history
       отдаёт их вместе с архивом, поэтому строка о бюджете появляется после
       ответа, а не рисуется из выдуманных чисел. */
    function loadHistoryInfo(link) {
        return apiCall('/api/link/' + encodeURIComponent(link.linkName) + '/history')
            .catch(function () { return null; });
    }

    function panelVersions(link) {
        const wrap = h('div', { class: 'stack' });
        const used = (link.history || []).reduce((sum, v) => sum + (v.sizeBytes || 0), 0);
        const hint = h('p', { class: 'field-hint', text: t('versions_hint_short', { size: formatBytes(used) }) });
        wrap.append(hint);

        loadHistoryInfo(link).then(function (data) {
            if (!data) return;
            /* История может быть выключена ручкой HISTORY_LIMIT=0 — тогда это
               не «пустой архив», а другое состояние, и говорит оно другое.
               Общий бюджет версий сервер не публикует, поэтому строка честно
               говорит про лимит и занятое место этой ссылки. */
            hint.textContent = data.limit
                ? t('versions_hint', { limit: data.limit, size: formatBytes(Number(data.bytes || 0)) })
                : t('versions_disabled');
        });

        const list = h('div', { class: 'stack stack--tight' });
        list.append(versionRow(link, { version: link.currentVersion, sizeBytes: link.sizeBytes, mtime: link.modTime, mimeType: link.mimeType }, true));
        (link.history || []).forEach(function (v) {
            list.append(versionRow(link, v, false));
        });
        if (!link.history.length) {
            list.append(h('p', { class: 'field-hint', text: t('versions_empty') }));
        }
        wrap.append(h('section', { class: 'section' },
            h('div', { class: 'section__head' }, h('span', { class: 'section__title', text: t('versions_history') })),
            list));
        return wrap;
    }

    function versionRow(link, version, isCurrent) {
        const row = h('div', { class: 'vrow' + (isCurrent ? ' vrow--current' : '') },
            h('span', { class: 'vrow__tag', text: 'v' + version.version }),
            h('span', { class: 'vrow__body' },
                h('span', { class: 'vrow__title', text: isCurrent ? t('versions_current') : entryTitle(version) }),
                h('span', { class: 'vrow__sub', text: formatDate(version.modTime || 0) + ' · ' + formatBytes(version.sizeBytes) })),
            h('span', { class: 'row__aside' },
                h('button', {
                    class: 'icon-btn icon-btn--sm', type: 'button', 'aria-label': t('versions_open'), 'data-tip': t('versions_open'),
                    onclick: () => openMediaVersion(link, version, isCurrent)
                }, icon('external')),
                isCurrent ? null : h('button', {
                    class: 'icon-btn icon-btn--sm', type: 'button', 'aria-label': t('versions_restore'), 'data-tip': t('versions_restore'),
                    onclick: function () {
                        /* Восстановление обратимо: сервер сам архивирует файл,
                           который был на месте восстанавливаемого. */
                        apiCall('/api/link/' + encodeURIComponent(link.linkName) + '/rollback',
                            'POST', { version: version.version })
                            .then(function (updated) {
                                applyLinkUpdate(updated);
                                toast(t('restored', { version: version.version }));
                            })
                            .catch(function () {});
                    }
                }, icon('rotate')),
                isCurrent ? null : h('button', {
                    class: 'icon-btn icon-btn--sm', type: 'button', 'aria-label': t('versions_delete'), 'data-tip': t('versions_delete'),
                    onclick: function () {
                        apiCall('/api/link/' + encodeURIComponent(link.linkName) + '/history/' + version.version, 'DELETE')
                            .then(function (updated) {
                                applyLinkUpdate(updated);
                                toast(t('version_deleted', { version: version.version }), { type: 'info' });
                            })
                            .catch(function () {});
                    }
                }, icon('trash'))));
        return row;
    }

    function panelPlaylist(link) {
        const wrap = h('div', { class: 'stack' });

        /* Поля ротации у сервера называются enabled / interval / order. */
        const rotate = link.rotate || { enabled: false, interval: 30, order: 'sequential' };
        const controls = h('div', { class: 'card-block stack' });
        const switchEl = h('label', { class: 'switch' },
            h('input', { type: 'checkbox', checked: rotate.enabled, onchange: markDirty }),
            h('span', { class: 'switch__track' }),
            h('span', { class: 'switch__text', text: t('rotate_enabled') }));
        const intervalInput = h('input', {
            class: 'input input--interval', type: 'number', min: '5', max: '86400', step: '5',
            id: 'rotateInterval', value: String(rotate.interval || 30), inputmode: 'numeric', oninput: markDirty
        });
        const orderSelect = h('select', { class: 'select', id: 'rotateOrder', onchange: markDirty },
            h('option', { value: 'sequential', text: t('rotate_sequential') }),
            h('option', { value: 'random', text: t('rotate_random') }));
        orderSelect.value = rotate.order;

        const saveBtn = h('button', {
            class: 'btn btn--primary btn--sm', type: 'button', disabled: true, text: t('rotate_save'),
            onclick: function () {
                const payload = {
                    enabled: switchEl.querySelector('input').checked,
                    interval: Math.min(86400, Math.max(5, Number(intervalInput.value) || 30)),
                    order: orderSelect.value
                };
                saveBtn.disabled = true;
                apiCall('/api/link/' + encodeURIComponent(link.linkName), 'PATCH', { rotate: payload })
                    .then(function (updated) {
                        applyLinkUpdate(updated);
                        toast(t('rotate_saved'));
                    })
                    .catch(function () { saveBtn.disabled = false; });
            }
        });
        function markDirty() {
            saveBtn.disabled = false;
            hint.textContent = t('rotate_unsaved');
        }
        const hint = h('p', { class: 'field-hint' });

        controls.append(switchEl,
            h('div', { class: 'inline inline--wrap' },
                h('label', { class: 'label', for: 'rotateInterval', text: t('rotate_interval') }),
                intervalInput,
                h('label', { class: 'label label--inline', for: 'rotateOrder', text: t('rotate_order') }),
                orderSelect),
            h('div', { class: 'split' }, hint, saveBtn));

        wrap.append(h('section', { class: 'section' },
            h('div', { class: 'section__head' },
                h('span', { class: 'section__title', text: t('rotate_enabled') }),
                h('span', { class: 'label', text: link.items.length ? t('playlist_count', { count: link.items.length }) : '' })),
            controls));

        const list = h('div', { class: 'stack stack--tight' });
        list.append(playlistRow(link, { id: '0', mimeType: link.mimeType, sizeBytes: link.sizeBytes, mtime: link.modTime }, true));
        link.items.forEach(function (item) {
            list.append(playlistRow(link, item, false));
        });
        if (!link.items.length) list.append(h('p', { class: 'field-hint', text: t('playlist_empty') }));
        /* Предел сервера (PLAYLIST_MAX) виден до нажатия: в приложении 0.12.1
           о нём сообщал только отказ на запрос. Когда файлов уже максимум,
           вместо кнопки стоит объяснение. */
        const max = state.config.playlistMax;
        if (max > 0 && link.items.length >= max) {
            list.append(h('p', { class: 'field-hint', text: t('playlist_full', { max: max }) }));
        } else {
            list.append(h('button', { class: 'btn btn--soft btn--block', type: 'button', onclick: () => pickFileFor(link, 'append') },
                icon('plus'), h('span', { text: t('upload_append') })));
        }
        wrap.append(h('section', { class: 'section' },
            h('div', { class: 'section__head' }, h('span', { class: 'section__title', text: t('panel_playlist') })),
            list));
        return wrap;
    }

    function playlistRow(link, item, isLive) {
        return h('div', { class: 'vrow' + (isLive ? ' vrow--current' : '') },
            h('span', { class: 'vrow__tag', text: isLive ? '0' : String(item.id) }),
            h('span', { class: 'vrow__body' },
                h('span', { class: 'vrow__title', text: isLive ? t('versions_current') : ('#' + item.id) }),
                h('span', { class: 'vrow__sub', text: entryMeta(item) + ' · ' + formatDate(item.modTime || item.addedAt || 0) })),
            h('span', { class: 'row__aside' },
                isLive ? null : h('button', {
                    class: 'icon-btn icon-btn--sm', type: 'button', 'aria-label': t('playlist_remove'), 'data-tip': t('playlist_remove'),
                    onclick: function () {
                        apiCall('/api/link/' + encodeURIComponent(link.linkName), 'PATCH', { removeItem: item.id })
                            .then(function (updated) {
                                applyLinkUpdate(updated);
                                toast(t('item_removed'), { type: 'info' });
                            })
                            .catch(function () {});
                    }
                }, icon('trash'))));
    }

    /* Тип и вес файла для строк версий и плейлиста: сервер отдаёт расширение
       (ext), размер и время — этого достаточно, размеров кадра у него нет. */
    const entryMeta = entry => formatEntryMeta(entry, formatBytes);
    const entryTitle = entry => formatEntryTitle(entry, formatBytes);


    function panelAccess(link) {
        const wrap = h('div', { class: 'stack' });
        const levels = [
            ['public', 'globe', 'access_public', 'access_public_hint'],
            ['local', 'lock', 'access_local', 'access_local_hint'],
            ['token', 'key', 'access_token', 'access_token_hint'],
            ['auth', 'user', 'access_auth', 'access_auth_hint']
        ];
        const list = h('div', { class: 'stack stack--tight' });
        levels.forEach(function (level) {
            const isOn = link.accessLevel === level[0];
            const input = h('input', {
                type: 'radio', name: 'access-' + link.linkName, value: level[0],
                checked: isOn,
                onchange: () => setAccessLevel(link, level[0])
            });
            list.append(h('label', { class: 'choice' + (isOn ? ' is-checked' : '') }, input,
                h('span', { class: 'choice__mark' }),
                h('span', { class: 'choice__body' },
                    h('span', { class: 'choice__title', text: t(level[2]) }),
                    h('span', { class: 'choice__hint', text: t(level[3]) })),
                h('span', { class: 'choice__icon' }, icon(level[1]))));
        });
        wrap.append(h('section', { class: 'section' },
            h('div', { class: 'section__head' }, h('span', { class: 'section__title', text: t('access_label') })),
            list));

        if (link.accessLevel === 'token') {
            wrap.append(h('div', { class: 'card-block stack' },
                h('span', { class: 'label', text: t('token_value') }),
                h('div', { class: 'url-line' },
                    h('span', { class: 'mono grow truncate', text: link.accessToken || '—' }),
                    h('button', {
                        class: 'icon-btn icon-btn--sm', type: 'button', 'aria-label': t('token_rotate'), 'data-tip': t('token_rotate'),
                        onclick: function () {
                            apiCall('/api/link/' + encodeURIComponent(link.linkName), 'PATCH', { rotateToken: true })
                                .then(function (updated) {
                                    applyLinkUpdate(updated);
                                    toast(t('token_rotated'));
                                })
                                .catch(function () {});
                        }
                    }, icon('refresh'))),
                h('button', { class: 'btn btn--soft', type: 'button', onclick: (ev) => copyLink(link, ev.currentTarget) },
                    icon('copy'), h('span', { text: t('token_copy') }))));
        }
        return wrap;
    }

    /* ========================================================
       13. ЗАГРУЗКА МЕДИА
       Три пути, как у сервера: файл с устройства (с предварительным сжатием
       в браузере), адрес в сети и файл из галереи сервера. Режим append
       добавляет файл за тот же адрес, не трогая текущий.
       ======================================================== */
    /* Куда положить выбранный файл: null — создать новые ссылки по именам
       файлов, иначе — заменить медиа конкретной ссылки (или добавить). */
    let fileTarget = null;

    function pickFileFor(link, mode) {
        fileTarget = { link: link, mode: mode || 'replace' };
        $('#filePicker').click();
    }

    const uploadFeature = createUploadFeature({
        compressor: () => STATE.compressor,
        t, toast, formatBytes, request: apiCall, applyUpdate: applyLinkUpdate
    });
    const uploadFileTo = (link, file, mode, opts) => uploadFeature.file(link, file, mode, opts);
    const uploadFilesTo = (link, files, mode) => uploadFeature.files(link, files, mode);
    const uploadUrlTo = (link, url, mode) => uploadFeature.url(link, url, mode);
    const uploadServerFileTo = (link, path) => uploadFeature.url(link, path, 'replace');

    /* Ссылки из перетащенных файлов: имя файла становится адресом, поэтому
       негодные имена пропускаем, а не создаём ссылку с ошибкой в ответ. */
    async function createLinksFromFiles(files) {
        const created = [];
        const skipped = [];
        for (const file of files) {
            const name = sanitizeId(file.name);
            if (!validLinkName(name) || findLink(name) || created.indexOf(name) >= 0) {
                skipped.push(file.name);
                continue;
            }
            try {
                const res = await apiCall('/api/link', 'POST', { linkName: name });
                const link = normalizeLink(res || { linkName: name });
                state.links.unshift(link);
                created.push(name);
                await uploadFileTo(link, file, 'replace');
            } catch (_) {
                skipped.push(file.name);
            }
        }
        if (created.length) {
            state.query = '';
            $('#searchInput').value = '';
            revealLinks(created);
            renderChips();
            render();
            toast(t('link_created', { name: created.join(', ') }), { duration: 5000 });
        }
        if (skipped.length) toast(t('files_skipped') + ': ' + skipped.join(', '), { type: 'info', duration: 5000 });
    }

    /* Расширения, которые сервер принимает как медиа: список один в один
       с config.AllowedMediaExts (config/constants.go). avif, m4v и mov
       здесь стояли зря — сервер их не принимает, и подсказка «годится»
       в диалоге загрузки по адресу обещала то, чего не будет. */
    const MEDIA_EXT = /\.(jpe?g|png|gif|webp|bmp|tiff?|mp4|webm)([?#]|$)/i;

    function openUrlDialog(link) {
        const node = $('#tplUrl').content.firstElementChild.cloneNode(true);
        const input = $('#urlInput', node);
        const status = $('#urlStatus', node);
        const submit = $('[data-submit]', node);
        $$('[data-close]', node).forEach(b => b.addEventListener('click', () => closeOverlay()));
        openOverlay(node, { focus: input });
        applyTranslations(node);
        /* Подсказку собираем после перевода: в ней подставляется лимит
           сервера (MAX_UPLOAD_MB), а не только статичный текст. */
        $('.dialog__text', node).textContent = t('url_hint');

        let timer = 0;
        /* Проверка до отправки повторяет серверную: адрес, длина, тип файла.
           Ошибку лучше увидеть здесь, чем после скачивания десятков мегабайт. */
        function setStatus(text, ok) {
            status.textContent = text || '';
            status.classList.toggle('field-hint--error', !!text && !ok);
            status.classList.toggle('field-hint--ok', !!ok);
            input.setAttribute('aria-invalid', String(!!text && !ok));
            submit.disabled = !ok;
        }
        function check() {
            const value = input.value.trim();
            clearTimeout(timer);
            if (!value) { setStatus('', false); return; }
            if (value.length > 2048) { setStatus(t('url_too_long'), false); return; }
            if (!/^https?:\/\/[^\s]+$/i.test(value)) { setStatus(t('url_invalid'), false); return; }
            setStatus(t('url_checking'), false);
            timer = setTimeout(function () {
                setStatus(MEDIA_EXT.test(value) ? t('url_ready') : t('url_not_media'), MEDIA_EXT.test(value));
            }, 420);
        }
        input.addEventListener('input', check);
        input.addEventListener('keydown', function (e) {
            if (e.key === 'Enter' && !submit.disabled) { e.preventDefault(); commit(); }
        });
        function commit() {
            if (submit.disabled) return;
            const value = input.value.trim();
            /* Сервер скачивает файл сам и отвечает готовой записью ссылки:
               размеры, вес и превью приходят в ответе. Пустое состояние
               показываем как «идёт загрузка» — запрос может быть долгим. */
            setStatus(t('uploading'), true);
            /* setStatus включает кнопку для «адрес годен» — на время запроса
               гасим её ещё раз, иначе второй клик отправил бы вторую загрузку. */
            submit.disabled = true;
            uploadUrlTo(link, value, 'replace').then(function (ok) {
                if (ok) { closeOverlay(true); return; }
                submit.disabled = false;
                setStatus(t('action_failed'), false);
            });
        }
        submit.addEventListener('click', commit);
    }

    /* ========================================================
       13-бис. ГАЛЕРЕЯ ФАЙЛОВ НА СЕРВЕРЕ
       ======================================================== */
    function openServerPicker(link) {
        const node = $('#tplServer').content.firstElementChild.cloneNode(true);
        const list = $('#serverList', node);
        const empty = $('#serverEmpty', node);
        const filter = $('#serverFilter', node);
        $$('[data-close]', node).forEach(b => b.addEventListener('click', () => closeOverlay()));
        openOverlay(node);
        applyTranslations(node);

        let files = [];
        const VIDEO_EXT = /(mp4|webm)$/i;
        function draw() {
            const q = filter.value.trim().toLowerCase();
            const found = files.filter(name => name.toLowerCase().indexOf(q) >= 0);
            list.replaceChildren();
            empty.classList.toggle('is-hidden', found.length > 0);
            found.forEach(function (name) {
                const isVideo = VIDEO_EXT.test(name);
                /* Превью галереи отдаёт сервер: полный файл в панель не
                   тянем, а для видео берём только метаданные. */
                const src = '/api/external-image-preview?path=' + encodeURIComponent(name);
                const thumb = isVideo
                    ? h('video', { class: 'picker__thumb', src: src, muted: true, preload: 'metadata', playsinline: true })
                    : h('img', { class: 'picker__thumb', src: src, alt: '', loading: 'lazy' });
                list.append(h('button', {
                    class: 'picker__item', type: 'button',
                    onclick: function () {
                        closeOverlay(true);
                        uploadServerFileTo(link, name);
                    }
                }, thumb,
                    h('span', { class: 'picker__body' },
                        h('span', { class: 'picker__name truncate', text: name }),
                        h('span', { class: 'picker__meta label' },
                            h('span', { text: isVideo ? t('filter_video') : t('filter_photo') }))),
                    h('span', { class: 'picker__take', text: t('upload_file') })));
            });
            hydrateIcons(list);
        }
        filter.addEventListener('input', draw);
        filter.focus();
        /* Список читаем один раз при открытии: папка может быть большой,
           а фильтр работает по уже полученному списку. */
        apiCall('/api/external-images').then(function (res) {
            files = Array.isArray(res) ? res.map(String) : [];
            if (!files.length) {
                empty.textContent = t('server_empty');
                empty.classList.remove('is-hidden');
                return;
            }
            draw();
        }).catch(function () {
            empty.textContent = t('server_error');
            empty.classList.remove('is-hidden');
        });
    }

    /* ========================================================
       14. ДИАЛОГИ: создание/переименование и подтверждение
       ======================================================== */
    function openCreateDialog(opts) {
        const o = opts || {};
        const node = $('#tplCreate').content.firstElementChild.cloneNode(true);
        const input = $('#createInput', node);
        $('#createHost', node).textContent = hostName() + '/';
        const hint = $('#createHint', node);
        const submit = $('[data-submit]', node);

        if (o.rename) {
            $('#createTitle', node).textContent = t('rename');
            $('#createTitle', node).removeAttribute('data-i18n');
            input.value = o.rename.linkName;
        } else {
            hint.textContent = t('create_hint');
        }

        $$('[data-close]', node).forEach(b => b.addEventListener('click', () => closeOverlay()));

        /* Проверки совпадают с utils/link.go: формат, длина, занятые имена
           служебных путей. Иначе панель предлагала бы имя, на котором сервер
           отвечает ошибкой, — а «Создать» выглядело бы рабочим. */
        function nameError(value) {
            if (!value) return t('invalid_id');
            if (!LINK_NAME_RE.test(value)) return t('invalid_id');
            if (RESERVED_NAMES.indexOf(value.toLowerCase()) >= 0) return t('link_reserved');
            const taken = state.links.some(l => l.linkName === value
                && (!o.rename || l.linkName !== o.rename.linkName));
            if (taken) return t('link_taken');
            return '';
        }
        function validate() {
            const value = input.value.trim();
            const err = nameError(value);
            if (err) { setError(err); return null; }
            setError('');
            return value;
        }
        function setError(text, ok) {
            hint.textContent = text || (o.rename ? '' : t('create_hint'));
            hint.classList.toggle('field-hint--error', !!text && !ok);
            hint.classList.toggle('field-hint--ok', !!ok);
            input.setAttribute('aria-invalid', String(!!text && !ok));
            submit.disabled = !!text && !ok;
            if (ok) submit.disabled = false;
        }
        /* Пока печатают — молчим. Через 250 мс после последней буквы
           показываем, свободен ли ID: обычно он свободен, и это заметно
           экономит одно нажатие на создание. */
        let checkTimer = 0;
        input.addEventListener('input', function () {
            clearTimeout(checkTimer);
            const value = input.value.trim();
            if (!value) { setError(''); return; }
            /* Формат проверяется локально и сразу: ждать ответа незачем, а
               кнопка не должна выглядеть готовой к негодному имени. */
            if (nameError(value)) { setError(nameError(value)); return; }
            setError('', false);
            checkTimer = setTimeout(function () {
                if (nameError(value)) setError(nameError(value));
                else if (value !== (o.rename && o.rename.linkName) || !o.rename) setError(t('id_available'), true);
            }, 250);
        });

        async function commit() {
            const name = validate();
            if (!name || state.busy) return;
            state.busy = true;
            submit.disabled = true;
            try {
                if (o.rename) {
                    const oldName = o.rename.linkName;
                    const updated = await apiCall('/api/link/' + encodeURIComponent(oldName),
                        'PATCH', { newLinkName: name });
                    /* Переименование меняет и адрес: карточка, пульт и режим
                       выбора должны переехать на новое имя вместе с ним. */
                    const idx = state.links.findIndex(l => l.linkName === oldName);
                    if (idx >= 0) state.links[idx] = normalizeLink(updated || { linkName: name });
                    if (state.selected.delete(oldName)) state.selected.add(name);
                    if (currentOverlay && currentOverlay.kind === 'panel'
                        && currentOverlay.node.dataset.name === oldName) {
                        currentOverlay.node.dataset.name = name;
                    }
                    closeOverlay();
                    revealLinks([name]);
                    renderChips();
                    render();
                    renderPanel();
                    toast(t('renamed', { name: name }));
                    return;
                }
                const res = await apiCall('/api/link', 'POST', { linkName: name });
                const link = normalizeLink(res || { linkName: name, hasImage: false });
                state.links.unshift(link);
                closeOverlay();
                state.query = '';
                $('#searchInput').value = '';
                revealLinks([name]);
                renderChips();
                render();
                toast(t('link_created', { name: name }), {
                    action: t('upload_file'),
                    duration: 8000,
                    onAction: () => openPanel(link)
                });
            } catch (_) {
                submit.disabled = false;
            } finally {
                state.busy = false;
                /* Подсветка и прокрутка — в jumpToRevealed(): она нужна и
                   после загрузки файлов, и после переименования. */
            }
        }

        submit.addEventListener('click', commit);
        input.addEventListener('keydown', function (e) { if (e.key === 'Enter') commit(); });
        openOverlay(node, { focus: input });
        input.select();
    }

    /* Диалог подтверждения отвечает «да/нет»: обработчик onConfirm остаётся
       для существующих вызовов, а промис нужен там, где ответом управляет
       другой файл (импорт списка). */
    function openConfirm(opts) {
        return new Promise(function (resolve) {
            let answered = false;
            const node = $('#tplConfirm').content.firstElementChild.cloneNode(true);
            $('#confirmTitle', node).textContent = opts.title;
            $('[data-text]', node).textContent = opts.text || '';
            $$('[data-close]', node).forEach(b => b.addEventListener('click', () => closeOverlay()));
            $('[data-confirm]', node).addEventListener('click', function () {
                answered = true;
                closeOverlay(true);
                if (opts.onConfirm) opts.onConfirm();
                resolve(true);
            });
            /* Закрытие любым другим способом (Esc, затемнение, крестик) —
               это отказ; ответ приходит ровно один раз. */
            openOverlay(node, {
                focus: $('[data-confirm]', node),
                stack: true, label: opts.title,
                onDismiss: function () { if (!answered) resolve(false); }
            });
        });
    }

    /* ========================================================
       15. НАСТРОЙКИ
       ======================================================== */
    const PALETTES = ['mono', 'indigo', 'sage', 'clay', 'graphite', 'ocean'];

    async function loadSessionSummary(node) {
        const summary = $('#sessionSummary', node);
        if (!summary) return;
        summary.textContent = t('sessions_loading');
        try {
            const sessions = await apiCall('/api/sessions');
            const list = Array.isArray(sessions) ? sessions : [];
            const current = list.find(function (item) { return item.current; });
            summary.textContent = current
                ? t('sessions_summary', { count: list.length, date: formatDate(current.expires) })
                : t('sessions_summary_basic', { count: list.length });
        } catch (_) {
            summary.textContent = t('sessions_load_error');
        }
    }

    function openSettings() {
        const node = $('#tplSettings').content.firstElementChild.cloneNode(true);
        $$('[data-close]', node).forEach(b => b.addEventListener('click', () => closeOverlay()));
        loadSessionSummary(node);

        /* Тема */
        $$('[data-theme-opt]', node).forEach(function (input) {
            input.checked = input.value === state.theme;
            input.addEventListener('change', function () {
                state.theme = input.value;
                savePrefs();
                applyTheme();
            });
        });

        /* Акцент */
        const row = $('#paletteRow', node);
        PALETTES.forEach(function (name) {
            const label = t('palette_' + name);
            const swatch = h('label', {
                class: 'swatch', 'data-tip': label, 'data-palette': name
            },
                h('input', {
                    type: 'radio', name: 'palette', value: name, 'aria-label': label,
                    checked: name === state.palette,
                    onchange: function () {
                        state.palette = name;
                        savePrefs();
                        applyPalette();
                    }
                }),
                h('span', { class: 'swatch__dot', 'data-palette': name }));
            row.append(swatch);
        });
        applyPalette();
        paintSwatches(node);

        /* Язык */
        const langSelect = $('#langSelect', node);
        LANGS.forEach(function (code) {
            const label = { ru: 'Русский', en: 'English', de: 'Deutsch', fr: 'Français', it: 'Italiano', es: 'Español' }[code] || code;
            langSelect.append(h('option', { value: code, text: label, selected: code === state.lang }));
        });
        langSelect.addEventListener('change', function () {
            setLang(langSelect.value);
        });

        /* Данные и обслуживание: экспорт, импорт и перегенерация превью —
           те же операции, что были в панели 0.12.1, просто в новом месте. */
        const installBtn = $('[data-act="install"]', node);
        if (installBtn) installBtn.hidden = !canInstall();
        $$('[data-act]', node).forEach(function (btn) {
            btn.addEventListener('click', function () {
                const act = btn.dataset.act;
                if (act === 'export') {
                    const feature = getFeature('link-list');
                    if (feature) feature.exportData();
                } else if (act === 'import') {
                    const feature = getFeature('link-list');
                    if (feature) feature.triggerImport();
                } else if (act === 'install') {
                    promptInstall();
                } else if (act === 'regen') {
                    regenPreviews(btn, $('span', btn));
                } else if (act === 'signout') {
                    signOut();
                } else if (act === 'signoutall') {
                    signOutAll();
                }
            });
        });

        /* Горячие клавиши: и однобуквенные, и старые сочетания с Ctrl/⌘ —
           панель не отбирает привычку, а добавляет к ней короткие клавиши. */
        const list = $('#shortcutList', node);
        [['/', 'sc_search'], ['n', 'sc_new'], ['t', 'sc_theme'], ['g', 'sc_view'],
         ['s', 'sc_select'], ['Esc', 'sc_close']].forEach(function (pair) {
            list.append(h('div', { class: 'split' },
                h('span', { class: 'label', text: t(pair[1]) }),
                h('span', { class: 'kbd', text: pair[0] })));
        });
        list.append(h('p', { class: 'field-hint', text: t('sc_extra') }));

        openOverlay(node);
    }

    function swatchColor(name) {
        const map = {
            mono: ['#1B1C1E', '#E9EAEC'],
            indigo: ['#5A66B5', '#9AA4E8'],
            sage: ['#4F7C68', '#93C7B2'],
            clay: ['#A25D44', '#E0A489'],
            graphite: ['#4E5763', '#AAB4C2'],
            ocean: ['#2F7C88', '#7CC6D2']
        };
        const pair = map[name] || map.mono;
        return effectiveTheme() === 'dark' ? pair[1] : pair[0];
    }

    /* ========================================================
       16. ПЕРЕТАСКИВАНИЕ ФАЙЛОВ
       ======================================================== */
    let dragDepth = 0;
    function initDragDrop() {
        document.addEventListener('dragenter', function (e) {
            if (!e.dataTransfer || !Array.from(e.dataTransfer.types || []).includes('Files')) return;
            dragDepth++;
            showDropScrim();
        });
        document.addEventListener('dragover', function (e) { e.preventDefault(); });
        document.addEventListener('dragleave', function () {
            dragDepth = Math.max(0, dragDepth - 1);
            if (!dragDepth) hideDropScrim();
        });
        document.addEventListener('drop', function (e) {
            e.preventDefault();
            dragDepth = 0;
            hideDropScrim();
            const files = Array.from((e.dataTransfer && e.dataTransfer.files) || []);
            if (!files.length) return;

            /* Пульт открыт — файл относится к той ссылке, которую правят.
               Иначе получилась бы новая ссылка, а пользователь этого не просил. */
            const openLink = currentOverlay && currentOverlay.node.dataset.name
                ? findLink(currentOverlay.node.dataset.name) : null;
            if (openLink) {
                uploadFilesTo(openLink, files, 'replace');
                return;
            }
            createLinksFromFiles(files);
        });
    }

    function showDropScrim() {
        if ($('#dropScrim')) return;
        const openLink = currentOverlay && currentOverlay.node.dataset.name
            ? findLink(currentOverlay.node.dataset.name) : null;
        const scrim = h('div', { class: 'drop-scrim', id: 'dropScrim' },
            h('div', { class: 'drop-scrim__inner' },
                icon('upload'),
                h('p', { class: 'drop-scrim__title', text: openLink ? t('drop_replace') : t('drop_files') }),
                h('p', { class: 'drop-scrim__hint', text: openLink ? openLink.linkName : t('drop_hint') })));
        document.body.append(scrim);
        hydrateIcons(scrim);
    }
    function hideDropScrim() {
        const scrim = $('#dropScrim');
        if (scrim) scrim.remove();
    }

    /* ========================================================
       17. ГОРЯЧИЕ КЛАВИШИ
       ======================================================== */
    function initShortcuts() {
        document.addEventListener('keydown', function (e) {
            /* Поверх открытого окна сочетания не работают: «n» открыло бы
               окно создания заново и стёрло уже введённое имя (openOverlay
               закрывает предыдущее окно), «g»/«t» поменяли бы вид и тему за
               спиной у пользователя, а «/» увела бы фокус из диалога. Esc и
               Tab обрабатывает отдельный обработчик оверлея, ему это не
               мешает. */
            if (currentOverlay) return;
            const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement.tagName)
                || document.activeElement.isContentEditable;
            const mod = e.ctrlKey || e.metaKey;
            /* Старые сочетания панели работают как раньше: Ctrl/⌘ + N, F, G, T.
               Однобуквенные добавлены рядом — они быстрее, но не отменяют
               привычку, и на них не срабатывает ввод в поле. */
            if (mod && !e.altKey && !e.shiftKey && !typing) {
                const combo = e.key.toLowerCase();
                if (combo === 'n') { e.preventDefault(); openCreateDialog({}); return; }
                if (combo === 'f') { e.preventDefault(); $('#searchInput').focus(); $('#searchInput').select(); return; }
                if (combo === 'g') { e.preventDefault(); setView(state.view === 'grid' ? 'list' : 'grid'); return; }
                if (combo === 't') { e.preventDefault(); cycleTheme(); return; }
            }
            if (e.key === '/' && !typing) { e.preventDefault(); $('#searchInput').focus(); return; }
            if (typing || mod || e.altKey) return;
            const key = e.key.toLowerCase();
            if (key === 'n') { e.preventDefault(); openCreateDialog({}); }
            else if (key === 't') { e.preventDefault(); cycleTheme(); }
            else if (key === 'g') { e.preventDefault(); setView(state.view === 'grid' ? 'list' : 'grid'); }
            else if (key === 's') { e.preventDefault(); setSelecting(!state.selecting); }
        });
    }
    function cycleTheme() {
        state.theme = effectiveTheme() === 'dark' ? 'light' : 'dark';
        savePrefs();
        applyTheme();
    }
    function setView(view) {
        state.view = view;
        savePrefs();
        applyView();
        /* В режиме списка в строке появляется переключатель доступа, в плитке
           его нет: это часть карточки, поэтому вид перерисовывает выдачу.
           Показанную порцию при этом не сбрасываем. */
        render();
        playViewSwitch();
    }

    /* ========================================================
       18. СОБЫТИЯ КАРТОЧЕК И ИНИЦИАЛИЗАЦИЯ
       ======================================================== */
    function initGridEvents() {
        const grid = $('#grid');
        grid.addEventListener('click', function (e) {
            const card = e.target.closest('.card');
            if (!card) return;
            const link = findLink(card.dataset.name);
            if (!link) return;

            if (e.target.closest('.card__menu-btn')) {
                openCardMenu(link, e.target.closest('.card__menu-btn'));
                return;
            }
            if (e.target.closest('.card__open')) {
                openMedia(link);
                return;
            }
            if (state.selecting) {
                toggleSelected(link);
                return;
            }
            openPanel(link);
        });
    }

    /* Служебные данные: версию показываем из /health (как раньше), настройки
       сжатия — из /api/compression-config. Оба запроса необязательные: если
       они не ответили, панель работает дальше без них. */
    async function loadAppVersion() {
        try {
            const res = await fetch('/health', { credentials: 'same-origin' });
            if (!res.ok) return;
            const data = await res.json();
            if (data.version) $('#appVersion').textContent = 'v' + data.version;
        } catch (_) { /* версия не критична */ }
    }

    async function loadCompressionConfig() {
        try {
            const res = await fetch('/api/compression-config', { credentials: 'same-origin' });
            if (!res.ok) return;
            const cfg = await res.json();
            if (typeof ImageCompressor === 'undefined' || !cfg) return;
            if (!Number.isFinite(cfg.quality) || !Number.isFinite(cfg.scale)) return;
            /* Размеры оставляем серверу: он применяет COMPRESSION_SCALE на
               каждом пути загрузки, и уменьшать дважды нельзя. */
            STATE.compressor = new ImageCompressor({
                quality: cfg.quality / 100,
                preserveOriginal: cfg.quality === 100 && cfg.scale === 100
            });
        } catch (_) { /* без настроек сжатия грузим как есть */ }
    }

    /* Перегенерация превью: та же ручка, что и раньше, — состояние кнопки
       показывает, что запрос идёт. */
    async function regenPreviews(btn, span) {
        const original = span ? span.textContent : '';
        btn.disabled = true;
        if (span) span.textContent = t('regen_running');
        let polling = false;
        const progressTimer = setInterval(async function () {
            if (polling) return;
            polling = true;
            try {
                const status = await request('/api/regenerate-previews');
                if (span && status && status.running) {
                    span.textContent = t('regen_progress', { done: status.completed, total: status.total });
                }
            } catch (_) { /* progress is best-effort; the POST owns errors */ }
            finally { polling = false; }
        }, 500);
        try {
            const result = await apiCall('/api/regenerate-previews', 'POST');
            if (result) {
                toast(t('regen_done', { ok: result.ok, errors: result.errors }), {
                    type: result.errors > 0 ? 'info' : 'success'
                });
                await reloadLinks();
            }
        } catch (_) { /* текст ошибки уже показан */ }
        finally {
            clearInterval(progressTimer);
            btn.disabled = false;
            if (span && original) span.textContent = original;
        }
    }

    /* Установка приложения: браузер сам решает, когда она доступна
       (Chrome/Edge). Кнопка в настройках появляется только тогда. */
    let installEvent = null;
    function canInstall() { return !!installEvent; }
    function initPWA() {
        if ('serviceWorker' in navigator) {
            navigator.serviceWorker.register('/sw.js', { scope: '/' }).catch(() => {});
            /* Старый воркер, зарегистрированный под /static/, снимаем: он
               кешировал то, чего кешировать нельзя. */
            navigator.serviceWorker.getRegistrations().then(function (regs) {
                regs.forEach(function (r) {
                    if (new URL(r.scope).pathname.replace(/\/+$/, '') === '/static') r.unregister();
                });
            }).catch(() => {});
        }
        window.addEventListener('beforeinstallprompt', function (e) {
            e.preventDefault();
            installEvent = e;
            const btn = $('#installBtn');
            if (btn) btn.hidden = false;
        });
        window.addEventListener('appinstalled', function () {
            installEvent = null;
            toast(t('installed'));
            renderPanel();
        });
    }
    async function promptInstall() {
        if (!installEvent) return;
        const event = installEvent;
        installEvent = null;
        event.prompt();
        try { await event.userChoice; } catch (_) {}
        const btn = $('#installBtn');
        if (btn) btn.hidden = true;
    }

    /* Перечитать список с сервера, не теряя состояние вида и выбранное. */
    function updateConnectivity(online) {
        document.documentElement.dataset.online = online ? 'true' : 'false';
        const banner = $('#networkBanner');
        if (banner) banner.classList.toggle('is-hidden', online);
    }

    async function reloadLinks() {
        try {
            await fetchLinks();
            /* Ссылки могли исчезнуть с сервера: выделение по именам, которых
               больше нет, показывало бы «выбрано 2», не рисуя ни одной. */
            const names = new Set(state.links.map(l => l.linkName));
            Array.from(state.selected).forEach(function (name) {
                if (!names.has(name)) state.selected.delete(name);
            });
            state.loading = false;
            state.loadError = false;
            renderChips();
            render();
            return true;
        } catch (e) {
            state.loading = false;
            state.loadError = true;
            render();
            return false;
        }
    }

    async function init() {
        loadPrefs();
        hydrateIcons(document);
        observeConnectivity(window, updateConnectivity);
        await loadDict();
        applyTranslations();
        applyTheme();
        applyPalette();
        applyView();
        renderChips();
        render();
        initGridEvents();
        initDragDrop();
        initShortcuts();
        initPWA();
        loadAppVersion();
        loadCompressionConfig();

        /* Кнопки шапки */
        $('#themeBtn').addEventListener('click', cycleTheme);
        $('#settingsBtn').addEventListener('click', openSettings);
        $('#viewGridBtn').addEventListener('click', () => setView('grid'));
        $('#viewListBtn').addEventListener('click', () => setView('list'));
        $('#selectBtn').addEventListener('click', () => setSelecting(!state.selecting));
        $('#selectAllBtn').addEventListener('click', function () {
            visibleLinks().forEach(l => state.selected.add(l.linkName));
            render();
        });
        $('#newLinkBtn').addEventListener('click', () => openCreateDialog({}));
        $('#fabBtn').addEventListener('click', () => openCreateDialog({}));
        $('#emptyCreateBtn').addEventListener('click', () => openCreateDialog({}));
        /* Пустое состояние: «перетащить файлы» открывает обычный выбор файлов
           — на телефоне перетаскивать нечего, а раньше кнопка только
           показывала подсказку. */
        $('#emptyDropBtn').addEventListener('click', () => $('#filePicker').click());
        $('#filePicker').addEventListener('change', function (e) {
            const files = Array.from(e.target.files || []);
            e.target.value = '';
            if (!files.length) return;
            /* Цель выбирает тот, кто открыл поле: пульт — заменить медиа
               ссылки, пустое состояние — создать ссылки по именам файлов. */
            const target = fileTarget;
            fileTarget = null;
            if (target && target.link) {
                uploadFilesTo(target.link, files, target.mode);
                return;
            }
            createLinksFromFiles(files);
        });
        $('#loadMoreBtn').addEventListener('click', showMore);
        $('#networkRetryBtn').addEventListener('click', reloadLinks);
        $('#retryBtn').addEventListener('click', function () {
            state.loading = true;
            state.loadError = false;
            render();
            reloadLinks();
        });
        $('#resetFiltersBtn').addEventListener('click', function () {
            state.query = ''; state.filter = 'all'; state.access = 'any';
            $('#searchInput').value = '';
            renderChips();
            render();
        });

        /* Поиск */
        /* Поиск: счётчики и состояние «ничего не найдено» отзываются сразу,
           а перерисовка сетки откладывается на 80 мс. На длинной библиотеке
           это экономит три перерисовки из четырёх при обычном наборе и
           незаметно на глаз. */
        const search = $('#searchInput');
        const searchField = search.closest('.field');
        let searchTimer = 0;
        search.addEventListener('input', function () {
            state.query = search.value;
            searchField.classList.toggle('has-value', !!search.value);
            resetShown();
            updateChipCounts();
            updateSearchStateNow();
            clearTimeout(searchTimer);
            searchTimer = setTimeout(render, 80);
        });
        /* Мгновенная часть: счётчики, пустое состояние и подписи —
           без перерисовки карточек. */
        function updateSearchStateNow() {
            const list = visibleLinks();
            const total = state.links.length;
            const narrowed = !!state.query.trim() || state.filter !== 'all' || state.access !== 'any';
            $('#noResults').classList.toggle('is-hidden', !(!state.loading && !list.length && total > 0 && narrowed));
            $('#pageCount').textContent = narrowed ? t('found', { shown: list.length, total }) : String(total);
        }
        $('#searchClear').addEventListener('click', function () {
            search.value = '';
            state.query = '';
            searchField.classList.remove('has-value');
            search.focus();
            resetShown();
            renderChips();
            render();
        });

        matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function () {
            if (state.theme === 'auto') applyTheme();   /* он же перекрасит образцы */
        });

        /* Загрузка списка с состоянием-скелетоном. Если ответа нет, панель
           показывает состояние с кнопкой «Повторить», а не пустую
           библиотеку: иначе кажется, что всё удалилось. */
        reloadLinks();

        /* Ярлык приложения из manifest.json ведёт на /admin?action=create:
           открываем диалог создания и убираем параметр из адреса, иначе он
           открывался бы снова при каждом обновлении страницы. */
        const params = new URLSearchParams(location.search);
        if (params.get('action') === 'create') {
            history.replaceState(null, '', location.pathname);
            openCreateDialog({});
        }
    }

    /* Export/import sees only this capability facade: mutable panel state and
       unrelated UI internals are intentionally not exposed on window. */
    registerPanelFacade({
        snapshot: () => panelSnapshot(state),
        t, request: apiCall, toast, confirm: openConfirm,
        validLinkName, reloadLinks
    });

    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
    else init();
})();
