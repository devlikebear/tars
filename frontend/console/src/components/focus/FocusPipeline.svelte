<script lang="ts">
  // The focus pipeline screen (ADR §3): header, stepper, one card at a time,
  // the stage instruction input, and one progress line while a turn runs.
  // Everything else of the workbench — dock, tool cards, sidebar, status
  // details — stays in Advanced ("View in Advanced").
  import { onDestroy, untrack } from 'svelte'
  import { t } from '../../i18n'
  import * as api from '../../lib/api'
  import { pipelinePhase, promoteDraft, stepperItems, type QAEntry } from '../../lib/focus'
  import { shortCwdLabel } from '../../lib/sessionLabels'
  import { FocusStore } from '../../lib/stores/focusStore.svelte'
  import type { FocusCard, FocusGateAction, FocusPlan, FocusStageId } from '../../lib/types'
  import FocusDeck from './FocusDeck.svelte'
  import FocusGraph from './FocusGraph.svelte'
  import FocusStepper from './FocusStepper.svelte'

  interface Props {
    sessionId: string
    onNavigate: (path: string) => void
  }

  let { sessionId, onNavigate }: Props = $props()

  const store = new FocusStore({
    getPipeline: api.getFocusPipeline,
    getSession: api.getSession,
    getHistory: api.getSessionHistory,
    listCheckpoints: api.listCheckpoints,
    getCheckpointDiff: api.getCheckpointDiff,
    streamChat: api.streamChat,
    attachChatStream: api.attachChatStream,
    gate: api.focusGate,
    card: api.focusCard,
    advance: api.focusAdvance,
    stop: api.focusStop,
    ask: api.askFocusQuestion,
    activity: api.getChatActivity,
  }, typeof localStorage === 'undefined' ? null : localStorage)

  // A turn started elsewhere (another tab, Advanced) is picked up within a
  // few seconds, and a prompt that waited for one goes out after it.
  const pollTimer = setInterval(() => void store.poll(), 3000)

  $effect(() => {
    const id = sessionId
    untrack(() => void store.load(id))
  })

  onDestroy(() => {
    clearInterval(pollTimer)
    store.dispose()
  })

  let instruction = $state('')
  let instructionBox = $state<HTMLTextAreaElement | null>(null)
  let menuOpen = $state(false)
  // The pipeline graph under the stage bar (ADR §9 P5).
  let showGraph = $state(false)

  let pipeline = $derived(store.pipeline)
  let phase = $derived(pipeline ? pipelinePhase(pipeline) : 'active')
  let steps = $derived(pipeline ? stepperItems(pipeline, $t.focus.stages) : [])
  let viewing = $derived(store.stage)
  let openGate = $derived(pipeline?.open_gate ?? '')
  let canMarkDone = $derived(!!pipeline && phase === 'active' && !openGate && pipeline.current !== 'plan' && !store.running && !store.busy)
  let worktree = $derived(store.session?.worktree ?? null)
  let cwd = $derived(store.session?.worktree?.source_dir || store.session?.current_dir || '')
  let baseDirs = $derived([store.session?.worktree?.path, store.session?.current_dir].filter((d): d is string => !!d))
  let progress = $derived(store.running ? store.progress(pipeline?.current, $t.focus.progress) : '')
  let noticeText = $derived(store.notice === 'stale' ? $t.focus.screen.stale : store.notice ? $t.focus.screen.warning(store.notice) : '')

  function selectStage(stage: FocusStageId) {
    store.showStage(stage)
  }

  function onGate(gate: string, action: FocusGateAction, note?: string, edits?: FocusPlan) {
    void store.gate(gate, action, note, edits)
  }

  function onDecide(card: FocusCard, decision: string) {
    void store.markCard(card.id, 'decided', decision)
  }

  // Promote to instruction (ADR §8): the answer becomes a draft in the stage
  // input; the developer edits it and sends it.
  function promote(card: FocusCard, entry: QAEntry) {
    const draft = promoteDraft(card.title, entry, $t.focus.qa)
    instruction = instruction.trim() ? `${instruction.trimEnd()}\n\n${draft}` : draft
    requestAnimationFrame(() => {
      instructionBox?.focus()
      instructionBox?.setSelectionRange(instruction.length, instruction.length)
    })
  }

  async function sendInstruction() {
    const text = instruction.trim()
    if (!text || store.running) return
    instruction = ''
    await store.send(text)
  }

  function onInstructionKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
      event.preventDefault()
      void sendInstruction()
    }
  }

  async function stop() {
    menuOpen = false
    if (!confirm($t.focus.screen.stopConfirm)) return
    await store.stop()
  }

  $effect(() => {
    if (!menuOpen) return
    const close = () => { menuOpen = false }
    const timer = setTimeout(() => document.addEventListener('click', close), 0)
    return () => {
      clearTimeout(timer)
      document.removeEventListener('click', close)
    }
  })
</script>

<div class="focus-screen" data-testid="focus-pipeline">
  <header class="focus-header">
    <button type="button" class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/focus')}>← {$t.focus.screen.back}</button>
    <div class="focus-title">
      <h2 data-content>{store.session?.title || pipeline?.goal || sessionId}</h2>
      {#if cwd || worktree}
        <span class="where">
          <!-- One line; a long path loses its start (rtl ellipsis), the full
               path is in the title. <bdi> keeps the path itself left-to-right. -->
          {#if cwd}<span class="mono cwd" title={cwd} data-testid="focus-cwd" data-content><bdi>{shortCwdLabel(cwd)}</bdi></span>{/if}
          {#if worktree}
            <!-- An isolated pipeline works on its own branch, not the checkout above. -->
            <span class="badge badge-accent mono worktree-chip" title={worktree.path} data-testid="focus-worktree-chip"><span aria-hidden="true">⑂</span> <span data-content>{worktree.branch}</span></span>
          {/if}
        </span>
      {/if}
    </div>
    <div class="focus-header-actions">
      <button
        type="button"
        class="btn btn-secondary btn-sm"
        title={$t.focus.screen.viewAdvancedTitle}
        data-testid="focus-view-advanced"
        onclick={() => onNavigate(`/console/chat/${encodeURIComponent(sessionId)}`)}
      >{$t.focus.screen.viewAdvanced}</button>
      <button type="button" class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console')} data-testid="focus-open-board">{$t.focus.home.advanced}</button>
      {#if pipeline && phase === 'active'}
        <div class="menu">
          <button type="button" class="btn btn-ghost btn-sm" aria-haspopup="menu" aria-expanded={menuOpen} aria-label={$t.focus.screen.more} title={$t.focus.screen.more} onclick={() => { menuOpen = !menuOpen }}>⋯</button>
          {#if menuOpen}
            <div class="menu-popover" role="menu">
              <button type="button" role="menuitem" class="danger" disabled={store.busy} onclick={() => void stop()}>{$t.focus.screen.stop}</button>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </header>

  {#if store.error === 'notFound'}
    <p class="banner">{$t.focus.screen.notFound}</p>
  {:else if store.error}
    <p class="banner error">{$t.focus.screen.loadFailed(store.error)}</p>
  {:else if pipeline}
    <div class="stage-bar">
      <FocusStepper items={steps} selected={viewing} onSelect={selectStage} />
      <button
        type="button"
        class="btn btn-ghost btn-sm"
        aria-pressed={showGraph}
        title={$t.focus.graph.openTitle}
        onclick={() => { showGraph = !showGraph }}
        data-testid="focus-graph-toggle"
      >{showGraph ? $t.focus.graph.close : $t.focus.graph.open}</button>
      {#if canMarkDone && viewing === pipeline.current}
        <button type="button" class="btn btn-ghost btn-sm" title={$t.focus.screen.markDoneTitle} onclick={() => void store.advance()} data-testid="focus-mark-done">{$t.focus.screen.markDone}</button>
      {/if}
    </div>

    {#if showGraph}<FocusGraph {pipeline} />{/if}

    {#if store.viewStage && store.viewStage !== pipeline.current}
      <p class="history-line">
        <span class="label">{$t.focus.screen.history($t.focus.stages[store.viewStage])}</span>
        <button type="button" class="btn btn-ghost btn-sm" onclick={() => store.showStage(null)}>{$t.focus.screen.backToCurrent}</button>
      </p>
    {/if}

    {#if phase === 'finished'}
      <p class="banner done">{$t.focus.screen.finished}</p>
    {:else if phase === 'stopped'}
      <p class="banner">{$t.focus.screen.stopped}</p>
    {/if}
    {#if noticeText}<p class="banner">{noticeText}</p>{/if}
    {#if store.actionError}<p class="banner error">{$t.focus.screen.actionFailed(store.actionError)}</p>{/if}

    <!-- A running turn shows one line above the deck; the cards stay. -->
    {#if progress}
      <p class="progress-line mono" role="status" data-testid="focus-progress"><span class="pulse" aria-hidden="true"></span>{progress}</p>
      {#if store.nextPrompt}
        <!-- The turn the server sent for the pipeline: shown, not editable. -->
        <p class="next-line" data-testid="focus-next-prompt"><span class="label">{$t.focus.progress.next}</span> <span data-content>{store.nextPrompt}</span></p>
      {/if}
    {/if}

    <FocusDeck
      cards={store.deck}
      history={store.history}
      {openGate}
      busy={store.busy}
      running={store.running}
      {baseDirs}
      {onGate}
      {onDecide}
      onSeen={(card) => void store.markSeen(card)}
      onAcknowledgeRest={(cards) => void store.acknowledgeRest(cards)}
      qaThread={(id) => store.qaThread(id)}
      qaAnswering={store.qaPending?.cardId ?? null}
      qaError={store.qaError}
      onAsk={(id, question) => store.ask(id, question)}
      onPromote={promote}
    />


    <form class="instruction" onsubmit={(e) => { e.preventDefault(); void sendInstruction() }}>
      <label class="label" for="focus-instruction">{$t.focus.screen.instruction}</label>
      <div class="instruction-row">
        <textarea
          id="focus-instruction"
          bind:this={instructionBox}
          rows="2"
          bind:value={instruction}
          placeholder={$t.focus.screen.instructionPlaceholder}
          onkeydown={onInstructionKeydown}
          disabled={store.running}
          data-testid="focus-instruction"
        ></textarea>
        <button type="submit" class="btn btn-secondary" disabled={store.running || !instruction.trim()}>{$t.focus.screen.send}</button>
      </div>
    </form>
  {/if}
</div>

<style>
  .focus-screen {
    box-sizing: border-box;
    width: 100%;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 880px;
    margin: 0 auto;
    padding: var(--space-6) var(--space-6) var(--space-10);
  }

  /* At narrow widths the button group wraps below the title; the title and
     path shrink, the buttons and the branch chip never do. */
  .focus-header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
  }

  .focus-header > .btn {
    flex: 0 0 auto;
  }

  .focus-title {
    flex: 1 1 240px;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .focus-title h2 {
    margin: 0;
    font-size: var(--text-lg);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .where {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .worktree-chip {
    flex: 0 0 auto;
    white-space: nowrap;
    font-size: var(--text-xs);
  }

  .cwd {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
    direction: rtl;
    text-align: left;
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .focus-header-actions {
    display: flex;
    flex: 0 0 auto;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    margin-left: auto;
  }

  .menu {
    position: relative;
  }

  .menu-popover {
    position: absolute;
    right: 0;
    top: calc(100% + var(--space-1));
    z-index: 20;
    min-width: 180px;
    padding: var(--space-1);
    background: var(--surface-elevated);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
  }

  .menu-popover button {
    width: 100%;
    padding: var(--space-2);
    background: transparent;
    border: 0;
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    text-align: left;
    font-size: var(--text-sm);
    cursor: pointer;
  }

  .menu-popover button:hover {
    background: var(--surface-hover);
  }

  .menu-popover .danger {
    color: var(--error);
  }

  .stage-bar {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-wrap: wrap;
  }

  .history-line {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0;
  }

  .banner {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .banner.error {
    background: var(--error-muted);
    color: var(--error);
  }

  .banner.done {
    background: var(--primary-muted);
    color: var(--primary-text);
  }

  .progress-line {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0;
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }

  .next-line {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .pulse {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--primary);
    animation: focus-pulse 1.2s ease-in-out infinite;
  }

  @keyframes focus-pulse {
    0%, 100% { opacity: 0.35; }
    50% { opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .pulse {
      animation: none;
    }
  }

  .instruction {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .instruction-row {
    display: flex;
    gap: var(--space-2);
    align-items: flex-end;
  }

  .instruction textarea {
    flex: 1;
    padding: var(--space-2) var(--space-3);
    background: var(--surface-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-base);
    resize: vertical;
  }

  .instruction textarea:focus {
    outline: none;
    border-color: var(--primary);
  }
</style>
