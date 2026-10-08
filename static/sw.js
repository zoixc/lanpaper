// SPDX-License-Identifier: MIT
/* Lanpaper service worker: public application assets only.
 * Admin/API responses and mutable media links must always reach the server.
 * In particular, an image that was public yesterday must not be replayed
 * offline after its link is changed to token-only or admin-only.
 */
const STATIC_CACHE = 'lanpaper-static-v10';
const STATIC_ASSETS = [
  '/static/css/style.css',
  '/static/js/prepaint.js',
  '/static/js/app.js',
  '/static/js/export-import.js',
  '/static/js/settings-menu.js',
  '/static/js/compressor.js',
  '/static/logo.svg',
  '/static/logo-dark.svg',
  '/static/favicon.svg',
  '/static/manifest.json',
  '/static/icons/apple-touch-icon.png',
  '/static/icons/icon-192.png',
  '/static/icons/icon-512.png',
  '/static/icons/icon-maskable-512.png',
  '/static/fonts/golos-text-latin.woff2',
  '/static/fonts/golos-text-cyrillic.woff2',
  '/static/i18n/en.json',
  '/static/i18n/ru.json',
  '/static/i18n/de.json',
  '/static/i18n/fr.json',
  '/static/i18n/it.json',
  '/static/i18n/es.json'
];

function cacheable(response) {
  if (!response || !response.ok || response.status !== 200) return false;
  const type = response.headers.get('content-type') || '';
  const control = (response.headers.get('cache-control') || '').toLowerCase();
  return !control.includes('no-store') && !control.includes('private') &&
    ['text/css', 'application/javascript', 'text/javascript', 'application/json',
     'image/', 'font/', 'application/font'].some(prefix => type.startsWith(prefix));
}

self.addEventListener('install', event => {
  event.waitUntil((async () => {
    const cache = await caches.open(STATIC_CACHE);
    await Promise.allSettled(STATIC_ASSETS.map(async path => {
      const response = await fetch(path, { cache: 'no-cache', credentials: 'same-origin' });
      if (cacheable(response)) await cache.put(path, response);
    }));
    await self.skipWaiting();
  })());
});

self.addEventListener('activate', event => {
  event.waitUntil((async () => {
    // Purge all old Lanpaper caches, including earlier runtime caches that
    // may contain public media or a credentialed /admin response.
    const names = await caches.keys();
    await Promise.all(names.filter(name => name.startsWith('lanpaper-') &&
      name !== STATIC_CACHE).map(name => caches.delete(name)));
    await self.clients.claim();
  })());
});

self.addEventListener('fetch', event => {
  const request = event.request;
  const url = new URL(request.url);
  if (request.method !== 'GET' || url.origin !== self.location.origin ||
      !url.pathname.startsWith('/static/') ||
      url.pathname.startsWith('/static/images/')) return;

  // Never use a cached response when the server is reachable: static code
  // changes on upgrades and its URLs are not content-hashed.
  event.respondWith((async () => {
    try {
      const response = await fetch(request);
      if (cacheable(response)) {
        const cache = await caches.open(STATIC_CACHE);
        await cache.put(request, response.clone());
      }
      return response;
    } catch (_) {
      return await caches.match(request) || Response.error();
    }
  })());
});
