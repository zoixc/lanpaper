/* SPDX-License-Identifier: MIT */
/* ============================================================
   LANPAPER 2.0 — интерактив макета.

   Это стенд, а не приложение: данные приходят из data.js, а
   «запросы к серверу» заменены обычными изменениями массива.
   Разметка, классы и состояния при этом настоящие — при
   переносе в приложение меняется только источник данных и
   вызовы fetch.
   ============================================================ */
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
            else if (key === 'style') el.setAttribute('style', val);
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
        sun: '<circle cx="12" cy="12" r="4.2"/><line x1="12" y1="2" x2="12" y2="4"/><line x1="12" y1="20" x2="12" y2="22"/><line x1="4.9" y1="4.9" x2="6.3" y2="6.3"/><line x1="17.7" y1="17.7" x2="19.1" y2="19.1"/><line x1="2" y1="12" x2="4" y2="12"/><line x1="20" y1="12" x2="22" y2="12"/><line x1="4.9" y1="19.1" x2="6.3" y2="17.7"/><line x1="17.7" y1="6.3" x2="19.1" y2="4.9"/>',
        moon: '<path d="M20.5 14.3A8.5 8.5 0 0 1 9.7 3.5a8.6 8.6 0 1 0 10.8 10.8z"/>',
        gear: '<circle cx="12" cy="12" r="3.1"/><path d="M12 2.6l1 2.3 2.5-.3 1 2.3 2.3 1-.3 2.5 1.6 1.9-1.6 1.9.3 2.5-2.3 1-1 2.3-2.5-.3-1 2.3-1-2.3-2.5.3-1-2.3-2.3-1 .3-2.5L2.6 12l1.6-1.9-.3-2.5 2.3-1 1-2.3 2.5.3z"/>',
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
        lock: '<rect x="4.5" y="10.5" width="15" height="10" rx="2.6"/><path d="M8 10.5V7.6a4 4 0 0 1 8 0v2.9"/>',
        key: '<circle cx="8" cy="15.5" r="3.6"/><path d="M10.7 12.8L20 3.5"/><path d="M17 4.5l2.5 2.5"/><path d="M14.5 7l2.5 2.5"/>',
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
            if (svg.dataset.hydrated) return;
            const path = ICONS[svg.dataset.icon];
            if (!path) return;
            svg.setAttribute('viewBox', '0 0 24 24');
            if (FILLED.has(svg.dataset.icon)) {
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
            svg.dataset.hydrated = '1';
        });
    }
    const icon = (name, cls) => {
        const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
        svg.dataset.icon = name;
        if (cls) svg.setAttribute('class', cls);
        return svg;
    };

    /* Форматирование */
    const UNITS = ['Б', 'КБ', 'МБ', 'ГБ'];
    function formatBytes(bytes) {
        if (!bytes) return '—';
        let i = 0;
        let v = bytes;
        while (v >= 1024 && i < UNITS.length - 1) { v /= 1024; i++; }
        const digits = v < 10 && i > 0 ? 1 : 0;
        return v.toFixed(digits).replace('.', ',') + ' ' + UNITS[i];
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
        return (h < 10 ? h.toFixed(1).replace('.', ',') : Math.round(h)) + ' ' + t('unit_hour');
    }
    function formatDate(ts) {
        return new Intl.DateTimeFormat(state.lang === 'ru' ? 'ru-RU' : 'en-GB',
            { day: 'numeric', month: 'short' }).format(new Date(ts * 1000));
    }
    function formatRelative(ts) {
        const diff = Math.floor(Date.now() / 1000 - ts);
        const rtf = new Intl.RelativeTimeFormat(state.lang === 'ru' ? 'ru' : 'en', { numeric: 'auto' });
        if (diff < 90) return rtf.format(-Math.max(1, Math.round(diff / 60)), 'minute');
        if (diff < 3600 * 22) return rtf.format(-Math.round(diff / 3600), 'hour');
        if (diff < 86400 * 6) return rtf.format(-Math.round(diff / 86400), 'day');
        return formatDate(ts);
    }
    const num = (n) => new Intl.NumberFormat(state.lang === 'ru' ? 'ru-RU' : 'en-US').format(n);

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
    /* Порция рендера: список из сотен ссылок не рисуем целиком — сначала
       первая порция, остальное по кнопке. Это самая дорогая часть работы
       (кадр + картинка + слушатели на каждую карточку). */
    const CHUNK = 12;

    const state = {
        links: [],
        query: '',
        scope: 'name',          /* name — имя и файл, как в приложении; all — плюс доступ и вес */
        filter: 'all',
        access: 'any',
        sort: 'date_desc',
        view: 'grid',
        theme: 'auto',
        palette: 'indigo',
        lang: 'ru',
        selecting: false,
        selected: new Set(),
        loading: true,
        loadError: false,
        shown: CHUNK,
        reveal: null,          /* ссылки, которые нужно оставить на виду */
        scrolledReveal: null,
        pendingScroll: null,
        lastDeleted: null,
        panelTab: 'media'
    };

    function loadPrefs() {
        let saved = {};
        try { saved = JSON.parse(localStorage.getItem(STORE_KEY) || '{}'); } catch (e) { saved = {}; }
        state.theme = saved.theme || 'auto';
        state.palette = saved.palette || 'indigo';
        state.lang = saved.lang || (document.documentElement.lang || 'ru');
        state.view = saved.view || 'grid';   /* плитка — вид по умолчанию на всех ширинах */
        state.sort = saved.sort || 'date_desc';
        state.scope = saved.scope || 'name';
        /* Переопределения из адреса (витрина и прямые ссылки на состояние) */
        const forced = window.LP_FORCED || {};
        ['theme', 'palette', 'lang', 'view'].forEach(function (key) {
            if (forced[key]) state[key] = forced[key];
        });
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
        const dict = (window.LP_I18N && window.LP_I18N[state.lang]) || window.LP_I18N.ru;
        let str = dict[key] || (window.LP_I18N.ru[key] || key);
        if (vars) {
            for (const k of Object.keys(vars)) str = str.replace(new RegExp('\\{\\{' + k + '\\}\\}', 'g'), vars[k]);
        }
        return str;
    }
    /* На телефоне полное «Поиск по имени или файлу» не помещается */
    const narrow = () => window.matchMedia('(max-width: 560px)').matches;
    function applyTranslations(root) {
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
    function effectiveTheme() {
        return state.theme === 'auto' ? (prefersDark() ? 'dark' : 'light') : state.theme;
    }
    function applyTheme() {
        const dark = effectiveTheme() === 'dark';
        document.documentElement.dataset.theme = dark ? 'dark' : 'light';
        document.documentElement.dataset.themeMode = state.theme;
        const svg = $('#themeBtn svg');
        if (svg) svg.dataset.icon = dark ? 'sun' : 'moon';
        hydrateIcons($('#themeBtn'));
        $$('[data-theme-opt]').forEach(input => { input.checked = input.value === state.theme; });
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
        $('#viewGridBtn').setAttribute('aria-pressed', String(state.view === 'grid'));
        $('#viewListBtn').setAttribute('aria-pressed', String(state.view === 'list'));
    }
    function setLang(lang) {
        state.lang = lang;
        document.documentElement.lang = lang;
        applyTranslations();
        applyPalette();
        renderChips();
        render();
        const sel = $('#langSelect');
        if (sel) sel.value = lang;
    }

    /* ========================================================
       5. ДАННЫЕ (заглушка вместо /api/wallpapers)
       ======================================================== */
    function fetchLinks() {
        return new Promise(function (resolve, reject) {
            /* В витрине скелетон не нужен: кадры должны показать готовый вид.
               fail=1 в адресе показывает состояние «сервер не ответил». */
            setTimeout(function () {
                if (window.LP_FORCED && window.LP_FORCED.fail) { reject(new Error('offline')); return; }
                state.links = window.LP_DATA.map(l => Object.assign({}, l, { history: (l.history || []).slice(), items: (l.items || []).slice() }));
                resolve(state.links);
            }, (window.LP_FORCED && window.LP_FORCED.instant) ? 0 : 420);
        });
    }
    const findLink = (name) => state.links.find(l => l.linkName === name);

    /* ========================================================
       6. ВЫБОРКА И СЕТКА
       ======================================================== */
    const SORTS = {
        date_desc: (a, b) => b.modTime - a.modTime,
        date_asc: (a, b) => a.modTime - b.modTime,
        name_asc: (a, b) => a.linkName.localeCompare(b.linkName, 'ru'),
        name_desc: (a, b) => b.linkName.localeCompare(a.linkName, 'ru'),
        size_desc: (a, b) => (b.sizeBytes + b.items.length * 1e6) - (a.sizeBytes + a.items.length * 1e6)
    };
    const FILTERS = {
        all: () => true,
        image: (l) => l.hasImage && l.category !== 'video',
        video: (l) => l.category === 'video',
        playlist: (l) => l.items.length > 0,
        pinned: (l) => l.pinned
    };

    /* Что попадает в поиск. «Имя и файл» — как в приложении 0.12.1:
       имя ссылки и тип файла. «Везде» добавляет уровень доступа, вес,
       версию и размеры — так ищут, когда помнят не имя, а примету:
       «локальная», «2 МБ», «png». */
    function matchesQuery(link) {
        const q = state.query.trim().toLowerCase();
        if (!q) return true;
        const parts = [link.linkName, link.mimeType, link.category];
        if (state.scope === 'all') {
            parts.push(t(accessMeta(link.accessLevel).key), formatBytes(link.sizeBytes),
                String(link.sizeBytes), 'v' + link.currentVersion, String(link.width || ''),
                String(link.height || ''), (link.items || []).length ? 'плейлист playlist' : '');
        }
        return parts.some(part => String(part || '').toLowerCase().includes(q));
    }
    function filterCount(key) {
        return state.links.filter(l => matchesQuery(l)
            && FILTERS[key](l)
            && (state.access === 'any' || l.accessLevel === state.access)).length;
    }
    /* Смена выборки возвращает список к первой порции: иначе «Показать ещё»
       относилось бы к прошлой выдаче. */
    function resetShown() {
        state.shown = CHUNK;
        state.reveal = null;
        state.scrolledReveal = null;
    }

    function visibleLinks() {
        let list = state.links.filter(matchesQuery).filter(FILTERS[state.filter]);
        if (state.access !== 'any') list = list.filter(l => l.accessLevel === state.access);
        list.sort(SORTS[state.sort] || SORTS.date_desc);
        list.sort((a, b) => Number(!!b.pinned) - Number(!!a.pinned));
        return list;
    }

    const ratio = (l) => (l.width && l.height ? l.width / l.height : 0);
    function fitMode(link) {
        if (!link.hasImage) return 'empty';
        const r = ratio(link);
        if (link.category === 'video') return r > 1.4 || !r ? 'cover' : 'contain';
        if (link.mimeType === 'image/png') return 'contain';   /* иконки и логотипы с альфой */
        if (!r) return 'cover';
        if (r < 1.25) return 'contain';                        /* вертикаль и квадрат */
        return 'cover';
    }

    function accessMeta(level) {
        return {
            public: { icon: 'globe', key: 'access_public' },
            local: { icon: 'lock', key: 'access_local' },
            token: { icon: 'key', key: 'access_token' },
            auth: { icon: 'user', key: 'access_auth' }
        }[level] || { icon: 'globe', key: 'access_public' };
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

        if (fit === 'empty') {
            frame.classList.add('card__frame--empty');
            img.remove();
            frame.append(icon('imageOff'));
        } else {
            img.src = link.preview || link.imageUrl;
            img.alt = '';
            if (fit === 'contain') {
                img.classList.add('card__media--contain');
                if (link.mimeType === 'image/png') frame.classList.add('card__frame--checker');
            }
        }

        if (link.category === 'video') {
            const play = $('.card__play', node);
            play.hidden = false;
            if (link.durationSec) {
                const dur = h('span', { class: 'on-media', style: 'position:absolute;right:8px;bottom:8px' },
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
        } else if (link.category === 'gif') {
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
        const dims = link.width && link.height ? link.width + '×' + link.height : null;
        const parts = [
            (link.mimeType.split('/')[1] || '—').toUpperCase(),
            dims,
            link.category === 'video' && link.durationSec ? formatDuration(link.durationSec) : null,
            link.hasImage ? formatBytes(link.sizeBytes) : t('no_image'),
            formatRelative(link.modTime)
        ].filter(Boolean);
        parts.forEach(function (part, i) {
            /* Разделители рисует CSS: если часть скрыта на узком экране,
               точка-разделитель исчезает вместе с ней. */
            meta.append(h('span', { class: i === parts.length - 1 ? 'meta-hide-sm' : '', text: part }));
        });

        /* Теги: доступ (если не публичный) и ротация */
        const tags = $('.card__tags', node);
        if (link.accessLevel !== 'public') {
            const acc = accessMeta(link.accessLevel);
            tags.append(h('span', { class: 'badge' }, icon(acc.icon), h('span', { text: t(acc.key) })));
        }
        if (link.rotate && link.rotate.enabled) {
            tags.append(h('span', { class: 'badge badge--accent' }, icon('rotate'),
                h('span', { text: t('meta_rotating') + ' ' + formatInterval(link.rotate.intervalSec) })));
        }
        if (!tags.children.length) tags.remove();

        /* В режиме списка строка широкая, и справа остаётся место — там
           помещается переключатель доступа. В приложении 0.12.1 он был в
           каждой карточке, из-за чего карточка росла в высоту; здесь он
           появляется только там, где место действительно есть. */
        if (state.view === 'list') {
            const select = h('select', {
                class: 'select select--mini', 'aria-label': t('access_label') + ': ' + link.linkName,
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
                onclick: function () {
                    closeMenu();
                    if (item.onclick) item.onclick();
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
       соответствие в одном месте, чтобы чип не показывал 'date_desc'. */
    const SORT_LABELS = {
        name_asc: 'name_asc', name_desc: 'name_desc',
        date_desc: 'date_new', date_asc: 'date_old', size_desc: 'size_desc'
    };

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
    let lastFocused = null;

    function openOverlay(node, opts) {
        // Запоминаем, откуда пришли: после закрытия фокус вернётся туда же.
        const origin = document.activeElement;
        closeOverlay(true);
        lastFocused = (origin && origin !== document.body) ? origin : lastFocused;
        const scrim = h('div', { class: 'scrim', onclick: () => { if (!(opts && opts.persistent)) closeOverlay(); } });
        host().append(scrim, node);
        currentOverlay = { node: node, scrim: scrim };
        document.body.style.overflow = 'hidden';
        hydrateIcons(node);
        const target = (opts && opts.focus) || node.querySelector('[autofocus], input, button');
        setTimeout(() => target && target.focus({ preventScroll: true }), 40);
        return node;
    }
    function closeOverlay(keepFocus) {
        if (!currentOverlay) return;
        currentOverlay.node.remove();
        currentOverlay.scrim.remove();
        currentOverlay = null;
        document.body.style.overflow = '';
        closeMenu();
        if (!keepFocus && lastFocused && lastFocused.isConnected) lastFocused.focus({ preventScroll: true });
        lastFocused = null;
    }
    document.addEventListener('keydown', function (e) {
        if (e.key === 'Escape') {
            if (openMenuEl) { closeMenu(); return; }
            if (currentOverlay) { closeOverlay(); }
        }
        if (e.key === 'Tab' && currentOverlay) {
            const focusables = $$('a[href], button:not([disabled]), input, select, textarea, [tabindex]:not([tabindex="-1"])', currentOverlay.node)
                .filter(el => el.offsetParent !== null);
            if (!focusables.length) return;
            const first = focusables[0];
            const last = focusables[focusables.length - 1];
            if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
            else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
        }
    });
    document.addEventListener('click', function (e) {
        if (openMenuEl && !openMenuEl.contains(e.target) && !e.target.closest('[aria-haspopup]')) closeMenu();
    });
    window.addEventListener('resize', closeMenu);
    window.addEventListener('scroll', closeMenu, true);

    /* ========================================================
       10. ТОСТЫ
       ======================================================== */
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
        while (container.children.length > 3) container.firstElementChild.remove();
        const timer = setTimeout(dismiss, o.duration || 4200);
        function dismiss() {
            clearTimeout(timer);
            if (!box.isConnected) return;
            box.classList.add('is-out');
            setTimeout(() => box.remove(), 200);
        }
    }

    /* ========================================================
       11. ДЕЙСТВИЯ СО ССЫЛКОЙ
       ======================================================== */
    const hostName = () => location.hostname && location.protocol !== 'file:' ? location.hostname : 'lanpaper.local';
    const linkUrl = (link, extra) => location.origin.replace(/\/$/, '') + '/' + link.linkName + (extra || '');

    function copyText(text) {
        const done = () => toast(t('copied'), { type: 'success', duration: 2000 });
        if (navigator.clipboard && window.isSecureContext) {
            navigator.clipboard.writeText(text).then(done).catch(() => toast(t('copy_error'), { type: 'error' }));
            return;
        }
        const ta = h('textarea', { style: 'position:fixed;top:-1000px' });
        ta.value = text;
        document.body.append(ta);
        ta.select();
        try { document.execCommand('copy'); done(); } catch (e) { toast(t('copy_error'), { type: 'error' }); }
        ta.remove();
    }
    function copyLink(link) {
        const extra = link.accessLevel === 'token' && link.accessToken ? '?token=' + link.accessToken : '';
        copyText(linkUrl(link, extra));
    }
    function openMedia(link) {
        if (!link.hasImage) { toast(t('no_image'), { type: 'info' }); return; }
        window.open(linkUrl(link), '_blank', 'noopener');
    }
    function togglePin(link) {
        link.pinned = !link.pinned;
        if (link.pinned) link.pinnedAt = Math.floor(Date.now() / 1000);
        toast(t(link.pinned ? 'pinned_toast' : 'unpinned_toast'), { type: 'info', duration: 2200 });
        render();
    }
    function setAccessLevel(link, level) {
        if (link.accessLevel === level && !(level === 'token' && !link.accessToken)) return;
        link.accessLevel = level;
        if (level === 'token' && !link.accessToken) link.accessToken = Math.random().toString(36).slice(2, 10);
        toast(t('access_updated'));
        render();
        /* Панель перерисовываем сами, сохраняя прокрутку и фокус: иначе
           переключение уровня отбрасывает пользователя в начало списка. */
        if (currentOverlay && state.panelName === link.linkName && state.panelTab === 'access') {
            const scroller = $('.sheet__body', currentOverlay.node);
            const top = scroller ? scroller.scrollTop : 0;
            renderPanel();
            const fresh = $('.sheet__body', currentOverlay.node);
            if (fresh) fresh.scrollTop = top;
        }
    }
    function deleteLink(link, skipConfirm) {
        const remove = function () {
            const index = state.links.indexOf(link);
            state.links.splice(index, 1);
            state.selected.delete(link.linkName);
            if (state.panelName === link.linkName) closeOverlay();
            state.lastDeleted = { link: link, index: index };
            render();
            toast(t('deleted', { name: link.linkName }), {
                type: 'info',
                action: t('undo'),
                duration: 6000,
                onAction: function () {
                    state.links.splice(state.lastDeleted.index, 0, state.lastDeleted.link);
                    toast(t('link_restored'));
                    render();
                }
            });
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
            { icon: 'copy', text: t('copy_url'), onclick: () => copyLink(link) },
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
        menu.addEventListener('mouseleave', () => {});
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
            class: 'btn btn--sm', type: 'button', onclick: function () {
                selectedLinks().forEach(l => { l.pinned = anyUnpinned; });
                toast(t('bulk_pinned', { count: state.selected.size }));
                render();
            }
        }, icon('pin'), h('span', { text: t(anyUnpinned ? 'bulk_pin' : 'bulk_unpin') })));
        bar.append(h('button', {
            class: 'btn btn--sm', type: 'button', onclick: function () {
                copyText(selectedLinks().map(l => linkUrl(l, l.accessLevel === 'token' ? '?token=' + l.accessToken : '')).join('\n'));
                toast(t('bulk_copied', { count: state.selected.size }));
            }
        }, icon('copy'), h('span', { text: t('bulk_copy') })));
        bar.append(h('button', {
            class: 'btn btn--sm btn--danger', type: 'button', onclick: function () {
                const count = state.selected.size;
                openConfirm({
                    title: t('bulk_delete_title', { count: count }),
                    text: t('bulk_delete_msg'),
                    onConfirm: function () {
                        state.links = state.links.filter(l => !state.selected.has(l.linkName));
                        state.selected.clear();
                        toast(t('bulk_deleted', { count: count }), { type: 'info' });
                        render();
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

        openOverlay(node, { focus: node.querySelector('[data-tab][aria-selected="true"]') });
        renderPanel();
    }

    function renderPanel() {
        if (!currentOverlay) return;
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
                h('span', { text: formatRelative(link.stats.last) })
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
                onclick: () => copyLink(link)
            }, icon('copy'))));

        /* Превью текущего файла */
        const fit = fitMode(link);
        const frame = h('div', { class: 'card__frame', style: 'border-radius:var(--r-md);border:1px solid var(--border);aspect-ratio:16/9' });
        if (fit === 'empty') {
            frame.classList.add('card__frame--empty');
            frame.append(icon('imageOff'));
        } else {
            const img = h('img', {
                class: 'card__media' + (fit === 'contain' ? ' card__media--contain' : ''),
                src: link.preview || link.imageUrl, alt: ''
            });
            if (fit === 'contain' && link.mimeType === 'image/png') frame.classList.add('card__frame--checker');
            frame.append(img);
        }
        wrap.append(frame);

        /* Сведения о файле */
        const info = h('div', { class: 'card-block card-block--soft stack stack--tight' });
        const rows = [
            ['Тип', (link.mimeType || '—') + (link.width ? ' · ' + link.width + '×' + link.height : '')],
            ['Размер', link.hasImage ? formatBytes(link.sizeBytes) : t('no_image')],
            ['Изменён', formatRelative(link.modTime)],
            ['Версия', 'v' + link.currentVersion + (link.history.length ? ' · ' + link.history.length + ' в архиве' : '')]
        ];
        rows.forEach(function (row) {
            info.append(h('div', { class: 'split' },
                h('span', { class: 'label', text: row[0] }),
                h('span', { class: 'num', style: 'font-size:var(--t-sm)', text: row[1] })));
        });
        wrap.append(h('section', { class: 'section' },
            h('div', { class: 'section__head' }, h('span', { class: 'section__title', text: t('change_media') })),
            info));

        /* Способы заменить медиа */
        wrap.append(h('div', { class: 'stack stack--tight' },
            h('button', { class: 'row', type: 'button', onclick: () => uploadDemo(link) },
                h('span', { class: 'row__thumb', style: 'display:grid;place-items:center' }, icon('upload')),
                h('span', { class: 'row__body' },
                    h('span', { class: 'row__title', text: t('upload_file') }),
                    h('span', { class: 'row__sub', text: t('dropzone_hint') })),
                h('span', { class: 'row__aside' }, icon('external'))),
            h('button', { class: 'row', type: 'button', onclick: () => openUrlDialog(link) },
                h('span', { class: 'row__thumb', style: 'display:grid;place-items:center' }, icon('globe')),
                h('span', { class: 'row__body' },
                    h('span', { class: 'row__title', text: t('upload_url') }),
                    h('span', { class: 'row__sub', text: t('url_hint', { mb: window.LP_CONFIG.maxUploadMB }) })),
                h('span', { class: 'row__aside' }, icon('external'))),
            h('button', { class: 'row', type: 'button', onclick: () => openServerPicker(link) },
                h('span', { class: 'row__thumb', style: 'display:grid;place-items:center' }, icon('folder')),
                h('span', { class: 'row__body' },
                    h('span', { class: 'row__title', text: t('upload_server') }),
                    h('span', { class: 'row__sub', text: t('server_hint') })),
                h('span', { class: 'row__aside' }, icon('external'))),
            (function () {
                /* Лимит сервера (PLAYLIST_MAX) виден заранее: в приложении
                   панель узнаёт о нём только из отказа. */
                const max = window.LP_CONFIG.playlistMax;
                const full = link.items.length >= max;
                const btn = h('button', {
                    class: 'row', type: 'button',
                    onclick: () => appendDemo(link)
                },
                    h('span', { class: 'row__thumb', style: 'display:grid;place-items:center' }, icon('playlist')),
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
        const last = link.stats && link.stats.last ? formatRelative(link.stats.last) : '';
        wrap.append(h('section', { class: 'section' },
            h('div', { class: 'section__head' },
                h('span', { class: 'section__title', text: t('stats_title') })),
            h('div', { class: 'card-block card-block--soft stack stack--tight' },
                h('div', { class: 'split' },
                    h('span', { class: 'num', style: 'font-size:var(--t-lg);font-weight:600', text: num(hits) }),
                    h('span', { class: 'label', text: formatBytes(held) })),
                h('span', { class: 'field-hint', text: hits
                    ? t('stats_last', { when: last })
                    : t('stats_none_hint') })),
            h('p', { class: 'field-hint', text: t('stats_reset_hint') })));
        return wrap;
    }

    function panelVersions(link) {
        const wrap = h('div', { class: 'stack' });
        const config = window.LP_CONFIG;
        const used = (link.history || []).reduce((sum, v) => sum + (v.sizeBytes || 0), 0);
        const off = config.historyLimit === 0;
        /* История может быть выключена ручкой HISTORY_LIMIT=0 — тогда это
           не «пустой архив», а другое состояние, и говорит оно другое. */
        wrap.append(h('p', {
            class: 'field-hint',
            text: off ? t('versions_disabled')
                : t('versions_hint', { limit: config.historyLimit, size: formatBytes(used) })
        }));
        if (!off) {
            const pct = Math.min(100, Math.round(used / config.historyBudgetBytes * 100));
            wrap.append(h('div', { class: 'meter' },
                h('div', { class: 'meter__fill', style: 'width:' + Math.max(2, pct) + '%' })),
                h('p', { class: 'field-hint', text: t('versions_budget', {
                    size: formatBytes(used), total: formatBytes(config.historyBudgetBytes)
                }) }));
        }

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
                h('span', { class: 'vrow__title', text: isCurrent ? t('versions_current') : formatBytes(version.sizeBytes) }),
                h('span', { class: 'vrow__sub', text: formatDate(version.mtime) + ' · ' + formatBytes(version.sizeBytes) })),
            h('span', { class: 'row__aside' },
                h('button', {
                    class: 'icon-btn icon-btn--sm', type: 'button', 'aria-label': t('versions_open'), 'data-tip': t('versions_open'),
                    onclick: () => openMedia(link)
                }, icon('external')),
                isCurrent ? null : h('button', {
                    class: 'icon-btn icon-btn--sm', type: 'button', 'aria-label': t('versions_restore'), 'data-tip': t('versions_restore'),
                    onclick: function () {
                        const moved = version;
                        link.history = link.history.filter(v => v.version !== version.version);
                        link.history.unshift({ version: link.currentVersion, sizeBytes: link.sizeBytes, mtime: link.modTime, mimeType: link.mimeType });
                        link.currentVersion = moved.version;
                        link.sizeBytes = moved.sizeBytes;
                        link.modTime = Math.floor(Date.now() / 1000);
                        toast(t('restored', { version: moved.version }));
                        render();
                        renderPanel();
                    }
                }, icon('rotate')),
                isCurrent ? null : h('button', {
                    class: 'icon-btn icon-btn--sm', type: 'button', 'aria-label': t('versions_delete'), 'data-tip': t('versions_delete'),
                    onclick: function () {
                        link.history = link.history.filter(v => v.version !== version.version);
                        toast(t('version_deleted', { version: version.version }), { type: 'info' });
                        renderPanel();
                    }
                }, icon('trash'))));
        return row;
    }

    function panelPlaylist(link) {
        const wrap = h('div', { class: 'stack' });

        const rotate = link.rotate || { enabled: false, intervalSec: 30, order: 'sequential' };
        const controls = h('div', { class: 'card-block stack' });
        const switchEl = h('label', { class: 'switch' },
            h('input', { type: 'checkbox', checked: rotate.enabled, onchange: markDirty }),
            h('span', { class: 'switch__track' }),
            h('span', { class: 'switch__text', text: t('rotate_enabled') }));
        const intervalInput = h('input', {
            class: 'input', type: 'number', min: '5', max: '86400', step: '5', id: 'rotateInterval',
            value: String(rotate.intervalSec), style: 'width:92px', inputmode: 'numeric', oninput: markDirty
        });
        const orderSelect = h('select', { class: 'select', id: 'rotateOrder', onchange: markDirty },
            h('option', { value: 'sequential', text: t('rotate_sequential') }),
            h('option', { value: 'random', text: t('rotate_random') }));
        orderSelect.value = rotate.order;

        const saveBtn = h('button', {
            class: 'btn btn--primary btn--sm', type: 'button', disabled: true, text: t('rotate_save'),
            onclick: function () {
                link.rotate = {
                    enabled: switchEl.querySelector('input').checked,
                    intervalSec: Math.min(86400, Math.max(5, Number(intervalInput.value) || 30)),
                    order: orderSelect.value
                };
                saveBtn.disabled = true;
                toast(t('rotate_saved'));
                render();
                renderPanel();
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
                h('label', { class: 'label', style: 'margin-left:6px', for: 'rotateOrder', text: t('rotate_order') }),
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
        const max = window.LP_CONFIG.playlistMax;
        if (link.items.length >= max) {
            list.append(h('p', { class: 'field-hint', text: t('playlist_full', { max: max }) }));
        } else {
            list.append(h('button', { class: 'btn btn--soft btn--block', type: 'button', onclick: () => appendDemo(link) },
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
                h('span', { class: 'vrow__sub', text: (item.mimeType || '').split('/')[1]?.toUpperCase() + ' · ' + formatBytes(item.sizeBytes) + ' · ' + formatDate(item.mtime) })),
            h('span', { class: 'row__aside' },
                isLive ? null : h('button', {
                    class: 'icon-btn icon-btn--sm', type: 'button', 'aria-label': t('playlist_remove'), 'data-tip': t('playlist_remove'),
                    onclick: function () {
                        link.items = link.items.filter(i => i.id !== item.id);
                        toast(t('item_removed'), { type: 'info' });
                        render();
                        renderPanel();
                    }
                }, icon('trash'))));
    }

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
                            link.accessToken = Math.random().toString(36).slice(2, 10);
                            toast(t('token_rotated'));
                            renderPanel();
                        }
                    }, icon('refresh'))),
                h('button', { class: 'btn btn--soft', type: 'button', onclick: () => copyLink(link) },
                    icon('copy'), h('span', { text: t('token_copy') }))));
        }
        return wrap;
    }

    /* Заглушки «серверных» операций */
    function uploadDemo(link) {
        toast(t('uploading'), { type: 'info', duration: 1200 });
        setTimeout(function () {
            applyMedia(link, {
                mimeType: link.mimeType, sizeBytes: Math.round(link.sizeBytes * (0.9 + Math.random() * 0.3)),
                width: link.width, height: link.height, durationSec: link.durationSec,
                src: link.imageUrl
            }, true);
        }, 900);
    }

    /* Одна точка, где медиа ссылки заменяется: архив версии, новая версия,
       обновление карточки и пульта. В приложении то же самое делает сервер
       (он сохраняет предыдущий файл в историю), а панель только принимает
       ответ — здесь мы повторяем его поведение ровно один раз, чтобы все
       три способа замены (файл, URL, галерея) не расходились. */
    function applyMedia(link, media, keepToast) {
        /* Архивируем только то, что было: у ссылки без файла архивировать
           нечего, и первая версия остаётся первой — как в приложении, где
           история появляется только после замены. */
        if (link.hasImage) {
            link.history.unshift({
                version: link.currentVersion, sizeBytes: link.sizeBytes,
                mtime: link.modTime, mimeType: link.mimeType
            });
            const limit = window.LP_CONFIG.historyLimit;
            if (limit >= 0) link.history = link.history.slice(0, limit);
            link.currentVersion += 1;
        }
        link.hasImage = true;
        link.mimeType = media.mimeType || link.mimeType;
        link.category = (media.mimeType || '').startsWith('video') ? 'video'
            : (/gif/.test(media.mimeType || '') ? 'gif' : 'image');
        link.sizeBytes = media.sizeBytes || link.sizeBytes;
        link.width = media.width || link.width;
        link.height = media.height || link.height;
        link.durationSec = media.durationSec || 0;
        link.imageUrl = media.src || link.imageUrl;
        link.preview = media.src || link.preview;
        link.modTime = Math.floor(Date.now() / 1000);
        if (!keepToast) toast(t('uploaded') + (media.name ? ': ' + media.name : ''));
        render();
        renderPanel();
    }

    function appendDemo(link) {
        const max = window.LP_CONFIG.playlistMax;
        if (!link.hasImage) { toast(t('append_needs_media'), { type: 'error' }); return; }
        if (link.items.length >= max) { toast(t('playlist_full', { max: max }), { type: 'error' }); return; }
        link.items.push({
            id: String(link.items.length + 1),
            mimeType: 'image/jpeg',
            sizeBytes: 120_000 + Math.round(Math.random() * 200_000),
            mtime: Math.floor(Date.now() / 1000)
        });
        toast(t('append_success'));
        render();
        renderPanel();
    }

    /* ========================================================
       13-бис. ЗАГРУЗКА ПО URL И ВЫБОР ФАЙЛА С СЕРВЕРА
       Оба пути есть в приложении: сервер скачивает URL сам
       (и падает с ошибкой, если это не медиа), а «галерея» —
       это файл из настроенной папки на сервере.
       ======================================================== */
    const MEDIA_EXT = /\.(jpe?g|png|gif|webp|bmp|tiff?|avif|mp4|webm|m4v|mov)([?#]|$)/i;
    /* Тип файла из адреса: сервер сохраняет расширение, поэтому и в макете
       PNG остаётся PNG, а не превращается в JPEG «по умолчанию». */
    const EXT_MIME = {
        jpg: 'image/jpeg', jpeg: 'image/jpeg', png: 'image/png', gif: 'image/gif',
        webp: 'image/webp', bmp: 'image/bmp', tif: 'image/tiff', tiff: 'image/tiff',
        avif: 'image/avif', mp4: 'video/mp4', webm: 'video/webm',
        m4v: 'video/x-m4v', mov: 'video/quicktime'
    };

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
        $('.dialog__text', node).textContent = t('url_hint', { mb: window.LP_CONFIG.maxUploadMB });

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
            const value = input.value.trim();
            const ext = (value.split(/[?#]/)[0].match(/\.([a-z0-9]+)$/i) || [null, ''])[1].toLowerCase();
            const mime = EXT_MIME[ext] || 'image/jpeg';
            const isVideo = mime.indexOf('video/') === 0;
            closeOverlay(true);
            toast(t('uploading'), { type: 'info', duration: 1000 });
            /* Сервер скачивает файл и делает превью. В макете кадр берётся из
               демонстрационного набора: размеры и вес приходят от сервера. */
            setTimeout(function () {
                applyMedia(link, isVideo
                    ? { mimeType: mime, sizeBytes: 6_400_000, width: 1280, height: 720, durationSec: 12, src: MEDIA_DEMO.video }
                    : { mimeType: mime, sizeBytes: 480_000, width: 1600, height: 900, src: MEDIA_DEMO.image });
            }, 700);
        }
        submit.addEventListener('click', commit);
    }

    /* Демонстрационные кадры для «скачанного» медиа — те же файлы, что в
       демо-библиотеке, чтобы макет не ходил в сеть. */
    const MEDIA_DEMO = {
        image: '../prototypes/media/wp-abstract.jpg',
        video: '../prototypes/media/wp-mono.jpg'
    };

    function openServerPicker(link) {
        const node = $('#tplServer').content.firstElementChild.cloneNode(true);
        const list = $('#serverList', node);
        const empty = $('#serverEmpty', node);
        const filter = $('#serverFilter', node);
        $$('[data-close]', node).forEach(b => b.addEventListener('click', () => closeOverlay()));
        openOverlay(node);
        applyTranslations(node);

        function draw() {
            const q = filter.value.trim().toLowerCase();
            const files = (window.LP_CONFIG.serverFiles || []).filter(f => f.name.toLowerCase().includes(q));
            list.replaceChildren();
            empty.classList.toggle('is-hidden', files.length > 0);
            files.forEach(function (file) {
                const isVideo = (file.mimeType || '').startsWith('video');
                const thumb = file.img
                    ? h('img', { class: 'picker__thumb', src: file.img, alt: '', loading: 'lazy' })
                    : h('span', { class: 'picker__thumb picker__thumb--icon' }, icon('image'));
                list.append(h('button', {
                    class: 'picker__item', type: 'button',
                    onclick: function () {
                        closeOverlay(true);
                        applyMedia(link, {
                            mimeType: file.mimeType, sizeBytes: file.sizeBytes, width: file.width,
                            height: file.height, durationSec: file.durationSec || 0,
                            src: file.img, name: file.name
                        });
                    }
                }, thumb,
                    h('span', { class: 'picker__body' },
                        h('span', { class: 'picker__name truncate', text: file.name }),
                        h('span', { class: 'picker__meta label' },
                            h('span', { text: (file.width || 0) + '×' + (file.height || 0) }),
                            h('span', { text: formatBytes(file.sizeBytes) }),
                            isVideo ? h('span', { class: 'badge', text: 'MP4' }) : null)),
                    h('span', { class: 'picker__take', text: t('upload_file') })));
            });
            hydrateIcons(list);
        }
        filter.addEventListener('input', draw);
        draw();
        filter.focus();
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

        function validate() {
            const value = input.value.trim();
            if (!value) { setError(t('invalid_id')); return null; }
            if (!/^[a-zA-Z0-9_-]{1,64}$/.test(value)) { setError(t('invalid_id')); return null; }
            const taken = state.links.some(l => l.linkName === value && (!o.rename || l !== o.rename));
            if (taken) { setError(t('link_taken')); return null; }
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
            /* Формат проверяется локально и сразу: ждать «сервер» незачем,
               а кнопка не должна выглядеть готовой к негодному имени. */
            if (!/^[a-zA-Z0-9_-]{1,64}$/.test(value)) { setError(t('invalid_id')); return; }
            setError('', false);
            checkTimer = setTimeout(function () {
                const busy = state.links.some(l => l.linkName === value && (!o.rename || l !== o.rename));
                if (busy) setError(t('link_taken'));
                else if (value !== (o.rename && o.rename.linkName) || !o.rename) setError(t('id_available'), true);
            }, 250);
        });

        function commit() {
            const name = validate();
            if (!name) return;
            if (o.rename) {
                const oldName = o.rename.linkName;
                o.rename.linkName = name;
                if (state.panelName === oldName) state.panelName = name;
                toast(t('renamed', { name: name }));
                closeOverlay();
                revealLinks([name]);
                render();
                return;
            }
            const link = {
                id: 'n' + Date.now(), linkName: name, category: 'empty', hasImage: false,
                imageUrl: '', preview: '', mimeType: '', width: 0, height: 0, sizeBytes: 0,
                created: Math.floor(Date.now() / 1000), modTime: Math.floor(Date.now() / 1000),
                pinned: false, accessLevel: 'public', accessToken: '', currentVersion: 1,
                history: [], items: [], rotate: null, stats: null
            };
            state.links.unshift(link);
            closeOverlay();
            state.query = '';
            $('#searchInput').value = '';
            revealLinks([name]);
            render();
            toast(t('link_created', { name: name }), {
                action: t('upload_file'),
                duration: 8000,
                onAction: () => openPanel(link)
            });
            /* Подсветка и прокрутка — в jumpToRevealed(): она нужна и после
               загрузки файлов, и после переименования, а не только здесь. */
        }

        submit.addEventListener('click', commit);
        input.addEventListener('keydown', function (e) { if (e.key === 'Enter') commit(); });
        openOverlay(node, { focus: input });
        input.select();
    }

    function openConfirm(opts) {
        const node = $('#tplConfirm').content.firstElementChild.cloneNode(true);
        $('#confirmTitle', node).textContent = opts.title;
        $('[data-text]', node).textContent = opts.text || '';
        $$('[data-close]', node).forEach(b => b.addEventListener('click', () => closeOverlay()));
        $('[data-confirm]', node).addEventListener('click', function () {
            closeOverlay(true);
            lastFocused = null;
            if (opts.onConfirm) opts.onConfirm();
        });
        openOverlay(node, { focus: $('[data-confirm]', node) });
    }

    /* ========================================================
       15. НАСТРОЙКИ
       ======================================================== */
    const PALETTES = ['indigo', 'sage', 'clay', 'graphite', 'ocean'];

    function openSettings() {
        const node = $('#tplSettings').content.firstElementChild.cloneNode(true);
        $$('[data-close]', node).forEach(b => b.addEventListener('click', () => closeOverlay()));

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
                h('span', { class: 'swatch__dot', style: 'background:' + swatchColor(name) }));
            row.append(swatch);
        });
        applyPalette();

        /* Язык */
        const langSelect = $('#langSelect', node);
        (window.LP_CONFIG.langs || ['ru', 'en']).forEach(function (code) {
            const label = { ru: 'Русский', en: 'English', de: 'Deutsch', fr: 'Français', it: 'Italiano', es: 'Español' }[code] || code;
            langSelect.append(h('option', { value: code, text: label, selected: code === state.lang }));
        });
        langSelect.addEventListener('change', function () {
            const code = langSelect.value;
            const label = (langSelect.options[langSelect.selectedIndex] || {}).textContent || code;
            setLang(code);
            savePrefs();
            /* Шесть языков, как в приложении, но переведены в макете два.
               Выбор «Deutsch» не должен выглядеть поломкой — говорим прямо,
               что строки придут при переносе из static/i18n. */
            if (!window.LP_I18N[code]) {
                toast(t('lang_mock_only', { lang: label }), { type: 'info', duration: 5000 });
            }
        });

        /* Данные и обслуживание */
        $$('[data-act]', node).forEach(function (btn) {
            btn.addEventListener('click', function () {
                const act = btn.dataset.act;
                if (act === 'export') toast(t('export_json') + ' — OK');
                else if (act === 'import') toast(t('import_json') + ' — ' + t('loading'), { type: 'info' });
                else if (act === 'install') toast(t('install_app'), { type: 'info' });
                else if (act === 'regen') {
                    toast(t('regen_running'), { type: 'info', duration: 1600 });
                    setTimeout(() => toast(t('regen_done', { ok: 13, errors: 0 })), 1700);
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
            indigo: ['#5A66B5', '#9AA4E8'],
            sage: ['#4F7C68', '#93C7B2'],
            clay: ['#A25D44', '#E0A489'],
            graphite: ['#4E5763', '#AAB4C2'],
            ocean: ['#2F7C88', '#7CC6D2']
        };
        const pair = map[name] || map.indigo;
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
                uploadDemo(openLink);
                return;
            }
            createLinksFromFiles(files);
        });
    }
    /* Имена файлов становятся адресами ссылок. Занятые имена пропускаем,
       повторы внутри одной пачки — тоже: две ссылки с одним адресом создать
       нельзя, а молча получить одну из двух — неприятно. */
    function createLinksFromFiles(files) {
        const created = [];
        const skipped = [];
        files.forEach(function (file, i) {
            const name = sanitizeId(file.name);
            if (state.links.some(l => l.linkName === name) || created.includes(name)) { skipped.push(name); return; }
            created.push(name);
            state.links.unshift({
                id: 'u' + Date.now() + i, linkName: name,
                category: file.type.startsWith('video') ? 'video' : 'image',
                hasImage: true, imageUrl: '', preview: '', mimeType: file.type || 'image/jpeg',
                width: 0, height: 0, sizeBytes: file.size,
                created: Math.floor(Date.now() / 1000), modTime: Math.floor(Date.now() / 1000),
                pinned: false, accessLevel: 'public', accessToken: '', currentVersion: 1,
                history: [], items: [], rotate: null, stats: null
            });
        });
        if (!created.length) return;
        revealLinks(created);
        render();
        toast(t('link_created', { name: created.join(', ') }), { duration: 5000 });
        if (skipped.length) toast(t('link_taken') + ': ' + skipped.join(', '), { type: 'info', duration: 4500 });
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

    function init() {
        loadPrefs();
        hydrateIcons(document);
        applyTranslations();
        applyTheme();
        applyPalette();
        applyView();
        renderChips();
        render();
        initGridEvents();
        initDragDrop();
        initShortcuts();

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
            createLinksFromFiles(Array.from(e.target.files || []));
            e.target.value = '';
        });
        $('#loadMoreBtn').addEventListener('click', showMore);
        $('#retryBtn').addEventListener('click', function () {
            state.loading = true;
            state.loadError = false;
            render();
            fetchLinks().then(function () {
                state.loading = false;
                render();
                renderChips();
            }).catch(function () {
                state.loading = false;
                state.loadError = true;
                render();
            });
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
            if (state.theme === 'auto') { applyTheme(); refreshSwatches(); }
        });
        function refreshSwatches() {
            $$('#paletteRow .swatch__dot').forEach(function (dot, i) {
                dot.style.background = swatchColor(PALETTES[i]);
            });
        }

        /* Загрузка «с сервера» с состоянием-скелетоном */
        fetchLinks().then(function () {
            state.loading = false;
            render();
            renderChips();
        }).catch(function () {
            /* Список не пришёл: показываем состояние с кнопкой «Повторить»,
               а не пустую библиотеку — иначе кажется, что всё удалилось. */
            state.loading = false;
            state.loadError = true;
            render();
        });
    }

    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
    else init();
})();
