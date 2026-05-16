(function () {
    const DB_NAME = 'postbaby-browser-storage';
    const DB_VERSION = 1;
    const SNAPSHOTS_STORE = 'snapshots';
    const META_STORE = 'meta';
    const PRIMARY_RECORD_ID = 'primary';

    function requestToPromise(request) {
        return new Promise(function (resolve, reject) {
            request.onsuccess = function () {
                resolve(request.result);
            };
            request.onerror = function () {
                reject(request.error || new Error('IndexedDB request failed.'));
            };
        });
    }

    function transactionToPromise(transaction) {
        return new Promise(function (resolve, reject) {
            transaction.oncomplete = function () {
                resolve();
            };
            transaction.onerror = function () {
                reject(transaction.error || new Error('IndexedDB transaction failed.'));
            };
            transaction.onabort = function () {
                reject(transaction.error || new Error('IndexedDB transaction aborted.'));
            };
        });
    }

    function isPlainObject(value) {
        return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
    }

    function cloneStringMap(data) {
        const next = {};
        if (!isPlainObject(data)) {
            return next;
        }

        Object.keys(data).forEach(function (key) {
            const value = data[key];
            if (typeof value === 'string') {
                next[key] = value;
            }
        });
        return next;
    }

    function openDatabase() {
        return new Promise(function (resolve, reject) {
            if (!window.indexedDB) {
                reject(new Error('IndexedDB is not available in this browser.'));
                return;
            }

            const request = window.indexedDB.open(DB_NAME, DB_VERSION);

            request.onupgradeneeded = function () {
                const db = request.result;
                if (!db.objectStoreNames.contains(SNAPSHOTS_STORE)) {
                    db.createObjectStore(SNAPSHOTS_STORE, { keyPath: 'id' });
                }
                if (!db.objectStoreNames.contains(META_STORE)) {
                    db.createObjectStore(META_STORE, { keyPath: 'id' });
                }
            };

            request.onsuccess = function () {
                resolve(request.result);
            };

            request.onerror = function () {
                reject(request.error || new Error('Failed to open IndexedDB.'));
            };

            request.onblocked = function () {
                reject(new Error('IndexedDB open was blocked.'));
            };
        });
    }

    async function readPrimarySnapshot(db) {
        const transaction = db.transaction(SNAPSHOTS_STORE, 'readonly');
        const request = transaction.objectStore(SNAPSHOTS_STORE).get(PRIMARY_RECORD_ID);
        const result = await requestToPromise(request);
        await transactionToPromise(transaction);
        return result || null;
    }

    async function writePrimarySnapshot(db, data) {
        const transaction = db.transaction(SNAPSHOTS_STORE, 'readwrite');
        transaction.objectStore(SNAPSHOTS_STORE).put({
            id: PRIMARY_RECORD_ID,
            data: cloneStringMap(data),
            updatedAt: new Date().toISOString()
        });
        await transactionToPromise(transaction);
    }

    async function readMetaRecord(db) {
        const transaction = db.transaction(META_STORE, 'readonly');
        const request = transaction.objectStore(META_STORE).get(PRIMARY_RECORD_ID);
        const result = await requestToPromise(request);
        await transactionToPromise(transaction);
        return result || null;
    }

    async function writeMetaRecord(db, meta) {
        const transaction = db.transaction(META_STORE, 'readwrite');
        const payload = Object.assign({ id: PRIMARY_RECORD_ID }, meta || {});
        transaction.objectStore(META_STORE).put(payload);
        await transactionToPromise(transaction);
    }

    function validateSnapshot(record, validKeys) {
        if (record === null || record === undefined) {
            return {
                ok: true,
                empty: true,
                data: {},
                keyCount: 0
            };
        }

        if (!isPlainObject(record) || !isPlainObject(record.data)) {
            return {
                ok: false,
                empty: false,
                reason: 'Snapshot record is not a plain object.'
            };
        }

        const validKeySet = validKeys instanceof Set ? validKeys : new Set(Array.isArray(validKeys) ? validKeys : []);
        const sanitized = {};
        const keys = Object.keys(record.data);

        for (let index = 0; index < keys.length; index += 1) {
            const key = keys[index];
            const value = record.data[key];

            if (typeof value !== 'string') {
                return {
                    ok: false,
                    empty: false,
                    reason: `Snapshot value for ${key} is not a string.`
                };
            }

            if (validKeySet.size === 0 || validKeySet.has(key)) {
                sanitized[key] = value;
            }
        }

        if (Object.prototype.hasOwnProperty.call(sanitized, 'tabs')) {
            try {
                JSON.parse(sanitized.tabs);
            } catch (error) {
                return {
                    ok: false,
                    empty: false,
                    reason: 'Snapshot tabs value is not valid JSON.'
                };
            }
        }

        return {
            ok: true,
            empty: Object.keys(sanitized).length === 0,
            data: sanitized,
            keyCount: Object.keys(sanitized).length
        };
    }

    window.postbabyIndexedDBStorage = {
        openDatabase: openDatabase,
        readPrimarySnapshot: readPrimarySnapshot,
        writePrimarySnapshot: writePrimarySnapshot,
        readMetaRecord: readMetaRecord,
        writeMetaRecord: writeMetaRecord,
        validateSnapshot: validateSnapshot
    };
})();
