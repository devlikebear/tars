<script lang="ts">
  // CI counts of a pull request as three small chips — passing, failing,
  // pending — used on the stepper's PR stages and the G4 merge summary.
  import { t } from '../../i18n'
  import type { FocusCICounts } from '../../lib/types'

  interface Props {
    ci: FocusCICounts
  }

  let { ci }: Props = $props()
</script>

<span class="ci-chips" data-testid="focus-ci-chips">
  {#if ci.passed}<span class="ci-chip pass" title={$t.focus.pr.ciPassed(ci.passed)}>✓{ci.passed}</span>{/if}
  {#if ci.failed}<span class="ci-chip fail" title={$t.focus.pr.ciFailed(ci.failed)}>✕{ci.failed}</span>{/if}
  {#if ci.pending}<span class="ci-chip pending" title={$t.focus.pr.ciPending(ci.pending)}>…{ci.pending}</span>{/if}
</span>

<style>
  .ci-chips {
    display: inline-flex;
    gap: 2px;
  }

  .ci-chip {
    padding: 0 var(--space-1);
    border-radius: var(--radius-sm);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    line-height: 1.4;
  }

  .ci-chip.pass {
    background: rgba(var(--primary-rgb), 0.15);
    color: var(--primary-text);
  }

  .ci-chip.fail {
    background: var(--error-muted);
    color: var(--error);
  }

  .ci-chip.pending {
    background: var(--surface-elevated);
    color: var(--text-secondary);
  }
</style>
