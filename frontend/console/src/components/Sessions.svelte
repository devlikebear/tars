<script lang="ts">
  import { onMount } from 'svelte'
  import { listSessions, deleteSession, compactSession, renameSession } from '../lib/api'
  import type { Session } from '../lib/types'
  import ChatPanel from './ChatPanel.svelte'

  let sessions: Session[] = $state([])
  let loading = $state(true)
  let error = $state('')
  let showHidden = $state(false)

  let selectedSession: Session | null = $state(null)
  let searchQuery = $state('')
  let sortBy: 'updated' | 'name' = $state('updated')

  // Rename state
  let renamingId: string | null = $state(null)
  let renameValue = $state('')

  // Action state
  let actionBusy = $state('')
  let actionError = $state('')

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function kindLabel(session: Session): string {
    if (session.kind === 'main') return 'main'
    if (session.hidden) return 'worker'
    return session.kind || 'session'
  }

  function kindBadge(session: Session): string {
    if (session.kind === 'main') return 'badge-accent'
    if (session.hidden) return 'badge-default'
    return 'badge-info'
  }

  function filteredSessions(): Session[] {
    let result = sessions
    const q = searchQuery.trim().toLowerCase()
    if (q) {
      result = result.filter((s) =>
        (s.title || '').toLowerCase().includes(q) ||
        s.id.toLowerCase().includes(q) ||
        (s.project_id || '').toLowerCase().includes(q)
      )
    }
    if (sortBy === 'name') {
      result = [...result].sort((a, b) => (a.title || a.id).localeCompare(b.title || b.id))
    } else {
      result = [...result].sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())
    }
    return result
  }

  async function load() {
    loading = true
    error = ''
    try {
      sessions = await listSessions(showHidden)
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load sessions'
    } finally {
      loading = false
    }
  }

  function selectSession(session: Session) {
    if (selectedSession?.id === session.id) {
      selectedSession = null
      return
    }
    selectedSession = session
    renamingId = null
    actionError = ''
  }

  function toggleHidden() {
    showHidden = !showHidden
    selectedSession = null
    void load()
  }

  function startRename(s: Session) {
    renamingId = s.id
    renameValue = s.title || s.id.slice(0, 12)
  }

  async function commitRename() {
    if (!renamingId || !renameValue.trim()) { renamingId = null; return }
    try {
      await renameSession(renamingId, renameValue.trim())
      await load()
      if (selectedSession?.id === renamingId) {
        selectedSession = sessions.find((s) => s.id === renamingId) || selectedSession
      }
    } catch { /* ignore */ }
    renamingId = null
  }

  async function handleDelete(id: string) {
    actionBusy = id
    actionError = ''
    try {
      await deleteSession(id)
      if (selectedSession?.id === id) selectedSession = null
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

  onMount(() => { void load() })
</script>

<div class="sessions">
  <div class="sessions-header">
    <div>
      <h2>Sessions</h2>
      <p class="sessions-subtitle">Chat sessions and transcripts.</p>
    </div>
    <label class="sessions-toggle">
      <input type="checkbox" checked={showHidden} onchange={toggleHidden} />
      <span>Show worker sessions</span>
    </label>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  <!-- Search + Sort toolbar -->
  <div class="sessions-toolbar">
    <input type="text" class="sessions-search" placeholder="Search sessions..." bind:value={searchQuery} />
    <div class="sessions-sort">
      <button class="btn btn-ghost btn-sm" class:active={sortBy === 'updated'} onclick={() => { sortBy = 'updated' }}>Recent</button>
      <button class="btn btn-ghost btn-sm" class:active={sortBy === 'name'} onclick={() => { sortBy = 'name' }}>Name</button>
    </div>
  </div>

  {#if actionError}
    <div class="error-banner" style="margin-bottom:var(--space-3)">{actionError}</div>
  {/if}

  {#if loading}
    <div class="sessions-loading">Loading sessions...</div>
  {:else if filteredSessions().length === 0}
    <div class="empty-state"><p>{searchQuery ? 'No matching sessions.' : 'No sessions found.'}</p></div>
  {:else}
    <div class="sessions-layout">
      <div class="sessions-list">
        {#each filteredSessions() as session}
          <div class="session-item-wrap" class:active={selectedSession?.id === session.id}>
            <button
              type="button"
              class="session-item"
              onclick={() => { void selectSession(session) }}
            >
              <div class="session-item-top">
                {#if renamingId === session.id}
                  <!-- svelte-ignore a11y_autofocus -->
                  <input
                    class="session-rename-input"
                    bind:value={renameValue}
                    autofocus
                    onkeydown={(e) => { if (e.key === 'Enter') commitRename(); if (e.key === 'Escape') { renamingId = null } }}
                    onblur={() => commitRename()}
                    onclick={(e) => e.stopPropagation()}
                  />
                {:else}
                  <strong class="session-item-title">{session.title || session.id.slice(0, 12)}</strong>
                {/if}
                <span class="badge {kindBadge(session)}">{kindLabel(session)}</span>
              </div>
              <div class="session-item-meta">
                {#if session.project_id}
                  <span>project: {session.project_id}</span>
                {/if}
                <span>{fmt(session.updated_at)}</span>
              </div>
            </button>
            <div class="session-item-actions">
              <button class="btn-icon" title="Rename" onclick={(e) => { e.stopPropagation(); startRename(session) }}>&#9998;</button>
              <button class="btn-icon" title="Compact" disabled={actionBusy === session.id} onclick={(e) => { e.stopPropagation(); handleCompact(session.id) }}>&#8858;</button>
              <button class="btn-icon btn-icon-danger" title="Delete" disabled={actionBusy === session.id} onclick={(e) => { e.stopPropagation(); handleDelete(session.id) }}>&times;</button>
            </div>
          </div>
        {/each}
      </div>

      {#if selectedSession}
        <div class="session-detail">
          <div class="session-detail-header">
            <h3>{selectedSession.title || selectedSession.id.slice(0, 12)}</h3>
            <span class="badge {kindBadge(selectedSession)}">{kindLabel(selectedSession)}</span>
          </div>

          <div class="session-chat">
            {#key selectedSession.id}
              <ChatPanel sessionId={selectedSession.id} projectId={selectedSession.project_id || undefined} onSessionChange={load} />
            {/key}
          </div>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .sessions {
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .sessions-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-4);
  }

  .sessions-header h2 {
    font-size: var(--text-2xl);
    margin-bottom: var(--space-1);
  }

  .sessions-subtitle {
    color: var(--text-tertiary);
  }

  .sessions-toggle {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    color: var(--text-secondary);
    cursor: pointer;
    flex-shrink: 0;
  }

  .sessions-toggle input {
    accent-color: var(--accent);
  }

  .sessions-toolbar {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }

  .sessions-search {
    flex: 1;
    padding: var(--space-2) var(--space-3);
    background: var(--bg-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-sm);
  }
  .sessions-search:focus {
    outline: none;
    border-color: var(--accent);
  }

  .sessions-sort {
    display: flex;
    gap: var(--space-1);
    flex-shrink: 0;
  }
  .sessions-sort .active {
    color: var(--accent);
    border-color: var(--accent);
  }

  .sessions-loading {
    padding: var(--space-10);
    text-align: center;
    color: var(--text-tertiary);
  }

  .sessions-layout {
    display: grid;
    grid-template-columns: minmax(280px, 360px) minmax(0, 1fr);
    gap: var(--space-4);
    align-items: start;
  }

  .sessions-list {
    display: grid;
    gap: var(--space-1);
    max-height: calc(100vh - 260px);
    overflow-y: auto;
    overflow-x: hidden;
    min-width: 0;
  }

  .session-item-wrap {
    display: flex;
    align-items: stretch;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--bg-surface);
    overflow: hidden;
    transition: border-color var(--duration-fast) var(--ease-out);
  }
  .session-item-wrap:hover {
    border-color: var(--border-default);
  }
  .session-item-wrap.active {
    border-color: var(--accent);
    background: var(--accent-muted);
  }

  .session-item {
    flex: 1;
    display: block;
    padding: var(--space-2) var(--space-3);
    background: none;
    border: none;
    text-align: left;
    cursor: pointer;
    min-width: 0;
  }

  .session-item-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: 2px;
  }

  .session-item-title {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }

  .session-item-meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-3);
    font-size: 10px;
    color: var(--text-ghost);
  }

  .session-item-actions {
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 2px;
    padding: var(--space-1);
    opacity: 0;
    transition: opacity var(--duration-fast);
  }
  .session-item-wrap:hover .session-item-actions {
    opacity: 1;
  }

  .btn-icon {
    background: none;
    border: none;
    color: var(--text-ghost);
    cursor: pointer;
    font-size: 12px;
    padding: 2px 4px;
    border-radius: var(--radius-sm);
    line-height: 1;
  }
  .btn-icon:hover { color: var(--accent); background: rgba(255,255,255,0.04); }
  .btn-icon-danger:hover { color: var(--error); }

  .session-rename-input {
    flex: 1;
    padding: 2px var(--space-1);
    font-size: var(--text-sm);
    background: var(--bg-base);
    border: 1px solid var(--accent);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    outline: none;
    min-width: 0;
  }

  .session-detail {
    background: var(--bg-surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    padding: var(--space-4);
    min-width: 0;
    overflow: hidden;
  }

  .session-detail-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    margin-bottom: var(--space-3);
  }

  .session-detail-header h3 {
    font-size: var(--text-lg);
    font-weight: 500;
  }

  .session-chat {
    min-height: 0;
  }

  @media (max-width: 900px) {
    .sessions-layout {
      grid-template-columns: 1fr;
    }
    .session-detail {
      position: static;
    }
  }
</style>
