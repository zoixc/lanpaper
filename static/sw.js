/**
 * Lanpaper Service Worker
 * Provides offline caching and PWA functionality
 */

const CACHE_NAME = 'lanpaper-v1.0.0';
const RUNTIME_CACHE = 'lanpaper-runtime';

// Static assets to cache on install
const STATIC_ASSETS = [
  '/admin.html',
  '/static/css/style.css',
  '/static/css/settings-menu.css',
  '/static/js/app.js',
  '/static/js/export-import.js',
  '/static/js/settings-menu.js',
  '/static/js/compressor.js',
  '/static/logo.svg',
  '/static/logo-dark.svg',
  '/static/favicon.svg',
  '/static/manifest.json'
];

// Install event - cache static assets
self.addEventListener('install', (event) => {
  console.log('[SW] Installing service worker...');
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => {
        console.log('[SW] Caching static assets');
        // Cache assets individually to avoid failure on missing files
        return Promise.allSettled(
          STATIC_ASSETS.map(url =>
            fetch(url)
              .then(response => {
                if (response.ok) {
                  return cache.put(url, response);
                }
                console.warn('[SW] Failed to cache:', url, response.status);
                return null;
              })
              .catch(error => {
                console.warn('[SW] Error caching:', url, error.message);
                return null;
              })
          )
        );
      })
      .then(() => {
        console.log('[SW] Cache complete');
        return self.skipWaiting();
      })
      .catch(error => {
        console.error('[SW] Cache failed:', error);
      })
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
      .then(() => {
        console.log('[SW] Activation complete');
        return self.clients.claim();
      })
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
  if (STATIC_ASSETS.some(asset => url.pathname === asset) || url.pathname.startsWith('/static/')) {
    event.respondWith(
      caches.match(request)
        .then(cached => {
          if (cached) {
            // Return cached, but update in background
            fetch(request)
              .then(response => {
                if (response && response.status === 200) {
                  caches.open(CACHE_NAME)
                    .then(cache => cache.put(request, response.clone()));
                }
              })
              .catch(() => {}); // Silent fail for background update
            return cached;
          }
          // Not in cache, fetch from network
          return fetch(request)
            .then(response => {
              if (response && response.status === 200) {
                const clone = response.clone();
                caches.open(CACHE_NAME)
                  .then(cache => cache.put(request, clone))
                  .catch(() => {}); // Silent fail
              }
              return response;
            });
        })
        .catch(() => {
          // Fallback to offline page
          return caches.match('/admin.html');
        })
    );
    return;
  }

  // For images and dynamic content: Network first, cache fallback
  event.respondWith(
    fetch(request)
      .then(response => {
        if (response && response.status === 200) {
          const clone = response.clone();
          caches.open(RUNTIME_CACHE)
            .then(cache => cache.put(request, clone))
            .catch(() => {}); // Silent fail
        }
        return response;
      })
      .catch(() => caches.match(request))
  );
});

// Message event - for manual cache updates
self.addEventListener('message', (event) => {
  if (event.data && event.data.action === 'skipWaiting') {
    self.skipWaiting();
  }
  if (event.data && event.data.action === 'clearCache') {
    event.waitUntil(
      caches.keys().then(names => {
        console.log('[SW] Clearing all caches');
        return Promise.all(names.map(name => caches.delete(name)));
      })
    );
  }
});
