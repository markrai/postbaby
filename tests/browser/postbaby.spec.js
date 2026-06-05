const fs = require('fs');
const { test, expect } = require('@playwright/test');

const DB_NAME = 'postbaby-browser-storage';
const SNAPSHOTS_STORE = 'snapshots';
const META_STORE = 'meta';
const PRIMARY_RECORD_ID = 'primary';
const TIMESTAMP = '2026-05-13T12:00:00.000Z';
const EDGE_ARM_DELAY_MS = 1000;
const EDGE_ARROW_TARGET_INSET = 2;
const GRAPH_MAX_NODES = 100;
const GRAPH_MAX_EDGES = 300;
const GRAPH_MAX_LABEL_CHARS = 240;
const GRAPH_DEFAULT_COLOR = '#ffee88';
const ITEM_SHAPES = ['default', 'circle', 'square', 'triangle', 'diamond', 'upsideDownTriangle', 'hexagon', 'oval'];
const FREEFORM_FOOTPRINT_SHAPES = ['default', 'oval'];
const FIXED_RATIO_SHAPES = ITEM_SHAPES.filter((shape) => !FREEFORM_FOOTPRINT_SHAPES.includes(shape));
const FIXED_SQUARE_RESIZE_SHAPES = ['square', 'circle', 'diamond'];
const DERIVED_HEIGHT_RESIZE_SHAPES = ['triangle', 'upsideDownTriangle', 'hexagon'];
const NON_RECTANGULAR_SHAPES = ['circle', 'triangle', 'diamond', 'upsideDownTriangle', 'hexagon', 'oval'];
const RESIZABLE_FIXED_RATIO_SHAPES = FIXED_SQUARE_RESIZE_SHAPES.concat(DERIVED_HEIGHT_RESIZE_SHAPES);
const FIXED_RATIO_HEIGHT_BY_WIDTH = {
  square: 1,
  circle: 1,
  diamond: 1,
  triangle: 150 / 170,
  upsideDownTriangle: 150 / 170,
  hexagon: 135 / 170
};
const SHAPE_TEXT_INSETS = {
  default: { top: 20, right: 20, bottom: 20, left: 20 },
  square: { top: 26, right: 26, bottom: 26, left: 26 },
  circle: { top: 30, right: 30, bottom: 30, left: 30 },
  diamond: { top: 34, right: 34, bottom: 34, left: 34 },
  triangle: { top: 30, right: 30, bottom: 24, left: 30 },
  upsideDownTriangle: { top: 22, right: 30, bottom: 34, left: 30 },
  hexagon: { top: 26, right: 34, bottom: 26, left: 34 },
  oval: { top: 24, right: 34, bottom: 24, left: 34 }
};
const SHAPE_TEXT_ALIGNMENT = {
  default: { horizontalAlign: 'left', verticalAlign: 'top' },
  square: { horizontalAlign: 'left', verticalAlign: 'top' },
  circle: { horizontalAlign: 'center', verticalAlign: 'center' },
  diamond: { horizontalAlign: 'center', verticalAlign: 'center' },
  triangle: { horizontalAlign: 'center', verticalAlign: 'center' },
  upsideDownTriangle: { horizontalAlign: 'left', verticalAlign: 'top' },
  hexagon: { horizontalAlign: 'center', verticalAlign: 'top' },
  oval: { horizontalAlign: 'left', verticalAlign: 'top' }
};
const SHAPE_AWARE_ARROW_TARGET_SIZES = {
  circle: { width: 240, height: 190 },
  oval: { width: 320, height: 200 },
  diamond: { width: 240, height: 190 },
  triangle: { width: 280, height: 190 },
  upsideDownTriangle: { width: 280, height: 190 },
  hexagon: { width: 280, height: 190 }
};
const SYNC_HASH_STORAGE_KEYS = [
  'tabs',
  'activeTabId',
  'theme',
  'disableColorChange',
  'disableNoteResize',
  'hideInstructions',
  'corporateMode',
  'defaultColorEnabled',
  'defaultColor',
  'hasRunBefore'
];
const FORBIDDEN_SHAPE_SIZE_FIELDS = ITEM_SHAPES
  .flatMap((shape) => [`${shape}Width`, `${shape}Height`])
  .concat(['shapeSizes', 'sizeByShape']);

function expectItemUsesSharedSizeFieldsOnly(item) {
  FORBIDDEN_SHAPE_SIZE_FIELDS.forEach((field) => {
    expect(item).not.toHaveProperty(field);
  });
}

function getExpectedFixedRatioHeight(shape, width) {
  return Math.round(width * FIXED_RATIO_HEIGHT_BY_WIDTH[shape]);
}

function getExpectedResponsiveInset(value, minimum) {
  return Math.max(minimum, Math.round(value));
}

function getExpectedShapeTextBounds(shape, width, height) {
  const minimumInsets = SHAPE_TEXT_INSETS[shape] || SHAPE_TEXT_INSETS.default;
  let insets = minimumInsets;

  if (Number.isFinite(width) && Number.isFinite(height)) {
    if (shape === 'circle') {
      const horizontalInset = getExpectedResponsiveInset(width * 0.214, minimumInsets.left);
      const verticalInset = getExpectedResponsiveInset(height * 0.214, minimumInsets.top);
      insets = { top: verticalInset, right: horizontalInset, bottom: verticalInset, left: horizontalInset };
    } else if (shape === 'diamond') {
      const horizontalInset = getExpectedResponsiveInset(width * 0.243, minimumInsets.left);
      const verticalInset = getExpectedResponsiveInset(height * 0.243, minimumInsets.top);
      insets = { top: verticalInset, right: horizontalInset, bottom: verticalInset, left: horizontalInset };
    } else if (shape === 'triangle') {
      const horizontalInset = getExpectedResponsiveInset(width * 0.182, minimumInsets.left);
      insets = {
        top: getExpectedResponsiveInset(height * 0.293, 44),
        right: horizontalInset,
        bottom: getExpectedResponsiveInset(height * 0.120, 18),
        left: horizontalInset
      };
    } else if (shape === 'upsideDownTriangle') {
      const horizontalInset = getExpectedResponsiveInset(width * 0.182, minimumInsets.left);
      insets = {
        top: getExpectedResponsiveInset(height * 0.120, 18),
        right: horizontalInset,
        bottom: getExpectedResponsiveInset(height * 0.293, 44),
        left: horizontalInset
      };
    } else if (shape === 'hexagon') {
      const horizontalInset = getExpectedResponsiveInset(width * 0.2, minimumInsets.left);
      const verticalInset = getExpectedResponsiveInset(height * 0.193, minimumInsets.top);
      insets = { top: verticalInset, right: horizontalInset, bottom: verticalInset, left: horizontalInset };
    } else if (shape === 'oval') {
      const horizontalInset = getExpectedResponsiveInset(width * 0.121, minimumInsets.left);
      const verticalInset = getExpectedResponsiveInset(height * 0.126, minimumInsets.top);
      insets = { top: verticalInset, right: horizontalInset, bottom: verticalInset, left: horizontalInset };
    }
  }

  const textWidth = Number.isFinite(width)
    ? Math.max(0, Math.round(width - insets.left - insets.right))
    : undefined;
  const textHeight = Number.isFinite(height)
    ? Math.max(0, Math.round(height - insets.top - insets.bottom))
    : undefined;
  const alignment = SHAPE_TEXT_ALIGNMENT[shape] || SHAPE_TEXT_ALIGNMENT.default;

  return {
    top: insets.top,
    right: insets.right,
    bottom: insets.bottom,
    left: insets.left,
    width: textWidth,
    height: textHeight,
    minContentHeight: textHeight === undefined ? 0 : textHeight,
    horizontalAlign: alignment.horizontalAlign,
    verticalAlign: alignment.verticalAlign
  };
}

function getExpectedTextJustifyContent(verticalAlign) {
  return verticalAlign === 'center' ? 'center' : 'flex-start';
}

function getExpectedTextAlign(horizontalAlign) {
  return horizontalAlign === 'center' ? 'center' : 'left';
}

function buildLocalSnapshot(noteText, options = {}) {
  return buildLocalSnapshotWithItems([buildNoteItem(noteText, options)], options);
}

function buildNoteItem(noteText, options = {}) {
  const item = {
    id: options.itemId || 'item-1',
    name: noteText,
    color: options.color || '#ffee88',
    position: options.position || {
      top: '24px',
      left: '24px'
    },
    shape: options.shape || 'default'
  };

  if (typeof options.width === 'number') {
    item.width = options.width;
  }

  if (typeof options.height === 'number') {
    item.height = options.height;
  }

  return item;
}

function buildLocalSnapshotWithItems(items, options = {}) {
  const tabId = options.tabId || 'tab-1';
  const snapshot = {
    tabs: JSON.stringify([{
      id: tabId,
      name: '1',
      items,
      colorIndex: 0,
      gridSetting: 'none',
      edges: options.edges || []
    }]),
    activeTabId: tabId,
    hasRunBefore: options.hasRunBefore === undefined ? 'true' : String(options.hasRunBefore),
    theme: options.theme || 'light'
  };

  if (options.disableColorChange !== undefined) {
    snapshot.disableColorChange = String(options.disableColorChange);
  }

  if (options.disableNoteResize !== undefined) {
    snapshot.disableNoteResize = String(options.disableNoteResize);
  }

  if (typeof options.syncVersion === 'number') {
    snapshot.postbabySyncVersion = String(options.syncVersion);
  }

  return snapshot;
}

function buildEmptySnapshot(options = {}) {
  return buildLocalSnapshotWithItems([], options);
}

function buildGraphNode(id, options = {}) {
  const node = {
    id
  };

  if (Object.prototype.hasOwnProperty.call(options, 'label')) {
    node.label = options.label;
  }

  if (Object.prototype.hasOwnProperty.call(options, 'shape')) {
    node.shape = options.shape;
  }

  if (Object.prototype.hasOwnProperty.call(options, 'width')) {
    node.width = options.width;
  }

  if (Object.prototype.hasOwnProperty.call(options, 'height')) {
    node.height = options.height;
  }

  if (Object.prototype.hasOwnProperty.call(options, 'x')) {
    node.x = options.x;
  }

  if (Object.prototype.hasOwnProperty.call(options, 'y')) {
    node.y = options.y;
  }

  return node;
}

function buildGraphEdge(from, to, options = {}) {
  const edge = {
    from,
    to
  };

  if (Object.prototype.hasOwnProperty.call(options, 'kind')) {
    edge.kind = options.kind;
  }

  if (Object.prototype.hasOwnProperty.call(options, 'label')) {
    edge.label = options.label;
  }

  return edge;
}

function buildNormalizedGraph(nodes, edges = [], options = {}) {
  return {
    nodes,
    edges,
    options
  };
}

function buildGeometryRegressionSnapshot() {
  return buildLocalSnapshotWithItems([
    buildNoteItem('Default fixture note', {
      itemId: 'fixture-default',
      position: { top: '40px', left: '40px' },
      width: 260,
      height: 170
    }),
    buildNoteItem('Square fixture note', {
      itemId: 'fixture-square',
      shape: 'square',
      position: { top: '40px', left: '360px' },
      width: 240,
      height: 180
    }),
    buildNoteItem('Circle short text', {
      itemId: 'fixture-circle',
      shape: 'circle',
      position: { top: '60px', left: '640px' },
      width: 240,
      height: 180
    }),
    buildNoteItem('Oval fixture note with longer wrapped copy to keep the safe rectangle honest.', {
      itemId: 'fixture-oval',
      shape: 'oval',
      position: { top: '280px', left: '40px' },
      width: 320,
      height: 190
    }),
    buildNoteItem('Diamond short', {
      itemId: 'fixture-diamond',
      shape: 'diamond',
      position: { top: '280px', left: '420px' },
      width: 240,
      height: 190
    }),
    buildNoteItem('Triangle fixture note with wrapped text for the lower safe area.', {
      itemId: 'fixture-triangle',
      shape: 'triangle',
      position: { top: '280px', left: '720px' },
      width: 280,
      height: 190
    }),
    buildNoteItem('Upside-down triangle fixture text.', {
      itemId: 'fixture-updown-triangle',
      shape: 'upsideDownTriangle',
      position: { top: '560px', left: '80px' },
      width: 280,
      height: 190
    }),
    buildNoteItem('Hexagon fixture text that wraps to a second line.', {
      itemId: 'fixture-hexagon',
      shape: 'hexagon',
      position: { top: '560px', left: '460px' },
      width: 280,
      height: 190
    })
  ], {
    edges: [
      { id: 'fixture-edge-1', fromItemId: 'fixture-default', toItemId: 'fixture-circle', kind: 'arrow' },
      { id: 'fixture-edge-2', fromItemId: 'fixture-oval', toItemId: 'fixture-triangle', kind: 'line' },
      { id: 'fixture-edge-3', fromItemId: 'fixture-updown-triangle', toItemId: 'fixture-hexagon', kind: 'arrow' }
    ]
  });
}

function buildIndexedDBMeta(snapshot, overrides = {}) {
  return Object.assign({
    id: PRIMARY_RECORD_ID,
    schemaVersion: 1,
    migrationState: 'complete',
    source: 'indexeddb',
    migratedAt: TIMESTAMP,
    lastValidatedAt: TIMESTAMP,
    lastWriteAt: TIMESTAMP,
    snapshotKeyCount: Object.keys(snapshot).length
  }, overrides);
}

function hashStringForTest(value) {
  let hash = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return `fnv1a32:${(hash >>> 0).toString(16).padStart(8, '0')}:${value.length}`;
}

function computeTestSnapshotHash(snapshot) {
  const canonical = {};
  SYNC_HASH_STORAGE_KEYS.forEach((key) => {
    if (Object.prototype.hasOwnProperty.call(snapshot, key) && typeof snapshot[key] === 'string') {
      canonical[key] = snapshot[key];
    }
  });
  return hashStringForTest(JSON.stringify(canonical));
}

function addDurableCloudSyncMetadata(snapshot, options = {}) {
  const localHash = options.localHash || computeTestSnapshotHash(snapshot);
  const uploadedHash = options.uploadedHash === undefined ? localHash : options.uploadedHash;
  const revision = typeof options.revision === 'number' ? options.revision : null;

  snapshot.postbabyLocalSnapshotHash = localHash;
  if (uploadedHash !== null) {
    snapshot.postbabyLastUploadedSnapshotHash = uploadedHash;
  }
  snapshot.postbabyPendingCloudUpload = options.pending ? 'true' : 'false';
  if (options.pending) {
    snapshot.postbabyLocalModifiedAt = options.localModifiedAt || new Date().toISOString();
  }
  if (options.lastCloudUploadedAt) {
    snapshot.postbabyLastCloudUploadedAt = options.lastCloudUploadedAt;
  }
  if (revision !== null) {
    snapshot.postbabyLastKnownServerRevision = String(revision);
    snapshot.postbabyLastSuccessfulUploadServerRevision = String(revision);
  }
  if (options.lastError) {
    snapshot.postbabyLastSyncError = options.lastError;
  }
  return snapshot;
}

function buildServerPayload(noteText, version) {
  return {
    version,
    updatedAt: TIMESTAMP,
    data: buildLocalSnapshot(noteText, { hasRunBefore: true, theme: 'light' })
  };
}

function normalizeScopeKey(scopeKey = PRIMARY_RECORD_ID) {
  return typeof scopeKey === 'string' && scopeKey.trim()
    ? scopeKey.trim()
    : PRIMARY_RECORD_ID;
}

function storageKeyForScope(scopeKey, key) {
  const normalizedScope = normalizeScopeKey(scopeKey);
  if (normalizedScope === PRIMARY_RECORD_ID) {
    return key;
  }
  return `postbabyScope:${encodeURIComponent(normalizedScope)}:${key}`;
}

async function prepareBlankPage(page) {
  await page.goto('/tests/browser/blank.html');
  await page.evaluate(async ({ dbName }) => {
    window.localStorage.clear();

    await new Promise((resolve) => {
      const request = window.indexedDB.deleteDatabase(dbName);
      request.onsuccess = function () { resolve(); };
      request.onerror = function () { resolve(); };
      request.onblocked = function () { resolve(); };
    });
  }, { dbName: DB_NAME });
}

async function seedLocalStorage(page, snapshot, scopeKey = PRIMARY_RECORD_ID) {
  await page.evaluate(({ data, targetScopeKey }) => {
    Object.entries(data).forEach(([key, value]) => {
      const storageKey = targetScopeKey === 'primary'
        ? key
        : `postbabyScope:${encodeURIComponent(targetScopeKey)}:${key}`;
      window.localStorage.setItem(storageKey, value);
    });
  }, {
    data: snapshot,
    targetScopeKey: normalizeScopeKey(scopeKey)
  });
}

async function seedIndexedDB(page, snapshot, meta, recordId = PRIMARY_RECORD_ID) {
  await page.evaluate(async ({ dbName, snapshotsStore, metaStore, recordId, snapshotData, metaRecord }) => {
    await new Promise((resolve, reject) => {
      const request = window.indexedDB.open(dbName, 1);
      request.onupgradeneeded = function () {
        const db = request.result;
        if (!db.objectStoreNames.contains(snapshotsStore)) {
          db.createObjectStore(snapshotsStore, { keyPath: 'id' });
        }
        if (!db.objectStoreNames.contains(metaStore)) {
          db.createObjectStore(metaStore, { keyPath: 'id' });
        }
      };
      request.onerror = function () {
        reject(request.error || new Error('Failed to open IndexedDB.'));
      };
      request.onsuccess = function () {
        const db = request.result;
        const transaction = db.transaction([snapshotsStore, metaStore], 'readwrite');
        transaction.objectStore(snapshotsStore).put({
          id: recordId,
          data: snapshotData,
          updatedAt: metaRecord.lastWriteAt || new Date().toISOString()
        });
        transaction.objectStore(metaStore).put(Object.assign({ id: recordId }, metaRecord));
        transaction.oncomplete = function () {
          db.close();
          resolve();
        };
        transaction.onerror = function () {
          reject(transaction.error || new Error('IndexedDB write failed.'));
        };
        transaction.onabort = function () {
          reject(transaction.error || new Error('IndexedDB write aborted.'));
        };
      };
    });
  }, {
    dbName: DB_NAME,
    snapshotsStore: SNAPSHOTS_STORE,
    metaStore: META_STORE,
    recordId,
    snapshotData: snapshot,
    metaRecord: meta
  });
}

async function importBackupSnapshot(page, snapshot, fileName = 'postbaby-backup.json') {
  await page.locator('#loadDataInput').setInputFiles({
    name: fileName,
    mimeType: 'application/json',
    buffer: Buffer.from(JSON.stringify(snapshot, null, 2))
  });
}

async function exportCurrentSnapshot(page) {
  const downloadPromise = page.waitForEvent('download');
  await page.evaluate(() => {
    document.getElementById('saveDataButton').click();
  });
  const download = await downloadPromise;
  const downloadPath = await download.path();
  if (!downloadPath) {
    throw new Error('Download path was not available.');
  }

  return JSON.parse(fs.readFileSync(downloadPath, 'utf8'));
}

async function readIndexedDBState(page, recordId = PRIMARY_RECORD_ID) {
  return page.evaluate(async ({ dbName, snapshotsStore, metaStore, recordId }) => {
    return new Promise((resolve, reject) => {
      const request = window.indexedDB.open(dbName, 1);
      request.onerror = function () {
        reject(request.error || new Error('Failed to open IndexedDB.'));
      };
      request.onsuccess = function () {
        const db = request.result;
        const transaction = db.transaction([snapshotsStore, metaStore], 'readonly');
        const snapshotRequest = transaction.objectStore(snapshotsStore).get(recordId);
        const metaRequest = transaction.objectStore(metaStore).get(recordId);
        transaction.oncomplete = function () {
          const snapshot = snapshotRequest.result ? snapshotRequest.result.data : null;
          const meta = metaRequest.result || null;
          db.close();
          resolve({ snapshot, meta });
        };
        transaction.onerror = function () {
          reject(transaction.error || new Error('IndexedDB read failed.'));
        };
        transaction.onabort = function () {
          reject(transaction.error || new Error('IndexedDB read aborted.'));
        };
      };
    });
  }, {
    dbName: DB_NAME,
    snapshotsStore: SNAPSHOTS_STORE,
    metaStore: META_STORE,
    recordId
  });
}

async function getLocalStorageValues(page, keys, scopeKey = PRIMARY_RECORD_ID) {
  return page.evaluate(({ requestedKeys, targetScopeKey }) => {
    const result = {};
    requestedKeys.forEach((key) => {
      const storageKey = targetScopeKey === 'primary'
        ? key
        : `postbabyScope:${encodeURIComponent(targetScopeKey)}:${key}`;
      result[key] = window.localStorage.getItem(storageKey);
    });
    return result;
  }, {
    requestedKeys: keys,
    targetScopeKey: normalizeScopeKey(scopeKey)
  });
}

async function openSettingsModal(page) {
  await page.locator('#settingsButton').click();
  await expect(page.locator('#settingsModal')).toBeVisible();
}

async function openAccountModal(page) {
  await page.locator('#accountButton').click();
  await expect(page.locator('#accountModal')).toBeVisible();
}

async function mockRuntimeConfig(page, runtime = {}) {
  const authAvailable = runtime.authAvailable === true;
  const syncAvailable = runtime.syncAvailable === true;
  const authRequired = runtime.authRequired === true;
  const apiBase = typeof runtime.apiBase === 'string' ? runtime.apiBase : '';
  const account = Object.prototype.hasOwnProperty.call(runtime, 'account') ? runtime.account : null;
  const extraLines = [];

  if (Object.prototype.hasOwnProperty.call(runtime, 'isAuthenticated')) {
    extraLines.push(`  isAuthenticated: ${runtime.isAuthenticated === true},`);
  }

  if (Object.prototype.hasOwnProperty.call(runtime, 'deploymentMode')) {
    extraLines.push(`  deploymentMode: ${JSON.stringify(runtime.deploymentMode)},`);
  }

  if (Object.prototype.hasOwnProperty.call(runtime, 'billingAvailable')) {
    extraLines.push(`  billingAvailable: ${runtime.billingAvailable === true},`);
  }

  if (Object.prototype.hasOwnProperty.call(runtime, 'authorityModel')) {
    extraLines.push(`  authorityModel: ${JSON.stringify(runtime.authorityModel)},`);
  }

  if (Object.prototype.hasOwnProperty.call(runtime, 'syncRequiresAuth')) {
    extraLines.push(`  syncRequiresAuth: ${runtime.syncRequiresAuth === true},`);
  }

  if (Object.prototype.hasOwnProperty.call(runtime, 'syncUsable')) {
    extraLines.push(`  syncUsable: ${runtime.syncUsable === true},`);
  }

  if (Object.prototype.hasOwnProperty.call(runtime, 'syncPausedReason')) {
    extraLines.push(`  syncPausedReason: ${JSON.stringify(runtime.syncPausedReason)},`);
  }

  if (Object.prototype.hasOwnProperty.call(runtime, 'entitlement')) {
    extraLines.push(`  entitlement: ${JSON.stringify(runtime.entitlement || {})},`);
  }

  const body = [
    'window.POSTBABY_RUNTIME = {',
    `  authAvailable: ${authAvailable},`,
    `  syncAvailable: ${syncAvailable},`,
    `  authRequired: ${authRequired},`,
    `  apiBase: ${JSON.stringify(apiBase)},`,
    ...extraLines,
    `  account: ${account === null ? 'null' : JSON.stringify(account)}`,
    '};'
  ].join('\n');

  await page.route('**/runtime-config.js', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/javascript; charset=utf-8',
      headers: {
        'Cache-Control': 'no-store'
      },
      body
    });
  });
}

async function dragNoteToTrash(page, note) {
  const trash = page.locator('#trash');
  await note.scrollIntoViewIfNeeded();
  await trash.scrollIntoViewIfNeeded();

  const noteBox = await note.boundingBox();
  const trashBox = await trash.boundingBox();
  if (!noteBox || !trashBox) {
    throw new Error('Note or trash bounding box was not available.');
  }

  const startX = noteBox.x + (noteBox.width / 2);
  const startY = noteBox.y + (noteBox.height / 2);
  const endX = trashBox.x + (trashBox.width / 2);
  const endY = trashBox.y + (trashBox.height / 2);

  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(endX, endY, { steps: 16 });
  await page.mouse.up();
}

async function expectNoteVisible(page, noteText) {
  await expect(page.locator('.grid-item span').filter({ hasText: noteText }).first()).toBeVisible();
}

async function readTabsSnapshot(page) {
  let tabsPayload = null;
  for (let attempt = 0; attempt < 40; attempt += 1) {
    const indexedDBState = await readIndexedDBState(page);
    if (indexedDBState && indexedDBState.snapshot && indexedDBState.snapshot.tabs) {
      tabsPayload = indexedDBState.snapshot.tabs;
      break;
    }
    await page.waitForTimeout(50);
  }

  if (tabsPayload === null) {
    throw new Error('IndexedDB tabs snapshot was not available.');
  }

  return JSON.parse(tabsPayload);
}

async function readItemSnapshot(page, itemId = 'item-1') {
  const tabsSnapshot = await readTabsSnapshot(page);
  for (const tab of tabsSnapshot) {
    const item = Array.isArray(tab.items) ? tab.items.find((candidate) => candidate.id === itemId) : null;
    if (item) {
      return item;
    }
  }
  return null;
}

async function createGraphForTest(page, graph) {
  await page.waitForFunction(() => typeof window.postbabyCreateGraphForTest === 'function');
  return page.evaluate(async (input) => {
    if (typeof window.postbabyCreateGraphForTest !== 'function') {
      throw new Error('postbabyCreateGraphForTest is not available.');
    }

    return window.postbabyCreateGraphForTest(input);
  }, graph);
}

async function parseMermaidForTest(page, source, options = {}) {
  await page.waitForFunction(() => typeof window.postbabyParseMermaidForTest === 'function');
  return page.evaluate((input) => {
    if (typeof window.postbabyParseMermaidForTest !== 'function') {
      throw new Error('postbabyParseMermaidForTest is not available.');
    }

    return window.postbabyParseMermaidForTest(input.source, input.options);
  }, { source, options });
}

async function createGraphFromMermaidForTest(page, source, options = {}) {
  await page.waitForFunction(() => typeof window.postbabyCreateGraphFromMermaidForTest === 'function');
  return page.evaluate(async (input) => {
    if (typeof window.postbabyCreateGraphFromMermaidForTest !== 'function') {
      throw new Error('postbabyCreateGraphFromMermaidForTest is not available.');
    }

    return window.postbabyCreateGraphFromMermaidForTest(input.source, input.options);
  }, { source, options });
}

async function readEdgeSnapshot(page, edgeId = 'edge-1') {
  const tabsSnapshot = await readTabsSnapshot(page);
  for (const tab of tabsSnapshot) {
    const edge = (tab.edges || []).find((candidate) => candidate.id === edgeId);
    if (edge) {
      return edge;
    }
  }
  return null;
}

async function readItemColor(page, itemId = 'item-1') {
  const item = await readItemSnapshot(page, itemId);
  return item ? item.color : null;
}

async function readNoteSize(locator) {
  return locator.evaluate((element) => ({
    width: element.offsetWidth,
    height: element.offsetHeight
  }));
}

async function readNoteOffsetGeometry(locator) {
  return locator.evaluate((element) => ({
    left: element.offsetLeft,
    top: element.offsetTop,
    width: element.offsetWidth,
    height: element.offsetHeight,
    centerX: element.offsetLeft + (element.offsetWidth / 2),
    centerY: element.offsetTop + (element.offsetHeight / 2)
  }));
}

async function readNotePresentation(locator) {
  return locator.evaluate((element) => {
    const style = window.getComputedStyle(element);
    const beforeStyle = window.getComputedStyle(element, '::before');
    const handle = element.querySelector('.grid-item-resize-handle');
    const handleStyle = handle ? window.getComputedStyle(handle) : null;
    return {
      sizeMode: element.dataset.sizeMode || '',
      usesExplicitSizeClass: element.classList.contains('grid-item--explicit-size'),
      overflow: style.overflow,
      beforeBorderRadius: beforeStyle.borderRadius,
      beforeClipPath: beforeStyle.clipPath,
      handleRight: handleStyle ? handleStyle.right : null,
      handleBottom: handleStyle ? handleStyle.bottom : null
    };
  });
}

async function readItemTextFlowPresentation(locator) {
  return locator.evaluate((element) => {
    const text = element.querySelector('.grid-item-text');
    const content = text ? text.querySelector('.grid-item-text-content') : null;
    const textStyle = text ? window.getComputedStyle(text) : null;
    const noteStyle = window.getComputedStyle(element);
    const noteRect = element.getBoundingClientRect();
    const textRect = text ? text.getBoundingClientRect() : null;
    const contentRect = content ? content.getBoundingClientRect() : null;
    const paddingTop = Number.parseFloat(noteStyle.paddingTop) || 0;
    const paddingRight = Number.parseFloat(noteStyle.paddingRight) || 0;
    const paddingBottom = Number.parseFloat(noteStyle.paddingBottom) || 0;
    const paddingLeft = Number.parseFloat(noteStyle.paddingLeft) || 0;
    return {
      noteWidth: element.offsetWidth,
      noteHeight: element.offsetHeight,
      flowHeight: textStyle ? textStyle.minHeight : '',
      textBoxMinHeight: textStyle ? Number.parseFloat(textStyle.minHeight) || 0 : 0,
      textAlign: textStyle ? textStyle.textAlign : '',
      justifyContent: textStyle ? textStyle.justifyContent : '',
      paddingTop,
      paddingRight,
      paddingBottom,
      paddingLeft,
      safeWidth: Math.max(0, element.clientWidth - paddingLeft - paddingRight),
      safeHeight: Math.max(0, element.clientHeight - paddingTop - paddingBottom),
      availableWidth: Math.max(0, element.clientWidth - paddingLeft - paddingRight),
      availableHeight: Math.max(0, element.clientHeight - paddingTop - paddingBottom),
      textTop: textRect ? textRect.top - noteRect.top : null,
      textLeft: textRect ? textRect.left - noteRect.left : null,
      textWidth: textRect ? textRect.width : 0,
      textHeight: textRect ? textRect.height : 0,
      contentTop: contentRect ? contentRect.top - noteRect.top : null,
      contentLeft: contentRect ? contentRect.left - noteRect.left : null,
      contentWidth: contentRect ? contentRect.width : 0,
      contentHeight: contentRect ? contentRect.height : 0,
      shaperCount: element.querySelectorAll('.grid-item-text-shaper').length
    };
  });
}

async function readEditTextareaPresentation(locator) {
  return locator.evaluate((element) => {
    const noteRect = element.getBoundingClientRect();
    const textarea = element.querySelector('.edit-textarea');
    if (!textarea) {
      return null;
    }

    const textareaRect = textarea.getBoundingClientRect();
    const textareaStyle = window.getComputedStyle(textarea);
    return {
      top: textareaRect.top - noteRect.top,
      left: textareaRect.left - noteRect.left,
      width: textareaRect.width,
      height: textareaRect.height,
      minHeight: Number.parseFloat(textareaStyle.minHeight) || 0,
      paddingTop: Number.parseFloat(textareaStyle.paddingTop) || 0,
      paddingRight: Number.parseFloat(textareaStyle.paddingRight) || 0,
      paddingBottom: Number.parseFloat(textareaStyle.paddingBottom) || 0,
      paddingLeft: Number.parseFloat(textareaStyle.paddingLeft) || 0
    };
  });
}

function expectTextLayoutMatchesBounds(layout, expectedBounds) {
  expect(layout.shaperCount).toBe(0);
  expect(layout.paddingTop).toBe(expectedBounds.top);
  expect(layout.paddingRight).toBe(expectedBounds.right);
  expect(layout.paddingBottom).toBe(expectedBounds.bottom);
  expect(layout.paddingLeft).toBe(expectedBounds.left);
  expect(layout.safeWidth).toBe(expectedBounds.width);
  expect(layout.safeHeight).toBe(expectedBounds.height);
  expect(layout.textBoxMinHeight).toBe(expectedBounds.minContentHeight);
  expect(layout.textTop).toBeGreaterThanOrEqual(expectedBounds.top - 1);
  expect(layout.textLeft).toBeGreaterThanOrEqual(expectedBounds.left - 1);
  expect(layout.textWidth).toBeLessThanOrEqual(expectedBounds.width + 1);
  expect(layout.contentWidth).toBeLessThanOrEqual(expectedBounds.width + 1);
  expect(layout.contentTop).toBeGreaterThanOrEqual(expectedBounds.top - 1);
}

function expectTextAlignmentMatches(layout, expectedBounds) {
  expect(layout.textAlign).toBe(getExpectedTextAlign(expectedBounds.horizontalAlign));
  expect(layout.justifyContent).toBe(getExpectedTextJustifyContent(expectedBounds.verticalAlign));

  if (expectedBounds.verticalAlign === 'center') {
    const centeredTop = expectedBounds.top + ((expectedBounds.height - layout.contentHeight) / 2);
    expect(Math.abs(layout.contentTop - centeredTop)).toBeLessThanOrEqual(2);
  } else {
    expect(Math.abs(layout.contentTop - expectedBounds.top)).toBeLessThanOrEqual(2);
  }
}

async function readEdgeCoordinates(locator) {
  return locator.evaluate((element) => ({
    x1: Number(element.getAttribute('x1')),
    y1: Number(element.getAttribute('y1')),
    x2: Number(element.getAttribute('x2')),
    y2: Number(element.getAttribute('y2'))
  }));
}

async function readEdgePresentation(locator) {
  return locator.evaluate((element) => ({
    markerEnd: element.getAttribute('marker-end') || '',
    stroke: element.getAttribute('stroke') || '',
    strokeOpacity: element.getAttribute('stroke-opacity') || '',
    armed: element.classList.contains('edge-line-armed')
  }));
}

function getTestBoundsCenter(bounds) {
  return {
    x: bounds.left + (bounds.width / 2),
    y: bounds.top + (bounds.height / 2)
  };
}

function applyExpectedAnchorInset(boundaryPoint, bounds, externalPoint, inset) {
  const center = getTestBoundsCenter(bounds);
  const dx = externalPoint.x - center.x;
  const dy = externalPoint.y - center.y;
  const distance = Math.sqrt((dx * dx) + (dy * dy));
  if (!distance || !Number.isFinite(inset) || inset === 0) {
    return boundaryPoint;
  }

  return {
    x: boundaryPoint.x + ((dx / distance) * inset),
    y: boundaryPoint.y + ((dy / distance) * inset)
  };
}

function getExpectedRectBoundaryPoint(bounds, externalPoint) {
  const center = getTestBoundsCenter(bounds);
  const dx = externalPoint.x - center.x;
  const dy = externalPoint.y - center.y;
  const distance = Math.sqrt((dx * dx) + (dy * dy));
  if (!distance) {
    return { x: center.x, y: center.y };
  }

  const halfWidth = bounds.width / 2;
  const halfHeight = bounds.height / 2;
  const scaleX = dx === 0 ? Infinity : halfWidth / Math.abs(dx);
  const scaleY = dy === 0 ? Infinity : halfHeight / Math.abs(dy);
  const scale = Math.min(scaleX, scaleY);
  return {
    x: center.x + (dx * scale),
    y: center.y + (dy * scale)
  };
}

function getExpectedEllipseBoundaryPoint(bounds, externalPoint) {
  const center = getTestBoundsCenter(bounds);
  const dx = externalPoint.x - center.x;
  const dy = externalPoint.y - center.y;
  if (Math.abs(dx) < 0.000001 && Math.abs(dy) < 0.000001) {
    return { x: center.x, y: center.y };
  }

  const radiusX = bounds.width / 2;
  const radiusY = bounds.height / 2;
  const scale = 1 / Math.sqrt(
    ((dx * dx) / (radiusX * radiusX))
    + ((dy * dy) / (radiusY * radiusY))
  );
  return {
    x: center.x + (dx * scale),
    y: center.y + (dy * scale)
  };
}

function getExpectedDiamondBoundaryPoint(bounds, externalPoint) {
  const center = getTestBoundsCenter(bounds);
  const dx = externalPoint.x - center.x;
  const dy = externalPoint.y - center.y;
  if (Math.abs(dx) < 0.000001 && Math.abs(dy) < 0.000001) {
    return { x: center.x, y: center.y };
  }

  const halfWidth = bounds.width / 2;
  const halfHeight = bounds.height / 2;
  const scale = 1 / (
    (Math.abs(dx) / halfWidth)
    + (Math.abs(dy) / halfHeight)
  );
  return {
    x: center.x + (dx * scale),
    y: center.y + (dy * scale)
  };
}

function crossProduct(ax, ay, bx, by) {
  return (ax * by) - (ay * bx);
}

function getExpectedPolygonPoints(shape, bounds) {
  let normalizedPoints = null;
  if (shape === 'triangle') {
    normalizedPoints = [
      { x: 0.5, y: 0 },
      { x: 0, y: 1 },
      { x: 1, y: 1 }
    ];
  } else if (shape === 'upsideDownTriangle') {
    normalizedPoints = [
      { x: 0, y: 0 },
      { x: 1, y: 0 },
      { x: 0.5, y: 1 }
    ];
  } else if (shape === 'hexagon') {
    normalizedPoints = [
      { x: 0.25, y: 0 },
      { x: 0.75, y: 0 },
      { x: 1, y: 0.5 },
      { x: 0.75, y: 1 },
      { x: 0.25, y: 1 },
      { x: 0, y: 0.5 }
    ];
  }

  if (!normalizedPoints) {
    return null;
  }

  return normalizedPoints.map((point) => ({
    x: bounds.left + (point.x * bounds.width),
    y: bounds.top + (point.y * bounds.height)
  }));
}

function getExpectedPolygonBoundaryPoint(shape, bounds, externalPoint) {
  const points = getExpectedPolygonPoints(shape, bounds);
  if (!points) {
    return null;
  }

  const center = getTestBoundsCenter(bounds);
  const directionX = externalPoint.x - center.x;
  const directionY = externalPoint.y - center.y;
  if (Math.abs(directionX) < 0.000001 && Math.abs(directionY) < 0.000001) {
    return { x: center.x, y: center.y };
  }

  let bestIntersection = null;
  for (let index = 0; index < points.length; index += 1) {
    const start = points[index];
    const end = points[(index + 1) % points.length];
    const edgeX = end.x - start.x;
    const edgeY = end.y - start.y;
    const originToStartX = start.x - center.x;
    const originToStartY = start.y - center.y;
    const denominator = crossProduct(directionX, directionY, edgeX, edgeY);
    if (Math.abs(denominator) < 0.000001) {
      continue;
    }

    const rayScale = crossProduct(originToStartX, originToStartY, edgeX, edgeY) / denominator;
    const segmentScale = crossProduct(originToStartX, originToStartY, directionX, directionY) / denominator;
    if (rayScale < -0.000001 || segmentScale < -0.000001 || segmentScale > 1.000001) {
      continue;
    }

    if (!bestIntersection || rayScale < bestIntersection.rayScale) {
      bestIntersection = {
        rayScale,
        x: center.x + (directionX * rayScale),
        y: center.y + (directionY * rayScale)
      };
    }
  }

  return bestIntersection
    ? { x: bestIntersection.x, y: bestIntersection.y }
    : null;
}

function getExpectedShapeBoundaryPoint(shape, bounds, externalPoint) {
  if (shape === 'circle' || shape === 'oval') {
    return getExpectedEllipseBoundaryPoint(bounds, externalPoint);
  }

  if (shape === 'diamond') {
    return getExpectedDiamondBoundaryPoint(bounds, externalPoint);
  }

  if (shape === 'triangle' || shape === 'upsideDownTriangle' || shape === 'hexagon') {
    return getExpectedPolygonBoundaryPoint(shape, bounds, externalPoint) || getExpectedRectBoundaryPoint(bounds, externalPoint);
  }

  return getExpectedRectBoundaryPoint(bounds, externalPoint);
}

function getExpectedShapeAnchorPoint(shape, bounds, externalPoint, inset = EDGE_ARROW_TARGET_INSET) {
  const boundaryPoint = getExpectedShapeBoundaryPoint(shape, bounds, externalPoint);
  return applyExpectedAnchorInset(boundaryPoint, bounds, externalPoint, inset);
}

function expectPointNear(actualPoint, expectedPoint, tolerance = 2.5) {
  expect(Math.abs(actualPoint.x - expectedPoint.x)).toBeLessThanOrEqual(tolerance);
  expect(Math.abs(actualPoint.y - expectedPoint.y)).toBeLessThanOrEqual(tolerance);
}

function buildShapeAwareArrowSnapshot(shape) {
  const targetSize = SHAPE_AWARE_ARROW_TARGET_SIZES[shape];
  return buildLocalSnapshotWithItems([
    buildNoteItem('Anchor Source', {
      itemId: 'item-1',
      position: { top: '80px', left: '80px' },
      width: 260,
      height: 170
    }),
    buildNoteItem(`${shape} Anchor Target`, {
      itemId: 'item-2',
      shape,
      position: { top: '250px', left: '430px' },
      width: targetSize.width,
      height: targetSize.height
    })
  ], {
    edges: [{
      id: 'edge-1',
      fromItemId: 'item-1',
      toItemId: 'item-2',
      kind: 'arrow'
    }]
  });
}

async function deleteEdgeViaContextMenu(page, edgeLine) {
  await edgeLine.evaluate((element) => {
    const group = element.parentElement;
    const hit = group ? group.querySelector('.edge-hit-line') : null;
    if (!hit) {
      throw new Error('Edge hit line was not available.');
    }

    hit.dispatchEvent(new MouseEvent('mouseenter', {
      bubbles: false,
      cancelable: false
    }));
  });
  await page.waitForTimeout(EDGE_ARM_DELAY_MS + 150);
  await edgeLine.evaluate((element) => {
    const group = element.parentElement;
    const hit = group ? group.querySelector('.edge-hit-line') : null;
    if (!hit) {
      throw new Error('Edge hit line was not available.');
    }

    hit.dispatchEvent(new MouseEvent('contextmenu', {
      bubbles: true,
      cancelable: true,
      button: 2,
      buttons: 2
    }));
  });
  await expect(page.locator('#confirmModal')).toBeVisible();
  await page.click('#confirmDelete');
}

async function dragNoteBy(page, note, deltaX, deltaY) {
  await note.scrollIntoViewIfNeeded();
  const box = await note.boundingBox();
  if (!box) {
    throw new Error('Note bounding box was not available.');
  }

  const startX = box.x + (box.width / 2);
  const startY = box.y + (box.height / 2);
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + deltaX, startY + deltaY, { steps: 12 });
  await page.mouse.up();
}

async function beginDragGesture(page, note, deltaX, deltaY) {
  await note.scrollIntoViewIfNeeded();
  const box = await note.boundingBox();
  if (!box) {
    throw new Error('Note bounding box was not available.');
  }

  const startX = box.x + (box.width / 2);
  const startY = box.y + (box.height / 2);
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + deltaX, startY + deltaY, { steps: 12 });
}

async function beginResizeGesture(page, note, deltaX, deltaY) {
  await note.scrollIntoViewIfNeeded();
  const handle = note.locator('.grid-item-resize-handle');
  await handle.scrollIntoViewIfNeeded();
  await expect(handle).toBeVisible();
  const box = await handle.boundingBox();
  if (!box) {
    throw new Error('Resize handle bounding box was not available.');
  }

  const startX = box.x + (box.width / 2);
  const startY = box.y + (box.height / 2);
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + deltaX, startY + deltaY, { steps: 12 });
}

async function beginEdgeGesture(page, fromNote, toNote, options = {}) {
  await fromNote.scrollIntoViewIfNeeded();
  await toNote.scrollIntoViewIfNeeded();

  const fromBox = await fromNote.boundingBox();
  const toBox = await toNote.boundingBox();
  if (!fromBox || !toBox) {
    throw new Error('Edge endpoints were not available.');
  }

  const fromX = fromBox.x + (fromBox.width / 2);
  const fromY = fromBox.y + (fromBox.height / 2);
  const toX = toBox.x + (toBox.width / 2);
  const toY = toBox.y + (toBox.height / 2);
  const modifier = options.modifier || 'Shift';
  const progress = typeof options.progress === 'number' ? options.progress : 1;
  const moveX = fromX + ((toX - fromX) * progress);
  const moveY = fromY + ((toY - fromY) * progress);

  await page.keyboard.down(modifier);
  await page.mouse.move(fromX, fromY);
  await page.mouse.down();
  await page.mouse.move(moveX, moveY, { steps: 12 });
}

async function beginMarqueeGesture(page, notes, options = {}) {
  if (!Array.isArray(notes) || notes.length === 0) {
    throw new Error('At least one note is required for marquee selection.');
  }

  const viewport = page.viewportSize();
  if (!viewport) {
    throw new Error('Viewport size was not available.');
  }

  const noteBoxes = [];
  for (const note of notes) {
    await note.scrollIntoViewIfNeeded();
    const box = await note.boundingBox();
    if (!box) {
      throw new Error('Note bounding box was not available.');
    }
    noteBoxes.push(box);
  }

  const margin = options.margin || 18;
  const minX = Math.min(...noteBoxes.map((box) => box.x));
  const minY = Math.min(...noteBoxes.map((box) => box.y));
  const maxX = Math.max(...noteBoxes.map((box) => box.x + box.width));
  const maxY = Math.max(...noteBoxes.map((box) => box.y + box.height));
  const startX = Math.max(6, minX - margin);
  const startY = Math.max(6, minY - margin);
  const endX = Math.min(viewport.width - 6, maxX + margin);
  const endY = Math.min(viewport.height - 6, maxY + margin);

  await page.evaluate(({ startX, startY, endX, endY }) => {
    document.body.dispatchEvent(new MouseEvent('mousedown', {
      bubbles: true,
      cancelable: true,
      button: 0,
      buttons: 1,
      clientX: startX,
      clientY: startY
    }));

    document.dispatchEvent(new MouseEvent('mousemove', {
      bubbles: true,
      cancelable: true,
      button: 0,
      buttons: 1,
      clientX: endX,
      clientY: endY
    }));
  }, { startX, startY, endX, endY });
}

async function clickEmptyGrid(page, options = {}) {
  const viewport = page.viewportSize();
  if (!viewport) {
    throw new Error('Viewport size was not available.');
  }

  const xPadding = options.xPadding || 80;
  const yPadding = options.yPadding || 280;
  const point = await page.evaluate(({ fallbackX, fallbackY }) => {
    const selectedNotes = Array.from(document.querySelectorAll('.grid-item.selected'));
    if (selectedNotes.length > 0) {
      const rects = selectedNotes.map((element) => element.getBoundingClientRect());
      const minLeft = Math.min(...rects.map((rect) => rect.left));
      const maxRight = Math.max(...rects.map((rect) => rect.right));
      const midY = rects.reduce((sum, rect) => sum + ((rect.top + rect.bottom) / 2), 0) / rects.length;
      let x = maxRight + 40;
      if (x > window.innerWidth - 20) {
        x = Math.max(20, minLeft - 40);
      }

      return {
        x: Math.round(x),
        y: Math.round(Math.max(20, Math.min(window.innerHeight - 20, midY)))
      };
    }

    return { x: fallbackX, y: fallbackY };
  }, {
    fallbackX: viewport.width - xPadding,
    fallbackY: yPadding
  });

  await page.evaluate(({ x, y }) => {
    document.body.dispatchEvent(new MouseEvent('mousedown', {
      bubbles: true,
      cancelable: true,
      button: 0,
      buttons: 1,
      clientX: x,
      clientY: y
    }));
    document.dispatchEvent(new MouseEvent('mouseup', {
      bubbles: true,
      cancelable: true,
      button: 0,
      buttons: 0,
      clientX: x,
      clientY: y
    }));
  }, point);
}

async function readSelectedNoteIds(page) {
  const selectedIds = await page.locator('.grid-item.selected').evaluateAll((elements) =>
    elements
      .map((element) => element.dataset.id || '')
      .filter(Boolean)
      .sort()
  );
  return selectedIds;
}

async function readBrowserSelectionText(page) {
  return page.evaluate(() => {
    const selection = window.getSelection ? window.getSelection() : null;
    return selection ? selection.toString() : '';
  });
}

async function marqueeSelectNotes(page, notes, options = {}) {
  if (!Array.isArray(notes) || notes.length === 0) {
    throw new Error('At least one note is required for marquee selection.');
  }

  const viewport = page.viewportSize();
  if (!viewport) {
    throw new Error('Viewport size was not available.');
  }

  const noteBoxes = [];
  for (const note of notes) {
    await note.scrollIntoViewIfNeeded();
    const box = await note.boundingBox();
    if (!box) {
      throw new Error('Note bounding box was not available.');
    }
    noteBoxes.push(box);
  }

  const margin = options.margin || 18;
  const minX = Math.min(...noteBoxes.map((box) => box.x));
  const minY = Math.min(...noteBoxes.map((box) => box.y));
  const maxX = Math.max(...noteBoxes.map((box) => box.x + box.width));
  const maxY = Math.max(...noteBoxes.map((box) => box.y + box.height));
  const anchor = options.anchor || 'top-left';
  const startX = anchor === 'bottom-right'
    ? Math.min(viewport.width - 6, maxX + margin)
    : Math.max(6, minX - margin);
  const startY = anchor === 'bottom-right'
    ? Math.min(viewport.height - 6, maxY + margin)
    : Math.max(6, minY - margin);
  const endX = anchor === 'bottom-right'
    ? Math.max(6, minX - margin)
    : Math.min(viewport.width - 6, maxX + margin);
  const endY = anchor === 'bottom-right'
    ? Math.max(6, minY - margin)
    : Math.min(viewport.height - 6, maxY + margin);

  await page.evaluate(({ startX, startY, endX, endY }) => {
    const steps = 12;
    const dispatch = (target, type, x, y, buttons) => {
      target.dispatchEvent(new MouseEvent(type, {
        bubbles: true,
        cancelable: true,
        button: 0,
        buttons,
        clientX: x,
        clientY: y
      }));
    };

    dispatch(document.body, 'mousedown', startX, startY, 1);
    for (let index = 1; index <= steps; index += 1) {
      const progress = index / steps;
      const x = startX + ((endX - startX) * progress);
      const y = startY + ((endY - startY) * progress);
      dispatch(document, 'mousemove', x, y, 1);
    }
    dispatch(document, 'mouseup', endX, endY, 0);
  }, { startX, startY, endX, endY });
}

async function drawEdgeBetweenNotes(page, fromNote, toNote, options = {}) {
  await fromNote.scrollIntoViewIfNeeded();
  await toNote.scrollIntoViewIfNeeded();

  const fromBox = await fromNote.boundingBox();
  const toBox = await toNote.boundingBox();
  if (!fromBox || !toBox) {
    throw new Error('Edge endpoints were not available.');
  }

  const fromX = fromBox.x + (fromBox.width / 2);
  const fromY = fromBox.y + (fromBox.height / 2);
  const toX = toBox.x + (toBox.width / 2);
  const toY = toBox.y + (toBox.height / 2);

  const modifier = options.modifier || 'Shift';
  await page.keyboard.down(modifier);
  await page.mouse.move(fromX, fromY);
  await page.mouse.down();
  await page.mouse.move(toX, toY, { steps: 12 });
  await page.mouse.up();
  await page.keyboard.up(modifier);
}

async function clickResizeHandle(page, note) {
  await note.scrollIntoViewIfNeeded();
  const handle = note.locator('.grid-item-resize-handle');
  await handle.scrollIntoViewIfNeeded();
  await expect(handle).toBeVisible();
  const box = await handle.boundingBox();
  if (!box) {
    throw new Error('Resize handle bounding box was not available.');
  }

  await page.mouse.click(box.x + (box.width / 2), box.y + (box.height / 2));
}

async function resizeNoteBy(page, note, deltaX, deltaY) {
  await note.scrollIntoViewIfNeeded();
  const handle = note.locator('.grid-item-resize-handle');
  await handle.scrollIntoViewIfNeeded();
  await expect(handle).toBeVisible();
  const box = await handle.boundingBox();
  if (!box) {
    throw new Error('Resize handle bounding box was not available.');
  }

  const startX = box.x + (box.width / 2);
  const startY = box.y + (box.height / 2);
  await page.mouse.move(startX, startY);
  await page.mouse.down();
  await page.mouse.move(startX + deltaX, startY + deltaY, { steps: 12 });
  await page.mouse.up();
}

async function cycleNoteShape(note, modifiers = ['Control']) {
  await note.click({
    button: 'right',
    modifiers
  });
}

async function cycleNoteShapeTo(note, targetShape, direction = 'forward') {
  const clickModifiers = direction === 'reverse' ? ['Shift', 'Control'] : ['Control'];
  for (let index = 0; index < ITEM_SHAPES.length; index += 1) {
    const currentShape = await note.getAttribute('data-shape');
    if (currentShape === targetShape) {
      return;
    }

    await cycleNoteShape(note, clickModifiers);
  }

  throw new Error(`Failed to cycle note to shape "${targetShape}".`);
}

function attachDialogHandler(page, steps, seenDialogs) {
  page.on('dialog', async (dialog) => {
    seenDialogs.push({
      type: dialog.type(),
      message: dialog.message()
    });

    const nextStep = steps.shift();
    if (!nextStep) {
      await dialog.dismiss();
      return;
    }

    if (nextStep.type) {
      expect(dialog.type()).toBe(nextStep.type);
    }
    if (nextStep.messageIncludes) {
      expect(dialog.message()).toContain(nextStep.messageIncludes);
    }

    if (nextStep.action === 'accept') {
      await dialog.accept();
    } else {
      await dialog.dismiss();
    }
  });
}

async function enableMockedSync(page, options = {}) {
  const metaPayload = options.metaPayload || { ok: true, exists: false, version: 0, updatedAt: TIMESTAMP };
  const documentPayload = options.documentPayload || buildServerPayload('Server Snapshot', metaPayload.version || 1);
  const onSave = options.onSave || null;
  const runtimeAccount = Object.assign({
    username: 'owner',
    displayName: 'owner',
    email: '',
    avatarUrl: '',
    isAdmin: true,
    storageKey: 'owner-scope',
    status: 'active'
  }, options.runtimeAccount || {});

  await page.route('**/runtime-config.js', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/javascript; charset=utf-8',
      headers: {
        'Cache-Control': 'no-store'
      },
      body: [
        'window.POSTBABY_RUNTIME = {',
        '  deploymentMode: "selfhosted",',
        '  authorityModel: "server_authoritative",',
        '  authAvailable: true,',
        '  syncAvailable: true,',
        '  authRequired: true,',
        '  syncRequiresAuth: true,',
        '  syncUsable: true,',
        '  syncPausedReason: "",',
        '  entitlement: { hostedSync: false, status: "none" },',
        '  apiBase: "",',
        `  account: ${JSON.stringify(runtimeAccount)}`,
        '};'
      ].join('\n')
    });
  });

  await page.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json; charset=utf-8',
      headers: {
        'Cache-Control': 'no-store'
      },
      body: JSON.stringify(metaPayload)
    });
  });

  await page.route(/\/api\/document(?:\?.*)?$/, async (route) => {
    const request = route.request();
    if (request.method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: {
          'Cache-Control': 'no-store'
        },
        body: JSON.stringify(documentPayload)
      });
      return;
    }

    if (request.method() === 'PUT') {
      const requestBody = JSON.parse(request.postData() || '{}');
      const response = onSave
        ? await onSave(requestBody)
        : {
            status: 200,
            body: {
              ok: true,
              version: metaPayload.exists ? metaPayload.version + 1 : 1,
              updatedAt: TIMESTAMP
            }
          };

      await route.fulfill({
        status: response.status || 200,
        contentType: 'application/json; charset=utf-8',
        headers: {
          'Cache-Control': 'no-store'
        },
        body: JSON.stringify(response.body)
      });
      return;
    }

    await route.fulfill({
      status: 405,
      contentType: 'application/json; charset=utf-8',
      body: JSON.stringify({ ok: false })
    });
  });
}

test.describe('IndexedDB migration', () => {
  test('migrates legacy localStorage into IndexedDB and preserves legacy keys', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Legacy Local Note', { theme: 'dark' });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Legacy Local Note');

    const indexedDBState = await readIndexedDBState(page);
    expect(indexedDBState.snapshot).toMatchObject(localSnapshot);
    expect(indexedDBState.meta).toMatchObject({
      migrationState: 'complete',
      source: 'localStorage'
    });

    const localValues = await getLocalStorageValues(page, ['tabs', 'activeTabId', 'theme', 'hasRunBefore']);
    expect(localValues).toMatchObject({
      tabs: localSnapshot.tabs,
      activeTabId: localSnapshot.activeTabId,
      theme: 'dark',
      hasRunBefore: 'true'
    });
  });

  test('uses migrated IndexedDB snapshot over preserved conflicting localStorage', async ({ page }) => {
    const indexedDBSnapshot = buildLocalSnapshot('IndexedDB Wins');
    const legacySnapshot = buildLocalSnapshot('Legacy Local Backup');

    await prepareBlankPage(page);
    await seedLocalStorage(page, legacySnapshot);
    await seedIndexedDB(page, indexedDBSnapshot, buildIndexedDBMeta(indexedDBSnapshot));

    await page.goto('/index.html');
    await expectNoteVisible(page, 'IndexedDB Wins');
    await expect(page.locator('.grid-item span').filter({ hasText: 'Legacy Local Backup' })).toHaveCount(0);

    const localValues = await getLocalStorageValues(page, ['tabs']);
    expect(localValues.tabs).toBe(legacySnapshot.tabs);
  });

  test('keeps theme mirrored in localStorage while storing it in IndexedDB', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Theme Mirror Note', { theme: 'dark' });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Theme Mirror Note');

    const indexedDBState = await readIndexedDBState(page);
    expect(indexedDBState.snapshot.theme).toBe('dark');

    const localValues = await getLocalStorageValues(page, ['theme']);
    expect(localValues.theme).toBe('dark');
  });

  test('migrates legacy localStorage resized notes into IndexedDB without losing width and height', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Migrated Sized Note', {
      width: 280,
      height: 190
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');
    await expectNoteVisible(page, 'Migrated Sized Note');

    const migratedItem = await readItemSnapshot(page);
    expect(migratedItem.width).toBe(280);
    expect(migratedItem.height).toBe(190);
  });
});

test.describe('Static behavior', () => {
  test('imports an old backup without shape, width, or height and renders it as a default note', async ({ page }) => {
    const syncRequests = [];
    const importedSnapshot = buildLocalSnapshotWithItems([{
      id: 'legacy-item',
      name: 'Legacy Import Note',
      color: '#ffee88',
      position: {
        top: '24px',
        left: '24px'
      }
    }]);

    await prepareBlankPage(page);
    page.on('request', (request) => {
      const url = request.url();
      if (url.includes('/api/document/meta') || url.includes('/api/document')) {
        syncRequests.push(url);
      }
    });

    await page.goto('/index.html');
    await importBackupSnapshot(page, importedSnapshot);
    await expectNoteVisible(page, 'Legacy Import Note');

    const note = page.locator('.grid-item[data-id="legacy-item"]');
    await expect(note).toHaveAttribute('data-shape', 'default');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    const importedItem = await readItemSnapshot(page, 'legacy-item');
    expect(importedItem.shape).toBe('default');
    expect(importedItem).not.toHaveProperty('width');
    expect(importedItem).not.toHaveProperty('height');
    expect(syncRequests).toEqual([]);
  });

  test('imports a backup with shape but no width or height fields', async ({ page }) => {
    const importedSnapshot = buildLocalSnapshotWithItems([{
      id: 'shape-only-item',
      name: 'Shape Only Import',
      color: '#ffee88',
      position: {
        top: '24px',
        left: '24px'
      },
      shape: 'square'
    }]);

    await prepareBlankPage(page);
    await page.goto('/index.html');
    await importBackupSnapshot(page, importedSnapshot);
    await expectNoteVisible(page, 'Shape Only Import');

    const note = page.locator('.grid-item[data-id="shape-only-item"]');
    await expect(note).toHaveAttribute('data-shape', 'square');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    const importedItem = await readItemSnapshot(page, 'shape-only-item');
    expect(importedItem.shape).toBe('square');
    expect(importedItem).not.toHaveProperty('width');
    expect(importedItem).not.toHaveProperty('height');
  });

  test('imports mixed sized and unsized notes without altering the existing tabs payload', async ({ page }) => {
    const importedSnapshot = buildLocalSnapshotWithItems([
      {
        id: 'sized-item',
        name: 'Sized Import Note',
        color: '#ffee88',
        position: {
          top: '24px',
          left: '24px'
        },
        shape: 'default',
        width: 280,
        height: 190
      },
      {
        id: 'unsized-item',
        name: 'Unsized Import Note',
        color: '#ffee88',
        position: {
          top: '260px',
          left: '24px'
        }
      }
    ]);

    await prepareBlankPage(page);
    await page.goto('/index.html');
    await importBackupSnapshot(page, importedSnapshot);
    await expectNoteVisible(page, 'Sized Import Note');
    await expectNoteVisible(page, 'Unsized Import Note');

    const sizedNote = page.locator('.grid-item[data-id="sized-item"]');
    const sizedNoteSize = await readNoteSize(sizedNote);
    expect(sizedNoteSize.width).toBe(280);
    expect(sizedNoteSize.height).toBe(190);

    const tabsSnapshot = await readTabsSnapshot(page);
    const sizedItem = tabsSnapshot[0].items.find((item) => item.id === 'sized-item');
    const unsizedItem = tabsSnapshot[0].items.find((item) => item.id === 'unsized-item');
    expect(sizedItem.width).toBe(280);
    expect(sizedItem.height).toBe(190);
    expect(unsizedItem.shape).toBe('default');
    expect(unsizedItem).not.toHaveProperty('width');
    expect(unsizedItem).not.toHaveProperty('height');
  });

  test('normalizes invalid imported note dimensions safely without crashing render', async ({ page }) => {
    const importedSnapshot = buildLocalSnapshotWithItems([
      {
        id: 'string-size-item',
        name: 'String Size Import',
        color: '#ffee88',
        position: {
          top: '24px',
          left: '24px'
        },
        shape: 'default',
        width: '320',
        height: '210'
      },
      {
        id: 'negative-size-item',
        name: 'Negative Size Import',
        color: '#ffee88',
        position: {
          top: '24px',
          left: '420px'
        },
        shape: 'default',
        width: -50,
        height: -10
      },
      {
        id: 'zero-size-item',
        name: 'Zero Size Import',
        color: '#ffee88',
        position: {
          top: '280px',
          left: '24px'
        },
        shape: 'default',
        width: 0,
        height: 0
      },
      {
        id: 'large-size-item',
        name: 'Large Size Import',
        color: '#ffee88',
        position: {
          top: '280px',
          left: '420px'
        },
        shape: 'default',
        width: 99999,
        height: 99999
      },
      {
        id: 'null-size-item',
        name: 'Null Size Import',
        color: '#ffee88',
        position: {
          top: '520px',
          left: '24px'
        },
        shape: 'default',
        width: null,
        height: null
      }
    ]);

    await prepareBlankPage(page);
    await page.goto('/index.html');
    await importBackupSnapshot(page, importedSnapshot);

    for (const noteText of [
      'String Size Import',
      'Negative Size Import',
      'Zero Size Import',
      'Large Size Import',
      'Null Size Import'
    ]) {
      await expectNoteVisible(page, noteText);
    }

    const stringSize = await readNoteSize(page.locator('.grid-item[data-id="string-size-item"]'));
    const negativeSize = await readNoteSize(page.locator('.grid-item[data-id="negative-size-item"]'));
    const zeroSize = await readNoteSize(page.locator('.grid-item[data-id="zero-size-item"]'));
    const largeSize = await readNoteSize(page.locator('.grid-item[data-id="large-size-item"]'));
    const nullSize = await readNoteSize(page.locator('.grid-item[data-id="null-size-item"]'));

    expect(stringSize.width).toBe(320);
    expect(stringSize.height).toBe(210);
    expect(negativeSize.width).toBe(120);
    expect(negativeSize.height).toBe(80);
    expect(zeroSize.width).toBe(120);
    expect(zeroSize.height).toBe(80);
    expect(largeSize.width).toBe(600);
    expect(largeSize.height).toBe(600);
    expect(nullSize.width).toBe(120);
    expect(nullSize.height).toBe(80);
  });

  test('keeps legacy notes without width and height fields unchanged until resized', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Legacy Size Note');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');
    await expectNoteVisible(page, 'Legacy Size Note');

    const tabsSnapshot = await readTabsSnapshot(page);
    expect(tabsSnapshot[0].items[0]).not.toHaveProperty('width');
    expect(tabsSnapshot[0].items[0]).not.toHaveProperty('height');
  });

  test('resizing a note does not color-cycle on mouse release', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Resize Without Recolor');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const initialColor = await readItemColor(page);

    await resizeNoteBy(page, note, 120, 80);
    await page.waitForTimeout(350);

    expect(await readItemColor(page)).toBe(initialColor);
  });

  test('clicking the resize handle without resizing does not color-cycle the note', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Handle Click Only');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const initialColor = await readItemColor(page);

    await clickResizeHandle(page, note);
    await page.waitForTimeout(350);

    expect(await readItemColor(page)).toBe(initialColor);
  });

  test('normal single-click color-cycle still works after a resize interaction', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Resize Then Click');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');

    await resizeNoteBy(page, note, 100, 60);
    await page.waitForTimeout(350);
    const colorAfterResize = await readItemColor(page);

    await note.click();
    await page.waitForTimeout(350);

    expect(await readItemColor(page)).not.toBe(colorAfterResize);
  });

  test('resizes a default-shape note on desktop and persists width and height across reload', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Resize Me');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    const beforeSize = await readNoteSize(note);
    await resizeNoteBy(page, note, 120, 80);
    const afterSize = await readNoteSize(note);

    expect(afterSize.width).toBeGreaterThan(beforeSize.width + 40);
    expect(afterSize.height).toBeGreaterThan(beforeSize.height + 20);

    const tabsSnapshot = await readTabsSnapshot(page);
    const resizedItem = tabsSnapshot[0].items[0];
    expect(resizedItem.width).toBe(afterSize.width);
    expect(resizedItem.height).toBe(afterSize.height);

    await page.reload();
    const reloadedNote = page.locator('.grid-item[data-id="item-1"]');
    const reloadedSize = await readNoteSize(reloadedNote);
    expect(reloadedSize.width).toBe(resizedItem.width);
    expect(reloadedSize.height).toBe(resizedItem.height);
  });

  test('default notes honor stored width and height and keep size fields shared', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Persisted Sized Default', {
      width: 280,
      height: 190
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    const storedItem = await readItemSnapshot(page);
    expect(storedItem.shape).toBe('default');
    expectItemUsesSharedSizeFieldsOnly(storedItem);
    expect(storedItem.width).toBe(280);
    expect(storedItem.height).toBe(190);

    const noteSize = await readNoteSize(note);
    expect(noteSize.width).toBe(280);
    expect(noteSize.height).toBe(190);
  });

  test('oval honors the shared rectangular footprint and shows a desktop resize handle', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Oval Resize Policy', {
      shape: 'oval',
      width: 280,
      height: 190
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const initialColor = await readItemColor(page);
    await expect(note).toHaveAttribute('data-shape', 'oval');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();
    await expect(page.locator('textarea.edit-textarea')).toHaveCount(0);
    await expectNoteVisible(page, 'Oval Resize Policy');

    const ovalBeforeResize = await readItemSnapshot(page);
    expect(ovalBeforeResize.shape).toBe('oval');
    expect(ovalBeforeResize.width).toBe(280);
    expect(ovalBeforeResize.height).toBe(190);
    expectItemUsesSharedSizeFieldsOnly(ovalBeforeResize);

    const ovalBeforeResizeSize = await readNoteSize(note);
    expect(ovalBeforeResizeSize.width).toBe(280);
    expect(ovalBeforeResizeSize.height).toBe(190);
    expect(await readItemColor(page)).toBe(initialColor);
  });

  for (const shape of FIXED_RATIO_SHAPES) {
    test(`${shape} uses width as its fixed-ratio visual reference and keeps shared size fields`, async ({ page }) => {
      const localSnapshot = buildLocalSnapshot(`${shape} Visual Policy`, {
        shape,
        width: 280,
        height: 190
      });

      await prepareBlankPage(page);
      await seedLocalStorage(page, localSnapshot);
      await page.goto('/index.html');

      const note = page.locator('.grid-item[data-id="item-1"]');
      await expect(note).toHaveAttribute('data-shape', shape);
      if (RESIZABLE_FIXED_RATIO_SHAPES.includes(shape)) {
        await expect(note.locator('.grid-item-resize-handle')).toBeVisible();
      } else {
        await expect(note.locator('.grid-item-resize-handle')).toHaveCount(0);
      }

      const storedItem = await readItemSnapshot(page);
      expect(storedItem.shape).toBe(shape);
      expect(storedItem.width).toBe(280);
      expect(storedItem.height).toBe(190);
      expectItemUsesSharedSizeFieldsOnly(storedItem);

      const noteSize = await readNoteSize(note);
      expect(noteSize.width).toBe(280);
      expect(noteSize.height).toBe(getExpectedFixedRatioHeight(shape, 280));
      expect(noteSize.width === storedItem.width && noteSize.height === storedItem.height).toBe(false);

      if (['circle', 'square', 'diamond'].includes(shape)) {
        expect(Math.abs(noteSize.width - noteSize.height)).toBeLessThanOrEqual(2);
      }
    });
  }

  for (const shape of ['circle', 'diamond']) {
    test(`${shape} keeps silhouette-friendly chrome and a usable resize handle after resize`, async ({ page }) => {
      const localSnapshot = buildLocalSnapshot(`${shape} Chrome`, {
        shape,
        width: 260,
        height: 260
      });

      await prepareBlankPage(page);
      await seedLocalStorage(page, localSnapshot);
      await page.goto('/index.html');

      const note = page.locator('.grid-item[data-id="item-1"]');
      await expect(note).toHaveAttribute('data-shape', shape);
      await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

      await resizeNoteBy(page, note, 80, 20);
      await page.waitForTimeout(350);

      const presentation = await readNotePresentation(note);
      expect(presentation.sizeMode).toBe('width-reference');
      expect(presentation.usesExplicitSizeClass).toBe(false);
      expect(presentation.overflow).toBe('visible');
      expect(Number.parseFloat(presentation.handleBottom || '0')).toBeGreaterThan(10);

      if (shape === 'circle') {
        expect(presentation.beforeBorderRadius).not.toBe('0px');
        expect(Number.parseFloat(presentation.handleRight || '0')).toBeGreaterThan(10);
      } else {
        expect(presentation.beforeClipPath).not.toBe('none');
        expect(Number.parseFloat(presentation.handleRight || '0')).toBeGreaterThan(30);
      }

      const colorBeforeHandleClick = await readItemColor(page);
      await clickResizeHandle(page, note);
      await page.waitForTimeout(350);
      await expect(note.locator('.grid-item-resize-handle')).toBeVisible();
      expect(await readItemColor(page)).toBe(colorBeforeHandleClick);
    });
  }

  test('oval keeps its explicit-size chrome path after resize', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Oval Chrome Regression', {
      shape: 'oval',
      width: 280,
      height: 190
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await expect(note).toHaveAttribute('data-shape', 'oval');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    await resizeNoteBy(page, note, 60, 30);
    await page.waitForTimeout(350);

    const presentation = await readNotePresentation(note);
    expect(presentation.sizeMode).toBe('freeform');
    expect(presentation.usesExplicitSizeClass).toBe(true);
    expect(presentation.overflow).toBe('visible');
    expect(presentation.beforeBorderRadius).not.toBe('0px');
  });

  for (const shape of DERIVED_HEIGHT_RESIZE_SHAPES) {
    test(`${shape} keeps a derived-height box and a usable resize handle after resize`, async ({ page }) => {
      const localSnapshot = buildLocalSnapshot(`${shape} Chrome`, {
        shape,
        width: 280,
        height: 190
      });

      await prepareBlankPage(page);
      await seedLocalStorage(page, localSnapshot);
      await page.goto('/index.html');

      const note = page.locator('.grid-item[data-id="item-1"]');
      await expect(note).toHaveAttribute('data-shape', shape);
      await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

      const expectedWidth = 370;
      const expectedHeight = getExpectedFixedRatioHeight(shape, expectedWidth);
      await resizeNoteBy(page, note, 90, 30);
      await expect.poll(async () => {
        const item = await readItemSnapshot(page);
        return `${item.width}x${item.height}`;
      }).toBe(`${expectedWidth}x${expectedHeight}`);

      const presentation = await readNotePresentation(note);
      const noteSize = await readNoteSize(note);
      expect(presentation.sizeMode).toBe('width-reference');
      expect(presentation.usesExplicitSizeClass).toBe(false);
      expect(presentation.overflow).toBe('visible');
      expect(presentation.beforeClipPath).not.toBe('none');
      expect(Number.parseFloat(presentation.handleRight || '0')).toBeGreaterThan(10);
      expect(Number.parseFloat(presentation.handleBottom || '0')).toBeGreaterThan(10);
      expect(noteSize.width).toBe(expectedWidth);
      expect(noteSize.height).toBe(expectedHeight);

      const colorBeforeHandleClick = await readItemColor(page);
      await clickResizeHandle(page, note);
      await page.waitForTimeout(350);
      await expect(note.locator('.grid-item-resize-handle')).toBeVisible();
      expect(await readItemColor(page)).toBe(colorBeforeHandleClick);
    });
  }

  for (const shape of NON_RECTANGULAR_SHAPES) {
    test(`${shape} uses responsive safe text bounds and alignment before and after resize`, async ({ page }) => {
      const localSnapshot = buildLocalSnapshot(`${shape} short text`, {
        shape,
        width: 280,
        height: 190
      });

      await prepareBlankPage(page);
      await seedLocalStorage(page, localSnapshot);
      await page.goto('/index.html');
      await page.waitForTimeout(650);

      const note = page.locator('.grid-item[data-id="item-1"]');
      await expect(note).toHaveAttribute('data-shape', shape);
      await expect(note.locator('.grid-item-text')).toBeVisible();

      const beforeSize = await readNoteSize(note);
      const beforeExpectedBounds = getExpectedShapeTextBounds(shape, beforeSize.width, beforeSize.height);
      const beforeLayout = await readItemTextFlowPresentation(note);
      expectTextLayoutMatchesBounds(beforeLayout, beforeExpectedBounds);
      expectTextAlignmentMatches(beforeLayout, beforeExpectedBounds);

      if (shape === 'triangle') {
        expect(beforeLayout.paddingTop).toBeGreaterThan(SHAPE_TEXT_INSETS.default.top);
      }

      if (shape === 'upsideDownTriangle') {
        expect(beforeLayout.paddingBottom).toBeGreaterThan(SHAPE_TEXT_INSETS.default.bottom);
      }

      await resizeNoteBy(page, note, 90, 30);
      await page.waitForTimeout(650);

      const afterSize = await readNoteSize(note);
      const afterExpectedBounds = getExpectedShapeTextBounds(shape, afterSize.width, afterSize.height);
      const afterLayout = await readItemTextFlowPresentation(note);
      expectTextLayoutMatchesBounds(afterLayout, afterExpectedBounds);
      expectTextAlignmentMatches(afterLayout, afterExpectedBounds);
      expect(afterLayout.safeHeight).toBeGreaterThan(beforeLayout.safeHeight);
      await expectNoteVisible(page, `${shape} short text`);
    });
  }

  for (const shape of NON_RECTANGULAR_SHAPES) {
    test(`${shape} keeps long wrapped text inside the responsive safe text box after resize`, async ({ page }) => {
      const localSnapshot = buildLocalSnapshot(
        `${shape} wrapped text stays inside the responsive safe bounds after resize.`,
        {
          shape,
          width: 280,
          height: 190
        }
      );

      await prepareBlankPage(page);
      await seedLocalStorage(page, localSnapshot);
      await page.goto('/index.html');
      await page.waitForTimeout(650);

      const note = page.locator('.grid-item[data-id="item-1"]');
      await expect(note).toHaveAttribute('data-shape', shape);
      await expect(note.locator('.grid-item-text')).toBeVisible();

      const beforeSize = await readNoteSize(note);
      const beforeExpectedBounds = getExpectedShapeTextBounds(shape, beforeSize.width, beforeSize.height);
      const beforeLayout = await readItemTextFlowPresentation(note);
      expectTextLayoutMatchesBounds(beforeLayout, beforeExpectedBounds);
      expect(beforeLayout.contentHeight).toBeLessThanOrEqual(beforeExpectedBounds.height + 1);
      expect(beforeLayout.textHeight).toBeLessThanOrEqual(beforeExpectedBounds.height + 1);

      await resizeNoteBy(page, note, 90, 30);
      await page.waitForTimeout(650);

      const afterSize = await readNoteSize(note);
      const afterExpectedBounds = getExpectedShapeTextBounds(shape, afterSize.width, afterSize.height);
      const afterLayout = await readItemTextFlowPresentation(note);
      expectTextLayoutMatchesBounds(afterLayout, afterExpectedBounds);
      expect(afterLayout.contentHeight).toBeLessThanOrEqual(afterExpectedBounds.height + 1);
      expect(afterLayout.textHeight).toBeLessThanOrEqual(afterExpectedBounds.height + 1);
      await expectNoteVisible(page, `${shape} wrapped text stays inside the responsive safe bounds after resize.`);
    });
  }

  for (const shape of NON_RECTANGULAR_SHAPES) {
    test(`${shape} edit mode reuses the same responsive safe text box after resize`, async ({ page }) => {
      const localSnapshot = buildLocalSnapshot(`${shape} edit alignment should reuse the responsive text box`, {
        shape,
        width: 280,
        height: 190
      });

      await prepareBlankPage(page);
      await seedLocalStorage(page, localSnapshot);
      await page.goto('/index.html');
      await page.waitForTimeout(650);

      const note = page.locator('.grid-item[data-id="item-1"]');
      await expect(note).toHaveAttribute('data-shape', shape);
      await expect(note.locator('.grid-item-text')).toBeVisible();

      await resizeNoteBy(page, note, 90, 30);
      await page.waitForTimeout(650);

      const resizedNoteSize = await readNoteSize(note);
      const expectedBounds = getExpectedShapeTextBounds(shape, resizedNoteSize.width, resizedNoteSize.height);
      const readOnlyLayout = await readItemTextFlowPresentation(note);
      expectTextLayoutMatchesBounds(readOnlyLayout, expectedBounds);
      expectTextAlignmentMatches(readOnlyLayout, expectedBounds);

      await note.dblclick();
      const textarea = note.locator('textarea.edit-textarea');
      await expect(textarea).toBeVisible();

      const editLayout = await readEditTextareaPresentation(note);
      expect(editLayout).not.toBeNull();
      expect(editLayout.paddingTop).toBe(10);
      expect(editLayout.paddingRight).toBe(15);
      expect(editLayout.paddingBottom).toBe(10);
      expect(editLayout.paddingLeft).toBe(10);
      expect(Math.abs(editLayout.top - expectedBounds.top)).toBeLessThanOrEqual(1);
      expect(Math.abs(editLayout.left - expectedBounds.left)).toBeLessThanOrEqual(1);
      expect(Math.abs(editLayout.width - expectedBounds.width)).toBeLessThanOrEqual(1);
      expect(editLayout.minHeight).toBe(Math.min(Math.max(expectedBounds.height, 40), 200));

      await textarea.press('Escape');
      await expect(textarea).toHaveCount(0);
      await expectNoteVisible(page, `${shape} edit alignment should reuse the responsive text box`);
    });
  }

  test('geometry regression fixture keeps non-rectangular safe bounds, edges, and marquee selection stable', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildGeometryRegressionSnapshot());
    await page.goto('/index.html');
    await page.waitForTimeout(650);

    await expect(page.locator('.grid-item')).toHaveCount(8);
    await expect(page.locator('.edge-line')).toHaveCount(3);

    const fixtureExpectations = [
      { itemId: 'fixture-circle', shape: 'circle' },
      { itemId: 'fixture-oval', shape: 'oval' },
      { itemId: 'fixture-diamond', shape: 'diamond' },
      { itemId: 'fixture-triangle', shape: 'triangle' },
      { itemId: 'fixture-updown-triangle', shape: 'upsideDownTriangle' },
      { itemId: 'fixture-hexagon', shape: 'hexagon' }
    ];

    for (const { itemId, shape } of fixtureExpectations) {
      const note = page.locator(`.grid-item[data-id="${itemId}"]`);
      const noteSize = await readNoteSize(note);
      const expectedBounds = getExpectedShapeTextBounds(shape, noteSize.width, noteSize.height);
      const layout = await readItemTextFlowPresentation(note);
      expectTextLayoutMatchesBounds(layout, expectedBounds);
      expectTextAlignmentMatches(layout, expectedBounds);
    }

    const edgeMarkers = await page.locator('.edge-line').evaluateAll((elements) =>
      elements.map((element) => element.getAttribute('marker-end') || '')
    );
    expect(edgeMarkers.filter((marker) => /^url\(#.+-arrow\)$/.test(marker)).length).toBe(2);
    expect(edgeMarkers.filter((marker) => marker === '').length).toBe(1);

    const circleNote = page.locator('.grid-item[data-id="fixture-circle"]');
    const diamondNote = page.locator('.grid-item[data-id="fixture-diamond"]');
    await marqueeSelectNotes(page, [circleNote, diamondNote], { margin: 8 });
    await expect(circleNote).toHaveClass(/selected/);
    await expect(diamondNote).toHaveClass(/selected/);
    await expect(page.locator('.grid-selection-marquee')).toHaveCount(0);
  });

  for (const shape of FIXED_SQUARE_RESIZE_SHAPES) {
    test(`${shape} resize keeps a fixed-ratio box, preserves interactions, and rerenders attached edges`, async ({ page }) => {
      const localSnapshot = buildLocalSnapshotWithItems([
        buildNoteItem(`${shape} Resize Subject`, {
          itemId: 'item-1',
          shape,
          width: 280,
          height: 190,
          position: { top: '40px', left: '320px' }
        }),
        buildNoteItem(`${shape} Resize Target`, {
          itemId: 'item-2',
          position: { top: '70px', left: '40px' }
        })
      ], {
        edges: [{
          id: 'edge-1',
          fromItemId: 'item-1',
          toItemId: 'item-2'
        }]
      });

      await prepareBlankPage(page);
      await seedLocalStorage(page, localSnapshot);
      await page.goto('/index.html');

      const note = page.locator('.grid-item[data-id="item-1"]');
      const edgeLine = page.locator('.edge-line').first();
      await expect.poll(async () => {
        const indexedDBState = await readIndexedDBState(page);
        return indexedDBState && indexedDBState.snapshot && indexedDBState.snapshot.tabs
          ? indexedDBState.snapshot.tabs
          : null;
      }).not.toBeNull();
      const initialItem = await readItemSnapshot(page);
      const initialColor = await readItemColor(page);
      const edgeBeforeResize = await readEdgeCoordinates(edgeLine);

      await expect(note).toHaveAttribute('data-shape', shape);
      await expect(note.locator('.grid-item-resize-handle')).toBeVisible();
      await expect(page.locator('textarea.edit-textarea')).toHaveCount(0);

      await page.waitForTimeout(600);
      const expectedResizedSide = 370;
      await resizeNoteBy(page, note, 90, 30);
      await expect.poll(async () => {
        const item = await readItemSnapshot(page);
        return `${item.width}x${item.height}`;
      }).toBe(`${expectedResizedSide}x${expectedResizedSide}`);

      const resizedItem = await readItemSnapshot(page);
      const resizedSize = await readNoteSize(note);
      const edgeAfterResize = await readEdgeCoordinates(edgeLine);
      expect(resizedItem.shape).toBe(shape);
      expect(resizedItem.width).toBe(resizedItem.height);
      expect(resizedItem.width).toBeGreaterThan(initialItem.width + 40);
      expect(resizedItem.position).toEqual(initialItem.position);
      expect(resizedItem.color).toBe(initialColor);
      expectItemUsesSharedSizeFieldsOnly(resizedItem);
      expect(resizedSize.width).toBe(resizedItem.width);
      expect(resizedSize.height).toBe(resizedItem.height);
      expect(Math.abs(resizedSize.width - resizedSize.height)).toBeLessThanOrEqual(1);
      expect(await readItemColor(page)).toBe(initialColor);
      await expect(page.locator('textarea.edit-textarea')).toHaveCount(0);
      expect(
        Math.abs(edgeAfterResize.x1 - edgeBeforeResize.x1) > 1
        || Math.abs(edgeAfterResize.y1 - edgeBeforeResize.y1) > 1
      ).toBe(true);

      await page.waitForTimeout(350);
      await note.click();
      await page.waitForTimeout(350);
      expect(await readItemColor(page)).not.toBe(initialColor);

      await page.reload();

      const reloadedNote = page.locator('.grid-item[data-id="item-1"]');
      await expect(reloadedNote).toHaveAttribute('data-shape', shape);
      await expect(reloadedNote.locator('.grid-item-resize-handle')).toBeVisible();

      const reloadedItem = await readItemSnapshot(page);
      const reloadedSize = await readNoteSize(reloadedNote);
      expect(reloadedItem.shape).toBe(shape);
      expect(reloadedItem.width).toBe(resizedItem.width);
      expect(reloadedItem.height).toBe(resizedItem.height);
      expect(reloadedItem.width).toBe(reloadedItem.height);
      expectItemUsesSharedSizeFieldsOnly(reloadedItem);
      expect(reloadedSize.width).toBe(resizedItem.width);
      expect(reloadedSize.height).toBe(resizedItem.height);
    });

    test(`${shape} resize normalizes the shared footprint through export/import and freeform shape cycling`, async ({ page }) => {
      const localSnapshot = buildLocalSnapshot(`${shape} Shared Footprint`, {
        width: 280,
        height: 190
      });

      await prepareBlankPage(page);
      await seedLocalStorage(page, localSnapshot);
      await page.goto('/index.html');

      const note = page.locator('.grid-item[data-id="item-1"]');
      await resizeNoteBy(page, note, 120, 80);
      const resizedDefaultItem = await readItemSnapshot(page);

      await cycleNoteShapeTo(note, shape);
      await expect(note).toHaveAttribute('data-shape', shape);
      await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

      const fixedRatioBeforeResize = await readItemSnapshot(page);
      expect(fixedRatioBeforeResize.width).toBe(resizedDefaultItem.width);
      expect(fixedRatioBeforeResize.height).toBe(resizedDefaultItem.height);
      expectItemUsesSharedSizeFieldsOnly(fixedRatioBeforeResize);

      await page.waitForTimeout(600);
      const expectedNormalizedSide = resizedDefaultItem.width + 80;
      await resizeNoteBy(page, note, 80, 20);
      await expect.poll(async () => {
        const item = await readItemSnapshot(page);
        return `${item.width}x${item.height}`;
      }).toBe(`${expectedNormalizedSide}x${expectedNormalizedSide}`);
      const normalizedFixedRatioItem = await readItemSnapshot(page);
      expect(normalizedFixedRatioItem.shape).toBe(shape);
      expect(normalizedFixedRatioItem.width).toBe(normalizedFixedRatioItem.height);
      expect(normalizedFixedRatioItem.width).toBeGreaterThan(resizedDefaultItem.width + 40);
      expectItemUsesSharedSizeFieldsOnly(normalizedFixedRatioItem);

      const exportedSnapshot = await exportCurrentSnapshot(page);
      const exportedTabs = JSON.parse(exportedSnapshot.tabs);
      const exportedItem = exportedTabs[0].items[0];
      expect(exportedSnapshot).not.toHaveProperty('width');
      expect(exportedSnapshot).not.toHaveProperty('height');
      expect(exportedItem.shape).toBe(shape);
      expect(exportedItem.width).toBe(normalizedFixedRatioItem.width);
      expect(exportedItem.height).toBe(normalizedFixedRatioItem.height);
      expectItemUsesSharedSizeFieldsOnly(exportedItem);

      await prepareBlankPage(page);
      await page.goto('/index.html');
      await importBackupSnapshot(page, exportedSnapshot);

      const importedNote = page.locator('.grid-item[data-id="item-1"]');
      await expect(importedNote).toHaveAttribute('data-shape', shape);
      await expect(importedNote.locator('.grid-item-resize-handle')).toBeVisible();

      const importedItem = await readItemSnapshot(page);
      expect(importedItem.shape).toBe(shape);
      expect(importedItem.width).toBe(normalizedFixedRatioItem.width);
      expect(importedItem.height).toBe(normalizedFixedRatioItem.height);
      expect(importedItem.width).toBe(importedItem.height);
      expectItemUsesSharedSizeFieldsOnly(importedItem);

      await cycleNoteShapeTo(importedNote, 'default');
      await expect(importedNote).toHaveAttribute('data-shape', 'default');
      await expect(importedNote.locator('.grid-item-resize-handle')).toBeVisible();

      const restoredDefaultSize = await readNoteSize(importedNote);
      expect(restoredDefaultSize.width).toBe(normalizedFixedRatioItem.width);
      expect(restoredDefaultSize.height).toBe(normalizedFixedRatioItem.height);

      await cycleNoteShapeTo(importedNote, 'oval');
      await expect(importedNote).toHaveAttribute('data-shape', 'oval');
      await expect(importedNote.locator('.grid-item-resize-handle')).toBeVisible();

      const restoredOvalSize = await readNoteSize(importedNote);
      expect(restoredOvalSize.width).toBe(normalizedFixedRatioItem.width);
      expect(restoredOvalSize.height).toBe(normalizedFixedRatioItem.height);

      const ovalItem = await readItemSnapshot(page);
      expect(ovalItem.shape).toBe('oval');
      expect(ovalItem.width).toBe(normalizedFixedRatioItem.width);
      expect(ovalItem.height).toBe(normalizedFixedRatioItem.height);
      expectItemUsesSharedSizeFieldsOnly(ovalItem);
    });
  }

  for (const shape of DERIVED_HEIGHT_RESIZE_SHAPES) {
    test(`${shape} resize keeps a derived-height box, preserves interactions, and rerenders attached edges`, async ({ page }) => {
      const localSnapshot = buildLocalSnapshotWithItems([
        buildNoteItem(`${shape} Resize Subject`, {
          itemId: 'item-1',
          shape,
          width: 280,
          height: 190,
          position: { top: '40px', left: '320px' }
        }),
        buildNoteItem(`${shape} Resize Target`, {
          itemId: 'item-2',
          position: { top: '70px', left: '40px' }
        })
      ], {
        edges: [{
          id: 'edge-1',
          fromItemId: 'item-1',
          toItemId: 'item-2'
        }]
      });

      await prepareBlankPage(page);
      await seedLocalStorage(page, localSnapshot);
      await page.goto('/index.html');

      const note = page.locator('.grid-item[data-id="item-1"]');
      const edgeLine = page.locator('.edge-line').first();
      await expect.poll(async () => {
        const indexedDBState = await readIndexedDBState(page);
        return indexedDBState && indexedDBState.snapshot && indexedDBState.snapshot.tabs
          ? indexedDBState.snapshot.tabs
          : null;
      }).not.toBeNull();
      const initialItem = await readItemSnapshot(page);
      const initialColor = await readItemColor(page);
      const edgeBeforeResize = await readEdgeCoordinates(edgeLine);

      await expect(note).toHaveAttribute('data-shape', shape);
      await expect(note.locator('.grid-item-resize-handle')).toBeVisible();
      await expect(page.locator('textarea.edit-textarea')).toHaveCount(0);

      await page.waitForTimeout(600);
      const expectedWidth = 370;
      const expectedHeight = getExpectedFixedRatioHeight(shape, expectedWidth);
      await resizeNoteBy(page, note, 90, 30);
      await expect.poll(async () => {
        const item = await readItemSnapshot(page);
        return `${item.width}x${item.height}`;
      }).toBe(`${expectedWidth}x${expectedHeight}`);

      const resizedItem = await readItemSnapshot(page);
      const resizedSize = await readNoteSize(note);
      const edgeAfterResize = await readEdgeCoordinates(edgeLine);
      expect(resizedItem.shape).toBe(shape);
      expect(resizedItem.width).toBe(expectedWidth);
      expect(resizedItem.height).toBe(expectedHeight);
      expect(resizedItem.position).toEqual(initialItem.position);
      expect(resizedItem.color).toBe(initialColor);
      expectItemUsesSharedSizeFieldsOnly(resizedItem);
      expect(resizedSize.width).toBe(expectedWidth);
      expect(resizedSize.height).toBe(expectedHeight);
      expect(await readItemColor(page)).toBe(initialColor);
      await expect(page.locator('textarea.edit-textarea')).toHaveCount(0);
      expect(
        Math.abs(edgeAfterResize.x1 - edgeBeforeResize.x1) > 1
        || Math.abs(edgeAfterResize.y1 - edgeBeforeResize.y1) > 1
      ).toBe(true);

      await page.waitForTimeout(350);
      await note.click();
      await page.waitForTimeout(350);
      expect(await readItemColor(page)).not.toBe(initialColor);

      await page.reload();

      const reloadedNote = page.locator('.grid-item[data-id="item-1"]');
      await expect(reloadedNote).toHaveAttribute('data-shape', shape);
      await expect(reloadedNote.locator('.grid-item-resize-handle')).toBeVisible();

      const reloadedItem = await readItemSnapshot(page);
      const reloadedSize = await readNoteSize(reloadedNote);
      expect(reloadedItem.shape).toBe(shape);
      expect(reloadedItem.width).toBe(expectedWidth);
      expect(reloadedItem.height).toBe(expectedHeight);
      expectItemUsesSharedSizeFieldsOnly(reloadedItem);
      expect(reloadedSize.width).toBe(expectedWidth);
      expect(reloadedSize.height).toBe(expectedHeight);
    });

    test(`${shape} resize normalizes the shared footprint through export/import and freeform shape cycling`, async ({ page }) => {
      const localSnapshot = buildLocalSnapshot(`${shape} Shared Footprint`, {
        width: 280,
        height: 190
      });

      await prepareBlankPage(page);
      await seedLocalStorage(page, localSnapshot);
      await page.goto('/index.html');

      const note = page.locator('.grid-item[data-id="item-1"]');
      await resizeNoteBy(page, note, 120, 80);
      const resizedDefaultItem = await readItemSnapshot(page);

      await cycleNoteShapeTo(note, shape);
      await expect(note).toHaveAttribute('data-shape', shape);
      await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

      const triangleBeforeResize = await readItemSnapshot(page);
      expect(triangleBeforeResize.width).toBe(resizedDefaultItem.width);
      expect(triangleBeforeResize.height).toBe(resizedDefaultItem.height);
      expectItemUsesSharedSizeFieldsOnly(triangleBeforeResize);

      await page.waitForTimeout(600);
      const expectedNormalizedWidth = resizedDefaultItem.width + 80;
      const expectedNormalizedHeight = getExpectedFixedRatioHeight(shape, expectedNormalizedWidth);
      await resizeNoteBy(page, note, 80, 20);
      await expect.poll(async () => {
        const item = await readItemSnapshot(page);
        return `${item.width}x${item.height}`;
      }).toBe(`${expectedNormalizedWidth}x${expectedNormalizedHeight}`);

      const normalizedTriangleItem = await readItemSnapshot(page);
      expect(normalizedTriangleItem.shape).toBe(shape);
      expect(normalizedTriangleItem.width).toBe(expectedNormalizedWidth);
      expect(normalizedTriangleItem.height).toBe(expectedNormalizedHeight);
      expectItemUsesSharedSizeFieldsOnly(normalizedTriangleItem);

      const exportedSnapshot = await exportCurrentSnapshot(page);
      const exportedTabs = JSON.parse(exportedSnapshot.tabs);
      const exportedItem = exportedTabs[0].items[0];
      expect(exportedSnapshot).not.toHaveProperty('width');
      expect(exportedSnapshot).not.toHaveProperty('height');
      expect(exportedItem.shape).toBe(shape);
      expect(exportedItem.width).toBe(expectedNormalizedWidth);
      expect(exportedItem.height).toBe(expectedNormalizedHeight);
      expectItemUsesSharedSizeFieldsOnly(exportedItem);

      await prepareBlankPage(page);
      await page.goto('/index.html');
      await importBackupSnapshot(page, exportedSnapshot);

      const importedNote = page.locator('.grid-item[data-id="item-1"]');
      await expect(importedNote).toHaveAttribute('data-shape', shape);
      await expect(importedNote.locator('.grid-item-resize-handle')).toBeVisible();

      const importedItem = await readItemSnapshot(page);
      expect(importedItem.shape).toBe(shape);
      expect(importedItem.width).toBe(expectedNormalizedWidth);
      expect(importedItem.height).toBe(expectedNormalizedHeight);
      expectItemUsesSharedSizeFieldsOnly(importedItem);

      await cycleNoteShapeTo(importedNote, 'default');
      await expect(importedNote).toHaveAttribute('data-shape', 'default');
      await expect(importedNote.locator('.grid-item-resize-handle')).toBeVisible();

      const restoredDefaultSize = await readNoteSize(importedNote);
      expect(restoredDefaultSize.width).toBe(expectedNormalizedWidth);
      expect(restoredDefaultSize.height).toBe(expectedNormalizedHeight);

      await cycleNoteShapeTo(importedNote, 'oval');
      await expect(importedNote).toHaveAttribute('data-shape', 'oval');
      await expect(importedNote.locator('.grid-item-resize-handle')).toBeVisible();

      const restoredOvalSize = await readNoteSize(importedNote);
      expect(restoredOvalSize.width).toBe(expectedNormalizedWidth);
      expect(restoredOvalSize.height).toBe(expectedNormalizedHeight);

      const ovalItem = await readItemSnapshot(page);
      expect(ovalItem.shape).toBe('oval');
      expect(ovalItem.width).toBe(expectedNormalizedWidth);
      expect(ovalItem.height).toBe(expectedNormalizedHeight);
      expectItemUsesSharedSizeFieldsOnly(ovalItem);
    });

    test(`cycling a resized oval through ${shape} restores the oval footprint on return`, async ({ page }) => {
      const localSnapshot = buildLocalSnapshot(`Oval Through ${shape}`, {
        shape: 'oval',
        width: 280,
        height: 190
      });

      await prepareBlankPage(page);
      await seedLocalStorage(page, localSnapshot);
      await page.goto('/index.html');

      const note = page.locator('.grid-item[data-id="item-1"]');
      await expect(note).toHaveAttribute('data-shape', 'oval');
      await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

      await cycleNoteShapeTo(note, shape);
      await expect(note).toHaveAttribute('data-shape', shape);
      await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

      const triangleSize = await readNoteSize(note);
      expect(triangleSize.width).toBe(280);
      expect(triangleSize.height).toBe(getExpectedFixedRatioHeight(shape, 280));

      await cycleNoteShapeTo(note, 'oval');
      await expect(note).toHaveAttribute('data-shape', 'oval');
      await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

      const restoredOvalSize = await readNoteSize(note);
      expect(restoredOvalSize.width).toBe(280);
      expect(restoredOvalSize.height).toBe(190);

      const restoredOvalItem = await readItemSnapshot(page);
      expect(restoredOvalItem.shape).toBe('oval');
      expect(restoredOvalItem.width).toBe(280);
      expect(restoredOvalItem.height).toBe(190);
      expectItemUsesSharedSizeFieldsOnly(restoredOvalItem);
    });
  }

  for (const shape of ['circle', 'diamond']) {
    test(`resized ${shape} still supports edit, drag, marquee selection, shape cycling, and delete`, async ({ page }) => {
      const localSnapshot = buildLocalSnapshot(`${shape} Interaction`, {
        shape,
        width: 260,
        height: 260
      });

      await prepareBlankPage(page);
      await seedLocalStorage(page, localSnapshot);
      await page.goto('/index.html');

      const note = page.locator('.grid-item[data-id="item-1"]');
      await expect(note).toHaveAttribute('data-shape', shape);
      await resizeNoteBy(page, note, 80, 20);
      await expect.poll(async () => {
        const item = await readItemSnapshot(page);
        return `${item.width}x${item.height}`;
      }).toBe('340x340');

      const resizedItem = await readItemSnapshot(page);
      const initialColor = await readItemColor(page);

      await note.dblclick();
      const textarea = page.locator('textarea.edit-textarea');
      await expect(textarea).toBeVisible();
      await textarea.fill(`${shape} Interaction Saved`);
      await textarea.press('Escape');
      await expect(textarea).toHaveCount(0);
      await expectNoteVisible(page, `${shape} Interaction Saved`);

      const editedItem = await readItemSnapshot(page);
      expect(editedItem.shape).toBe(shape);
      expect(editedItem.width).toBe(resizedItem.width);
      expect(editedItem.height).toBe(resizedItem.height);
      expect(editedItem.color).toBe(initialColor);

      await marqueeSelectNotes(page, [note]);
      await expect(note).toHaveClass(/selected/);
      await clickEmptyGrid(page);
      await expect(note).not.toHaveClass(/selected/);

      await dragNoteBy(page, note, 80, 45);
      await page.waitForTimeout(350);
      const draggedItem = await readItemSnapshot(page);
      expect(draggedItem.position).not.toEqual(editedItem.position);
      expect(draggedItem.color).toBe(initialColor);

      await cycleNoteShape(note);
      await expect(note).not.toHaveAttribute('data-shape', shape);
      await expect(page.locator('textarea.edit-textarea')).toHaveCount(0);
      expect(await readItemColor(page)).toBe(initialColor);

      await cycleNoteShape(note, ['Shift', 'Control']);
      await expect(note).toHaveAttribute('data-shape', shape);
      await expectNoteVisible(page, `${shape} Interaction Saved`);

      await note.click({ button: 'right' });
      await expect(page.locator('#confirmModal')).toBeVisible();
      await page.click('#confirmDelete');
      await expect(note).toHaveCount(0);
      await expect(page.locator('#confirmModal')).toBeHidden();
    });
  }

  for (const shape of DERIVED_HEIGHT_RESIZE_SHAPES) {
    test(`resized ${shape} still supports edit, drag, marquee selection, shape cycling, and delete`, async ({ page }) => {
      const localSnapshot = buildLocalSnapshot(`${shape} Interaction`, {
        shape,
        width: 280,
        height: 190
      });

      await prepareBlankPage(page);
      await seedLocalStorage(page, localSnapshot);
      await page.goto('/index.html');

      const note = page.locator('.grid-item[data-id="item-1"]');
      await expect(note).toHaveAttribute('data-shape', shape);
      await resizeNoteBy(page, note, 60, 20);

      const expectedWidth = 340;
      const expectedHeight = getExpectedFixedRatioHeight(shape, expectedWidth);
      await expect.poll(async () => {
        const item = await readItemSnapshot(page);
        return `${item.width}x${item.height}`;
      }).toBe(`${expectedWidth}x${expectedHeight}`);

      const resizedItem = await readItemSnapshot(page);
      const initialColor = await readItemColor(page);

      await note.dblclick();
      const textarea = page.locator('textarea.edit-textarea');
      await expect(textarea).toBeVisible();
      await textarea.fill(`${shape} Interaction Saved`);
      await textarea.press('Escape');
      await expect(textarea).toHaveCount(0);
      await expectNoteVisible(page, `${shape} Interaction Saved`);

      const editedItem = await readItemSnapshot(page);
      expect(editedItem.shape).toBe(shape);
      expect(editedItem.width).toBe(resizedItem.width);
      expect(editedItem.height).toBe(resizedItem.height);
      expect(editedItem.color).toBe(initialColor);

      await marqueeSelectNotes(page, [note]);
      await expect(note).toHaveClass(/selected/);
      await clickEmptyGrid(page);
      await expect(note).not.toHaveClass(/selected/);

      await dragNoteBy(page, note, 80, 45);
      await page.waitForTimeout(350);
      const draggedItem = await readItemSnapshot(page);
      expect(draggedItem.position).not.toEqual(editedItem.position);
      expect(draggedItem.color).toBe(initialColor);

      await cycleNoteShape(note);
      await expect(note).not.toHaveAttribute('data-shape', shape);
      await expect(page.locator('textarea.edit-textarea')).toHaveCount(0);
      expect(await readItemColor(page)).toBe(initialColor);

      await cycleNoteShape(note, ['Shift', 'Control']);
      await expect(note).toHaveAttribute('data-shape', shape);
      await expectNoteVisible(page, `${shape} Interaction Saved`);

      await note.click({ button: 'right' });
      await expect(page.locator('#confirmModal')).toBeVisible();
      await page.click('#confirmDelete');
      await expect(note).toHaveCount(0);
      await expect(page.locator('#confirmModal')).toBeHidden();
    });
  }

  test('fixed-ratio shapes preserve the shared footprint in data without stretching into the stored rectangle during shape cycling', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Fixed Ratio Shape Policy');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await resizeNoteBy(page, note, 260, 160);
    const resizedItem = await readItemSnapshot(page);
    const initialColor = await readItemColor(page);

    for (const nextShape of FIXED_RATIO_SHAPES) {
      await cycleNoteShape(note);
      await expect(note).toHaveAttribute('data-shape', nextShape);
      if (RESIZABLE_FIXED_RATIO_SHAPES.includes(nextShape)) {
        await expect(note.locator('.grid-item-resize-handle')).toBeVisible();
      } else {
        await expect(note.locator('.grid-item-resize-handle')).toHaveCount(0);
      }
      await expect(page.locator('textarea.edit-textarea')).toHaveCount(0);
      await expectNoteVisible(page, 'Fixed Ratio Shape Policy');

      const cycledItem = await readItemSnapshot(page);
      expect(cycledItem.shape).toBe(nextShape);
      expect(cycledItem.width).toBe(resizedItem.width);
      expect(cycledItem.height).toBe(resizedItem.height);
      expect(cycledItem.position).toEqual(resizedItem.position);
      expect(cycledItem.color).toBe(initialColor);
      expectItemUsesSharedSizeFieldsOnly(cycledItem);
      expect(await readItemColor(page)).toBe(initialColor);

      const cycledSize = await readNoteSize(note);
      expect(cycledSize.width).toBe(resizedItem.width);
      expect(cycledSize.height).toBe(getExpectedFixedRatioHeight(nextShape, resizedItem.width));
      expect(cycledSize.width === resizedItem.width && cycledSize.height === resizedItem.height).toBe(false);
    }
  });

  test('cycling from a fixed-ratio shape back to oval restores the stored rectangular footprint and resize handle', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Fixed Ratio Back To Oval');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await resizeNoteBy(page, note, 180, 120);
    const resizedItem = await readItemSnapshot(page);

    await cycleNoteShapeTo(note, 'square');
    await expect(note).toHaveAttribute('data-shape', 'square');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    await cycleNoteShapeTo(note, 'oval');
    await expect(note).toHaveAttribute('data-shape', 'oval');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    const ovalSize = await readNoteSize(note);
    expect(ovalSize.width).toBe(resizedItem.width);
    expect(ovalSize.height).toBe(resizedItem.height);

    const ovalItem = await readItemSnapshot(page);
    expect(ovalItem.shape).toBe('oval');
    expect(ovalItem.width).toBe(resizedItem.width);
    expect(ovalItem.height).toBe(resizedItem.height);
    expectItemUsesSharedSizeFieldsOnly(ovalItem);
  });

  test('cycling a resized oval through a fixed-ratio shape restores the oval footprint on return', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Oval Through Fixed Ratio', {
      shape: 'oval',
      width: 280,
      height: 190
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await expect(note).toHaveAttribute('data-shape', 'oval');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    await cycleNoteShapeTo(note, 'circle', 'reverse');
    await expect(note).toHaveAttribute('data-shape', 'circle');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    const circleSize = await readNoteSize(note);
    expect(circleSize.width).toBe(280);
    expect(circleSize.height).toBe(getExpectedFixedRatioHeight('circle', 280));

    await cycleNoteShapeTo(note, 'oval');
    await expect(note).toHaveAttribute('data-shape', 'oval');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    const restoredOvalSize = await readNoteSize(note);
    expect(restoredOvalSize.width).toBe(280);
    expect(restoredOvalSize.height).toBe(190);

    const restoredOvalItem = await readItemSnapshot(page);
    expect(restoredOvalItem.shape).toBe('oval');
    expect(restoredOvalItem.width).toBe(280);
    expect(restoredOvalItem.height).toBe(190);
    expectItemUsesSharedSizeFieldsOnly(restoredOvalItem);
  });

  test('reverse shape cycling back to default restores the saved size and resize handle', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Reverse Shape Cycle');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await resizeNoteBy(page, note, 160, 110);
    const resizedItem = await readItemSnapshot(page);

    await cycleNoteShape(note);
    await expect(note).toHaveAttribute('data-shape', 'circle');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    await cycleNoteShape(note, ['Shift', 'Control']);
    await expect(note).toHaveAttribute('data-shape', 'default');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    const restoredSize = await readNoteSize(note);
    expect(restoredSize.width).toBe(resizedItem.width);
    expect(restoredSize.height).toBe(resizedItem.height);

    await resizeNoteBy(page, note, 60, 40);
    const updatedItem = await readItemSnapshot(page);
    expectItemUsesSharedSizeFieldsOnly(updatedItem);
    expect(updatedItem.width).toBeGreaterThan(resizedItem.width + 20);
    expect(updatedItem.height).toBeGreaterThan(resizedItem.height + 10);
  });

  test('reload after shape cycling a resized note to a non-default shape stays consistent', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Reload Shape Cycle');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await resizeNoteBy(page, note, 180, 120);
    const resizedItem = await readItemSnapshot(page);

    await cycleNoteShapeTo(note, 'hexagon');
    await expect(note).toHaveAttribute('data-shape', 'hexagon');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    await page.reload();
    const reloadedNote = page.locator('.grid-item[data-id="item-1"]');
    await expect(reloadedNote).toHaveAttribute('data-shape', 'hexagon');
    await expect(reloadedNote.locator('.grid-item-resize-handle')).toBeVisible();
    await expectNoteVisible(page, 'Reload Shape Cycle');

    const reloadedItem = await readItemSnapshot(page);
    expect(reloadedItem.shape).toBe('hexagon');
    expect(reloadedItem.width).toBe(resizedItem.width);
    expect(reloadedItem.height).toBe(resizedItem.height);
    expectItemUsesSharedSizeFieldsOnly(reloadedItem);
  });

  test('shape cycling a resized note to a fixed-ratio shape keeps edge rendering clean', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Edge Shape Left', {
        itemId: 'item-1',
        position: { top: '40px', left: '40px' }
      }),
      buildNoteItem('Edge Shape Right', {
        itemId: 'item-2',
        position: { top: '60px', left: '360px' }
      })
    ], {
      edges: [{
        id: 'edge-1',
        fromItemId: 'item-1',
        toItemId: 'item-2'
      }]
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const edgeLine = page.locator('.edge-line').first();

    await resizeNoteBy(page, note, 180, 120);
    const beforeShapeCycle = await readEdgeCoordinates(edgeLine);

    await cycleNoteShapeTo(note, 'hexagon');
    await expect(note).toHaveAttribute('data-shape', 'hexagon');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    const afterShapeCycle = await readEdgeCoordinates(edgeLine);
    Object.values(afterShapeCycle).forEach((coordinate) => {
      expect(Number.isFinite(coordinate)).toBe(true);
    });
    expect(
      Math.abs(afterShapeCycle.x1 - beforeShapeCycle.x1) > 1
      || Math.abs(afterShapeCycle.y1 - beforeShapeCycle.y1) > 1
    ).toBe(true);
  });

  test('oval resize rerenders attached edge endpoints and keeps interaction side effects suppressed', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Oval Edge Left', {
        itemId: 'item-1',
        position: { top: '40px', left: '40px' }
      }),
      buildNoteItem('Oval Edge Right', {
        itemId: 'item-2',
        position: { top: '70px', left: '360px' }
      })
    ], {
      edges: [{
        id: 'edge-1',
        fromItemId: 'item-1',
        toItemId: 'item-2'
      }]
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const edgeLine = page.locator('.edge-line').first();
    await resizeNoteBy(page, note, 180, 120);
    await cycleNoteShapeTo(note, 'oval');
    await expect(note).toHaveAttribute('data-shape', 'oval');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    const colorBeforeOvalResize = await readItemColor(page);
    const itemBeforeOvalResize = await readItemSnapshot(page);
    const edgeBeforeOvalResize = await readEdgeCoordinates(edgeLine);

    await resizeNoteBy(page, note, 90, 50);
    await page.waitForTimeout(350);

    const itemAfterOvalResize = await readItemSnapshot(page);
    const edgeAfterOvalResize = await readEdgeCoordinates(edgeLine);
    expect(itemAfterOvalResize.position).toEqual(itemBeforeOvalResize.position);
    expect(itemAfterOvalResize.width).toBeGreaterThan(itemBeforeOvalResize.width + 20);
    expect(itemAfterOvalResize.height).toBeGreaterThan(itemBeforeOvalResize.height + 20);
    expect(itemAfterOvalResize.color).toBe(colorBeforeOvalResize);
    expectItemUsesSharedSizeFieldsOnly(itemAfterOvalResize);
    expect(await readItemColor(page)).toBe(colorBeforeOvalResize);
    await expect(page.locator('textarea.edit-textarea')).toHaveCount(0);
    expect(
      Math.abs(edgeAfterOvalResize.x1 - edgeBeforeOvalResize.x1) > 1
      || Math.abs(edgeAfterOvalResize.y1 - edgeBeforeOvalResize.y1) > 1
    ).toBe(true);
  });

  test('oval still edits correctly with an explicit resized footprint', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Oval Edit After Resize', {
      shape: 'oval',
      width: 280,
      height: 190
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await expect(note).toHaveAttribute('data-shape', 'oval');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    await note.dblclick();
    const textarea = page.locator('textarea.edit-textarea');
    await expect(textarea).toBeVisible();
    await expect(textarea).toBeFocused();
    await expect(textarea).toHaveValue('Oval Edit After Resize');

    await textarea.fill('Oval Edit Saved');
    await textarea.press('Escape');

    await expect(textarea).toHaveCount(0);
    await expectNoteVisible(page, 'Oval Edit Saved');

    const savedItem = await readItemSnapshot(page);
    expect(savedItem.shape).toBe('oval');
    expect(savedItem.width).toBe(280);
    expect(savedItem.height).toBe(190);
    expectItemUsesSharedSizeFieldsOnly(savedItem);
  });

  test('export and import round-trip preserves only shared width and height fields and restores the rectangular footprint on oval', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Export Import Shape Cycle');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await resizeNoteBy(page, note, 210, 140);
    const resizedItem = await readItemSnapshot(page);

    await cycleNoteShapeTo(note, 'square');
    await expect(note).toHaveAttribute('data-shape', 'square');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();

    const exportedSnapshot = await exportCurrentSnapshot(page);
    expect(Object.keys(exportedSnapshot).sort()).toEqual([
      'activeTabId',
      'hasRunBefore',
      'tabs',
      'theme'
    ]);
    const exportedTabs = JSON.parse(exportedSnapshot.tabs);
    expect(exportedTabs[0].items[0]).toMatchObject({
      name: 'Export Import Shape Cycle',
      shape: 'square',
      width: resizedItem.width,
      height: resizedItem.height
    });
    expectItemUsesSharedSizeFieldsOnly(exportedTabs[0].items[0]);
    expect(exportedSnapshot).not.toHaveProperty('width');
    expect(exportedSnapshot).not.toHaveProperty('height');

    await prepareBlankPage(page);
    await page.goto('/index.html');
    await importBackupSnapshot(page, exportedSnapshot);

    const importedNote = page.locator('.grid-item[data-id="item-1"]');
    await expect(importedNote).toHaveAttribute('data-shape', 'square');
    await expect(importedNote.locator('.grid-item-resize-handle')).toBeVisible();
    await expectNoteVisible(page, 'Export Import Shape Cycle');

    const importedItem = await readItemSnapshot(page);
    expect(importedItem.shape).toBe('square');
    expect(importedItem.width).toBe(resizedItem.width);
    expect(importedItem.height).toBe(resizedItem.height);
    expectItemUsesSharedSizeFieldsOnly(importedItem);

    await cycleNoteShapeTo(importedNote, 'oval');
    await expect(importedNote).toHaveAttribute('data-shape', 'oval');
    await expect(importedNote.locator('.grid-item-resize-handle')).toBeVisible();

    const restoredSize = await readNoteSize(importedNote);
    expect(restoredSize.width).toBe(resizedItem.width);
    expect(restoredSize.height).toBe(resizedItem.height);

    const ovalImportedItem = await readItemSnapshot(page);
    expect(ovalImportedItem.shape).toBe('oval');
    expect(ovalImportedItem.width).toBe(resizedItem.width);
    expect(ovalImportedItem.height).toBe(resizedItem.height);
    expectItemUsesSharedSizeFieldsOnly(ovalImportedItem);
  });

  test('exports resized note dimensions only inside the tabs payload and keeps top-level keys unchanged', async ({ page }) => {
    const localSnapshot = Object.assign(
      buildLocalSnapshot('Export Sized Note', { theme: 'dark' }),
      {
        hideInstructions: 'true',
        corporateMode: 'false',
        defaultColorEnabled: 'true',
        defaultColor: '#112233'
      }
    );
    const expectedTopLevelKeys = [
      'activeTabId',
      'corporateMode',
      'defaultColor',
      'defaultColorEnabled',
      'hasRunBefore',
      'hideInstructions',
      'tabs',
      'theme'
    ];

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const resizedBeforeExport = await readNoteSize(note);
    await resizeNoteBy(page, note, 100, 70);
    const resizedAfterExport = await readNoteSize(note);
    expect(resizedAfterExport.width).toBeGreaterThan(resizedBeforeExport.width + 40);
    expect(resizedAfterExport.height).toBeGreaterThan(resizedBeforeExport.height + 20);

    const exportedSnapshot = await exportCurrentSnapshot(page);
    expect(Object.keys(exportedSnapshot).sort()).toEqual(expectedTopLevelKeys);
    expect(exportedSnapshot).not.toHaveProperty('width');
    expect(exportedSnapshot).not.toHaveProperty('height');

    const exportedTabs = JSON.parse(exportedSnapshot.tabs);
    const exportedItem = exportedTabs[0].items[0];
    expectItemUsesSharedSizeFieldsOnly(exportedItem);
    expect(exportedItem.width).toBe(resizedAfterExport.width);
    expect(exportedItem.height).toBe(resizedAfterExport.height);
  });

  test('dragging a note does not color-cycle on mouse release', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Drag Without Recolor');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const initialColor = await readItemColor(page);

    await dragNoteBy(page, note, 100, 50);
    await page.waitForTimeout(350);

    expect(await readItemColor(page)).toBe(initialColor);
  });

  test('rerenders attached edge endpoints when a default note is resized', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Left Note', {
        itemId: 'item-1',
        position: { top: '40px', left: '40px' }
      }),
      buildNoteItem('Right Note', {
        itemId: 'item-2',
        position: { top: '40px', left: '320px' }
      })
    ], {
      edges: [{
        id: 'edge-1',
        fromItemId: 'item-1',
        toItemId: 'item-2'
      }]
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const edgeLine = page.locator('.edge-line').first();
    const beforeEdge = await edgeLine.evaluate((element) => ({
      x1: Number(element.getAttribute('x1')),
      y1: Number(element.getAttribute('y1'))
    }));

    await resizeNoteBy(page, note, 100, 60);

    const afterEdge = await edgeLine.evaluate((element) => ({
      x1: Number(element.getAttribute('x1')),
      y1: Number(element.getAttribute('y1'))
    }));

    expect(afterEdge.x1).toBeGreaterThan(beforeEdge.x1 + 20);
    expect(afterEdge.y1).toBeGreaterThan(beforeEdge.y1 + 10);
  });

  test('Ctrl+drag creates a directed edge without note interaction side effects', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Arrow Source', {
        itemId: 'item-1',
        position: { top: '100px', left: '140px' }
      }),
      buildNoteItem('Arrow Target', {
        itemId: 'item-2',
        position: { top: '140px', left: '420px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const sourceNote = page.locator('.grid-item[data-id="item-1"]');
    const targetNote = page.locator('.grid-item[data-id="item-2"]');
    const beforeSource = await readItemSnapshot(page, 'item-1');
    const colorBefore = await readItemColor(page, 'item-1');

    await drawEdgeBetweenNotes(page, sourceNote, targetNote, { modifier: 'Control' });

    const tabsSnapshot = await readTabsSnapshot(page);
    expect(tabsSnapshot[0].edges).toHaveLength(1);
    expect(tabsSnapshot[0].edges[0]).toMatchObject({
      fromItemId: 'item-1',
      toItemId: 'item-2',
      kind: 'arrow'
    });

    const edgeLine = page.locator('.edge-line').first();
    const edgePresentation = await readEdgePresentation(edgeLine);
    expect(edgePresentation.markerEnd).toMatch(/^url\(#.+-arrow\)$/);

    const afterSource = await readItemSnapshot(page, 'item-1');
    expect(afterSource.position).toEqual(beforeSource.position);
    expect(await readItemColor(page, 'item-1')).toBe(colorBefore);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual([]);
    await expect(page.locator('.grid-selection-marquee')).toHaveCount(0);
    await expect(page.locator('textarea.edit-textarea')).toHaveCount(0);
  });

  test('legacy edges without kind still render as normal lines and stay unmigrated', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Legacy Edge Source', {
        itemId: 'item-1',
        position: { top: '100px', left: '140px' }
      }),
      buildNoteItem('Legacy Edge Target', {
        itemId: 'item-2',
        position: { top: '140px', left: '420px' }
      })
    ], {
      edges: [{
        id: 'edge-1',
        fromItemId: 'item-1',
        toItemId: 'item-2'
      }]
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const storedEdge = await readEdgeSnapshot(page, 'edge-1');
    expect(storedEdge).not.toBeNull();
    expect(Object.prototype.hasOwnProperty.call(storedEdge, 'kind')).toBe(false);

    const edgePresentation = await readEdgePresentation(page.locator('.edge-line').first());
    expect(edgePresentation.markerEnd).toBe('');
  });

  test('arrow edges rerender attached endpoints when a default note is resized', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Arrow Left', {
        itemId: 'item-1',
        position: { top: '40px', left: '40px' }
      }),
      buildNoteItem('Arrow Right', {
        itemId: 'item-2',
        position: { top: '40px', left: '320px' }
      })
    ], {
      edges: [{
        id: 'edge-1',
        fromItemId: 'item-1',
        toItemId: 'item-2',
        kind: 'arrow'
      }]
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const edgeLine = page.locator('.edge-line').first();
    const beforeEdge = await readEdgeCoordinates(edgeLine);
    const beforePresentation = await readEdgePresentation(edgeLine);

    await resizeNoteBy(page, note, 100, 60);

    const afterEdge = await readEdgeCoordinates(edgeLine);
    const afterPresentation = await readEdgePresentation(edgeLine);
    expect(afterEdge.x1).toBeGreaterThan(beforeEdge.x1 + 20);
    expect(afterEdge.y1).toBeGreaterThan(beforeEdge.y1 + 10);
    expect(beforePresentation.markerEnd).toMatch(/^url\(#.+-arrow\)$/);
    expect(afterPresentation.markerEnd).toMatch(/^url\(#.+-arrow\)$/);
  });

  test('arrow edges keep directional marker rendering through shape cycling', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Cycle Arrow Source', {
        itemId: 'item-1',
        position: { top: '80px', left: '120px' },
        width: 280,
        height: 190
      }),
      buildNoteItem('Cycle Arrow Target', {
        itemId: 'item-2',
        position: { top: '110px', left: '430px' }
      })
    ], {
      edges: [{
        id: 'edge-1',
        fromItemId: 'item-1',
        toItemId: 'item-2',
        kind: 'arrow'
      }]
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const edgeLine = page.locator('.edge-line').first();
    const beforeEdge = await readEdgeCoordinates(edgeLine);

    await cycleNoteShapeTo(note, 'hexagon');
    await expect(note).toHaveAttribute('data-shape', 'hexagon');

    const afterEdge = await readEdgeCoordinates(edgeLine);
    const afterPresentation = await readEdgePresentation(edgeLine);
    expect(
      Math.abs(afterEdge.x1 - beforeEdge.x1) > 1
      || Math.abs(afterEdge.y1 - beforeEdge.y1) > 1
    ).toBe(true);
    expect(afterPresentation.markerEnd).toMatch(/^url\(#.+-arrow\)$/);
  });

  for (const shape of NON_RECTANGULAR_SHAPES) {
    test(`arrow endpoint attaches to the visible ${shape} boundary`, async ({ page }) => {
      await prepareBlankPage(page);
      await seedLocalStorage(page, buildShapeAwareArrowSnapshot(shape));
      await page.goto('/index.html');

      const sourceNote = page.locator('.grid-item[data-id="item-1"]');
      const targetNote = page.locator('.grid-item[data-id="item-2"]');
      const edgeLine = page.locator('.edge-line').first();

      const sourceGeometry = await readNoteOffsetGeometry(sourceNote);
      const targetGeometry = await readNoteOffsetGeometry(targetNote);
      const edgeCoordinates = await readEdgeCoordinates(edgeLine);
      const edgePresentation = await readEdgePresentation(edgeLine);
      const expectedTargetAnchor = getExpectedShapeAnchorPoint(shape, {
        left: targetGeometry.left,
        top: targetGeometry.top,
        width: targetGeometry.width,
        height: targetGeometry.height
      }, {
        x: sourceGeometry.centerX,
        y: sourceGeometry.centerY
      });

      expectPointNear({ x: edgeCoordinates.x1, y: edgeCoordinates.y1 }, {
        x: sourceGeometry.centerX,
        y: sourceGeometry.centerY
      });
      expectPointNear({ x: edgeCoordinates.x2, y: edgeCoordinates.y2 }, expectedTargetAnchor);
      expect(Math.hypot(
        edgeCoordinates.x2 - targetGeometry.centerX,
        edgeCoordinates.y2 - targetGeometry.centerY
      )).toBeGreaterThan(10);
      expect(edgePresentation.markerEnd).toMatch(/^url\(#.+-arrow\)$/);
    });
  }

  for (const shape of NON_RECTANGULAR_SHAPES) {
    test(`arrow endpoint rerenders when a ${shape} target is resized`, async ({ page }) => {
      await prepareBlankPage(page);
      await seedLocalStorage(page, buildShapeAwareArrowSnapshot(shape));
      await page.goto('/index.html');

      const sourceNote = page.locator('.grid-item[data-id="item-1"]');
      const targetNote = page.locator('.grid-item[data-id="item-2"]');
      const edgeLine = page.locator('.edge-line').first();
      const beforeEdge = await readEdgeCoordinates(edgeLine);

      await resizeNoteBy(page, targetNote, 90, 30);

      const sourceGeometry = await readNoteOffsetGeometry(sourceNote);
      const targetGeometry = await readNoteOffsetGeometry(targetNote);
      const afterEdge = await readEdgeCoordinates(edgeLine);
      const afterPresentation = await readEdgePresentation(edgeLine);
      const expectedTargetAnchor = getExpectedShapeAnchorPoint(shape, {
        left: targetGeometry.left,
        top: targetGeometry.top,
        width: targetGeometry.width,
        height: targetGeometry.height
      }, {
        x: sourceGeometry.centerX,
        y: sourceGeometry.centerY
      });

      expect(
        Math.abs(afterEdge.x2 - beforeEdge.x2) > 1
        || Math.abs(afterEdge.y2 - beforeEdge.y2) > 1
      ).toBe(true);
      expectPointNear({ x: afterEdge.x2, y: afterEdge.y2 }, expectedTargetAnchor);
      expect(afterPresentation.markerEnd).toMatch(/^url\(#.+-arrow\)$/);
    });
  }

  test('dragging either endpoint item rerenders a shape-aware arrow endpoint', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildShapeAwareArrowSnapshot('triangle'));
    await page.goto('/index.html');

    const sourceNote = page.locator('.grid-item[data-id="item-1"]');
    const targetNote = page.locator('.grid-item[data-id="item-2"]');
    const edgeLine = page.locator('.edge-line').first();
    const beforeEdge = await readEdgeCoordinates(edgeLine);

    await dragNoteBy(page, targetNote, 80, 30);
    const afterTargetDragEdge = await readEdgeCoordinates(edgeLine);
    expect(
      Math.abs(afterTargetDragEdge.x2 - beforeEdge.x2) > 1
      || Math.abs(afterTargetDragEdge.y2 - beforeEdge.y2) > 1
    ).toBe(true);

    await dragNoteBy(page, sourceNote, 60, 20);
    const sourceGeometry = await readNoteOffsetGeometry(sourceNote);
    const targetGeometry = await readNoteOffsetGeometry(targetNote);
    const afterSourceDragEdge = await readEdgeCoordinates(edgeLine);
    const expectedTargetAnchor = getExpectedShapeAnchorPoint('triangle', {
      left: targetGeometry.left,
      top: targetGeometry.top,
      width: targetGeometry.width,
      height: targetGeometry.height
    }, {
      x: sourceGeometry.centerX,
      y: sourceGeometry.centerY
    });

    expect(
      Math.abs(afterSourceDragEdge.x1 - afterTargetDragEdge.x1) > 1
      || Math.abs(afterSourceDragEdge.y1 - afterTargetDragEdge.y1) > 1
    ).toBe(true);
    expectPointNear({ x: afterSourceDragEdge.x1, y: afterSourceDragEdge.y1 }, {
      x: sourceGeometry.centerX,
      y: sourceGeometry.centerY
    });
    expectPointNear({ x: afterSourceDragEdge.x2, y: afterSourceDragEdge.y2 }, expectedTargetAnchor);
  });

  test('line edges still end at the target center for non-rectangular shapes', async ({ page }) => {
    const targetSize = SHAPE_AWARE_ARROW_TARGET_SIZES.circle;
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Line Source', {
        itemId: 'item-1',
        position: { top: '80px', left: '80px' },
        width: 260,
        height: 170
      }),
      buildNoteItem('Circle Line Target', {
        itemId: 'item-2',
        shape: 'circle',
        position: { top: '250px', left: '430px' },
        width: targetSize.width,
        height: targetSize.height
      })
    ], {
      edges: [{
        id: 'edge-1',
        fromItemId: 'item-1',
        toItemId: 'item-2',
        kind: 'line'
      }]
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const targetGeometry = await readNoteOffsetGeometry(page.locator('.grid-item[data-id="item-2"]'));
    const edgeCoordinates = await readEdgeCoordinates(page.locator('.edge-line').first());
    const edgePresentation = await readEdgePresentation(page.locator('.edge-line').first());

    expectPointNear({ x: edgeCoordinates.x2, y: edgeCoordinates.y2 }, {
      x: targetGeometry.centerX,
      y: targetGeometry.centerY
    });
    expect(edgePresentation.markerEnd).toBe('');
  });

  test('edge hover still arms the line before context delete', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildShapeAwareArrowSnapshot('hexagon'));
    await page.goto('/index.html');

    const edgeLine = page.locator('.edge-line').first();
    await edgeLine.evaluate((element) => {
      const group = element.parentElement;
      const hit = group ? group.querySelector('.edge-hit-line') : null;
      if (!hit) {
        throw new Error('Edge hit line was not available.');
      }

      hit.dispatchEvent(new MouseEvent('mouseenter', {
        bubbles: false,
        cancelable: false
      }));
    });
    await page.waitForTimeout(EDGE_ARM_DELAY_MS + 150);

    const armedPresentation = await readEdgePresentation(edgeLine);
    expect(armedPresentation.armed).toBe(true);
    expect(armedPresentation.markerEnd).toMatch(/^url\(#.+-arrow-armed\)$/);

    await edgeLine.evaluate((element) => {
      const group = element.parentElement;
      const hit = group ? group.querySelector('.edge-hit-line') : null;
      if (!hit) {
        throw new Error('Edge hit line was not available.');
      }

      hit.dispatchEvent(new MouseEvent('mouseleave', {
        bubbles: false,
        cancelable: false
      }));
    });

    const relaxedPresentation = await readEdgePresentation(edgeLine);
    expect(relaxedPresentation.armed).toBe(false);
  });

  test('duplicate prevention blocks additional connections once a line exists regardless of kind or direction', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Line Source', {
        itemId: 'item-1',
        position: { top: '100px', left: '140px' }
      }),
      buildNoteItem('Line Target', {
        itemId: 'item-2',
        position: { top: '140px', left: '420px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const sourceNote = page.locator('.grid-item[data-id="item-1"]');
    const targetNote = page.locator('.grid-item[data-id="item-2"]');

    await drawEdgeBetweenNotes(page, sourceNote, targetNote);
    expect((await readTabsSnapshot(page))[0].edges).toHaveLength(1);

    await drawEdgeBetweenNotes(page, sourceNote, targetNote);
    expect((await readTabsSnapshot(page))[0].edges).toHaveLength(1);

    await drawEdgeBetweenNotes(page, sourceNote, targetNote, { modifier: 'Control' });
    expect((await readTabsSnapshot(page))[0].edges).toHaveLength(1);

    await drawEdgeBetweenNotes(page, targetNote, sourceNote, { modifier: 'Control' });
    const finalEdges = (await readTabsSnapshot(page))[0].edges;
    expect(finalEdges).toHaveLength(1);
    expect(finalEdges[0].kind).toBe('line');
  });

  test('duplicate prevention blocks a line once an arrow already exists on the pair', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Arrow First Source', {
        itemId: 'item-1',
        position: { top: '100px', left: '140px' }
      }),
      buildNoteItem('Arrow First Target', {
        itemId: 'item-2',
        position: { top: '140px', left: '420px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const sourceNote = page.locator('.grid-item[data-id="item-1"]');
    const targetNote = page.locator('.grid-item[data-id="item-2"]');

    await drawEdgeBetweenNotes(page, sourceNote, targetNote, { modifier: 'Control' });
    expect((await readTabsSnapshot(page))[0].edges).toHaveLength(1);

    await drawEdgeBetweenNotes(page, sourceNote, targetNote);
    await drawEdgeBetweenNotes(page, targetNote, sourceNote);

    const finalEdges = (await readTabsSnapshot(page))[0].edges;
    expect(finalEdges).toHaveLength(1);
    expect(finalEdges[0].kind).toBe('arrow');
  });

  test('edge delete still works for both line and arrow edges', async ({ page }) => {
    const lineSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Delete Line Source', {
        itemId: 'item-1',
        position: { top: '100px', left: '140px' }
      }),
      buildNoteItem('Delete Line Target', {
        itemId: 'item-2',
        position: { top: '140px', left: '420px' }
      })
    ], {
      edges: [{
        id: 'edge-1',
        fromItemId: 'item-1',
        toItemId: 'item-2',
        kind: 'line'
      }]
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, lineSnapshot);
    await page.goto('/index.html');

    await deleteEdgeViaContextMenu(page, page.locator('.edge-line').first());
    await expect(page.locator('.edge-line')).toHaveCount(0);
    expect((await readTabsSnapshot(page))[0].edges).toHaveLength(0);

    const arrowSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Delete Arrow Source', {
        itemId: 'item-1',
        position: { top: '100px', left: '140px' }
      }),
      buildNoteItem('Delete Arrow Target', {
        itemId: 'item-2',
        position: { top: '140px', left: '420px' }
      })
    ], {
      edges: [{
        id: 'edge-1',
        fromItemId: 'item-1',
        toItemId: 'item-2',
        kind: 'arrow'
      }]
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, arrowSnapshot);
    await page.goto('/index.html');

    await deleteEdgeViaContextMenu(page, page.locator('.edge-line').first());
    await expect(page.locator('.edge-line')).toHaveCount(0);
    expect((await readTabsSnapshot(page))[0].edges).toHaveLength(0);
  });

  test('export and import round-trip preserves arrow kind only inside the tabs payload', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Export Arrow Source', {
        itemId: 'item-1',
        position: { top: '100px', left: '140px' }
      }),
      buildNoteItem('Export Arrow Target', {
        itemId: 'item-2',
        position: { top: '140px', left: '420px' }
      })
    ], {
      edges: [{
        id: 'edge-1',
        fromItemId: 'item-1',
        toItemId: 'item-2',
        kind: 'arrow'
      }]
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const exportedSnapshot = await exportCurrentSnapshot(page);
    expect(Object.keys(exportedSnapshot).sort()).toEqual([
      'activeTabId',
      'hasRunBefore',
      'tabs',
      'theme'
    ]);
    expect(exportedSnapshot).not.toHaveProperty('kind');

    const exportedTabs = JSON.parse(exportedSnapshot.tabs);
    expect(exportedTabs[0].edges).toHaveLength(1);
    expect(exportedTabs[0].edges[0]).toMatchObject({
      id: 'edge-1',
      fromItemId: 'item-1',
      toItemId: 'item-2',
      kind: 'arrow'
    });

    await prepareBlankPage(page);
    await page.goto('/index.html');
    await importBackupSnapshot(page, exportedSnapshot);

    await expectNoteVisible(page, 'Export Arrow Source');
    await expect(page.locator('.edge-line')).toHaveCount(1);
    const importedEdge = await readEdgeSnapshot(page, 'edge-1');
    expect(importedEdge).not.toBeNull();
    expect(importedEdge.kind).toBe('arrow');
    expect(await readEdgePresentation(page.locator('.edge-line').first())).toMatchObject({
      markerEnd: expect.stringMatching(/^url\(#.+-arrow\)$/)
    });
  });

  test('graph hook creates ordinary Postbaby items and edges from normalized graph data', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const graph = buildNormalizedGraph([
      buildGraphNode('A', { label: 'Start' }),
      buildGraphNode('B', { label: 'Decision', shape: 'unsupported-shape' }),
      buildGraphNode('C', { shape: 'circle' })
    ], [
      buildGraphEdge('A', 'B', { kind: 'arrow' }),
      buildGraphEdge('B', 'C', { kind: 'line' })
    ], {
      direction: 'LR'
    });

    const result = await createGraphForTest(page, graph);
    expect(result.ok).toBe(true);
    expect(result.items).toHaveLength(3);
    expect(result.edges).toHaveLength(2);
    expect(result.createdNodeIds).toEqual(['A', 'B', 'C']);
    expect(result.createdItemIds).toHaveLength(3);

    await expect(page.locator('.grid-item')).toHaveCount(3);
    await expect(page.locator('.edge-line')).toHaveCount(2);

    const tabsSnapshot = await readTabsSnapshot(page);
    expect(tabsSnapshot[0].items).toHaveLength(3);
    expect(tabsSnapshot[0].edges).toHaveLength(2);

    const startItem = tabsSnapshot[0].items.find((item) => item.name === 'Start');
    const decisionItem = tabsSnapshot[0].items.find((item) => item.name === 'Decision');
    const fallbackLabelItem = tabsSnapshot[0].items.find((item) => item.name === 'C');
    expect(startItem).toBeTruthy();
    expect(decisionItem).toBeTruthy();
    expect(fallbackLabelItem).toBeTruthy();

    [startItem, decisionItem, fallbackLabelItem].forEach((item) => {
      expect(item.id).toBeTruthy();
      expect(item.color).toBe(GRAPH_DEFAULT_COLOR);
      expect(item.position.top).toMatch(/px$/);
      expect(item.position.left).toMatch(/px$/);
      expect(item.width).toEqual(expect.any(Number));
      expect(item.height).toEqual(expect.any(Number));
      expect(item).not.toHaveProperty('sourceNodeId');
      expect(item).not.toHaveProperty('graphNodeId');
    });

    expect(parseInt(startItem.position.left, 10)).toBeGreaterThan(0);
    expect(parseInt(startItem.position.top, 10)).toBeGreaterThan(0);
    expect(decisionItem.shape).toBe('default');
    expect(decisionItem.width).toBe(220);
    expect(decisionItem.height).toBe(120);
    expect(fallbackLabelItem.shape).toBe('circle');

    tabsSnapshot[0].edges.forEach((edge) => {
      expect(edge.id).toBeTruthy();
      expect(edge.fromItemId).toBeTruthy();
      expect(edge.toItemId).toBeTruthy();
      expect(['line', 'arrow']).toContain(edge.kind);
      expect(edge).not.toHaveProperty('label');
      expect(edge).not.toHaveProperty('from');
      expect(edge).not.toHaveProperty('to');
    });
  });

  [
    {
      name: 'duplicate node IDs',
      graph: buildNormalizedGraph([
        buildGraphNode('A'),
        buildGraphNode('A')
      ]),
      expectedCode: 'duplicate_node_id'
    },
    {
      name: 'missing-node edge endpoints',
      graph: buildNormalizedGraph([
        buildGraphNode('A')
      ], [
        buildGraphEdge('A', 'B', { kind: 'arrow' })
      ]),
      expectedCode: 'missing_edge_to_node'
    },
    {
      name: 'self-edges',
      graph: buildNormalizedGraph([
        buildGraphNode('A')
      ], [
        buildGraphEdge('A', 'A', { kind: 'arrow' })
      ]),
      expectedCode: 'self_edge_not_allowed'
    },
    {
      name: 'duplicate logical edges',
      graph: buildNormalizedGraph([
        buildGraphNode('A'),
        buildGraphNode('B')
      ], [
        buildGraphEdge('A', 'B', { kind: 'arrow' }),
        buildGraphEdge('B', 'A', { kind: 'line' })
      ]),
      expectedCode: 'duplicate_edge_connection'
    },
    {
      name: 'graphs over the node limit',
      graph: buildNormalizedGraph(
        Array.from({ length: GRAPH_MAX_NODES + 1 }, (_, index) => buildGraphNode(`N${index + 1}`))
      ),
      expectedCode: 'graph_node_limit_exceeded'
    },
    {
      name: 'graphs over the edge limit',
      graph: (() => {
        const nodes = Array.from({ length: GRAPH_MAX_NODES }, (_, index) => buildGraphNode(`N${index + 1}`));
        const edges = [];
        for (let leftIndex = 0; leftIndex < nodes.length; leftIndex += 1) {
          for (let rightIndex = leftIndex + 1; rightIndex < nodes.length; rightIndex += 1) {
            edges.push(buildGraphEdge(nodes[leftIndex].id, nodes[rightIndex].id, { kind: 'arrow' }));
            if (edges.length > GRAPH_MAX_EDGES) {
              return {
                graph: buildNormalizedGraph(nodes, edges),
                expectedCode: 'graph_edge_limit_exceeded'
              };
            }
          }
        }
        throw new Error('Unable to build an edge-limit graph fixture.');
      })()
    },
    {
      name: 'labels over the max length',
      graph: buildNormalizedGraph([
        buildGraphNode('A', { label: 'x'.repeat(GRAPH_MAX_LABEL_CHARS + 1) })
      ]),
      expectedCode: 'node_label_too_long'
    }
  ].forEach((scenario) => {
    const graph = scenario.graph && scenario.graph.graph ? scenario.graph.graph : scenario.graph;
    const expectedCode = scenario.graph && scenario.graph.expectedCode ? scenario.graph.expectedCode : scenario.expectedCode;

    test(`graph hook rejects ${scenario.name} without creating anything`, async ({ page }) => {
      await prepareBlankPage(page);
      await seedLocalStorage(page, buildEmptySnapshot());
      await page.goto('/index.html');

      const result = await createGraphForTest(page, graph);
      expect(result.ok).toBe(false);
      expect(result.errors.map((error) => error.code)).toContain(expectedCode);
      await expect(page.locator('.grid-item')).toHaveCount(0);
      await expect(page.locator('.edge-line')).toHaveCount(0);

      const tabsSnapshot = await readTabsSnapshot(page);
      expect(tabsSnapshot[0].items).toHaveLength(0);
      expect(tabsSnapshot[0].edges).toHaveLength(0);
    });
  });

  test('graph hook uses deterministic left-to-right layered layout across repeated runs', async ({ page }) => {
    const graph = buildNormalizedGraph([
      buildGraphNode('A'),
      buildGraphNode('B'),
      buildGraphNode('C'),
      buildGraphNode('D'),
      buildGraphNode('E')
    ], [
      buildGraphEdge('A', 'B', { kind: 'arrow' }),
      buildGraphEdge('A', 'C', { kind: 'arrow' }),
      buildGraphEdge('B', 'D', { kind: 'arrow' }),
      buildGraphEdge('C', 'E', { kind: 'arrow' })
    ], {
      originX: 100,
      originY: 60,
      direction: 'LR'
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    await createGraphForTest(page, graph);
    const firstRunTabs = await readTabsSnapshot(page);
    const firstRunPositions = Object.fromEntries(firstRunTabs[0].items.map((item) => [item.name, item.position]));

    expect(parseInt(firstRunPositions.A.left, 10)).toBeLessThan(parseInt(firstRunPositions.B.left, 10));
    expect(parseInt(firstRunPositions.B.left, 10)).toBe(parseInt(firstRunPositions.C.left, 10));
    expect(parseInt(firstRunPositions.B.top, 10)).toBeLessThan(parseInt(firstRunPositions.C.top, 10));
    expect(parseInt(firstRunPositions.D.left, 10)).toBeGreaterThan(parseInt(firstRunPositions.B.left, 10));
    expect(parseInt(firstRunPositions.D.left, 10)).toBe(parseInt(firstRunPositions.E.left, 10));

    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    await createGraphForTest(page, graph);
    const secondRunTabs = await readTabsSnapshot(page);
    const secondRunPositions = Object.fromEntries(secondRunTabs[0].items.map((item) => [item.name, item.position]));

    expect(secondRunPositions).toEqual(firstRunPositions);
  });

  test('graph hook uses deterministic top-to-bottom layered layout for TD graphs', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const graph = buildNormalizedGraph([
      buildGraphNode('A'),
      buildGraphNode('B'),
      buildGraphNode('C')
    ], [
      buildGraphEdge('A', 'B', { kind: 'arrow' }),
      buildGraphEdge('B', 'C', { kind: 'arrow' })
    ], {
      originX: 120,
      originY: 90,
      direction: 'TD'
    });

    await createGraphForTest(page, graph);
    const tabsSnapshot = await readTabsSnapshot(page);
    const positions = Object.fromEntries(tabsSnapshot[0].items.map((item) => [item.name, item.position]));

    expect(parseInt(positions.A.top, 10)).toBeLessThan(parseInt(positions.B.top, 10));
    expect(parseInt(positions.B.top, 10)).toBeLessThan(parseInt(positions.C.top, 10));
    expect(parseInt(positions.A.left, 10)).toBe(parseInt(positions.B.left, 10));
    expect(parseInt(positions.B.left, 10)).toBe(parseInt(positions.C.left, 10));
  });

  test('graph hook respects explicit x/y positions relative to origin', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const graph = buildNormalizedGraph([
      buildGraphNode('A', { label: 'Explicit A', x: 0, y: 0 }),
      buildGraphNode('B', { label: 'Explicit B', x: 140, y: 60 })
    ], [
      buildGraphEdge('A', 'B', { kind: 'arrow' })
    ], {
      originX: 200,
      originY: 300,
      direction: 'LR'
    });

    await createGraphForTest(page, graph);
    const tabsSnapshot = await readTabsSnapshot(page);
    const positions = Object.fromEntries(tabsSnapshot[0].items.map((item) => [item.name, item.position]));

    expect(positions['Explicit A']).toEqual({ top: '300px', left: '200px' });
    expect(positions['Explicit B']).toEqual({ top: '360px', left: '340px' });
  });

  test('graph-created arrow edges use shape-aware anchors and line edges keep target-center behavior', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const graph = buildNormalizedGraph([
      buildGraphNode('S', { label: 'Source', x: 0, y: 0, width: 220, height: 120 }),
      buildGraphNode('C', { label: 'Circle Target', shape: 'circle', x: 360, y: 180 }),
      buildGraphNode('D', { label: 'Diamond Target', shape: 'diamond', x: 360, y: 0 })
    ], [
      buildGraphEdge('S', 'C', { kind: 'arrow' }),
      buildGraphEdge('S', 'D', { kind: 'line' })
    ], {
      originX: 80,
      originY: 80
    });

    const result = await createGraphForTest(page, graph);
    expect(result.ok).toBe(true);

    const sourceNote = page.locator(`.grid-item[data-id="${result.items[0].id}"]`);
    const circleNote = page.locator(`.grid-item[data-id="${result.items[1].id}"]`);
    const diamondNote = page.locator(`.grid-item[data-id="${result.items[2].id}"]`);
    const arrowEdge = page.locator(`.edge-group[data-edge-id="${result.edges[0].id}"] .edge-line`);
    const lineEdge = page.locator(`.edge-group[data-edge-id="${result.edges[1].id}"] .edge-line`);

    const sourceGeometry = await readNoteOffsetGeometry(sourceNote);
    const circleGeometry = await readNoteOffsetGeometry(circleNote);
    const diamondGeometry = await readNoteOffsetGeometry(diamondNote);
    const arrowCoordinates = await readEdgeCoordinates(arrowEdge);
    const lineCoordinates = await readEdgeCoordinates(lineEdge);
    const arrowPresentation = await readEdgePresentation(arrowEdge);
    const linePresentation = await readEdgePresentation(lineEdge);

    const expectedArrowEnd = getExpectedShapeAnchorPoint('circle', {
      left: circleGeometry.left,
      top: circleGeometry.top,
      width: circleGeometry.width,
      height: circleGeometry.height
    }, {
      x: sourceGeometry.centerX,
      y: sourceGeometry.centerY
    });

    expectPointNear(
      { x: arrowCoordinates.x2, y: arrowCoordinates.y2 },
      expectedArrowEnd
    );
    expect(arrowPresentation.markerEnd).toMatch(/^url\(#.+-arrow\)$/);
    expect(Math.abs(lineCoordinates.x2 - diamondGeometry.centerX)).toBeLessThanOrEqual(0.5);
    expect(Math.abs(lineCoordinates.y2 - diamondGeometry.centerY)).toBeLessThanOrEqual(0.5);
    expect(linePresentation.markerEnd).toBe('');
  });

  test('graph-created notes and edges survive reload and manual note creation still works', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const graph = buildNormalizedGraph([
      buildGraphNode('A', { label: 'Reload Source' }),
      buildGraphNode('B', { label: 'Reload Target', shape: 'circle' })
    ], [
      buildGraphEdge('A', 'B', { kind: 'arrow' })
    ], {
      originX: 160,
      originY: 120
    });

    await createGraphForTest(page, graph);
    await page.reload();

    await expectNoteVisible(page, 'Reload Source');
    await expectNoteVisible(page, 'Reload Target');
    await expect(page.locator('.edge-line')).toHaveCount(1);

    await page.keyboard.press('Escape');
    await page.keyboard.press('n');
    const textarea = page.locator('textarea.edit-textarea');
    await expect(textarea).toBeVisible();
    await textarea.fill('Manual Note After Graph');
    await textarea.press('Escape');

    await expectNoteVisible(page, 'Manual Note After Graph');
    const tabsSnapshot = await readTabsSnapshot(page);
    expect(tabsSnapshot[0].items.map((item) => item.name)).toEqual(
      expect.arrayContaining(['Reload Source', 'Reload Target', 'Manual Note After Graph'])
    );
    expect(tabsSnapshot[0].edges).toHaveLength(1);
  });

  test('manual edge creation still works after graph creation', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const graph = buildNormalizedGraph([
      buildGraphNode('A', { label: 'Manual Edge Source', x: 0, y: 0 }),
      buildGraphNode('B', { label: 'Manual Edge Target', x: 280, y: 60 })
    ], [], {
      originX: 160,
      originY: 140
    });

    const result = await createGraphForTest(page, graph);
    expect(result.ok).toBe(true);
    await page.keyboard.press('Escape');

    const sourceNote = page.locator(`.grid-item[data-id="${result.items[0].id}"]`);
    const targetNote = page.locator(`.grid-item[data-id="${result.items[1].id}"]`);

    await drawEdgeBetweenNotes(page, sourceNote, targetNote, { modifier: 'Control' });

    const tabsSnapshot = await readTabsSnapshot(page);
    expect(tabsSnapshot[0].edges).toHaveLength(1);
    expect(tabsSnapshot[0].edges[0]).toMatchObject({
      fromItemId: result.items[0].id,
      toItemId: result.items[1].id,
      kind: 'arrow'
    });
    expect(await readEdgePresentation(page.locator('.edge-line').first())).toMatchObject({
      markerEnd: expect.stringMatching(/^url\(#.+-arrow\)$/)
    });
  });

  [
    {
      name: 'flowchart LR parses to direction LR',
      source: 'flowchart LR\nA --> B',
      expectedDirection: 'LR',
      expectedKind: 'arrow'
    },
    {
      name: 'flowchart TD parses to direction TD',
      source: 'flowchart TD\nA --> B',
      expectedDirection: 'TD',
      expectedKind: 'arrow'
    },
    {
      name: 'graph TB parses to direction TD',
      source: 'graph TB\nA --- B',
      expectedDirection: 'TD',
      expectedKind: 'line'
    }
  ].forEach((scenario) => {
    test(`mermaid parser ${scenario.name}`, async ({ page }) => {
      await prepareBlankPage(page);
      await seedLocalStorage(page, buildEmptySnapshot());
      await page.goto('/index.html');

      const result = await parseMermaidForTest(page, scenario.source);
      expect(result.ok).toBe(true);
      expect(result.graph.options.direction).toBe(scenario.expectedDirection);
      expect(result.graph.edges).toHaveLength(1);
      expect(result.graph.edges[0].kind).toBe(scenario.expectedKind);
      expect(result.warnings).toEqual([]);
    });
  });

  test('mermaid parser maps supported node forms to normalized labels and shapes', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const source = [
      'flowchart LR',
      'A',
      'B[Start]',
      'C["Start here"]',
      'D[\'Single quoted\']',
      'E(Fallback)',
      'F((Round))',
      'G{Decision}'
    ].join('\n');

    const result = await parseMermaidForTest(page, source);
    expect(result.ok).toBe(true);

    const nodesById = Object.fromEntries(result.graph.nodes.map((node) => [node.id, node]));
    expect(nodesById.A).toMatchObject({ label: 'A', shape: 'default' });
    expect(nodesById.B).toMatchObject({ label: 'Start', shape: 'default' });
    expect(nodesById.C).toMatchObject({ label: 'Start here', shape: 'default' });
    expect(nodesById.D).toMatchObject({ label: 'Single quoted', shape: 'default' });
    expect(nodesById.E).toMatchObject({ label: 'Fallback', shape: 'default' });
    expect(nodesById.F).toMatchObject({ label: 'Round', shape: 'circle' });
    expect(nodesById.G).toMatchObject({ label: 'Decision', shape: 'diamond' });
  });

  test('mermaid parser preserves simple edge labels in normalized output', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const source = [
      'flowchart LR',
      'A -->|Yes| B',
      'B -- No --> C',
      'C ==> D',
      'D -.-> E'
    ].join('\n');

    const result = await parseMermaidForTest(page, source);
    expect(result.ok).toBe(true);
    expect(result.graph.edges).toEqual([
      { from: 'A', to: 'B', kind: 'arrow', label: 'Yes' },
      { from: 'B', to: 'C', kind: 'arrow', label: 'No' },
      { from: 'C', to: 'D', kind: 'arrow' },
      { from: 'D', to: 'E', kind: 'arrow' }
    ]);
  });

  test('mermaid parser updates later labels for bare nodes and warns on conflicts', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const source = [
      'flowchart LR',
      'A --> B',
      'A[Start]',
      'B["Keep Me"]',
      'B[Conflict]'
    ].join('\n');

    const result = await parseMermaidForTest(page, source);
    expect(result.ok).toBe(true);

    const nodesById = Object.fromEntries(result.graph.nodes.map((node) => [node.id, node]));
    expect(nodesById.A).toMatchObject({ label: 'Start', shape: 'default' });
    expect(nodesById.B).toMatchObject({ label: 'Keep Me', shape: 'default' });
    expect(result.warnings.map((warning) => warning.code)).toContain('conflicting_node_label');
  });

  test('mermaid parser ignores comments and warns for class/style noise', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const source = [
      'flowchart LR',
      '%% comment',
      '',
      'subgraph Phase One',
      'A[Start] --> B{Decision}',
      'end',
      'classDef important fill:#f9f,stroke:#333;',
      'class A important',
      'style B fill:#ccc',
      'linkStyle 0 stroke:#333',
      'click A "https://example.com"'
    ].join('\n');

    const result = await parseMermaidForTest(page, source);
    expect(result.ok).toBe(true);
    expect(result.graph.nodes).toEqual([
      { id: 'A', label: 'Start', shape: 'default' },
      { id: 'B', label: 'Decision', shape: 'diamond' }
    ]);
    expect(result.graph.edges).toEqual([
      { from: 'A', to: 'B', kind: 'arrow' }
    ]);
    expect(result.warnings.filter((warning) => warning.code === 'ignored_mermaid_statement')).toHaveLength(7);
  });

  test('mermaid parser rejects unsupported diagram types cleanly', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const result = await parseMermaidForTest(page, [
      'sequenceDiagram',
      'Alice->>Bob: Hello'
    ].join('\n'));

    expect(result.ok).toBe(false);
    expect(result.errors.map((error) => error.code)).toContain('unsupported_diagram_type');
  });

  test('mermaid parse output can be passed into graph creation and duplicate logical edges are rejected consistently', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const parseResult = await parseMermaidForTest(page, [
      'flowchart LR',
      'A --> B',
      'B --- A'
    ].join('\n'));

    expect(parseResult.ok).toBe(true);
    expect(parseResult.graph.edges).toHaveLength(2);

    const createResult = await createGraphForTest(page, parseResult.graph);
    expect(createResult.ok).toBe(false);
    expect(createResult.errors.map((error) => error.code)).toContain('duplicate_edge_connection');
    await expect(page.locator('.grid-item')).toHaveCount(0);
    await expect(page.locator('.edge-line')).toHaveCount(0);
  });

  test('mermaid-derived graphs create ordinary Postbaby items and edges through the existing pipeline', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const parseResult = await parseMermaidForTest(page, [
      'flowchart LR',
      'A[Source] --> B((Circle Target))',
      'A --- C{Decision}'
    ].join('\n'), {
      originX: 120,
      originY: 140
    });

    expect(parseResult.ok).toBe(true);

    const createResult = await createGraphForTest(page, parseResult.graph);
    expect(createResult.ok).toBe(true);
    expect(createResult.items).toHaveLength(3);
    expect(createResult.edges).toHaveLength(2);

    const tabsSnapshot = await readTabsSnapshot(page);
    expect(tabsSnapshot[0].items).toHaveLength(3);
    expect(tabsSnapshot[0].edges).toHaveLength(2);

    const sourceItem = tabsSnapshot[0].items.find((item) => item.name === 'Source');
    const circleItem = tabsSnapshot[0].items.find((item) => item.name === 'Circle Target');
    const diamondItem = tabsSnapshot[0].items.find((item) => item.name === 'Decision');
    expect(sourceItem).toBeTruthy();
    expect(circleItem).toBeTruthy();
    expect(diamondItem).toBeTruthy();
    expect(sourceItem.shape).toBe('default');
    expect(circleItem.shape).toBe('circle');
    expect(diamondItem.shape).toBe('diamond');

    const arrowEdge = tabsSnapshot[0].edges.find((edge) => edge.kind === 'arrow');
    const lineEdge = tabsSnapshot[0].edges.find((edge) => edge.kind === 'line');
    expect(arrowEdge).toBeTruthy();
    expect(lineEdge).toBeTruthy();

    await expect(page.locator('.grid-item')).toHaveCount(3);
    await expect(page.locator('.edge-line')).toHaveCount(2);
  });

  test('mermaid-derived graph edges keep current arrow and line rendering behavior and survive reload', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const createResult = await createGraphFromMermaidForTest(page, [
      'flowchart LR',
      'A[Source] --> B((Circle Target))',
      'A --- C{Decision}'
    ].join('\n'), {
      originX: 180,
      originY: 120
    });

    expect(createResult.ok).toBe(true);
    expect(createResult.graph.options.direction).toBe('LR');

    const sourceNote = page.locator(`.grid-item[data-id="${createResult.items[0].id}"]`);
    const circleNote = page.locator(`.grid-item[data-id="${createResult.items[1].id}"]`);
    const diamondNote = page.locator(`.grid-item[data-id="${createResult.items[2].id}"]`);
    const arrowEdgeId = createResult.edges.find((edge) => edge.kind === 'arrow').id;
    const lineEdgeId = createResult.edges.find((edge) => edge.kind === 'line').id;
    const arrowEdge = page.locator(`.edge-group[data-edge-id="${arrowEdgeId}"] .edge-line`);
    const lineEdge = page.locator(`.edge-group[data-edge-id="${lineEdgeId}"] .edge-line`);

    const sourceGeometry = await readNoteOffsetGeometry(sourceNote);
    const circleGeometry = await readNoteOffsetGeometry(circleNote);
    const diamondGeometry = await readNoteOffsetGeometry(diamondNote);
    const arrowCoordinates = await readEdgeCoordinates(arrowEdge);
    const lineCoordinates = await readEdgeCoordinates(lineEdge);
    const arrowPresentation = await readEdgePresentation(arrowEdge);
    const linePresentation = await readEdgePresentation(lineEdge);

    const expectedArrowEnd = getExpectedShapeAnchorPoint('circle', {
      left: circleGeometry.left,
      top: circleGeometry.top,
      width: circleGeometry.width,
      height: circleGeometry.height
    }, {
      x: sourceGeometry.centerX,
      y: sourceGeometry.centerY
    });

    expectPointNear(
      { x: arrowCoordinates.x2, y: arrowCoordinates.y2 },
      expectedArrowEnd
    );
    expect(Math.abs(lineCoordinates.x2 - diamondGeometry.centerX)).toBeLessThanOrEqual(0.5);
    expect(Math.abs(lineCoordinates.y2 - diamondGeometry.centerY)).toBeLessThanOrEqual(0.5);
    expect(arrowPresentation.markerEnd).toMatch(/^url\(#.+-arrow\)$/);
    expect(linePresentation.markerEnd).toBe('');

    await page.reload();

    await expectNoteVisible(page, 'Source');
    await expectNoteVisible(page, 'Circle Target');
    await expectNoteVisible(page, 'Decision');
    await expect(page.locator('.edge-line')).toHaveCount(2);
  });

  test('dragging on empty canvas selects overlapping notes on mouseup and replaces prior selection', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Select One', {
        itemId: 'item-1',
        position: { top: '80px', left: '80px' }
      }),
      buildNoteItem('Select Two', {
        itemId: 'item-2',
        position: { top: '110px', left: '220px' }
      }),
      buildNoteItem('Select Three', {
        itemId: 'item-3',
        position: { top: '70px', left: '350px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const noteOne = page.locator('.grid-item[data-id="item-1"]');
    const noteTwo = page.locator('.grid-item[data-id="item-2"]');
    const noteThree = page.locator('.grid-item[data-id="item-3"]');

    await marqueeSelectNotes(page, [noteOne, noteTwo], { margin: 8 });
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1', 'item-2']);

    await marqueeSelectNotes(page, [noteThree], { margin: 6 });
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-3']);
    await expect(page.locator('.grid-selection-marquee')).toHaveCount(0);
  });

  test('marquee drag suppresses native text selection during and after selection', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Selected By Marquee', {
        itemId: 'item-1',
        position: { top: '80px', left: '120px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await note.scrollIntoViewIfNeeded();
    const box = await note.boundingBox();
    if (!box) {
      throw new Error('Note bounding box was not available.');
    }
    const viewport = page.viewportSize();
    if (!viewport) {
      throw new Error('Viewport size was not available.');
    }

    const startX = Math.max(6, box.x - 18);
    const startY = Math.max(6, box.y - 18);
    const endX = Math.min(viewport.width - 6, box.x + box.width + 18);
    const endY = Math.min(viewport.height - 6, box.y + box.height + 18);

    await page.evaluate(({ startX, startY, endX, endY }) => {
      document.body.dispatchEvent(new MouseEvent('mousedown', {
        bubbles: true,
        cancelable: true,
        button: 0,
        buttons: 1,
        clientX: startX,
        clientY: startY
      }));

      document.dispatchEvent(new MouseEvent('mousemove', {
        bubbles: true,
        cancelable: true,
        button: 0,
        buttons: 1,
        clientX: endX,
        clientY: endY
      }));
    }, { startX, startY, endX, endY });

    await expect.poll(() => page.evaluate(() => document.body.style.userSelect)).toBe('none');

    await page.evaluate(({ endX, endY }) => {
      document.dispatchEvent(new MouseEvent('mouseup', {
        bubbles: true,
        cancelable: true,
        button: 0,
        buttons: 0,
        clientX: endX,
        clientY: endY
      }));
    }, { endX, endY });

    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1']);
    await expect.poll(() => page.evaluate(() => document.body.style.userSelect)).not.toBe('none');
    await expect.poll(() => readBrowserSelectionText(page)).toBe('');
  });

  test('empty canvas click clears marquee selection without selecting notes', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Selected Then Cleared', {
        itemId: 'item-1',
        position: { top: '80px', left: '120px' }
      }),
      buildNoteItem('Other Note', {
        itemId: 'item-2',
        position: { top: '120px', left: '360px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await marqueeSelectNotes(page, [note]);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1']);

    await clickEmptyGrid(page);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual([]);
  });

  test('empty canvas click over the logo area clears marquee selection', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Selected Then Cleared', {
        itemId: 'item-1',
        position: { top: '80px', left: '120px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await marqueeSelectNotes(page, [note]);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1']);

    await page.evaluate(() => {
      const logoContainer = document.querySelector('.logo-container');
      if (!(logoContainer instanceof HTMLElement)) {
        throw new Error('Logo container was not available.');
      }

      const rect = logoContainer.getBoundingClientRect();
      const x = Math.round(rect.left + (rect.width / 2));
      const y = Math.round(rect.top + Math.min(rect.height / 2, 40));

      logoContainer.dispatchEvent(new MouseEvent('mousedown', {
        bubbles: true,
        cancelable: true,
        button: 0,
        buttons: 1,
        clientX: x,
        clientY: y
      }));
      document.dispatchEvent(new MouseEvent('mouseup', {
        bubbles: true,
        cancelable: true,
        button: 0,
        buttons: 0,
        clientX: x,
        clientY: y
      }));
    });

    await expect.poll(() => readSelectedNoteIds(page)).toEqual([]);
  });

  test('marquee drag can start over the logo area and still selects overlapping notes', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Select Me', {
        itemId: 'item-1',
        position: { top: '80px', left: '120px' }
      }),
      buildNoteItem('Also Select Me', {
        itemId: 'item-2',
        position: { top: '110px', left: '280px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const notes = [
      page.locator('.grid-item[data-id="item-1"]'),
      page.locator('.grid-item[data-id="item-2"]')
    ];
    const boxes = [];
    for (const note of notes) {
      await note.scrollIntoViewIfNeeded();
      const box = await note.boundingBox();
      if (!box) {
        throw new Error('Note bounding box was not available.');
      }
      boxes.push(box);
    }

    await page.evaluate(({ noteBoxes }) => {
      const logoContainer = document.querySelector('.logo-container');
      if (!(logoContainer instanceof HTMLElement)) {
        throw new Error('Logo container was not available.');
      }

      const logoRect = logoContainer.getBoundingClientRect();
      const startX = Math.round(logoRect.left + (logoRect.width / 2));
      const startY = Math.round(logoRect.top + Math.min(logoRect.height / 2, 40));
      const left = Math.min(...noteBoxes.map((box) => box.x)) - 12;
      const top = Math.min(...noteBoxes.map((box) => box.y)) - 12;
      const right = Math.max(...noteBoxes.map((box) => box.x + box.width)) + 12;
      const bottom = Math.max(...noteBoxes.map((box) => box.y + box.height)) + 12;
      const endX = Math.round(startX <= left ? right : left);
      const endY = Math.round(startY <= top ? bottom : top);

      logoContainer.dispatchEvent(new MouseEvent('mousedown', {
        bubbles: true,
        cancelable: true,
        button: 0,
        buttons: 1,
        clientX: startX,
        clientY: startY
      }));
      document.dispatchEvent(new MouseEvent('mousemove', {
        bubbles: true,
        cancelable: true,
        button: 0,
        buttons: 1,
        clientX: endX,
        clientY: endY
      }));
      document.dispatchEvent(new MouseEvent('mouseup', {
        bubbles: true,
        cancelable: true,
        button: 0,
        buttons: 0,
        clientX: endX,
        clientY: endY
      }));
    }, { noteBoxes: boxes });

    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1', 'item-2']);
  });

  test('marquee-selected group drag moves the whole group and keeps selection after drop', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Drag One', {
        itemId: 'item-1',
        position: { top: '80px', left: '100px' }
      }),
      buildNoteItem('Drag Two', {
        itemId: 'item-2',
        position: { top: '100px', left: '280px' }
      }),
      buildNoteItem('Stay Put', {
        itemId: 'item-3',
        position: { top: '260px', left: '520px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const noteOne = page.locator('.grid-item[data-id="item-1"]');
    const noteTwo = page.locator('.grid-item[data-id="item-2"]');
    await marqueeSelectNotes(page, [noteOne, noteTwo]);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1', 'item-2']);

    const beforeOne = await readItemSnapshot(page, 'item-1');
    const beforeTwo = await readItemSnapshot(page, 'item-2');
    const beforeThree = await readItemSnapshot(page, 'item-3');

    await dragNoteBy(page, noteOne, 120, 70);
    await page.waitForTimeout(350);

    const afterOne = await readItemSnapshot(page, 'item-1');
    const afterTwo = await readItemSnapshot(page, 'item-2');
    const afterThree = await readItemSnapshot(page, 'item-3');
    expect(afterOne.position).not.toEqual(beforeOne.position);
    expect(afterTwo.position).not.toEqual(beforeTwo.position);
    expect(afterThree.position).toEqual(beforeThree.position);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1', 'item-2']);
  });

  test('Delete removes the current marquee selection as a batch', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Delete One', {
        itemId: 'item-1',
        position: { top: '80px', left: '100px' }
      }),
      buildNoteItem('Delete Two', {
        itemId: 'item-2',
        position: { top: '120px', left: '300px' }
      }),
      buildNoteItem('Keep Me', {
        itemId: 'item-3',
        position: { top: '300px', left: '520px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const noteOne = page.locator('.grid-item[data-id="item-1"]');
    const noteTwo = page.locator('.grid-item[data-id="item-2"]');
    await marqueeSelectNotes(page, [noteOne, noteTwo]);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1', 'item-2']);

    await page.keyboard.press('Delete');
    await expect(page.locator('#confirmModal')).toBeVisible();
    await page.click('#confirmDelete');

    await expect(noteOne).toHaveCount(0);
    await expect(noteTwo).toHaveCount(0);
    await expectNoteVisible(page, 'Keep Me');
  });

  test('Ctrl+right-click shape cycles the current marquee selection as a group', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Shape One', {
        itemId: 'item-1',
        position: { top: '80px', left: '100px' }
      }),
      buildNoteItem('Shape Two', {
        itemId: 'item-2',
        position: { top: '120px', left: '300px' }
      }),
      buildNoteItem('Shape Three', {
        itemId: 'item-3',
        position: { top: '300px', left: '520px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const noteOne = page.locator('.grid-item[data-id="item-1"]');
    const noteTwo = page.locator('.grid-item[data-id="item-2"]');
    const noteThree = page.locator('.grid-item[data-id="item-3"]');
    await marqueeSelectNotes(page, [noteOne, noteTwo]);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1', 'item-2']);

    await cycleNoteShape(noteOne);
    await expect(noteOne).toHaveAttribute('data-shape', 'circle');
    await expect(noteTwo).toHaveAttribute('data-shape', 'circle');
    await expect(noteThree).toHaveAttribute('data-shape', 'default');
  });

  test('right-click on a selected marquee group deletes that group', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Group Delete One', {
        itemId: 'item-1',
        position: { top: '80px', left: '100px' }
      }),
      buildNoteItem('Group Delete Two', {
        itemId: 'item-2',
        position: { top: '120px', left: '300px' }
      }),
      buildNoteItem('Survivor', {
        itemId: 'item-3',
        position: { top: '300px', left: '520px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const noteOne = page.locator('.grid-item[data-id="item-1"]');
    const noteTwo = page.locator('.grid-item[data-id="item-2"]');
    await marqueeSelectNotes(page, [noteOne, noteTwo]);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1', 'item-2']);

    await noteOne.click({ button: 'right' });
    await expect(page.locator('#confirmModal')).toBeVisible();
    await page.click('#confirmDelete');

    await expect(noteOne).toHaveCount(0);
    await expect(noteTwo).toHaveCount(0);
    await expectNoteVisible(page, 'Survivor');
  });

  test('clicking an unselected note clears the marquee selection and still runs normal color cycling', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Selected Note', {
        itemId: 'item-1',
        position: { top: '80px', left: '100px' }
      }),
      buildNoteItem('Clicked Note', {
        itemId: 'item-2',
        position: { top: '120px', left: '340px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const selectedNote = page.locator('.grid-item[data-id="item-1"]');
    const clickedNote = page.locator('.grid-item[data-id="item-2"]');
    const selectedColorBefore = await readItemColor(page, 'item-1');
    const clickedColorBefore = await readItemColor(page, 'item-2');

    await marqueeSelectNotes(page, [selectedNote]);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1']);

    await clickedNote.click();
    await page.waitForTimeout(350);

    await expect.poll(() => readSelectedNoteIds(page)).toEqual([]);
    expect(await readItemColor(page, 'item-1')).toBe(selectedColorBefore);
    expect(await readItemColor(page, 'item-2')).not.toBe(clickedColorBefore);
  });

  test('clicking a selected note keeps the marquee selection and still runs normal color cycling', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Keep Selection On Click', {
      position: { top: '100px', left: '160px' }
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const colorBefore = await readItemColor(page);

    await marqueeSelectNotes(page, [note]);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1']);

    await note.click();
    await page.waitForTimeout(350);

    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1']);
    expect(await readItemColor(page)).not.toBe(colorBefore);
  });

  test('Shift+drag edge creation still works and does not start marquee selection', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Edge Source', {
        itemId: 'item-1',
        position: { top: '100px', left: '140px' }
      }),
      buildNoteItem('Edge Target', {
        itemId: 'item-2',
        position: { top: '140px', left: '420px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const sourceNote = page.locator('.grid-item[data-id="item-1"]');
    const targetNote = page.locator('.grid-item[data-id="item-2"]');

    await drawEdgeBetweenNotes(page, sourceNote, targetNote);

    await expect(page.locator('.edge-line')).toHaveCount(1);
    const createdEdge = (await readTabsSnapshot(page))[0].edges[0];
    expect(createdEdge).toMatchObject({
      fromItemId: 'item-1',
      toItemId: 'item-2',
      kind: 'line'
    });
    expect((await readEdgePresentation(page.locator('.edge-line').first())).markerEnd).toBe('');
    await expect.poll(() => readSelectedNoteIds(page)).toEqual([]);
    await expect(page.locator('.grid-selection-marquee')).toHaveCount(0);
  });

  test('Escape clears the current marquee selection', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Escape One', {
        itemId: 'item-1',
        position: { top: '80px', left: '100px' }
      }),
      buildNoteItem('Escape Two', {
        itemId: 'item-2',
        position: { top: '120px', left: '300px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const noteOne = page.locator('.grid-item[data-id="item-1"]');
    const noteTwo = page.locator('.grid-item[data-id="item-2"]');
    await marqueeSelectNotes(page, [noteOne, noteTwo]);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1', 'item-2']);

    await page.keyboard.press('Escape');
    await expect.poll(() => readSelectedNoteIds(page)).toEqual([]);
  });

  test('switching tabs clears the current marquee selection', async ({ page }) => {
    const localSnapshot = {
      tabs: JSON.stringify([
        {
          id: 'tab-1',
          name: '1',
          items: [
            buildNoteItem('Tab One Note', {
              itemId: 'item-1',
              position: { top: '100px', left: '160px' }
            })
          ],
          colorIndex: 0,
          gridSetting: 'none',
          edges: []
        },
        {
          id: 'tab-2',
          name: '2',
          items: [
            buildNoteItem('Tab Two Note', {
              itemId: 'item-2',
              position: { top: '100px', left: '160px' }
            })
          ],
          colorIndex: 0,
          gridSetting: 'none',
          edges: []
        }
      ]),
      activeTabId: 'tab-1',
      hasRunBefore: 'true',
      theme: 'light'
    };

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await marqueeSelectNotes(page, [note]);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1']);

    await page.locator('.tab[data-tab-id="tab-2"]').click();
    await expectNoteVisible(page, 'Tab Two Note');
    await page.locator('.tab[data-tab-id="tab-1"]').click();
    await expectNoteVisible(page, 'Tab One Note');
    await expect(note).not.toHaveClass(/selected/);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual([]);
  });

  test('Control-click no longer toggles selection and still behaves like a normal note click', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Control Click No Selection', {
      position: { top: '100px', left: '160px' }
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const colorBefore = await readItemColor(page);

    await note.click({ modifiers: ['Control'] });
    await page.waitForTimeout(350);

    await expect.poll(() => readSelectedNoteIds(page)).toEqual([]);
    expect(await readItemColor(page)).not.toBe(colorBefore);
  });

  test('window blur cancels an active drag and later drags still work', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Blur Drag', {
      position: { top: '120px', left: '160px' }
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const before = await readItemSnapshot(page);

    await beginDragGesture(page, note, 120, 70);
    await page.evaluate(() => window.dispatchEvent(new Event('blur')));
    await page.mouse.up();

    await expect.poll(async () => {
      const item = await readItemSnapshot(page);
      return item ? item.position : null;
    }).toEqual(before.position);

    await dragNoteBy(page, note, 90, 40);
    await page.waitForTimeout(350);

    const after = await readItemSnapshot(page);
    expect(after.position).not.toEqual(before.position);
  });

  test('window blur cancels an active resize and keeps later resize interactions usable', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Blur Resize', {
      width: 240,
      height: 170,
      position: { top: '120px', left: '160px' }
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const before = await readItemSnapshot(page);

    await beginResizeGesture(page, note, 100, 70);
    await page.evaluate(() => window.dispatchEvent(new Event('blur')));
    await page.mouse.up();

    await expect.poll(async () => {
      const item = await readItemSnapshot(page);
      return item ? { width: item.width, height: item.height } : null;
    }).toEqual({ width: before.width, height: before.height });

    await resizeNoteBy(page, note, 60, 40);
    await page.waitForTimeout(350);

    const after = await readItemSnapshot(page);
    expect(after.width).not.toBe(before.width);
    expect(after.height).not.toBe(before.height);
  });

  test('Escape cancels an active edge draw and leaves the next edge draw working', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Edge Escape Source', {
        itemId: 'item-1',
        position: { top: '100px', left: '140px' }
      }),
      buildNoteItem('Edge Escape Target', {
        itemId: 'item-2',
        position: { top: '150px', left: '430px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const sourceNote = page.locator('.grid-item[data-id="item-1"]');
    const targetNote = page.locator('.grid-item[data-id="item-2"]');
    await sourceNote.scrollIntoViewIfNeeded();
    const sourceBox = await sourceNote.boundingBox();
    if (!sourceBox) {
      throw new Error('Source note bounding box was not available.');
    }
    const startX = sourceBox.x + (sourceBox.width / 2);
    const startY = sourceBox.y + (sourceBox.height / 2);

    await page.keyboard.down('Shift');
    await page.mouse.move(startX, startY);
    await page.mouse.down();
    await page.mouse.move(startX + 60, startY + 40, { steps: 8 });
    await expect(page.locator('.edge-preview')).toHaveCount(1);

    await page.keyboard.press('Escape');
    await page.mouse.up();
    await page.keyboard.up('Shift');

    await expect(page.locator('.edge-preview')).toHaveCount(0);
    await expect(page.locator('.edge-line')).toHaveCount(0);

    await drawEdgeBetweenNotes(page, sourceNote, targetNote);
    await expect(page.locator('.edge-line')).toHaveCount(1);
  });

  test('switching tabs cancels an active edge draw without leaving a stuck preview', async ({ page }) => {
    const localSnapshot = {
      tabs: JSON.stringify([
        {
          id: 'tab-1',
          name: '1',
          items: [
            buildNoteItem('Tab One Source', {
              itemId: 'item-1',
              position: { top: '100px', left: '140px' }
            }),
            buildNoteItem('Tab One Target', {
              itemId: 'item-2',
              position: { top: '150px', left: '430px' }
            })
          ],
          colorIndex: 0,
          gridSetting: 'none',
          edges: []
        },
        {
          id: 'tab-2',
          name: '2',
          items: [
            buildNoteItem('Tab Two Note', {
              itemId: 'item-3',
              position: { top: '120px', left: '200px' }
            })
          ],
          colorIndex: 0,
          gridSetting: 'none',
          edges: []
        }
      ]),
      activeTabId: 'tab-1',
      hasRunBefore: 'true',
      theme: 'light'
    };

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const sourceNote = page.locator('.grid-item[data-id="item-1"]');
    const targetNote = page.locator('.grid-item[data-id="item-2"]');
    await sourceNote.scrollIntoViewIfNeeded();
    const sourceBox = await sourceNote.boundingBox();
    if (!sourceBox) {
      throw new Error('Source note bounding box was not available.');
    }
    const startX = sourceBox.x + (sourceBox.width / 2);
    const startY = sourceBox.y + (sourceBox.height / 2);

    await page.keyboard.down('Shift');
    await page.mouse.move(startX, startY);
    await page.mouse.down();
    await page.mouse.move(startX + 60, startY + 40, { steps: 8 });
    await expect(page.locator('.edge-preview')).toHaveCount(1);

    await page.evaluate(() => {
      const nextTab = document.querySelector('.tab[data-tab-id="tab-2"]');
      if (!(nextTab instanceof HTMLElement)) {
        throw new Error('Target tab was not available.');
      }

      nextTab.dispatchEvent(new MouseEvent('click', {
        bubbles: true,
        cancelable: true,
        button: 0,
        buttons: 0
      }));
    });
    await page.mouse.up();
    await page.keyboard.up('Shift');

    await expect(page.locator('.tab-pane.active')).toHaveAttribute('data-tab-id', 'tab-2');
    await expect(page.locator('.edge-preview')).toHaveCount(0);
    await expect(page.locator('.edge-line')).toHaveCount(0);

    await page.locator('.tab[data-tab-id="tab-1"]').click();
    await drawEdgeBetweenNotes(page, sourceNote, targetNote);
    await expect(page.locator('.edge-line')).toHaveCount(1);
  });

  test('window blur cancels an active marquee and restores the canvas for later selection', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Blur Marquee', {
        itemId: 'item-1',
        position: { top: '80px', left: '120px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');

    await beginMarqueeGesture(page, [note], { margin: 12 });
    await expect(page.locator('.grid-selection-marquee')).toHaveCount(1);
    await expect.poll(() => page.evaluate(() => document.body.style.userSelect)).toBe('none');

    await page.evaluate(() => window.dispatchEvent(new Event('blur')));

    await expect(page.locator('.grid-selection-marquee')).toHaveCount(0);
    await expect.poll(() => page.evaluate(() => document.body.style.userSelect)).not.toBe('none');
    await expect.poll(() => readSelectedNoteIds(page)).toEqual([]);

    await marqueeSelectNotes(page, [note]);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1']);
  });

  test('import while a marquee selection exists reloads cleanly and leaves later note interactions working', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Selected Before Import', {
        itemId: 'item-1',
        position: { top: '90px', left: '120px' }
      }),
      buildNoteItem('Keep Before Import', {
        itemId: 'item-2',
        position: { top: '150px', left: '340px' }
      })
    ]);
    const importedSnapshot = buildLocalSnapshot('Imported After Selection', {
      itemId: 'item-9',
      position: { top: '120px', left: '200px' }
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const selectedNote = page.locator('.grid-item[data-id="item-1"]');
    await marqueeSelectNotes(page, [selectedNote]);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1']);

    await importBackupSnapshot(page, importedSnapshot, 'interaction-hardening-import.json');
    await expectNoteVisible(page, 'Imported After Selection');
    await expect.poll(() => readSelectedNoteIds(page)).toEqual([]);

    const importedNote = page.locator('.grid-item[data-id="item-9"]');
    const colorBefore = await readItemColor(page, 'item-9');
    await importedNote.click();
    await page.waitForTimeout(350);
    expect(await readItemColor(page, 'item-9')).not.toBe(colorBefore);
  });

  test('switching tabs while editing commits the hidden editor and does not block later interactions', async ({ page }) => {
    const localSnapshot = {
      tabs: JSON.stringify([
        {
          id: 'tab-1',
          name: '1',
          items: [
            buildNoteItem('Editable Tab One', {
              itemId: 'item-1',
              position: { top: '100px', left: '160px' }
            })
          ],
          colorIndex: 0,
          gridSetting: 'none',
          edges: []
        },
        {
          id: 'tab-2',
          name: '2',
          items: [
            buildNoteItem('Clickable Tab Two', {
              itemId: 'item-2',
              position: { top: '120px', left: '220px' }
            })
          ],
          colorIndex: 0,
          gridSetting: 'none',
          edges: []
        }
      ]),
      activeTabId: 'tab-1',
      hasRunBefore: 'true',
      theme: 'light'
    };

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const tabOneNote = page.locator('.grid-item[data-id="item-1"]');
    await tabOneNote.dblclick();
    const textarea = page.locator('textarea.edit-textarea');
    await expect(textarea).toBeVisible();
    await textarea.fill('Edited Tab One');

    await page.locator('.tab[data-tab-id="tab-2"]').click();
    await expect(page.locator('.tab-pane.active')).toHaveAttribute('data-tab-id', 'tab-2');
    await expect(page.locator('.tab-pane.active textarea.edit-textarea')).toHaveCount(0);

    const tabTwoNote = page.locator('.grid-item[data-id="item-2"]');
    const colorBefore = await readItemColor(page, 'item-2');
    await tabTwoNote.click();
    await page.waitForTimeout(350);
    expect(await readItemColor(page, 'item-2')).not.toBe(colorBefore);

    await page.locator('.tab[data-tab-id="tab-1"]').click();
    await expect(page.locator('.grid-item[data-id="item-1"] .grid-item-text')).toContainText('Edited Tab One');
  });

  test('Disable Color Change setting prevents desktop click recolor and persists across reload', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Disable Recolor');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const colorBefore = await readItemColor(page);

    await openSettingsModal(page);
    const disableToggle = page.locator('#toggleDisableColorChange');
    await expect(disableToggle).not.toBeChecked();
    await disableToggle.evaluate((element) => element.click());
    await expect(disableToggle).toBeChecked();
    await expect.poll(async () => {
      const indexedDBState = await readIndexedDBState(page);
      return indexedDBState.snapshot ? indexedDBState.snapshot.disableColorChange : null;
    }).toBe('true');
    await page.locator('.close-settings').click();
    await expect(page.locator('#settingsModal')).toBeHidden();

    await note.click();
    await page.waitForTimeout(350);
    expect(await readItemColor(page)).toBe(colorBefore);

    await page.reload();
    await expectNoteVisible(page, 'Disable Recolor');
    await note.click();
    await page.waitForTimeout(350);
    expect(await readItemColor(page)).toBe(colorBefore);

    await openSettingsModal(page);
    await expect(page.locator('#toggleDisableColorChange')).toBeChecked();
  });

  test('re-enabling Disable Color Change restores desktop click recolor', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Re-enable Recolor', {
      disableColorChange: true
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    const colorBefore = await readItemColor(page);

    await openSettingsModal(page);
    const disableToggle = page.locator('#toggleDisableColorChange');
    await expect(disableToggle).toBeChecked();
    await disableToggle.evaluate((element) => element.click());
    await expect(disableToggle).not.toBeChecked();
    await expect.poll(async () => {
      const indexedDBState = await readIndexedDBState(page);
      return indexedDBState.snapshot ? indexedDBState.snapshot.disableColorChange : null;
    }).toBe('false');
    await page.locator('.close-settings').click();
    await expect(page.locator('#settingsModal')).toBeHidden();

    await note.click();
    await page.waitForTimeout(350);
    expect(await readItemColor(page)).not.toBe(colorBefore);
  });

  test('Disable Note Resize hides resize handles, persists across reload, and restores resizing when re-enabled', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Disable Resize', {
      width: 220,
      height: 140
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();
    const sizeBeforeDisable = await readNoteSize(note);

    await openSettingsModal(page);
    const disableResizeToggle = page.locator('#toggleDisableNoteResize');
    await expect(disableResizeToggle).not.toBeChecked();
    await disableResizeToggle.evaluate((element) => element.click());
    await expect(disableResizeToggle).toBeChecked();
    await expect.poll(async () => {
      const indexedDBState = await readIndexedDBState(page);
      return indexedDBState.snapshot ? indexedDBState.snapshot.disableNoteResize : null;
    }).toBe('true');
    await page.locator('.close-settings').click();
    await expect(page.locator('#settingsModal')).toBeHidden();

    await expect(note.locator('.grid-item-resize-handle')).toHaveCount(0);
    expect(await readNoteSize(note)).toEqual(sizeBeforeDisable);

    await page.reload();
    await expectNoteVisible(page, 'Disable Resize');
    await expect(note.locator('.grid-item-resize-handle')).toHaveCount(0);
    expect(await readNoteSize(note)).toEqual(sizeBeforeDisable);

    await openSettingsModal(page);
    const reloadedDisableResizeToggle = page.locator('#toggleDisableNoteResize');
    await expect(reloadedDisableResizeToggle).toBeChecked();
    await reloadedDisableResizeToggle.evaluate((element) => element.click());
    await expect(reloadedDisableResizeToggle).not.toBeChecked();
    await expect.poll(async () => {
      const indexedDBState = await readIndexedDBState(page);
      return indexedDBState.snapshot ? indexedDBState.snapshot.disableNoteResize : null;
    }).toBe('false');
    await page.locator('.close-settings').click();
    await expect(page.locator('#settingsModal')).toBeHidden();

    await expect(note.locator('.grid-item-resize-handle')).toBeVisible();
    await resizeNoteBy(page, note, 80, 40);
    const resizedNote = await readNoteSize(note);
    expect(resizedNote.width).toBeGreaterThan(sizeBeforeDisable.width);
    expect(resizedNote.height).toBeGreaterThan(sizeBeforeDisable.height);
  });

  test('enters textarea edit mode on desktop double-click', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Edit Me');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await note.dblclick();

    const textarea = page.locator('textarea.edit-textarea');
    await expect(textarea).toBeVisible();
    await expect(textarea).toBeFocused();
    await expect(textarea).toHaveValue('Edit Me');
  });

  test('commits note edits on blur', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Blur Me');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await note.dblclick();

    const textarea = page.locator('textarea.edit-textarea');
    await textarea.fill('Blur Saved');
    await textarea.evaluate((element) => element.blur());

    await expect(textarea).toHaveCount(0);
    await expectNoteVisible(page, 'Blur Saved');

    const indexedDBState = await readIndexedDBState(page);
    expect(JSON.parse(indexedDBState.snapshot.tabs)[0].items[0].name).toBe('Blur Saved');
  });

  test('keeps Enter as newline and commits multiline text on Escape', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Line 0');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await note.dblclick();

    const textarea = page.locator('textarea.edit-textarea');
    await textarea.fill('Line 1');
    await textarea.press('Enter');
    await expect(textarea).toBeVisible();
    await textarea.type('Line 2');
    await expect(textarea).toHaveValue('Line 1\nLine 2');
    await textarea.press('Escape');

    await expect(textarea).toHaveCount(0);
    await expect(page.locator('.grid-item span').filter({ hasText: 'Line 1' }).first()).toBeVisible();
    await expect(page.locator('.grid-item span').filter({ hasText: 'Line 2' }).first()).toBeVisible();

    const indexedDBState = await readIndexedDBState(page);
    expect(JSON.parse(indexedDBState.snapshot.tabs)[0].items[0].name).toBe('Line 1\nLine 2');
  });

  test('keeps a brand new blank note in edit mode until it is named', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Existing Note');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const grid = page.locator('.grid-container').first();
    await expect(grid).toBeVisible();
    await grid.evaluate((element) => {
      element.dispatchEvent(new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        button: 2,
        clientX: 900,
        clientY: 300
      }));
    });

    const textarea = page.locator('textarea.edit-textarea');
    await expect(textarea).toBeVisible();
    await expect(textarea).toHaveValue('');

    await textarea.evaluate((element) => element.blur());
    await expect(textarea).toBeVisible();
    await expect(textarea).toBeFocused();

    await textarea.fill('Named New Note');
    await textarea.press('Escape');

    await expect(textarea).toHaveCount(0);
    await expectNoteVisible(page, 'Named New Note');
  });

  test('does not add duplicate dblclick edit handlers across repeated edit cycles', async ({ page }) => {
    await page.addInitScript(() => {
      const originalAddEventListener = EventTarget.prototype.addEventListener;
      window.__gridItemDblclickListenerAdds = {};

      EventTarget.prototype.addEventListener = function patchedAddEventListener(type, listener, options) {
        if (
          type === 'dblclick' &&
          this instanceof HTMLElement &&
          this.classList &&
          this.classList.contains('grid-item')
        ) {
          const itemId = this.dataset && this.dataset.id ? this.dataset.id : '__unknown__';
          window.__gridItemDblclickListenerAdds[itemId] =
            (window.__gridItemDblclickListenerAdds[itemId] || 0) + 1;
        }

        return originalAddEventListener.call(this, type, listener, options);
      };
    });

    const localSnapshot = buildLocalSnapshot('Cycle 0');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    for (const nextText of ['Cycle 1', 'Cycle 2', 'Cycle 3']) {
      await note.dblclick();
      const textarea = page.locator('textarea.edit-textarea');
      await textarea.fill(nextText);
      await textarea.press('Escape');
      await expect(textarea).toHaveCount(0);
      await expectNoteVisible(page, nextText);
    }

    const listenerAdds = await page.evaluate(() => window.__gridItemDblclickListenerAdds);
    expect(listenerAdds['item-1']).toBe(1);
  });

  test('right-click delete flow does not recolor the note', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Do Not Recolor');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await note.click({ button: 'right' });
    await expect(page.locator('#confirmModal')).toBeVisible();

    await page.waitForTimeout(350);
    const indexedDBState = await readIndexedDBState(page);
    expect(JSON.parse(indexedDBState.snapshot.tabs)[0].items[0].color).toBe('#ffee88');

    await page.click('#cancelDelete');
    await expect(page.locator('#confirmModal')).toBeHidden();
  });

  test('does not call sync endpoints and persists rapid note edits across reload', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Edit Me');
    const syncRequests = [];

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);

    page.on('request', (request) => {
      const url = request.url();
      if (url.includes('/api/document/meta') || url.includes('/api/document')) {
        syncRequests.push(url);
      }
    });

    await page.goto('/index.html');
    const note = page.locator('.grid-item').filter({ hasText: 'Edit Me' }).first();
    await expect(note).toBeVisible();
    await note.dblclick();
    const textarea = page.locator('textarea.edit-textarea');
    await expect(textarea).toBeVisible();
    await textarea.fill('Edited Immediately');
    await textarea.press('Escape');
    await expect(textarea).toHaveCount(0);

    await page.reload();
    await expectNoteVisible(page, 'Edited Immediately');
    expect(syncRequests).toEqual([]);
  });

  test('exposes debug helpers in static mode', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Debug Helper Note', { theme: 'dark' });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');
    await expectNoteVisible(page, 'Debug Helper Note');

    const debugState = await page.evaluate(async () => {
      return {
        storageHelperType: typeof window.postbabyDebugStorage,
        syncHelperType: typeof window.postbabyDebugSync,
        storage: await window.postbabyDebugStorage(),
        sync: window.postbabyDebugSync()
      };
    });

    expect(debugState.storageHelperType).toBe('function');
    expect(debugState.syncHelperType).toBe('function');
    expect(debugState.storage.ready).toBe(true);
    expect(debugState.storage.mode).toBe('indexeddb-primary');
    expect(debugState.storage.hasTabs).toBe(true);
    expect(debugState.storage.themeFromCache).toBe('dark');
    expect(debugState.sync.runtimeConfig.syncAvailable).toBe(false);
    expect(debugState.sync.shouldStartSyncImmediately).toBe(false);
  });
});

test.describe('Touch note editing behavior', () => {
  test.use({
    hasTouch: true,
    isMobile: true,
    viewport: { width: 390, height: 844 }
  });

  test('does not expose resize handles on touch devices', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Touch Resize Hidden');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    await expect(page.locator('.grid-item-resize-handle')).toHaveCount(0);
  });

  test('double-tap enters textarea edit mode on touch devices', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Touch Edit');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await note.tap();
    await page.waitForTimeout(75);
    await note.tap();

    const textarea = page.locator('textarea.edit-textarea');
    await expect(textarea).toBeVisible();
    await expect(textarea).toBeFocused();
    await expect(textarea).toHaveValue('Touch Edit');
  });
});

test.describe('Mocked sync startup reconciliation', () => {
  test('auto-loads server snapshot when local state is replaceable', async ({ page }) => {
    const dialogs = [];
    attachDialogHandler(page, [], dialogs);
    await prepareBlankPage(page);
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 5, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Server Replaceable Snapshot', 5)
    });

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Server Replaceable Snapshot');
    expect(dialogs).toEqual([]);
  });

  test('cloud first paid sync asks before claiming anonymous browser data', async ({ page }) => {
    const anonymousSnapshot = buildLocalSnapshot('Anonymous Claim Note');
    const dialogs = [];
    let capturedSaveBody = null;

    await prepareBlankPage(page);
    await seedLocalStorage(page, anonymousSnapshot);
    await mockRuntimeConfig(page, {
      deploymentMode: 'cloud',
      authorityModel: 'subscription_sync',
      authAvailable: true,
      authRequired: false,
      isAuthenticated: true,
      billingAvailable: true,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: true,
      syncPausedReason: '',
      entitlement: { hostedSync: true, status: 'active' },
      account: {
        username: 'paid-user',
        displayName: 'paid-user',
        email: '',
        avatarUrl: '',
        isAdmin: false,
        storageKey: 'paid-account-scope',
        status: 'active'
      }
    });
    await page.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: true, exists: false })
      });
    });
    await page.route(/\/api\/document(?:\?.*)?$/, async (route) => {
      const request = route.request();
      if (request.method() === 'PUT') {
        capturedSaveBody = JSON.parse(request.postData() || '{}');
        await route.fulfill({
          status: 200,
          contentType: 'application/json; charset=utf-8',
          headers: { 'Cache-Control': 'no-store' },
          body: JSON.stringify({ ok: true, version: 1, updatedAt: TIMESTAMP })
        });
        return;
      }
      await route.fulfill({
        status: 404,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: false, error: { code: 'document_not_found' } })
      });
    });
    attachDialogHandler(page, [{
      type: 'confirm',
      action: 'accept',
      messageIncludes: 'Move this browser\'s notes into your account?'
    }], dialogs);

    await page.goto('/index.html');

    await expect.poll(() => capturedSaveBody !== null).toBe(true);
    expect(dialogs).toHaveLength(1);
    expect(capturedSaveBody.version).toBe(0);
    expect(capturedSaveBody.baseServerRevision).toBe(0);
    expect(JSON.parse(capturedSaveBody.data.tabs)[0].items[0].name).toBe('Anonymous Claim Note');
  });

  test('self-hosted authenticated startup blocks when account storage key is missing', async ({ page }) => {
    const leakedPrimarySnapshot = buildLocalSnapshot('Primary Leak Note');
    const syncRequests = [];

    await prepareBlankPage(page);
    await seedLocalStorage(page, leakedPrimarySnapshot);
    await mockRuntimeConfig(page, {
      deploymentMode: 'selfhosted',
      authorityModel: 'server_authoritative',
      authAvailable: true,
      authRequired: true,
      isAuthenticated: true,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: true,
      syncPausedReason: '',
      account: {
        username: 'broken-user',
        displayName: 'broken-user',
        email: '',
        avatarUrl: '',
        isAdmin: false,
        storageKey: '',
        status: 'active'
      }
    });

    page.on('request', (request) => {
      const url = request.url();
      if (url.includes('/api/document/meta') || url.includes('/api/document')) {
        syncRequests.push(url);
      }
    });

    await page.goto('/index.html');

    await expect(page.locator('.postbaby-startup-blocked')).toBeVisible();
    await expect(page.locator('.postbaby-startup-blocked')).toContainText('missing its private browser storage scope');
    await expect(page.locator('.grid-item span').filter({ hasText: 'Primary Leak Note' })).toHaveCount(0);

    const debugState = await page.evaluate(async () => ({
      storage: await window.postbabyDebugStorage(),
      sync: window.postbabyDebugSync()
    }));

    expect(syncRequests).toEqual([]);
    expect(debugState.storage.startupBlockedReason).toBe('missing-storage-key');
    expect(debugState.sync.startupBlockedReason).toBe('missing-storage-key');
    expect(debugState.sync.storageScopeKey).toBeNull();
  });

  test('self-hosted new account ignores primary browser data and starts blank', async ({ page }) => {
    const leakedPrimarySnapshot = buildLocalSnapshot('Primary Leak Note');
    let capturedSaveBody = null;

    await prepareBlankPage(page);
    await seedLocalStorage(page, leakedPrimarySnapshot);
    await mockRuntimeConfig(page, {
      deploymentMode: 'selfhosted',
      authorityModel: 'server_authoritative',
      authAvailable: true,
      authRequired: true,
      isAuthenticated: true,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: true,
      syncPausedReason: '',
      account: {
        username: 'fresh-user',
        displayName: 'fresh-user',
        email: '',
        avatarUrl: '',
        isAdmin: false,
        storageKey: 'fresh-user-scope',
        status: 'active'
      }
    });
    await page.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: true, exists: false })
      });
    });
    await page.route(/\/api\/document(?:\?.*)?$/, async (route) => {
      const request = route.request();
      if (request.method() === 'PUT') {
        capturedSaveBody = JSON.parse(request.postData() || '{}');
        await route.fulfill({
          status: 200,
          contentType: 'application/json; charset=utf-8',
          headers: { 'Cache-Control': 'no-store' },
          body: JSON.stringify({ ok: true, version: 1, updatedAt: TIMESTAMP })
        });
        return;
      }

      await route.fulfill({
        status: 404,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: false, error: { code: 'document_not_found' } })
      });
    });

    await page.goto('/index.html');

    await expect(page.locator('.grid-item span').filter({ hasText: 'Primary Leak Note' })).toHaveCount(0);
    await expect(page.locator('.grid-item')).toHaveCount(0);
    await page.waitForTimeout(2200);
    expect(capturedSaveBody).toBeNull();

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.storageScopeKey).toBe('fresh-user-scope');
  });

  test('self-hosted accounts stay isolated across two users in the same browser', async ({ page }) => {
    const serverSnapshots = new Map();
    const users = {
      alice: {
        username: 'alice',
        displayName: 'alice',
        email: '',
        avatarUrl: '',
        isAdmin: true,
        storageKey: 'alice-scope',
        status: 'active'
      },
      bob: {
        username: 'bob',
        displayName: 'bob',
        email: '',
        avatarUrl: '',
        isAdmin: false,
        storageKey: 'bob-scope',
        status: 'active'
      }
    };
    let activeUser = users.alice;

    await prepareBlankPage(page);
    await page.route('**/runtime-config.js', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/javascript; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: [
          'window.POSTBABY_RUNTIME = {',
          '  deploymentMode: "selfhosted",',
          '  authorityModel: "server_authoritative",',
          '  authAvailable: true,',
          '  authRequired: true,',
          '  isAuthenticated: true,',
          '  syncAvailable: true,',
          '  syncRequiresAuth: true,',
          '  syncUsable: true,',
          '  syncPausedReason: "",',
          '  entitlement: { hostedSync: false, status: "none" },',
          '  apiBase: "",',
          `  account: ${JSON.stringify(activeUser)}`,
          '};'
        ].join('\n')
      });
    });
    await page.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
      const snapshot = serverSnapshots.get(activeUser.username) || null;
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify(snapshot
          ? { ok: true, exists: true, version: snapshot.version, updatedAt: snapshot.updatedAt }
          : { ok: true, exists: false })
      });
    });
    await page.route(/\/api\/document(?:\?.*)?$/, async (route) => {
      const request = route.request();
      const currentSnapshot = serverSnapshots.get(activeUser.username) || null;

      if (request.method() === 'GET') {
        if (!currentSnapshot) {
          await route.fulfill({
            status: 404,
            contentType: 'application/json; charset=utf-8',
            headers: { 'Cache-Control': 'no-store' },
            body: JSON.stringify({ ok: false, error: { code: 'document_not_found' } })
          });
          return;
        }

        await route.fulfill({
          status: 200,
          contentType: 'application/json; charset=utf-8',
          headers: { 'Cache-Control': 'no-store' },
          body: JSON.stringify(currentSnapshot)
        });
        return;
      }

      if (request.method() === 'PUT') {
        const requestBody = JSON.parse(request.postData() || '{}');
        const nextVersion = currentSnapshot ? currentSnapshot.version + 1 : 1;
        const nextSnapshot = {
          version: nextVersion,
          updatedAt: TIMESTAMP,
          data: requestBody.data
        };
        serverSnapshots.set(activeUser.username, nextSnapshot);
        await route.fulfill({
          status: 200,
          contentType: 'application/json; charset=utf-8',
          headers: { 'Cache-Control': 'no-store' },
          body: JSON.stringify({ ok: true, version: nextVersion, updatedAt: TIMESTAMP })
        });
        return;
      }

      await route.fallback();
    });

    await page.goto('/index.html');

    const grid = page.locator('.grid-container').first();
    await expect(grid).toBeVisible();
    await grid.evaluate((element) => {
      element.dispatchEvent(new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        button: 2,
        clientX: 900,
        clientY: 300
      }));
    });

    const textarea = page.locator('textarea.edit-textarea');
    await expect(textarea).toBeVisible();
    await textarea.fill('Alice Private Note');
    await textarea.press('Escape');
    await expectNoteVisible(page, 'Alice Private Note');
    await expect.poll(() => {
      const snapshot = serverSnapshots.get('alice');
      if (!snapshot) {
        return null;
      }
      return JSON.parse(snapshot.data.tabs)[0].items[0].name;
    }).toBe('Alice Private Note');

    activeUser = users.bob;
    await page.reload();
    await expect(page.locator('.grid-item span').filter({ hasText: 'Alice Private Note' })).toHaveCount(0);
    await page.waitForTimeout(2200);
    expect(serverSnapshots.has('bob')).toBe(false);
    const bobDebugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(bobDebugState.storageScopeKey).toBe('bob-scope');

    activeUser = users.alice;
    await page.reload();
    await expectNoteVisible(page, 'Alice Private Note');
    const aliceDebugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(aliceDebugState.storageScopeKey).toBe('alice-scope');
  });

  test('reactivation prompts before uploading local fork with base revision', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Inactive Fork Note', { syncVersion: 100 });
    const dialogs = [];
    let capturedSaveBody = null;

    localSnapshot.postbabySyncLocalForkDirty = 'true';
    localSnapshot.postbabySyncLocalForkBaseRevision = '100';
    localSnapshot.postbabySyncLocalForkPausedReason = 'subscription_inactive';
    localSnapshot.postbabySyncLocalForkCreatedAt = TIMESTAMP;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'paid-account-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'paid-account-scope' }), 'paid-account-scope');
    await mockRuntimeConfig(page, {
      deploymentMode: 'cloud',
      authorityModel: 'subscription_sync',
      authAvailable: true,
      authRequired: false,
      isAuthenticated: true,
      billingAvailable: true,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: true,
      syncPausedReason: '',
      entitlement: { hostedSync: true, status: 'active' },
      account: {
        username: 'paid-user',
        displayName: 'paid-user',
        email: '',
        avatarUrl: '',
        isAdmin: false,
        storageKey: 'paid-account-scope',
        status: 'active'
      }
    });
    await page.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: true, exists: true, version: 100, updatedAt: TIMESTAMP })
      });
    });
    await page.route(/\/api\/document(?:\?.*)?$/, async (route) => {
      const request = route.request();
      if (request.method() === 'PUT') {
        capturedSaveBody = JSON.parse(request.postData() || '{}');
        await route.fulfill({
          status: 200,
          contentType: 'application/json; charset=utf-8',
          headers: { 'Cache-Control': 'no-store' },
          body: JSON.stringify({ ok: true, version: 101, updatedAt: TIMESTAMP })
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify(buildServerPayload('Cloud Snapshot 100', 100))
      });
    });
    attachDialogHandler(page, [{
      type: 'confirm',
      action: 'accept',
      messageIncludes: 'Use this browser\'s version for your account?'
    }], dialogs);

    await page.goto('/index.html');

    await expect.poll(() => capturedSaveBody !== null).toBe(true);
    expect(dialogs).toHaveLength(1);
    expect(capturedSaveBody.version).toBe(100);
    expect(capturedSaveBody.baseServerRevision).toBe(100);
    expect(JSON.parse(capturedSaveBody.data.tabs)[0].items[0].name).toBe('Inactive Fork Note');
  });

  test('reactivation blocks stale local fork before upload', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Stale Inactive Fork', { syncVersion: 100 });
    const dialogs = [];
    let saveAttempted = false;

    localSnapshot.postbabySyncLocalForkDirty = 'true';
    localSnapshot.postbabySyncLocalForkBaseRevision = '100';
    localSnapshot.postbabySyncLocalForkPausedReason = 'subscription_inactive';
    localSnapshot.postbabySyncLocalForkCreatedAt = TIMESTAMP;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'paid-account-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'paid-account-scope' }), 'paid-account-scope');
    await mockRuntimeConfig(page, {
      deploymentMode: 'cloud',
      authorityModel: 'subscription_sync',
      authAvailable: true,
      authRequired: false,
      isAuthenticated: true,
      billingAvailable: true,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: true,
      syncPausedReason: '',
      entitlement: { hostedSync: true, status: 'active' },
      account: {
        username: 'paid-user',
        displayName: 'paid-user',
        email: '',
        avatarUrl: '',
        isAdmin: false,
        storageKey: 'paid-account-scope',
        status: 'active'
      }
    });
    await page.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: true, exists: true, version: 101, updatedAt: TIMESTAMP })
      });
    });
    await page.route(/\/api\/document(?:\?.*)?$/, async (route) => {
      if (route.request().method() === 'PUT') {
        saveAttempted = true;
      }
      await route.fulfill({
        status: 500,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: false })
      });
    });
    attachDialogHandler(page, [
      {
        type: 'confirm',
        action: 'accept',
        messageIncludes: 'Use this browser\'s version for your account?'
      },
      {
        type: 'alert',
        action: 'accept',
        messageIncludes: 'cloud version changed while sync was paused'
      }
    ], dialogs);

    await page.goto('/index.html');

    await expect.poll(() => dialogs.length).toBe(2);
    expect(saveAttempted).toBe(false);
    await expect(page.locator('#syncStateStatus')).toContainText('Conflict - cloud changed');
  });

  test('prompts on missing local sync version and loads account notes when accepted', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Meaningful Local Note');
    const dialogs = [];

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 13, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Account Notes From Server', 13)
    });
    attachDialogHandler(page, [{
      type: 'confirm',
      action: 'accept',
      messageIncludes: 'Use the account notes on this browser?'
    }], dialogs);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Account Notes From Server');
    expect(dialogs).toHaveLength(1);
  });

  test('prompts on stale local sync version and can intentionally upload local notes', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Local Upload Candidate', {
      syncVersion: 136,
      width: 280,
      height: 190
    });
    const dialogs = [];
    let capturedSaveBody = null;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 13, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Server Snapshot 13', 13),
      onSave: async (body) => {
        capturedSaveBody = body;
        return {
          status: 200,
          body: {
            ok: true,
            version: 14,
            updatedAt: TIMESTAMP
          }
        };
      }
    });
    attachDialogHandler(page, [
      {
        type: 'confirm',
        action: 'dismiss',
        messageIncludes: 'Use the account notes on this browser?'
      },
      {
        type: 'confirm',
        action: 'accept',
        messageIncludes: 'replace the synced account copy'
      }
    ], dialogs);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Local Upload Candidate');

    await expect.poll(() => dialogs.length).toBe(2);
    await expect.poll(() => capturedSaveBody !== null).toBe(true);
    expect(capturedSaveBody).not.toBeNull();
    expect(capturedSaveBody.version).toBe(13);
    expect(Object.keys(capturedSaveBody.data).sort()).toEqual([
      'activeTabId',
      'hasRunBefore',
      'tabs',
      'theme'
    ]);
    expect(capturedSaveBody.data).not.toHaveProperty('width');
    expect(capturedSaveBody.data).not.toHaveProperty('height');
    expect(JSON.parse(capturedSaveBody.data.tabs)[0].items[0]).toMatchObject({
      name: 'Local Upload Candidate',
      width: 280,
      height: 190
    });

    const indexedDBState = await readIndexedDBState(page, 'owner-scope');
    expect(indexedDBState.snapshot.postbabySyncVersion).toBe('14');
  });

  test('sync upload keeps arrow kind embedded inside the tabs payload', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Arrow Sync Source', {
        itemId: 'item-1',
        position: { top: '100px', left: '140px' }
      }),
      buildNoteItem('Arrow Sync Target', {
        itemId: 'item-2',
        position: { top: '140px', left: '420px' }
      })
    ], {
      edges: [{
        id: 'edge-1',
        fromItemId: 'item-1',
        toItemId: 'item-2',
        kind: 'arrow'
      }],
      syncVersion: 136
    });
    const dialogs = [];
    let capturedSaveBody = null;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 13, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Server Snapshot 13', 13),
      onSave: async (body) => {
        capturedSaveBody = body;
        return {
          status: 200,
          body: {
            ok: true,
            version: 14,
            updatedAt: TIMESTAMP
          }
        };
      }
    });
    attachDialogHandler(page, [
      {
        type: 'confirm',
        action: 'dismiss',
        messageIncludes: 'Use the account notes on this browser?'
      },
      {
        type: 'confirm',
        action: 'accept',
        messageIncludes: 'replace the synced account copy'
      }
    ], dialogs);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Arrow Sync Source');
    await expect.poll(() => capturedSaveBody !== null).toBe(true);

    expect(Object.keys(capturedSaveBody.data).sort()).toEqual([
      'activeTabId',
      'hasRunBefore',
      'tabs',
      'theme'
    ]);
    expect(capturedSaveBody.data).not.toHaveProperty('kind');

    const savedTabs = JSON.parse(capturedSaveBody.data.tabs);
    expect(savedTabs[0].edges).toHaveLength(1);
    expect(savedTabs[0].edges[0]).toMatchObject({
      id: 'edge-1',
      fromItemId: 'item-1',
      toItemId: 'item-2',
      kind: 'arrow'
    });
  });

  test('prompts on stale local sync version and keeps local notes when user cancels', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Keep Local Notes', { syncVersion: 136 });
    const dialogs = [];

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 13, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Server Snapshot 13', 13)
    });
    attachDialogHandler(page, [
      {
        type: 'confirm',
        action: 'dismiss',
        messageIncludes: 'Use the account notes on this browser?'
      },
      {
        type: 'confirm',
        action: 'dismiss',
        messageIncludes: 'upload them to your account instead'
      },
      {
        type: 'alert',
        action: 'accept',
        messageIncludes: 'do not match the synced account copy'
      }
    ], dialogs);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Keep Local Notes');
    await expect(page.locator('#syncStateStatus')).toContainText('Conflict - choose local or server copy');
    expect(dialogs).toHaveLength(3);
  });

  test('auto-loads newer server snapshot when local version is behind and local state is clean', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Behind Local Version', { syncVersion: 10 });
    const dialogs = [];

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 11, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Server Version 11', 11)
    });
    attachDialogHandler(page, [], dialogs);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Server Version 11');
    expect(dialogs).toEqual([]);
  });

  test('repairs durable pending upload after reload when server revision is unchanged', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshot('Quick Close Repair Note', { syncVersion: 6 }),
      {
        pending: true,
        uploadedHash: 'previous-uploaded-hash',
        revision: 6,
        localModifiedAt: new Date().toISOString(),
        lastCloudUploadedAt: TIMESTAMP
      }
    );
    let capturedSaveBody = null;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Server Version 6', 6),
      onSave: async (body) => {
        capturedSaveBody = body;
        return {
          status: 200,
          body: { ok: true, version: 7, updatedAt: TIMESTAMP }
        };
      }
    });

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Quick Close Repair Note');
    await expect.poll(() => capturedSaveBody !== null).toBe(true);
    expect(capturedSaveBody.baseServerRevision).toBe(6);
    expect(JSON.parse(capturedSaveBody.data.tabs)[0].items[0].name).toBe('Quick Close Repair Note');

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.syncStoredVersion).toBe(7);
    expect(debugState.durablePendingCloudUpload).toBe(false);
    expect(debugState.durableSyncMetadata.pendingCloudUpload).toBe(false);
    expect(debugState.durableSyncMetadata.localSnapshotHash).toBe(debugState.durableSyncMetadata.lastUploadedSnapshotHash);
  });

  test('shows pending instead of synced when local hash differs while server revision matches', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshot('Pending Equal Revision Note', { syncVersion: 6 }),
      {
        pending: true,
        uploadedHash: 'previous-uploaded-hash',
        revision: 6,
        localModifiedAt: new Date().toISOString()
      }
    );
    let releaseMeta;
    const metaGate = new Promise((resolve) => {
      releaseMeta = resolve;
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await mockRuntimeConfig(page, {
      deploymentMode: 'selfhosted',
      authorityModel: 'server_authoritative',
      authAvailable: true,
      authRequired: true,
      isAuthenticated: true,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: true,
      syncPausedReason: '',
      account: {
        username: 'owner',
        displayName: 'owner',
        email: '',
        avatarUrl: '',
        isAdmin: true,
        storageKey: 'owner-scope',
        status: 'active'
      }
    });
    await page.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
      await metaGate;
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: true, exists: true, version: 6, updatedAt: TIMESTAMP })
      });
    });
    await page.route(/\/api\/document(?:\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 500,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: false })
      });
    });

    await page.goto('/index.html');
    await expect(page.locator('#syncStateStatus')).toContainText('Sync pending');
    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.syncState).toBe('dirty');
    expect(debugState.durablePendingCloudUpload).toBe(true);
    releaseMeta();
  });

  test('manual Sync now uploads pending changes and clears durable pending only after success', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshot('Manual Sync Success', { syncVersion: 6 }),
      {
        pending: false,
        revision: 6,
        lastCloudUploadedAt: TIMESTAMP
      }
    );
    let capturedSaveBody = null;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Server Version 6', 6),
      onSave: async (body) => {
        capturedSaveBody = body;
        return {
          status: 200,
          body: { ok: true, version: 7, updatedAt: TIMESTAMP }
        };
      }
    });

    await page.goto('/index.html');
    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();
    await expect.poll(() => capturedSaveBody !== null).toBe(true);

    await expect.poll(async () => {
      const state = await page.evaluate(() => window.postbabyDebugSync());
      return state.syncStoredVersion;
    }).toBe(7);
    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(capturedSaveBody.baseServerRevision).toBe(6);
    expect(debugState.durablePendingCloudUpload).toBe(false);
  });

  test('manual Sync now preserves durable pending state on upload failure', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshot('Manual Sync Failure', { syncVersion: 6 }),
      {
        pending: false,
        revision: 6,
        lastCloudUploadedAt: TIMESTAMP
      }
    );
    let saveAttempts = 0;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Server Version 6', 6),
      onSave: async () => {
        saveAttempts += 1;
        return {
          status: 500,
          body: { ok: false, error: { code: 'server_error', message: 'forced failure' } }
        };
      }
    });
    attachDialogHandler(page, [{ type: 'alert', action: 'accept', messageIncludes: 'forced failure' }], []);

    await page.goto('/index.html');
    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();
    await expect.poll(() => saveAttempts).toBeGreaterThan(0);
    await expect.poll(async () => {
      const state = await page.evaluate(() => window.postbabyDebugSync());
      return state.durableSyncMetadata.lastError || '';
    }).toContain('forced failure');

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.durablePendingCloudUpload).toBe(true);
    expect(debugState.durableSyncMetadata.lastError).toContain('forced failure');
  });

  test('structural tab creation syncs immediately while note drag remains debounced', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshot('Debounced Drag Note', { syncVersion: 6 }),
      {
        pending: false,
        revision: 6,
        lastCloudUploadedAt: TIMESTAMP
      }
    );
    const saveTimes = [];

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Server Version 6', 6),
      onSave: async () => {
        saveTimes.push(Date.now());
        return {
          status: 200,
          body: { ok: true, version: 7 + saveTimes.length, updatedAt: TIMESTAMP }
        };
      }
    });

    await page.goto('/index.html');
    const createStartedAt = Date.now();
    await page.locator('#addTab').click();
    await expect.poll(() => saveTimes.length).toBeGreaterThan(0);
    expect(saveTimes[0] - createStartedAt).toBeLessThan(1000);

    await prepareBlankPage(page);
    const dragSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshot('Debounced Drag Note', { syncVersion: 6 }),
      {
        pending: false,
        revision: 6,
        lastCloudUploadedAt: TIMESTAMP
      }
    );
    saveTimes.length = 0;
    await seedLocalStorage(page, dragSnapshot, 'owner-scope');
    await seedIndexedDB(page, dragSnapshot, buildIndexedDBMeta(dragSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await page.goto('/index.html');
    await dragNoteBy(page, page.locator('.grid-item[data-id="item-1"]'), 80, 40);
    await page.waitForTimeout(700);
    expect(saveTimes).toHaveLength(0);
    await expect.poll(() => saveTimes.length).toBeGreaterThan(0);
  });

  test('durable pending local change conflicts when server revision advanced', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshot('Local Pending Conflict', { syncVersion: 6 }),
      {
        pending: true,
        uploadedHash: 'previous-uploaded-hash',
        revision: 6,
        localModifiedAt: new Date().toISOString()
      }
    );
    let saveAttempted = false;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 7, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Server Version 7', 7),
      onSave: async () => {
        saveAttempted = true;
        return {
          status: 200,
          body: { ok: true, version: 8, updatedAt: TIMESTAMP }
        };
      }
    });

    await page.goto('/index.html');
    await expect(page.locator('#syncStateStatus')).toContainText('Conflict needs review');
    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.syncState).toBe('conflict');
    expect(debugState.durablePendingCloudUpload).toBe(true);
    expect(saveAttempted).toBe(false);

    await page.locator('#syncStatusButton').click();
    await expect(page.locator('#syncUseBrowserButton')).toBeVisible();
    await expect(page.locator('#syncUseCloudButton')).toBeVisible();
  });
});

test.describe('Settings and Account UI', () => {
  test('opens Settings from the gear button and not from trash click', async ({ page }) => {
    await prepareBlankPage(page);
    await page.goto('/index.html');

    await page.locator('#trash').click();
    await expect(page.locator('#settingsModal')).toBeHidden();

    await openSettingsModal(page);
    await expect(page.locator('#settingsModal')).toBeVisible();
  });

  test('drag-to-delete still removes notes including corporate trash image mode', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Delete Me');
    localSnapshot.corporateMode = 'true';

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    await expect(page.locator('#trash')).toHaveAttribute('src', /corporatetrash\.png/);
    const note = page.locator('.grid-item[data-id="item-1"]');
    await expectNoteVisible(page, 'Delete Me');

    await dragNoteToTrash(page, note);
    await expect(page.locator('#confirmModal')).toBeVisible();
    await page.click('#confirmDelete');
    await expect(page.locator('.grid-item[data-id="item-1"]')).toHaveCount(0);
  });

  test('static UI shows only gear and includes the local-only settings note', async ({ page }) => {
    await prepareBlankPage(page);
    await page.goto('/index.html');

    await expect(page.locator('#settingsButton .top-app-settings-icon')).toBeVisible();
    await expect(page.locator('#topAppControls button:visible')).toHaveCount(1);
    await expect(page.locator('#accountButton')).toBeHidden();

    await openSettingsModal(page);
    await expect(page.locator('.questions')).toContainText('Questions/Comments:');
    await expect(page.locator('#staticSettingsNote')).toBeVisible();
    await expect(page.locator('#staticSettingsNote')).toContainText('This static version saves notes only in this browser on this device.');
  });

  test('cloud logged-out account modal shows local-only copy and auth actions', async ({ page }) => {
    await prepareBlankPage(page);
    await mockRuntimeConfig(page, {
      deploymentMode: 'cloud',
      authorityModel: 'subscription_sync',
      authAvailable: true,
      authRequired: false,
      billingAvailable: true,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: false,
      syncPausedReason: 'auth_required',
      entitlement: { hostedSync: false, status: 'none' },
      account: null
    });
    await page.goto('/index.html');

    await openAccountModal(page);
    await expect(page.locator('#accountLocalOnlyCopy')).toBeVisible();
    await expect(page.locator('#accountUnavailableCopy')).toBeHidden();
    await expect(page.locator('#loginLink')).toBeVisible();
    await expect(page.locator('#signupLink')).toBeVisible();
    await expect(page.locator('#signupLink')).toHaveText('Upgrade');
    await expect(page.locator('#loginLink')).toHaveAttribute('href', '/login');
    await expect(page.locator('#signupLink')).toHaveAttribute('href', '/signup');
  });

  test('cloud logged-out account modal uses upgrade wording', async ({ page }) => {
    await prepareBlankPage(page);
    await mockRuntimeConfig(page, {
      deploymentMode: 'cloud',
      authorityModel: 'subscription_sync',
      authAvailable: true,
      authRequired: false,
      isAuthenticated: false,
      billingAvailable: true,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: false,
      syncPausedReason: 'auth_required',
      entitlement: { hostedSync: false, status: 'none' },
      account: null
    });
    await page.goto('/index.html');

    await openAccountModal(page);
    await expect(page.locator('#accountLocalOnlyCopy')).toContainText('Upgrade to sync');
    await expect(page.locator('#signupLink')).toBeVisible();
    await expect(page.locator('#signupLink')).toHaveText('Upgrade');
  });

  test('cloud logged-out account modal hides upgrade when billing is unavailable', async ({ page }) => {
    await prepareBlankPage(page);
    await mockRuntimeConfig(page, {
      deploymentMode: 'cloud',
      authorityModel: 'subscription_sync',
      authAvailable: true,
      authRequired: false,
      isAuthenticated: false,
      billingAvailable: false,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: false,
      syncPausedReason: 'auth_required',
      entitlement: { hostedSync: false, status: 'none' },
      account: null
    });
    await page.goto('/index.html');

    await openAccountModal(page);
    await expect(page.locator('#accountLocalOnlyCopy')).toContainText('Log in to access any existing account on this server.');
    await expect(page.locator('#accountLocalOnlyCopy')).not.toContainText('Upgrade to sync');
    await expect(page.locator('#accountUnavailableCopy')).toBeVisible();
    await expect(page.locator('#accountUnavailableCopy')).toContainText('Billing is not configured on this server right now');
    await expect(page.locator('#accountBenefitsCopy')).toBeHidden();
    await expect(page.locator('#loginLink')).toBeVisible();
    await expect(page.locator('#signupLink')).toBeHidden();
  });

  test('cloud unentitled account shows upgrade copy only when billing is available', async ({ page }) => {
    await prepareBlankPage(page);
    await mockRuntimeConfig(page, {
      deploymentMode: 'cloud',
      authorityModel: 'subscription_sync',
      authAvailable: true,
      authRequired: false,
      isAuthenticated: true,
      billingAvailable: true,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: false,
      syncPausedReason: 'subscription_required',
      entitlement: { hostedSync: false, status: 'none' },
      account: {
        username: 'new-user',
        displayName: 'New User',
        email: '',
        avatarUrl: '',
        isAdmin: false,
        storageKey: 'new-user-scope',
        status: 'active'
      }
    });
    await page.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: true, exists: false })
      });
    });
    await page.goto('/index.html');

    await openAccountModal(page);
    await expect(page.locator('.settings-sync-copy')).toContainText('Cloud sync is not active for this account yet.');
    await expect(page.locator('.settings-sync-copy')).not.toContainText('subscription is inactive');
    await expect(page.locator('#syncStateStatus')).toContainText('Upgrade to start account sync');
    await expect(page.locator('#billingCheckoutForm')).toBeVisible();
    await expect(page.locator('#billingCheckoutForm button')).toHaveText('Upgrade');
    await expect(page.locator('#billingPortalForm')).toBeHidden();
  });

  test('cloud unentitled account explains billing-disabled fallback', async ({ page }) => {
    await prepareBlankPage(page);
    await mockRuntimeConfig(page, {
      deploymentMode: 'cloud',
      authorityModel: 'subscription_sync',
      authAvailable: true,
      authRequired: false,
      isAuthenticated: true,
      billingAvailable: false,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: false,
      syncPausedReason: 'subscription_required',
      entitlement: { hostedSync: false, status: 'none' },
      account: {
        username: 'new-user',
        displayName: 'New User',
        email: '',
        avatarUrl: '',
        isAdmin: false,
        storageKey: 'new-user-scope',
        status: 'active'
      }
    });
    await page.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: true, exists: false })
      });
    });
    await page.goto('/index.html');

    await openAccountModal(page);
    await expect(page.locator('.settings-sync-copy')).toContainText('Cloud sync is not active for this account yet.');
    await expect(page.locator('.settings-sync-copy')).toContainText('Billing is not configured on this server right now, so upgrades are unavailable here.');
    await expect(page.locator('#syncStateStatus')).toContainText('Account sync unavailable on this server');
    await expect(page.locator('#billingCheckoutForm')).toBeHidden();
    await expect(page.locator('#billingPortalForm')).toBeHidden();
  });

  test('cloud inactive account shows paused copy and reactivation controls', async ({ page }) => {
    await prepareBlankPage(page);
    await mockRuntimeConfig(page, {
      deploymentMode: 'cloud',
      authorityModel: 'subscription_sync',
      authAvailable: true,
      authRequired: false,
      isAuthenticated: true,
      billingAvailable: true,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: false,
      syncPausedReason: 'subscription_inactive',
      entitlement: { hostedSync: false, status: 'canceled' },
      account: {
        username: 'paid-user',
        displayName: 'Paid User',
        email: '',
        avatarUrl: '',
        isAdmin: false,
        storageKey: 'paid-account-scope',
        status: 'active'
      }
    });
    await page.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: true, exists: false })
      });
    });
    await page.goto('/index.html');

    await openAccountModal(page);
    await expect(page.locator('.settings-sync-copy')).toContainText('Cloud sync is paused because your subscription is inactive');
    await expect(page.locator('#syncStateStatus')).toContainText('Sync paused');
    await expect(page.locator('#billingCheckoutForm')).toBeVisible();
    await expect(page.locator('#billingCheckoutForm button')).toHaveText('Reactivate');
    await expect(page.locator('#billingPortalForm')).toBeVisible();
  });

  test('cloud inactive account explains billing-disabled fallback', async ({ page }) => {
    await prepareBlankPage(page);
    await mockRuntimeConfig(page, {
      deploymentMode: 'cloud',
      authorityModel: 'subscription_sync',
      authAvailable: true,
      authRequired: false,
      isAuthenticated: true,
      billingAvailable: false,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: false,
      syncPausedReason: 'subscription_inactive',
      entitlement: { hostedSync: false, status: 'canceled' },
      account: {
        username: 'paid-user',
        displayName: 'Paid User',
        email: '',
        avatarUrl: '',
        isAdmin: false,
        storageKey: 'paid-account-scope',
        status: 'active'
      }
    });
    await page.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: true, exists: false })
      });
    });
    await page.goto('/index.html');

    await openAccountModal(page);
    await expect(page.locator('.settings-sync-copy')).toContainText('Cloud sync is paused because your subscription is inactive');
    await expect(page.locator('.settings-sync-copy')).toContainText('reactivation is unavailable here');
    await expect(page.locator('#billingCheckoutForm')).toBeHidden();
    await expect(page.locator('#billingPortalForm')).toBeHidden();
  });

  test('cloud checkout-pending account shows continue-checkout state when billing is available', async ({ page }) => {
    await prepareBlankPage(page);
    await mockRuntimeConfig(page, {
      deploymentMode: 'cloud',
      authorityModel: 'subscription_sync',
      authAvailable: true,
      authRequired: false,
      isAuthenticated: true,
      billingAvailable: true,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: false,
      syncPausedReason: 'checkout_pending',
      entitlement: { hostedSync: false, status: 'none' },
      account: {
        username: 'checkout-user',
        displayName: 'Checkout User',
        email: '',
        avatarUrl: '',
        isAdmin: false,
        storageKey: 'checkout-user-scope',
        status: 'checkout_pending'
      }
    });
    await page.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: true, exists: false })
      });
    });
    await page.goto('/index.html');

    await openAccountModal(page);
    await expect(page.locator('.settings-sync-copy')).toContainText('Cloud sync is waiting for checkout to finish.');
    await expect(page.locator('#syncStateStatus')).toContainText('Checkout incomplete');
    await expect(page.locator('#billingCheckoutForm')).toBeVisible();
    await expect(page.locator('#billingCheckoutForm button')).toHaveText('Continue checkout');
    await expect(page.locator('#billingPortalForm')).toBeHidden();
  });

  test('cloud checkout-pending account explains billing-disabled fallback', async ({ page }) => {
    await prepareBlankPage(page);
    await mockRuntimeConfig(page, {
      deploymentMode: 'cloud',
      authorityModel: 'subscription_sync',
      authAvailable: true,
      authRequired: false,
      isAuthenticated: true,
      billingAvailable: false,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: false,
      syncPausedReason: 'checkout_pending',
      entitlement: { hostedSync: false, status: 'none' },
      account: {
        username: 'checkout-user',
        displayName: 'Checkout User',
        email: '',
        avatarUrl: '',
        isAdmin: false,
        storageKey: 'checkout-user-scope',
        status: 'checkout_pending'
      }
    });
    await page.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: true, exists: false })
      });
    });
    await page.goto('/index.html');

    await openAccountModal(page);
    await expect(page.locator('.settings-sync-copy')).toContainText('Cloud sync is waiting for checkout to finish.');
    await expect(page.locator('.settings-sync-copy')).toContainText('checkout cannot continue here');
    await expect(page.locator('#syncStateStatus')).toContainText('Checkout incomplete - saved locally only');
    await expect(page.locator('#billingCheckoutForm')).toBeHidden();
    await expect(page.locator('#billingPortalForm')).toBeHidden();
  });

  test('logged-in UI shows initials and Account identity label on the account button', async ({ page }) => {
    await prepareBlankPage(page);
    await mockRuntimeConfig(page, {
      deploymentMode: 'selfhosted',
      authAvailable: true,
      authRequired: true,
      isAuthenticated: true,
      account: {
        username: 'owner',
        displayName: 'Owner Name',
        email: '',
        avatarUrl: '',
        storageKey: 'owner-scope'
      }
    });
    await page.goto('/index.html');

    await expect(page.locator('#accountButton')).toHaveAttribute('aria-label', 'Account: Owner Name');
    await expect(page.locator('#accountButton')).toHaveAttribute('title', 'Account: Owner Name');
    await expect(page.locator('#accountButton .top-app-account-initials')).toHaveText('ON');

    await openAccountModal(page);
    await expect(page.locator('#accountLoggedInPanel')).toBeVisible();
    await expect(page.locator('#accountDisplayName')).toHaveText('Owner Name');
    await expect(page.locator('#syncStateStatus')).toBeVisible();
    await expect(page.locator('#logoutForm')).toBeVisible();
  });

  test('self-hosted logout copy does not claim notes stay locally available', async ({ page }) => {
    const dialogs = [];

    await prepareBlankPage(page);
    await mockRuntimeConfig(page, {
      deploymentMode: 'selfhosted',
      authorityModel: 'server_authoritative',
      authAvailable: true,
      authRequired: true,
      isAuthenticated: true,
      syncAvailable: true,
      syncRequiresAuth: true,
      syncUsable: true,
      syncPausedReason: '',
      account: {
        username: 'owner',
        displayName: 'owner',
        email: '',
        avatarUrl: '',
        isAdmin: true,
        storageKey: 'owner-scope',
        status: 'active'
      }
    });
    await page.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: true, exists: false })
      });
    });
    await page.route(/\/api\/document(?:\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 404,
        contentType: 'application/json; charset=utf-8',
        headers: { 'Cache-Control': 'no-store' },
        body: JSON.stringify({ ok: false, error: { code: 'document_not_found' } })
      });
    });
    attachDialogHandler(page, [{
      type: 'confirm',
      action: 'dismiss',
      messageIncludes: 'workspace will not be available again until you sign back into this account'
    }], dialogs);

    await page.goto('/index.html');
    await openAccountModal(page);
    await page.locator('#logoutForm button').click();

    expect(dialogs).toHaveLength(1);
    expect(dialogs[0].message).not.toContain('Your local notes stay saved in this browser on this device.');
  });

  test('Settings modal contains only local settings plus import and export', async ({ page }) => {
    await prepareBlankPage(page);
    await mockRuntimeConfig(page, {
      deploymentMode: 'selfhosted',
      authAvailable: true,
      authRequired: true,
      isAuthenticated: true,
      account: {
        username: 'owner',
        displayName: 'owner',
        email: '',
        avatarUrl: '',
        storageKey: 'owner-scope'
      }
    });
    await page.goto('/index.html');

    await openSettingsModal(page);
    await expect(page.locator('#settingsModal .modal-title')).toHaveText('Settings');
    await expect(page.locator('#saveDataButton')).toBeVisible();
    await expect(page.locator('#loadDataButton')).toBeVisible();
    await expect(page.locator('#settingsModal .settings-sync-panel')).toHaveCount(0);
    await expect(page.locator('#settingsModal #logoutForm')).toHaveCount(0);
    await expect(page.locator('#settingsModal #syncStateStatus')).toHaveCount(0);
    await expect(page.locator('#staticSettingsNote')).toBeHidden();
  });

  test('import and export still work from Settings', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Settings Export Note');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    await openSettingsModal(page);
    const exported = await exportCurrentSnapshot(page);
    expect(exported.tabs).toBeTruthy();

    await importBackupSnapshot(page, buildLocalSnapshot('Imported From Settings'));
    await expectNoteVisible(page, 'Imported From Settings');
  });

  test('top-right controls stay compact at narrow widths', async ({ page }) => {
    await page.setViewportSize({ width: 360, height: 640 });
    await prepareBlankPage(page);
    await page.goto('/index.html');

    const controlsBox = await page.locator('#topAppControls').boundingBox();
    const addTabBox = await page.locator('#addTab').boundingBox();
    if (!controlsBox || !addTabBox) {
      throw new Error('Top control bounding boxes were not available.');
    }

    expect(controlsBox.width).toBeLessThanOrEqual(120);
    expect(controlsBox.x).toBeGreaterThan(addTabBox.x + addTabBox.width);
  });

  test('desktop tab bar ends before settings and account controls', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await prepareBlankPage(page);
    await page.goto('/index.html');

    const tabBarBox = await page.locator('#tabBar').boundingBox();
    const controlsBox = await page.locator('#topAppControls').boundingBox();
    const addTabBox = await page.locator('#addTab').boundingBox();
    if (!tabBarBox || !controlsBox || !addTabBox) {
      throw new Error('Tab bar layout bounding boxes were not available.');
    }

    expect(tabBarBox.x + tabBarBox.width).toBeLessThanOrEqual(controlsBox.x);
    expect(addTabBox.x + addTabBox.width).toBeLessThanOrEqual(controlsBox.x);
    await expect(page.locator('.tab-bar-header')).toHaveCSS('display', 'flex');
    await expect(page.locator('#topAppControls')).toHaveCSS('position', 'static');
  });
});
