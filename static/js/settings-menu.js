// SPDX-License-Identifier: MIT
/**
 * Settings Menu Control
 * Manages the dropdown with language selection, data tools and app actions.
 * Outside clicks and Escape are handled centrally in app.js.
 */

(function() {
    'use strict';

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initSettingsMenu);
    } else {
        initSettingsMenu();
    }

    function initSettingsMenu() {
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
