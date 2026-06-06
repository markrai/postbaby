(function () {
    const DEFAULT_CAMERA = Object.freeze({
        x: 0,
        y: 0,
        zoom: 1
    });
    const MIN_ZOOM = 0.25;
    const MAX_ZOOM = 3;
    const ZOOM_STEP = 1.1;

    function toFiniteNumber(value, fallbackValue) {
        return Number.isFinite(value) ? value : fallbackValue;
    }

    function clamp(value, minValue, maxValue) {
        return Math.min(Math.max(value, minValue), maxValue);
    }

    function roundCameraScalar(value) {
        return Math.round(value * 10000) / 10000;
    }

    function clampZoom(zoom) {
        const nextZoom = toFiniteNumber(zoom, DEFAULT_CAMERA.zoom);
        return clamp(nextZoom, MIN_ZOOM, MAX_ZOOM);
    }

    function normalizeCameraState(camera) {
        const source = camera && typeof camera === 'object' ? camera : {};
        return {
            x: roundCameraScalar(toFiniteNumber(source.x, DEFAULT_CAMERA.x)),
            y: roundCameraScalar(toFiniteNumber(source.y, DEFAULT_CAMERA.y)),
            zoom: roundCameraScalar(clampZoom(source.zoom))
        };
    }

    function getNumericWorldBounds(worldBounds) {
        if (worldBounds && typeof worldBounds === 'object') {
            const hasExplicitBounds = Number.isFinite(worldBounds.minX)
                && Number.isFinite(worldBounds.maxX)
                && Number.isFinite(worldBounds.minY)
                && Number.isFinite(worldBounds.maxY);
            if (hasExplicitBounds) {
                return {
                    minX: worldBounds.minX,
                    maxX: worldBounds.maxX,
                    minY: worldBounds.minY,
                    maxY: worldBounds.maxY
                };
            }
        }

        return null;
    }

    function clampCameraState(camera, worldBounds) {
        const normalizedCamera = normalizeCameraState(camera);
        const resolvedBounds = getNumericWorldBounds(worldBounds);
        if (!resolvedBounds) {
            return normalizedCamera;
        }

        return {
            x: roundCameraScalar(clamp(normalizedCamera.x, resolvedBounds.minX, resolvedBounds.maxX)),
            y: roundCameraScalar(clamp(normalizedCamera.y, resolvedBounds.minY, resolvedBounds.maxY)),
            zoom: normalizedCamera.zoom
        };
    }

    function viewportPointToWorldPoint(viewportX, viewportY, camera) {
        const normalizedCamera = normalizeCameraState(camera);
        return {
            x: normalizedCamera.x + (toFiniteNumber(viewportX, 0) / normalizedCamera.zoom),
            y: normalizedCamera.y + (toFiniteNumber(viewportY, 0) / normalizedCamera.zoom)
        };
    }

    function worldPointToViewportPoint(worldX, worldY, camera) {
        const normalizedCamera = normalizeCameraState(camera);
        return {
            x: (toFiniteNumber(worldX, 0) - normalizedCamera.x) * normalizedCamera.zoom,
            y: (toFiniteNumber(worldY, 0) - normalizedCamera.y) * normalizedCamera.zoom
        };
    }

    function getViewportRect(viewportElement) {
        if (!viewportElement || typeof viewportElement.getBoundingClientRect !== 'function') {
            return null;
        }
        return viewportElement.getBoundingClientRect();
    }

    function clientPointToWorldPoint(clientX, clientY, viewportElement, camera) {
        const viewportRect = getViewportRect(viewportElement);
        if (!viewportRect) {
            return viewportPointToWorldPoint(clientX, clientY, camera);
        }

        return viewportPointToWorldPoint(
            toFiniteNumber(clientX, viewportRect.left) - viewportRect.left,
            toFiniteNumber(clientY, viewportRect.top) - viewportRect.top,
            camera
        );
    }

    function worldPointToClientPoint(worldX, worldY, viewportElement, camera) {
        const viewportRect = getViewportRect(viewportElement);
        const viewportPoint = worldPointToViewportPoint(worldX, worldY, camera);
        return {
            x: viewportPoint.x + (viewportRect ? viewportRect.left : 0),
            y: viewportPoint.y + (viewportRect ? viewportRect.top : 0)
        };
    }

    function screenDeltaToWorldDelta(deltaX, deltaY, camera) {
        const normalizedCamera = normalizeCameraState(camera);
        return {
            x: toFiniteNumber(deltaX, 0) / normalizedCamera.zoom,
            y: toFiniteNumber(deltaY, 0) / normalizedCamera.zoom
        };
    }

    function getViewportWorldRect(camera, viewportElement) {
        const normalizedCamera = normalizeCameraState(camera);
        const width = viewportElement && Number.isFinite(viewportElement.clientWidth)
            ? viewportElement.clientWidth
            : window.innerWidth;
        const height = viewportElement && Number.isFinite(viewportElement.clientHeight)
            ? viewportElement.clientHeight
            : window.innerHeight;
        const worldWidth = width / normalizedCamera.zoom;
        const worldHeight = height / normalizedCamera.zoom;
        return {
            x: normalizedCamera.x,
            y: normalizedCamera.y,
            left: normalizedCamera.x,
            top: normalizedCamera.y,
            width: worldWidth,
            height: worldHeight,
            right: normalizedCamera.x + worldWidth,
            bottom: normalizedCamera.y + worldHeight
        };
    }

    function zoomCameraAtClientPoint(camera, clientX, clientY, nextZoom, viewportElement, worldBounds) {
        const normalizedCamera = normalizeCameraState(camera);
        const beforeWorldPoint = clientPointToWorldPoint(clientX, clientY, viewportElement, normalizedCamera);
        const clampedZoom = clampZoom(nextZoom);
        const viewportRect = getViewportRect(viewportElement);
        const viewportX = viewportRect ? clientX - viewportRect.left : clientX;
        const viewportY = viewportRect ? clientY - viewportRect.top : clientY;
        return clampCameraState({
            x: beforeWorldPoint.x - (viewportX / clampedZoom),
            y: beforeWorldPoint.y - (viewportY / clampedZoom),
            zoom: clampedZoom
        }, worldBounds);
    }

    function centerCameraOnWorldPoint(camera, viewportElement, worldX, worldY, worldBounds) {
        const normalizedCamera = normalizeCameraState(camera);
        const viewportRect = getViewportWorldRect(normalizedCamera, viewportElement);
        return clampCameraState({
            x: toFiniteNumber(worldX, 0) - (viewportRect.width / 2),
            y: toFiniteNumber(worldY, 0) - (viewportRect.height / 2),
            zoom: normalizedCamera.zoom
        }, worldBounds);
    }

    function fitCameraToWorldRect(bounds, viewportElement, options) {
        if (!bounds || !Number.isFinite(bounds.left) || !Number.isFinite(bounds.top)
            || !Number.isFinite(bounds.right) || !Number.isFinite(bounds.bottom)) {
            return normalizeCameraState(DEFAULT_CAMERA);
        }

        const resolvedOptions = options && typeof options === 'object' ? options : {};
        const padding = Number.isFinite(resolvedOptions.padding) ? resolvedOptions.padding : 80;
        const width = viewportElement && Number.isFinite(viewportElement.clientWidth)
            ? viewportElement.clientWidth
            : window.innerWidth;
        const height = viewportElement && Number.isFinite(viewportElement.clientHeight)
            ? viewportElement.clientHeight
            : window.innerHeight;
        const paddedWorldWidth = Math.max(bounds.right - bounds.left, 1);
        const paddedWorldHeight = Math.max(bounds.bottom - bounds.top, 1);
        const availableWidth = Math.max(width - (padding * 2), 1);
        const availableHeight = Math.max(height - (padding * 2), 1);
        const nextZoom = clampZoom(Math.min(
            availableWidth / paddedWorldWidth,
            availableHeight / paddedWorldHeight
        ));
        const worldCenterX = bounds.left + ((bounds.right - bounds.left) / 2);
        const worldCenterY = bounds.top + ((bounds.bottom - bounds.top) / 2);
        return centerCameraOnWorldPoint({
            x: bounds.left,
            y: bounds.top,
            zoom: nextZoom
        }, viewportElement, worldCenterX, worldCenterY, resolvedOptions.worldBounds);
    }

    window.PostbabyCanvasCamera = Object.freeze({
        DEFAULT_CAMERA,
        MIN_ZOOM,
        MAX_ZOOM,
        ZOOM_STEP,
        normalizeCameraState,
        clampCameraState,
        clampZoom,
        viewportPointToWorldPoint,
        worldPointToViewportPoint,
        clientPointToWorldPoint,
        worldPointToClientPoint,
        screenDeltaToWorldDelta,
        getViewportWorldRect,
        zoomCameraAtClientPoint,
        centerCameraOnWorldPoint,
        fitCameraToWorldRect
    });
})();
