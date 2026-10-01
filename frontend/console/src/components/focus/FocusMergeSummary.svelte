<script lang="ts">
  // G4's summary (ADR §4, §7): facts the server collected — the PR, its
  // checks, how the PR's findings were decided — never the model's word.
  import { t } from '../../i18n'
  import type { FocusMergeSummary } from '../../lib/types'
  import FocusCIChips from './FocusCIChips.svelte'

  interface Props {
    summary: FocusMergeSummary
  }

  let { summary }: Props = $props()
  let hasChecks = $derived(summary.checks.passed + summary.checks.failed + summary.checks.pending > 0)
</script>

<div class="merge-summary" data-testid="focus-merge-summary">
  {#if summary.pr && summary.pr.number > 0}
    <p class="pr-line">
      {#if summary.pr.url}
        <a class="mono" href={summary.pr.url} target="_blank" rel="noreferrer">{$t.focus.pr.prLink(summary.pr.number)}</a>
      {:else}
        <span class="mono">{$t.focus.pr.prLink(summary.pr.number)}</span>
      {/if}
      {#if summary.title}<span data-content>{summary.title}</span>{/if}
    </p>
  {:else if summary.title}
    <p class="pr-line" data-content>{summary.title}</p>
  {/if}
  <p class="facts">
    <span class="label">{$t.focus.pr.checks}</span>
    {#if hasChecks}<FocusCIChips ci={summary.checks} />{:else}<span class="muted">{$t.focus.pr.noChecks}</span>{/if}
  </p>
  <p class="muted">{$t.focus.pr.findings(summary.fixed, summary.dismissed, summary.undecided)}</p>
  {#if summary.merge_state}<p class="muted mono">{$t.focus.pr.mergeState(summary.merge_state)}</p>{/if}
  <p class="hint">{$t.focus.pr.mergeHint}</p>
</div>

<style>
  .merge-summary {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  p {
    margin: 0;
  }

  .pr-line,
  .facts {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .pr-line a {
    color: var(--primary-text);
  }

  .muted,
  .hint {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }
</style>
