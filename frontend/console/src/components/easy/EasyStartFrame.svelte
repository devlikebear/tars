<script lang="ts">
  // The first run's frame (screens/D-Start): on the left the three things
  // this computer needs, each done, now or still to come; on the right the
  // one thing to do now. Setting TARS up (EasySetup) and the Start screen
  // (EasyStart) share it, so the person sees the same three steps from the
  // first second.
  import type { Snippet } from 'svelte'
  import { t } from '../../i18n'
  import { START_STEPS, type EasyStartStep, type EasyStepState } from '../../lib/easy'

  let { steps, children, foot }: { steps: Record<EasyStartStep, EasyStepState>; children: Snippet; foot?: Snippet } = $props()
</script>

<div class="frame" data-testid="easy-start-frame">
  <aside>
    <div class="mark"><span class="ring" aria-hidden="true"></span><b>TARS</b></div>
    <h1>{$t.easy.start.title}</h1>
    <p class="sub">{$t.easy.start.subtitle}</p>
    <ol aria-label={$t.easy.start.stepsLabel}>
      {#each START_STEPS as step, i (step)}
        <li class={steps[step]} data-testid="easy-start-step-{step}" data-state={steps[step]} aria-current={steps[step] === 'now' ? 'step' : undefined}>
          <span class="num" aria-hidden="true">{steps[step] === 'done' ? '✓' : i + 1}</span>
          <span class="name">{$t.easy.start.steps[step]}</span>
          <span class="state">{$t.easy.start.stepState[steps[step]]}</span>
        </li>
      {/each}
    </ol>
    {#if foot}<div class="foot">{@render foot()}</div>{/if}
  </aside>
  <main>
    {@render children()}
  </main>
</div>

<style>
  .frame {
    display: flex;
    min-height: 560px;
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    overflow: hidden;
  }
  aside {
    width: 320px;
    flex-shrink: 0;
    box-sizing: border-box;
    background: var(--surface);
    border-right: 1px solid var(--border-subtle);
    padding: var(--space-8) var(--space-6) var(--space-6);
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }
  .mark {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    letter-spacing: 0.04em;
  }
  .ring {
    width: 20px;
    height: 20px;
    box-sizing: border-box;
    border: 4px solid var(--primary);
    border-radius: 50%;
  }
  h1 {
    margin: var(--space-2) 0 0;
    font-family: var(--font-display);
    font-size: var(--text-2xl);
    font-weight: 600;
    line-height: 1.35;
  }
  .sub {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--text-secondary);
    line-height: 1.65;
  }
  ol {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }
  li {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3);
    border: 1px solid transparent;
    border-radius: var(--radius-md);
    color: var(--text-secondary);
  }
  li.now {
    color: var(--text-primary);
    font-weight: 600;
    background: var(--surface-elevated);
    border-color: var(--primary);
  }
  .num {
    width: 24px;
    height: 24px;
    flex: none;
    box-sizing: border-box;
    border-radius: 50%;
    border: 1px solid var(--border-strong);
    display: grid;
    place-items: center;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  li.done .num {
    background: var(--primary);
    border-color: var(--primary);
    color: var(--primary-contrast);
    font-family: var(--font-body);
    font-weight: 700;
  }
  li.now .num {
    border: 2px solid var(--primary);
    color: var(--primary-text);
  }
  .state {
    margin-left: auto;
    font-size: var(--text-xs);
    font-weight: 400;
  }
  li.done .state,
  li.now .state {
    color: var(--primary-text);
  }
  .foot {
    margin-top: auto;
    font-size: var(--text-sm);
  }
  main {
    flex: 1;
    min-width: 0;
    box-sizing: border-box;
    padding: var(--space-8);
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }
  @media (max-width: 820px) {
    .frame {
      flex-direction: column;
    }
    aside {
      width: auto;
      border-right: 0;
      border-bottom: 1px solid var(--border-subtle);
      padding: var(--space-5);
    }
    main {
      padding: var(--space-5);
    }
  }
</style>
