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
    await loadLinks();
    setupGlobalListeners();
});

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
    
    // Update theme-color meta tag for PWA
    const themeColorMeta = document.querySelector('meta[name="theme-color"]');
    if (themeColorMeta) {
        themeColorMeta.content = STATE.isDark ? '#191919' : '#ffffff';
    }
    
    // Update logo based on theme
    const logo = document.querySelector('.logo');
    if (logo) {
        logo.src = STATE.isDark ? '/static/logo-dark.svg' : '/static/logo.svg';
    }
    
    const icons = DOM.themeBtn.querySelectorAll('.theme-icon');
    icons.forEach(icon => icon.classList.remove('active'));
    
    if (STATE.isDark) {
        const sun = DOM.themeBtn.querySelector('img[alt="Light"]');
        if(sun) sun.classList.add('active');
    } else {
        const moon = DOM.themeBtn.querySelector('img[alt="Dark"]');
        if(moon) moon.classList.add('active');
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
    const langs = ['en', 'ru', 'de', 'fr', 'it'];
    
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

// ===== 🔍 SEARCH & SORT FUNCTIONS (НОВЫЕ) =====
function initSearchSort() {
    const searchInput = DOM.searchInput;
    const sortSelect = DOM.sortSelect;
    if (!searchInput || !sortSelect) return;

    // Restore state
    STATE.searchQuery = localStorage.getItem('searchQuery') || '';
    STATE.sortBy = localStorage.getItem('sortBy') || 'date_desc';
    searchInput.value = STATE.searchQuery;
    sortSelect.value = STATE.sortBy;

    // Search debounce
    let timer;
    searchInput.addEventListener('input', (e) => {
        clearTimeout(timer);
        timer = setTimeout(() => {
            STATE.searchQuery = e.target.value.toLowerCase().trim();
            localStorage.setItem('searchQuery', STATE.searchQuery);
            filterAndSort();
        }, 250);
    });

    // Sort change
    sortSelect.addEventListener('change', (e) => {
        STATE.sortBy = e.target.value;
        localStorage.setItem('sortBy', STATE.sortBy);
        filterAndSort();
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
            sorted.sort((a, b) => (b.modTime || 0) - (a.modTime || 0)); break;
        case 'date_asc': 
            sorted.sort((a, b) => (a.modTime || 0) - (b.modTime || 0)); break;
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
    
    statsEl.textContent = STATE.searchQuery 
        ? `Found ${shown} of ${total}` 
        : `Total: ${total}`;
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
        
        // Header translation
        DOM.modalTitle.textContent = t(titleKey, 'Input'); 
        
        DOM.modalOverlay.classList.remove('hidden');
        DOM.modalOverlay.setAttribute('aria-hidden', 'false');

        DOM.modalInput.value = '';
        DOM.modalInput.style.display = 'none';
        DOM.modalList.innerHTML = '';
        DOM.modalList.classList.add('hidden');
        DOM.modalConfirm.onclick = null;

        if (type === 'input') {
            DOM.modalInput.style.display = 'block';
            // Placeholder translation
            DOM.modalInput.placeholder = t(placeholderKey, 'https://...'); 
            DOM.modalInput.focus();
            
            DOM.modalInput.onkeydown = (e) => {
                if (e.key === 'Enter') confirmModal();
            };
        } 
        else if (type === 'grid') {
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
        if (selected) {
            result = selected.dataset.value;
        }
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
    // Uploading status translation
    DOM.modalList.innerHTML = `<div style="grid-column: 1/-1; text-align: center;">${t('loading', 'Loading...')}</div>`;
    
    try {
        const res = await fetch('/api/external-images');
        if (!res.ok) throw new Error('Failed');
        const files = await res.json();
        
        DOM.modalList.innerHTML = '';
        
        if (!files || files.length === 0) {
            // Empty list translation
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
        // Error translation
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
        console.log('🔄 Loading wallpapers...');  // ✅ ЛОГ
        
        const wallpapers = await apiCall('/api/wallpapers');  // ✅ УБРАЛ no_cache
        console.log('📥 Loaded:', wallpapers);  // ✅ ЛОГ
        
        STATE.wallpapers = wallpapers || [];
        filterAndSort();
    } catch (e) {
        console.error('❌ LoadLinks error:', e);
        showToast('Failed to load links', 'error');
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
}

function updateCard(card, link) {
    const linkName = link.linkName || link.id;
    card.querySelector('.link-id').textContent = linkName;

    const fullUrl = `${window.location.origin}/${linkName}`;
    card.querySelector('.preview-link').href = fullUrl;

    card.querySelector('.link-meta').textContent = 
        `${link.category || 'other'} · ${link.hasImage ? 'Image' : 'No image'}`;

    const previewWrapper = card.querySelector('.preview-wrapper');
    previewWrapper.innerHTML = '';

    if (link.hasImage && link.previewPath) {
        // ✅ Backend дал путь к превью
        const img = document.createElement('img');
        img.src = link.previewPath + `?t=${Date.now()}`;
        img.alt = "Preview";
        img.className = "preview";
        img.onerror = () => previewWrapper.innerHTML = '<div class="no-image">Preview unavailable</div>';
        previewWrapper.appendChild(img);
    } else if (link.hasImage) {
        // ✅ Fallback — главная картинка
        const img = document.createElement('img');
        img.src = fullUrl;
        img.alt = "Image";
        img.className = "preview";
        img.style.maxHeight = '200px';
        img.onerror = () => previewWrapper.innerHTML = '<div class="no-image">Image unavailable</div>';
        previewWrapper.appendChild(img);
    } else {
        const noImg = document.createElement('div');
        noImg.className = 'no-image';
        noImg.textContent = 'No image';
        previewWrapper.appendChild(noImg);
    }

    // Copy button
    const copyBtn = card.querySelector('.copy-url-btn');
    const newCopyBtn = copyBtn.cloneNode(true);
    copyBtn.parentNode.replaceChild(newCopyBtn, copyBtn);
    newCopyBtn.onclick = (e) => {
        e.preventDefault();
        navigator.clipboard.writeText(fullUrl).then(() => {
            newCopyBtn.textContent = 'Copied!';
            newCopyBtn.classList.add('copied');
            setTimeout(() => {
                newCopyBtn.classList.remove('copied');
                newCopyBtn.textContent = 'Copy';
            }, 1500);
        });
    };
}

function setupCardEvents(card, link) {
    const fileInput = card.querySelector('.file-input');
    
    card.querySelector('.upload-file-btn').onclick = () => fileInput.click();
    
    fileInput.onchange = async () => {
        if (!fileInput.files.length) return;
        await handleUpload(link, fileInput.files[0], card);
        fileInput.value = '';
    };

    card.ondragover = e => { e.preventDefault(); card.style.borderColor = 'var(--border-focus)'; };
    card.ondragleave = () => { card.style.borderColor = 'var(--border)'; };
    card.ondrop = async e => {
        e.preventDefault();
        card.style.borderColor = 'var(--border)';
        if (e.dataTransfer.files.length) {
            await handleUpload(link, e.dataTransfer.files[0], card);
        }
    };

    // Modal header translation
    card.querySelector('.paste-url-btn').onclick = async () => {
        const url = await showModal('input', 'enter_image_url_title', 'url_placeholder');
        if (url) await handleUpload(link, url, card, true);
    };

    card.querySelector('.select-server-btn').onclick = async () => {
        const filename = await showModal('grid', 'select_server_title');
        if (filename) {
            await handleUpload(link, filename, card, true);
        }
    };

    card.querySelector('.delete-btn').onclick = async () => {
        if (confirm(t('confirm_delete', 'Delete link?'))) {
            await apiCall(`/api/link/${link.linkName}`, 'DELETE');
            card.remove();
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
        updateCard(card, updatedLink);
        reorderCard(card, updatedLink);
        showToast(t('upload_success', 'Uploaded!'), 'success');
        filterAndSort();  // ← перерисовать с учетом поиска/сортировки
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
                modTime: Math.floor(Date.now() / 1000),
                preview: ''
            };

            const clone = DOM.template.content.cloneNode(true);
            const article = clone.querySelector('article');
            updateCard(article, newLinkObj);
            setupCardEvents(article, newLinkObj);
            
            applyTranslations(article);

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
    if (!ts) return '—';
    return new Date(ts * 1000).toLocaleDateString();
}