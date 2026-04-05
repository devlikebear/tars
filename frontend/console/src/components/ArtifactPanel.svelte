<script lang="ts">
  import type { Artifact } from '../lib/artifacts'
  import { fileIcon } from '../lib/artifacts'
  import { listWorkspaceFiles, readWorkspaceFile, getSessionWorkDirs, updateSessionWorkDirs, browseFilesystem, type WorkspaceFileEntry, type WorkspaceFileContent } from '../lib/api'
  import type { SessionWorkDirs } from '../lib/types'

  interface Props {
    artifacts: Artifact[]
    sessionId: string
    onClose: () => void
  }

  let { artifacts, sessionId, onClose }: Props = $props()

  type Tab = 'session' | 'workspace'
  let activeTab: Tab = $state(artifacts.length > 0 ? 'session' : 'workspace')

  // WorkDirs state
  let workDirs: SessionWorkDirs = $state({ work_dirs: [], current_dir: '' })
  let pickingDir = $state(false)
  let pickPath = $state('')
  let pickParent = $state('')
  let pickFiles: WorkspaceFileEntry[] = $state([])
  let pickLoading = $state(false)

  // Workspace browser state
  let currentPath = $state('.')
  let wsFiles: WorkspaceFileEntry[] = $state([])
  let wsLoading = $state(false)
  let wsError = $state('')

  // File preview state
  let previewFile: WorkspaceFileContent | null = $state(null)
  let previewLoading = $state(false)
  let previewError = $state('')
  let copied = $state(false)

  const effectiveRoot = $derived(workDirs.current_dir || undefined)

  async function loadWorkDirs() {
    if (!sessionId) return
    try {
      workDirs = await getSessionWorkDirs(sessionId)
    } catch { /* ignore */ }
  }

  async function switchDir(dir: string) {
    if (!sessionId) return
    await updateSessionWorkDirs(sessionId, { ...workDirs, current_dir: dir })
    workDirs.current_dir = dir
    currentPath = '.'
    await browseDir('.')
  }

  async function removeDir(dir: string) {
    if (!sessionId) return
    const dirs = workDirs.work_dirs.filter(d => d !== dir)
    const cd = dir === workDirs.current_dir ? (dirs[0] || '') : workDirs.current_dir
    await updateSessionWorkDirs(sessionId, { work_dirs: dirs, current_dir: cd })
    workDirs = { work_dirs: dirs, current_dir: cd }
    currentPath = '.'
    await browseDir('.')
  }

  // Directory picker — browse filesystem to select a working directory
  async function startPicking() {
    pickingDir = true
    pickPath = ''
    pickParent = ''
    await browsePick(undefined)
  }

  function cancelPicking() {
    pickingDir = false
  }

  async function browsePick(path: string | undefined) {
    pickLoading = true
    try {
      const result = await browseFilesystem(path)
      pickFiles = result.entries.filter(e => e.is_dir).map(e => ({
        name: e.name,
        path: result.path + '/' + e.name,
        is_dir: true,
      }))
      pickPath = result.path
      pickParent = result.parent
    } catch {
      pickFiles = []
    } finally {
      pickLoading = false
    }
  }

  async function selectPickedDir() {
    if (!sessionId) return
    const absPath = pickPath
    const dirs = [...workDirs.work_dirs, absPath]
    await updateSessionWorkDirs(sessionId, { work_dirs: dirs, current_dir: absPath })
    workDirs = { work_dirs: dirs, current_dir: absPath }
    pickingDir = false
    currentPath = '.'
    await browseDir('.')
  }

  async function browseDir(path: string) {
    wsLoading = true
    wsError = ''
    try {
      const result = await listWorkspaceFiles(path, effectiveRoot)
      wsFiles = result.files || []
      currentPath = result.path || path
    } catch (err) {
      wsError = err instanceof Error ? err.message : 'Failed to list files'
    } finally {
      wsLoading = false
    }
  }

  async function openFile(path: string) {
    previewLoading = true
    previewError = ''
    previewFile = null
    try {
      previewFile = await readWorkspaceFile(path, effectiveRoot)
    } catch (err) {
      previewError = err instanceof Error ? err.message : 'Failed to read file'
    } finally {
      previewLoading = false
    }
  }

  function closePreview() {
    previewFile = null
    previewError = ''
  }

  function downloadFile() {
    if (!previewFile) return
    const blob = new Blob([previewFile.content], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = previewFile.name
    a.click()
    URL.revokeObjectURL(url)
  }

  function copyContent() {
    if (!previewFile) return
    navigator.clipboard.writeText(previewFile.content).then(() => {
      copied = true
      setTimeout(() => { copied = false }, 1500)
    }).catch(() => {})
  }

  function parentPath(path: string): string {
    const parts = path.split('/').filter(Boolean)
    if (parts.length <= 1) return '.'
    return parts.slice(0, -1).join('/')
  }

  function breadcrumbs(path: string): Array<{ label: string; path: string }> {
    const rootLabel = effectiveRoot ? (effectiveRoot.split('/').pop() || 'root') : 'workspace'
    if (path === '.') return [{ label: rootLabel, path: '.' }]
    const parts = path.split('/').filter(Boolean)
    const crumbs = [{ label: rootLabel, path: '.' }]
    for (let i = 0; i < parts.length; i++) {
      crumbs.push({ label: parts[i], path: parts.slice(0, i + 1).join('/') })
    }
    return crumbs
  }

  function formatSize(bytes: number | undefined): string {
    if (!bytes) return ''
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  function relativeTime(ts: number | string | undefined): string {
    if (!ts) return ''
    const d = typeof ts === 'number' ? ts : new Date(ts).getTime()
    const seconds = Math.floor((Date.now() - d) / 1000)
    if (seconds < 60) return `${seconds}s ago`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
    return `${Math.floor(seconds / 86400)}d ago`
  }

  function basename(path: string): string {
    return path.split('/').pop() || path
  }

  function dirname(path: string): string {
    const parts = path.split('/')
    return parts.length <= 1 ? '' : parts.slice(0, -1).join('/')
  }

  function handleFileClick(entry: WorkspaceFileEntry) {
    if (entry.is_dir) {
      browseDir(entry.path)
    } else {
      openFile(entry.path)
    }
  }

  // Also allow opening session artifact files
  function openArtifactFile(artifact: Artifact) {
    openFile(artifact.path)
  }

  export function refresh() {
    void browseDir(currentPath)
  }

  $effect(() => {
    if (activeTab === 'workspace') {
      void loadWorkDirs().then(() => browseDir(currentPath))
    }
  })
</script>

<div class="artifact-panel">
  <div class="artifact-header">
    <span class="artifact-title">Files</span>
    <div class="artifact-tabs">
      <button type="button" class="tab-btn" class:active={activeTab === 'session'} onclick={() => { activeTab = 'session' }}>
        Session{#if artifacts.length > 0} <span class="tab-count">{artifacts.length}</span>{/if}
      </button>
      <button type="button" class="tab-btn" class:active={activeTab === 'workspace'} onclick={() => { activeTab = 'workspace' }}>
        Workspace
      </button>
    </div>
    <button type="button" class="artifact-close" onclick={onClose}>&times;</button>
  </div>

  {#if activeTab === 'session'}
    <div class="artifact-list">
      {#if artifacts.length === 0}
        <div class="artifact-empty">No files created in this session yet.</div>
      {:else}
        {#each artifacts as artifact}
          <button type="button" class="artifact-item" onclick={() => openArtifactFile(artifact)}>
            <span class="artifact-icon">{fileIcon(artifact.path)}</span>
            <div class="artifact-info">
              <span class="artifact-name">{basename(artifact.path)}</span>
              {#if dirname(artifact.path)}
                <span class="artifact-dir">{dirname(artifact.path)}</span>
              {/if}
            </div>
            <div class="artifact-meta">
              <span class="badge {artifact.action === 'created' ? 'badge-success' : 'badge-accent'}" style="font-size:9px;padding:1px 5px">
                {artifact.action}
              </span>
              <span class="artifact-time">{relativeTime(artifact.timestamp)}</span>
            </div>
          </button>
        {/each}
      {/if}
    </div>

  {:else}
    <!-- WorkDir bar -->
    <div class="workdir-bar">
      {#if workDirs.work_dirs.length > 0}
        <div class="workdir-list">
          <select class="workdir-select" value={workDirs.current_dir} onchange={(e) => switchDir(e.currentTarget.value)}>
            {#each workDirs.work_dirs as dir}
              <option value={dir}>{dir.split('/').pop() || dir}</option>
            {/each}
          </select>
          <button type="button" class="btn btn-ghost btn-sm" title="Remove current directory" onclick={() => removeDir(workDirs.current_dir)}>&#x2212;</button>
          <button type="button" class="btn btn-ghost btn-sm" title="Add directory" onclick={startPicking}>+</button>
        </div>
      {:else}
        <div class="workdir-list">
          <span class="workdir-default">artifacts/</span>
          <button type="button" class="btn btn-ghost btn-sm" title="Add working directory" onclick={startPicking}>+</button>
        </div>
      {/if}
    </div>

    {#if pickingDir}
      <!-- Directory picker overlay -->
      <div class="pick-overlay">
        <div class="pick-header">
          <span class="pick-title">Select Directory</span>
          <div class="pick-actions">
            <button type="button" class="btn btn-primary btn-sm" onclick={selectPickedDir}>Select Here</button>
            <button type="button" class="btn btn-ghost btn-sm" onclick={cancelPicking}>Cancel</button>
          </div>
        </div>
        <div class="pick-current">{pickPath || '/'}</div>
        <div class="pick-list">
          {#if pickLoading}
            <div class="artifact-empty">Loading...</div>
          {:else}
            {#if pickParent}
              <button type="button" class="artifact-item" onclick={() => browsePick(pickParent)}>
                <span class="artifact-icon">&#x2191;</span>
                <span class="artifact-name">..</span>
              </button>
            {/if}
            {#each pickFiles as entry}
              <button type="button" class="artifact-item" onclick={() => browsePick(entry.path)}>
                <span class="artifact-icon">&#x1f4c1;</span>
                <span class="artifact-name">{entry.name}</span>
              </button>
            {/each}
            {#if pickFiles.length === 0 && pickParent}
              <div class="artifact-empty">No subdirectories</div>
            {/if}
          {/if}
        </div>
      </div>
    {/if}

    <!-- Workspace browser -->
    <div class="ws-breadcrumbs">
      {#each breadcrumbs(currentPath) as crumb, i}
        {#if i > 0}<span class="ws-sep">/</span>{/if}
        <button type="button" class="ws-crumb" class:active={i === breadcrumbs(currentPath).length - 1} onclick={() => browseDir(crumb.path)}>
          {crumb.label}
        </button>
      {/each}
    </div>

    <div class="artifact-list">
      {#if wsLoading}
        <div class="artifact-empty">Loading...</div>
      {:else if wsError}
        <div class="artifact-empty" style="color:var(--error)">{wsError}</div>
      {:else if wsFiles.length === 0}
        <div class="artifact-empty">Empty directory</div>
      {:else}
        {#if currentPath !== '.'}
          <button type="button" class="artifact-item ws-parent" onclick={() => browseDir(parentPath(currentPath))}>
            <span class="artifact-icon">&#x2191;</span>
            <div class="artifact-info"><span class="artifact-name">..</span></div>
          </button>
        {/if}
        {#each wsFiles as entry}
          <button type="button" class="artifact-item" onclick={() => handleFileClick(entry)}>
            <span class="artifact-icon">{entry.is_dir ? '\ud83d\udcc1' : fileIcon(entry.name)}</span>
            <div class="artifact-info">
              <span class="artifact-name">{entry.name}</span>
              {#if !entry.is_dir && entry.size}
                <span class="artifact-dir">{formatSize(entry.size)}</span>
              {/if}
            </div>
            {#if entry.updated_at}
              <span class="artifact-time">{relativeTime(entry.updated_at)}</span>
            {/if}
          </button>
        {/each}
      {/if}
    </div>
  {/if}
</div>

<!-- File Preview Modal -->
{#if previewFile || previewLoading || previewError}
  <div class="preview-overlay" onclick={closePreview} onkeydown={(e) => e.key === 'Escape' && closePreview()} role="dialog" tabindex="-1">
    <div class="preview-modal" onclick={(e) => e.stopPropagation()} role="document">
      <div class="preview-header">
        <div class="preview-title-row">
          <span class="artifact-icon">{previewFile ? fileIcon(previewFile.name) : ''}</span>
          <span class="preview-filename">{previewFile?.name || 'Loading...'}</span>
          {#if previewFile}
            <span class="preview-size">{formatSize(previewFile.size)}</span>
          {/if}
        </div>
        <div class="preview-actions">
          {#if previewFile}
            <button type="button" class="btn btn-ghost btn-sm" onclick={copyContent}>
              {copied ? 'Copied!' : 'Copy'}
            </button>
            <button type="button" class="btn btn-ghost btn-sm" onclick={downloadFile}>Download</button>
          {/if}
          <button type="button" class="btn btn-ghost btn-sm" onclick={closePreview}>&times;</button>
        </div>
      </div>
      <div class="preview-body">
        {#if previewLoading}
          <div class="artifact-empty">Loading file...</div>
        {:else if previewError}
          <div class="artifact-empty" style="color:var(--error)">{previewError}</div>
        {:else if previewFile}
          <pre class="preview-content"><code>{previewFile.content}</code></pre>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .artifact-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
  }

  .artifact-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
  }

  .artifact-title {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
    color: var(--text-primary);
  }

  .artifact-tabs {
    display: flex;
    gap: 1px;
    margin-left: var(--space-2);
  }

  .tab-btn {
    background: none;
    border: 1px solid var(--border-subtle);
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
    cursor: pointer;
    padding: 2px 8px;
    border-radius: var(--radius-sm);
    transition: all var(--duration-fast);
  }
  .tab-btn:hover { color: var(--text-primary); border-color: var(--border-default); }
  .tab-btn.active { color: var(--accent); border-color: var(--accent); background: rgba(224, 145, 69, 0.08); }

  .tab-count {
    font-size: 9px;
    background: var(--bg-elevated);
    padding: 0 4px;
    border-radius: var(--radius-sm);
  }

  .artifact-close {
    margin-left: auto;
    background: none;
    border: none;
    color: var(--text-ghost);
    cursor: pointer;
    font-size: var(--text-md);
    padding: 0 2px;
    line-height: 1;
  }
  .artifact-close:hover { color: var(--text-primary); }

  /* WorkDir bar */
  .workdir-bar {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
  }

  .workdir-list {
    display: flex;
    gap: var(--space-1);
    align-items: center;
  }

  .workdir-select {
    flex: 1;
    background: var(--bg-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: 11px;
    padding: 4px 8px;
  }

  .workdir-default {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-secondary);
    padding: 4px 0;
  }

  .pick-overlay {
    display: flex;
    flex-direction: column;
    flex: 1;
    overflow: hidden;
    border-top: 1px solid var(--accent);
    background: var(--bg-surface);
  }

  .pick-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
  }

  .pick-title {
    font-family: var(--font-display);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--accent-text);
  }

  .pick-actions {
    display: flex;
    gap: var(--space-1);
  }

  .pick-current {
    padding: var(--space-1) var(--space-3);
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-secondary);
    background: var(--bg-inset);
    border-bottom: 1px solid var(--border-subtle);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .pick-list {
    flex: 1;
    overflow-y: auto;
    padding: var(--space-2);
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .artifact-list {
    flex: 1;
    overflow-y: auto;
    padding: var(--space-2);
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .artifact-empty {
    padding: var(--space-4);
    text-align: center;
    color: var(--text-ghost);
    font-size: var(--text-xs);
  }

  .artifact-item {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2);
    border-radius: var(--radius-sm);
    background: none;
    border: none;
    width: 100%;
    text-align: left;
    cursor: pointer;
    color: var(--text-primary);
    transition: background var(--duration-fast) var(--ease-out);
  }
  .artifact-item:hover { background: var(--bg-hover); }

  .artifact-icon {
    font-size: var(--text-md);
    flex-shrink: 0;
    width: 20px;
    text-align: center;
  }

  .artifact-info { flex: 1; min-width: 0; }

  .artifact-name {
    display: block;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    font-weight: 500;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .artifact-dir {
    display: block;
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
  }

  .artifact-meta {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 2px;
    flex-shrink: 0;
  }

  .artifact-time {
    font-size: 10px;
    color: var(--text-ghost);
    flex-shrink: 0;
  }

  /* Workspace breadcrumbs */
  .ws-breadcrumbs {
    display: flex;
    align-items: center;
    gap: 2px;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
    overflow-x: auto;
  }

  .ws-crumb {
    background: none;
    border: none;
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: 10px;
    cursor: pointer;
    padding: 1px 4px;
    border-radius: var(--radius-sm);
  }
  .ws-crumb:hover { color: var(--text-primary); background: var(--bg-hover); }
  .ws-crumb.active { color: var(--accent); }
  .ws-sep { color: var(--text-ghost); font-size: 10px; }

  /* Preview modal */
  .preview-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    padding: var(--space-6);
  }

  .preview-modal {
    background: var(--bg-surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    width: 100%;
    max-width: 900px;
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .preview-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
  }

  .preview-title-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .preview-filename {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    font-weight: 500;
    color: var(--text-primary);
  }

  .preview-size {
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  .preview-actions {
    display: flex;
    gap: var(--space-1);
    flex-shrink: 0;
  }

  .preview-body {
    flex: 1;
    overflow: auto;
    padding: var(--space-3);
  }

  .preview-content {
    margin: 0;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    line-height: 1.6;
    color: var(--text-secondary);
    white-space: pre-wrap;
    word-break: break-all;
  }
</style>
