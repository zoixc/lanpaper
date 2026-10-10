/* SPDX-License-Identifier: MIT */

const ACCESS = Object.freeze({
    public: Object.freeze({ icon: 'globe', key: 'access_public' }),
    local: Object.freeze({ icon: 'lock', key: 'access_local' }),
    token: Object.freeze({ icon: 'key', key: 'access_token' }),
    auth: Object.freeze({ icon: 'user', key: 'access_auth' })
});

export function accessMeta(level) {
    return ACCESS[level] || ACCESS.public;
}

export function entryMeta(entry, formatBytes) {
    return [
        entry.ext ? String(entry.ext).toUpperCase() : '',
        entry.sizeBytes ? formatBytes(entry.sizeBytes) : ''
    ].filter(Boolean).join(' · ');
}

export function entryTitle(entry, formatBytes) {
    return entryMeta(entry, formatBytes) || formatBytes(entry.sizeBytes);
}

export function uploadForm(linkName, mode, sourceName, source) {
    const form = new FormData();
    form.append('linkName', linkName);
    if (mode === 'append') form.append('mode', 'append');
    form.append(sourceName, source);
    return form;
}

export function panelSnapshot(state) {
    return Object.freeze({
        lang: state.lang,
        theme: state.theme,
        view: state.view,
        sort: state.sort,
        links: state.links.map(link => Object.assign({}, link))
    });
}
