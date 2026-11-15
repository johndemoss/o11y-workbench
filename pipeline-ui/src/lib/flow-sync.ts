// Re-export from sub-modules for backward compatibility
export type { StageNodeData } from "./flow-graph";
export { topologicalSort, wouldCreateCycle } from "./flow-graph";
export {
	pipelineToFlow,
	flowToPipeline,
	pipelinesToFlow,
	flowToPipelines,
	preserveInfraConfigs,
} from "./flow-serialization";

import { type Edge, type Node } from "@xyflow/svelte";
import type { SignalType, StageResult } from "./types";
import { signalEdgeProps } from "./types";
import type { StageNodeData } from "./flow-graph";
import { FIRST_STAGE_X, STAGE_GAP_TOTAL, NODE_Y } from "./svg-utils";

/** Rebuild the complete linear path for one signal, including endpoints. */
export function rebuildLane(
	flowNodes: Node<StageNodeData>[], flowEdges: Edge[], parentId: string,
): { nodes: Node<StageNodeData>[]; edges: Edge[] } {
	const lane = flowNodes.filter((node) => node.parentId === parentId);
	const stages = lane.filter((node) => node.type === "stage").sort((a, b) => a.position.x - b.position.x);
	const ids = new Set(lane.map((node) => node.id));
	const edges = flowEdges.filter((edge) => !(ids.has(edge.source) && ids.has(edge.target)));
	const signal = parentId.replace("swimlane-", "") as SignalType;
	const connect = (source: string, target: string) => edges.push({
		id: `e-${source}-${target}`, source, target, type: "straight", ...signalEdgeProps(signal),
	});
	if (stages.length) {
		for (const node of lane.filter((node) => node.type === "infra")) {
			if (node.data.type === "receiver") connect(node.id, stages[0].id);
			if (node.data.type === "exporter") connect(stages[stages.length - 1].id, node.id);
		}
		for (let i = 1; i < stages.length; i++) connect(stages[i - 1].id, stages[i].id);
	}
	const index = new Map(stages.map((node, i) => [node.id, i]));
	const nodes = flowNodes.map((node) => {
		const i = index.get(node.id);
		return i === undefined ? node : { ...node, position: { x: FIRST_STAGE_X + i * STAGE_GAP_TOTAL, y: NODE_Y }, data: { ...node.data, stageIndex: i } };
	});
	return { nodes, edges };
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
		allInLane[i].position = { x: FIRST_STAGE_X + i * STAGE_GAP_TOTAL, y: NODE_Y };
	}

	return rebuildLane(nodes, edges, parentId ?? "");
}
