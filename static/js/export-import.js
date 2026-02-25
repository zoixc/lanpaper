/**
 * Export/Import functionality for Lanpaper
 * Allows backing up and restoring all data
 */

// DOM Elements for export/import
DOM.exportBtn = document.getElementById('exportBtn');
DOM.importBtn = document.getElementById('importBtn');
DOM.importFile = document.getElementById('importFile');


/**
 * Export all data to JSON file
 */
function exportData() {
    try {
        const exportData = {
            version: '1.0.0',
            exportDate: new Date().toISOString(),
            settings: {
                lang: STATE.lang,
                theme: localStorage.getItem('theme'),
                viewMode: STATE.viewMode,
                sortBy: STATE.sortBy
            },
            wallpapers: STATE.wallpapers
        };

        const dataStr = JSON.stringify(exportData, null, 2);
        const dataBlob = new Blob([dataStr], { type: 'application/json' });
        
        const url = URL.createObjectURL(dataBlob);
        const link = document.createElement('a');
        link.href = url;
        link.download = `lanpaper-backup-${new Date().toISOString().split('T')[0]}.json`;
        
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        
        URL.revokeObjectURL(url);
        
        showToast(`✅ ${t('export_success', 'Data exported successfully')}`, 'success');
    } catch (error) {
        console.error('Export error:', error);
        showToast(`❌ ${t('export_error', 'Export failed')}`, 'error');
    }
}


/**
 * Import data from JSON file
 */
async function importData(file) {
    try {
        const text = await file.text();
        const data = JSON.parse(text);
        
        // Validate data structure
        if (!data.wallpapers || !Array.isArray(data.wallpapers)) {
            throw new Error('Invalid data format');
        }

        // Confirm import
        const count = data.wallpapers.length;
        const confirmMsg = t('import_confirm', `Import ${count} links? This will replace current data.`)
            .replace('{{count}}', count);
        
        if (!confirm(confirmMsg)) {
            return;
        }

        // Restore settings if available
        if (data.settings) {
            if (data.settings.lang) {
                await setLanguage(data.settings.lang);
            }
            if (data.settings.theme) {
                STATE.isDark = data.settings.theme === 'dark';
                localStorage.setItem('theme', data.settings.theme);
                applyTheme();
            }
            if (data.settings.viewMode) {
                STATE.viewMode = data.settings.viewMode;
                localStorage.setItem('viewMode', data.settings.viewMode);
                applyViewMode(data.settings.viewMode);
            }
            if (data.settings.sortBy) {
                STATE.sortBy = data.settings.sortBy;
                localStorage.setItem('sortBy', data.settings.sortBy);
                if (DOM.sortSelect) DOM.sortSelect.value = data.settings.sortBy;
            }
        }

        // Restore wallpapers data
        STATE.wallpapers = data.wallpapers;
        filterAndSort();
        
        showToast(`✅ ${t('import_success', 'Data imported successfully')}`, 'success');
    } catch (error) {
        console.error('Import error:', error);
        showToast(`❌ ${t('import_error', 'Import failed: Invalid file')}`, 'error');
    }
}


// Event listeners
if (DOM.exportBtn) {
    DOM.exportBtn.addEventListener('click', exportData);
}

if (DOM.importBtn && DOM.importFile) {
    DOM.importBtn.addEventListener('click', () => {
        DOM.importFile.click();
    });
    
    DOM.importFile.addEventListener('change', (e) => {
        const file = e.target.files[0];
        if (file) {
            importData(file);
        }
        // Reset input so same file can be selected again
        e.target.value = '';
    });
}
