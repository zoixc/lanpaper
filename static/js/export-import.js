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
        
        console.log('[Export] Successfully exported', exportData.wallpapers.length, 'wallpapers');
        showToast(`✅ ${t('export_success', 'Data exported successfully')}`, 'success');
    } catch (error) {
        console.error('[Export] Error:', error);
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
            console.error('[Import] Trigger error:', error);
        }
    }
}


/**
 * Process imported data from file
 */
async function importData(file) {
    console.log('[Import] Starting import from file:', file.name);
    
    try {
        const text = await file.text();
        const data = JSON.parse(text);
        
        console.log('[Import] Parsed data:', {
            version: data.version,
            wallpapers: data.wallpapers?.length,
            settings: Object.keys(data.settings || {})
        });
        
        // Validate data structure
        if (!data.wallpapers || !Array.isArray(data.wallpapers)) {
            throw new Error('Invalid data format: missing wallpapers array');
        }

        // Confirm import
        const count = data.wallpapers.length;
        const confirmMsg = t('import_confirm', `Import ${count} links? This will replace current data.`)
            .replace('{{count}}', count);
        
        if (!confirm(confirmMsg)) {
            console.log('[Import] User cancelled');
            return;
        }

        // Show loading toast
        showToast('⏳ Syncing data with server...', 'info');

        // Restore settings if available
        if (data.settings) {
            console.log('[Import] Restoring settings:', data.settings);
            
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
        console.log('[Import] Starting server sync...');
        const syncResult = await syncImportedLinksWithServer(data.wallpapers);
        console.log('[Import] Sync result:', syncResult);
        
        // Reload from server to ensure consistency
        console.log('[Import] Reloading from server...');
        await loadLinks();
        console.log('[Import] Reload complete. Current wallpapers:', STATE.wallpapers.length);
        
        showToast(`✅ ${t('import_success', 'Data imported successfully')}`, 'success');
    } catch (error) {
        console.error('[Import] Error:', error);
        showToast(`❌ ${t('import_error', 'Import failed: Invalid file')}`, 'error');
    }
}


/**
 * Sync imported wallpapers with server
 * Creates missing links on server (without images)
 */
async function syncImportedLinksWithServer(importedWallpapers) {
    console.log('[Sync] Syncing', importedWallpapers.length, 'wallpapers with server');
    
    try {
        // Get current server links
        console.log('[Sync] Fetching current server links...');
        const serverLinks = await apiCall('/api/wallpapers');
        console.log('[Sync] Server has', serverLinks.length, 'links');
        
        const serverLinkNames = new Set(serverLinks.map(link => link.linkName || link.id));
        console.log('[Sync] Server link names:', Array.from(serverLinkNames));
        
        // Find links that exist in import but not on server
        const missingLinks = importedWallpapers.filter(
            wp => !serverLinkNames.has(wp.linkName || wp.id)
        );
        
        console.log('[Sync] Missing links to create:', missingLinks.map(l => l.linkName || l.id));
        
        if (missingLinks.length === 0) {
            console.log('[Sync] No missing links, skipping sync');
            return { total: 0, success: 0, failed: 0 };
        }
        
        // Create missing links on server sequentially
        const results = [];
        for (const link of missingLinks) {
            const linkName = link.linkName || link.id;
            console.log('[Sync] Creating link:', linkName);
            
            try {
                await apiCall('/api/link', 'POST', { linkName });
                console.log('[Sync] Created:', linkName);
                results.push({ success: true, linkName });
            } catch (error) {
                console.error('[Sync] Failed to create:', linkName, error);
                results.push({ success: false, linkName, error: error.message });
            }
        }
        
        const successCount = results.filter(r => r.success).length;
        const failCount = results.filter(r => !r.success).length;
        
        console.log('[Sync] Complete:', successCount, 'created,', failCount, 'failed');
        
        if (failCount > 0) {
            const failed = results.filter(r => !r.success);
            console.warn('[Sync] Failed links:', failed);
        }
        
        return {
            total: missingLinks.length,
            success: successCount,
            failed: failCount,
            results
        };
    } catch (error) {
        console.error('[Sync] Fatal error during sync:', error);
        throw error;
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

console.log('[Export/Import] Module loaded');
