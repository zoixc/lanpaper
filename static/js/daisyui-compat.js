/**
 * DaisyUI Compatibility Layer
 * 
 * This file provides compatibility between existing Lanpaper JS
 * and DaisyUI components (modals, themes, dropdowns)
 */

// Theme Management
function initThemeSystem() {
  const themeButtons = document.querySelectorAll('[data-theme-select]');
  
  themeButtons.forEach(btn => {
    btn.addEventListener('click', (e) => {
      e.preventDefault();
      const theme = btn.getAttribute('data-theme-select');
      document.documentElement.setAttribute('data-theme', theme);
      localStorage.setItem('theme', theme);
    });
  });
}

// Language Switcher
function initLanguageSwitcher() {
  const langButtons = document.querySelectorAll('[data-lang]');
  
  langButtons.forEach(btn => {
    btn.addEventListener('click', (e) => {
      e.preventDefault();
      const lang = btn.getAttribute('data-lang');
      // Trigger language change event for i18n system
      const event = new CustomEvent('languagechange', { detail: { language: lang } });
      window.dispatchEvent(event);
    });
  });
}

// Modal Helpers (DaisyUI uses <dialog> element)
window.showModal = function(modalId) {
  const modal = document.getElementById(modalId);
  if (modal && modal.tagName === 'DIALOG') {
    modal.showModal();
  } else if (modal) {
    // Fallback for non-dialog modals
    modal.classList.remove('hidden');
  }
};

window.closeModal = function(modalId) {
  const modal = document.getElementById(modalId);
  if (modal && modal.tagName === 'DIALOG') {
    modal.close();
  } else if (modal) {
    // Fallback for non-dialog modals
    modal.classList.add('hidden');
  }
};

// Toast Helper (DaisyUI toast positioning)
window.showToast = function(message, type = 'info') {
  const container = document.getElementById('toastContainer');
  if (!container) return;
  
  const toast = document.createElement('div');
  toast.className = `alert alert-${type} shadow-lg mb-2`;
  toast.innerHTML = `
    <div>
      <span>${message}</span>
    </div>
  `;
  
  container.appendChild(toast);
  
  // Auto-remove after 3 seconds
  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transition = 'opacity 0.3s';
    setTimeout(() => toast.remove(), 300);
  }, 3000);
};

// View Toggle (Grid/List)
function initViewToggle() {
  const viewToggle = document.getElementById('viewToggle');
  const linksList = document.getElementById('linksList');
  
  if (!viewToggle || !linksList) return;
  
  viewToggle.addEventListener('click', () => {
    const isGrid = linksList.classList.contains('links-grid');
    
    if (isGrid) {
      // Switch to list view
      linksList.classList.remove('links-grid');
      linksList.classList.add('links-list-view', 'flex', 'flex-col', 'gap-4');
      
      // Toggle icons
      viewToggle.querySelectorAll('.view-icon').forEach(icon => {
        icon.classList.toggle('hidden');
      });
    } else {
      // Switch to grid view
      linksList.classList.add('links-grid');
      linksList.classList.remove('links-list-view', 'flex', 'flex-col', 'gap-4');
      
      // Toggle icons
      viewToggle.querySelectorAll('.view-icon').forEach(icon => {
        icon.classList.toggle('hidden');
      });
    }
  });
}

// Initialize on DOM load
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => {
    initThemeSystem();
    initLanguageSwitcher();
    initViewToggle();
  });
} else {
  initThemeSystem();
  initLanguageSwitcher();
  initViewToggle();
}
