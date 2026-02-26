/**
 * Lanpaper Service Worker
 * Provides offline caching and PWA functionality
 * Following OWASP and W3C 2026 security best practices
 */

const CACHE_NAME = 'lanpaper-v1.0.0';
const RUNTIME_CACHE = 'lanpaper-runtime';
const CACHE_MAX_AGE = 24 * 60 * 60 * 1000; // 24 hours
const RUNTIME_CACHE_MAX_SIZE = 50; // Max number of runtime cached items

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

// Install event - cache static assets with integrity checks
self.addEventListener('install', (event) => {
  console.log('[SW] Installing service worker...');
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => {
        console.log('[SW] Caching static assets');
        // Cache assets individually to handle missing files gracefully
        return Promise.allSettled(
          STATIC_ASSETS.map(url =>
            fetch(url, {
              cache: 'no-cache', // Always fetch fresh during install
              credentials: 'same-origin' // Security: only same-origin
            })
              .then(response => {
                // Only cache successful responses
                if (response && response.ok && response.status === 200) {
                  // Verify content type is safe
                  const contentType = response.headers.get('content-type') || '';
                  const safeTypes = ['text/', 'application/json', 'application/javascript', 'image/', 'font/'];
                  if (safeTypes.some(type => contentType.startsWith(type))) {
                    return cache.put(url, response);
                  }
                  console.warn('[SW] Skipped unsafe content-type:', url, contentType);
                  return null;
                }
                console.warn('[SW] Failed to cache:', url, response?.status);
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
    Promise.all([
      // Delete old caches
      caches.keys().then(cacheNames => {
        return Promise.all(
          cacheNames
            .filter(name => name !== CACHE_NAME && name !== RUNTIME_CACHE)
            .map(name => {
              console.log('[SW] Deleting old cache:', name);
              return caches.delete(name);
            })
        );
      }),
      // Trim runtime cache to max size
      trimRuntimeCache()
    ]).then(() => {
      console.log('[SW] Activation complete');
      return self.clients.claim();
    })
  );
});

// Trim runtime cache to prevent unlimited growth
async function trimRuntimeCache() {
  const cache = await caches.open(RUNTIME_CACHE);
  const keys = await cache.keys();
  if (keys.length > RUNTIME_CACHE_MAX_SIZE) {
    console.log('[SW] Trimming runtime cache:', keys.length, '->', RUNTIME_CACHE_MAX_SIZE);
    // Delete oldest entries
    const deleteCount = keys.length - RUNTIME_CACHE_MAX_SIZE;
    await Promise.all(keys.slice(0, deleteCount).map(key => cache.delete(key)));
  }
}

// Check if cached response is still fresh
function isCacheFresh(response) {
  const cachedTime = response.headers.get('sw-cached-time');
  if (!cachedTime) return false;
  const age = Date.now() - parseInt(cachedTime, 10);
  return age < CACHE_MAX_AGE;
}

// Add timestamp to cached response
async function cacheWithTimestamp(cacheName, request, response) {
  const cache = await caches.open(cacheName);
  const clonedResponse = response.clone();
  const headers = new Headers(clonedResponse.headers);
  headers.set('sw-cached-time', Date.now().toString());
  
  const newResponse = new Response(clonedResponse.body, {
    status: clonedResponse.status,
    statusText: clonedResponse.statusText,
    headers: headers
  });
  
  await cache.put(request, newResponse);
}

// Fetch event - secure caching with integrity checks
self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);

  // Security: Only handle same-origin requests
  if (url.origin !== location.origin) return;

  // Skip non-GET requests
  if (request.method !== 'GET') return;

  // Skip API calls (always fetch fresh)
  if (url.pathname.startsWith('/api/')) return;

  // For static assets: Stale-while-revalidate strategy
  if (STATIC_ASSETS.some(asset => url.pathname === asset) || url.pathname.startsWith('/static/')) {
    event.respondWith(
      caches.match(request)
        .then(cached => {
          // Return cached response immediately
          const fetchPromise = fetch(request, {
            credentials: 'same-origin'
          })
            .then(response => {
              // Update cache in background if response is valid
              if (response && response.ok && response.status === 200) {
                cacheWithTimestamp(CACHE_NAME, request, response.clone())
                  .catch(err => console.warn('[SW] Cache update failed:', err));
              }
              return response;
            })
            .catch(() => null);

          // Return cached if available, otherwise wait for network
          return cached || fetchPromise || caches.match('/admin.html');
        })
    );
    return;
  }

  // For images and dynamic content: Network first with secure caching
  event.respondWith(
    fetch(request, {
      credentials: 'same-origin'
    })
      .then(response => {
        // Cache valid responses
        if (response && response.ok && response.status === 200) {
          const contentType = response.headers.get('content-type') || '';
          // Only cache images and safe content
          if (contentType.startsWith('image/') || contentType.startsWith('video/')) {
            cacheWithTimestamp(RUNTIME_CACHE, request, response.clone())
              .then(() => trimRuntimeCache()) // Trim after adding
              .catch(err => console.warn('[SW] Runtime cache failed:', err));
          }
        }
        return response;
      })
      .catch(() => {
        // Fallback to cache on network error
        return caches.match(request);
      })
  );
});

// Message event - for manual cache control
self.addEventListener('message', (event) => {
  if (!event.data || typeof event.data.action !== 'string') return;

  switch (event.data.action) {
    case 'skipWaiting':
      self.skipWaiting();
      break;
    
    case 'clearCache':
      event.waitUntil(
        caches.keys().then(names => {
          console.log('[SW] Clearing all caches');
          return Promise.all(names.map(name => caches.delete(name)));
        })
      );
      break;
    
    case 'trimCache':
      event.waitUntil(trimRuntimeCache());
      break;
    
    default:
      console.warn('[SW] Unknown action:', event.data.action);
  }
});

// Error event - log service worker errors
self.addEventListener('error', (event) => {
  console.error('[SW] Service worker error:', event.error);
});

// Unhandled rejection - log promise rejections
self.addEventListener('unhandledrejection', (event) => {
  console.error('[SW] Unhandled promise rejection:', event.reason);
});
