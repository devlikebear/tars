<script lang="ts">
  // G1 (ADR §4, §7): the proposed plan — tasks, the stages that apply, and
  // the verification commands — which the developer edits and approves
  // before any change is made. Unticking a stage skips it; approve sends the
  // edited plan as `edits`.
  import { untrack } from 'svelte'
  import { t } from '../../i18n'
  import { planEdits, stageLabel } from '../../lib/focus'
  import type { FocusPipeline, FocusPlan, FocusStageId } from '../../lib/types'

  interface Props {
    // The gate card: a new card (a revised plan) resets the edits; the same
    // card re-read from the server keeps them.
    cardId: string
    plan: FocusPlan | null
    // The gate is open and the card undecided: the plan can be edited.
    open: boolean
    busy: boolean
    onApprove: (edits: FocusPlan) => void
    onRequestChanges: (note: string) => void
    onStop: () => void
    // The pipeline: the stages of its template can be skipped, plan cannot.
    pipeline?: FocusPipeline | null
  }

  let { cardId, plan, open, busy, onApprove, onRequestChanges, onStop, pipeline = null }: Props = $props()

  const devStages: FocusStageId[] = ['build', 'review', 'pr', 'pr_review', 'merge']
  let optionalStages = $derived(pipeline ? pipeline.stages.map((s) => s.id).filter((id) => id !== 'plan') : devStages)

  let skipped = $state(new Set<string>())
  let verify = $state('')
  let asking = $state(false)
  let note = $state('')
  let seededFor: string | null = null

  $effect(() => {
    if (cardId === seededFor) return
    seededFor = cardId
    untrack(() => seed())
  })

  function seed() {
    skipped = new Set(optionalStages.filter((s) => !plan?.stages.includes(s)))
    verify = (plan?.verify ?? []).join('\n')
    asking = false
    note = ''
  }

  function toggle(stage: FocusStageId) {
    const next = new Set(skipped)
    if (next.has(stage)) next.delete(stage)
    else next.add(stage)
    skipped = next
  }

  function approve() {
    if (!plan) return
    // Stages the plan did not list but the developer ticked are added back.
    const withAll = { ...plan, stages: ['plan', ...optionalStages] as FocusStageId[] }
    onApprove(planEdits(withAll, { skipped, verify }))
  }

  function sendChanges() {
    const text = note.trim()
    if (!text) return
    onRequestChanges(text)
  }
</script>

{#if !plan}
  <p class="muted">{$t.focus.gate.noPlan}</p>
{:else}
  <div class="plan-gate" data-testid="focus-plan-gate">
    <section>
      <h4 class="label">{$t.focus.gate.tasks}</h4>
      <ol class="plan-tasks">
        {#each plan.tasks as task, i (i)}
          <li>
            <span class="task-title" data-content>{task.title}</span>
            {#if task.done}<span class="task-done" data-content>{$t.focus.gate.taskDone(task.done)}</span>{/if}
          </li>
        {/each}
      </ol>
    </section>

    <section>
      <h4 class="label">{$t.focus.gate.stages}</h4>
      <div class="plan-stages">
        {#each optionalStages as stage (stage)}
          <label class="stage-toggle" class:off={skipped.has(stage)}>
            <input type="checkbox" checked={!skipped.has(stage)} disabled={!open || busy} onchange={() => toggle(stage)} data-testid={`focus-plan-stage-${stage}`} />
            {stageLabel(pipeline, stage, $t.focus)}
          </label>
        {/each}
      </div>
      {#if open}<p class="hint">{$t.focus.gate.stagesHint}</p>{/if}
    </section>

    <section>
      <label class="label" for="focus-verify">{$t.focus.gate.verify}</label>
      {#if open}
        <textarea id="focus-verify" class="mono" rows={Math.max(2, verify.split('\n').length)} bind:value={verify} disabled={busy} data-testid="focus-plan-verify"></textarea>
        <p class="hint">{$t.focus.gate.verifyHint}</p>
      {:else}
        <pre class="mono" data-content>{plan.verify.join('\n')}</pre>
      {/if}
      {#if plan.e2e?.length}
        <h4 class="label">{$t.focus.gate.e2e}</h4>
        <pre class="mono" data-content>{plan.e2e.join('\n')}</pre>
      {/if}
    </section>

    {#if open}
      {#if asking}
        <div class="changes-form">
          <textarea rows="3" bind:value={note} placeholder={$t.focus.gate.notePlaceholder} disabled={busy} data-testid="focus-gate-note"></textarea>
          <div class="actions">
            <button type="button" class="btn btn-secondary btn-sm" disabled={busy || !note.trim()} onclick={sendChanges}>{$t.focus.gate.sendChanges}</button>
            <button type="button" class="btn btn-ghost btn-sm" disabled={busy} onclick={() => { asking = false }}>{$t.focus.gate.cancel}</button>
          </div>
        </div>
      {:else}
        <div class="actions">
          <button type="button" class="btn btn-primary" disabled={busy} onclick={approve} data-testid="focus-gate-approve">{$t.focus.gate.approve}</button>
          <button type="button" class="btn btn-secondary" disabled={busy} onclick={() => { asking = true }} data-testid="focus-gate-request-changes">{$t.focus.gate.requestChanges}</button>
          <button type="button" class="btn btn-danger" disabled={busy} onclick={onStop}>{$t.focus.gate.stop}</button>
        </div>
      {/if}
    {/if}
  </div>
{/if}

<style>
  .plan-gate {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  h4 {
    margin: 0;
  }

  .plan-tasks {
    margin: 0;
    padding-left: var(--space-5);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .task-title {
    display: block;
    color: var(--text-primary);
  }

  .task-done {
    display: block;
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .plan-stages {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .stage-toggle {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-2);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    cursor: pointer;
  }

  .stage-toggle.off {
    border-style: dashed;
    color: var(--text-tertiary);
    text-decoration: line-through;
  }

  textarea {
    width: 100%;
    box-sizing: border-box;
    padding: var(--space-2);
    background: var(--surface-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-sm);
    resize: vertical;
  }

  textarea:focus {
    outline: none;
    border-color: var(--primary);
  }

  pre {
    margin: 0;
    padding: var(--space-2);
    background: var(--surface-inset);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    white-space: pre-wrap;
  }

  .hint,
  .muted {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .changes-form {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
</style>
