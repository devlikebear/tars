<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { locale, t } from '../i18n'
  import { listSessions, deleteSession, compactSession, renameSession, getSessionHistory, runMemorySearch } from '../lib/api'
  import { highlightTerms } from '../lib/markdown'
  import type { MemorySearchMatch, Session } from '../lib/types'
  import ChatPanel from './ChatPanel.svelte'

  type SessionSearchSnippet = {
    date: string
    snippet: string
  }

  let sessions: Session[] = $state([])
  let loading = $state(true)
  let error = $state('')
  let showHidden = $state(false)

  let selectedSession: Session | null = $state(null)
  let searchQuery = $state('')
  let sortBy: 'updated' | 'name' = $state('updated')
  let filterKind: 'all' | 'session' | 'main' | 'worker' = $state('all')

  // Rename state
  let renamingId: string | null = $state(null)
  let renameValue = $state('')

  // Delete confirm state
  let deleteConfirmId: string | null = $state(null)

  // Action state
  let actionBusy = $state('')
  let actionError = $state('')
  let transcriptSearchLoading = $state(false)
  let transcriptSearchError = $state('')
  let sessionSearchSnippets: Record<string, SessionSearchSnippet[]> = $state({})
  const sessionFilters = ['all', 'session', 'main', 'worker'] as const
  const maxSnippetsPerSession = 3
  let transcriptSearchTimer: ReturnType<typeof setTimeout> | null = null
  let transcriptSearchToken = 0

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat($locale, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
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
    // Kind filter
    if (filterKind !== 'all') {
      result = result.filter((s) => sessionKind(s) === filterKind)
    }
    // Search
    const q = searchQuery.trim().toLowerCase()
    if (q) {
      result = result.filter((s) =>
        (s.title || '').toLowerCase().includes(q) ||
        s.id.toLowerCase().includes(q) ||
        (sessionSearchSnippets[s.id]?.length ?? 0) > 0
      )
    }
    // Sort
    if (sortBy === 'name') {
      result = [...result].sort((a, b) => (a.title || a.id).localeCompare(b.title || b.id))
    } else {
      result = [...result].sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())
    }
    return result
  }

  function sessionIdFromSearchSource(source: string): string {
    const prefix = 'session:'
    const value = source.trim()
    if (!value.startsWith(prefix)) return ''
    return value.slice(prefix.length).trim()
  }

  function groupSessionSearchMatches(matches: MemorySearchMatch[]): Record<string, SessionSearchSnippet[]> {
    const grouped: Record<string, SessionSearchSnippet[]> = {}
    for (const match of matches) {
      if (!match.source.startsWith('session:')) continue
      const id = sessionIdFromSearchSource(match.source)
      const snippet = match.snippet.trim()
      if (!id || !snippet) continue
      const list = grouped[id] ?? []
      if (list.length >= maxSnippetsPerSession || list.some((item) => item.snippet === snippet)) {
        grouped[id] = list
        continue
      }
      list.push({ date: match.date, snippet })
      grouped[id] = list
    }
    return grouped
  }

  function searchHighlightTerms(): string[] {
    return searchQuery.trim().split(/\s+/).filter((term) => term.length > 0)
  }

  function highlightSearchSnippet(snippet: string): string {
    return highlightTerms(snippet, searchHighlightTerms())
  }

  function snippetsForSession(sessionID: string): SessionSearchSnippet[] {
    return sessionSearchSnippets[sessionID] ?? []
  }

  function resetTranscriptSearch() {
    transcriptSearchToken += 1
    transcriptSearchLoading = false
    transcriptSearchError = ''
    sessionSearchSnippets = {}
    if (transcriptSearchTimer) {
      clearTimeout(transcriptSearchTimer)
      transcriptSearchTimer = null
    }
  }

  function scheduleTranscriptSearch(query: string) {
    if (transcriptSearchTimer) {
      clearTimeout(transcriptSearchTimer)
      transcriptSearchTimer = null
    }
    const trimmed = query.trim()
    if (!trimmed) {
      resetTranscriptSearch()
      return
    }
    const token = ++transcriptSearchToken
    transcriptSearchLoading = true
    transcriptSearchError = ''
    transcriptSearchTimer = setTimeout(() => {
      transcriptSearchTimer = null
      void runTranscriptSearch(trimmed, token)
    }, 250)
  }

  async function runTranscriptSearch(query: string, token: number) {
    try {
      const result = await runMemorySearch({
        query,
        limit: 30,
        include_memory: false,
        include_daily: false,
        include_sessions: true,
      })
      if (token !== transcriptSearchToken) return
      sessionSearchSnippets = groupSessionSearchMatches(result.results ?? [])
    } catch (err) {
      if (token !== transcriptSearchToken) return
      transcriptSearchError = err instanceof Error ? err.message : 'Transcript search failed'
      sessionSearchSnippets = {}
    } finally {
      if (token === transcriptSearchToken) {
        transcriptSearchLoading = false
      }
    }
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
    deleteConfirmId = null
    actionError = ''
  }

  function toggleHidden() {
    showHidden = !showHidden
    selectedSession = null
    void load()
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
      if (selectedSession?.id === renamingId) {
        selectedSession = sessions.find((s) => s.id === renamingId) || selectedSession
      }
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
      const r = await compactSession(id)
      if (r.compacted) {
        const pct = r.tokens_before > 0 ? Math.round(((r.tokens_before - r.tokens_after) / r.tokens_before) * 100) : 0
        actionError = `Compacted ${r.compacted_count} msgs (${pct}% saved)`
      } else {
        actionError = r.reason || 'Nothing to compact'
      }
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
        // Use first user message, cleaned up
        const raw = userMsgs[0].content.trim()
        // Remove newlines, collapse whitespace
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
        if (selectedSession?.id === s.id) {
          selectedSession = sessions.find((ss) => ss.id === s.id) || selectedSession
        }
      }
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Generate title failed'
    } finally {
      actionBusy = ''
    }
  }

  onMount(() => { void load() })

  onDestroy(() => {
    transcriptSearchToken += 1
    if (transcriptSearchTimer) {
      clearTimeout(transcriptSearchTimer)
    }
  })

  $effect(() => {
    scheduleTranscriptSearch(searchQuery)
  })
</script>

<div class="sessions">
  <div class="sessions-header">
    <div>
      <h2>{$t.sessions.title}</h2>
      <p class="sessions-subtitle">{$t.sessions.subtitle}</p>
    </div>
    <label class="sessions-toggle">
      <input type="checkbox" checked={showHidden} onchange={toggleHidden} />
      <span>{$t.sessions.showWorkers}</span>
    </label>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  <!-- Toolbar: search + kind filter + sort -->
  <div class="sessions-toolbar">
    <input type="text" class="sessions-search" placeholder={$t.sessions.searchPlaceholder} bind:value={searchQuery} />
    <div class="sessions-filters">
      {#each sessionFilters as kind}
        <button class="btn btn-ghost btn-sm" class:active={filterKind === kind} onclick={() => { filterKind = kind }}>{$t.sessions.filters[kind]}</button>
      {/each}
    </div>
    <div class="sessions-sort">
      <button class="btn btn-ghost btn-sm" class:active={sortBy === 'updated'} onclick={() => { sortBy = 'updated' }}>{$t.sessions.sort.recent}</button>
      <button class="btn btn-ghost btn-sm" class:active={sortBy === 'name'} onclick={() => { sortBy = 'name' }}>{$t.sessions.sort.name}</button>
    </div>
  </div>

  {#if actionError}
    <div class="error-banner" style="margin-bottom:var(--space-3)">{actionError}</div>
  {/if}

  {#if transcriptSearchError}
    <div class="error-banner" style="margin-bottom:var(--space-3)">{transcriptSearchError}</div>
  {/if}

  {#if loading}
    <div class="sessions-loading">{$t.sessions.loading}</div>
  {:else if transcriptSearchLoading && filteredSessions().length === 0}
    <div class="sessions-loading">{$t.sessions.searchingTranscripts}</div>
  {:else if filteredSessions().length === 0}
    <div class="empty-state"><p>{searchQuery || filterKind !== 'all' ? $t.sessions.noMatches : $t.sessions.empty}</p></div>
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
                <span class="badge {kindBadge(session)}">{sessionKind(session)}</span>
              </div>
              <div class="session-item-meta">
                <span>{fmt(session.updated_at)}</span>
              </div>
              {#if snippetsForSession(session.id).length > 0}
                <div class="session-snippet-list" aria-label={$t.sessions.transcriptMatches}>
                  {#each snippetsForSession(session.id) as match}
                    <div class="session-snippet">
                      <span class="session-snippet-date">{match.date || 'session'}</span>
                      <span class="session-snippet-text">{@html highlightSearchSnippet(match.snippet)}</span>
                    </div>
                  {/each}
                </div>
              {/if}
            </button>
            <div class="session-item-actions">
              {#if !isMainSession(session)}
                <button class="btn-icon" title={$t.common.actions.rename} onclick={(e) => { e.stopPropagation(); startRename(session) }}>&#9998;</button>
                <button class="btn-icon" title={$t.sessions.autoTitle} disabled={actionBusy === session.id} onclick={(e) => { e.stopPropagation(); handleGenerateTitle(session) }}>&#9733;</button>
              {/if}
              <button class="btn-icon" title={$t.common.actions.compact} disabled={actionBusy === session.id} onclick={(e) => { e.stopPropagation(); handleCompact(session.id) }}>&#8858;</button>
              {#if !isMainSession(session)}
                <button
                  class="btn-icon btn-icon-danger"
                  title={deleteConfirmId === session.id ? $t.sessions.clickAgainToConfirm : $t.common.actions.delete}
                  disabled={actionBusy === session.id}
                  onclick={(e) => { e.stopPropagation(); requestDelete(session.id) }}
                >{deleteConfirmId === session.id ? '!!' : '\u00d7'}</button>
              {/if}
            </div>
          </div>
        {/each}
      </div>

      {#if selectedSession}
        <div class="session-detail">
          <div class="session-detail-header">
            <h3>{selectedSession.title || selectedSession.id.slice(0, 12)}</h3>
            <div class="session-detail-actions">
              <span class="badge {kindBadge(selectedSession)}">{sessionKind(selectedSession)}</span>
              {#if !isMainSession(selectedSession)}
                <button class="btn btn-ghost btn-sm" disabled={actionBusy === selectedSession.id} onclick={() => { if (selectedSession) startRename(selectedSession) }}>{$t.common.actions.rename}</button>
                <button class="btn btn-ghost btn-sm" disabled={actionBusy === selectedSession.id} onclick={() => { if (selectedSession) handleGenerateTitle(selectedSession) }}>{$t.sessions.autoTitle}</button>
              {/if}
              <button class="btn btn-ghost btn-sm" disabled={actionBusy === selectedSession?.id} onclick={() => { if (selectedSession) handleCompact(selectedSession.id) }}>{$t.common.actions.compact}</button>
              {#if !isMainSession(selectedSession)}
                <button class="btn btn-danger btn-sm" disabled={actionBusy === selectedSession.id} onclick={() => { if (selectedSession) requestDelete(selectedSession.id) }}>
                  {deleteConfirmId === selectedSession.id ? $t.sessions.confirmDelete : $t.common.actions.delete}
                </button>
              {/if}
            </div>
          </div>

          <div class="session-chat">
            {#key selectedSession.id}
              <ChatPanel sessionId={selectedSession.id} onSessionChange={load} />
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
    accent-color: var(--primary);
  }

  .sessions-toolbar {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
    flex-wrap: wrap;
  }

  .sessions-search {
    flex: 1;
    min-width: 160px;
    padding: var(--space-2) var(--space-3);
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-sm);
  }
  .sessions-search:focus {
    outline: none;
    border-color: var(--primary);
  }

  .sessions-filters {
    display: flex;
    gap: var(--space-1);
    flex-shrink: 0;
  }
  .sessions-filters .active {
    color: var(--primary);
    border-color: var(--primary);
  }

  .sessions-sort {
    display: flex;
    gap: var(--space-1);
    flex-shrink: 0;
  }
  .sessions-sort .active {
    color: var(--primary);
    border-color: var(--primary);
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
    max-height: calc(100vh - 300px);
    overflow-y: auto;
    overflow-x: hidden;
    min-width: 0;
  }

  .session-item-wrap {
    display: flex;
    align-items: stretch;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
    overflow: hidden;
    transition: border-color var(--duration-fast) var(--ease-out);
  }
  .session-item-wrap:hover {
    border-color: var(--border-default);
  }
  .session-item-wrap.active {
    border-color: var(--primary);
    background: var(--primary-muted);
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

  .session-snippet-list {
    display: grid;
    gap: 3px;
    margin-top: var(--space-2);
  }

  .session-snippet {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: var(--space-2);
    align-items: start;
    min-width: 0;
    color: var(--text-tertiary);
    font-size: 10px;
    line-height: 1.35;
  }

  .session-snippet-date {
    color: var(--text-ghost);
    font-family: var(--font-mono);
    white-space: nowrap;
  }

  .session-snippet-text {
    display: -webkit-box;
    min-width: 0;
    overflow: hidden;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    word-break: break-word;
  }

  .session-snippet-text :global(mark) {
    padding: 0 2px;
    border-radius: 3px;
    background: rgba(224, 145, 69, 0.24);
    color: var(--text-primary);
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
  .btn-icon:hover { color: var(--primary); background: rgba(255,255,255,0.04); }
  .btn-icon-danger:hover { color: var(--error); }

  .session-rename-input {
    flex: 1;
    padding: 2px var(--space-1);
    font-size: var(--text-sm);
    background: var(--surface-base);
    border: 1px solid var(--primary);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    outline: none;
    min-width: 0;
  }

  .session-detail {
    background: var(--surface);
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
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .session-detail-actions {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    flex-shrink: 0;
    flex-wrap: wrap;
  }

  .session-chat {
    min-height: 0;
  }

  @media (max-width: 900px) {
    .sessions-layout {
      grid-template-columns: 1fr;
    }
  }
</style>
