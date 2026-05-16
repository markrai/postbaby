(function () {
    const indexedDBStorage = window.postbabyIndexedDBStorage || null;
    const modeIndexedDBPrimary = 'indexeddb-primary';
    const modeLegacyFallback = 'legacy-localstorage-fallback';

    const state = {
        ready: false,
        mode: modeLegacyFallback,
        db: null,
        cache: {},
        managedKeys: [],
        managedKeySet: new Set(),
        emergencyMetadataKeys: [],
        emergencyMetadataKeySet: new Set(),
        themeMirrorKey: 'theme',
        bootstrapPromise: null,
        pendingWriteGeneration: 0,
        flushedWriteGeneration: 0,
        flushTimer: null,
        flushPromise: null,
        flushRequestedWhileBusy: false,
        flushRequestedReason: null,
        meta: null
    };

    function cloneSnapshot(source) {
        const next = {};
        if (!source || typeof source !== 'object' || Array.isArray(source)) {
            return next;
        }

        Object.keys(source).forEach(function (key) {
            if (typeof source[key] === 'string') {
                next[key] = source[key];
            }
        });
        return next;
    }

    function keysOfSnapshot(snapshot) {
        return Object.keys(snapshot || {});
    }

    function setFromArray(values) {
        return new Set(Array.isArray(values) ? values : []);
    }

    function localStorageGet(key) {
        return window.localStorage.getItem(key);
    }

    function localStorageSet(key, value) {
        window.localStorage.setItem(key, String(value));
    }

    function localStorageRemove(key) {
        window.localStorage.removeItem(key);
    }

    function snapshotHasAnyData(snapshot) {
        return keysOfSnapshot(snapshot).length > 0;
    }

    function isManagedStorageKey(key) {
        return state.managedKeySet.has(key);
    }

    function normalizeManagedKeys(options) {
        const managedKeys = Array.isArray(options && options.managedKeys) ? options.managedKeys.slice() : [];
        state.managedKeys = managedKeys.filter(function (key, index) {
            return typeof key === 'string' && managedKeys.indexOf(key) === index;
        });
        state.managedKeySet = setFromArray(state.managedKeys);
        state.emergencyMetadataKeys = Array.isArray(options && options.emergencyMetadataKeys)
            ? options.emergencyMetadataKeys.filter(function (key) {
                return typeof key === 'string' && key.length > 0;
            })
            : [];
        state.emergencyMetadataKeySet = setFromArray(state.emergencyMetadataKeys);
        state.themeMirrorKey = options && typeof options.themeMirrorKey === 'string' && options.themeMirrorKey
            ? options.themeMirrorKey
            : 'theme';
    }

    function readRecognizedLocalSnapshot() {
        const data = {};
        let malformed = false;

        state.managedKeys.forEach(function (key) {
            const value = localStorageGet(key);
            if (value !== null) {
                data[key] = value;
            }
        });

        if (Object.prototype.hasOwnProperty.call(data, 'tabs')) {
            try {
                JSON.parse(data.tabs);
            } catch (error) {
                malformed = true;
            }
        }

        return {
            data: malformed ? {} : data,
            valid: !malformed,
            malformed: malformed,
            hasData: !malformed && snapshotHasAnyData(data)
        };
    }

    function readEmergencyFallbackMetadata() {
        const dirtyKey = state.emergencyMetadataKeys[0];
        const dirtyAtKey = state.emergencyMetadataKeys[1];
        const reasonKey = state.emergencyMetadataKeys[2];
        return {
            dirty: dirtyKey ? localStorageGet(dirtyKey) === 'true' : false,
            dirtyAt: dirtyAtKey ? localStorageGet(dirtyAtKey) : null,
            reason: reasonKey ? localStorageGet(reasonKey) : null
        };
    }

    function getCurrentFallbackReason(defaultReason) {
        const metadata = readEmergencyFallbackMetadata();
        return metadata.reason || defaultReason || 'write_failed';
    }

    function setEmergencyFallbackMetadata(reason) {
        if (state.emergencyMetadataKeys.length === 0) {
            return;
        }

        const dirtyKey = state.emergencyMetadataKeys[0];
        const dirtyAtKey = state.emergencyMetadataKeys[1];
        const reasonKey = state.emergencyMetadataKeys[2];
        const timestamp = new Date().toISOString();

        if (dirtyKey) {
            localStorageSet(dirtyKey, 'true');
        }
        if (dirtyAtKey) {
            localStorageSet(dirtyAtKey, timestamp);
        }
        if (reasonKey) {
            localStorageSet(reasonKey, reason || 'write_failed');
        }
    }

    function clearEmergencyFallbackMetadata() {
        state.emergencyMetadataKeys.forEach(function (key) {
            localStorageRemove(key);
        });
    }

    function clearEmergencyFallbackMetadataIfPresent() {
        if (readEmergencyFallbackMetadata().dirty) {
            clearEmergencyFallbackMetadata();
        }
    }

    function safeParseTabsSummary(snapshot) {
        const emptySummary = {
            valid: false,
            tabCount: 0,
            itemCount: 0
        };

        if (!snapshot || typeof snapshot.tabs !== 'string' || snapshot.tabs.length === 0) {
            return emptySummary;
        }

        try {
            const parsedTabs = JSON.parse(snapshot.tabs);
            if (!Array.isArray(parsedTabs)) {
                return emptySummary;
            }

            const itemCount = parsedTabs.reduce(function (count, tab) {
                if (!tab || !Array.isArray(tab.items)) {
                    return count;
                }
                return count + tab.items.length;
            }, 0);

            return {
                valid: true,
                tabCount: parsedTabs.length,
                itemCount: itemCount
            };
        } catch (error) {
            return emptySummary;
        }
    }

    function summarizeIndexedDBMeta(metaRecord) {
        if (!metaRecord || typeof metaRecord !== 'object') {
            return null;
        }

        return {
            migrationState: metaRecord.migrationState || null,
            source: metaRecord.source || null,
            migratedAt: metaRecord.migratedAt || null,
            lastWriteAt: metaRecord.lastWriteAt || null,
            snapshotKeyCount: typeof metaRecord.snapshotKeyCount === 'number' ? metaRecord.snapshotKeyCount : 0
        };
    }

    async function readIndexedDBMetaForDebug() {
        if (state.db && indexedDBStorage && typeof indexedDBStorage.readMetaRecord === 'function') {
            try {
                const metaRecord = await indexedDBStorage.readMetaRecord(state.db);
                if (metaRecord) {
                    return metaRecord;
                }
            } catch (error) {
                // Ignore debug-read failures and fall back to the in-memory copy.
            }
        }

        return state.meta || null;
    }

    function setThemeMirrorFromSnapshot(snapshot) {
        if (!state.themeMirrorKey) {
            return;
        }

        if (Object.prototype.hasOwnProperty.call(snapshot, state.themeMirrorKey)) {
            localStorageSet(state.themeMirrorKey, snapshot[state.themeMirrorKey]);
        } else {
            localStorageRemove(state.themeMirrorKey);
        }
    }

    function persistSnapshotToLocalStorage(snapshot) {
        state.managedKeys.forEach(function (key) {
            if (Object.prototype.hasOwnProperty.call(snapshot, key)) {
                localStorageSet(key, snapshot[key]);
            } else {
                localStorageRemove(key);
            }
        });
    }

    function snapshotsEqual(left, right) {
        const leftKeys = keysOfSnapshot(left).sort();
        const rightKeys = keysOfSnapshot(right).sort();
        if (leftKeys.length !== rightKeys.length) {
            return false;
        }

        for (let index = 0; index < leftKeys.length; index += 1) {
            const key = leftKeys[index];
            if (key !== rightKeys[index] || left[key] !== right[key]) {
                return false;
            }
        }

        return true;
    }

    function setCache(snapshot) {
        state.cache = cloneSnapshot(snapshot);
    }

    function recordPendingWrite() {
        state.pendingWriteGeneration += 1;
    }

    function scheduleIndexedDBFlush(reason) {
        if (!state.ready || state.mode !== modeIndexedDBPrimary) {
            return;
        }

        if (reason) {
            state.flushRequestedReason = reason;
        }

        if (state.flushTimer !== null) {
            return;
        }

        state.flushTimer = window.setTimeout(function () {
            state.flushTimer = null;
            storageAdapter.flushPendingWrites(state.flushRequestedReason).catch(function (error) {
                console.error('Failed to flush IndexedDB writes:', error);
            });
            state.flushRequestedReason = null;
        }, 0);
    }

    async function writeSnapshotAndMeta(db, snapshot, metaOverrides, generationToFlush) {
        const now = new Date().toISOString();
        const previousMeta = state.meta || {};
        const nextMeta = Object.assign({}, previousMeta, metaOverrides || {}, {
            id: 'primary',
            schemaVersion: 1,
            lastValidatedAt: now,
            lastWriteAt: now,
            snapshotKeyCount: keysOfSnapshot(snapshot).length
        });

        if (!nextMeta.migratedAt && nextMeta.migrationState && nextMeta.migrationState !== 'none') {
            nextMeta.migratedAt = now;
        }

        await indexedDBStorage.writePrimarySnapshot(db, snapshot);
        await indexedDBStorage.writeMetaRecord(db, nextMeta);

        const verifiedSnapshotRecord = await indexedDBStorage.readPrimarySnapshot(db);
        const verifiedValidation = indexedDBStorage.validateSnapshot(verifiedSnapshotRecord, state.managedKeys);
        if (!verifiedValidation.ok || !snapshotsEqual(verifiedValidation.data, snapshot)) {
            throw new Error('IndexedDB snapshot verification failed.');
        }

        state.meta = nextMeta;
        state.flushedWriteGeneration = generationToFlush;
    }

    async function repairIndexedDBFromSnapshot(db, snapshot, metaOverrides) {
        const normalizedSnapshot = cloneSnapshot(snapshot);
        const generationToFlush = state.pendingWriteGeneration;
        await writeSnapshotAndMeta(db, normalizedSnapshot, metaOverrides, generationToFlush);
        setCache(normalizedSnapshot);
        state.mode = modeIndexedDBPrimary;
        clearEmergencyFallbackMetadata();
        setThemeMirrorFromSnapshot(normalizedSnapshot);
    }

    async function activateFallbackMode(reason) {
        // This path is intentionally dirty: the live cache was newer than IndexedDB
        // and was copied into localStorage as the emergency current snapshot.
        persistSnapshotToLocalStorage(state.cache);
        setThemeMirrorFromSnapshot(state.cache);
        setEmergencyFallbackMetadata(reason || 'write_failed');
        state.mode = modeLegacyFallback;
        state.flushedWriteGeneration = state.pendingWriteGeneration;
    }

    function applyFallbackWrite(key, value) {
        if (!isManagedStorageKey(key)) {
            return;
        }

        if (value === null) {
            localStorageRemove(key);
        } else {
            localStorageSet(key, value);
        }

        if (key === state.themeMirrorKey) {
            if (value === null) {
                localStorageRemove(key);
            } else {
                localStorageSet(key, value);
            }
        }

        // Preserved legacy localStorage is only a backup until fallback writes occur.
        // This marker means localStorage may now be newer than IndexedDB.
        setEmergencyFallbackMetadata(getCurrentFallbackReason('write_failed'));
        state.flushedWriteGeneration = state.pendingWriteGeneration;
    }

    function flushTimerIfNeeded() {
        if (state.flushTimer !== null) {
            clearTimeout(state.flushTimer);
            state.flushTimer = null;
        }
    }

    const storageAdapter = {
        async bootstrap(options) {
            if (state.bootstrapPromise) {
                return state.bootstrapPromise;
            }

            normalizeManagedKeys(options || {});

            state.bootstrapPromise = (async function () {
                const localSnapshot = readRecognizedLocalSnapshot();
                const fallbackMetadata = readEmergencyFallbackMetadata();
                const startEmptyFallback = async function () {
                    setCache(localSnapshot.valid ? localSnapshot.data : {});
                    state.mode = modeLegacyFallback;
                    state.ready = true;
                    state.pendingWriteGeneration = 0;
                    state.flushedWriteGeneration = 0;
                    // Entering fallback because IndexedDB could not be used does not
                    // mean localStorage is newer. The dirty marker is reserved for
                    // fallback-mode writes and post-update IndexedDB write failures.
                    if (localSnapshot.valid) {
                        setThemeMirrorFromSnapshot(state.cache);
                    }
                };

                try {
                    if (!indexedDBStorage || typeof indexedDBStorage.openDatabase !== 'function') {
                        await startEmptyFallback();
                        return;
                    }

                    try {
                        state.db = await indexedDBStorage.openDatabase();
                    } catch (error) {
                        console.warn('IndexedDB unavailable during bootstrap, using legacy localStorage fallback.', error);
                        await startEmptyFallback();
                        return;
                    }

                    let snapshotRecord = null;
                    let metaRecord = null;
                    let validation = null;

                    try {
                        snapshotRecord = await indexedDBStorage.readPrimarySnapshot(state.db);
                        metaRecord = await indexedDBStorage.readMetaRecord(state.db);
                        validation = indexedDBStorage.validateSnapshot(snapshotRecord, state.managedKeys);
                    } catch (error) {
                        console.warn('IndexedDB read failed during bootstrap.', error);
                        if (fallbackMetadata.dirty && localSnapshot.valid && localSnapshot.hasData) {
                            await repairIndexedDBFromSnapshot(state.db, localSnapshot.data, {
                                migrationState: 'repaired',
                                source: 'fallback-repair'
                            });
                            state.ready = true;
                            return;
                        }
                        await startEmptyFallback();
                        return;
                    }

                    const hasMigrationMarker = Boolean(
                        metaRecord
                        && typeof metaRecord === 'object'
                        && metaRecord.migrationState
                        && metaRecord.migrationState !== 'none'
                    );

                    if (validation && validation.ok && hasMigrationMarker) {
                        if (fallbackMetadata.dirty && localSnapshot.valid && localSnapshot.hasData) {
                            await repairIndexedDBFromSnapshot(state.db, localSnapshot.data, {
                                migrationState: 'repaired',
                                source: 'fallback-repair',
                                migratedAt: metaRecord && metaRecord.migratedAt ? metaRecord.migratedAt : new Date().toISOString()
                            });
                            state.ready = true;
                            return;
                        }

                        clearEmergencyFallbackMetadataIfPresent();
                        state.meta = metaRecord || null;
                        setCache(validation.data);
                        state.mode = modeIndexedDBPrimary;
                        state.ready = true;
                        state.pendingWriteGeneration = 0;
                        state.flushedWriteGeneration = 0;
                        setThemeMirrorFromSnapshot(state.cache);
                        return;
                    }

                    if (validation && validation.ok && localSnapshot.valid && localSnapshot.hasData) {
                        await repairIndexedDBFromSnapshot(state.db, localSnapshot.data, {
                            migrationState: 'complete',
                            source: 'localStorage'
                        });
                        state.ready = true;
                        return;
                    }

                    if ((!validation || !validation.ok) && localSnapshot.valid) {
                        await repairIndexedDBFromSnapshot(state.db, localSnapshot.data, {
                            migrationState: 'repaired',
                            source: 'fallback-repair'
                        });
                        state.ready = true;
                        return;
                    }

                    if (validation && validation.ok) {
                        clearEmergencyFallbackMetadataIfPresent();
                        state.meta = metaRecord || null;
                        setCache(validation.data);
                        state.mode = modeIndexedDBPrimary;
                        state.ready = true;
                        state.pendingWriteGeneration = 0;
                        state.flushedWriteGeneration = 0;
                        setThemeMirrorFromSnapshot(state.cache);
                        return;
                    }

                    setCache({});
                    state.mode = modeIndexedDBPrimary;
                    state.meta = metaRecord || null;
                    state.ready = true;
                    state.pendingWriteGeneration = 0;
                    state.flushedWriteGeneration = 0;
                    clearEmergencyFallbackMetadataIfPresent();
                    setThemeMirrorFromSnapshot(state.cache);
                } catch (error) {
                    console.warn('IndexedDB bootstrap migration failed, using legacy localStorage fallback.', error);
                    await startEmptyFallback();
                }
            })();

            return state.bootstrapPromise;
        },

        async flushPendingWrites(reason) {
            flushTimerIfNeeded();
            if (!state.ready) {
                await state.bootstrapPromise;
            }

            if (state.mode === modeLegacyFallback) {
                if (state.pendingWriteGeneration === state.flushedWriteGeneration) {
                    setThemeMirrorFromSnapshot(state.cache);
                    return;
                }

                persistSnapshotToLocalStorage(state.cache);
                setThemeMirrorFromSnapshot(state.cache);
                // Only mark dirty after actual fallback writes changed the current snapshot.
                setEmergencyFallbackMetadata(getCurrentFallbackReason('write_failed'));
                state.flushedWriteGeneration = state.pendingWriteGeneration;
                return;
            }

            if (state.pendingWriteGeneration === state.flushedWriteGeneration) {
                setThemeMirrorFromSnapshot(state.cache);
                return;
            }

            if (state.flushPromise) {
                state.flushRequestedWhileBusy = true;
                if (reason) {
                    state.flushRequestedReason = reason;
                }
                await state.flushPromise;
                if (state.pendingWriteGeneration !== state.flushedWriteGeneration) {
                    return storageAdapter.flushPendingWrites(reason);
                }
                return;
            }

            state.flushPromise = (async function () {
                try {
                    const generationToFlush = state.pendingWriteGeneration;
                    const snapshotToFlush = cloneSnapshot(state.cache);
                    await writeSnapshotAndMeta(state.db, snapshotToFlush, {
                        migrationState: state.meta && state.meta.migrationState ? state.meta.migrationState : 'complete',
                        source: 'indexeddb',
                        migratedAt: state.meta && state.meta.migratedAt ? state.meta.migratedAt : new Date().toISOString()
                    }, generationToFlush);
                    clearEmergencyFallbackMetadata();
                    setThemeMirrorFromSnapshot(state.cache);
                } catch (error) {
                    console.error('IndexedDB write failed, switching to localStorage fallback for this session.', error);
                    await activateFallbackMode('write_failed');
                } finally {
                    state.flushPromise = null;
                    const needsAnotherFlush = state.flushRequestedWhileBusy || state.pendingWriteGeneration !== state.flushedWriteGeneration;
                    state.flushRequestedWhileBusy = false;
                    if (needsAnotherFlush) {
                        await storageAdapter.flushPendingWrites(state.flushRequestedReason || reason || 'queued');
                    }
                    state.flushRequestedReason = null;
                }
            })();

            return state.flushPromise;
        },

        getItem(key) {
            if (Object.prototype.hasOwnProperty.call(state.cache, key)) {
                return state.cache[key];
            }
            return null;
        },

        setItem(key, value) {
            if (!isManagedStorageKey(key)) {
                return;
            }

            const nextValue = String(value);
            if (state.cache[key] === nextValue) {
                return;
            }
            state.cache[key] = nextValue;
            recordPendingWrite();

            if (state.mode === modeLegacyFallback) {
                applyFallbackWrite(key, nextValue);
                return;
            }

            if (key === state.themeMirrorKey) {
                localStorageSet(key, nextValue);
            }
            scheduleIndexedDBFlush(`set:${key}`);
        },

        removeItem(key) {
            if (!isManagedStorageKey(key)) {
                return;
            }

            if (!Object.prototype.hasOwnProperty.call(state.cache, key)) {
                return;
            }
            delete state.cache[key];
            recordPendingWrite();

            if (state.mode === modeLegacyFallback) {
                applyFallbackWrite(key, null);
                return;
            }

            if (key === state.themeMirrorKey) {
                localStorageRemove(key);
            }
            scheduleIndexedDBFlush(`remove:${key}`);
        },

        clear() {
            const managedKeysToRemove = Object.keys(state.cache).filter(function (key) {
                return isManagedStorageKey(key);
            });

            if (managedKeysToRemove.length === 0) {
                return;
            }

            managedKeysToRemove.forEach(function (key) {
                delete state.cache[key];
            });
            recordPendingWrite();

            if (state.mode === modeLegacyFallback) {
                persistSnapshotToLocalStorage(state.cache);
                setThemeMirrorFromSnapshot(state.cache);
                // A fallback-mode clear changes the emergency current snapshot.
                setEmergencyFallbackMetadata(getCurrentFallbackReason('write_failed'));
                state.flushedWriteGeneration = state.pendingWriteGeneration;
                return;
            }

            localStorageRemove(state.themeMirrorKey);
            scheduleIndexedDBFlush('clear');
        },

        keys() {
            return Object.keys(state.cache);
        },

        getJSON(key) {
            const value = storageAdapter.getItem(key);
            return value === null ? null : JSON.parse(value);
        },

        setJSON(key, obj) {
            storageAdapter.setItem(key, JSON.stringify(obj));
        },

        async getDebugState() {
            const fallbackMetadata = readEmergencyFallbackMetadata();
            const tabsSummary = safeParseTabsSummary(state.cache);
            const metaRecord = await readIndexedDBMetaForDebug();

            return {
                mode: state.mode,
                ready: state.ready,
                managedKeys: state.managedKeys.slice(),
                cacheKeys: Object.keys(state.cache).sort(),
                hasTabs: tabsSummary.tabCount > 0,
                tabCount: tabsSummary.tabCount,
                itemCount: tabsSummary.itemCount,
                hasSyncVersion: Object.prototype.hasOwnProperty.call(state.cache, 'postbabySyncVersion'),
                syncVersion: Object.prototype.hasOwnProperty.call(state.cache, 'postbabySyncVersion')
                    ? state.cache.postbabySyncVersion
                    : null,
                pendingImportAdopt: Object.prototype.hasOwnProperty.call(state.cache, 'postbabyPendingImportAdopt')
                    ? state.cache.postbabyPendingImportAdopt
                    : null,
                themeFromCache: Object.prototype.hasOwnProperty.call(state.cache, state.themeMirrorKey)
                    ? state.cache[state.themeMirrorKey]
                    : null,
                themeFromLocalStorage: state.themeMirrorKey ? localStorageGet(state.themeMirrorKey) : null,
                fallbackDirty: fallbackMetadata.dirty,
                fallbackDirtyAt: fallbackMetadata.dirtyAt,
                fallbackReason: fallbackMetadata.reason,
                indexedDBMeta: summarizeIndexedDBMeta(metaRecord)
            };
        }
    };

    window.storageAdapter = storageAdapter;
})();
