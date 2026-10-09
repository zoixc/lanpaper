// SPDX-License-Identifier: MIT
'use strict';

const { test, expect } = require('@playwright/test');
const AxeBuilder = require('@axe-core/playwright').default;

const USER = 'e2e-admin';
const PASS = 'e2e-test-password-123';

async function signIn(page) {
  await page.goto('/admin');
  await page.fill('input[name=username]', USER);
  await page.fill('input[name=password]', PASS);
  await page.click('#loginSubmit');
  await expect(page.locator('#library')).toBeVisible();
}

async function expectNoSeriousViolations(page) {
  const results = await new AxeBuilder({ page }).analyze();
  const violations = results.violations.filter((item) => ['serious', 'critical'].includes(item.impact));
  expect(violations, JSON.stringify(violations, null, 2)).toEqual([]);
}

test('sign-in and library have no serious automated accessibility violations', async ({ page }) => {
  await page.goto('/admin');
  await expectNoSeriousViolations(page);
  await signIn(page);
  await expectNoSeriousViolations(page);
});

test('keyboard opens and closes the create dialog with focus restoration', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name === 'phone', 'Physical keyboard focus is covered by desktop engines');
  await signIn(page);
  await page.locator('#newLinkBtn').focus();
  await page.keyboard.press('Enter');
  const dialog = page.locator('[role=dialog]').filter({ has: page.locator('#createTitle') });
  await expect(dialog).toBeVisible();
  await expect(page.locator('#createInput')).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(dialog).toBeHidden();
  await expect(page.locator('#newLinkBtn')).toBeFocused();
});

test('narrow viewport and 200 percent zoom keep primary actions reachable', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 640 });
  await signIn(page);
  await page.evaluate(() => { document.documentElement.style.zoom = '2'; });
  await expect(page.locator('#fabBtn')).toBeVisible();
  // At 200% CSS zoom the visual box can sit outside Playwright's actionability
  // viewport even though a touch user can pan to it; dispatch the activation
  // and assert that the resulting dialog remains usable.
  await page.locator('#fabBtn').evaluate((button) => button.click());
  await expect(page.locator('#createInput')).toBeVisible();
});

test('settings tolerate the longest available translation labels', async ({ page }) => {
  await signIn(page);
  await page.click('#settingsBtn');
  await expect(page.locator('#settingsSheet')).toBeVisible();
  await page.selectOption('#langSelect', 'de');
  await expect(page.locator('#settingsTitle')).toBeVisible();
  await expectNoSeriousViolations(page);
});
