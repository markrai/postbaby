// sw.js

const PUBLIC_JS_REVISION = '11ea6596eeb7';
const CACHE_NAME = 'postbaby-cache-v8-' + PUBLIC_JS_REVISION;

function assetUrl(path) {
  return `${path}?v=${PUBLIC_JS_REVISION}`;
}

const urlsToCache = [
  assetUrl('/favicon.ico'),
  assetUrl('/manifest.json'),
  assetUrl('/css/theme.css'),
  assetUrl('/css/style.css'),
  assetUrl('/css/grids.css'),
  assetUrl('/css/mobile.css'),
  assetUrl('/css/calendar.css'),
  assetUrl('/css/ios.css'),
  assetUrl('/img/logotexttransparent.png'),
  assetUrl('/img/icon-192x192.png'),
  assetUrl('/img/icon-512x512.png'),
  assetUrl('/fonts/kalam.ttf'),
  assetUrl('/fonts/comfortaa.ttf'),
  assetUrl('/fonts/cherrybombone.ttf'),
  assetUrl('/js/version.js'),
  assetUrl('/js/utils.js'),
  assetUrl('/js/strings.js'),
  assetUrl('/js/indexeddb-storage.js'),
  assetUrl('/js/storage-adapter.js'),
  assetUrl('/js/persistence.js'),
  assetUrl('/js/selectors.js'),
  assetUrl('/js/modal.js'),
  assetUrl('/js/script.js')
];

// Install event - caching assets
self.addEventListener('install', (event) => {
  self.skipWaiting();
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
    }).then(() => self.clients.claim())
  );
});
