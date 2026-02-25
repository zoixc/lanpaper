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

        // Show loading toast
        showToast('⏳ Syncing data with server...', 'info');

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

        // Sync imported links with server
        await syncImportedLinksWithServer(data.wallpapers);
        
        // Reload from server to ensure consistency
        await loadLinks();
        
        showToast(`✅ ${t('import_success', 'Data imported successfully')}`, 'success');
    } catch (error) {
        console.error('Import error:', error);
        showToast(`❌ ${t('import_error', 'Import failed: Invalid file')}`, 'error');
    }
}


/**
 * Sync imported wallpapers with server
 * Creates missing links on server (without images)
 */
async function syncImportedLinksWithServer(importedWallpapers) {
    // Get current server links
    const serverLinks = await apiCall('/api/wallpapers');
    const serverLinkNames = new Set(serverLinks.map(link => link.linkName));
    
    // Find links that exist in import but not on server
    const missingLinks = importedWallpapers.filter(
        wp => !serverLinkNames.has(wp.linkName)
    );
    
    if (missingLinks.length === 0) {
        return; // Nothing to sync
    }
    
    // Create missing links on server
    const createPromises = missingLinks.map(async (link) => {
        try {
            await apiCall('/api/link', 'POST', { linkName: link.linkName });
            return { success: true, linkName: link.linkName };
        } catch (error) {
            console.warn(`Failed to create link ${link.linkName}:`, error);
            return { success: false, linkName: link.linkName, error };
        }
    });
    
    const results = await Promise.allSettled(createPromises);
    
    // Log results
    const successCount = results.filter(r => r.status === 'fulfilled' && r.value.success).length;
    const failCount = results.length - successCount;
    
    if (failCount > 0) {
        console.warn(`Import: ${successCount} links created, ${failCount} failed`);
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
