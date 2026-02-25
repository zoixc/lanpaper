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
            
            // Close all other dropdowns
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

        // Populate language options immediately with supported languages
        populateLanguageOptions();
    }

    function populateLanguageOptions() {
        const langOptions = document.getElementById('langOptions');
        if (!langOptions) return;

        // Supported languages
        const LANGS = {
            'en': 'EN',
            'ru': 'RU',
            'de': 'DE',
            'fr': 'FR',
            'it': 'IT',
            'es': 'ES'
        };
        
        // Get current language from localStorage or default
        const currentLang = localStorage.getItem('lang') || 'en';
        
        Object.keys(LANGS).forEach(code => {
            const btn = document.createElement('button');
            btn.className = 'lang-option';
            btn.textContent = LANGS[code];
            btn.dataset.lang = code;
            btn.type = 'button';
            
            // Check if this is the current language
            if (code === currentLang) {
                btn.classList.add('active');
            }
            
            btn.addEventListener('click', async (e) => {
                e.stopPropagation();
                
                // Remove active from all
                document.querySelectorAll('.lang-option').forEach(opt => {
                    opt.classList.remove('active');
                });
                
                // Add active to clicked
                btn.classList.add('active');
                
                // Set language (this will be handled by app.js setLanguage function)
                if (typeof window.setLanguage === 'function') {
                    await window.setLanguage(code);
                } else {
                    // Fallback if app.js hasn't loaded yet
                    localStorage.setItem('lang', code);
                    location.reload();
                }
                
                // Close dropdown after selection
                setTimeout(closeSettingsDropdown, 200);
            });
            
            langOptions.appendChild(btn);
        });
    }

    function closeSettingsDropdown() {
        const settingsDropdown = document.getElementById('settingsDropdown');
        const settingsBtn = document.getElementById('settingsBtn');
        
        if (settingsDropdown) {
            settingsDropdown.classList.remove('open');
        }
        if (settingsBtn) {
            settingsBtn.setAttribute('aria-expanded', 'false');
        }
    }

    function closeAllDropdowns() {
        // Close upload dropdowns
        document.querySelectorAll('.upload-dropdown.open').forEach(dropdown => {
            dropdown.classList.remove('open');
        });
        
        // Close custom selects
        document.querySelectorAll('.custom-select.open').forEach(select => {
            select.classList.remove('open');
        });
    }
})();
