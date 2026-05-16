import { type Edge, MarkerType, type Node } from "@xyflow/svelte";
import type { Pipeline, PipelinesBySignal, ProcessorConfig, SignalType, Stage, StageResult } from "./types";

/**
 * Data type stored in each Svelte Flow node representing a pipeline stage.
 */
export interface StageNodeData extends Record<string, unknown> {
	stage: Stage;
	processor: ProcessorConfig;
	stageIndex: number;
	results?: StageResult;
}

/**
 * Converts a Pipeline model into Svelte Flow nodes and edges.
 *
 * Each pipeline stage becomes one node positioned vertically with 150px spacing.
 * Sequential stages are connected by straight edges with arrow markers.
 *
 * @param pipeline - The pipeline model to convert
 * @returns An object containing the nodes and edges arrays
 */
export function pipelineToFlow(pipeline: Pipeline): {
	nodes: Node<StageNodeData>[];
	edges: Edge[];
} {
	const nodes: Node<StageNodeData>[] = pipeline.stages.map((stage, index) => ({
		id: stage.id,
		type: "stage",
		position: { x: 200, y: index * 150 },
		data: {
			stage,
			processor: pipeline.processors[stage.processor_ref] ?? {},
			stageIndex: index,
		},
	}));

	const edges: Edge[] = [];
	for (let i = 0; i < pipeline.stages.length - 1; i++) {
		const source = pipeline.stages[i].id;
		const target = pipeline.stages[i + 1].id;
		edges.push({
			id: `e-${source}-${target}`,
			source,
			target,
			type: "straight",
			markerEnd: { type: MarkerType.ArrowClosed },
		});
	}

	return { nodes, edges };
}

/**
 * Converts Svelte Flow nodes and edges back into a Pipeline model.
 *
 * Uses topological sort to determine the execution order of stages.
 * Processor configurations are preserved from the current pipeline,
 * with any new processors from the flow nodes merged in.
 *
 * @param nodes - The Svelte Flow nodes
 * @param edges - The Svelte Flow edges
 * @param currentPipeline - The current pipeline (used to preserve processor configs)
 * @returns The reconstructed pipeline model
 * @throws {Error} If the graph contains a cycle
 */
export function flowToPipeline(
	nodes: Node<StageNodeData>[],
	edges: Edge[],
	currentPipeline: Pipeline,
): Pipeline {
	const sortedIds = topologicalSort(nodes, edges);

	const stages: Stage[] = [];
	for (const id of sortedIds) {
		const node = nodes.find((n) => n.id === id);
		if (node) {
			stages.push({ ...node.data.stage });
		}
	}

	// Preserve existing processor configs and merge any new ones from nodes
	const processors: Record<string, ProcessorConfig> = {
		...currentPipeline.processors,
	};
	for (const node of nodes) {
		const ref = node.data.stage.processor_ref;
		if (!(ref in processors)) {
			processors[ref] = node.data.processor;
		}
	}

	return { stages, processors };
}

/**
 * Topological sort of a directed graph using Kahn's algorithm.
 *
 * Builds an adjacency list from edges, tracks in-degree for each node,
 * and processes nodes with in-degree 0 first. Disconnected subgraphs
 * are appended after the connected portion.
 *
 * @param nodes - The graph nodes
 * @param edges - The graph edges
 * @returns Node IDs in topological order
 * @throws {Error} If a cycle is detected in the graph
 */
export function topologicalSort(nodes: Node[], edges: Edge[]): string[] {
	const adjacency = new Map<string, string[]>();
	const inDegree = new Map<string, number>();

	for (const node of nodes) {
		adjacency.set(node.id, []);
		inDegree.set(node.id, 0);
	}

	for (const edge of edges) {
		const neighbors = adjacency.get(edge.source);
		if (neighbors !== undefined) {
			neighbors.push(edge.target);
		}
		inDegree.set(edge.target, (inDegree.get(edge.target) ?? 0) + 1);
	}

	const queue: string[] = [];
	for (const [id, degree] of inDegree) {
		if (degree === 0) {
			queue.push(id);
		}
	}

	const sorted: string[] = [];
	while (queue.length > 0) {
		const id = queue.shift() as string;
		sorted.push(id);

		for (const neighbor of adjacency.get(id) ?? []) {
			const currentDegree = inDegree.get(neighbor) ?? 1;
			const newDegree = currentDegree - 1;
			inDegree.set(neighbor, newDegree);
			if (newDegree === 0) {
				queue.push(neighbor);
			}
		}
	}

	if (sorted.length !== nodes.length) {
		throw new Error("Cycle detected in pipeline graph");
	}

	return sorted;
}

/**
 * Checks whether adding a new edge between two nodes would create a cycle.
 *
 * Performs validation in order:
 * 1. Self-loops are always cycles
 * 2. Duplicate edges are rejected
 * 3. A temporary edge is added and topological sort is run to detect cycles
 *
 * @param edges - The current edges (NOT mutated)
 * @param newEdge - The proposed new edge
 * @returns True if the edge would create a cycle, false otherwise
 */
export function wouldCreateCycle(
	edges: Edge[],
	newEdge: { source: string; target: string },
): boolean {
	// Self-loop is always a cycle
	if (newEdge.source === newEdge.target) {
		return true;
	}

	// Duplicate edge check
	for (const edge of edges) {
		if (edge.source === newEdge.source && edge.target === newEdge.target) {
			return true;
		}
	}

	// Collect all unique node IDs referenced in edges
	const nodeIds = new Set<string>();
	for (const edge of edges) {
		nodeIds.add(edge.source);
		nodeIds.add(edge.target);
	}
	nodeIds.add(newEdge.source);
	nodeIds.add(newEdge.target);

	// Create minimal node objects for testing
	const testNodes: Node[] = Array.from(nodeIds).map((id) => ({
		id,
		position: { x: 0, y: 0 },
		data: {},
	}));

	const testEdge: Edge = {
		id: "test",
		source: newEdge.source,
		target: newEdge.target,
	};
	const testEdges: Edge[] = [...edges, testEdge];

	try {
		topologicalSort(testNodes, testEdges);
		return false;
	} catch {
		return true;
	}
}

/**
 * Merges StageResults back into their corresponding Svelte Flow nodes.
 *
 * Each result is matched to a node by its `stage_id`. If no result is found
 * for a node, its `results` field is set to `undefined`.
 *
 * @param nodes - The Svelte Flow nodes to merge results into
 * @param results - The stage results to merge
 * @returns A new array of nodes with merged results (input is NOT mutated)
 */
export function mergeResults(
	nodes: Node<StageNodeData>[],
	results: StageResult[],
): Node<StageNodeData>[] {
	return nodes.map((node) => {
		const result = results.find((r) => r.stage_id === node.id);
		return {
			...node,
			data: {
				...node.data,
				results: result ?? undefined,
			},
		};
	});
}

// ─── Multi-swimlane functions ─────────────────────────────────

/**
 * Converts PipelinesBySignal into Svelte Flow nodes and edges with dagre layout.
 *
 * Creates:
 * - One swimlane parent node per signal (type: 'swimlane')
 * - One stage child node per pipeline stage (type: 'stage', parentId: signal swimlane)
 * - Edges between consecutive stages within each swimlane
 * - Uses dagre.js with rankdir: TB for parent swimlane vertical positioning
 */
export function pipelinesToFlow(
	pipelines: PipelinesBySignal,
): { nodes: Node<StageNodeData>[]; edges: Edge[] } {
	const nodes: Node<StageNodeData>[] = [];
	const edges: Edge[] = [];

	const signals: SignalType[] = ["traces", "logs", "metrics"];

	for (const signal of signals) {
		const pipeline = pipelines[signal];
		if (!pipeline) continue;

		const defaultReceivers = pipeline.receivers ?? ['otlp']
		const defaultExporters = pipeline.exporters ?? ['debug']

		const stageCount = pipeline.stages.length;
		const swimlaneWidth = Math.max(640, stageCount * 210 + 430);
		const swimlaneHeight = 150;

		const swimlaneId = `swimlane-${signal}`;
		nodes.push({
			id: swimlaneId,
			type: "swimlane",
			position: { x: 0, y: 0 },
			width: swimlaneWidth,
			height: swimlaneHeight,
			style: { width: `${swimlaneWidth}px` },
			draggable: false,
			selectable: false,
			data: { signal, label: signal.charAt(0).toUpperCase() + signal.slice(1), disabled: signal !== 'traces' },
		} as any);

		for (let i = 0; i < stageCount; i++) {
			const stage = pipeline.stages[i];
			const processor = pipeline.processors[stage.processor_ref] ?? {};

			nodes.push({
				id: stage.id,
				type: "stage",
				parentId: swimlaneId,
				extent: "parent" as const,
				position: { x: 230 + i * Math.max(210, (swimlaneWidth - 640) / Math.max(1, stageCount - 1)), y: 35 },
				width: 160,
				draggable: false,
				zIndex: 10,
				data: { stage, processor, stageIndex: i, disabled: signal !== 'traces' } as StageNodeData,
			} as any);

			if (i > 0) {
				const prevStage = pipeline.stages[i - 1];
				edges.push({
					id: `e-${prevStage.id}-${stage.id}`,
					source: prevStage.id,
					target: stage.id,
					type: "straight",
					markerEnd: { type: MarkerType.ArrowClosed },
				});
			}
		}

		// Infra receiver nodes (positioned at left edge, stacked vertically)
		for (const [i, recv] of defaultReceivers.entries()) {
			nodes.push({
				id: `infra-${signal}-receiver-${recv}`,
				type: 'infra',
				parentId: swimlaneId,
				extent: 'parent' as const,
				position: { x: 20, y: 35 + i * 90 },
				width: 160,
				height: 80,
				draggable: false,
				selectable: false,
				data: {
					label: recv.toUpperCase(),
					type: 'receiver' as const,
					disabled: signal !== 'traces',
				},
			} as any)
		}

		// Infra exporter nodes (positioned at right edge, stacked vertically)
		for (const [i, exp] of defaultExporters.entries()) {
			nodes.push({
				id: `infra-${signal}-exporter-${exp}`,
				type: 'infra',
				parentId: swimlaneId,
				extent: 'parent' as const,
				position: { x: swimlaneWidth - 180 - 20, y: 35 + i * 90 },
				width: 160,
				height: 80,
				draggable: false,
				selectable: false,
				data: {
					label: exp.toUpperCase(),
					type: 'exporter' as const,
					disabled: signal !== 'traces',
				},
			} as any)
		}

		// Add infra edges: receiver → first stage, last stage → exporter
		const stageNodesInSwimlane = nodes.filter(n => n.parentId === swimlaneId && n.type === 'stage')
		if (stageNodesInSwimlane.length > 0) {
			const firstStage = stageNodesInSwimlane[0]
			const lastStage = stageNodesInSwimlane[stageNodesInSwimlane.length - 1]

			for (const recv of defaultReceivers) {
				const recvId = `infra-${signal}-receiver-${recv}`
				edges.push({
					id: `e-${recvId}-${firstStage.id}`,
					source: recvId,
					target: firstStage.id,
					type: 'straight',
					markerEnd: { type: MarkerType.ArrowClosed },
				})
			}

			for (const exp of defaultExporters) {
				const expId = `infra-${signal}-exporter-${exp}`
				edges.push({
					id: `e-${lastStage.id}-${expId}`,
					source: lastStage.id,
					target: expId,
					type: 'straight',
					markerEnd: { type: MarkerType.ArrowClosed },
				})
			}
		}
	}

	// Position swimlanes vertically (no dagre needed — they have no edges between them)
	const swimlaneHeight = 150;
	const swimlaneGap = 20;
	let yOffset = 0;
	for (const node of nodes) {
		if (node.type === "swimlane") {
			node.position = { x: 20, y: yOffset };
			yOffset += swimlaneHeight + swimlaneGap;
		}
	}

	return { nodes, edges };
}

/**
 * Converts Svelte Flow swimlane layout back into PipelinesBySignal.
 *
 * Groups stage nodes by their parent swimlane, topological sorts within
 * each swimlane, and reconstructs the PipelinesBySignal structure while
 * preserving processor configurations from the current pipelines.
 */
export function flowToPipelines(
	flowNodes: Node<StageNodeData>[],
	flowEdges: Edge[],
	currentPipelines: PipelinesBySignal,
): PipelinesBySignal {
	const result: PipelinesBySignal = {
		traces: { stages: [], processors: { ...currentPipelines.traces?.processors }, receivers: [...(currentPipelines.traces?.receivers ?? ['otlp'])], exporters: [...(currentPipelines.traces?.exporters ?? ['debug'])] },
		logs: { stages: [], processors: { ...currentPipelines.logs?.processors }, receivers: [...(currentPipelines.logs?.receivers ?? ['otlp'])], exporters: [...(currentPipelines.logs?.exporters ?? ['debug'])] },
		metrics: { stages: [], processors: { ...currentPipelines.metrics?.processors }, receivers: [...(currentPipelines.metrics?.receivers ?? ['otlp'])], exporters: [...(currentPipelines.metrics?.exporters ?? ['debug'])] },
	};

	const stageNodes = flowNodes.filter((n) => n.type === "stage");

	const byParent = new Map<string, Node<StageNodeData>[]>();
	for (const node of stageNodes) {
		if (!node.parentId) continue;
		if (!byParent.has(node.parentId)) byParent.set(node.parentId, []);
		byParent.get(node.parentId)!.push(node);
	}

	for (const [parentId, swimlaneStageNodes] of byParent) {
		const signal = parentId.replace("swimlane-", "") as SignalType;
		if (!result[signal]) continue;

		const swimlaneIds = new Set(swimlaneStageNodes.map((n) => n.id));
		const swimlaneEdges = flowEdges.filter(
			(e) => swimlaneIds.has(e.source) && swimlaneIds.has(e.target),
		);

		const orderedIds = topologicalSort(swimlaneStageNodes as any, swimlaneEdges);

		const signalStages: Stage[] = [];
		const signalProcessors: Record<string, ProcessorConfig> = {
			...currentPipelines[signal]?.processors,
		};

		for (const id of orderedIds) {
			const n = swimlaneStageNodes.find((sn) => sn.id === id);
			if (n) {
				signalStages.push({ ...n.data.stage });
				if (n.data.processor) {
					signalProcessors[n.data.stage.processor_ref] = { ...n.data.processor };
				}
			}
		}

		result[signal] = { stages: signalStages, processors: signalProcessors, receivers: [...(currentPipelines[signal]?.receivers ?? ['otlp'])], exporters: [...(currentPipelines[signal]?.exporters ?? ['debug'])] };
	}

	return result;
}

/**
 * Reorders a stage within its swimlane based on horizontal drag position.
 *
 * The further right the node is dragged, the later in the execution order it goes.
 * Edges within the swimlane are rebuilt to reflect the new sequential order.
 *
 * @returns A new { nodes, edges } object (inputs are NOT mutated).
 */
export function reorderStage(
	flowNodes: Node<StageNodeData>[],
	flowEdges: Edge[],
	nodeId: string,
	newX: number,
): { nodes: Node<StageNodeData>[]; edges: Edge[] } {
	const nodes = flowNodes.map((n) => ({ ...n }));
	const edges = flowEdges.map((e) => ({ ...e }));

	const draggedNode = nodes.find((n) => n.id === nodeId);
	if (!draggedNode) return { nodes, edges };

	draggedNode.position = { ...draggedNode.position, x: newX };

	const parentId = draggedNode.parentId;
	const siblings = nodes
		.filter((n) => n.type === "stage" && n.parentId === parentId && n.id !== nodeId)
		.sort((a, b) => a.position.x - b.position.x);

	let newIndex = siblings.length;
	for (let i = 0; i < siblings.length; i++) {
		if (newX < siblings[i].position.x) {
			newIndex = i;
			break;
		}
	}

	const allInLane = [...siblings.slice(0, newIndex), draggedNode, ...siblings.slice(newIndex)];

	for (let i = 0; i < allInLane.length; i++) {
		allInLane[i].position = { x: 230 + i * 210, y: allInLane[i].position.y };
	}

	const swimlaneIds = new Set(allInLane.map((n) => n.id));
	const remainingEdges = edges.filter((e) => !(swimlaneIds.has(e.source) && swimlaneIds.has(e.target)));

	for (let i = 1; i < allInLane.length; i++) {
		remainingEdges.push({
			id: `e-${allInLane[i - 1].id}-${allInLane[i].id}`,
			source: allInLane[i - 1].id,
			target: allInLane[i].id,
			type: "straight",
			markerEnd: { type: MarkerType.ArrowClosed },
		});
	}

	return { nodes: [...nodes], edges: remainingEdges };
}
