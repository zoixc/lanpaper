/**
 * Export/Import functionality for Lanpaper
 * Allows backing up and restoring all data
 * No hidden input elements - uses modern File System Access API with fallback
 */

// DOM Elements for export/import
DOM.exportBtn = document.getElementById('exportBtn');
DOM.importBtn = document.getElementById('importBtn');


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
 * Uses File System Access API with fallback to classic input method
 */
async function triggerImport() {
    try {
        // Try modern File System Access API first (Chrome 86+, Edge 86+)
        if ('showOpenFilePicker' in window) {
            const [fileHandle] = await window.showOpenFilePicker({
                types: [{
                    description: 'JSON Files',
                    accept: { 'application/json': ['.json'] }
                }],
                multiple: false
            });
            
            const file = await fileHandle.getFile();
            await importData(file);
        } else {
            // Fallback: Create temporary input element
            const input = document.createElement('input');
            input.type = 'file';
            input.accept = 'application/json,.json';
            
            input.onchange = async (e) => {
                const file = e.target.files[0];
                if (file) {
                    await importData(file);
                }
                input.remove();
            };
            
            input.click();
        }
    } catch (error) {
        // User cancelled or other error
        if (error.name !== 'AbortError') {
            console.error('Import trigger error:', error);
        }
    }
}


/**
 * Process imported data from file
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
    DOM.exportBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        exportData();
    });
}

if (DOM.importBtn) {
    DOM.importBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        triggerImport();
    });
}
