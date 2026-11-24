<script lang="ts">
  // The blueprint to approve (docs/plans/vibe-mode-redesign unit 4,
  // screens/D-Plan): the goal, the screens sketched, how it will be
  // checked, what is kept and what is left out — the leader's plan in its
  // own words. A plan from before blueprints shows its goal and tasks.
  import { t } from '../../i18n'
  import { hasBlueprint, type EasyBlueprint } from '../../lib/easy'
  import EasySketch from './EasySketch.svelte'

  let { blueprint }: { blueprint: EasyBlueprint } = $props()
  const full = $derived(hasBlueprint(blueprint))
</script>

<section class="blueprint" data-testid="easy-blueprint">
  <div class="head">
    <h2>{$t.easy.workspace.blueprint.title}</h2>
    {#if blueprint.screens.length}<span class="sub">{$t.easy.workspace.blueprint.sub}</span>{/if}
  </div>
  <p class="goal" data-testid="easy-blueprint-goal">{blueprint.goal}</p>

  {#if blueprint.screens.length}
    <div class="screens">
      {#each blueprint.screens as screen, i (i)}
        <EasySketch {screen} />
      {/each}
    </div>
  {/if}

  <div class="facts" class:single={!full}>
    {#if blueprint.checks.length}
      <div class="box" data-testid="easy-blueprint-checks">
        <div class="box-head"><b>{$t.easy.workspace.blueprint.checks}</b><span class="sub">{$t.easy.workspace.blueprint.checksSub}</span></div>
        <ol class="numbered">
          {#each blueprint.checks as label, i (i)}
            <li><span class="n" aria-hidden="true">{i + 1}</span>{label}</li>
          {/each}
        </ol>
      </div>
    {/if}
    <div class="side">
      {#if blueprint.saved.length}
        <div class="box" data-testid="easy-blueprint-saved">
          <b>{$t.easy.workspace.blueprint.saved}</b>
          <span>{blueprint.saved.join(' · ')}</span>
        </div>
      {/if}
      {#if blueprint.notNow.length}
        <div class="box" data-testid="easy-blueprint-not-now">
          <b>{$t.easy.workspace.blueprint.notNow}</b>
          <span>{blueprint.notNow.join(' · ')}</span>
        </div>
      {/if}
      {#if blueprint.tasks.length}
        <div class="box" data-testid="easy-blueprint-tasks">
          <b>{$t.easy.workspace.blueprint.tasks}</b>
          <ol class="tasks">
            {#each blueprint.tasks as task, i (i)}
              <li>{task}</li>
            {/each}
          </ol>
        </div>
      {/if}
    </div>
  </div>
</section>

<style>
  .blueprint {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }
  .head {
    display: flex;
    align-items: baseline;
    flex-wrap: wrap;
    gap: var(--space-3);
  }
  h2 {
    margin: 0;
    font-size: var(--text-xl);
    font-weight: 600;
  }
  .sub {
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }
  .goal {
    margin: 0;
    font-weight: 600;
  }
  .screens {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
    gap: var(--space-4);
  }
  .facts {
    display: grid;
    grid-template-columns: 1.25fr 1fr;
    gap: var(--space-4);
    font-size: var(--text-base);
    align-items: start;
  }
  .facts.single {
    grid-template-columns: 1fr;
  }
  .side {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }
  .box {
    background: var(--surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    padding: var(--space-3) var(--space-4);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .box b {
    font-weight: 600;
  }
  .box-head {
    display: flex;
    align-items: baseline;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .box-head .sub {
    font-size: var(--text-xs);
  }
  .numbered {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .numbered li {
    display: flex;
    gap: var(--space-2);
  }
  .n {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
  }
  .tasks {
    margin: 0;
    padding-left: var(--space-5);
    color: var(--text-secondary);
  }
  @media (max-width: 900px) {
    .facts {
      grid-template-columns: 1fr;
    }
  }
</style>
