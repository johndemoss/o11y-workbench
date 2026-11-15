import { type Edge, type Node } from "@xyflow/svelte";
import type { ProcessorConfig, SignalType, Stage, StageResult } from "./types";

/**
 * Data type stored in each Svelte Flow node representing a pipeline stage.
 */
export interface StageNodeData extends Record<string, unknown> {
	stage: Stage;
	processor: ProcessorConfig;
	stageIndex: number;
	/** Signal lane this stage belongs to (traces/logs/metrics). */
	signal?: SignalType;
	results?: StageResult;
}

/**
 * Topological sort of a directed graph via the Go backend API.
 *
 * Sends the graph nodes and edges to `/api/ottl/pipeline/topological-sort`
 * and returns the sorted node IDs.
 *
 * @param nodes - The graph nodes
 * @param edges - The graph edges
 * @returns Node IDs in topological order
 * @throws {Error} If a cycle is detected or the API request fails
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
		throw err instanceof Error
			? err
			: new Error("Cycle check API request failed");
	}
}