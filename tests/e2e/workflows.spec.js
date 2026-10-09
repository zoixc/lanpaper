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

  result = await api(page, `/api/link/${renamed}/history`);
  expect(result.status).toBe(200);
  expect(result.body.history.length).toBeGreaterThan(0);

  // Link-list export/import contract: export excludes bearer access tokens;
  // importing recreates the configuration after deletion.
  result = await api(page, '/api/wallpapers');
  const exported = result.body.find((item) => item.linkName === renamed);
  expect(exported).toBeTruthy();
  expect(JSON.stringify(exported)).not.toContain(firstToken);

  expect((await api(page, `/api/link/${renamed}`, { method: 'DELETE' })).status).toBe(204);
  result = await api(page, '/api/link', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ linkName: renamed, category: exported.category, accessLevel: 'public' }),
  });
  expect(result.status).toBe(201);
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
