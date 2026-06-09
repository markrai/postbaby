(function () {
    const DEFAULT_LOCALE = 'en';
    const LOCALE_STORAGE_KEY = 'postbabyLocale';
    const SUPPORTED_LOCALES = Object.freeze({
        en: {
            lang: 'en',
            file: './locales/en.json'
        },
        pseudo: {
            lang: 'en-xa',
            file: './locales/pseudo.json'
        }
    });
    // Keep this fallback intentionally small. Static English copy comes from the
    // DOM markup, while locales/en.json remains the canonical catalog when it
    // loads successfully.
    const EMBEDDED_DYNAMIC_ENGLISH_FALLBACK = Object.freeze({
        settings: {
            toasts: {
                loadDataInputMissing: 'Error: Unable to find load data input.',
                noFileSelected: 'No file selected.',
                importedLocallyReloading: 'Imported locally. Reloading...',
                importCanceled: 'Import canceled.',
                importedAndSyncedReloading: 'Imported and synced. Reloading...',
                importedLocalSignInAgain: 'Imported locally. Sign in again to sync this backup. Reloading...',
                importedLocalReconnect: 'Imported locally. Reconnect to sync this backup. Reloading...',
                importedLocalServerAvailable: 'Imported locally. Postbaby will sync this backup when the server is available. Reloading...',
                invalidFileFormat: 'Failed to load data: Invalid file format.',
                fileReadFailed: 'Failed to read the file.',
                dataSavedSuccessfully: 'Data saved successfully.'
            }
        },
        mermaid: {
            status: {
                completedTitle: 'Mermaid import completed.',
                failedTitle: 'Mermaid import failed.',
                createdSummary: 'Created {notes} and {edges}.',
                warningsSummary: 'Review the warnings below if you want to simplify or adjust the imported shapes.',
                ordinarySummary: 'These are now ordinary Postbaby notes and edges.',
                validationFailedMessage: 'Nothing was imported. Fix the Mermaid source below and try again.',
                unexpectedFailedMessage: 'Nothing was imported. An unexpected error prevented Mermaid import.'
            },
            groups: {
                errors: 'Errors',
                warnings: 'Warnings'
            },
            count: {
                note: {
                    one: '{count} note',
                    other: '{count} notes'
                },
                edge: {
                    one: '{count} edge',
                    other: '{count} edges'
                }
            },
            issue: {
                linePrefix: 'Line {line}: {message}',
                unknown: 'Unknown Mermaid import issue.'
            },
            issues: {
                unexpectedMermaidImportError: 'Unexpected Mermaid import error.',
                unsupportedDiagramType: 'Unsupported Mermaid diagram type "{diagramType}". Only flowchart/graph TD, TB, and LR are supported.',
                invalidDiagramHeader: 'Mermaid source must start with flowchart/graph TD, TB, or LR.',
                invalidNodeReference: 'Node references must not be empty.',
                nodeLabelTooLong: 'Node label exceeds {maxChars} characters.',
                unsupportedNodeShape: 'Mermaid parallelogram nodes are not supported yet and were normalized to the default Postbaby shape.',
                unsupportedNodeSyntax: 'Unsupported Mermaid node syntax: {syntax}',
                conflictingNodeLabel: 'Node "{nodeId}" was defined with multiple labels. Keeping the first non-empty label "{label}".',
                conflictingNodeShape: 'Node "{nodeId}" was defined with multiple shapes. Keeping the earlier Mermaid-compatible shape mapping "{shape}".',
                unsupportedEdgeSyntax: 'Unsupported Mermaid edge syntax: {syntax}',
                invalidSource: 'Mermaid source must be a string.',
                ignoredMermaidStatement: 'Ignoring unsupported Mermaid {statementType} statement.',
                missingDiagramHeader: 'Mermaid source must include a supported flowchart/graph header.',
                missingActiveTab: 'Postbaby does not have an active tab for graph creation.',
                missingActiveTabRecord: 'The active tab record could not be found for graph creation.',
                missingActiveGrid: 'The active grid container is not available for graph creation.',
                tabItemLimitExceeded: 'This import is too large for one tab.',
                tabEdgeLimitExceeded: 'This import is too large for one tab.'
            }
        }
    });
    const TRANSLATABLE_SELECTORS = [
        '[data-i18n]',
        '[data-i18n-aria-label]',
        '[data-i18n-title]',
        '[data-i18n-placeholder]'
    ].join(', ');
    const catalogs = Object.create(null);
    const catalogPromises = Object.create(null);
    const domEnglishFallbacks = Object.create(null);
    let activeLocale = readStoredLocale();

    function normalizeLocaleCode(code) {
        return Object.prototype.hasOwnProperty.call(SUPPORTED_LOCALES, code)
            ? code
            : DEFAULT_LOCALE;
    }

    function readStoredLocale() {
        try {
            return normalizeLocaleCode(window.localStorage.getItem(LOCALE_STORAGE_KEY));
        } catch (error) {
            return DEFAULT_LOCALE;
        }
    }

    function persistLocale(code) {
        try {
            window.localStorage.setItem(LOCALE_STORAGE_KEY, code);
        } catch (error) {
        }
    }

    function updateDocumentLanguage(code) {
        if (document && document.documentElement) {
            document.documentElement.lang = SUPPORTED_LOCALES[code].lang;
        }
    }

    function getMessageValue(source, key) {
        return key.split('.').reduce(function (value, part) {
            if (!value || typeof value !== 'object') {
                return undefined;
            }
            return value[part];
        }, source);
    }

    function interpolateMessage(template, params) {
        return template.replace(/\{([A-Za-z0-9_]+)\}/g, function (match, token) {
            if (!params || !Object.prototype.hasOwnProperty.call(params, token)) {
                return match;
            }
            const value = params[token];
            return value === null || value === undefined ? '' : String(value);
        });
    }

    function formatMessage(value, params) {
        if (value && typeof value === 'object' && !Array.isArray(value)) {
            if (params && Number.isFinite(params.count)) {
                value = params.count === 1 ? value.one : value.other;
            } else if (typeof value.other === 'string') {
                value = value.other;
            }
        }

        if (typeof value !== 'string') {
            return '';
        }

        return interpolateMessage(value, params);
    }

    function rememberDomEnglishFallback(key, value) {
        if (typeof key !== 'string' || !key) {
            return;
        }
        if (typeof value !== 'string' || !value) {
            return;
        }
        if (!Object.prototype.hasOwnProperty.call(domEnglishFallbacks, key)) {
            domEnglishFallbacks[key] = value;
        }
    }

    function collectTranslatableElements(rootElement) {
        const root = rootElement && rootElement.nodeType === 1
            ? rootElement
            : document;
        const elements = [];

        if (root.nodeType === 1 && root.matches(TRANSLATABLE_SELECTORS)) {
            elements.push(root);
        }

        if (typeof root.querySelectorAll === 'function') {
            elements.push.apply(elements, root.querySelectorAll(TRANSLATABLE_SELECTORS));
        }

        return elements;
    }

    function captureDomEnglishFallbacks(rootElement) {
        collectTranslatableElements(rootElement).forEach(function (element) {
            if (element.dataset.i18n) {
                rememberDomEnglishFallback(element.dataset.i18n, element.textContent);
            }
            if (element.dataset.i18nAriaLabel) {
                rememberDomEnglishFallback(
                    element.dataset.i18nAriaLabel,
                    element.getAttribute('aria-label')
                );
            }
            if (element.dataset.i18nTitle) {
                rememberDomEnglishFallback(
                    element.dataset.i18nTitle,
                    element.getAttribute('title')
                );
            }
            if (element.dataset.i18nPlaceholder) {
                rememberDomEnglishFallback(
                    element.dataset.i18nPlaceholder,
                    element.getAttribute('placeholder')
                );
            }
        });
    }

    function getFallbackMessage(key, params) {
        const domFallback = formatMessage(domEnglishFallbacks[key], params);
        if (domFallback) {
            return domFallback;
        }

        return formatMessage(
            getMessageValue(EMBEDDED_DYNAMIC_ENGLISH_FALLBACK, key),
            params
        );
    }

    async function loadCatalog(code) {
        const normalizedCode = normalizeLocaleCode(code);
        if (catalogs[normalizedCode]) {
            return catalogs[normalizedCode];
        }

        if (catalogPromises[normalizedCode]) {
            return catalogPromises[normalizedCode];
        }

        catalogPromises[normalizedCode] = fetch(SUPPORTED_LOCALES[normalizedCode].file, {
            credentials: 'same-origin'
        }).then(function (response) {
            if (!response.ok) {
                throw new Error(`Failed to load locale catalog: ${normalizedCode}`);
            }
            return response.json();
        }).then(function (data) {
            catalogs[normalizedCode] = data && typeof data === 'object' ? data : {};
            return catalogs[normalizedCode];
        }).finally(function () {
            delete catalogPromises[normalizedCode];
        });

        return catalogPromises[normalizedCode];
    }

    function warmEnglishCatalog() {
        loadCatalog(DEFAULT_LOCALE).then(function () {
            if (activeLocale === DEFAULT_LOCALE) {
                apply(document);
            }
        }).catch(function (error) {
            console.warn('Failed to load English locale catalog, using built-in fallbacks.', error);
        });
    }

    function apply(rootElement) {
        collectTranslatableElements(rootElement).forEach(function (element) {
            if (element.dataset.i18n) {
                element.textContent = t(element.dataset.i18n);
            }
            if (element.dataset.i18nAriaLabel) {
                element.setAttribute('aria-label', t(element.dataset.i18nAriaLabel));
            }
            if (element.dataset.i18nTitle) {
                element.setAttribute('title', t(element.dataset.i18nTitle));
            }
            if (element.dataset.i18nPlaceholder) {
                element.setAttribute('placeholder', t(element.dataset.i18nPlaceholder));
            }
        });
    }

    function getLocale() {
        return activeLocale;
    }

    async function setLocale(code) {
        let nextLocale = normalizeLocaleCode(code);

        if (nextLocale !== DEFAULT_LOCALE) {
            try {
                await loadCatalog(nextLocale);
            } catch (error) {
                console.error('Failed to initialize locale catalog:', error);
                nextLocale = DEFAULT_LOCALE;
            }
        }

        activeLocale = nextLocale;
        persistLocale(activeLocale);
        updateDocumentLanguage(activeLocale);
        apply(document);
        warmEnglishCatalog();
        return activeLocale;
    }

    function t(key, params) {
        const normalizedKey = typeof key === 'string' ? key : '';
        if (!normalizedKey) {
            return '';
        }

        const currentMessage = formatMessage(
            getMessageValue(catalogs[activeLocale], normalizedKey),
            params
        );
        if (currentMessage) {
            return currentMessage;
        }

        const fallbackMessage = formatMessage(
            getMessageValue(catalogs[DEFAULT_LOCALE], normalizedKey),
            params
        );
        if (fallbackMessage) {
            return fallbackMessage;
        }

        const builtInFallback = getFallbackMessage(normalizedKey, params);
        if (builtInFallback) {
            return builtInFallback;
        }

        return normalizedKey;
    }

    captureDomEnglishFallbacks(document);
    updateDocumentLanguage(activeLocale);
    warmEnglishCatalog();

    window.PostbabyI18n = Object.freeze({
        getLocale: getLocale,
        setLocale: setLocale,
        t: t,
        apply: apply
    });
})();
