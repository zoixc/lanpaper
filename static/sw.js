/**
 * Service Worker for Lanpaper PWA
 * Provides offline support and caching strategies
 */

const CACHE_VERSION = 'lanpaper-v1.2';
const STATIC_CACHE = `${CACHE_VERSION}-static`;
const DYNAMIC_CACHE = `${CACHE_VERSION}-dynamic`;
const IMAGE_CACHE = `${CACHE_VERSION}-images`;

// Assets to cache on install
const STATIC_ASSETS = [
    '/',
    '/static/css/style.css',
    '/static/js/app.js',
    '/static/js/image-compressor.js',
    '/static/logo.svg',
    '/static/logo-dark.svg',
    '/static/i18n/en.json',
    '/static/i18n/ru.json',
    '/static/i18n/de.json',
    '/static/i18n/fr.json',
    '/static/i18n/it.json',
    '/static/i18n/es.json',
];

// Install event - cache static assets
self.addEventListener('install', (event) => {
    console.log('[SW] Installing...');
    event.waitUntil(
        caches.open(STATIC_CACHE)
            .then(cache => {
                console.log('[SW] Caching static assets');
                return cache.addAll(STATIC_ASSETS);
            })
            .then(() => self.skipWaiting())
            .catch(err => console.error('[SW] Install error:', err))
    );
});

// Activate event - clean up old caches
self.addEventListener('activate', (event) => {
    console.log('[SW] Activating...');
    event.waitUntil(
        caches.keys()
            .then(cacheNames => {
                return Promise.all(
                    cacheNames
                        .filter(name => name.startsWith('lanpaper-') && name !== STATIC_CACHE && name !== DYNAMIC_CACHE && name !== IMAGE_CACHE)
                        .map(name => {
                            console.log('[SW] Deleting old cache:', name);
                            return caches.delete(name);
                        })
                );
            })
            .then(() => self.clients.claim())
    );
});

// Fetch event - serve from cache, fallback to network
self.addEventListener('fetch', (event) => {
    const { request } = event;
    const url = new URL(request.url);

    // Skip non-GET requests
    if (request.method !== 'GET') return;

    // Skip external requests
    if (url.origin !== location.origin) return;

    // API requests - Network first, cache fallback
    if (url.pathname.startsWith('/api/')) {
        event.respondWith(
            fetch(request)
                .then(response => {
                    // Clone and cache successful responses
                    if (response.ok && url.pathname === '/api/wallpapers') {
                        const clone = response.clone();
                        caches.open(DYNAMIC_CACHE).then(cache => cache.put(request, clone));
                    }
                    return response;
                })
                .catch(() => {
                    // Fallback to cache on network error
                    return caches.match(request).then(cached => {
                        if (cached) return cached;
                        // Return offline response
                        return new Response(JSON.stringify({ error: 'Offline' }), {
                            status: 503,
                            headers: { 'Content-Type': 'application/json' }
                        });
                    });
                })
        );
        return;
    }

    // Images and media - Cache first, network fallback
    if (url.pathname.match(/\.(jpg|jpeg|png|gif|webp|svg|mp4|webm)$/i)) {
        event.respondWith(
            caches.match(request)
                .then(cached => {
                    if (cached) return cached;

                    return fetch(request)
                        .then(response => {
                            if (response.ok) {
                                const clone = response.clone();
                                caches.open(IMAGE_CACHE).then(cache => cache.put(request, clone));
                            }
                            return response;
                        });
                })
        );
        return;
    }

    // Static assets - Cache first, network fallback
    event.respondWith(
        caches.match(request)
            .then(cached => {
                if (cached) return cached;

                return fetch(request)
                    .then(response => {
                        if (response.ok && !url.pathname.includes('?')) {
                            const clone = response.clone();
                            caches.open(STATIC_CACHE).then(cache => cache.put(request, clone));
                        }
                        return response;
                    });
            })
    );
});

// Message event - handle cache clearing
self.addEventListener('message', (event) => {
    if (event.data && event.data.type === 'CLEAR_CACHE') {
        event.waitUntil(
            caches.keys().then(cacheNames => {
                return Promise.all(
                    cacheNames
                        .filter(name => name.startsWith('lanpaper-'))
                        .map(name => caches.delete(name))
                );
            })
        );
    }
});
