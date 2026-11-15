<script lang="ts">
  import type { SignalType } from './types'
  let { finalSpanCount, stageCount, signal = 'traces' }: {
    finalSpanCount: number
    stageCount: number
    signal?: SignalType
  } = $props()
</script>

<div class="summary-card">
  <div class="summary-stat">
    <span class="stat-value">{finalSpanCount}</span>
    <span class="stat-label">Retained {signal === 'traces' ? 'Span' : signal === 'logs' ? 'Log Record' : 'Metric Data Point'} Count</span>
  </div>
  <div class="summary-stat">
    <span class="stat-value">{stageCount}</span>
    <span class="stat-label">Stages</span>
  </div>
</div>

<style>
  .summary-card {
    display: flex;
    gap: var(--space-lg, 1.5rem);
    background: var(--color-surface, #1e293b);
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-lg, 8px);
    padding: var(--space-md, 1rem) 1.25rem;
    margin-bottom: var(--space-md, 1rem);
  }
  .summary-stat {
    display: flex;
    flex-direction: column;
  }
  .stat-value {
    font-size: 1.5rem;
    font-weight: 700;
    color: var(--color-text, #f1f5f9);
  }
  .stat-label {
    font-size: 0.75rem;
    color: var(--color-text-secondary, #94a3b8);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
</style>
