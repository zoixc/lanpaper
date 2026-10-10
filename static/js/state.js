/* SPDX-License-Identifier: MIT */

export const RENDER_CHUNK = 12;

export function createAppState(overrides = {}) {
    return Object.assign({
        links: [], dict: {},
        config: { maxUploadMB: 50, playlistMax: 8, historyLimit: 3, historyBudgetBytes: 0, langs: [] },
        busy: false, query: '', scope: 'name', filter: 'all', access: 'any', sort: 'date_desc',
        view: 'grid', theme: 'auto', palette: 'mono', lang: 'ru', selecting: false,
        selected: new Set(), loading: true, loadError: false, shown: RENDER_CHUNK,
        reveal: null, scrolledReveal: null, pendingScroll: null, lastDeleted: null, panelTab: 'media'
    }, overrides);
}

export function normalizeLink(raw = {}) {
    const link = Object.assign({}, raw);
    link.linkName = link.linkName || link.id || '';
    link.history = Array.isArray(link.history) ? link.history : [];
    link.items = Array.isArray(link.items) ? link.items : [];
    link.currentVersion = Math.max(1, Number(link.currentVersion) || 1);
    link.pinned = !!link.pinned;
    link.hasImage = !!link.hasImage;
    link.accessLevel = link.accessLevel || 'public';
    link.category = link.category || 'other';
    return link;
}

export const mediaExt = link => String(link.mimeType || '').toLowerCase();
export const isVideoMedia = link => ['mp4', 'webm'].includes(mediaExt(link));

const SORTS = {
    date_desc: (a, b) => b.modTime - a.modTime,
    date_asc: (a, b) => a.modTime - b.modTime,
    name_asc: (a, b) => a.linkName.localeCompare(b.linkName, 'ru'),
    name_desc: (a, b) => b.linkName.localeCompare(a.linkName, 'ru'),
    size_desc: (a, b) => (b.sizeBytes + b.items.length * 1e6) - (a.sizeBytes + a.items.length * 1e6)
};

const FILTERS = {
    all: () => true,
    image: link => link.hasImage && !isVideoMedia(link),
    video: isVideoMedia,
    playlist: link => link.items.length > 0,
    pinned: link => link.pinned
};

export function matchesQuery(state, link, helpers = {}) {
    const q = state.query.trim().toLowerCase();
    if (!q) return true;
    const parts = [link.linkName, link.mimeType, link.category];
    if (state.scope === 'all') {
        const accessText = helpers.accessText || (value => value);
        const formatBytes = helpers.formatBytes || (value => String(value || 0));
        parts.push(accessText(link.accessLevel), formatBytes(link.sizeBytes), String(link.sizeBytes),
            'v' + link.currentVersion, String(link.width || ''), String(link.height || ''),
            link.items.length ? 'плейлист playlist' : '');
    }
    return parts.some(part => String(part || '').toLowerCase().includes(q));
}

export function selectVisibleLinks(state, helpers) {
    const filter = FILTERS[state.filter] || FILTERS.all;
    let links = state.links.filter(link => matchesQuery(state, link, helpers)).filter(filter);
    if (state.access !== 'any') links = links.filter(link => link.accessLevel === state.access);
    links.sort(SORTS[state.sort] || SORTS.date_desc);
    links.sort((a, b) => Number(!!b.pinned) - Number(!!a.pinned));
    return links;
}

export function countFilteredLinks(state, key, helpers) {
    const filter = FILTERS[key] || FILTERS.all;
    return state.links.filter(link => matchesQuery(state, link, helpers)
        && filter(link) && (state.access === 'any' || link.accessLevel === state.access)).length;
}

export function resetIncrementalRender(state) {
    state.shown = RENDER_CHUNK;
    state.reveal = null;
    state.scrolledReveal = null;
}
