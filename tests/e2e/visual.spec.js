// SPDX-License-Identifier: MIT
'use strict';

// Visual review: screenshots, not pixel assertions.
//
// Layout cannot be reviewed from a diff. The 0.13.0 redesign passed jsdom
// assertions, a cascade analysis and contrast checks and still broke in a
// real browser, which is why ROADMAP.md asks for a visual review gate. This
// spec shoots the states a reviewer needs — desktop, tablet and phone, both
// themes, a full and an empty library, the link panel and the settings sheet —
// into tests/e2e/screenshots/ and deliberately asserts nothing about pixels.
// CI uploads the directory as an artifact; locally, open the PNGs.
//
// Two things make the shots comparable between runs. The saved preferences
// (theme and language) are seeded before the first navigation, so neither the
// runner's locale nor its colour scheme changes what is on screen. And the
// library this spec looks at is its own: earlier specs leave links behind on
// the shared one-shot server, so every shot is taken with the filter set to
// this spec's name prefix.
const { test, expect } = require('@playwright/test');
const fs = require('node:fs');
const path = require('node:path');

const USER = 'e2e-admin';
const PASS = 'e2e-test-password-123';

// A prefix of its own: the grid in the shots contains exactly these links.
const PREFIX = 'visual-';
const SEED = [
  { name: 'visual-01', file: 'preview.mp4', type: 'video/mp4' },
  { name: 'visual-02', file: 'preview.webm', type: 'video/webm' },
  { name: 'visual-03', file: 'animated.gif', type: 'image/gif' },
  { name: 'visual-04', file: 'preview-portrait.webm', type: 'video/webm' },
  // Without media: the empty frame is a state of its own, and a library of
  // nothing but thumbnails hides it.
  { name: 'visual-05', file: null },
  { name: 'visual-06', file: null }
];
const VIEWPORTS = [
  { name: 'desktop-1440', viewport: { width: 1440, height: 900 } },
  { name: 'tablet-834', viewport: { width: 834, height: 1112 } },
  { name: 'phone-390', viewport: { width: 390, height: 844 } }
];
const THEMES = ['light', 'dark'];

async function signIn(page) {
  await page.goto('/admin');
  await page.fill('input[name=username]', USER);
  await page.fill('input[name=password]', PASS);
  await page.click('#loginSubmit');
  // A phone keeps its rail open — it carries the filters and the only copy
  // of the action buttons — so it is offered no collapse control at all.
  if (await page.evaluate(() => innerWidth > 760)) {
    await expect(page.locator('#studioNavToggle')).toBeVisible();
  } else {
    await expect(page.locator('#studioNavToggle')).toBeHidden();
    await expect(page.locator('#studioNavigation')).toBeVisible();
  }
}

// Best effort: a link that already exists (a rerun against a server that
// survived) or a fixture that changed shape must not fail a screenshot run.
async function seed(page) {
  for (const item of SEED) {
    const encoded = item.file
      ? fs.readFileSync(path.join(__dirname, 'fixtures', item.file)).toString('base64')
      : '';
    await page.evaluate(async ({ name, file, type, encoded }) => {
      await fetch('/api/link', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ linkName: name, accessLevel: 'public' })
      });
      if (!file) return;
      const form = new FormData();
      form.set('linkName', name);
      form.set('mode', 'replace');
      form.set('file', new Blob([Uint8Array.from(atob(encoded), (c) => c.charCodeAt(0))], { type }), file);
      await fetch('/api/upload', { method: 'POST', body: form });
    }, { name: item.name, file: item.file, type: item.type, encoded });
  }
}

// Empties the library so the shot shows the real first-run state rather than
// the "nothing found" block. Safe: every other spec creates the links it
// needs, and the server writes into a throwaway directory.
async function clearLibrary(page) {
  return page.evaluate(async () => {
    const names = [];
    for (let n = 1; ; n += 1) {
      const response = await fetch(`/api/wallpapers?page=${n}&page_size=200`);
      if (!response.ok) break;
      const data = await response.json();
      const batch = Array.isArray(data) ? data : (data && Array.isArray(data.data) ? data.data : []);
      names.push(...batch.map((link) => link.linkName).filter(Boolean));
      if (Array.isArray(data) || !data || n >= data.totalPages || batch.length === 0) break;
    }
    for (const name of names) {
      await fetch('/api/link/' + encodeURIComponent(name), { method: 'DELETE' });
    }
    return names.length;
  });
}

// Fonts loaded and every finite animation finished: otherwise a shot catches
// the panel mid-shimmer or with a fallback face.
async function settle(page) {
  await page.evaluate(async () => {
    await document.fonts.ready;
    await Promise.allSettled(document.getAnimations()
      .filter((animation) => animation.effect && animation.effect.getTiming().iterations !== Infinity)
      .map((animation) => animation.finished));
  });
}

for (const item of VIEWPORTS) {
  for (const theme of THEMES) {
    test(`${item.name} ${theme} library, panel, settings and empty state`, async ({ page }) => {
      // Six uploads and four page settlements, not a slow assertion: the
      // default timeout is sized for a functional test.
      test.slow();
      await page.setViewportSize(item.viewport);
      await page.addInitScript((saved) => {
        // prepaint.js reads lp-ui before the first paint, so the page comes up
        // in the wanted theme instead of flashing the system one.
        localStorage.setItem('lp-ui', JSON.stringify(Object.assign({
          palette: 'mono', lang: 'en', view: 'grid', sort: 'date_desc', scope: 'name'
        }, saved)));
      }, { theme });
      await signIn(page);
      await seed(page);

      const shot = (name) => page.screenshot({
        path: path.join('screenshots', `${name}-${item.name}-${theme}.png`)
      });

      // Полная библиотека.
      await page.reload();
      await page.fill('#searchInput', PREFIX);
      await expect(page.locator('#grid .card')).toHaveCount(SEED.length);
      await settle(page);
      await shot('library');

      // Пульт ссылки — с портретным видео: именно его кадр раньше обрезался
      // (см. docs/STUDIO_QA.md), и это единственный снимок, где видно посадку
      // медиа в панели. Видео само не грузится — только по нажатию.
      await page.locator('#grid .card[data-name="visual-04"] .card__name').click();
      const panel = page.locator('[data-sheet=panel]');
      await expect(panel).toBeVisible();
      await settle(page);
      await shot('panel');
      await page.keyboard.press('Escape');
      await expect(panel).toBeHidden();

      await page.click('#settingsBtn');
      await expect(page.locator('#settingsSheet')).toBeVisible();
      await settle(page);
      await shot('settings');
      await page.keyboard.press('Escape');
      await expect(page.locator('#settingsSheet')).toBeHidden();

      // Пустая библиотека — последней: ссылки удаляются, а снимки выше уже
      // сделаны. Если очистить не удалось, снимок просто пропускается.
      if (await clearLibrary(page) > 0) {
        await page.reload();
        await expect(page.locator('#emptyState')).toBeVisible();
        await settle(page);
        await shot('library-empty');
      }
    });
  }
}
