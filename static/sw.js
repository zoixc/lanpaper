/**
 * Lanpaper Service Worker
 * Provides offline caching and PWA functionality
 */

const CACHE_NAME = 'lanpaper-v1.0.0';
const RUNTIME_CACHE = 'lanpaper-runtime';

// Static assets to cache on install
const STATIC_ASSETS = [
  '/',
  '/admin.html',
  '/static/css/style.css',
  '/static/js/app.js',
  '/static/logo.svg',
  '/static/logo-dark.svg',
  '/static/favicon.svg',
];

// Install event - cache static assets
self.addEventListener('install', (event) => {
  console.log('[SW] Installing service worker...');
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => {
        console.log('[SW] Caching static assets');
        return cache.addAll(STATIC_ASSETS);
      })
      .then(() => self.skipWaiting())
  );
});

// Activate event - clean up old caches
self.addEventListener('activate', (event) => {
  console.log('[SW] Activating service worker...');
  event.waitUntil(
    caches.keys()
      .then(cacheNames => {
        return Promise.all(
          cacheNames
            .filter(name => name !== CACHE_NAME && name !== RUNTIME_CACHE)
            .map(name => {
              console.log('[SW] Deleting old cache:', name);
              return caches.delete(name);
            })
        );
      })
      .then(() => self.clients.claim())
  );
});

// Fetch event - network first, then cache
self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);

  // Skip non-GET requests
  if (request.method !== 'GET') return;

  // Skip API calls (always fetch fresh)
  if (url.pathname.startsWith('/api/')) return;

  // For static assets: Cache first, then network
  if (STATIC_ASSETS.some(asset => url.pathname === asset || url.pathname.startsWith('/static/'))) {
    event.respondWith(
      caches.match(request)
        .then(cached => {
          if (cached) return cached;
          return fetch(request)
            .then(response => {
              if (response.status === 200) {
                const clone = response.clone();
                caches.open(CACHE_NAME)
                  .then(cache => cache.put(request, clone));
              }
              return response;
            });
        })
        .catch(() => caches.match('/admin.html')) // Fallback to offline page
    );
    return;
  }

  // For images and dynamic content: Network first, cache fallback
  event.respondWith(
    fetch(request)
      .then(response => {
        if (response.status === 200) {
          const clone = response.clone();
          caches.open(RUNTIME_CACHE)
            .then(cache => cache.put(request, clone));
        }
        return response;
      })
      .catch(() => caches.match(request))
  );
});

// Message event - for manual cache updates
self.addEventListener('message', (event) => {
  if (event.data.action === 'skipWaiting') {
    self.skipWaiting();
  }
  if (event.data.action === 'clearCache') {
    event.waitUntil(
      caches.keys().then(names => Promise.all(names.map(name => caches.delete(name))))
    );
  }
});
