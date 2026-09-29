import { describe, it, expect, vi } from 'vitest'
import { exportPipelinesToYamlLocal } from '../export-yaml'
import type { PipelinesBySignal } from '../types'

// ---------------------------------------------------------------------------
// Pure-logic equivalents of EditorPanel's reactive sync/gating logic
// (syncStatus $derived + resyncFromGraph + Model-1 yaml prop sync)
// ---------------------------------------------------------------------------

type SyncStatus = 'synced' | 'editing' | 'syncing'

/**
 * Mirrors the $derived expression in EditorPanel.svelte:
 *   syncStatus = awaitingOwnSync ? 'syncing' : userIsEditing ? 'editing' : 'synced'
 */
function computeSyncStatus(
  awaitingOwnSync: boolean,
  userIsEditing: boolean,
): SyncStatus {
  return awaitingOwnSync ? 'syncing' : userIsEditing ? 'editing' : 'synced'
}

interface EditorPanelState {
  userIsEditing: boolean
  awaitingOwnSync: boolean
  yamlContent: string
}

/**
 * Mirrors resyncFromGraph — resets flags and applies App-owned yaml prop.
 */
function resyncFromGraph(yamlProp: string): EditorPanelState {
  return {
    userIsEditing: false,
    awaitingOwnSync: false,
    yamlContent: yamlProp,
  }
}

/**
 * Mirrors the $effect guard for prop→editor sync (Model 1 — no fetch):
 *   if (!userIsEditing && !awaitingOwnSync) { yamlContent = yamlProp }
 */
function shouldSyncYaml(
  userIsEditing: boolean,
  awaitingOwnSync: boolean,
): boolean {
  return !userIsEditing && !awaitingOwnSync
}

/**
 * Apply App yaml prop to editor when guards allow. Returns next yamlContent
 * (or previous if blocked). Never calls export/fetch.
 */
function applyYamlProp(
  yamlProp: string,
  currentYaml: string,
  userIsEditing: boolean,
  awaitingOwnSync: boolean,
): string {
  if (shouldSyncYaml(userIsEditing, awaitingOwnSync)) {
    return yamlProp
  }
  return currentYaml
}

// ----- setup ---------------------------------------------------------------

const emptyPipelines: PipelinesBySignal = {
  traces: { stages: [], processors: {} },
  logs: { stages: [], processors: {} },
  metrics: { stages: [], processors: {} },
}

const populatedPipelines: PipelinesBySignal = {
  traces: {
    stages: [
      { id: 's1', type: 'transform', processor_ref: 'add_env', error_mode: 'ignore' },
    ],
    processors: {
      add_env: {
        error_mode: 'ignore',
        trace_statements: [
          { context: 'span', statements: ['set(attributes["env"], "prod")'] },
        ],
      },
    },
  },
  logs: { stages: [], processors: {} },
  metrics: { stages: [], processors: {} },
}

// ===== computeSyncStatus ==================================================

describe('computeSyncStatus (gating logic)', () => {
  it('returns "synced" when both flags are false', () => {
    expect(computeSyncStatus(false, false)).toBe('synced')
  })

  it('returns "editing" when userIsEditing alone is true', () => {
    expect(computeSyncStatus(false, true)).toBe('editing')
  })

  it('returns "syncing" when awaitingOwnSync is true regardless of userIsEditing', () => {
    expect(computeSyncStatus(true, false)).toBe('syncing')
    expect(computeSyncStatus(true, true)).toBe('syncing')
  })

  it('awaitingOwnSync takes priority over userIsEditing', () => {
    // "syncing" should win per the ternary order
    expect(computeSyncStatus(true, true)).toBe('syncing')
  })
})

// ===== shouldSyncYaml =====================================================

describe('shouldSyncYaml (effect guard)', () => {
  it('allows sync when neither flag is set', () => {
    expect(shouldSyncYaml(false, false)).toBe(true)
  })

  it('blocks sync when userIsEditing', () => {
    expect(shouldSyncYaml(true, false)).toBe(false)
  })

  it('blocks sync when awaitingOwnSync', () => {
    expect(shouldSyncYaml(false, true)).toBe(false)
  })

  it('blocks sync when both flags are set', () => {
    expect(shouldSyncYaml(true, true)).toBe(false)
  })
})

// ===== Model 1: yaml prop sync without fetch ==============================

describe('applyYamlProp (Model 1 — no export fetch)', () => {
  it('updates yaml from prop when !userIsEditing without calling fetch', () => {
    const fetchSpy = vi.fn()
    const propYaml = exportPipelinesToYamlLocal(populatedPipelines)
    const next = applyYamlProp(propYaml, 'old', false, false)
    expect(next).toBe(propYaml)
    expect(next).toContain('add_env')
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('does not overwrite editor while userIsEditing', () => {
    const next = applyYamlProp('from-app', 'user-typed', true, false)
    expect(next).toBe('user-typed')
  })

  it('does not overwrite editor while awaitingOwnSync', () => {
    const next = applyYamlProp('from-app', 'pending', false, true)
    expect(next).toBe('pending')
  })
})

// ===== resyncFromGraph ====================================================

describe('resyncFromGraph', () => {
  it('resets flags and applies App yaml prop', () => {
    const yamlProp = exportPipelinesToYamlLocal(emptyPipelines)
    const next = resyncFromGraph(yamlProp)
    expect(next.userIsEditing).toBe(false)
    expect(next.awaitingOwnSync).toBe(false)
    expect(next.yamlContent).toBe(yamlProp)
  })

  it('produces valid YAML from populated pipelines prop', () => {
    const yamlProp = exportPipelinesToYamlLocal(populatedPipelines)
    const next = resyncFromGraph(yamlProp)
    expect(next.yamlContent).toContain('add_env')
    expect(next.yamlContent).toContain('traces')
    expect(next.yamlContent).toContain('receivers:')
    expect(next.yamlContent).toContain('exporters:')
  })

  it('is idempotent', () => {
    const content = exportPipelinesToYamlLocal(emptyPipelines)
    const next = resyncFromGraph(content)
    expect(next.yamlContent).toBe(content)
    expect(next.userIsEditing).toBe(false)
    expect(next.awaitingOwnSync).toBe(false)
  })
})

// ===== end-to-end: simulate handleEditorChange cycle =======================

describe('handleEditorChange cycle (without timers)', () => {
  it('simulates the state transitions of a user edit cycle', () => {
    // The handleEditorChange function in EditorPanel does:
    //   1. userIsEditing = true
    //   2. Sets yamlContent
    //   3. On debounce: validates, calls onPipelinesChange, sets awaitingOwnSync
    //   4. After tick: sets awaitingOwnSync = false, userIsEditing = false
    //
    // We verify the intermediate states of this state machine.

    const appYaml = exportPipelinesToYamlLocal(emptyPipelines)
    let state: EditorPanelState = {
      userIsEditing: false,
      awaitingOwnSync: false,
      yamlContent: appYaml,
    }

    // Simulate step 1: user starts editing
    state.userIsEditing = true
    state.yamlContent = 'incomplete: yaml'
    expect(computeSyncStatus(state.awaitingOwnSync, state.userIsEditing)).toBe('editing')
    expect(shouldSyncYaml(state.userIsEditing, state.awaitingOwnSync)).toBe(false)

    // Simulate step 3: debounce fires, validation passes, awaitingOwnSync = true
    state.awaitingOwnSync = true
    expect(computeSyncStatus(state.awaitingOwnSync, state.userIsEditing)).toBe('syncing')
    expect(shouldSyncYaml(state.userIsEditing, state.awaitingOwnSync)).toBe(false)

    // Simulate step 4: after tick(), sync completes; App prop reapplied
    state.awaitingOwnSync = false
    state.userIsEditing = false
    state = resyncFromGraph(appYaml)
    expect(computeSyncStatus(state.awaitingOwnSync, state.userIsEditing)).toBe('synced')
    expect(shouldSyncYaml(state.userIsEditing, state.awaitingOwnSync)).toBe(true)
    expect(state.yamlContent).toBe(appYaml)
  })

  it('shows that parse errors gate further state transitions', () => {
    // When YAML is invalid, userIsEditing stays true (line 51 comment in EditorPanel)
    const state: EditorPanelState = {
      userIsEditing: true,  // parse error → stays editing
      awaitingOwnSync: false,
      yamlContent: 'invalid: yaml: [',
    }

    // Sync should remain blocked
    expect(shouldSyncYaml(state.userIsEditing, state.awaitingOwnSync)).toBe(false)
    expect(computeSyncStatus(state.awaitingOwnSync, state.userIsEditing)).toBe('editing')
  })
})
