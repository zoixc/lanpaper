// SPDX-License-Identifier: MIT
import { getPanelFacade, registerFeature } from './features.js';

/**
 * Export and import of the link list and the panel settings.
 *
 * The link-list export holds metadata only: media files are not embedded and
 * access tokens are never written to the file. Importing re-creates the links
 * the server does not have yet and leaves every existing link (and its media)
 * untouched.
 *
 * The file stays independent of app.js internals: it goes through the small
 * public interface in the panel capability facade, so a panel that failed to start
 * gives a clear message here instead of a half-working dialog.
 */
(function () {
    'use strict';

    function app() { return getPanelFacade(); }
    function ready() { const a = app(); return !!a; }
    function t(key, vars) { return ready() ? app().t(key, vars) : key; }
    function say(key, type) { if (ready()) app().toast(t(key), { type: type || 'info' }); }

    /**
     * Export all data to a JSON file.
     */
    function exportData() {
        if (!ready()) return;
        const a = app();
        try {
            const snapshot = a.snapshot();
            const payload = {
                version: '1.0.0',
                exportDate: new Date().toISOString(),
                settings: {
                    lang: snapshot.lang,
                    theme: snapshot.theme,
                    view: snapshot.view,
                    sort: snapshot.sort
                },
                // Access tokens are secrets and an import never restores them,
                // so they are left out of the link-list file entirely.
                wallpapers: snapshot.links.map(function (link) {
                    const copy = Object.assign({}, link);
                    delete copy.accessToken;
                    return copy;
                })
            };

            const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
            const url = URL.createObjectURL(blob);
            const anchor = document.createElement('a');
            anchor.href = url;
            anchor.download = 'lanpaper-link-list-' + new Date().toISOString().slice(0, 10) + '.json';
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
     * Read a link-list file and create the links the server is missing.
     */
    async function importData(file) {
        if (!ready()) return;
        const a = app();
        let progress = null;
        try {
            // A link-list export describes links; a file far larger than that is not one.
            if (file.size > 10 * 1024 * 1024) throw new Error('Link-list export too large');
            const data = JSON.parse(await file.text());

            if (!Array.isArray(data.wallpapers) || data.wallpapers.length > 5000) {
                throw new Error('Invalid link-list export: missing or too many links');
            }
            const records = [];
            const names = new Set();
            for (const link of data.wallpapers) {
                const name = link && (link.linkName || link.id);
                if (typeof name !== 'string' || !a.validLinkName(name) || names.has(name)) {
                    throw new Error('Invalid or duplicate link name in link-list export');
                }
                names.add(name);
                // Secrets are deliberately excluded. A token link receives a
                // newly generated token from the server.
                records.push({ linkName: name, category: link.category || '', accessLevel: link.accessLevel || '' });
            }

            // Validate every bounded batch before creating the first record.
            const validation = await submitBatches(records, true);
            const readyCount = validation.results.filter(item => item.status === 'ready').length;
            if (!readyCount) {
                a.toast(t('import_nothing'), { type: 'info' });
                return;
            }
            const confirmed = await a.confirm({
                title: t('import_confirm_title'),
                text: t('import_confirm', { count: readyCount })
            });
            if (!confirmed) return;

            const controller = new AbortController();
            progress = a.toast(t('import_progress', { done: 0, total: records.length }), {
                type: 'info', duration: 0, action: t('cancel'), onAction: () => controller.abort()
            });
            const result = await submitBatches(records, false, controller.signal, function (done) {
                const text = progress.querySelector('.toast__text');
                if (text) text.textContent = t('import_progress', { done, total: records.length });
            });
            progress.dismissToast();
            progress = null;
            downloadReport(result);
            await a.reloadLinks();
            if (result.failed) a.toast(t('import_partial', { count: result.failed }), { type: 'error' });
            else a.toast(t('import_success', { count: result.created }), { type: 'success' });
        } catch (error) {
            if (progress) progress.dismissToast();
            if ((error && error.name === 'AbortError') || (error && error.kind === 'cancelled')) {
                say('import_cancelled', 'info');
                return;
            }
            console.error('[Import] Error:', error);
            say('import_error', 'error');
        }
    }

    async function submitBatches(records, dryRun, signal, onProgress) {
        const a = app();
        const report = { dryRun, total: records.length, created: 0, skipped: 0, failed: 0, results: [] };
        for (let offset = 0; offset < records.length; offset += 100) {
            const batch = records.slice(offset, offset + 100);
            const response = await a.request('/api/import/links', 'POST', { records: batch, dryRun }, false, signal);
            report.created += response.created || 0;
            report.skipped += response.skipped || 0;
            response.results.forEach(item => report.results.push(Object.assign({}, item, { index: item.index + offset })));
            if (onProgress) onProgress(Math.min(offset + batch.length, records.length));
        }
        report.failed = report.results.filter(item => item.status === 'invalid').length;
        return report;
    }

    function downloadReport(report) {
        const blob = new Blob([JSON.stringify(report, null, 2)], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const anchor = document.createElement('a');
        anchor.href = url;
        anchor.download = 'lanpaper-link-list-import-report-' + new Date().toISOString().slice(0, 10) + '.json';
        document.body.appendChild(anchor);
        anchor.click();
        anchor.remove();
        URL.revokeObjectURL(url);
    }

    const feature = { exportData, triggerImport, importData };
    registerFeature('link-list', feature);
    /* Temporary test/automation compatibility; unlike the old panel facade,
       this surface is limited to the three link-list actions. */
    window.LanpaperBackup = feature;
})();
