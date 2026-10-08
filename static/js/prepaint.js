// SPDX-License-Identifier: MIT
/**
 * Theme and layout before the first paint.
 *
 * The panel runs under a Content-Security-Policy without 'unsafe-inline', so
 * this cannot be an inline <script>. It is loaded synchronously in <head>:
 * the saved theme must be applied before the browser paints, otherwise a dark
 * panel flashes white on every load.
 *
 * Keys: 'lp-ui' is the current one; 'theme', 'viewMode', 'sortBy' and 'lang'
 * are the keys panels installed before 2.0 wrote, and they are read so an
 * update does not lose the user's choice.
 */
(function () {
    'use strict';

    (function () {
        try {
            var saved = JSON.parse(localStorage.getItem('lp-ui') || '{}');
            var legacyTheme = localStorage.getItem('theme');
            var legacyView = localStorage.getItem('viewMode');
            var legacyLang = localStorage.getItem('lang');
            var root = document.documentElement;
            var theme = saved.theme || (legacyTheme === 'dark' ? 'dark' : (legacyTheme === 'light' ? 'light' : 'auto'));
            var dark = theme === 'dark' || (theme === 'auto' && matchMedia('(prefers-color-scheme: dark)').matches);
            root.dataset.theme = dark ? 'dark' : 'light';
            root.dataset.themeMode = theme;
            /* Полоса браузера: у <meta name="theme-color"> только media-варианты,
               и ручной выбор темы они не видят. Цвет действующей темы ставим
               обеим меткам — до первой отрисовки, иначе полоса мигнёт системным
               цветом. */
            var ring = dark ? '#14161B' : '#F5F6F9';
            var metas = document.querySelectorAll('meta[name="theme-color"]');
            for (var i = 0; i < metas.length; i++) metas[i].setAttribute('content', ring);
            root.dataset.palette = saved.palette || 'mono';
            root.dataset.view = saved.view || legacyView || 'grid';
            if (saved.lang || legacyLang) root.lang = saved.lang || legacyLang;
        } catch (e) { /* приватный режим: остаются значения по умолчанию */ }
    })();
})();
