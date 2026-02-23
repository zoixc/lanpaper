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
};


// DOM ELEMENTS
const DOM = {
    langSwitcher: document.getElementById('langSwitcher'),
    langBtn: document.getElementById('langBtn'),
    langList: document.getElementById('langList'),
    langLabel: document.getElementById('langLabel'),
    themeBtn: document.getElementById('themeToggle'),
    viewBtn: document.getElementById('viewToggle'),
    linksList: document.getElementById('linksList'),
    emptyState: document.getElementById('emptyState'),
    toastContainer: document.getElementById('toastContainer'),
    searchInput: document.getElementById('searchInput'),
    searchStats: document.getElementById('searchStats'),
    sortSelect: document.getElementById('sortSelect'),
    appVersion: document.getElementById('appVersion'),

    // Modal Elements
    modalOverlay: document.getElementById('modalOverlay'),
    modalTitle: document.getElementById('modalTitle'),
    modalInput: document.getElementById('modalInput'),
    modalList: document.getElementById('modalList'),
    modalCancel: document.getElementById('modalCancelBtn'),
    modalConfirm: document.getElementById('modalConfirmBtn'),

    // Creation
    createBtn: document.getElementById('createLinkBtn'),
    createInput: document.getElementById('newLinkId'),
    createForm: document.getElementById('createForm'),

    // Templates
    template: document.getElementById('linkCardTemplate'),
};


// INITIALIZATION
document.addEventListener('DOMContentLoaded', async () => {
    initTheme();
    initLanguage();
    initView();
    initSearchSort();
    loadAppVersion();
    await loadLinks();
    setupGlobalListeners();
});


// VERSION
async function loadAppVersion() {
    try {
        const res = await fetch('/health');
        if (!res.ok) return;
        const data = await res.json();
        if (data.version && DOM.appVersion) {
            DOM.appVersion.textContent = `v${data.version}`;
        }
    } catch (e) {
        // silently ignore — footer will show 'v...'
    }
}


// THEME MANAGER
function initTheme() {
    const saved = localStorage.getItem('theme');
    if (saved) {
        STATE.isDark = saved === 'dark';
    } else {
        STATE.isDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    }
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

    const themeColorMeta = document.querySelector('meta[name="theme-color"]');
    if (themeColorMeta) {
        themeColorMeta.content = STATE.isDark ? '#191919' : '#ffffff';
    }

    const logo = document.querySelector('.logo');
    if (logo) {
        logo.src = STATE.isDark ? '/static/logo-dark.svg' : '/static/logo.svg';
    }

    const icons = DOM.themeBtn.querySelectorAll('.theme-icon');
    icons.forEach(icon => icon.classList.remove('active'));

    if (STATE.isDark) {
        const sun = DOM.themeBtn.querySelector('img[alt="Light"]');
        if (sun) sun.classList.add('active');
    } else {
        const moon = DOM.themeBtn.querySelector('img[alt="Dark"]');
        if (moon) moon.classList.add('active');
    }
}


// VIEW MODE MANAGER
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
    if (animate) {
        DOM.linksList.classList.add('switching');
        setTimeout(() => {
            updateClasses(mode);
            void DOM.linksList.offsetHeight;
            requestAnimationFrame(() => {
                DOM.linksList.classList.remove('switching');
            });
        }, 200);
    } else {
        updateClasses(mode);
    }
}


function updateClasses(mode) {
    if (mode === 'grid') {
        DOM.linksList.classList.add('grid-view');
        DOM.viewBtn.querySelectorAll('.list-icon').forEach(el => el.classList.add('active'));
        DOM.viewBtn.querySelectorAll('.grid-icon').forEach(el => el.classList.remove('active'));
    } else {
        DOM.linksList.classList.remove('grid-view');
        DOM.viewBtn.querySelectorAll('.list-icon').forEach(el => el.classList.remove('active'));
        DOM.viewBtn.querySelectorAll('.grid-icon').forEach(el => el.classList.add('active'));
    }
}


// LANGUAGE MANAGER
async function initLanguage() {
    const langs = ['en', 'ru', 'de', 'fr', 'it', 'es'];

    langs.forEach(lang => {
        const li = document.createElement('li');
        li.textContent = lang.toUpperCase();
        li.dataset.lang = lang;
        li.tabIndex = 0;
        li.addEventListener('click', () => setLanguage(lang));
        li.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') setLanguage(lang);
        });
        DOM.langList.appendChild(li);
    });

    await setLanguage(STATE.lang);

    DOM.langBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        DOM.langList.classList.toggle('open');
        DOM.langBtn.setAttribute('aria-expanded', DOM.langList.classList.contains('open'));
    });

    document.addEventListener('click', (e) => {
        if (!DOM.langSwitcher.contains(e.target)) {
            DOM.langList.classList.remove('open');
            DOM.langBtn.setAttribute('aria-expanded', 'false');
        }
    });
}


async function setLanguage(lang) {
    STATE.lang = lang;
    localStorage.setItem('lang', lang);
    DOM.langLabel.textContent = lang.toUpperCase();
    DOM.langList.classList.remove('open');

    try {
        const res = await fetch(`/static/i18n/${lang}.json`);
        if (res.ok) {
            STATE.translations = await res.json();
        } else {
            STATE.translations = {};
        }
    } catch (e) {
        console.warn('Translation load failed', e);
        STATE.translations = {};
    }

    applyTranslations();
    syncCustomSelectLabels();
    updateSearchStats();
    updateAriaLabels();
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


// Update dynamic aria-labels after language switch
function updateAriaLabels() {
    document.querySelectorAll('[data-i18n-aria]').forEach(el => {
        const key = el.dataset.i18nAria;
        if (STATE.translations[key]) el.setAttribute('aria-label', STATE.translations[key]);
    });
}


// SEARCH & SORT
function initSearchSort() {
    const searchInput = DOM.searchInput;
    const sortSelect = DOM.sortSelect;
    if (!searchInput) return;

    STATE.searchQuery = localStorage.getItem('searchQuery') || '';
    STATE.sortBy = localStorage.getItem('sortBy') || 'date_desc';
    searchInput.value = STATE.searchQuery;
    if (sortSelect) sortSelect.value = STATE.sortBy;

    let timer;
    searchInput.addEventListener('input', (e) => {
        clearTimeout(timer);
        timer = setTimeout(() => {
            STATE.searchQuery = e.target.value.toLowerCase().trim();
            localStorage.setItem('searchQuery', STATE.searchQuery);
            filterAndSort();
        }, 250);
    });

    if (sortSelect) {
        sortSelect.addEventListener('change', (e) => {
            STATE.sortBy = e.target.value;
            localStorage.setItem('sortBy', STATE.sortBy);
            filterAndSort();
        });
    }

    initCustomSelect();
}


// CUSTOM SELECT
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
        if (i18nKey && STATE.translations[i18nKey]) {
            opt.textContent = STATE.translations[i18nKey];
        }
        const isSelected = opt.dataset.value === STATE.sortBy;
        opt.classList.toggle('selected', isSelected);
        if (isSelected && label) {
            label.textContent = opt.textContent;
        }
    });
}


function filterWallpapers() {
    const query = STATE.searchQuery;
    if (!query) {
        STATE.filteredWallpapers = [...STATE.wallpapers];
    } else {
        STATE.filteredWallpapers = STATE.wallpapers.filter(wp => {
            const name = (wp.linkName || wp.id || '').toLowerCase();
            return name.includes(query);
        });
    }
}


function sortWallpapers(list) {
    const sorted = [...list];
    switch (STATE.sortBy) {
        case 'name_asc':
            sorted.sort((a, b) => a.linkName.localeCompare(b.linkName)); break;
        case 'name_desc':
            sorted.sort((a, b) => b.linkName.localeCompare(a.linkName)); break;
        case 'date_desc':
            sorted.sort((a, b) => (b.createdAt || 0) - (a.createdAt || 0)); break;
        case 'date_asc':
            sorted.sort((a, b) => (a.createdAt || 0) - (b.createdAt || 0)); break;
    }
    return sorted;
}


function filterAndSort() {
    filterWallpapers();
    STATE.filteredWallpapers = sortWallpapers(STATE.filteredWallpapers);
    updateSearchStats();
    renderLinks(STATE.filteredWallpapers);
}


function updateSearchStats() {
    const statsEl = DOM.searchStats;
    if (!statsEl) return;

    const total = STATE.wallpapers.length;
    const shown = STATE.filteredWallpapers.length;

    if (STATE.searchQuery) {
        const tpl = t('search_found', 'Found {{shown}} of {{total}}');
        statsEl.textContent = tpl
            .replace('{{shown}}', shown)
            .replace('{{total}}', total);
    } else {
        const tpl = t('search_total', 'Total: {{total}}');
        statsEl.textContent = tpl.replace('{{total}}', total);
    }
}


// NOTIFICATIONS (TOASTS)
function showToast(message, type = 'success') {
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.innerHTML = `<span>${message}</span>`;

    DOM.toastContainer.appendChild(toast);

    setTimeout(() => {
        toast.classList.add('hiding');
        setTimeout(() => {
            if (toast.parentNode) toast.remove();
        }, 400);
    }, 3000);
}


// MODAL MANAGER
let modalResolve = null;


function showModal(type, titleKey, placeholderKey = '') {
    return new Promise((resolve) => {
        modalResolve = resolve;

        DOM.modalTitle.textContent = t(titleKey, t('modal_default_title', 'Input'));
        DOM.modalOverlay.classList.remove('hidden');
        DOM.modalOverlay.setAttribute('aria-hidden', 'false');

        DOM.modalInput.value = '';
        DOM.modalInput.style.display = 'none';
        DOM.modalList.innerHTML = '';
        DOM.modalList.classList.add('hidden');
        DOM.modalConfirm.onclick = null;

        if (type === 'input') {
            DOM.modalInput.style.display = 'block';
            DOM.modalInput.placeholder = placeholderKey ? t(placeholderKey, 'https://...') : t('url_placeholder', 'https://...');
            DOM.modalInput.focus();
            DOM.modalInput.onkeydown = (e) => {
                if (e.key === 'Enter') confirmModal();
            };
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
    let result = null;

    if (DOM.modalInput.style.display !== 'none') {
        result = DOM.modalInput.value.trim();
    } else {
        const selected = DOM.modalList.querySelector('.selected');
        if (selected) result = selected.dataset.value;
    }

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
    DOM.modalList.innerHTML = `<div style="grid-column: 1/-1; text-align: center;">${t('loading', 'Loading...')}</div>`;

    try {
        const res = await fetch('/api/external-images');
        if (!res.ok) throw new Error('Failed');
        const files = await res.json();

        DOM.modalList.innerHTML = '';

        if (!files || files.length === 0) {
            DOM.modalList.innerHTML = `<div style="grid-column: 1/-1; text-align: center; color: var(--text-muted);">${t('server_empty', 'No images found')}</div>`;
            return;
        }

        files.forEach(file => {
            const div = document.createElement('div');
            div.className = 'image-option';
            div.dataset.value = file;

            const previewUrl = `/api/external-image-preview?path=${encodeURIComponent(file)}`;
            div.innerHTML = `
                <img src="${previewUrl}" loading="lazy" alt="${file}">
                <div class="image-name">${file}</div>
            `;

            div.onclick = () => {
                DOM.modalList.querySelectorAll('.image-option').forEach(el => el.classList.remove('selected'));
                div.classList.add('selected');
            };
            DOM.modalList.appendChild(div);
        });
    } catch (e) {
        DOM.modalList.innerHTML = `<div style="color:red; text-align:center;">${t('server_error', 'Error loading images')}</div>`;
    }
}


// API HELPERS
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

        const contentType = res.headers.get("content-type");
        if (contentType && contentType.indexOf("application/json") !== -1) {
            return res.json();
        }
        return null;
    } catch (e) {
        showToast(e.message, 'error');
        throw e;
    }
}


// APP LOGIC
async function loadLinks() {
    try {
        const wallpapers = await apiCall('/api/wallpapers');
        STATE.wallpapers = wallpapers || [];
        updateSearchStats();
        filterAndSort();
    } catch (e) {
        console.error('LoadLinks error:', e);
        showToast(t('load_error', 'Failed to load links'), 'error');
        STATE.wallpapers = [];
        renderLinks([]);
    }
}


function renderLinks(wallpapers) {
    DOM.linksList.innerHTML = '';

    if (!wallpapers || wallpapers.length === 0) {
        DOM.emptyState.style.display = 'block';
        return;
    }
    DOM.emptyState.style.display = 'none';

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
    if (mime.startsWith('video/')) return 'video';
    if (mime.startsWith('image/')) return 'image';
    const path = link.imagePath || link.imageUrl || '';
    const ext = path.split('.').pop().toLowerCase();
    if (['mp4', 'webm'].includes(ext)) return 'video';
    if (['jpg', 'jpeg', 'png', 'webp', 'gif'].includes(ext)) return 'image';
    return 'other';
}


function updateCard(card, link) {
    const linkName = link.linkName || link.id;
    card.querySelector('.link-id').textContent = linkName;

    const fullUrl = `${window.location.origin}/${linkName}`;

    const previewLink = card.querySelector('.preview-link');
    previewLink.href = fullUrl;
    previewLink.setAttribute('aria-label', t('aria_open_image', 'Open image'));

    const linkIdEl = card.querySelector('.link-id');
    linkIdEl.setAttribute('aria-label', t('aria_link_id', 'Link ID'));

    const category = link.hasImage ? detectCategory(link) : 'other';

    let fileType;
    if (link.mimeType) {
        fileType = (link.mimeType.split('/')[1] || link.mimeType).toUpperCase();
    } else if (link.hasImage) {
        const path = link.imagePath || link.imageUrl || '';
        fileType = path.split('.').pop().toUpperCase() || 'IMAGE';
    } else {
        fileType = t('no_image', 'No image');
    }

    const dateStr = link.createdAt ? formatDate(link.createdAt) : '\u2014';
    const sizeStr = link.sizeBytes ? ` \u00b7 ${formatKB(link.sizeBytes)}` : '';

    const linkMeta = card.querySelector('.link-meta');
    linkMeta.textContent = `${category} \u00b7 ${fileType}${sizeStr} \u00b7 ${dateStr}`;
    linkMeta.setAttribute('aria-label', t('aria_file_info', 'File info'));

    const previewWrapper = card.querySelector('.preview-wrapper');
    previewWrapper.innerHTML = '';

    const resolvedPreview = link.previewPath || link.preview || '';

    if (link.hasImage && resolvedPreview) {
        const img = document.createElement('img');
        img.src = '/' + resolvedPreview.replace(/^\//, '') + `?t=${Date.now()}`;
        img.alt = 'Preview';
        img.className = 'preview';
        img.onerror = () => {
            previewWrapper.innerHTML = `<div class="no-image">${t('preview_unavailable', 'Preview unavailable')}</div>`;
        };
        previewWrapper.appendChild(img);
    } else if (link.hasImage) {
        const img = document.createElement('img');
        img.src = '/' + (link.imageUrl || '').replace(/^\//, '') || fullUrl;
        img.alt = 'Image';
        img.className = 'preview';
        img.onerror = () => {
            previewWrapper.innerHTML = `<div class="no-image">${t('image_unavailable', 'Image unavailable')}</div>`;
        };
        previewWrapper.appendChild(img);
    } else {
        const noImg = document.createElement('div');
        noImg.className = 'no-image';
        noImg.textContent = t('no_image', 'No image');
        previewWrapper.appendChild(noImg);
    }

    const copyBtn = card.querySelector('.copy-url-btn');
    const newCopyBtn = copyBtn.cloneNode(true);
    copyBtn.parentNode.replaceChild(newCopyBtn, copyBtn);
    newCopyBtn.onclick = (e) => {
        e.preventDefault();
        navigator.clipboard.writeText(fullUrl).then(() => {
            newCopyBtn.textContent = t('copied', 'Copied!');
            newCopyBtn.classList.add('copied');
            setTimeout(() => {
                newCopyBtn.classList.remove('copied');
                newCopyBtn.textContent = t('copy_url', 'Copy URL');
            }, 1500);
        });
    };
}


function setupCardEvents(card, link) {
    const fileInput = card.querySelector('.file-input');
    const dropdown = card.querySelector('.upload-dropdown');
    const toggleBtn = card.querySelector('.upload-toggle-btn');

    toggleBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        const isOpen = dropdown.classList.contains('open');
        document.querySelectorAll('.upload-dropdown.open').forEach(d => d.classList.remove('open'));
        if (!isOpen) dropdown.classList.add('open');
        toggleBtn.setAttribute('aria-expanded', String(!isOpen));
    });

    document.addEventListener('click', (e) => {
        if (!dropdown.contains(e.target)) {
            dropdown.classList.remove('open');
            toggleBtn.setAttribute('aria-expanded', 'false');
        }
    });

    card.querySelector('.upload-file-btn').addEventListener('click', () => {
        dropdown.classList.remove('open');
        fileInput.click();
    });

    fileInput.onchange = async () => {
        if (!fileInput.files.length) return;
        await handleUpload(link, fileInput.files[0], card);
        fileInput.value = '';
    };

    card.querySelector('.paste-url-btn').addEventListener('click', async () => {
        dropdown.classList.remove('open');
        const url = await showModal('input', 'enter_image_url_title', 'url_placeholder');
        if (url) await handleUpload(link, url, card, true);
    });

    card.querySelector('.select-server-btn').addEventListener('click', async () => {
        dropdown.classList.remove('open');
        const filename = await showModal('grid', 'select_server_title');
        if (filename) await handleUpload(link, filename, card, true);
    });

    card.ondragover = e => { e.preventDefault(); card.style.borderColor = 'var(--border-focus)'; };
    card.ondragleave = () => { card.style.borderColor = 'var(--border)'; };
    card.ondrop = async e => {
        e.preventDefault();
        card.style.borderColor = 'var(--border)';
        if (e.dataTransfer.files.length) {
            await handleUpload(link, e.dataTransfer.files[0], card);
        }
    };

    card.querySelector('.delete-btn').onclick = async () => {
        if (confirm(t('confirm_delete', 'Delete link?'))) {
            await apiCall(`/api/link/${link.linkName}`, 'DELETE');
            card.remove();
            STATE.wallpapers = STATE.wallpapers.filter(wp => wp.linkName !== link.linkName);
            updateSearchStats();
            if (DOM.linksList.children.length === 0) DOM.emptyState.style.display = 'block';
            showToast(t('deleted_success', 'Link deleted'), 'success');
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
        formData.append('file', fileOrUrl);
    }

    try {
        const updatedLink = await apiCall('/api/upload', 'POST', formData, true);

        if (!updatedLink.createdAt && link.createdAt) {
            updatedLink.createdAt = link.createdAt;
        }

        const idx = STATE.wallpapers.findIndex(wp => wp.linkName === updatedLink.linkName);
        if (idx !== -1) {
            STATE.wallpapers[idx] = updatedLink;
        } else {
            STATE.wallpapers.push(updatedLink);
        }

        updateCard(card, updatedLink);
        reorderCard(card, updatedLink);
        filterAndSort();
        showToast(t('upload_success', 'Uploaded!'), 'success');
    } catch (e) {
        console.error(e);
    }
}


function reorderCard(card, link) {
    if (link.hasImage) {
        DOM.linksList.prepend(card);
    }
}


function setupGlobalListeners() {
    DOM.createForm.onsubmit = async (e) => {
        e.preventDefault();
        const id = DOM.createInput.value.trim();
        if (!id) {
            showToast(t('invalid_id', 'ID is required'), 'error');
            return;
        }

        const idRe = /^[a-zA-Z0-9_-]{1,64}$/;
        if (!idRe.test(id)) {
            showToast(t('invalid_id_chars', 'Invalid ID format'), 'error');
            return;
        }

        try {
            await apiCall('/api/link', 'POST', { linkName: id });

            DOM.createInput.value = '';

            const newLinkObj = {
                linkName: id,
                hasImage: false,
                mimeType: '',
                sizeBytes: 0,
                createdAt: Math.floor(Date.now() / 1000),
                imageUrl: '',
                preview: '',
            };

            const clone = DOM.template.content.cloneNode(true);
            const article = clone.querySelector('article');
            updateCard(article, newLinkObj);
            setupCardEvents(article, newLinkObj);
            applyTranslations(article);
            updateAriaLabels();

            DOM.emptyState.style.display = 'none';
            DOM.linksList.appendChild(article);
            STATE.wallpapers.push(newLinkObj);
            filterAndSort();

            article.animate([
                { opacity: 0, transform: 'translateY(10px)' },
                { opacity: 1, transform: 'translateY(0)' }
            ], { duration: 300 });

            showToast(t('created_success', 'Link created'), 'success');
        } catch (e) {
            console.error(e);
        }
    };

    DOM.modalOverlay.onclick = (e) => {
        if (e.target === DOM.modalOverlay) closeModal();
    };
}


// UTILS
function formatKB(bytes) {
    if (!bytes) return '0 KB';
    const kb = bytes / 1024;
    return kb < 10 ? `${kb.toFixed(1)} KB` : `${Math.round(kb)} KB`;
}


function formatDate(ts) {
    if (!ts) return '\u2014';
    return new Date(ts * 1000).toLocaleDateString();
}
