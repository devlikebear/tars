<script lang="ts">
  // Making (screens/D-Make): the bot at work, who does what in order, and
  // the blueprint being made with the checks still to run. No preview and
  // no estimate (cut from the first version).
  import { t } from '../../i18n'
  import type { EasyBot, EasyView } from '../../lib/easy'
  import EasySketch from './EasySketch.svelte'

  let { view }: { view: EasyView } = $props()

  const order: Exclude<EasyBot, 'publisher'>[] = ['leader', 'maker', 'reviewer', 'checker']
  const face = (bot: EasyBot) => Array.from($t.easy.bots[bot].name)[0] ?? ''
</script>

<section class="making" data-testid="easy-making">
  {#if view.working}
    <div class="now" data-testid="easy-working" data-bot={view.working}>
      <span class="face" aria-hidden="true">{face(view.working)}</span>
      <div>
        <b>{$t.easy.workspace.making.who[view.working]}</b>
        <p>{$t.easy.workspace.making.nothingToDo}</p>
      </div>
    </div>
  {/if}

  <div class="strip">
    <h3>{$t.easy.workspace.making.strip}</h3>
    <ol>
      {#each order as bot (bot)}
        <li class="state-{view.bots[bot]}" data-testid="easy-strip-{bot}" data-state={view.bots[bot]}>
          <span class="when">{$t.easy.workspace.making.stripState[view.bots[bot]]}</span>
          <b>{$t.easy.workspace.making.stage[bot]}</b>
          <span class="by">
            {$t.easy.bots[bot].name}{#if bot === 'checker' && view.checks.length}{' · '}{$t.easy.workspace.making.checksCount(view.checks.length)}{/if}
          </span>
        </li>
      {/each}
    </ol>
  </div>

  {#if view.blueprint?.screens.length}
    <div class="part">
      <h3>{$t.easy.workspace.making.blueprint}</h3>
      <div class="screens">
        {#each view.blueprint.screens as screen, i (i)}
          <EasySketch {screen} small />
        {/each}
      </div>
    </div>
  {/if}

  {#if view.checks.length}
    <div class="part">
      <h3>{$t.easy.workspace.making.checks} <span class="sub">{$t.easy.workspace.making.checksSub}</span></h3>
      <ol class="checks" data-testid="easy-making-checks">
        {#each view.checks as check, i (i)}
          <li><span class="dot" aria-hidden="true"></span>{check.label}</li>
        {/each}
      </ol>
    </div>
  {:else if view.blueprint?.tasks.length}
    <div class="part">
      <h3>{$t.easy.workspace.blueprint.tasks}</h3>
      <ol class="tasks">
        {#each view.blueprint.tasks as task, i (i)}
          <li>{task}</li>
        {/each}
      </ol>
    </div>
  {/if}
</section>

<style>
  .making {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }
  h3 {
    margin: 0 0 var(--space-3);
    font-size: var(--text-md);
    font-weight: 600;
  }
  .sub {
    font-size: var(--text-xs);
    font-weight: 400;
    color: var(--text-secondary);
    margin-left: var(--space-2);
  }
  .now {
    display: flex;
    gap: var(--space-4);
    align-items: flex-start;
    padding: var(--space-4) var(--space-5);
    border: 1px solid rgba(var(--primary-rgb), 0.45);
    border-radius: var(--radius-lg);
    background: var(--surface);
  }
  .now b {
    font-size: var(--text-lg);
    font-weight: 600;
  }
  .now p {
    margin: var(--space-1) 0 0;
    color: var(--text-secondary);
  }
  .face {
    flex-shrink: 0;
    width: 44px;
    height: 44px;
    border-radius: var(--radius-md);
    background: rgba(var(--primary-rgb), 0.12);
    border: 1.5px solid var(--primary);
    display: grid;
    place-items: center;
    font-size: var(--text-lg);
    color: var(--primary-text);
    box-sizing: border-box;
    animation: pulse 1.6s ease-in-out infinite;
  }
  .strip ol {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: var(--space-2);
  }
  .strip li {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
    font-size: var(--text-sm);
    min-width: 0;
  }
  .when {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }
  .by {
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }
  .strip .state-done .when {
    color: var(--primary-text);
  }
  .strip .state-working {
    border-color: rgba(var(--primary-rgb), 0.55);
  }
  .strip .state-working .when {
    color: var(--primary-text);
  }
  .strip .state-needs_you .when {
    color: var(--warning);
  }
  .screens {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
    gap: var(--space-3);
  }
  .checks {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .checks li {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
  }
  .dot {
    flex-shrink: 0;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    border: 1px solid var(--border-strong);
  }
  .tasks {
    margin: 0;
    padding-left: var(--space-5);
    color: var(--text-secondary);
  }
  @keyframes pulse {
    50% {
      opacity: 0.6;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .face {
      animation: none;
    }
  }
  @media (max-width: 640px) {
    .strip ol {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>
