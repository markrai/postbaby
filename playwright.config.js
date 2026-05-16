const { defineConfig } = require('@playwright/test');

module.exports = defineConfig({
  testDir: './tests/browser',
  testMatch: /.*\.spec\.js/,
  timeout: 30000,
  expect: {
    timeout: 10000
  },
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: 'http://127.0.0.1:4173',
    browserName: 'chromium',
    headless: true,
    serviceWorkers: 'block',
    trace: 'retain-on-failure'
  },
  webServer: {
    command: 'node tests/browser/static-server.js',
    url: 'http://127.0.0.1:4173',
    reuseExistingServer: true,
    timeout: 10000
  }
});
