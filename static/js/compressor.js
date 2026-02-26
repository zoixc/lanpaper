/**
 * Simple Image Compressor
 * Client-side image compression before upload
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
   * @returns {Promise<File>} Compressed image file
   */
  async compress(file) {
    // Skip if not an image
    if (!file.type.startsWith('image/')) {
      return file;
    }

    // Skip if already compressed format
    if (file.type === 'image/webp' || file.type === 'image/avif') {
      return file;
    }

    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      
      reader.onload = (e) => {
        const img = new Image();
        
        img.onload = () => {
          try {
            const compressed = this._compressImage(img, file.name, file.type);
            resolve(compressed);
          } catch (error) {
            reject(error);
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
   * Internal compression logic
   * @private
   */
  _compressImage(img, fileName, originalType) {
    const canvas = document.createElement('canvas');
    const ctx = canvas.getContext('2d');

    // Calculate new dimensions
    let { width, height } = img;
    const ratio = Math.min(
      this.maxWidth / width,
      this.maxHeight / height,
      1 // Don't upscale
    );

    canvas.width = Math.floor(width * ratio);
    canvas.height = Math.floor(height * ratio);

    // Enable image smoothing for better quality
    ctx.imageSmoothingEnabled = true;
    ctx.imageSmoothingQuality = 'high';

    // Draw and compress
    ctx.drawImage(img, 0, 0, canvas.width, canvas.height);

    // Convert to blob
    return new Promise((resolve, reject) => {
      canvas.toBlob(
        (blob) => {
          if (!blob) {
            reject(new Error('Canvas to Blob conversion failed'));
            return;
          }

          // Create new file with compressed data
          const newFileName = fileName.replace(/\.[^.]+$/, '.jpg');
          const compressedFile = new File([blob], newFileName, {
            type: this.mimeType,
            lastModified: Date.now()
          });

          resolve(compressedFile);
        },
        this.mimeType,
        this.quality
      );
    });
  }

  /**
   * Get compression statistics
   * @param {number} originalSize - Original file size in bytes
   * @param {number} compressedSize - Compressed file size in bytes
   * @returns {Object} Compression info
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

// Export for use in app.js
if (typeof module !== 'undefined' && module.exports) {
  module.exports = ImageCompressor;
}
