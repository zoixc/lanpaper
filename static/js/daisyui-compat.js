/**
 * DaisyUI Compatibility Layer
 * Bridges gap between DaisyUI modals/dropdowns and existing app.js code
 */

(function() {
    'use strict';

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    function init() {
        setupModalCompat();
        setupDropdownCompat();
        setupThemeHandlers();
        setupLanguageHandlers();
        setupViewToggle();
    }

    // ========== MODAL COMPATIBILITY ==========
    // DaisyUI uses <dialog> with showModal()/close()
    // Our app.js uses classList.add/remove('hidden')
    function setupModalCompat() {
        const modals = ['modalOverlay', 'confirmOverlay'];
        
        modals.forEach(id => {
            const modal = document.getElementById(id);
            if (!modal) return;

            // Override remove('hidden') -> showModal()
            const originalRemove = modal.classList.remove.bind(modal.classList);
            modal.classList.remove = function(...args) {
                if (args.includes('hidden')) {
                    if (modal.tagName === 'DIALOG') {
                        modal.showModal();
                    }
                }
                originalRemove(...args);
            };

            // Override add('hidden') -> close()
            const originalAdd = modal.classList.add.bind(modal.classList);
            modal.classList.add = function(...args) {
                if (args.includes('hidden')) {
                    if (modal.tagName === 'DIALOG') {
                        modal.close();
                    }
                }
                originalAdd(...args);
            };

            // Override contains('hidden') -> check !open
            const originalContains = modal.classList.contains.bind(modal.classList);
            modal.classList.contains = function(className) {
                if (className === 'hidden' && modal.tagName === 'DIALOG') {
                    return !modal.open;
                }
                return originalContains(className);
            };
        });
    }

    // ========== DROPDOWN COMPATIBILITY ==========
    function setupDropdownCompat() {
        // Close dropdowns when clicking outside
        document.addEventListener('click', (e) => {
            if (!e.target.closest('.dropdown')) {
                document.querySelectorAll('.dropdown.open').forEach(d => {
                    d.classList.remove('open');
                    const toggle = d.querySelector('[aria-haspopup]');
                    if (toggle) toggle.setAttribute('aria-expanded', 'false');
                });
            }
        });
    }

    // ========== THEME HANDLING ==========
    function setupThemeHandlers() {
        document.querySelectorAll('[data-theme-select]').forEach(btn => {
            btn.addEventListener('click', () => {
                const theme = btn.dataset.themeSelect;
                document.documentElement.setAttribute('data-theme', theme);
                localStorage.setItem('theme', theme);
            });
        });
    }

    // ========== LANGUAGE HANDLING ==========
    function setupLanguageHandlers() {
        document.querySelectorAll('[data-lang]').forEach(btn => {
            btn.addEventListener('click', () => {
                const lang = btn.dataset.lang;
                if (window.setLanguage) {
                    window.setLanguage(lang);
                }
            });
        });
    }

    // ========== VIEW TOGGLE ==========
    function setupViewToggle() {
        const viewToggle = document.getElementById('viewToggle');
        const linksList = document.getElementById('linksList');
        
        if (!viewToggle || !linksList) return;
        
        viewToggle.addEventListener('click', () => {
            const isGrid = linksList.classList.contains('links-grid');
            
            if (isGrid) {
                // Switch to list
                linksList.classList.remove('links-grid');
                linksList.classList.add('links-list-view', 'flex', 'flex-col', 'gap-4');
                viewToggle.querySelectorAll('.view-icon').forEach(icon => {
                    icon.classList.toggle('hidden');
                });
            } else {
                // Switch to grid
                linksList.classList.add('links-grid');
                linksList.classList.remove('links-list-view', 'flex', 'flex-col', 'gap-4');
                viewToggle.querySelectorAll('.view-icon').forEach(icon => {
                    icon.classList.toggle('hidden');
                });
            }
        });
    }

})();
