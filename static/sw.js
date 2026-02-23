// Service Worker for Lanpaper PWA v0.8.0
const CACHE_NAME = 'lanpaper-v0.8.0';
const RUNTIME_CACHE = 'lanpaper-runtime';

// Static assets to cache immediately on install
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

// Install event - cache static assets
self.addEventListener('install', event => {
  console.log('[SW] Installing Service Worker...');
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => {
        console.log('[SW] Caching static assets');
        return cache.addAll(STATIC_CACHE_URLS);
      })
      .then(() => self.skipWaiting())
  );
});

// Activate event - clean up old caches
self.addEventListener('activate', event => {
  console.log('[SW] Activating Service Worker...');
  event.waitUntil(
    caches.keys().then(cacheNames => {
      return Promise.all(
        cacheNames.map(cacheName => {
          if (cacheName !== CACHE_NAME && cacheName !== RUNTIME_CACHE) {
            console.log('[SW] Deleting old cache:', cacheName);
            return caches.delete(cacheName);
          }
        })
      );
    }).then(() => self.clients.claim())
  );
});

// Fetch event - smart caching strategies
self.addEventListener('fetch', event => {
  const { request } = event;
  const url = new URL(request.url);

  // Skip non-GET requests
  if (request.method !== 'GET') return;

  // API calls - network first, cache fallback
  if (url.pathname.startsWith('/api/')) {
    event.respondWith(
      fetch(request)
        .then(response => {
          if (response.ok) {
            const clone = response.clone();
            caches.open(RUNTIME_CACHE).then(cache => {
              cache.put(request, clone);
            });
          }
          return response;
        })
        .catch(() => caches.match(request))
    );
    return;
  }

  // Images/Videos - cache first strategy
  if (url.pathname.startsWith('/static/images/') || 
      url.pathname.match(/\.(jpg|jpeg|png|gif|webp|svg|mp4|webm)$/)) {
    event.respondWith(
      caches.match(request)
        .then(cached => {
          if (cached) return cached;
          return fetch(request).then(response => {
            if (response.ok) {
              const clone = response.clone();
              caches.open(RUNTIME_CACHE).then(cache => {
                cache.put(request, clone);
              });
            }
            return response;
          });
        })
    );
    return;
  }

  // Static assets - stale-while-revalidate
  event.respondWith(
    caches.match(request)
      .then(cached => {
        const fetchPromise = fetch(request)
          .then(response => {
            if (response.ok && url.origin === location.origin) {
              const clone = response.clone();
              caches.open(CACHE_NAME).then(cache => {
                cache.put(request, clone);
              });
            }
            return response;
          })
          .catch(() => {});

        return cached || fetchPromise;
      })
      .catch(() => {
        if (request.mode === 'navigate') {
          return caches.match('/admin');
        }
      })
  );
});

// Handle messages from clients
self.addEventListener('message', event => {
  if (event.data?.type === 'SKIP_WAITING') {
    self.skipWaiting();
  }
});

console.log('[SW] Service Worker loaded');
