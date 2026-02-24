// Service Worker for Lanpaper PWA
const CACHE_VERSION = 'v0.8.0';
const CACHE_NAME = `lanpaper-${CACHE_VERSION}`;
const RUNTIME_CACHE = 'lanpaper-runtime';

// Maximum number of entries kept in the runtime cache to bound storage usage.
const RUNTIME_CACHE_LIMIT = 60;

// Static assets to cache immediately on install.
const STATIC_CACHE_URLS = [
  '/admin',
  '/static/css/style.css',
  '/static/js/app.js',
  '/static/logo.svg',
  '/static/manifest.json',
  '/static/icons/favicon.png',
  '/static/icons/sun.png',
  '/static/icons/moon.png',
  '/static/icons/grid-dark.png',
  '/static/icons/grid-light.png',
  '/static/icons/list-dark.png',
  '/static/icons/list-light.png',
  '/static/icons/lang-dark.png',
  '/static/icons/lang-light.png',
  '/static/i18n/en.json',
  '/static/i18n/ru.json'
];

// Install: cache static assets and activate immediately.
self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => cache.addAll(STATIC_CACHE_URLS))
      .then(() => self.skipWaiting())
  );
});

// Activate: remove stale caches from previous versions.
self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys()
      .then(names => Promise.all(
        names
          .filter(n => n !== CACHE_NAME && n !== RUNTIME_CACHE)
          .map(n => caches.delete(n))
      ))
      .then(() => self.clients.claim())
  );
});

// Trim the runtime cache to at most RUNTIME_CACHE_LIMIT entries.
async function trimRuntimeCache() {
  const cache = await caches.open(RUNTIME_CACHE);
  const keys = await cache.keys();
  if (keys.length > RUNTIME_CACHE_LIMIT) {
    await Promise.all(keys.slice(0, keys.length - RUNTIME_CACHE_LIMIT).map(k => cache.delete(k)));
  }
}

// Fetch: strategy varies by resource type.
self.addEventListener('fetch', event => {
  const { request } = event;
  const url = new URL(request.url);

  if (request.method !== 'GET') return;

  // API calls — network first, stale cache fallback.
  if (url.pathname.startsWith('/api/')) {
    event.respondWith(
      fetch(request)
        .then(response => {
          if (response.ok) {
            const clone = response.clone();
            caches.open(RUNTIME_CACHE).then(cache => {
              cache.put(request, clone);
              trimRuntimeCache();
            });
          }
          return response;
        })
        .catch(() => caches.match(request))
    );
    return;
  }

  // Images/Videos — cache first, network fallback.
  if (url.pathname.startsWith('/static/images/') ||
      url.pathname.match(/\.(jpg|jpeg|png|gif|webp|svg|mp4|webm)$/)) {
    event.respondWith(
      caches.match(request).then(cached => {
        if (cached) return cached;
        return fetch(request).then(response => {
          if (response.ok) {
            const clone = response.clone();
            caches.open(RUNTIME_CACHE).then(cache => {
              cache.put(request, clone);
              trimRuntimeCache();
            });
          }
          return response;
        });
      })
    );
    return;
  }

  // Static assets — stale-while-revalidate.
  event.respondWith(
    caches.match(request).then(cached => {
      const fetchPromise = fetch(request)
        .then(response => {
          if (response.ok && url.origin === self.location.origin) {
            const clone = response.clone();
            caches.open(CACHE_NAME).then(cache => cache.put(request, clone));
          }
          return response;
        })
        .catch(() => {});

      return cached || fetchPromise;
    }).catch(() => {
      if (request.mode === 'navigate') {
        return caches.match('/admin');
      }
    })
  );
});

// Handle messages from clients.
self.addEventListener('message', event => {
  if (event.data?.type === 'SKIP_WAITING') {
    self.skipWaiting();
  }
});
