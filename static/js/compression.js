/**
 * Image Compression Utility
 * Compresses images before upload using Canvas API
 */

class ImageCompressor {
  constructor(options = {}) {
    this.maxWidth = options.maxWidth || 1920;
    this.maxHeight = options.maxHeight || 1080;
    this.quality = options.quality || 0.85;
    this.format = options.format || 'image/jpeg';
  }

  /**
   * Compress an image file
   * @param {File} file - Image file to compress
   * @returns {Promise<File>} Compressed image file
   */
  async compress(file) {
    // Skip if not an image
    if (!file.type.startsWith('image/')) {
      return file;
    }

    // Skip if already small (< 500KB)
    if (file.size < 500 * 1024) {
      return file;
    }

    try {
      const bitmap = await createImageBitmap(file);
      const { width, height } = this.calculateDimensions(bitmap.width, bitmap.height);

      const canvas = new OffscreenCanvas(width, height);
      const ctx = canvas.getContext('2d');
      ctx.drawImage(bitmap, 0, 0, width, height);

      const blob = await canvas.convertToBlob({
        type: this.format,
        quality: this.quality
      });

      const compressedFile = new File(
        [blob],
        file.name.replace(/\.[^.]+$/, '.jpg'),
        { type: this.format }
      );

      // Return original if compression made it larger
      return compressedFile.size < file.size ? compressedFile : file;
    } catch (error) {
      console.warn('Compression failed, using original:', error);
      return file;
    }
  }

  /**
   * Calculate new dimensions maintaining aspect ratio
   */
  calculateDimensions(width, height) {
    if (width <= this.maxWidth && height <= this.maxHeight) {
      return { width, height };
    }

    const ratio = Math.min(this.maxWidth / width, this.maxHeight / height);
    return {
      width: Math.round(width * ratio),
      height: Math.round(height * ratio)
    };
  }

  /**
   * Get compression info for display
   */
  static getCompressionInfo(originalSize, compressedSize) {
    const savedBytes = originalSize - compressedSize;
    const savedPercent = Math.round((savedBytes / originalSize) * 100);
    return {
      saved: savedBytes,
      percent: savedPercent,
      original: originalSize,
      compressed: compressedSize
    };
  }
}

// Export for use in app.js
if (typeof module !== 'undefined' && module.exports) {
  module.exports = ImageCompressor;
}
