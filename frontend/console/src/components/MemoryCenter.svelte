<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { locale, t } from '../i18n'
  import {
    getMemoryFile,
    listMemoryInbox,
    listMemoryAssets,
    reviewMemoryCandidate,
    runMemoryPrefetch,
    runMemorySearch,
    saveMemoryFile,
    streamEvents,
  } from '../lib/api'
  import type {
    MemoryAsset,
    MemoryCandidate,
    MemoryCandidateAction,
    MemoryCandidateHint,
    MemoryPrefetchResult,
    MemorySearchResult,
  } from '../lib/types'
  import {
    getMemoryAssetMetadata,
    isMemoryAssetStale,
  } from '../lib/memoryAssetMetadata'

  interface Props {
    onAskAI?: (prompt: string) => void
  }

  let { onAskAI }: Props = $props()

  const MEMORY_INTRO_STORAGE_KEY = 'tars.memory.intro.dismissed'
  type MemorySearchMode = 'tool' | 'prefetch'
  type MemoryTab = 'inbox' | 'durable' | 'search'

  let activeTab = $state<MemoryTab>('inbox')
  let error = $state('')
  let success = $state('')
  let stopStream: (() => void) | null = null
  let memoryIntroDismissed = $state(false)

  let memoryCandidates: MemoryCandidate[] = $state([])
  let loadingInbox = $state(true)
  let reviewingCandidate = $state('')

  let memoryAssets: MemoryAsset[] = $state([])
  let loadingMemory = $state(true)
  let savingMemory = $state(false)
  let selectedMemoryPath = $state('')
  let selectedMemoryKind = $state('')
  let memoryEditorContent = $state('')
  let memoryUpdatedAt = $state('')
  let memorySizeBytes = $state(0)

  let searchQueryInput = $state('')
  let searchMode = $state<MemorySearchMode>('tool')
  let prefetchSessionId = $state('')
  let searchLimit = $state(8)
  let searchIncludeMemory = $state(true)
  let searchIncludeDaily = $state(true)
  let searchIncludeSessions = $state(true)
  let searching = $state(false)
  let searchResult: MemorySearchResult | null = $state(null)
  let prefetchResult: MemoryPrefetchResult | null = $state(null)

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '-'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat($locale, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function formatBytes(size = 0): string {
    if (size <= 0) return '0 B'
    if (size < 1024) return `${size} B`
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
    return `${(size / (1024 * 1024)).toFixed(1)} MB`
  }

  function assetKindLabel(kind: string): string {
    switch (kind) {
      case 'long_term_memory':
        return 'MEMORY.md'
      case 'experience_log':
        return $t.memory.introHeadings.experiences
      case 'memory_inbox':
        return $t.memory.tabs.inbox
      case 'daily_memory':
        return $t.memory.introHeadings.dailyLogs
      case 'semantic_index':
        return $t.memory.introHeadings.semanticIndex
      case 'semantic_raw':
        return 'Semantic Raw'
      default:
        return kind || $t.memory.title
    }
  }

  function sourceClass(source_tag: string): string {
    return source_tag.trim().toLowerCase().replace(/[^a-z0-9_-]/g, '-') || 'context'
  }

  function currentSearchHitCount(): number {
    if (searchMode === 'prefetch') {
      return prefetchResult?.items?.length ?? 0
    }
    return searchResult?.results?.length ?? 0
  }

  function pendingCandidateCount(): number {
    return memoryCandidates.filter((candidate) => candidate.status === 'pending').length
  }

  function currentSearchMeta(): string {
    if (searchMode === 'prefetch') {
      if (!prefetchResult) return $t.memory.search.prefetchEmpty
      return `${prefetchResult.relevant_tokens.toLocaleString()} / ${prefetchResult.relevant_budget_tokens.toLocaleString()} tokens (${prefetchResult.budget_percent}%)`
    }
    return searchResult?.message || $t.memory.search.defaultToolMeta
  }

  function candidateProvenance(candidate: MemoryCandidate): string {
    const provenance = candidate.provenance
    const parts = [
      provenance?.source || 'memory',
      provenance?.session_id || candidate.source_session || '',
      provenance?.message_range || '',
    ].filter(Boolean)
    return parts.join(' · ') || '-'
  }

  function bestMergeTarget(candidate: MemoryCandidate): string {
    return candidate.similar?.[0]?.summary || ''
  }

  function formatHintScore(hint: MemoryCandidateHint): string {
    if (typeof hint.score !== 'number' || Number.isNaN(hint.score)) return ''
    return `${Math.round(hint.score * 100)}%`
  }

  async function loadMemoryInbox() {
    loadingInbox = true
    try {
      const res = await listMemoryInbox('pending')
      memoryCandidates = res.items || []
    } finally {
      loadingInbox = false
    }
  }

  async function loadMemory(path?: string) {
    loadingMemory = true
    try {
      const assetsRes = await listMemoryAssets()
      memoryAssets = assetsRes.items || []
      const target = path || selectedMemoryPath || memoryAssets[0]?.path || 'MEMORY.md'
      if (target) {
        await selectMemoryAsset(target)
      }
    } finally {
      loadingMemory = false
    }
  }

  async function selectMemoryAsset(path: string) {
    const file = await getMemoryFile(path)
    selectedMemoryPath = file.path
    selectedMemoryKind = file.kind
    memoryEditorContent = file.content
    memoryUpdatedAt = file.updated_at || ''
    memorySizeBytes = file.size_bytes || 0
  }

  async function reviewCandidate(candidate: MemoryCandidate, action: MemoryCandidateAction) {
    if (!candidate.id || reviewingCandidate) return
    reviewingCandidate = candidate.id
    error = ''
    success = ''
    try {
      const mergeTarget = action === 'merge' ? bestMergeTarget(candidate) : ''
      await reviewMemoryCandidate(candidate.id, action, mergeTarget)
      await Promise.all([loadMemoryInbox(), loadMemory(selectedMemoryPath)])
      if (action === 'approve') success = $t.memory.inbox.approved
      if (action === 'reject') success = $t.memory.inbox.rejected
      if (action === 'merge') success = $t.memory.inbox.merged
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to review memory candidate'
    } finally {
      reviewingCandidate = ''
    }
  }

  async function saveSelectedMemoryAsset() {
    if (!selectedMemoryPath.trim()) return
    savingMemory = true
    error = ''
    success = ''
    try {
      const file = await saveMemoryFile(selectedMemoryPath, memoryEditorContent)
      selectedMemoryPath = file.path
      selectedMemoryKind = file.kind
      memoryUpdatedAt = file.updated_at || ''
      memorySizeBytes = file.size_bytes || 0
      await loadMemory(selectedMemoryPath)
      success = `${selectedMemoryPath} updated.`
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to save memory file'
    } finally {
      savingMemory = false
    }
  }

  async function runSearchTest() {
    if (!searchQueryInput.trim()) return
    searching = true
    error = ''
    success = ''
    try {
      if (searchMode === 'prefetch') {
        prefetchResult = await runMemoryPrefetch({
          query: searchQueryInput.trim(),
          session_id: prefetchSessionId.trim() || undefined,
        })
      } else {
        searchResult = await runMemorySearch({
          query: searchQueryInput.trim(),
          limit: searchLimit,
          include_memory: searchIncludeMemory,
          include_daily: searchIncludeDaily,
          include_sessions: searchIncludeSessions,
        })
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to run memory search'
    } finally {
      searching = false
    }
  }

  async function loadAll() {
    error = ''
    try {
      await Promise.all([loadMemory(selectedMemoryPath), loadMemoryInbox()])
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load memory console'
    }
  }

  function applyRouteSearchParams() {
    const params = new URLSearchParams(window.location.search)
    const query = (params.get('q') || params.get('query') || '').trim()
    const tab = params.get('tab')
    if (tab === 'durable' || tab === 'inbox') {
      activeTab = tab
    }
    if (tab === 'search' || query) {
      activeTab = 'search'
    }
    if (query) {
      searchQueryInput = query
    }
  }

  function askAIPrompt(): string {
    switch (activeTab) {
      case 'inbox':
        return 'Help me review these memory candidates and decide what should be approved: '
      case 'search':
        return 'Help me review this memory search query and improve recall: '
      default:
        return 'Review these stored knowledge assets and suggest improvements: '
    }
  }

  function loadMemoryIntroPreference() {
    try {
      memoryIntroDismissed = window.localStorage.getItem(MEMORY_INTRO_STORAGE_KEY) === 'true'
    } catch {
      memoryIntroDismissed = false
    }
  }

  function dismissMemoryIntro() {
    memoryIntroDismissed = true
    try {
      window.localStorage.setItem(MEMORY_INTRO_STORAGE_KEY, 'true')
    } catch {
      // Ignore storage failures; the current session still hides the intro.
    }
  }

  onMount(() => {
    loadMemoryIntroPreference()
    applyRouteSearchParams()
    void loadAll()
    stopStream = streamEvents(
      (event) => {
        if (event.category === 'chat') {
          void loadAll()
        }
      },
    )
  })

  onDestroy(() => {
    stopStream?.()
  })
</script>

<div class="memory-page">
  <div class="page-header">
    <div>
      <h2>{$t.memory.title}</h2>
      <p class="page-subtitle">{$t.memory.assetCount(memoryAssets.length)}</p>
    </div>
    <div class="page-actions">
      {#if onAskAI}
        <button class="btn btn-ghost btn-sm" type="button" onclick={() => onAskAI(askAIPrompt())}>
          {$t.common.actions.askAI}
        </button>
      {/if}
      <button class="btn btn-ghost btn-sm" type="button" onclick={loadAll}>{$t.common.actions.refresh}</button>
    </div>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}
  {#if success}
    <div class="success-banner">{success}</div>
  {/if}

  {#if !memoryIntroDismissed}
    <section class="memory-intro-card" aria-label={$t.memory.introTitle}>
      <div class="intro-main">
        <div>
          <p class="intro-eyebrow">{$t.memory.introLabel}</p>
          <h3>{$t.memory.introTitle}</h3>
          <p>{$t.memory.introBody}</p>
        </div>
        <button class="btn btn-ghost btn-sm intro-dismiss" type="button" aria-label={$t.common.actions.dismiss} title={$t.common.actions.dismiss} onclick={dismissMemoryIntro}>{$t.common.actions.dismiss}</button>
      </div>
      <div class="intro-grid">
        <div class="intro-item">
          <strong>{$t.memory.introHeadings.memory}</strong>
          <span>{$t.memory.introItems.memory}</span>
        </div>
        <div class="intro-item">
          <strong>{$t.memory.introHeadings.experiences}</strong>
          <span>{$t.memory.introItems.experiences}</span>
        </div>
        <div class="intro-item">
          <strong>{$t.memory.introHeadings.dailyLogs}</strong>
          <span>{$t.memory.introItems.dailyLogs}</span>
        </div>
        <div class="intro-item">
          <strong>{$t.memory.introHeadings.semanticIndex}</strong>
          <span>{$t.memory.introItems.semanticIndex}</span>
        </div>
      </div>
      <p class="intro-footer">{$t.memory.introFooter}</p>
    </section>
  {/if}

  <div class="memory-stats">
    <div class="stat-card">
      <span class="stat-label">{$t.memory.stats.inbox}</span>
      <strong class="stat-value">{$t.memory.stats.pending(pendingCandidateCount())}</strong>
      <span class="stat-meta">{$t.memory.inbox.subtitle}</span>
    </div>
    <div class="stat-card">
      <span class="stat-label">{$t.memory.stats.selectedAsset}</span>
      <strong class="stat-value">{selectedMemoryPath || 'MEMORY.md'}</strong>
      <span class="stat-meta">{assetKindLabel(selectedMemoryKind)}</span>
    </div>
    <div class="stat-card">
      <span class="stat-label">{$t.memory.stats.search}</span>
      <strong class="stat-value">{$t.memory.stats.hits(currentSearchHitCount())}</strong>
      <span class="stat-meta">{currentSearchMeta()}</span>
    </div>
  </div>

  <div class="tab-row">
    <button class="tab-btn" class:active={activeTab === 'inbox'} type="button" onclick={() => { activeTab = 'inbox' }}>{$t.memory.tabs.inbox}</button>
    <button class="tab-btn" class:active={activeTab === 'durable'} type="button" onclick={() => { activeTab = 'durable' }}>{$t.memory.tabs.storedKnowledge}</button>
    <button class="tab-btn" class:active={activeTab === 'search'} type="button" onclick={() => { activeTab = 'search' }}>{$t.memory.tabs.trySearch}</button>
  </div>

  {#if activeTab === 'inbox'}
    <section class="inbox-panel">
      <div class="panel-header">
        <div>
          <span class="card-title">{$t.memory.inbox.title}</span>
          <div class="panel-subtitle">{$t.memory.inbox.subtitle}</div>
        </div>
        <button class="btn btn-ghost btn-sm" type="button" onclick={loadMemoryInbox}>{$t.common.actions.reload}</button>
      </div>
      {#if loadingInbox}
        <div class="empty-state">{$t.memory.inbox.loading}</div>
      {:else if memoryCandidates.length === 0}
        <div class="empty-state">{$t.memory.inbox.empty}</div>
      {:else}
        <div class="candidate-list">
          {#each memoryCandidates as candidate}
            <article class="candidate-row">
              <div class="candidate-main">
                <div>
                  <div class="candidate-meta">
                    <span class="note-kind">{candidate.category}</span>
                    <span>{fmt(candidate.created_at)}</span>
                    {#if candidate.importance}
                      <span>importance {candidate.importance}</span>
                    {/if}
                  </div>
                  <p class="candidate-summary">{candidate.summary}</p>
                </div>
                <div class="candidate-actions">
                  <button class="btn btn-primary btn-sm" type="button" disabled={reviewingCandidate === candidate.id} onclick={() => reviewCandidate(candidate, 'approve')}>
                    {reviewingCandidate === candidate.id ? $t.memory.inbox.reviewing : $t.memory.inbox.approve}
                  </button>
                  <button class="btn btn-ghost btn-sm" type="button" disabled={reviewingCandidate === candidate.id} onclick={() => reviewCandidate(candidate, 'reject')}>{$t.memory.inbox.reject}</button>
                  <button class="btn btn-ghost btn-sm" type="button" disabled={reviewingCandidate === candidate.id || !bestMergeTarget(candidate)} title={bestMergeTarget(candidate) ? $t.memory.inbox.mergedInto(bestMergeTarget(candidate)) : $t.memory.inbox.similar} onclick={() => reviewCandidate(candidate, 'merge')}>{$t.memory.inbox.merge}</button>
                </div>
              </div>
              <div class="candidate-source">
                <strong>{$t.memory.inbox.provenance}</strong>
                <span>{candidateProvenance(candidate)}</span>
              </div>
              {#if candidate.provenance?.source_summary}
                <p class="candidate-source-summary">{candidate.provenance.source_summary}</p>
              {/if}
              {#if candidate.similar?.length || candidate.conflicts?.length}
                <div class="hint-grid">
                  {#if candidate.similar?.length}
                    <div class="hint-group">
                      <strong>{$t.memory.inbox.similar}</strong>
                      {#each candidate.similar as hint}
                        <div class="hint-row">
                          <span>{hint.summary}</span>
                          <em>{formatHintScore(hint)}</em>
                        </div>
                      {/each}
                    </div>
                  {/if}
                  {#if candidate.conflicts?.length}
                    <div class="hint-group conflict">
                      <strong>{$t.memory.inbox.conflicts}</strong>
                      {#each candidate.conflicts as hint}
                        <div class="hint-row">
                          <span>{hint.summary}</span>
                          <em>{hint.reason || formatHintScore(hint)}</em>
                        </div>
                      {/each}
                    </div>
                  {/if}
                </div>
              {/if}
            </article>
          {/each}
        </div>
      {/if}
    </section>
  {:else if activeTab === 'durable'}
    <div class="memory-layout">
      <aside class="assets-panel card">
        <div class="panel-header">
          <span class="card-title">{$t.memory.assets}</span>
        </div>
        {#if loadingMemory}
          <div class="empty-state">{$t.memory.loadingAssets}</div>
        {:else if memoryAssets.length === 0}
          <div class="empty-state">{$t.memory.emptyAssets}</div>
        {:else}
          <div class="asset-list">
            {#each memoryAssets as asset}
              {@const metadata = getMemoryAssetMetadata(asset)}
              {@const stale = isMemoryAssetStale(asset)}
              <button class="asset-row" class:active={selectedMemoryPath === asset.path} type="button" title={metadata.description} onclick={() => selectMemoryAsset(asset.path)}>
                <div class="asset-row-top">
                  <strong>{asset.path}</strong>
                  <div class="asset-badges">
                    <span class="note-kind">{assetKindLabel(asset.kind)}</span>
                    {#if stale}
                      <span class="stale-badge">{$t.memory.stale}</span>
                    {/if}
                  </div>
                </div>
                <div class="note-meta">
                  <span>{formatBytes(asset.size_bytes)}</span>
                  <span>{fmt(asset.updated_at)}</span>
                </div>
                <p class="asset-description">{metadata.description}</p>
                <div class="asset-flow">
                  <div class="asset-flow-line">
                    <strong>{$t.memory.filledBy}</strong>
                    <span>{metadata.filledBy.join(', ')}</span>
                  </div>
                  <div class="asset-flow-line">
                    <strong>{$t.memory.readBy}</strong>
                    <span>{metadata.readBy.join(', ')}</span>
                  </div>
                </div>
              </button>
            {/each}
          </div>
        {/if}
      </aside>

      <section class="editor-panel card">
        <div class="panel-header">
          <div>
            <span class="card-title">{$t.memory.editor}</span>
            <div class="panel-subtitle">{selectedMemoryPath || $t.memory.selectFile}</div>
          </div>
          <div class="editor-actions">
            <button class="btn btn-ghost btn-sm" type="button" onclick={() => selectedMemoryPath && selectMemoryAsset(selectedMemoryPath)}>{$t.common.actions.reload}</button>
            <button class="btn btn-primary btn-sm" type="button" disabled={!selectedMemoryPath || savingMemory} onclick={saveSelectedMemoryAsset}>
              {savingMemory ? $t.common.actions.saving : $t.common.actions.save}
            </button>
          </div>
        </div>
        <div class="editor-meta">
          <span>{assetKindLabel(selectedMemoryKind)}</span>
          <span>{formatBytes(memorySizeBytes)}</span>
          <span>{fmt(memoryUpdatedAt)}</span>
        </div>
        <textarea class="memory-editor" bind:value={memoryEditorContent} placeholder={$t.memory.placeholders.editor}></textarea>
      </section>
    </div>
  {:else if activeTab === 'search'}
    <div class="search-layout">
      <section class="card search-panel">
        <div class="panel-header">
          <span class="card-title">{$t.memory.search.title}</span>
          <button class="btn btn-primary btn-sm" type="button" disabled={!searchQueryInput.trim() || searching} onclick={runSearchTest}>
            {searching ? $t.memory.search.running : searchMode === 'prefetch' ? $t.memory.search.runPrefetch : $t.memory.search.runSearch}
          </button>
        </div>
        <div class="mode-toggle" role="group" aria-label={$t.memory.search.modeLabel}>
          <button class="mode-btn" class:active={searchMode === 'tool'} type="button" onclick={() => { searchMode = 'tool' }}>{$t.memory.search.toolPath}</button>
          <button class="mode-btn" class:active={searchMode === 'prefetch'} type="button" onclick={() => { searchMode = 'prefetch' }}>{$t.memory.search.prefetchPath}</button>
        </div>
        <div class="form-grid">
          <label class="form-field form-span-2">
            <span>{$t.memory.search.query}</span>
            <input class="form-input" bind:value={searchQueryInput} placeholder={$t.memory.placeholders.query} />
          </label>
          {#if searchMode === 'tool'}
            <label class="form-field">
              <span>{$t.memory.search.limit}</span>
              <input class="form-input" type="number" min="1" max="30" bind:value={searchLimit} />
            </label>
          {:else}
            <label class="form-field">
              <span>{$t.memory.search.sessionId}</span>
              <input class="form-input" bind:value={prefetchSessionId} placeholder={$t.memory.placeholders.sessionId} />
            </label>
          {/if}
        </div>
        {#if searchMode === 'tool'}
          <div class="search-flags">
            <label><input type="checkbox" bind:checked={searchIncludeMemory} /> MEMORY.md</label>
            <label><input type="checkbox" bind:checked={searchIncludeDaily} /> {$t.memory.search.dailyLogs}</label>
            <label><input type="checkbox" bind:checked={searchIncludeSessions} /> {$t.memory.search.sessionHistory}</label>
          </div>
        {/if}
      </section>

      <section class="card results-panel">
        <div class="panel-header">
          <span class="card-title">{$t.memory.search.results}</span>
          {#if searchMode === 'prefetch' && prefetchResult}
            <span class="note-meta">{$t.memory.search.matches(prefetchResult.items?.length ?? 0)}</span>
          {:else if searchResult}
            <span class="note-meta">{$t.memory.search.matches(searchResult.results?.length ?? 0)}</span>
          {/if}
        </div>
        {#if searchMode === 'prefetch'}
          {#if !prefetchResult}
            <div class="empty-state">{$t.memory.search.prefetchEmpty}</div>
          {:else}
            <div class="prefetch-summary">
              <span>{prefetchResult.relevant_tokens.toLocaleString()} / {prefetchResult.relevant_budget_tokens.toLocaleString()} tokens</span>
              <span>{prefetchResult.budget_percent}% budget</span>
              {#if prefetchResult.session_id}
                <span>{prefetchResult.session_id}</span>
              {/if}
            </div>
            {#if prefetchResult.items.length === 0}
              <div class="empty-state">{prefetchResult.message || $t.memory.search.noPriorMatches}</div>
            {:else}
              <div class="result-list">
                {#each prefetchResult.items as item}
                  <article class="result-row">
                    <div class="result-meta">
                      <strong><span class="source-badge tag-{sourceClass(item.source_tag)}">{item.source_tag}</span> {item.source}</strong>
                      <span>{item.tokens.toLocaleString()} tokens</span>
                    </div>
                    <p>{item.snippet}</p>
                  </article>
                {/each}
              </div>
            {/if}
            <details class="prefetch-section" open>
              <summary>{$t.memory.search.priorContext}</summary>
              <pre>{prefetchResult.section || '## Prior Context\n\n'}</pre>
            </details>
          {/if}
        {:else if !searchResult}
          <div class="empty-state">{$t.memory.search.searchEmpty}</div>
        {:else if !searchResult.results || searchResult.results.length === 0}
          <div class="empty-state">{searchResult.message || $t.memory.search.noMatches}</div>
        {:else}
          <div class="result-list">
            {#each searchResult.results as item}
              <article class="result-row">
                <div class="result-meta">
                  <strong>{item.source}</strong>
                  <span>{item.date}{item.line ? ` · line ${item.line}` : ''}</span>
                </div>
                <p>{item.snippet}</p>
              </article>
            {/each}
          </div>
        {/if}
      </section>
    </div>
  {/if}
</div>

<style>
  .memory-page {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    overflow-y: auto;
  }

  .page-header,
  .panel-header,
  .note-meta,
  .editor-actions,
  .asset-row-top,
  .result-meta {
    display: flex;
    justify-content: space-between;
    gap: var(--space-2);
    align-items: center;
  }

  .page-header {
    align-items: flex-start;
    gap: var(--space-4);
  }

  .page-subtitle,
  .panel-subtitle {
    margin-top: var(--space-1);
    color: var(--text-secondary);
  }

  .page-actions,
  .tab-row,
  .mode-toggle,
  .search-flags {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .memory-intro-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    padding: var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--surface);
  }

  .intro-main {
    display: flex;
    justify-content: space-between;
    gap: var(--space-4);
    align-items: flex-start;
  }

  .intro-main h3 {
    margin: 0;
    font-size: var(--text-lg);
  }

  .intro-main p,
  .intro-footer,
  .intro-item span {
    color: var(--text-secondary);
  }

  .intro-eyebrow {
    margin: 0 0 var(--space-1);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0;
    color: var(--text-ghost);
  }

  .intro-main p:not(.intro-eyebrow),
  .intro-footer {
    margin: var(--space-2) 0 0;
    line-height: 1.55;
  }

  .intro-grid {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .intro-item {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding-left: var(--space-3);
    border-left: 2px solid var(--border-subtle);
    min-width: 0;
  }

  .intro-item strong {
    color: var(--text-primary);
  }

  .intro-item span {
    font-size: var(--text-sm);
    line-height: 1.45;
  }

  .intro-dismiss {
    flex: 0 0 auto;
  }

  .memory-stats {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--space-4);
  }

  .stat-card,
  .inbox-panel,
  .editor-panel,
  .assets-panel,
  .search-panel,
  .results-panel {
    padding: var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--surface);
  }

  .stat-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .stat-label,
  .form-field span {
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--text-ghost);
  }

  .stat-value {
    font-size: var(--text-md);
  }

  .stat-meta,
  .note-kind,
  .note-meta,
  .asset-description,
  .asset-flow-line {
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .asset-description {
    line-height: 1.45;
  }

  .tab-btn {
    border-radius: var(--radius-pill);
    padding: 8px 12px;
    border: 1px solid var(--border-subtle);
    background: var(--surface);
    color: var(--text-secondary);
    font: inherit;
  }

  .tab-btn.active {
    border-color: var(--primary);
    background: var(--primary-muted);
    color: var(--primary-text);
  }

  .mode-toggle {
    padding: 4px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-base);
  }

  .mode-btn {
    border: 0;
    border-radius: var(--radius-sm);
    padding: 8px 10px;
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    cursor: pointer;
  }

  .mode-btn.active {
    background: var(--primary-muted);
    color: var(--primary-text);
  }

  .memory-layout {
    display: grid;
    grid-template-columns: minmax(280px, 360px) minmax(0, 1fr);
    gap: var(--space-4);
    min-height: 520px;
  }

  .search-layout {
    display: grid;
    grid-template-columns: minmax(320px, 420px) minmax(0, 1fr);
    gap: var(--space-4);
  }

  .editor-panel,
  .assets-panel,
  .inbox-panel,
  .search-panel,
  .results-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .asset-list,
  .candidate-list,
  .result-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    overflow: auto;
  }

  .asset-row,
  .candidate-row,
  .result-row {
    text-align: left;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-base);
    padding: var(--space-3);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .asset-row,
  .tab-btn,
  .mode-btn {
    cursor: pointer;
    transition: border-color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
  }

  .asset-row:hover,
  .asset-row.active {
    border-color: var(--primary);
    background: var(--primary-muted);
  }

  .asset-badges,
  .asset-flow-line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .asset-badges {
    justify-content: flex-end;
  }

  .asset-flow {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .asset-flow-line strong {
    color: var(--text-primary);
  }

  .candidate-main,
  .candidate-source,
  .candidate-meta,
  .candidate-actions,
  .hint-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .candidate-main {
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .candidate-main > div:first-child {
    min-width: 0;
  }

  .candidate-actions {
    flex: 0 0 auto;
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .candidate-meta,
  .candidate-source,
  .hint-row {
    flex-wrap: wrap;
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }

  .candidate-summary,
  .candidate-source-summary {
    margin: 0;
    line-height: 1.5;
  }

  .candidate-summary {
    margin-top: var(--space-2);
    color: var(--text-primary);
    font-weight: 600;
    word-break: break-word;
  }

  .candidate-source strong,
  .hint-group strong {
    color: var(--text-primary);
  }

  .candidate-source-summary {
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .hint-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .hint-group {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding-top: var(--space-2);
    border-top: 1px solid var(--border-subtle);
  }

  .hint-group.conflict {
    color: var(--warning);
  }

  .hint-row {
    justify-content: space-between;
    align-items: flex-start;
  }

  .hint-row span {
    min-width: 0;
    color: var(--text-secondary);
  }

  .hint-row em {
    flex: 0 0 auto;
    color: var(--text-ghost);
    font-style: normal;
  }

  .stale-badge {
    padding: 2px 6px;
    border-radius: var(--radius-sm);
    background: var(--warning-muted);
    color: var(--warning);
    font-size: var(--text-xs);
    font-weight: 600;
  }

  .editor-meta {
    display: flex;
    gap: var(--space-3);
    align-items: center;
    flex-wrap: wrap;
  }

  .form-input,
  .memory-editor {
    width: 100%;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
    color: var(--text-primary);
    padding: 10px 12px;
    font: inherit;
  }

  .form-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .form-field {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .form-span-2 {
    grid-column: span 2;
  }

  .memory-editor {
    min-height: 480px;
    resize: vertical;
    font-family: var(--font-mono);
    line-height: 1.55;
  }

  .search-flags label {
    display: inline-flex;
    gap: 8px;
    align-items: center;
    padding: 8px 10px;
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
    color: var(--text-secondary);
  }

  .prefetch-summary {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .source-badge {
    display: inline-block;
    border-radius: var(--radius-sm);
    padding: 2px 6px;
    margin-right: var(--space-1);
    font-size: var(--text-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0;
    color: var(--text-primary);
    background: rgba(99, 102, 241, 0.14);
    border: 1px solid rgba(99, 102, 241, 0.24);
  }

  .tag-experience {
    background: rgba(34, 197, 94, 0.14);
    border-color: rgba(34, 197, 94, 0.24);
  }

  .tag-project {
    background: rgba(14, 165, 233, 0.14);
    border-color: rgba(14, 165, 233, 0.24);
  }

  .tag-daily {
    background: rgba(245, 158, 11, 0.14);
    border-color: rgba(245, 158, 11, 0.24);
  }

  .tag-conversation {
    background: rgba(168, 85, 247, 0.14);
    border-color: rgba(168, 85, 247, 0.24);
  }

  .prefetch-section {
    border-top: 1px solid var(--border-subtle);
    padding-top: var(--space-2);
  }

  .prefetch-section summary {
    cursor: pointer;
    color: var(--text-secondary);
    font-size: var(--text-sm);
    font-weight: 600;
  }

  .prefetch-section pre {
    max-height: 300px;
    overflow: auto;
    margin: var(--space-2) 0 0;
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-base);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    white-space: pre-wrap;
    word-break: break-word;
  }

  .error-banner,
  .success-banner,
  .empty-state {
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-md);
  }

  .error-banner {
    background: var(--error-muted);
    color: var(--error);
  }

  .success-banner {
    background: var(--success-muted);
    color: var(--success);
  }

  .empty-state {
    background: var(--surface-elevated);
    color: var(--text-secondary);
  }

  @media (max-width: 1024px) {
    .memory-stats,
    .memory-layout,
    .search-layout,
    .hint-grid,
    .form-grid,
    .intro-grid {
      grid-template-columns: 1fr;
    }

    .candidate-main {
      flex-direction: column;
    }

    .candidate-actions {
      justify-content: flex-start;
    }

    .form-span-2 {
      grid-column: span 1;
    }

    .intro-main {
      flex-direction: column;
    }
  }
</style>
