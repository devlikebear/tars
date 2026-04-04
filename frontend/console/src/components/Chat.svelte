<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { getEventsHistory, getHeartbeatStatus, listProjects, streamEvents } from '../lib/api'
  import type { HeartbeatStatus, NotificationMessage, Project, Session } from '../lib/types'
  import SessionSidebar from './SessionSidebar.svelte'
  import ChatPanel from './ChatPanel.svelte'

  interface Props {
    sessionId?: string
    onNavigate: (path: string) => void
    initialPrompt?: string
  }

  let { sessionId, onNavigate, initialPrompt }: Props = $props()

  // Dashboard mini state
  let projects: Project[] = $state([])
  let heartbeat: HeartbeatStatus | null = $state(null)
  let unreadCount = $state(0)

  // Session selection — synced via $effect from sessionId prop
  let selectedSessionId: string | null = $state(null)
  let chatKey = $state(0) // force ChatPanel re-mount
  // Initialize from prop on first render
  $effect(() => {
    selectedSessionId = sessionId || null
    chatKey++
  })

  let sidebarRef: SessionSidebar | undefined = $state()
  let stopStream: (() => void) | null = null

  function relativeTime(value?: string): string {
    if (!value?.trim()) return 'never'
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
    if (seconds < 60) return `${seconds}s ago`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
    return `${Math.floor(seconds / 86400)}d ago`
  }

  function handleSelectSession(session: Session) {
    selectedSessionId = session.id
    chatKey++
    onNavigate(`/console/chat/${encodeURIComponent(session.id)}`)
  }

  function handleNewSession() {
    selectedSessionId = null
    chatKey++
    onNavigate('/console/chat')
  }

  function handleSessionChange() {
    sidebarRef?.load()
  }


  async function loadDashboard() {
    const [p, h, e] = await Promise.allSettled([
      listProjects(),
      getHeartbeatStatus(),
      getEventsHistory(1),
    ])
    projects = p.status === 'fulfilled' ? p.value : []
    heartbeat = h.status === 'fulfilled' ? h.value : null
    if (e.status === 'fulfilled') {
      unreadCount = e.value.unread_count ?? 0
    }
  }

  onMount(() => {
    void loadDashboard()
    stopStream = streamEvents(
      undefined,
      () => { unreadCount++ },
    )
  })

  onDestroy(() => {
    stopStream?.()
  })
</script>

<div class="chat-page">
  <!-- Mini dashboard pulse -->
  <div class="chat-pulse">
    <div class="pulse-item">
      <span class="pulse-val">{projects.length}</span>
      <span class="pulse-lbl">Projects</span>
    </div>
    <div class="pulse-sep"></div>
    <div class="pulse-item">
      <span class="pulse-val" class:warn={!!heartbeat?.last_error}>
        {heartbeat?.interval || 'off'}
      </span>
      <span class="pulse-lbl">Heartbeat</span>
    </div>
    <div class="pulse-sep"></div>
    <div class="pulse-item">
      <span class="pulse-val">{heartbeat?.last_run_at ? relativeTime(heartbeat.last_run_at) : 'never'}</span>
      <span class="pulse-lbl">Last run</span>
    </div>
    <div class="pulse-sep"></div>
    <div class="pulse-item">
      <span class="pulse-val">{unreadCount}</span>
      <span class="pulse-lbl">Unread</span>
    </div>
  </div>

  <div class="chat-layout">
    <!-- Session sidebar -->
    <aside class="chat-sidebar">
      <SessionSidebar
        bind:this={sidebarRef}
        selectedSessionId={selectedSessionId}
        onSelect={handleSelectSession}
        onNewSession={handleNewSession}
      />
    </aside>

    <!-- Chat area -->
    <main class="chat-main">
      {#key chatKey}
        <ChatPanel
          sessionId={selectedSessionId || undefined}
          {initialPrompt}
          onSessionChange={handleSessionChange}
        />
      {/key}
    </main>
  </div>
</div>

<style>
  .chat-page {
    display: flex;
    flex-direction: column;
    height: calc(100vh - var(--header-height));
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  /* Mini dashboard */
  .chat-pulse {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-2) var(--space-4);
    background: var(--bg-surface);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
  }

  .pulse-item {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .pulse-val {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-primary);
  }
  .pulse-val.warn { color: var(--error); }
  .pulse-lbl {
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }
  .pulse-sep {
    width: 1px;
    height: 16px;
    background: var(--border-subtle);
    flex-shrink: 0;
  }

  /* Layout */
  .chat-layout {
    flex: 1;
    display: grid;
    grid-template-columns: 280px 1fr;
    min-height: 0;
  }

  .chat-sidebar {
    border-right: 1px solid var(--border-subtle);
    background: var(--bg-surface);
    padding: var(--space-3);
    overflow: hidden;
  }

  .chat-main {
    display: flex;
    flex-direction: column;
    min-height: 0;
    padding: var(--space-4);
    overflow: hidden;
  }

  @media (max-width: 768px) {
    .chat-layout {
      grid-template-columns: 1fr;
    }
    .chat-sidebar {
      display: none;
    }
    .chat-pulse {
      flex-wrap: wrap;
      gap: var(--space-2);
    }
    .pulse-sep { display: none; }
  }
</style>
