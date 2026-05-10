<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { listSessions, deleteSession, compactSession, renameSession, getSessionHistory, runMemorySearch, setSessionArchived, setSessionPinned, recommendSessionCleanup } from '../lib/api'
  import { highlightTerms } from '../lib/markdown'
  import { cleanupCandidateSessions, groupSessions, isArchived, isPinned, organizeSessions, sessionKind, type SessionGroup, type SessionKindFilter, type SessionSortMode } from '../lib/sessionOrganization'
  import { t } from '../i18n'
  import type { MemorySearchMatch, Session, SessionCleanupMode, SessionCleanupSuggestion, SessionCleanupSuggestionResponse } from '../lib/types'

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
  let sortBy = $state<SessionSortMode>('updated')
  let filterKind = $state<SessionKindFilter>('all')

  let renamingId: string | null = $state(null)
  let renameValue = $state('')
  let deleteConfirmId: string | null = $state(null)
  let actionBusy = $state('')
  let actionError = $state('')
  let transcriptSearchLoading = $state(false)
  let transcriptSearchError = $state('')
  let sessionSearchSnippets: Record<string, SessionSearchSnippet[]> = $state({})
  let aiCleanupResponse = $state<SessionCleanupSuggestionResponse | null>(null)
  let aiCleanupSelected = $state<Record<string, boolean>>({})
  let aiDeleteConfirm = $state(false)
  let openMoreMenuId: string | null = $state(null)
  let moreMenuPos = $state({ top: 0, right: 0 })

  const sessionFilters = ['all', 'session', 'main', 'worker', 'archived'] as const
  const maxSnippetsPerSession = 3
  let transcriptSearchTimer: ReturnType<typeof setTimeout> | null = null
  let transcriptSearchToken = 0
  let cleanupSuggestions = $derived(cleanupCandidateSessions(sessions))
  let aiCleanupMode = $derived(sessionCleanupModeForFilter(filterKind))
  let aiCleanupSuggestions = $derived(sessionCleanupSuggestionsForMode(aiCleanupResponse, aiCleanupMode))
  let showAICleanupPanel = $derived(
    (filterKind === 'all' || filterKind === 'session' || filterKind === 'archived') && !searchQuery.trim(),
  )

  let sessionGroups = $derived(
    groupSessions(
      filteredSessions(),
      { useGroups: filterKind !== 'archived' && !searchQuery.trim() },
    )
  )

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

  function sessionCleanupModeForFilter(kind: SessionKindFilter): SessionCleanupMode {
    return kind === 'archived' ? 'delete' : 'archive'
  }

  function sessionCleanupSuggestionsForMode(
    response: SessionCleanupSuggestionResponse | null,
    mode: SessionCleanupMode,
  ): SessionCleanupSuggestion[] {
    if (!response || response.mode !== mode) return []
    return response.suggestions
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
    openMoreMenuId = null
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

  function selectedAISuggestions(): SessionCleanupSuggestion[] {
    return aiCleanupSuggestions.filter((suggestion) => aiCleanupSelected[suggestion.session_id] !== false)
  }

  async function handleAnalyzeSessionCleanup() {
    actionBusy = 'ai-cleanup'
    actionError = ''
    aiDeleteConfirm = false
    try {
      const response = await recommendSessionCleanup({ mode: aiCleanupMode, limit: 8 })
      aiCleanupResponse = response
      const selected: Record<string, boolean> = {}
      for (const suggestion of response.suggestions) {
        selected[suggestion.session_id] = true
      }
      aiCleanupSelected = selected
    } catch (e) {
      actionError = e instanceof Error ? e.message : $t.sessions.errors.aiCleanupFailed
      aiCleanupResponse = null
      aiCleanupSelected = {}
    } finally {
      actionBusy = ''
    }
  }

  async function handleApplyAISuggestions() {
    const suggestions = selectedAISuggestions()
    if (suggestions.length === 0) return
    if (aiCleanupMode === 'delete' && !aiDeleteConfirm) {
      aiDeleteConfirm = true
      return
    }
    actionBusy = 'ai-cleanup'
    actionError = ''
    try {
      if (aiCleanupMode === 'archive') {
        await Promise.all(suggestions.map((suggestion) => setSessionArchived(suggestion.session_id, true)))
      } else {
        await Promise.all(suggestions.map((suggestion) => deleteSession(suggestion.session_id)))
      }
      if (selectedSessionId && suggestions.some((suggestion) => suggestion.session_id === selectedSessionId)) onNewSession()
      aiCleanupResponse = null
      aiCleanupSelected = {}
      aiDeleteConfirm = false
      await load()
    } catch (e) {
      actionError = e instanceof Error ? e.message : $t.sessions.errors.aiCleanupFailed
    } finally {
      actionBusy = ''
    }
  }

  function toggleAISuggestion(sessionID: string) {
    aiCleanupSelected = {
      ...aiCleanupSelected,
      [sessionID]: aiCleanupSelected[sessionID] === false,
    }
    aiDeleteConfirm = false
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

  function toggleMoreMenu(id: string, e: MouseEvent) {
    e.stopPropagation()
    if (openMoreMenuId === id) {
      openMoreMenuId = null
      deleteConfirmId = null
      return
    }
    const btn = e.currentTarget as HTMLElement
    const rect = btn.getBoundingClientRect()
    moreMenuPos = { top: rect.bottom + 4, right: window.innerWidth - rect.right }
    openMoreMenuId = id
  }

  function groupLabel(group: SessionGroup): string {
    return $t.sessions.groups[group.key]
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

  $effect(() => {
    aiCleanupMode
    searchQuery.trim()
    aiCleanupResponse = null
    aiCleanupSelected = {}
    aiDeleteConfirm = false
  })

  $effect(() => {
    function handleGlobalClick() {
      if (openMoreMenuId !== null) {
        openMoreMenuId = null
        deleteConfirmId = null
      }
    }
    document.addEventListener('click', handleGlobalClick)
    return () => document.removeEventListener('click', handleGlobalClick)
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
      <button class="filter-btn" class:active={sortBy === 'updated'} onclick={() => { sortBy = 'updated' }} title={$t.sessions.sort.recentTitle}>{'↓'}</button>
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
    <details class="cleanup-panel">
      <summary class="cleanup-summary" aria-label={$t.sessions.cleanup.title}>
        <span class="cleanup-summary-title">{$t.sessions.cleanup.title}</span>
        <strong class="cleanup-summary-count">{$t.sessions.cleanup.count(cleanupSuggestions.length)}</strong>
        <span class="cleanup-chevron">›</span>
      </summary>
      <div class="cleanup-body">
        <div class="cleanup-preview">{cleanupSuggestions.slice(0, 3).map((session) => session.title || session.id.slice(0, 12)).join(' · ')}</div>
        <button class="cleanup-action" type="button" disabled={actionBusy === 'cleanup'} onclick={handleArchiveCleanupCandidates}>
          {$t.sessions.cleanup.archiveSuggested(cleanupSuggestions.length)}
        </button>
      </div>
    </details>
  {/if}

  {#if showAICleanupPanel}
    <details class="cleanup-panel cleanup-panel-ai">
      <summary class="cleanup-summary" aria-label={aiCleanupMode === 'delete' ? $t.sessions.aiCleanup.deleteTitle : $t.sessions.aiCleanup.archiveTitle}>
        <span class="cleanup-summary-title">{aiCleanupMode === 'delete' ? $t.sessions.aiCleanup.deleteTitle : $t.sessions.aiCleanup.archiveTitle}</span>
        {#if aiCleanupResponse}
          <strong class="cleanup-summary-count">{$t.sessions.aiCleanup.source(aiCleanupResponse.analyzed_count, aiCleanupResponse.excluded_count)}</strong>
        {/if}
        <span class="cleanup-chevron">›</span>
      </summary>
      <div class="cleanup-body">
        <button class="cleanup-action" type="button" disabled={actionBusy === 'ai-cleanup'} onclick={handleAnalyzeSessionCleanup}>
          {actionBusy === 'ai-cleanup' ? $t.sessions.aiCleanup.analyzing : aiCleanupMode === 'delete' ? $t.sessions.aiCleanup.analyzeDelete : $t.sessions.aiCleanup.analyzeArchive}
        </button>
        {#if aiCleanupResponse}
          {#if aiCleanupSuggestions.length === 0}
            <div class="cleanup-preview">{$t.sessions.aiCleanup.empty}</div>
          {:else}
            <div class="ai-suggestion-list">
              {#each aiCleanupSuggestions as suggestion}
                <label class="ai-suggestion-row">
                  <input
                    type="checkbox"
                    checked={aiCleanupSelected[suggestion.session_id] !== false}
                    onchange={() => toggleAISuggestion(suggestion.session_id)}
                  />
                  <span class="ai-suggestion-copy">
                    <strong>{suggestion.title || suggestion.session_id.slice(0, 12)}</strong>
                    <span>{suggestion.reason}</span>
                  </span>
                  <span class="ai-confidence">{$t.sessions.aiCleanup.confidence(suggestion.confidence)}</span>
                </label>
              {/each}
            </div>
            <button
              class="cleanup-action"
              class:ai-delete-confirm={aiCleanupMode === 'delete' && aiDeleteConfirm}
              type="button"
              disabled={actionBusy === 'ai-cleanup' || selectedAISuggestions().length === 0}
              onclick={handleApplyAISuggestions}
            >
              {#if aiCleanupMode === 'delete' && aiDeleteConfirm}
                {$t.sessions.aiCleanup.confirmDelete(selectedAISuggestions().length)}
              {:else if aiCleanupMode === 'delete'}
                {$t.sessions.aiCleanup.applyDelete(selectedAISuggestions().length)}
              {:else}
                {$t.sessions.aiCleanup.applyArchive(selectedAISuggestions().length)}
              {/if}
            </button>
          {/if}
        {/if}
      </div>
    </details>
  {/if}

  <div class="session-list">
    {#if loading}
      <div class="sidebar-loading">{$t.sessions.sidebarLoading}</div>
    {:else if filteredSessions().length === 0}
      {#if transcriptSearchLoading}
        <div class="sidebar-loading">{$t.sessions.sidebarSearchingTranscripts}</div>
      {:else}
        <div class="sidebar-empty">{searchQuery || filterKind !== 'all' ? $t.sessions.sidebarNoMatches : $t.sessions.sidebarEmpty}</div>
      {/if}
    {:else}
      {#if transcriptSearchLoading}
        <div class="sidebar-searching-hint">{$t.sessions.sidebarSearchingTranscripts}</div>
      {/if}
      {#each sessionGroups as group}
        {#if sessionGroups.length > 1}
          <div class="session-group-label">{groupLabel(group)}</div>
        {/if}
        {#each group.sessions as session}
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
                {#if isPinned(session)}
                  <span class="session-pin-indicator" title={$t.sessions.actions.unpin} aria-label={$t.sessions.actions.unpin}>★</span>
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
                <button
                  class="act-btn"
                  class:active={isPinned(session)}
                  aria-label={isPinned(session) ? $t.sessions.actions.unpin : $t.sessions.actions.pin}
                  title={isPinned(session) ? $t.sessions.actions.unpin : $t.sessions.actions.pin}
                  disabled={actionBusy === session.id}
                  onclick={(e) => { e.stopPropagation(); handleSetPinned(session.id, !isPinned(session)) }}
                >{isPinned(session) ? '★' : '☆'}</button>
              {/if}
              <button
                class="act-btn more-btn"
                aria-label={$t.sessions.actions.more}
                title={$t.sessions.actions.more}
                onclick={(e) => toggleMoreMenu(session.id, e)}
              >⋯</button>
            </div>

            {#if openMoreMenuId === session.id}
              <div
                class="session-more-menu"
                role="menu"
                tabindex="-1"
                style="top:{moreMenuPos.top}px;right:{moreMenuPos.right}px"
                onclick={(e) => e.stopPropagation()}
                onkeydown={(e) => e.stopPropagation()}
              >
                {#if !isMainSession(session)}
                  <button
                    role="menuitem"
                    class="more-menu-item"
                    onclick={(e) => { e.stopPropagation(); openMoreMenuId = null; startRename(session) }}
                  >{$t.sessions.actions.rename}</button>
                  <button
                    role="menuitem"
                    class="more-menu-item"
                    disabled={actionBusy === session.id}
                    onclick={(e) => { e.stopPropagation(); openMoreMenuId = null; void handleGenerateTitle(session) }}
                  >{$t.sessions.actions.autoTitle}</button>
                {/if}
                <button
                  role="menuitem"
                  class="more-menu-item"
                  disabled={actionBusy === session.id}
                  onclick={(e) => { e.stopPropagation(); openMoreMenuId = null; void handleCompact(session.id) }}
                >{$t.sessions.actions.compact}</button>
                {#if !isMainSession(session)}
                  <button
                    role="menuitem"
                    class="more-menu-item"
                    disabled={actionBusy === session.id}
                    onclick={(e) => { e.stopPropagation(); openMoreMenuId = null; void handleSetArchived(session.id, !isArchived(session)) }}
                  >{isArchived(session) ? $t.sessions.actions.restore : $t.sessions.actions.archive}</button>
                  <div class="more-menu-divider" role="separator"></div>
                  <button
                    role="menuitem"
                    class="more-menu-item more-menu-item-danger"
                    disabled={actionBusy === session.id}
                    onclick={(e) => { e.stopPropagation(); requestDelete(session.id) }}
                  >{deleteConfirmId === session.id ? $t.sessions.actions.confirm : $t.sessions.actions.delete}</button>
                {/if}
              </div>
            {/if}
          </div>
        {/each}
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

  /* Cleanup panels */
  .cleanup-panel {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
  }

  .cleanup-panel-ai {
    border-color: rgba(224, 145, 69, 0.22);
    background: rgba(224, 145, 69, 0.05);
  }

  .cleanup-summary {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    padding: 5px var(--space-2);
    cursor: pointer;
    list-style: none;
    user-select: none;
    color: var(--text-secondary);
    font-size: 10px;
    font-family: var(--font-mono);
    text-transform: uppercase;
  }
  .cleanup-summary::-webkit-details-marker { display: none; }
  .cleanup-summary:hover { color: var(--text-primary); }

  .cleanup-summary-title {
    flex: 1;
  }

  .cleanup-summary-count {
    color: var(--primary);
    font-weight: 600;
  }

  .cleanup-chevron {
    color: var(--text-ghost);
    font-size: 12px;
    transition: transform var(--duration-fast);
    display: inline-block;
  }

  details[open] .cleanup-chevron {
    transform: rotate(90deg);
  }

  .cleanup-body {
    display: grid;
    gap: var(--space-1);
    padding: 0 var(--space-2) var(--space-2);
  }

  .ai-suggestion-list {
    display: grid;
    gap: 3px;
  }

  .ai-suggestion-row {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    gap: var(--space-1);
    align-items: start;
    color: var(--text-secondary);
    font-size: 10px;
  }

  .ai-suggestion-row input {
    margin-top: 2px;
  }

  .ai-suggestion-copy {
    display: grid;
    gap: 1px;
    min-width: 0;
  }

  .ai-suggestion-copy strong {
    overflow: hidden;
    color: var(--text-primary);
    font-size: 10px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .ai-suggestion-copy span {
    display: -webkit-box;
    overflow: hidden;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    color: var(--text-tertiary);
    line-height: 1.25;
  }

  .ai-confidence {
    color: var(--primary);
    font-family: var(--font-mono);
    font-size: 9px;
  }

  .ai-delete-confirm {
    border-color: var(--error);
    color: var(--error);
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

  /* Session list */
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

  .sidebar-searching-hint {
    padding: 3px var(--space-2) 2px;
    font-size: 10px;
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-style: italic;
  }

  /* Group labels */
  .session-group-label {
    padding: 6px var(--space-2) 2px;
    font-size: 9px;
    font-family: var(--font-mono);
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-ghost);
  }

  .session-item {
    position: relative;
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
    gap: var(--space-1);
    flex-wrap: nowrap;
  }

  .session-time {
    font-size: 10px;
    color: var(--text-ghost);
    margin-left: auto;
  }

  .session-pin-indicator {
    font-size: 10px;
    color: var(--primary);
    line-height: 1;
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

  /* Action area: pin + more button */
  .session-actions {
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 1px;
    padding: 0 2px;
    opacity: 0;
    transition: opacity var(--duration-fast);
  }
  .session-item:hover .session-actions,
  .session-item:focus-within .session-actions {
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

  .more-btn {
    letter-spacing: 0.05em;
  }

  /* More menu dropdown */
  .session-more-menu {
    position: fixed;
    z-index: 200;
    min-width: 130px;
    background: var(--surface-elevated);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.4);
    padding: 3px 0;
    display: flex;
    flex-direction: column;
  }

  .more-menu-item {
    background: none;
    border: none;
    color: var(--text-secondary);
    cursor: pointer;
    font-size: var(--text-xs);
    font-family: var(--font-body);
    padding: 5px var(--space-2);
    text-align: left;
    white-space: nowrap;
    transition: background var(--duration-fast), color var(--duration-fast);
  }
  .more-menu-item:hover:not(:disabled) {
    background: var(--surface-hover);
    color: var(--text-primary);
  }
  .more-menu-item:disabled {
    color: var(--text-ghost);
    cursor: not-allowed;
  }

  .more-menu-item-danger {
    color: var(--text-secondary);
  }
  .more-menu-item-danger:hover:not(:disabled) {
    color: var(--error);
    background: var(--error-muted);
  }

  .more-menu-divider {
    height: 1px;
    background: var(--border-subtle);
    margin: 2px 0;
  }

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
