import { type Edge, type Node } from "@xyflow/svelte";
import type { ProcessorConfig, Stage, StageResult } from "./types";

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
