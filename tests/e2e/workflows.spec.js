// SPDX-License-Identifier: MIT
'use strict';

const { test, expect } = require('@playwright/test');

const USER = 'e2e-admin';
const PASS = 'e2e-test-password-123';
const PNG = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=';

async function signIn(page) {
  await page.goto('/admin');
  await page.fill('input[name=username]', USER);
  await page.fill('input[name=password]', PASS);
  await page.click('#loginSubmit');
  await expect(page.locator('#library')).toBeVisible();
}

async function api(page, path, options = {}) {
  return page.evaluate(async ({ path, options }) => {
    const response = await fetch(path, options);
    const text = await response.text();
    return { status: response.status, body: text ? JSON.parse(text) : null };
  }, { path, options });
}

async function upload(page, name, mode = 'replace') {
  return page.evaluate(async ({ name, mode, encoded }) => {
    const bytes = Uint8Array.from(atob(encoded), (c) => c.charCodeAt(0));
    const form = new FormData();
    form.set('linkName', name);
    form.set('mode', mode);
    form.set('file', new Blob([bytes], { type: 'image/png' }), 'pixel.png');
    const response = await fetch('/api/upload', { method: 'POST', body: form });
    return { status: response.status, body: await response.text() };
  }, { name, mode, encoded: PNG });
}

test('create, upload, rename, access, history, playlist, export/import and delete', async ({ page }) => {
  await signIn(page);
  const suffix = Date.now().toString(36);
  const original = `flow-${suffix}`;
  const renamed = `renamed-${suffix}`;

  let result = await api(page, '/api/link', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ linkName: original, accessLevel: 'public' }),
  });
  expect(result.status).toBe(201);

  expect((await upload(page, original)).status).toBe(200);
  expect((await upload(page, original)).status).toBe(200);
  expect((await upload(page, original, 'append')).status).toBe(200);

  result = await api(page, `/api/link/${original}`, {
    method: 'PATCH', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ newLinkName: renamed }),
  });
  expect(result.status).toBe(200);

  result = await api(page, `/api/link/${renamed}`, {
    method: 'PATCH', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ accessLevel: 'token' }),
  });
  expect(result.status).toBe(200);
  const firstToken = result.body.accessToken;
  expect(firstToken).toBeTruthy();

  result = await api(page, `/api/link/${renamed}`, {
    method: 'PATCH', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ rotateToken: true }),
  });
  expect(result.status).toBe(200);
  expect(result.body.accessToken).not.toBe(firstToken);
  const currentToken = result.body.accessToken;

  result = await api(page, `/api/link/${renamed}/history`);
  expect(result.status).toBe(200);
  expect(result.body.history.length).toBeGreaterThan(0);

  // Refresh the panel's client-side state after the direct browser fetches,
  // then exercise the real settings export. Bearer tokens must never enter
  // the downloaded link-list file.
  await page.reload();
  await expect(page.locator('#library')).toBeVisible();
  await page.click('#settingsBtn');
  const downloadPromise = page.waitForEvent('download');
  await page.click('#exportBtn');
  const download = await downloadPromise;
  const stream = await download.createReadStream();
  let exportedText = '';
  for await (const chunk of stream) exportedText += chunk.toString();
  const exported = JSON.parse(exportedText);
  expect(exported.wallpapers.some((item) => item.linkName === renamed)).toBe(true);
  expect(exportedText).not.toContain(firstToken);
  expect(exportedText).not.toContain(currentToken);

  // Delete and import through the real browser module. Import deliberately
  // recreates link names only and asks for interactive confirmation.
  expect((await api(page, `/api/link/${renamed}`, { method: 'DELETE' })).status).toBe(204);
  await page.evaluate((text) => {
    const file = new File([text], 'links.json', { type: 'application/json' });
    window.LanpaperBackup.importData(file);
  }, exportedText);
  await expect(page.locator('#confirmTitle')).toBeVisible();
  await page.click('[data-confirm]');
  await expect.poll(async () => {
    const listed = await api(page, '/api/wallpapers');
    const links = Array.isArray(listed.body) ? listed.body : listed.body.data;
    return links.some((item) => item.linkName === renamed);
  }).toBe(true);
  expect((await api(page, `/api/link/${renamed}`, { method: 'DELETE' })).status).toBe(204);
});

test('logout expires every open tab without a Basic-auth fallback', async ({ page, context }) => {
  await signIn(page);
  const other = await context.newPage();
  await other.goto('/admin');
  await expect(other.locator('#library')).toBeVisible();

  await page.click('#settingsBtn');
  await page.click('#signOutBtn');
  await expect(page.locator('#loginForm')).toBeVisible();

  const response = await other.evaluate(() => fetch('/api/wallpapers').then((r) => r.status));
  expect(response).toBe(401);
  await other.reload();
  await expect(other.locator('#loginForm')).toBeVisible();
});
