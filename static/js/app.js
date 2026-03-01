/**
 * Lanpaper Frontend Logic
 * Handles UI interactions, API calls, and state management.
 */


// STATE & CONFIG
const STATE = {
    translations: {},
    lang: localStorage.getItem('lang') || navigator.language.slice(0, 2) || 'en',
    isDark: false,
    viewMode: localStorage.getItem('viewMode') || 'list',
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

    createInput: document.getElementById('newLinkId'),
    createForm: document.getElementById('createForm'),

    template: document.getElementById('linkCardTemplate'),
};

const log = (...args) => STATE.isDebug && console.log(...args);

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
    document.querySelectorAll('.custom-select.open').forEach(select => {
        if (select !== exceptElement) {
            select.classList.remove('open');
            const btn = select.querySelector('.custom-select-btn');
            if (btn) btn.setAttribute('aria-expanded', 'false');
        }
    });
};


// INITIALIZATION
document.addEventListener('DOMContentLoaded', async () => {
    initTheme();
    await initLanguage();
    initView();
    initSearchSort();
    initLazyLoading();
    initKeyboardShortcuts();
    initPWA();
    await loadCompressionConfig();
    initCompression();
    loadAppVersion();
    showSkeletons();
    await loadLinks();
    setupGlobalListeners();
});


function initPWA() {
    if ('serviceWorker' in navigator) {
        navigator.serviceWorker.register('/static/sw.js').catch(() => {});
    }
}


async function loadCompressionConfig() {
    try {
        const res = await fetch('/api/compression-config');
        if (res.ok) STATE.compressionConfig = await res.json();
    } catch (_) {}
}


function initCompression() {
    if (typeof ImageCompressor === 'undefined') return;

    const quality = STATE.compressionConfig?.quality ?? 85;
    const scale = STATE.compressionConfig?.scale ?? 100;

    const maxWidth = Math.floor((1920 * scale) / 100);
    const maxHeight = Math.floor((1080 * scale) / 100);

    STATE.compressor = new ImageCompressor({ maxWidth, maxHeight, quality: quality / 100 });
    log(`[Compression] ${quality}% quality, ${scale}% scale (${maxWidth}x${maxHeight})`);
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
        if (e.target.matches('input, textarea')) return;

        const keyMap = {
            'n': () => (e.ctrlKey || e.metaKey) && DOM.createInput.focus(),
            'f': () => {
                if (e.ctrlKey || e.metaKey) {
                    DOM.searchInput.focus();
                    DOM.searchInput.select();
                }
            },
            'g': () => (e.ctrlKey || e.metaKey) && DOM.viewBtn.click(),
            'Escape': () => {
                if (!DOM.confirmOverlay.classList.contains('hidden')) {
                    closeConfirm();
                } else if (!DOM.modalOverlay.classList.contains('hidden')) {
                    closeModal();
                } else if (DOM.searchInput.value) {
                    DOM.searchInput.value = '';
                    DOM.searchInput.dispatchEvent(new Event('input'));
                }
            },
            't': () => DOM.themeBtn.click(),
            'T': () => DOM.themeBtn.click(),
        };

        const handler = keyMap[e.key];
        if (handler) {
            e.preventDefault();
            handler();
        }
    });

    if (!localStorage.getItem('shortcuts-seen')) {
        setTimeout(() => {
            showToast(`💡 ${t('shortcuts_hint', 'Shortcuts: Ctrl+N (new), Ctrl+F (search), Ctrl+G (view), T (theme)')}`, 'success');
            localStorage.setItem('shortcuts-seen', 'true');
        }, 2000);
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

    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', e => {
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
    document.body.classList.toggle('dark', STATE.isDark);

    document.querySelectorAll('meta[name="theme-color"]').forEach(meta => {
        const media = meta.getAttribute('media') || '';
        if (media.includes('dark')) {
            meta.content = STATE.isDark ? '#1c1c20' : '#1c1c20';
        } else {
            meta.content = STATE.isDark ? '#1c1c20' : '#ffffff';
        }
    });

    const logo = document.querySelector('.logo');
    if (logo) logo.src = STATE.isDark ? '/static/logo-dark.svg' : '/static/logo.svg';

    const icons = DOM.themeBtn.querySelectorAll('.theme-icon');
    icons.forEach(icon => icon.classList.remove('active'));

    const activeIcon = STATE.isDark
        ? DOM.themeBtn.querySelector('img[alt="Light"]')
        : DOM.themeBtn.querySelector('img[alt="Dark"]');
    if (activeIcon) activeIcon.classList.add('active');

    document.documentElement.setAttribute('data-theme', STATE.isDark ? 'dark' : 'light');
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
}


function t(key, defaultText) {
    return STATE.translations[key] || defaultText;
}


function updateAriaLabels() {
    document.querySelectorAll('[data-i18n-aria]').forEach(el => {
        const key = el.dataset.i18nAria;
        if (STATE.translations[key]) el.setAttribute('aria-label', STATE.translations[key]);
    });
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
        opt.addEventListener('click', () => {
            const val = opt.dataset.value;
            options.forEach(o => o.classList.remove('selected'));
            opt.classList.add('selected');
            if (label) label.textContent = opt.textContent;
            customSelect.classList.remove('open');
            btn.setAttribute('aria-expanded', 'false');
            if (DOM.sortSelect) DOM.sortSelect.value = val;
            STATE.sortBy = val;
            localStorage.setItem('sortBy', val);
            filterAndSort();
        });
    });

    document.addEventListener('click', (e) => {
        if (!customSelect.contains(e.target)) {
            customSelect.classList.remove('open');
            btn.setAttribute('aria-expanded', 'false');
        }
    });

    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && customSelect.classList.contains('open')) {
            customSelect.classList.remove('open');
            btn.setAttribute('aria-expanded', 'false');
        }
    });
}


function syncCustomSelectLabels() {
    const customSelect = document.getElementById('customSortSelect');
    if (!customSelect) return;

    const label = document.getElementById('customSortLabel');
    const options = customSelect.querySelectorAll('.custom-select-option');

    options.forEach(opt => {
        const i18nKey = opt.dataset.i18n;
        if (i18nKey && STATE.translations[i18nKey]) opt.textContent = STATE.translations[i18nKey];
        const isSelected = opt.dataset.value === STATE.sortBy;
        opt.classList.toggle('selected', isSelected);
        if (isSelected && label) label.textContent = opt.textContent;
    });
}


function filterWallpapers() {
    if (!STATE.searchQuery) {
        STATE.filteredWallpapers = [...STATE.wallpapers];
        return;
    }
    const query = STATE.searchQuery;
    STATE.filteredWallpapers = STATE.wallpapers.filter(wp => {
        const name = (wp.linkName || wp.id || '').toLowerCase();
        return name.includes(query);
    });
}


function sortWallpapers(list) {
    const sorted = [...list];
    const sortFns = {
        name_asc:  (a, b) => (a.linkName || '').localeCompare(b.linkName || ''),
        name_desc: (a, b) => (b.linkName || '').localeCompare(a.linkName || ''),
        date_desc: (a, b) => (b.createdAt || 0) - (a.createdAt || 0),
        date_asc:  (a, b) => (a.createdAt || 0) - (b.createdAt || 0),
    };
    const sortFn = sortFns[STATE.sortBy];
    if (sortFn) sorted.sort(sortFn);
    return sorted;
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
        DOM.searchStats.style.cursor = 'pointer';
    } else {
        const tpl = t('search_total', 'Total: {{total}}');
        DOM.searchStats.textContent = tpl.replace('{{total}}', total);
        DOM.searchStats.title = '';
        DOM.searchStats.style.cursor = 'default';
    }
}


// TOASTS
const TOAST_ICONS = {
    success: `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>`,
    error:   `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>`,
    info:    `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>`,
};

function showToast(message, type = 'success') {
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    const icon = TOAST_ICONS[type] || TOAST_ICONS.info;
    toast.innerHTML = `
        <div class="toast-icon">${icon}</div>
        <div class="toast-content">${message}</div>
    `;
    DOM.toastContainer.appendChild(toast);
    setTimeout(() => {
        toast.classList.add('hiding');
        setTimeout(() => toast.remove(), 400);
    }, 3000);
}


// CONFIRM MODAL
let confirmResolve = null;

function showConfirm(message) {
    return new Promise((resolve) => {
        confirmResolve = resolve;
        if (DOM.confirmMessage) DOM.confirmMessage.textContent = message;
        DOM.confirmOverlay.classList.remove('hidden');
        DOM.confirmOverlay.setAttribute('aria-hidden', 'false');
        DOM.confirmDelete.focus();
    });
}

function closeConfirm(result = false) {
    DOM.confirmOverlay.classList.add('hidden');
    DOM.confirmOverlay.setAttribute('aria-hidden', 'true');
    if (confirmResolve) confirmResolve(result);
    confirmResolve = null;
}


// MODAL
let modalResolve = null;

function showModal(type, titleKey, placeholderKey = '') {
    return new Promise((resolve) => {
        modalResolve = resolve;
        DOM.modalTitle.textContent = t(titleKey, t('modal_default_title', 'Input'));
        DOM.modalOverlay.classList.remove('hidden');
        DOM.modalOverlay.setAttribute('aria-hidden', 'false');
        DOM.modalInput.value = '';
        DOM.modalInput.classList.add('d-none');
        DOM.modalList.innerHTML = '';
        DOM.modalList.classList.add('hidden');
        DOM.modalConfirm.onclick = null;

        if (type === 'input') {
            DOM.modalInput.classList.remove('d-none');
            DOM.modalInput.placeholder = placeholderKey ? t(placeholderKey, 'https://...') : t('url_placeholder', 'https://...');
            DOM.modalInput.focus();
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
    DOM.modalOverlay.classList.add('hidden');
    DOM.modalOverlay.setAttribute('aria-hidden', 'true');
    if (modalResolve) modalResolve(null);
    modalResolve = null;
}


function confirmModal() {
    let result = !DOM.modalInput.classList.contains('d-none')
        ? DOM.modalInput.value.trim()
        : DOM.modalList.querySelector('.selected')?.dataset.value;

    if (result) {
        DOM.modalOverlay.classList.add('hidden');
        if (modalResolve) modalResolve(result);
        modalResolve = null;
    } else {
        DOM.modalInput.classList.add('shake');
        setTimeout(() => DOM.modalInput.classList.remove('shake'), 300);
    }
}


async function loadExternalImages() {
    DOM.modalList.innerHTML = `<div class="modal-list-msg">${t('loading', 'Loading...')}</div>`;
    try {
        const res = await fetch('/api/external-images');
        if (!res.ok) throw new Error('Failed');
        const files = await res.json();

        if (!files?.length) {
            DOM.modalList.innerHTML = `<div class="modal-list-msg muted">${t('server_empty', 'No images found')}</div>`;
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
            const img = document.createElement('img');
            img.alt = file;
            img.className = 'lazy-image-fade';
            if (STATE.lazyObserver) {
                img.dataset.src = previewUrl;
                img.src = 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"%3E%3C/svg%3E';
                STATE.lazyObserver.observe(img);
                img.addEventListener('load', () => img.classList.add('loaded'));
            } else {
                img.src = previewUrl;
                img.classList.add('loaded');
            }
            div.appendChild(img);
            div.appendChild(nameEl);
            div.onclick = () => {
                DOM.modalList.querySelectorAll('.image-option').forEach(el => el.classList.remove('selected'));
                div.classList.add('selected');
            };
            frag.appendChild(div);
        });
        DOM.modalList.appendChild(frag);
    } catch (_) {
        DOM.modalList.innerHTML = `<div class="modal-list-msg error">${t('server_error', 'Error loading images')}</div>`;
    }
}


// API
async function apiCall(url, method = 'GET', body = null, isFormData = false) {
    const options = {
        method,
        headers: isFormData ? {} : { 'Content-Type': 'application/json' }
    };
    if (body) options.body = isFormData ? body : JSON.stringify(body);
    try {
        const res = await fetch(url, options);
        if (!res.ok) {
            const text = await res.text();
            throw new Error(text || `HTTP ${res.status}`);
        }
        const contentType = res.headers.get('content-type');
        return contentType?.includes('application/json') ? res.json() : null;
    } catch (e) {
        showToast(e.message, 'error');
        throw e;
    }
}


async function loadLinks() {
    try {
        STATE.wallpapers = await apiCall('/api/wallpapers') || [];
        filterAndSort();
    } catch (_) {
        showToast(t('load_error', 'Failed to load links'), 'error');
        STATE.wallpapers = [];
        renderLinks([]);
    }
}


function renderLinks(wallpapers) {
    DOM.linksList.innerHTML = '';
    if (!wallpapers?.length) {
        DOM.emptyState.classList.remove('d-none');
        return;
    }
    DOM.emptyState.classList.add('d-none');

    const fragment = document.createDocumentFragment();
    wallpapers.forEach(link => {
        const clone = DOM.template.content.cloneNode(true);
        const article = clone.querySelector('article');
        updateCard(article, link);
        setupCardEvents(article, link);
        fragment.appendChild(article);
    });
    DOM.linksList.appendChild(fragment);
    applyTranslations(DOM.linksList);
    updateAriaLabels();
}


function detectCategory(link) {
    const mime = link.mimeType || '';
    if (mime === 'mp4' || mime === 'webm') return 'video';
    if (mime) return 'image';
    const ext = (link.imageUrl || '').split('.').pop().toLowerCase();
    if (ext === 'mp4' || ext === 'webm') return 'video';
    if (ext) return 'image';
    return 'other';
}


function createLazyImage(src, alt = 'Image', className = 'preview', errorMsg) {
    const img = document.createElement('img');
    if (STATE.lazyObserver) {
        img.dataset.src = src;
        img.src = 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"%3E%3C/svg%3E';
        STATE.lazyObserver.observe(img);
    } else {
        img.src = src;
    }
    img.alt = alt;
    img.className = className;
    img.loading = 'lazy';
    if (errorMsg) {
        img.onerror = () => { img.parentElement.innerHTML = `<div class="no-image">${errorMsg}</div>`; };
    }
    return img;
}


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

                // Sync state
                const idx = STATE.wallpapers.findIndex(wp => wp.linkName === currentName);
                if (idx !== -1) STATE.wallpapers[idx] = updated;
                link.linkName = updated.linkName;

                // Re-render copy button URL
                updateCard(card, updated);
                setupInlineRename(card, updated);

                showToast(t('renamed_success', `Renamed to "${updated.linkName}"`), 'success');
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
}


function updateCard(card, link) {
    const linkName = link.linkName || link.id;
    const linkIdEl = card.querySelector('.link-id');

    // Don't overwrite if currently in edit mode
    if (!linkIdEl.querySelector('input')) {
        linkIdEl.textContent = linkName;
    }
    card.dataset.linkName = linkName;

    const fullUrl = `${window.location.origin}/${linkName}`;

    const previewLink = card.querySelector('.preview-link');
    previewLink.href = fullUrl;
    previewLink.setAttribute('aria-label', t('aria_open_image', 'Open image'));
    linkIdEl.setAttribute('aria-label', t('aria_link_id', 'Link ID'));

    const category = link.hasImage ? detectCategory(link) : 'other';

    let fileType;
    if (link.mimeType) {
        fileType = link.mimeType.toUpperCase();
    } else if (link.hasImage) {
        const ext = (link.imageUrl || '').split('.').pop();
        fileType = ext ? ext.toUpperCase() : 'IMAGE';
    } else {
        fileType = t('no_image', 'No image');
    }

    const dateStr = link.createdAt ? formatDate(link.createdAt) : '\u2014';
    const sizeStr = link.sizeBytes ? ` \u00b7 ${formatKB(link.sizeBytes)}` : '';

    const linkMeta = card.querySelector('.link-meta');
    linkMeta.textContent = `${category} \u00b7 ${fileType}${sizeStr} \u00b7 ${dateStr}`;
    linkMeta.setAttribute('aria-label', t('aria_file_info', 'File info'));

    const previewWrapper = card.querySelector('.preview-wrapper');

    // Only re-build preview when it has actually changed (avoid video flicker)
    const prevSrc = previewWrapper.dataset.src || '';
    const newSrc  = link.hasImage ? (link.preview || link.imageUrl || '') : '';
    const srcChanged = prevSrc !== newSrc;

    if (srcChanged) {
        previewWrapper.dataset.src = newSrc;
        previewWrapper.innerHTML = '';

        if (link.hasImage) {
            const isVid = (category === 'video');
            if (isVid) {
                const videoSrc = '/' + (link.imageUrl || '').replace(/^\//, '') + `?t=${link.modTime || Date.now()}`;
                const video = document.createElement('video');
                video.src = videoSrc;
                video.className = 'preview';
                video.autoplay = true;
                video.muted = true;
                video.loop = true;
                video.playsInline = true;
                video.setAttribute('playsinline', '');
                video.setAttribute('preload', 'metadata');
                video.onerror = () => {
                    previewWrapper.innerHTML = '';
                    previewWrapper.appendChild(buildNoImageSVG());
                };
                previewWrapper.appendChild(video);
            } else {
                const resolvedPreview = link.preview || '';
                const imgSrc = resolvedPreview
                    ? '/' + resolvedPreview.replace(/^\//, '') + `?t=${link.modTime || Date.now()}`
                    : '/' + (link.imageUrl || '').replace(/^\//, '');
                const img = createLazyImage(
                    imgSrc,
                    resolvedPreview ? 'Preview' : 'Image',
                    'preview'
                );
                img.classList.add('preview-top-center');
                img.onerror = () => {
                    previewWrapper.innerHTML = '';
                    previewWrapper.appendChild(buildNoImageSVG());
                };
                previewWrapper.appendChild(img);
            }
        } else {
            previewWrapper.appendChild(buildNoImageSVG());
        }
    }

    // Copy button
    const copyBtn = card.querySelector('.copy-url-btn');
    const newCopyBtn = copyBtn.cloneNode(true);
    copyBtn.parentNode.replaceChild(newCopyBtn, copyBtn);

    const copyText = newCopyBtn.querySelector('.copy-text');
    if (copyText) copyText.textContent = t('copy_url', 'Copy URL');

    let copyResetTimer = null;

    newCopyBtn.onclick = (e) => {
        e.preventDefault();
        navigator.clipboard.writeText(fullUrl).then(() => {
            if (copyResetTimer) clearTimeout(copyResetTimer);

            newCopyBtn.classList.add('copied');
            if (copyText) copyText.textContent = t('copied', 'Copied!');
            newCopyBtn.setAttribute('aria-label', t('copied', 'Copied!'));

            copyResetTimer = setTimeout(() => {
                newCopyBtn.classList.add('fading-out');
                copyResetTimer = setTimeout(() => {
                    newCopyBtn.classList.remove('copied', 'fading-out');
                    if (copyText) copyText.textContent = t('copy_url', 'Copy URL');
                    newCopyBtn.setAttribute('aria-label', t('copy_url', 'Copy URL'));
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

    const ac = new AbortController();
    const { signal } = ac;

    new MutationObserver((_, obs) => {
        if (!document.contains(card)) { ac.abort(); obs.disconnect(); }
    }).observe(document.body, { childList: true, subtree: true });

    // Inline rename
    setupInlineRename(card, link);

    toggleBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        const isOpen = dropdown.classList.contains('open');
        if (!isOpen) closeAllDropdowns(dropdown);
        dropdown.classList.toggle('open', !isOpen);
        toggleBtn.setAttribute('aria-expanded', String(!isOpen));
    });

    document.addEventListener('click', (e) => {
        if (!dropdown.contains(e.target)) {
            dropdown.classList.remove('open');
            toggleBtn.setAttribute('aria-expanded', 'false');
        }
    }, { signal });

    card.querySelector('.upload-file-btn').addEventListener('click', () => {
        dropdown.classList.remove('open');
        toggleBtn.setAttribute('aria-expanded', 'false');
        fileInput.click();
    });

    fileInput.onchange = async () => {
        if (!fileInput.files.length) return;
        await handleUpload(link, fileInput.files[0], card);
        fileInput.value = '';
    };

    card.querySelector('.paste-url-btn').addEventListener('click', async () => {
        dropdown.classList.remove('open');
        toggleBtn.setAttribute('aria-expanded', 'false');
        const url = await showModal('input', 'enter_image_url_title', 'url_placeholder');
        if (url) await handleUpload(link, url, card, true);
    });

    card.querySelector('.select-server-btn').addEventListener('click', async () => {
        dropdown.classList.remove('open');
        toggleBtn.setAttribute('aria-enabled', 'false');
        const filename = await showModal('grid', 'select_server_title');
        if (filename) await handleUpload(link, filename, card, true);
    });

    card.ondragover = e => { e.preventDefault(); card.classList.add('drag-over'); };
    card.ondragleave = () => card.classList.remove('drag-over');
    card.ondrop = async e => {
        e.preventDefault();
        card.classList.remove('drag-over');
        if (e.dataTransfer.files.length) await handleUpload(link, e.dataTransfer.files[0], card);
    };

    card.querySelector('.delete-btn').onclick = async () => {
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
            ac.abort();
            card.remove();
            STATE.wallpapers = STATE.wallpapers.filter(wp => wp.linkName !== link.linkName);
            filterAndSort();
            showToast(t('deleted_success', 'Link deleted'), 'success');
        } catch (_) {
            // Remove animation class on error
            card.classList.remove('deleting');
        }
    };
}


async function handleUpload(link, fileOrUrl, card, isUrl = false) {
    const formData = new FormData();
    formData.append('linkName', link.linkName);

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
                    showToast(`🗜️ Compressed: ${info.percent}% smaller (${formatKB(info.saved)} saved)`, 'success');
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
        updateCard(card, updatedLink);
        filterAndSort();
        showToast(t('upload_success', 'Uploaded!'), 'success');
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
        if (btn) btn.disabled = true;
        try {
            await apiCall('/api/link', 'POST', { linkName: id });
            DOM.createInput.value = '';
            // Return focus to input so user can create next link immediately
            DOM.createInput.focus();
            const newLinkObj = {
                linkName: id,
                hasImage: false,
                mimeType: '',
                sizeBytes: 0,
                createdAt: Math.floor(Date.now() / 1000),
                imageUrl: '',
                preview: '',
            };
            STATE.wallpapers.push(newLinkObj);
            filterAndSort();
            const newCard = DOM.linksList.querySelector(`[data-link-name="${CSS.escape(id)}"]`)
                ?? DOM.linksList.lastElementChild;
            if (newCard) {
                newCard.animate([
                    { opacity: 0, transform: 'translateY(10px)' },
                    { opacity: 1, transform: 'translateY(0)' }
                ], { duration: 300 });
            }
            showToast(t('created_success', 'Link created'), 'success');
        } catch (_) {}
        finally {
            STATE.createPending = false;
            if (btn) btn.disabled = false;
        }
    });

    DOM.modalOverlay.onclick = (e) => {
        if (e.target === DOM.modalOverlay) closeModal();
    };

    DOM.confirmCancel.onclick = () => closeConfirm(false);
    DOM.confirmDelete.onclick = () => closeConfirm(true);
    DOM.confirmOverlay.onclick = (e) => {
        if (e.target === DOM.confirmOverlay) closeConfirm(false);
    };

    const regenBtn = document.getElementById('regenPreviewsBtn');
    if (regenBtn) {
        regenBtn.addEventListener('click', async () => {
            regenBtn.disabled = true;
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
                if (spanEl && origText) spanEl.textContent = origText;
            }
        });
    }
}


// UTILS
function formatKB(bytes) {
    if (!bytes) return '0 KB';
    const kb = bytes / 1024;
    return kb < 10 ? `${kb.toFixed(1)} KB` : `${Math.round(kb)} KB`;
}

function formatDate(ts) {
    return ts ? new Date(ts * 1000).toLocaleDateString() : '\u2014';
}
