const fs = require('fs');
const { test, expect } = require('@playwright/test');

const DB_NAME = 'postbaby-browser-storage';
const SNAPSHOTS_STORE = 'snapshots';
const META_STORE = 'meta';
const PRIMARY_RECORD_ID = 'primary';
const TIMESTAMP = '2026-05-13T12:00:00.000Z';
const EDGE_ARM_DELAY_MS = 1000;
const ITEM_SHAPES = ['default', 'circle', 'square', 'triangle', 'diamond', 'upsideDownTriangle', 'hexagon', 'oval'];
const FREEFORM_FOOTPRINT_SHAPES = ['default', 'oval'];
const FIXED_RATIO_SHAPES = ITEM_SHAPES.filter((shape) => !FREEFORM_FOOTPRINT_SHAPES.includes(shape));
const FIXED_SQUARE_RESIZE_SHAPES = ['square', 'circle', 'diamond'];
const DERIVED_HEIGHT_RESIZE_SHAPES = ['triangle', 'upsideDownTriangle', 'hexagon'];
const RESIZABLE_FIXED_RATIO_SHAPES = FIXED_SQUARE_RESIZE_SHAPES.concat(DERIVED_HEIGHT_RESIZE_SHAPES);
const FIXED_RATIO_HEIGHT_BY_WIDTH = {
  square: 1,
  circle: 1,
  diamond: 1,
  triangle: 150 / 170,
  upsideDownTriangle: 150 / 170,
  hexagon: 135 / 170
};
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

function buildServerPayload(noteText, version) {
  return {
    version,
    updatedAt: TIMESTAMP,
    data: buildLocalSnapshot(noteText, { hasRunBefore: true, theme: 'light' })
  };
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

async function seedLocalStorage(page, snapshot) {
  await page.evaluate((data) => {
    Object.entries(data).forEach(([key, value]) => {
      window.localStorage.setItem(key, value);
    });
  }, snapshot);
}

async function seedIndexedDB(page, snapshot, meta) {
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
    recordId: PRIMARY_RECORD_ID,
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

async function readIndexedDBState(page) {
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
    recordId: PRIMARY_RECORD_ID
  });
}

async function getLocalStorageValues(page, keys) {
  return page.evaluate((requestedKeys) => {
    const result = {};
    requestedKeys.forEach((key) => {
      result[key] = window.localStorage.getItem(key);
    });
    return result;
  }, keys);
}

async function openSettingsModal(page) {
  await page.locator('#trash').click();
  await expect(page.locator('#settingsModal')).toBeVisible();
}

async function expectNoteVisible(page, noteText) {
  await expect(page.locator('.grid-item span').filter({ hasText: noteText }).first()).toBeVisible();
}

async function readTabsSnapshot(page) {
  const indexedDBState = await readIndexedDBState(page);
  return JSON.parse(indexedDBState.snapshot.tabs);
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
    const textStyle = text ? window.getComputedStyle(text) : null;
    const leftShaper = element.querySelector('.grid-item-text-shaper--left');
    const rightShaper = element.querySelector('.grid-item-text-shaper--right');
    const leftStyle = leftShaper ? window.getComputedStyle(leftShaper) : null;
    const rightStyle = rightShaper ? window.getComputedStyle(rightShaper) : null;
    return {
      flowHeight: textStyle ? textStyle.minHeight : '',
      beforeFloat: leftStyle ? leftStyle.float : '',
      beforeShapeOutside: leftStyle ? leftStyle.getPropertyValue('shape-outside') : '',
      afterFloat: rightStyle ? rightStyle.float : '',
      afterShapeOutside: rightStyle ? rightStyle.getPropertyValue('shape-outside') : ''
    };
  });
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
    strokeOpacity: element.getAttribute('stroke-opacity') || ''
  }));
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
  const runtimeConfig = Object.assign({
    deploymentMode: 'selfhosted_single_user',
    authAvailable: true,
    authRequired: true,
    isAuthenticated: true,
    billingAvailable: false,
    syncAvailable: true,
    syncRequiresAuth: true,
    syncUsable: true,
    entitlement: {
      hostedSync: false
    },
    setupAvailable: false,
    apiBase: ''
  }, options.runtimeConfig || {});
  const metaStatus = options.metaStatus || 200;
  const metaBody = options.metaBody || metaPayload;
  const documentStatus = options.documentStatus || 200;
  const documentBody = options.documentBody || documentPayload;

  await page.route('**/runtime-config.js', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/javascript; charset=utf-8',
      headers: {
        'Cache-Control': 'no-store'
      },
      body: `window.POSTBABY_RUNTIME = ${JSON.stringify(runtimeConfig)};\n`
    });
  });

  await page.route(/\/api\/document\/meta(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      status: metaStatus,
      contentType: 'application/json; charset=utf-8',
      headers: {
        'Cache-Control': 'no-store'
      },
      body: JSON.stringify(metaBody)
    });
  });

  await page.route(/\/api\/document(?:\?.*)?$/, async (route) => {
    const request = route.request();
    if (request.method() === 'GET') {
      await route.fulfill({
        status: documentStatus,
        contentType: 'application/json; charset=utf-8',
        headers: {
          'Cache-Control': 'no-store'
        },
        body: JSON.stringify(documentBody)
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

test.describe('Static local-only behavior', () => {
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

  for (const shape of ['triangle', 'upsideDownTriangle']) {
    test(`${shape} uses tapered text flow that grows with the note height`, async ({ page }) => {
      const localSnapshot = buildLocalSnapshot(`${shape} text should stay visually contained inside the tapered note shape after resize`, {
        shape,
        width: 280,
        height: 190
      });

      await prepareBlankPage(page);
      await seedLocalStorage(page, localSnapshot);
      await page.goto('/index.html');

      const note = page.locator('.grid-item[data-id="item-1"]');
      await expect(note).toHaveAttribute('data-shape', shape);
      await expect(note.locator('.grid-item-text')).toBeVisible();

      const beforeFlow = await readItemTextFlowPresentation(note);
      expect(Number.parseFloat(beforeFlow.flowHeight || '0')).toBeGreaterThan(100);
      expect(beforeFlow.beforeFloat).toBe('left');
      expect(beforeFlow.afterFloat).toBe('right');
      expect(beforeFlow.beforeShapeOutside).not.toBe('none');
      expect(beforeFlow.afterShapeOutside).not.toBe('none');

      await resizeNoteBy(page, note, 90, 30);
      const afterFlow = await readItemTextFlowPresentation(note);
      expect(Number.parseFloat(afterFlow.flowHeight || '0')).toBeGreaterThan(Number.parseFloat(beforeFlow.flowHeight || '0'));
      expect(afterFlow.beforeShapeOutside).not.toBe('none');
      expect(afterFlow.afterShapeOutside).not.toBe('none');
      await expectNoteVisible(page, `${shape} text should stay visually contained inside the tapered note shape after resize`);
    });
  }

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

  test('exposes debug helpers in static local-only mode', async ({ page }) => {
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

test.describe('Runtime auth and sync gating', () => {
  test('static local runtime keeps the sync panel hidden', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Static Local Runtime');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await page.goto('/index.html');
    await expectNoteVisible(page, 'Static Local Runtime');

    await expect(page.locator('.settings-sync-panel')).toBeHidden();

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.runtimeConfig.authAvailable).toBe(false);
    expect(debugState.runtimeConfig.billingAvailable).toBe(false);
    expect(debugState.runtimeConfig.syncAvailable).toBe(false);
    expect(debugState.runtimeConfig.syncUsable).toBe(false);
    expect(debugState.runtimeConfig.entitlement.hostedSync).toBe(false);
    expect(debugState.shouldStartSyncImmediately).toBe(false);
    expect(debugState.shouldShowSyncUI).toBe(false);
  });

  test('self-hosted authenticated runtime auto-starts sync and shows logout controls', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Self-Hosted Runtime');
    const syncRequests = [];

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await enableMockedSync(page);
    page.on('request', (request) => {
      const url = request.url();
      if (url.includes('/api/document/meta') || url.includes('/api/document')) {
        syncRequests.push(url);
      }
    });

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Self-Hosted Runtime');
    await expect.poll(() => syncRequests.some((url) => url.includes('/api/document/meta'))).toBe(true);

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.runtimeConfig.billingAvailable).toBe(false);
    expect(debugState.runtimeConfig.syncUsable).toBe(true);
    expect(debugState.runtimeConfig.entitlement.hostedSync).toBe(false);
    expect(debugState.shouldStartSyncImmediately).toBe(true);
    expect(debugState.shouldShowSyncUI).toBe(true);
    expect(debugState.shouldShowLogoutControl).toBe(true);
    expect(debugState.isBackgroundSyncActive).toBe(true);
    await expect(page.locator('#logoutForm')).toBeVisible();
    await expect(page.locator('#authLinks')).toBeHidden();

    await openSettingsModal(page);
    await expect(page.locator('.settings-sync-panel')).toBeVisible();
    await page.locator('.close-settings').click();
  });

  test('optional-auth runtime stays local-only until the user is authenticated', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Optional Auth Preview');
    const syncRequests = [];

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await enableMockedSync(page, {
      runtimeConfig: {
        deploymentMode: 'cloud_multi_user',
        authRequired: false,
        isAuthenticated: false,
        syncUsable: false,
        entitlement: {
          hostedSync: false
        }
      }
    });
    page.on('request', (request) => {
      const url = request.url();
      if (url.includes('/api/document/meta') || url.includes('/api/document')) {
        syncRequests.push(url);
      }
    });

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Optional Auth Preview');
    await page.waitForTimeout(250);
    expect(syncRequests).toEqual([]);

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.runtimeConfig.authAvailable).toBe(true);
    expect(debugState.runtimeConfig.authRequired).toBe(false);
    expect(debugState.runtimeConfig.isAuthenticated).toBe(false);
    expect(debugState.runtimeConfig.billingAvailable).toBe(false);
    expect(debugState.runtimeConfig.syncAvailable).toBe(true);
    expect(debugState.runtimeConfig.syncRequiresAuth).toBe(true);
    expect(debugState.runtimeConfig.syncUsable).toBe(false);
    expect(debugState.runtimeConfig.entitlement.hostedSync).toBe(false);
    expect(debugState.shouldStartSyncImmediately).toBe(false);
    expect(debugState.shouldShowSyncUI).toBe(true);
    expect(debugState.shouldShowLogoutControl).toBe(false);
    expect(debugState.shouldShowHostedAuthLinks).toBe(true);
    expect(debugState.isSyncAwaitingAuthentication).toBe(true);
    expect(debugState.isSyncBlockedByEntitlement).toBe(false);
    expect(debugState.isBackgroundSyncActive).toBe(false);
    await expect(page.locator('#authLinks')).toBeVisible();
    await expect(page.locator('#loginLink')).toHaveAttribute('href', '/login');
    await expect(page.locator('#signupLink')).toHaveAttribute('href', '/signup');
    await expect(page.locator('#logoutForm')).toBeHidden();

    await openSettingsModal(page);
    await expect(page.locator('.settings-sync-panel')).toBeVisible();
    await expect(page.locator('#syncStateStatus')).toContainText('Sign in to sync across devices');
    await expect(page.locator('#syncVersionStatus')).toContainText('Server version: sign in to access sync');
    await page.locator('.close-settings').click();
  });

  test('logged-in unpaid cloud runtime with billingAvailable=false keeps the Phase 1 status-only UI', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Optional Auth Unpaid');
    const syncRequests = [];

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await enableMockedSync(page, {
      runtimeConfig: {
        deploymentMode: 'cloud_multi_user',
        authRequired: false,
        isAuthenticated: true,
        syncUsable: false,
        entitlement: {
          hostedSync: false
        }
      }
    });
    page.on('request', (request) => {
      const url = request.url();
      if (url.includes('/api/document/meta') || url.includes('/api/document')) {
        syncRequests.push(url);
      }
    });

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Optional Auth Unpaid');
    await page.waitForTimeout(250);
    expect(syncRequests).toEqual([]);

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.runtimeConfig.authRequired).toBe(false);
    expect(debugState.runtimeConfig.isAuthenticated).toBe(true);
    expect(debugState.runtimeConfig.billingAvailable).toBe(false);
    expect(debugState.runtimeConfig.syncAvailable).toBe(true);
    expect(debugState.runtimeConfig.syncRequiresAuth).toBe(true);
    expect(debugState.runtimeConfig.syncUsable).toBe(false);
    expect(debugState.runtimeConfig.entitlement.hostedSync).toBe(false);
    expect(debugState.shouldStartSyncImmediately).toBe(false);
    expect(debugState.shouldShowLogoutControl).toBe(true);
    expect(debugState.shouldShowHostedAuthLinks).toBe(false);
    expect(debugState.shouldShowBillingUpgradeControl).toBe(false);
    expect(debugState.shouldShowManageBillingControl).toBe(false);
    expect(debugState.isSyncAwaitingAuthentication).toBe(false);
    expect(debugState.isSyncBlockedByEntitlement).toBe(true);
    expect(debugState.isBackgroundSyncActive).toBe(false);
    expect(debugState.syncStatusMessage).toBe('Account sync is not enabled for this account yet');
    await expect(page.locator('#authLinks')).toBeHidden();
    await expect(page.locator('#logoutForm')).toBeVisible();
    await expect(page.locator('#billingCheckoutForm')).toBeHidden();
    await expect(page.locator('#billingPortalForm')).toBeHidden();

    await openSettingsModal(page);
    await expect(page.locator('#syncStateStatus')).toContainText('Account sync is not enabled for this account yet');
    await expect(page.locator('#syncVersionStatus')).toContainText('Server version: hosted sync not enabled for this account');
    await expect(page.locator('#billingCheckoutForm')).toBeHidden();
    await expect(page.locator('#billingPortalForm')).toBeHidden();
    await page.locator('.close-settings').click();
  });

  test('logged-in unpaid cloud runtime with billingAvailable=true shows Upgrade', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Optional Auth Upgrade');

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await enableMockedSync(page, {
      runtimeConfig: {
        deploymentMode: 'cloud_multi_user',
        authRequired: false,
        isAuthenticated: true,
        billingAvailable: true,
        syncUsable: false,
        entitlement: {
          hostedSync: false
        }
      }
    });

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Optional Auth Upgrade');

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.runtimeConfig.billingAvailable).toBe(true);
    expect(debugState.runtimeConfig.stripeSecretKey).toBeUndefined();
    expect(debugState.shouldShowBillingUpgradeControl).toBe(true);
    expect(debugState.shouldShowManageBillingControl).toBe(false);

    await openSettingsModal(page);
    await expect(page.locator('#billingCheckoutForm')).toBeVisible();
    await expect(page.locator('#billingCheckoutForm')).toHaveAttribute('action', '/billing/checkout');
    await expect(page.locator('#billingCheckoutForm button')).toContainText('Upgrade');
    await expect(page.locator('#billingPortalForm')).toBeHidden();
    await page.locator('.close-settings').click();
  });

  test('optional-auth authenticated and entitled runtime auto-starts sync', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Optional Auth Signed In');
    const syncRequests = [];

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await enableMockedSync(page, {
      runtimeConfig: {
        deploymentMode: 'cloud_multi_user',
        authRequired: false,
        isAuthenticated: true,
        syncUsable: true,
        entitlement: {
          hostedSync: true
        }
      }
    });
    page.on('request', (request) => {
      const url = request.url();
      if (url.includes('/api/document/meta') || url.includes('/api/document')) {
        syncRequests.push(url);
      }
    });

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Optional Auth Signed In');
    await expect.poll(() => syncRequests.some((url) => url.includes('/api/document/meta'))).toBe(true);

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.runtimeConfig.authRequired).toBe(false);
    expect(debugState.runtimeConfig.isAuthenticated).toBe(true);
    expect(debugState.runtimeConfig.syncUsable).toBe(true);
    expect(debugState.runtimeConfig.entitlement.hostedSync).toBe(true);
    expect(debugState.shouldStartSyncImmediately).toBe(true);
    expect(debugState.shouldShowLogoutControl).toBe(true);
    expect(debugState.shouldShowHostedAuthLinks).toBe(false);
    expect(debugState.shouldShowBillingUpgradeControl).toBe(false);
    expect(debugState.shouldShowManageBillingControl).toBe(false);
    expect(debugState.isSyncBlockedByEntitlement).toBe(false);
    expect(debugState.isBackgroundSyncActive).toBe(true);
    await expect(page.locator('#authLinks')).toBeHidden();
    await expect(page.locator('#logoutForm')).toBeVisible();

    await openSettingsModal(page);
    await page.locator('.close-settings').click();
  });

  test('logged-in entitled cloud runtime with billingAvailable=true shows Manage Billing', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Optional Auth Manage Billing');
    const syncRequests = [];

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await enableMockedSync(page, {
      runtimeConfig: {
        deploymentMode: 'cloud_multi_user',
        authRequired: false,
        isAuthenticated: true,
        billingAvailable: true,
        syncUsable: true,
        entitlement: {
          hostedSync: true
        }
      }
    });
    page.on('request', (request) => {
      const url = request.url();
      if (url.includes('/api/document/meta') || url.includes('/api/document')) {
        syncRequests.push(url);
      }
    });

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Optional Auth Manage Billing');
    await expect.poll(() => syncRequests.some((url) => url.includes('/api/document/meta'))).toBe(true);

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.runtimeConfig.billingAvailable).toBe(true);
    expect(debugState.shouldShowBillingUpgradeControl).toBe(false);
    expect(debugState.shouldShowManageBillingControl).toBe(true);
    expect(debugState.isBackgroundSyncActive).toBe(true);

    await openSettingsModal(page);
    await expect(page.locator('#billingPortalForm')).toBeVisible();
    await expect(page.locator('#billingPortalForm')).toHaveAttribute('action', '/billing/portal');
    await expect(page.locator('#billingPortalForm button')).toContainText('Manage Billing');
    await expect(page.locator('#billingCheckoutForm')).toBeHidden();
    await page.locator('.close-settings').click();
  });

  test('optional-auth entitlement-required sync responses do not redirect or log out the user', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Optional Auth Entitlement Required');
    const dialogs = [];

    attachDialogHandler(page, [], dialogs);
    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await enableMockedSync(page, {
      runtimeConfig: {
        deploymentMode: 'cloud_multi_user',
        authRequired: false,
        isAuthenticated: true,
        syncUsable: true,
        entitlement: {
          hostedSync: true
        }
      },
      metaStatus: 403,
      metaBody: {
        ok: false,
        error: {
          code: 'entitlement_required',
          message: 'hosted sync is not enabled for this account'
        }
      }
    });

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Optional Auth Entitlement Required');
    await expect(page).toHaveURL(/\/index\.html$/);
    expect(dialogs).toEqual([]);

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.runtimeConfig.authRequired).toBe(false);
    expect(debugState.runtimeConfig.isAuthenticated).toBe(true);
    expect(debugState.runtimeConfig.syncUsable).toBe(false);
    expect(debugState.runtimeConfig.entitlement.hostedSync).toBe(false);
    expect(debugState.shouldStartSyncImmediately).toBe(false);
    expect(debugState.shouldShowLogoutControl).toBe(true);
    expect(debugState.shouldShowHostedAuthLinks).toBe(false);
    expect(debugState.isSyncBlockedByEntitlement).toBe(true);
    expect(debugState.isBackgroundSyncActive).toBe(false);
    expect(debugState.syncStatusMessage).toBe('Account sync is not enabled for this account yet');
    await expect(page.locator('#authLinks')).toBeHidden();
    await expect(page.locator('#logoutForm')).toBeVisible();

    await openSettingsModal(page);
    await expect(page.locator('#syncStateStatus')).toContainText('Account sync is not enabled for this account yet');
    await expect(page.locator('#syncVersionStatus')).toContainText('Server version: hosted sync not enabled for this account');
    await page.locator('.close-settings').click();
  });

  test('optional-auth unauthorized sync responses do not redirect away from the board', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Optional Auth Unauthorized');
    const dialogs = [];

    attachDialogHandler(page, [], dialogs);
    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
    await enableMockedSync(page, {
      runtimeConfig: {
        deploymentMode: 'cloud_multi_user',
        authRequired: false,
        isAuthenticated: true,
        syncUsable: true,
        entitlement: {
          hostedSync: true
        }
      },
      metaStatus: 401,
      metaBody: {
        ok: false,
        error: {
          code: 'unauthorized',
          message: 'unauthorized'
        }
      }
    });

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Optional Auth Unauthorized');
    await expect(page).toHaveURL(/\/index\.html$/);
    expect(dialogs).toEqual([]);

    const debugState = await page.evaluate(() => window.postbabyDebugSync());
    expect(debugState.runtimeConfig.authRequired).toBe(false);
    expect(debugState.runtimeConfig.isAuthenticated).toBe(false);
    expect(debugState.shouldStartSyncImmediately).toBe(false);
    expect(debugState.shouldShowLogoutControl).toBe(false);
    expect(debugState.shouldShowHostedAuthLinks).toBe(true);
    expect(debugState.isSyncBlockedByEntitlement).toBe(false);
    expect(debugState.isBackgroundSyncActive).toBe(false);
    expect(debugState.syncStatusMessage).toBe('Login expired - sign in again');
    await expect(page.locator('#authLinks')).toBeVisible();
    await expect(page.locator('#logoutForm')).toBeHidden();

    await openSettingsModal(page);
    await expect(page.locator('#syncStateStatus')).toContainText('Login expired - sign in again');
    await page.locator('.close-settings').click();
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

  test('prompts on missing local sync version and loads account notes when accepted', async ({ page }) => {
    const localSnapshot = buildLocalSnapshot('Meaningful Local Note');
    const dialogs = [];

    await prepareBlankPage(page);
    await seedLocalStorage(page, localSnapshot);
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
    await seedLocalStorage(page, localSnapshot);
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

    const indexedDBState = await readIndexedDBState(page);
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
    await seedLocalStorage(page, localSnapshot);
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
    await seedLocalStorage(page, localSnapshot);
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
    await seedLocalStorage(page, localSnapshot);
    await enableMockedSync(page, {
      metaPayload: { ok: true, exists: true, version: 11, updatedAt: TIMESTAMP },
      documentPayload: buildServerPayload('Server Version 11', 11)
    });
    attachDialogHandler(page, [], dialogs);

    await page.goto('/index.html');
    await expectNoteVisible(page, 'Server Version 11');
    expect(dialogs).toEqual([]);
  });
});
