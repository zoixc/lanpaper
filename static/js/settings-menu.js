/**
 * Settings Menu Control
 * Manages dropdown menu with language selection and export/import
 */

(function() {
    'use strict';

    // Initialize after DOM is ready
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

        // Toggle dropdown
        settingsBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            const isOpen = settingsDropdown.classList.contains('open');
            
            closeAllDropdowns();
            
            if (!isOpen) {
                settingsDropdown.classList.add('open');
                settingsBtn.setAttribute('aria-expanded', 'true');
            }
        });

        // Close dropdown when clicking outside
        document.addEventListener('click', (e) => {
            if (!settingsDropdown.contains(e.target)) {
                closeSettingsDropdown();
            }
        });

        populateLanguageOptions();
    }

    function populateLanguageOptions() {
        const langOptions = document.getElementById('langOptions');
        if (!langOptions) return;

        const LANGS = {
            'en': 'EN',
            'ru': 'RU',
            'de': 'DE',
            'fr': 'FR',
            'it': 'IT',
            'es': 'ES'
        };
        
        const currentLang = localStorage.getItem('lang') || 'en';
        
        Object.entries(LANGS).forEach(([code, label]) => {
            const btn = document.createElement('button');
            btn.className = 'lang-option';
            btn.textContent = label;
            btn.dataset.lang = code;
            btn.type = 'button';
            
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
                
                closeSettingsDropdown();
            });
            
            langOptions.appendChild(btn);
        });
    }

    function closeSettingsDropdown() {
        const settingsDropdown = document.getElementById('settingsDropdown');
        const settingsBtn = document.getElementById('settingsBtn');
        
        settingsDropdown?.classList.remove('open');
        settingsBtn?.setAttribute('aria-expanded', 'false');
    }

    function closeAllDropdowns() {
        document.querySelectorAll('.upload-dropdown.open, .custom-select.open').forEach(el => {
            el.classList.remove('open');
        });
    }
})();
