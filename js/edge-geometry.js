(function () {
    const geometryDom = window.PostbabyGeometryDom;
    if (!geometryDom) {
        throw new Error('PostbabyGeometryDom is missing.');
    }

    const {
        getContainerRelativePointFromClient,
        getElementOffsetCenter
    } = geometryDom;

    const EDGE_KIND_LINE = 'line';
    const EDGE_KIND_ARROW = 'arrow';

    function normalizeEdgeKind(edgeOrKind) {
        const rawKind = typeof edgeOrKind === 'string'
            ? edgeOrKind
            : edgeOrKind && typeof edgeOrKind === 'object'
                ? edgeOrKind.kind
                : undefined;
        return rawKind === EDGE_KIND_ARROW ? EDGE_KIND_ARROW : EDGE_KIND_LINE;
    }

    function getRectBoundaryAnchorFromOffsetCenters(fromCenter, toCenter, targetElement, inset) {
        const dx = toCenter.cx - fromCenter.cx;
        const dy = toCenter.cy - fromCenter.cy;
        const distance = Math.sqrt((dx * dx) + (dy * dy));
        if (!distance) {
            return {
                x: toCenter.cx,
                y: toCenter.cy
            };
        }

        const halfWidth = targetElement.offsetWidth / 2;
        const halfHeight = targetElement.offsetHeight / 2;
        const scaleX = dx === 0 ? Infinity : halfWidth / Math.abs(dx);
        const scaleY = dy === 0 ? Infinity : halfHeight / Math.abs(dy);
        const scale = Math.min(scaleX, scaleY);
        const boundaryPoint = {
            x: toCenter.cx - (dx * scale),
            y: toCenter.cy - (dy * scale)
        };

        return {
            x: boundaryPoint.x - ((dx / distance) * inset),
            y: boundaryPoint.y - ((dy / distance) * inset)
        };
    }

    function getEdgeLinePointsFromElements(fromElement, toElement, edgeKind, inset) {
        const start = getElementOffsetCenter(fromElement);
        const targetCenter = getElementOffsetCenter(toElement);
        const normalizedKind = normalizeEdgeKind(edgeKind);
        const end = normalizedKind === EDGE_KIND_ARROW
            ? getRectBoundaryAnchorFromOffsetCenters(start, targetCenter, toElement, inset)
            : {
                x: targetCenter.cx,
                y: targetCenter.cy
            };
        return {
            start: start,
            end: end,
            targetCenter: targetCenter,
            edgeKind: normalizedKind
        };
    }

    function getEdgePreviewPointFromClient(clientX, clientY, containerElement) {
        return getContainerRelativePointFromClient(clientX, clientY, containerElement);
    }

    window.PostbabyEdgeGeometry = Object.freeze({
        EDGE_KIND_LINE,
        EDGE_KIND_ARROW,
        normalizeEdgeKind,
        getRectBoundaryAnchorFromOffsetCenters,
        getEdgeLinePointsFromElements,
        getEdgePreviewPointFromClient
    });
})();
