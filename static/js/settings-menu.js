// SPDX-License-Identifier: MIT
/**
 * The languages the panel ships, in menu order.
 *
 * This is the single definition: app.js fills the language list in Settings
 * from it, and tests/i18n.test.cjs reads it to check that every language has
 * exactly the same keys. Adding a translation means adding the file in
 * static/i18n/ and the code here — nowhere else.
 */
window.LANPAPER_LANGS = ['en', 'ru', 'de', 'fr', 'it', 'es'];
