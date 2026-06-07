const fs = require('fs');
const { test, expect } = require('@playwright/test');

const DB_NAME = 'postbaby-browser-storage';
const SNAPSHOTS_STORE = 'snapshots';
const META_STORE = 'meta';
const PRIMARY_RECORD_ID = 'primary';
const TIMESTAMP = '2026-05-13T12:00:00.000Z';
const TEST_BASE_URL = 'http://127.0.0.1:4173';
const EDGE_ARM_DELAY_MS = 1000;
const EDGE_ARROW_TARGET_INSET = 2;
const MIN_CANVAS_COORD = -100000;
const MAX_CANVAS_COORD = 100000;
const DEFAULT_CAMERA = { x: 0, y: 0, zoom: 1 };
const MIN_ZOOM = 0.25;
const MAX_ZOOM = 3;
const MAX_ITEMS_PER_TAB = 500;
const MAX_EDGES_PER_TAB = 2000;
const MAX_ITEM_TEXT_CHARS = 4000;
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
  'hideCameraControls',
  'corporateMode',
  'defaultColorEnabled',
  'defaultColor',
  'hasRunBefore'
];
const LOCAL_ONLY_OUTBOX_KEYS = [
  'postbabySyncClientId',
  'postbabySyncDeviceId',
  'postbabySyncMutationCounter',
  'postbabySyncMutationOutbox'
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

function buildBulkNoteItems(count, options = {}) {
  const startLeft = options.startLeft === undefined ? 24 : options.startLeft;
  const startTop = options.startTop === undefined ? 24 : options.startTop;
  const stepX = options.stepX === undefined ? 160 : options.stepX;
  const stepY = options.stepY === undefined ? 120 : options.stepY;
  const columns = options.columns === undefined ? 20 : options.columns;

  return Array.from({ length: count }, (_, index) => {
    const row = Math.floor(index / columns);
    const column = index % columns;
    return buildNoteItem(options.labelBuilder ? options.labelBuilder(index) : `Bulk Item ${index + 1}`, {
      itemId: options.idBuilder ? options.idBuilder(index) : `bulk-item-${index + 1}`,
      position: {
        top: `${startTop + (row * stepY)}px`,
        left: `${startLeft + (column * stepX)}px`
      }
    });
  });
}

function buildDenseUndirectedEdges(itemIds, count, options = {}) {
  const edges = [];
  const kind = options.kind || 'line';

  for (let leftIndex = 0; leftIndex < itemIds.length; leftIndex += 1) {
    for (let rightIndex = leftIndex + 1; rightIndex < itemIds.length; rightIndex += 1) {
      edges.push({
        id: options.idBuilder ? options.idBuilder(edges.length) : `edge-${edges.length + 1}`,
        fromItemId: itemIds[leftIndex],
        toItemId: itemIds[rightIndex],
        kind
      });
      if (edges.length === count) {
        return edges;
      }
    }
  }

  throw new Error(`Unable to build ${count} unique undirected edges from ${itemIds.length} items.`);
}

function findMissingUndirectedEdgePair(itemIds, edges) {
  const edgeKeys = new Set((edges || []).map((edge) => {
    return [edge.fromItemId, edge.toItemId].sort().join('::');
  }));

  for (let leftIndex = 0; leftIndex < itemIds.length; leftIndex += 1) {
    for (let rightIndex = leftIndex + 1; rightIndex < itemIds.length; rightIndex += 1) {
      const key = [itemIds[leftIndex], itemIds[rightIndex]].sort().join('::');
      if (!edgeKeys.has(key)) {
        return [itemIds[leftIndex], itemIds[rightIndex]];
      }
    }
  }

  throw new Error('Unable to find an unconnected item pair.');
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

function buildServerSnapshotPayload(snapshot, version, updatedAt = TIMESTAMP) {
  return {
    version,
    updatedAt,
    data: snapshot
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

async function switchSettingsTab(page, tabName) {
  const tab = page.getByRole('tab', { name: tabName });
  await tab.click();
  await expect(tab).toHaveAttribute('aria-selected', 'true');
}

async function openSettingsImportExportTab(page) {
  await openSettingsModal(page);
  await switchSettingsTab(page, 'Import & Export');
  await expect(page.locator('#settingsImportExportPanel')).toBeVisible();
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

async function openNewNoteEditorAt(page, point = { x: 900, y: 300 }) {
  const grid = page.locator('.grid-container').first();
  await expect(grid).toBeVisible();
  await grid.evaluate((element, clickPoint) => {
    element.dispatchEvent(new MouseEvent('contextmenu', {
      bubbles: true,
      cancelable: true,
      button: 2,
      clientX: clickPoint.x,
      clientY: clickPoint.y
    }));
  }, point);

  const textarea = page.locator('textarea.edit-textarea');
  await expect(textarea).toBeVisible();
  return textarea;
}

async function createNoteAt(page, noteText, point = { x: 900, y: 300 }) {
  const textarea = await openNewNoteEditorAt(page, point);
  await textarea.fill(noteText);
  await textarea.press('Escape');
  await expect(textarea).toHaveCount(0);
  await expectNoteVisible(page, noteText);
}

async function openNoteDeleteConfirm(page, itemId) {
  const note = page.locator(`.grid-item[data-id="${itemId}"]`);
  await note.evaluate((element) => {
    element.dispatchEvent(new MouseEvent('contextmenu', {
      bubbles: true,
      cancelable: true,
      button: 2,
      buttons: 2
    }));
  });
  await expect(page.locator('#confirmModal')).toBeVisible();
}

async function deleteNoteViaContextMenu(page, itemId) {
  await openNoteDeleteConfirm(page, itemId);
  await page.click('#confirmDelete');
  await expect(page.locator(`.grid-item[data-id="${itemId}"]`)).toHaveCount(0);
}

async function readTabsSnapshot(page) {
  let scopedRecordId = PRIMARY_RECORD_ID;
  try {
    const debugState = await page.evaluate(() => (
      typeof window.postbabyDebugSync === 'function'
        ? window.postbabyDebugSync()
        : null
    ));
    if (debugState && typeof debugState.storageScopeKey === 'string' && debugState.storageScopeKey.trim()) {
      scopedRecordId = debugState.storageScopeKey.trim();
    }
  } catch (error) {
  }

  let tabsPayload = null;
  for (let attempt = 0; attempt < 40; attempt += 1) {
    const indexedDBState = await readIndexedDBState(page, scopedRecordId);
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

async function readItemPositionViaGeometryDom(page, itemId = 'item-1') {
  const item = await readItemSnapshot(page, itemId);
  if (!item) {
    return null;
  }

  return page.evaluate((snapshotItem) => {
    return window.PostbabyGeometryDom.getItemPositionXY(snapshotItem);
  }, item);
}

async function readItemPositionsById(page, itemIds) {
  const entries = await Promise.all(itemIds.map(async (itemId) => {
    const item = await readItemSnapshot(page, itemId);
    return [itemId, item ? item.position : null];
  }));
  return Object.fromEntries(entries);
}

async function readWindowScroll(page) {
  return page.evaluate(() => ({
    x: window.scrollX,
    y: window.scrollY,
    documentX: document.scrollingElement ? document.scrollingElement.scrollLeft : window.scrollX,
    documentY: document.scrollingElement ? document.scrollingElement.scrollTop : window.scrollY,
    width: window.innerWidth,
    height: window.innerHeight
  }));
}

async function readWorkspaceScroll(page) {
  return page.evaluate(() => {
    const workspace = document.getElementById('tabContent');
    const cameraHook = window.postbabyGetCameraForTest;
    if (!workspace) {
      return {
        x: 0,
        y: 0,
        width: window.innerWidth,
        height: window.innerHeight,
        scrollWidth: window.innerWidth,
        scrollHeight: window.innerHeight
      };
    }

    if (typeof cameraHook === 'function') {
      const camera = cameraHook();
      return {
        x: camera.x,
        y: camera.y,
        width: workspace.clientWidth,
        height: workspace.clientHeight,
        scrollWidth: workspace.clientWidth,
        scrollHeight: workspace.clientHeight
      };
    }

    return {
      x: workspace.scrollLeft,
      y: workspace.scrollTop,
      width: workspace.clientWidth,
      height: workspace.clientHeight,
      scrollWidth: workspace.scrollWidth,
      scrollHeight: workspace.scrollHeight
    };
  });
}

async function readCamera(page, tabId = null) {
  return page.evaluate((resolvedTabId) => {
    if (typeof window.postbabyGetCameraForTest !== 'function') {
      throw new Error('postbabyGetCameraForTest is not available.');
    }
    return window.postbabyGetCameraForTest(resolvedTabId);
  }, tabId);
}

async function setCamera(page, camera, tabId = null) {
  return page.evaluate(({ nextCamera, resolvedTabId }) => {
    if (typeof window.postbabySetCameraForTest !== 'function') {
      throw new Error('postbabySetCameraForTest is not available.');
    }
    return window.postbabySetCameraForTest(nextCamera, resolvedTabId);
  }, {
    nextCamera: camera,
    resolvedTabId: tabId
  });
}

async function readCanvasMode(page) {
  return page.evaluate(() => {
    if (typeof window.postbabyGetCanvasModeForTest !== 'function') {
      throw new Error('postbabyGetCanvasModeForTest is not available.');
    }
    return window.postbabyGetCanvasModeForTest();
  });
}

async function setCanvasMode(page, mode) {
  return page.evaluate((nextMode) => {
    if (typeof window.postbabySetCanvasModeForTest !== 'function') {
      throw new Error('postbabySetCanvasModeForTest is not available.');
    }
    return window.postbabySetCanvasModeForTest(nextMode);
  }, mode);
}

async function resetCamera(page, tabId = null) {
  return page.evaluate((resolvedTabId) => {
    if (typeof window.postbabyResetCameraForTest !== 'function') {
      throw new Error('postbabyResetCameraForTest is not available.');
    }
    return window.postbabyResetCameraForTest(resolvedTabId);
  }, tabId);
}

async function centerCameraOnWorldPoint(page, x, y, tabId = null) {
  return page.evaluate(({ worldX, worldY, resolvedTabId }) => {
    if (typeof window.postbabyCenterCameraOnWorldPointForTest !== 'function') {
      throw new Error('postbabyCenterCameraOnWorldPointForTest is not available.');
    }
    return window.postbabyCenterCameraOnWorldPointForTest(worldX, worldY, resolvedTabId);
  }, {
    worldX: x,
    worldY: y,
    resolvedTabId: tabId
  });
}

async function resetWorkspaceScroll(page) {
  await page.evaluate(() => {
    if (typeof window.postbabyResetCameraForTest === 'function') {
      window.postbabyResetCameraForTest();
      return;
    }
    const workspace = document.getElementById('tabContent');
    if (workspace && typeof workspace.scrollTo === 'function') {
      workspace.scrollTo({ left: 0, top: 0, behavior: 'auto' });
      return;
    }

    window.scrollTo(0, 0);
  });
}

async function scrollWorkspaceTo(page, left, top) {
  await page.evaluate((position) => {
    if (typeof window.postbabySetCameraForTest === 'function') {
      window.postbabySetCameraForTest({
        x: position.left,
        y: position.top,
        zoom: 1
      });
      return;
    }
    const workspace = document.getElementById('tabContent');
    if (workspace && typeof workspace.scrollTo === 'function') {
      workspace.scrollTo({
        left: position.left,
        top: position.top,
        behavior: 'auto'
      });
      return;
    }

    window.scrollTo(position.left, position.top);
  }, { left, top });
}

async function readViewportRecoveryFootprint(page) {
  return page.evaluate(() => {
    const sizer = document.getElementById('viewportRecoverySizer');
    const workspace = document.getElementById('tabContent');
    return {
      sizerLeft: sizer ? parseFloat(sizer.style.left || '0') : 0,
      sizerTop: sizer ? parseFloat(sizer.style.top || '0') : 0,
      scrollWidth: workspace ? workspace.scrollWidth : window.innerWidth,
      scrollHeight: workspace ? workspace.scrollHeight : window.innerHeight,
      scrollX: workspace ? workspace.scrollLeft : window.scrollX,
      scrollY: workspace ? workspace.scrollTop : window.scrollY,
      viewportWidth: workspace ? workspace.clientWidth : window.innerWidth,
      viewportHeight: workspace ? workspace.clientHeight : window.innerHeight,
      windowScrollX: window.scrollX,
      windowScrollY: window.scrollY
    };
  });
}

async function runViewportRecoveryHook(page, hookName) {
  await page.waitForFunction((name) => typeof window[name] === 'function', hookName);
  return page.evaluate((name) => {
    const hook = window[name];
    if (typeof hook !== 'function') {
      throw new Error(`${name} is not available.`);
    }
    return hook();
  }, hookName);
}

async function showAllItemsForTest(page) {
  return runViewportRecoveryHook(page, 'postbabyShowAllItemsForTest');
}

async function jumpNewestItemForTest(page) {
  return runViewportRecoveryHook(page, 'postbabyJumpNewestItemForTest');
}

async function jumpLastEditedItemForTest(page) {
  return runViewportRecoveryHook(page, 'postbabyJumpLastEditedItemForTest');
}

async function readItemClientRect(page, itemId) {
  return page.locator(`.grid-item[data-id="${itemId}"]`).evaluate((element) => {
    const rect = element.getBoundingClientRect();
    return {
      left: rect.left,
      top: rect.top,
      right: rect.right,
      bottom: rect.bottom,
      width: rect.width,
      height: rect.height
    };
  });
}

async function dispatchWheelEvent(page, selector, options = {}) {
  return page.locator(selector).evaluate((element, wheelOptions) => {
    const rect = element.getBoundingClientRect();
    const clientX = Number.isFinite(wheelOptions.clientX)
      ? wheelOptions.clientX
      : rect.left + (rect.width / 2);
    const clientY = Number.isFinite(wheelOptions.clientY)
      ? wheelOptions.clientY
      : rect.top + (rect.height / 2);
    const wheelEvent = new WheelEvent('wheel', {
      bubbles: true,
      cancelable: wheelOptions.cancelable !== false,
      deltaX: wheelOptions.deltaX || 0,
      deltaY: wheelOptions.deltaY || 0,
      deltaMode: wheelOptions.deltaMode || 0,
      ctrlKey: wheelOptions.ctrlKey === true,
      metaKey: wheelOptions.metaKey === true,
      shiftKey: wheelOptions.shiftKey === true,
      clientX,
      clientY
    });
    const dispatchResult = element.dispatchEvent(wheelEvent);
    return {
      defaultPrevented: wheelEvent.defaultPrevented,
      canceled: dispatchResult === false
    };
  }, options);
}

async function dispatchWorkspaceWheel(page, options = {}) {
  return dispatchWheelEvent(page, '#tabContent', options);
}

async function readCameraControlsState(page) {
  return page.evaluate(() => {
    const controls = document.getElementById('cameraControls');
    const zoomOut = document.getElementById('cameraZoomOutButton');
    const zoomReset = document.getElementById('cameraZoomResetButton');
    const zoomIn = document.getElementById('cameraZoomInButton');
    const fitAll = document.getElementById('cameraFitAllButton');
    const grid = document.querySelector('.tab-pane.active .grid-container');
    return {
      hidden: !controls || controls.hidden,
      text: zoomReset ? zoomReset.textContent : '',
      zoomOutDisabled: zoomOut ? zoomOut.disabled : null,
      zoomResetDisabled: zoomReset ? zoomReset.disabled : null,
      zoomInDisabled: zoomIn ? zoomIn.disabled : null,
      fitDisabled: fitAll ? fitAll.disabled : null,
      insideGrid: Boolean(controls && grid && grid.contains(controls))
    };
  });
}

async function readCanvasModeToggleState(page) {
  return page.evaluate(() => {
    const button = document.getElementById('canvasModeToggleButton');
    const grid = document.querySelector('.tab-pane.active .grid-container');
    return {
      hidden: !button || button.hidden,
      insideGrid: Boolean(button && grid && grid.contains(button)),
      pressed: button ? button.getAttribute('aria-pressed') : null,
      label: button ? button.getAttribute('aria-label') : '',
      title: button ? button.getAttribute('title') : '',
      storageValue: window.localStorage.getItem('postbabyCanvasMode')
    };
  });
}

async function dispatchWorkspaceTouchPinch(page, options = {}) {
  await page.locator('#tabContent').evaluate(async (element, pinchOptions) => {
    const rect = element.getBoundingClientRect();
    const startPointers = [
      {
        pointerId: 1,
        x: rect.left + (pinchOptions.startLeftX || 140),
        y: rect.top + (pinchOptions.startY || 180)
      },
      {
        pointerId: 2,
        x: rect.left + (pinchOptions.startRightX || 320),
        y: rect.top + (pinchOptions.startY || 180)
      }
    ];
    const endPointers = [
      {
        pointerId: 1,
        x: rect.left + (pinchOptions.endLeftX || 100),
        y: rect.top + (pinchOptions.endY || 220)
      },
      {
        pointerId: 2,
        x: rect.left + (pinchOptions.endRightX || 400),
        y: rect.top + (pinchOptions.endY || 220)
      }
    ];

    const dispatchPointer = (target, type, pointer) => {
      target.dispatchEvent(new PointerEvent(type, {
        bubbles: true,
        cancelable: true,
        pointerId: pointer.pointerId,
        pointerType: 'touch',
        isPrimary: pointer.pointerId === 1,
        clientX: pointer.x,
        clientY: pointer.y
      }));
    };

    dispatchPointer(element, 'pointerdown', startPointers[0]);
    dispatchPointer(element, 'pointerdown', startPointers[1]);
    await new Promise((resolve) => window.requestAnimationFrame(resolve));
    dispatchPointer(document, 'pointermove', endPointers[0]);
    dispatchPointer(document, 'pointermove', endPointers[1]);
    await new Promise((resolve) => window.requestAnimationFrame(resolve));

    if (pinchOptions.cancel === true) {
      dispatchPointer(document, 'pointercancel', endPointers[1]);
      dispatchPointer(document, 'pointercancel', endPointers[0]);
      return;
    }

    dispatchPointer(document, 'pointerup', endPointers[1]);
    dispatchPointer(document, 'pointerup', endPointers[0]);
  }, options);
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

async function readMutationOutbox(page) {
  const debugState = await page.evaluate(() => window.postbabyDebugSync());
  return debugState.mutationOutbox;
}

async function waitForDeltaMetadataCheckCount(page, count) {
  await expect.poll(async () => {
    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    return debugState.deltaMetadataCheckCount || 0;
  }).toBe(count);
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
    const noteRect = element.getBoundingClientRect();
    const handleRect = handle ? handle.getBoundingClientRect() : null;
    return {
      sizeMode: element.dataset.sizeMode || '',
      usesExplicitSizeClass: element.classList.contains('grid-item--explicit-size'),
      overflow: style.overflow,
      beforeBorderRadius: beforeStyle.borderRadius,
      beforeClipPath: beforeStyle.clipPath,
      handleRight: handleStyle ? handleStyle.right : null,
      handleBottom: handleStyle ? handleStyle.bottom : null,
      handleNotchAngle: handleStyle ? handleStyle.getPropertyValue('--resize-handle-notch-angle').trim() : null,
      handleCenterXRatio: handleRect && noteRect.width > 0
        ? ((handleRect.left + (handleRect.width / 2)) - noteRect.left) / noteRect.width
        : null,
      handleCenterYRatio: handleRect && noteRect.height > 0
        ? ((handleRect.top + (handleRect.height / 2)) - noteRect.top) / noteRect.height
        : null
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
  await openEdgeDeleteConfirm(page, edgeLine);
  await page.click('#confirmDelete');
}

async function openEdgeDeleteConfirm(page, edgeLine) {
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

  await page.mouse.click(point.x, point.y);
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

function buildMockedSyncRuntimeConfigBody(runtimeAccount, runtimeOverrides = {}) {
  const bodyLines = [
    'window.POSTBABY_RUNTIME = {',
    `  deploymentMode: ${JSON.stringify(runtimeOverrides.deploymentMode || 'selfhosted')},`,
    `  authorityModel: ${JSON.stringify(runtimeOverrides.authorityModel || 'server_authoritative')},`,
    `  authAvailable: ${runtimeOverrides.authAvailable === undefined ? true : runtimeOverrides.authAvailable === true},`,
    `  syncAvailable: ${runtimeOverrides.syncAvailable === undefined ? true : runtimeOverrides.syncAvailable === true},`,
    `  authRequired: ${runtimeOverrides.authRequired === undefined ? true : runtimeOverrides.authRequired === true},`,
    `  syncRequiresAuth: ${runtimeOverrides.syncRequiresAuth === undefined ? true : runtimeOverrides.syncRequiresAuth === true},`,
    `  syncUsable: ${runtimeOverrides.syncUsable === undefined ? true : runtimeOverrides.syncUsable === true},`,
    `  syncPausedReason: ${JSON.stringify(runtimeOverrides.syncPausedReason || '')},`,
    `  entitlement: ${JSON.stringify(runtimeOverrides.entitlement || { hostedSync: false, status: 'none' })},`,
    `  apiBase: ${JSON.stringify(runtimeOverrides.apiBase || '')},`,
    `  account: ${JSON.stringify(runtimeAccount)}`,
    '};'
  ];

  if (Object.prototype.hasOwnProperty.call(runtimeOverrides, 'billingAvailable')) {
    bodyLines.splice(4, 0, `  billingAvailable: ${runtimeOverrides.billingAvailable === true},`);
  }

  if (Object.prototype.hasOwnProperty.call(runtimeOverrides, 'isAuthenticated')) {
    bodyLines.splice(4, 0, `  isAuthenticated: ${runtimeOverrides.isAuthenticated === true},`);
  }

  return bodyLines.join('\n');
}

async function enableMockedSync(page, options = {}) {
  const metaPayload = options.metaPayload || { ok: true, exists: false, version: 0, updatedAt: TIMESTAMP };
  const documentPayload = options.documentPayload || buildServerPayload('Server Snapshot', metaPayload.version || 1);
  const onSave = options.onSave || null;
  const onMutations = options.onMutations || null;
  const onDelta = options.onDelta || null;
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
      body: buildMockedSyncRuntimeConfigBody(runtimeAccount)
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

  await page.route(/\/api\/sync\/mutations(?:\?.*)?$/, async (route) => {
    const request = route.request();
    if (request.method() !== 'POST') {
      await route.fulfill({
        status: 405,
        contentType: 'application/json; charset=utf-8',
        body: JSON.stringify({ ok: false })
      });
      return;
    }

    const requestBody = JSON.parse(request.postData() || '{}');
    const response = onMutations
      ? await onMutations(requestBody, request)
      : {
          status: 404,
          body: {
            ok: false,
            error: {
              code: 'not_found',
              message: 'not found'
            }
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
  });

  await page.route(/\/api\/sync\/delta(?:\?.*)?$/, async (route) => {
    const request = route.request();
    if (request.method() !== 'GET') {
      await route.fulfill({
        status: 405,
        contentType: 'application/json; charset=utf-8',
        headers: {
          Allow: 'GET'
        },
        body: JSON.stringify({ ok: false })
      });
      return;
    }

    const response = onDelta
      ? await onDelta(request)
      : {
          status: 404,
          body: {
            ok: false,
            error: {
              code: 'not_found',
              message: 'not found'
            }
          }
        };

    if (response && response.abort) {
      await route.abort(response.abort);
      return;
    }

    await route.fulfill({
      status: response.status || 200,
      contentType: 'application/json; charset=utf-8',
      headers: {
        'Cache-Control': 'no-store'
      },
      body: JSON.stringify(response.body)
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

async function enableDynamicMockedSync(target, options = {}) {
  const runtimeAccount = Object.assign({
    username: 'owner',
    displayName: 'owner',
    email: '',
    avatarUrl: '',
    isAdmin: true,
    storageKey: 'owner-scope',
    status: 'active'
  }, options.runtimeAccount || {});
  const runtimeOverrides = options.runtimeOverrides || {};
  const getMetaPayload = typeof options.getMetaPayload === 'function'
    ? options.getMetaPayload
    : (() => ({ ok: true, exists: false, version: 0, updatedAt: TIMESTAMP }));
  const getDocumentPayload = typeof options.getDocumentPayload === 'function'
    ? options.getDocumentPayload
    : (() => buildServerPayload('Server Snapshot', 1));
  const onSave = typeof options.onSave === 'function'
    ? options.onSave
    : null;
  const onMutations = typeof options.onMutations === 'function'
    ? options.onMutations
    : null;
  const onDelta = typeof options.onDelta === 'function'
    ? options.onDelta
    : null;

  await target.route('**/runtime-config.js', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/javascript; charset=utf-8',
      headers: {
        'Cache-Control': 'no-store'
      },
      body: buildMockedSyncRuntimeConfigBody(runtimeAccount, runtimeOverrides)
    });
  });

  await target.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json; charset=utf-8',
      headers: {
        'Cache-Control': 'no-store'
      },
      body: JSON.stringify(getMetaPayload(route.request()))
    });
  });

  await target.route(/\/api\/sync\/mutations(?:\?.*)?$/, async (route) => {
    const request = route.request();
    if (request.method() !== 'POST') {
      await route.fulfill({
        status: 405,
        contentType: 'application/json; charset=utf-8',
        body: JSON.stringify({ ok: false })
      });
      return;
    }

    const requestBody = JSON.parse(request.postData() || '{}');
    const response = onMutations
      ? await onMutations(requestBody, request)
      : {
          status: 404,
          body: {
            ok: false,
            error: {
              code: 'not_found',
              message: 'not found'
            }
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
  });

  await target.route(/\/api\/sync\/delta(?:\?.*)?$/, async (route) => {
    const request = route.request();
    if (request.method() !== 'GET') {
      await route.fulfill({
        status: 405,
        contentType: 'application/json; charset=utf-8',
        headers: {
          Allow: 'GET'
        },
        body: JSON.stringify({ ok: false })
      });
      return;
    }

    const response = onDelta
      ? await onDelta(request)
      : {
          status: 404,
          body: {
            ok: false,
            error: {
              code: 'not_found',
              message: 'not found'
            }
          }
        };

    if (response && response.abort) {
      await route.abort(response.abort);
      return;
    }

    await route.fulfill({
      status: response.status || 200,
      contentType: 'application/json; charset=utf-8',
      headers: {
        'Cache-Control': 'no-store'
      },
      body: JSON.stringify(response.body)
    });
  });

  await target.route(/\/api\/document(?:\?.*)?$/, async (route) => {
    const request = route.request();
    if (request.method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json; charset=utf-8',
        headers: {
          'Cache-Control': 'no-store'
        },
        body: JSON.stringify(getDocumentPayload(request))
      });
      return;
    }

    if (request.method() === 'PUT') {
      const requestBody = JSON.parse(request.postData() || '{}');
      const response = onSave
        ? await onSave(requestBody, request)
        : {
            status: 200,
            body: {
              ok: true,
              version: 1,
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
        expect(presentation.handleCenterXRatio).toBeGreaterThan(0.62);
        expect(presentation.handleCenterXRatio).toBeLessThan(0.72);
        expect(presentation.handleCenterYRatio).toBeGreaterThan(0.78);
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

      if (shape === 'upsideDownTriangle') {
        expect(presentation.handleNotchAngle).toBe('117deg');
        expect(presentation.handleCenterXRatio).toBeGreaterThan(0.54);
        expect(presentation.handleCenterXRatio).toBeLessThan(0.62);
        expect(presentation.handleCenterYRatio).toBeGreaterThan(0.79);
      }

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

  test('geometry-dom position helpers preserve the top/left compatibility format', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildLocalSnapshot('Helper Seed', {
      width: 220,
      height: 120,
      position: { top: '20px', left: '10px' }
    }));
    await page.goto('/index.html');

    const helperResult = await page.evaluate((limits) => {
      const geometryDom = window.PostbabyGeometryDom;
      const canvasLimits = window.PostbabyCanvasLimits;
      const mutableItem = {
        shape: 'default',
        width: 220,
        height: 120,
        position: { top: '20px', left: '10px' }
      };

      return {
        parsePositive: geometryDom.parseCssPixelValue('123px', 0),
        parseNegative: geometryDom.parseCssPixelValue('-50px', 0),
        parseFallback: geometryDom.parseCssPixelValue('bad', 7),
        formattedPixel: geometryDom.formatCssPixelValue(123),
        formattedPosition: geometryDom.formatItemPosition(10, 20),
        clampHigh: canvasLimits.clampCanvasCoord(limits.max + 1),
        clampLow: canvasLimits.clampCanvasCoord(limits.min - 1),
        clampPassThrough: canvasLimits.clampCanvasCoord(123),
        parsedPosition: geometryDom.getItemPositionXY(mutableItem),
        updatedPosition: geometryDom.setItemPositionXY(mutableItem, 30, 40),
        clampedPosition: geometryDom.setItemPositionXY(mutableItem, limits.max + 2000, limits.min - 2000),
        mutablePosition: mutableItem.position,
        rect: geometryDom.getItemRectFromData(mutableItem),
        center: geometryDom.getItemCenterFromData(mutableItem)
      };
    }, {
      min: MIN_CANVAS_COORD,
      max: MAX_CANVAS_COORD
    });

    expect(helperResult.parsePositive).toBe(123);
    expect(helperResult.parseNegative).toBe(-50);
    expect(helperResult.parseFallback).toBe(7);
    expect(helperResult.formattedPixel).toBe('123px');
    expect(helperResult.formattedPosition).toEqual({ top: '20px', left: '10px' });
    expect(helperResult.clampHigh).toBe(MAX_CANVAS_COORD);
    expect(helperResult.clampLow).toBe(MIN_CANVAS_COORD);
    expect(helperResult.clampPassThrough).toBe(123);
    expect(helperResult.parsedPosition).toEqual({ x: 10, y: 20 });
    expect(helperResult.updatedPosition).toEqual({ top: '40px', left: '30px' });
    expect(helperResult.clampedPosition).toEqual({
      top: `${MIN_CANVAS_COORD}px`,
      left: `${MAX_CANVAS_COORD}px`
    });
    expect(helperResult.mutablePosition).toEqual({
      top: `${MIN_CANVAS_COORD}px`,
      left: `${MAX_CANVAS_COORD}px`
    });
    expect(helperResult.rect).toMatchObject({
      x: MAX_CANVAS_COORD,
      y: MIN_CANVAS_COORD,
      left: MAX_CANVAS_COORD,
      top: MIN_CANVAS_COORD,
      width: 220,
      height: 120,
      right: MAX_CANVAS_COORD + 220,
      bottom: MIN_CANVAS_COORD + 120
    });
    expect(helperResult.center).toEqual({
      x: MAX_CANVAS_COORD + 110,
      y: MIN_CANVAS_COORD + 60,
      cx: MAX_CANVAS_COORD + 110,
      cy: MIN_CANVAS_COORD + 60
    });

    const storedPosition = await readItemPositionViaGeometryDom(page);
    expect(storedPosition).toEqual({ x: 10, y: 20 });
  });

  test('canvas-camera helpers round-trip world and screen coordinates and keep zoom-at-point stable', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const helperResult = await page.evaluate(() => {
      const camera = window.PostbabyCanvasCamera;
      const viewport = document.getElementById('tabContent');
      const initialCamera = { x: 120, y: 80, zoom: 1.5 };
      const viewportPoint = camera.worldPointToViewportPoint(220, 140, initialCamera);
      const roundTrip = camera.viewportPointToWorldPoint(viewportPoint.x, viewportPoint.y, initialCamera);
      const viewportRect = viewport.getBoundingClientRect();
      const clientX = viewportRect.left + 180;
      const clientY = viewportRect.top + 140;
      const pointedWorldBefore = camera.clientPointToWorldPoint(clientX, clientY, viewport, initialCamera);
      const zoomedCamera = camera.zoomCameraAtClientPoint(initialCamera, clientX, clientY, 2, viewport);
      const pointedWorldAfter = camera.clientPointToWorldPoint(clientX, clientY, viewport, zoomedCamera);
      const centeredCamera = camera.centerCameraOnWorldPoint(initialCamera, viewport, 400, 300);
      const fitCamera = camera.fitCameraToWorldRect({
        left: 200,
        top: 150,
        right: 600,
        bottom: 450
      }, viewport);

      return {
        viewportPoint,
        roundTrip,
        worldDelta: camera.screenDeltaToWorldDelta(90, 45, { x: 0, y: 0, zoom: 2 }),
        pointedWorldBefore,
        pointedWorldAfter,
        centeredCamera,
        fitCamera,
        normalized: camera.normalizeCameraState({ x: 'bad', y: 20, zoom: 99 }),
        clampedZoomLow: camera.clampZoom(0.1),
        clampedZoomHigh: camera.clampZoom(9)
      };
    });

    expect(helperResult.viewportPoint).toEqual({ x: 150, y: 90 });
    expect(helperResult.roundTrip.x).toBeCloseTo(220, 4);
    expect(helperResult.roundTrip.y).toBeCloseTo(140, 4);
    expect(helperResult.worldDelta.x).toBeCloseTo(45, 4);
    expect(helperResult.worldDelta.y).toBeCloseTo(22.5, 4);
    expect(helperResult.pointedWorldAfter.x).toBeCloseTo(helperResult.pointedWorldBefore.x, 4);
    expect(helperResult.pointedWorldAfter.y).toBeCloseTo(helperResult.pointedWorldBefore.y, 4);
    expect(helperResult.centeredCamera.zoom).toBeCloseTo(1.5, 4);
    expect(helperResult.fitCamera.zoom).toBeGreaterThanOrEqual(MIN_ZOOM);
    expect(helperResult.fitCamera.zoom).toBeLessThanOrEqual(MAX_ZOOM);
    expect(helperResult.normalized).toEqual({ x: 0, y: 20, zoom: MAX_ZOOM });
    expect(helperResult.clampedZoomLow).toBe(MIN_ZOOM);
    expect(helperResult.clampedZoomHigh).toBe(MAX_ZOOM);
  });

  test('camera state stays per-tab and memory-only while canvas mode persists browser-locally across reloads', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    expect(await readCanvasMode(page)).toBe('select');
    expect(await readCamera(page)).toEqual(DEFAULT_CAMERA);
    await setCamera(page, { x: 420, y: 360, zoom: 1.75 });
    await setCanvasMode(page, 'pan');
    expect(await readCamera(page)).toEqual({ x: 420, y: 360, zoom: 1.75 });
    expect(await readCanvasMode(page)).toBe('pan');

    await page.click('#addTab');
    const newestTabId = await page.locator('#tabBar .tab[data-tab-id]').last().getAttribute('data-tab-id');
    if (!newestTabId) {
      throw new Error('New tab id was not available.');
    }

    expect(await readCamera(page, newestTabId)).toEqual(DEFAULT_CAMERA);
    await page.locator('.tab[data-tab-id="tab-1"]').click();
    expect(await readCamera(page, 'tab-1')).toEqual({ x: 420, y: 360, zoom: 1.75 });

    const tabsSnapshot = await readTabsSnapshot(page);
    expect(JSON.stringify(tabsSnapshot)).not.toContain('"camera"');
    expect(JSON.stringify(tabsSnapshot)).not.toContain('"viewport"');
    expect(JSON.stringify(tabsSnapshot)).not.toContain('"pan"');
    expect(JSON.stringify(tabsSnapshot)).not.toContain('"zoom"');
    expect(JSON.stringify(tabsSnapshot)).not.toContain('"canvasMode"');
    expect(await page.evaluate(() => window.localStorage.getItem('postbabyCanvasMode'))).toBe('pan');

    await page.reload();
    expect(await readCamera(page)).toEqual(DEFAULT_CAMERA);
    expect(await readCanvasMode(page)).toBe('pan');
  });

  test('space-drag pan and Shift+wheel horizontal pan update camera without mutating stored note positions', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Camera Pan Note', {
        itemId: 'camera-pan-note',
        position: { top: '640px', left: '720px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const beforePositions = await readItemPositionsById(page, ['camera-pan-note']);
    const viewportBox = await page.locator('#tabContent').boundingBox();
    if (!viewportBox) {
      throw new Error('Camera viewport bounding box was not available.');
    }

    const startX = Math.round(viewportBox.x + (viewportBox.width / 2));
    const startY = Math.round(viewportBox.y + (viewportBox.height / 2));
    await page.keyboard.down(' ');
    await page.mouse.move(startX, startY);
    await page.mouse.down();
    await page.mouse.move(startX + 120, startY + 80, { steps: 12 });
    await page.mouse.up();
    await page.keyboard.up(' ');

    const afterPanCamera = await readCamera(page);
    expect(afterPanCamera.x).toBeLessThan(0);
    expect(afterPanCamera.y).toBeLessThan(0);
    expect(await readItemPositionsById(page, ['camera-pan-note'])).toEqual(beforePositions);

    const shiftPanResult = await dispatchWorkspaceWheel(page, {
      deltaY: 140,
      shiftKey: true
    });
    expect(shiftPanResult.defaultPrevented).toBe(true);
    expect(shiftPanResult.canceled).toBe(true);

    const afterShiftPanCamera = await readCamera(page);
    expect(afterShiftPanCamera.x).toBeGreaterThan(afterPanCamera.x);
    expect(afterShiftPanCamera.y).toBeCloseTo(afterPanCamera.y, 3);
    expect(await readItemPositionsById(page, ['camera-pan-note'])).toEqual(beforePositions);
  });

  test('ordinary wheel, tilt-style deltaX, and Shift+wheel horizontal pan update only camera state', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Wheel Pan Note', {
        itemId: 'wheel-pan-note',
        position: { top: '860px', left: '940px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const beforePositions = await readItemPositionsById(page, ['wheel-pan-note']);
    const verticalPanResult = await dispatchWorkspaceWheel(page, { deltaY: 180 });
    expect(verticalPanResult.defaultPrevented).toBe(true);
    expect(verticalPanResult.canceled).toBe(true);
    const afterVerticalPan = await readCamera(page);
    expect(afterVerticalPan.y).toBeGreaterThan(0);
    expect(afterVerticalPan.zoom).toBe(1);

    const trackpadPanResult = await dispatchWorkspaceWheel(page, { deltaX: 120, deltaY: 80 });
    expect(trackpadPanResult.defaultPrevented).toBe(true);
    expect(trackpadPanResult.canceled).toBe(true);
    const afterTrackpadPan = await readCamera(page);
    expect(afterTrackpadPan.x).toBeGreaterThan(afterVerticalPan.x);
    expect(afterTrackpadPan.y).toBeGreaterThan(afterVerticalPan.y);

    const shiftHorizontalPanResult = await dispatchWorkspaceWheel(page, {
      deltaY: 160,
      shiftKey: true
    });
    expect(shiftHorizontalPanResult.defaultPrevented).toBe(true);
    expect(shiftHorizontalPanResult.canceled).toBe(true);
    const afterShiftHorizontalPan = await readCamera(page);
    expect(afterShiftHorizontalPan.x).toBeGreaterThan(afterTrackpadPan.x);
    expect(afterShiftHorizontalPan.y).toBeCloseTo(afterTrackpadPan.y, 3);
    expect(await readItemPositionsById(page, ['wheel-pan-note'])).toEqual(beforePositions);
  });

  test('wheel over edit textareas, settings modals, and top-right chrome does not pan or zoom the camera', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Protected Wheel Note', {
        itemId: 'protected-wheel-note',
        position: { top: '240px', left: '260px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    await page.locator('.grid-item[data-id="protected-wheel-note"]').dblclick();
    await expect(page.locator('textarea.edit-textarea')).toBeVisible();
    const beforeTextareaWheelCamera = await readCamera(page);
    await dispatchWheelEvent(page, 'textarea.edit-textarea', { deltaY: 240 });
    expect(await readCamera(page)).toEqual(beforeTextareaWheelCamera);
    await page.locator('textarea.edit-textarea').press('Escape');

    await openSettingsModal(page);
    const beforeModalWheelCamera = await readCamera(page);
    await dispatchWheelEvent(page, '#settingsModal', { deltaY: 240 });
    expect(await readCamera(page)).toEqual(beforeModalWheelCamera);

    const beforeChromeWheelCamera = await readCamera(page);
    await dispatchWheelEvent(page, '#canvasModeToggleButton', { deltaY: 240, shiftKey: true });
    expect(await readCamera(page)).toEqual(beforeChromeWheelCamera);
  });

  test('Ctrl-wheel zoom, Ctrl+Shift-wheel precedence, and visible camera controls update only camera state', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Control Fit Note', {
        itemId: 'control-fit-note',
        position: { top: '2280px', left: '2440px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const beforePositions = await readItemPositionsById(page, ['control-fit-note']);
    const workspaceBox = await page.locator('#tabContent').boundingBox();
    if (!workspaceBox) {
      throw new Error('Workspace viewport bounding box was not available.');
    }

    const anchorPoint = {
      clientX: Math.round(workspaceBox.x + (workspaceBox.width * 0.65)),
      clientY: Math.round(workspaceBox.y + (workspaceBox.height * 0.35))
    };

    const worldBeforeZoom = await page.evaluate((point) => {
      const viewport = document.getElementById('tabContent');
      return window.PostbabyCanvasCamera.clientPointToWorldPoint(
        point.clientX,
        point.clientY,
        viewport,
        window.postbabyGetCameraForTest()
      );
    }, anchorPoint);

    const ctrlZoomResult = await dispatchWorkspaceWheel(page, {
      deltaY: -180,
      ctrlKey: true,
      clientX: anchorPoint.clientX,
      clientY: anchorPoint.clientY
    });
    expect(ctrlZoomResult.defaultPrevented).toBe(true);
    expect(ctrlZoomResult.canceled).toBe(true);
    const afterCtrlZoomCamera = await readCamera(page);
    expect(afterCtrlZoomCamera.zoom).toBeGreaterThan(1);

    const worldAfterCtrlZoom = await page.evaluate((point) => {
      const viewport = document.getElementById('tabContent');
      return window.PostbabyCanvasCamera.clientPointToWorldPoint(
        point.clientX,
        point.clientY,
        viewport,
        window.postbabyGetCameraForTest()
      );
    }, anchorPoint);
    expect(worldAfterCtrlZoom.x).toBeCloseTo(worldBeforeZoom.x, 1);
    expect(worldAfterCtrlZoom.y).toBeCloseTo(worldBeforeZoom.y, 1);

    const ctrlShiftZoomResult = await dispatchWorkspaceWheel(page, {
      deltaY: -120,
      ctrlKey: true,
      shiftKey: true,
      clientX: anchorPoint.clientX,
      clientY: anchorPoint.clientY
    });
    expect(ctrlShiftZoomResult.defaultPrevented).toBe(true);
    expect(ctrlShiftZoomResult.canceled).toBe(true);
    const afterCtrlShiftZoomCamera = await readCamera(page);
    expect(afterCtrlShiftZoomCamera.zoom).toBeGreaterThan(afterCtrlZoomCamera.zoom);

    const controlsBeforeFit = await readCameraControlsState(page);
    expect(controlsBeforeFit.hidden).toBe(false);
    expect(controlsBeforeFit.insideGrid).toBe(false);
    expect(controlsBeforeFit.text).not.toBe('100%');
    const cameraControlsBox = await page.locator('#cameraControls').boundingBox();
    const viewportSize = page.viewportSize();
    if (!cameraControlsBox || !viewportSize) {
      throw new Error('Camera controls bounding box or viewport size was not available.');
    }
    expect(Math.abs((cameraControlsBox.x + (cameraControlsBox.width / 2)) - (viewportSize.width / 2))).toBeLessThanOrEqual(4);

    await page.click('#cameraFitAllButton');
    const afterFitCamera = await readCamera(page);
    const fittedNoteRect = await readItemClientRect(page, 'control-fit-note');
    expect(fittedNoteRect.left).toBeLessThan(workspaceBox.x + workspaceBox.width);
    expect(fittedNoteRect.top).toBeLessThan(workspaceBox.y + workspaceBox.height);
    expect(fittedNoteRect.right).toBeGreaterThan(workspaceBox.x);
    expect(fittedNoteRect.bottom).toBeGreaterThan(workspaceBox.y);

    await page.click('#cameraZoomOutButton');
    const afterControlZoomOut = await readCamera(page);
    expect(afterControlZoomOut.zoom).toBeLessThan(afterFitCamera.zoom);

    await page.click('#cameraZoomResetButton');
    expect(await readCamera(page)).toEqual(DEFAULT_CAMERA);
    const controlsAfterReset = await readCameraControlsState(page);
    expect(controlsAfterReset.text).toBe('100%');
    expect(await readItemPositionsById(page, ['control-fit-note'])).toEqual(beforePositions);
  });

  test('0 and f camera keyboard shortcuts work even when top-right chrome has focus', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Keyboard Fit Note', {
        itemId: 'keyboard-fit-note',
        position: { top: '2280px', left: '2440px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    await setCamera(page, { x: 900, y: 800, zoom: 1.5 });
    await page.locator('#settingsButton').focus();
    await page.keyboard.press('0');
    expect(await readCamera(page)).toEqual(DEFAULT_CAMERA);

    await setCamera(page, { x: 1200, y: 1000, zoom: 2 });
    await page.locator('#canvasModeToggleButton').focus();
    await page.keyboard.press('f');
    const afterFitCamera = await readCamera(page);
    expect(afterFitCamera).not.toEqual({ x: 1200, y: 1000, zoom: 2 });
  });

  test('grid underlay and labels render below notes in the stacking order', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Grid Stack Note', {
      position: { top: '180px', left: '220px' }
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    await openSettingsModal(page);
    await page.locator('#useGrids').selectOption('kanban');
    await page.locator('.close-settings').click();
    await expect(page.locator('#settingsModal')).toBeHidden();

    const stacking = await page.evaluate(() => {
      const tabContent = document.getElementById('tabContent');
      const gridUnderlay = document.getElementById('gridUnderlay');
      const note = document.querySelector('.grid-item[data-id="item-1"]');
      if (!tabContent || !gridUnderlay || !note) {
        throw new Error('Expected tab content, grid underlay, and note to exist.');
      }

      return {
        tabContentZ: window.getComputedStyle(tabContent).zIndex,
        gridUnderlayZ: window.getComputedStyle(gridUnderlay).zIndex,
        noteZ: window.getComputedStyle(note).zIndex
      };
    });

    expect(stacking.tabContentZ).toBe('1');
    expect(stacking.gridUnderlayZ).toBe('0');
    expect(stacking.noteZ).toBe('2');
  });

  test('top-right hand toggle persists browser-locally and empty-canvas drag pans only in hand mode', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Mode Toggle Note', {
        itemId: 'mode-toggle-note',
        position: { top: '360px', left: '520px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const initialToggleState = await readCanvasModeToggleState(page);
    expect(initialToggleState.hidden).toBe(false);
    expect(initialToggleState.insideGrid).toBe(false);
    expect(initialToggleState.pressed).toBe('false');
    expect(initialToggleState.label).toContain('Canvas mode: Select');
    expect(initialToggleState.storageValue).toBe('select');

    const workspaceBox = await page.locator('#tabContent').boundingBox();
    if (!workspaceBox) {
      throw new Error('Workspace viewport bounding box was not available.');
    }

    const blankStartX = Math.round(workspaceBox.x + workspaceBox.width - 220);
    const blankStartY = Math.round(workspaceBox.y + workspaceBox.height - 180);
    await page.mouse.move(blankStartX, blankStartY);
    await page.mouse.down();
    await page.mouse.move(blankStartX + 80, blankStartY + 60, { steps: 6 });
    await expect(page.locator('.grid-selection-marquee')).toHaveCount(1);
    await page.mouse.up();
    await expect(page.locator('.grid-selection-marquee')).toHaveCount(0);

    const beforePanModeCamera = await readCamera(page);
    await page.click('#canvasModeToggleButton');
    const panToggleState = await readCanvasModeToggleState(page);
    expect(panToggleState.pressed).toBe('true');
    expect(panToggleState.label).toContain('Canvas mode: Hand pan');
    expect(panToggleState.storageValue).toBe('pan');
    expect(await readCanvasMode(page)).toBe('pan');

    await page.mouse.move(blankStartX, blankStartY);
    await page.mouse.down();
    await page.mouse.move(blankStartX + 120, blankStartY + 70, { steps: 8 });
    await expect(page.locator('.grid-selection-marquee')).toHaveCount(0);
    await page.mouse.up();
    const afterPanModeCamera = await readCamera(page);
    expect(afterPanModeCamera.x).toBeLessThan(beforePanModeCamera.x);
    expect(afterPanModeCamera.y).toBeLessThan(beforePanModeCamera.y);

    const noteBeforeDrag = await readItemPositionViaGeometryDom(page, 'mode-toggle-note');
    await dragNoteBy(page, page.locator('.grid-item[data-id="mode-toggle-note"]'), 90, 40);
    await page.waitForTimeout(250);
    const noteAfterDrag = await readItemPositionViaGeometryDom(page, 'mode-toggle-note');
    expect(noteAfterDrag.x).toBe(noteBeforeDrag.x + 90);
    expect(noteAfterDrag.y).toBe(noteBeforeDrag.y + 40);

    await page.reload();
    expect(await readCamera(page)).toEqual(DEFAULT_CAMERA);
    expect(await readCanvasMode(page)).toBe('pan');
    const reloadedToggleState = await readCanvasModeToggleState(page);
    expect(reloadedToggleState.pressed).toBe('true');

    await page.click('#canvasModeToggleButton');
    expect(await readCanvasMode(page)).toBe('select');
    expect(await page.evaluate(() => window.localStorage.getItem('postbabyCanvasMode'))).toBe('select');
  });

  test('two-pointer touch pinch pans and zooms the camera without mutating item positions and cleans up on cancel', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Pinch Note', {
        itemId: 'pinch-note',
        position: { top: '680px', left: '760px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const beforePositions = await readItemPositionsById(page, ['pinch-note']);
    const beforeCamera = await readCamera(page);
    await dispatchWorkspaceTouchPinch(page, {
      startLeftX: 160,
      startRightX: 320,
      startY: 180,
      endLeftX: 120,
      endRightX: 420,
      endY: 240
    });
    const afterPinchCamera = await readCamera(page);
    expect(afterPinchCamera.zoom).toBeGreaterThan(beforeCamera.zoom);
    expect(afterPinchCamera.x).not.toBe(beforeCamera.x);
    expect(afterPinchCamera.y).not.toBe(beforeCamera.y);
    expect(await readItemPositionsById(page, ['pinch-note'])).toEqual(beforePositions);
    expect(await page.locator('#tabContent').evaluate((element) => element.classList.contains('camera-viewport--panning'))).toBe(false);

    await dispatchWorkspaceTouchPinch(page, {
      startLeftX: 180,
      startRightX: 300,
      startY: 220,
      endLeftX: 140,
      endRightX: 340,
      endY: 250,
      cancel: true
    });
    expect(await page.locator('#tabContent').evaluate((element) => element.classList.contains('camera-viewport--panning'))).toBe(false);
    expect(await readItemPositionsById(page, ['pinch-note'])).toEqual(beforePositions);
  });

  test('render, shape refresh, and edge rerender do not mutate persisted item positions', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Invariant Source', {
        itemId: 'item-1',
        width: 220,
        height: 120,
        position: { top: '100px', left: '140px' }
      }),
      buildNoteItem('Invariant Target', {
        itemId: 'item-2',
        shape: 'circle',
        width: 220,
        position: { top: '150px', left: '430px' }
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

    const trackedIds = ['item-1', 'item-2'];
    const beforePositions = await readItemPositionsById(page, trackedIds);

    await page.reload();
    expect(await readItemPositionsById(page, trackedIds)).toEqual(beforePositions);

    const sourceNote = page.locator('.grid-item[data-id="item-1"]');
    await sourceNote.click();
    await page.keyboard.press('c');
    await page.waitForTimeout(350);
    expect(await readItemPositionsById(page, trackedIds)).toEqual(beforePositions);

    const targetNote = page.locator('.grid-item[data-id="item-2"]');
    await resizeNoteBy(page, targetNote, 40, 20);
    await page.waitForTimeout(350);
    expect(await readItemPositionsById(page, trackedIds)).toEqual(beforePositions);
  });

  test('legacy out-of-bounds positions stay unchanged across reload, rerender, edge rerender, and recovery until a new write occurs', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Legacy Far Source', {
        itemId: 'far-item',
        width: 220,
        height: 120,
        position: {
          top: '220px',
          left: `${MAX_CANVAS_COORD + 500}px`
        }
      }),
      buildNoteItem('Legacy Near Target', {
        itemId: 'near-item',
        shape: 'circle',
        width: 220,
        position: { top: '180px', left: '380px' }
      })
    ], {
      edges: [{
        id: 'edge-1',
        fromItemId: 'far-item',
        toItemId: 'near-item',
        kind: 'arrow'
      }]
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const beforePositions = await readItemPositionsById(page, ['far-item', 'near-item']);
    expect(beforePositions['far-item']).toEqual({
      top: '220px',
      left: `${MAX_CANVAS_COORD + 500}px`
    });

    await page.reload();
    expect(await readItemPositionsById(page, ['far-item', 'near-item'])).toEqual(beforePositions);

    const nearNote = page.locator('.grid-item[data-id="near-item"]');
    await nearNote.click();
    await page.waitForTimeout(350);
    expect(await readItemPositionsById(page, ['far-item', 'near-item'])).toEqual(beforePositions);

    await resizeNoteBy(page, nearNote, 40, 20);
    await page.waitForTimeout(350);
    expect((await readItemPositionsById(page, ['far-item']))['far-item']).toEqual(beforePositions['far-item']);

    await showAllItemsForTest(page);
    expect((await readItemPositionsById(page, ['far-item']))['far-item']).toEqual(beforePositions['far-item']);
  });

  test('manual note creation clamps pointer-based positions within bounds without introducing numeric x/y fields', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    await page.locator('.grid-container').first().evaluate((element, point) => {
      element.dispatchEvent(new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        button: 2,
        clientX: point.clientX,
        clientY: point.clientY
      }));
    }, {
      clientX: MAX_CANVAS_COORD + 5000,
      clientY: MAX_CANVAS_COORD + 7000
    });

    const textarea = page.locator('textarea.edit-textarea');
    await expect(textarea).toBeVisible();
    await textarea.fill('Bounded Note');
    await textarea.press('Escape');
    await expectNoteVisible(page, 'Bounded Note');

    const tabsSnapshot = await readTabsSnapshot(page);
    expect(tabsSnapshot[0].items).toHaveLength(1);
    const item = tabsSnapshot[0].items[0];
    expect(item.position).toEqual({
      top: `${MAX_CANVAS_COORD}px`,
      left: `${MAX_CANVAS_COORD}px`
    });
    expect(item).not.toHaveProperty('x');
    expect(item).not.toHaveProperty('y');
  });

  test('manual note creation refuses once the tab reaches the note limit', async ({ page }) => {
    const items = buildBulkNoteItems(MAX_ITEMS_PER_TAB);

    await prepareBlankPage(page);
    await seedLocalStorage(page, buildLocalSnapshotWithItems(items));
    await page.goto('/index.html');

    await page.locator('.grid-container').first().evaluate((element) => {
      element.dispatchEvent(new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        button: 2,
        clientX: 220,
        clientY: 240
      }));
    });

    await expect(page.locator('.toast').last()).toContainText('This tab has reached the note limit.');
    await expect(page.locator('textarea.edit-textarea')).toHaveCount(0);
    const tabsSnapshot = await readTabsSnapshot(page);
    expect(tabsSnapshot[0].items).toHaveLength(MAX_ITEMS_PER_TAB);
  });

  test('manual edge creation refuses once the tab reaches the connection limit', async ({ page }) => {
    const items = buildBulkNoteItems(65, { columns: 8 });
    const itemIds = items.map((item) => item.id);
    const edges = buildDenseUndirectedEdges(itemIds, MAX_EDGES_PER_TAB, { kind: 'line' });
    const [sourceId, targetId] = findMissingUndirectedEdgePair(itemIds, edges);

    await prepareBlankPage(page);
    await seedLocalStorage(page, buildLocalSnapshotWithItems(items, { edges }));
    await page.goto('/index.html');

    const sourceNote = page.locator(`.grid-item[data-id="${sourceId}"]`);
    const targetNote = page.locator(`.grid-item[data-id="${targetId}"]`);
    await drawEdgeBetweenNotes(page, sourceNote, targetNote, { modifier: 'Shift' });

    await expect(page.locator('.toast').last()).toContainText('This tab has reached the connection limit.');
    const tabsSnapshot = await readTabsSnapshot(page);
    expect(tabsSnapshot[0].edges).toHaveLength(MAX_EDGES_PER_TAB);
  });

  test('duplicate and self-edge attempts do not show the connection-limit toast or create partial edges', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Edge Source', {
        itemId: 'item-1',
        position: { top: '80px', left: '80px' }
      }),
      buildNoteItem('Edge Target', {
        itemId: 'item-2',
        position: { top: '220px', left: '360px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const sourceNote = page.locator('.grid-item[data-id="item-1"]');
    const targetNote = page.locator('.grid-item[data-id="item-2"]');

    await drawEdgeBetweenNotes(page, sourceNote, targetNote, { modifier: 'Shift' });
    await expect(page.locator('.edge-line')).toHaveCount(1);

    await drawEdgeBetweenNotes(page, sourceNote, targetNote, { modifier: 'Control' });
    await expect(page.locator('.edge-line')).toHaveCount(1);
    await expect(page.locator('.toast').filter({ hasText: 'This tab has reached the connection limit.' })).toHaveCount(0);

    await drawEdgeBetweenNotes(page, sourceNote, sourceNote, { modifier: 'Shift' });
    await expect(page.locator('.edge-line')).toHaveCount(1);
    await expect(page.locator('.toast').filter({ hasText: 'This tab has reached the connection limit.' })).toHaveCount(0);

    const tabsSnapshot = await readTabsSnapshot(page);
    expect(tabsSnapshot[0].edges).toHaveLength(1);
  });

  test('existing saved out-of-bounds items render unchanged until drag commit clamps them back into bounds', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Out Of Bounds Seed', {
      itemId: 'far-item',
      position: {
        top: `${MAX_CANVAS_COORD + 250}px`,
        left: `${MAX_CANVAS_COORD + 500}px`
      }
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const beforeItem = await readItemSnapshot(page, 'far-item');
    expect(beforeItem.position).toEqual({
      top: `${MAX_CANVAS_COORD + 250}px`,
      left: `${MAX_CANVAS_COORD + 500}px`
    });

    await showAllItemsForTest(page);
    await expect.poll(async () => {
      const rect = await readItemClientRect(page, 'far-item');
      const viewport = await readWorkspaceScroll(page);
      return rect.left < viewport.width && rect.right > 0 && rect.top < viewport.height && rect.bottom > 0;
    }).toBe(true);

    expect((await readItemSnapshot(page, 'far-item')).position).toEqual(beforeItem.position);

    const farNote = page.locator('.grid-item[data-id="far-item"]');
    await dragNoteBy(page, farNote, 40, 25);
    await page.waitForTimeout(350);

    const afterItem = await readItemSnapshot(page, 'far-item');
    expect(afterItem.position).toEqual({
      top: `${MAX_CANVAS_COORD}px`,
      left: `${MAX_CANVAS_COORD}px`
    });
  });

  test('manual note edits over the text limit are blocked without truncating the draft or mutating saved content', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildLocalSnapshot('Short note'));
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await note.dblclick();
    const textarea = page.locator('textarea.edit-textarea');
    const longText = 'x'.repeat(MAX_ITEM_TEXT_CHARS + 1);
    await textarea.fill(longText);
    await textarea.evaluate((element) => element.blur());

    await expect(page.locator('.toast').last()).toContainText(`Notes can be up to ${MAX_ITEM_TEXT_CHARS} characters.`);
    await expect(textarea).toBeVisible();
    await expect(textarea).toHaveValue(longText);
    expect((await readItemSnapshot(page, 'item-1')).name).toBe('Short note');

    await textarea.fill('Trimmed note');
    await textarea.press('Escape');
    await expectNoteVisible(page, 'Trimmed note');
    expect((await readItemSnapshot(page, 'item-1')).name).toBe('Trimmed note');
  });

  test('existing over-limit saved note text still loads unchanged until the user saves a shorter edit', async ({ page }) => {
    const longSavedText = 'L'.repeat(MAX_ITEM_TEXT_CHARS + 1);

    await prepareBlankPage(page);
    await seedLocalStorage(page, buildLocalSnapshot(longSavedText));
    await page.goto('/index.html');

    expect((await readItemSnapshot(page, 'item-1')).name.length).toBe(MAX_ITEM_TEXT_CHARS + 1);
  });

  test('legacy over-limit note edits can be canceled safely after a blocked save attempt', async ({ page }) => {
    const longSavedText = 'L'.repeat(MAX_ITEM_TEXT_CHARS + 1);

    await prepareBlankPage(page);
    await seedLocalStorage(page, buildLocalSnapshot(longSavedText));
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await note.evaluate((element) => {
      element.dispatchEvent(new MouseEvent('dblclick', { bubbles: true, cancelable: true }));
    });
    const textarea = page.locator('textarea.edit-textarea');
    await expect(textarea).toHaveValue(longSavedText);
    await textarea.press('Escape');
    await expect(textarea).toHaveCount(0);
    expect((await readItemSnapshot(page, 'item-1')).name).toBe(longSavedText);

    await note.evaluate((element) => {
      element.dispatchEvent(new MouseEvent('dblclick', { bubbles: true, cancelable: true }));
    });
    await textarea.fill(`${longSavedText}x`);
    await textarea.evaluate((element) => element.blur());
    await expect(page.locator('.toast').last()).toContainText(`Notes can be up to ${MAX_ITEM_TEXT_CHARS} characters.`);
    await expect(textarea).toBeVisible();
    await expect(textarea).toHaveValue(`${longSavedText}x`);
    expect((await readItemSnapshot(page, 'item-1')).name).toBe(longSavedText);

    await textarea.press('Escape');
    await expect(textarea).toHaveCount(0);
    expect((await readItemSnapshot(page, 'item-1')).name).toBe(longSavedText);

    await note.evaluate((element) => {
      element.dispatchEvent(new MouseEvent('dblclick', { bubbles: true, cancelable: true }));
    });
    await textarea.fill('T'.repeat(MAX_ITEM_TEXT_CHARS));
    await textarea.evaluate((element) => element.blur());
    await expect(textarea).toHaveCount(0);
    expect((await readItemSnapshot(page, 'item-1')).name).toBe('T'.repeat(MAX_ITEM_TEXT_CHARS));
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
      expect(item).not.toHaveProperty('x');
      expect(item).not.toHaveProperty('y');
      expect(item.width).toEqual(expect.any(Number));
      expect(item.height).toEqual(expect.any(Number));
      expect(item).not.toHaveProperty('sourceNodeId');
      expect(item).not.toHaveProperty('graphNodeId');
    });

    const startPosition = await page.evaluate((item) => {
      return window.PostbabyGeometryDom.getItemPositionXY(item);
    }, startItem);
    expect(startPosition.x).toBeGreaterThan(0);
    expect(startPosition.y).toBeGreaterThan(0);
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

  test('graph and mermaid-created items keep top/left CSS-string storage and shared numeric helper access', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const graphResult = await createGraphForTest(page, buildNormalizedGraph([
      buildGraphNode('A', { label: 'Graph Source', x: 0, y: 0 }),
      buildGraphNode('B', { label: 'Graph Target', shape: 'circle', x: 240, y: 80 })
    ], [
      buildGraphEdge('A', 'B', { kind: 'arrow' })
    ], {
      originX: 120,
      originY: 140
    }));

    expect(graphResult.ok).toBe(true);
    const graphItem = await readItemSnapshot(page, graphResult.createdItemIds[0]);
    expect(graphItem).toBeTruthy();
    expect(graphItem.position.top).toMatch(/px$/);
    expect(graphItem.position.left).toMatch(/px$/);
    expect(graphItem).not.toHaveProperty('x');
    expect(graphItem).not.toHaveProperty('y');
    const graphPosition = await page.evaluate((item) => {
      return window.PostbabyGeometryDom.getItemPositionXY(item);
    }, graphItem);
    expect(graphPosition.x).toBeGreaterThanOrEqual(120);
    expect(graphPosition.y).toBeGreaterThanOrEqual(140);

    const mermaidResult = await createGraphFromMermaidForTest(page, [
      'flowchart LR',
      'M[Mermaid Source] --> N((Mermaid Target))'
    ].join('\n'), {
      originX: 420,
      originY: 220
    });

    expect(mermaidResult.ok).toBe(true);
    const mermaidItem = await readItemSnapshot(page, mermaidResult.createdItemIds[0]);
    expect(mermaidItem).toBeTruthy();
    expect(mermaidItem.position.top).toMatch(/px$/);
    expect(mermaidItem.position.left).toMatch(/px$/);
    expect(mermaidItem).not.toHaveProperty('x');
    expect(mermaidItem).not.toHaveProperty('y');
    const mermaidPosition = await page.evaluate((item) => {
      return window.PostbabyGeometryDom.getItemPositionXY(item);
    }, mermaidItem);
    expect(mermaidPosition.x).toBeGreaterThanOrEqual(420);
    expect(mermaidPosition.y).toBeGreaterThanOrEqual(220);
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
    },
    {
      name: 'explicit node coordinates outside the bounded canvas',
      graph: buildNormalizedGraph([
        buildGraphNode('A', {
          x: MAX_CANVAS_COORD + 1,
          y: 0
        })
      ]),
      expectedCode: 'node_position_out_of_bounds'
    },
    {
      name: 'generated positions outside the bounded canvas',
      graph: buildNormalizedGraph([
        buildGraphNode('A'),
        buildGraphNode('B')
      ], [
        buildGraphEdge('A', 'B', { kind: 'arrow' })
      ], {
        originX: MAX_CANVAS_COORD,
        originY: 0,
        direction: 'LR'
      }),
      expectedCode: 'generated_node_position_out_of_bounds'
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

  test('graph hook rejects imports that exceed the target tab note capacity', async ({ page }) => {
    const items = buildBulkNoteItems(MAX_ITEMS_PER_TAB - 1);

    await prepareBlankPage(page);
    await seedLocalStorage(page, buildLocalSnapshotWithItems(items));
    await page.goto('/index.html');

    const result = await createGraphForTest(page, buildNormalizedGraph([
      buildGraphNode('A', { label: 'Capacity A' }),
      buildGraphNode('B', { label: 'Capacity B' })
    ], [
      buildGraphEdge('A', 'B', { kind: 'arrow' })
    ]));

    expect(result.ok).toBe(false);
    expect(result.errors.map((error) => error.code)).toContain('tab_item_limit_exceeded');
    expect((await readTabsSnapshot(page))[0].items).toHaveLength(MAX_ITEMS_PER_TAB - 1);
  });

  test('graph hook rejects imports that exceed the target tab edge capacity', async ({ page }) => {
    const items = buildBulkNoteItems(65, { columns: 8 });
    const edges = buildDenseUndirectedEdges(items.map((item) => item.id), MAX_EDGES_PER_TAB - 1, { kind: 'line' });

    await prepareBlankPage(page);
    await seedLocalStorage(page, buildLocalSnapshotWithItems(items, { edges }));
    await page.goto('/index.html');

    const result = await createGraphForTest(page, buildNormalizedGraph([
      buildGraphNode('A', { label: 'Capacity A' }),
      buildGraphNode('B', { label: 'Capacity B' }),
      buildGraphNode('C', { label: 'Capacity C' })
    ], [
      buildGraphEdge('A', 'B', { kind: 'arrow' }),
      buildGraphEdge('A', 'C', { kind: 'line' })
    ]));

    expect(result.ok).toBe(false);
    expect(result.errors.map((error) => error.code)).toContain('tab_edge_limit_exceeded');
    expect((await readTabsSnapshot(page))[0].edges).toHaveLength(MAX_EDGES_PER_TAB - 1);
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

  test('graph-created items keep stored positions across rerender and reload', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const graph = buildNormalizedGraph([
      buildGraphNode('A', { label: 'Graph Position A', x: 0, y: 0 }),
      buildGraphNode('B', { label: 'Graph Position B', shape: 'circle', x: 260, y: 80 })
    ], [
      buildGraphEdge('A', 'B', { kind: 'arrow' })
    ], {
      originX: 180,
      originY: 120
    });

    const result = await createGraphForTest(page, graph);
    expect(result.ok).toBe(true);

    const beforePositions = await readItemPositionsById(page, result.createdItemIds);
    const firstNote = page.locator(`.grid-item[data-id="${result.createdItemIds[0]}"]`);
    await firstNote.click();
    await page.keyboard.press('c');
    await page.waitForTimeout(350);
    expect(await readItemPositionsById(page, result.createdItemIds)).toEqual(beforePositions);

    await page.reload();
    expect(await readItemPositionsById(page, result.createdItemIds)).toEqual(beforePositions);
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
    [sourceItem, circleItem, diamondItem].forEach((item) => {
      expect(item.position.top).toMatch(/px$/);
      expect(item.position.left).toMatch(/px$/);
      expect(item).not.toHaveProperty('x');
      expect(item).not.toHaveProperty('y');
    });

    const arrowEdge = tabsSnapshot[0].edges.find((edge) => edge.kind === 'arrow');
    const lineEdge = tabsSnapshot[0].edges.find((edge) => edge.kind === 'line');
    expect(arrowEdge).toBeTruthy();
    expect(lineEdge).toBeTruthy();

    await expect(page.locator('.grid-item')).toHaveCount(3);
    await expect(page.locator('.edge-line')).toHaveCount(2);
  });

  test('mermaid-derived graphs enforce remaining tab note capacity', async ({ page }) => {
    const items = buildBulkNoteItems(MAX_ITEMS_PER_TAB - 1);

    await prepareBlankPage(page);
    await seedLocalStorage(page, buildLocalSnapshotWithItems(items));
    await page.goto('/index.html');

    const result = await createGraphFromMermaidForTest(page, [
      'flowchart LR',
      'A[Mermaid A] --> B[Mermaid B]'
    ].join('\n'));

    expect(result.ok).toBe(false);
    expect(result.errors.map((error) => error.code)).toContain('tab_item_limit_exceeded');
    expect((await readTabsSnapshot(page))[0].items).toHaveLength(MAX_ITEMS_PER_TAB - 1);
  });

  test('mermaid-derived graphs enforce remaining tab edge capacity', async ({ page }) => {
    const items = buildBulkNoteItems(65, { columns: 8 });
    const edges = buildDenseUndirectedEdges(items.map((item) => item.id), MAX_EDGES_PER_TAB - 1, { kind: 'line' });

    await prepareBlankPage(page);
    await seedLocalStorage(page, buildLocalSnapshotWithItems(items, { edges }));
    await page.goto('/index.html');

    const result = await createGraphFromMermaidForTest(page, [
      'flowchart LR',
      'A --> B',
      'A --- C'
    ].join('\n'));

    expect(result.ok).toBe(false);
    expect(result.errors.map((error) => error.code)).toContain('tab_edge_limit_exceeded');
    expect((await readTabsSnapshot(page))[0].edges).toHaveLength(MAX_EDGES_PER_TAB - 1);
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

  test('mermaid-created items keep stored positions across rerender and reload', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const result = await createGraphFromMermaidForTest(page, [
      'flowchart LR',
      'A[Mermaid Position A] --> B((Mermaid Position B))'
    ].join('\n'), {
      originX: 220,
      originY: 160
    });

    expect(result.ok).toBe(true);

    const beforePositions = await readItemPositionsById(page, result.createdItemIds);
    const firstNote = page.locator(`.grid-item[data-id="${result.createdItemIds[0]}"]`);
    await firstNote.click();
    await page.keyboard.press('c');
    await page.waitForTimeout(350);
    expect(await readItemPositionsById(page, result.createdItemIds)).toEqual(beforePositions);

    await page.reload();
    expect(await readItemPositionsById(page, result.createdItemIds)).toEqual(beforePositions);
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

  test('hand mode empty canvas click clears marquee selection', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Hand Mode Selected', {
        itemId: 'item-1',
        position: { top: '80px', left: '120px' }
      }),
      buildNoteItem('Hand Mode Other', {
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

    await setCanvasMode(page, 'pan');
    expect(await readCanvasMode(page)).toBe('pan');

    await clickEmptyGrid(page);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual([]);
    expect(await readCanvasMode(page)).toBe('pan');
  });

  test('hand mode blank canvas drag keeps selection and pans camera', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Hand Mode Drag Selected', {
        itemId: 'item-1',
        position: { top: '360px', left: '520px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const note = page.locator('.grid-item[data-id="item-1"]');
    await marqueeSelectNotes(page, [note]);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1']);

    await setCanvasMode(page, 'pan');
    expect(await readCanvasMode(page)).toBe('pan');

    const workspaceBox = await page.locator('#tabContent').boundingBox();
    if (!workspaceBox) {
      throw new Error('Workspace viewport bounding box was not available.');
    }

    const blankStartX = Math.round(workspaceBox.x + workspaceBox.width - 220);
    const blankStartY = Math.round(workspaceBox.y + workspaceBox.height - 180);
    const beforePanCamera = await readCamera(page);

    await page.mouse.move(blankStartX, blankStartY);
    await page.mouse.down();
    await page.mouse.move(blankStartX + 120, blankStartY + 70, { steps: 8 });
    await page.mouse.up();

    const afterPanCamera = await readCamera(page);
    expect(afterPanCamera.x).toBeLessThan(beforePanCamera.x);
    expect(afterPanCamera.y).toBeLessThan(beforePanCamera.y);
    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1']);
  });

  test('middle mouse blank canvas click does not clear selection', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Middle Mouse Selected', {
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

    const workspaceBox = await page.locator('#tabContent').boundingBox();
    if (!workspaceBox) {
      throw new Error('Workspace viewport bounding box was not available.');
    }

    const blankX = Math.round(workspaceBox.x + workspaceBox.width - 220);
    const blankY = Math.round(workspaceBox.y + workspaceBox.height - 180);
    await page.mouse.click(blankX, blankY, { button: 'middle' });

    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1']);
  });

  test('Space pan blank canvas click does not clear selection', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Space Pan Selected', {
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

    const workspaceBox = await page.locator('#tabContent').boundingBox();
    if (!workspaceBox) {
      throw new Error('Workspace viewport bounding box was not available.');
    }

    const blankX = Math.round(workspaceBox.x + workspaceBox.width - 220);
    const blankY = Math.round(workspaceBox.y + workspaceBox.height - 180);

    await page.keyboard.down('Space');
    await page.mouse.move(blankX, blankY);
    await page.mouse.down();
    await page.mouse.up();
    await page.keyboard.up('Space');

    await expect.poll(() => readSelectedNoteIds(page)).toEqual(['item-1']);
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
        position: { top: '260px', left: '460px' }
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

  test('Hide Camera Remote hides camera controls, persists across reload, and restores them when re-enabled', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Camera Remote Toggle');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    await expect.poll(async () => (await readCameraControlsState(page)).hidden).toBe(false);

    await openSettingsModal(page);
    const hideCameraControlsToggle = page.locator('#toggleHideCameraControls');
    await expect(hideCameraControlsToggle).not.toBeChecked();
    await hideCameraControlsToggle.evaluate((element) => element.click());
    await expect(hideCameraControlsToggle).toBeChecked();
    await expect.poll(async () => (await readCameraControlsState(page)).hidden).toBe(true);
    await expect.poll(async () => {
      const indexedDBState = await readIndexedDBState(page);
      return indexedDBState.snapshot ? indexedDBState.snapshot.hideCameraControls : null;
    }).toBe('true');
    await page.locator('.close-settings').click();
    await expect(page.locator('#settingsModal')).toBeHidden();
    await expect.poll(async () => (await readCameraControlsState(page)).hidden).toBe(true);

    await page.reload();
    await expectNoteVisible(page, 'Camera Remote Toggle');
    await expect.poll(async () => (await readCameraControlsState(page)).hidden).toBe(true);

    await openSettingsModal(page);
    const reloadedHideCameraControlsToggle = page.locator('#toggleHideCameraControls');
    await expect(reloadedHideCameraControlsToggle).toBeChecked();
    await reloadedHideCameraControlsToggle.evaluate((element) => element.click());
    await expect(reloadedHideCameraControlsToggle).not.toBeChecked();
    await expect.poll(async () => (await readCameraControlsState(page)).hidden).toBe(false);
    await expect.poll(async () => {
      const indexedDBState = await readIndexedDBState(page);
      return indexedDBState.snapshot ? indexedDBState.snapshot.hideCameraControls : null;
    }).toBe('false');
    await page.locator('.close-settings').click();
    await expect(page.locator('#settingsModal')).toBeHidden();
    await expect.poll(async () => (await readCameraControlsState(page)).hidden).toBe(false);
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

  test('static mode does not call the delta metadata endpoint', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Static Delta Probe Note');
    let deltaRequests = 0;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.route(/\/api\/sync\/delta(?:\?.*)?$/, async (route) => {
      deltaRequests += 1;
      await route.fulfill({
        status: 500,
        contentType: 'application/json; charset=utf-8',
        body: JSON.stringify({ ok: false })
      });
    });

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Static Delta Probe Note');
    await page.waitForTimeout(200);

    expect(deltaRequests).toBe(0);
    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.deltaMetadataAvailable).toBe(false);
    expect(debugState.deltaMetadataLastCheckedAt).toBeNull();
    expect(debugState.deltaMetadataUnavailableForSession).toBe(false);
  });

  test('self-hosted startup probe records debug state without mutating notes, outbox, camera, or applying replay metadata locally', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Probe Stable Note', { syncVersion: 6 });
    const requestSequence = [];
    let releaseDelta = null;
    const deltaGate = new Promise((resolve) => {
      releaseDelta = resolve;
    });

    page.on('request', (request) => {
      const url = request.url();
      if (url.includes('/api/document/meta')) {
        requestSequence.push('meta');
      } else if (url.includes('/api/sync/delta')) {
        requestSequence.push('delta');
      } else if (url.includes('/api/document?')) {
        requestSequence.push('document');
      }
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Probe Stable Note', 6),
      onDelta: async () => {
        await deltaGate;
        return {
          status: 200,
          body: {
            ok: true,
            appId: 'postbaby-web',
            currentDocumentVersion: 6,
            currentDocumentHash: 'server-hash-6',
            clientVersion: 6,
            requiresSnapshotRefresh: false,
            reason: 'applications_available',
            applicationWatermark: 41,
            nextApplicationWatermark: 43,
            applications: [
              {
                mutationId: 'mut-applied-1',
                applicationStatus: 'authoritativeApplied',
                applicationReason: 'policy_allowed',
                canonicalDocumentVersionBefore: 5,
                canonicalDocumentVersionAfter: 6,
                canonicalDocumentHashBefore: 'hash-5',
                canonicalDocumentHashAfter: 'hash-6',
                replayObservationId: 11,
                createdAt: TIMESTAMP
              },
              {
                mutationId: 'mut-skipped-1',
                applicationStatus: 'authoritativeSkipped',
                applicationReason: 'already_reflected',
                canonicalDocumentVersionBefore: 6,
                canonicalDocumentVersionAfter: 6,
                canonicalDocumentHashBefore: 'hash-6',
                canonicalDocumentHashAfter: 'hash-6',
                replayObservationId: 11,
                createdAt: TIMESTAMP
              }
            ],
            warnings: []
          }
        };
      }
    });

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Probe Stable Note');
    await expect.poll(() => requestSequence.filter((entry) => entry === 'delta').length).toBe(1);

    await setCamera(page, { x: 160, y: 90, zoom: 1.25 });
    const tabsBefore = await readTabsSnapshot(page);
    const outboxBefore = await readMutationOutbox(page);
    const cameraBefore = await readCamera(page);

    releaseDelta();

    await expect.poll(async () => {
      const state = await page.evaluate(() => window.postbabyDebugSync());
      return Boolean(state.deltaMetadataLastCheckedAt);
    }).toBe(true);

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    const tabsAfter = await readTabsSnapshot(page);
    const outboxAfter = await readMutationOutbox(page);
    const cameraAfter = await readCamera(page);

    expect(requestSequence.indexOf('meta')).toBeGreaterThanOrEqual(0);
    expect(requestSequence.indexOf('delta')).toBeGreaterThan(requestSequence.indexOf('meta'));
    expect(debugState.deltaMetadataAvailable).toBe(true);
    expect(debugState.deltaMetadataLastReason).toBe('applications_available');
    expect(debugState.deltaMetadataLastError).toBe('');
    expect(debugState.deltaMetadataUnavailableForSession).toBe(false);
    expect(tabsAfter).toEqual(tabsBefore);
    expect(outboxAfter).toEqual(outboxBefore);
    expect(cameraAfter).toEqual(cameraBefore);
    expect(JSON.stringify(tabsAfter)).not.toContain('mut-applied-1');
    expect(JSON.stringify(tabsAfter)).not.toContain('mut-skipped-1');
  });

  test('delta capability probe treats 404 not_found as unavailable without failing startup sync', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Delta 404 Note', { syncVersion: 6 });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Delta 404 Note', 6),
      onDelta: async () => ({
        status: 404,
        body: {
          ok: false,
          error: {
            code: 'not_found',
            message: 'not found'
          }
        }
      })
    });

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Delta 404 Note');
    await expect.poll(async () => {
      const state = await page.evaluate(() => window.postbabyDebugSync());
      return Boolean(state.deltaMetadataLastCheckedAt);
    }).toBe(true);

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.syncState).toBe('synced');
    expect(debugState.deltaMetadataAvailable).toBe(false);
    expect(debugState.deltaMetadataUnavailableForSession).toBe(true);
    expect(debugState.deltaMetadataLastReason).toBe('not_found');
    expect(debugState.deltaMetadataLastError).toBe('');
  });

  test('delta capability probe network failure is non-fatal', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Delta Network Failure Note', { syncVersion: 6 });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Delta Network Failure Note', 6),
      onDelta: async () => ({
        abort: 'failed'
      })
    });

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Delta Network Failure Note');
    await expect.poll(async () => {
      const state = await page.evaluate(() => window.postbabyDebugSync());
      return Boolean(state.deltaMetadataLastCheckedAt);
    }).toBe(true);

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.syncState).toBe('synced');
    expect(debugState.deltaMetadataAvailable).toBe(false);
    expect(debugState.deltaMetadataUnavailableForSession).toBe(false);
    expect(debugState.deltaMetadataLastReason).toBe('network_error');
    expect(debugState.deltaMetadataLastError).not.toBe('');
  });

  test('delta capability probe unauthorized failure stays probe-local and keeps snapshot sync usable', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Delta Unauthorized Note', { syncVersion: 6 });
    const dialogs = [];
    let capturedSaveBody = null;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Delta Unauthorized Note', 6),
      onDelta: async () => ({
        status: 401,
        body: {
          ok: false,
          error: {
            code: 'auth_required',
            message: 'login required for delta'
          }
        }
      }),
      onSave: async (body) => {
        capturedSaveBody = body;
        return {
          status: 200,
          body: { ok: true, version: 7, updatedAt: TIMESTAMP }
        };
      }
    });
    attachDialogHandler(page, [], dialogs);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Delta Unauthorized Note');
    await expect.poll(async () => {
      const state = await page.evaluate(() => window.postbabyDebugSync());
      return state.isBackgroundSyncActive === true;
    }).toBe(true);

    await setCamera(page, { x: 160, y: 90, zoom: 1.25 });
    const tabsBefore = await readTabsSnapshot(page);
    const outboxBefore = await readMutationOutbox(page);
    const cameraBefore = await readCamera(page);

    await expect.poll(async () => {
      const state = await page.evaluate(() => window.postbabyDebugSync());
      return state.deltaMetadataLastError;
    }).toBe('unauthorized');

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    const tabsAfterProbe = await readTabsSnapshot(page);
    const outboxAfterProbe = await readMutationOutbox(page);
    const cameraAfterProbe = await readCamera(page);

    expect(page.url()).not.toContain('/login');
    expect(dialogs).toEqual([]);
    expect(debugState.syncState).toBe('synced');
    expect(debugState.runtimeConfig.isAuthenticated).toBe(true);
    expect(debugState.runtimeConfig.syncUsable).toBe(true);
    expect(debugState.isSyncAwaitingAuthentication).toBe(false);
    expect(debugState.isSyncBlockedByEntitlement).toBe(false);
    expect(debugState.isBackgroundSyncActive).toBe(true);
    expect(debugState.deltaMetadataAvailable).toBe(false);
    expect(debugState.deltaMetadataUnavailableForSession).toBe(true);
    expect(debugState.deltaMetadataLastReason).toBe('auth_required');
    expect(debugState.deltaMetadataLastError).toBe('unauthorized');
    expect(tabsAfterProbe).toEqual(tabsBefore);
    expect(outboxAfterProbe).toEqual(outboxBefore);
    expect(cameraAfterProbe).toEqual(cameraBefore);

    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();
    await expect.poll(() => capturedSaveBody !== null).toBe(true);
    await expect.poll(async () => {
      const state = await page.evaluate(() => window.postbabyDebugSync());
      return state.syncStoredVersion;
    }).toBe(7);

    const debugStateAfterManualSync = await page.evaluate(() => window.postbabyDebugSync());
    expect(page.url()).not.toContain('/login');
    expect(capturedSaveBody.baseServerRevision).toBe(6);
    expect(debugStateAfterManualSync.runtimeConfig.syncUsable).toBe(true);
    expect(debugStateAfterManualSync.syncState).toBe('synced');
  });

  test('delta capability probe entitlement failure stays probe-local and keeps snapshot sync usable', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Delta Entitlement Note', { syncVersion: 6 });
    const dialogs = [];
    let capturedSaveBody = null;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Delta Entitlement Note', 6),
      onDelta: async () => ({
        status: 403,
        body: {
          ok: false,
          error: {
            code: 'entitlement_required',
            message: 'delta entitlement unavailable'
          }
        }
      }),
      onSave: async (body) => {
        capturedSaveBody = body;
        return {
          status: 200,
          body: { ok: true, version: 7, updatedAt: TIMESTAMP }
        };
      }
    });
    attachDialogHandler(page, [], dialogs);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Delta Entitlement Note');
    await expect.poll(async () => {
      const state = await page.evaluate(() => window.postbabyDebugSync());
      return state.isBackgroundSyncActive === true;
    }).toBe(true);

    await setCamera(page, { x: 200, y: 120, zoom: 1.4 });
    const tabsBefore = await readTabsSnapshot(page);
    const outboxBefore = await readMutationOutbox(page);
    const cameraBefore = await readCamera(page);

    await expect.poll(async () => {
      const state = await page.evaluate(() => window.postbabyDebugSync());
      return state.deltaMetadataLastError;
    }).toBe('entitlement_required');

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    const tabsAfterProbe = await readTabsSnapshot(page);
    const outboxAfterProbe = await readMutationOutbox(page);
    const cameraAfterProbe = await readCamera(page);

    expect(page.url()).not.toContain('/login');
    expect(dialogs).toEqual([]);
    expect(debugState.syncState).toBe('synced');
    expect(debugState.runtimeConfig.isAuthenticated).toBe(true);
    expect(debugState.runtimeConfig.syncUsable).toBe(true);
    expect(debugState.isSyncAwaitingAuthentication).toBe(false);
    expect(debugState.isSyncBlockedByEntitlement).toBe(false);
    expect(debugState.isBackgroundSyncActive).toBe(true);
    expect(debugState.deltaMetadataAvailable).toBe(false);
    expect(debugState.deltaMetadataUnavailableForSession).toBe(true);
    expect(debugState.deltaMetadataLastReason).toBe('entitlement_required');
    expect(debugState.deltaMetadataLastError).toBe('entitlement_required');
    expect(tabsAfterProbe).toEqual(tabsBefore);
    expect(outboxAfterProbe).toEqual(outboxBefore);
    expect(cameraAfterProbe).toEqual(cameraBefore);

    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();
    await expect.poll(() => capturedSaveBody !== null).toBe(true);
    await expect.poll(async () => {
      const state = await page.evaluate(() => window.postbabyDebugSync());
      return state.syncStoredVersion;
    }).toBe(7);

    const debugStateAfterManualSync = await page.evaluate(() => window.postbabyDebugSync());
    expect(capturedSaveBody.baseServerRevision).toBe(6);
    expect(debugStateAfterManualSync.runtimeConfig.syncUsable).toBe(true);
    expect(debugStateAfterManualSync.syncState).toBe('synced');
  });

  test('manual advisory delta check runs only after successful manual sync and stays diagnostic-only', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Manual Advisory Stable Note', { syncVersion: 6 });
    const dialogs = [];
    const requestSequence = [];
    let capturedSaveBody = null;
    let deltaCallCount = 0;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Manual Advisory Stable Note', 6),
      onSave: async (body) => {
        capturedSaveBody = body;
        requestSequence.push(`document-put:${body.baseServerRevision}`);
        return {
          status: 200,
          body: { ok: true, version: 7, updatedAt: TIMESTAMP }
        };
      },
      onDelta: async (request) => {
        deltaCallCount += 1;
        const url = new URL(request.url());
        const sinceVersion = url.searchParams.get('sinceVersion');
        requestSequence.push(`delta:${deltaCallCount}:${sinceVersion}`);
        if (deltaCallCount === 1) {
          return {
            status: 200,
            body: {
              ok: true,
              reason: 'up_to_date',
              currentDocumentVersion: 6,
              currentDocumentHash: 'hash-6',
              clientVersion: 6,
              requiresSnapshotRefresh: false,
              warnings: []
            }
          };
        }

        return {
          status: 200,
          body: {
            ok: true,
            reason: 'applications_available',
            currentDocumentVersion: 7,
            currentDocumentHash: 'hash-7',
            clientVersion: 7,
            requiresSnapshotRefresh: false,
            applicationWatermark: 41,
            nextApplicationWatermark: 43,
            applications: [
              {
                mutationId: 'mut-applied-manual',
                applicationStatus: 'authoritativeApplied',
                applicationReason: 'policy_allowed',
                canonicalDocumentVersionBefore: 6,
                canonicalDocumentVersionAfter: 7,
                canonicalDocumentHashBefore: 'hash-6',
                canonicalDocumentHashAfter: 'hash-7',
                replayObservationId: 19,
                createdAt: TIMESTAMP
              }
            ],
            warnings: []
          }
        };
      }
    });
    attachDialogHandler(page, [], dialogs);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Manual Advisory Stable Note');
    await waitForDeltaMetadataCheckCount(page, 1);

    requestSequence.length = 0;

    await setCamera(page, { x: 180, y: 110, zoom: 1.35 });
    const tabsBefore = await readTabsSnapshot(page);
    const outboxBefore = await readMutationOutbox(page);
    const cameraBefore = await readCamera(page);

    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();

    await expect.poll(() => capturedSaveBody !== null).toBe(true);
    await waitForDeltaMetadataCheckCount(page, 2);
    await expect.poll(async () => {
      const state = await page.evaluate(() => window.postbabyDebugSync());
      return state.deltaMetadataLastTrigger;
    }).toBe('manual-sync');

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    const tabsAfter = await readTabsSnapshot(page);
    const outboxAfter = await readMutationOutbox(page);
    const cameraAfter = await readCamera(page);

    expect(capturedSaveBody.baseServerRevision).toBe(6);
    expect(requestSequence[0]).toBe('document-put:6');
    expect(requestSequence).toContain('delta:2:7');
    expect(requestSequence.indexOf('delta:2:7')).toBeGreaterThan(requestSequence.indexOf('document-put:6'));
    expect(dialogs).toEqual([]);
    expect(page.url()).not.toContain('/login');
    expect(debugState.syncState).toBe('synced');
    expect(debugState.syncStoredVersion).toBe(7);
    expect(debugState.deltaMetadataAvailable).toBe(true);
    expect(debugState.deltaMetadataUnavailableForSession).toBe(false);
    expect(debugState.deltaMetadataLastTrigger).toBe('manual-sync');
    expect(debugState.deltaMetadataLastReason).toBe('applications_available');
    expect(debugState.deltaMetadataLastError).toBe('');
    expect(debugState.deltaMetadataLastServerVersion).toBe(7);
    expect(debugState.deltaMetadataCheckCount).toBe(2);
    expect(debugState.deltaMetadataLastRequiresSnapshotRefresh).toBe(false);
    expect(tabsAfter).toEqual(tabsBefore);
    expect(outboxAfter).toEqual(outboxBefore);
    expect(cameraAfter).toEqual(cameraBefore);
    expect(JSON.stringify(tabsAfter)).not.toContain('mut-applied-manual');
  });

  test('manual advisory document_version_changed stays diagnostic-only and does not fetch another snapshot', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Manual Advisory Version Note', { syncVersion: 6 });
    const dialogs = [];
    const requestSequence = [];
    let deltaCallCount = 0;

    page.on('request', (request) => {
      const url = request.url();
      if (url.includes('/api/document/meta')) {
        return;
      }
      if (url.includes('/api/document')) {
        requestSequence.push(request.method() === 'GET' ? 'document-get' : 'document-put');
      } else if (url.includes('/api/sync/delta')) {
        const sinceVersion = new URL(url).searchParams.get('sinceVersion');
        requestSequence.push(`delta:${sinceVersion}`);
      }
    });

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Manual Advisory Version Note', 6),
      onSave: async () => ({
        status: 200,
        body: { ok: true, version: 7, updatedAt: TIMESTAMP }
      }),
      onDelta: async () => {
        deltaCallCount += 1;
        if (deltaCallCount === 1) {
          return {
            status: 200,
            body: {
              ok: true,
              reason: 'up_to_date',
              currentDocumentVersion: 6,
              currentDocumentHash: 'hash-6',
              clientVersion: 6,
              requiresSnapshotRefresh: false,
              warnings: []
            }
          };
        }

        return {
          status: 200,
          body: {
            ok: true,
            reason: 'document_version_changed',
            currentDocumentVersion: 8,
            currentDocumentHash: 'hash-8',
            clientVersion: 7,
            requiresSnapshotRefresh: true,
            warnings: []
          }
        };
      }
    });
    attachDialogHandler(page, [], dialogs);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Manual Advisory Version Note');
    await waitForDeltaMetadataCheckCount(page, 1);

    requestSequence.length = 0;

    await setCamera(page, { x: 150, y: 90, zoom: 1.2 });
    const tabsBefore = await readTabsSnapshot(page);
    const outboxBefore = await readMutationOutbox(page);
    const cameraBefore = await readCamera(page);

    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();

    await waitForDeltaMetadataCheckCount(page, 2);
    await expect.poll(async () => {
      const state = await page.evaluate(() => window.postbabyDebugSync());
      return state.deltaMetadataLastTrigger;
    }).toBe('manual-sync');

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    const tabsAfter = await readTabsSnapshot(page);
    const outboxAfter = await readMutationOutbox(page);
    const cameraAfter = await readCamera(page);

    expect(requestSequence[0]).toBe('document-put');
    expect(requestSequence).toContain('delta:7');
    expect(requestSequence.filter((entry) => entry === 'document-get')).toEqual([]);
    expect(dialogs).toEqual([]);
    expect(debugState.syncState).toBe('synced');
    expect(debugState.syncStoredVersion).toBe(7);
    expect(debugState.deltaMetadataAvailable).toBe(true);
    expect(debugState.deltaMetadataLastReason).toBe('document_version_changed');
    expect(debugState.deltaMetadataLastTrigger).toBe('manual-sync');
    expect(debugState.deltaMetadataLastServerVersion).toBe(7);
    expect(debugState.deltaMetadataLastRequiresSnapshotRefresh).toBe(true);
    expect(tabsAfter).toEqual(tabsBefore);
    expect(outboxAfter).toEqual(outboxBefore);
    expect(cameraAfter).toEqual(cameraBefore);
  });

  test('manual advisory delta 404 after manual sync is non-fatal', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Manual Advisory 404 Note', { syncVersion: 6 });
    const dialogs = [];
    let deltaCallCount = 0;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Manual Advisory 404 Note', 6),
      onSave: async () => ({
        status: 200,
        body: { ok: true, version: 7, updatedAt: TIMESTAMP }
      }),
      onDelta: async () => {
        deltaCallCount += 1;
        if (deltaCallCount === 1) {
          return {
            status: 200,
            body: {
              ok: true,
              reason: 'up_to_date',
              currentDocumentVersion: 6,
              clientVersion: 6,
              requiresSnapshotRefresh: false,
              warnings: []
            }
          };
        }

        return {
          status: 404,
          body: {
            ok: false,
            error: {
              code: 'not_found',
              message: 'not found'
            }
          }
        };
      }
    });
    attachDialogHandler(page, [], dialogs);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Manual Advisory 404 Note');
    await waitForDeltaMetadataCheckCount(page, 1);

    await setCamera(page, { x: 210, y: 130, zoom: 1.45 });
    const tabsBefore = await readTabsSnapshot(page);
    const outboxBefore = await readMutationOutbox(page);
    const cameraBefore = await readCamera(page);

    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();

    await waitForDeltaMetadataCheckCount(page, 2);
    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    const tabsAfter = await readTabsSnapshot(page);
    const outboxAfter = await readMutationOutbox(page);
    const cameraAfter = await readCamera(page);

    expect(dialogs).toEqual([]);
    expect(debugState.syncState).toBe('synced');
    expect(debugState.syncStoredVersion).toBe(7);
    expect(debugState.runtimeConfig.syncUsable).toBe(true);
    expect(debugState.deltaMetadataAvailable).toBe(false);
    expect(debugState.deltaMetadataUnavailableForSession).toBe(true);
    expect(debugState.deltaMetadataLastTrigger).toBe('manual-sync');
    expect(debugState.deltaMetadataLastReason).toBe('not_found');
    expect(debugState.deltaMetadataLastError).toBe('');
    expect(tabsAfter).toEqual(tabsBefore);
    expect(outboxAfter).toEqual(outboxBefore);
    expect(cameraAfter).toEqual(cameraBefore);
  });

  test('manual advisory delta network failure after manual sync is non-fatal', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Manual Advisory Network Note', { syncVersion: 6 });
    const dialogs = [];
    let deltaCallCount = 0;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Manual Advisory Network Note', 6),
      onSave: async () => ({
        status: 200,
        body: { ok: true, version: 7, updatedAt: TIMESTAMP }
      }),
      onDelta: async () => {
        deltaCallCount += 1;
        if (deltaCallCount === 1) {
          return {
            status: 200,
            body: {
              ok: true,
              reason: 'up_to_date',
              currentDocumentVersion: 6,
              clientVersion: 6,
              requiresSnapshotRefresh: false,
              warnings: []
            }
          };
        }

        return { abort: 'failed' };
      }
    });
    attachDialogHandler(page, [], dialogs);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Manual Advisory Network Note');
    await waitForDeltaMetadataCheckCount(page, 1);

    await setCamera(page, { x: 160, y: 100, zoom: 1.25 });
    const tabsBefore = await readTabsSnapshot(page);
    const outboxBefore = await readMutationOutbox(page);
    const cameraBefore = await readCamera(page);

    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();

    await waitForDeltaMetadataCheckCount(page, 2);
    await expect.poll(async () => {
      const state = await page.evaluate(() => window.postbabyDebugSync());
      return state.deltaMetadataLastError;
    }).not.toBe('');

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    const tabsAfter = await readTabsSnapshot(page);
    const outboxAfter = await readMutationOutbox(page);
    const cameraAfter = await readCamera(page);

    expect(dialogs).toEqual([]);
    expect(debugState.syncState).toBe('synced');
    expect(debugState.syncStoredVersion).toBe(7);
    expect(debugState.runtimeConfig.syncUsable).toBe(true);
    expect(debugState.deltaMetadataAvailable).toBe(false);
    expect(debugState.deltaMetadataUnavailableForSession).toBe(false);
    expect(debugState.deltaMetadataLastTrigger).toBe('manual-sync');
    expect(debugState.deltaMetadataLastReason).toBe('network_error');
    expect(debugState.deltaMetadataLastError).not.toBe('');
    expect(tabsAfter).toEqual(tabsBefore);
    expect(outboxAfter).toEqual(outboxBefore);
    expect(cameraAfter).toEqual(cameraBefore);
  });

  test('manual advisory delta unauthorized failure stays probe-local after manual sync', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Manual Advisory Unauthorized Note', { syncVersion: 6 });
    const dialogs = [];
    let deltaCallCount = 0;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Manual Advisory Unauthorized Note', 6),
      onSave: async () => ({
        status: 200,
        body: { ok: true, version: 7, updatedAt: TIMESTAMP }
      }),
      onDelta: async () => {
        deltaCallCount += 1;
        if (deltaCallCount === 1) {
          return {
            status: 200,
            body: {
              ok: true,
              reason: 'up_to_date',
              currentDocumentVersion: 6,
              clientVersion: 6,
              requiresSnapshotRefresh: false,
              warnings: []
            }
          };
        }

        return {
          status: 401,
          body: {
            ok: false,
            error: {
              code: 'auth_required',
              message: 'login required for delta'
            }
          }
        };
      }
    });
    attachDialogHandler(page, [], dialogs);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Manual Advisory Unauthorized Note');
    await waitForDeltaMetadataCheckCount(page, 1);

    await setCamera(page, { x: 170, y: 120, zoom: 1.3 });
    const tabsBefore = await readTabsSnapshot(page);
    const outboxBefore = await readMutationOutbox(page);
    const cameraBefore = await readCamera(page);

    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();

    await waitForDeltaMetadataCheckCount(page, 2);
    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    const tabsAfter = await readTabsSnapshot(page);
    const outboxAfter = await readMutationOutbox(page);
    const cameraAfter = await readCamera(page);

    expect(dialogs).toEqual([]);
    expect(page.url()).not.toContain('/login');
    expect(debugState.syncState).toBe('synced');
    expect(debugState.syncStoredVersion).toBe(7);
    expect(debugState.runtimeConfig.syncUsable).toBe(true);
    expect(debugState.isSyncAwaitingAuthentication).toBe(false);
    expect(debugState.isSyncBlockedByEntitlement).toBe(false);
    expect(debugState.isBackgroundSyncActive).toBe(true);
    expect(debugState.deltaMetadataAvailable).toBe(false);
    expect(debugState.deltaMetadataUnavailableForSession).toBe(true);
    expect(debugState.deltaMetadataLastTrigger).toBe('manual-sync');
    expect(debugState.deltaMetadataLastReason).toBe('auth_required');
    expect(debugState.deltaMetadataLastError).toBe('unauthorized');
    expect(tabsAfter).toEqual(tabsBefore);
    expect(outboxAfter).toEqual(outboxBefore);
    expect(cameraAfter).toEqual(cameraBefore);
  });

  test('manual advisory delta entitlement failure stays probe-local after manual sync', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Manual Advisory Entitlement Note', { syncVersion: 6 });
    const dialogs = [];
    let deltaCallCount = 0;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Manual Advisory Entitlement Note', 6),
      onSave: async () => ({
        status: 200,
        body: { ok: true, version: 7, updatedAt: TIMESTAMP }
      }),
      onDelta: async () => {
        deltaCallCount += 1;
        if (deltaCallCount === 1) {
          return {
            status: 200,
            body: {
              ok: true,
              reason: 'up_to_date',
              currentDocumentVersion: 6,
              clientVersion: 6,
              requiresSnapshotRefresh: false,
              warnings: []
            }
          };
        }

        return {
          status: 403,
          body: {
            ok: false,
            error: {
              code: 'entitlement_required',
              message: 'delta entitlement unavailable'
            }
          }
        };
      }
    });
    attachDialogHandler(page, [], dialogs);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Manual Advisory Entitlement Note');
    await waitForDeltaMetadataCheckCount(page, 1);

    await setCamera(page, { x: 220, y: 140, zoom: 1.5 });
    const tabsBefore = await readTabsSnapshot(page);
    const outboxBefore = await readMutationOutbox(page);
    const cameraBefore = await readCamera(page);

    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();

    await waitForDeltaMetadataCheckCount(page, 2);
    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    const tabsAfter = await readTabsSnapshot(page);
    const outboxAfter = await readMutationOutbox(page);
    const cameraAfter = await readCamera(page);

    expect(dialogs).toEqual([]);
    expect(debugState.syncState).toBe('synced');
    expect(debugState.syncStoredVersion).toBe(7);
    expect(debugState.runtimeConfig.syncUsable).toBe(true);
    expect(debugState.isSyncAwaitingAuthentication).toBe(false);
    expect(debugState.isSyncBlockedByEntitlement).toBe(false);
    expect(debugState.isBackgroundSyncActive).toBe(true);
    expect(debugState.deltaMetadataAvailable).toBe(false);
    expect(debugState.deltaMetadataUnavailableForSession).toBe(true);
    expect(debugState.deltaMetadataLastTrigger).toBe('manual-sync');
    expect(debugState.deltaMetadataLastReason).toBe('entitlement_required');
    expect(debugState.deltaMetadataLastError).toBe('entitlement_required');
    expect(tabsAfter).toEqual(tabsBefore);
    expect(outboxAfter).toEqual(outboxBefore);
    expect(cameraAfter).toEqual(cameraBefore);
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

  test('covers the Titan/Venom quick-close failure class without requiring another Titan edit', async ({ browser }) => {
    const serverSnapshotData = buildEmptySnapshot({ hasRunBefore: true, theme: 'light' });
    const serverState = {
      snapshot: buildServerSnapshotPayload(serverSnapshotData, 6),
      blockUploads: true
    };
    const capturedSaveBodies = [];
    const localSnapshot = addDurableCloudSyncMetadata(
      buildEmptySnapshot({ hasRunBefore: true, theme: 'light', syncVersion: 6 }),
      {
        pending: false,
        revision: 6,
        lastCloudUploadedAt: TIMESTAMP
      }
    );

    async function openDevicePage() {
      const context = await browser.newContext({
        baseURL: TEST_BASE_URL,
        serviceWorkers: 'block'
      });
      await enableDynamicMockedSync(context, {
        runtimeOverrides: {
          isAuthenticated: true
        },
        getMetaPayload: () => ({
          ok: true,
          exists: true,
          version: serverState.snapshot.version,
          updatedAt: serverState.snapshot.updatedAt
        }),
        getDocumentPayload: () => serverState.snapshot,
        onSave: async (body) => {
          if (serverState.blockUploads) {
            return {
              status: 503,
              body: { ok: false, error: { code: 'blocked_for_test', message: 'blocked for test' } }
            };
          }

          capturedSaveBodies.push(body);
          const nextVersion = serverState.snapshot.version + 1;
          serverState.snapshot = buildServerSnapshotPayload(body.data, nextVersion);
          return {
            status: 200,
            body: { ok: true, version: nextVersion, updatedAt: TIMESTAMP }
          };
        }
      });

      const page = await context.newPage();
      await prepareBlankPage(page);
      await seedLocalStorage(page, localSnapshot, 'owner-scope');
      await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
      return { context, page };
    }

    const titan = await openDevicePage();
    const venom = await openDevicePage();

    try {
      await titan.page.goto('/index.html');
      await venom.page.goto('/index.html');
      await createNoteAt(titan.page, 'Titan Quick Close Note', { x: 920, y: 320 });

      const titanIndexedDBBeforeReload = await readIndexedDBState(titan.page, 'owner-scope');
      const titanTabsBeforeReload = JSON.parse(titanIndexedDBBeforeReload.snapshot.tabs);
      expect(titanTabsBeforeReload[0].items.some((item) => item.name === 'Titan Quick Close Note')).toBe(true);
      expect(titanIndexedDBBeforeReload.snapshot.postbabyPendingCloudUpload).toBe('true');
      expect(serverState.snapshot.version).toBe(6);

      await titan.page.reload();
      await expectNoteVisible(titan.page, 'Titan Quick Close Note');

      const titanDebugAfterReload = await titan.page.evaluate(() => window.postbabyDebugSync());
      expect(titanDebugAfterReload.syncStoredVersion).toBe(6);
      expect(titanDebugAfterReload.durablePendingCloudUpload).toBe(true);
      expect(titanDebugAfterReload.durableSyncMetadata.lastKnownServerRevision).toBe(6);
      expect(titanDebugAfterReload.syncState).not.toBe('synced');
      expect(titanDebugAfterReload.mutationOutbox.pendingCount).toBe(2);
      expect(titanDebugAfterReload.mutationOutbox.records.map((record) => record.operationType)).toEqual(['CreateNode', 'UpdateNode']);
      expect(titanDebugAfterReload.mutationOutbox.records.map((record) => record.status)).toEqual(['pending', 'pending']);

      serverState.blockUploads = false;
      await titan.page.locator('#syncStatusButton').click();
      await titan.page.locator('#syncNowButton').click();

      await expect.poll(() => serverState.snapshot.version).toBe(7);
      await expect.poll(async () => {
        const state = await titan.page.evaluate(() => window.postbabyDebugSync());
        return state.syncStoredVersion;
      }).toBe(7);

      const titanDebugAfterSync = await titan.page.evaluate(() => window.postbabyDebugSync());
      expect(titanDebugAfterSync.durablePendingCloudUpload).toBe(false);
      expect(titanDebugAfterSync.mutationOutbox.pendingCount).toBe(0);
      expect(titanDebugAfterSync.mutationOutbox.snapshotConfirmedCount).toBe(2);
      expect(titanDebugAfterSync.mutationOutbox.records.map((record) => record.status)).toEqual(['snapshotConfirmed', 'snapshotConfirmed']);
      expect(capturedSaveBodies).toHaveLength(1);
      LOCAL_ONLY_OUTBOX_KEYS.forEach((key) => {
        expect(Object.prototype.hasOwnProperty.call(capturedSaveBodies[0].data, key)).toBe(false);
      });

      await venom.page.reload();
      await expectNoteVisible(venom.page, 'Titan Quick Close Note');

      const venomDebugAfterReload = await venom.page.evaluate(() => window.postbabyDebugSync());
      expect(venomDebugAfterReload.syncStoredVersion).toBe(7);
    } finally {
      await titan.context.close();
      await venom.context.close();
    }
  });

  test('records CreateNode immediately and records a later rename as UpdateNode', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildEmptySnapshot({ hasRunBefore: true, theme: 'light', syncVersion: 6 }),
      {
        pending: false,
        revision: 6,
        lastCloudUploadedAt: TIMESTAMP
      }
    );

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerSnapshotPayload(buildEmptySnapshot({ hasRunBefore: true, theme: 'light' }), 6),
      onSave: async () => ({
        status: 503,
        body: { ok: false, error: { code: 'blocked_for_test', message: 'blocked for test' } }
      })
    });

    await page.goto('/index.html');
    const textarea = await openNewNoteEditorAt(page, { x: 760, y: 260 });

    let outbox = await readMutationOutbox(page);
    expect(outbox.pendingCount).toBe(1);
    expect(outbox.snapshotConfirmedCount).toBe(0);
    expect(outbox.records).toHaveLength(1);
    expect(outbox.records[0].operationType).toBe('CreateNode');
    expect(outbox.records[0].status).toBe('pending');
    expect(outbox.records[0].payload.name).toMatch(/^New Item /);

    await textarea.fill('Created Then Renamed');
    await textarea.press('Escape');
    await expect(textarea).toHaveCount(0);
    await expectNoteVisible(page, 'Created Then Renamed');

    outbox = await readMutationOutbox(page);
    expect(outbox.records.map((record) => record.operationType)).toEqual(['CreateNode', 'UpdateNode']);
    expect(outbox.records[1].status).toBe('pending');
    expect(outbox.records[1].payload).toEqual({
      tabId: 'tab-1',
      changes: {
        name: 'Created Then Renamed'
      }
    });
  });

  test('records one MoveNode envelope per completed move and keeps mutation ids unique across rapid operations', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshot('Move Outbox Note', { syncVersion: 6 }),
      {
        pending: false,
        revision: 6,
        lastCloudUploadedAt: TIMESTAMP
      }
    );

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Move Outbox Note', 6),
      onSave: async () => ({
        status: 503,
        body: { ok: false, error: { code: 'blocked_for_test', message: 'blocked for test' } }
      })
    });

    await page.goto('/index.html');
    const note = page.locator('.grid-item[data-id="item-1"]');
    await dragNoteBy(page, note, 80, 40);
    await dragNoteBy(page, note, 70, 35);

    const outbox = await readMutationOutbox(page);
    const moveRecords = outbox.records.filter((record) => record.operationType === 'MoveNode');
    expect(moveRecords).toHaveLength(2);
    expect(new Set(moveRecords.map((record) => record.mutationId)).size).toBe(2);
    expect(new Set(outbox.records.map((record) => record.mutationId)).size).toBe(outbox.records.length);
    expect(moveRecords.every((record) => record.status === 'pending')).toBe(true);
  });

  test('records CreateEdge once and records DeleteNode and DeleteEdge only after confirm commit', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshotWithItems([
        buildNoteItem('Delete Edge Source', {
          itemId: 'item-1',
          position: { top: '100px', left: '140px' }
        }),
        buildNoteItem('Delete Edge Target', {
          itemId: 'item-2',
          position: { top: '140px', left: '420px' }
        })
      ], {
        syncVersion: 6
      }),
      {
        pending: false,
        revision: 6,
        lastCloudUploadedAt: TIMESTAMP
      }
    );

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerSnapshotPayload(buildLocalSnapshotWithItems([
        buildNoteItem('Delete Edge Source', {
          itemId: 'item-1',
          position: { top: '100px', left: '140px' }
        }),
        buildNoteItem('Delete Edge Target', {
          itemId: 'item-2',
          position: { top: '140px', left: '420px' }
        })
      ]), 6),
      onSave: async () => ({
        status: 503,
        body: { ok: false, error: { code: 'blocked_for_test', message: 'blocked for test' } }
      })
    });

    await page.goto('/index.html');
    await drawEdgeBetweenNotes(page, page.locator('.grid-item[data-id="item-1"]'), page.locator('.grid-item[data-id="item-2"]'));
    let outbox = await readMutationOutbox(page);
    expect(outbox.records.filter((record) => record.operationType === 'CreateEdge')).toHaveLength(1);

    await openEdgeDeleteConfirm(page, page.locator('.edge-line').first());
    outbox = await readMutationOutbox(page);
    expect(outbox.records.filter((record) => record.operationType === 'DeleteEdge')).toHaveLength(0);
    await page.click('#confirmDelete');

    outbox = await readMutationOutbox(page);
    expect(outbox.records.filter((record) => record.operationType === 'DeleteEdge')).toHaveLength(1);

    await openNoteDeleteConfirm(page, 'item-1');
    outbox = await readMutationOutbox(page);
    expect(outbox.records.filter((record) => record.operationType === 'DeleteNode')).toHaveLength(0);
    await page.click('#confirmDelete');

    outbox = await readMutationOutbox(page);
    expect(outbox.records.filter((record) => record.operationType === 'DeleteNode')).toHaveLength(1);
  });

  test('records UpdateNode envelopes for rename, color, shape, and resize changes', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshot('Update Outbox Note', { syncVersion: 6 }),
      {
        pending: false,
        revision: 6,
        lastCloudUploadedAt: TIMESTAMP
      }
    );

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Update Outbox Note', 6),
      onSave: async () => ({
        status: 503,
        body: { ok: false, error: { code: 'blocked_for_test', message: 'blocked for test' } }
      })
    });

    await page.goto('/index.html');
    const note = page.locator('.grid-item[data-id="item-1"]');
    await note.dblclick();
    const textarea = page.locator('textarea.edit-textarea');
    await textarea.fill('Update Outbox Renamed');
    await textarea.press('Escape');
    await expect(textarea).toHaveCount(0);

    await note.click();
    await expect.poll(async () => {
      const currentOutbox = await readMutationOutbox(page);
      return currentOutbox.records.filter((record) => (
        record.operationType === 'UpdateNode'
        && record.payload
        && record.payload.changes
        && Object.prototype.hasOwnProperty.call(record.payload.changes, 'color')
      )).length;
    }).toBe(1);
    const updatedColor = await readItemColor(page, 'item-1');
    await cycleNoteShape(note);
    await resizeNoteBy(page, note, 80, 50);
    const updatedItem = await readItemSnapshot(page, 'item-1');

    const outbox = await readMutationOutbox(page);
    const updateRecords = outbox.records.filter((record) => record.operationType === 'UpdateNode');
    expect(updateRecords).toHaveLength(4);
    expect(updateRecords.map((record) => record.payload)).toEqual(expect.arrayContaining([
      {
        tabId: 'tab-1',
        changes: {
          name: 'Update Outbox Renamed'
        }
      },
      {
        tabId: 'tab-1',
        changes: {
          color: updatedColor
        }
      },
      {
        tabId: 'tab-1',
        changes: {
          shape: 'circle'
        }
      },
      {
        tabId: 'tab-1',
        changes: {
          width: updatedItem.width,
          height: updatedItem.height
        }
      }
    ]));
  });

  test('keeps pending envelopes across reload and marks them snapshotConfirmed after upload without uploading outbox metadata', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshot('Reload Pending Note', { syncVersion: 6 }),
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
      documentPayload: buildServerPayload('Reload Pending Note', 6),
      onSave: async (body) => {
        capturedSaveBody = body;
        return {
          status: 200,
          body: { ok: true, version: 7, updatedAt: TIMESTAMP }
        };
      }
    });

    await page.goto('/index.html');
    const note = page.locator('.grid-item[data-id="item-1"]');
    await dragNoteBy(page, note, 90, 40);

    let outbox = await readMutationOutbox(page);
    expect(outbox.pendingCount).toBe(1);
    const mutationIdBeforeReload = outbox.records[0].mutationId;

    await page.reload();
    outbox = await readMutationOutbox(page);
    expect(outbox.pendingCount).toBe(1);
    expect(outbox.records[0].mutationId).toBe(mutationIdBeforeReload);
    expect(outbox.records[0].status).toBe('pending');

    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();
    await expect.poll(() => capturedSaveBody !== null).toBe(true);

    outbox = await readMutationOutbox(page);
    expect(outbox.pendingCount).toBe(0);
    expect(outbox.snapshotConfirmedCount).toBe(1);
    expect(outbox.records[0].mutationId).toBe(mutationIdBeforeReload);
    expect(outbox.records[0].status).toBe('snapshotConfirmed');
    LOCAL_ONLY_OUTBOX_KEYS.forEach((key) => {
      expect(Object.prototype.hasOwnProperty.call(capturedSaveBody.data, key)).toBe(false);
    });
  });

  test('snapshot conflict keeps retryable envelopes and excludes outbox metadata from the uploaded document payload', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshot('Conflict Pending Note', { syncVersion: 6 }),
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
      documentPayload: buildServerPayload('Conflict Pending Note', 6),
      onSave: async (body) => {
        capturedSaveBody = body;
        return {
          status: 409,
          body: {
            ok: false,
            error: {
              code: 'version_conflict',
              currentVersion: 7,
              message: 'forced version conflict'
            }
          }
        };
      }
    });

    await page.goto('/index.html');
    await dragNoteBy(page, page.locator('.grid-item[data-id="item-1"]'), 70, 35);
    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();
    await expect.poll(() => capturedSaveBody !== null).toBe(true);

    const outbox = await readMutationOutbox(page);
    expect(outbox.pendingCount).toBe(1);
    expect(outbox.snapshotConflictCount).toBe(1);
    expect(outbox.records[0].status).toBe('snapshotConflict');
    LOCAL_ONLY_OUTBOX_KEYS.forEach((key) => {
      expect(Object.prototype.hasOwnProperty.call(capturedSaveBody.data, key)).toBe(false);
    });
  });

  test('generates unique mutation ids across same-context tabs opened from the same starting counter', async ({ browser }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildEmptySnapshot({ hasRunBefore: true, theme: 'light', syncVersion: 6 }),
      {
        pending: false,
        revision: 6,
        lastCloudUploadedAt: TIMESTAMP
      }
    );
    const context = await browser.newContext({
      baseURL: TEST_BASE_URL,
      serviceWorkers: 'block'
    });

    try {
      await enableDynamicMockedSync(context, {
        runtimeOverrides: {
          isAuthenticated: true
        },
        getMetaPayload: () => ({
          ok: true,
          exists: true,
          version: 6,
          updatedAt: TIMESTAMP
        }),
        getDocumentPayload: () => buildServerSnapshotPayload(buildEmptySnapshot({ hasRunBefore: true, theme: 'light' }), 6),
        onSave: async () => ({
          status: 503,
          body: { ok: false, error: { code: 'blocked_for_test', message: 'blocked for test' } }
        })
      });

      const firstPage = await context.newPage();
      await prepareBlankPage(firstPage);
      await seedLocalStorage(firstPage, localSnapshot, 'owner-scope');
      await seedIndexedDB(firstPage, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');

      const secondPage = await context.newPage();
      await Promise.all([
        firstPage.goto('/index.html'),
        secondPage.goto('/index.html')
      ]);

      await openNewNoteEditorAt(firstPage, { x: 720, y: 250 });
      await openNewNoteEditorAt(secondPage, { x: 780, y: 300 });

      const firstOutbox = await readMutationOutbox(firstPage);
      const secondOutbox = await readMutationOutbox(secondPage);
      const firstCreateId = firstOutbox.records.find((record) => record.operationType === 'CreateNode').mutationId;
      const secondCreateId = secondOutbox.records.find((record) => record.operationType === 'CreateNode').mutationId;

      expect(firstCreateId).not.toBe(secondCreateId);
    } finally {
      await context.close();
    }
  });

  test('sends pending local mutations to the receipt endpoint and marks accepted results serverAcked', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshot('Ack Pending Note', { syncVersion: 6 }),
      {
        pending: false,
        revision: 6,
        lastCloudUploadedAt: TIMESTAMP
      }
    );
    const capturedMutationBodies = [];
    const capturedSaveBodies = [];

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Ack Pending Note', 6),
      onMutations: async (body) => {
        capturedMutationBodies.push(body);
        return {
          status: 200,
          body: {
            ok: true,
            appId: body.appId,
            results: body.mutations.map((mutation) => ({
              mutationId: mutation.mutationId,
              status: 'accepted',
              duplicate: false,
              acceptedAt: TIMESTAMP
            }))
          }
        };
      },
      onSave: async (body) => {
        capturedSaveBodies.push(body);
        return {
          status: 200,
          body: { ok: true, version: 7, updatedAt: TIMESTAMP }
        };
      }
    });

    await page.goto('/index.html');
    await dragNoteBy(page, page.locator('.grid-item[data-id="item-1"]'), 80, 40);
    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();

    await expect.poll(() => capturedMutationBodies.length).toBe(1);
    await expect.poll(() => capturedSaveBodies.length).toBe(1);
    await expect.poll(async () => {
      const outbox = await readMutationOutbox(page);
      return outbox.serverAckedCount;
    }).toBe(1);

    const outbox = await readMutationOutbox(page);
    expect(outbox.totalCount).toBe(1);
    expect(outbox.serverPendingCount).toBe(0);
    expect(outbox.serverAckedCount).toBe(1);
    expect(outbox.records[0].status).toBe('serverAcked');
    expect(capturedMutationBodies[0].appId).toBe('postbaby-web');
    expect(capturedMutationBodies[0].mutations).toHaveLength(1);
    expect(capturedMutationBodies[0].mutations[0].operationType).toBe('MoveNode');
    expect(capturedMutationBodies[0].mutations[0].mutationId).toBe(outbox.records[0].mutationId);
    expect(capturedMutationBodies[0].mutations[0].payload).toHaveProperty('position');
    LOCAL_ONLY_OUTBOX_KEYS.forEach((key) => {
      expect(Object.prototype.hasOwnProperty.call(capturedSaveBodies[0].data, key)).toBe(false);
    });
    expect(Object.prototype.hasOwnProperty.call(capturedSaveBodies[0], 'mutations')).toBe(false);
  });

  test('duplicate receipt ACK responses do not create duplicate client-side state', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshot('Duplicate Ack Note', { syncVersion: 6 }),
      {
        pending: false,
        revision: 6,
        lastCloudUploadedAt: TIMESTAMP
      }
    );

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Duplicate Ack Note', 6),
      onMutations: async (body) => ({
        status: 200,
        body: {
          ok: true,
          appId: body.appId,
          results: body.mutations.map((mutation) => ({
            mutationId: mutation.mutationId,
            status: 'accepted',
            duplicate: true,
            acceptedAt: TIMESTAMP
          }))
        }
      }),
      onSave: async () => ({
        status: 200,
        body: { ok: true, version: 7, updatedAt: TIMESTAMP }
      })
    });

    await page.goto('/index.html');
    await dragNoteBy(page, page.locator('.grid-item[data-id="item-1"]'), 75, 35);
    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();

    await expect.poll(async () => {
      const outbox = await readMutationOutbox(page);
      return outbox.serverAckedCount;
    }).toBe(1);

    const outbox = await readMutationOutbox(page);
    expect(outbox.totalCount).toBe(1);
    expect(outbox.records[0].status).toBe('serverAcked');
  });

  test('receipt validation rejections mark local mutations serverNacked', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshot('Rejected Ack Note', { syncVersion: 6 }),
      {
        pending: false,
        revision: 6,
        lastCloudUploadedAt: TIMESTAMP
      }
    );

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Rejected Ack Note', 6),
      onMutations: async (body) => ({
        status: 200,
        body: {
          ok: true,
          appId: body.appId,
          results: body.mutations.map((mutation) => ({
            mutationId: mutation.mutationId,
            status: 'rejected',
            error: {
              code: 'invalid_payload',
              message: 'payload must be a JSON object within the allowed size limit'
            }
          }))
        }
      }),
      onSave: async () => ({
        status: 200,
        body: { ok: true, version: 7, updatedAt: TIMESTAMP }
      })
    });

    await page.goto('/index.html');
    await dragNoteBy(page, page.locator('.grid-item[data-id="item-1"]'), 55, 25);
    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();

    await expect.poll(async () => {
      const outbox = await readMutationOutbox(page);
      return outbox.serverNackedCount;
    }).toBe(1);

    const outbox = await readMutationOutbox(page);
    expect(outbox.totalCount).toBe(1);
    expect(outbox.serverAckedCount).toBe(0);
    expect(outbox.serverPendingCount).toBe(0);
    expect(outbox.records[0].status).toBe('serverNacked');
    expect(outbox.records[0].statusReason).toContain('invalid_payload');
  });

  test('failed receipt ACK requests keep receipts retryable while snapshot upload still succeeds', async ({ page }) => {
    const localSnapshot = addDurableCloudSyncMetadata(
      buildLocalSnapshot('Ack Retry Note', { syncVersion: 6 }),
      {
        pending: false,
        revision: 6,
        lastCloudUploadedAt: TIMESTAMP
      }
    );
    const capturedMutationBodies = [];
    const capturedSaveBodies = [];
    let mutationAttempt = 0;

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot, 'owner-scope');
    await seedIndexedDB(page, localSnapshot, buildIndexedDBMeta(localSnapshot, { id: 'owner-scope' }), 'owner-scope');
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 6, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Ack Retry Note', 6),
      onMutations: async (body) => {
        mutationAttempt += 1;
        capturedMutationBodies.push(body);
        if (mutationAttempt < 2) {
          return {
            status: 503,
            body: {
              ok: false,
              error: {
                code: 'server_unavailable',
                message: 'server unavailable'
              }
            }
          };
        }
        return {
          status: 200,
          body: {
            ok: true,
            appId: body.appId,
            results: body.mutations.map((mutation) => ({
              mutationId: mutation.mutationId,
              status: 'accepted',
              duplicate: mutationAttempt > 3,
              acceptedAt: TIMESTAMP
            }))
          }
        };
      },
      onSave: async (body) => {
        capturedSaveBodies.push(body);
        return {
          status: 200,
          body: { ok: true, version: 7 + capturedSaveBodies.length - 1, updatedAt: TIMESTAMP }
        };
      }
    });

    await page.goto('/index.html');
    await dragNoteBy(page, page.locator('.grid-item[data-id="item-1"]'), 65, 30);
    await page.locator('#syncStatusButton').click();
    await page.locator('#syncNowButton').click();

    await expect.poll(() => capturedSaveBodies.length).toBe(1);
    await expect.poll(() => capturedMutationBodies.length).toBe(1);

    let outbox = await readMutationOutbox(page);
    expect(outbox.snapshotConfirmedCount).toBe(1);
    expect(outbox.serverAckedCount).toBe(0);
    expect(outbox.serverPendingCount).toBe(1);
    expect(outbox.records[0].status).toBe('snapshotConfirmed');
    LOCAL_ONLY_OUTBOX_KEYS.forEach((key) => {
      expect(Object.prototype.hasOwnProperty.call(capturedSaveBodies[0].data, key)).toBe(false);
    });

    await page.locator('#syncNowButton').click();
    await expect.poll(() => capturedMutationBodies.length).toBe(2);
    await expect.poll(async () => {
      const currentOutbox = await readMutationOutbox(page);
      return currentOutbox.serverAckedCount;
    }).toBe(1);

    outbox = await readMutationOutbox(page);
    expect(outbox.totalCount).toBe(1);
    expect(outbox.serverPendingCount).toBe(0);
    expect(outbox.records[0].status).toBe('serverAcked');
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

  test('static UI shows the gear plus the hand/select toggle and includes the local-only settings note', async ({ page }) => {
    await prepareBlankPage(page);
    await page.goto('/index.html');

    await expect(page.locator('#settingsButton .top-app-settings-icon')).toBeVisible();
    await expect(page.locator('#canvasModeToggleButton')).toBeVisible();
    await expect(page.locator('#topAppControls button:visible')).toHaveCount(2);
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

  test('Settings modal splits Preferences and Import & Export while keeping recovery controls hidden by default', async ({ page }) => {
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
    await expect(page.getByRole('tab', { name: 'Preferences' })).toHaveAttribute('aria-selected', 'true');
    await expect(page.getByRole('tab', { name: 'Import & Export' })).toHaveAttribute('aria-selected', 'false');
    await expect(page.locator('#settingsPreferencesPanel')).toBeVisible();
    await expect(page.locator('#settingsImportExportPanel')).toBeHidden();
    await expect(page.locator('#settingsPreferencesPanel .settings-option').filter({ hasText: 'Dark Mode' })).toBeVisible();
    await expect(page.locator('#settingsRecoverySection')).toBeHidden();
    await expect(page.locator('#showAllItemsButton')).toBeHidden();
    await expect(page.locator('#jumpNewestItemButton')).toBeHidden();
    await expect(page.locator('#jumpLastEditedItemButton')).toBeHidden();
    await expect(page.locator('#saveDataButton')).toBeHidden();
    await expect(page.locator('#loadDataButton')).toBeHidden();
    await expect(page.locator('#mermaidImportSource')).toBeHidden();
    await expect(page.locator('#settingsModal .settings-sync-panel')).toHaveCount(0);
    await expect(page.locator('#settingsModal #logoutForm')).toHaveCount(0);
    await expect(page.locator('#settingsModal #syncStateStatus')).toHaveCount(0);

    await switchSettingsTab(page, 'Import & Export');
    await expect(page.locator('#settingsPreferencesPanel')).toBeHidden();
    await expect(page.locator('#settingsImportExportPanel')).toBeVisible();
    await expect(page.locator('#saveDataButton')).toBeVisible();
    await expect(page.locator('#loadDataButton')).toBeVisible();
    await expect(page.locator('#settingsImportExportPanel .settings-modal-divider')).toBeVisible();
    await expect(page.locator('#mermaidImportSource')).toBeVisible();
    await expect(page.locator('#mermaidImportButton')).toBeVisible();
    await expect(page.locator('#mermaidImportStatus')).toBeHidden();
    await expect(page.locator('#settingsRecoverySection')).toBeHidden();
    await expect(page.locator('#showAllItemsButton')).toBeHidden();
    await expect(page.locator('#jumpNewestItemButton')).toBeHidden();
    await expect(page.locator('#jumpLastEditedItemButton')).toBeHidden();

    await switchSettingsTab(page, 'Preferences');
    await expect(page.locator('#staticSettingsNote')).toBeHidden();
  });

  test('Recovery debug flag reveals the development-only recovery section in Settings', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    await page.evaluate(() => {
      window.localStorage.setItem('postbabyDebugRecovery', 'true');
    });

    await openSettingsModal(page);
    await expect(page.locator('#settingsRecoverySection')).toBeVisible();
    await expect(page.locator('#settingsRecoveryTitle')).toHaveText('Recovery (Development Only)');
    await expect(page.locator('#showAllItemsButton')).toBeVisible();
    await expect(page.locator('#jumpNewestItemButton')).toBeVisible();
    await expect(page.locator('#jumpLastEditedItemButton')).toBeVisible();
  });

  test('Show All Notes recovery hook recovers far-away notes after reload without mutating coordinates', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Far Away Recovery', {
        itemId: 'far-item',
        position: { top: '2200px', left: '2400px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');
    await page.reload();

    const beforeSnapshot = await readTabsSnapshot(page);
    const beforeRect = await readItemClientRect(page, 'far-item');
    const beforeViewport = await readWorkspaceScroll(page);
    expect(beforeRect.left).toBeGreaterThan(beforeViewport.width);
    expect(beforeRect.top).toBeGreaterThan(beforeViewport.height);
    expect(await readCamera(page)).toEqual(DEFAULT_CAMERA);

    await openSettingsModal(page);
    await showAllItemsForTest(page);
    await expect(page.locator('#settingsModal')).toBeHidden();

    const windowScroll = await readWindowScroll(page);
    expect(windowScroll.x).toBe(0);
    expect(windowScroll.y).toBe(0);
    expect(windowScroll.documentX).toBe(0);
    expect(windowScroll.documentY).toBe(0);

    await expect.poll(async () => {
      const rect = await readItemClientRect(page, 'far-item');
      const viewport = await readWorkspaceScroll(page);
      return rect.left < viewport.width && rect.right > 0 && rect.top < viewport.height && rect.bottom > 0;
    }).toBe(true);

    const headerBox = await page.locator('.tab-bar-header').boundingBox();
    if (!headerBox) {
      throw new Error('Tab bar header bounding box was not available after recovery.');
    }
    expect(headerBox.y).toBeGreaterThanOrEqual(0);

    expect(await readTabsSnapshot(page)).toEqual(beforeSnapshot);
  });

  test('Jump To Newest recovery hook uses stable item order fallback and does not mutate coordinates', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Older Note', {
        itemId: 'older-item',
        position: { top: '24px', left: '24px' }
      }),
      buildNoteItem('Newest Fallback', {
        itemId: 'newest-item',
        position: { top: '2000px', left: '2300px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const beforeSnapshot = await readTabsSnapshot(page);
    await jumpNewestItemForTest(page);

    await expect.poll(async () => {
      const rect = await readItemClientRect(page, 'newest-item');
      const viewport = await readWorkspaceScroll(page);
      return rect.left < viewport.width && rect.right > 0 && rect.top < viewport.height && rect.bottom > 0;
    }).toBe(true);

    expect(await readTabsSnapshot(page)).toEqual(beforeSnapshot);
  });

  test('Jump To Last Edited recovery hook uses local session edits and does not mutate stored coordinates during recovery', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Near Note', {
        itemId: 'near-item',
        position: { top: '24px', left: '24px' }
      }),
      buildNoteItem('Far Edited Note', {
        itemId: 'far-edited-item',
        position: { top: '2200px', left: '2400px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    await openSettingsModal(page);
    await showAllItemsForTest(page);
    await expect(page.locator('#settingsModal')).toBeHidden();
    await page.locator('.grid-item[data-id="far-edited-item"]').click();
    await page.waitForTimeout(350);
    const afterEditSnapshot = await readTabsSnapshot(page);

    await resetWorkspaceScroll(page);
    await jumpLastEditedItemForTest(page);

    await expect.poll(async () => {
      const rect = await readItemClientRect(page, 'far-edited-item');
      const viewport = await readWorkspaceScroll(page);
      return rect.left < viewport.width && rect.right > 0 && rect.top < viewport.height && rect.bottom > 0;
    }).toBe(true);

    expect(await readTabsSnapshot(page)).toEqual(afterEditSnapshot);
  });

  test('Jump To Newest recovery hook does not overwrite the session last edited note', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Last Edited Anchor', {
        itemId: 'edited-item',
        position: { top: '24px', left: '24px' }
      }),
      buildNoteItem('Newest Offscreen Note', {
        itemId: 'newest-item',
        position: { top: '2100px', left: '2400px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    await page.locator('.grid-item[data-id="edited-item"]').click();
    await page.waitForTimeout(350);
    const afterEditSnapshot = await readTabsSnapshot(page);

    await jumpNewestItemForTest(page);

    await expect.poll(async () => {
      const rect = await readItemClientRect(page, 'newest-item');
      const viewport = await readWorkspaceScroll(page);
      return rect.left < viewport.width && rect.right > 0 && rect.top < viewport.height && rect.bottom > 0;
    }).toBe(true);

    await jumpLastEditedItemForTest(page);

    await expect.poll(async () => {
      const rect = await readItemClientRect(page, 'edited-item');
      const viewport = await readWorkspaceScroll(page);
      return rect.left < viewport.width && rect.right > 0 && rect.top < viewport.height && rect.bottom > 0;
    }).toBe(true);

    expect(await readTabsSnapshot(page)).toEqual(afterEditSnapshot);
  });

  test('Recovery hooks show safe toasts when the active tab has no notes', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    await showAllItemsForTest(page);
    await expect(page.locator('.toast').last()).toContainText('No notes to recover in this tab.');

    await jumpNewestItemForTest(page);
    await expect(page.locator('.toast').last()).toContainText('No notes to recover in this tab.');

    await jumpLastEditedItemForTest(page);
    await expect(page.locator('.toast').last()).toContainText('No edited note found yet in this tab.');

    const tabsSnapshot = await readTabsSnapshot(page);
    expect(tabsSnapshot[0].items || []).toHaveLength(0);
  });

  test('deleting far-away content keeps camera recovery stable and clears stale last-edited state', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Near Survivor', {
        itemId: 'near-item',
        position: { top: '24px', left: '24px' }
      }),
      buildNoteItem('Far Deleted Note', {
        itemId: 'far-item',
        position: { top: '2400px', left: '2600px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    const beforeNearPosition = await readItemPositionsById(page, ['near-item']);

    await showAllItemsForTest(page);
    const farNote = page.locator('.grid-item[data-id="far-item"]');
    await expect(farNote).toBeVisible();
    await farNote.click();
    await page.waitForTimeout(350);
    await farNote.click({ button: 'right' });
    await expect(page.locator('#confirmModal')).toBeVisible();
    await page.click('#confirmDelete');
    await expect(farNote).toHaveCount(0);

    expect(await readItemPositionsById(page, ['near-item'])).toEqual(beforeNearPosition);

    await jumpLastEditedItemForTest(page);
    await expect(page.locator('.toast').last()).toContainText('No edited note found yet in this tab.');

    await showAllItemsForTest(page);
    await expect.poll(async () => {
      const rect = await readItemClientRect(page, 'near-item');
      const viewport = await readWindowScroll(page);
      return rect.left < viewport.width && rect.right > 0 && rect.top < viewport.height && rect.bottom > 0;
    }).toBe(true);
  });

  test('switching tabs keeps camera recovery scoped to the active tab only', async ({ page }) => {
    const localSnapshot = {
      tabs: JSON.stringify([
        {
          id: 'tab-1',
          name: '1',
          items: [
            buildNoteItem('Tab One Far Note', {
              itemId: 'tab-one-far',
              position: { top: '2200px', left: '2500px' }
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
            buildNoteItem('Tab Two Near Note', {
              itemId: 'tab-two-near',
              position: { top: '24px', left: '24px' }
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

    await showAllItemsForTest(page);
    await expect.poll(async () => {
      const rect = await readItemClientRect(page, 'tab-one-far');
      const viewport = await readWorkspaceScroll(page);
      return rect.left < viewport.width && rect.right > 0 && rect.top < viewport.height && rect.bottom > 0;
    }).toBe(true);

    await page.locator('.tab[data-tab-id="tab-2"]').click();
    await expectNoteVisible(page, 'Tab Two Near Note');
    expect(await readCamera(page, 'tab-2')).toEqual(DEFAULT_CAMERA);

    await page.locator('.tab[data-tab-id="tab-1"]').click();
    await showAllItemsForTest(page);

    await expect.poll(async () => {
      const rect = await readItemClientRect(page, 'tab-one-far');
      const viewport = await readWorkspaceScroll(page);
      return rect.left < viewport.width && rect.right > 0 && rect.top < viewport.height && rect.bottom > 0;
    }).toBe(true);
  });

  test('Show All Notes recovery hook recovers graph-created notes without rewriting stored positions', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const graphResult = await createGraphForTest(page, buildNormalizedGraph([
      buildGraphNode('A', { label: 'Graph Source' }),
      buildGraphNode('B', { label: 'Graph Target', shape: 'circle' })
    ], [
      buildGraphEdge('A', 'B', { kind: 'arrow' })
    ], {
      originX: 2300,
      originY: 1800,
      direction: 'LR'
    }));

    expect(graphResult.ok).toBe(true);
    const beforePositions = await readItemPositionsById(page, graphResult.createdItemIds);
    await resetWorkspaceScroll(page);

    await showAllItemsForTest(page);

    await expect.poll(async () => {
      const rect = await readItemClientRect(page, graphResult.createdItemIds[0]);
      const viewport = await readWorkspaceScroll(page);
      return rect.left < viewport.width && rect.right > 0 && rect.top < viewport.height && rect.bottom > 0;
    }).toBe(true);

    expect(await readItemPositionsById(page, graphResult.createdItemIds)).toEqual(beforePositions);

    const graphSource = page.locator(`.grid-item[data-id="${graphResult.createdItemIds[0]}"]`);
    await dragNoteBy(page, graphSource, 90, 45);
    await page.waitForTimeout(350);
    await expect(page.locator('.edge-line')).toHaveCount(1);
    expect(await readEdgePresentation(page.locator('.edge-line').first())).toMatchObject({
      markerEnd: expect.stringMatching(/^url\(#.+-arrow\)$/)
    });
  });

  test('Jump To Newest recovery hook recovers Mermaid-created notes without rewriting stored positions', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const mermaidResult = await createGraphFromMermaidForTest(page, [
      'flowchart LR',
      'A[Mermaid Source] --> B((Mermaid Target))'
    ].join('\n'), {
      originX: 2200,
      originY: 1900
    });

    expect(mermaidResult.ok).toBe(true);
    const newestCreatedItemId = mermaidResult.createdItemIds[mermaidResult.createdItemIds.length - 1];
    const beforePositions = await readItemPositionsById(page, mermaidResult.createdItemIds);
    await resetWorkspaceScroll(page);

    await jumpNewestItemForTest(page);

    await expect.poll(async () => {
      const rect = await readItemClientRect(page, newestCreatedItemId);
      const viewport = await readWorkspaceScroll(page);
      return rect.left < viewport.width && rect.right > 0 && rect.top < viewport.height && rect.bottom > 0;
    }).toBe(true);

    expect(await readItemPositionsById(page, mermaidResult.createdItemIds)).toEqual(beforePositions);

    const mermaidSource = page.locator(`.grid-item[data-id="${mermaidResult.createdItemIds[0]}"]`);
    await dragNoteBy(page, mermaidSource, 80, 40);
    await page.waitForTimeout(350);
    await expect(page.locator('.edge-line')).toHaveCount(1);
    expect(await readEdgePresentation(page.locator('.edge-line').first())).toMatchObject({
      markerEnd: expect.stringMatching(/^url\(#.+-arrow\)$/)
    });
  });

  test('Show All Notes recovery hook leaves manual note creation, drag, resize, and edge creation usable', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Recovered Source', {
        itemId: 'source-item',
        position: { top: '2100px', left: '2200px' }
      }),
      buildNoteItem('Recovered Target', {
        itemId: 'target-item',
        position: { top: '2280px', left: '2520px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    await showAllItemsForTest(page);

    const sourceNote = page.locator('.grid-item[data-id="source-item"]');
    const targetNote = page.locator('.grid-item[data-id="target-item"]');
    const beforeSource = await readItemSnapshot(page, 'source-item');
    const beforeTarget = await readItemSnapshot(page, 'target-item');

    await dragNoteBy(page, sourceNote, 120, 60);
    await page.waitForTimeout(350);
    const afterSource = await readItemSnapshot(page, 'source-item');
    expect(afterSource.position).not.toEqual(beforeSource.position);

    await resizeNoteBy(page, targetNote, 90, 50);
    await page.waitForTimeout(350);
    const afterTarget = await readItemSnapshot(page, 'target-item');
    expect(afterTarget.width).not.toBe(beforeTarget.width);
    expect(afterTarget.height).not.toBe(beforeTarget.height);

    await drawEdgeBetweenNotes(page, sourceNote, targetNote, { modifier: 'Control' });
    await expect(page.locator('.edge-line')).toHaveCount(1);

    const grid = page.locator('.grid-container').first();
    await grid.evaluate((element) => {
      element.dispatchEvent(new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        button: 2,
        clientX: 900,
        clientY: 320
      }));
    });

    const textarea = page.locator('textarea.edit-textarea');
    await expect(textarea).toBeVisible();
    await textarea.fill('Manual Note After Recovery');
    await textarea.press('Escape');
    await expectNoteVisible(page, 'Manual Note After Recovery');
  });

  test('clearing far-away graph-created content leaves camera recovery safe after recovery', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const graphResult = await createGraphForTest(page, buildNormalizedGraph([
      buildGraphNode('A', { label: 'Graph Clear A' }),
      buildGraphNode('B', { label: 'Graph Clear B' })
    ], [
      buildGraphEdge('A', 'B', { kind: 'arrow' })
    ], {
      originX: 2500,
      originY: 2200,
      direction: 'LR'
    }));

    expect(graphResult.ok).toBe(true);

    await showAllItemsForTest(page);
    const recoveredCamera = await readCamera(page);
    expect(recoveredCamera.x).toBeGreaterThan(0);
    expect(recoveredCamera.y).toBeGreaterThan(0);

    await expect.poll(async () => {
      const rect = await readItemClientRect(page, graphResult.createdItemIds[0]);
      const viewport = await readWindowScroll(page);
      return rect.left < viewport.width && rect.right > 0 && rect.top < viewport.height && rect.bottom > 0;
    }).toBe(true);

    await page.keyboard.press('c');
    await expect(page.locator('#confirmModal')).toBeVisible();
    await page.click('#confirmDelete');
    await page.waitForLoadState('domcontentloaded');
    await expect(page.locator('.grid-item')).toHaveCount(0);

    await showAllItemsForTest(page);
    await expect(page.locator('.toast').last()).toContainText('No notes to recover in this tab.');
  });

  test('camera-panned note creation keeps right-click and Add Item placement inside the visible workspace', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Scroll Anchor', {
        itemId: 'scroll-anchor',
        position: { top: '2400px', left: '2600px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    await scrollWorkspaceTo(page, 1800, 1600);
    await expect.poll(async () => {
      const scroll = await readWorkspaceScroll(page);
      return scroll.x > 0 && scroll.y > 0;
    }).toBe(true);
    const scrolledWorkspace = await readWorkspaceScroll(page);

    const workspaceBox = await page.locator('#tabContent').boundingBox();
    if (!workspaceBox) {
      throw new Error('Workspace viewport bounding box was not available.');
    }

    const createClientPoint = {
      clientX: Math.round(workspaceBox.x + 180),
      clientY: Math.round(workspaceBox.y + 160)
    };
    const expectedGridPoint = await page.evaluate((point) => {
      const workspace = document.getElementById('tabContent');
      if (!workspace) {
        throw new Error('Workspace viewport was not available.');
      }

      return window.PostbabyCanvasCamera.clientPointToWorldPoint(
        point.clientX,
        point.clientY,
        workspace,
        window.postbabyGetCameraForTest()
      );
    }, createClientPoint);

    await page.locator('.grid-container').first().evaluate((element, point) => {
      element.dispatchEvent(new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        button: 2,
        clientX: point.clientX,
        clientY: point.clientY
      }));
    }, createClientPoint);

    let textarea = page.locator('textarea.edit-textarea');
    await expect(textarea).toBeVisible();
    await textarea.fill('Scrolled Context Note');
    await textarea.press('Escape');
    await expectNoteVisible(page, 'Scrolled Context Note');

    const contextItem = (await readTabsSnapshot(page))[0].items.find((item) => item.name === 'Scrolled Context Note');
    expect(contextItem).toBeTruthy();
    const contextPosition = await page.evaluate((item) => {
      return window.PostbabyGeometryDom.getItemPositionXY(item);
    }, contextItem);
    expect(Math.abs(contextPosition.x - expectedGridPoint.x)).toBeLessThanOrEqual(4);
    expect(Math.abs(contextPosition.y - expectedGridPoint.y)).toBeLessThanOrEqual(4);

    await page.evaluate(() => {
      const addButton = document.querySelector('.tab-pane.active .addItemButton');
      if (!(addButton instanceof HTMLButtonElement)) {
        throw new Error('Active Add Item button was not available.');
      }
      addButton.click();
    });

    textarea = page.locator('textarea.edit-textarea');
    await expect(textarea).toBeVisible();
    await textarea.fill('Scrolled Add Button Note');
    await textarea.press('Escape');
    await expectNoteVisible(page, 'Scrolled Add Button Note');

    const addButtonItem = (await readTabsSnapshot(page))[0].items.find((item) => item.name === 'Scrolled Add Button Note');
    expect(addButtonItem).toBeTruthy();
    const addButtonPosition = await page.evaluate((item) => {
      return window.PostbabyGeometryDom.getItemPositionXY(item);
    }, addButtonItem);
    expect(addButtonPosition.x).toBeGreaterThanOrEqual(scrolledWorkspace.x);
    expect(addButtonPosition.x).toBeLessThanOrEqual(scrolledWorkspace.x + scrolledWorkspace.width);
    expect(addButtonPosition.y).toBeGreaterThanOrEqual(scrolledWorkspace.y);
    expect(addButtonPosition.y).toBeLessThanOrEqual(scrolledWorkspace.y + scrolledWorkspace.height);
  });

  test('workspace-scrolled drag, resize, edge creation, marquee selection, and context deletion keep using workspace coordinates', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Scrolled Source', {
        itemId: 'scroll-source',
        position: { top: '2200px', left: '2200px' }
      }),
      buildNoteItem('Scrolled Target', {
        itemId: 'scroll-target',
        position: { top: '2240px', left: '2520px' }
      }),
      buildNoteItem('Scrolled Outside', {
        itemId: 'scroll-outside',
        position: { top: '2500px', left: '2860px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    await scrollWorkspaceTo(page, 1900, 1800);
    await expect.poll(async () => {
      const scroll = await readWorkspaceScroll(page);
      return scroll.x > 0 && scroll.y > 0;
    }).toBe(true);

    const sourceNote = page.locator('.grid-item[data-id="scroll-source"]');
    const targetNote = page.locator('.grid-item[data-id="scroll-target"]');
    const outsideNote = page.locator('.grid-item[data-id="scroll-outside"]');
    const sourceBefore = await readItemPositionViaGeometryDom(page, 'scroll-source');
    const targetBeforeSize = await readNoteSize(targetNote);

    await dragNoteBy(page, sourceNote, 120, 60);
    await page.waitForTimeout(350);
    const sourceAfter = await readItemPositionViaGeometryDom(page, 'scroll-source');
    expect(sourceAfter.x).toBe(sourceBefore.x + 120);
    expect(sourceAfter.y).toBe(sourceBefore.y + 60);

    await resizeNoteBy(page, targetNote, 80, 50);
    await page.waitForTimeout(350);
    const targetAfterSize = await readNoteSize(targetNote);
    expect(targetAfterSize.width).toBeGreaterThan(targetBeforeSize.width);
    expect(targetAfterSize.height).toBeGreaterThan(targetBeforeSize.height);

    await drawEdgeBetweenNotes(page, sourceNote, targetNote, { modifier: 'Control' });
    await expect(page.locator('.edge-line')).toHaveCount(1);

    await marqueeSelectNotes(page, [sourceNote, targetNote], { margin: 8 });
    await expect(page.locator('.grid-item.selected')).toHaveCount(2);
    await expect(sourceNote).toHaveClass(/selected/);
    await expect(targetNote).toHaveClass(/selected/);
    await expect(outsideNote).not.toHaveClass(/selected/);

    await page.keyboard.press('Escape');
    await expect(page.locator('.grid-item.selected')).toHaveCount(0);

    await targetNote.click({ button: 'right' });
    await expect(page.locator('#confirmModal')).toBeVisible();
    await page.click('#confirmDelete');
    await expect(targetNote).toHaveCount(0);
    await expect(page.locator('.edge-line')).toHaveCount(0);
    await expect(sourceNote).toBeVisible();
  });

  test('camera-zoomed creation, drag, resize, edge creation, and marquee selection stay world-correct', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Zoom Source', {
        itemId: 'zoom-source',
        position: { top: '640px', left: '620px' }
      }),
      buildNoteItem('Zoom Target', {
        itemId: 'zoom-target',
        position: { top: '720px', left: '860px' }
      }),
      buildNoteItem('Zoom Outside', {
        itemId: 'zoom-outside',
        position: { top: '1120px', left: '1400px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    await setCamera(page, { x: 520, y: 540, zoom: 2 });
    const sourceNote = page.locator('.grid-item[data-id="zoom-source"]');
    const targetNote = page.locator('.grid-item[data-id="zoom-target"]');
    const outsideNote = page.locator('.grid-item[data-id="zoom-outside"]');
    const sourceBefore = await readItemPositionViaGeometryDom(page, 'zoom-source');
    const targetBeforeSize = await readNoteSize(targetNote);

    const workspaceBox = await page.locator('#tabContent').boundingBox();
    if (!workspaceBox) {
      throw new Error('Workspace viewport bounding box was not available.');
    }

    await dragNoteBy(page, sourceNote, 120, 60);
    await page.waitForTimeout(350);
    const sourceAfter = await readItemPositionViaGeometryDom(page, 'zoom-source');
    expect(sourceAfter.x).toBeCloseTo(sourceBefore.x + 60, 0);
    expect(sourceAfter.y).toBeCloseTo(sourceBefore.y + 30, 0);

    await resizeNoteBy(page, targetNote, 80, 40);
    await page.waitForTimeout(350);
    const targetAfterSize = await readNoteSize(targetNote);
    expect(targetAfterSize.width).toBeGreaterThan(targetBeforeSize.width + 30);
    expect(targetAfterSize.height).toBeGreaterThan(targetBeforeSize.height + 10);

    await drawEdgeBetweenNotes(page, sourceNote, targetNote, { modifier: 'Control' });
    await expect(page.locator('.edge-line')).toHaveCount(1);

    await marqueeSelectNotes(page, [sourceNote, targetNote], { margin: 8 });
    await expect(page.locator('.grid-item.selected')).toHaveCount(2);
    await expect(sourceNote).toHaveClass(/selected/);
    await expect(targetNote).toHaveClass(/selected/);
    await expect(outsideNote).not.toHaveClass(/selected/);

    const createClientPoint = {
      clientX: Math.round(workspaceBox.x + workspaceBox.width - 180),
      clientY: Math.round(workspaceBox.y + 120)
    };
    const expectedWorldPoint = await page.evaluate((point) => {
      const workspace = document.getElementById('tabContent');
      return window.PostbabyCanvasCamera.clientPointToWorldPoint(
        point.clientX,
        point.clientY,
        workspace,
        window.postbabyGetCameraForTest()
      );
    }, createClientPoint);

    await page.locator('.grid-container').first().evaluate((element, point) => {
      element.dispatchEvent(new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        button: 2,
        clientX: point.clientX,
        clientY: point.clientY
      }));
    }, createClientPoint);

    let textarea = page.locator('textarea.edit-textarea');
    await expect(textarea).toBeVisible();
    await textarea.fill('Camera Zoom Created');
    await textarea.press('Escape');
    const createdItem = (await readTabsSnapshot(page))[0].items.find((item) => item.name === 'Camera Zoom Created');
    expect(createdItem).toBeTruthy();
    const createdPosition = await page.evaluate((item) => window.PostbabyGeometryDom.getItemPositionXY(item), createdItem);
    expect(createdPosition.x).toBeCloseTo(expectedWorldPoint.x, 1);
    expect(createdPosition.y).toBeCloseTo(expectedWorldPoint.y, 1);
  });

  test('workspace scroll keeps top chrome hit targets usable for settings, tabs, and trash interactions', async ({ page }) => {
    const localSnapshot = buildLocalSnapshotWithItems([
      buildNoteItem('Scrollable Trash Note', {
        itemId: 'trash-note',
        position: { top: '2300px', left: '2500px' }
      })
    ]);

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    await scrollWorkspaceTo(page, 1850, 1700);
    await expect.poll(async () => {
      const scroll = await readWorkspaceScroll(page);
      return scroll.x > 0 && scroll.y > 0;
    }).toBe(true);

    await page.click('#settingsButton');
    await expect(page.locator('#settingsModal')).toBeVisible();
    await page.locator('.close-settings').click();
    await expect(page.locator('#settingsModal')).toBeHidden();

    const tabLocator = page.locator('#tabBar .tab[data-tab-id]');
    const tabCountBefore = await tabLocator.count();
    await page.click('#addTab');
    await expect(tabLocator).toHaveCount(tabCountBefore + 1);
    const newestTab = tabLocator.nth(tabCountBefore);
    const newestTabId = await newestTab.getAttribute('data-tab-id');
    if (!newestTabId) {
      throw new Error('Newly created tab id was not available.');
    }
    await newestTab.click();
    await expect(page.locator(`.tab-pane[data-tab-id="${newestTabId}"]`)).toHaveClass(/active/);
    await page.locator('.tab[data-tab-id="tab-1"]').click();
    await expect(page.locator('.tab-pane[data-tab-id="tab-1"]')).toHaveClass(/active/);

    const trashNote = page.locator('.grid-item[data-id="trash-note"]');
    await dragNoteToTrash(page, trashNote);
    await expect(page.locator('#confirmModal')).toBeVisible();
    await page.click('#confirmDelete');
    await expect(trashNote).toHaveCount(0);
  });

  test('graph hook default placement follows the visible workspace area before and after workspace scroll', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const nearGraphResult = await createGraphForTest(page, buildNormalizedGraph([
      buildGraphNode('A', { label: 'Graph Origin Visible' })
    ]));
    expect(nearGraphResult.ok).toBe(true);
    await expect(page.locator(`.grid-item[data-id="${nearGraphResult.createdItemIds[0]}"]`)).toBeVisible();

    const farAnchorGraphResult = await createGraphForTest(page, buildNormalizedGraph([
      buildGraphNode('F', { label: 'Graph Far Anchor' })
    ], [], {
      originX: 2600,
      originY: 2300
    }));
    expect(farAnchorGraphResult.ok).toBe(true);

    await scrollWorkspaceTo(page, 2100, 1900);
    await expect.poll(async () => {
      const scroll = await readWorkspaceScroll(page);
      return scroll.x > 0 && scroll.y > 0;
    }).toBe(true);
    const scrolledWorkspace = await readWorkspaceScroll(page);

    const scrolledGraphResult = await createGraphForTest(page, buildNormalizedGraph([
      buildGraphNode('B', { label: 'Graph Scrolled Visible' })
    ]));
    expect(scrolledGraphResult.ok).toBe(true);

    const scrolledGraphItem = (await readTabsSnapshot(page))[0].items.find((item) => item.id === scrolledGraphResult.createdItemIds[0]);
    expect(scrolledGraphItem).toBeTruthy();
    const scrolledGraphPosition = await page.evaluate((item) => {
      return window.PostbabyGeometryDom.getItemPositionXY(item);
    }, scrolledGraphItem);
    expect(scrolledGraphPosition.x).toBeGreaterThanOrEqual(scrolledWorkspace.x);
    expect(scrolledGraphPosition.x).toBeLessThanOrEqual(scrolledWorkspace.x + scrolledWorkspace.width);
    expect(scrolledGraphPosition.y).toBeGreaterThanOrEqual(scrolledWorkspace.y);
    expect(scrolledGraphPosition.y).toBeLessThanOrEqual(scrolledWorkspace.y + scrolledWorkspace.height);
    await expect(page.locator(`.grid-item[data-id="${scrolledGraphResult.createdItemIds[0]}"]`)).toBeVisible();
  });

  test('Mermaid graph creation default placement follows the visible workspace area before and after workspace scroll', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    const nearMermaidResult = await createGraphFromMermaidForTest(page, [
      'flowchart LR',
      'A[Mermaid Origin Visible]'
    ].join('\n'));
    expect(nearMermaidResult.ok).toBe(true);
    await expect(page.locator(`.grid-item[data-id="${nearMermaidResult.createdItemIds[0]}"]`)).toBeVisible();

    const farAnchorMermaidResult = await createGraphFromMermaidForTest(page, [
      'flowchart LR',
      'F[Mermaid Far Anchor]'
    ].join('\n'), {
      originX: 2700,
      originY: 2400
    });
    expect(farAnchorMermaidResult.ok).toBe(true);

    await scrollWorkspaceTo(page, 2200, 2000);
    await expect.poll(async () => {
      const scroll = await readWorkspaceScroll(page);
      return scroll.x > 0 && scroll.y > 0;
    }).toBe(true);
    const scrolledWorkspace = await readWorkspaceScroll(page);

    const scrolledMermaidResult = await createGraphFromMermaidForTest(page, [
      'flowchart TD',
      'B[Mermaid Scrolled Visible]'
    ].join('\n'));
    expect(scrolledMermaidResult.ok).toBe(true);

    const scrolledMermaidItem = (await readTabsSnapshot(page))[0].items.find((item) => item.id === scrolledMermaidResult.createdItemIds[0]);
    expect(scrolledMermaidItem).toBeTruthy();
    const scrolledMermaidPosition = await page.evaluate((item) => {
      return window.PostbabyGeometryDom.getItemPositionXY(item);
    }, scrolledMermaidItem);
    expect(scrolledMermaidPosition.x).toBeGreaterThanOrEqual(scrolledWorkspace.x);
    expect(scrolledMermaidPosition.x).toBeLessThanOrEqual(scrolledWorkspace.x + scrolledWorkspace.width);
    expect(scrolledMermaidPosition.y).toBeGreaterThanOrEqual(scrolledWorkspace.y);
    expect(scrolledMermaidPosition.y).toBeLessThanOrEqual(scrolledWorkspace.y + scrolledWorkspace.height);
    await expect(page.locator(`.grid-item[data-id="${scrolledMermaidResult.createdItemIds[0]}"]`)).toBeVisible();
  });

  test('import and export still work from Settings', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Settings Export Note');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');

    await openSettingsImportExportTab(page);
    await expect(page.locator('#saveDataButton')).toBeVisible();
    await expect(page.locator('#loadDataButton')).toBeVisible();
    const exported = await exportCurrentSnapshot(page);
    expect(exported.tabs).toBeTruthy();

    await importBackupSnapshot(page, buildLocalSnapshot('Imported From Settings'));
    await expectNoteVisible(page, 'Imported From Settings');
  });

  test('Mermaid import from Settings creates ordinary Postbaby items and edges and keeps the modal open', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    await openSettingsImportExportTab(page);
    await page.locator('#mermaidImportSource').fill([
      'flowchart LR',
      'A[Source] --> B((Circle Target))',
      'A --- C{Decision}'
    ].join('\n'));
    await page.locator('#mermaidImportButton').click();

    await expect(page.locator('#settingsModal')).toBeVisible();
    await expect(page.locator('#mermaidImportStatus')).toContainText('Mermaid import completed.');
    await expect(page.locator('#mermaidImportStatus')).toContainText('Created 3 notes and 2 edges.');
    await expect(page.locator('#mermaidImportSource')).toHaveValue('');
    await expectNoteVisible(page, 'Source');
    await expectNoteVisible(page, 'Circle Target');
    await expectNoteVisible(page, 'Decision');
    await expect(page.locator('.grid-item.selected')).toHaveCount(3);
    await expect(page.locator('.edge-line')).toHaveCount(2);

    const tabsSnapshot = await readTabsSnapshot(page);
    const createdItems = tabsSnapshot[0].items;
    const createdEdges = tabsSnapshot[0].edges;
    expect(createdItems.map((item) => item.name)).toEqual(
      expect.arrayContaining(['Source', 'Circle Target', 'Decision'])
    );
    expect(createdItems.find((item) => item.name === 'Circle Target')?.shape).toBe('circle');
    expect(createdItems.find((item) => item.name === 'Decision')?.shape).toBe('diamond');
    expect(createdEdges.map((edge) => edge.kind).sort()).toEqual(['arrow', 'line']);
  });

  test('Mermaid import in hand mode can be deselected by blank canvas click', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    await setCanvasMode(page, 'pan');
    expect(await readCanvasMode(page)).toBe('pan');

    await openSettingsImportExportTab(page);
    await page.locator('#mermaidImportSource').fill([
      'flowchart LR',
      'A[Source] --> B((Circle Target))',
      'A --- C{Decision}'
    ].join('\n'));
    await page.locator('#mermaidImportButton').click();

    await expect(page.locator('#settingsModal')).toBeVisible();
    await expect(page.locator('#mermaidImportStatus')).toContainText('Mermaid import completed.');
    await expect(page.locator('.grid-item.selected')).toHaveCount(3);

    await page.locator('.close-settings').click();
    await expect(page.locator('#settingsModal')).toBeHidden();
    await expect(page.locator('.grid-item.selected')).toHaveCount(3);

    await clickEmptyGrid(page);
    await expect(page.locator('.grid-item.selected')).toHaveCount(0);
    expect(await readCanvasMode(page)).toBe('pan');
  });

  test('Mermaid import from Settings surfaces warnings inline when supported syntax is normalized', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    await openSettingsImportExportTab(page);
    await page.locator('#mermaidImportSource').fill([
      'flowchart LR',
      'A>Skewed] --> B'
    ].join('\n'));
    await page.locator('#mermaidImportButton').click();

    await expect(page.locator('#mermaidImportStatus')).toContainText('Mermaid import completed.');
    await expect(page.locator('#mermaidImportStatus')).toContainText('Warnings');
    await expect(page.locator('#mermaidImportStatus')).toContainText('parallelogram nodes are not supported yet');
    await expectNoteVisible(page, 'Skewed');
    const tabsSnapshot = await readTabsSnapshot(page);
    expect(tabsSnapshot[0].items.find((item) => item.name === 'Skewed')?.shape).toBe('default');
    expect(tabsSnapshot[0].edges).toHaveLength(1);
  });

  test('invalid Mermaid from Settings shows inline errors and creates nothing', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    await openSettingsImportExportTab(page);
    await page.locator('#mermaidImportSource').fill([
      'sequenceDiagram',
      'A->>B: Hello'
    ].join('\n'));
    await page.locator('#mermaidImportButton').click();

    await expect(page.locator('#settingsModal')).toBeVisible();
    await expect(page.locator('#mermaidImportStatus')).toContainText('Mermaid import failed.');
    await expect(page.locator('#mermaidImportStatus')).toContainText('Unsupported Mermaid diagram type');
    await expect(page.locator('#mermaidImportSource')).toHaveValue([
      'sequenceDiagram',
      'A->>B: Hello'
    ].join('\n'));

    const tabsSnapshot = await readTabsSnapshot(page);
    expect(tabsSnapshot[0].items || []).toHaveLength(0);
    expect(tabsSnapshot[0].edges || []).toHaveLength(0);
  });

  test('Mermaid import from Settings surfaces tab-capacity failures without partial items or edges', async ({ page }) => {
    const nearlyFullItems = buildBulkNoteItems(MAX_ITEMS_PER_TAB - 1);

    await prepareBlankPage(page);
    await seedLocalStorage(page, buildLocalSnapshotWithItems(nearlyFullItems));
    await page.goto('/index.html');

    await openSettingsImportExportTab(page);
    await page.locator('#mermaidImportSource').fill([
      'flowchart LR',
      'A[Too Big Source] --> B[Too Big Target]'
    ].join('\n'));
    await page.locator('#mermaidImportButton').click();

    await expect(page.locator('#settingsModal')).toBeVisible();
    await expect(page.locator('#mermaidImportStatus')).toContainText('Mermaid import failed.');
    await expect(page.locator('#mermaidImportStatus')).toContainText('Fix the Mermaid source below and try again.');
    await expect(page.locator('#mermaidImportStatus')).toContainText('This import is too large for one tab.');
    await expect(page.locator('#mermaidImportSource')).toHaveValue([
      'flowchart LR',
      'A[Too Big Source] --> B[Too Big Target]'
    ].join('\n'));

    const tabsSnapshot = await readTabsSnapshot(page);
    expect(tabsSnapshot[0].items).toHaveLength(MAX_ITEMS_PER_TAB - 1);
    expect(tabsSnapshot[0].edges || []).toHaveLength(0);
    expect(tabsSnapshot[0].items.some((item) => item.name === 'Too Big Source')).toBe(false);
    expect(tabsSnapshot[0].items.some((item) => item.name === 'Too Big Target')).toBe(false);
  });

  test('manual note and manual edge creation still work after Mermaid import from Settings', async ({ page }) => {
    await prepareBlankPage(page);
    await seedLocalStorage(page, buildEmptySnapshot());
    await page.goto('/index.html');

    await openSettingsImportExportTab(page);
    await page.locator('#mermaidImportSource').fill([
      'flowchart LR',
      'A[Manual Mermaid Source]',
      'B((Manual Mermaid Target))'
    ].join('\n'));
    await page.locator('#mermaidImportButton').click();
    await expect(page.locator('#mermaidImportStatus')).toContainText('Mermaid import completed.');

    await page.locator('.close-settings').click();
    await expect(page.locator('#settingsModal')).toBeHidden();

    await page.keyboard.press('n');
    const textarea = page.locator('textarea.edit-textarea');
    await expect(textarea).toBeVisible();
    await textarea.fill('Manual Note After Mermaid Import');
    await textarea.press('Escape');
    await expectNoteVisible(page, 'Manual Note After Mermaid Import');

    const sourceNote = page.locator('.grid-item', { hasText: 'Manual Mermaid Source' }).first();
    const targetNote = page.locator('.grid-item', { hasText: 'Manual Mermaid Target' }).first();
    await drawEdgeBetweenNotes(page, sourceNote, targetNote);

    const tabsSnapshot = await readTabsSnapshot(page);
    expect(tabsSnapshot[0].items.map((item) => item.name)).toEqual(
      expect.arrayContaining(['Manual Mermaid Source', 'Manual Mermaid Target', 'Manual Note After Mermaid Import'])
    );
    expect(tabsSnapshot[0].edges).toHaveLength(1);
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

    const cameraControlsBox = await page.locator('#cameraControls').boundingBox();
    if (!cameraControlsBox) {
      throw new Error('Camera controls bounding box was not available.');
    }

    expect(Math.abs((cameraControlsBox.x + (cameraControlsBox.width / 2)) - 180)).toBeLessThanOrEqual(4);
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
