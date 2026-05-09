<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { listSessions, deleteSession, compactSession, renameSession, getSessionHistory, runMemorySearch, setSessionArchived, setSessionPinned } from '../lib/api'
  import { highlightTerms } from '../lib/markdown'
  import { cleanupCandidateSessions, isArchived, isPinned, organizeSessions, sessionKind, type SessionKindFilter, type SessionSortMode } from '../lib/sessionOrganization'
  import { t } from '../i18n'
  import type { MemorySearchMatch, Session } from '../lib/types'

  type SessionSearchSnippet = {
    date: string
    snippet: string
  }

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
  let sortBy: SessionSortMode = $state('updated')
  let filterKind: SessionKindFilter = $state('all')

  let renamingId: string | null = $state(null)
  let renameValue = $state('')
  let deleteConfirmId: string | null = $state(null)
  let actionBusy = $state('')
  let actionError = $state('')
  let transcriptSearchLoading = $state(false)
  let transcriptSearchError = $state('')
  let sessionSearchSnippets: Record<string, SessionSearchSnippet[]> = $state({})
  const sessionFilters = ['all', 'session', 'main', 'worker', 'archived'] as const
  const maxSnippetsPerSession = 3
  let transcriptSearchTimer: ReturnType<typeof setTimeout> | null = null
  let transcriptSearchToken = 0
  let cleanupSuggestions = $derived(cleanupCandidateSessions(sessions))

  function relativeTime(value?: string): string {
    if (!value?.trim()) return ''
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    if (date.getFullYear() <= 1) return ''
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
    const labels = $t.sessions.relativeTime
    if (seconds < 60) return labels.secondsAgo(seconds)
    if (seconds < 3600) return labels.minutesAgo(Math.floor(seconds / 60))
    if (seconds < 86400) return labels.hoursAgo(Math.floor(seconds / 3600))
    return labels.daysAgo(Math.floor(seconds / 86400))
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
    return organizeSessions(sessions, {
      filterKind,
      sortBy,
      query: searchQuery,
      hasTranscriptMatch: (id) => (sessionSearchSnippets[id]?.length ?? 0) > 0,
    })
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
      transcriptSearchError = err instanceof Error ? err.message : $t.sessions.errors.transcriptSearchFailed
      sessionSearchSnippets = {}
    } finally {
      if (token === transcriptSearchToken) {
        transcriptSearchLoading = false
      }
    }
  }

  export async function load() {
    loading = true
    error = ''
    try {
      sessions = await listSessions(true, 'include')
    } catch (err) {
      error = err instanceof Error ? err.message : $t.sessions.errors.loadFailed
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
      actionError = e instanceof Error ? e.message : $t.sessions.errors.deleteFailed
    } finally {
      actionBusy = ''
    }
  }

  async function handleSetArchived(id: string, archived: boolean) {
    actionBusy = id
    actionError = ''
    try {
      await setSessionArchived(id, archived)
      if (archived && selectedSessionId === id) onNewSession()
      await load()
    } catch (e) {
      actionError = e instanceof Error ? e.message : $t.sessions.errors.archiveFailed
    } finally {
      actionBusy = ''
    }
  }

  async function handleSetPinned(id: string, pinned: boolean) {
    actionBusy = id
    actionError = ''
    try {
      await setSessionPinned(id, pinned)
      await load()
    } catch (e) {
      actionError = e instanceof Error ? e.message : $t.sessions.errors.pinFailed
    } finally {
      actionBusy = ''
    }
  }

  async function handleArchiveCleanupCandidates() {
    const candidates = cleanupSuggestions
    if (candidates.length === 0) return
    actionBusy = 'cleanup'
    actionError = ''
    try {
      await Promise.all(candidates.map((session) => setSessionArchived(session.id, true)))
      if (selectedSessionId && candidates.some((session) => session.id === selectedSessionId)) onNewSession()
      await load()
    } catch (e) {
      actionError = e instanceof Error ? e.message : $t.sessions.errors.archiveFailed
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
        actionError = $t.sessions.errors.compactSuccess(r.compacted_count, pct)
      } else {
        actionError = r.reason || $t.sessions.errors.nothingToCompact
      }
      await load()
    } catch (e) {
      actionError = e instanceof Error ? e.message : $t.sessions.errors.compactFailed
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
      actionError = e instanceof Error ? e.message : $t.sessions.errors.generateTitleFailed
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

<div class="sidebar">
  <div class="sidebar-header">
    <button type="button" class="btn btn-primary btn-sm new-chat-btn" onclick={onNewSession}>
      {$t.sessions.newChat}
    </button>
  </div>

  <input type="text" class="sidebar-search" placeholder={$t.sessions.sidebarSearchPlaceholder} bind:value={searchQuery} />

  <div class="sidebar-filters">
    {#each sessionFilters as kind}
      <button
        class="filter-btn"
        class:active={filterKind === kind}
        onclick={() => { filterKind = kind }}
      >{$t.sessions.filters[kind as keyof typeof $t.sessions.filters]}</button>
    {/each}
    <div class="sort-btns">
      <button class="filter-btn" class:active={sortBy === 'updated'} onclick={() => { sortBy = 'updated' }} title={$t.sessions.sort.recentTitle}>{'\u2193'}</button>
      <button class="filter-btn" class:active={sortBy === 'name'} onclick={() => { sortBy = 'name' }} title={$t.sessions.sort.nameTitle}>A</button>
    </div>
  </div>

  {#if error}
    <div class="error-banner" style="margin:var(--space-2);font-size:var(--text-xs)">{error}</div>
  {/if}
  {#if actionError}
    <div class="error-banner" style="margin:var(--space-2);font-size:var(--text-xs)">{actionError}</div>
  {/if}
  {#if transcriptSearchError}
    <div class="error-banner" style="margin:var(--space-2);font-size:var(--text-xs)">{transcriptSearchError}</div>
  {/if}

  {#if cleanupSuggestions.length > 0 && (filterKind === 'all' || filterKind === 'session') && !searchQuery.trim()}
    <section class="cleanup-panel" aria-label={$t.sessions.cleanup.title}>
      <div class="cleanup-head">
        <span>{$t.sessions.cleanup.title}</span>
        <strong>{$t.sessions.cleanup.count(cleanupSuggestions.length)}</strong>
      </div>
      <div class="cleanup-preview">{cleanupSuggestions.slice(0, 3).map((session) => session.title || session.id.slice(0, 12)).join(' · ')}</div>
      <button class="cleanup-action" type="button" disabled={actionBusy === 'cleanup'} onclick={handleArchiveCleanupCandidates}>
        {$t.sessions.cleanup.archiveSuggested(cleanupSuggestions.length)}
      </button>
    </section>
  {/if}

  <div class="session-list">
    {#if loading}
      <div class="sidebar-loading">{$t.sessions.sidebarLoading}</div>
    {:else if transcriptSearchLoading && filteredSessions().length === 0}
      <div class="sidebar-loading">{$t.sessions.sidebarSearchingTranscripts}</div>
    {:else if filteredSessions().length === 0}
      <div class="sidebar-empty">{searchQuery || filterKind !== 'all' ? $t.sessions.sidebarNoMatches : $t.sessions.sidebarEmpty}</div>
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
              <span class="badge {kindBadge(session)}" style="font-size:9px;padding:1px 5px">{$t.sessions.filters[sessionKind(session) as keyof typeof $t.sessions.filters] ?? sessionKind(session)}</span>
              {#if isArchived(session)}
                <span class="badge badge-default" style="font-size:9px;padding:1px 5px">{$t.sessions.filters.archived}</span>
              {/if}
              <span class="session-time">{relativeTime(session.updated_at)}</span>
            </div>
            {#if snippetsForSession(session.id).length > 0}
              <div class="sidebar-snippet-list" aria-label={$t.sessions.transcriptMatches}>
                {#each snippetsForSession(session.id) as match}
                  <div class="sidebar-snippet">
                    <span class="sidebar-snippet-date">{match.date || $t.sessions.snippetFallbackKind}</span>
                    <span class="sidebar-snippet-text">{@html highlightSearchSnippet(match.snippet)}</span>
                  </div>
                {/each}
              </div>
            {/if}
          </button>
          <div class="session-actions">
            {#if !isMainSession(session)}
              <button class="act-btn" class:active={isPinned(session)} aria-label={isPinned(session) ? $t.sessions.actions.unpin : $t.sessions.actions.pin} title={isPinned(session) ? $t.sessions.actions.unpin : $t.sessions.actions.pin} disabled={actionBusy === session.id} onclick={(e) => { e.stopPropagation(); handleSetPinned(session.id, !isPinned(session)) }}>{isPinned(session) ? '★' : '☆'}</button>
              <button class="act-btn" aria-label={$t.sessions.actions.rename} title={$t.sessions.actions.rename} onclick={(e) => { e.stopPropagation(); startRename(session) }}>&#9998;</button>
              <button class="act-btn" aria-label={$t.sessions.actions.autoTitle} title={$t.sessions.actions.autoTitle} disabled={actionBusy === session.id} onclick={(e) => { e.stopPropagation(); handleGenerateTitle(session) }}>A</button>
            {/if}
            <button class="act-btn" aria-label={$t.sessions.actions.compact} title={$t.sessions.actions.compact} disabled={actionBusy === session.id} onclick={(e) => { e.stopPropagation(); handleCompact(session.id) }}>&#8858;</button>
            {#if !isMainSession(session)}
              <button class="act-btn" aria-label={isArchived(session) ? $t.sessions.actions.restore : $t.sessions.actions.archive} title={isArchived(session) ? $t.sessions.actions.restore : $t.sessions.actions.archive} disabled={actionBusy === session.id} onclick={(e) => { e.stopPropagation(); handleSetArchived(session.id, !isArchived(session)) }}>{isArchived(session) ? '⤴' : '⤓'}</button>
              <button
                class="act-btn act-btn-danger"
                aria-label={deleteConfirmId === session.id ? $t.sessions.actions.confirm : $t.sessions.actions.delete}
                title={deleteConfirmId === session.id ? $t.sessions.actions.confirm : $t.sessions.actions.delete}
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
  .filter-btn.active { color: var(--primary); }

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

  .cleanup-panel {
    display: grid;
    gap: var(--space-1);
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
  }

  .cleanup-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    color: var(--text-secondary);
    font-size: 10px;
    font-family: var(--font-mono);
    text-transform: uppercase;
  }

  .cleanup-head strong {
    color: var(--primary);
    font-weight: 600;
  }

  .cleanup-preview {
    min-width: 0;
    overflow: hidden;
    color: var(--text-ghost);
    font-size: 10px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cleanup-action {
    justify-self: start;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
    font-size: 10px;
    padding: 2px 6px;
  }

  .cleanup-action:hover {
    border-color: var(--primary-muted);
    color: var(--primary);
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
    background: var(--surface-hover);
  }
  .session-item.active {
    background: var(--primary-muted);
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

  .sidebar-snippet-list {
    display: grid;
    gap: 3px;
    margin-top: var(--space-1);
  }

  .sidebar-snippet {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: var(--space-1);
    align-items: start;
    min-width: 0;
    color: var(--text-tertiary);
    font-size: 10px;
    line-height: 1.3;
  }

  .sidebar-snippet-date {
    color: var(--text-ghost);
    font-family: var(--font-mono);
    white-space: nowrap;
  }

  .sidebar-snippet-text {
    display: -webkit-box;
    min-width: 0;
    overflow: hidden;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    word-break: break-word;
  }

  .sidebar-snippet-text :global(mark) {
    padding: 0 2px;
    border-radius: 3px;
    background: rgba(224, 145, 69, 0.24);
    color: var(--text-primary);
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
  .act-btn:hover { color: var(--primary); background: rgba(255,255,255,0.04); }
  .act-btn.active { color: var(--primary); }
  .act-btn-danger:hover { color: var(--error); }

  .rename-input {
    flex: 1;
    padding: 1px var(--space-1);
    font-size: var(--text-xs);
    background: var(--surface-base);
    border: 1px solid var(--primary);
    border-radius: 2px;
    color: var(--text-primary);
    outline: none;
    min-width: 0;
  }
</style>
