/**
 * VIEW TOGGLE - MORPHING ANIMATION
 * Handles view switching with CSS morphing animation
 */

(function() {
    'use strict';

    const viewToggle = document.getElementById('viewToggleMorphing');
    const linksList = document.getElementById('linksList');
    const STORAGE_KEY = 'lanpaper_view';

    if (!viewToggle || !linksList) return;

    // Load saved view preference
    const savedView = localStorage.getItem(STORAGE_KEY) || 'list';
    applyView(savedView, false);

    // Toggle on click
    viewToggle.addEventListener('click', () => {
        const currentView = viewToggle.getAttribute('data-view');
        const newView = currentView === 'list' ? 'grid' : 'list';
        applyView(newView, true);
        localStorage.setItem(STORAGE_KEY, newView);
    });

    /**
     * Apply view mode
     * @param {string} view - 'list' or 'grid'
     * @param {boolean} animate - Whether to animate the transition
     */
    function applyView(view, animate) {
        // Update button state
        viewToggle.setAttribute('data-view', view);

        // Update aria-label
        const label = view === 'list' ? 'Switch to grid view' : 'Switch to list view';
        viewToggle.setAttribute('aria-label', label);

        // Update links list
        if (animate) {
            // Smooth transition
            linksList.classList.add('switching');
            setTimeout(() => {
                linksList.classList.toggle('grid-view', view === 'grid');
                linksList.classList.remove('switching');
            }, 200);
        } else {
            // Instant (on page load)
            linksList.classList.toggle('grid-view', view === 'grid');
        }
    }

    // Expose for potential external use
    window.viewToggleMorphing = {
        setView: (view) => applyView(view, true),
        getView: () => viewToggle.getAttribute('data-view')
    };
})();
