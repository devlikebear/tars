<script lang="ts">
  import { getEventsHistory, markEventsRead } from '../lib/api'
  import type { NotificationMessage } from '../lib/types'

  interface Props {
    serverHealth?: string
    unreadCount?: number
    onUnreadChange?: (count: number) => void
  }

  let {
    serverHealth = 'ok',
    unreadCount = 0,
    onUnreadChange,
  }: Props = $props()

  let panelOpen = $state(false)
  let notifications: NotificationMessage[] = $state([])
  let panelLoading = $state(false)
  let lastId = $state(0)
  let readCursor = $state(0)
  let notifFilter: 'all' | 'unread' | 'read' = $state('all')

  let filteredNotifs = $derived.by(() => {
    // Sort newest first
    const sorted = [...notifications].sort((a, b) => {
      const ta = new Date(a.timestamp).getTime()
      const tb = new Date(b.timestamp).getTime()
      return tb - ta
    })
    if (notifFilter === 'unread') return sorted.filter((n) => (n.id ?? 0) > readCursor)
    if (notifFilter === 'read') return sorted.filter((n) => (n.id ?? 0) <= readCursor)
    return sorted
  })

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    const now = new Date()
    const diff = now.getTime() - date.getTime()
    if (diff < 60_000) return 'just now'
    if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`
    if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`
    return new Intl.DateTimeFormat('en', { dateStyle: 'short', timeStyle: 'short' }).format(date)
  }

  function severityClass(severity: string): string {
    switch (severity) {
      case 'error': return 'notif-error'
      case 'warn': case 'warning': return 'notif-warn'
      case 'success': return 'notif-success'
      default: return 'notif-info'
    }
  }

  function categoryIcon(category: string): string {
    switch (category) {
      case 'cron': return '\u23f0'
      case 'ops': return '\u2699'
      case 'usage': return '\u2261'
      case 'gateway': return '\u29bf'
      case 'chat': return '\u2709'
      default: return '\u2022'
    }
  }

  async function togglePanel() {
    panelOpen = !panelOpen
    if (panelOpen) {
      panelLoading = true
      try {
        const history = await getEventsHistory(50)
        notifications = history.items ?? []
        lastId = history.last_id ?? 0
        readCursor = history.read_cursor ?? 0
      } catch {
        notifications = []
      } finally {
        panelLoading = false
      }
    }
  }

  async function handleMarkAllRead() {
    if (lastId <= 0) return
    try {
      const result = await markEventsRead(lastId)
      readCursor = lastId
      unreadCount = result.unread_count
      onUnreadChange?.(result.unread_count)
    } catch {
      // ignore
    }
  }

  function handleClickOutside(e: MouseEvent) {
    const target = e.target as HTMLElement
    if (!target.closest('.header-notif-wrapper')) {
      panelOpen = false
    }
  }
</script>

<svelte:document onclick={handleClickOutside} />

<header class="header">
  <div class="header-left">
    <h1 class="header-title">Console</h1>
  </div>

  <div class="header-right">
    <div class="header-indicator" class:healthy={serverHealth === 'ok'}>
      <span class="header-dot"></span>
      <span class="header-indicator-label">{serverHealth === 'ok' ? 'Connected' : 'Disconnected'}</span>
    </div>

    <!-- Notification badge + panel -->
    <div class="header-notif-wrapper">
      <button class="header-badge-btn" class:has-unread={unreadCount > 0} onclick={togglePanel} title="Notifications">
        <span class="badge-icon">{'\u266a'}</span>
        {#if unreadCount > 0}
          <span class="badge-count">{unreadCount}</span>
        {/if}
      </button>

      {#if panelOpen}
        <div class="notif-panel">
          <div class="notif-panel-header">
            <span class="notif-panel-title">Notifications</span>
            <div class="notif-panel-actions">
              {#if unreadCount > 0}
                <button class="btn btn-ghost btn-sm" onclick={handleMarkAllRead}>Mark all read</button>
              {/if}
            </div>
          </div>

          <div class="notif-filter-bar">
            <button class="notif-filter-tab" class:active={notifFilter === 'all'} onclick={() => { notifFilter = 'all' }}>All</button>
            <button class="notif-filter-tab" class:active={notifFilter === 'unread'} onclick={() => { notifFilter = 'unread' }}>Unread</button>
            <button class="notif-filter-tab" class:active={notifFilter === 'read'} onclick={() => { notifFilter = 'read' }}>Read</button>
          </div>

          <div class="notif-panel-body">
            {#if panelLoading}
              <div class="notif-empty">Loading...</div>
            {:else if filteredNotifs.length === 0}
              <div class="notif-empty">{notifFilter === 'unread' ? 'No unread notifications' : notifFilter === 'read' ? 'No read notifications' : 'No notifications'}</div>
            {:else}
              {#each filteredNotifs as item}
                <div class="notif-item {severityClass(item.severity)}" class:notif-unread={(item.id ?? 0) > readCursor}>
                  <div class="notif-item-top">
                    <span class="notif-cat-icon">{categoryIcon(item.category)}</span>
                    <strong class="notif-title">{item.title}</strong>
                    <span class="notif-time">{fmt(item.timestamp)}</span>
                  </div>
                  <p class="notif-message">{item.message}</p>
                  <div class="notif-meta">
                    <span class="badge badge-default">{item.category}</span>
                  </div>
                </div>
              {/each}
            {/if}
          </div>
        </div>
      {/if}
    </div>
  </div>
</header>

<style>
  .header {
    position: sticky;
    top: 0;
    z-index: 30;
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: var(--header-height);
    padding: 0 var(--space-6);
    background: var(--bg-base);
    border-bottom: 1px solid var(--border-subtle);
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: var(--space-4);
  }

  .header-title {
    font-size: var(--text-md);
    font-weight: 500;
    color: var(--text-secondary);
  }

  .header-right {
    display: flex;
    align-items: center;
    gap: var(--space-5);
  }

  .header-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .header-meta-value {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .header-indicator {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .header-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--error);
    flex-shrink: 0;
  }
  .header-indicator.healthy .header-dot {
    background: var(--success);
  }

  .header-indicator-label {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  /* ── Notification badge button ──────────── */
  .header-notif-wrapper {
    position: relative;
  }

  .header-badge-btn {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--text-tertiary);
    font-size: var(--text-md);
    cursor: pointer;
    transition: all var(--duration-fast) var(--ease-out);
  }
  .header-badge-btn:hover {
    background: var(--bg-elevated);
    border-color: var(--border-default);
    color: var(--text-primary);
  }
  .header-badge-btn.has-unread {
    border-color: var(--accent);
  }

  .badge-icon {
    line-height: 1;
  }

  .badge-count {
    position: absolute;
    top: -5px;
    right: -5px;
    display: flex;
    align-items: center;
    justify-content: center;
    min-width: 16px;
    height: 16px;
    padding: 0 4px;
    border-radius: 8px;
    background: var(--accent);
    color: #fff;
    font-family: var(--font-display);
    font-size: 10px;
    font-weight: 600;
  }

  /* ── Notification panel ─────────────────── */
  .notif-panel {
    position: absolute;
    top: calc(100% + var(--space-2));
    right: 0;
    width: 400px;
    max-height: 500px;
    display: flex;
    flex-direction: column;
    background: var(--bg-surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
    overflow: hidden;
    animation: panelIn 0.15s var(--ease-out);
    z-index: 50;
  }

  @keyframes panelIn {
    from { opacity: 0; transform: translateY(-4px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .notif-panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
  }

  .notif-panel-title {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-primary);
  }

  .notif-panel-actions {
    display: flex;
    gap: var(--space-2);
  }

  .notif-filter-bar {
    display: flex;
    gap: 2px;
    padding: var(--space-1) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    background: var(--bg-elevated);
  }
  .notif-filter-tab {
    padding: 3px var(--space-2);
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-tertiary);
    font-family: var(--font-display);
    font-size: 11px;
    font-weight: 500;
    cursor: pointer;
    transition: all var(--duration-fast) var(--ease-out);
  }
  .notif-filter-tab:hover { color: var(--text-primary); }
  .notif-filter-tab.active { background: var(--accent); color: #fff; }

  .notif-panel-body {
    flex: 1;
    overflow-y: auto;
  }

  .notif-empty {
    padding: var(--space-6);
    text-align: center;
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }

  /* ── Individual notification ────────────── */
  .notif-item {
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
    border-left: 2px solid transparent;
    transition: background var(--duration-fast) var(--ease-out);
  }
  .notif-item:last-child { border-bottom: none; }
  .notif-item:hover { background: rgba(255, 255, 255, 0.02); }

  .notif-unread { background: rgba(224, 145, 69, 0.04); }
  .notif-unread .notif-title { color: var(--accent-text); }

  .notif-error { border-left-color: var(--error); }
  .notif-warn { border-left-color: var(--warning); }
  .notif-success { border-left-color: var(--success); }
  .notif-info { border-left-color: var(--accent); }

  .notif-item-top {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: 2px;
  }

  .notif-cat-icon {
    font-size: var(--text-sm);
    flex-shrink: 0;
  }

  .notif-title {
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 500;
    color: var(--text-primary);
    flex: 1;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .notif-time {
    font-size: 10px;
    color: var(--text-ghost);
    flex-shrink: 0;
  }

  .notif-message {
    font-size: var(--text-xs);
    color: var(--text-secondary);
    line-height: 1.4;
    margin-bottom: var(--space-1);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .notif-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .notif-meta-tag {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
  }

  @media (max-width: 768px) {
    .header { padding: 0 var(--space-4); }
    .header-meta { display: none; }
    .notif-panel { width: calc(100vw - var(--space-8)); right: calc(-1 * var(--space-2)); }
  }
</style>
