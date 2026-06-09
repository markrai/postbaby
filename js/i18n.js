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
    const BUILTIN_ENGLISH_CATALOG = JSON.parse(`{
  "common": {
    "languages": {
      "english": "English",
      "pseudo": "Pseudo"
    }
  },
  "settings": {
    "trigger": "Settings",
    "close": "Close Settings Modal",
    "title": "Settings",
    "sections": "Settings Sections",
    "tabs": {
      "preferences": "Preferences",
      "importExport": "Import & Export"
    },
    "preferences": {
      "darkMode": "Dark Mode",
      "disableColorChange": "Disable Color Change",
      "disableNoteResize": "Disable Note Resize",
      "fullScreen": "Full Screen",
      "corporateMode": "Corporate Mode",
      "hideCameraRemote": "Hide Camera Remote",
      "language": "Language",
      "useGrids": "Use Grids (Tab Specific)",
      "defaultColor": "Default Color for New Items (Global)"
    },
    "gridOptions": {
      "none": "None",
      "kanban": "Kanban Board",
      "eisenhower": "Eisenhower Matrix",
      "priority": "Priority Matrix",
      "smartgoals": "SMART Goals",
      "swot": "SWOT Analysis",
      "calendar": "Calendar",
      "nowlater": "Now & Later",
      "week": "Week"
    },
    "recovery": {
      "title": "Recovery (Development Only)",
      "body": "Temporary scroll-only recovery controls for debugging offscreen notes without changing stored positions.",
      "showAll": "Show All Notes",
      "jumpNewest": "Jump To Newest",
      "jumpLastEdited": "Jump To Last Edited"
    },
    "questionsComments": "Questions/Comments:",
    "staticNote": "Note: This static version saves notes only in this browser on this device. Account sign-in, cloud sync, billing, and cross-device recovery are not available in this mode. Clearing site data or using a different browser/device can make these notes unavailable.",
    "actions": {
      "exportData": "Export Data",
      "importData": "Import Data",
      "importMermaid": "Import Mermaid"
    },
    "mermaid": {
      "title": "Mermaid Import",
      "descriptionBeforeAction": "Turn a supported Mermaid flowchart or graph into ordinary Postbaby notes and edges. Paste Mermaid source below, then click",
      "descriptionAfterAction": "when you are ready to create notes.",
      "placeholder": "flowchart LR\\nIdea --> Task\\nTask --> Done"
    },
    "toasts": {
      "loadDataInputMissing": "Error: Unable to find load data input.",
      "noFileSelected": "No file selected.",
      "importedLocallyReloading": "Imported locally. Reloading...",
      "importCanceled": "Import canceled.",
      "importedAndSyncedReloading": "Imported and synced. Reloading...",
      "importedLocalSignInAgain": "Imported locally. Sign in again to sync this backup. Reloading...",
      "importedLocalReconnect": "Imported locally. Reconnect to sync this backup. Reloading...",
      "importedLocalServerAvailable": "Imported locally. Postbaby will sync this backup when the server is available. Reloading...",
      "invalidFileFormat": "Failed to load data: Invalid file format.",
      "fileReadFailed": "Failed to read the file.",
      "dataSavedSuccessfully": "Data saved successfully."
    }
  },
  "shortcuts": {
    "trigger": "Keyboard shortcuts",
    "close": "Close Shortcuts",
    "title": "Shortcuts",
    "desktop": {
      "create": {
        "action": "create new:",
        "prefix": "n or",
        "gesture": "right-click on canvas"
      },
      "changeColor": {
        "action": "change color:",
        "gesture": "single-click item"
      },
      "editItem": {
        "action": "edit item:",
        "gesture": "double-click item"
      },
      "deleteItem": {
        "action": "delete item:",
        "gesture": "right-click item or drag to toilet-roll"
      },
      "nextShape": {
        "action": "next shape:",
        "gesture": "ctrl + right-click item"
      },
      "previousShape": {
        "action": "previous shape:",
        "gesture": "shift + ctrl + right-click item"
      },
      "clearCurrentTab": {
        "action": "clear current tab:",
        "keys": "c"
      },
      "deleteAllTabs": {
        "action": "delete all tabs:",
        "keys": "ctrl+c"
      },
      "jumpTabs": {
        "action": "jump tabs:",
        "keys": "keys 1-9"
      },
      "toggleGrids": {
        "action": "toggle between grids:",
        "keys": "g"
      },
      "selectPanMode": {
        "action": "select / pan mode:",
        "gesture": "hand/select toggle (top-right)"
      },
      "emptyCanvasDrag": {
        "action": "empty-canvas drag:",
        "gesture": "marquee in select mode / pan in hand mode"
      },
      "drawLine": {
        "action": "draw line:",
        "gesture": "shift + mouse-drag"
      },
      "drawArrow": {
        "action": "draw arrow:",
        "gesture": "ctrl + mouse-drag"
      },
      "panCanvas": {
        "action": "pan canvas:",
        "gesture": "mouse wheel, trackpad scroll, shift + wheel, middle-drag, or space + drag"
      },
      "zoomCanvas": {
        "action": "zoom canvas:",
        "gesture": "ctrl/cmd + wheel or camera controls"
      },
      "resetFitView": {
        "action": "reset / fit view:",
        "prefix": "0 / f or",
        "gesture": "camera controls"
      },
      "settings": {
        "action": "settings:",
        "gesture": "click the settings button (top-right)"
      }
    },
    "mobile": {
      "create": {
        "action": "create new:",
        "gesture": "long-press on canvas"
      },
      "edit": {
        "action": "edit:",
        "gesture": "double-tap on item or tab"
      },
      "delete": {
        "action": "delete:",
        "gesture": "long-press on item or tab"
      },
      "changeColor": {
        "action": "change color:",
        "gesture": "single tap on item or tab"
      },
      "navigateCanvas": {
        "action": "navigate canvas:",
        "gesture": "two-finger pinch / pan or camera controls"
      },
      "selectPanMode": {
        "action": "select or pan mode:",
        "gesture": "use the hand/select toggle (top-right)"
      },
      "settings": {
        "action": "settings:",
        "gesture": "tap the settings button (top-right)"
      }
    }
  },
  "mermaid": {
    "status": {
      "completedTitle": "Mermaid import completed.",
      "failedTitle": "Mermaid import failed.",
      "createdSummary": "Created {notes} and {edges}.",
      "warningsSummary": "Review the warnings below if you want to simplify or adjust the imported shapes.",
      "ordinarySummary": "These are now ordinary Postbaby notes and edges.",
      "validationFailedMessage": "Nothing was imported. Fix the Mermaid source below and try again.",
      "unexpectedFailedMessage": "Nothing was imported. An unexpected error prevented Mermaid import."
    },
    "groups": {
      "errors": "Errors",
      "warnings": "Warnings"
    },
    "count": {
      "note": {
        "one": "{count} note",
        "other": "{count} notes"
      },
      "edge": {
        "one": "{count} edge",
        "other": "{count} edges"
      }
    },
    "issue": {
      "linePrefix": "Line {line}: {message}",
      "unknown": "Unknown Mermaid import issue."
    },
    "issues": {
      "unexpectedMermaidImportError": "Unexpected Mermaid import error.",
      "unsupportedDiagramType": "Unsupported Mermaid diagram type \\"{diagramType}\\". Only flowchart/graph TD, TB, and LR are supported.",
      "invalidDiagramHeader": "Mermaid source must start with flowchart/graph TD, TB, or LR.",
      "invalidNodeReference": "Node references must not be empty.",
      "nodeLabelTooLong": "Node label exceeds {maxChars} characters.",
      "unsupportedNodeShape": "Mermaid parallelogram nodes are not supported yet and were normalized to the default Postbaby shape.",
      "unsupportedNodeSyntax": "Unsupported Mermaid node syntax: {syntax}",
      "conflictingNodeLabel": "Node \\"{nodeId}\\" was defined with multiple labels. Keeping the first non-empty label \\"{label}\\".",
      "conflictingNodeShape": "Node \\"{nodeId}\\" was defined with multiple shapes. Keeping the earlier Mermaid-compatible shape mapping \\"{shape}\\".",
      "unsupportedEdgeSyntax": "Unsupported Mermaid edge syntax: {syntax}",
      "invalidSource": "Mermaid source must be a string.",
      "ignoredMermaidStatement": "Ignoring unsupported Mermaid {statementType} statement.",
      "missingDiagramHeader": "Mermaid source must include a supported flowchart/graph header.",
      "missingActiveTab": "Postbaby does not have an active tab for graph creation.",
      "missingActiveTabRecord": "The active tab record could not be found for graph creation.",
      "missingActiveGrid": "The active grid container is not available for graph creation.",
      "tabItemLimitExceeded": "This import is too large for one tab.",
      "tabEdgeLimitExceeded": "This import is too large for one tab."
    }
  }
}`);
    const catalogs = Object.create(null);
    catalogs[DEFAULT_LOCALE] = BUILTIN_ENGLISH_CATALOG;
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

    async function loadCatalog(code) {
        const normalizedCode = normalizeLocaleCode(code);
        if (catalogs[normalizedCode]) {
            return catalogs[normalizedCode];
        }

        const response = await fetch(SUPPORTED_LOCALES[normalizedCode].file, {
            credentials: 'same-origin'
        });
        if (!response.ok) {
            throw new Error(`Failed to load locale catalog: ${normalizedCode}`);
        }

        const data = await response.json();
        catalogs[normalizedCode] = data && typeof data === 'object' ? data : {};
        return catalogs[normalizedCode];
    }

    function collectTranslatableElements(rootElement) {
        const selectors = [
            '[data-i18n]',
            '[data-i18n-aria-label]',
            '[data-i18n-title]',
            '[data-i18n-placeholder]'
        ].join(', ');
        const root = rootElement && rootElement.nodeType === 1
            ? rootElement
            : document;
        const elements = [];

        if (root.nodeType === 1 && root.matches(selectors)) {
            elements.push(root);
        }

        if (typeof root.querySelectorAll === 'function') {
            elements.push.apply(elements, root.querySelectorAll(selectors));
        }

        return elements;
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

        try {
            await loadCatalog(DEFAULT_LOCALE);
            if (nextLocale !== DEFAULT_LOCALE) {
                await loadCatalog(nextLocale);
            }
        } catch (error) {
            console.error('Failed to initialize locale catalog:', error);
            nextLocale = DEFAULT_LOCALE;
            try {
                await loadCatalog(DEFAULT_LOCALE);
            } catch (fallbackError) {
                console.error('Failed to initialize fallback locale catalog:', fallbackError);
            }
        }

        activeLocale = nextLocale;
        persistLocale(activeLocale);
        updateDocumentLanguage(activeLocale);
        apply(document);
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

        return normalizedKey;
    }

    updateDocumentLanguage(activeLocale);

    window.PostbabyI18n = Object.freeze({
        getLocale: getLocale,
        setLocale: setLocale,
        t: t,
        apply: apply
    });
})();
