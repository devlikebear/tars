<script lang="ts">
  // The chat workbench grid: left/right/bottom/fullscreen dock zones around
  // the conversation column (passed in as `children`), the panels that fill
  // them, and the integrated terminal. Layout state lives in the shared dock
  // store so the toolbar and session header can open panels directly.
  import { onMount, tick, type Snippet } from 'svelte'
  import { t } from '../i18n'
  import type { SessionHealthAction } from '../lib/sessionHealth'
  import type { Session } from '../lib/types'
  import type { DockZone } from '../lib/dock/layout'
  import { loadChatComponent } from '../lib/chatComponents'
  import { chatSession } from '../lib/stores/chatSession'
  import { chatDock, type ChatDockPanelID } from '../lib/stores/chatDockStore.svelte'
  import SessionSidebar from './SessionSidebar.svelte'
  import SessionConfigPanel from './SessionConfigPanel.svelte'
  import ContextMonitor from './ContextMonitor.svelte'
  import PromptEditor from './PromptEditor.svelte'
  import PriorContextPanel from './PriorContextPanel.svelte'
  import TasksPanel from './TasksPanel.svelte'
  import GitInspector from './GitInspector.svelte'
  import SkillExtractionPanel from './SkillExtractionPanel.svelte'
  import SessionCronPanel from './SessionCronPanel.svelte'
  import SessionHealthPanel from './SessionHealthPanel.svelte'
  import DockPanelFrame from './DockPanelFrame.svelte'

  interface Props {
    children: Snippet
    onSelectSession: (session: Session) => void
    onNewSession: () => Promise<void>
    onSendMessage: (text: string) => Promise<void>
    onHealthAction: (action: SessionHealthAction) => Promise<void>
  }

  let { children, onSelectSession, onNewSession, onSendMessage, onHealthAction }: Props = $props()

  let selectedSessionId = $derived(chatSession.activeSessionId)
  let selectedSession = $derived(chatSession.activeSession)
  let chatArtifacts = $derived(chatSession.artifacts)
  let chatDraft = $derived(chatSession.draft)
  let chatContextInfo = $derived(chatSession.contextInfo)
  let contextRefreshVersion = $derived(chatSession.contextVersion)
  let sessionHealth = $derived(chatSession.health)
  let sessionHealthLoading = $derived(chatSession.healthLoading)

  let activeLeftPanel = $derived(chatDock.left)
  let activeRightPanel = $derived(chatDock.right)
  let activeBottomPanel = $derived(chatDock.bottom)
  let activeFullscreenPanel = $derived(chatDock.fullscreen)

  // Which zone the terminal panel currently lives in. Tracked separately
  // so the terminal can render from a single, stable parent — moving it
  // between zones via CSS instead of unmounting/remounting the dock-pane
  // wrapper, which would tear down xterm + the WebSocket every time the
  // user dragged the panel (#667).
  let terminalActiveZone = $derived(chatDock.zoneOf('terminal'))

  interface TerminalDockTab {
    id: string
    cwd: string
    label: string
  }
  let terminalDockSessionId = $state('')
  let terminalDockTabs = $state<TerminalDockTab[]>([])
  let terminalDockActiveId = $state<string | null>(null)

  let tasksPanelRef: { load: () => void; openEvidence: () => Promise<void> } | undefined = $state()
  let artifactPanelRef: { refresh: () => void; openArtifactPath: (path: string) => Promise<void> } | undefined = $state()

  type DockSizeZone = 'left' | 'right' | 'bottom'

  function panelTitle(panelID: ChatDockPanelID): string {
    const panels = $t.chat.panels
    const titles: Record<ChatDockPanelID, string> = {
      sessions: panels.sessions,
      artifacts: panels.files,
      config: panels.config,
      context: panels.context,
      prompt: panels.prompt,
      prior: panels.priorFull,
      tasks: panels.tasks,
      git: panels.git,
      skillExtraction: panels.skillsInbox,
      cron: panels.cron,
      health: panels.health,
      terminal: panels.terminal,
    }
    return titles[panelID] ?? panelID
  }

  function panelCloseable(panelID: ChatDockPanelID): boolean {
    return panelID !== 'sessions'
  }

  function openPanel(panelID: ChatDockPanelID) {
    chatDock.open(panelID)
  }

  function closePanel(panelID: ChatDockPanelID) {
    chatDock.close(panelID)
  }

  function dockPanel(panelID: ChatDockPanelID, zone: DockZone) {
    chatDock.move(panelID, zone)
  }

  function showFeedback(message: string) {
    chatSession.notify(message)
  }

  function refreshSessionHealth(): Promise<void> {
    return chatSession.refreshHealth()
  }

  function handleSelectSession(session: Session) {
    onSelectSession(session)
  }

  function handleNewSession() {
    void onNewSession()
  }

  async function handleHealthAction(action: SessionHealthAction) {
    await onHealthAction(action)
  }

  function dockStyle(): string {
    const sizes = chatDock.layout.sizes
    return [
      `--dock-left-size:${activeLeftPanel ? sizes.left : 0}px`,
      `--dock-right-size:${activeRightPanel ? sizes.right : 0}px`,
      `--dock-bottom-size:${activeBottomPanel ? sizes.bottom : 0}px`,
    ].join(';')
  }

  function openIntegratedTerminalDock(target: { cwd: string; label: string }) {
    if (!selectedSessionId) {
      showFeedback($t.chat.panels.dockEmpty)
      return
    }
    if (terminalDockSessionId !== selectedSessionId) {
      terminalDockSessionId = selectedSessionId
      terminalDockTabs = []
      terminalDockActiveId = null
    }
    const existing = terminalDockTabs.find((t) => t.cwd === target.cwd && t.label === target.label)
    if (existing) {
      terminalDockActiveId = existing.id
    } else {
      const id = `${target.cwd}:${target.label}:${Date.now()}:${Math.random().toString(36).slice(2, 6)}`
      terminalDockTabs = [...terminalDockTabs, { id, cwd: target.cwd, label: target.label }]
      terminalDockActiveId = id
    }
    openPanel('terminal')
  }

  function closeTerminalTab(id: string) {
    const idx = terminalDockTabs.findIndex((t) => t.id === id)
    if (idx === -1) return
    const next = terminalDockTabs.filter((t) => t.id !== id)
    terminalDockTabs = next
    if (terminalDockActiveId === id) {
      terminalDockActiveId = next.length === 0
        ? null
        : (next[Math.min(idx, next.length - 1)]?.id ?? null)
    }
    if (next.length === 0) {
      closePanel('terminal')
    }
  }

  function addTerminalTab(cwd: string, label: string) {
    if (!selectedSessionId) {
      showFeedback($t.chat.panels.dockEmpty)
      return
    }
    if (terminalDockSessionId !== selectedSessionId) {
      terminalDockSessionId = selectedSessionId
      terminalDockTabs = []
    }
    const id = `${cwd}:${label}:${Date.now()}:${Math.random().toString(36).slice(2, 6)}`
    terminalDockTabs = [...terminalDockTabs, { id, cwd, label }]
    terminalDockActiveId = id
    openPanel('terminal')
  }

  function startDockResize(zone: DockSizeZone, event: PointerEvent) {
    event.preventDefault()
    const startX = event.clientX
    const startY = event.clientY
    const startSize = chatDock.layout.sizes[zone]
    const move = (next: PointerEvent) => {
      const delta = zone === 'left'
        ? next.clientX - startX
        : zone === 'right'
          ? startX - next.clientX
          : startY - next.clientY
      chatDock.resize(zone, startSize + delta)
    }
    const stop = () => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', stop)
      window.removeEventListener('pointercancel', stop)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', stop)
    window.addEventListener('pointercancel', stop)
  }

  // Called by Chat when a tool finishes, so the matching panel reloads.
  export function notifyToolComplete(toolName: string) {
    const taskTools = ['tasks']
    const fileTools = ['write_file', 'edit_file', 'exec', 'list_dir', 'read_file', 'apply_patch']

    if (taskTools.includes(toolName)) {
      tasksPanelRef?.load()
    }
    if (fileTools.includes(toolName)) {
      artifactPanelRef?.refresh()
    }
  }

  export async function openArtifact(path: string) {
    openPanel('artifacts')
    await tick()
    await artifactPanelRef?.openArtifactPath(path)
  }

  export async function openEvidence() {
    openPanel('tasks')
    await tick()
    await tasksPanelRef?.openEvidence()
  }

  let dockLayoutLoaded = $state(false)

  onMount(() => {
    chatDock.restore(window.localStorage)
    dockLayoutLoaded = true
  })

  $effect(() => {
    void chatDock.layout
    if (!dockLayoutLoaded) return
    chatDock.persist(window.localStorage)
  })
</script>

{#snippet renderDockPanel(panelID: ChatDockPanelID, zone: DockZone)}
  <DockPanelFrame
    title={panelTitle(panelID)}
    {zone}
    closeable={panelCloseable(panelID)}
    onDock={(nextZone) => dockPanel(panelID, nextZone)}
    onClose={() => closePanel(panelID)}
  >
    {#if panelID === 'sessions'}
      <SessionSidebar
        selectedSessionId={selectedSessionId}
        onSelect={handleSelectSession}
        onNewSession={handleNewSession}
      />
    {:else if panelID === 'artifacts'}
      {#await loadChatComponent('artifact-panel')}
        <div class="dock-empty">Loading...</div>
      {:then module}
        {@const ArtifactPanelRoute = module.default}
        <ArtifactPanelRoute
          bind:this={artifactPanelRef}
          artifacts={chatArtifacts}
          sessionId={selectedSessionId || ''}
          onClose={() => closePanel(panelID)}
          onOpenIntegratedTerminal={openIntegratedTerminalDock}
        />
      {:catch}
        <div class="dock-empty">{$t.chat.panels.dockEmpty}</div>
      {/await}
    {:else if panelID === 'config' && selectedSessionId}
      <SessionConfigPanel
        sessionId={selectedSessionId ?? ''}
        onClose={() => closePanel(panelID)}
        onChange={() => {
          chatSession.bumpContextVersion()
          void refreshSessionHealth()
        }}
      />
    {:else if panelID === 'context'}
      <ContextMonitor
        sessionId={selectedSessionId ?? ''}
        contextInfo={chatContextInfo}
        refreshVersion={contextRefreshVersion}
        onClose={() => closePanel(panelID)}
      />
    {:else if panelID === 'prompt'}
      <PromptEditor sessionId={selectedSessionId ?? ''} onClose={() => closePanel(panelID)} />
    {:else if panelID === 'prior'}
      <PriorContextPanel sessionId={selectedSessionId ?? ''} draftQuery={chatDraft} onClose={() => closePanel(panelID)} />
    {:else if panelID === 'tasks' && selectedSessionId}
      <TasksPanel
        bind:this={tasksPanelRef}
        sessionId={selectedSessionId}
        onClose={() => closePanel(panelID)}
        onSendMessage={async (text) => { await onSendMessage(text) }}
      />
    {:else if panelID === 'git' && selectedSessionId}
      <GitInspector sessionId={selectedSessionId} onClose={() => closePanel(panelID)} />
    {:else if panelID === 'skillExtraction' && selectedSessionId}
      <SkillExtractionPanel
        sessionId={selectedSessionId}
        onClose={() => closePanel(panelID)}
        onApproved={(path) => {
          showFeedback(path ? $t.chat.feedback.savedSkillDraft(path) : $t.chat.feedback.savedSkillDraftPlain)
          chatSession.bumpContextVersion()
        }}
      />
    {:else if panelID === 'cron' && selectedSessionId}
      <SessionCronPanel sessionId={selectedSessionId} sessionKind={selectedSession?.kind ?? ''} onClose={() => closePanel(panelID)} />
    {:else if panelID === 'health' && selectedSessionId}
      <SessionHealthPanel
        report={sessionHealth}
        loading={sessionHealthLoading}
        onRefresh={() => { void refreshSessionHealth() }}
        onAction={(action) => { void handleHealthAction(action) }}
      />
    {:else}
      <div class="dock-empty">{$t.chat.panels.dockEmpty}</div>
    {/if}
  </DockPanelFrame>
{/snippet}

<div
  class="chat-layout dock-layout"
  style={dockStyle()}
  class:has-left-panel={!!activeLeftPanel}
  class:has-right-panel={!!activeRightPanel}
  class:has-bottom-panel={!!activeBottomPanel}
>
  {#if activeLeftPanel}
    {#if activeLeftPanel !== 'terminal'}
      <aside class="dock-pane dock-left">
        {@render renderDockPanel(activeLeftPanel, 'left')}
      </aside>
    {/if}
    <button type="button" class="dock-resizer dock-resizer-left" aria-label="Resize left dock" onpointerdown={(event) => startDockResize('left', event)}></button>
  {/if}

  <!-- Conversation column -->
  {@render children()}

  {#if activeRightPanel}
    {#if activeRightPanel !== 'terminal'}
      <aside class="dock-pane dock-right">
        {@render renderDockPanel(activeRightPanel, 'right')}
      </aside>
    {/if}
    <button type="button" class="dock-resizer dock-resizer-right" aria-label="Resize right dock" onpointerdown={(event) => startDockResize('right', event)}></button>
  {/if}

  {#if activeBottomPanel}
    {#if activeBottomPanel !== 'terminal'}
      <section class="dock-pane dock-bottom">
        {@render renderDockPanel(activeBottomPanel, 'bottom')}
      </section>
    {/if}
    <button type="button" class="dock-resizer dock-resizer-bottom" aria-label="Resize bottom dock" onpointerdown={(event) => startDockResize('bottom', event)}></button>
  {/if}

  {#if activeFullscreenPanel && activeFullscreenPanel !== 'terminal'}
    <section class="dock-pane dock-fullscreen">
      {@render renderDockPanel(activeFullscreenPanel, 'fullscreen')}
    </section>
  {/if}

  <!-- Terminal panel rendered from a single, stable parent so its xterm
       instance + WebSocket survive zone changes. data-zone selects the
       grid area / fullscreen positioning via CSS (#667). -->
  {#if terminalActiveZone && terminalDockSessionId && terminalDockTabs.length > 0}
    <section class="dock-pane dock-terminal" data-zone={terminalActiveZone}>
      <DockPanelFrame
        title={panelTitle('terminal')}
        zone={terminalActiveZone}
        closeable={panelCloseable('terminal')}
        onDock={(nextZone) => dockPanel('terminal', nextZone)}
        onClose={() => closePanel('terminal')}
      >
        {#await loadChatComponent('terminal-tabs')}
          <div class="dock-empty">Loading...</div>
        {:then module}
          {@const TerminalTabsRoute = module.default}
          <TerminalTabsRoute
            sessionId={terminalDockSessionId}
            tabs={terminalDockTabs}
            activeId={terminalDockActiveId}
            onActivate={(id: string) => { terminalDockActiveId = id }}
            onCloseTab={closeTerminalTab}
            onAddTab={addTerminalTab}
          />
        {:catch}
          <div class="dock-empty">{$t.chat.panels.dockEmpty}</div>
        {/await}
      </DockPanelFrame>
    </section>
  {/if}
</div>

<style>
  /* Layout */
  .chat-layout {
    flex: 1;
    display: grid;
    grid-template-columns: var(--dock-left-size, 0px) minmax(0, 1fr) var(--dock-right-size, 0px);
    grid-template-rows: minmax(0, 1fr) var(--dock-bottom-size, 0px);
    grid-template-areas:
      "left main right"
      "bottom bottom bottom";
    min-height: 0;
    position: relative;
  }

  .dock-pane {
    background: var(--surface);
    overflow: hidden;
    min-width: 0;
    min-height: 0;
  }

  .dock-left {
    grid-area: left;
    border-right: 1px solid var(--border-subtle);
  }

  .dock-right {
    grid-area: right;
    border-left: 1px solid var(--border-subtle);
  }

  .dock-bottom {
    grid-area: bottom;
    border-top: 1px solid var(--border-subtle);
  }

  .dock-fullscreen {
    position: absolute;
    inset: var(--space-3);
    z-index: 30;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: 0 24px 80px rgba(0, 0, 0, 0.45);
  }

  /* Terminal pane is rendered once and re-positioned via data-zone so the
     xterm instance + PTY survive zone moves. Mirrors the styling of the
     regular dock-left/right/bottom/fullscreen panes per zone. */
  .dock-terminal[data-zone='left'] {
    grid-area: left;
    border-right: 1px solid var(--border-subtle);
  }

  .dock-terminal[data-zone='right'] {
    grid-area: right;
    border-left: 1px solid var(--border-subtle);
  }

  .dock-terminal[data-zone='bottom'] {
    grid-area: bottom;
    border-top: 1px solid var(--border-subtle);
  }

  .dock-terminal[data-zone='fullscreen'] {
    position: absolute;
    inset: var(--space-3);
    z-index: 30;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: 0 24px 80px rgba(0, 0, 0, 0.45);
  }

  .dock-resizer {
    position: absolute;
    z-index: 20;
    padding: 0;
    border: 0;
    background: transparent;
  }

  .dock-resizer:hover,
  .dock-resizer:focus-visible {
    background: color-mix(in srgb, var(--primary) 35%, transparent);
    outline: none;
  }

  .dock-resizer-left {
    top: 0;
    bottom: var(--dock-bottom-size, 0px);
    left: calc(var(--dock-left-size, 0px) - 3px);
    width: 6px;
    cursor: col-resize;
  }

  .dock-resizer-right {
    top: 0;
    right: calc(var(--dock-right-size, 0px) - 3px);
    bottom: var(--dock-bottom-size, 0px);
    width: 6px;
    cursor: col-resize;
  }

  .dock-resizer-bottom {
    right: 0;
    bottom: calc(var(--dock-bottom-size, 0px) - 3px);
    left: 0;
    height: 6px;
    cursor: row-resize;
  }

  .dock-empty {
    padding: var(--space-4);
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }

  @media (max-width: 900px) {
    .chat-layout {
      grid-template-columns: 1fr;
      grid-template-rows: minmax(0, 1fr);
      grid-template-areas: "main";
    }
    .dock-resizer {
      display: none;
    }
    /* On mobile, the grid collapses to a single column; render any active
       dock panel as a fullscreen overlay over the chat area so the user can
       still see/use it (otherwise the panel toggles would do nothing
       visible). */
    .dock-left,
    .dock-right,
    .dock-bottom,
    .dock-fullscreen,
    .dock-terminal {
      position: absolute;
      inset: var(--space-2);
      z-index: 30;
      border: 1px solid var(--border-strong);
      border-radius: var(--radius-lg);
      box-shadow: 0 24px 80px rgba(0, 0, 0, 0.45);
    }
  }
</style>
