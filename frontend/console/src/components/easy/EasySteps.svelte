<script lang="ts">
  // The workspace header's four steps (plan, make, check, publish), each
  // done, now or next as easyView answers.
  import { t } from '../../i18n'
  import { EASY_STEPS, type EasyStep, type EasyStepState } from '../../lib/easy'

  let { steps }: { steps: Record<EasyStep, EasyStepState> } = $props()
</script>

<ol class="steps" aria-label={$t.easy.workspace.stepsLabel} data-testid="easy-steps">
  {#each EASY_STEPS as step, i (step)}
    {#if i > 0}<li class="rule" aria-hidden="true"></li>{/if}
    <li class="step {steps[step]}" data-testid="easy-step-{step}" data-state={steps[step]} aria-current={steps[step] === 'now' ? 'step' : undefined}>
      <span class="mark" aria-hidden="true">{steps[step] === 'done' ? '✓' : i + 1}</span>
      {$t.easy.workspace.steps[step]}
      {#if steps[step] === 'done'}<span class="sr-only">({$t.easy.workspace.stepDone})</span>{/if}
    </li>
  {/each}
</ol>

<style>
  .steps {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }
  .step {
    display: flex;
    align-items: center;
    gap: 7px;
    white-space: nowrap;
  }
  .rule {
    width: 28px;
    height: 1px;
    background: var(--border-default);
  }
  .mark {
    width: 20px;
    height: 20px;
    border-radius: 50%;
    border: 1px solid var(--border-strong);
    box-sizing: border-box;
    display: grid;
    place-items: center;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  .step.now {
    color: var(--text-primary);
    font-weight: 600;
  }
  .step.now .mark {
    border: 2px solid var(--primary);
    color: var(--primary-text);
  }
  .step.done .mark {
    border-color: rgba(var(--primary-rgb), 0.5);
    background: rgba(var(--primary-rgb), 0.12);
    color: var(--primary-text);
  }
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
  }
  @media (max-width: 640px) {
    .rule {
      width: 10px;
    }
    .step:not(.now) {
      font-size: 0;
      gap: 0;
    }
  }
</style>
