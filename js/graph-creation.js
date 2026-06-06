(function () {
    const shapeGeometry = window.PostbabyShapeGeometry;
    if (!shapeGeometry) {
        throw new Error('PostbabyShapeGeometry is missing.');
    }
    const geometryDom = window.PostbabyGeometryDom;
    if (!geometryDom) {
        throw new Error('PostbabyGeometryDom is missing.');
    }

    const edgeGeometry = window.PostbabyEdgeGeometry;
    if (!edgeGeometry) {
        throw new Error('PostbabyEdgeGeometry is missing.');
    }

    const {
        DEFAULT_ITEM_SHAPE,
        normalizeItemShape,
        normalizeItemDimension,
        itemHasFreeformFootprint,
        itemUsesWidthReferenceSize,
        getFixedRatioHeightFromWidth,
        MIN_RESIZABLE_ITEM_WIDTH,
        MIN_RESIZABLE_ITEM_HEIGHT,
        MAX_RESIZABLE_ITEM_WIDTH,
        MAX_RESIZABLE_ITEM_HEIGHT
    } = shapeGeometry;
    const {
        formatItemPosition
    } = geometryDom;
    const {
        normalizeEdgeKind
    } = edgeGeometry;

    const GRAPH_DIRECTION_LR = 'LR';
    const GRAPH_DIRECTION_TD = 'TD';
    const GRAPH_MAX_NODES = 100;
    const GRAPH_MAX_EDGES = 300;
    const GRAPH_MAX_LABEL_CHARS = 240;
    const GRAPH_DEFAULT_SPACING_X = 260;
    const GRAPH_DEFAULT_SPACING_Y = 180;
    const GRAPH_DEFAULT_GAP_X = 40;
    const GRAPH_DEFAULT_GAP_Y = 40;
    const GRAPH_DEFAULT_COLOR = '#ffee88';

    function buildValidationError(code, message, path) {
        return {
            code: code,
            message: message,
            path: path
        };
    }

    function isPlainObject(value) {
        return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
    }

    function isFiniteNumber(value) {
        return Number.isFinite(value);
    }

    function normalizeTrimmedString(value) {
        return typeof value === 'string' ? value.trim() : '';
    }

    function normalizeGraphDirection(value) {
        return value === GRAPH_DIRECTION_TD ? GRAPH_DIRECTION_TD : GRAPH_DIRECTION_LR;
    }

    function getPositiveNumberOrFallback(value, fallbackValue) {
        return Number.isFinite(value) && value > 0
            ? Math.round(value)
            : fallbackValue;
    }

    function getGraphNodeDefaultDimensions(shape) {
        const normalizedShape = normalizeItemShape(shape);

        if (normalizedShape === 'default') {
            return {
                width: 220,
                height: 120
            };
        }

        if (normalizedShape === 'oval') {
            return {
                width: 260,
                height: 120
            };
        }

        if (normalizedShape === 'square'
            || normalizedShape === 'circle'
            || normalizedShape === 'diamond') {
            return {
                width: 180,
                height: getFixedRatioHeightFromWidth(normalizedShape, 180)
            };
        }

        if (normalizedShape === 'triangle'
            || normalizedShape === 'upsideDownTriangle'
            || normalizedShape === 'hexagon') {
            return {
                width: 170,
                height: getFixedRatioHeightFromWidth(normalizedShape, 170)
            };
        }

        return {
            width: 220,
            height: 120
        };
    }

    function normalizeGraphNodeDimensions(node, index, shape, errors) {
        const hasWidth = Object.prototype.hasOwnProperty.call(node, 'width');
        const hasHeight = Object.prototype.hasOwnProperty.call(node, 'height');
        const nodePath = `nodes[${index}]`;

        if (hasWidth && !isFiniteNumber(node.width)) {
            errors.push(buildValidationError(
                'invalid_node_width',
                'Node width must be a finite number when provided.',
                `${nodePath}.width`
            ));
        }

        if (hasHeight && !isFiniteNumber(node.height)) {
            errors.push(buildValidationError(
                'invalid_node_height',
                'Node height must be a finite number when provided.',
                `${nodePath}.height`
            ));
        }

        if (errors.length > 0) {
            return null;
        }

        if (!hasWidth && !hasHeight) {
            return getGraphNodeDefaultDimensions(shape);
        }

        if (itemHasFreeformFootprint(shape)) {
            if (hasWidth !== hasHeight) {
                errors.push(buildValidationError(
                    'incomplete_node_dimensions',
                    'Freeform node dimensions must provide both width and height together.',
                    nodePath
                ));
                return null;
            }

            return {
                width: normalizeItemDimension(
                    node.width,
                    MIN_RESIZABLE_ITEM_WIDTH,
                    MAX_RESIZABLE_ITEM_WIDTH
                ),
                height: normalizeItemDimension(
                    node.height,
                    MIN_RESIZABLE_ITEM_HEIGHT,
                    MAX_RESIZABLE_ITEM_HEIGHT
                )
            };
        }

        if (hasHeight && !hasWidth) {
            errors.push(buildValidationError(
                'incomplete_node_dimensions',
                'Fixed-ratio node dimensions must provide width when height is provided.',
                nodePath
            ));
            return null;
        }

        const normalizedWidth = normalizeItemDimension(
            node.width,
            MIN_RESIZABLE_ITEM_WIDTH,
            MAX_RESIZABLE_ITEM_WIDTH
        );

        if (hasWidth && !hasHeight && itemUsesWidthReferenceSize(shape)) {
            return {
                width: normalizedWidth,
                height: getFixedRatioHeightFromWidth(shape, normalizedWidth)
            };
        }

        if (hasWidth && hasHeight) {
            return {
                width: normalizedWidth,
                height: normalizeItemDimension(
                    node.height,
                    MIN_RESIZABLE_ITEM_HEIGHT,
                    MAX_RESIZABLE_ITEM_HEIGHT
                )
            };
        }

        return getGraphNodeDefaultDimensions(shape);
    }

    function normalizeGraphNodes(rawNodes, errors) {
        const normalizedNodes = [];
        const nodeIdSet = new Set();

        rawNodes.forEach((rawNode, index) => {
            const nodePath = `nodes[${index}]`;
            if (!isPlainObject(rawNode)) {
                errors.push(buildValidationError(
                    'invalid_node',
                    'Each graph node must be an object.',
                    nodePath
                ));
                return;
            }

            const nodeId = normalizeTrimmedString(rawNode.id);
            if (!nodeId) {
                errors.push(buildValidationError(
                    'missing_node_id',
                    'Each graph node must provide a non-empty string id.',
                    `${nodePath}.id`
                ));
                return;
            }

            if (nodeIdSet.has(nodeId)) {
                errors.push(buildValidationError(
                    'duplicate_node_id',
                    `Duplicate graph node id "${nodeId}".`,
                    `${nodePath}.id`
                ));
                return;
            }
            nodeIdSet.add(nodeId);

            if (Object.prototype.hasOwnProperty.call(rawNode, 'label')
                && typeof rawNode.label !== 'string') {
                errors.push(buildValidationError(
                    'invalid_node_label',
                    'Node label must be a string when provided.',
                    `${nodePath}.label`
                ));
                return;
            }

            const label = normalizeTrimmedString(rawNode.label);
            if (label.length > GRAPH_MAX_LABEL_CHARS) {
                errors.push(buildValidationError(
                    'node_label_too_long',
                    `Node label exceeds ${GRAPH_MAX_LABEL_CHARS} characters.`,
                    `${nodePath}.label`
                ));
                return;
            }

            const hasX = Object.prototype.hasOwnProperty.call(rawNode, 'x');
            const hasY = Object.prototype.hasOwnProperty.call(rawNode, 'y');
            if (hasX !== hasY) {
                errors.push(buildValidationError(
                    'incomplete_node_position',
                    'Explicit node positions must provide both x and y together.',
                    nodePath
                ));
                return;
            }

            if (hasX && !isFiniteNumber(rawNode.x)) {
                errors.push(buildValidationError(
                    'invalid_node_x',
                    'Node x must be a finite number when provided.',
                    `${nodePath}.x`
                ));
                return;
            }

            if (hasY && !isFiniteNumber(rawNode.y)) {
                errors.push(buildValidationError(
                    'invalid_node_y',
                    'Node y must be a finite number when provided.',
                    `${nodePath}.y`
                ));
                return;
            }

            const normalizedShape = normalizeItemShape(rawNode.shape);
            const dimensionErrors = [];
            const dimensions = normalizeGraphNodeDimensions(
                rawNode,
                index,
                normalizedShape,
                dimensionErrors
            );
            if (dimensionErrors.length > 0) {
                errors.push.apply(errors, dimensionErrors);
                return;
            }

            normalizedNodes.push({
                id: nodeId,
                label: label || nodeId,
                shape: normalizedShape,
                width: dimensions.width,
                height: dimensions.height,
                x: hasX ? Math.round(rawNode.x) : undefined,
                y: hasY ? Math.round(rawNode.y) : undefined,
                hasExplicitPosition: hasX && hasY
            });
        });

        return normalizedNodes;
    }

    function buildLogicalEdgeKey(fromNodeId, toNodeId) {
        return [fromNodeId, toNodeId].sort().join('\u0000');
    }

    function normalizeGraphEdges(rawEdges, nodeIdSet, errors) {
        const normalizedEdges = [];
        const logicalEdgeSet = new Set();

        rawEdges.forEach((rawEdge, index) => {
            const edgePath = `edges[${index}]`;
            if (!isPlainObject(rawEdge)) {
                errors.push(buildValidationError(
                    'invalid_edge',
                    'Each graph edge must be an object.',
                    edgePath
                ));
                return;
            }

            const fromNodeId = normalizeTrimmedString(rawEdge.from);
            const toNodeId = normalizeTrimmedString(rawEdge.to);
            if (!fromNodeId) {
                errors.push(buildValidationError(
                    'missing_edge_from',
                    'Each graph edge must provide a non-empty from node id.',
                    `${edgePath}.from`
                ));
                return;
            }

            if (!toNodeId) {
                errors.push(buildValidationError(
                    'missing_edge_to',
                    'Each graph edge must provide a non-empty to node id.',
                    `${edgePath}.to`
                ));
                return;
            }

            if (!nodeIdSet.has(fromNodeId)) {
                errors.push(buildValidationError(
                    'missing_edge_from_node',
                    `Graph edge source "${fromNodeId}" does not exist.`,
                    `${edgePath}.from`
                ));
                return;
            }

            if (!nodeIdSet.has(toNodeId)) {
                errors.push(buildValidationError(
                    'missing_edge_to_node',
                    `Graph edge target "${toNodeId}" does not exist.`,
                    `${edgePath}.to`
                ));
                return;
            }

            if (fromNodeId === toNodeId) {
                errors.push(buildValidationError(
                    'self_edge_not_allowed',
                    'Graph edges cannot connect a node to itself.',
                    edgePath
                ));
                return;
            }

            const logicalEdgeKey = buildLogicalEdgeKey(fromNodeId, toNodeId);
            if (logicalEdgeSet.has(logicalEdgeKey)) {
                errors.push(buildValidationError(
                    'duplicate_edge_connection',
                    `Duplicate graph connection between "${fromNodeId}" and "${toNodeId}".`,
                    edgePath
                ));
                return;
            }
            logicalEdgeSet.add(logicalEdgeKey);

            normalizedEdges.push({
                from: fromNodeId,
                to: toNodeId,
                kind: normalizeEdgeKind(rawEdge.kind)
            });
        });

        return normalizedEdges;
    }

    function normalizeGraphOptions(rawOptions) {
        const options = isPlainObject(rawOptions) ? rawOptions : {};
        return {
            originX: Number.isFinite(options.originX) ? Math.round(options.originX) : undefined,
            originY: Number.isFinite(options.originY) ? Math.round(options.originY) : undefined,
            direction: normalizeGraphDirection(options.direction),
            spacingX: getPositiveNumberOrFallback(options.spacingX, GRAPH_DEFAULT_SPACING_X),
            spacingY: getPositiveNumberOrFallback(options.spacingY, GRAPH_DEFAULT_SPACING_Y)
        };
    }

    function normalizeGraphDefinition(graph) {
        const errors = [];
        if (!isPlainObject(graph)) {
            return {
                ok: false,
                errors: [
                    buildValidationError(
                        'invalid_graph',
                        'Normalized graph input must be an object.',
                        'graph'
                    )
                ]
            };
        }

        const rawNodes = Array.isArray(graph.nodes) ? graph.nodes : null;
        const rawEdges = Array.isArray(graph.edges) ? graph.edges : null;

        if (!rawNodes) {
            errors.push(buildValidationError(
                'invalid_graph_nodes',
                'Normalized graph input must provide a nodes array.',
                'nodes'
            ));
        }

        if (!rawEdges) {
            errors.push(buildValidationError(
                'invalid_graph_edges',
                'Normalized graph input must provide an edges array.',
                'edges'
            ));
        }

        if (errors.length > 0) {
            return {
                ok: false,
                errors: errors
            };
        }

        if (rawNodes.length > GRAPH_MAX_NODES) {
            errors.push(buildValidationError(
                'graph_node_limit_exceeded',
                `Graph exceeds the node limit of ${GRAPH_MAX_NODES}.`,
                'nodes'
            ));
        }

        if (rawEdges.length > GRAPH_MAX_EDGES) {
            errors.push(buildValidationError(
                'graph_edge_limit_exceeded',
                `Graph exceeds the edge limit of ${GRAPH_MAX_EDGES}.`,
                'edges'
            ));
        }

        const normalizedNodes = normalizeGraphNodes(rawNodes, errors);
        const nodeIdSet = new Set(normalizedNodes.map(node => node.id));
        const normalizedEdges = normalizeGraphEdges(rawEdges, nodeIdSet, errors);
        const normalizedOptions = normalizeGraphOptions(graph.options);

        if (errors.length > 0) {
            return {
                ok: false,
                errors: errors
            };
        }

        return {
            ok: true,
            normalized: {
                nodes: normalizedNodes,
                edges: normalizedEdges,
                options: normalizedOptions
            }
        };
    }

    function validateNormalizedGraphInput(graph) {
        const normalized = normalizeGraphDefinition(graph);
        if (!normalized.ok) {
            return normalized;
        }

        return {
            ok: true,
            errors: []
        };
    }

    function buildLayerAssignments(nodes, edges) {
        const nodeOrder = new Map();
        const indegree = new Map();
        const outgoing = new Map();
        const layerByNodeId = new Map();
        const processedNodeIds = new Set();

        nodes.forEach((node, index) => {
            nodeOrder.set(node.id, index);
            indegree.set(node.id, 0);
            outgoing.set(node.id, []);
            layerByNodeId.set(node.id, 0);
        });

        edges.forEach((edge) => {
            outgoing.get(edge.from).push(edge.to);
            indegree.set(edge.to, (indegree.get(edge.to) || 0) + 1);
        });

        while (processedNodeIds.size < nodes.length) {
            const nextNode = nodes.find((node) =>
                !processedNodeIds.has(node.id) && indegree.get(node.id) === 0
            );
            if (!nextNode) {
                break;
            }

            processedNodeIds.add(nextNode.id);
            const currentLayer = layerByNodeId.get(nextNode.id) || 0;
            const nextTargets = outgoing.get(nextNode.id) || [];
            nextTargets
                .slice()
                .sort((leftNodeId, rightNodeId) => nodeOrder.get(leftNodeId) - nodeOrder.get(rightNodeId))
                .forEach((targetNodeId) => {
                    indegree.set(targetNodeId, (indegree.get(targetNodeId) || 0) - 1);
                    layerByNodeId.set(
                        targetNodeId,
                        Math.max(layerByNodeId.get(targetNodeId) || 0, currentLayer + 1)
                    );
                });
        }

        const maxResolvedLayer = processedNodeIds.size > 0
            ? Math.max.apply(null, Array.from(processedNodeIds).map((nodeId) => layerByNodeId.get(nodeId) || 0))
            : -1;
        let fallbackLayer = maxResolvedLayer + 1;
        nodes.forEach((node) => {
            if (!processedNodeIds.has(node.id)) {
                layerByNodeId.set(node.id, fallbackLayer);
                fallbackLayer += 1;
            }
        });

        return layerByNodeId;
    }

    function buildRelativeNodePositions(nodes, edges, options) {
        const autoLayoutNodes = nodes.filter((node) => !node.hasExplicitPosition);
        const maxNodeWidth = nodes.reduce((maxWidth, node) => Math.max(maxWidth, node.width || 0), 0);
        const maxNodeHeight = nodes.reduce((maxHeight, node) => Math.max(maxHeight, node.height || 0), 0);
        const effectiveSpacingX = Math.max(options.spacingX, maxNodeWidth + GRAPH_DEFAULT_GAP_X);
        const effectiveSpacingY = Math.max(options.spacingY, maxNodeHeight + GRAPH_DEFAULT_GAP_Y);
        const relativePositionsByNodeId = new Map();

        if (autoLayoutNodes.length > 0) {
            const layerByNodeId = buildLayerAssignments(nodes, edges);
            const nodesByLayer = new Map();

            autoLayoutNodes.forEach((node) => {
                const layer = layerByNodeId.get(node.id) || 0;
                if (!nodesByLayer.has(layer)) {
                    nodesByLayer.set(layer, []);
                }
                nodesByLayer.get(layer).push(node);
            });

            Array.from(nodesByLayer.keys())
                .sort((leftLayer, rightLayer) => leftLayer - rightLayer)
                .forEach((layer) => {
                    const layerNodes = nodesByLayer.get(layer) || [];
                    layerNodes.forEach((node, index) => {
                        if (options.direction === GRAPH_DIRECTION_TD) {
                            relativePositionsByNodeId.set(node.id, {
                                x: index * effectiveSpacingX,
                                y: layer * effectiveSpacingY
                            });
                            return;
                        }

                        relativePositionsByNodeId.set(node.id, {
                            x: layer * effectiveSpacingX,
                            y: index * effectiveSpacingY
                        });
                    });
                });
        }

        nodes.forEach((node) => {
            if (node.hasExplicitPosition) {
                relativePositionsByNodeId.set(node.id, {
                    x: node.x,
                    y: node.y
                });
            } else if (!relativePositionsByNodeId.has(node.id)) {
                relativePositionsByNodeId.set(node.id, { x: 0, y: 0 });
            }
        });

        return {
            effectiveSpacingX: effectiveSpacingX,
            effectiveSpacingY: effectiveSpacingY,
            relativePositionsByNodeId: relativePositionsByNodeId
        };
    }

    function getIdFactory(buildOptions) {
        if (buildOptions && typeof buildOptions.idFactory === 'function') {
            return buildOptions.idFactory;
        }

        if (typeof window.generateUUID === 'function') {
            return window.generateUUID;
        }

        throw new Error('Graph creation requires an idFactory or window.generateUUID.');
    }

    function getDefaultColor(buildOptions) {
        const rawColor = buildOptions && typeof buildOptions.defaultColor === 'string'
            ? buildOptions.defaultColor.trim()
            : '';
        return rawColor || GRAPH_DEFAULT_COLOR;
    }

    function buildPostbabyGraphFromNormalizedGraph(graph, buildOptions) {
        const normalized = normalizeGraphDefinition(graph);
        if (!normalized.ok) {
            return normalized;
        }

        const idFactory = getIdFactory(buildOptions || {});
        const defaultColor = getDefaultColor(buildOptions || {});
        const normalizedGraph = normalized.normalized;
        const options = Object.assign({}, normalizedGraph.options, {
            originX: Number.isFinite(buildOptions && buildOptions.originX)
                ? Math.round(buildOptions.originX)
                : normalizedGraph.options.originX || 0,
            originY: Number.isFinite(buildOptions && buildOptions.originY)
                ? Math.round(buildOptions.originY)
                : normalizedGraph.options.originY || 0
        });
        const layout = buildRelativeNodePositions(
            normalizedGraph.nodes,
            normalizedGraph.edges,
            options
        );
        const itemIdByNodeId = new Map();
        const items = normalizedGraph.nodes.map((node) => {
            const itemId = idFactory();
            itemIdByNodeId.set(node.id, itemId);
            const relativePosition = layout.relativePositionsByNodeId.get(node.id) || { x: 0, y: 0 };
            return {
                id: itemId,
                name: node.label,
                color: defaultColor,
                position: formatItemPosition(
                    options.originX + relativePosition.x,
                    options.originY + relativePosition.y
                ),
                shape: node.shape,
                width: node.width,
                height: node.height
            };
        });
        const edges = normalizedGraph.edges.map((edge) => ({
            id: idFactory(),
            fromItemId: itemIdByNodeId.get(edge.from),
            toItemId: itemIdByNodeId.get(edge.to),
            kind: edge.kind
        }));

        return {
            ok: true,
            items: items,
            edges: edges,
            createdNodeIds: normalizedGraph.nodes.map((node) => node.id),
            createdItemIds: items.map((item) => item.id),
            effectiveSpacingX: layout.effectiveSpacingX,
            effectiveSpacingY: layout.effectiveSpacingY
        };
    }

    window.PostbabyGraphCreation = Object.freeze({
        GRAPH_DIRECTION_LR,
        GRAPH_DIRECTION_TD,
        GRAPH_MAX_NODES,
        GRAPH_MAX_EDGES,
        GRAPH_MAX_LABEL_CHARS,
        GRAPH_DEFAULT_SPACING_X,
        GRAPH_DEFAULT_SPACING_Y,
        GRAPH_DEFAULT_GAP_X,
        GRAPH_DEFAULT_GAP_Y,
        normalizeGraphDirection,
        validateNormalizedGraphInput,
        buildPostbabyGraphFromNormalizedGraph
    });
})();
