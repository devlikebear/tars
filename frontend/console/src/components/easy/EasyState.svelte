<script lang="ts">
  // The workspace's right side when the team is not at work
  // (docs/plans/vibe-mode-redesign unit 5, screens/D-States): getting the
  // project ready, could not start, a pause on the usage limit, stuck. One
  // card with a title and one sentence; what to press sits in the
  // workspace's bar. Everything shown is easyView's.
  import { locale, t } from '../../i18n'
  import { easyLimitResets, easyStuck, type EasyView } from '../../lib/easy'

  let { view, note = $bindable(''), busy = false }: { view: EasyView; note?: string; busy?: boolean } = $props()

  const resets = $derived(view.limit ? easyLimitResets(view.limit, $locale) : '')
  const stuck = $derived(view.phase === 'stuck' ? easyStuck(view) : null)
  const stepOfBot = { leader: 'plan', maker: 'make', reviewer: 'check', checker: 'check', publisher: 'publish' } as const
</script>

{#if view.phase === 'creating'}
  <section class="card" data-testid="easy-state" data-state="creating">
    <h2><span class="spin" aria-hidden="true"></span>{$t.easy.states.creating.title}</h2>
    <p class="say" data-testid="easy-message">{$t.easy.states.creating.body}</p>
    <ol class="steps">
      {#each $t.easy.states.creating.steps as step (step)}
        <li><span class="ring" aria-hidden="true"></span>{step}</li>
      {/each}
    </ol>
  </section>
{:else if view.phase === 'failed'}
  <section class="card bad" data-testid="easy-state" data-state="failed">
    <h2>{$t.easy.states.failed.title}</h2>
    <p class="say" data-testid="easy-message">{$t.easy.errors[view.errorCode ?? 'unknown'] ?? $t.easy.errors.unknown}</p>
    <p class="dim">{$t.easy.states.failed.hint}</p>
  </section>
{:else if view.phase === 'limit' && view.limit}
  <section class="card wait" data-testid="easy-state" data-state="limit">
    <h2>{$t.easy.states.limit.title}</h2>
    <p class="say" data-testid="easy-message">{$t.easy.states.limit.body}</p>
    <div class="box">
      {#if resets}
        <span class="dim small">{$t.easy.states.limit.when}</span>
        <span class="time" data-testid="easy-limit-time">{resets}</span>
      {:else}
        <span data-testid="easy-limit-time">{$t.easy.states.limit.soon}</span>
      {/if}
      {#if view.stoppedAt}
        <span class="dim small">{$t.easy.states.limit.stoppedAt($t.easy.workspace.steps[stepOfBot[view.stoppedAt]], $t.easy.bots[view.stoppedAt].name)}</span>
      {/if}
    </div>
  </section>
{:else if view.phase === 'stuck' && stuck}
  <section class="card bad" data-testid="easy-state" data-state="stuck">
    <h2>{$t.easy.states.stuck.title}</h2>
    <p class="say" data-testid="easy-message">{stuck.report || stuck.label ? $t.easy.states.stuck.bodyCheck : $t.easy.states.stuck.body}</p>
    {#if stuck.report || stuck.label}
      <div class="box" data-testid="easy-stuck-check">
        <span class="failed"><span class="x" aria-hidden="true">✕</span>{stuck.label ?? stuck.report?.screen ?? ''}</span>
        {#if stuck.report}
          <dl class="report" data-testid="easy-check-report">
            <div><dt>{$t.easy.workspace.check.expected}</dt><dd>{stuck.report.expected}</dd></div>
            <div><dt>{$t.easy.workspace.check.seen}</dt><dd>{stuck.report.seen}</dd></div>
          </dl>
        {:else}
          <span class="dim small">{stuck.timedOut ? $t.easy.workspace.check.timedOut : $t.easy.workspace.check.failedPlain}</span>
        {/if}
        {#if stuck.total}
          <span class="dim small" data-testid="easy-stuck-count">{$t.easy.states.stuck.passed(stuck.passed, stuck.total)}</span>
        {/if}
      </div>
    {/if}
    {#if view.retry}
      <label class="hint-label" for="easy-stuck-note">{$t.easy.states.stuck.hintLabel}</label>
      <textarea
        id="easy-stuck-note"
        rows="3"
        data-testid="easy-stuck-note"
        placeholder={$t.easy.states.stuck.hintPlaceholder}
        bind:value={note}
        disabled={busy}
      ></textarea>
    {/if}
  </section>
{/if}

<style>
  .card {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 640px;
    padding: var(--space-6);
    background: var(--surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
  }
  .card.wait {
    border-color: var(--warning);
  }
  .card.bad {
    border-color: var(--error);
  }
  h2 {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin: 0;
    font-size: var(--text-xl);
    font-weight: 600;
    line-height: 1.35;
  }
  .say {
    margin: 0;
    color: var(--text-secondary);
    line-height: 1.65;
  }
  .dim {
    color: var(--text-secondary);
    margin: 0;
  }
  .small {
    font-size: var(--text-sm);
  }
  .steps {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  .steps li {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .ring {
    width: 22px;
    height: 22px;
    flex: none;
    box-sizing: border-box;
    border-radius: 50%;
    border: 1px solid var(--border-strong);
  }
  .spin {
    width: 18px;
    height: 18px;
    flex: none;
    box-sizing: border-box;
    border-radius: 50%;
    border: 2px solid var(--primary);
    border-top-color: transparent;
    animation: spin 1s linear infinite;
  }
  .box {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-4);
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
  }
  .time {
    font-family: var(--font-mono);
    font-size: var(--text-2xl);
    font-weight: 500;
  }
  .failed {
    display: flex;
    gap: var(--space-3);
  }
  .x {
    color: var(--error);
  }
  .report {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-2);
    margin: 0;
    font-size: var(--text-sm);
  }
  .report div {
    padding: var(--space-2) var(--space-3);
    background: var(--surface);
    border-radius: var(--radius-sm);
  }
  dt {
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .hint-label {
    font-size: var(--text-sm);
    color: var(--text-secondary);
    margin-bottom: calc(-1 * var(--space-2));
  }
  textarea {
    font: inherit;
    font-size: var(--text-base);
    color: var(--text-primary);
    background: var(--surface-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    padding: var(--space-3);
    resize: vertical;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .spin {
      animation: none;
    }
  }
</style>
