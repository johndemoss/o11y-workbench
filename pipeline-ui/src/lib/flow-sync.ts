// Re-export from sub-modules for backward compatibility
export type { StageNodeData } from "./flow-graph";
export { topologicalSort, wouldCreateCycle } from "./flow-graph";
export { pipelineToFlow, flowToPipeline, pipelinesToFlow, flowToPipelines } from "./flow-serialization";

import { type Edge, type Node } from "@xyflow/svelte";
import type { SignalType, StageResult } from "./types";
import { signalEdgeProps } from "./types";
import type { StageNodeData } from "./flow-graph";

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

	const signal = ((parentId ?? "").replace("swimlane-", "") || "traces") as SignalType;
	for (let i = 1; i < allInLane.length; i++) {
		remainingEdges.push({
			id: `e-${allInLane[i - 1].id}-${allInLane[i].id}`,
			source: allInLane[i - 1].id,
			target: allInLane[i].id,
			type: "straight",
			...signalEdgeProps(signal),
		});
	}

	return { nodes: [...nodes], edges: remainingEdges };
}
