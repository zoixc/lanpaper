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
    this.preserveOriginal = !!options.preserveOriginal;
  }

  /**
   * Compress an image file
   * @param {File} file - The image file to compress
   * @returns {Promise<File>} Compressed image file
   */
  async compress(file) {
    // Preserve lossless mode, transparency, animation and uncommon formats.
    // The server validates and optimizes every supported image format.
    if (this.preserveOriginal || file.type !== 'image/jpeg') return file;

    const img = await this._loadImage(file);
    const compressed = await this._compressImage(img, file.name);
    return compressed.size < file.size ? compressed : file;
  }

  /**
   * Load image from file
   * @private
   */
  _loadImage(file) {
    return new Promise((resolve, reject) => {
      const url = URL.createObjectURL(file);
      const img = new Image();
      img.onload = () => { URL.revokeObjectURL(url); resolve(img); };
      img.onerror = () => { URL.revokeObjectURL(url); reject(new Error('Failed to load image')); };
      img.src = url;
    });
  }

  /**
   * Internal compression logic
   * @private
   */
  async _compressImage(img, fileName) {
    const canvas = document.createElement('canvas');
    const ctx = canvas.getContext('2d', { alpha: false });

    // Calculate new dimensions
    const ratio = Math.min(
      this.maxWidth / img.width,
      this.maxHeight / img.height,
      1 // Don't upscale
    );

    canvas.width = Math.floor(img.width * ratio);
    canvas.height = Math.floor(img.height * ratio);

    // Enable image smoothing for better quality
    ctx.imageSmoothingEnabled = true;
    ctx.imageSmoothingQuality = 'high';

    // Draw and compress
    ctx.drawImage(img, 0, 0, canvas.width, canvas.height);

    // Convert to blob
    const blob = await new Promise((resolve, reject) => {
      canvas.toBlob(
        (blob) => blob ? resolve(blob) : reject(new Error('Canvas to Blob conversion failed')),
        this.mimeType,
        this.quality
      );
    });

    // Create new file with compressed data
    const newFileName = fileName.replace(/\.[^.]+$/, '.jpg');
    return new File([blob], newFileName, {
      type: this.mimeType,
      lastModified: Date.now()
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
    const percent = originalSize > 0 ? Math.round((saved / originalSize) * 100) : 0;
    
    return {
      original: originalSize,
      compressed: compressedSize,
      saved,
      percent
    };
  }
}

// Export for use in app.js
if (typeof module !== 'undefined' && module.exports) {
  module.exports = ImageCompressor;
}
