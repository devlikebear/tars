<script lang="ts">
  import { tick } from 'svelte'
  import type { Artifact } from '../lib/artifacts'
  import { fileIcon } from '../lib/artifacts'
  import { listWorkspaceFiles, readWorkspaceFile, getSessionWorkDirs, updateSessionWorkDirs, browseFilesystem, createFilesystemDirectory, createWorkspaceDirectory, renameWorkspaceDirectory, type WorkspaceFileEntry, type WorkspaceFileContent } from '../lib/api'
  import { renderHighlightedCodeBlock } from '../lib/markdown'
  import type { SessionWorkDirs } from '../lib/types'
  import MarkdownContent from './MarkdownContent.svelte'

  interface Props {
    artifacts: Artifact[]
    sessionId: string
    onClose: () => void
  }

  let { artifacts, sessionId, onClose }: Props = $props()

  type Tab = 'session' | 'workspace'
  let activeTab: Tab = $state('workspace')
  let activeTabInitialized = $state(false)

  // WorkDirs state
  let workDirs: SessionWorkDirs = $state({ work_dirs: [], current_dir: '' })
  let pickingDir = $state(false)
  let pickPath = $state('')
  let pickParent = $state('')
  let pickFiles: WorkspaceFileEntry[] = $state([])
  let pickLoading = $state(false)
  let pickActionError = $state('')
  let pickActionBusy = $state(false)
  let pickCreatingFolder = $state(false)
  let pickNewFolderName = $state('')

  // Workspace browser state
  let currentPath = $state('.')
  let wsFiles: WorkspaceFileEntry[] = $state([])
  let wsLoading = $state(false)
  let wsError = $state('')
  let wsActionError = $state('')
  let wsActionBusy = $state(false)
  let creatingFolder = $state(false)
  let newFolderName = $state('')
  let renamingDirPath = $state('')
  let renameDirName = $state('')

  // File preview state
  let previewFile: WorkspaceFileContent | null = $state(null)
  let previewLoading = $state(false)
  let previewError = $state('')
  let copied = $state(false)
  let previewMode = $state<'primary' | 'raw'>('primary')
  let imageZoom = $state(1)
  let sessionArtifactListEl: HTMLDivElement | undefined = $state()
  let activeArtifactPath = $state('')

  const effectiveRoot = $derived(workDirs.current_dir || undefined)
  const mandatoryWorkDir = $derived(workDirs.work_dirs[0] || '')

  async function loadWorkDirs(): Promise<SessionWorkDirs | null> {
    if (!sessionId) return null
    try {
      workDirs = await getSessionWorkDirs(sessionId)
      return workDirs
    } catch {
      return null
    }
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
    if (dir === mandatoryWorkDir) return
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
    pickActionError = ''
    pickActionBusy = false
    pickCreatingFolder = false
    pickNewFolderName = ''
    await browsePick(undefined)
  }

  function cancelPicking() {
    pickingDir = false
    pickActionError = ''
    pickActionBusy = false
    pickCreatingFolder = false
    pickNewFolderName = ''
  }

  async function browsePick(path: string | undefined) {
    pickLoading = true
    pickActionError = ''
    try {
      const result = await browseFilesystem(path)
      pickFiles = result.entries.filter(e => e.is_dir).map(e => ({
        name: e.name,
        path: joinFilesystemPath(result.path, e.name),
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
    const dirs = Array.from(new Set([...workDirs.work_dirs, absPath]))
    await updateSessionWorkDirs(sessionId, { work_dirs: dirs, current_dir: absPath })
    workDirs = { work_dirs: dirs, current_dir: absPath }
    pickingDir = false
    currentPath = '.'
    await browseDir('.')
  }

  function beginPickCreateFolder() {
    pickActionError = ''
    pickCreatingFolder = true
    pickNewFolderName = ''
  }

  function cancelPickCreateFolder() {
    pickCreatingFolder = false
    pickNewFolderName = ''
  }

  async function submitPickCreateFolder() {
    const name = pickNewFolderName.trim()
    if (!name || !pickPath || pickActionBusy) return
    pickActionBusy = true
    pickActionError = ''
    try {
      const created = await createFilesystemDirectory(pickPath, name)
      pickCreatingFolder = false
      pickNewFolderName = ''
      await browsePick(created.path)
    } catch (err) {
      pickActionError = err instanceof Error ? err.message : 'Failed to create folder'
    } finally {
      pickActionBusy = false
    }
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

  function beginCreateFolder() {
    wsActionError = ''
    renamingDirPath = ''
    renameDirName = ''
    creatingFolder = true
    newFolderName = ''
  }

  function cancelCreateFolder() {
    creatingFolder = false
    newFolderName = ''
  }

  async function submitCreateFolder() {
    const name = newFolderName.trim()
    if (!name || wsActionBusy) return
    wsActionBusy = true
    wsActionError = ''
    try {
      await createWorkspaceDirectory(currentPath, name, effectiveRoot)
      creatingFolder = false
      newFolderName = ''
      await browseDir(currentPath)
    } catch (err) {
      wsActionError = err instanceof Error ? err.message : 'Failed to create folder'
    } finally {
      wsActionBusy = false
    }
  }

  function startRenameFolder(entry: WorkspaceFileEntry) {
    wsActionError = ''
    creatingFolder = false
    newFolderName = ''
    renamingDirPath = entry.path
    renameDirName = entry.name
  }

  function cancelRenameFolder() {
    renamingDirPath = ''
    renameDirName = ''
  }

  async function submitRenameFolder() {
    const nextName = renameDirName.trim()
    if (!renamingDirPath || !nextName || wsActionBusy) return
    wsActionBusy = true
    wsActionError = ''
    try {
      await renameWorkspaceDirectory(renamingDirPath, nextName, effectiveRoot)
      renamingDirPath = ''
      renameDirName = ''
      await browseDir(currentPath)
    } catch (err) {
      wsActionError = err instanceof Error ? err.message : 'Failed to rename folder'
    } finally {
      wsActionBusy = false
    }
  }

  async function openFile(path: string, rootOverride?: string) {
    previewLoading = true
    previewError = ''
    previewFile = null
    copied = false
    imageZoom = 1
    previewMode = 'primary'
    try {
      previewFile = await readWorkspaceFile(path, rootOverride || effectiveRoot)
      previewMode = defaultPreviewMode(previewFile)
    } catch (err) {
      previewError = err instanceof Error ? err.message : 'Failed to read file'
    } finally {
      previewLoading = false
    }
  }

  function closePreview() {
    previewFile = null
    previewError = ''
    previewMode = 'primary'
    imageZoom = 1
  }

  function downloadFile() {
    if (!previewFile) return
    let blob: Blob
    if (previewFile.content_base64) {
      const bytes = Uint8Array.from(atob(previewFile.content_base64), (char) => char.charCodeAt(0))
      blob = new Blob([bytes], { type: previewFile.mime_type || 'application/octet-stream' })
    } else if (previewFile.content !== undefined) {
      blob = new Blob([previewFile.content], { type: previewFile.mime_type || 'text/plain;charset=utf-8' })
    } else {
      return
    }
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = previewFile.name
    a.click()
    URL.revokeObjectURL(url)
  }

  function copyContent() {
    if (!previewFile?.content) return
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
    const rootLabel = effectiveRoot ? workDirLabel(effectiveRoot) : defaultWorkDirLabel()
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
    if (new Date(d).getFullYear() <= 1) return ''
    const seconds = Math.floor((Date.now() - d) / 1000)
    if (seconds < 60) return `${seconds}s ago`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
    return `${Math.floor(seconds / 86400)}d ago`
  }

  function basename(path: string): string {
    return path.split('/').pop() || path
  }

  function joinFilesystemPath(parent: string, name: string): string {
    if (parent === '/') return `/${name}`
    return `${parent.replace(/\/+$/, '')}/${name}`
  }

  function dirname(path: string): string {
    const parts = path.split('/')
    return parts.length <= 1 ? '' : parts.slice(0, -1).join('/')
  }

  function defaultWorkDirLabel(): string {
    return sessionId ? `artifacts/${sessionId}` : 'artifacts/'
  }

  function workDirLabel(dir: string): string {
    if (!dir) return defaultWorkDirLabel()
    const artifactSuffix = `/artifacts/${sessionId}`
    if (sessionId && dir.endsWith(artifactSuffix)) {
      return `artifacts/${sessionId}`
    }
    return basename(dir)
  }

  const codeExtensions = new Set([
    'c', 'cc', 'cpp', 'cs', 'css', 'diff', 'dockerfile', 'go', 'graphql', 'h', 'hpp',
    'html', 'java', 'js', 'json', 'jsx', 'kt', 'mjs', 'php', 'py', 'rb', 'rs',
    'scss', 'sh', 'sql', 'svelte', 'swift', 'toml', 'ts', 'tsx', 'vue', 'xml', 'yaml', 'yml',
  ])

  function extension(name: string): string {
    return name.split('.').pop()?.toLowerCase() || ''
  }

  function isCodePreview(file: WorkspaceFileContent): boolean {
    if (file.kind !== 'text') return false
    return codeExtensions.has(extension(file.name))
  }

  function hasRawMode(file: WorkspaceFileContent): boolean {
    return file.kind === 'markdown' || isCodePreview(file)
  }

  function defaultPreviewMode(file: WorkspaceFileContent): 'primary' | 'raw' {
    return 'primary'
  }

  function primaryModeLabel(file: WorkspaceFileContent): string {
    if (file.kind === 'markdown') return 'Preview'
    if (file.kind === 'image') return 'Preview'
    if (isCodePreview(file)) return 'Code'
    return 'Text'
  }

  function guessCodeLanguage(file: WorkspaceFileContent): string | undefined {
    const ext = extension(file.name)
    if (ext) return ext
    if (file.mime_type.includes('json')) return 'json'
    if (file.mime_type.includes('xml')) return 'xml'
    if (file.mime_type.includes('yaml')) return 'yaml'
    if (file.mime_type.includes('javascript')) return 'javascript'
    if (file.mime_type.includes('typescript')) return 'typescript'
    return undefined
  }

  function canCopyPreview(file: WorkspaceFileContent): boolean {
    return !!file.content && file.kind !== 'image' && file.kind !== 'binary'
  }

  function canDownloadPreview(file: WorkspaceFileContent): boolean {
    return !!file.content || !!file.content_base64
  }

  function imagePreviewSrc(file: WorkspaceFileContent): string {
    if (!file.content_base64) return ''
    return `data:${file.mime_type || 'image/*'};base64,${file.content_base64}`
  }

  function zoomIn() {
    imageZoom = Math.min(4, Number((imageZoom + 0.25).toFixed(2)))
  }

  function zoomOut() {
    imageZoom = Math.max(0.25, Number((imageZoom - 0.25).toFixed(2)))
  }

  function resetZoom() {
    imageZoom = 1
  }

  function handleFileClick(entry: WorkspaceFileEntry) {
    if (entry.is_dir) {
      browseDir(entry.path)
    } else {
      openFile(entry.path)
    }
  }

  async function openArtifactPreview(path: string, mandatoryDir: string) {
    const artifactSuffix = `/artifacts/${sessionId}`
    if (sessionId && path.startsWith('workspace/') && mandatoryDir.endsWith(artifactSuffix)) {
      const workspaceRoot = mandatoryDir.slice(0, -artifactSuffix.length)
      await openFile(path, workspaceRoot)
      return
    }
    await openFile(path)
  }

  function focusArtifactItem(path: string) {
    if (!sessionArtifactListEl) return
    for (const item of sessionArtifactListEl.querySelectorAll<HTMLElement>('[data-artifact-path]')) {
      if (item.dataset.artifactPath === path) {
        item.scrollIntoView({ block: 'nearest' })
        break
      }
    }
  }

  // Also allow opening session artifact files
  async function openArtifactFile(artifact: Artifact) {
    await openArtifactPath(artifact.path)
  }

  export function refresh() {
    void browseDir(currentPath)
  }

  export async function openArtifactPath(path: string) {
    activeTab = 'session'
    activeArtifactPath = path
    const loadedWorkDirs = await loadWorkDirs()
    await openArtifactPreview(path, loadedWorkDirs?.work_dirs[0] || mandatoryWorkDir)
    await tick()
    focusArtifactItem(path)
  }

  $effect(() => {
    if (!activeTabInitialized) {
      activeTab = artifacts.length > 0 ? 'session' : 'workspace'
      activeTabInitialized = true
      return
    }
    if (artifacts.length === 0 && activeTab === 'session') {
      activeTab = 'workspace'
    }
  })

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
    <div class="artifact-list" bind:this={sessionArtifactListEl}>
      {#if artifacts.length === 0}
        <div class="artifact-empty">No files created in this session yet.</div>
      {:else}
        {#each artifacts as artifact}
          <button
            type="button"
            class="artifact-item"
            class:active={activeArtifactPath === artifact.path}
            data-artifact-path={artifact.path}
            onclick={() => openArtifactFile(artifact)}
          >
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
              <option value={dir}>{workDirLabel(dir)}</option>
            {/each}
          </select>
          <button
            type="button"
            class="btn btn-ghost btn-sm"
            title={workDirs.current_dir === mandatoryWorkDir ? 'Session artifact directory is required' : 'Remove current directory'}
            disabled={workDirs.current_dir === mandatoryWorkDir}
            onclick={() => removeDir(workDirs.current_dir)}
          >&#x2212;</button>
          <button type="button" class="btn btn-ghost btn-sm" title="Add directory" onclick={startPicking}>+</button>
        </div>
      {:else}
        <div class="workdir-list">
          <span class="workdir-default">{defaultWorkDirLabel()}</span>
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
            <button type="button" class="btn btn-primary btn-sm" disabled={pickLoading || pickActionBusy || !pickPath} onclick={selectPickedDir}>Select Here</button>
            <button type="button" class="btn btn-ghost btn-sm" disabled={pickActionBusy} onclick={cancelPicking}>Cancel</button>
          </div>
        </div>
        <div class="pick-current">{pickPath || '/'}</div>
        <div class="pick-toolbar">
          {#if pickCreatingFolder}
            <div class="ws-inline-form">
              <span class="artifact-icon">&#x1f4c1;</span>
              <input
                class="ws-inline-input"
                bind:value={pickNewFolderName}
                placeholder="New folder name"
                onkeydown={(e) => {
                  if (e.key === 'Enter') submitPickCreateFolder()
                  if (e.key === 'Escape') cancelPickCreateFolder()
                }}
              />
              <button type="button" class="btn btn-primary btn-sm" disabled={pickActionBusy || !pickNewFolderName.trim()} onclick={submitPickCreateFolder}>Create</button>
              <button type="button" class="btn btn-ghost btn-sm" disabled={pickActionBusy} onclick={cancelPickCreateFolder}>Cancel</button>
            </div>
          {:else}
            <button type="button" class="btn btn-ghost btn-sm" disabled={pickLoading || pickActionBusy || !pickPath} onclick={beginPickCreateFolder}>New Folder</button>
          {/if}
        </div>
        {#if pickActionError}
          <div class="pick-error">{pickActionError}</div>
        {/if}
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

    <div class="ws-toolbar">
      {#if creatingFolder}
        <div class="ws-inline-form">
          <span class="artifact-icon">&#x1f4c1;</span>
          <input
            class="ws-inline-input"
            bind:value={newFolderName}
            placeholder="New folder name"
            onkeydown={(e) => {
              if (e.key === 'Enter') submitCreateFolder()
              if (e.key === 'Escape') cancelCreateFolder()
            }}
          />
          <button type="button" class="btn btn-primary btn-sm" disabled={wsActionBusy || !newFolderName.trim()} onclick={submitCreateFolder}>Create</button>
          <button type="button" class="btn btn-ghost btn-sm" disabled={wsActionBusy} onclick={cancelCreateFolder}>Cancel</button>
        </div>
      {:else}
        <button type="button" class="btn btn-ghost btn-sm" disabled={wsLoading || wsActionBusy} onclick={beginCreateFolder}>New Folder</button>
      {/if}
    </div>

    {#if wsActionError}
      <div class="ws-inline-error">{wsActionError}</div>
    {/if}

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
          {#if renamingDirPath === entry.path}
            <div class="artifact-item-row artifact-item-row-editing">
              <div class="artifact-item artifact-item-static">
                <span class="artifact-icon">&#x1f4c1;</span>
                <div class="artifact-info">
                  <input
                    class="ws-inline-input"
                    bind:value={renameDirName}
                    placeholder="Folder name"
                    onkeydown={(e) => {
                      if (e.key === 'Enter') submitRenameFolder()
                      if (e.key === 'Escape') cancelRenameFolder()
                    }}
                  />
                </div>
                <div class="artifact-item-actions">
                  <button type="button" class="btn btn-primary btn-sm" disabled={wsActionBusy || !renameDirName.trim()} onclick={submitRenameFolder}>Save</button>
                  <button type="button" class="btn btn-ghost btn-sm" disabled={wsActionBusy} onclick={cancelRenameFolder}>Cancel</button>
                </div>
              </div>
            </div>
          {:else}
            <div class="artifact-item-row">
              <button type="button" class="artifact-item artifact-item-main" onclick={() => handleFileClick(entry)}>
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
              {#if entry.is_dir}
                <button type="button" class="artifact-row-action" disabled={wsActionBusy} onclick={() => startRenameFolder(entry)}>Rename</button>
              {/if}
            </div>
          {/if}
        {/each}
      {/if}
    </div>
  {/if}
</div>

<!-- File Preview Modal -->
{#if previewFile || previewLoading || previewError}
  <div
    class="preview-overlay"
    onclick={(e) => {
      if (e.target === e.currentTarget) closePreview()
    }}
    onkeydown={(e) => e.key === 'Escape' && closePreview()}
    role="dialog"
    tabindex="-1"
  >
    <div class="preview-modal" role="document">
      <div class="preview-header">
        <div class="preview-title-row">
          <span class="artifact-icon">{previewFile ? fileIcon(previewFile.name) : ''}</span>
          <span class="preview-filename">{previewFile?.name || 'Loading...'}</span>
          {#if previewFile}
            <span class="preview-size">{formatSize(previewFile.size)}</span>
            <span class="preview-mime">{previewFile.mime_type}</span>
          {/if}
        </div>
        <div class="preview-actions">
          {#if previewFile}
            {#if previewFile.kind !== 'binary'}
              <div class="preview-modes">
                <button
                  type="button"
                  class="preview-mode-btn"
                  class:active={previewMode === 'primary'}
                  onclick={() => { previewMode = 'primary' }}
                >{primaryModeLabel(previewFile)}</button>
                {#if hasRawMode(previewFile)}
                  <button
                    type="button"
                    class="preview-mode-btn"
                    class:active={previewMode === 'raw'}
                    onclick={() => { previewMode = 'raw' }}
                  >Raw</button>
                {/if}
              </div>
            {/if}
            {#if canCopyPreview(previewFile)}
              <button type="button" class="btn btn-ghost btn-sm" onclick={copyContent}>
                {copied ? 'Copied!' : 'Copy'}
              </button>
            {/if}
            {#if canDownloadPreview(previewFile)}
              <button type="button" class="btn btn-ghost btn-sm" onclick={downloadFile}>Download</button>
            {/if}
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
          {#if previewFile.message}
            <div class="preview-notice">{previewFile.message}</div>
          {/if}

          {#if previewFile.kind === 'binary'}
            <div class="preview-binary">
              <div class="preview-binary-title">Binary file</div>
              <div class="preview-binary-meta">{previewFile.mime_type} · {formatSize(previewFile.size)}</div>
            </div>
          {:else if previewFile.kind === 'image'}
            {#if previewFile.content_base64}
              <div class="preview-image-toolbar">
                <button type="button" class="btn btn-ghost btn-sm" onclick={zoomOut}>-</button>
                <button type="button" class="btn btn-ghost btn-sm" onclick={resetZoom}>{Math.round(imageZoom * 100)}%</button>
                <button type="button" class="btn btn-ghost btn-sm" onclick={zoomIn}>+</button>
              </div>
              <div class="preview-image-stage">
                <img
                  class="preview-image"
                  src={imagePreviewSrc(previewFile)}
                  alt={previewFile.name}
                  style={`transform: scale(${imageZoom});`}
                />
              </div>
            {:else}
              <div class="artifact-empty">Image preview unavailable.</div>
            {/if}
          {:else if previewMode === 'raw'}
            <pre class="preview-content"><code>{previewFile.content || ''}</code></pre>
          {:else if previewFile.kind === 'markdown'}
            <div class="preview-markdown">
              <MarkdownContent text={previewFile.content || ''} />
            </div>
          {:else if isCodePreview(previewFile)}
            <div class="chat-md preview-code-surface">
              {@html renderHighlightedCodeBlock(previewFile.content || '', guessCodeLanguage(previewFile))}
            </div>
          {:else}
            <pre class="preview-content"><code>{previewFile.content || ''}</code></pre>
          {/if}
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

  .pick-toolbar {
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
  }

  .pick-error {
    padding: 0 var(--space-3) var(--space-2);
    color: var(--error);
    font-size: 10px;
    flex-shrink: 0;
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

  .ws-toolbar {
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
  }

  .ws-inline-form {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .ws-inline-input {
    flex: 1;
    min-width: 0;
    padding: 6px 8px;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    background: var(--bg-inset);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: 11px;
  }

  .ws-inline-error {
    padding: 0 var(--space-3) var(--space-2);
    color: var(--error);
    font-size: 10px;
    flex-shrink: 0;
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
  .artifact-item.active {
    background: rgba(224, 145, 69, 0.12);
    box-shadow: inset 0 0 0 1px rgba(224, 145, 69, 0.35);
  }

  .artifact-item-row {
    display: flex;
    align-items: center;
    gap: var(--space-1);
  }

  .artifact-item-main {
    flex: 1;
  }

  .artifact-item-static {
    flex: 1;
    cursor: default;
  }

  .artifact-item-row-editing {
    padding: var(--space-1) 0;
  }

  .artifact-item-actions {
    display: flex;
    gap: var(--space-1);
    flex-shrink: 0;
  }

  .artifact-row-action {
    flex-shrink: 0;
    background: transparent;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
    cursor: pointer;
    padding: 3px 8px;
    transition: all var(--duration-fast) var(--ease-out);
  }
  .artifact-row-action:hover:not(:disabled) {
    color: var(--accent);
    border-color: var(--accent);
  }
  .artifact-row-action:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

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

  .preview-mime {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
  }

  .preview-actions {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    flex-shrink: 0;
  }

  .preview-modes {
    display: flex;
    gap: 2px;
    margin-right: var(--space-1);
  }

  .preview-mode-btn {
    padding: 2px 8px;
    background: transparent;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
    cursor: pointer;
  }
  .preview-mode-btn.active {
    color: var(--accent);
    border-color: var(--accent);
    background: rgba(224, 145, 69, 0.08);
  }

  .preview-body {
    flex: 1;
    overflow: auto;
    padding: var(--space-3);
  }

  .preview-notice {
    margin-bottom: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--bg-inset);
    color: var(--text-secondary);
    font-size: var(--text-xs);
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

  .preview-markdown :global(.chat-md) {
    margin: 0;
  }

  .preview-code-surface :global(.code-block) {
    margin: 0;
  }

  .preview-code-surface :global(.code-block pre) {
    border-radius: var(--radius-md);
  }

  .preview-binary {
    padding: var(--space-5);
    border: 1px dashed var(--border-subtle);
    border-radius: var(--radius-md);
    text-align: center;
  }

  .preview-binary-title {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    color: var(--text-primary);
    margin-bottom: var(--space-2);
  }

  .preview-binary-meta {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
  }

  .preview-image-toolbar {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-1);
    margin-bottom: var(--space-2);
  }

  .preview-image-stage {
    min-height: 320px;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: auto;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background:
      linear-gradient(45deg, rgba(255,255,255,0.03) 25%, transparent 25%, transparent 75%, rgba(255,255,255,0.03) 75%),
      linear-gradient(45deg, rgba(255,255,255,0.03) 25%, transparent 25%, transparent 75%, rgba(255,255,255,0.03) 75%);
    background-size: 20px 20px;
    background-position: 0 0, 10px 10px;
    padding: var(--space-4);
  }

  .preview-image {
    max-width: 100%;
    max-height: 70vh;
    transform-origin: center center;
    transition: transform var(--duration-fast) var(--ease-out);
  }
</style>
