(function () {
    const MIN_CANVAS_COORD = -100000;
    const MAX_CANVAS_COORD = 100000;
    const MAX_ITEMS_PER_TAB = 500;
    const MAX_EDGES_PER_TAB = 2000;
    const MAX_ITEM_TEXT_CHARS = 4000;
    const MAX_GRAPH_IMPORT_NODES = 100;
    const MAX_GRAPH_IMPORT_EDGES = 300;
    const MAX_GRAPH_LABEL_CHARS = 240;

    function normalizeFiniteCoord(value, fallback) {
        if (Number.isFinite(value)) {
            return Math.round(value);
        }

        return Number.isFinite(fallback) ? Math.round(fallback) : 0;
    }

    function clampCanvasCoord(value, fallback) {
        const numericValue = normalizeFiniteCoord(value, fallback);
        return Math.min(MAX_CANVAS_COORD, Math.max(MIN_CANVAS_COORD, numericValue));
    }

    function clampItemPositionXY(x, y) {
        return {
            x: clampCanvasCoord(x, 0),
            y: clampCanvasCoord(y, 0)
        };
    }

    function isCanvasCoordWithinBounds(value) {
        return Number.isFinite(value) && value >= MIN_CANVAS_COORD && value <= MAX_CANVAS_COORD;
    }

    function isItemPositionWithinCanvas(x, y) {
        return isCanvasCoordWithinBounds(x) && isCanvasCoordWithinBounds(y);
    }

    window.PostbabyCanvasLimits = Object.freeze({
        MIN_CANVAS_COORD,
        MAX_CANVAS_COORD,
        MAX_ITEMS_PER_TAB,
        MAX_EDGES_PER_TAB,
        MAX_ITEM_TEXT_CHARS,
        MAX_GRAPH_IMPORT_NODES,
        MAX_GRAPH_IMPORT_EDGES,
        MAX_GRAPH_LABEL_CHARS,
        clampCanvasCoord,
        clampItemPositionXY,
        isCanvasCoordWithinBounds,
        isItemPositionWithinCanvas
    });
})();
