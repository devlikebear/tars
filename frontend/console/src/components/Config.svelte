<script lang="ts">
  import { onMount } from 'svelte'
  import { getConfig, getConfigSchema, saveConfig, patchConfigValues, deleteAllProjects, resetWorkspace, restartServer } from '../lib/api'
  import type { ConfigFieldMeta, ConfigSchema } from '../lib/types'

  type ViewMode = 'form' | 'yaml'

  let configPath = $state('')
  let schema: ConfigFieldMeta[] = $state([])
  let values: Record<string, unknown> = $state({})
  let yamlContent = $state('')
  let originalYaml = $state('')
  let loading = $state(true)
  let saving = $state(false)
  let error = $state('')
  let success = $state('')
  let viewMode: ViewMode = $state('form')
  let expandedSections: Record<string, boolean> = $state({})

  // -- Field editing --
  let editingKey: string | null = $state(null)
  let editValue: string = $state('')
  let editBool: boolean = $state(false)
  let fieldSaving = $state(false)
  let dirtyFields: Record<string, unknown> = $state({})

  let hasDirtyFields = $derived(Object.keys(dirtyFields).length > 0)

  // -- Restart --
  let restartBusy = $state(false)
  let restartConfirm = $state(false)

  async function handleRestart() {
    if (!restartConfirm) { restartConfirm = true; return }
    restartBusy = true
    error = ''
    success = ''
    try {
      const result = await restartServer()
      success = `Restart initiated (${result.mode}). ${result.info}. Page will reconnect shortly.`
      restartConfirm = false
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to restart server'
    } finally {
      restartBusy = false
    }
  }

  // -- Reset / Danger zone --
  let deleteAllConfirm = $state(false)
  let deleteAllBusy = $state(false)
  let resetWsBusy = $state(false)
  let resetWsConfirm = $state(false)

  async function handleDeleteAllProjects() {
    if (!deleteAllConfirm) { deleteAllConfirm = true; return }
    deleteAllBusy = true
    error = ''
    success = ''
    try {
      const result = await deleteAllProjects()
      success = `Deleted ${result.deleted} projects.`
      deleteAllConfirm = false
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete projects'
    } finally {
      deleteAllBusy = false
    }
  }

  async function handleResetWorkspace() {
    if (!resetWsConfirm) { resetWsConfirm = true; return }
    resetWsBusy = true
    error = ''
    success = ''
    try {
      const result = await resetWorkspace()
      success = `Workspace reset: ${result.removed_dirs} directories removed. Restart TARS to reinitialize.`
      resetWsConfirm = false
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to reset workspace'
    } finally {
      resetWsBusy = false
    }
  }

  let sections = $derived.by(() => {
    const order: string[] = []
    const groups: Record<string, ConfigFieldMeta[]> = {}
    for (const f of schema) {
      if (!groups[f.section]) {
        order.push(f.section)
        groups[f.section] = []
      }
      groups[f.section].push(f)
    }
    return order.map((name) => ({ name, fields: groups[name] }))
  })

  let isYamlDirty = $derived(yamlContent !== originalYaml)

  async function load() {
    loading = true
    error = ''
    try {
      const [schemaResp, rawResp] = await Promise.all([getConfigSchema(), getConfig()])
      configPath = schemaResp.path
      schema = schemaResp.fields
      values = schemaResp.values
      yamlContent = rawResp.content
      originalYaml = rawResp.content
      dirtyFields = {}
      const sectionNames = [...new Set(schemaResp.fields.map((f) => f.section))]
      for (let i = 0; i < Math.min(3, sectionNames.length); i++) {
        expandedSections[sectionNames[i]] = true
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load config'
    } finally {
      loading = false
    }
  }

  function startEdit(field: ConfigFieldMeta) {
    if (field.sensitive) return // Don't edit masked fields in form view
    editingKey = field.key
    const current = dirtyFields[field.key] !== undefined ? dirtyFields[field.key] : values[field.key]
    if (field.type === 'bool') {
      editBool = !!current
    } else {
      editValue = current !== undefined && current !== null ? String(current) : ''
    }
  }

  function cancelEdit() {
    editingKey = null
    editValue = ''
  }

  function commitEdit(field: ConfigFieldMeta) {
    if (editingKey === null) return // already committed (e.g. Enter then blur)
    let parsed: unknown
    if (field.type === 'bool') {
      parsed = editBool
    } else if (field.type === 'int') {
      parsed = editValue.trim() === '' ? 0 : parseInt(editValue, 10)
      if (isNaN(parsed as number)) { cancelEdit(); return }
    } else if (field.type === 'float') {
      parsed = editValue.trim() === '' ? 0 : parseFloat(editValue)
      if (isNaN(parsed as number)) { cancelEdit(); return }
    } else {
      parsed = editValue
    }

    // Check if changed from original
    const original = values[field.key]
    if (parsed === original || (String(parsed) === String(original))) {
      delete dirtyFields[field.key]
    } else {
      dirtyFields[field.key] = parsed
    }
    dirtyFields = { ...dirtyFields }
    editingKey = null
    editValue = ''
  }

  function selectField(field: ConfigFieldMeta, value: string) {
    const original = values[field.key]
    if (value === String(original ?? '')) {
      delete dirtyFields[field.key]
    } else {
      dirtyFields[field.key] = value
    }
    dirtyFields = { ...dirtyFields }
  }

  function toggleBool(field: ConfigFieldMeta) {
    const current = dirtyFields[field.key] !== undefined ? dirtyFields[field.key] : values[field.key]
    const newVal = !current
    if (newVal === values[field.key]) {
      delete dirtyFields[field.key]
    } else {
      dirtyFields[field.key] = newVal
    }
    dirtyFields = { ...dirtyFields }
  }

  function getDisplayValue(field: ConfigFieldMeta): unknown {
    return dirtyFields[field.key] !== undefined ? dirtyFields[field.key] : values[field.key]
  }

  function isDirty(key: string): boolean {
    return dirtyFields[key] !== undefined
  }

  async function handleSaveFields() {
    if (!hasDirtyFields) return
    fieldSaving = true
    error = ''
    success = ''
    try {
      await patchConfigValues(dirtyFields)
      success = 'Config saved. Restart TARS to apply changes.'
      dirtyFields = {}
      // Reload to get fresh values and YAML
      const [schemaResp, rawResp] = await Promise.all([getConfigSchema(), getConfig()])
      values = schemaResp.values
      yamlContent = rawResp.content
      originalYaml = rawResp.content
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to save config'
    } finally {
      fieldSaving = false
    }
  }

  function handleDiscardFields() {
    dirtyFields = {}
    success = ''
    error = ''
  }

  async function handleSaveYaml() {
    saving = true
    error = ''
    success = ''
    try {
      await saveConfig(yamlContent)
      originalYaml = yamlContent
      success = 'Config saved. Restart TARS to apply changes.'
      const schemaResp = await getConfigSchema()
      values = schemaResp.values
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to save config'
    } finally {
      saving = false
    }
  }

  function handleResetYaml() {
    yamlContent = originalYaml
    error = ''
    success = ''
  }

  function handleKeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === 's') {
      e.preventDefault()
      if (viewMode === 'yaml' && isYamlDirty && !saving) handleSaveYaml()
      if (viewMode === 'form' && hasDirtyFields && !fieldSaving) handleSaveFields()
    }
  }

  function handleFieldKeydown(e: KeyboardEvent, field: ConfigFieldMeta) {
    if (e.key === 'Enter') {
      e.preventDefault()
      commitEdit(field)
    } else if (e.key === 'Escape') {
      cancelEdit()
    }
  }

  function toggleSection(name: string) {
    expandedSections[name] = !expandedSections[name]
    expandedSections = { ...expandedSections }
  }

  function formatValue(field: ConfigFieldMeta): string {
    const v = getDisplayValue(field)
    if (v === undefined || v === null || v === '') return '\u2014'
    if (field.sensitive && typeof v === 'string' && v.includes('*')) return v
    return String(v)
  }

  const sectionIcons: Record<string, string> = {
    Runtime: '\u2699', API: '\u26bf', LLM: '\u2726', Memory: '\u29bf',
    Usage: '\u2261', Automation: '\u21bb', Assistant: '\u2318', Tools: '\u2692',
    Vault: '\u26bf', Browser: '\u2317', Gateway: '\u29bf', Channels: '\u2709',
    Extensions: '\u2756',
  }

  onMount(() => { load() })
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="config-page" onkeydown={handleKeydown}>
  <div class="page-header">
    <div class="page-header-left">
      <h2 class="page-title">Settings</h2>
      {#if configPath}
        <span class="config-path">{configPath}</span>
      {/if}
    </div>
    <div class="page-header-right">
      {#if viewMode === 'form' && hasDirtyFields}
        <span class="badge badge-warning">{Object.keys(dirtyFields).length} changed</span>
        <button class="btn btn-ghost btn-sm" onclick={handleDiscardFields}>Discard</button>
        <button class="btn btn-primary btn-sm" disabled={fieldSaving} onclick={handleSaveFields}>
          {fieldSaving ? 'Saving...' : 'Save'}
        </button>
      {/if}
      <div class="view-toggle">
        <button class="toggle-btn" class:active={viewMode === 'form'} onclick={() => { viewMode = 'form' }}>Fields</button>
        <button class="toggle-btn" class:active={viewMode === 'yaml'} onclick={() => { viewMode = 'yaml' }}>YAML</button>
      </div>
    </div>
  </div>

  {#if loading}
    <div class="loading">Loading configuration...</div>
  {:else if !configPath}
    <div class="card empty-state">
      <p>No config file configured. Start TARS with <code>--config &lt;path&gt;</code> to manage settings.</p>
    </div>
  {:else}
    {#if error}
      <div class="message message-error">{error}</div>
    {/if}
    {#if success}
      <div class="message message-success">{success}</div>
    {/if}

    {#if viewMode === 'form'}
      <div class="sections">
        {#each sections as section}
          <div class="section-card card">
            <button class="section-header" onclick={() => toggleSection(section.name)}>
              <div class="section-header-left">
                <span class="section-icon">{sectionIcons[section.name] || '\u2699'}</span>
                <span class="section-title">{section.name}</span>
                <span class="section-count">{section.fields.length}</span>
              </div>
              <span class="section-chevron" class:open={expandedSections[section.name]}>{'\u25b8'}</span>
            </button>

            {#if expandedSections[section.name]}
              <div class="section-body">
                {#each section.fields as field}
                  <div class="field-row" class:field-dirty={isDirty(field.key)}>
                    <div class="field-info">
                      <span class="field-label">{field.label}</span>
                      <span class="field-desc">{field.description}</span>
                      <span class="field-key">{field.key}</span>
                    </div>
                    <div class="field-value">
                      {#if field.type === 'bool'}
                        <!-- Bool: clickable toggle -->
                        <button
                          class="bool-toggle"
                          class:bool-on={!!getDisplayValue(field)}
                          class:dirty={isDirty(field.key)}
                          onclick={() => toggleBool(field)}
                          title="Click to toggle"
                        >
                          {getDisplayValue(field) ? 'ON' : 'OFF'}
                        </button>
                      {:else if field.type === 'select' && field.options}
                        <!-- Select dropdown -->
                        <select
                          class="field-select"
                          class:dirty={isDirty(field.key)}
                          value={String(getDisplayValue(field) ?? '')}
                          onchange={(e) => selectField(field, (e.target as HTMLSelectElement).value)}
                        >
                          {#each field.options as opt}
                            <option value={opt}>{opt || '(none)'}</option>
                          {/each}
                        </select>
                      {:else if editingKey === field.key}
                        <!-- Editing mode -->
                        <div class="field-edit">
                          <input
                            type={field.type === 'int' || field.type === 'float' ? 'number' : 'text'}
                            step={field.type === 'float' ? '0.01' : undefined}
                            class="field-input"
                            bind:value={editValue}
                            onkeydown={(e) => handleFieldKeydown(e, field)}
                            onblur={() => commitEdit(field)}
                          />
                        </div>
                      {:else if field.sensitive}
                        <!-- Sensitive: show masked, not editable inline -->
                        <span class="value-text sensitive" title="Edit in YAML tab">{formatValue(field)}</span>
                      {:else}
                        <!-- Clickable value -->
                        <button class="value-btn" class:dirty={isDirty(field.key)} onclick={() => startEdit(field)} title="Click to edit">
                          <span class="value-text">{formatValue(field)}</span>
                        </button>
                      {/if}
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {:else}
      <div class="editor-card card">
        <div class="card-header">
          <span class="card-title">Configuration (YAML)</span>
          <div class="card-actions">
            <button class="btn btn-ghost btn-sm" disabled={!isYamlDirty} onclick={handleResetYaml}>Reset</button>
            <button class="btn btn-primary btn-sm" disabled={!isYamlDirty || saving} onclick={handleSaveYaml}>
              {saving ? 'Saving...' : 'Save'}
            </button>
          </div>
        </div>
        <textarea
          class="config-editor"
          bind:value={yamlContent}
          onkeydown={handleKeydown}
          spellcheck={false}
        ></textarea>
        <div class="editor-footer">
          {#if isYamlDirty}
            <span class="badge badge-warning">Unsaved changes</span>
          {:else}
            <span class="badge badge-default">No changes</span>
          {/if}
          <span class="hint">Ctrl+S / Cmd+S to save</span>
        </div>
      </div>
    {/if}

    <!-- Server Restart -->
    <div class="restart-section card">
      <div class="card-header">
        <span class="card-title">Server</span>
      </div>
      <div class="danger-body">
        <div class="danger-row">
          <div class="danger-info">
            <strong>Restart server</strong>
            <span>Apply config changes by restarting the TARS process. Service mode uses launchctl, dev mode re-execs the process.</span>
          </div>
          <button
            class="btn btn-primary btn-sm"
            disabled={restartBusy}
            onclick={handleRestart}
          >{restartConfirm ? 'Click again to confirm' : restartBusy ? 'Restarting...' : 'Restart Server'}</button>
        </div>
      </div>
    </div>

    <!-- Danger Zone -->
    <div class="danger-zone card">
      <div class="card-header">
        <span class="card-title danger-title">Danger Zone</span>
      </div>
      <div class="danger-body">
        <div class="danger-row">
          <div class="danger-info">
            <strong>Delete all projects</strong>
            <span>Permanently remove all project directories and their data.</span>
          </div>
          <button
            class="btn btn-danger btn-sm"
            disabled={deleteAllBusy}
            onclick={handleDeleteAllProjects}
          >{deleteAllConfirm ? 'Click again to confirm' : deleteAllBusy ? 'Deleting...' : 'Delete All Projects'}</button>
        </div>
        <div class="danger-row">
          <div class="danger-info">
            <strong>Reset workspace</strong>
            <span>Remove sessions, cron state, gateway data, logs, and memory. Config and projects are preserved.</span>
          </div>
          <button
            class="btn btn-danger btn-sm"
            disabled={resetWsBusy}
            onclick={handleResetWorkspace}
          >{resetWsConfirm ? 'Click again to confirm' : resetWsBusy ? 'Resetting...' : 'Reset Workspace'}</button>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .config-page {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    padding: var(--space-6);
    max-width: 960px;
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .page-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    flex-wrap: wrap;
  }

  .page-header-left {
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
    flex-wrap: wrap;
  }

  .page-header-right {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .page-title {
    font-family: var(--font-display);
    font-size: var(--text-xl);
    font-weight: 600;
    color: var(--text-primary);
    margin: 0;
  }

  .config-path {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-ghost);
    background: var(--bg-elevated);
    padding: 2px var(--space-2);
    border-radius: var(--radius-sm);
  }

  .view-toggle {
    display: flex;
    background: var(--bg-elevated);
    border-radius: var(--radius-md);
    padding: 2px;
    gap: 2px;
  }

  .toggle-btn {
    padding: var(--space-1) var(--space-3);
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
    cursor: pointer;
    transition: all var(--duration-fast) var(--ease-out);
  }
  .toggle-btn:hover { color: var(--text-primary); }
  .toggle-btn.active { background: var(--accent); color: #fff; }

  .loading { color: var(--text-secondary); font-size: var(--text-sm); padding: var(--space-6); }

  .empty-state { padding: var(--space-6); text-align: center; color: var(--text-secondary); font-size: var(--text-sm); }
  .empty-state code { font-family: var(--font-mono); background: var(--bg-elevated); padding: 2px var(--space-1); border-radius: var(--radius-sm); font-size: var(--text-xs); }

  .message { font-size: var(--text-sm); padding: var(--space-2) var(--space-3); border-radius: var(--radius-md); }
  .message-error { background: rgba(220, 60, 60, 0.15); color: var(--red); border: 1px solid rgba(220, 60, 60, 0.3); }
  .message-success { background: rgba(60, 180, 100, 0.15); color: var(--green); border: 1px solid rgba(60, 180, 100, 0.3); }

  /* ── Sections ────────────────────────────── */
  .sections { display: flex; flex-direction: column; gap: var(--space-3); }
  .section-card { overflow: hidden; }

  .section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    padding: var(--space-3) var(--space-4);
    background: transparent;
    border: none;
    cursor: pointer;
    transition: background var(--duration-fast) var(--ease-out);
  }
  .section-header:hover { background: var(--bg-elevated); }

  .section-header-left { display: flex; align-items: center; gap: var(--space-3); }
  .section-icon { font-size: var(--text-md); color: var(--accent); width: 20px; text-align: center; }
  .section-title { font-family: var(--font-display); font-size: var(--text-sm); font-weight: 600; color: var(--text-primary); }
  .section-count { font-size: var(--text-xs); color: var(--text-ghost); }

  .section-chevron {
    color: var(--text-ghost);
    font-size: var(--text-sm);
    transition: transform var(--duration-fast) var(--ease-out);
    display: inline-block;
  }
  .section-chevron.open { transform: rotate(90deg); }
  .section-body { border-top: 1px solid var(--border-subtle); }

  /* ── Field rows ──────────────────────────── */
  .field-row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
    transition: background var(--duration-fast) var(--ease-out);
  }
  .field-row:last-child { border-bottom: none; }
  .field-row:hover { background: rgba(255, 255, 255, 0.015); }
  .field-row.field-dirty { background: rgba(224, 145, 69, 0.06); border-left: 2px solid var(--accent); }

  .field-info { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
  .field-label { font-family: var(--font-display); font-size: var(--text-sm); font-weight: 500; color: var(--text-primary); }
  .field-desc { font-size: var(--text-xs); color: var(--text-tertiary); line-height: 1.4; }
  .field-key { font-family: var(--font-mono); font-size: 10px; color: var(--text-ghost); }

  .field-value { flex-shrink: 0; max-width: 300px; text-align: right; display: flex; align-items: center; }

  .value-text {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-secondary);
    word-break: break-all;
  }
  .value-text.sensitive { color: var(--text-ghost); letter-spacing: 0.5px; }

  /* ── Value button (clickable to edit) ──── */
  .value-btn {
    background: none;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    padding: var(--space-1) var(--space-2);
    cursor: pointer;
    transition: all var(--duration-fast) var(--ease-out);
    text-align: right;
    max-width: 300px;
  }
  .value-btn:hover {
    border-color: var(--border-default);
    background: var(--bg-elevated);
  }
  .value-btn.dirty .value-text {
    color: var(--accent);
    font-weight: 500;
  }

  /* ── Bool toggle ─────────────────────────── */
  .bool-toggle {
    display: inline-block;
    padding: 3px var(--space-2);
    border-radius: var(--radius-sm);
    border: 1px solid transparent;
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: 0.04em;
    cursor: pointer;
    transition: all var(--duration-fast) var(--ease-out);
    background: rgba(255, 255, 255, 0.04);
    color: var(--text-ghost);
  }
  .bool-toggle.bool-on {
    background: rgba(60, 180, 100, 0.15);
    color: var(--green);
  }
  .bool-toggle:hover {
    border-color: var(--border-default);
    transform: scale(1.05);
  }
  .bool-toggle.dirty {
    box-shadow: 0 0 0 1px var(--accent);
  }

  /* ── Field select ─────────────────────────── */
  .field-select {
    padding: 3px var(--space-2);
    background: var(--bg-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    cursor: pointer;
    min-width: 120px;
    text-align: right;
    appearance: none;
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='8' height='5' viewBox='0 0 8 5'%3E%3Cpath fill='%23888' d='M0 0l4 5 4-5z'/%3E%3C/svg%3E");
    background-repeat: no-repeat;
    background-position: right 8px center;
    padding-right: var(--space-5);
  }
  .field-select:hover { border-color: var(--border-default); }
  .field-select:focus { outline: none; border-color: var(--accent); box-shadow: 0 0 0 2px rgba(224, 145, 69, 0.3); }
  .field-select.dirty { border-color: var(--accent); color: var(--accent); font-weight: 500; }

  /* ── Field input ─────────────────────────── */
  .field-edit { display: flex; }
  .field-input {
    width: 200px;
    padding: var(--space-1) var(--space-2);
    background: var(--bg-base);
    border: 1px solid var(--accent);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    text-align: right;
    outline: none;
  }
  .field-input:focus {
    box-shadow: 0 0 0 2px rgba(224, 145, 69, 0.3);
  }

  /* ── YAML Editor ─────────────────────────── */
  .editor-card { display: flex; flex-direction: column; }
  .card-actions { display: flex; gap: var(--space-2); }

  .config-editor {
    width: 100%;
    min-height: 500px;
    padding: var(--space-3);
    background: var(--bg-base);
    color: var(--text-primary);
    border: none;
    border-top: 1px solid var(--border-subtle);
    border-bottom: 1px solid var(--border-subtle);
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    line-height: 1.6;
    resize: vertical;
    tab-size: 2;
    white-space: pre;
    overflow-x: auto;
  }
  .config-editor:focus { outline: none; box-shadow: inset 0 0 0 1px var(--accent); }

  .editor-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-2) var(--space-3);
  }
  .hint { font-family: var(--font-mono); font-size: var(--text-xs); color: var(--text-ghost); }

  /* ── Restart section ──────────────────────── */
  .restart-section { margin-top: var(--space-4); }

  /* ── Danger Zone ─────────────────────────── */
  .danger-zone {
    border-color: rgba(220, 60, 60, 0.3);
    margin-top: var(--space-4);
  }
  .danger-title { color: var(--red); }
  .danger-body { display: flex; flex-direction: column; }
  .danger-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
  }
  .danger-row:last-child { border-bottom: none; }
  .danger-info { display: flex; flex-direction: column; gap: 2px; }
  .danger-info strong { font-family: var(--font-display); font-size: var(--text-sm); font-weight: 500; color: var(--text-primary); }
  .danger-info span { font-size: var(--text-xs); color: var(--text-tertiary); }
</style>
