<script lang="ts">
  // Easy mode's progress screen (docs/decisions/vibe-coder-identity.md §4–§5):
  // the bot team doing the stages in turn, and at most one thing for the
  // person to do — approve the plan. After that the server goes on by goal
  // mode's policy (GoalAfterPlan) and saves the finished version, so the
  // screen only reads: the project, then its pipeline, every two seconds
  // while it still changes on its own.
  import { onDestroy, onMount } from 'svelte'
  import { t } from '../../i18n'
  import { changeEasyProject, getEasyProject } from '../../lib/api/easy'
  import { focusGate, getFocusPipeline } from '../../lib/api/focus'
  import { APIRequestError } from '../../lib/api/client'
  import { EASY_BOTS, easyBase, easyCanChange, easyLive, easyView, type EasyProject } from '../../lib/easy'
  import type { FocusPipeline } from '../../lib/types'
  import EasyPreview from './EasyPreview.svelte'
  import EasyVersions from './EasyVersions.svelte'

  let { projectId, onNavigate }: { projectId: string; onNavigate: (path: string) => void } = $props()

  const pollMs = 2000

  let project = $state<EasyProject | null>(null)
  let pipeline = $state<FocusPipeline | null>(null)
  let notFound = $state(false)
  let loadFailed = $state(false)
  let approving = $state(false)
  let actionFailed = $state(false)
  let timer: ReturnType<typeof setTimeout> | undefined
  let stopped = false

  let view = $derived(project ? easyView(project, pipeline) : null)

  async function read() {
    try {
      const next = await getEasyProject(projectId)
      // The project and its pipeline are shown together: after a change the
      // project names a new pipeline, and showing it with the old one (or
      // none) would flash the wrong step.
      const nextPipeline = next.status === 'ready' && next.session_id ? await getFocusPipeline(next.session_id) : null
      project = next
      pipeline = nextPipeline
      loadFailed = false
    } catch (err) {
      if (err instanceof APIRequestError && err.status === 404) {
        notFound = true
        return
      }
      loadFailed = true
    }
  }

  async function loop() {
    await read()
    if (stopped || notFound) return
    if (!view || easyLive(view) || loadFailed) timer = setTimeout(loop, pollMs)
  }

  onMount(loop)
  onDestroy(() => {
    stopped = true
    clearTimeout(timer)
  })

  async function approve() {
    if (!project?.session_id || approving) return
    approving = true
    actionFailed = false
    try {
      const card = pipeline?.cards.findLast((c) => c.kind === 'gate' && c.state !== 'decided')
      await focusGate(project.session_id, 'plan', 'approve', card ? { card_id: card.id } : {})
      clearTimeout(timer)
      await loop()
    } catch {
      actionFailed = true
    } finally {
      approving = false
    }
  }

  let previewKey = $state(0)
  let changeRequest = $state('')
  let changing = $state(false)
  let changeError = $state('')

  async function submitChange() {
    if (!project || changing) return
    if (!changeRequest.trim()) {
      changeError = $t.easy.errors.change_required
      return
    }
    changing = true
    changeError = ''
    try {
      await changeEasyProject(project.id, changeRequest)
      changeRequest = ''
      clearTimeout(timer)
      await loop()
    } catch (err) {
      const code = err instanceof APIRequestError ? String((err.payload as { code?: string } | undefined)?.code ?? '') : ''
      changeError = (code && $t.easy.errors[code]) || $t.easy.errors.unknown
    } finally {
      changing = false
    }
  }

  function phaseText(): string {
    if (!view) return ''
    switch (view.phase) {
      case 'failed':
        return $t.easy.errors[view.errorCode ?? 'unknown'] ?? $t.easy.errors.unknown
      case 'finished':
        if (view.saved === undefined) return $t.easy.project.finishedSaving
        return view.saved ? $t.easy.project.finished : $t.easy.project.finishedNotSaved
      case 'approve':
        return $t.easy.project.approveIntro
      default:
        return $t.easy.project[view.phase]
    }
  }
</script>

<div class="easy" data-testid="easy-root">
  <nav class="top">
    <button type="button" class="btn btn-ghost btn-sm" onclick={() => onNavigate(easyBase)}>← {$t.easy.project.back}</button>
    {#if project?.session_id}
      <button type="button" class="btn btn-ghost btn-sm" data-testid="easy-expert" onclick={() => onNavigate(`/console/focus/${encodeURIComponent(project!.session_id!)}`)}>
        {$t.easy.project.expert}
      </button>
    {/if}
  </nav>

  {#if notFound}
    <p class="muted">{$t.easy.project.notFound}</p>
  {:else if !project || !view}
    {#if loadFailed}<p class="muted">{$t.easy.project.loadFailed}</p>{/if}
  {:else}
    <header>
      <span class="phase phase-{view.phase}" data-testid="easy-phase" data-phase={view.phase}>{$t.easy.status[view.phase]}</span>
      <h1>{project.name}</h1>
      <p class="say" data-testid="easy-message">{phaseText()}</p>
      {#if loadFailed}<p class="muted">{$t.easy.project.loadFailed}</p>{/if}
      {#if view.change && view.phase !== 'finished'}
        <p class="asked" data-testid="easy-asked"><span>{$t.easy.project.asked}</span> {view.change}</p>
      {/if}
    </header>

    {#if view.phase === 'finished'}
      <!-- Going back stops the preview on the server; a new key starts it
           again on the files the folder holds now. -->
      {#key previewKey}
        <EasyPreview projectId={project.id} />
      {/key}
    {/if}

    {#if easyCanChange(view)}
      <section class="change" data-testid="easy-change">
        <h2>{$t.easy.change.title}</h2>
        <p class="muted">{$t.easy.change.hint}</p>
        <textarea
          rows="3"
          data-testid="easy-change-input"
          placeholder={$t.easy.change.placeholder}
          aria-label={$t.easy.change.title}
          bind:value={changeRequest}
        ></textarea>
        <button type="button" class="btn btn-primary" data-testid="easy-change-submit" disabled={changing} onclick={submitChange}>
          {changing ? $t.easy.change.submitting : $t.easy.change.submit}
        </button>
        {#if changeError}<p class="error" role="alert" data-testid="easy-change-error">{changeError}</p>{/if}
      </section>
      <EasyVersions projectId={project.id} onRestored={() => (previewKey += 1)} />
    {/if}

    {#if view.phase === 'approve' && view.plan}
      <section class="your-turn" data-testid="easy-your-turn">
        <h2>{$t.easy.project.yourTurn}</h2>
        <p class="plan-goal">{view.plan.goal}</p>
        <ol>
          {#each view.plan.tasks as task, i (i)}
            <li>{task}</li>
          {/each}
        </ol>
        <button type="button" class="btn btn-primary" data-testid="easy-approve" disabled={approving} onclick={approve}>
          {approving ? $t.easy.project.approving : $t.easy.project.approve}
        </button>
        {#if actionFailed}<p class="error" role="alert">{$t.easy.project.actionFailed}</p>{/if}
      </section>
    {/if}

    {#if view.check && view.phase !== 'finished'}
      <p class="check" data-testid="easy-check">{$t.easy.project.checkFailed(view.check.screen ?? '', view.check.expected, view.check.seen)}</p>
    {/if}

    <section class="team">
      <h2>{$t.easy.project.team}</h2>
      <ul>
        {#each EASY_BOTS as bot (bot)}
          <li class="bot state-{view.bots[bot]}" data-testid="easy-bot-{bot}" data-state={view.bots[bot]}>
            <span class="dot" aria-hidden="true"></span>
            <span class="who">
              <strong>{$t.easy.bots[bot].name}</strong>
              <span class="role">{$t.easy.bots[bot].role}</span>
            </span>
            <span class="state">{$t.easy.botState[view.bots[bot]]}</span>
          </li>
        {/each}
      </ul>
    </section>
  {/if}
</div>

<style>
  .easy {
    display: flex;
    flex-direction: column;
    gap: var(--space-6);
    max-width: 720px;
    margin: 0 auto;
    padding: var(--space-6) var(--space-6) var(--space-10);
  }
  .top {
    display: flex;
    justify-content: space-between;
  }
  h1 {
    font-family: var(--font-display);
    font-size: var(--text-2xl);
    margin: var(--space-2) 0;
  }
  h2 {
    font-size: var(--text-lg);
    margin: 0 0 var(--space-3);
  }
  .say {
    font-size: var(--text-md);
    color: var(--text-secondary);
    margin: 0;
  }
  .muted {
    color: var(--text-secondary);
  }
  .phase {
    display: inline-block;
    font-size: var(--text-xs);
    padding: 2px var(--space-2);
    border-radius: 999px;
    background: var(--surface-elevated);
    color: var(--text-secondary);
  }
  .phase-approve,
  .phase-stuck {
    background: var(--warning-muted);
    color: var(--warning);
  }
  .phase-failed {
    background: var(--error-muted);
    color: var(--error);
  }
  .phase-finished {
    background: var(--success-muted);
    color: var(--success);
  }
  .your-turn {
    border: 1px solid rgba(var(--primary-rgb), 0.45);
    border-radius: var(--radius-lg);
    padding: var(--space-5);
    background: var(--surface);
  }
  .plan-goal {
    font-weight: 600;
    margin: 0 0 var(--space-2);
  }
  ol {
    margin: 0 0 var(--space-4);
    padding-left: var(--space-5);
    color: var(--text-secondary);
  }
  .error {
    color: var(--error);
  }
  .asked {
    margin: var(--space-3) 0 0;
    padding: var(--space-2) var(--space-3);
    border-left: 3px solid rgba(var(--primary-rgb), 0.6);
    background: var(--surface);
  }
  .asked span {
    display: block;
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }
  .change {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-3);
  }
  .change h2,
  .change .muted {
    margin: 0;
  }
  .change textarea {
    width: 100%;
    box-sizing: border-box;
    padding: var(--space-3);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--text-md);
    resize: vertical;
  }
  .check {
    border-left: 3px solid var(--warning);
    padding: var(--space-2) var(--space-3);
    background: var(--warning-muted);
    margin: 0;
  }
  .team ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .bot {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
  }
  .who {
    display: flex;
    flex-direction: column;
    flex: 1;
  }
  .role,
  .state {
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }
  .dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: var(--border-strong);
  }
  .state-working .dot {
    background: var(--primary);
    animation: pulse 1.2s ease-in-out infinite;
  }
  .state-done .dot {
    background: var(--success);
  }
  .state-needs_you .dot {
    background: var(--warning);
  }
  .state-later {
    opacity: 0.55;
  }
  .state-working .state {
    color: var(--primary-text);
  }
  @keyframes pulse {
    50% {
      opacity: 0.35;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .state-working .dot {
      animation: none;
    }
  }
</style>
