<script lang="ts">
  // One vibe project on one desktop, in the app (docs/plans/vibe-mode-
  // redesign unit 8; screens/M-Plan, M-Progress, M-Result, M-Versions,
  // M-Paused). The same project state and actions as the desk's workspace
  // (EasyProjectStore) and the same parts (EasySteps, EasyTeam,
  // EasyBlueprint, EasyMaking, EasyChecks, EasyState, EasyVersions), every
  // request sent to the desk that owns the project. What differs at phone
  // width lives here: one pane at a time behind tabs, and one bar at the
  // bottom with the single thing to press. The running app stays on the
  // desk (unit 7), so its tab says where it is.
  import { onDestroy, onMount } from 'svelte'
  import { t } from '../i18n'
  import { remoteEasyApi } from '../lib/mobile/easyRemote'
  import { targetOf, type PairedDesktop } from '../lib/mobile/desktops'
  import type { DesktopEvents } from '../lib/mobile/desktopEvents'
  import { defaultPhoneTab, phoneTabs, type PhoneTab } from '../lib/mobile/projects'
  import { easyCanChange } from '../lib/easy'
  import { EasyProjectStore } from '../lib/stores/easyProjectStore.svelte'
  import EasyBlueprint from '../components/easy/EasyBlueprint.svelte'
  import EasyChecks from '../components/easy/EasyChecks.svelte'
  import EasyMaking from '../components/easy/EasyMaking.svelte'
  import EasyState from '../components/easy/EasyState.svelte'
  import EasySteps from '../components/easy/EasySteps.svelte'
  import EasyTeam from '../components/easy/EasyTeam.svelte'
  import EasyVersions from '../components/easy/EasyVersions.svelte'

  interface Props {
    desktop: PairedDesktop
    projectId: string
    // The desktops' event streams: an answer given at the desk shows here
    // at once. Without them the screen reads on its timer.
    events?: DesktopEvents
    // A project that could not be set up was started again as this one.
    onOpen: (projectId: string) => void
    onClose: () => void
  }

  let { desktop, projectId, events, onOpen, onClose }: Props = $props()

  // MobileApp remounts this screen ({#key}) for another project or desk.
  // svelte-ignore state_referenced_locally
  const api = remoteEasyApi(targetOf(desktop), { timeoutMs: 15_000 })
  // svelte-ignore state_referenced_locally
  const deskID = desktop.id
  // The stream names the run that changed; a project's run changes with
  // each "change it like this", so any change on its desk reads it again.
  // svelte-ignore state_referenced_locally
  const changes = events ? { listen: (onChange: () => void) => events.listen((c) => c.desktopID === deskID && onChange()) } : undefined
  // svelte-ignore state_referenced_locally
  const store = new EasyProjectStore({ api, projectId, changes })
  onMount(() => store.start())
  onDestroy(() => store.stop())

  const view = $derived(store.view)
  const tabs = $derived(view ? phoneTabs(view) : [])
  let picked = $state<PhoneTab | null>(null)
  // The pane shown: the one the person picked while it is still offered,
  // else the phase's own.
  const tab = $derived<PhoneTab | null>(view ? (picked && tabs.includes(picked) ? picked : defaultPhoneTab(view)) : null)
  // A new phase opens on its own pane.
  let lastPhase = ''
  $effect(() => {
    const phase = view?.phase ?? ''
    if (phase !== lastPhase) {
      lastPhase = phase
      picked = null
    }
  })

  function tabLabel(name: PhoneTab): string {
    if (name === 'team') return `${$t.phone.project.tabs.team}${view && view.log.length > 1 ? ` ${view.log.length - 1}` : ''}`
    return $t.phone.project.tabs[name]
  }

  // "Say what to change" on the blueprint opens a box in the bar.
  let sayOpen = $state(false)
  let say = $state('')
  const sayError = $derived(store.sayError === 'required' ? $t.easy.workspace.say.required : store.sayError === 'failed' ? $t.easy.project.actionFailed : '')
  async function sayToTeam(event: SubmitEvent) {
    event.preventDefault()
    if (await store.sayToTeam(say)) {
      say = ''
      sayOpen = false
    }
  }

  let stuckNote = $state('')
  async function retryStuck() {
    if (await store.retryStuck(stuckNote)) stuckNote = ''
  }

  async function recreate() {
    const id = await store.recreate()
    if (id) onOpen(id)
  }

  let changeRequest = $state('')
  const changeError = $derived(store.changeError === null ? '' : $t.easy.errors[store.changeError] || $t.easy.errors.unknown)
  async function submitChange(event: SubmitEvent) {
    event.preventDefault()
    if (await store.change(changeRequest)) changeRequest = ''
  }

  let currentNumber = $state<number | undefined>(undefined)

  function barHint(): string {
    if (!view) return ''
    switch (view.phase) {
      case 'questions':
        return $t.easy.workspace.bar.questions
      case 'approve':
        return $t.easy.workspace.bar.plan
      case 'limit':
        return $t.easy.states.limit.early
      case 'stuck':
        return view.retry ? $t.easy.states.stuck.bar : $t.easy.states.stuck.noRetry
      case 'failed':
        return $t.easy.states.failed.hint
      case 'finished':
        return $t.easy.result.publishLater
      case 'building':
      case 'planning':
      case 'creating':
        return $t.phone.project.nothingToDo
    }
    return ''
  }
</script>

<div class="mp" data-testid="phone-project" data-desktop={desktop.name}>
  <header class="head">
    <button type="button" class="back" onclick={onClose} data-testid="phone-project-back">{$t.phone.project.back}</button>
    {#if store.project && view}
      <div class="title-row">
        <h1>{store.project.name}</h1>
        <span class="phase phase-{view.phase}" data-testid="easy-phase" data-phase={view.phase}>{$t.easy.status[view.phase]}</span>
      </div>
      <span class="desk">{$t.phone.project.on(desktop.name)}</span>
      <EasySteps steps={view.steps} />
    {/if}
  </header>

  {#if store.notFound}
    <p class="muted pad">{$t.easy.project.notFound}</p>
  {:else if !store.project || !view}
    {#if store.loadFailed}<p class="muted pad">{$t.easy.project.loadFailed}</p>{/if}
  {:else}
    {#if tabs.length > 1}
      <div class="tabs" role="tablist">
        {#each tabs as name (name)}
          <button
            type="button"
            role="tab"
            aria-selected={tab === name}
            class:active={tab === name}
            data-testid="phone-project-tab"
            data-tab={name}
            onclick={() => (picked = name)}
          >
            {name === 'versions' && currentNumber ? $t.easy.versions.label(currentNumber) : tabLabel(name)}
          </button>
        {/each}
      </div>
    {/if}

    <main class="pane" data-testid="phone-project-pane" data-tab={tab}>
      {#if store.loadFailed}<p class="muted">{$t.easy.project.loadFailed}</p>{/if}
      {#if view.change && view.phase !== 'finished'}
        <p class="asked"><span>{$t.easy.project.asked}</span> {view.change}</p>
      {/if}

      {#if tab === 'team'}
        {#if view.phase === 'planning' || view.phase === 'questions'}
          <p class="say">{$t.easy.project[view.phase]}</p>
        {/if}
        <EasyTeam {view} answering={store.answering} onAnswer={(cardId, choice) => store.answer(cardId, choice)} />
        {#if store.actionFailed && view.phase === 'questions'}<p class="error" role="alert">{$t.easy.project.actionFailed}</p>{/if}
      {:else if tab === 'blueprint' && view.blueprint}
        <div data-testid={view.phase === 'approve' ? 'easy-your-turn' : undefined}>
          <EasyBlueprint blueprint={view.blueprint} />
        </div>
      {:else if tab === 'progress'}
        {#if view.phase === 'building' && view.step === 'make'}
          <EasyMaking {view} />
        {:else if view.phase === 'building'}
          <EasyChecks {view} />
        {:else}
          <EasyState {view} bind:note={stuckNote} busy={store.retrying} />
        {/if}
      {:else if tab === 'app'}
        <section class="app-here" data-testid="phone-app-elsewhere">
          {#if currentNumber}<span class="version">{$t.easy.result.version(currentNumber)}</span>{/if}
          <p>{$t.phone.project.appOnDesk(desktop.name)}</p>
          <p class="muted">{$t.phone.project.appLater}</p>
        </section>
      {:else if tab === 'checks'}
        {#if view.checks.length}
          <EasyChecks {view} />
        {:else}
          <p class="muted">{$t.phone.project.noChecks}</p>
        {/if}
      {/if}

      <!-- Versions stay mounted on the result so the tab names the current
           one; only the Versions tab shows them. -->
      {#if view.phase === 'finished' && easyCanChange(view)}
        <div class="versions" hidden={tab !== 'versions'}>
          <p class="muted">{$t.easy.versions.hint}</p>
          <EasyVersions projectId={store.project.id} {api} full onRestored={() => void store.refresh()} onCurrent={(n) => (currentNumber = n)} />
        </div>
      {/if}
    </main>

    <footer class="bar" data-testid="easy-bar">
      {#if view.phase === 'approve' && sayOpen}
        <form class="say-form" onsubmit={sayToTeam} data-testid="easy-say">
          <label class="sr-only" for="phone-say-input">{$t.easy.workspace.say.label}</label>
          <textarea id="phone-say-input" rows="2" data-testid="easy-say-input" placeholder={$t.easy.workspace.say.placeholder} bind:value={say} disabled={store.saying}></textarea>
          <div class="row">
            <button type="button" class="btn btn-ghost" onclick={() => (sayOpen = false)} disabled={store.saying}>{$t.phone.projects.cancel}</button>
            <button type="submit" class="btn btn-secondary" data-testid="easy-say-send" disabled={store.saying}>
              {store.saying ? $t.easy.workspace.say.sending : $t.easy.workspace.say.send}
            </button>
          </div>
          {#if sayError}<p class="error" role="alert" data-testid="easy-say-error">{sayError}</p>{/if}
        </form>
      {:else if view.phase === 'finished' && easyCanChange(view)}
        <form class="say-form" onsubmit={submitChange} data-testid="easy-change">
          <label class="sr-only" for="phone-change-input">{$t.easy.change.title}</label>
          <textarea id="phone-change-input" rows="2" data-testid="easy-change-input" placeholder={$t.easy.change.placeholder} bind:value={changeRequest} disabled={store.changing}></textarea>
          <button type="submit" class="btn btn-secondary" data-testid="easy-change-submit" disabled={store.changing}>
            {store.changing ? $t.easy.change.submitting : $t.easy.change.title}
          </button>
          {#if changeError}<p class="error" role="alert" data-testid="easy-change-error">{changeError}</p>{/if}
        </form>
      {/if}

      {#if store.actionFailed && view.phase !== 'questions'}<p class="error" role="alert">{$t.easy.project.actionFailed}</p>{/if}

      {#if view.phase === 'approve' && !sayOpen}
        <div class="row">
          <button type="button" class="btn btn-secondary" data-testid="phone-say-open" onclick={() => (sayOpen = true)}>{$t.phone.project.sayOpen}</button>
          <button type="button" class="btn btn-primary go" data-testid="easy-approve" disabled={store.approving} onclick={() => store.approve()}>
            {store.approving ? $t.easy.project.approving : $t.easy.project.approve}
          </button>
        </div>
      {:else if view.phase === 'limit' && view.limit}
        <button type="button" class="btn btn-primary go" data-testid="easy-continue" disabled={store.continuing} onclick={() => store.continueAfterLimit()}>
          {store.continuing ? $t.easy.project.continuing : $t.easy.project.continue}
        </button>
      {:else if view.phase === 'stuck' && view.retry}
        <button type="button" class="btn btn-primary go" data-testid="easy-retry" disabled={store.retrying} onclick={retryStuck}>
          {store.retrying ? $t.easy.states.stuck.retrying : $t.easy.states.stuck.retry}
        </button>
      {:else if view.phase === 'failed'}
        <button type="button" class="btn btn-primary go" data-testid="easy-recreate" disabled={store.recreating} onclick={recreate}>
          {store.recreating ? $t.easy.states.failed.retrying : $t.easy.states.failed.retry}
        </button>
      {:else if view.phase === 'finished'}
        <button type="button" class="btn btn-primary go" data-testid="easy-publish" disabled aria-describedby="phone-publish-later">{$t.easy.result.publish}</button>
      {/if}
      <span class="hint" id="phone-publish-later" data-testid="easy-message">{barHint()}</span>
    </footer>
  {/if}
</div>

<style>
  .mp {
    min-height: 100dvh;
    display: flex;
    flex-direction: column;
    padding-top: env(safe-area-inset-top);
    max-width: 640px;
    margin: 0 auto;
  }
  .head {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
  }
  .back {
    align-self: flex-start;
    min-height: 36px;
    border: 0;
    background: none;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--text-sm);
    padding: 0;
  }
  .title-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  h1 {
    margin: 0;
    flex: 1;
    min-width: 0;
    font-family: var(--font-display);
    font-size: var(--text-lg);
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .desk {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }
  /* The four steps fill the phone's width. */
  .head :global(.steps) {
    width: 100%;
  }
  .phase {
    flex: none;
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
  .phase-planning,
  .phase-creating {
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
  .tabs {
    display: flex;
    gap: var(--space-1);
    margin: var(--space-3) var(--space-4) 0;
    padding: 3px;
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    overflow-x: auto;
  }
  .tabs button {
    flex: 1 0 auto;
    min-height: 36px;
    padding: 0 var(--space-3);
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--text-sm);
    white-space: nowrap;
  }
  .tabs button.active {
    background: var(--surface-elevated);
    color: var(--text-primary);
  }
  .pane {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    padding: var(--space-4);
  }
  .pad {
    padding: var(--space-4);
  }
  .muted {
    color: var(--text-secondary);
    margin: 0;
  }
  .say {
    margin: 0;
    color: var(--text-secondary);
  }
  .error {
    color: var(--error);
    margin: 0;
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
  .app-here {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    border: 1px dashed var(--border-default);
    border-radius: var(--radius-lg);
    padding: var(--space-5);
  }
  .app-here p {
    margin: 0;
  }
  .version {
    align-self: flex-start;
    font-size: var(--text-xs);
    padding: 2px var(--space-2);
    border-radius: 999px;
    background: var(--surface-elevated);
    color: var(--text-secondary);
  }
  .versions {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  /* The versions as one column: a phone has no room for the desk's strip
     of three. */
  .versions :global(.strip) {
    display: flex;
    flex-direction: column;
  }
  .bar {
    position: sticky;
    bottom: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4) calc(env(safe-area-inset-bottom) + var(--space-3));
    border-top: 1px solid var(--border-subtle);
    background: var(--surface);
  }
  .row {
    display: flex;
    gap: var(--space-2);
  }
  .row .btn {
    flex: 1;
  }
  .go {
    width: 100%;
    min-height: 48px;
    font-size: var(--text-md);
  }
  .row .go {
    width: auto;
  }
  .bar .btn {
    min-height: 44px;
  }
  .hint {
    font-size: var(--text-xs);
    color: var(--text-secondary);
    text-align: center;
  }
  .say-form {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  textarea {
    width: 100%;
    box-sizing: border-box;
    padding: var(--space-3);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-primary);
    font: inherit;
    font-size: 16px;
    resize: vertical;
  }
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
  }
  /* The team column at phone width: the log is the pane itself. */
  .pane :global(.team) {
    border: 0;
  }
</style>
