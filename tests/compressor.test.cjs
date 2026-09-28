const assert = require('node:assert/strict');
const test = require('node:test');
const ImageCompressor = require('../static/js/compressor.js');

test('browser preprocessing preserves lossless mode, animated and transparent formats', async () => {
  const compressor = new ImageCompressor({ preserveOriginal: true });
  const jpeg = { type: 'image/jpeg', size: 100, name: 'photo.jpg' };
  assert.equal(await compressor.compress(jpeg), jpeg);
  compressor.preserveOriginal = false;
  for (const type of ['image/png', 'image/gif', 'image/webp', 'image/avif',
    'image/tiff', 'image/bmp', 'video/mp4']) {
    const file = { type, size: 100, name: 'file' };
    assert.equal(await compressor.compress(file), file, `${type} changed client-side`);
  }
});

test('client sends original JPEG when recompression would make it bigger', async () => {
  const compressor = new ImageCompressor();
  const original = { type: 'image/jpeg', size: 100, name: 'photo.jpg' };
  const compressed = { type: 'image/jpeg', size: 110, name: 'photo.jpg' };
  compressor._loadImage = async () => ({ width: 4000, height: 3000 });
  compressor._compressImage = async () => compressed;
  assert.equal(await compressor.compress(original), original);
  compressed.size = 70;
  assert.equal(await compressor.compress(original), compressed);
  assert.equal(ImageCompressor.getCompressionInfo(100, 70).percent, 30);
  assert.equal(ImageCompressor.getCompressionInfo(0, 0).percent, 0);
});

test('client keeps the original resolution unless a cap is requested', () => {
  // The server applies COMPRESSION_SCALE; a browser-side cap would silently
  // downscale uploads made through the admin panel (and scale them twice).
  const compressor = new ImageCompressor({ quality: 0.85 });
  assert.equal(compressor.maxWidth, Infinity);
  assert.equal(compressor.maxHeight, Infinity);
  const capped = new ImageCompressor({ maxWidth: 800, maxHeight: 600 });
  assert.equal(capped.maxWidth, 800);
  assert.equal(capped.maxHeight, 600);
});

test('images too large for a browser canvas are uploaded unchanged', async () => {
  const compressor = new ImageCompressor();
  const original = { type: 'image/jpeg', size: 100, name: 'huge.jpg' };
  compressor._loadImage = async () => ({ width: 8000, height: 6000 });
  compressor._compressImage = async () => { throw new Error('must not draw an oversized canvas'); };
  assert.equal(await compressor.compress(original), original);
  compressor._loadImage = async () => ({ width: 0, height: 0 });
  assert.equal(await compressor.compress(original), original);
});
