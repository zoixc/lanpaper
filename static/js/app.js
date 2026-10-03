// SPDX-License-Identifier: MIT
/**
 * Lanpaper Frontend Logic
 * Handles UI interactions, API calls, and state management.
 */


// STATE & CONFIG
const SUPPORTED_LANGS = new Set(['en', 'ru', 'de', 'fr', 'it', 'es']);
const STATE = {
    translations: {},
    lang: localStorage.getItem('lang') || navigator.language.slice(0, 2) || 'en',
    isDark: false,
    // Tiles are the default view: the card is designed as a tile.
    viewMode: localStorage.getItem('viewMode') || 'grid',
    searchQuery: '',
    sortBy: 'date_desc',
    wallpapers: [],
    filteredWallpapers: [],
    compressor: null,
    lazyObserver: null,
    compressionConfig: null,
    isDebug: false,
    createPending: false,
};


// DOM ELEMENTS
const DOM = {
    themeBtn: document.getElementById('themeToggle'),
    viewBtn: document.getElementById('viewToggle'),
    linksList: document.getElementById('linksList'),
    emptyState: document.getElementById('emptyState'),
    toastContainer: document.getElementById('toastContainer'),
    searchInput: document.getElementById('searchInput'),
    searchStats: document.getElementById('searchStats'),
    sortSelect: document.getElementById('sortSelect'),
    appVersion: document.getElementById('appVersion'),

    modalOverlay: document.getElementById('modalOverlay'),
    modalTitle: document.getElementById('modalTitle'),
    modalInput: document.getElementById('modalInput'),
    modalList: document.getElementById('modalList'),
    modalCancel: document.getElementById('modalCancelBtn'),
    modalConfirm: document.getElementById('modalConfirmBtn'),

    confirmOverlay: document.getElementById('confirmOverlay'),
    confirmTitle: document.getElementById('confirmTitle'),
    confirmMessage: document.getElementById('confirmMessage'),
    confirmCancel: document.getElementById('confirmCancelBtn'),
    confirmDelete: document.getElementById('confirmDeleteBtn'),

    versionsOverlay: document.getElementById('versionsOverlay'),
    versionsLinkName: document.getElementById('versionsLinkName'),
    versionsHint: document.getElementById('versionsHint'),
    versionsList: document.getElementById('versionsList'),
    playlistList: document.getElementById('playlistList'),
    rotateEnabled: document.getElementById('rotateEnabled'),
    rotateInterval: document.getElementById('rotateInterval'),
    rotateOrder: document.getElementById('rotateOrder'),
    rotateSave: document.getElementById('rotateSaveBtn'),
    statsSection: document.getElementById('statsSection'),
    statsBody: document.getElementById('statsBody'),
    versionsClose: document.getElementById('versionsCloseBtn'),

    createInput: document.getElementById('newLinkId'),
    createForm: document.getElementById('createForm'),

    template: document.getElementById('linkCardTemplate'),
    dropOverlay: document.getElementById('dropOverlay'),
};

const log = (...args) => STATE.isDebug && console.log(...args);

// --- Compatibility shims ----------------------------------------------------
// Small fallbacks for older browsers (Smart-TV WebViews, older Safari) and
// plain-HTTP LAN deployments. A missing API must never break the whole UI.
if (typeof window.CSS !== 'object') window.CSS = {};
if (typeof window.CSS.escape !== 'function') {
    window.CSS.escape = (s) => String(s).replace(/[^a-zA-Z0-9_-]/g, (c) => '\\' + c);
}

// navigator.clipboard only exists in secure contexts (HTTPS or localhost).
// On plain-HTTP LAN deployments (the typical Lanpaper setup) fall back to the
// legacy execCommand path so "Copy URL" keeps working. The fallback also
// covers writeText being refused (permission policy, unfocused document).
function copyToClipboard(text) {
    if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
        return navigator.clipboard.writeText(text).catch(() => legacyCopy(text));
    }
    return legacyCopy(text);
}

function legacyCopy(text) {
    return new Promise((resolve, reject) => {
        const ta = document.createElement('textarea');
        ta.value = text;
        ta.setAttribute('readonly', '');
        ta.style.position = 'fixed';
        ta.style.top = '-1000px';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        ta.setSelectionRange(0, ta.value.length);
        let ok = false;
        try { ok = document.execCommand('copy'); } catch (_) { ok = false; }
        ta.remove();
        ok ? resolve() : reject(new Error('copy failed'));
    });
}

function onMediaChange(mql, handler) {
    if (typeof mql.addEventListener === 'function') mql.addEventListener('change', handler);
    else if (typeof mql.addListener === 'function') mql.addListener(handler);
}

window.closeAllDropdowns = function(exceptElement) {
    const settingsDropdown = document.getElementById('settingsDropdown');
    const settingsBtn = document.getElementById('settingsBtn');
    if (settingsDropdown && settingsDropdown !== exceptElement) {
        settingsDropdown.classList.remove('open');
        if (settingsBtn) settingsBtn.setAttribute('aria-expanded', 'false');
    }
    document.querySelectorAll('.upload-dropdown.open').forEach(dropdown => {
        if (dropdown !== exceptElement) {
            dropdown.classList.remove('open');
            const btn = dropdown.querySelector('.upload-toggle-btn');
            if (btn) btn.setAttribute('aria-expanded', 'false');
        }
    });
    document.querySelectorAll('.more-dropdown.open').forEach(dropdown => {
        if (dropdown !== exceptElement) {
            dropdown.classList.remove('open');
            const btn = dropdown.querySelector('.more-toggle-btn');
            if (btn) btn.setAttribute('aria-expanded', 'false');
        }
    });
    document.querySelectorAll('.custom-select.open').forEach(select => {
        if (select !== exceptElement) {
            select.classList.remove('open');
            const btn = select.querySelector('.custom-select-btn');
            if (btn) btn.setAttribute('aria-expanded', 'false');
        }
    });
};


// A single delegated listener closes every open dropdown when the user
// clicks anywhere else. Toggle buttons call stopPropagation(), so opening
// clicks never reach this; menu item clicks close their own menu.
function initDropdownCloser() {
    document.addEventListener('click', (e) => {
        if (e.target.closest('.upload-dropdown, .more-dropdown, .custom-select, .settings-dropdown')) return;
        closeAllDropdowns();
    });
}


// INITIALIZATION
// Every step is isolated: one broken API/feature in an exotic browser must
// never take the rest of the app down with it (in particular the create-link
// form, which is bound first).
function safeStep(name, fn) {
    try {
        return fn();
    } catch (e) {
        console.warn(`[init] ${name} failed:`, e);
        return undefined;
    }
}

async function safeStepAsync(name, fn) {
    try {
        return await fn();
    } catch (e) {
        console.warn(`[init] ${name} failed:`, e);
        return undefined;
    }
}

function initApp() {
    // Bind all interactive controls BEFORE anything async so the create form,
    // modals and dialogs work even if loading data or translations fails.
    safeStep('listeners', setupGlobalListeners);
    safeStep('theme', initTheme);
    safeStep('view', initView);
    safeStep('search-sort', initSearchSort);
    safeStep('lazy', initLazyLoading);
    safeStep('shortcuts', initKeyboardShortcuts);
    safeStep('dropdown-closer', initDropdownCloser);
    safeStep('pwa', initPWA);
    safeStep('drop-zone', setupGlobalDropZone);
    safeStep('drag-hint', showDragDropHint);
    safeStep('skeletons', showSkeletons);

    safeStep('launch-action', handleLaunchAction);

    return (async () => {
        await safeStepAsync('language', initLanguage);
        await safeStepAsync('compression-config', loadCompressionConfig);
        safeStep('compression', initCompression);
        safeStep('app-version', loadAppVersion);
        await safeStepAsync('links', loadLinks);
    })();
}

// PWA shortcut from manifest.json ("Create new link" -> /admin?action=create).
function handleLaunchAction() {
    const params = new URLSearchParams(window.location.search);
    if (params.get('action') !== 'create') return;
    history.replaceState(null, '', window.location.pathname);
    focusAfterPaint(DOM.createInput);
}

document.addEventListener('DOMContentLoaded', () => {
    initApp();
});


function initPWA() {
    if ('serviceWorker' in navigator) {
        // Keep the root-scoped worker to replace older versions that cached
        // admin pages and public media. The current worker only caches /static/
        // assets; admin, API and media requests always go to the network.
        navigator.serviceWorker.register('/sw.js', { scope: '/' }).catch(() => {});
        // Remove the legacy worker from versions that registered it under
        // /static/ where it never actually controlled the app.
        navigator.serviceWorker.getRegistrations()
            .then(regs => regs.forEach(r => {
                if (new URL(r.scope).pathname.replace(/\/+$/, '') === '/static') {
                    r.unregister();
                }
            }))
            .catch(() => {});
    }

    // "Install app" entry: shown only while the browser offers installation
    // (Chrome/Edge desktop & Android). iOS installs via the share sheet.
    const installBtn = document.getElementById('installBtn');
    let installEvent = null;

    window.addEventListener('beforeinstallprompt', (e) => {
        e.preventDefault();
        installEvent = e;
        if (installBtn) installBtn.hidden = false;
    });

    if (installBtn) {
        installBtn.addEventListener('click', async () => {
            if (!installEvent) return;
            installBtn.hidden = true;
            installEvent.prompt();
            try {
                const choice = await installEvent.userChoice;
                log('[PWA] install outcome:', choice && choice.outcome);
            } catch (_) {}
            installEvent = null;
        });
    }

    window.addEventListener('appinstalled', () => {
        if (installBtn) installBtn.hidden = true;
        installEvent = null;
        showToast(t('installed', 'App installed'), 'success');
    });
}


async function loadCompressionConfig() {
    try {
        const res = await fetch('/api/compression-config');
        if (res.ok) STATE.compressionConfig = await res.json();
    } catch (_) {}
}


// Browser-side JPEG re-encoding only reduces upload size. Dimensions are left
// to the server, which applies COMPRESSION_SCALE to every upload path (file,
// URL, server gallery) exactly once. Without the server settings the browser
// cannot know whether lossless mode is active, so originals are sent as-is.
function initCompression() {
    if (typeof ImageCompressor === 'undefined' || !STATE.compressionConfig) return;

    const { quality, scale } = STATE.compressionConfig;
    if (!Number.isFinite(quality) || !Number.isFinite(scale)) return;

    STATE.compressor = new ImageCompressor({
        quality: quality / 100,
        preserveOriginal: quality === 100 && scale === 100
    });
    log(`[Compression] ${quality}% quality, ${scale}% scale (applied by the server)`);
}


function initLazyLoading() {
    if (!('IntersectionObserver' in window)) return;

    STATE.lazyObserver = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
            if (entry.isIntersecting) {
                const img = entry.target;
                if (img.dataset.src) {
                    img.src = img.dataset.src;
                    img.removeAttribute('data-src');
                    STATE.lazyObserver.unobserve(img);
                }
            }
        });
    // Larger rootMargin for taller mobile cards (160px preview height)
    }, { rootMargin: '200px 0px', threshold: 0.01 });
}


function initKeyboardShortcuts() {
    document.addEventListener('keydown', (e) => {
        // e.target is usually an Element, but can be document in exotic cases
        // where .matches does not exist.
        if (e.target && typeof e.target.matches === 'function' && e.target.matches('input, textarea')) return;

        const mod = e.ctrlKey || e.metaKey;

        if (mod && !e.altKey && !e.shiftKey) {
            switch ((e.key || '').toLowerCase()) {
                case 'n': e.preventDefault(); DOM.createInput.focus(); return;
                case 'f':
                    e.preventDefault();
                    DOM.searchInput.focus();
                    DOM.searchInput.select();
                    return;
                case 'g': e.preventDefault(); DOM.viewBtn.click(); return;
            }
            return;
        }

        if (mod || e.altKey) return;

        if (e.key === 'Escape') {
            if (!DOM.confirmOverlay.classList.contains('hidden')) {
                closeConfirm();
            } else if (!DOM.modalOverlay.classList.contains('hidden')) {
                closeModal();
            } else if (DOM.versionsOverlay && !DOM.versionsOverlay.classList.contains('hidden')) {
                closeVersions();
            } else if (document.querySelector('.upload-dropdown.open, .more-dropdown.open, .custom-select.open, .settings-dropdown.open')) {
                closeAllDropdowns();
            } else if (DOM.searchInput.value) {
                DOM.searchInput.value = '';
                DOM.searchInput.dispatchEvent(new Event('input'));
            }
        } else if (e.key === 't' || e.key === 'T') {
            DOM.themeBtn.click();
        }
    });

    if (!localStorage.getItem('shortcuts-seen')) {
        setTimeout(() => {
            showToast(t('shortcuts_hint', 'Shortcuts: Ctrl+N (new), Ctrl+F (search), Ctrl+G (view), T (theme)'), 'info');
            localStorage.setItem('shortcuts-seen', 'true');
        }, 2000);
    }
}


function showDragDropHint() {
    if (!localStorage.getItem('dragdrop-hint-seen')) {
        setTimeout(() => {
            showToast(t('dragdrop_hint', 'Drag & drop files anywhere to upload'), 'info');
            localStorage.setItem('dragdrop-hint-seen', 'true');
        }, 4000);
    }
}


async function loadAppVersion() {
    try {
        const res = await fetch('/health');
        if (!res.ok) return;
        const data = await res.json();
        if (data.version && DOM.appVersion) DOM.appVersion.textContent = `v${data.version}`;
    } catch (_) {}
}


// SKELETON LOADING
function buildSkeletonCard(isGrid) {
    const card = document.createElement('div');
    card.className = `skeleton-card ${isGrid ? 'grid-skeleton' : 'list-skeleton'}`;
    card.setAttribute('aria-hidden', 'true');
    card.innerHTML = `
        <div class="skeleton-bone skeleton-preview"></div>
        <div class="skeleton-info">
            <div class="skeleton-bone skeleton-title"></div>
            <div class="skeleton-bone skeleton-meta"></div>
            <div class="skeleton-bone skeleton-meta-short"></div>
        </div>
        <div class="skeleton-actions">
            <div class="skeleton-bone skeleton-btn"></div>
            <div class="skeleton-bone skeleton-btn-sq"></div>
        </div>
    `;
    return card;
}

function showSkeletons(count = 4) {
    DOM.linksList.innerHTML = '';
    DOM.emptyState.classList.add('d-none');
    const isGrid = STATE.viewMode === 'grid';
    const frag = document.createDocumentFragment();
    for (let i = 0; i < count; i++) {
        frag.appendChild(buildSkeletonCard(isGrid));
    }
    DOM.linksList.appendChild(frag);
}


// THEME
function initTheme() {
    const saved = localStorage.getItem('theme');
    STATE.isDark = saved ? saved === 'dark' : window.matchMedia('(prefers-color-scheme: dark)').matches;
    applyTheme();

    onMediaChange(window.matchMedia('(prefers-color-scheme: dark)'), e => {
        if (!localStorage.getItem('theme')) {
            STATE.isDark = e.matches;
            applyTheme();
        }
    });

    DOM.themeBtn.addEventListener('click', () => {
        STATE.isDark = !STATE.isDark;
        localStorage.setItem('theme', STATE.isDark ? 'dark' : 'light');
        applyTheme();
    });
}


function applyTheme() {
    const isDark = STATE.isDark;
    document.body.classList.toggle('dark', isDark);
    // Native form controls and scrollbars follow color-scheme.
    document.documentElement.style.colorScheme = isDark ? 'dark' : 'light';

    // One resolved color for the browser UI: media attributes are dropped so
    // a manual toggle wins over the OS preference from now on.
    const themeColor = isDark ? '#0b0c10' : '#f3f2f7';
    document.querySelectorAll('meta[name="theme-color"]').forEach(meta => {
        meta.removeAttribute('media');
        meta.content = themeColor;
    });

    const logo = document.querySelector('.logo');
    if (logo) logo.src = isDark ? '/static/logo-dark.svg' : '/static/logo.svg';

    // Sun icon shows in light mode, moon in dark mode.
    DOM.themeBtn.querySelectorAll('.theme-icon').forEach(icon => {
        icon.classList.toggle('active', (icon.dataset.icon === 'moon') === isDark);
    });

    document.documentElement.setAttribute('data-theme', isDark ? 'dark' : 'light');
}


// VIEW
function initView() {
    applyViewMode(STATE.viewMode, false);
    DOM.viewBtn.addEventListener('click', () => {
        const newMode = STATE.viewMode === 'list' ? 'grid' : 'list';
        STATE.viewMode = newMode;
        localStorage.setItem('viewMode', newMode);
        applyViewMode(newMode, true);
    });
}


function applyViewMode(mode, animate = false) {
    updateIconClasses(mode);
    if (animate) {
        DOM.linksList.classList.add('switching');
        setTimeout(() => {
            updateLayoutClasses(mode);
            void DOM.linksList.offsetHeight;
            requestAnimationFrame(() => DOM.linksList.classList.remove('switching'));
        }, 100);
    } else {
        updateLayoutClasses(mode);
    }
}


function updateIconClasses(mode) {
    const isGrid = mode === 'grid';
    DOM.viewBtn.querySelectorAll('.list-icon').forEach(el => el.classList.toggle('active', isGrid));
    DOM.viewBtn.querySelectorAll('.grid-icon').forEach(el => el.classList.toggle('active', !isGrid));
}


function updateLayoutClasses(mode) {
    DOM.linksList.classList.toggle('grid-view', mode === 'grid');
}


// LANGUAGE
async function initLanguage() {
    await setLanguage(STATE.lang);
}

window.setLanguage = setLanguage;

async function setLanguage(lang) {
    // Language can come from localStorage or an imported backup. Never use an
    // arbitrary value as a same-origin URL path.
    lang = SUPPORTED_LANGS.has(lang) ? lang : 'en';
    STATE.lang = lang;
    localStorage.setItem('lang', lang);
    document.documentElement.lang = lang;

    try {
        const res = await fetch(`/static/i18n/${lang}.json`);
        STATE.translations = res.ok ? await res.json() : {};
    } catch (_) {
        STATE.translations = {};
    }

    applyTranslations();
    syncCustomSelectLabels();
    updateSearchStats();
    updateAriaLabels();
    refreshCardsLanguage();

    document.querySelectorAll('.lang-option').forEach(opt => {
        opt.classList.toggle('active', opt.dataset.lang === lang);
    });
}


function applyTranslations(root = document) {
    root.querySelectorAll('[data-i18n]').forEach(el => {
        const key = el.dataset.i18n;
        if (STATE.translations[key]) el.textContent = STATE.translations[key];
    });
    root.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
        const key = el.dataset.i18nPlaceholder;
        if (STATE.translations[key]) el.placeholder = STATE.translations[key];
    });
    root.querySelectorAll('[data-i18n-title]').forEach(el => {
        const key = el.dataset.i18nTitle;
        if (STATE.translations[key]) el.title = STATE.translations[key];
    });
}


function t(key, defaultText) {
    return STATE.translations[key] || defaultText;
}

// The settings menu builds the palette switcher before translations land.
window.t = t;


function updateAriaLabels() {
    document.querySelectorAll('[data-i18n-aria]').forEach(el => {
        const key = el.dataset.i18nAria;
        if (STATE.translations[key]) el.setAttribute('aria-label', STATE.translations[key]);
    });
}


// Language-independent pieces of a card are rebuilt at render time; this pass
// refreshes the ones a language switch would otherwise leave in the old
// language: the meta line, the access row built by setupAccessControl() and
// the alt text of the preview.
function refreshCardsLanguage() {
    const list = DOM.linksList;
    if (!list) return;
    list.querySelectorAll('.link-card').forEach(card => {
        if (card._link) {
            updateCard(card, card._link);
            setupPinButton(card, card._link);
        }
        card.querySelectorAll('img.preview').forEach(img => {
            img.alt = t('alt_preview', 'Preview');
        });
    });
}


// SERVER ERROR TRANSLATION
function translateServerError(errorText) {
    // Map known server errors to translation keys
    const errorMap = {
        'Link name already taken': 'link_taken',
        'Link exists': 'link_taken',
        'Invalid link name': 'invalid_id_chars',
        'Link not found': 'link_not_found',
        'Invalid JSON': 'invalid_json',
    };

    const key = errorMap[errorText];
    return key ? t(key, errorText) : errorText;
}


// SEARCH & SORT
function initSearchSort() {
    if (!DOM.searchInput) return;

    STATE.searchQuery = localStorage.getItem('searchQuery') || '';
    STATE.sortBy = localStorage.getItem('sortBy') || 'date_desc';
    DOM.searchInput.value = STATE.searchQuery;
    if (DOM.sortSelect) DOM.sortSelect.value = STATE.sortBy;

    let timer;
    DOM.searchInput.addEventListener('input', (e) => {
        clearTimeout(timer);
        timer = setTimeout(() => {
            STATE.searchQuery = e.target.value.toLowerCase().trim();
            localStorage.setItem('searchQuery', STATE.searchQuery);
            filterAndSort();
        }, 250);
    });

    if (DOM.sortSelect) {
        DOM.sortSelect.addEventListener('change', (e) => {
            STATE.sortBy = e.target.value;
            localStorage.setItem('sortBy', STATE.sortBy);
            filterAndSort();
        });
    }

    // Clickable counter to reset search
    if (DOM.searchStats) {
        DOM.searchStats.addEventListener('click', () => {
            if (STATE.searchQuery) {
                DOM.searchInput.value = '';
                STATE.searchQuery = '';
                localStorage.setItem('searchQuery', '');
                filterAndSort();
                showToast(t('search_reset', 'Search cleared'), 'info');
            }
        });
    }

    initCustomSelect();
}


function initCustomSelect() {
    const customSelect = document.getElementById('customSortSelect');
    if (!customSelect) return;

    const btn = customSelect.querySelector('.custom-select-btn');
    const label = document.getElementById('customSortLabel');
    const options = customSelect.querySelectorAll('.custom-select-option');

    syncCustomSelectLabels();

    btn.addEventListener('click', (e) => {
        e.stopPropagation();
        const isOpen = customSelect.classList.contains('open');
        if (!isOpen) closeAllDropdowns(customSelect);
        customSelect.classList.toggle('open', !isOpen);
        btn.setAttribute('aria-expanded', String(!isOpen));
    });

    options.forEach(opt => {
        opt.addEventListener('click', () => selectOption(opt));
        // Keyboard: the list is a listbox, so the arrow keys must move
        // through it and Enter must pick — buttons alone are not enough.
        opt.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                selectOption(opt);
                btn.focus();
                return;
            }
            if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
            e.preventDefault();
            const list = Array.from(options);
            const index = list.indexOf(opt);
            const next = list[(index + (e.key === 'ArrowDown' ? 1 : list.length - 1)) % list.length];
            if (next) next.focus();
        });
        opt.addEventListener('focus', () => {
            options.forEach(o => o.classList.toggle('focused', o === opt));
        });
        opt.addEventListener('blur', () => opt.classList.remove('focused'));
    });

    // Opening the menu puts the keyboard on the selected option.
    btn.addEventListener('keydown', (e) => {
        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault();
            if (!customSelect.classList.contains('open')) btn.click();
            const current = customSelect.querySelector('.custom-select-option.selected') || options[0];
            if (current) current.focus();
        }
    });

    function selectOption(opt) {
        const val = opt.dataset.value;
        options.forEach(o => {
            o.classList.remove('selected');
            o.setAttribute('aria-selected', 'false');
        });
        opt.classList.add('selected');
        opt.setAttribute('aria-selected', 'true');
        if (label) label.textContent = optionLabel(opt);
        customSelect.classList.remove('open');
        btn.setAttribute('aria-expanded', 'false');
        if (DOM.sortSelect) DOM.sortSelect.value = val;
        STATE.sortBy = val;
        localStorage.setItem('sortBy', val);
        filterAndSort();
    }
}


// Each option is an icon plus a <span data-i18n>: the label is read from the
// span so translating or selecting an option never wipes its icon.
function optionLabel(opt) {
    const textEl = opt.querySelector('[data-i18n]');
    return (textEl || opt).textContent.trim();
}

function syncCustomSelectLabels() {
    const customSelect = document.getElementById('customSortSelect');
    if (!customSelect) return;

    const label = document.getElementById('customSortLabel');
    const options = customSelect.querySelectorAll('.custom-select-option');

    options.forEach(opt => {
        const textEl = opt.querySelector('[data-i18n]');
        const i18nKey = textEl && textEl.dataset.i18n;
        if (i18nKey && STATE.translations[i18nKey]) textEl.textContent = STATE.translations[i18nKey];
        const isSelected = opt.dataset.value === STATE.sortBy;
        opt.classList.toggle('selected', isSelected);
        opt.setAttribute('aria-selected', String(isSelected));
        if (isSelected && label) label.textContent = optionLabel(opt);
    });
}


function filterWallpapers() {
    if (!STATE.searchQuery) {
        STATE.filteredWallpapers = [...STATE.wallpapers];
        return;
    }
    const query = STATE.searchQuery;
    const typeQuery = query.replace(/^\./, '');
    STATE.filteredWallpapers = STATE.wallpapers.filter(wp => {
        const name = (wp.linkName || wp.id || '').toLowerCase();
        // "Search by file": a file type such as "png" or ".mp4" also matches.
        const type = (wp.mimeType || '').toLowerCase();
        return name.includes(query) || (type !== '' && type === typeQuery);
    });
}


function sortWallpapers(list) {
    const sortFns = {
        name_asc:  (a, b) => (a.linkName || '').localeCompare(b.linkName || ''),
        name_desc: (a, b) => (b.linkName || '').localeCompare(a.linkName || ''),
        date_desc: (a, b) => (b.createdAt || 0) - (a.createdAt || 0),
        date_asc:  (a, b) => (a.createdAt || 0) - (b.createdAt || 0),
    };
    const sortFn = sortFns[STATE.sortBy] || sortFns.date_desc;
    return [...list].sort((a, b) => {
        if (!!a.pinned !== !!b.pinned) return a.pinned ? -1 : 1;
        return sortFn(a, b);
    });
}


function filterAndSort() {
    filterWallpapers();
    STATE.filteredWallpapers = sortWallpapers(STATE.filteredWallpapers);
    updateSearchStats();
    renderLinks(STATE.filteredWallpapers);
}


function updateSearchStats() {
    if (!DOM.searchStats) return;
    const total = STATE.wallpapers.length;
    const shown = STATE.filteredWallpapers.length;

    // Make counter clickable when filtered
    const isFiltered = STATE.searchQuery !== '';
    DOM.searchStats.classList.toggle('clickable', isFiltered);

    if (isFiltered) {
        const tpl = t('search_found', 'Found {{shown}} of {{total}}');
        DOM.searchStats.textContent = tpl.replace('{{shown}}', shown).replace('{{total}}', total);
        DOM.searchStats.title = t('click_to_reset', 'Click to reset search');
    } else {
        const tpl = t('search_total', 'Total: {{total}}');
        DOM.searchStats.textContent = tpl.replace('{{total}}', total);
        DOM.searchStats.title = '';
    }
}


// ============================================================
// GLOBAL DRAG & DROP
// ============================================================
function sanitizeLinkName(filename) {
    // Remove extension
    let name = filename.replace(/\.[^.]+$/, '');
    // Replace non-alphanumeric with hyphens
    name = name.replace(/[^a-zA-Z0-9]+/g, '-');
    // Trim hyphens
    name = name.replace(/^-+|-+$/g, '');
    // Lowercase
    name = name.toLowerCase();
    // Max length
    return name.slice(0, 64);
}

function showDropOverlay() {
    if (DOM.dropOverlay) DOM.dropOverlay.classList.remove('hidden');
}

function hideDropOverlay() {
    if (DOM.dropOverlay) DOM.dropOverlay.classList.add('hidden');
}

function setupGlobalDropZone() {
    let dragCounter = 0;

    document.body.addEventListener('dragenter', (e) => {
        if (e.target.closest('.link-card')) return;
        dragCounter++;
        if (dragCounter === 1) showDropOverlay();
    });

    document.body.addEventListener('dragleave', (e) => {
        if (e.target.closest('.link-card')) return;
        dragCounter--;
        if (dragCounter === 0) hideDropOverlay();
    });

    document.body.addEventListener('dragover', (e) => {
        if (e.target.closest('.link-card')) return;
        e.preventDefault();
    });

    document.body.addEventListener('drop', async (e) => {
        if (e.target.closest('.link-card')) return;
        e.preventDefault();
        dragCounter = 0;
        hideDropOverlay();

        const files = Array.from(e.dataTransfer.files);
        if (!files.length) return;

        for (const file of files) {
            await createAndUpload(file);
        }
    });
}

async function createAndUpload(file) {
    const linkName = sanitizeLinkName(file.name);
    
    if (!linkName) {
        showToast(t('invalid_id', 'Invalid filename'), 'error');
        return;
    }

    // Check if file is valid image/video
    if (!file.type.startsWith('image/') && !file.type.startsWith('video/')) {
        showToast(t('invalid_image', 'Invalid file format'), 'error');
        return;
    }

    try {
        // Step 1: Create link
        await apiCall('/api/link', 'POST', { linkName });

        // Step 2: Upload file
        const formData = new FormData();
        formData.append('linkName', linkName);

        // Compress if image
        let fileToUpload = file;
        if (STATE.compressor && file.type.startsWith('image/')) {
            const originalSize = file.size;
            try {
                fileToUpload = await STATE.compressor.compress(file);
                if (fileToUpload.size < originalSize) {
                    const info = ImageCompressor.getCompressionInfo(originalSize, fileToUpload.size);
                    const msg = t('compression_saved', 'Compressed: {{percent}}% smaller ({{saved}} saved)')
                        .replace('{{percent}}', info.percent)
                        .replace('{{saved}}', formatSize(info.saved));
                    showToast(msg, 'success');
                }
            } catch (_) {
                fileToUpload = file;
            }
        }

        formData.append('file', fileToUpload);
        await apiCall('/api/upload', 'POST', formData, true);

        const msg = t('link_created_uploaded', 'Created "{{name}}" and uploaded')
            .replace('{{name}}', linkName);
        showToast(msg, 'success');

        // Refresh list
        await loadLinks();
    } catch (_) {
        // Error already shown by apiCall
    }
}


// TOASTS
const TOAST_ICONS = {
    success: `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>`,
    error:   `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>`,
    info:    `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>`,
};

const MAX_TOASTS = 4;

function showToast(message, type = 'success') {
    // Keep the stack short: a burst of errors must not flood the screen.
    while (DOM.toastContainer.children.length >= MAX_TOASTS) {
        DOM.toastContainer.firstElementChild.remove();
    }
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    const icon = TOAST_ICONS[type] || TOAST_ICONS.info;
    // Build DOM nodes instead of innerHTML so server/user text cannot inject HTML.
    const iconEl = document.createElement('div');
    iconEl.className = 'toast-icon';
    iconEl.innerHTML = icon; // icons are static trusted SVG constants only
    const contentEl = document.createElement('div');
    contentEl.className = 'toast-content';
    contentEl.textContent = message;
    toast.appendChild(iconEl);
    toast.appendChild(contentEl);
    DOM.toastContainer.appendChild(toast);
    setTimeout(() => {
        toast.classList.add('hiding');
        setTimeout(() => toast.remove(), 320);
    }, 3000);
}


// Focus management: keep Tab inside an open dialog and restore focus to
// the element that opened it when the dialog closes.
let lastFocused = null;

function trapFocus(overlay, e) {
    const focusables = overlay.querySelectorAll('button, input, select, a[href], [tabindex]:not([tabindex="-1"])');
    if (!focusables.length) return;
    const first = focusables[0];
    const last = focusables[focusables.length - 1];
    if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
    }
}

function openDialog(overlay) {
    lastFocused = document.activeElement;
    overlay.classList.remove('hidden');
    overlay.setAttribute('aria-hidden', 'false');
}

// Focus on open: do it immediately and again after the first painted frame.
// Some engines ignore focus() while the unhide transition is starting;
// others (headless shells) may never run the deferred call. Together the
// two attempts cover both worlds and are harmless when both run.
function focusAfterPaint(el) {
    if (!el) return;
    const tryFocus = () => { try { el.focus({ preventScroll: true }); } catch (_) { el.focus(); } };
    tryFocus();
    requestAnimationFrame(tryFocus);
}

function closeDialog(overlay) {
    overlay.classList.add('hidden');
    overlay.setAttribute('aria-hidden', 'true');
    if (lastFocused && document.contains(lastFocused)) {
        try { lastFocused.focus(); } catch (_) {}
    }
    lastFocused = null;
}


// CONFIRM MODAL
let confirmResolve = null;

function showConfirm(message) {
    return new Promise((resolve) => {
        confirmResolve = resolve;
        if (DOM.confirmMessage) DOM.confirmMessage.textContent = message;
        openDialog(DOM.confirmOverlay);
        focusAfterPaint(DOM.confirmDelete);
    });
}

function closeConfirm(result = false) {
    closeDialog(DOM.confirmOverlay);
    if (confirmResolve) confirmResolve(result);
    confirmResolve = null;
}


// MODAL
let modalResolve = null;

function showModal(type, titleKey, placeholderKey = '') {
    return new Promise((resolve) => {
        modalResolve = resolve;
        DOM.modalTitle.textContent = t(titleKey, t('modal_default_title', 'Input'));
        openDialog(DOM.modalOverlay);
        DOM.modalInput.value = '';
        DOM.modalInput.classList.add('d-none');
        DOM.modalList.innerHTML = '';
        DOM.modalList.classList.add('hidden');
        DOM.modalConfirm.onclick = null;

        if (type === 'input') {
            DOM.modalInput.classList.remove('d-none');
            DOM.modalInput.placeholder = placeholderKey ? t(placeholderKey, 'https://...') : t('url_placeholder', 'https://...');
            focusAfterPaint(DOM.modalInput);
            DOM.modalInput.onkeydown = (e) => { if (e.key === 'Enter') confirmModal(); };
        } else if (type === 'grid') {
            DOM.modalList.classList.remove('hidden');
            loadExternalImages();
        }

        DOM.modalCancel.onclick = closeModal;
        DOM.modalConfirm.onclick = confirmModal;
    });
}


function closeModal() {
    closeDialog(DOM.modalOverlay);
    if (modalResolve) modalResolve(null);
    modalResolve = null;
}


function confirmModal() {
    let result = !DOM.modalInput.classList.contains('d-none')
        ? DOM.modalInput.value.trim()
        : DOM.modalList.querySelector('.selected')?.dataset.value;

    if (result) {
        closeDialog(DOM.modalOverlay);
        if (modalResolve) modalResolve(result);
        modalResolve = null;
    } else {
        DOM.modalInput.classList.add('shake');
        setTimeout(() => DOM.modalInput.classList.remove('shake'), 300);
    }
}


function showModalListMessage(text, variant = '') {
    const message = document.createElement('div');
    message.className = `modal-list-msg ${variant}`.trim();
    message.textContent = text;
    DOM.modalList.replaceChildren(message);
}

async function loadExternalImages() {
    showModalListMessage(t('loading', 'Loading...'));
    try {
        const res = await fetch('/api/external-images');
        if (!res.ok) throw new Error('Failed');
        const files = await res.json();

        if (!files?.length) {
            showModalListMessage(t('server_empty', 'No images found'), 'muted');
            return;
        }

        DOM.modalList.innerHTML = '';
        const frag = document.createDocumentFragment();
        files.forEach(file => {
            const div = document.createElement('div');
            div.className = 'image-option';
            div.dataset.value = file;
            const previewUrl = `/api/external-image-preview?path=${encodeURIComponent(file)}`;
            const nameEl = document.createElement('div');
            nameEl.className = 'image-name';
            nameEl.textContent = file;
            const isVid = /\.(mp4|webm)$/i.test(file);
            let media;
            if (isVid) {
                // Metadata only: shows the first frame without downloading the video.
                media = document.createElement('video');
                media.muted = true;
                media.playsInline = true;
                media.preload = 'metadata';
                media.setAttribute('aria-label', file);
            } else {
                media = document.createElement('img');
                media.alt = file;
            }
            media.className = 'lazy-image-fade';
            const format = (file.split('.').pop() || '').toLowerCase();
            const markLoaded = () => {
                if (media.dataset.src) return; // lazy placeholder, not the file
                media.classList.add('loaded');
                applyPreviewFit(div, media, format, GALLERY_FRAME);
            };
            media.addEventListener(isVid ? 'loadeddata' : 'load', markLoaded);
            if (STATE.lazyObserver) {
                media.dataset.src = previewUrl;
                if (!isVid) media.src = LAZY_PLACEHOLDER;
                STATE.lazyObserver.observe(media);
            } else {
                media.src = previewUrl;
                markLoaded();
            }
            div.appendChild(media);
            div.appendChild(nameEl);
            div.onclick = () => {
                DOM.modalList.querySelectorAll('.image-option').forEach(el => el.classList.remove('selected'));
                div.classList.add('selected');
            };
            frag.appendChild(div);
        });
        DOM.modalList.appendChild(frag);
    } catch (_) {
        showModalListMessage(t('server_error', 'Error loading images'), 'error');
    }
}


// API
async function apiCall(url, method = 'GET', body = null, isFormData = false) {
    const options = {
        method,
        // Explicit credentials: WebKit historically omits HTTP-auth on fetch
        // without this, which would surface as spurious 401s.
        credentials: 'same-origin',
        headers: isFormData ? {} : { 'Content-Type': 'application/json' }
    };
    if (body) options.body = isFormData ? body : JSON.stringify(body);
    try {
        const res = await fetch(url, options);
        if (!res.ok) {
            // Server errors are plain text with a trailing newline.
            const text = (await res.text()).trim();
            const err = new Error(text || `HTTP ${res.status}`);
            err.status = res.status;
            throw err;
        }
        const contentType = res.headers.get('content-type');
        return contentType?.includes('application/json') ? res.json() : null;
    } catch (e) {
        // Better network error handling
        if (e.name === 'TypeError' || e.message === 'Failed to fetch') {
            showToast(t('network_error', 'Network error - check your connection'), 'error');
        } else if (e.status === 401) {
            showToast(t('auth_required', 'Authentication required — sign in again'), 'error');
        } else if (e.status === 403) {
            showToast(t('forbidden', 'Access denied'), 'error');
        } else {
            const translatedMsg = translateServerError(e.message);
            showToast(translatedMsg, 'error');
        }
        throw e;
    }
}


// Toggle the empty-state text between "no links" and a load-error hint.
function setEmptyStateError(isError) {
    const el = DOM.emptyState && DOM.emptyState.querySelector('.empty-state-text');
    if (!el) return;
    el.classList.toggle('error', !!isError);
    el.textContent = isError
        ? t('load_error_hint', 'Failed to load links — check the connection and refresh the page.')
        : t('no_links', 'No links yet. Create one above!');
}

async function loadLinks() {
    try {
        const res = await apiCall('/api/wallpapers');
        // The endpoint returns a bare array; tolerate a wrapped/paginated
        // shape defensively so a response change can never blank the list.
        STATE.wallpapers = Array.isArray(res) ? res : (res && Array.isArray(res.data) ? res.data : []);
        filterAndSort();
    } catch (_) {
        showToast(t('load_error', 'Failed to load links'), 'error');
        STATE.wallpapers = [];
        renderLinks([]);
        setEmptyStateError(true);
    }
}


// ============================================================
// PIN / UNPIN
// ============================================================
async function togglePin(link) {
    try {
        const updatedLink = await apiCall(
            `/api/link/${encodeURIComponent(link.linkName)}/pin`,
            'POST'
        );
        if (!updatedLink) return;

        // Update state; filterAndSort() below re-renders the card.
        const idx = STATE.wallpapers.findIndex(wp => wp.linkName === link.linkName);
        if (idx !== -1) {
            STATE.wallpapers[idx] = updatedLink;
        }
        link.pinned = updatedLink.pinned;

        // Pop the badge on the card that changed, not on every card at load.
        const card = DOM.linksList && DOM.linksList.querySelector(`[data-link-name="${CSS.escape(link.linkName)}"]`);
        const pinBtn = card && card.querySelector('.pin-btn');
        if (pinBtn) {
            pinBtn.classList.remove('bump');
            void pinBtn.offsetWidth;
            pinBtn.classList.add('bump');
            pinBtn.addEventListener('animationend', () => pinBtn.classList.remove('bump'), { once: true });
        }

        // Show toast
        const msgKey = updatedLink.pinned ? 'pinned' : 'unpinned';
        const msg = t(msgKey, updatedLink.pinned ? 'Pinned to top' : 'Unpinned');
        showToast(msg, 'success');

        // Re-sort and re-render to move item
        filterAndSort();
    } catch (_) {
        // Error already shown by apiCall
    }
}


function setupPinButton(card, link) {
    const pinBtn = card.querySelector('.pin-btn');
    if (!pinBtn) return;

    // Set initial state
    pinBtn.classList.toggle('pinned', !!link.pinned);
    const ariaKey = link.pinned ? 'aria_unpin' : 'aria_pin';
    const ariaLabel = t(ariaKey, link.pinned ? 'Unpin this link' : 'Pin this link to top');
    pinBtn.setAttribute('aria-label', ariaLabel);
    pinBtn.title = ariaLabel;
    // The badge carries the key for the current state, so a later language
    // switch re-labels it through applyTranslations()/updateAriaLabels().
    pinBtn.setAttribute('data-i18n-aria', ariaKey);
    pinBtn.setAttribute('data-i18n-title', ariaKey);

    // Called again on every language change: bind the handler only once.
    if (!pinBtn.dataset.bound) {
        pinBtn.dataset.bound = '1';
        pinBtn.addEventListener('click', (e) => {
            e.preventDefault();
            e.stopPropagation();
            togglePin(link);
        });
    }

    // The same action, worded for the menu: pin or unpin.
    const menuPin = card.querySelector('.more-pin-btn');
    if (menuPin) {
        const label = menuPin.querySelector('[data-i18n]');
        if (label) label.setAttribute('data-i18n', link.pinned ? 'menu_unpin' : 'menu_pin');
        applyTranslations(card);
    }
}


function createLinkCard(link) {
    const clone = DOM.template.content.cloneNode(true);
    const card = clone.querySelector('article');
    card._link = link;
    updateCard(card, link);
    setupCardEvents(card, link);
    setupPinButton(card, link);
    return card;
}

function renderLinks(wallpapers) {
    const links = wallpapers || [];
    if (!links.length) {
        DOM.linksList.replaceChildren();
        previewVideos.sweep();
        setEmptyStateError(false);
        DOM.emptyState.classList.remove('d-none');
        return;
    }
    DOM.emptyState.classList.add('d-none');

    // Reconcile by stable link ID. Search and sort only move existing cards;
    // they do not rebuild previews, copy handlers, or document listeners.
    // Children without a link ID (skeleton placeholders) are always removed.
    const existing = new Map();
    for (const card of Array.from(DOM.linksList.children)) {
        const name = card.dataset.linkName;
        if (name && !existing.has(name)) existing.set(name, card);
    }
    const wanted = new Set(links.map(link => link.linkName || link.id));
    for (const card of Array.from(DOM.linksList.children)) {
        if (!card.dataset.linkName || !wanted.has(card.dataset.linkName)) card.remove();
    }

    let cursor = DOM.linksList.firstElementChild;
    for (const link of links) {
        const name = link.linkName || link.id;
        let card = existing.get(name);
        if (!card || card._link !== link) {
            if (card) {
                if (card === cursor) cursor = card.nextElementSibling;
                card.remove();
            }
            card = createLinkCard(link);
        }

        if (card !== cursor) DOM.linksList.insertBefore(card, cursor);
        cursor = card.nextElementSibling;
    }

    applyTranslations(DOM.linksList);
    updateAriaLabels();
    previewVideos.sweep();
}


function detectCategory(link) {
    const mime = link.mimeType || '';
    if (mime === 'mp4' || mime === 'webm') return 'video';
    return mime || link.hasImage ? 'image' : 'other';
}


const LAZY_PLACEHOLDER = 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"%3E%3C/svg%3E';

function createLazyImage(src, alt, className = 'preview') {
    const img = document.createElement('img');
    if (STATE.lazyObserver) {
        img.dataset.src = src;
        img.src = LAZY_PLACEHOLDER;
        STATE.lazyObserver.observe(img);
    } else {
        img.src = src;
    }
    img.alt = alt || t('alt_image', 'Image');
    img.className = className;
    img.loading = 'lazy';
    // Decoding off the main thread: a long grid of previews no longer
    // blocks the first paint after a page load.
    img.decoding = 'async';
    return img;
}


// ============================================================
// PREVIEW FIT & VIDEO PLAYBACK
// ============================================================
// Thumbnails keep the source aspect ratio and are never upscaled, so an app
// icon arrives as a small square. Filling a 16:9 frame with it (cover) would
// crop and enlarge it. previewFit() picks how CSS presents each image:
//   'fit-icon'     small, or square with possible transparency: natural size,
//                  centred on a blurred copy of itself
//   'fit-contain'  far from the frame's aspect ratio (portrait, panorama):
//                  the whole image on the same blurred backdrop
//   ''             photos and wallpapers: fill the frame
const ICON_MAX_SIDE = 256;
const ALPHA_FORMATS = new Set(['png', 'webp', 'gif']);
const CARD_FRAME = { min: 1.3, max: 2.4 };     // 16:9 card previews
const GALLERY_FRAME = { min: 0.95, max: 1.9 }; // 4:3 gallery tiles

function previewFit(width, height, format, frame) {
    if (!width || !height) return '';
    const ratio = width / height;
    const squarish = ratio >= 0.8 && ratio <= 1.25;
    if (Math.max(width, height) <= ICON_MAX_SIDE || (squarish && ALPHA_FORMATS.has(format))) {
        return 'fit-icon';
    }
    return ratio < frame.min || ratio > frame.max ? 'fit-contain' : '';
}

const cssUrl = url => `url("${url.replace(/["\\\n\r]/g, c => encodeURIComponent(c))}")`;

function applyPreviewFit(container, media, format, frame) {
    const isImg = media.tagName === 'IMG';
    let fit = isImg
        ? previewFit(media.naturalWidth, media.naturalHeight, format, frame)
        : previewFit(media.videoWidth, media.videoHeight, '', frame);
    if (!isImg && fit) fit = 'fit-contain'; // videos are letterboxed, never icons
    container.classList.toggle('fit-icon', fit === 'fit-icon');
    container.classList.toggle('fit-contain', fit === 'fit-contain');
    // Checkerboard only where transparency is actually possible (a fitted
    // PNG/WebP/GIF), never behind a photo or a video frame.
    container.classList.toggle('has-alpha', !!fit && ALPHA_FORMATS.has(format));
    // The backdrop reuses the loaded image from the memory cache (no second
    // request). CSSOM style changes are permitted by the CSP.
    if (fit && isImg) container.style.setProperty('--thumb', cssUrl(media.currentSrc || media.src));
    else container.style.removeProperty('--thumb');
    updateMediaBadges(container, media, format);
}

function resetPreviewFit(container) {
    container.classList.remove('fit-icon', 'fit-contain', 'has-alpha');
    container.style.removeProperty('--thumb');
    hideMediaBadges(container);
}

// ------------------------------------------------------------
// MEDIA BADGES — format, aspect ratio and duration
// ------------------------------------------------------------
// Read off the file that actually loaded, so a card never claims JPEG for a
// file that turned out to be a PNG. Only cards have .preview-meta; modals
// and the gallery share applyPreviewFit() but show no badges.
function gcd(a, b) {
    while (b) { const rest = a % b; a = b; b = rest; }
    return a;
}

// Camera and phone sizes rarely reduce to a small pair (768x1376 is 24:43),
// so the label snaps to the nearest familiar ratio first.
const COMMON_RATIOS = [
    [1, 1], [4, 3], [3, 2], [16, 10], [16, 9], [21, 9], [2, 1],
    [9, 16], [3, 4], [2, 3], [10, 16], [9, 21], [1, 2],
];

function ratioLabel(width, height) {
    if (!width || !height) return '';
    const ratio = width / height;
    let best = '';
    let bestDiff = 0.02;   // 2 % off a familiar ratio
    for (const [w, h] of COMMON_RATIOS) {
        const diff = Math.abs(ratio - w / h) / (w / h);
        if (diff < bestDiff) {
            bestDiff = diff;
            best = `${w}:${h}`;
        }
    }
    if (best) return best;
    const g = gcd(width, height) || 1;
    const w = Math.round(width / g);
    const h = Math.round(height / g);
    return (w <= 64 && h <= 64) ? `${w}:${h}` : '';
}

function formatDuration(seconds) {
    if (!isFinite(seconds) || seconds <= 0) return '';
    const total = Math.round(seconds);
    const minutes = Math.floor(total / 60);
    const rest = total % 60;
    return `${minutes}:${String(rest).padStart(2, '0')}`;
}

function mediaBadgeTargets(container) {
    const link = container.parentElement;
    const meta = link && link.querySelector('.preview-meta');
    if (!meta) return null;
    return {
        fmt: meta.querySelector('.media-badge--fmt'),
        ratio: meta.querySelector('.media-badge--ratio'),
        dur: meta.querySelector('.media-badge--dur'),
    };
}

function setBadge(el, text) {
    if (!el) return;
    if (text) {
        el.textContent = text;
        el.hidden = false;
    } else {
        el.textContent = '';
        el.hidden = true;
    }
}

function hideMediaBadges(container) {
    const badges = mediaBadgeTargets(container);
    if (!badges) return;
    setBadge(badges.fmt, '');
    setBadge(badges.ratio, '');
    setBadge(badges.dur, '');
}

function updateMediaBadges(container, media, format) {
    const badges = mediaBadgeTargets(container);
    if (!badges) return;

    const isImg = media.tagName === 'IMG';
    const width = isImg ? media.naturalWidth : media.videoWidth;
    const height = isImg ? media.naturalHeight : media.videoHeight;

    // The stored MIME type is a short token ('jpeg', 'mp4'); fall back to the
    // file extension of what was actually loaded.
    let fmt = (format || '').toUpperCase();
    if (!fmt) {
        const src = media.currentSrc || media.src || '';
        const ext = src.split('?')[0].split('.').pop();
        fmt = /^[a-z0-9]{2,5}$/i.test(ext) ? ext.toUpperCase() : '';
    }
    setBadge(badges.fmt, fmt);

    // The ratio only earns its place when it differs from the 16:9 frame.
    const ratio = width && height ? width / height : 0;
    const nearFrame = ratio && Math.abs(ratio - 16 / 9) / (16 / 9) < 0.04;
    setBadge(badges.ratio, nearFrame ? '' : ratioLabel(width, height));

    setBadge(badges.dur, isImg ? '' : formatDuration(media.duration));
}

// Card videos play only while at least a quarter of them is on screen and
// the tab is visible, and never with reduced motion: offscreen previews cost
// no downloads, decoding or battery.
const previewVideos = (() => {
    const reduceMotion = window.matchMedia ? window.matchMedia('(prefers-reduced-motion: reduce)') : null;
    const observed = new Set();
    const visible = new Set();
    const allowed = () => !document.hidden && !(reduceMotion && reduceMotion.matches);
    const sync = video => {
        if (visible.has(video) && video.isConnected && allowed()) {
            const playing = video.play();
            if (playing) playing.catch(() => {}); // autoplay policy, or removed meanwhile
        } else if (!video.paused) {
            video.pause();
        }
    };
    const syncAll = () => visible.forEach(sync);
    const observer = 'IntersectionObserver' in window
        ? new IntersectionObserver(entries => {
            for (const entry of entries) {
                if (entry.isIntersecting) visible.add(entry.target);
                else visible.delete(entry.target);
                sync(entry.target);
            }
        }, { threshold: 0.25 })
        : null;
    document.addEventListener('visibilitychange', syncAll);
    if (reduceMotion && reduceMotion.addEventListener) reduceMotion.addEventListener('change', syncAll);
    return {
        observe(video) {
            if (observer) {
                observed.add(video);
                observer.observe(video);
            } else if (allowed()) {
                video.autoplay = true; // no IntersectionObserver: previous behaviour
            }
        },
        // Stop tracking videos whose cards were removed or rebuilt.
        sweep() {
            for (const video of observed) {
                if (video.isConnected) continue;
                observer.unobserve(video);
                observed.delete(video);
                visible.delete(video);
            }
        },
    };
})();


// ============================================================
// INLINE RENAME
// ============================================================
const VALID_LINK_RE = /^[a-zA-Z0-9][a-zA-Z0-9_\-]{0,63}$/;

function setupInlineRename(card, link) {
    const linkIdEl = card.querySelector('.link-id');
    if (!linkIdEl) return;

    // Pencil hint shown on hover via CSS; double-click to activate
    linkIdEl.title = t('rename_hint', 'Double-click to rename');
    linkIdEl.setAttribute('role', 'button');
    linkIdEl.tabIndex = 0;

    const startEdit = () => {
        if (linkIdEl.querySelector('input')) return; // already editing

        const currentName = link.linkName;
        const input = document.createElement('input');
        input.type = 'text';
        input.value = currentName;
        input.className = 'link-id-input';
        input.maxLength = 64;
        input.setAttribute('aria-label', t('rename_input_label', 'New link name'));
        input.setAttribute('pattern', '[a-zA-Z0-9_\\-]+');
        input.spellcheck = false;

        linkIdEl.textContent = '';
        linkIdEl.appendChild(input);
        linkIdEl.classList.add('editing');

        // Select all on focus so user can type immediately
        input.focus();
        input.select();

        let committed = false;

        const commit = async () => {
            if (committed) return;
            committed = true;

            const newName = input.value.trim();

            // Restore label regardless of outcome first
            linkIdEl.classList.remove('editing');

            if (!newName || newName === currentName) {
                linkIdEl.textContent = currentName;
                return;
            }

            if (!VALID_LINK_RE.test(newName)) {
                linkIdEl.textContent = currentName;
                showToast(t('invalid_id_chars', 'Invalid ID format'), 'error');
                return;
            }

            // Optimistic update
            linkIdEl.textContent = newName;
            card.dataset.linkName = newName;

            try {
                const updated = await apiCall(
                    `/api/link/${encodeURIComponent(currentName)}`,
                    'PATCH',
                    { newLinkName: newName }
                );
                if (!updated) throw new Error('Empty response');

                // Sync state; filterAndSort() below re-renders the card.
                const idx = STATE.wallpapers.findIndex(wp => wp.linkName === currentName);
                if (idx !== -1) STATE.wallpapers[idx] = updated;
                link.linkName = updated.linkName;

                const msg = t('renamed_success', 'Renamed to "{{name}}"').replace('{{name}}', updated.linkName);
                showToast(msg, 'success');
                filterAndSort();
            } catch (_) {
                // Roll back on error (apiCall already showed toast)
                linkIdEl.textContent = currentName;
                card.dataset.linkName = currentName;
                link.linkName = currentName;
            }
        };

        const cancel = () => {
            if (committed) return;
            committed = true;
            linkIdEl.classList.remove('editing');
            linkIdEl.textContent = currentName;
        };

        input.addEventListener('keydown', (e) => {
            if (e.key === 'Enter')  { e.preventDefault(); commit(); }
            if (e.key === 'Escape') { e.preventDefault(); cancel(); }
        });
        input.addEventListener('blur', commit);
    };

    linkIdEl.addEventListener('dblclick', startEdit);
    linkIdEl.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' || e.key === 'F2') { e.preventDefault(); startEdit(); }
    });

    // Same entry from the card menu (touch screens have no double-click).
    card._startRename = startEdit;
}



const ACCESS_LEVELS = ['public', 'local', 'token', 'auth'];

// Static, trusted SVG icons for the token action buttons.
const TOKEN_COPY_SVG = `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  <rect x="9" y="9" width="13" height="13" rx="2"/>
  <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
</svg>`;
const TOKEN_ROTATE_SVG = `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  <polyline points="1 4 1 10 7 10"/>
  <path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"/>
</svg>`;

function accessLabel(level) {
    const key = 'access_' + level;
    const defaults = {
        public: 'Public',
        local: 'Local network',
        token: 'Token',
        auth: 'Admin only',
    };
    return t(key, defaults[level] || level);
}

function publicLinkURL(link) {
    const name = link.linkName || link.id;
    let url = `${window.location.origin}/${name}`;
    if (link.accessLevel === 'token' && link.accessToken) {
        url += `?token=${encodeURIComponent(link.accessToken)}`;
    }
    return url;
}

function setupAccessControl(card, link) {
    let row = card.querySelector('.access-row');
    if (!row) {
        row = document.createElement('div');
        row.className = 'access-row';
        // The tile has no room for a permanent access row: it lives in the
        // ⋯ menu, right under the pin action.
        const host = card.querySelector('.card-menu') || card.querySelector('.link-info');
        if (host) host.appendChild(row);
        else return;
    }
    row.innerHTML = '';

    const label = document.createElement('label');
    label.className = 'access-label';
    label.textContent = t('access_label', 'Access');
    // htmlFor takes a plain id (CSS.escape is only for selectors).
    label.htmlFor = `access-${link.linkName || link.id}`;

    const select = document.createElement('select');
    select.className = 'access-select';
    select.id = `access-${link.linkName || link.id}`;
    select.setAttribute('aria-label', t('access_label', 'Access'));
    const current = link.accessLevel || 'public';
    ACCESS_LEVELS.forEach(level => {
        const opt = document.createElement('option');
        opt.value = level;
        opt.textContent = accessLabel(level);
        if (level === current) opt.selected = true;
        select.appendChild(opt);
    });

    select.addEventListener('change', async () => {
        const newLevel = select.value;
        select.disabled = true;
        try {
            const updated = await apiCall(
                `/api/link/${encodeURIComponent(link.linkName)}`,
                'PATCH',
                { accessLevel: newLevel }
            );
            if (!updated) {
                select.value = current;
                return;
            }
            Object.assign(link, {
                accessLevel: updated.accessLevel,
                accessToken: updated.accessToken || '',
            });
            const idx = STATE.wallpapers.findIndex(wp => wp.linkName === link.linkName);
            if (idx !== -1) {
                STATE.wallpapers[idx].accessLevel = link.accessLevel;
                STATE.wallpapers[idx].accessToken = link.accessToken;
            }
            // Refresh copy URL + token display
            setupAccessControl(card, link);
            updateCopyURL(card, link);
            showToast(t('access_updated', 'Access level updated'), 'success');
        } catch (_) {
            select.value = current;
        } finally {
            select.disabled = false;
        }
    });

    row.appendChild(label);
    row.appendChild(select);

    // Token chip with inline copy/rotate actions for token level
    if ((link.accessLevel || 'public') === 'token') {
        const tokenBox = document.createElement('div');
        tokenBox.className = 'access-token-box';

        const tokenEl = document.createElement('code');
        tokenEl.className = 'access-token-value';
        tokenEl.textContent = link.accessToken || '—';
        tokenEl.title = link.accessToken || '';
        tokenEl.setAttribute('aria-label', t('access_token', 'Access token'));

        const copyTok = document.createElement('button');
        copyTok.type = 'button';
        copyTok.className = 'token-icon-btn';
        copyTok.innerHTML = TOKEN_COPY_SVG; // static trusted markup
        const copyLabel = t('copy_token', 'Copy token URL');
        copyTok.setAttribute('aria-label', copyLabel);
        copyTok.title = copyLabel;
        copyTok.addEventListener('click', (e) => {
            e.preventDefault();
            copyToClipboard(publicLinkURL(link)).then(() => {
                showToast(t('copied', 'Copied!'), 'success');
            }).catch(() => showToast(t('copy_error', 'Failed to copy URL'), 'error'));
        });

        const rotateBtn = document.createElement('button');
        rotateBtn.type = 'button';
        rotateBtn.className = 'token-icon-btn';
        rotateBtn.innerHTML = TOKEN_ROTATE_SVG; // static trusted markup
        const rotateLabel = t('rotate_token', 'Rotate');
        rotateBtn.setAttribute('aria-label', rotateLabel);
        rotateBtn.title = rotateLabel;
        rotateBtn.addEventListener('click', async (e) => {
            e.preventDefault();
            rotateBtn.disabled = true;
            try {
                const updated = await apiCall(
                    `/api/link/${encodeURIComponent(link.linkName)}`,
                    'PATCH',
                    { rotateToken: true }
                );
                if (!updated) return;
                link.accessToken = updated.accessToken || '';
                const idx = STATE.wallpapers.findIndex(wp => wp.linkName === link.linkName);
                if (idx !== -1) STATE.wallpapers[idx].accessToken = link.accessToken;
                setupAccessControl(card, link);
                updateCopyURL(card, link);
                showToast(t('token_rotated', 'Token rotated'), 'success');
            } catch (_) {}
            finally { rotateBtn.disabled = false; }
        });

        tokenBox.appendChild(tokenEl);
        tokenBox.appendChild(copyTok);
        tokenBox.appendChild(rotateBtn);
        row.appendChild(tokenBox);
    }
}

// Keep the "open" link and the Copy URL button in sync with access changes.
function updateCopyURL(card, link) {
    const fullUrl = publicLinkURL(link);
    const previewLink = card.querySelector('.preview-link');
    if (previewLink) previewLink.href = fullUrl;
    const menuOpen = card.querySelector('.more-open-btn');
    if (menuOpen) menuOpen.href = fullUrl;
    card.dataset.publicUrl = fullUrl;
}


function updateCard(card, link) {
    const linkName = link.linkName || link.id;
    const linkIdEl = card.querySelector('.link-id');

    // Don't overwrite if currently in edit mode
    if (!linkIdEl.querySelector('input')) {
        linkIdEl.textContent = linkName;
    }
    card.dataset.linkName = linkName;

    const fullUrl = publicLinkURL(link);
    card.dataset.publicUrl = fullUrl;

    const previewLink = card.querySelector('.preview-link');
    previewLink.href = fullUrl;
    previewLink.setAttribute('aria-label', t('aria_open_image', 'Open image'));
    linkIdEl.setAttribute('aria-label', t('aria_link_id', 'Link ID'));

    const category = link.hasImage ? detectCategory(link) : 'other';
    const categoryLabel = t('cat_' + category, category);

    let fileType;
    if (link.mimeType) {
        fileType = link.mimeType.toUpperCase();
    } else if (link.hasImage) {
        fileType = 'IMAGE';
    } else {
        fileType = t('no_image', 'No image');
    }

    const dateStr = link.createdAt ? formatDate(link.createdAt) : '—';
    const sizeStr = link.sizeBytes ? ` · ${formatSize(link.sizeBytes)}` : '';

    const linkMeta = card.querySelector('.link-meta');
    // Version, playlist, rotation and hit counters are appended only when the
    // link actually uses them, so untouched cards read exactly as before.
    const extras = [];
    if (link.currentVersion > 1) {
        extras.push(t('meta_version', 'v{{n}}').replace('{{n}}', link.currentVersion));
    }
    if (Array.isArray(link.items) && link.items.length) {
        extras.push(t('meta_items', '{{n}} in playlist').replace('{{n}}', link.items.length));
    }
    if (link.rotate && link.rotate.enabled) extras.push(t('meta_rotating', 'rotating'));
    if (link.stats && link.stats.hits) {
        extras.push(t('meta_hits', '{{n}} hits').replace('{{n}}', link.stats.hits));
    }
    linkMeta.textContent = [`${categoryLabel} · ${fileType}${sizeStr} · ${dateStr}`, ...extras].join(' · ');
    linkMeta.setAttribute('aria-label', t('aria_file_info', 'File info'));

    setupAccessControl(card, link);

    const previewWrapper = card.querySelector('.preview-wrapper');

    // Only re-build preview when it has actually changed (avoid video flicker)
    const prevSrc = previewWrapper.dataset.src || '';
    const newSrc  = link.hasImage ? (link.preview || link.imageUrl || '') : '';
    const srcChanged = prevSrc !== newSrc;

    if (srcChanged) {
        previewWrapper.dataset.src = newSrc;
        
        // Save pin button reference before clearing
        const existingPinBtn = previewWrapper.querySelector('.pin-btn');
        
        // Clear content
        previewWrapper.innerHTML = '';
        resetPreviewFit(previewWrapper);
        
        // Re-add pin button first
        if (existingPinBtn) {
            previewWrapper.appendChild(existingPinBtn);
        }

        if (link.hasImage) {
            const isVid = (category === 'video');
            // Bust browser/API caches only when the file actually changed.
            const bust = link.modTime ? `?t=${link.modTime}` : '';
            if (isVid) {
                // Admin preview route is auth-protected and works for all access levels.
                const videoSrc = (link.preview || ('/api/preview/' + encodeURIComponent(linkName))) + bust;
                const video = document.createElement('video');
                video.src = videoSrc;
                video.className = 'preview';
                video.muted = true;
                video.loop = true;
                video.playsInline = true;
                video.setAttribute('playsinline', '');
                video.setAttribute('preload', 'metadata');
                video.addEventListener('loadedmetadata', () =>
                    applyPreviewFit(previewWrapper, video, category, CARD_FRAME));
                video.onerror = () => {
                    // Keep pin button when showing error
                    const pinBtn = previewWrapper.querySelector('.pin-btn');
                    previewWrapper.innerHTML = '';
                    resetPreviewFit(previewWrapper);
                    if (pinBtn) previewWrapper.appendChild(pinBtn);
                    previewWrapper.appendChild(buildNoImageSVG());
                    previewVideos.sweep();
                };
                previewWrapper.appendChild(video);
                previewVideos.observe(video);
            } else {
                const resolvedPreview = link.preview || ('/api/preview/' + encodeURIComponent(linkName));
                const imgSrc = (resolvedPreview.startsWith('/') ? resolvedPreview : '/' + resolvedPreview) + bust;
                const img = createLazyImage(
                    imgSrc,
                    resolvedPreview ? t('alt_preview', 'Preview') : t('alt_image', 'Image'),
                    'preview'
                );
                img.classList.add('preview-top-center');
                img.addEventListener('load', () => {
                    // Skip the lazy-loading placeholder; classify the real preview.
                    if (!img.dataset.src) applyPreviewFit(previewWrapper, img, link.mimeType, CARD_FRAME);
                });
                img.onerror = () => {
                    // Keep pin button when showing error
                    const pinBtn = previewWrapper.querySelector('.pin-btn');
                    previewWrapper.innerHTML = '';
                    resetPreviewFit(previewWrapper);
                    if (pinBtn) previewWrapper.appendChild(pinBtn);
                    previewWrapper.appendChild(buildNoImageSVG());
                };
                previewWrapper.appendChild(img);
            }
        } else {
            previewWrapper.appendChild(buildNoImageSVG());
        }
    }

    // Copy: the icon next to the name and the entry in the ⋯ menu.
    const copyBtn = card.querySelector('.copy-url-btn');
    if (copyBtn) {
        const newCopyBtn = copyBtn.cloneNode(true);
        copyBtn.parentNode.replaceChild(newCopyBtn, copyBtn);
        bindCopyButton(newCopyBtn, card, false);
    }
    const menuCopyBtn = card.querySelector('.more-copy-btn');
    if (menuCopyBtn) bindCopyButton(menuCopyBtn, card, true);
}


// The menu closes on copy, so a closing menu would hide its own "Copied!"
// feedback — that entry reports through a toast instead.
function bindCopyButton(btn, card, isMenuEntry) {
    const copyText = btn.querySelector('.copy-text');
    if (copyText) copyText.textContent = t('copy_url', 'Copy URL');

    let resetTimer = null;

    btn.onclick = (e) => {
        e.preventDefault();
        // Read the URL at click time: access level changes and token
        // rotation update it after this handler was bound.
        copyToClipboard(card.dataset.publicUrl || '').then(() => {
            if (isMenuEntry) {
                showToast(t('copied', 'Copied!'), 'success');
                return;
            }
            if (resetTimer) clearTimeout(resetTimer);

            btn.classList.add('copied');
            if (copyText) copyText.textContent = t('copied', 'Copied!');
            btn.setAttribute('aria-label', t('copied', 'Copied!'));

            resetTimer = setTimeout(() => {
                btn.classList.add('fading-out');
                resetTimer = setTimeout(() => {
                    btn.classList.remove('copied', 'fading-out');
                    if (copyText) copyText.textContent = t('copy_url', 'Copy URL');
                    btn.setAttribute('aria-label', t('copy_url', 'Copy URL'));
                }, 300);
            }, 1500);
        }).catch(() => {
            showToast(t('copy_error', 'Failed to copy URL'), 'error');
        });
    };
}


// Build the SVG no-image placeholder programmatically (same shape as in HTML template)
function buildNoImageSVG() {
    const wrap = document.createElement('div');
    wrap.className = 'no-image';
    wrap.innerHTML = `<svg class="no-image-icon" viewBox="0 0 64 64" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <path d="M8 48 L24 24 L36 38 L44 28 L56 48 Z" stroke="currentColor" stroke-width="2" stroke-linejoin="round" fill="none"/>
      <circle cx="46" cy="18" r="5" stroke="currentColor" stroke-width="2" fill="none"/>
      <rect x="6" y="8" width="52" height="44" rx="4" stroke="currentColor" stroke-width="2" fill="none"/>
    </svg>`;
    return wrap;
}


function setupCardEvents(card, link) {
    const fileInput = card.querySelector('.file-input');
    const dropdown = card.querySelector('.upload-dropdown');
    const toggleBtn = card.querySelector('.upload-toggle-btn');

    // Outside clicks are handled by the single delegated dropdown closer.

    // Inline rename
    setupInlineRename(card, link);

    toggleBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        const isOpen = dropdown.classList.contains('open');
        if (!isOpen) closeAllDropdowns(dropdown);
        dropdown.classList.toggle('open', !isOpen);
        toggleBtn.setAttribute('aria-expanded', String(!isOpen));
    });

    card.querySelector('.upload-file-btn').addEventListener('click', () => {
        dropdown.classList.remove('open');
        toggleBtn.setAttribute('aria-expanded', 'false');
        fileInput.click();
    });

    fileInput.onchange = async () => {
        if (!fileInput.files.length) return;
        await handleUpload(link, fileInput.files[0]);
        fileInput.value = '';
    };

    card.querySelector('.paste-url-btn').addEventListener('click', async () => {
        dropdown.classList.remove('open');
        toggleBtn.setAttribute('aria-expanded', 'false');
        const url = await showModal('input', 'enter_image_url_title', 'url_placeholder');
        if (url) await handleUpload(link, url, true);
    });

    card.querySelector('.select-server-btn').addEventListener('click', async () => {
        dropdown.classList.remove('open');
        toggleBtn.setAttribute('aria-expanded', 'false');
        const filename = await showModal('grid', 'select_server_title');
        if (filename) await handleUpload(link, filename, true);
    });

    // Playlist: a second file behind the same URL. It needs media to add to,
    // which the server also enforces.
    const appendBtn = card.querySelector('.append-file-btn');
    const appendInput = card.querySelector('.append-file-input');
    if (appendBtn && appendInput) {
        appendBtn.addEventListener('click', () => {
            dropdown.classList.remove('open');
            toggleBtn.setAttribute('aria-expanded', 'false');
            if (!link.hasImage) {
                showToast(t('append_needs_media', 'Upload a file first — there is nothing to add to yet.'), 'error');
                return;
            }
            appendInput.click();
        });
        appendInput.onchange = async () => {
            if (!appendInput.files.length) return;
            await handleUpload(link, appendInput.files[0], false, 'append');
            appendInput.value = '';
        };
    }

    const versionsBtn = card.querySelector('.versions-btn');
    if (versionsBtn) {
        versionsBtn.addEventListener('click', () => {
            dropdown.classList.remove('open');
            toggleBtn.setAttribute('aria-expanded', 'false');
            openVersions(link);
        });
    }

    // ⋯ menu: everything that is not "change the file". Secondary actions
    // stay one click away instead of sitting on top of the preview.
    const moreDropdown = card.querySelector('.more-dropdown');
    const moreToggle = card.querySelector('.more-toggle-btn');
    const closeMore = () => {
        if (!moreDropdown) return;
        moreDropdown.classList.remove('open');
        if (moreToggle) moreToggle.setAttribute('aria-expanded', 'false');
    };

    if (moreDropdown && moreToggle) {
        moreToggle.addEventListener('click', (e) => {
            e.stopPropagation();
            const isOpen = moreDropdown.classList.contains('open');
            if (!isOpen) closeAllDropdowns(moreDropdown);
            moreDropdown.classList.toggle('open', !isOpen);
            moreToggle.setAttribute('aria-expanded', String(!isOpen));
        });

        const menuVersions = card.querySelector('.more-versions-btn');
        if (menuVersions) menuVersions.addEventListener('click', () => {
            closeMore();
            openVersions(link);
        });

        const menuOpenLink = card.querySelector('.more-open-btn');
        if (menuOpenLink) menuOpenLink.addEventListener('click', closeMore);

        const menuRename = card.querySelector('.more-rename-btn');
        if (menuRename) menuRename.addEventListener('click', () => {
            closeMore();
            if (typeof card._startRename === 'function') card._startRename();
        });

        const menuPin = card.querySelector('.more-pin-btn');
        if (menuPin) menuPin.addEventListener('click', () => {
            closeMore();
            togglePin(link);
        });
    }

    card.ondragover = e => { e.preventDefault(); card.classList.add('drag-over'); };
    card.ondragleave = () => card.classList.remove('drag-over');
    card.ondrop = async e => {
        e.preventDefault();
        card.classList.remove('drag-over');
        if (e.dataTransfer.files.length) await handleUpload(link, e.dataTransfer.files[0]);
    };

    card.querySelector('.delete-btn').onclick = async () => {
        if (typeof closeMore === 'function') closeMore();
        const msg = t('confirm_delete_msg', 'Delete "{{name}}"? This cannot be undone.')
            .replace('{{name}}', link.linkName);
        const confirmed = await showConfirm(msg);
        if (!confirmed) return;

        // Add delete animation
        card.classList.add('deleting');

        // Wait for animation before API call
        await new Promise(resolve => setTimeout(resolve, 350));

        try {
            await apiCall(`/api/link/${encodeURIComponent(link.linkName)}`, 'DELETE');
            
            // Remove from state
            STATE.wallpapers = STATE.wallpapers.filter(wp => wp.linkName !== link.linkName);
            STATE.filteredWallpapers = STATE.filteredWallpapers.filter(wp => wp.linkName !== link.linkName);
            
            // Update stats without re-rendering
            updateSearchStats();
            
            // Remove card from DOM
            card.remove();
            previewVideos.sweep();
            
            // Show empty state if needed
            if (!DOM.linksList.children.length) {
                DOM.emptyState.classList.remove('d-none');
            }
            
            showToast(t('deleted_success', 'Link deleted'), 'success');
        } catch (_) {
            // Remove animation class on error
            card.classList.remove('deleting');
        }
    };
}


async function handleUpload(link, fileOrUrl, isUrl = false, mode = 'replace') {
    const formData = new FormData();
    formData.append('linkName', link.linkName);
    // "append" adds a playlist item behind the same URL and leaves the live
    // file — and therefore every existing embed — untouched.
    const isAppend = mode === 'append';
    if (isAppend) formData.append('mode', 'append');

    if (isUrl) {
        formData.append('url', fileOrUrl);
    } else {
        if (!fileOrUrl.type.startsWith('image/') && !fileOrUrl.type.startsWith('video/')) {
            showToast(t('invalid_image', 'Invalid file format'), 'error');
            return;
        }
        let fileToUpload = fileOrUrl;
        if (STATE.compressor && fileOrUrl.type.startsWith('image/')) {
            const originalSize = fileOrUrl.size;
            try {
                fileToUpload = await STATE.compressor.compress(fileOrUrl);
                if (fileToUpload.size < originalSize) {
                    const info = ImageCompressor.getCompressionInfo(originalSize, fileToUpload.size);
                    const msg = t('compression_saved', 'Compressed: {{percent}}% smaller ({{saved}} saved)')
                        .replace('{{percent}}', info.percent)
                        .replace('{{saved}}', formatSize(info.saved));
                    showToast(msg, 'success');
                }
            } catch (_) {
                fileToUpload = fileOrUrl;
            }
        }
        formData.append('file', fileToUpload);
    }

    try {
        const updatedLink = await apiCall('/api/upload', 'POST', formData, true);
        if (!updatedLink) return;
        if (!updatedLink.createdAt && link.createdAt) updatedLink.createdAt = link.createdAt;
        const idx = STATE.wallpapers.findIndex(wp => wp.linkName === updatedLink.linkName);
        if (idx !== -1) STATE.wallpapers[idx] = updatedLink;
        else STATE.wallpapers.push(updatedLink);
        // Re-rendering replaces the card with one built from the new data.
        filterAndSort();
        showToast(isAppend
            ? t('append_success', 'Added to playlist')
            : t('upload_success', 'Uploaded!'), 'success');
    } catch (_) {}
}


// VERSIONS & PLAYLIST
// One dialog per link: archived versions (restore/delete), playlist items
// (open/remove), rotation settings and the in-memory access counters.
let versionsState = null;

// mediaSelectorURL keeps the token of a protected link and adds ?v= or ?i=.
function mediaSelectorURL(link, selector) {
    const base = publicLinkURL(link);
    if (!selector) return base;
    return base + (base.includes('?') ? '&' : '?') + selector;
}

// applyLinkUpdate stores a record returned by the API and refreshes only its
// card, so an open dialog survives the change.
function applyLinkUpdate(updated) {
    if (!updated || !updated.linkName) return null;
    const idx = STATE.wallpapers.findIndex(wp => wp.linkName === updated.linkName);
    if (idx !== -1) STATE.wallpapers[idx] = updated;
    else STATE.wallpapers.push(updated);
    if (versionsState && versionsState.link.linkName === updated.linkName) {
        versionsState.link = updated;
    }
    const card = DOM.linksList.querySelector(`[data-link-name="${CSS.escape(updated.linkName)}"]`);
    if (card) updateCard(card, updated);
    return updated;
}

function versionsMessage(text, variant = '') {
    const el = document.createElement('p');
    el.className = `versions-msg ${variant}`.trim();
    el.textContent = text;
    return el;
}

function versionsRow(kind, title, meta) {
    const row = document.createElement('div');
    row.className = `versions-row versions-row--${kind}`;

    const info = document.createElement('div');
    info.className = 'versions-row-info';
    const titleEl = document.createElement('span');
    titleEl.className = 'versions-row-title';
    titleEl.textContent = title;
    info.appendChild(titleEl);
    if (meta) {
        const metaEl = document.createElement('span');
        metaEl.className = 'versions-row-meta';
        metaEl.textContent = meta;
        info.appendChild(metaEl);
    }
    row.appendChild(info);

    const actions = document.createElement('div');
    actions.className = 'versions-row-actions';
    row.appendChild(actions);
    return { row, actions };
}

function versionsButton(label, variant, onClick) {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = `btn btn--small ${variant}`.trim();
    btn.textContent = label;
    btn.addEventListener('click', onClick);
    return btn;
}

function versionsOpenLink(label, href) {
    const a = document.createElement('a');
    a.className = 'btn btn--small';
    a.href = href;
    a.target = '_blank';
    a.rel = 'noopener noreferrer';
    a.textContent = label;
    return a;
}

function entryMeta(entry) {
    return [
        entry.ext ? entry.ext.toUpperCase() : '',
        entry.sizeBytes ? formatSize(entry.sizeBytes) : '',
        formatDate(entry.savedAt || entry.modTime || 0),
    ].filter(Boolean).join(' · ');
}

async function openVersions(link) {
    versionsState = { link };
    DOM.versionsLinkName.textContent = link.linkName;
    openDialog(DOM.versionsOverlay);
    applyTranslations(DOM.versionsOverlay);
    focusAfterPaint(DOM.versionsClose);
    await refreshVersions();
}

function closeVersions() {
    closeDialog(DOM.versionsOverlay);
    versionsState = null;
}

async function refreshVersions() {
    if (!versionsState) return;
    const link = versionsState.link;
    const name = encodeURIComponent(link.linkName);

    let data = null;
    try {
        data = await apiCall(`/api/link/${name}/history`, 'GET');
    } catch (_) {
        DOM.versionsList.replaceChildren(versionsMessage(
            t('load_error_hint', 'Failed to load — check the connection and try again.'), 'error'));
        DOM.playlistList.replaceChildren();
        return;
    }
    if (!data) return;

    // --- Version history -------------------------------------------------
    const rows = [];
    if (link.hasImage) {
        const live = data.live || {};
        const { row, actions } = versionsRow('live',
            `${t('versions_current', 'Current')} · v${data.currentVersion}`, entryMeta(live));
        actions.appendChild(versionsOpenLink(t('versions_open', 'Open'), mediaSelectorURL(link, '')));
        rows.push(row);
    }
    const history = Array.isArray(data.history) ? data.history : [];
    if (!history.length) {
        rows.push(versionsMessage(data.limit > 0
            ? t('versions_empty', 'No archived versions yet. Replacing the file keeps the previous one here.')
            : t('versions_disabled', 'Version history is disabled on this server (HISTORY_LIMIT=0).')));
    }
    for (const entry of history) {
        const { row, actions } = versionsRow('archived', `v${entry.version}`, entryMeta(entry));
        actions.appendChild(versionsOpenLink(t('versions_open', 'Open'),
            mediaSelectorURL(link, `v=${entry.version}`)));
        actions.appendChild(versionsButton(t('versions_restore', 'Restore'), 'btn--primary',
            () => restoreVersion(link, entry.version)));
        actions.appendChild(versionsButton(t('delete_btn', 'Delete'), 'btn--danger',
            () => deleteVersion(link, entry.version)));
        rows.push(row);
    }
    DOM.versionsList.replaceChildren(...rows);
    DOM.versionsHint.textContent = data.limit > 0
        ? t('versions_hint', 'Up to {{limit}} archived versions per link · {{size}} used')
            .replace('{{limit}}', data.limit).replace('{{size}}', formatSize(data.bytes || 0))
        : '';

    // --- Playlist --------------------------------------------------------
    const items = Array.isArray(link.items) ? link.items : [];
    const itemRows = [];
    if (!items.length) {
        itemRows.push(versionsMessage(
            t('playlist_empty', 'No playlist items yet. Use Upload → Add to playlist.')));
    }
    for (const item of items) {
        const { row, actions } = versionsRow('item', `#${item.id}`, entryMeta(item));
        actions.appendChild(versionsOpenLink(t('versions_open', 'Open'),
            mediaSelectorURL(link, `i=${item.id}`)));
        actions.appendChild(versionsButton(t('playlist_remove', 'Remove'), 'btn--danger',
            () => removePlaylistItem(link, item.id)));
        itemRows.push(row);
    }
    itemRows.push(versionsMessage(
        t('playlist_count', '{{count}} items · the live file is always position 0')
            .replace('{{count}}', items.length)));
    DOM.playlistList.replaceChildren(...itemRows);

    // Rotation needs something to rotate to.
    const rotate = link.rotate || {};
    const hasItems = items.length > 0;
    DOM.rotateEnabled.checked = !!rotate.enabled;
    DOM.rotateInterval.value = rotate.interval || 60;
    DOM.rotateOrder.value = rotate.order === 'random' ? 'random' : 'sequential';
    for (const el of [DOM.rotateEnabled, DOM.rotateInterval, DOM.rotateOrder, DOM.rotateSave]) {
        if (el) el.disabled = !hasItems;
    }

    // --- Statistics ------------------------------------------------------
    const stats = link.stats;
    DOM.statsBody.textContent = stats && stats.hits
        ? t('stats_hits', '{{hits}} requests · {{bytes}} transferred · last {{when}}')
            .replace('{{hits}}', stats.hits)
            .replace('{{bytes}}', formatSize(stats.bytes || 0))
            .replace('{{when}}', new Date((stats.lastHit || 0) * 1000).toLocaleString())
        : t('stats_none', 'Not requested since the server started. Counters are kept in memory and reset on restart.');
}

async function restoreVersion(link, version) {
    const msg = t('confirm_restore_msg', 'Restore version {{version}} of "{{name}}"? The file it replaces is archived.')
        .replace('{{version}}', version).replace('{{name}}', link.linkName);
    if (!await showConfirm(msg)) return;
    try {
        const updated = await apiCall(
            `/api/link/${encodeURIComponent(link.linkName)}/rollback`, 'POST', { version });
        applyLinkUpdate(updated);
        showToast(t('restored_success', 'Version {{version}} restored').replace('{{version}}', version), 'success');
        await refreshVersions();
    } catch (_) {}
}

async function deleteVersion(link, version) {
    const msg = t('confirm_delete_version_msg', 'Delete archived version {{version}}? This cannot be undone.')
        .replace('{{version}}', version);
    if (!await showConfirm(msg)) return;
    try {
        const updated = await apiCall(
            `/api/link/${encodeURIComponent(link.linkName)}/history/${version}`, 'DELETE');
        applyLinkUpdate(updated);
        showToast(t('version_deleted', 'Version {{version}} deleted').replace('{{version}}', version), 'success');
        await refreshVersions();
    } catch (_) {}
}

async function removePlaylistItem(link, id) {
    const msg = t('confirm_remove_item_msg', 'Remove playlist item #{{id}}? This cannot be undone.')
        .replace('{{id}}', id);
    if (!await showConfirm(msg)) return;
    try {
        const updated = await apiCall(`/api/link/${encodeURIComponent(link.linkName)}`, 'PATCH', { removeItem: id });
        applyLinkUpdate(updated);
        showToast(t('item_removed', 'Playlist item removed'), 'success');
        await refreshVersions();
    } catch (_) {}
}

async function saveRotation(link) {
    const enabled = DOM.rotateEnabled.checked;
    const raw = parseInt(DOM.rotateInterval.value, 10);
    // 0 restores the server default; anything else must be inside the bounds.
    const interval = Number.isFinite(raw) && raw > 0 ? raw : 0;
    if (enabled && interval !== 0 && (interval < 5 || interval > 86400)) {
        showToast(t('rotate_interval_invalid', 'The interval must be between 5 and 86400 seconds.'), 'error');
        return;
    }
    try {
        const updated = await apiCall(`/api/link/${encodeURIComponent(link.linkName)}`, 'PATCH', {
            rotate: { enabled, interval, order: DOM.rotateOrder.value }
        });
        applyLinkUpdate(updated);
        showToast(t('rotate_saved', 'Rotation updated'), 'success');
        await refreshVersions();
    } catch (_) {}
}

function setupGlobalListeners() {
    DOM.createForm.addEventListener('submit', async (e) => {
        e.preventDefault();
        if (STATE.createPending) return;
        const id = DOM.createInput.value.trim();
        if (!id) { showToast(t('invalid_id', 'ID is required'), 'error'); return; }
        if (!VALID_LINK_RE.test(id)) {
            showToast(t('invalid_id_chars', 'Invalid ID format'), 'error');
            return;
        }
        STATE.createPending = true;
        const btn = DOM.createForm.querySelector('[type="submit"]');
        if (btn) {
            btn.disabled = true;
            btn.classList.add('is-busy');
            btn.setAttribute('aria-busy', 'true');
        }
        try {
            const created = await apiCall('/api/link', 'POST', { linkName: id });
            DOM.createInput.value = '';
            // Return focus to input so user can create next link immediately
            DOM.createInput.focus();
            STATE.wallpapers.push(created || {
                linkName: id,
                hasImage: false,
                createdAt: Math.floor(Date.now() / 1000),
                pinned: false,
                accessLevel: 'public',
            });
            filterAndSort();
            const newCard = DOM.linksList.querySelector(`[data-link-name="${CSS.escape(id)}"]`)
                ?? DOM.linksList.lastElementChild;
            if (newCard && typeof newCard.animate === 'function') {
                newCard.animate([
                    { opacity: 0, transform: 'translateY(10px)' },
                    { opacity: 1, transform: 'translateY(0)' }
                ], { duration: 300 });
            }
            showToast(t('created_success', 'Link created'), 'success');
        } catch (_) {}
        finally {
            STATE.createPending = false;
            if (btn) {
                btn.disabled = false;
                btn.classList.remove('is-busy');
                btn.removeAttribute('aria-busy');
            }
        }
    });

    DOM.modalOverlay.onclick = (e) => {
        if (e.target === DOM.modalOverlay) closeModal();
    };

    DOM.modalOverlay.addEventListener('keydown', (e) => {
        if (e.key === 'Tab' && !DOM.modalOverlay.classList.contains('hidden')) trapFocus(DOM.modalOverlay, e);
    });

    DOM.confirmCancel.onclick = () => closeConfirm(false);
    DOM.confirmDelete.onclick = () => closeConfirm(true);
    DOM.confirmOverlay.onclick = (e) => {
        if (e.target === DOM.confirmOverlay) closeConfirm(false);
    };

    DOM.confirmOverlay.addEventListener('keydown', (e) => {
        if (DOM.confirmOverlay.classList.contains('hidden')) return;
        if (e.key === 'Enter') {
            e.preventDefault();
            closeConfirm(true); // Confirm deletion on Enter
        } else if (e.key === 'Tab') {
            trapFocus(DOM.confirmOverlay, e);
        }
    });

    if (DOM.versionsOverlay) {
        DOM.versionsClose.onclick = closeVersions;
        DOM.versionsOverlay.onclick = (e) => {
            if (e.target === DOM.versionsOverlay) closeVersions();
        };
        DOM.versionsOverlay.addEventListener('keydown', (e) => {
            if (DOM.versionsOverlay.classList.contains('hidden')) return;
            // Escape must work while the rotation interval input has focus, which
            // the document-level shortcut handler ignores on purpose.
            if (e.key === 'Escape') {
                e.stopPropagation();
                closeVersions();
                return;
            }
            if (e.key === 'Tab') trapFocus(DOM.versionsOverlay, e);
        });
        DOM.rotateSave.onclick = () => {
            if (versionsState) saveRotation(versionsState.link);
        };
    }

    const regenBtn = document.getElementById('regenPreviewsBtn');
    if (regenBtn) {
        regenBtn.addEventListener('click', async () => {
            regenBtn.disabled = true;
            regenBtn.classList.add('is-spinning');
            regenBtn.setAttribute('aria-busy', 'true');
            const spanEl = regenBtn.querySelector('span');
            const origText = spanEl?.textContent;
            if (spanEl) spanEl.textContent = t('regen_previews_running', 'Regenerating...');
            try {
                const result = await apiCall('/api/regenerate-previews', 'POST');
                if (!result) return;
                showToast(
                    t('regen_previews_done', 'Done: {{ok}} ok, {{errors}} errors, {{skipped}} skipped')
                        .replace('{{ok}}', result.ok)
                        .replace('{{errors}}', result.errors)
                        .replace('{{skipped}}', result.skipped),
                    result.errors > 0 ? 'info' : 'success'
                );
                await loadLinks();
            } catch (_) {}
            finally {
                regenBtn.disabled = false;
                regenBtn.classList.remove('is-spinning');
                regenBtn.removeAttribute('aria-busy');
                if (spanEl && origText) spanEl.textContent = origText;
            }
        });
    }
}


// UTILS
function formatSize(bytes) {
    if (!bytes || bytes < 0) return '0 KB';
    if (bytes < 1024 * 1024) {
        const kb = bytes / 1024;
        return `${kb < 10 ? kb.toFixed(1) : Math.round(kb)} KB`;
    }
    if (bytes < 1024 * 1024 * 1024) {
        const mb = bytes / (1024 * 1024);
        return `${mb < 10 ? mb.toFixed(1) : Math.round(mb)} MB`;
    }
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function formatDate(ts) {
    if (!ts) return '—';
    // The date follows the interface language, not the browser locale: a
    // German panel should read 03.10.2026 even on an English system.
    try {
        return new Date(ts * 1000).toLocaleDateString(STATE.lang || undefined);
    } catch (_) {
        return new Date(ts * 1000).toLocaleDateString();
    }
}
