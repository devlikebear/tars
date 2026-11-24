<script lang="ts">
  // Easy mode's project workspace (docs/decisions/vibe-coder-identity.md
  // §4–§5, docs/plans/vibe-mode-redesign unit 4): the four steps above,
  // the bot team and its log on the left, the step's own content on the
  // right, and one bar with the single thing to press. The person answers
  // the leader's questions (decision cards), approves the blueprint (the
  // plan gate) or says what to change in it; after that the server goes on
  // by goal mode's policy (GoalAfterPlan) and saves the finished version,
  // so the screen only reads: the project, then its pipeline, every two
  // seconds while it still changes on its own. Everything shown comes from
  // easyView; the result, versions and the paused and stuck states are the
  // screens they were (unit 5 redraws them).
  import { onDestroy, onMount } from 'svelte'
  import { locale, t } from '../../i18n'
  import { changeEasyProject, getEasyProject } from '../../lib/api/easy'
  import { focusCard, focusGate, getFocusPipeline } from '../../lib/api/focus'
  import { APIRequestError } from '../../lib/api/client'
  import { easyCanChange, easyLimitResets, easyLive, easyView, type EasyProject } from '../../lib/easy'
  import type { FocusPipeline } from '../../lib/types'
  import { consoleMode } from '../../lib/stores/consoleMode.svelte'
  import EasyBlueprint from './EasyBlueprint.svelte'
  import EasyChecks from './EasyChecks.svelte'
  import EasyMaking from './EasyMaking.svelte'
  import EasyPreview from './EasyPreview.svelte'
  import EasySteps from './EasySteps.svelte'
  import EasyTeam from './EasyTeam.svelte'
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

  // The plan gate's open card: the action names it so a gate the server
  // replaced since refuses it.
  function planCard(): { card_id?: string } {
    const card = pipeline?.cards.findLast((c) => c.kind === 'gate' && c.state !== 'decided')
    return card ? { card_id: card.id } : {}
  }

  async function approve() {
    if (!project?.session_id || approving) return
    approving = true
    actionFailed = false
    try {
      await focusGate(project.session_id, 'plan', 'approve', planCard())
      clearTimeout(timer)
      await loop()
    } catch {
      actionFailed = true
    } finally {
      approving = false
    }
  }

  // The usage limit stopped the team: Continue sends the turn it refused
  // again, and the server puts goal mode back on (GoalDueOnApproval).
  let continuing = $state(false)
  async function continueAfterLimit() {
    if (!project?.session_id || !view?.limit || continuing) return
    continuing = true
    actionFailed = false
    try {
      await focusGate(project.session_id, 'blocked', 'retry', { card_id: view.limit.cardId })
      clearTimeout(timer)
      await loop()
    } catch {
      actionFailed = true
    } finally {
      continuing = false
    }
  }

  // One of the leader's questions answered: its decision card is decided
  // with the choice's words. The last answer sends them all to the leader,
  // who then draws the blueprint (the server's own rule).
  let answering = $state('')
  async function answer(cardId: string, choice: string) {
    if (!project?.session_id || answering) return
    answering = cardId
    actionFailed = false
    try {
      await focusCard(project.session_id, cardId, 'decided', choice)
      clearTimeout(timer)
      await loop()
    } catch {
      actionFailed = true
    } finally {
      answering = ''
    }
  }

  // "Tell the team" on the blueprint: the plan gate's request for changes,
  // with the person's words as the note; the leader draws it again.
  let say = $state('')
  let saying = $state(false)
  let sayError = $state('')
  async function sayToTeam(event: SubmitEvent) {
    event.preventDefault()
    if (!project?.session_id || saying) return
    if (!say.trim()) {
      sayError = $t.easy.workspace.say.required
      return
    }
    saying = true
    sayError = ''
    try {
      await focusGate(project.session_id, 'plan', 'request_changes', { note: say.trim(), ...planCard() })
      say = ''
      clearTimeout(timer)
      await loop()
    } catch {
      sayError = $t.easy.project.actionFailed
    } finally {
      saying = false
    }
  }

  // The sentence above the step's content, for the steps the workspace
  // does not draw a pane of its own for.
  const leadPhases = new Set(['creating', 'failed', 'planning', 'questions', 'stuck', 'limit', 'finished'])

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
      case 'stuck':
        return consoleMode.expert ? $t.easy.project.stuck : $t.easy.project.stuckAlone
      case 'limit':
        return $t.easy.project.limitPaused(view.limit ? easyLimitResets(view.limit, $locale) : '')
      default:
        return $t.easy.project[view.phase]
    }
  }
</script>

<div class="ws" data-testid="easy-root">
  {#if notFound}
    <p class="muted pad">{$t.easy.project.notFound}</p>
  {:else if !project || !view}
    {#if loadFailed}<p class="muted pad">{$t.easy.project.loadFailed}</p>{/if}
  {:else}
    <header class="ws-head">
      <h1>{project.name}</h1>
      <EasySteps steps={view.steps} />
      <div class="head-end">
        <span class="phase phase-{view.phase}" data-testid="easy-phase" data-phase={view.phase}>{$t.easy.status[view.phase]}</span>
        {#if consoleMode.expert && project.session_id}
          <button type="button" class="btn btn-ghost btn-sm" data-testid="easy-expert" onclick={() => onNavigate(`/console/focus/${encodeURIComponent(project!.session_id!)}`)}>
            {$t.easy.project.expert}
          </button>
        {/if}
      </div>
    </header>

    <div class="ws-body">
      <aside class="ws-team">
        <EasyTeam {view} {answering} onAnswer={answer} />
        {#if view.phase === 'approve'}
          <form class="say-form" onsubmit={sayToTeam} data-testid="easy-say">
            <label class="sr-only" for="easy-say-input">{$t.easy.workspace.say.label}</label>
            <input
              id="easy-say-input"
              data-testid="easy-say-input"
              placeholder={$t.easy.workspace.say.placeholder}
              bind:value={say}
              disabled={saying}
            />
            <button type="submit" class="btn btn-secondary" data-testid="easy-say-send" disabled={saying}>
              {saying ? $t.easy.workspace.say.sending : $t.easy.workspace.say.send}
            </button>
          </form>
          {#if sayError}<p class="error say-error" role="alert" data-testid="easy-say-error">{sayError}</p>{/if}
        {/if}
      </aside>

      <main class="ws-stage">
        <div class="ws-content">
          {#if leadPhases.has(view.phase)}
            <p class="say" data-testid="easy-message">{phaseText()}</p>
          {/if}
          {#if loadFailed}<p class="muted">{$t.easy.project.loadFailed}</p>{/if}
          {#if view.change && view.phase !== 'finished'}
            <p class="asked" data-testid="easy-asked"><span>{$t.easy.project.asked}</span> {view.change}</p>
          {/if}
          {#if actionFailed && view.phase === 'questions'}<p class="error" role="alert">{$t.easy.project.actionFailed}</p>{/if}

          {#if view.phase === 'approve' && view.blueprint}
            <div data-testid="easy-your-turn">
              <EasyBlueprint blueprint={view.blueprint} />
            </div>
          {:else if view.phase === 'planning' || view.phase === 'questions'}
            <div class="placeholder" data-testid="easy-blueprint-waiting">
              <h2>{$t.easy.workspace.blueprint.title}</h2>
              <p class="muted">{$t.easy.workspace.blueprint.waiting}</p>
            </div>
          {:else if view.phase === 'building' && view.step === 'make'}
            <EasyMaking {view} />
          {:else if view.phase === 'building'}
            <EasyChecks {view} />
          {/if}

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

          {#if view.phase === 'limit' && view.limit}
            <section class="your-turn" data-testid="easy-limit">
              <button type="button" class="btn btn-primary" data-testid="easy-continue" disabled={continuing} onclick={continueAfterLimit}>
                {continuing ? $t.easy.project.continuing : $t.easy.project.continue}
              </button>
              {#if actionFailed}<p class="error" role="alert">{$t.easy.project.actionFailed}</p>{/if}
            </section>
          {/if}

          {#if view.phase === 'stuck'}
            {#if view.checks.some((c) => c.state !== 'not_run')}
              <EasyChecks {view} />
            {:else if view.check}
              <p class="check" data-testid="easy-check">{$t.easy.project.checkFailed(view.check.screen ?? '', view.check.expected, view.check.seen)}</p>
            {/if}
          {:else if view.check && view.phase === 'building' && view.checks.length === 0}
            <p class="check" data-testid="easy-check">{$t.easy.project.checkFailed(view.check.screen ?? '', view.check.expected, view.check.seen)}</p>
          {/if}
        </div>

        {#if view.phase === 'approve' || view.phase === 'questions' || view.phase === 'building'}
          <footer class="ws-bar" data-testid="easy-bar">
            <span class="hint">
              {#if view.phase === 'questions'}
                {$t.easy.workspace.bar.questions}
              {:else if view.phase === 'approve'}
                {$t.easy.workspace.bar.plan}
              {:else if view.step === 'make'}
                {$t.easy.workspace.bar.making}
              {:else}
                {$t.easy.workspace.bar.checking}
              {/if}
            </span>
            {#if view.phase === 'approve'}
              {#if actionFailed}<span class="error" role="alert">{$t.easy.project.actionFailed}</span>{/if}
              <button type="button" class="btn btn-primary go" data-testid="easy-approve" disabled={approving || saying} onclick={approve}>
                {approving ? $t.easy.project.approving : $t.easy.project.approve}
              </button>
            {/if}
          </footer>
        {/if}
      </main>
    </div>
  {/if}
</div>

<style>
  /* The workspace fills the shell's body: a header, then the team column
     beside the step's own content, whose bar stays at the bottom. */
  .ws {
    display: flex;
    flex-direction: column;
    min-height: calc(100dvh - var(--header-height) - 49px);
  }
  .pad {
    padding: var(--space-6);
  }
  .ws-head {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-3) var(--space-5);
    padding: var(--space-3) var(--space-6);
    border-bottom: 1px solid var(--border-subtle);
    background: var(--surface);
  }
  h1 {
    font-family: var(--font-display);
    font-size: var(--text-lg);
    font-weight: 600;
    margin: 0;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 320px;
  }
  .ws-head :global(.steps) {
    margin: 0 auto;
  }
  .head-end {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .ws-body {
    flex: 1;
    display: flex;
    min-height: 0;
  }
  .ws-team {
    width: 360px;
    flex-shrink: 0;
    border-right: 1px solid var(--border-subtle);
    background: var(--surface);
    display: flex;
    flex-direction: column;
  }
  .ws-team :global(.team) {
    flex: 1;
  }
  .say-form {
    display: flex;
    gap: var(--space-2);
    padding: var(--space-3);
    border-top: 1px solid var(--border-subtle);
    position: sticky;
    bottom: 0;
    background: var(--surface);
  }
  .say-form input {
    flex: 1;
    min-width: 0;
    font: inherit;
    font-size: var(--text-base);
    color: var(--text-primary);
    background: var(--surface-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    padding: var(--space-3);
  }
  .say-error {
    margin: 0;
    padding: 0 var(--space-3) var(--space-3);
  }
  .ws-stage {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .ws-content {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    padding: var(--space-5) var(--space-6) var(--space-8);
    max-width: 980px;
    width: 100%;
    box-sizing: border-box;
  }
  .ws-bar {
    position: sticky;
    bottom: 0;
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-3) var(--space-4);
    /* With expert mode on, CASE sits in the bottom-right corner: the bar
       keeps its button clear of CASE's own. */
    padding: var(--space-3) calc(var(--space-6) + var(--case-button-width)) var(--space-3) var(--space-6);
    border-top: 1px solid var(--border-subtle);
    background: var(--surface);
  }
  .hint {
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }
  .go {
    margin-left: auto;
    font-size: var(--text-md);
    padding: var(--space-3) var(--space-5);
  }
  h2 {
    font-size: var(--text-lg);
    margin: 0 0 var(--space-3);
  }
  .placeholder {
    border: 1px dashed var(--border-default);
    border-radius: var(--radius-lg);
    padding: var(--space-6);
  }
  .placeholder p {
    margin: 0;
  }
  .say {
    font-size: var(--text-md);
    color: var(--text-secondary);
    margin: 0;
  }
  .muted {
    color: var(--text-secondary);
  }
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
  }
  .phase {
    display: inline-block;
    font-size: var(--text-xs);
    padding: 3px var(--space-3);
    border-radius: 999px;
    background: var(--surface-elevated);
    color: var(--text-secondary);
    white-space: nowrap;
  }
  .phase-approve,
  .phase-questions,
  .phase-limit,
  .phase-stuck {
    background: var(--warning-muted);
    color: var(--warning);
  }
  .phase-building,
  .phase-planning {
    background: var(--primary-muted);
    color: var(--primary-text);
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
  .error {
    color: var(--error);
  }
  .asked {
    margin: 0;
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
  @media (max-width: 900px) {
    .ws-head {
      padding: var(--space-3) var(--space-4);
    }
    .ws-head :global(.steps) {
      margin: 0;
      order: 3;
      width: 100%;
    }
    .ws-body {
      flex-direction: column;
    }
    .ws-team {
      width: auto;
      border-right: 0;
      border-bottom: 1px solid var(--border-subtle);
    }
    .ws-content {
      padding-left: var(--space-4);
      padding-right: var(--space-4);
    }
    .ws-bar {
      padding-left: var(--space-4);
    }
  }
</style>
