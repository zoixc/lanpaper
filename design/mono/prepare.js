// SPDX-License-Identifier: MIT
try {
    if (!localStorage.getItem('lp-mono-demo')) {
        localStorage.setItem('lp-mono-demo', JSON.stringify({ theme: 'light', palette: 'mono', lang: 'ru', view: 'grid' }));
    }
} catch (_) { /* Storage can be disabled; the panel still runs. */ }
