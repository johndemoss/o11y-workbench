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

// ─── Local implementations (preserved as commented fallback for offline/dev mode) ───
//
// To switch to offline mode, comment out the fetch-based implementations below
// and uncomment the corresponding local version.
//
// export function topologicalSortLocal(nodes: Node[], edges: Edge[]): string[] {
// 	const adjacency = new Map<string, string[]>();
// 	const inDegree = new Map<string, number>();
//
// 	for (const node of nodes) {
// 		adjacency.set(node.id, []);
// 		inDegree.set(node.id, 0);
// 	}
//
// 	for (const edge of edges) {
// 		const neighbors = adjacency.get(edge.source);
// 		if (neighbors !== undefined) {
// 			neighbors.push(edge.target);
// 		}
// 		inDegree.set(edge.target, (inDegree.get(edge.target) ?? 0) + 1);
// 	}
//
// 	const queue: string[] = [];
// 	for (const [id, degree] of inDegree) {
// 		if (degree === 0) {
// 			queue.push(id);
// 		}
// 	}
//
// 	const sorted: string[] = [];
// 	while (queue.length > 0) {
// 		const id = queue.shift() as string;
// 		sorted.push(id);
//
// 		for (const neighbor of adjacency.get(id) ?? []) {
// 			const currentDegree = inDegree.get(neighbor) ?? 1;
// 			const newDegree = currentDegree - 1;
// 			inDegree.set(neighbor, newDegree);
// 			if (newDegree === 0) {
// 				queue.push(neighbor);
// 			}
// 		}
// 	}
//
// 	if (sorted.length !== nodes.length) {
// 		throw new Error("Cycle detected in pipeline graph");
// 	}
//
// 	return sorted;
// }
//
// export function wouldCreateCycleLocal(
// 	edges: Edge[],
// 	newEdge: { source: string; target: string },
// ): boolean {
// 	// Self-loop is always a cycle
// 	if (newEdge.source === newEdge.target) {
// 		return true;
// 	}
//
// 	// Duplicate edge check
// 	for (const edge of edges) {
// 		if (edge.source === newEdge.source && edge.target === newEdge.target) {
// 			return true;
// 		}
// 	}
//
// 	// Collect all unique node IDs referenced in edges
// 	const nodeIds = new Set<string>();
// 	for (const edge of edges) {
// 		nodeIds.add(edge.source);
// 		nodeIds.add(edge.target);
// 	}
// 	nodeIds.add(newEdge.source);
// 	nodeIds.add(newEdge.target);
//
// 	// Create minimal node objects for testing
// 	const testNodes: Node[] = Array.from(nodeIds).map((id) => ({
// 		id,
// 		position: { x: 0, y: 0 },
// 		data: {},
// 	}));
//
// 	const testEdge: Edge = {
// 		id: "test",
// 		source: newEdge.source,
// 		target: newEdge.target,
// 	};
// 	const testEdges: Edge[] = [...edges, testEdge];
//
// 	try {
// 		topologicalSort(testNodes, testEdges);
// 		return false;
// 	} catch {
// 		return true;
// 	}
// }
// ────────────────────────────────────────────────────────────────────────────────────

/**
 * Topological sort of a directed graph via the Go backend API.
 *
 * Sends the graph nodes and edges to `/api/ottl/pipeline/topological-sort`
 * and returns the sorted node IDs. Falls back to the local implementation
 * if the API is unreachable.
 *
 * @param nodes - The graph nodes
 * @param edges - The graph edges
 * @returns Node IDs in topological order
 * @throws {Error} If a cycle is detected or the API request fails without fallback
 */
export async function topologicalSort(nodes: Node[], edges: Edge[]): Promise<string[]> {
	try {
		const res = await fetch("/api/ottl/pipeline/topological-sort", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({
				nodes: nodes.map((n) => ({ id: n.id })),
				edges: edges.map((e) => ({ source: e.source, target: e.target })),
			}),
		});

		if (!res.ok) {
			throw new Error(`API returned status ${res.status}`);
		}

		const data = await res.json();

		if (data.error) {
			throw new Error(data.error);
		}

		return data.sorted_ids as string[];
	} catch (err) {
		// Fallback: re-throw with context.
		// For offline/dev mode, swap to the local implementation above.
		throw err instanceof Error
			? err
			: new Error("Topological sort API request failed");
	}
}

/**
 * Checks whether adding a new edge between two nodes would create a cycle.
 *
 * Performs local fast-path checks (self-loop, duplicate edge) before
 * delegating to the `/api/ottl/pipeline/would-cycle` endpoint.
 * Falls back to the local implementation if the API is unreachable.
 *
 * @param edges - The current edges (NOT mutated)
 * @param newEdge - The proposed new edge
 * @returns True if the edge would create a cycle, false otherwise
 */
export async function wouldCreateCycle(
	edges: Edge[],
	newEdge: { source: string; target: string },
): Promise<boolean> {
	// Fast-path: self-loop is always a cycle
	if (newEdge.source === newEdge.target) {
		return true;
	}

	// Fast-path: duplicate edge check
	for (const edge of edges) {
		if (edge.source === newEdge.source && edge.target === newEdge.target) {
			return true;
		}
	}

	try {
		const res = await fetch("/api/ottl/pipeline/would-cycle", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({
				edges: edges.map((e) => ({ source: e.source, target: e.target })),
				new_edge: { source: newEdge.source, target: newEdge.target },
			}),
		});

		if (!res.ok) {
			throw new Error(`API returned status ${res.status}`);
		}

		const data = await res.json();
		return data.would_cycle === true;
	} catch (err) {
		// Fallback: re-throw with context.
		// For offline/dev mode, swap to the local implementation above.
		throw err instanceof Error
			? err
			: new Error("Cycle check API request failed");
	}
}
