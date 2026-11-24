<script lang="ts">
  // Checking (screens/D-Check): the blueprint's checks with the server's
  // latest result each — not tried yet, passed, or not as expected with
  // what the check expected and saw, in the check's own words. The screen
  // never judges a check: every state here is the server's.
  import { t } from '../../i18n'
  import { checksPassed, type EasyView } from '../../lib/easy'

  let { view }: { view: EasyView } = $props()
  const count = $derived(checksPassed(view.checks))
  const fixing = $derived(view.phase === 'building' && view.checks.some((c) => c.state === 'failed'))
</script>

<section class="checks" data-testid="easy-checks">
  <div class="head">
    <div>
      <h2>{$t.easy.workspace.check.title}</h2>
      <p class="sub">{$t.easy.workspace.check.sub}</p>
    </div>
    {#if view.checks.length}
      <p class="count" data-testid="easy-checks-count" data-passed={count.passed} data-total={count.total}>
        <b>{count.passed}</b>
        {$t.easy.workspace.check.of(count.total)}
      </p>
    {/if}
  </div>

  {#if view.checks.length}
    <ol>
      {#each view.checks as check, i (i)}
        <li class="row {check.state}" data-testid="easy-check-row" data-state={check.state}>
          <span class="mark" aria-hidden="true">{check.state === 'passed' ? '✓' : check.state === 'failed' ? '✕' : ''}</span>
          <div class="body">
            <div class="line">
              <span class="words">{check.label}</span>
              <span class="state">{$t.easy.workspace.check.state[check.state]}</span>
            </div>
            {#if check.state === 'failed'}
              {#if check.report}
                {#if check.report.screen}<span class="screen">{check.report.screen}</span>{/if}
                <dl class="report" data-testid="easy-check-report">
                  <dt>{$t.easy.workspace.check.expected}</dt>
                  <dd>{check.report.expected}</dd>
                  <dt>{$t.easy.workspace.check.seen}</dt>
                  <dd>{check.report.seen}</dd>
                </dl>
              {:else}
                <span class="plain">{check.timedOut ? $t.easy.workspace.check.timedOut : $t.easy.workspace.check.failedPlain}</span>
              {/if}
            {/if}
          </div>
        </li>
      {/each}
    </ol>
    {#if fixing}<p class="fixing" data-testid="easy-checks-fixing">{$t.easy.workspace.check.fixing}</p>{/if}
  {:else}
    <p class="sub">{$t.easy.workspace.check.none}</p>
  {/if}
</section>

<style>
  .checks {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }
  .head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }
  h2 {
    margin: 0;
    font-size: var(--text-xl);
    font-weight: 600;
  }
  .sub {
    margin: var(--space-1) 0 0;
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }
  .count {
    margin: 0;
    white-space: nowrap;
    color: var(--text-secondary);
  }
  .count b {
    font-family: var(--font-mono);
    font-size: var(--text-2xl);
    color: var(--text-primary);
    margin-right: 4px;
  }
  ol {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .row {
    display: flex;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
  }
  .row.failed {
    border-left: 3px solid var(--warning);
  }
  .mark {
    flex-shrink: 0;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    font-size: var(--text-xs);
    border: 1px solid var(--border-strong);
    box-sizing: border-box;
  }
  .passed .mark {
    border: 0;
    background: rgba(var(--primary-rgb), 0.15);
    color: var(--primary-text);
  }
  .failed .mark {
    border: 0;
    background: var(--warning-muted);
    color: var(--warning);
  }
  .body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .line {
    display: flex;
    gap: var(--space-3);
    justify-content: space-between;
    align-items: baseline;
  }
  .label {
    overflow-wrap: anywhere;
  }
  .state {
    flex-shrink: 0;
    font-size: var(--text-sm);
    color: var(--text-tertiary);
  }
  .passed .state {
    color: var(--primary-text);
  }
  .failed .state {
    color: var(--warning);
  }
  .screen {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }
  .report {
    margin: 0;
    display: grid;
    grid-template-columns: auto 1fr;
    gap: var(--space-1) var(--space-3);
    padding: var(--space-2) var(--space-3);
    background: var(--surface-inset);
    border-radius: var(--radius-sm);
    font-size: var(--text-sm);
  }
  .report dt {
    color: var(--text-tertiary);
  }
  .report dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .plain {
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }
  .fixing {
    margin: 0;
    color: var(--text-secondary);
  }
</style>
