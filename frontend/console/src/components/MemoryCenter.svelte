<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import {
    createKnowledgeNote,
    deleteKnowledgeNote,
    getKnowledgeGraph,
    getKnowledgeNote,
    getMemoryFile,
    listKnowledgeNotes,
    listMemoryAssets,
    runMemorySearch,
    saveMemoryFile,
    streamEvents,
    updateKnowledgeNote,
  } from '../lib/api'
  import type {
    KnowledgeGraph,
    KnowledgeLink,
    KnowledgeNote,
    MemoryAsset,
    MemorySearchResult,
  } from '../lib/types'

  interface Props {
    onAskAI?: (prompt: string) => void
  }

  let { onAskAI }: Props = $props()

  let activeTab = $state<'durable' | 'search' | 'knowledge'>('durable')
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
  let searchIncludeKnowledge = $state(false)
  let searchIncludeSessions = $state(true)
  let searching = $state(false)
  let searchResult: MemorySearchResult | null = $state(null)

  let notes: KnowledgeNote[] = $state([])
  let graph: KnowledgeGraph = $state({ nodes: [], edges: [] })
  let loadingKnowledge = $state(true)
  let savingKnowledge = $state(false)
  let deletingKnowledge = $state(false)
  let selectedSlug: string | null = $state(null)

  let searchQuery = $state('')
  let filterKind = $state('all')

  let editorSlug = $state('')
  let editorTitle = $state('')
  let editorKind = $state('note')
  let editorSummary = $state('')
  let editorBody = $state('')
  let editorTags = $state('')
  let editorAliases = $state('')
  let editorSourceSession = $state('')
  let editorLinks = $state('')

  const kindOptions = ['all', 'note', 'preference', 'fact', 'decision', 'habit', 'workflow', 'project_note', 'topic', 'person']

  let filtered = $derived.by(() => {
    let items = notes
    if (filterKind !== 'all') {
      items = items.filter((note) => (note.kind || 'note') === filterKind)
    }
    if (searchQuery.trim()) {
      const q = searchQuery.trim().toLowerCase()
      items = items.filter((note) =>
        note.slug.toLowerCase().includes(q) ||
        note.title.toLowerCase().includes(q) ||
        note.summary?.toLowerCase().includes(q) ||
        note.tags?.some((tag) => tag.toLowerCase().includes(q)),
      )
    }
    return items
  })

  let kindCounts = $derived.by(() => {
    const counts: Record<string, number> = {}
    for (const note of notes) {
      const key = note.kind || 'note'
      counts[key] = (counts[key] || 0) + 1
    }
    return Object.entries(counts).sort((a, b) => b[1] - a[1])
  })

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '-'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function compact(value?: string, max = 120): string {
    const text = value?.trim()
    if (!text) return '-'
    return text.length <= max ? text : `${text.slice(0, max - 1)}...`
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
        include_knowledge: searchIncludeKnowledge,
        include_sessions: searchIncludeSessions,
      })
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to run memory search'
    } finally {
      searching = false
    }
  }

  function resetKnowledgeEditor() {
    selectedSlug = null
    editorSlug = ''
    editorTitle = ''
    editorKind = 'note'
    editorSummary = ''
    editorBody = ''
    editorTags = ''
    editorAliases = ''
    editorSourceSession = ''
    editorLinks = ''
    success = ''
  }

  function fillEditor(note: KnowledgeNote) {
    selectedSlug = note.slug
    editorSlug = note.slug
    editorTitle = note.title
    editorKind = note.kind || 'note'
    editorSummary = note.summary || ''
    editorBody = note.body || ''
    editorTags = (note.tags || []).join(', ')
    editorAliases = (note.aliases || []).join(', ')
    editorSourceSession = note.source_session || ''
    editorLinks = formatLinks(note.links || [])
    success = ''
  }

  function formatLinks(links: KnowledgeLink[]): string {
    return links
      .map((link) => {
        const relation = link.relation?.trim()
        return relation ? `${relation}:${link.target}` : link.target
      })
      .join('\n')
  }

  function parseTags(raw: string): string[] {
    return raw.split(',').map((item) => item.trim()).filter(Boolean)
  }

  function parseLinks(raw: string): KnowledgeLink[] {
    return raw
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean)
      .map((line) => {
        const idx = line.indexOf(':')
        if (idx <= 0) {
          return { target: line }
        }
        return {
          relation: line.slice(0, idx).trim(),
          target: line.slice(idx + 1).trim(),
        }
      })
      .filter((link) => link.target)
  }

  async function loadKnowledge(selectSlug?: string | null) {
    loadingKnowledge = true
    try {
      const [notesRes, graphRes] = await Promise.all([
        listKnowledgeNotes({ limit: 200 }),
        getKnowledgeGraph(),
      ])
      notes = notesRes.items || []
      graph = graphRes
      const target = selectSlug || selectedSlug
      if (target) {
        const note = await getKnowledgeNote(target)
        fillEditor(note)
      } else if (notes.length > 0) {
        fillEditor(await getKnowledgeNote(notes[0].slug))
      } else {
        resetKnowledgeEditor()
      }
    } finally {
      loadingKnowledge = false
    }
  }

  async function selectNote(slug: string) {
    error = ''
    success = ''
    try {
      const note = await getKnowledgeNote(slug)
      fillEditor(note)
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load note'
    }
  }

  async function saveNote() {
    if (!editorTitle.trim()) return
    savingKnowledge = true
    error = ''
    success = ''
    const payload: Partial<KnowledgeNote> = {
      slug: editorSlug.trim() || undefined,
      title: editorTitle.trim(),
      kind: editorKind,
      summary: editorSummary.trim() || undefined,
      body: editorBody.trim() || undefined,
      tags: parseTags(editorTags),
      aliases: parseTags(editorAliases),
      links: parseLinks(editorLinks),
      source_session: editorSourceSession.trim() || undefined,
    }
    try {
      const saved = selectedSlug
        ? await updateKnowledgeNote(selectedSlug, payload)
        : await createKnowledgeNote(payload)
      await loadKnowledge(saved.slug)
      success = selectedSlug ? 'Knowledge note updated.' : 'Knowledge note created.'
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to save note'
    } finally {
      savingKnowledge = false
    }
  }

  async function removeNote() {
    if (!selectedSlug) return
    deletingKnowledge = true
    error = ''
    success = ''
    try {
      await deleteKnowledgeNote(selectedSlug)
      await loadKnowledge(null)
      success = 'Knowledge note deleted.'
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete note'
    } finally {
      deletingKnowledge = false
    }
  }

  async function loadAll() {
    error = ''
    try {
      await Promise.all([
        loadMemory(selectedMemoryPath),
        loadKnowledge(selectedSlug),
      ])
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load memory console'
    }
  }

  function askAIPrompt(): string {
    switch (activeTab) {
      case 'search':
        return 'Help me debug this memory search query and improve recall: '
      case 'knowledge':
        return 'Update the knowledge base for this topic: '
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
      <p class="page-subtitle">{memoryAssets.length} durable assets, {notes.length} KB notes, {graph.edges.length} KB relations</p>
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
    <div class="stat-card">
      <span class="stat-label">Knowledge Mix</span>
      <div class="kind-list">
        {#if kindCounts.length === 0}
          <span class="kind-pill">No notes</span>
        {:else}
          {#each kindCounts.slice(0, 6) as [kind, count]}
            <span class="kind-pill">{kind} · {count}</span>
          {/each}
        {/if}
      </div>
    </div>
  </div>

  <div class="tab-row">
    <button class="tab-btn" class:active={activeTab === 'durable'} type="button" onclick={() => { activeTab = 'durable' }}>Durable Memory</button>
    <button class="tab-btn" class:active={activeTab === 'search'} type="button" onclick={() => { activeTab = 'search' }}>Search Test</button>
    <button class="tab-btn" class:active={activeTab === 'knowledge'} type="button" onclick={() => { activeTab = 'knowledge' }}>Knowledge Base</button>
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
          <label><input type="checkbox" bind:checked={searchIncludeKnowledge} /> Knowledge base</label>
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
  {:else}
    <div class="filter-bar">
      <input type="text" class="filter-search" placeholder="Search notes..." bind:value={searchQuery} />
      <select class="filter-select" bind:value={filterKind}>
        {#each kindOptions as option}
          <option value={option}>{option === 'all' ? 'All kinds' : option}</option>
        {/each}
      </select>
      <button class="btn btn-primary btn-sm" type="button" onclick={resetKnowledgeEditor}>+ New Note</button>
    </div>

    <div class="knowledge-layout">
      <aside class="notes-panel card">
        <div class="panel-header">
          <span class="card-title">Notes</span>
        </div>
        {#if loadingKnowledge}
          <div class="empty-state">Loading knowledge base...</div>
        {:else if filtered.length === 0}
          <div class="empty-state">{searchQuery || filterKind !== 'all' ? 'No matching notes.' : 'No notes yet.'}</div>
        {:else}
          <div class="note-list">
            {#each filtered as note}
              <button class="note-row" class:active={selectedSlug === note.slug} type="button" onclick={() => selectNote(note.slug)}>
                <div class="note-row-top">
                  <strong>{note.title}</strong>
                  <span class="note-kind">{note.kind || 'note'}</span>
                </div>
                <p>{compact(note.summary || note.body, 90)}</p>
                <div class="note-meta">
                  <span>{note.slug}</span>
                  <span>{fmt(note.updated_at)}</span>
                </div>
              </button>
            {/each}
          </div>
        {/if}
      </aside>

      <section class="editor-panel card">
        <div class="panel-header">
          <span class="card-title">{selectedSlug ? 'Edit Note' : 'New Note'}</span>
          {#if selectedSlug}
            <button class="btn btn-danger btn-sm" type="button" disabled={deletingKnowledge} onclick={removeNote}>
              {deletingKnowledge ? 'Deleting...' : 'Delete'}
            </button>
          {/if}
        </div>

        <div class="form-grid">
          <label class="form-field">
            <span>Slug</span>
            <input class="form-input" bind:value={editorSlug} placeholder="auto-generated if blank" />
          </label>
          <label class="form-field">
            <span>Kind</span>
            <select class="form-select" bind:value={editorKind}>
              {#each kindOptions.filter((kind) => kind !== 'all') as option}
                <option value={option}>{option}</option>
              {/each}
            </select>
          </label>
          <label class="form-field form-span-2">
            <span>Title</span>
            <input class="form-input" bind:value={editorTitle} placeholder="Coffee Preference" />
          </label>
          <label class="form-field form-span-2">
            <span>Summary</span>
            <textarea class="form-textarea form-textarea-sm" bind:value={editorSummary} placeholder="Short durable summary"></textarea>
          </label>
          <label class="form-field form-span-2">
            <span>Details</span>
            <textarea class="form-textarea" bind:value={editorBody} placeholder="Durable wiki-style note body"></textarea>
          </label>
          <label class="form-field">
            <span>Tags</span>
            <input class="form-input" bind:value={editorTags} placeholder="coffee, preference" />
          </label>
          <label class="form-field">
            <span>Aliases</span>
            <input class="form-input" bind:value={editorAliases} placeholder="black coffee" />
          </label>
          <label class="form-field">
            <span>Source Session</span>
            <input class="form-input" bind:value={editorSourceSession} placeholder="optional" />
          </label>
          <label class="form-field form-span-2">
            <span>Links</span>
            <textarea class="form-textarea form-textarea-sm" bind:value={editorLinks} placeholder="related_to:coffee-routine&#10;depends_on:daily-ritual"></textarea>
          </label>
        </div>

        <div class="editor-actions">
          <button class="btn btn-ghost btn-sm" type="button" onclick={resetKnowledgeEditor}>Reset</button>
          <button class="btn btn-primary btn-sm" type="button" disabled={!editorTitle.trim() || savingKnowledge} onclick={saveNote}>
            {savingKnowledge ? 'Saving...' : selectedSlug ? 'Save Changes' : 'Create Note'}
          </button>
        </div>
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
  .note-row-top,
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
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--space-4);
  }

  .stat-card,
  .notes-panel,
  .editor-panel,
  .assets-panel,
  .search-panel,
  .results-panel {
    padding: var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--bg-surface);
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

  .kind-list {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .kind-pill,
  .tab-btn {
    border-radius: var(--radius-pill);
    padding: 8px 12px;
    border: 1px solid var(--border-subtle);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font: inherit;
  }

  .tab-btn.active {
    border-color: var(--accent);
    background: var(--accent-muted);
    color: var(--accent-text);
  }

  .memory-layout,
  .knowledge-layout {
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

  .notes-panel,
  .editor-panel,
  .assets-panel,
  .search-panel,
  .results-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .asset-list,
  .note-list,
  .result-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    overflow: auto;
  }

  .asset-row,
  .note-row,
  .result-row {
    text-align: left;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--bg-base);
    padding: var(--space-3);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .asset-row,
  .note-row,
  .tab-btn {
    cursor: pointer;
    transition: border-color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
  }

  .asset-row:hover,
  .asset-row.active,
  .note-row:hover,
  .note-row.active {
    border-color: var(--accent);
    background: var(--accent-muted);
  }

  .filter-bar,
  .editor-meta {
    display: flex;
    gap: var(--space-3);
    align-items: center;
    flex-wrap: wrap;
  }

  .filter-search,
  .filter-select,
  .form-input,
  .form-select,
  .form-textarea,
  .memory-editor {
    width: 100%;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    padding: 10px 12px;
    font: inherit;
  }

  .filter-select {
    max-width: 200px;
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

  .form-textarea {
    min-height: 180px;
    resize: vertical;
  }

  .form-textarea-sm {
    min-height: 96px;
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
    background: var(--bg-elevated);
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
    background: var(--bg-elevated);
    color: var(--text-secondary);
  }

  @media (max-width: 1024px) {
    .memory-stats,
    .memory-layout,
    .knowledge-layout,
    .search-layout,
    .form-grid {
      grid-template-columns: 1fr;
    }

    .form-span-2 {
      grid-column: span 1;
    }
  }
</style>
