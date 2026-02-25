/**
 * Image Compressor
 * Compresses images client-side before upload
 */

class ImageCompressor {
    constructor(options = {}) {
        this.maxWidth = options.maxWidth || 1920;
        this.maxHeight = options.maxHeight || 1080;
        this.quality = options.quality || 0.85;
        this.mimeType = options.mimeType || 'image/jpeg';
    }

    /**
     * Compress an image file
     * @param {File} file - The image file to compress
     * @returns {Promise<File>} - Compressed image file
     */
    async compress(file) {
        return new Promise((resolve, reject) => {
            // Skip if not an image or already small
            if (!file.type.startsWith('image/')) {
                resolve(file);
                return;
            }

            // Skip GIFs and SVGs
            if (file.type === 'image/gif' || file.type === 'image/svg+xml') {
                resolve(file);
                return;
            }

            const reader = new FileReader();
            reader.onload = (e) => {
                const img = new Image();
                img.onload = () => {
                    try {
                        const compressed = this._compressImage(img, file.name, file.type);
                        resolve(compressed);
                    } catch (err) {
                        console.error('[Compressor] Error:', err);
                        resolve(file); // Fallback to original
                    }
                };
                img.onerror = () => reject(new Error('Failed to load image'));
                img.src = e.target.result;
            };
            reader.onerror = () => reject(new Error('Failed to read file'));
            reader.readAsDataURL(file);
        });
    }

    /**
     * Compress image using canvas
     * @private
     */
    _compressImage(img, filename, originalType) {
        // Calculate new dimensions
        let { width, height } = img;
        
        if (width > this.maxWidth || height > this.maxHeight) {
            const ratio = Math.min(this.maxWidth / width, this.maxHeight / height);
            width = Math.floor(width * ratio);
            height = Math.floor(height * ratio);
        }

        // Create canvas
        const canvas = document.createElement('canvas');
        canvas.width = width;
        canvas.height = height;

        const ctx = canvas.getContext('2d');
        
        // Use better image smoothing
        ctx.imageSmoothingEnabled = true;
        ctx.imageSmoothingQuality = 'high';
        
        // Draw image
        ctx.drawImage(img, 0, 0, width, height);

        // Determine output format
        let mimeType = this.mimeType;
        if (originalType === 'image/png' && this._hasTransparency(ctx, width, height)) {
            mimeType = 'image/png';
        }

        // Convert to blob
        return new Promise((resolve) => {
            canvas.toBlob((blob) => {
                if (!blob) {
                    throw new Error('Canvas to Blob failed');
                }
                
                // Create File object
                const file = new File([blob], filename, {
                    type: mimeType,
                    lastModified: Date.now()
                });
                
                resolve(file);
            }, mimeType, this.quality);
        });
    }

    /**
     * Check if image has transparency
     * @private
     */
    _hasTransparency(ctx, width, height) {
        try {
            const imageData = ctx.getImageData(0, 0, width, height);
            const data = imageData.data;
            
            // Sample every 10th pixel for performance
            for (let i = 3; i < data.length; i += 40) {
                if (data[i] < 255) {
                    return true;
                }
            }
            return false;
        } catch (e) {
            // SecurityError for cross-origin images
            return false;
        }
    }

    /**
     * Get compression statistics
     * @static
     */
    static getCompressionInfo(originalSize, compressedSize) {
        const saved = originalSize - compressedSize;
        const percent = Math.round((saved / originalSize) * 100);
        
        return {
            original: originalSize,
            compressed: compressedSize,
            saved: saved,
            percent: percent
        };
    }
}

// Export for use in modules or global scope
if (typeof module !== 'undefined' && module.exports) {
    module.exports = ImageCompressor;
}
