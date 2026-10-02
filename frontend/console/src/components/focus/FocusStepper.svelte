<script lang="ts">
  // The pipeline stepper (ADR §3): done ✓, the current stage highlighted
  // with a ↻N badge for loops, pending, skipped struck through. Clicking a
  // stage shows its card history.
  import { t } from '../../i18n'
  import type { StepperItem } from '../../lib/focus'
  import type { PRStageChip } from '../../lib/focusPR'
  import type { FocusStageId } from '../../lib/types'
  import FocusCIChips from './FocusCIChips.svelte'

  interface Props {
    items: StepperItem[]
    // The stage whose cards are shown.
    selected: FocusStageId | null
    onSelect: (stage: FocusStageId) => void
    // The PR stages' PR number and CI counts (P4).
    chips?: Partial<Record<FocusStageId, PRStageChip>>
  }

  let { items, selected, onSelect, chips = {} }: Props = $props()

  function mark(item: StepperItem): string {
    switch (item.status) {
      case 'done':
        return '✓'
      case 'skipped':
        return '–'
      case 'blocked':
        return '!'
      default:
        return item.current ? '●' : '○'
    }
  }
</script>

<ol class="focus-stepper" aria-label={$t.focus.home.stage}>
  {#each items as item, i (item.id)}
    <li class="step-wrap">
      {#if i > 0}<span class="step-line" class:done={item.status === 'done' || item.current} aria-hidden="true"></span>{/if}
      <button
        type="button"
        class="step step-{item.status}"
        class:current={item.current}
        class:selected={selected === item.id}
        aria-current={item.current ? 'step' : undefined}
        title={`${item.label} · ${$t.focus.status[item.status]}`}
        data-testid={`focus-step-${item.id}`}
        data-status={item.status}
        onclick={() => onSelect(item.id)}
      >
        <span class="step-mark" aria-hidden="true">{mark(item)}</span>
        <span class="step-label">{item.label}</span>
        {#if item.current && item.iteration > 1}
          <span class="step-iter" title={$t.focus.iteration(item.iteration)}>↻{item.iteration}</span>
        {/if}
        {#if chips[item.id]?.ci}<FocusCIChips ci={chips[item.id]!.ci!} />{/if}
      </button>
      {#if chips[item.id]}
        {@const chip = chips[item.id]!}
        {#if chip.url}
          <a class="step-pr mono" href={chip.url} target="_blank" rel="noreferrer" data-testid={`focus-step-pr-${item.id}`}>#{chip.number}</a>
        {:else}
          <span class="step-pr mono" data-testid={`focus-step-pr-${item.id}`}>#{chip.number}</span>
        {/if}
      {/if}
    </li>
  {/each}
</ol>

<style>
  .focus-stepper {
    display: flex;
    align-items: center;
    gap: 0;
    margin: 0;
    padding: 0;
    list-style: none;
    overflow-x: auto;
  }

  .step-wrap {
    display: flex;
    align-items: center;
    flex: 0 0 auto;
  }

  .step-line {
    width: var(--space-6);
    height: 1px;
    background: var(--border-default);
  }

  .step-line.done {
    background: rgba(var(--primary-rgb), 0.5);
  }

  .step {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    cursor: pointer;
  }

  .step:hover {
    border-color: var(--border-strong);
  }

  .step-done {
    color: var(--text-primary);
  }

  .step-done .step-mark {
    color: var(--primary);
  }

  .step.current {
    border-color: var(--primary);
    background: var(--primary-muted);
    color: var(--primary-text);
  }

  .step-skipped {
    border-style: dashed;
    color: var(--text-tertiary);
  }

  .step-skipped .step-label {
    text-decoration: line-through;
  }

  .step-blocked {
    border-color: var(--warning);
    color: var(--warning);
  }

  .step.selected:not(.current) {
    background: var(--surface-elevated);
    border-color: var(--border-strong);
  }

  .step-pr {
    margin-left: var(--space-1);
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  a.step-pr:hover {
    color: var(--primary-text);
  }

  .step-iter {
    padding: 0 var(--space-1);
    border-radius: var(--radius-sm);
    background: rgba(var(--primary-rgb), 0.2);
  }
</style>
