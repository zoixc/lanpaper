// SPDX-License-Identifier: MIT
'use strict';

const { defineConfig, devices } = require('@playwright/test');

const PORT = Number(process.env.PORT || 18090);

module.exports = defineConfig({
  testDir: '.',
  testMatch: '*.spec.js',
  timeout: 30000,
  // The sign-in tests share one server and its per-IP failure counter.
  workers: 1,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    trace: 'retain-on-failure',
    launchOptions: process.env.CHROMIUM_PATH
      ? { executablePath: process.env.CHROMIUM_PATH }
      : {},
  },
  webServer: {
    command: 'sh serve.sh',
    url: `http://127.0.0.1:${PORT}/health`,
    reuseExistingServer: false,
    timeout: 30000,
  },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    {
      // Home-screen web apps on iOS run in a standalone window with a
      // phone-sized viewport; the sign-in form must work there too.
      name: 'phone',
      use: { ...devices['Pixel 5'] },
    },
  ],
});
