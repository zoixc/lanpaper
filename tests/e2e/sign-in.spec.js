// SPDX-License-Identifier: MIT
'use strict';

const { test, expect } = require('@playwright/test');

const USER = 'e2e-admin';
const PASS = 'e2e-test-password-123';

async function signIn(page, password = PASS) {
  await page.goto('/admin');
  await page.fill('input[name=username]', USER);
  await page.fill('input[name=password]', password);
  await page.click('#loginSubmit');
}

test('signed-out visit to /admin shows the sign-in form', async ({ page }) => {
  await page.goto('/admin');
  await expect(page.locator('#loginForm')).toBeVisible();
  await expect(page.locator('#library')).toBeHidden();
  expect(new URL(page.url()).pathname).toBe('/admin');
});

test('wrong password keeps the form and shows an error', async ({ page }) => {
  await signIn(page, 'not-the-password');
  await expect(page.locator('#loginError')).toBeVisible();
  await expect(page.locator('#loginForm')).toBeVisible();
  await expect(page.locator('#library')).toBeHidden();
  await expect(page.locator('input[name=password]')).toHaveValue('');
});

test('correct password opens the admin panel', async ({ page }) => {
  await signIn(page);
  await expect(page.locator('#library')).toBeVisible();
  await expect(page.locator('#loginForm')).toHaveCount(0);
});

test('session survives a reload and a new page in the same browser', async ({ page, context }) => {
  await signIn(page);
  await expect(page.locator('#library')).toBeVisible();

  await page.reload();
  await expect(page.locator('#library')).toBeVisible();

  const other = await context.newPage();
  await other.goto('/admin');
  await expect(other.locator('#library')).toBeVisible();
  await other.close();
});

test('the session cookie is HttpOnly and SameSite=Lax', async ({ page, context }) => {
  await signIn(page);
  await expect(page.locator('#library')).toBeVisible();
  const cookies = await context.cookies();
  const session = cookies.find((c) => c.name === 'lanpaper_session');
  expect(session, 'session cookie is set').toBeTruthy();
  expect(session.httpOnly).toBe(true);
  expect(session.sameSite).toBe('Lax');
  expect(session.path).toBe('/');
});

test('sign out on all devices asks for confirmation and ends the session', async ({ page, context }) => {
  await signIn(page);
  await expect(page.locator('#library')).toBeVisible();

  await page.click('#settingsBtn');
  await page.click('#signOutAllBtn');
  await expect(page.locator('#confirmTitle')).toBeVisible();
  await page.click('[data-confirm]');

  await expect(page.locator('#loginForm')).toBeVisible();
  const cookies = await context.cookies();
  expect(cookies.find((c) => c.name === 'lanpaper_session')).toBeUndefined();
  const res = await page.request.get('/api/wallpapers');
  expect(res.status()).toBe(401);
});

test('sign out returns to the form and ends the session', async ({ page, context }) => {
  await signIn(page);
  await expect(page.locator('#library')).toBeVisible();

  await page.click('#settingsBtn');
  await page.click('#signOutBtn');

  await expect(page.locator('#loginForm')).toBeVisible();
  await expect(page.locator('#library')).toBeHidden();
  const cookies = await context.cookies();
  expect(cookies.find((c) => c.name === 'lanpaper_session')).toBeUndefined();

  // The admin API must refuse requests once the session is gone.
  const res = await page.request.get('/api/wallpapers');
  expect(res.status()).toBe(401);
});
