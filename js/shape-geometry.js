(function () {
    const ITEM_SHAPES = ['default', 'circle', 'square', 'triangle', 'diamond', 'upsideDownTriangle', 'hexagon', 'oval'];
    const DEFAULT_ITEM_SHAPE = ITEM_SHAPES[0];
    const ITEM_SHAPE_SET = new Set(ITEM_SHAPES);
    const FREEFORM_FOOTPRINT_ITEM_SHAPES = new Set([DEFAULT_ITEM_SHAPE, 'oval']);
    const FIXED_RATIO_ITEM_SHAPES = new Set(
        ITEM_SHAPES.filter(shape => !FREEFORM_FOOTPRINT_ITEM_SHAPES.has(shape))
    );
    const WIDTH_REFERENCE_RESIZE_ITEM_SHAPES = new Set([
        'square',
        'circle',
        'diamond',
        'triangle',
        'upsideDownTriangle',
        'hexagon'
    ]);
    const RESIZE_ENABLED_ITEM_SHAPES = new Set([
        DEFAULT_ITEM_SHAPE,
        'oval',
        ...WIDTH_REFERENCE_RESIZE_ITEM_SHAPES
    ]);
    const FIXED_RATIO_ITEM_HEIGHT_BY_WIDTH = {
        square: 1,
        circle: 1,
        diamond: 1,
        triangle: 150 / 170,
        upsideDownTriangle: 150 / 170,
        hexagon: 135 / 170
    };
    const DEFAULT_SHAPE_TEXT_INSETS = Object.freeze({
        top: 20,
        right: 20,
        bottom: 20,
        left: 20
    });
    const SHAPE_TEXT_INSETS = Object.freeze({
        default: DEFAULT_SHAPE_TEXT_INSETS,
        square: Object.freeze({
            top: 26,
            right: 26,
            bottom: 26,
            left: 26
        }),
        circle: Object.freeze({
            top: 30,
            right: 30,
            bottom: 30,
            left: 30
        }),
        diamond: Object.freeze({
            top: 34,
            right: 34,
            bottom: 34,
            left: 34
        }),
        triangle: Object.freeze({
            top: 30,
            right: 30,
            bottom: 24,
            left: 30
        }),
        upsideDownTriangle: Object.freeze({
            top: 22,
            right: 30,
            bottom: 34,
            left: 30
        }),
        hexagon: Object.freeze({
            top: 26,
            right: 34,
            bottom: 26,
            left: 34
        }),
        oval: Object.freeze({
            top: 24,
            right: 34,
            bottom: 24,
            left: 34
        })
    });
    const DEFAULT_SHAPE_TEXT_ALIGNMENT = Object.freeze({
        horizontalAlign: 'left',
        verticalAlign: 'top'
    });
    const SHAPE_TEXT_ALIGNMENT = Object.freeze({
        default: DEFAULT_SHAPE_TEXT_ALIGNMENT,
        square: DEFAULT_SHAPE_TEXT_ALIGNMENT,
        circle: Object.freeze({
            horizontalAlign: 'center',
            verticalAlign: 'center'
        }),
        diamond: Object.freeze({
            horizontalAlign: 'center',
            verticalAlign: 'center'
        }),
        triangle: Object.freeze({
            horizontalAlign: 'center',
            verticalAlign: 'center'
        }),
        upsideDownTriangle: DEFAULT_SHAPE_TEXT_ALIGNMENT,
        hexagon: Object.freeze({
            horizontalAlign: 'center',
            verticalAlign: 'top'
        }),
        oval: DEFAULT_SHAPE_TEXT_ALIGNMENT
    });
    const MIN_RESIZABLE_ITEM_WIDTH = 120;
    const MIN_RESIZABLE_ITEM_HEIGHT = 80;
    const MAX_RESIZABLE_ITEM_WIDTH = 600;
    const MAX_RESIZABLE_ITEM_HEIGHT = 600;

    function normalizeItemShape(value) {
        return ITEM_SHAPE_SET.has(value) ? value : DEFAULT_ITEM_SHAPE;
    }

    function normalizeItemDimension(value, min, max) {
        const numericValue = Number(value);
        if (!Number.isFinite(numericValue)) {
            return undefined;
        }

        return Math.round(Math.max(min, Math.min(max, numericValue)));
    }

    function getNormalizedItemShape(itemOrShape) {
        const rawShape = typeof itemOrShape === 'string'
            ? itemOrShape
            : itemOrShape && typeof itemOrShape === 'object'
                ? itemOrShape.shape
                : undefined;
        return normalizeItemShape(rawShape);
    }

    function itemHasFreeformFootprint(itemOrShape) {
        return FREEFORM_FOOTPRINT_ITEM_SHAPES.has(getNormalizedItemShape(itemOrShape));
    }

    function itemUsesWidthReferenceSize(itemOrShape) {
        return FIXED_RATIO_ITEM_SHAPES.has(getNormalizedItemShape(itemOrShape));
    }

    function itemHonorsStoredWidthAndHeight(itemOrShape) {
        return itemHasFreeformFootprint(itemOrShape);
    }

    function itemSupportsFreeformResize(itemOrShape) {
        return itemHasFreeformFootprint(itemOrShape);
    }

    function itemSupportsWidthReferenceResize(itemOrShape) {
        return WIDTH_REFERENCE_RESIZE_ITEM_SHAPES.has(getNormalizedItemShape(itemOrShape));
    }

    function getFixedRatioHeightFromWidth(itemOrShape, width) {
        const normalizedShape = getNormalizedItemShape(itemOrShape);
        const ratio = FIXED_RATIO_ITEM_HEIGHT_BY_WIDTH[normalizedShape];
        if (!Number.isFinite(width) || !Number.isFinite(ratio)) {
            return undefined;
        }

        return Math.round(width * ratio);
    }

    function getItemDerivedVisualBox(item) {
        if (!item || typeof item !== 'object') {
            return null;
        }

        const normalizedShape = getNormalizedItemShape(item);
        const explicitWidth = Number.isFinite(item.width) ? item.width : undefined;
        const explicitHeight = Number.isFinite(item.height) ? item.height : undefined;

        if (itemHonorsStoredWidthAndHeight(normalizedShape)) {
            if (explicitWidth === undefined || explicitHeight === undefined) {
                return null;
            }

            return {
                width: explicitWidth,
                height: explicitHeight,
                sizeMode: 'freeform'
            };
        }

        if (itemUsesWidthReferenceSize(normalizedShape) && explicitWidth !== undefined) {
            const derivedHeight = getFixedRatioHeightFromWidth(normalizedShape, explicitWidth);
            if (!Number.isFinite(derivedHeight)) {
                return null;
            }

            return {
                width: explicitWidth,
                height: derivedHeight,
                sizeMode: 'width-reference'
            };
        }

        return null;
    }

    function getShapeTextInsets(itemOrShape) {
        const normalizedShape = getNormalizedItemShape(itemOrShape);
        return SHAPE_TEXT_INSETS[normalizedShape] || DEFAULT_SHAPE_TEXT_INSETS;
    }

    function getShapeTextAlignment(itemOrShape) {
        const normalizedShape = getNormalizedItemShape(itemOrShape);
        return SHAPE_TEXT_ALIGNMENT[normalizedShape] || DEFAULT_SHAPE_TEXT_ALIGNMENT;
    }

    function getResponsiveInset(value, minimum) {
        return Math.max(minimum, Math.round(value));
    }

    function getShapeResponsiveTextInsets(itemOrShape, outerWidth, outerHeight) {
        const normalizedShape = getNormalizedItemShape(itemOrShape);
        const minimumInsets = getShapeTextInsets(normalizedShape);
        if (!Number.isFinite(outerWidth) || !Number.isFinite(outerHeight)) {
            return minimumInsets;
        }

        if (normalizedShape === 'circle') {
            const horizontalInset = getResponsiveInset(outerWidth * 0.214, minimumInsets.left);
            const verticalInset = getResponsiveInset(outerHeight * 0.214, minimumInsets.top);
            return {
                top: verticalInset,
                right: horizontalInset,
                bottom: verticalInset,
                left: horizontalInset
            };
        }

        if (normalizedShape === 'diamond') {
            const horizontalInset = getResponsiveInset(outerWidth * 0.243, minimumInsets.left);
            const verticalInset = getResponsiveInset(outerHeight * 0.243, minimumInsets.top);
            return {
                top: verticalInset,
                right: horizontalInset,
                bottom: verticalInset,
                left: horizontalInset
            };
        }

        if (normalizedShape === 'triangle') {
            const horizontalInset = getResponsiveInset(outerWidth * 0.182, minimumInsets.left);
            return {
                top: getResponsiveInset(outerHeight * 0.293, 44),
                right: horizontalInset,
                bottom: getResponsiveInset(outerHeight * 0.120, 18),
                left: horizontalInset
            };
        }

        if (normalizedShape === 'upsideDownTriangle') {
            const horizontalInset = getResponsiveInset(outerWidth * 0.182, minimumInsets.left);
            return {
                top: getResponsiveInset(outerHeight * 0.120, 18),
                right: horizontalInset,
                bottom: getResponsiveInset(outerHeight * 0.293, 44),
                left: horizontalInset
            };
        }

        if (normalizedShape === 'hexagon') {
            const horizontalInset = getResponsiveInset(outerWidth * 0.200, minimumInsets.left);
            const verticalInset = getResponsiveInset(outerHeight * 0.193, minimumInsets.top);
            return {
                top: verticalInset,
                right: horizontalInset,
                bottom: verticalInset,
                left: horizontalInset
            };
        }

        if (normalizedShape === 'oval') {
            const horizontalInset = getResponsiveInset(outerWidth * 0.121, minimumInsets.left);
            const verticalInset = getResponsiveInset(outerHeight * 0.126, minimumInsets.top);
            return {
                top: verticalInset,
                right: horizontalInset,
                bottom: verticalInset,
                left: horizontalInset
            };
        }

        return minimumInsets;
    }

    function getShapeTextBounds(itemOrShape, outerWidth, outerHeight) {
        const textInsets = getShapeResponsiveTextInsets(itemOrShape, outerWidth, outerHeight);
        const textAlignment = getShapeTextAlignment(itemOrShape);
        const textWidth = Number.isFinite(outerWidth)
            ? Math.max(0, Math.round(outerWidth - textInsets.left - textInsets.right))
            : undefined;
        const textHeight = Number.isFinite(outerHeight)
            ? Math.max(0, Math.round(outerHeight - textInsets.top - textInsets.bottom))
            : undefined;

        return {
            top: textInsets.top,
            right: textInsets.right,
            bottom: textInsets.bottom,
            left: textInsets.left,
            width: textWidth,
            height: textHeight,
            minContentHeight: textHeight === undefined ? 0 : textHeight,
            horizontalAlign: textAlignment.horizontalAlign,
            verticalAlign: textAlignment.verticalAlign
        };
    }

    function getWidthReferenceResizeWidth(nextWidth) {
        return normalizeItemDimension(
            nextWidth,
            MIN_RESIZABLE_ITEM_WIDTH,
            MAX_RESIZABLE_ITEM_WIDTH
        );
    }

    function getItemResizeFootprintFromPointer(itemOrShape, startWidth, startHeight, deltaX, deltaY) {
        if (itemSupportsFreeformResize(itemOrShape)) {
            return {
                width: normalizeItemDimension(
                    startWidth + deltaX,
                    MIN_RESIZABLE_ITEM_WIDTH,
                    MAX_RESIZABLE_ITEM_WIDTH
                ),
                height: normalizeItemDimension(
                    startHeight + deltaY,
                    MIN_RESIZABLE_ITEM_HEIGHT,
                    MAX_RESIZABLE_ITEM_HEIGHT
                )
            };
        }

        if (itemSupportsWidthReferenceResize(itemOrShape)) {
            const dominantDelta = Math.abs(deltaX) >= Math.abs(deltaY) ? deltaX : deltaY;
            const width = getWidthReferenceResizeWidth(startWidth + dominantDelta);
            const derivedHeight = getFixedRatioHeightFromWidth(itemOrShape, width);
            if (!Number.isFinite(derivedHeight)) {
                return null;
            }

            return {
                width: width,
                height: derivedHeight
            };
        }

        return null;
    }

    function normalizeItemForRuntime(item) {
        if (!item || typeof item !== 'object') {
            return item;
        }
        item.shape = normalizeItemShape(item.shape);
        item.width = normalizeItemDimension(
            item.width,
            MIN_RESIZABLE_ITEM_WIDTH,
            MAX_RESIZABLE_ITEM_WIDTH
        );
        item.height = normalizeItemDimension(
            item.height,
            MIN_RESIZABLE_ITEM_HEIGHT,
            MAX_RESIZABLE_ITEM_HEIGHT
        );
        return item;
    }

    function cycleItemShapeValue(currentShape, direction) {
        const normalizedShape = normalizeItemShape(currentShape);
        const currentIndex = ITEM_SHAPES.indexOf(normalizedShape);
        const step = direction < 0 ? -1 : 1;
        const nextIndex = (currentIndex + step + ITEM_SHAPES.length) % ITEM_SHAPES.length;
        return ITEM_SHAPES[nextIndex];
    }

    window.PostbabyShapeGeometry = Object.freeze({
        ITEM_SHAPES,
        DEFAULT_ITEM_SHAPE,
        ITEM_SHAPE_SET,
        FREEFORM_FOOTPRINT_ITEM_SHAPES,
        FIXED_RATIO_ITEM_SHAPES,
        WIDTH_REFERENCE_RESIZE_ITEM_SHAPES,
        RESIZE_ENABLED_ITEM_SHAPES,
        FIXED_RATIO_ITEM_HEIGHT_BY_WIDTH,
        DEFAULT_SHAPE_TEXT_INSETS,
        SHAPE_TEXT_INSETS,
        MIN_RESIZABLE_ITEM_WIDTH,
        MIN_RESIZABLE_ITEM_HEIGHT,
        MAX_RESIZABLE_ITEM_WIDTH,
        MAX_RESIZABLE_ITEM_HEIGHT,
        normalizeItemShape,
        normalizeItemDimension,
        getNormalizedItemShape,
        itemHasFreeformFootprint,
        itemUsesWidthReferenceSize,
        itemHonorsStoredWidthAndHeight,
        itemSupportsFreeformResize,
        itemSupportsWidthReferenceResize,
        getFixedRatioHeightFromWidth,
        getItemDerivedVisualBox,
        getShapeTextInsets,
        getShapeTextBounds,
        getWidthReferenceResizeWidth,
        getItemResizeFootprintFromPointer,
        normalizeItemForRuntime,
        cycleItemShapeValue
    });
})();
