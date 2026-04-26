<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import {
    getMemoryFile,
    listMemoryAssets,
    runMemorySearch,
    saveMemoryFile,
    streamEvents,
  } from '../lib/api'
  import type {
    MemoryAsset,
    MemorySearchResult,
  } from '../lib/types'

  interface Props {
    onAskAI?: (prompt: string) => void
  }

  let { onAskAI }: Props = $props()

  let activeTab = $state<'durable' | 'search'>('durable')
  let error = $state('')
  let success = $state('')
  let stopStream: (() => void) | null = null

  let memoryAssets: MemoryAsset[] = $state([])
  let loadingMemory = $state(true)
  let savingMemory = $state(false)
  let selectedMemoryPath = $state('')
  let selectedMemoryKind = $state('')
  let memoryEditorContent = $state('')
  let memoryUpdatedAt = $state('')
  let memorySizeBytes = $state(0)

  let searchQueryInput = $state('')
  let searchLimit = $state(8)
  let searchIncludeMemory = $state(true)
  let searchIncludeDaily = $state(true)
  let searchIncludeSessions = $state(true)
  let searching = $state(false)
  let searchResult: MemorySearchResult | null = $state(null)

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '-'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
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
        return 'Experiences'
      case 'daily_memory':
        return 'Daily Memory'
      case 'semantic_index':
        return 'Semantic Index'
      case 'semantic_raw':
        return 'Semantic Raw'
      default:
        return kind || 'Memory'
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
      searchResult = await runMemorySearch({
        query: searchQueryInput.trim(),
        limit: searchLimit,
        include_memory: searchIncludeMemory,
        include_daily: searchIncludeDaily,
        include_sessions: searchIncludeSessions,
      })
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to run memory search'
    } finally {
      searching = false
    }
  }

  async function loadAll() {
    error = ''
    try {
      await loadMemory(selectedMemoryPath)
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load memory console'
    }
  }

  function askAIPrompt(): string {
    switch (activeTab) {
      case 'search':
        return 'Help me debug this memory search query and improve recall: '
      default:
        return 'Review these durable memory files and suggest improvements: '
    }
  }

  onMount(() => {
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
      <h2>Memory</h2>
      <p class="page-subtitle">{memoryAssets.length} durable assets</p>
    </div>
    <div class="page-actions">
      {#if onAskAI}
        <button class="btn btn-ghost btn-sm" type="button" onclick={() => onAskAI(askAIPrompt())}>
          Ask AI
        </button>
      {/if}
      <button class="btn btn-ghost btn-sm" type="button" onclick={loadAll}>Refresh</button>
    </div>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}
  {#if success}
    <div class="success-banner">{success}</div>
  {/if}

  <div class="memory-stats">
    <div class="stat-card">
      <span class="stat-label">Selected Asset</span>
      <strong class="stat-value">{selectedMemoryPath || 'MEMORY.md'}</strong>
      <span class="stat-meta">{assetKindLabel(selectedMemoryKind)}</span>
    </div>
    <div class="stat-card">
      <span class="stat-label">Memory Search</span>
      <strong class="stat-value">{searchResult?.results?.length ?? 0} hits</strong>
      <span class="stat-meta">{searchResult?.message || 'Run a query to validate recall.'}</span>
    </div>
  </div>

  <div class="tab-row">
    <button class="tab-btn" class:active={activeTab === 'durable'} type="button" onclick={() => { activeTab = 'durable' }}>Durable Memory</button>
    <button class="tab-btn" class:active={activeTab === 'search'} type="button" onclick={() => { activeTab = 'search' }}>Search Test</button>
  </div>

  {#if activeTab === 'durable'}
    <div class="memory-layout">
      <aside class="assets-panel card">
        <div class="panel-header">
          <span class="card-title">Assets</span>
        </div>
        {#if loadingMemory}
          <div class="empty-state">Loading durable memory assets...</div>
        {:else if memoryAssets.length === 0}
          <div class="empty-state">No durable memory assets found.</div>
        {:else}
          <div class="asset-list">
            {#each memoryAssets as asset}
              <button class="asset-row" class:active={selectedMemoryPath === asset.path} type="button" onclick={() => selectMemoryAsset(asset.path)}>
                <div class="asset-row-top">
                  <strong>{asset.path}</strong>
                  <span class="note-kind">{assetKindLabel(asset.kind)}</span>
                </div>
                <div class="note-meta">
                  <span>{formatBytes(asset.size_bytes)}</span>
                  <span>{fmt(asset.updated_at)}</span>
                </div>
              </button>
            {/each}
          </div>
        {/if}
      </aside>

      <section class="editor-panel card">
        <div class="panel-header">
          <div>
            <span class="card-title">Editor</span>
            <div class="panel-subtitle">{selectedMemoryPath || 'Select a memory file'}</div>
          </div>
          <div class="editor-actions">
            <button class="btn btn-ghost btn-sm" type="button" onclick={() => selectedMemoryPath && selectMemoryAsset(selectedMemoryPath)}>Reload</button>
            <button class="btn btn-primary btn-sm" type="button" disabled={!selectedMemoryPath || savingMemory} onclick={saveSelectedMemoryAsset}>
              {savingMemory ? 'Saving...' : 'Save'}
            </button>
          </div>
        </div>
        <div class="editor-meta">
          <span>{assetKindLabel(selectedMemoryKind)}</span>
          <span>{formatBytes(memorySizeBytes)}</span>
          <span>{fmt(memoryUpdatedAt)}</span>
        </div>
        <textarea class="memory-editor" bind:value={memoryEditorContent} placeholder="Select a durable memory file to inspect or edit."></textarea>
      </section>
    </div>
  {:else if activeTab === 'search'}
    <div class="search-layout">
      <section class="card search-panel">
        <div class="panel-header">
          <span class="card-title">Memory Search Test</span>
          <button class="btn btn-primary btn-sm" type="button" disabled={!searchQueryInput.trim() || searching} onclick={runSearchTest}>
            {searching ? 'Running...' : 'Run Search'}
          </button>
        </div>
        <div class="form-grid">
          <label class="form-field form-span-2">
            <span>Query</span>
            <input class="form-input" bind:value={searchQueryInput} placeholder="예: 관심있는 주식, coffee preference, previous decision" />
          </label>
          <label class="form-field">
            <span>Limit</span>
            <input class="form-input" type="number" min="1" max="30" bind:value={searchLimit} />
          </label>
        </div>
        <div class="search-flags">
          <label><input type="checkbox" bind:checked={searchIncludeMemory} /> MEMORY.md</label>
          <label><input type="checkbox" bind:checked={searchIncludeDaily} /> Daily logs</label>
          <label><input type="checkbox" bind:checked={searchIncludeSessions} /> Session history</label>
        </div>
      </section>

      <section class="card results-panel">
        <div class="panel-header">
          <span class="card-title">Results</span>
          {#if searchResult}
            <span class="note-meta">{searchResult.results?.length ?? 0} matches</span>
          {/if}
        </div>
        {#if !searchResult}
          <div class="empty-state">Run a query here to test durable memory recall before changing prompts or storage behavior.</div>
        {:else if !searchResult.results || searchResult.results.length === 0}
          <div class="empty-state">{searchResult.message || 'No matches found.'}</div>
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
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
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
  .search-flags {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .memory-stats {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-4);
  }

  .stat-card,
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
  .note-meta {
    font-size: var(--text-xs);
    color: var(--text-secondary);
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
  .search-panel,
  .results-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .asset-list,
  .result-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    overflow: auto;
  }

  .asset-row,
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
  .tab-btn {
    cursor: pointer;
    transition: border-color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
  }

  .asset-row:hover,
  .asset-row.active {
    border-color: var(--primary);
    background: var(--primary-muted);
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
    .form-grid {
      grid-template-columns: 1fr;
    }

    .form-span-2 {
      grid-column: span 1;
    }
  }
</style>
