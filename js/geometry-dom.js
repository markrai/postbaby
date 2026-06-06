(function () {
    const shapeGeometry = window.PostbabyShapeGeometry || null;
    const canvasLimits = window.PostbabyCanvasLimits || null;

    // These helpers keep today's persisted top/left CSS-string format while
    // giving the app a single numeric read/write path for future bounded-canvas work.
    function parseCssPixelValue(value, fallback) {
        const fallbackValue = Number.isFinite(fallback) ? fallback : 0;

        if (typeof value === 'number' && Number.isFinite(value)) {
            return value;
        }

        if (typeof value !== 'string') {
            return fallbackValue;
        }

        const trimmedValue = value.trim();
        const match = /^(-?\d+(?:\.\d+)?)px$/i.exec(trimmedValue);
        if (!match) {
            return fallbackValue;
        }

        const numericValue = Number(match[1]);
        return Number.isFinite(numericValue) ? numericValue : fallbackValue;
    }

    function formatCssPixelValue(value, fallback) {
        const numericValue = Number.isFinite(value)
            ? value
            : Number.isFinite(fallback)
                ? fallback
                : 0;
        return `${numericValue}px`;
    }

    function parseItemPosition(position, fallbackX, fallbackY) {
        const source = position && typeof position === 'object' ? position : {};
        const resolvedFallbackX = Number.isFinite(fallbackX) ? fallbackX : 0;
        const resolvedFallbackY = Number.isFinite(fallbackY) ? fallbackY : 0;
        return {
            x: parseCssPixelValue(source.left, resolvedFallbackX),
            y: parseCssPixelValue(source.top, resolvedFallbackY)
        };
    }

    function formatItemPosition(x, y) {
        return {
            top: formatCssPixelValue(y, 0),
            left: formatCssPixelValue(x, 0)
        };
    }

    function getItemPositionXY(item, fallbackX, fallbackY) {
        const source = item && typeof item === 'object' ? item.position : null;
        return parseItemPosition(source, fallbackX, fallbackY);
    }

    function setItemPositionXY(item, x, y) {
        const nextCoordinates = canvasLimits && typeof canvasLimits.clampItemPositionXY === 'function'
            ? canvasLimits.clampItemPositionXY(x, y)
            : { x: x, y: y };
        const nextPosition = formatItemPosition(nextCoordinates.x, nextCoordinates.y);
        if (item && typeof item === 'object') {
            item.position = nextPosition;
        }
        return nextPosition;
    }

    function getItemRectFromData(item) {
        const position = getItemPositionXY(item);
        const derivedVisualBox = shapeGeometry && typeof shapeGeometry.getItemDerivedVisualBox === 'function'
            ? shapeGeometry.getItemDerivedVisualBox(item)
            : null;
        const width = derivedVisualBox && Number.isFinite(derivedVisualBox.width)
            ? derivedVisualBox.width
            : item && Number.isFinite(item.width)
                ? item.width
                : undefined;
        const height = derivedVisualBox && Number.isFinite(derivedVisualBox.height)
            ? derivedVisualBox.height
            : item && Number.isFinite(item.height)
                ? item.height
                : undefined;

        return {
            x: position.x,
            y: position.y,
            left: position.x,
            top: position.y,
            width: width,
            height: height,
            right: Number.isFinite(width) ? position.x + width : undefined,
            bottom: Number.isFinite(height) ? position.y + height : undefined,
            sizeMode: derivedVisualBox && derivedVisualBox.sizeMode ? derivedVisualBox.sizeMode : null
        };
    }

    function getItemCenterFromData(item) {
        const rect = getItemRectFromData(item);
        if (!Number.isFinite(rect.width) || !Number.isFinite(rect.height)) {
            return null;
        }

        const centerX = rect.left + (rect.width / 2);
        const centerY = rect.top + (rect.height / 2);
        return {
            x: centerX,
            y: centerY,
            cx: centerX,
            cy: centerY
        };
    }

    function mergeRectBounds(currentBounds, nextBounds) {
        if (!nextBounds) {
            return currentBounds;
        }

        if (!currentBounds) {
            return {
                left: nextBounds.left,
                top: nextBounds.top,
                right: nextBounds.right,
                bottom: nextBounds.bottom,
                width: nextBounds.right - nextBounds.left,
                height: nextBounds.bottom - nextBounds.top
            };
        }

        const left = Math.min(currentBounds.left, nextBounds.left);
        const top = Math.min(currentBounds.top, nextBounds.top);
        const right = Math.max(currentBounds.right, nextBounds.right);
        const bottom = Math.max(currentBounds.bottom, nextBounds.bottom);
        return {
            left: left,
            top: top,
            right: right,
            bottom: bottom,
            width: right - left,
            height: bottom - top
        };
    }

    function getItemsBoundsFromData(items) {
        if (!Array.isArray(items) || items.length === 0) {
            return null;
        }

        let bounds = null;
        items.forEach(function (item) {
            const rect = getItemRectFromData(item);
            if (
                !rect
                || !Number.isFinite(rect.left)
                || !Number.isFinite(rect.top)
                || !Number.isFinite(rect.right)
                || !Number.isFinite(rect.bottom)
            ) {
                return;
            }

            bounds = mergeRectBounds(bounds, {
                left: rect.left,
                top: rect.top,
                right: rect.right,
                bottom: rect.bottom
            });
        });

        return bounds;
    }

    function getElementPositionXY(element, fallbackX, fallbackY) {
        const resolvedFallbackX = Number.isFinite(fallbackX) ? fallbackX : 0;
        const resolvedFallbackY = Number.isFinite(fallbackY) ? fallbackY : 0;
        if (!element) {
            return {
                x: resolvedFallbackX,
                y: resolvedFallbackY
            };
        }

        return {
            x: parseCssPixelValue(element.style.left, resolvedFallbackX),
            y: parseCssPixelValue(element.style.top, resolvedFallbackY)
        };
    }

    function setElementPositionXY(element, x, y) {
        if (!element) {
            return;
        }

        element.style.left = formatCssPixelValue(x, 0);
        element.style.top = formatCssPixelValue(y, 0);
    }

    function getPointerClientPoint(event) {
        if (event.type.startsWith('touch')) {
            return {
                x: event.touches[0].clientX,
                y: event.touches[0].clientY
            };
        }

        return {
            x: event.clientX,
            y: event.clientY
        };
    }

    function getPointerPagePoint(event) {
        if (event.type.startsWith('touch')) {
            return {
                x: event.touches[0].pageX,
                y: event.touches[0].pageY
            };
        }

        return {
            x: event.pageX,
            y: event.pageY
        };
    }

    function getContainerRelativePointFromClient(clientX, clientY, containerElement) {
        const containerRect = containerElement.getBoundingClientRect();
        return {
            x: clientX - containerRect.left,
            y: clientY - containerRect.top
        };
    }

    function getContainerRelativePositionStringsFromPage(pageX, pageY, containerElement) {
        const containerRect = containerElement.getBoundingClientRect();
        const containerLeft = containerRect.left + window.scrollX;
        const containerTop = containerRect.top + window.scrollY;
        return formatItemPosition(pageX - containerLeft, pageY - containerTop);
    }

    function getElementClientRect(element) {
        return element.getBoundingClientRect();
    }

    function getItemsBoundsFromElements(elements) {
        if (!elements) {
            return null;
        }

        let bounds = null;
        Array.from(elements).forEach(function (element) {
            if (!element || typeof element.getBoundingClientRect !== 'function') {
                return;
            }

            const rect = getElementClientRect(element);
            if (
                !Number.isFinite(rect.left)
                || !Number.isFinite(rect.top)
                || !Number.isFinite(rect.right)
                || !Number.isFinite(rect.bottom)
            ) {
                return;
            }

            bounds = mergeRectBounds(bounds, rect);
        });

        return bounds;
    }

    function getElementCenterClientPoint(element) {
        const rect = getElementClientRect(element);
        return {
            x: rect.left + rect.width / 2,
            y: rect.top + rect.height / 2
        };
    }

    function getElementOffsetRect(element) {
        return {
            left: element.offsetLeft,
            top: element.offsetTop,
            width: element.offsetWidth,
            height: element.offsetHeight,
            right: element.offsetLeft + element.offsetWidth,
            bottom: element.offsetTop + element.offsetHeight
        };
    }

    function getElementOffsetCenter(element) {
        const rect = getElementOffsetRect(element);
        return {
            cx: rect.left + rect.width / 2,
            cy: rect.top + rect.height / 2
        };
    }

    function scrollClientRectIntoViewCentered(clientRect, options) {
        if (
            !clientRect
            || !Number.isFinite(clientRect.left)
            || !Number.isFinite(clientRect.top)
            || !Number.isFinite(clientRect.width)
            || !Number.isFinite(clientRect.height)
        ) {
            return null;
        }

        const scrollOptions = options && typeof options === 'object' ? options : {};
        const behavior = typeof scrollOptions.behavior === 'string' ? scrollOptions.behavior : 'auto';
        const targetLeft = Math.max(
            0,
            Math.round(window.scrollX + clientRect.left + (clientRect.width / 2) - (window.innerWidth / 2))
        );
        const targetTop = Math.max(
            0,
            Math.round(window.scrollY + clientRect.top + (clientRect.height / 2) - (window.innerHeight / 2))
        );

        window.scrollTo({
            left: targetLeft,
            top: targetTop,
            behavior: behavior
        });

        return {
            left: targetLeft,
            top: targetTop
        };
    }

    function scrollElementIntoViewCentered(element, options) {
        if (!element) {
            return null;
        }

        return scrollClientRectIntoViewCentered(getElementClientRect(element), options);
    }

    function buildClientRectFromPoints(startClientPoint, endClientPoint) {
        const left = Math.min(startClientPoint.x, endClientPoint.x);
        const top = Math.min(startClientPoint.y, endClientPoint.y);
        const right = Math.max(startClientPoint.x, endClientPoint.x);
        const bottom = Math.max(startClientPoint.y, endClientPoint.y);
        return {
            left: left,
            top: top,
            right: right,
            bottom: bottom,
            width: right - left,
            height: bottom - top
        };
    }

    function clientRectsIntersect(a, b) {
        return !(
            a.top > b.bottom
            || a.right < b.left
            || a.bottom < b.top
            || a.left > b.right
        );
    }

    window.PostbabyGeometryDom = Object.freeze({
        parseCssPixelValue,
        formatCssPixelValue,
        parseItemPosition,
        formatItemPosition,
        getItemPositionXY,
        setItemPositionXY,
        getItemRectFromData,
        getItemCenterFromData,
        getItemsBoundsFromData,
        getElementPositionXY,
        setElementPositionXY,
        getPointerClientPoint,
        getPointerPagePoint,
        getContainerRelativePointFromClient,
        getContainerRelativePositionStringsFromPage,
        getElementClientRect,
        getItemsBoundsFromElements,
        getElementCenterClientPoint,
        getElementOffsetRect,
        getElementOffsetCenter,
        scrollClientRectIntoViewCentered,
        scrollElementIntoViewCentered,
        buildClientRectFromPoints,
        clientRectsIntersect
    });
})();
