<script lang="ts">
  import { onMount } from 'svelte'
  import {
    getSyspromptPreview,
    getSyspromptFile,
    listChatTools,
    listSyspromptFiles,
    saveSyspromptFile,
    type ChatToolInfo,
  } from '../lib/api'
  import {
    getSyspromptTemplates,
    isSyspromptTemplateEligible,
  } from '../lib/syspromptTemplates'
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
  let selectedTemplateId = $state('')
  let previewOpen = $state(false)
  let previewLoading = $state(false)
  let previewError = $state('')
  let preview = $state<Awaited<ReturnType<typeof getSyspromptPreview>> | null>(null)

  const relevantToolNames = ['workspace']

  let workspaceFiles = $derived(files.filter((item) => item.scope === 'workspace'))
  let agentFiles = $derived(files.filter((item) => item.scope === 'agent'))
  let relevantTools = $derived(
    tools.filter((tool) => relevantToolNames.includes(tool.name))
  )
  let starterTemplates = $derived(getSyspromptTemplates(selectedPath))
  let selectedTemplate = $derived(starterTemplates.find((template) => template.id === selectedTemplateId))
  let canInsertTemplate = $derived.by(() => {
    const file = selectedFile
    return Boolean(
      file
      && starterTemplates.length > 0
      && isSyspromptTemplateEligible(editorContent, file.starter_content),
    )
  })

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

  function previewTargetFor(file: SyspromptFile | null): 'main_agent' | 'sub_agent' {
    return file?.scope === 'agent' ? 'sub_agent' : 'main_agent'
  }

  function promptImpactLine(file: SyspromptFile): string {
    const impact = file.prompt_impact
    if (!impact) return ''
    return `${formatBytes(file.size_bytes || 0)} · ~${impact.estimated_tokens} tokens · ${impact.chars} / ${impact.max_chars} chars · -> ## ${impact.section} (${impact.role})`
  }

  function promptImpactWarning(file: SyspromptFile): string {
    const impact = file.prompt_impact
    if (!impact?.will_truncate) return ''
    return `${impact.chars} / ${impact.max_chars} chars - will be truncated to ${impact.max_chars}`
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
      selectedTemplateId = ''
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

  async function loadPreview() {
    if (!selectedFile) return
    previewOpen = true
    previewLoading = true
    previewError = ''
    preview = null
    try {
      preview = await getSyspromptPreview(previewTargetFor(selectedFile))
    } catch (err) {
      previewError = err instanceof Error ? err.message : 'Failed to load system prompt preview'
    } finally {
      previewLoading = false
    }
  }

  function handleTemplateInsert() {
    const template = selectedTemplate
    if (!template) return
    editorContent = template.content
    selectedTemplateId = ''
    error = ''
    success = `${template.label} template inserted. Review and Save to apply.`
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
              {#if file.prompt_impact}
                <div class="file-impact-line">{promptImpactLine(file)}</div>
                {#if file.prompt_impact.will_truncate}
                  <div class="file-impact-warning">{promptImpactWarning(file)}</div>
                {/if}
              {/if}
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
              {#if file.prompt_impact}
                <div class="file-impact-line">{promptImpactLine(file)}</div>
                {#if file.prompt_impact.will_truncate}
                  <div class="file-impact-warning">{promptImpactWarning(file)}</div>
                {/if}
              {/if}
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
          <button class="btn btn-ghost btn-sm" type="button" disabled={!selectedFile || previewLoading} onclick={loadPreview}>
            {previewLoading ? 'Loading...' : 'Reload preview'}
          </button>
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
          {#if selectedFile.prompt_impact}
            <span>~{selectedFile.prompt_impact.estimated_tokens} tokens</span>
            <span>## {selectedFile.prompt_impact.section} ({selectedFile.prompt_impact.role})</span>
          {/if}
        </div>
        {#if selectedFile.prompt_impact?.will_truncate}
          <div class="impact-warning">{promptImpactWarning(selectedFile)}</div>
        {/if}
        <div class="description-card">
          <strong>{selectedFile.title}</strong>
          <p>{selectedFile.description || roleCopy(selectedFile.path)}</p>
        </div>
        {#if canInsertTemplate}
          <div class="starter-template-bar">
            <div>
              <strong>Insert template</strong>
              <p>{selectedTemplate?.description || 'Choose a starter template, then review it before saving.'}</p>
            </div>
            <div class="starter-template-actions">
              <select class="template-select" bind:value={selectedTemplateId} aria-label="Starter template">
                <option value="" disabled>Choose template...</option>
                {#each starterTemplates as template}
                  <option value={template.id}>{template.label}</option>
                {/each}
              </select>
              <button class="btn btn-ghost btn-sm" type="button" disabled={!selectedTemplate} onclick={handleTemplateInsert}>
                Insert
              </button>
            </div>
          </div>
        {/if}
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
          <div><strong>USER.md</strong><span>User identity — name, language, preferences.</span></div>
          <div><strong>IDENTITY.md</strong><span>TARS persona, voice, and behavioral style.</span></div>
          <div><strong>PROJECT.md</strong><span>Workspace-level project execution policy.</span></div>
          <div><strong>AGENTS.md</strong><span>Sub-agent execution rules and autonomy.</span></div>
          <div><strong>TOOLS.md</strong><span>Tool constraints and usage patterns.</span></div>
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

  {#if previewOpen}
    <div class="preview-backdrop" role="presentation">
      <div class="preview-modal" role="dialog" aria-modal="true" aria-label="System prompt preview" tabindex="0">
        <div class="preview-header">
          <div>
            <div class="group-label">System Prompt Preview</div>
            <h2>{preview?.target === 'sub_agent' ? 'Sub-agent prompt' : 'Main agent prompt'}</h2>
          </div>
          <div class="preview-actions">
            <button class="btn btn-ghost btn-sm" type="button" disabled={previewLoading} onclick={loadPreview}>
              {previewLoading ? 'Loading...' : 'Reload preview'}
            </button>
            <button class="btn btn-ghost btn-sm" type="button" onclick={() => { previewOpen = false }}>Close</button>
          </div>
        </div>
        {#if previewError}
          <div class="error-banner">{previewError}</div>
        {:else if previewLoading && !preview}
          <div class="empty-state">Loading system prompt preview...</div>
        {:else if preview}
          <div class="preview-meta">
            <span class="badge badge-accent">{preview.target}</span>
            <span>{preview.total_tokens} total tokens</span>
            <span>{preview.static_tokens} static</span>
            <span>{preview.relevant_tokens} memory</span>
          </div>
          <pre class="preview-body">{preview.prompt}</pre>
        {/if}
      </div>
    </div>
  {/if}
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
      var(--surface);
  }

  .eyebrow {
    margin-bottom: var(--space-2);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--primary-text);
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
    background: var(--surface-inset);
    text-align: left;
    cursor: pointer;
    transition: border-color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
  }

  .file-row:hover, .file-row.active {
    border-color: var(--primary);
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

  .file-impact-line {
    margin-top: var(--space-2);
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    line-height: 1.5;
  }

  .file-impact-warning,
  .impact-warning {
    color: var(--warning);
    font-size: var(--text-xs);
    line-height: 1.5;
  }

  .file-impact-warning {
    margin-top: var(--space-1);
  }

  .impact-warning {
    padding: var(--space-2) var(--space-3);
    border: 1px solid rgba(251, 191, 36, 0.24);
    border-radius: var(--radius-md);
    background: rgba(251, 191, 36, 0.08);
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
    background: var(--surface-inset);
  }

  .description-card p {
    margin-top: var(--space-2);
  }

  .starter-template-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-3) var(--space-4);
    border: 1px solid rgba(224, 145, 69, 0.24);
    border-radius: var(--radius-md);
    background: rgba(224, 145, 69, 0.08);
  }

  .starter-template-bar strong {
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 600;
  }

  .starter-template-bar p {
    margin-top: var(--space-1);
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .starter-template-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
  }

  .template-select {
    min-width: 190px;
    padding: 7px var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    color: var(--text-primary);
    font-size: var(--text-sm);
  }

  .template-select:focus {
    outline: none;
    border-color: var(--primary);
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
    background: var(--surface-inset);
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

  .preview-backdrop {
    position: fixed;
    inset: 0;
    z-index: 80;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-6);
    background: rgba(0, 0, 0, 0.58);
  }

  .preview-modal {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    width: min(980px, 100%);
    max-height: min(760px, calc(100vh - 48px));
    padding: var(--space-5);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--surface);
    box-shadow: var(--shadow-lg);
  }

  .preview-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .preview-header h2 {
    margin-top: var(--space-1);
    font-size: var(--text-xl);
  }

  .preview-actions,
  .preview-meta {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .preview-meta {
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .preview-body {
    flex: 1;
    min-height: 320px;
    margin: 0;
    padding: var(--space-4);
    overflow: auto;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    line-height: 1.6;
    white-space: pre-wrap;
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

    .starter-template-bar,
    .starter-template-actions,
    .preview-header,
    .preview-actions {
      align-items: stretch;
      flex-direction: column;
    }
  }
</style>
