// SPDX-License-Identifier: MIT
/**
 * Export and import of the link list and the panel settings.
 *
 * The backup holds link metadata only: media files are not embedded and
 * access tokens are never written to the file. Importing re-creates the links
 * the server does not have yet and leaves every existing link (and its media)
 * untouched.
 *
 * The file stays independent of app.js internals: it goes through the small
 * public interface in window.LanpaperApp, so a panel that failed to start
 * gives a clear message here instead of a half-working dialog.
 */
(function () {
    'use strict';

    function app() { return window.LanpaperApp; }
    function ready() { const a = app(); return !!(a && a.state && a.apiCall); }
    function t(key, vars) { return ready() ? app().t(key, vars) : key; }
    function say(key, type) { if (ready()) app().toast(t(key), { type: type || 'info' }); }

    /**
     * Export all data to a JSON file.
     */
    function exportData() {
        if (!ready()) return;
        const a = app();
        try {
            const payload = {
                version: '1.0.0',
                exportDate: new Date().toISOString(),
                settings: {
                    lang: a.state.lang,
                    theme: a.state.theme,
                    view: a.state.view,
                    sort: a.state.sort
                },
                // Access tokens are secrets and an import never restores them,
                // so they are left out of the backup file entirely.
                wallpapers: a.state.links.map(function (link) {
                    const copy = Object.assign({}, link);
                    delete copy.accessToken;
                    return copy;
                })
            };

            const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
            const url = URL.createObjectURL(blob);
            const anchor = document.createElement('a');
            anchor.href = url;
            anchor.download = 'lanpaper-backup-' + new Date().toISOString().slice(0, 10) + '.json';
            document.body.appendChild(anchor);
            anchor.click();
            anchor.remove();
            URL.revokeObjectURL(url);
            say('export_success', 'success');
        } catch (error) {
            console.error('[Export] Error:', error);
            say('export_error', 'error');
        }
    }

    /**
     * Import data from a JSON file. Uses the File System Access API where it
     * exists and a classic file input everywhere else.
     */
    async function triggerImport() {
        if (!ready()) return;
        try {
            if ('showOpenFilePicker' in window) {
                const picked = await window.showOpenFilePicker({
                    types: [{ description: 'JSON Files', accept: { 'application/json': ['.json'] } }],
                    multiple: false
                });
                await importData(await picked[0].getFile());
                return;
            }
            const input = document.createElement('input');
            input.type = 'file';
            input.accept = 'application/json,.json';
            input.addEventListener('change', async function () {
                const file = input.files && input.files[0];
                input.remove();
                if (file) await importData(file);
            });
            input.click();
        } catch (error) {
            // The user closed the picker — that is not an error.
            if (error && error.name !== 'AbortError') console.error('[Import] Trigger error:', error);
        }
    }

    /**
     * Read a backup file and create the links the server is missing.
     */
    async function importData(file) {
        if (!ready()) return;
        const a = app();
        try {
            // A backup describes links; a file far larger than that is not one.
            if (file.size > 10 * 1024 * 1024) throw new Error('Backup too large');
            const data = JSON.parse(await file.text());

            if (!Array.isArray(data.wallpapers) || data.wallpapers.length > 5000) {
                throw new Error('Invalid backup: missing or too many links');
            }
            const names = new Set();
            for (const link of data.wallpapers) {
                const name = link && (link.linkName || link.id);
                // The same rule the server applies to a link name: a name that
                // fails here is rejected before a single request is made.
                if (typeof name !== 'string' || !a.validLinkName(name)) {
                    throw new Error('Invalid link name in backup');
                }
                names.add(name);
            }

            const confirmed = await a.openConfirm({
                title: t('import_confirm_title'),
                text: t('import_confirm', { count: names.size })
            });
            if (!confirmed) return;

            a.toast(t('sync_in_progress'), { type: 'info' });
            const result = await syncImportedLinksWithServer(Array.from(names));
            if (result.failed) throw new Error(result.failed + ' links could not be imported');

            await a.reloadLinks();
            a.toast(t('import_success'), { type: 'success' });
        } catch (error) {
            console.error('[Import] Error:', error);
            say('import_error', 'error');
        }
    }

    /**
     * Create the links from a backup that the server does not have yet.
     */
    async function syncImportedLinksWithServer(importedNames) {
        const a = app();
        const response = await a.apiCall('/api/wallpapers');
        const list = Array.isArray(response) ? response : ((response && response.data) || []);
        const present = new Set(list.map((link) => link.linkName || link.id));

        const missing = importedNames.filter((name) => !present.has(name));
        const results = [];
        for (const linkName of missing) {
            try {
                await a.apiCall('/api/link', 'POST', { linkName: linkName });
                results.push({ success: true, linkName: linkName });
            } catch (error) {
                results.push({ success: false, linkName: linkName, error: error.message });
            }
        }
        return {
            total: missing.length,
            success: results.filter((r) => r.success).length,
            failed: results.filter((r) => !r.success).length,
            results: results
        };
    }

    window.LanpaperBackup = { exportData: exportData, triggerImport: triggerImport, importData: importData };
})();
