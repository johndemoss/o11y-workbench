import { type Edge, type Node } from "@xyflow/svelte";
import type { Pipeline, PipelinesBySignal, ProcessorConfig, SignalType, Stage } from "./types";
import { signalEdgeProps } from "./types";
import type { StageNodeData } from "./flow-graph";
import { topologicalSort } from "./flow-graph";
import { NODE_HEIGHT, NODE_Y, SWIMLANE_HEIGHT } from "./svg-utils";

/** Built-in defaults for infrastructure names referenced without stored configs (A9). */
export function preserveInfraConfigs(pipeline: Pipeline): {
	receivers: string[];
	exporters: string[];
	receiver_configs: Record<string, Record<string, unknown>>;
	exporter_configs: Record<string, Record<string, unknown>>;
} {
	const receivers = [...(pipeline.receivers ?? ["otlp"])];
	const exporters = [...(pipeline.exporters ?? ["debug"])];
	const receiver_configs: Record<string, Record<string, unknown>> = {
		...(pipeline.receiver_configs ?? {}),
	};
	const exporter_configs: Record<string, Record<string, unknown>> = {
		...(pipeline.exporter_configs ?? {}),
	};
	if (receivers.includes("otlp") && !("otlp" in receiver_configs)) {
		receiver_configs.otlp = { protocols: { grpc: {} } };
	}
	if (exporters.includes("debug") && !("debug" in exporter_configs)) {
		exporter_configs.debug = {};
	}
	return { receivers, exporters, receiver_configs, exporter_configs };
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
			...signalEdgeProps("traces"),
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
export async function flowToPipeline(
	nodes: Node<StageNodeData>[],
	edges: Edge[],
	currentPipeline: Pipeline,
): Promise<Pipeline> {
	const sortedIds = await topologicalSort(nodes, edges);

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

	const infra = preserveInfraConfigs(currentPipeline);
	return { ...currentPipeline, ...infra, stages, processors };
}

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

	const receiverSharedCount = (name: string): number =>
		signals.filter((sig) => (pipelines[sig]?.receivers ?? ['otlp']).includes(name)).length;

	for (const signal of signals) {
		const pipeline = pipelines[signal];
		if (!pipeline) continue;

		const defaultReceivers = pipeline.receivers ?? ['otlp'];
		const defaultExporters = pipeline.exporters ?? ['debug'];

		const stageCount = pipeline.stages.length;
		const stageWidth = 164;
		const stageGap = 48;
		const infraWidth = 110;
		const leftPad = 20;
		const rightPad = 20;
		const nodeY = NODE_Y;
		const nodeHeight = NODE_HEIGHT;
		const firstStageX = leftPad + infraWidth + stageGap; // 178
		// Pack receiver → stages → exporter with fixed 48px edge slots (design plan).
		// Extra swimlane width (min 640) stays AFTER the exporter — never stretches edges.
		const exporterX =
			stageCount > 0
				? firstStageX + (stageCount - 1) * (stageWidth + stageGap) + stageWidth + stageGap
				: firstStageX;
		const contentWidth = exporterX + infraWidth + rightPad;
		const swimlaneWidth = Math.max(640, contentWidth);
		const swimlaneHeight = Math.max(SWIMLANE_HEIGHT, NODE_Y + Math.max(defaultReceivers.length, defaultExporters.length, 1) * (nodeHeight + 8) + 28);

		const swimlaneId = `swimlane-${signal}`;
		nodes.push({
			id: swimlaneId,
			type: "swimlane",
			position: { x: 0, y: 0 },
			width: swimlaneWidth,
			height: swimlaneHeight,
			style: { width: `${swimlaneWidth}px`, height: `${swimlaneHeight}px` },
			draggable: false,
			selectable: false,
			data: { signal, label: signal.charAt(0).toUpperCase() + signal.slice(1), disabled: false },
		} as any);

		for (let i = 0; i < stageCount; i++) {
			const stage = pipeline.stages[i];
			const processor = pipeline.processors[stage.processor_ref] ?? {};

			nodes.push({
				id: stage.id,
				type: "stage",
				parentId: swimlaneId,
				extent: "parent" as const,
				position: {
					x: firstStageX + i * (stageWidth + stageGap),
					y: nodeY,
				},
				width: stageWidth,
				height: nodeHeight,
				draggable: true,
				zIndex: 10,
				data: { stage, processor, stageIndex: i, signal, disabled: false } as StageNodeData,
			} as any);

			if (i > 0) {
				const prevStage = pipeline.stages[i - 1];
				edges.push({
					id: `e-${prevStage.id}-${stage.id}`,
					source: prevStage.id,
					target: stage.id,
					type: "straight",
					...signalEdgeProps(signal),
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
				position: { x: leftPad, y: nodeY + i * (nodeHeight + 8) },
				width: infraWidth,
				height: nodeHeight,
				draggable: false,
				selectable: true,
				data: {
					label: recv,
					type: 'receiver' as const,
					disabled: false,
					sharedCount: receiverSharedCount(recv),
				},
			} as any);
		}

		// Infra exporter nodes — packed after last stage (or after receiver when empty)
		for (const [i, exp] of defaultExporters.entries()) {
			nodes.push({
				id: `infra-${signal}-exporter-${exp}`,
				type: 'infra',
				parentId: swimlaneId,
				extent: 'parent' as const,
				position: { x: exporterX, y: nodeY + i * (nodeHeight + 8) },
				width: infraWidth,
				height: nodeHeight,
				draggable: false,
				selectable: false,
				data: {
					label: exp,
					type: 'exporter' as const,
					disabled: false,
				},
			} as any);
		}

		// Add infra edges: receiver → first stage, last stage → exporter
		const stageNodesInSwimlane = nodes.filter(n => n.parentId === swimlaneId && n.type === 'stage');
		if (stageNodesInSwimlane.length > 0) {
			const firstStage = stageNodesInSwimlane[0];
			const lastStage = stageNodesInSwimlane[stageNodesInSwimlane.length - 1];

			for (const recv of defaultReceivers) {
				const recvId = `infra-${signal}-receiver-${recv}`;
				edges.push({
					id: `e-${recvId}-${firstStage.id}`,
					source: recvId,
					target: firstStage.id,
					type: 'straight',
					...signalEdgeProps(signal),
				});
			}

			for (const exp of defaultExporters) {
				const expId = `infra-${signal}-exporter-${exp}`;
				edges.push({
					id: `e-${lastStage.id}-${expId}`,
					source: lastStage.id,
					target: expId,
					type: 'straight',
					...signalEdgeProps(signal),
				});
			}
		}
	}

	// Position swimlanes vertically (no dagre needed — they have no edges between them)
	// Each lane may have a different number of infrastructure cards.
	const swimlaneGap = 24;
	let yOffset = 0;
	for (const node of nodes) {
		if (node.type === "swimlane") {
			node.position = { x: 20, y: yOffset };
			yOffset += (node.height ?? SWIMLANE_HEIGHT) + swimlaneGap;
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
export async function flowToPipelines(
	flowNodes: Node<StageNodeData>[],
	flowEdges: Edge[],
	currentPipelines: PipelinesBySignal,
): Promise<PipelinesBySignal> {
	const result = {} as PipelinesBySignal;
	for (const signal of ["traces", "logs", "metrics"] as const) {
		const pipeline = currentPipelines[signal];
		const infra = preserveInfraConfigs(pipeline);
		result[signal] = {
			...pipeline,
			stages: [],
			processors: { ...pipeline.processors },
			...infra,
		};
	}

	const stageNodes = flowNodes.filter((n) => n.type === "stage");
	for (const signal of ["traces", "logs", "metrics"] as const) {
		const remainingRefs = new Set(stageNodes.filter((node) => node.parentId === `swimlane-${signal}`).map((node) => node.data.stage.processor_ref));
		for (const stage of currentPipelines[signal].stages) {
			if (!remainingRefs.has(stage.processor_ref)) delete result[signal].processors[stage.processor_ref];
		}
	}

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

		const orderedIds = await topologicalSort(swimlaneStageNodes as any, swimlaneEdges);

		const signalStages: Stage[] = [];
		const signalProcessors: Record<string, ProcessorConfig> = {
			...result[signal].processors,
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

		result[signal] = { ...result[signal], stages: signalStages, processors: signalProcessors };
	}

	return result;
}
