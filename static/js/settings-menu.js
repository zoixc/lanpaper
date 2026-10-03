// SPDX-License-Identifier: MIT
/**
 * Settings Menu Control
 * Manages the dropdown with language selection, accent palette, data tools
 * and app actions. Outside clicks and Escape are handled centrally in app.js.
 */

(function() {
    'use strict';

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initSettingsMenu);
    } else {
        initSettingsMenu();
    }

    // ----------------------------------------------------------
    // ACCENT PALETTE
    // ----------------------------------------------------------
    // Four extra palettes on top of the default sunset gradient. Only accent
    // tokens change (see style.css); the choice is stored in localStorage and
    // applied as data-palette on <body>.
    const PALETTES = ['sunset', 'indigo', 'terra', 'sage', 'graphite'];
    const PALETTE_KEY = 'palette';
    const PALETTE_LABELS = {
        sunset: 'Sunset',
        indigo: 'Indigo',
        terra: 'Terracotta',
        sage: 'Sage',
        graphite: 'Graphite',
    };

    function applyPalette(name) {
        const palette = PALETTES.indexOf(name) === -1 ? PALETTES[0] : name;
        // 'sunset' is the built-in default: it matches no selector, so the
        // original accent tokens stay in force.
        document.body.dataset.palette = palette;
        document.querySelectorAll('[data-palette-btn]').forEach((btn) => {
            btn.setAttribute('aria-pressed', String(btn.dataset.paletteBtn === palette));
        });
    }

    function initPalette() {
        const host = document.getElementById('paletteOptions');
        if (!host) return;

        PALETTES.forEach((name) => {
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'palette-dot';
            btn.dataset.paletteBtn = name;
            btn.setAttribute('aria-pressed', 'false');
            // Localized name: aria-label and tooltip come from i18n, the dot
            // itself shows the colour.
            btn.setAttribute('aria-label', PALETTE_LABELS[name]);
            btn.dataset.i18nAria = 'palette_' + name;
            btn.dataset.i18nTitle = 'palette_' + name;
            btn.title = PALETTE_LABELS[name];
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                localStorage.setItem(PALETTE_KEY, name);
                applyPalette(name);
            });
            host.appendChild(btn);
        });

        applyPalette(localStorage.getItem(PALETTE_KEY) || PALETTES[0]);
    }

    function initSettingsMenu() {
        // The palette switcher works even if the dropdown markup is missing.
        initPalette();

        const settingsDropdown = document.getElementById('settingsDropdown');
        const settingsBtn = document.getElementById('settingsBtn');
        const langOptions = document.getElementById('langOptions');

        if (!settingsDropdown || !settingsBtn || !langOptions) return;

        settingsBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            const isOpen = settingsDropdown.classList.contains('open');
            // Close ALL dropdowns first (global helper from app.js), then
            // re-open this one unless the click was meant to close it.
            if (typeof window.closeAllDropdowns === 'function') {
                window.closeAllDropdowns(isOpen ? null : settingsDropdown);
            }
            settingsDropdown.classList.toggle('open', !isOpen);
            settingsBtn.setAttribute('aria-expanded', String(!isOpen));
        });

        populateLanguageOptions();
    }

    function populateLanguageOptions() {
        const langOptions = document.getElementById('langOptions');
        if (!langOptions) return;

        const LANGS = ['en', 'ru', 'de', 'fr', 'it', 'es'];
        const currentLang = localStorage.getItem('lang') || 'en';

        LANGS.forEach((code) => {
            const btn = document.createElement('button');
            btn.className = 'lang-option';
            btn.textContent = code.toUpperCase();
            btn.dataset.lang = code;
            btn.type = 'button';
            btn.setAttribute('aria-label', code.toUpperCase());

            if (code === currentLang) btn.classList.add('active');

            btn.addEventListener('click', async (e) => {
                e.stopPropagation();

                document.querySelectorAll('.lang-option').forEach(opt => opt.classList.remove('active'));
                btn.classList.add('active');

                if (typeof window.setLanguage === 'function') {
                    await window.setLanguage(code);
                } else {
                    localStorage.setItem('lang', code);
                    location.reload();
                }

                if (typeof window.closeAllDropdowns === 'function') window.closeAllDropdowns();
            });

            langOptions.appendChild(btn);
        });
    }
})();
