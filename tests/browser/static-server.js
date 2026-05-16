const http = require('http');
const fs = require('fs');
const path = require('path');
const { URL } = require('url');

const rootDir = path.resolve(__dirname, '..', '..');
const host = '127.0.0.1';
const port = Number.parseInt(process.env.PORT || '4173', 10);

const contentTypes = {
  '.css': 'text/css; charset=utf-8',
  '.html': 'text/html; charset=utf-8',
  '.ico': 'image/x-icon',
  '.js': 'application/javascript; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.md': 'text/markdown; charset=utf-8',
  '.png': 'image/png',
  '.svg': 'image/svg+xml',
  '.txt': 'text/plain; charset=utf-8',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2'
};

function resolveRequestPath(requestUrl) {
  const parsed = new URL(requestUrl, `http://${host}:${port}`);
  let pathname = decodeURIComponent(parsed.pathname);

  if (pathname === '/') {
    pathname = '/index.html';
  }

  const absolutePath = path.resolve(rootDir, `.${pathname}`);
  if (!absolutePath.startsWith(rootDir)) {
    return null;
  }

  return absolutePath;
}

function sendFile(response, filePath) {
  const extension = path.extname(filePath).toLowerCase();
  const contentType = contentTypes[extension] || 'application/octet-stream';

  fs.readFile(filePath, function (error, buffer) {
    if (error) {
      response.writeHead(error.code === 'ENOENT' ? 404 : 500, {
        'Content-Type': 'text/plain; charset=utf-8',
        'Cache-Control': 'no-store'
      });
      response.end(error.code === 'ENOENT' ? 'Not found' : 'Server error');
      return;
    }

    response.writeHead(200, {
      'Content-Type': contentType,
      'Cache-Control': 'no-store'
    });
    response.end(buffer);
  });
}

const server = http.createServer(function (request, response) {
  const filePath = resolveRequestPath(request.url || '/');
  if (!filePath) {
    response.writeHead(400, {
      'Content-Type': 'text/plain; charset=utf-8',
      'Cache-Control': 'no-store'
    });
    response.end('Bad request');
    return;
  }

  fs.stat(filePath, function (error, stats) {
    if (error || !stats.isFile()) {
      response.writeHead(404, {
        'Content-Type': 'text/plain; charset=utf-8',
        'Cache-Control': 'no-store'
      });
      response.end('Not found');
      return;
    }

    sendFile(response, filePath);
  });
});

server.listen(port, host, function () {
  console.log(`[postbaby-browser-tests] static server listening on http://${host}:${port}`);
});

function shutdown() {
  server.close(function () {
    process.exit(0);
  });
}

process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);
