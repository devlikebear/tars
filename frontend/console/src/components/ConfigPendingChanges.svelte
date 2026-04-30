<script lang="ts">
  type ConfigDiffEntry = {
    key: string
    label: string
    path: string
    oldVal: string
    newVal: string
    impact: string[]
  }

  interface Props {
    entries: ConfigDiffEntry[]
    onClose: () => void
  }

  let { entries, onClose }: Props = $props()
</script>

<div class="diff-panel card">
  <div class="card-header">
    <span class="card-title">Pending Changes</span>
    <button class="btn btn-ghost btn-sm" onclick={onClose}>Close</button>
  </div>
  <div class="diff-body">
    {#each entries as entry}
      <div class="diff-row">
        <div class="diff-field">
          <span class="diff-label">{entry.label}</span>
          <span class="diff-key">{entry.path}</span>
        </div>
        <div class="diff-values">
          <span class="diff-old">{entry.oldVal}</span>
          <span class="diff-arrow">&rarr;</span>
          <span class="diff-new">{entry.newVal}</span>
        </div>
        {#if entry.impact.length > 0}
          <div class="diff-impact" aria-label={`${entry.label} Impact`}>
            <span class="diff-impact-title">Impact</span>
            <ul>
              {#each entry.impact as item}
                <li>{item}</li>
              {/each}
            </ul>
          </div>
        {/if}
      </div>
    {/each}
  </div>
</div>

<style>
  .diff-panel { border-color: rgba(224, 145, 69, 0.3); }
  .diff-body { display: flex; flex-direction: column; }

  .diff-row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--space-4);
    padding: var(--space-2) var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
    font-size: var(--text-xs);
  }
  .diff-row:last-child { border-bottom: none; }

  .diff-field { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
  .diff-label { font-family: var(--font-display); font-weight: 500; color: var(--text-primary); }
  .diff-key { font-family: var(--font-mono); font-size: 10px; color: var(--text-ghost); }

  .diff-values { display: flex; align-items: center; gap: var(--space-2); flex-shrink: 0; font-family: var(--font-mono); }
  .diff-old { color: var(--red); text-decoration: line-through; max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .diff-arrow { color: var(--text-ghost); }
  .diff-new { color: var(--green); font-weight: 600; max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .diff-impact {
    flex-basis: 100%;
    margin-top: var(--space-2);
    padding: var(--space-3);
    border-radius: var(--radius-md);
    background: var(--surface-base);
  }
  .diff-impact-title {
    display: block;
    margin-bottom: var(--space-1);
    color: var(--text-primary);
    font-family: var(--font-display);
    font-weight: 600;
  }
  .diff-impact ul {
    margin: 0;
    padding-left: var(--space-4);
    color: var(--text-secondary);
  }
  .diff-impact li + li { margin-top: 2px; }
</style>
