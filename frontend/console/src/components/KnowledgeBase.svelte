<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import {
    createKnowledgeNote,
    deleteKnowledgeNote,
    getKnowledgeGraph,
    getKnowledgeNote,
    listKnowledgeNotes,
    streamEvents,
    updateKnowledgeNote,
  } from '../lib/api'
  import type { KnowledgeGraph, KnowledgeNote, KnowledgeLink } from '../lib/types'

  interface Props {
    onAskAI?: (prompt: string) => void
  }

  let { onAskAI }: Props = $props()

  let notes: KnowledgeNote[] = $state([])
  let graph: KnowledgeGraph = $state({ nodes: [], edges: [] })
  let loading = $state(true)
  let saving = $state(false)
  let deleting = $state(false)
  let error = $state('')
  let success = $state('')
  let selectedSlug: string | null = $state(null)
  let stopStream: (() => void) | null = null

  let searchQuery = $state('')
  let filterKind = $state('all')

  let editorSlug = $state('')
  let editorTitle = $state('')
  let editorKind = $state('note')
  let editorSummary = $state('')
  let editorBody = $state('')
  let editorTags = $state('')
  let editorAliases = $state('')
  let editorProjectID = $state('')
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
    if (!text) return '\u2014'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function compact(value?: string, max = 120): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    return text.length <= max ? text : `${text.slice(0, max - 1)}\u2026`
  }

  function resetEditor() {
    selectedSlug = null
    editorSlug = ''
    editorTitle = ''
    editorKind = 'note'
    editorSummary = ''
    editorBody = ''
    editorTags = ''
    editorAliases = ''
    editorProjectID = ''
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
    editorProjectID = note.project_id || ''
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

  async function load(selectSlug?: string | null) {
    loading = true
    error = ''
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
        resetEditor()
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load knowledge base'
    } finally {
      loading = false
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
    saving = true
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
      project_id: editorProjectID.trim() || undefined,
      source_session: editorSourceSession.trim() || undefined,
    }
    try {
      const saved = selectedSlug
        ? await updateKnowledgeNote(selectedSlug, payload)
        : await createKnowledgeNote(payload)
      await load(saved.slug)
      success = selectedSlug ? 'Note updated.' : 'Note created.'
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to save note'
    } finally {
      saving = false
    }
  }

  async function removeNote() {
    if (!selectedSlug) return
    deleting = true
    error = ''
    success = ''
    try {
      await deleteKnowledgeNote(selectedSlug)
      await load(null)
      success = 'Note deleted.'
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete note'
    } finally {
      deleting = false
    }
  }

  onMount(() => {
    void load(null)
    stopStream = streamEvents(
      undefined,
      (event) => {
        if (event.category === 'chat' || event.category === 'project') {
          void load(selectedSlug)
        }
      },
    )
  })

  onDestroy(() => {
    stopStream?.()
  })
</script>

<div class="knowledge-page">
  <div class="page-header">
    <div>
      <h2>Knowledge Base</h2>
      <p class="page-subtitle">{notes.length} notes, {graph.edges.length} relations</p>
    </div>
    <div class="page-actions">
      {#if onAskAI}
        <button class="btn btn-ghost btn-sm" onclick={() => onAskAI('Update the knowledge base for this topic: ')}>
          Ask AI
        </button>
      {/if}
      <button class="btn btn-ghost btn-sm" onclick={() => load(selectedSlug)}>Refresh</button>
      <button class="btn btn-primary btn-sm" onclick={resetEditor}>+ New Note</button>
    </div>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}
  {#if success}
    <div class="success-banner">{success}</div>
  {/if}

  <div class="knowledge-stats">
    <div class="stat-card">
      <span class="stat-label">Graph Updated</span>
      <strong class="stat-value">{fmt(graph.updated_at)}</strong>
    </div>
    <div class="stat-card">
      <span class="stat-label">Kinds</span>
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
    <div class="stat-card">
      <span class="stat-label">Recent Relations</span>
      <div class="relation-list">
        {#if graph.edges.length === 0}
          <span class="relation-empty">No links yet.</span>
        {:else}
          {#each graph.edges.slice(0, 4) as edge}
            <div class="relation-item">{edge.source} → {edge.target}{edge.relation ? ` (${edge.relation})` : ''}</div>
          {/each}
        {/if}
      </div>
    </div>
  </div>

  <div class="filter-bar">
    <input type="text" class="filter-search" placeholder="Search notes..." bind:value={searchQuery} />
    <select class="filter-select" bind:value={filterKind}>
      {#each kindOptions as option}
        <option value={option}>{option === 'all' ? 'All kinds' : option}</option>
      {/each}
    </select>
  </div>

  <div class="knowledge-layout">
    <aside class="notes-panel card">
      <div class="panel-header">
        <span class="card-title">Notes</span>
      </div>
      {#if loading}
        <div class="empty-state">Loading knowledge base...</div>
      {:else if filtered.length === 0}
        <div class="empty-state">{searchQuery || filterKind !== 'all' ? 'No matching notes.' : 'No notes yet.'}</div>
      {:else}
        <div class="note-list">
          {#each filtered as note}
            <button class="note-row" class:active={selectedSlug === note.slug} onclick={() => selectNote(note.slug)}>
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
          <button class="btn btn-danger btn-sm" disabled={deleting} onclick={removeNote}>
            {deleting ? 'Deleting...' : 'Delete'}
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
          <span>Project ID</span>
          <input class="form-input" bind:value={editorProjectID} placeholder="optional" />
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
        <button class="btn btn-ghost btn-sm" onclick={resetEditor}>Reset</button>
        <button class="btn btn-primary btn-sm" disabled={!editorTitle.trim() || saving} onclick={saveNote}>
          {saving ? 'Saving...' : selectedSlug ? 'Save Changes' : 'Create Note'}
        </button>
      </div>
    </section>
  </div>
</div>

<style>
  .knowledge-page {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .page-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .page-subtitle {
    margin-top: var(--space-1);
    color: var(--text-secondary);
  }

  .page-actions {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .knowledge-stats {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--space-4);
  }

  .stat-card {
    padding: var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--bg-surface);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .stat-label {
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--text-ghost);
  }

  .stat-value {
    font-size: var(--text-md);
  }

  .kind-list,
  .relation-list {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .kind-pill,
  .relation-item,
  .relation-empty {
    font-size: var(--text-xs);
    padding: 6px 10px;
    border-radius: var(--radius-pill);
    background: var(--bg-elevated);
    color: var(--text-secondary);
  }

  .filter-bar {
    display: flex;
    gap: var(--space-3);
    align-items: center;
  }

  .filter-search,
  .filter-select,
  .form-input,
  .form-select,
  .form-textarea {
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

  .knowledge-layout {
    display: grid;
    grid-template-columns: minmax(280px, 360px) minmax(0, 1fr);
    gap: var(--space-4);
    min-height: 520px;
  }

  .notes-panel,
  .editor-panel {
    padding: var(--space-4);
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .note-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    overflow: auto;
  }

  .note-row {
    text-align: left;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--bg-base);
    padding: var(--space-3);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    cursor: pointer;
    transition: border-color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
  }

  .note-row:hover,
  .note-row.active {
    border-color: var(--accent);
    background: var(--accent-muted);
  }

  .note-row-top,
  .note-meta,
  .editor-actions {
    display: flex;
    justify-content: space-between;
    gap: var(--space-2);
    align-items: center;
  }

  .note-kind,
  .note-meta {
    font-size: var(--text-xs);
    color: var(--text-secondary);
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

  .form-field span {
    font-size: var(--text-xs);
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.08em;
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
    .knowledge-stats,
    .knowledge-layout,
    .form-grid {
      grid-template-columns: 1fr;
    }

    .form-span-2 {
      grid-column: span 1;
    }
  }
</style>
