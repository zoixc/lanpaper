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

        // Populate language options
        populateLanguageOptions();
    }

    function populateLanguageOptions() {
        const langOptions = document.getElementById('langOptions');
        if (!langOptions) return;

        // Wait for LANGS to be available from app.js
        const checkLangs = setInterval(() => {
            if (typeof LANGS !== 'undefined') {
                clearInterval(checkLangs);
                
                Object.keys(LANGS).forEach(code => {
                    const btn = document.createElement('button');
                    btn.className = 'lang-option';
                    btn.textContent = code.toUpperCase();
                    btn.dataset.lang = code;
                    
                    // Check if this is the current language
                    const currentLang = localStorage.getItem('lang') || 'en';
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
                        if (typeof setLanguage === 'function') {
                            await setLanguage(code);
                        }
                        
                        // Close dropdown after selection
                        setTimeout(closeSettingsDropdown, 200);
                    });
                    
                    langOptions.appendChild(btn);
                });
            }
        }, 50);
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
        
        // Close old language switcher if exists
        const langSwitcher = document.getElementById('langSwitcher');
        if (langSwitcher) {
            langSwitcher.classList.remove('open');
        }
    }

    // Make closeSettingsDropdown available globally for other scripts
    window.closeSettingsDropdown = closeSettingsDropdown;
})();
