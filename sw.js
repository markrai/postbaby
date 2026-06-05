// sw.js

<<<<<<< HEAD
const PUBLIC_JS_REVISION = '5f2f3551d092';
=======
const PUBLIC_JS_REVISION = 'ca15540baf6a';
>>>>>>> fe10c3af5664144e9b70117ad18593197687cb7d
const CACHE_NAME = 'postbaby-cache-v8-' + PUBLIC_JS_REVISION;
const urlsToCache = [
  '/favicon.ico',
  '/manifest.json',
  '/css/theme.css',
  '/css/style.css',
  '/css/grids.css',
  '/css/mobile.css',
  '/css/calendar.css',
  '/css/ios.css',
  '/img/logotexttransparent.png',
  '/img/icon-192x192.png',
  '/img/icon-512x512.png',
  '/fonts/kalam.ttf',
  '/fonts/comfortaa.ttf',
  '/fonts/cherrybombone.ttf',
  '/js/version.js',
  '/js/utils.js',
  '/js/strings.js',
  '/js/indexeddb-storage.js',
  '/js/storage-adapter.js',
  '/js/persistence.js',
  '/js/selectors.js',
  '/js/modal.js',
  '/js/script.js',
  // Add any other assets you want to cache
];

// Install event - caching assets
self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then((cache) => {
        console.log('Opened cache');
        return cache.addAll(urlsToCache);
      })
  );
});

const networkOnlyPaths = new Set(['/','/index.html','/login','/logout','/setup','/runtime-config.js']);

function shouldBypassCache(request) {
  if (request.method !== 'GET') {
    return true;
  }

  const url = new URL(request.url);
  if (url.origin !== self.location.origin) {
    return false;
  }

  if (url.pathname.startsWith('/api/')) {
    return true;
  }

  return networkOnlyPaths.has(url.pathname);
}

// Fetch event - serving cached assets
self.addEventListener('fetch', (event) => {
  // Auth pages, runtime config, and sync endpoints are deployment/session
  // scoped and must always hit the network so stale cached responses never
  // leak across sessions or modes.
  if (event.request.mode === 'navigate' || shouldBypassCache(event.request)) {
    event.respondWith(fetch(event.request).catch(() => Response.error()));
    return;
  }

  event.respondWith(
    fetch(event.request).then((response) => {
      if (!response || response.status !== 200 || response.type !== 'basic') {
        return response;
      }

      const responseToCache = response.clone();
      event.waitUntil(
        caches.open(CACHE_NAME)
          .then((cache) => cache.put(event.request, responseToCache))
      );

      return response;
    }).catch(async () => {
      const cachedResponse = await caches.match(event.request);
      if (cachedResponse) {
        return cachedResponse;
      }

      return Response.error();
    })
  );
});

// Activate event - cleaning up old caches
self.addEventListener('activate', (event) => {
  const cacheWhitelist = [CACHE_NAME];
  event.waitUntil(
    caches.keys().then((cacheNames) => {
      return Promise.all(
        cacheNames.map((cacheName) => {
          if (!cacheWhitelist.includes(cacheName)) {
            console.log('Deleting old cache:', cacheName);
            return caches.delete(cacheName);
          }
        })
      );
    })
  );
});
