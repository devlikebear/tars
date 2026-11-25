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
  // easyView. When the team is done the right side is the result (unit 5):
  // the app running, its versions, and "change it like this" in the team
  // column; when it is not at work, the state it is in (EasyState) with
  // the one thing to press in the bar.
  import { onDestroy, onMount } from 'svelte'
  import { t } from '../../i18n'
  import { localEasyApi, type EasyAPI } from '../../lib/api/easy'
  import { easyCanChange, easyProjectPath } from '../../lib/easy'
  import { EasyProjectStore } from '../../lib/stores/easyProjectStore.svelte'
  import { consoleMode } from '../../lib/stores/consoleMode.svelte'
  import EasyBlueprint from './EasyBlueprint.svelte'
  import EasyChecks from './EasyChecks.svelte'
  import EasyMaking from './EasyMaking.svelte'
  import EasyPreview from './EasyPreview.svelte'
  import EasyState from './EasyState.svelte'
  import EasySteps from './EasySteps.svelte'
  import EasyTeam from './EasyTeam.svelte'
  import EasyVersions from './EasyVersions.svelte'

  let { projectId, onNavigate, api = localEasyApi }: { projectId: string; onNavigate: (path: string) => void; api?: EasyAPI } = $props()

  // The project, its run and every action on it (shared with the phone).
  // svelte-ignore state_referenced_locally
  const store = new EasyProjectStore({ api, projectId })
  onMount(() => store.start())
  onDestroy(() => store.stop())

  let project = $derived(store.project)
  let view = $derived(store.view)
  let notFound = $derived(store.notFound)
  let loadFailed = $derived(store.loadFailed)
  let approving = $derived(store.approving)
  let continuing = $derived(store.continuing)
  let answering = $derived(store.answering)
  let saying = $derived(store.saying)
  let retrying = $derived(store.retrying)
  let recreating = $derived(store.recreating)
  let changing = $derived(store.changing)
  let actionFailed = $derived(store.actionFailed)

  const approve = () => store.approve()
  const continueAfterLimit = () => store.continueAfterLimit()
  const answer = (cardId: string, choice: string) => store.answer(cardId, choice)

  // "Tell the team" on the blueprint: the plan gate's request for changes,
  // with the person's words as the note; the leader draws it again.
  let say = $state('')
  let sayError = $derived(store.sayError === 'required' ? $t.easy.workspace.say.required : store.sayError === 'failed' ? $t.easy.project.actionFailed : '')
  async function sayToTeam(event: SubmitEvent) {
    event.preventDefault()
    if (await store.sayToTeam(say)) say = ''
  }

  // The sentence above the step's content, for the steps the workspace
  // does not draw a pane of its own for.
  const leadPhases = new Set(['planning', 'questions'])
  // The phases EasyState draws.
  const statePhases = new Set(['creating', 'failed', 'limit', 'stuck'])

  // Try again on a stuck project, with the person's words when they
  // explained again.
  let stuckNote = $state('')
  async function retryStuck() {
    if (await store.retryStuck(stuckNote)) stuckNote = ''
  }

  // Try again on a project that could not be set up: the same idea as a new
  // project, which the screen then follows.
  async function recreate() {
    const id = await store.recreate()
    if (id) onNavigate(easyProjectPath(id))
  }

  // The number of the version the folder holds, for the result's header.
  let currentNumber = $state<number | undefined>(undefined)

  let previewKey = $state(0)
  let changeRequest = $state('')
  let changeError = $derived(store.changeError === null ? '' : $t.easy.errors[store.changeError] || $t.easy.errors.unknown)

  async function submitChange() {
    if (await store.change(changeRequest)) changeRequest = ''
  }

  function phaseText(): string {
    if (!view) return ''
    switch (view.phase) {
      case 'planning':
      case 'questions':
        return $t.easy.project[view.phase]
      default:
        return ''
    }
  }

  // The result bar's sentence: still saving, not saved, or where the app is.
  function resultText(): string {
    if (!view || view.saved === undefined) return $t.easy.project.finishedSaving
    return view.saved ? $t.easy.result.bar : $t.easy.project.finishedNotSaved
  }

  const barPhases = new Set(['approve', 'questions', 'building', 'finished', 'limit', 'stuck', 'failed'])
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
        {#if easyCanChange(view)}
          <section class="change" data-testid="easy-change">
            <label for="easy-change-input">{$t.easy.change.title}</label>
            <textarea
              id="easy-change-input"
              rows="3"
              data-testid="easy-change-input"
              placeholder={$t.easy.change.placeholder}
              bind:value={changeRequest}
              disabled={changing}
            ></textarea>
            <div class="change-row">
              <span class="hint">{$t.easy.change.hint}</span>
              <button type="button" class="btn btn-secondary" data-testid="easy-change-submit" disabled={changing} onclick={submitChange}>
                {changing ? $t.easy.change.submitting : $t.easy.change.submit}
              </button>
            </div>
            {#if changeError}<p class="error" role="alert" data-testid="easy-change-error">{changeError}</p>{/if}
          </section>
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

          {#if statePhases.has(view.phase)}
            <EasyState {view} bind:note={stuckNote} busy={retrying} />
          {/if}

          {#if view.phase === 'finished'}
            <!-- Going back stops the preview on the server; a new key starts it
                 again on the files the folder holds now. -->
            {#key previewKey}
              <EasyPreview projectId={project.id} version={currentNumber} {api} />
            {/key}
            {#if easyCanChange(view)}
              <EasyVersions projectId={project.id} {api} onRestored={() => (previewKey += 1)} onCurrent={(n) => (currentNumber = n)} />
            {/if}
          {/if}

          {#if view.check && view.phase === 'building' && view.checks.length === 0}
            <p class="check" data-testid="easy-check">{$t.easy.project.checkFailed(view.check.screen ?? '', view.check.expected, view.check.seen)}</p>
          {/if}
        </div>

        {#if barPhases.has(view.phase)}
          <footer class="ws-bar" data-testid="easy-bar">
            <span class="hint" data-testid={view.phase === 'finished' ? 'easy-message' : undefined}>
              {#if view.phase === 'questions'}
                {$t.easy.workspace.bar.questions}
              {:else if view.phase === 'approve'}
                {$t.easy.workspace.bar.plan}
              {:else if view.phase === 'finished'}
                {resultText()}
              {:else if view.phase === 'limit'}
                {$t.easy.states.limit.early}
              {:else if view.phase === 'stuck'}
                {view.retry ? $t.easy.states.stuck.bar : $t.easy.states.stuck.noRetry}
              {:else if view.phase === 'failed'}
                {$t.easy.states.failed.hint}
              {:else if view.step === 'make'}
                {$t.easy.workspace.bar.making}
              {:else}
                {$t.easy.workspace.bar.checking}
              {/if}
            </span>
            {#if actionFailed && view.phase !== 'questions'}<span class="error" role="alert">{$t.easy.project.actionFailed}</span>{/if}
            {#if view.phase === 'approve'}
              <button type="button" class="btn btn-primary go" data-testid="easy-approve" disabled={approving || saying} onclick={approve}>
                {approving ? $t.easy.project.approving : $t.easy.project.approve}
              </button>
            {:else if view.phase === 'limit' && view.limit}
              <button type="button" class="btn btn-primary go" data-testid="easy-continue" disabled={continuing} onclick={continueAfterLimit}>
                {continuing ? $t.easy.project.continuing : $t.easy.project.continue}
              </button>
            {:else if view.phase === 'stuck' && view.retry}
              <button type="button" class="btn btn-primary go" data-testid="easy-retry" disabled={retrying} onclick={retryStuck}>
                {retrying ? $t.easy.states.stuck.retrying : $t.easy.states.stuck.retry}
              </button>
            {:else if view.phase === 'failed'}
              <button type="button" class="btn btn-primary go" data-testid="easy-recreate" disabled={recreating} onclick={recreate}>
                {recreating ? $t.easy.states.failed.retrying : $t.easy.states.failed.retry}
              </button>
            {:else if view.phase === 'finished'}
              <span class="later" id="easy-publish-later">{$t.easy.result.publishLater}</span>
              <button type="button" class="btn btn-primary go" data-testid="easy-publish" disabled aria-describedby="easy-publish-later">
                {$t.easy.result.publish}
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
  /* "Change it like this" closes the team column on the result. */
  .change {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    border-top: 1px solid var(--border-subtle);
    position: sticky;
    bottom: 0;
    background: var(--surface);
  }
  .change label {
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .change textarea {
    width: 100%;
    box-sizing: border-box;
    padding: var(--space-3);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--text-base);
    resize: vertical;
  }
  .change-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .change-row .hint {
    flex: 1;
    font-size: var(--text-xs);
  }
  .change .error {
    margin: 0;
  }
  .later {
    margin-left: auto;
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }
  .later + .go {
    margin-left: 0;
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
