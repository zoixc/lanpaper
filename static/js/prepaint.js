// SPDX-License-Identifier: MIT
/**
 * Theme and layout before the first paint.
 *
 * The panel runs under a Content-Security-Policy without 'unsafe-inline', so
 * this cannot be an inline <script>. It is loaded synchronously in <head>:
 * the saved theme must be applied before the browser paints, otherwise a dark
 * panel flashes white on every load. For the same reason it arms the boot
 * screen: the panel keeps its unassembled markup hidden until app.js has
 * built the real one.
 *
 * Keys: 'lp-ui' is the current one; 'theme', 'viewMode', 'sortBy' and 'lang'
 * are the keys panels installed before 2.0 wrote, and they are read so an
 * update does not lose the user's choice.
 */
(function () {
    'use strict';

    (function () {
        /* Экран загрузки включается здесь, до первой отрисовки: app.js —
           модуль, он выполняется после разбора документа и ждёт словарь,
           а до этого разметка панели — не тот вид, который получится.
           Снимает экран app.js; если он не выполнится (нет модулей, упала
           сеть, ошибка в скрипте), экран снимается по таймауту: пустая
           страница хуже промежуточной. Входной страницы это не касается —
           она статична и собирается без скрипта. */
        if (document.documentElement.dataset.layout !== 'studio') return;
        var timer = 0;

        function reveal() {
            if (document.documentElement.dataset.boot !== 'pending') return;
            document.documentElement.dataset.boot = 'done';
            clearTimeout(timer);
            /* Узел снимаем позже: метка убирает его с глаз (opacity), а
               сама панель к этому моменту уже нарисована. */
            setTimeout(function () {
                var screen = document.getElementById('bootScreen');
                if (screen && screen.parentNode) screen.parentNode.removeChild(screen);
            }, 300);
        }

        document.documentElement.dataset.boot = 'pending';
        /* 8 с: панель успевает собраться даже на медленном сервере, а
           сломанный скрипт не оставит её пустой навсегда. */
        timer = setTimeout(reveal, 8000);
        /* capture: так ловится и скрипт, который не загрузился (у события
           нет всплытия), и необработанное исключение — в обоих случаях
           init() до своей метки не дойдёт. Остальные ресурсы (картинка,
           шрифт, стили) в счёт не идут: без них панель собирается. */
        window.addEventListener('error', function (event) {
            var target = event.target;
            if (target && target !== window && target.tagName !== 'SCRIPT') return;
            reveal();
        }, true);
        window.LANPAPER_BOOT = { reveal: reveal };
    })();

    (function () {
        try {
            var saved = JSON.parse(localStorage.getItem('lp-ui') || '{}');
            var legacyTheme = localStorage.getItem('theme');
            var legacyView = localStorage.getItem('viewMode');
            var legacyLang = localStorage.getItem('lang');
            var root = document.documentElement;
            var theme = saved.theme || (legacyTheme === 'dark' ? 'dark' : (legacyTheme === 'light' ? 'light' : 'auto'));
            var dark = theme === 'dark' || (theme === 'auto' && matchMedia('(prefers-color-scheme: dark)').matches);
            var palette = saved.palette || 'mono';
            root.dataset.theme = dark ? 'dark' : 'light';
            root.dataset.themeMode = theme;
            root.dataset.palette = palette;
            if (root.dataset.layout === 'studio') {
                root.classList.add('studio', 'radical');
                /* Рельсу на телефоне не сворачивают: в ней лежат фильтры и
                   кнопки, а .sidebar-collapsed снимает её целиком. Класс
                   ставим только там, где сворачивание возможно, — иначе
                   полоса мигнёт пустотой до того, как studio-layout.js
                   вернёт её на место. */
                if (localStorage.getItem('lp-studio-nav-collapsed') === 'true' &&
                    !matchMedia('(max-width: 760px)').matches) {
                    root.classList.add('sidebar-collapsed');
                }
            } else if (root.dataset.layout) {
                root.classList.remove('studio', 'radical', 'sidebar-collapsed');
            }
            /* Полоса браузера: у <meta name="theme-color"> только media-варианты,
               и ручной выбор темы они не видят. Цвет действующей темы ставим
               обеим меткам — до первой отрисовки, иначе полоса мигнёт системным
               цветом. */
            var ring = palette === 'mono'
                ? (dark ? '#171717' : '#FAFAFA')
                : (dark ? '#14161B' : '#F5F6F9');
            var metas = document.querySelectorAll('meta[name="theme-color"]');
            for (var i = 0; i < metas.length; i++) metas[i].setAttribute('content', ring);
            root.dataset.view = saved.view || legacyView || 'grid';
            if (saved.lang || legacyLang) root.lang = saved.lang || legacyLang;
        } catch (e) { /* приватный режим: остаются значения по умолчанию */ }
    })();

    (function () {
        /* Второй поднабор гарнитуры. Интерфейс — Manrope, но в разметке
           предзагружен только латинский поднабор: он нужен при любом языке,
           потому что имена ссылок всегда латиницей. Если интерфейс русский,
           кириллический поднабор надо запросить тоже, и тоже до отрисовки:
           иначе бренд, заголовок и названия карточек сначала рисуются
           подменой и перещёлкиваются на каждой загрузке. Язык определяем так
           же, как langFromPrefs() в app.js: сохранённый выбор, затем язык
           браузера. Кириллица из шести языков панели нужна только русскому. */
        var stored = '';
        try {
            stored = localStorage.getItem('lang')
                || (JSON.parse(localStorage.getItem('lp-ui') || '{}')).lang || '';
        } catch (e) { stored = ''; }
        var lang = String(stored || navigator.language || '').slice(0, 2).toLowerCase();
        if (lang !== 'ru') return;
        var link = document.createElement('link');
        link.rel = 'preload';
        link.as = 'font';
        link.type = 'font/woff2';
        /* Шрифты загружаются в CORS-режиме: без crossorigin предзагрузка не
           совпадёт с настоящим запросом, и файл придёт дважды. */
        link.crossOrigin = 'anonymous';
        link.href = '/static/fonts/manrope-cyrillic-wght-normal.woff2';
        document.head.appendChild(link);
    })();
})();
