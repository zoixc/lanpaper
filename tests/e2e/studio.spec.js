// SPDX-License-Identifier: MIT
'use strict';
const { test, expect } = require('@playwright/test');
const fs = require('node:fs');
const path = require('node:path');
async function signIn(page) {
  await page.goto('/admin');
  await page.fill('[name=username]', 'e2e-admin');
  await page.fill('[name=password]', 'e2e-test-password-123');
  await page.click('#loginSubmit');
  await expect(page.locator('#studioNavToggle')).toBeVisible();
}
async function media(page, name, file, type) {
  const bytes = fs.readFileSync(path.join(__dirname, 'fixtures', file)).toString('base64');
  const result = await page.evaluate(async ({ name, file, type, bytes }) => {
    const created = await fetch('/api/link', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ linkName: name, accessLevel: 'auth' }) });
    const form = new FormData();
    form.set('linkName', name); form.set('mode', 'replace');
    form.set('file', new Blob([Uint8Array.from(atob(bytes), c => c.charCodeAt(0))], { type }), file);
    const uploaded = await fetch('/api/upload', { method: 'POST', body: form });
    return [created.status, uploaded.status];
  }, { name, file, type, bytes });
  expect(result).toEqual([201, 200]);
}

test('palette name exists on first mount, stays centered, and updates without moving swatches', async ({ page }) => {
  await signIn(page);
  await page.evaluate(() => {
    window.paletteAtMount = null;
    new MutationObserver(() => {
      const el = document.getElementById('paletteName');
      if (el && window.paletteAtMount === null) window.paletteAtMount = el.textContent;
    }).observe(document.getElementById('overlayHost'), { childList: true, subtree: true });
  });
  await page.click('#settingsBtn');
  expect(await page.evaluate(() => window.paletteAtMount)).toBe('Mono');
  await page.locator('#settingsSheet').evaluate(async el => {
    await Promise.all(el.getAnimations().map(a => a.finished));
    await document.fonts.ready;
  });
  const row = page.locator('#paletteRow');
  const before = await row.boundingBox();
  const label = await page.locator('#paletteName').boundingBox();
  expect(Math.abs(label.y + label.height / 2 - before.y - before.height / 2)).toBeLessThan(1.5);
  await page.locator('#paletteRow input[value=ocean]').check();
  await expect(page.locator('#paletteName')).toHaveText('Ocean');
  expect((await row.boundingBox()).x).toBeCloseTo(before.x, 0);
  expect((await row.boundingBox()).y).toBeCloseTo(before.y, 0);
  await page.selectOption('#langSelect', 'de');
  await expect(page.locator('#paletteName')).not.toHaveText('Ocean');
});

test('navigation collapse retains controls, translations and preference across reloads', async ({ page }) => {
  await signIn(page);
  if (await page.evaluate(() => innerWidth <= 760)) await expect(page.locator('.catalog-label')).toBeHidden();
  await page.click('#studioNavToggle');
  await expect(page.locator('#studioNavigation')).toBeHidden();
  for (const id of ['newLinkBtn', 'themeBtn', 'settingsBtn']) await expect(page.locator(`#${id}`)).toBeVisible();
  await page.click('#settingsBtn');
  await page.selectOption('#langSelect', 'de');
  await page.locator('#settingsSheet [data-close]').first().click();
  await expect(page.locator('#studioNavToggle')).toHaveAttribute('aria-label', 'Navigation ausklappen');
  await page.reload();
  await expect(page.locator('#studioNavToggle')).toHaveAttribute('aria-expanded', 'false');
  await page.click('#studioNavToggle');
  await expect(page.locator('#studioNavigation')).toBeVisible();
  await expect(page.locator('#appVersion')).toHaveText('v' + fs.readFileSync(path.join(__dirname, '../../VERSION'), 'utf8').trim());
});

for (const [ext, type, file] of [['webm', 'video/webm', 'preview.webm'], ['mp4', 'video/mp4', 'preview.mp4'], ['portrait', 'video/webm', 'preview-portrait.webm']]) {
  test(`${ext}: no eager media requests; authenticated player runs on click and releases on tab/close`, async ({ page, browser }) => {
    await signIn(page);
    const name = `video-${ext}-${Date.now().toString(36)}`;
    await media(page, name, file, type);
    const requests = [];
    page.on('request', req => { if (new URL(req.url()).pathname === `/api/preview/${name}`) requests.push(req); });
    await page.reload();
    await page.fill('#searchInput', name);
    const card = page.locator(`#grid .card[data-name="${name}"]`);
    await expect(card).toBeVisible();
    expect(requests).toHaveLength(0);
    await card.locator('.card__name').click();
    await expect(page.locator('.video-preview__start')).toBeVisible();
    expect(requests).toHaveLength(0);
    await expect(page.locator('video')).toHaveCount(0);
    await page.click('.video-preview__start');
    const player = page.locator('video');
    await expect(player).toBeVisible();
    await expect.poll(() => player.evaluate(v => v.currentTime)).toBeGreaterThan(0);
    const geometry = await player.evaluate(v => {
      const video = v.getBoundingClientRect(), frame = v.closest('.panel-preview').getBoundingClientRect();
      return { clipped: video.bottom > frame.bottom + 1 || video.height > frame.height, controls: v.controls, fit: getComputedStyle(v).objectFit };
    });
    expect(geometry).toEqual({ clipped: false, controls: true, fit: 'contain' });
    expect(requests.length).toBeGreaterThan(0);
    expect(requests.every(req => !req.url().includes('token'))).toBe(true);
    await page.evaluate(() => { window.previousPlayer = document.querySelector('video'); });
    await page.click('[data-tab="access"]');
    expect(await page.evaluate(() => [window.previousPlayer.paused, window.previousPlayer.getAttribute('src')])).toEqual([true, null]);
    await page.click('[data-tab="media"]');
    await expect(page.locator('video')).toHaveCount(0);
    await page.click('.video-preview__start');
    await page.evaluate(() => { window.previousPlayer = document.querySelector('video'); });
    await page.locator('[data-sheet=panel] [data-close]').first().click();
    expect(await page.evaluate(() => [window.previousPlayer.paused, window.previousPlayer.getAttribute('src')])).toEqual([true, null]);
    const ranged = await page.request.get(`/api/preview/${name}`, { headers: { Range: 'bytes=0-63' } });
    expect(ranged.status()).toBe(206);
    expect(ranged.headers()['cache-control']).toBe('private, no-cache');
    expect((await ranged.body()).length).toBe(64);
    const cachedAPI = await page.evaluate(async () => {
      const entries = await Promise.all((await caches.keys()).map(async key => (await (await caches.open(key)).keys()).map(req => req.url)));
      return entries.flat().filter(url => new URL(url).pathname.startsWith('/api/'));
    });
    expect(cachedAPI).toEqual([]);
    const anonymous = await browser.newContext();
    try { expect((await anonymous.request.get(new URL(`/api/preview/${name}`, page.url()).href)).status()).toBe(401); }
    finally { await anonymous.close(); }
  });
}

test('GIF upload produces a static WebP thumbnail', async ({ page }) => {
  await signIn(page);
  const name = `gif-${Date.now().toString(36)}`;
  await media(page, name, 'animated.gif', 'image/gif');
  const preview = await page.request.get(`/api/preview/${name}`);
  expect(preview.status()).toBe(200);
  expect(preview.headers()['content-type']).toBe('image/webp');
  const bytes = await preview.body();
  expect(bytes.subarray(8, 12).toString()).toBe('WEBP');
  expect(bytes.includes(Buffer.from('ANIM'))).toBe(false);
});

test('touch scroll is independent of grip; grip swipe closes and restores focus', async ({ page, context }, info) => {
  test.skip(info.project.name !== 'phone', 'CDP touch input on Chromium phone emulation');
  await signIn(page);
  await page.click('#settingsBtn');
  const sheet = page.locator('#settingsSheet');
  const body = page.locator('#settingsSheet .sheet__body');
  const client = await context.newCDPSession(page);
  async function swipe(x, y, dy) {
    await client.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] });
    for (let i = 1; i <= 6; i++) await client.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x, y: y + dy * i / 6 }] });
    await client.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  }
  const b = await body.boundingBox();
  await swipe(b.x + b.width - 8, b.y + b.height * .7, -100);
  await expect.poll(() => body.evaluate(el => el.scrollTop)).toBeGreaterThan(0);
  await expect(sheet).toBeVisible();
  const grip = await sheet.locator('.sheet__grip').boundingBox();
  expect(grip.height).toBeGreaterThanOrEqual(44);
  await swipe(grip.x + grip.width / 2, grip.y + grip.height / 2, 100);
  await expect(sheet).toBeHidden();
  await expect(page.locator('#settingsBtn')).toBeFocused();
  expect(await page.evaluate(() => document.body.style.overflow)).toBe('');
});

test('unplayable video reports an error without leaving a player or breaking the panel', async ({ page }) => {
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await signIn(page);
  const name = `bad-video-${Date.now().toString(36)}`;
  await media(page, name, 'preview.webm', 'video/webm');
  await page.route(`**/api/preview/${name}`, route => route.fulfill({ status: 200, contentType: 'video/webm', body: 'invalid video' }));
  await page.reload();
  await page.fill('#searchInput', name);
  await page.locator(`#grid .card[data-name="${name}"] .card__name`).click();
  await page.click('.video-preview__start');
  await expect(page.locator('.video-preview [role=status]')).toContainText('cannot be played');
  await expect(page.locator('video')).toHaveCount(0);
  await page.locator('[data-sheet=panel] [data-close]').first().click();
  expect(errors).toEqual([]);
});
