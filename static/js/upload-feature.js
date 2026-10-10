/* SPDX-License-Identifier: MIT */
import { uploadForm } from './feature-domain.js';
import { createOperationState } from './operation-state.js';

export function createUploadFeature(deps) {
    const operations = createOperationState();
    async function compress(file) {
        if (!deps.compressor() || !file.type.startsWith('image/')) return file;
        try {
            const smaller = await deps.compressor().compress(file);
            if (smaller && smaller.size < file.size) {
                const saved = file.size - smaller.size;
                const percent = file.size ? Math.round((saved / file.size) * 100) : 0;
                deps.toast(deps.t('compression_saved', {
                    percent, saved: deps.formatBytes(saved)
                }), { type: 'info', duration: 3000 });
                return smaller;
            }
        } catch (_) { /* fall back to the original media */ }
        return file;
    }

    async function file(link, source, mode, opts) {
        const key = 'upload:' + link.linkName;
        if (!operations.begin(key)) {
            if (!(opts && opts.quiet)) deps.toast(deps.t('operation_in_progress'), { type: 'info' });
            return false;
        }
        let busy = null;
        try {
            if (!source.type.startsWith('image/') && !source.type.startsWith('video/')) {
                deps.toast(deps.t('invalid_image'), { type: 'error' });
                return false;
            }
            const quiet = !!(opts && opts.quiet);
            const form = uploadForm(link.linkName, mode, 'file', await compress(source));
            busy = quiet ? null : deps.toast(deps.t('uploading'), { type: 'info', duration: 0 });
            deps.applyUpdate(await deps.request('/api/upload', 'POST', form, true, undefined,
                () => file(link, source, mode, opts)));
            if (!quiet) deps.toast(deps.t(mode === 'append' ? 'append_success' : 'upload_success'));
            return true;
        } catch (_) { return false; }
        finally {
            operations.end(key);
            if (busy) busy.dismissToast();
        }
    }

    async function files(link, sources, mode) {
        if (mode !== 'append') {
            if (sources.length > 1) deps.toast(deps.t('upload_first_only', {
                count: sources.length - 1
            }), { type: 'info', duration: 4500 });
            return file(link, sources[0], mode);
        }
        let done = 0;
        for (const source of sources) {
            if (await file(link, source, 'append', { quiet: true })) done += 1;
        }
        if (done) deps.toast(deps.t('append_many', { count: done }));
        return done > 0;
    }

    async function url(link, value, mode) {
        const key = 'upload:' + link.linkName;
        if (!operations.begin(key)) {
            deps.toast(deps.t('operation_in_progress'), { type: 'info' });
            return false;
        }
        const form = uploadForm(link.linkName, mode, 'url', value);
        const busy = deps.toast(deps.t('uploading'), { type: 'info', duration: 0 });
        try {
            deps.applyUpdate(await deps.request('/api/upload', 'POST', form, true, undefined,
                () => url(link, value, mode)));
            deps.toast(deps.t(mode === 'append' ? 'append_success' : 'upload_success'));
            return true;
        } catch (_) { return false; }
        finally { operations.end(key); busy.dismissToast(); }
    }

    return Object.freeze({ file, files, url });
}
