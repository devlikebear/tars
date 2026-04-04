<script lang="ts">
  import { onMount } from 'svelte'
  import {
    getSyspromptFile,
    listChatTools,
    listSyspromptFiles,
    saveSyspromptFile,
    type ChatToolInfo,
  } from '../lib/api'
  import type { SyspromptFile, SyspromptScope } from '../lib/types'

  let loading = $state(true)
  let saving = $state(false)
  let error = $state('')
  let success = $state('')

  let files: SyspromptFile[] = $state([])
  let selectedScope: SyspromptScope = $state('workspace')
  let selectedPath = $state('USER.md')
  let selectedFile: SyspromptFile | null = $state(null)
  let editorContent = $state('')
  let tools: ChatToolInfo[] = $state([])

  const relevantToolNames = [
    'workspace_sysprompt_get',
    'workspace_sysprompt_set',
    'agent_sysprompt_get',
    'agent_sysprompt_set',
  ]

  let workspaceFiles = $derived(files.filter((item) => item.scope === 'workspace'))
  let agentFiles = $derived(files.filter((item) => item.scope === 'agent'))
  let relevantTools = $derived(
    tools.filter((tool) => relevantToolNames.includes(tool.name))
  )

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '-'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function formatBytes(size = 0): string {
    if (!size) return '0 B'
    if (size < 1024) return `${size} B`
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
    return `${(size / (1024 * 1024)).toFixed(1)} MB`
  }

  async function load(targetScope?: SyspromptScope, targetPath?: string) {
    loading = true
    error = ''
    try {
      const [filesResp, toolsResp] = await Promise.all([
        listSyspromptFiles(),
        listChatTools(),
      ])
      files = filesResp.items ?? []
      tools = toolsResp.tools ?? []

      const nextScope = targetScope ?? selectedScope
      const nextPath = targetPath ?? selectedPath
      const fallback = files.find((item) => item.scope === nextScope && item.path === nextPath)
        ?? files.find((item) => item.path === 'USER.md')
        ?? files[0]

      if (fallback) {
        await selectFile(fallback.scope, fallback.path)
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load system prompt files'
    } finally {
      loading = false
    }
  }

  async function selectFile(scope: SyspromptScope, path: string) {
    error = ''
    success = ''
    try {
      const file = await getSyspromptFile(scope, path)
      selectedScope = file.scope
      selectedPath = file.path
      selectedFile = file
      editorContent = file.content?.length ? file.content : (file.starter_content || '')
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load file'
    }
  }

  async function save() {
    if (!selectedFile) return
    saving = true
    error = ''
    success = ''
    try {
      const saved = await saveSyspromptFile(selectedFile.scope, selectedFile.path, editorContent)
      selectedFile = saved
      editorContent = saved.content || editorContent
      await load(saved.scope, saved.path)
      success = `${saved.path} updated.`
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to save file'
    } finally {
      saving = false
    }
  }

  function roleCopy(path: string): string {
    switch (path) {
      case 'USER.md':
        return 'User identity and durable personal context.'
      case 'IDENTITY.md':
        return 'TARS persona, voice, and self-identity.'
      case 'AGENTS.md':
        return 'Agent operating rules and autonomy boundaries.'
      case 'TOOLS.md':
        return 'Tool environment guidance and usage expectations.'
      case 'PROJECT.md':
        return 'Workspace-level project policy.'
      case 'HEARTBEAT.md':
        return 'Heartbeat/background execution guidance.'
      case 'SOUL.md':
        return 'Legacy persona extension folded into identity.'
      default:
        return 'System prompt source file.'
    }
  }

  onMount(() => {
    void load()
  })
</script>

<div class="sysprompt-page">
  <header class="hero card">
    <div>
      <div class="eyebrow">System Prompt Control</div>
      <h1>Workspace Identity and Agent Rules</h1>
      <p>
        Manage the prompt-source files that define who the user is, who TARS is,
        and how agents should operate.
      </p>
    </div>
    <div class="hero-stats">
      <div class="stat">
        <span class="stat-label">Workspace Files</span>
        <strong>{workspaceFiles.length}</strong>
      </div>
      <div class="stat">
        <span class="stat-label">Agent Files</span>
        <strong>{agentFiles.length}</strong>
      </div>
      <div class="stat">
        <span class="stat-label">Built-in Tools</span>
        <strong>{relevantTools.length}</strong>
      </div>
    </div>
  </header>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}
  {#if success}
    <div class="success-banner">{success}</div>
  {/if}

  <div class="layout">
    <aside class="card file-panel">
      <div class="panel-header">
        <span class="card-title">Workspace Prompt</span>
      </div>
      {#if loading}
        <div class="empty-state">Loading system prompt files...</div>
      {:else}
        <div class="file-group">
          <div class="group-label">Workspace Identity</div>
          {#each workspaceFiles as file}
            <button class="file-row" class:active={selectedScope === file.scope && selectedPath === file.path} type="button" onclick={() => selectFile(file.scope, file.path)}>
              <div class="file-row-top">
                <strong>{file.path}</strong>
                <span class:badge-success={file.exists} class:badge-warning={!file.exists} class="badge">
                  {file.exists ? 'present' : 'missing'}
                </span>
              </div>
              <p>{roleCopy(file.path)}</p>
            </button>
          {/each}
        </div>

        <div class="file-group">
          <div class="group-label">Agent Rules</div>
          {#each agentFiles as file}
            <button class="file-row" class:active={selectedScope === file.scope && selectedPath === file.path} type="button" onclick={() => selectFile(file.scope, file.path)}>
              <div class="file-row-top">
                <strong>{file.path}</strong>
                <span class:badge-success={file.exists} class:badge-warning={!file.exists} class="badge">
                  {file.exists ? 'present' : 'missing'}
                </span>
              </div>
              <p>{roleCopy(file.path)}</p>
            </button>
          {/each}
        </div>
      {/if}
    </aside>

    <section class="card editor-panel">
      <div class="panel-header">
        <div>
          <span class="card-title">Editor</span>
          <div class="panel-subtitle">{selectedFile ? `${selectedFile.scope} · ${selectedFile.path}` : 'Select a file'}</div>
        </div>
        <div class="editor-actions">
          <button class="btn btn-ghost btn-sm" type="button" disabled={!selectedFile} onclick={() => selectedFile && selectFile(selectedFile.scope, selectedFile.path)}>Reload</button>
          <button class="btn btn-primary btn-sm" type="button" disabled={!selectedFile || saving} onclick={save}>
            {saving ? 'Saving...' : 'Save'}
          </button>
        </div>
      </div>

      {#if selectedFile}
        <div class="meta-row">
          <span class="badge badge-accent">{selectedFile.scope === 'workspace' ? 'Main Agent Prompt' : 'Sub-agent Prompt'}</span>
          <span>{selectedFile.exists ? 'Existing file' : 'Missing file, starter template loaded'}</span>
          <span>{formatBytes(selectedFile.size_bytes || 0)}</span>
          <span>{fmt(selectedFile.updated_at)}</span>
        </div>
        <div class="description-card">
          <strong>{selectedFile.title}</strong>
          <p>{selectedFile.description || roleCopy(selectedFile.path)}</p>
        </div>
        <textarea class="sysprompt-editor" bind:value={editorContent}></textarea>
      {:else}
        <div class="empty-state">Select a system prompt file to inspect or edit.</div>
      {/if}
    </section>

    <aside class="card diagnostics-panel">
      <div class="panel-header">
        <span class="card-title">Diagnostics</span>
      </div>

      <div class="diag-block">
        <div class="group-label">Role Semantics</div>
        <div class="diag-list">
          <div><strong>USER.md</strong><span>User identity and durable personal facts.</span></div>
          <div><strong>IDENTITY.md</strong><span>TARS persona and self-identity.</span></div>
          <div><strong>AGENTS.md</strong><span>Agent execution rules and autonomy boundaries.</span></div>
          <div><strong>TOOLS.md</strong><span>Tool environment guidance and usage policy.</span></div>
        </div>
      </div>

      <div class="diag-block">
        <div class="group-label">Relevant Built-in Tools</div>
        {#if relevantTools.length === 0}
          <div class="empty-state">No sysprompt tools detected.</div>
        {:else}
          <div class="tool-list">
            {#each relevantTools as tool}
              <div class="tool-row">
                <strong>{tool.name}</strong>
                <p>{tool.description}</p>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </aside>
  </div>
</div>

<style>
  .sysprompt-page {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .hero {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-6);
    background:
      radial-gradient(circle at top right, rgba(224, 145, 69, 0.12), transparent 34%),
      linear-gradient(135deg, rgba(255, 255, 255, 0.02), transparent),
      var(--bg-surface);
  }

  .eyebrow {
    margin-bottom: var(--space-2);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--accent-text);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  .hero p {
    margin-top: var(--space-3);
    max-width: 58ch;
    color: var(--text-secondary);
  }

  .hero-stats {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--space-3);
    width: min(360px, 100%);
  }

  .stat {
    padding: var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: rgba(255, 255, 255, 0.02);
  }

  .stat strong {
    display: block;
    margin-top: var(--space-2);
    font-size: var(--text-xl);
  }

  .stat-label, .group-label {
    font-family: var(--font-display);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }

  .layout {
    display: grid;
    grid-template-columns: 280px minmax(0, 1fr) 320px;
    gap: var(--space-4);
    min-height: 620px;
  }

  .file-panel, .editor-panel, .diagnostics-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .file-group {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .file-row {
    width: 100%;
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--bg-inset);
    text-align: left;
    cursor: pointer;
    transition: border-color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
  }

  .file-row:hover, .file-row.active {
    border-color: var(--accent);
    background: rgba(224, 145, 69, 0.08);
  }

  .file-row-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }

  .file-row p, .description-card p, .tool-row p, .diag-list span {
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .panel-subtitle {
    margin-top: var(--space-1);
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }

  .editor-actions {
    display: flex;
    gap: var(--space-2);
  }

  .meta-row {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }

  .description-card {
    padding: var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--bg-inset);
  }

  .description-card p {
    margin-top: var(--space-2);
  }

  .sysprompt-editor {
    min-height: 420px;
    flex: 1;
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    line-height: 1.6;
  }

  .diag-block {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .diag-list, .tool-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .diag-list div, .tool-row {
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--bg-inset);
  }

  .diag-list span {
    display: block;
    margin-top: var(--space-1);
  }

  .success-banner {
    padding: var(--space-3) var(--space-4);
    background: var(--success-muted);
    border: 1px solid rgba(74, 222, 128, 0.24);
    border-radius: var(--radius-md);
    color: var(--success);
    font-size: var(--text-sm);
  }

  @media (max-width: 1200px) {
    .layout {
      grid-template-columns: 240px minmax(0, 1fr);
    }

    .diagnostics-panel {
      grid-column: 1 / -1;
    }
  }

  @media (max-width: 900px) {
    .hero {
      flex-direction: column;
    }

    .hero-stats,
    .layout {
      grid-template-columns: 1fr;
    }
  }
</style>
