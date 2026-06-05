(function () {
    const geometryDom = window.PostbabyGeometryDom;
    if (!geometryDom) {
        throw new Error('PostbabyGeometryDom is missing.');
    }
    const shapeGeometry = window.PostbabyShapeGeometry;
    if (!shapeGeometry) {
        throw new Error('PostbabyShapeGeometry is missing.');
    }

    const {
        getContainerRelativePointFromClient,
        getElementOffsetCenter,
        getElementOffsetRect
    } = geometryDom;
    const {
        normalizeItemShape
    } = shapeGeometry;

    const EDGE_KIND_LINE = 'line';
    const EDGE_KIND_ARROW = 'arrow';
    const EPSILON = 0.000001;

    function normalizeEdgeKind(edgeOrKind) {
        const rawKind = typeof edgeOrKind === 'string'
            ? edgeOrKind
            : edgeOrKind && typeof edgeOrKind === 'object'
                ? edgeOrKind.kind
                : undefined;
        return rawKind === EDGE_KIND_ARROW ? EDGE_KIND_ARROW : EDGE_KIND_LINE;
    }

    function getBoundsCenter(bounds) {
        return {
            cx: bounds.left + bounds.width / 2,
            cy: bounds.top + bounds.height / 2
        };
    }

    function applyShapeAnchorInset(boundaryPoint, center, externalPoint, inset) {
        if (!Number.isFinite(inset) || inset === 0) {
            return boundaryPoint;
        }

        const dx = externalPoint.x - center.cx;
        const dy = externalPoint.y - center.cy;
        const distance = Math.sqrt((dx * dx) + (dy * dy));
        if (!distance) {
            return boundaryPoint;
        }

        return {
            x: boundaryPoint.x + ((dx / distance) * inset),
            y: boundaryPoint.y + ((dy / distance) * inset)
        };
    }

    function getRectBoundaryPointOnBounds(bounds, externalPoint) {
        const center = getBoundsCenter(bounds);
        const dx = externalPoint.x - center.cx;
        const dy = externalPoint.y - center.cy;
        const distance = Math.sqrt((dx * dx) + (dy * dy));
        if (!distance) {
            return {
                x: center.cx,
                y: center.cy
            };
        }

        const halfWidth = bounds.width / 2;
        const halfHeight = bounds.height / 2;
        const scaleX = dx === 0 ? Infinity : halfWidth / Math.abs(dx);
        const scaleY = dy === 0 ? Infinity : halfHeight / Math.abs(dy);
        const scale = Math.min(scaleX, scaleY);
        return {
            x: center.cx + (dx * scale),
            y: center.cy + (dy * scale)
        };
    }

    function getEllipseBoundaryPointOnBounds(bounds, externalPoint) {
        const center = getBoundsCenter(bounds);
        const dx = externalPoint.x - center.cx;
        const dy = externalPoint.y - center.cy;
        if (Math.abs(dx) < EPSILON && Math.abs(dy) < EPSILON) {
            return {
                x: center.cx,
                y: center.cy
            };
        }

        const radiusX = bounds.width / 2;
        const radiusY = bounds.height / 2;
        const scale = 1 / Math.sqrt(
            ((dx * dx) / (radiusX * radiusX))
            + ((dy * dy) / (radiusY * radiusY))
        );
        return {
            x: center.cx + (dx * scale),
            y: center.cy + (dy * scale)
        };
    }

    function getDiamondBoundaryPointOnBounds(bounds, externalPoint) {
        const center = getBoundsCenter(bounds);
        const dx = externalPoint.x - center.cx;
        const dy = externalPoint.y - center.cy;
        if (Math.abs(dx) < EPSILON && Math.abs(dy) < EPSILON) {
            return {
                x: center.cx,
                y: center.cy
            };
        }

        const halfWidth = bounds.width / 2;
        const halfHeight = bounds.height / 2;
        const scale = 1 / (
            (Math.abs(dx) / halfWidth)
            + (Math.abs(dy) / halfHeight)
        );
        return {
            x: center.cx + (dx * scale),
            y: center.cy + (dy * scale)
        };
    }

    function cross(ax, ay, bx, by) {
        return (ax * by) - (ay * bx);
    }

    function getPolygonPointsForShape(bounds, shape) {
        const left = bounds.left;
        const top = bounds.top;
        const width = bounds.width;
        const height = bounds.height;
        const normalizedShape = normalizeItemShape(shape);
        let normalizedPoints = null;

        if (normalizedShape === 'triangle') {
            normalizedPoints = [
                { x: 0.5, y: 0 },
                { x: 0, y: 1 },
                { x: 1, y: 1 }
            ];
        } else if (normalizedShape === 'upsideDownTriangle') {
            normalizedPoints = [
                { x: 0, y: 0 },
                { x: 1, y: 0 },
                { x: 0.5, y: 1 }
            ];
        } else if (normalizedShape === 'hexagon') {
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
            x: left + (point.x * width),
            y: top + (point.y * height)
        }));
    }

    function getPolygonBoundaryPointOnBounds(bounds, shape, externalPoint) {
        const points = getPolygonPointsForShape(bounds, shape);
        if (!points || points.length < 3) {
            return null;
        }

        const center = getBoundsCenter(bounds);
        const directionX = externalPoint.x - center.cx;
        const directionY = externalPoint.y - center.cy;
        if (Math.abs(directionX) < EPSILON && Math.abs(directionY) < EPSILON) {
            return {
                x: center.cx,
                y: center.cy
            };
        }

        let bestIntersection = null;
        for (let index = 0; index < points.length; index += 1) {
            const start = points[index];
            const end = points[(index + 1) % points.length];
            const edgeX = end.x - start.x;
            const edgeY = end.y - start.y;
            const originToStartX = start.x - center.cx;
            const originToStartY = start.y - center.cy;
            const denominator = cross(directionX, directionY, edgeX, edgeY);

            if (Math.abs(denominator) < EPSILON) {
                continue;
            }

            const rayScale = cross(originToStartX, originToStartY, edgeX, edgeY) / denominator;
            const segmentScale = cross(originToStartX, originToStartY, directionX, directionY) / denominator;
            if (rayScale < -EPSILON || segmentScale < -EPSILON || segmentScale > 1 + EPSILON) {
                continue;
            }

            if (!bestIntersection || rayScale < bestIntersection.rayScale) {
                bestIntersection = {
                    rayScale: rayScale,
                    x: center.cx + (directionX * rayScale),
                    y: center.cy + (directionY * rayScale)
                };
            }
        }

        return bestIntersection
            ? { x: bestIntersection.x, y: bestIntersection.y }
            : null;
    }

    function getShapeBoundaryPointOnBounds(bounds, shape, externalPoint) {
        const normalizedShape = normalizeItemShape(shape);
        if (normalizedShape === 'circle' || normalizedShape === 'oval') {
            return getEllipseBoundaryPointOnBounds(bounds, externalPoint);
        }

        if (normalizedShape === 'diamond') {
            return getDiamondBoundaryPointOnBounds(bounds, externalPoint);
        }

        if (normalizedShape === 'triangle'
            || normalizedShape === 'upsideDownTriangle'
            || normalizedShape === 'hexagon') {
            return getPolygonBoundaryPointOnBounds(bounds, normalizedShape, externalPoint)
                || getRectBoundaryPointOnBounds(bounds, externalPoint);
        }

        return getRectBoundaryPointOnBounds(bounds, externalPoint);
    }

    function getShapeAnchorPointOnBounds(bounds, shape, externalPoint, inset) {
        const center = getBoundsCenter(bounds);
        const boundaryPoint = getShapeBoundaryPointOnBounds(bounds, shape, externalPoint);
        return applyShapeAnchorInset(boundaryPoint, center, externalPoint, inset);
    }

    function getRectBoundaryAnchorFromOffsetCenters(fromCenter, toCenter, targetElement, inset) {
        const targetBounds = getElementOffsetRect(targetElement);
        return getShapeAnchorPointOnBounds(targetBounds, 'default', {
            x: fromCenter.cx,
            y: fromCenter.cy
        }, inset);
    }

    function getShapeBoundaryAnchorFromOffsetCenters(fromCenter, toCenter, targetElement, targetShape, inset) {
        const targetBounds = getElementOffsetRect(targetElement);
        return getShapeAnchorPointOnBounds(targetBounds, targetShape, {
            x: fromCenter.cx,
            y: fromCenter.cy
        }, inset);
    }

    function getShapeAwareEdgeLinePointsFromElements(fromElement, toElement, fromShape, toShape, edgeKind, inset) {
        const start = getElementOffsetCenter(fromElement);
        const targetCenter = getElementOffsetCenter(toElement);
        const normalizedKind = normalizeEdgeKind(edgeKind);
        const end = normalizedKind === EDGE_KIND_ARROW
            ? getShapeBoundaryAnchorFromOffsetCenters(
                start,
                targetCenter,
                toElement,
                toShape,
                inset
            )
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

    function getEdgeLinePointsFromElements(fromElement, toElement, edgeKind, inset) {
        return getShapeAwareEdgeLinePointsFromElements(
            fromElement,
            toElement,
            fromElement && fromElement.dataset ? fromElement.dataset.shape : undefined,
            toElement && toElement.dataset ? toElement.dataset.shape : undefined,
            edgeKind,
            inset
        );
    }

    function getEdgePreviewPointFromClient(clientX, clientY, containerElement) {
        return getContainerRelativePointFromClient(clientX, clientY, containerElement);
    }

    window.PostbabyEdgeGeometry = Object.freeze({
        EDGE_KIND_LINE,
        EDGE_KIND_ARROW,
        normalizeEdgeKind,
        getShapeAnchorPointOnBounds,
        getRectBoundaryAnchorFromOffsetCenters,
        getShapeBoundaryAnchorFromOffsetCenters,
        getShapeAwareEdgeLinePointsFromElements,
        getEdgeLinePointsFromElements,
        getEdgePreviewPointFromClient
    });
})();
