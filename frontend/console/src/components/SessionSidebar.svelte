<script lang="ts">
  import { onMount } from 'svelte'
  import { listSessions, deleteSession, compactSession, renameSession, getSessionHistory } from '../lib/api'
  import type { Session } from '../lib/types'

  interface Props {
    selectedSessionId: string | null
    onSelect: (session: Session) => void
    onNewSession: () => void
  }

  let { selectedSessionId, onSelect, onNewSession }: Props = $props()

  let sessions: Session[] = $state([])
  let loading = $state(true)
  let error = $state('')

  let searchQuery = $state('')
  let sortBy: 'updated' | 'name' = $state('updated')
  let filterKind: 'all' | 'session' | 'main' | 'worker' = $state('all')

  let renamingId: string | null = $state(null)
  let renameValue = $state('')
  let deleteConfirmId: string | null = $state(null)
  let actionBusy = $state('')
  let actionError = $state('')

  function relativeTime(value?: string): string {
    if (!value?.trim()) return ''
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
    if (seconds < 60) return `${seconds}s ago`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
    return `${Math.floor(seconds / 86400)}d ago`
  }

  function sessionKind(session: Session): string {
    if (session.kind === 'main') return 'main'
    if (session.hidden) return 'worker'
    return 'session'
  }

  function kindBadge(session: Session): string {
    switch (sessionKind(session)) {
      case 'main': return 'badge-accent'
      case 'worker': return 'badge-default'
      default: return 'badge-info'
    }
  }

  function isMainSession(session: Session): boolean {
    return session.kind === 'main'
  }

  function filteredSessions(): Session[] {
    let result = sessions
    if (filterKind !== 'all') {
      result = result.filter((s) => sessionKind(s) === filterKind)
    }
    const q = searchQuery.trim().toLowerCase()
    if (q) {
      result = result.filter((s) =>
        (s.title || '').toLowerCase().includes(q) ||
        s.id.toLowerCase().includes(q)
      )
    }
    if (sortBy === 'name') {
      result = [...result].sort((a, b) => (a.title || a.id).localeCompare(b.title || b.id))
    } else {
      result = [...result].sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())
    }
    return result
  }

  export async function load() {
    loading = true
    error = ''
    try {
      sessions = await listSessions(true)
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load sessions'
    } finally {
      loading = false
    }
  }

  function startRename(s: Session) {
    if (isMainSession(s)) return
    renamingId = s.id
    renameValue = s.title || s.id.slice(0, 12)
  }

  async function commitRename() {
    if (!renamingId || !renameValue.trim()) { renamingId = null; return }
    try {
      await renameSession(renamingId, renameValue.trim())
      await load()
    } catch { /* ignore */ }
    renamingId = null
  }

  function requestDelete(id: string) {
    if (deleteConfirmId === id) {
      void handleDelete(id)
    } else {
      deleteConfirmId = id
    }
  }

  async function handleDelete(id: string) {
    actionBusy = id
    actionError = ''
    deleteConfirmId = null
    try {
      await deleteSession(id)
      if (selectedSessionId === id) onNewSession()
      await load()
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Delete failed'
    } finally {
      actionBusy = ''
    }
  }

  async function handleCompact(id: string) {
    actionBusy = id
    actionError = ''
    try {
      await compactSession(id)
      await load()
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Compact failed'
    } finally {
      actionBusy = ''
    }
  }

  async function handleGenerateTitle(s: Session) {
    actionBusy = s.id
    actionError = ''
    try {
      const history = await getSessionHistory(s.id)
      const userMsgs = history.filter((m) => m.role === 'user')
      const assistantMsgs = history.filter((m) => m.role === 'assistant')
      let title = ''
      if (userMsgs.length > 0) {
        const raw = userMsgs[0].content.trim()
        const clean = raw.replace(/\n/g, ' ').replace(/\s+/g, ' ')
        title = clean.length > 50 ? clean.slice(0, 47) + '...' : clean
      } else if (assistantMsgs.length > 0) {
        const raw = assistantMsgs[0].content.trim()
        const clean = raw.replace(/\n/g, ' ').replace(/\s+/g, ' ')
        title = clean.length > 50 ? clean.slice(0, 47) + '...' : clean
      }
      if (title) {
        await renameSession(s.id, title)
        await load()
      }
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Generate title failed'
    } finally {
      actionBusy = ''
    }
  }

  onMount(() => { void load() })
</script>

<div class="sidebar">
  <div class="sidebar-header">
    <button type="button" class="btn btn-primary btn-sm new-chat-btn" onclick={onNewSession}>
      + New Chat
    </button>
  </div>

  <input type="text" class="sidebar-search" placeholder="Search..." bind:value={searchQuery} />

  <div class="sidebar-filters">
    {#each ['all', 'session', 'main', 'worker'] as kind}
      <button
        class="filter-btn"
        class:active={filterKind === kind}
        onclick={() => { filterKind = kind as typeof filterKind }}
      >{kind}</button>
    {/each}
    <div class="sort-btns">
      <button class="filter-btn" class:active={sortBy === 'updated'} onclick={() => { sortBy = 'updated' }} title="Sort by recent">{'\u2193'}</button>
      <button class="filter-btn" class:active={sortBy === 'name'} onclick={() => { sortBy = 'name' }} title="Sort by name">A</button>
    </div>
  </div>

  {#if error}
    <div class="error-banner" style="margin:var(--space-2);font-size:var(--text-xs)">{error}</div>
  {/if}
  {#if actionError}
    <div class="error-banner" style="margin:var(--space-2);font-size:var(--text-xs)">{actionError}</div>
  {/if}

  <div class="session-list">
    {#if loading}
      <div class="sidebar-loading">Loading...</div>
    {:else if filteredSessions().length === 0}
      <div class="sidebar-empty">{searchQuery || filterKind !== 'all' ? 'No matches.' : 'No sessions.'}</div>
    {:else}
      {#each filteredSessions() as session}
        <div class="session-item" class:active={selectedSessionId === session.id}>
          <button
            type="button"
            class="session-btn"
            onclick={() => onSelect(session)}
          >
            {#if renamingId === session.id}
              <!-- svelte-ignore a11y_autofocus -->
              <input
                class="rename-input"
                bind:value={renameValue}
                autofocus
                onkeydown={(e) => { if (e.key === 'Enter') commitRename(); if (e.key === 'Escape') { renamingId = null } }}
                onblur={() => commitRename()}
                onclick={(e) => e.stopPropagation()}
              />
            {:else}
              <span class="session-title">{session.title || session.id.slice(0, 12)}</span>
            {/if}
            <div class="session-meta">
              <span class="badge {kindBadge(session)}" style="font-size:9px;padding:1px 5px">{sessionKind(session)}</span>
              <span class="session-time">{relativeTime(session.updated_at)}</span>
            </div>
          </button>
          <div class="session-actions">
            {#if !isMainSession(session)}
              <button class="act-btn" title="Rename" onclick={(e) => { e.stopPropagation(); startRename(session) }}>&#9998;</button>
              <button class="act-btn" title="Auto title" disabled={actionBusy === session.id} onclick={(e) => { e.stopPropagation(); handleGenerateTitle(session) }}>&#9733;</button>
            {/if}
            <button class="act-btn" title="Compact" disabled={actionBusy === session.id} onclick={(e) => { e.stopPropagation(); handleCompact(session.id) }}>&#8858;</button>
            {#if !isMainSession(session)}
              <button
                class="act-btn act-btn-danger"
                title={deleteConfirmId === session.id ? 'Confirm' : 'Delete'}
                disabled={actionBusy === session.id}
                onclick={(e) => { e.stopPropagation(); requestDelete(session.id) }}
              >{deleteConfirmId === session.id ? '!!' : '\u00d7'}</button>
            {/if}
          </div>
        </div>
      {/each}
    {/if}
  </div>
</div>

<style>
  .sidebar {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    height: 100%;
    overflow: hidden;
  }

  .sidebar-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .new-chat-btn {
    flex: 1;
  }

  .sidebar-search {
    padding: var(--space-1) var(--space-2) !important;
    font-size: var(--text-xs) !important;
    min-height: 0 !important;
    border-radius: var(--radius-sm) !important;
  }

  .sidebar-filters {
    display: flex;
    align-items: center;
    gap: 2px;
    flex-wrap: wrap;
  }

  .filter-btn {
    background: none;
    border: none;
    color: var(--text-ghost);
    font-size: 10px;
    font-family: var(--font-mono);
    padding: 2px 5px;
    border-radius: var(--radius-sm);
    cursor: pointer;
    transition: color var(--duration-fast);
  }
  .filter-btn:hover { color: var(--text-secondary); }
  .filter-btn.active { color: var(--accent); }

  .sort-btns {
    margin-left: auto;
    display: flex;
    gap: 2px;
  }

  .session-list {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .sidebar-loading, .sidebar-empty {
    padding: var(--space-4);
    text-align: center;
    color: var(--text-ghost);
    font-size: var(--text-xs);
  }

  .session-item {
    display: flex;
    align-items: stretch;
    border-radius: var(--radius-sm);
    transition: background var(--duration-fast) var(--ease-out);
  }
  .session-item:hover {
    background: var(--bg-hover);
  }
  .session-item.active {
    background: var(--accent-muted);
  }

  .session-btn {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: var(--space-2);
    background: none;
    border: none;
    text-align: left;
    cursor: pointer;
    min-width: 0;
  }

  .session-title {
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 500;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .session-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .session-time {
    font-size: 10px;
    color: var(--text-ghost);
  }

  .session-actions {
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 1px;
    padding: 0 2px;
    opacity: 0;
    transition: opacity var(--duration-fast);
  }
  .session-item:hover .session-actions {
    opacity: 1;
  }

  .act-btn {
    background: none;
    border: none;
    color: var(--text-ghost);
    cursor: pointer;
    font-size: 11px;
    padding: 1px 3px;
    border-radius: 2px;
    line-height: 1;
  }
  .act-btn:hover { color: var(--accent); background: rgba(255,255,255,0.04); }
  .act-btn-danger:hover { color: var(--error); }

  .rename-input {
    flex: 1;
    padding: 1px var(--space-1);
    font-size: var(--text-xs);
    background: var(--bg-base);
    border: 1px solid var(--accent);
    border-radius: 2px;
    color: var(--text-primary);
    outline: none;
    min-width: 0;
  }
</style>
