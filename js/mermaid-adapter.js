(function () {
    const graphCreation = window.PostbabyGraphCreation;
    if (!graphCreation) {
        throw new Error('PostbabyGraphCreation is missing.');
    }

    const {
        GRAPH_DIRECTION_LR,
        GRAPH_DIRECTION_TD,
        GRAPH_MAX_LABEL_CHARS
    } = graphCreation;

    const SUPPORTED_DIAGRAM_HEADERS = Object.freeze([
        'flowchart TD',
        'flowchart TB',
        'flowchart LR',
        'graph TD',
        'graph TB',
        'graph LR'
    ]);
    const IGNORED_STATEMENT_PREFIXES = Object.freeze([
        'classDef',
        'class',
        'style',
        'linkStyle',
        'click',
        'subgraph',
        'end'
    ]);
    const UNSUPPORTED_DIAGRAM_PREFIXES = Object.freeze([
        'sequenceDiagram',
        'classDiagram',
        'stateDiagram',
        'erDiagram',
        'mindmap',
        'journey',
        'gantt',
        'pie',
        'gitGraph'
    ]);

    function buildIssue(code, message, lineNumber, path) {
        const issue = {
            code: code,
            message: message
        };

        if (Number.isInteger(lineNumber) && lineNumber > 0) {
            issue.line = lineNumber;
        }

        if (path) {
            issue.path = path;
        }

        return issue;
    }

    function normalizeTrimmedString(value) {
        return typeof value === 'string' ? value.trim() : '';
    }

    function stripTrailingSemicolons(value) {
        return value.replace(/;+$/, '').trim();
    }

    function stripWrappingQuotes(value) {
        if (value.length < 2) {
            return value;
        }

        const first = value.charAt(0);
        const last = value.charAt(value.length - 1);
        if ((first === '"' || first === "'") && first === last) {
            return value.slice(1, -1).trim();
        }

        return value;
    }

    function normalizeLabelText(value) {
        return stripWrappingQuotes(normalizeTrimmedString(value));
    }

    function isFiniteNumber(value) {
        return Number.isFinite(value);
    }

    function getSupportedMermaidSubset() {
        return {
            headers: SUPPORTED_DIAGRAM_HEADERS.slice(),
            nodeForms: [
                'A',
                'A[Text]',
                'A["Text with spaces"]',
                'A[\'Text with spaces\']',
                'A(Text)',
                'A((Text))',
                'A{Text}',
                'A([Text])',
                'A[[Text]]',
                'A[(Text)]',
                'A>Text]'
            ],
            edgeForms: [
                'A --> B',
                'A --- B',
                'A -.-> B',
                'A ==> B',
                'A -- text --> B',
                'A -->|text| B'
            ],
            ignoredStatements: IGNORED_STATEMENT_PREFIXES.slice()
        };
    }

    function buildGraphOptions(direction, options) {
        const graphOptions = {
            direction: direction
        };
        const sourceOptions = options && typeof options === 'object' ? options : {};

        if (isFiniteNumber(sourceOptions.originX)) {
            graphOptions.originX = Math.round(sourceOptions.originX);
        }

        if (isFiniteNumber(sourceOptions.originY)) {
            graphOptions.originY = Math.round(sourceOptions.originY);
        }

        if (isFiniteNumber(sourceOptions.spacingX) && sourceOptions.spacingX > 0) {
            graphOptions.spacingX = Math.round(sourceOptions.spacingX);
        }

        if (isFiniteNumber(sourceOptions.spacingY) && sourceOptions.spacingY > 0) {
            graphOptions.spacingY = Math.round(sourceOptions.spacingY);
        }

        return graphOptions;
    }

    function normalizeMermaidDirection(rawDirection) {
        const normalizedDirection = normalizeTrimmedString(rawDirection).toUpperCase();
        if (normalizedDirection === 'LR') {
            return GRAPH_DIRECTION_LR;
        }

        if (normalizedDirection === 'TD' || normalizedDirection === 'TB') {
            return GRAPH_DIRECTION_TD;
        }

        return null;
    }

    function parseDiagramHeader(line, lineNumber) {
        const normalizedLine = stripTrailingSemicolons(line);
        const supportedMatch = normalizedLine.match(/^(flowchart|graph)\s+(TD|TB|LR)$/i);
        if (supportedMatch) {
            return {
                ok: true,
                direction: normalizeMermaidDirection(supportedMatch[2])
            };
        }

        const unsupportedMatch = normalizedLine.match(/^([A-Za-z][A-Za-z0-9]*)\b/);
        if (unsupportedMatch) {
            const diagramType = unsupportedMatch[1];
            const normalizedDiagramType = diagramType.toLowerCase();
            if (UNSUPPORTED_DIAGRAM_PREFIXES.some((candidate) => candidate.toLowerCase() === normalizedDiagramType)) {
                return {
                    ok: false,
                    error: buildIssue(
                        'unsupported_diagram_type',
                        `Unsupported Mermaid diagram type "${diagramType}". Only flowchart/graph TD, TB, and LR are supported.`,
                        lineNumber,
                        `line:${lineNumber}`
                    )
                };
            }
        }

        return {
            ok: false,
            error: buildIssue(
                'invalid_diagram_header',
                'Mermaid source must start with flowchart/graph TD, TB, or LR.',
                lineNumber,
                `line:${lineNumber}`
            )
        };
    }

    function buildNodeDefinition(id, label, shape, labelScore, shapeScore) {
        return {
            id: id,
            label: label,
            shape: shape,
            _labelScore: labelScore,
            _shapeScore: shapeScore
        };
    }

    function parseNodeReference(token, lineNumber, warnings) {
        const trimmedToken = normalizeTrimmedString(token);
        const path = `line:${lineNumber}`;

        if (!trimmedToken) {
            return {
                ok: false,
                error: buildIssue(
                    'invalid_node_reference',
                    'Node references must not be empty.',
                    lineNumber,
                    path
                )
            };
        }

        const patterns = [
            {
                regex: /^([A-Za-z0-9_][A-Za-z0-9_-]*)\(\(([\s\S]*)\)\)$/,
                shape: 'circle',
                labelScore: 1,
                shapeScore: 3
            },
            {
                regex: /^([A-Za-z0-9_][A-Za-z0-9_-]*)\(\[([\s\S]*)\]\)$/,
                shape: 'oval',
                labelScore: 1,
                shapeScore: 3
            },
            {
                regex: /^([A-Za-z0-9_][A-Za-z0-9_-]*)\[\(([\s\S]*)\)\]$/,
                shape: 'oval',
                labelScore: 1,
                shapeScore: 3
            },
            {
                regex: /^([A-Za-z0-9_][A-Za-z0-9_-]*)\[\[([\s\S]*)\]\]$/,
                shape: 'default',
                labelScore: 1,
                shapeScore: 2
            },
            {
                regex: /^([A-Za-z0-9_][A-Za-z0-9_-]*)\{([\s\S]*)\}$/,
                shape: 'diamond',
                labelScore: 1,
                shapeScore: 3
            },
            {
                regex: /^([A-Za-z0-9_][A-Za-z0-9_-]*)\(([\s\S]*)\)$/,
                shape: 'default',
                labelScore: 1,
                shapeScore: 2
            },
            {
                regex: /^([A-Za-z0-9_][A-Za-z0-9_-]*)\[([\s\S]*)\]$/,
                shape: 'default',
                labelScore: 1,
                shapeScore: 2
            }
        ];

        for (let index = 0; index < patterns.length; index += 1) {
            const pattern = patterns[index];
            const match = trimmedToken.match(pattern.regex);
            if (!match) {
                continue;
            }

            const label = normalizeLabelText(match[2]);
            if (label.length > GRAPH_MAX_LABEL_CHARS) {
                return {
                    ok: false,
                    error: buildIssue(
                        'node_label_too_long',
                        `Node label exceeds ${GRAPH_MAX_LABEL_CHARS} characters.`,
                        lineNumber,
                        path
                    )
                };
            }

            return {
                ok: true,
                node: buildNodeDefinition(
                    match[1],
                    label || match[1],
                    pattern.shape,
                    label ? pattern.labelScore : 0,
                    pattern.shapeScore
                )
            };
        }

        const unsupportedShapeMatch = trimmedToken.match(/^([A-Za-z0-9_][A-Za-z0-9_-]*)>([\s\S]*)\]$/);
        if (unsupportedShapeMatch) {
            const label = normalizeLabelText(unsupportedShapeMatch[2]);
            if (label.length > GRAPH_MAX_LABEL_CHARS) {
                return {
                    ok: false,
                    error: buildIssue(
                        'node_label_too_long',
                        `Node label exceeds ${GRAPH_MAX_LABEL_CHARS} characters.`,
                        lineNumber,
                        path
                    )
                };
            }

            warnings.push(buildIssue(
                'unsupported_node_shape',
                'Mermaid parallelogram nodes are not supported yet and were normalized to the default Postbaby shape.',
                lineNumber,
                path
            ));
            return {
                ok: true,
                node: buildNodeDefinition(
                    unsupportedShapeMatch[1],
                    label || unsupportedShapeMatch[1],
                    'default',
                    label ? 1 : 0,
                    1
                )
            };
        }

        const bareMatch = trimmedToken.match(/^([A-Za-z0-9_][A-Za-z0-9_-]*)$/);
        if (bareMatch) {
            return {
                ok: true,
                node: buildNodeDefinition(
                    bareMatch[1],
                    bareMatch[1],
                    'default',
                    0,
                    0
                )
            };
        }

        return {
            ok: false,
            error: buildIssue(
                'unsupported_node_syntax',
                `Unsupported Mermaid node syntax: ${trimmedToken}`,
                lineNumber,
                path
            )
        };
    }

    function mergeNodeDefinition(nodesById, nodeOrder, nextNode, warnings, lineNumber) {
        const existingNode = nodesById.get(nextNode.id);
        if (!existingNode) {
            nodesById.set(nextNode.id, nextNode);
            nodeOrder.push(nextNode.id);
            return;
        }

        if (nextNode._labelScore > existingNode._labelScore) {
            existingNode.label = nextNode.label;
            existingNode._labelScore = nextNode._labelScore;
        } else if (
            nextNode._labelScore > 0
            && existingNode._labelScore > 0
            && nextNode.label !== existingNode.label
        ) {
            warnings.push(buildIssue(
                'conflicting_node_label',
                `Node "${nextNode.id}" was defined with multiple labels. Keeping the first non-empty label "${existingNode.label}".`,
                lineNumber,
                `line:${lineNumber}`
            ));
        }

        if (nextNode._shapeScore > existingNode._shapeScore) {
            existingNode.shape = nextNode.shape;
            existingNode._shapeScore = nextNode._shapeScore;
        } else if (
            nextNode._shapeScore > 0
            && existingNode._shapeScore > 0
            && nextNode.shape !== existingNode.shape
        ) {
            warnings.push(buildIssue(
                'conflicting_node_shape',
                `Node "${nextNode.id}" was defined with multiple shapes. Keeping the earlier Mermaid-compatible shape mapping "${existingNode.shape}".`,
                lineNumber,
                `line:${lineNumber}`
            ));
        }
    }

    function normalizeEdgeLabel(value) {
        const label = normalizeLabelText(value);
        return label || undefined;
    }

    function parseEdgeStatement(line, lineNumber, warnings) {
        const edgePatterns = [
            {
                regex: /^(.*?)\s*(-->|-\.->|==>)\|([^|]*)\|\s*(.*?)$/,
                getKind: function () {
                    return 'arrow';
                },
                getLabel: function (match) {
                    return normalizeEdgeLabel(match[3]);
                },
                getRightToken: function (match) {
                    return match[4];
                }
            },
            {
                regex: /^(.*?)\s*--\s+(.+?)\s*-->\s*(.*?)$/,
                getKind: function () {
                    return 'arrow';
                },
                getLabel: function (match) {
                    return normalizeEdgeLabel(match[2]);
                },
                getRightToken: function (match) {
                    return match[3];
                }
            },
            {
                regex: /^(.*?)\s*(-\.->|==>|-->|---)\s*(.*?)$/,
                getKind: function (match) {
                    return match[2] === '---' ? 'line' : 'arrow';
                },
                getLabel: function () {
                    return undefined;
                }
            }
        ];

        for (let index = 0; index < edgePatterns.length; index += 1) {
            const pattern = edgePatterns[index];
            const match = line.match(pattern.regex);
            if (!match) {
                continue;
            }

            const leftResult = parseNodeReference(match[1], lineNumber, warnings);
            if (!leftResult.ok) {
                return leftResult;
            }

            const rightToken = typeof pattern.getRightToken === 'function' ? pattern.getRightToken(match) : match[3];
            const rightResult = parseNodeReference(rightToken, lineNumber, warnings);
            if (!rightResult.ok) {
                return rightResult;
            }

            const edge = {
                from: leftResult.node.id,
                to: rightResult.node.id,
                kind: pattern.getKind(match)
            };
            const label = pattern.getLabel(match);
            if (label) {
                edge.label = label;
            }

            return {
                ok: true,
                edge: edge,
                nodes: [leftResult.node, rightResult.node]
            };
        }

            if (/-->|---|-\.->|==>/.test(line) || /--\s+.+\s+-->/.test(line)) {
            return {
                ok: false,
                error: buildIssue(
                    'unsupported_edge_syntax',
                    `Unsupported Mermaid edge syntax: ${line}`,
                    lineNumber,
                    `line:${lineNumber}`
                )
            };
        }

        return {
            ok: false,
            unmatched: true
        };
    }

    function classifyIgnoredStatement(line) {
        const lowerLine = line.toLowerCase();
        if (lowerLine.startsWith('classdef ')) {
            return 'classDef';
        }
        if (lowerLine.startsWith('class ')) {
            return 'class';
        }
        if (lowerLine.startsWith('style ')) {
            return 'style';
        }
        if (lowerLine.startsWith('linkstyle ')) {
            return 'linkStyle';
        }
        if (lowerLine.startsWith('click ')) {
            return 'click';
        }
        if (lowerLine.startsWith('subgraph ')) {
            return 'subgraph';
        }
        if (lowerLine === 'end') {
            return 'end';
        }

        return '';
    }

    function parseMermaidToNormalizedGraph(source, options) {
        if (typeof source !== 'string') {
            return {
                ok: false,
                errors: [
                    buildIssue(
                        'invalid_source',
                        'Mermaid source must be a string.',
                        0,
                        'source'
                    )
                ],
                warnings: []
            };
        }

        const lines = source.replace(/\r\n?/g, '\n').split('\n');
        const warnings = [];
        const errors = [];
        const nodesById = new Map();
        const nodeOrder = [];
        const edges = [];
        let direction = GRAPH_DIRECTION_LR;
        let headerSeen = false;

        for (let lineIndex = 0; lineIndex < lines.length; lineIndex += 1) {
            const lineNumber = lineIndex + 1;
            const trimmedLine = normalizeTrimmedString(lines[lineIndex]);

            if (!trimmedLine || trimmedLine.startsWith('%%')) {
                continue;
            }

            if (!headerSeen) {
                const headerResult = parseDiagramHeader(trimmedLine, lineNumber);
                if (!headerResult.ok) {
                    return {
                        ok: false,
                        errors: [headerResult.error],
                        warnings: warnings
                    };
                }

                headerSeen = true;
                direction = headerResult.direction;
                continue;
            }

            const statement = stripTrailingSemicolons(trimmedLine);
            if (!statement) {
                continue;
            }

            const ignoredStatement = classifyIgnoredStatement(statement);
            if (ignoredStatement) {
                warnings.push(buildIssue(
                    'ignored_mermaid_statement',
                    `Ignoring unsupported Mermaid ${ignoredStatement} statement.`,
                    lineNumber,
                    `line:${lineNumber}`
                ));
                continue;
            }

            const edgeResult = parseEdgeStatement(statement, lineNumber, warnings);
            if (edgeResult.ok) {
                edgeResult.nodes.forEach((node) => {
                    mergeNodeDefinition(nodesById, nodeOrder, node, warnings, lineNumber);
                });
                edges.push(edgeResult.edge);
                continue;
            }

            if (!edgeResult.unmatched) {
                errors.push(edgeResult.error);
                continue;
            }

            const nodeResult = parseNodeReference(statement, lineNumber, warnings);
            if (nodeResult.ok) {
                mergeNodeDefinition(nodesById, nodeOrder, nodeResult.node, warnings, lineNumber);
                continue;
            }

            errors.push(nodeResult.error);
        }

        if (!headerSeen) {
            return {
                ok: false,
                errors: [
                    buildIssue(
                        'missing_diagram_header',
                        'Mermaid source must include a supported flowchart/graph header.',
                        0,
                        'source'
                    )
                ],
                warnings: warnings
            };
        }

        if (errors.length > 0) {
            return {
                ok: false,
                errors: errors,
                warnings: warnings
            };
        }

        return {
            ok: true,
            graph: {
                nodes: nodeOrder.map((nodeId) => {
                    const node = nodesById.get(nodeId);
                    return {
                        id: node.id,
                        label: node.label,
                        shape: node.shape
                    };
                }),
                edges: edges,
                options: buildGraphOptions(direction, options)
            },
            warnings: warnings
        };
    }

    function validateMermaidSource(source, options) {
        const parseResult = parseMermaidToNormalizedGraph(source, options);
        return {
            ok: parseResult.ok,
            errors: parseResult.errors || [],
            warnings: parseResult.warnings || []
        };
    }

    window.PostbabyMermaidAdapter = Object.freeze({
        parseMermaidToNormalizedGraph,
        validateMermaidSource,
        getSupportedMermaidSubset
    });
})();
