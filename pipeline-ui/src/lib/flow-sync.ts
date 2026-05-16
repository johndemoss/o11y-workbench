import { type Edge, MarkerType, type Node } from "@xyflow/svelte";
import type { Pipeline, ProcessorConfig, Stage, StageResult } from "./types";

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
 * Sequential stages are connected by smoothstep edges with arrow markers.
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
			type: "smoothstep",
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
