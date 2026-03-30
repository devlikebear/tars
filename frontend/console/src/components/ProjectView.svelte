<script lang="ts">
  import { onMount } from 'svelte'
  import {
    deleteProject,
    getProject,
    getProjectSession,
    clearProjectSession,
    compactProjectSession,
    listProjectActivity,
    updateProject,
    listProjectFiles,
    getProjectFileContent,
  } from '../lib/api'
  import type { ProjectFile } from '../lib/api'
  import { renderMarkdown } from '../lib/markdown'
  import type {
    Project,
    ProjectActivity,
    ProjectSessionInfo,
  } from '../lib/types'
  import ChatPanel from './ChatPanel.svelte'

  interface Props {
    projectId: string
  }

  let { projectId }: Props = $props()

  let project: Project | null = $state(null)
  let sessionInfo: ProjectSessionInfo | null = $state(null)
  let sessionBusy = $state(false)
  let activity: ProjectActivity[] = $state([])

  let loadingDetail = $state(true)
  let detailError = $state('')
  let panelError = $state('')

  // -- Edit / Delete --
  let editing = $state(false)
  let editName = $state('')
  let editObjective = $state('')
  let editGitRepo = $state('')
  let editSaving = $state(false)
  let editError = $state('')
  let deleting = $state(false)
  let deleteConfirm = $state(false)

  // -- Artifacts --
  let projectFiles: ProjectFile[] = $state([])
  let selectedFile: { name: string; content: string } | null = $state(null)
  let fileLoading = $state(false)

  async function loadFiles() {
    try {
      projectFiles = await listProjectFiles(projectId)
    } catch { projectFiles = [] }
  }

  async function viewFile(name: string) {
    if (selectedFile?.name === name) { selectedFile = null; return }
    fileLoading = true
    try {
      const result = await getProjectFileContent(projectId, name)
      selectedFile = { name: result.name, content: result.content }
    } catch { selectedFile = { name, content: 'Failed to load file.' } }
    finally { fileLoading = false }
  }

  function enterEdit() {
    if (!project) return
    editName = project.name
    editObjective = project.objective || ''
    editGitRepo = project.git_repo || ''
    editError = ''
    editing = true
  }

  function cancelEdit() {
    editing = false
    editError = ''
  }

  async function handleSaveEdit() {
    if (!project || !editName.trim()) return
    editSaving = true
    editError = ''
    try {
      project = await updateProject(projectId, {
        name: editName.trim(),
        objective: editObjective.trim() || undefined,
        git_repo: editGitRepo.trim() || undefined,
      })
      editing = false
    } catch (e) {
      editError = e instanceof Error ? e.message : 'Failed to update project'
    } finally {
      editSaving = false
    }
  }

  async function handleDelete() {
    if (!deleteConfirm) {
      deleteConfirm = true
      return
    }
    deleting = true
    try {
      await deleteProject(projectId)
      window.history.pushState(null, '', '/console')
      window.dispatchEvent(new PopStateEvent('popstate'))
    } catch (e) {
      panelError = e instanceof Error ? e.message : 'Failed to delete project'
      deleteConfirm = false
    } finally {
      deleting = false
    }
  }

  async function loadSessionInfo() {
    try {
      sessionInfo = await getProjectSession(projectId)
    } catch { sessionInfo = null }
  }

  async function handleClearSession() {
    sessionBusy = true
    try {
      await clearProjectSession(projectId)
      await loadSessionInfo()
    } catch (e) {
      panelError = e instanceof Error ? e.message : 'Failed to clear session'
    } finally { sessionBusy = false }
  }

  async function handleCompactSession() {
    sessionBusy = true
    try {
      await compactProjectSession(projectId)
      await loadSessionInfo()
    } catch (e) {
      panelError = e instanceof Error ? e.message : 'Failed to compact session'
    } finally { sessionBusy = false }
  }

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  async function loadDetail() {
    loadingDetail = true
    detailError = ''
    try {
      project = await getProject(projectId)
    } catch (err) {
      project = null
      detailError = err instanceof Error ? err.message : 'Failed to load project'
    } finally {
      loadingDetail = false
    }
  }

  async function loadActivity() {
    try {
      activity = await listProjectActivity(projectId, 20)
    } catch { activity = [] }
  }

  onMount(() => {
    void loadDetail()
    void loadActivity()
    void loadFiles()
    void loadSessionInfo()
  })
</script>

<div class="pv">
  {#if loadingDetail}
    <div class="pv-loading">Loading project...</div>
  {:else if detailError}
    <div class="error-banner">{detailError}</div>
  {:else if project}
    <!-- Project header -->
    <div class="pv-header">
      <div>
        {#if editing}
          <div class="inline-form">
            {#if editError}
              <div class="form-error">{editError}</div>
            {/if}
            <input type="text" placeholder="Project name *" bind:value={editName} class="form-input" />
            <input type="text" placeholder="Objective" bind:value={editObjective} class="form-input" />
            <input type="text" placeholder="Git repo URL" bind:value={editGitRepo} class="form-input" />
            <div style="display:flex;gap:var(--space-2)">
              <button class="btn btn-primary btn-sm" disabled={!editName.trim() || editSaving} onclick={handleSaveEdit}>
                {editSaving ? 'Saving...' : 'Save'}
              </button>
              <button class="btn btn-ghost btn-sm" onclick={cancelEdit}>Cancel</button>
            </div>
          </div>
        {:else}
          <h2>{project.name}</h2>
          <p class="pv-objective">{project.objective || 'No objective recorded.'}</p>
        {/if}
      </div>
      <div class="pv-header-meta">
        <span class="badge badge-default">{project.status || 'active'}</span>
        {#if !editing}
          <button class="btn btn-ghost btn-sm" onclick={enterEdit}>Edit</button>
          <button class="btn btn-danger btn-sm" disabled={deleting} onclick={handleDelete}>
            {deleteConfirm ? 'Confirm Delete?' : 'Delete'}
          </button>
        {/if}
      </div>
    </div>

    {#if panelError}
      <div class="error-banner" style="margin-bottom: var(--space-4)">{panelError}</div>
    {/if}

    <!-- Session info -->
    {#if sessionInfo}
      <div class="card" style="margin-top:0.75rem;">
        <div style="display:flex;align-items:center;justify-content:space-between;">
          <div>
            <span class="label">Session</span>
            <span class="badge-default" style="margin-left:0.5rem;">{sessionInfo.messages} msgs</span>
            <span class="badge-default" style="margin-left:0.25rem;">~{Math.round(sessionInfo.tokens / 1000)}k tokens</span>
          </div>
          <div style="display:flex;gap:0.5rem;">
            <button class="btn-ghost btn-sm" onclick={handleCompactSession} disabled={sessionBusy}>Compact</button>
            <button class="btn-danger btn-sm" onclick={handleClearSession} disabled={sessionBusy}>Clear</button>
          </div>
        </div>
      </div>
    {/if}

    <!-- Artifacts -->
    {#if projectFiles.length > 0}
      <section class="card" style="margin-bottom: var(--space-4)">
        <div class="card-header">
          <span class="card-title">Artifacts</span>
          <span class="badge badge-default">{projectFiles.filter(f => !f.system).length} files</span>
        </div>
        <div class="pv-files-list">
          {#each projectFiles.filter(f => !f.system) as file}
            <button class="pv-file-item" class:active={selectedFile?.name === file.name} onclick={() => viewFile(file.name)}>
              <span class="pv-file-name">{file.name}</span>
              <span class="pv-file-size">{file.size > 1024 ? (file.size / 1024).toFixed(1) + ' KB' : file.size + ' B'}</span>
            </button>
          {/each}
          {#if projectFiles.some(f => f.system)}
            <div class="pv-files-sep">System files</div>
            {#each projectFiles.filter(f => f.system) as file}
              <button class="pv-file-item system" class:active={selectedFile?.name === file.name} onclick={() => viewFile(file.name)}>
                <span class="pv-file-name">{file.name}</span>
                <span class="pv-file-size">{file.size > 1024 ? (file.size / 1024).toFixed(1) + ' KB' : file.size + ' B'}</span>
              </button>
            {/each}
          {/if}
        </div>
        {#if selectedFile}
          <div class="pv-file-preview">
            <div class="pv-file-preview-header">
              <strong>{selectedFile.name}</strong>
              <button class="btn btn-ghost btn-sm" onclick={() => { selectedFile = null }}>Close</button>
            </div>
            {#if fileLoading}
              <div class="pv-file-preview-loading">Loading...</div>
            {:else}
              <div class="pv-file-preview-content pv-md">{@html renderMarkdown(selectedFile.content)}</div>
            {/if}
          </div>
        {/if}
      </section>
    {/if}

    <div class="pv-grid">
      <!-- Identity -->
      <section class="card">
        <span class="card-title">Identity</span>
        <dl class="pv-facts">
          <div><dt>ID</dt><dd class="mono">{project.id}</dd></div>
          <div><dt>Type</dt><dd>{project.type || '\u2014'}</dd></div>
          <div><dt>Path</dt><dd class="mono">{project.path || '\u2014'}</dd></div>
          <div><dt>Repo</dt><dd class="mono">{project.git_repo || '\u2014'}</dd></div>
        </dl>
      </section>

      <!-- Chat -->
      <section class="card pv-wide">
        <span class="card-title">Chat</span>
        <ChatPanel {projectId} />
      </section>

      <!-- Activity -->
      <section class="card pv-wide">
        <div class="card-header">
          <span class="card-title">Activity</span>
          <span class="badge badge-default">{activity.length}</span>
        </div>
        {#if activity.length === 0}
          <div class="empty-state"><p>No activity recorded yet.</p></div>
        {:else}
          <div class="pv-timeline">
            {#each activity as item}
              <div class="pv-timeline-item">
                <div class="pv-timeline-top">
                  <strong>{item.kind}</strong>
                  <span class="badge badge-default">{item.status || item.source}</span>
                </div>
                <p>{item.message || 'No message'}</p>
                <div class="pv-timeline-meta">
                  <span>{fmt(item.timestamp)}</span>
                  {#if item.agent}<span>{item.agent}</span>{/if}
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </section>

    </div>
  {/if}
</div>

<style>
  .pv {
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .pv-loading {
    padding: var(--space-10);
    text-align: center;
    color: var(--text-tertiary);
  }

  .pv-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-5);
  }

  .pv-header h2 {
    font-size: var(--text-2xl);
    margin-bottom: var(--space-1);
  }

  .pv-objective {
    color: var(--text-secondary);
    max-width: 600px;
  }

  .pv-header-meta {
    display: flex;
    gap: var(--space-2);
    flex-shrink: 0;
  }

  /* ── Artifacts ─────────────────────────────── */
  .pv-files-list { display: flex; flex-direction: column; }
  .pv-file-item {
    display: flex; align-items: center; justify-content: space-between;
    padding: var(--space-2) var(--space-4);
    background: none; border: none; border-bottom: 1px solid var(--border-subtle);
    cursor: pointer; text-align: left;
    transition: background var(--duration-fast);
  }
  .pv-file-item:hover { background: rgba(255,255,255,0.02); }
  .pv-file-item.active { background: rgba(224,145,69,0.08); border-left: 2px solid var(--accent); }
  .pv-file-item.system { opacity: 0.5; }
  .pv-file-item:last-child { border-bottom: none; }
  .pv-file-name { font-family: var(--font-mono); font-size: var(--text-xs); color: var(--text-primary); }
  .pv-file-size { font-family: var(--font-mono); font-size: 10px; color: var(--text-ghost); }
  .pv-files-sep {
    padding: var(--space-1) var(--space-4); font-size: 10px; color: var(--text-ghost);
    border-bottom: 1px solid var(--border-subtle); background: var(--bg-base);
  }
  .pv-file-preview {
    border-top: 1px solid var(--border-subtle);
    max-height: 400px; overflow-y: auto;
  }
  .pv-file-preview-header {
    display: flex; align-items: center; justify-content: space-between;
    padding: var(--space-2) var(--space-4); position: sticky; top: 0;
    background: var(--bg-elevated); border-bottom: 1px solid var(--border-subtle);
  }
  .pv-file-preview-header strong { font-family: var(--font-mono); font-size: var(--text-xs); }
  .pv-file-preview-loading { padding: var(--space-4); color: var(--text-tertiary); font-size: var(--text-xs); }
  .pv-file-preview-content {
    padding: var(--space-3) var(--space-4); font-size: var(--text-sm); line-height: 1.6; color: var(--text-secondary);
  }
  .pv-md :global(h1), .pv-md :global(h2), .pv-md :global(h3) { font-family: var(--font-display); font-weight: 600; color: var(--text-primary); margin: var(--space-3) 0 var(--space-1); }
  .pv-md :global(p) { margin: 0 0 var(--space-2); }
  .pv-md :global(ul), .pv-md :global(ol) { margin: var(--space-1) 0; padding-left: var(--space-5); }
  .pv-md :global(li) { margin-bottom: var(--space-1); font-size: var(--text-sm); }
  .pv-md :global(code) { font-family: var(--font-mono); font-size: 0.9em; background: rgba(255,255,255,0.06); padding: 1px 5px; border-radius: 3px; }
  .pv-md :global(pre) { background: var(--bg-base); border: 1px solid var(--border-subtle); border-radius: var(--radius-sm); padding: var(--space-2); overflow-x: auto; margin: var(--space-2) 0; font-family: var(--font-mono); font-size: var(--text-xs); }
  .pv-md :global(strong) { font-weight: 600; color: var(--text-primary); }

  /* ── Grid ─────────────────────────────────────── */
  .pv-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-4);
  }

  .pv-wide {
    grid-column: 1 / -1;
  }

  /* ── Facts ────────────────────────────────────── */
  .pv-facts {
    display: grid;
    gap: var(--space-3);
    margin-top: var(--space-3);
  }

  .pv-facts div {
    display: grid;
    grid-template-columns: 100px minmax(0, 1fr);
    gap: var(--space-3);
  }

  .pv-facts dt {
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }

  .pv-facts dd {
    margin: 0;
    word-break: break-word;
    font-size: var(--text-sm);
  }

  /* ── Timeline ─────────────────────────────────── */
  .pv-timeline {
    display: grid;
    gap: var(--space-2);
  }

  .pv-timeline-item {
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-md);
    background: var(--bg-base);
  }

  .pv-timeline-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: var(--space-1);
  }

  .pv-timeline-top strong {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
  }

  .pv-timeline-item p {
    font-size: var(--text-sm);
    color: var(--text-secondary);
    margin-bottom: var(--space-2);
    white-space: pre-wrap;
  }

  .pv-timeline-meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-3);
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }


  /* ── Inline form ──────────────────────────────── */
  .inline-form {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .inline-form .form-input {
    width: 100%;
    max-width: 500px;
    padding: var(--space-2) var(--space-3);
    background: var(--bg-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-sm);
  }
  .inline-form .form-input:focus {
    outline: none;
    border-color: var(--accent);
  }
  .form-error {
    font-size: var(--text-xs);
    color: var(--error);
  }

  /* ── Responsive ───────────────────────────────── */
  @media (max-width: 900px) {
    .pv-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
