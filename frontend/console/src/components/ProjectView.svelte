<script lang="ts">
  import { onMount } from 'svelte'
  import {
    deleteProject,
    getProject,
    getProjectBoard,
    getProjectState,
    getProjectSession,
    clearProjectSession,
    compactProjectSession,
    listProjectActivity,
    updateProject,
    updateProjectBoard,
    listProjectFiles,
    getProjectFileContent,
  } from '../lib/api'
  import type { ProjectFile } from '../lib/api'
  import { renderMarkdown } from '../lib/markdown'
  import type {
    Project,
    ProjectActivity,
    ProjectBoard,
    ProjectState,
    ProjectSessionInfo,
    BoardTask,
  } from '../lib/types'
  import ChatPanel from './ChatPanel.svelte'

  interface Props {
    projectId: string
  }

  let { projectId }: Props = $props()

  let project: Project | null = $state(null)
  let board: ProjectBoard | null = $state(null)
  let projectState: ProjectState | null = $state(null)
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
  let editSourcePath = $state('')
  let editSkillsAllow = $state('')
  let editSaving = $state(false)
  let editError = $state('')
  let deleting = $state(false)
  let deleteConfirm = $state(false)

  let showAllActivity = $state(false)
  let activityFilter = $state('all')

  function activitySources(): string[] {
    return [...new Set(activity.map((a) => a.source).filter(Boolean))]
  }

  function filteredActivity(): ProjectActivity[] {
    return activityFilter === 'all' ? activity : activity.filter((a) => a.source === activityFilter)
  }

  // -- Board editing --
  let newTaskTitle = $state('')
  let boardBusy = $state(false)
  let editingTaskId: string | null = $state(null)
  let editingTaskTitle = $state('')

  async function addTask() {
    if (!newTaskTitle.trim() || !board || boardBusy) return
    boardBusy = true
    try {
      const id = `task-${Date.now()}`
      const tasks: BoardTask[] = [...board.tasks, { id, title: newTaskTitle.trim(), status: 'todo' }]
      board = await updateProjectBoard(projectId, tasks)
      newTaskTitle = ''
    } catch (e) {
      panelError = e instanceof Error ? e.message : 'Add task failed'
    } finally { boardBusy = false }
  }

  async function removeTask(taskId: string) {
    if (!board || boardBusy) return
    boardBusy = true
    try {
      const tasks = board.tasks.filter((t) => t.id !== taskId)
      board = await updateProjectBoard(projectId, tasks)
    } catch (e) {
      panelError = e instanceof Error ? e.message : 'Remove task failed'
    } finally { boardBusy = false }
  }

  async function changeTaskStatus(taskId: string, status: string) {
    if (!board || boardBusy) return
    boardBusy = true
    try {
      const tasks = board.tasks.map((t) => t.id === taskId ? { ...t, status } : t)
      board = await updateProjectBoard(projectId, tasks)
    } catch (e) {
      panelError = e instanceof Error ? e.message : 'Update task failed'
    } finally { boardBusy = false }
  }

  function startEditTask(task: BoardTask) {
    editingTaskId = task.id
    editingTaskTitle = task.title
  }

  async function commitEditTask() {
    if (!editingTaskId || !editingTaskTitle.trim() || !board) { editingTaskId = null; return }
    boardBusy = true
    try {
      const tasks = board.tasks.map((t) => t.id === editingTaskId ? { ...t, title: editingTaskTitle.trim() } : t)
      board = await updateProjectBoard(projectId, tasks)
    } catch (e) {
      panelError = e instanceof Error ? e.message : 'Edit task failed'
    } finally { boardBusy = false; editingTaskId = null }
  }

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
    editSourcePath = project.source_path || ''
    editSkillsAllow = (project.skills_allow || []).join(', ')
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
      const skillsAllow = editSkillsAllow.trim()
        ? editSkillsAllow.split(',').map((s: string) => s.trim()).filter(Boolean)
        : undefined
      project = await updateProject(projectId, {
        name: editName.trim(),
        objective: editObjective.trim() || undefined,
        git_repo: editGitRepo.trim() || undefined,
        source_path: editSourcePath.trim() || undefined,
        skills_allow: skillsAllow,
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

  async function loadBoard() {
    try {
      board = await getProjectBoard(projectId)
    } catch { board = null }
  }

  async function loadState() {
    try {
      projectState = await getProjectState(projectId)
    } catch { projectState = null }
  }

  function statusBadgeClass(status: string): string {
    switch (status) {
      case 'done': return 'badge-success'
      case 'in_progress': return 'badge-accent'
      case 'review': return 'badge-info'
      case 'todo': return 'badge-default'
      default: return 'badge-default'
    }
  }

  function refreshAll() {
    void loadDetail()
    void loadBoard()
    void loadState()
    void loadActivity()
    void loadFiles()
    void loadSessionInfo()
  }

  onMount(refreshAll)
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
            <input type="text" placeholder="Source path (absolute path to codebase)" bind:value={editSourcePath} class="form-input" />
            <input type="text" placeholder="Skills (comma-separated, e.g. github-dev)" bind:value={editSkillsAllow} class="form-input" />
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
        {#if project.execution_mode === 'autonomous'}
          <span class="badge badge-accent">autonomous</span>
        {:else}
          <span class="badge badge-default">manual</span>
        {/if}
        {#if project.skills_allow?.length}
          {#each project.skills_allow as skill}
            <span class="badge badge-info">{skill}</span>
          {/each}
        {/if}
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

    <!-- State + Board -->
    {#if projectState || board}
      <div class="pv-state-board">
        {#if projectState}
          <div class="card pv-state-card">
            <span class="card-title">State</span>
            <dl class="pv-facts">
              <div><dt>Phase</dt><dd>
                <span class="badge {statusBadgeClass(projectState.phase || '')}">{projectState.phase || '\u2014'}</span>
                {#if projectState.phase_number}
                  <span style="margin-left:var(--space-1);font-size:var(--text-xs);color:var(--text-ghost)">#{projectState.phase_number}{#if project?.max_phases}/{project.max_phases}{/if}</span>
                {/if}
              </dd></div>
              <div><dt>Status</dt><dd><span class="badge {statusBadgeClass(projectState.status || '')}">{projectState.status || '\u2014'}</span></dd></div>
              {#if projectState.next_action}
                <div><dt>Next</dt><dd>{projectState.next_action}</dd></div>
              {/if}
              {#if projectState.last_run_summary}
                <div><dt>Summary</dt><dd>{projectState.last_run_summary}</dd></div>
              {/if}
            </dl>
          </div>
        {/if}
        <div class="card pv-board-card">
          <div class="card-header">
            <span class="card-title">Board</span>
            <span class="badge badge-default">{board?.tasks.length ?? 0} tasks</span>
          </div>
          {#if board && board.tasks.length > 0}
            <div class="pv-board-tasks">
              {#each board.tasks as task}
                <div class="pv-board-task">
                  <select
                    class="pv-board-status-select"
                    value={task.status}
                    disabled={boardBusy}
                    onchange={(e) => changeTaskStatus(task.id, (e.target as HTMLSelectElement).value)}
                  >
                    <option value="todo">todo</option>
                    <option value="in_progress">in_progress</option>
                    <option value="review">review</option>
                    <option value="done">done</option>
                  </select>
                  {#if editingTaskId === task.id}
                    <input
                      class="pv-board-task-edit"
                      bind:value={editingTaskTitle}
                      onkeydown={(e) => { if (e.key === 'Enter') commitEditTask(); if (e.key === 'Escape') editingTaskId = null }}
                      onblur={() => commitEditTask()}
                    />
                  {:else}
                    <button class="pv-board-task-title-btn" onclick={() => startEditTask(task)} title="Click to edit">
                      {task.title}
                    </button>
                  {/if}
                  {#if task.worker_kind}
                    <span class="pv-board-task-meta">{task.worker_kind}</span>
                  {/if}
                  <button class="pv-board-task-remove" disabled={boardBusy} onclick={() => removeTask(task.id)} title="Remove">&times;</button>
                </div>
              {/each}
            </div>
          {:else}
            <div class="pv-board-empty">No tasks. Add one below.</div>
          {/if}
          <form class="pv-board-add" onsubmit={(e) => { e.preventDefault(); addTask() }}>
            <input
              class="pv-board-add-input"
              type="text"
              placeholder="New task..."
              bind:value={newTaskTitle}
              disabled={boardBusy}
            />
            <button class="btn btn-primary btn-sm" type="submit" disabled={!newTaskTitle.trim() || boardBusy}>Add</button>
          </form>
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
          {#if project.source_path}
            <div><dt>Source Path</dt><dd class="mono">{project.source_path}</dd></div>
          {/if}
          <div><dt>Workspace Path</dt><dd class="mono">{project.path || '\u2014'}</dd></div>
          <div><dt>Repo</dt><dd class="mono">{project.git_repo || '\u2014'}</dd></div>
        </dl>
      </section>

      <!-- Sub-Agents -->
      {#if project.sub_agents && project.sub_agents.length > 0}
        <section class="card">
          <div class="card-header">
            <span class="card-title">Sub-Agents</span>
            <span class="badge badge-default">{project.sub_agents.length}</span>
          </div>
          <div class="pv-agents-list">
            {#each project.sub_agents as agent}
              {@const role = typeof agent === 'string' ? agent : agent.role}
              {@const desc = typeof agent === 'string' ? '' : (agent.description || '')}
              {@const trigger = typeof agent === 'string' ? 'phase_done' : (agent.run_after || 'phase_done')}
              {@const agentActivity = activity.filter((a) => a.source === role)}
              <div class="pv-agent-item">
                <div class="pv-agent-top">
                  <span class="badge badge-info">{role}</span>
                  <span class="badge badge-default">{trigger}</span>
                  <span class="pv-agent-count">{agentActivity.length} activities</span>
                </div>
                {#if desc}
                  <div class="pv-agent-desc">{desc}</div>
                {/if}
                {#if agentActivity.length > 0}
                  <div class="pv-agent-last">
                    <span class="pv-agent-last-label">Last:</span>
                    <span class="pv-agent-last-msg">{agentActivity[0].message?.slice(0, 100) || 'No message'}{(agentActivity[0].message?.length ?? 0) > 100 ? '...' : ''}</span>
                    <span class="pv-agent-last-time">{fmt(agentActivity[0].timestamp)}</span>
                  </div>
                {:else}
                  <div class="pv-agent-idle">No activity yet</div>
                {/if}
              </div>
            {/each}
          </div>
        </section>
      {/if}

      <!-- Chat -->
      <section class="card pv-wide">
        <span class="card-title">Chat</span>
        <ChatPanel {projectId} onSessionChange={refreshAll} />
      </section>

      <!-- Activity -->
      <section class="card pv-wide">
        <div class="card-header">
          <span class="card-title">Activity</span>
          <span class="badge badge-default">{filteredActivity().length}</span>
          {#if activitySources().length > 1}
            <div class="pv-activity-filters">
              <button class="btn btn-ghost btn-sm" class:active={activityFilter === 'all'} onclick={() => { activityFilter = 'all' }}>all</button>
              {#each activitySources() as src}
                <button class="btn btn-ghost btn-sm" class:active={activityFilter === src} onclick={() => { activityFilter = src }}>{src}</button>
              {/each}
            </div>
          {/if}
          {#if filteredActivity().length > 5 && !showAllActivity}
            <button class="btn btn-ghost btn-sm" onclick={() => { showAllActivity = true }}>Show all</button>
          {:else if showAllActivity}
            <button class="btn btn-ghost btn-sm" onclick={() => { showAllActivity = false }}>Collapse</button>
          {/if}
        </div>
        {#if filteredActivity().length === 0}
          <div class="empty-state"><p>No activity recorded yet.</p></div>
        {:else}
          <div class="pv-timeline">
            {#each (showAllActivity ? filteredActivity() : filteredActivity().slice(0, 5)) as item}
              <div class="pv-timeline-item">
                <div class="pv-timeline-top">
                  <strong>{item.kind}</strong>
                  <span class="badge badge-default">{item.status || item.source}</span>
                  {#if item.source && item.source !== 'system'}
                    <span class="badge badge-info">{item.source}</span>
                  {/if}
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

  /* ── State + Board ────────────────────────────── */
  .pv-state-board {
    display: grid;
    grid-template-columns: 1fr;
    gap: var(--space-4);
    margin-bottom: var(--space-4);
  }

  .pv-state-card {
    min-width: 0;
  }

  .pv-board-tasks {
    display: grid;
    gap: var(--space-1);
  }

  .pv-board-card {
    min-width: 0;
    overflow: hidden;
  }

  .pv-board-task {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    background: var(--bg-base);
    border-radius: var(--radius-sm);
    min-width: 0;
    overflow: hidden;
  }

  .pv-board-task-title {
    font-size: var(--text-sm);
    color: var(--text-primary);
    flex: 1;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .pv-board-task-meta {
    font-size: var(--text-xs);
    color: var(--text-ghost);
    font-family: var(--font-mono);
    flex-shrink: 0;
  }

  .pv-board-status-select {
    padding: 2px 4px;
    font-size: var(--text-xs);
    background: var(--bg-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    flex-shrink: 0;
  }

  .pv-board-task-title-btn {
    flex: 1;
    min-width: 0;
    background: none;
    border: none;
    text-align: left;
    cursor: pointer;
    font-size: var(--text-sm);
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    padding: 0;
  }
  .pv-board-task-title-btn:hover {
    color: var(--accent);
  }

  .pv-board-task-edit {
    flex: 1;
    min-width: 0;
    padding: 2px var(--space-1);
    font-size: var(--text-sm);
    background: var(--bg-base);
    border: 1px solid var(--accent);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    outline: none;
  }

  .pv-board-task-remove {
    background: none;
    border: none;
    color: var(--text-ghost);
    cursor: pointer;
    font-size: 14px;
    padding: 0 4px;
    flex-shrink: 0;
    line-height: 1;
  }
  .pv-board-task-remove:hover { color: var(--error); }

  .pv-board-empty {
    padding: var(--space-3);
    color: var(--text-ghost);
    font-size: var(--text-sm);
  }

  .pv-board-add {
    display: flex;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-top: 1px solid var(--border-subtle);
  }

  .pv-board-add-input {
    flex: 1;
    padding: var(--space-1) var(--space-2);
    background: var(--bg-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    font-size: var(--text-sm);
  }
  .pv-board-add-input:focus {
    outline: none;
    border-color: var(--accent);
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

  /* ── Sub-Agents ───────────────────────────── */
  .pv-agents-list {
    display: grid;
    gap: var(--space-2);
  }

  .pv-agent-item {
    padding: var(--space-2) var(--space-3);
    background: var(--bg-base);
    border-radius: var(--radius-sm);
  }

  .pv-agent-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--space-1);
  }

  .pv-agent-count {
    font-size: var(--text-xs);
    color: var(--text-ghost);
    margin-left: auto;
  }

  .pv-agent-desc {
    font-size: var(--text-xs);
    color: var(--text-secondary);
    margin-top: 2px;
    line-height: 1.4;
  }

  .pv-agent-last {
    display: grid;
    gap: 2px;
    font-size: var(--text-xs);
  }

  .pv-agent-last-label {
    color: var(--text-tertiary);
  }

  .pv-agent-last-msg {
    color: var(--text-secondary);
    line-height: 1.4;
  }

  .pv-agent-last-time {
    color: var(--text-ghost);
  }

  .pv-agent-idle {
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  /* ── Activity filters ─────────────────────── */
  .pv-activity-filters {
    display: flex;
    gap: var(--space-1);
  }
  .pv-activity-filters .active {
    color: var(--accent);
    border-color: var(--accent);
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
