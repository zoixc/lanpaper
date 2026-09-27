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
  compressor._loadImage = async () => ({});
  compressor._compressImage = async () => compressed;
  assert.equal(await compressor.compress(original), original);
  compressed.size = 70;
  assert.equal(await compressor.compress(original), compressed);
  assert.equal(ImageCompressor.getCompressionInfo(100, 70).percent, 30);
  assert.equal(ImageCompressor.getCompressionInfo(0, 0).percent, 0);
});
