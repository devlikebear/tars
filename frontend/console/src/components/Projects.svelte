<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import {
    createProject,
    deleteProject,
    listProjects,
    streamEvents,
  } from '../lib/api'
  import type { Project } from '../lib/types'

  interface Props {
    onNavigate: (path: string) => void
    onAskAI?: (prompt: string) => void
  }

  let { onNavigate, onAskAI }: Props = $props()

  let projects: Project[] = $state([])
  let loading = $state(true)
  let error = $state('')
  let stopStream: (() => void) | null = null

  // -- Filter --
  let filterStatus = $state('all')
  let searchQuery = $state('')

  let filtered = $derived.by(() => {
    let list = projects
    if (filterStatus !== 'all') {
      list = list.filter((p) => (p.status || 'active') === filterStatus)
    }
    if (searchQuery.trim()) {
      const q = searchQuery.trim().toLowerCase()
      list = list.filter((p) =>
        p.name.toLowerCase().includes(q) ||
        p.objective?.toLowerCase().includes(q) ||
        p.id.toLowerCase().includes(q),
      )
    }
    return list
  })

  // -- New Project form --
  let showNewProject = $state(false)
  let newProjectName = $state('')
  let newProjectObjective = $state('')
  let newProjectGitRepo = $state('')
  let newProjectType = $state('')
  let newProjectMode: 'manual' | 'autonomous' = $state('manual')
  let newProjectMaxPhases = $state(3)
  let newProjectSubAgents = $state('')
  let newProjectSaving = $state(false)
  let newProjectError = $state('')

  // -- Delete --
  let deleteConfirmId: string | null = $state(null)
  let deletingId = $state('')

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function goToProject(projectId: string) {
    onNavigate(`/console/projects/${encodeURIComponent(projectId)}`)
  }

  async function load() {
    loading = true
    error = ''
    try {
      projects = await listProjects()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load projects'
    } finally {
      loading = false
    }
  }

  async function handleCreateProject() {
    if (!newProjectName.trim()) return
    newProjectSaving = true
    newProjectError = ''
    try {
      const subAgents = newProjectSubAgents.trim()
        ? newProjectSubAgents.split(',').map((s) => s.trim()).filter(Boolean)
        : undefined
      const p = await createProject({
        name: newProjectName.trim(),
        type: newProjectType.trim() || undefined,
        objective: newProjectObjective.trim() || undefined,
        git_repo: newProjectGitRepo.trim() || undefined,
        execution_mode: newProjectMode,
        max_phases: newProjectMode === 'autonomous' ? newProjectMaxPhases : undefined,
        sub_agents: subAgents,
      })
      projects = [p, ...projects]
      showNewProject = false
      newProjectName = ''
      newProjectObjective = ''
      newProjectGitRepo = ''
      newProjectType = ''
      newProjectMode = 'manual'
      newProjectMaxPhases = 3
      newProjectSubAgents = ''
      goToProject(p.id)
    } catch (e) {
      newProjectError = e instanceof Error ? e.message : 'Failed to create project'
    } finally {
      newProjectSaving = false
    }
  }

  async function handleDelete(projectId: string) {
    if (deleteConfirmId !== projectId) {
      deleteConfirmId = projectId
      return
    }
    deletingId = projectId
    try {
      await deleteProject(projectId)
      projects = projects.filter((p) => p.id !== projectId)
      deleteConfirmId = null
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete project'
    } finally {
      deletingId = ''
    }
  }

  onMount(() => {
    void load()
    stopStream = streamEvents(
      undefined,
      (event) => {
        if (event.category === 'project') {
          void listProjects().then((list) => { projects = list })
        }
      },
    )
  })

  onDestroy(() => {
    stopStream?.()
  })
</script>

<div class="projects-page">
  <div class="page-header">
    <div>
      <h2>Projects</h2>
      <p class="page-subtitle">{projects.length} projects total</p>
    </div>
    <div class="page-actions">
      {#if onAskAI}
        <button class="btn btn-ghost btn-sm ask-ai-btn" onclick={() => onAskAI('Create a new project: ')}>
          Ask AI
        </button>
      {/if}
      <button class="btn btn-primary btn-sm" onclick={() => { showNewProject = !showNewProject }}>
        {showNewProject ? 'Cancel' : '+ New Project'}
      </button>
    </div>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  {#if showNewProject}
    <div class="card new-project-form">
      <span class="card-title">New Project</span>
      {#if newProjectError}
        <div class="form-error">{newProjectError}</div>
      {/if}
      <div class="form-grid">
        <input type="text" placeholder="Project name *" bind:value={newProjectName} class="form-input" />
        <input type="text" placeholder="Type (optional)" bind:value={newProjectType} class="form-input" />
        <input type="text" placeholder="Objective (optional)" bind:value={newProjectObjective} class="form-input form-span-2" />
        <input type="text" placeholder="Git repo URL (optional)" bind:value={newProjectGitRepo} class="form-input form-span-2" />
        <div class="form-span-2 form-row">
          <label class="form-label">
            Execution mode
            <select bind:value={newProjectMode} class="form-select">
              <option value="manual">Manual</option>
              <option value="autonomous">Autonomous</option>
            </select>
          </label>
          {#if newProjectMode === 'autonomous'}
            <label class="form-label">
              Max phases
              <input type="number" min="1" max="20" bind:value={newProjectMaxPhases} class="form-input form-input-sm" />
            </label>
            <label class="form-label">
              Sub-agents
              <input type="text" placeholder="critic, reviewer (comma-separated)" bind:value={newProjectSubAgents} class="form-input" />
            </label>
          {/if}
        </div>
      </div>
      <button
        class="btn btn-primary btn-sm"
        disabled={!newProjectName.trim() || newProjectSaving}
        onclick={handleCreateProject}
      >{newProjectSaving ? 'Creating...' : 'Create'}</button>
    </div>
  {/if}

  <!-- Filters -->
  <div class="filter-bar">
    <input
      type="text"
      class="filter-search"
      placeholder="Search projects..."
      bind:value={searchQuery}
    />
    <div class="filter-tabs">
      <button class="filter-tab" class:active={filterStatus === 'all'} onclick={() => { filterStatus = 'all' }}>All</button>
      <button class="filter-tab" class:active={filterStatus === 'active'} onclick={() => { filterStatus = 'active' }}>Active</button>
      <button class="filter-tab" class:active={filterStatus === 'archived'} onclick={() => { filterStatus = 'archived' }}>Archived</button>
    </div>
  </div>

  {#if loading}
    <div class="page-loading">Loading projects...</div>
  {:else if filtered.length === 0}
    <div class="empty-state">
      <p>{searchQuery || filterStatus !== 'all' ? 'No matching projects.' : 'No projects yet. Click + New Project to create one.'}</p>
    </div>
  {:else}
    <div class="project-table">
      <div class="table-header">
        <span class="col-name">Name</span>
        <span class="col-objective">Objective</span>
        <span class="col-status">Status</span>
        <span class="col-updated">Updated</span>
        <span class="col-actions"></span>
      </div>
      {#each filtered as project}
        <div class="table-row">
          <button class="col-name project-name-btn" onclick={() => goToProject(project.id)}>
            <strong>{project.name}</strong>
            {#if project.type}
              <span class="project-type">{project.type}</span>
            {/if}
          </button>
          <span class="col-objective project-objective">{project.objective || '\u2014'}</span>
          <span class="col-status">
            <span class="badge" class:badge-success={project.status === 'active' || !project.status} class:badge-default={project.status === 'archived'}>
              {project.status || 'active'}
            </span>
          </span>
          <span class="col-updated">{fmt(project.updated_at)}</span>
          <span class="col-actions">
            {#if onAskAI}
              <button
                class="btn btn-ghost btn-sm ask-ai-btn"
                onclick={(e: MouseEvent) => { e.stopPropagation(); onAskAI(`Update project "${project.name}" (${project.id}): `) }}
              >AI</button>
            {/if}
            <button
              class="btn btn-danger btn-sm"
              disabled={deletingId === project.id}
              onclick={(e: MouseEvent) => { e.stopPropagation(); handleDelete(project.id) }}
            >{deleteConfirmId === project.id ? 'Confirm?' : 'Delete'}</button>
          </span>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .projects-page {
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .page-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-5);
  }
  .page-header h2 { font-size: var(--text-2xl); margin-bottom: var(--space-1); }
  .page-subtitle { color: var(--text-tertiary); font-size: var(--text-sm); }
  .page-actions { display: flex; gap: var(--space-2); flex-shrink: 0; }
  .page-loading { padding: var(--space-10); text-align: center; color: var(--text-tertiary); }

  /* ── New project form ────────────────────── */
  .new-project-form {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }
  .form-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-2);
  }
  .form-span-2 { grid-column: 1 / -1; }
  .form-input {
    width: 100%;
    padding: var(--space-2) var(--space-3);
    background: var(--bg-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-sm);
  }
  .form-input:focus { outline: none; border-color: var(--accent); }
  .form-input-sm { max-width: 80px; }
  .form-select {
    padding: var(--space-2) var(--space-3);
    background: var(--bg-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-sm);
  }
  .form-row {
    display: flex;
    gap: var(--space-3);
    align-items: flex-end;
    flex-wrap: wrap;
  }
  .form-label {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }
  .form-error { font-size: var(--text-xs); color: var(--error); }

  /* ── Filter bar ──────────────────────────── */
  .filter-bar {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }
  .filter-search {
    flex: 1;
    max-width: 300px;
    padding: var(--space-2) var(--space-3);
    background: var(--bg-surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-sm);
  }
  .filter-search:focus { outline: none; border-color: var(--accent); }
  .filter-search::placeholder { color: var(--text-ghost); }

  .filter-tabs {
    display: flex;
    gap: 2px;
    background: var(--bg-elevated);
    border-radius: var(--radius-md);
    padding: 2px;
  }
  .filter-tab {
    padding: var(--space-1) var(--space-3);
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 500;
    cursor: pointer;
    transition: all var(--duration-fast) var(--ease-out);
  }
  .filter-tab:hover { color: var(--text-primary); }
  .filter-tab.active { background: var(--accent); color: #fff; }

  /* ── Project table ───────────────────────── */
  .project-table {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    overflow: hidden;
  }

  .table-header {
    display: grid;
    grid-template-columns: 1.2fr 2fr 80px 140px 120px;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-4);
    background: var(--bg-elevated);
    border-bottom: 1px solid var(--border-subtle);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 600;
    color: var(--text-tertiary);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .table-row {
    display: grid;
    grid-template-columns: 1.2fr 2fr 80px 140px 120px;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
    align-items: center;
    transition: background var(--duration-fast) var(--ease-out);
  }
  .table-row:last-child { border-bottom: none; }
  .table-row:hover { background: rgba(255, 255, 255, 0.015); }

  .project-name-btn {
    display: flex;
    flex-direction: column;
    gap: 2px;
    background: none;
    border: none;
    text-align: left;
    cursor: pointer;
    padding: 0;
    min-width: 0;
  }
  .project-name-btn strong {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .project-name-btn:hover strong { color: var(--accent); }
  .project-type {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
  }

  .project-objective {
    font-size: var(--text-xs);
    color: var(--text-secondary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .col-updated {
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  .col-actions {
    text-align: right;
  }

  .ask-ai-btn { color: var(--accent); }
  .ask-ai-btn:hover { background: var(--accent-muted); }
  .col-actions { display: flex; gap: var(--space-1); justify-content: flex-end; }

  @media (max-width: 900px) {
    .table-header, .table-row {
      grid-template-columns: 1fr 80px 80px;
    }
    .col-objective, .col-updated { display: none; }
    .filter-bar { flex-wrap: wrap; }
    .filter-search { max-width: 100%; }
  }
</style>
