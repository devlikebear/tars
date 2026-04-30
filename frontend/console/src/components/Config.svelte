<script lang="ts">
  import { onMount } from 'svelte'
  import { getConfig, getConfigSchema, getProviderModels, saveConfig, patchConfigValues, resetWorkspace, restartServer } from '../lib/api'
  import { buildConfigImpactPreview } from '../lib/configImpact'
  import { buildConfigMetaBadges } from '../lib/configMetaBadges'
  import { buildQuickStartItems, quickStartProgress } from '../lib/quickStartFields'
  import {
    buildLLMTiersFromDrafts,
    configValuesEqual,
    extractLLMProviderAliases,
    formatConfigDisplayValue,
    makeLLMTierDrafts,
    parseStructuredJSONEdit,
    prettyConfigJSON,
    stringifyConfigValue,
    type ConfigDisplaySummary,
    type LLMTierDraft,
    type LLMTierDraftErrors,
    type LLMTierDraftField,
  } from '../lib/configStructured'
  import type { ConfigFieldMeta, ConfigSchema } from '../lib/types'

  type ViewMode = 'quick' | 'form' | 'yaml'

  let configPath = $state('')
  let schemaUpdatedAt = $state('')
  let schema: ConfigFieldMeta[] = $state([])
  let values: Record<string, unknown> = $state({})
  let yamlContent = $state('')
  let originalYaml = $state('')
  let loading = $state(true)
  let saving = $state(false)
  let error = $state('')
  let success = $state('')
  let viewMode: ViewMode = $state('quick')
  let expandedSections: Record<string, boolean> = $state({})

  // -- Field editing --
  let editingKey: string | null = $state(null)
  let editValue: string = $state('')
  let editBool: boolean = $state(false)
  let fieldSaving = $state(false)
  let dirtyFields: Record<string, unknown> = $state({})
  let jsonEditorField: ConfigFieldMeta | null = $state(null)
  let jsonEditorText = $state('')
  let jsonEditorError = $state('')
  let tierEditorField: ConfigFieldMeta | null = $state(null)
  let tierDrafts: LLMTierDraft[] = $state([])
  let tierEditorErrors: LLMTierDraftErrors = $state({})
  let tierProviderOptions: string[] = $state([])
  let tierDraftSeq = 0
  let llmTestBusy = $state(false)
  let llmTestResult = $state('')
  let llmTestKind: 'success' | 'error' | '' = $state('')

  let hasDirtyFields = $derived(Object.keys(dirtyFields).length > 0)
  let quickStartItems = $derived(buildQuickStartItems(schema, values, dirtyFields))
  let quickStartStats = $derived(quickStartProgress(quickStartItems))

  // -- Diff popup --
  let showDiff = $state(false)

  let diffEntries = $derived.by(() => {
    return Object.entries(dirtyFields).map(([key, newVal]) => {
      const field = schema.find((f) => f.key === key)
      return {
        key,
        label: field?.label || key,
        path: field?.path || key,
        oldVal: stringifyConfigValue(values[key]),
        newVal: stringifyConfigValue(newVal),
        impact: buildConfigImpactPreview(field, values[key], newVal).items,
      }
    })
  })

  // -- Search/filter --
  let searchQuery = $state('')

  let filteredSections = $derived.by(() => {
    const q = searchQuery.trim().toLowerCase()
    if (!q) return sections
    return sections
      .map((section) => ({
        name: section.name,
        fields: section.fields.filter(
          (f) =>
            f.label.toLowerCase().includes(q) ||
            f.key.toLowerCase().includes(q) ||
            f.path.toLowerCase().includes(q) ||
            f.description.toLowerCase().includes(q) ||
            f.section.toLowerCase().includes(q)
        ),
      }))
      .filter((s) => s.fields.length > 0)
  })

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
  let resetWsBusy = $state(false)
  let resetWsConfirm = $state(false)

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
      schemaUpdatedAt = schemaResp.updated_at || ''
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
    } else if (field.type === 'string_list') {
      if (Array.isArray(current)) {
        editValue = current.map((item) => String(item)).join('\n')
      } else {
        editValue = current !== undefined && current !== null ? String(current) : ''
      }
    } else if (field.type === 'json') {
      openStructuredEditor(field)
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
    } else if (field.type === 'string_list') {
      parsed = editValue
        .split(/\r?\n|,/)
        .map((item) => item.trim())
        .filter(Boolean)
    } else if (field.type === 'json') {
      const result = parseStructuredJSONEdit(editValue)
      if (!result.ok) {
        error = result.error
        return
      }
      parsed = result.value
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
    if (configValuesEqual(parsed, original)) {
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
    if (configValuesEqual(value, original)) {
      delete dirtyFields[field.key]
    } else {
      dirtyFields[field.key] = value
    }
    dirtyFields = { ...dirtyFields }
  }

  function toggleBool(field: ConfigFieldMeta) {
    const current = dirtyFields[field.key] !== undefined ? dirtyFields[field.key] : values[field.key]
    const newVal = !current
    if (configValuesEqual(newVal, values[field.key])) {
      delete dirtyFields[field.key]
    } else {
      dirtyFields[field.key] = newVal
    }
    dirtyFields = { ...dirtyFields }
  }

  function handleSelectChange(field: ConfigFieldMeta, event: Event) {
    const target = event.currentTarget
    if (!(target instanceof HTMLSelectElement)) return
    selectField(field, target.value)
  }

  function getDisplayValue(field: ConfigFieldMeta): unknown {
    return dirtyFields[field.key] !== undefined ? dirtyFields[field.key] : values[field.key]
  }

  function structuredSummary(field: ConfigFieldMeta): ConfigDisplaySummary {
    return formatConfigDisplayValue(getDisplayValue(field))
  }

  function isDirty(key: string): boolean {
    return dirtyFields[key] !== undefined
  }

  function defaultValueSummary(field: ConfigFieldMeta): string {
    if (!Object.prototype.hasOwnProperty.call(field, 'default_value')) return ''
    return stringifyConfigValue(field.default_value)
  }

  async function testLLMConnection() {
    llmTestBusy = true
    llmTestResult = ''
    llmTestKind = ''
    try {
      const result = await getProviderModels()
      const count = Array.isArray(result.models) ? result.models.length : 0
      const provider = result.provider || 'provider'
      llmTestResult = count > 0 ? `${provider}: ${count} models available` : `${provider}: connection returned no model list`
      llmTestKind = result.warning ? 'error' : 'success'
    } catch (e) {
      llmTestResult = e instanceof Error ? e.message : 'Connection test failed'
      llmTestKind = 'error'
    } finally {
      llmTestBusy = false
    }
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
      schemaUpdatedAt = schemaResp.updated_at || ''
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
    closeJSONEditor()
    closeTierEditor()
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
      schemaUpdatedAt = schemaResp.updated_at || ''
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
    const multiline = field.type === 'json' || field.type === 'string_list'
    if (e.key === 'Enter' && (!multiline || e.metaKey || e.ctrlKey)) {
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
    if (field.sensitive && typeof v === 'string' && v.includes('*')) return v
    const summary = formatConfigDisplayValue(v)
    if (field.type === 'json' || summary.kind === 'array' || summary.kind === 'object') return summary.text
    return summary.raw
  }

  function fieldPath(field: ConfigFieldMeta): string {
    return field.path || field.key
  }

  function openJSONEditor(field: ConfigFieldMeta) {
    if (field.sensitive) return
    editingKey = null
    editValue = ''
    closeTierEditor()
    jsonEditorField = field
    jsonEditorText = prettyConfigJSON(getDisplayValue(field))
    jsonEditorError = ''
  }

  function closeJSONEditor() {
    jsonEditorField = null
    jsonEditorText = ''
    jsonEditorError = ''
  }

  function resetJSONEditor() {
    if (!jsonEditorField) return
    jsonEditorText = prettyConfigJSON(values[jsonEditorField.key])
    jsonEditorError = ''
  }

  function applyJSONEditor() {
    if (!jsonEditorField) return
    const result = parseStructuredJSONEdit(jsonEditorText)
    if (!result.ok) {
      jsonEditorError = result.error
      return
    }
    const original = values[jsonEditorField.key]
    if (configValuesEqual(result.value, original)) {
      delete dirtyFields[jsonEditorField.key]
    } else {
      dirtyFields[jsonEditorField.key] = result.value
    }
    dirtyFields = { ...dirtyFields }
    closeJSONEditor()
  }

  function openStructuredEditor(field: ConfigFieldMeta) {
    if (field.key === 'llm_tiers') {
      openTierEditor(field)
      return
    }
    openJSONEditor(field)
  }

  function openTierEditor(field: ConfigFieldMeta) {
    if (field.sensitive) return
    editingKey = null
    editValue = ''
    closeJSONEditor()
    tierProviderOptions = extractLLMProviderAliases(getValueByKey('llm_providers'))
    const drafts = makeLLMTierDrafts(getDisplayValue(field))
    tierDrafts = drafts.length > 0 ? drafts : [createTierDraft('standard')]
    tierEditorField = field
    tierEditorErrors = {}
  }

  function closeTierEditor() {
    tierEditorField = null
    tierDrafts = []
    tierEditorErrors = {}
    tierProviderOptions = []
  }

  function resetTierEditor() {
    if (!tierEditorField) return
    const drafts = makeLLMTierDrafts(values[tierEditorField.key])
    tierDrafts = drafts.length > 0 ? drafts : [createTierDraft('standard')]
    tierEditorErrors = {}
  }

  function addTierDraft() {
    tierDrafts = [...tierDrafts, createTierDraft(nextTierName())]
  }

  function removeTierDraft(id: string) {
    if (tierDrafts.length <= 1) return
    tierDrafts = tierDrafts.filter((draft) => draft.id !== id)
    const { [id]: _removed, ...remaining } = tierEditorErrors
    tierEditorErrors = remaining
  }

  function updateTierDraft(id: string, field: LLMTierDraftField, value: string) {
    tierDrafts = tierDrafts.map((draft) => draft.id === id ? { ...draft, [field]: value } : draft)
    const rowErrors = tierEditorErrors[id]
    if (!rowErrors?.[field]) return
    const nextRowErrors = { ...rowErrors }
    delete nextRowErrors[field]
    const nextErrors = { ...tierEditorErrors }
    if (Object.keys(nextRowErrors).length === 0) {
      delete nextErrors[id]
    } else {
      nextErrors[id] = nextRowErrors
    }
    tierEditorErrors = nextErrors
  }

  function applyTierEditor() {
    if (!tierEditorField) return
    const result = buildLLMTiersFromDrafts(tierDrafts, tierProviderOptions)
    if (!result.ok) {
      tierEditorErrors = result.errors
      return
    }
    const original = values[tierEditorField.key]
    if (configValuesEqual(result.value, original)) {
      delete dirtyFields[tierEditorField.key]
    } else {
      dirtyFields[tierEditorField.key] = result.value
    }
    dirtyFields = { ...dirtyFields }
    closeTierEditor()
  }

  function createTierDraft(name: string): LLMTierDraft {
    tierDraftSeq += 1
    return {
      id: `new-tier-${tierDraftSeq}`,
      originalName: '',
      name,
      provider: tierProviderOptions[0] || '',
      model: '',
      reasoning_effort: '',
      thinking_budget: '',
      service_tier: '',
    }
  }

  function nextTierName(): string {
    const names = new Set(tierDrafts.map((draft) => draft.name.trim()).filter(Boolean))
    let idx = tierDrafts.length + 1
    let candidate = `tier${idx}`
    while (names.has(candidate)) {
      idx += 1
      candidate = `tier${idx}`
    }
    return candidate
  }

  function getValueByKey(key: string): unknown {
    return dirtyFields[key] !== undefined ? dirtyFields[key] : values[key]
  }

  function inputValue(event: Event): string {
    const target = event.currentTarget
    if (target instanceof HTMLInputElement || target instanceof HTMLSelectElement) return target.value
    return ''
  }

  function tierError(id: string, field: LLMTierDraftField): string {
    return tierEditorErrors[id]?.[field] || ''
  }

  function tierProviderChoices(current: string): string[] {
    const choices = new Set(tierProviderOptions)
    const value = current.trim()
    if (value) choices.add(value)
    return [...choices].sort()
  }

  function tierReasoningChoices(current: string): string[] {
    const defaults = ['', 'minimal', 'low', 'medium', 'high']
    const value = current.trim()
    if (value && !defaults.includes(value)) return [...defaults, value]
    return defaults
  }

  const sectionIcons: Record<string, string> = {
    Runtime: '\u2699', API: '\u26bf', LLM: '\u2726', Memory: '\u29bf',
    Usage: '\u2261', Automation: '\u21bb', Assistant: '\u2318', Tools: '\u2692',
    Vault: '\u26bf', Browser: '\u2317', 'Agent Runtime': '\u29bf', Channels: '\u2709',
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
        <button class="badge badge-warning diff-badge" onclick={() => { showDiff = !showDiff }} title="View changes">{Object.keys(dirtyFields).length} changed</button>
        <button class="btn btn-ghost btn-sm" onclick={handleDiscardFields}>Discard</button>
        <button class="btn btn-primary btn-sm" disabled={fieldSaving} onclick={handleSaveFields}>
          {fieldSaving ? 'Saving...' : 'Save'}
        </button>
      {/if}
      <div class="view-toggle">
        <button class="toggle-btn" class:active={viewMode === 'quick'} onclick={() => { viewMode = 'quick' }}>Quick Start</button>
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

    {#if showDiff && hasDirtyFields}
      <div class="diff-panel card">
        <div class="card-header">
          <span class="card-title">Pending Changes</span>
          <button class="btn btn-ghost btn-sm" onclick={() => { showDiff = false }}>Close</button>
        </div>
        <div class="diff-body">
          {#each diffEntries as entry}
            <div class="diff-row">
              <div class="diff-field">
                <span class="diff-label">{entry.label}</span>
                <span class="diff-key">{entry.path}</span>
              </div>
              <div class="diff-values">
                <span class="diff-old">{entry.oldVal}</span>
                <span class="diff-arrow">&rarr;</span>
                <span class="diff-new">{entry.newVal}</span>
              </div>
              {#if entry.impact.length > 0}
                <div class="diff-impact" aria-label={`${entry.label} Impact`}>
                  <span class="diff-impact-title">Impact</span>
                  <ul>
                    {#each entry.impact as item}
                      <li>{item}</li>
                    {/each}
                  </ul>
                </div>
              {/if}
            </div>
          {/each}
        </div>
      </div>
    {/if}

    {#if viewMode === 'quick'}
      <div class="quick-start-panel">
        <div class="quick-start-header">
          <div>
            <span class="quick-start-kicker">Essential setup</span>
            <h3>Quick Start</h3>
          </div>
          <span class="quick-start-progress">{quickStartStats.ready}/{quickStartStats.total} ready</span>
        </div>
        <div class="quick-start-grid">
          {#each quickStartItems as item}
            {@const field = item.field}
            {@const metaBadges = buildConfigMetaBadges(field, item.value, item.dirty, schemaUpdatedAt)}
            <div class="quick-start-card" class:quick-attention={item.status.kind === 'attention'}>
              <div class="quick-start-card-main">
                <div class="quick-start-title-row">
                  <span class="quick-start-title">{item.title}</span>
                  <span class={`quick-status status-${item.status.kind}`} title={item.status.message}>{item.status.label}</span>
                </div>
                <p>{item.description}</p>
                <span class="quick-status-message">{item.status.message}</span>
                {#if defaultValueSummary(field)}
                  <span class="quick-default">Default {defaultValueSummary(field)}</span>
                {/if}
                {#if metaBadges.length > 0}
                  <div class="field-meta-badges" aria-label={`${field.label} metadata`}>
                    {#each metaBadges as badge}
                      <span class={`field-meta-badge badge-${badge.tone}`} title={badge.title}>{badge.label}</span>
                    {/each}
                  </div>
                {/if}
              </div>
              <div class="quick-start-control">
                {#if field.type === 'bool'}
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
                  <select
                    class="field-select"
                    class:dirty={isDirty(field.key)}
                    value={String(getDisplayValue(field) ?? '')}
                    onchange={(e) => handleSelectChange(field, e)}
                  >
                    {#each field.options as opt}
                      <option value={opt}>{opt || '(none)'}</option>
                    {/each}
                  </select>
                {:else if editingKey === field.key}
                  <div class="field-edit">
                    {#if field.type === 'string_list'}
                      <textarea
                        class="field-textarea"
                        bind:value={editValue}
                        onkeydown={(e) => handleFieldKeydown(e, field)}
                        onblur={() => commitEdit(field)}
                      ></textarea>
                    {:else}
                      <input
                        type={field.type === 'int' || field.type === 'float' ? 'number' : 'text'}
                        step={field.type === 'float' ? '0.01' : undefined}
                        class="field-input"
                        bind:value={editValue}
                        onkeydown={(e) => handleFieldKeydown(e, field)}
                        onblur={() => commitEdit(field)}
                      />
                    {/if}
                  </div>
                {:else if field.sensitive}
                  <span class="value-text sensitive" title="Edit in YAML tab">{formatValue(field)}</span>
                {:else}
                  <button
                    class:value-btn={field.type !== 'json'}
                    class:structured-value-btn={field.type === 'json'}
                    class:dirty={isDirty(field.key)}
                    onclick={() => field.type === 'json' ? openStructuredEditor(field) : startEdit(field)}
                    title="Click to edit"
                  >
                    {#if field.type === 'json'}
                      {@const summary = structuredSummary(field)}
                      <span class="structured-main">{summary.text}</span>
                      {#if summary.preview.length > 0}
                        <span class="structured-preview">
                          {#each summary.preview as preview}
                            <span>{preview}</span>
                          {/each}
                        </span>
                      {/if}
                    {:else}
                      <span class="value-text">{formatValue(field)}</span>
                    {/if}
                  </button>
                {/if}
                {#if item.key === 'llm_providers'}
                  <button class="btn btn-ghost btn-sm" disabled={llmTestBusy} onclick={testLLMConnection}>
                    {llmTestBusy ? 'Testing...' : 'Test connection'}
                  </button>
                  {#if llmTestResult}
                    <span class={`quick-test-result test-${llmTestKind}`}>{llmTestResult}</span>
                  {/if}
                {/if}
              </div>
            </div>
          {/each}
        </div>
      </div>
    {:else if viewMode === 'form'}
      <div class="search-bar">
        <input
          type="text"
          class="search-input"
          placeholder="Filter settings..."
          bind:value={searchQuery}
        />
        {#if searchQuery}
          <button class="search-clear" onclick={() => { searchQuery = '' }}>&times;</button>
        {/if}
      </div>
      <div class="sections">
        {#each filteredSections as section}
          <div class="section-card card">
            <button class="section-header" onclick={() => toggleSection(section.name)}>
              <div class="section-header-left">
                <span class="section-icon">{sectionIcons[section.name] || '\u2699'}</span>
                <span class="section-title">{section.name}</span>
                <span class="section-count">{section.fields.length}</span>
              </div>
              <span class="section-chevron" class:open={expandedSections[section.name]}>{'\u25b8'}</span>
            </button>

            {#if expandedSections[section.name] || searchQuery.trim()}
              <div class="section-body">
                {#each section.fields as field}
                  {@const metaBadges = buildConfigMetaBadges(field, getDisplayValue(field), isDirty(field.key), schemaUpdatedAt)}
                  <div class="field-row" class:field-dirty={isDirty(field.key)}>
                    <div class="field-info">
                      <span class="field-label">{field.label}</span>
                      <span class="field-desc">{field.description}</span>
                      <span class="field-key">{fieldPath(field)}</span>
                      {#if metaBadges.length > 0}
                        <div class="field-meta-badges" aria-label={`${field.label} metadata`}>
                          {#each metaBadges as badge}
                            <span class={`field-meta-badge badge-${badge.tone}`} title={badge.title}>{badge.label}</span>
                          {/each}
                        </div>
                      {/if}
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
                          onchange={(e) => handleSelectChange(field, e)}
                        >
                          {#each field.options as opt}
                            <option value={opt}>{opt || '(none)'}</option>
                          {/each}
                        </select>
                      {:else if editingKey === field.key}
                        <!-- Editing mode -->
                        <div class="field-edit">
                          {#if field.type === 'string_list'}
                            <textarea
                              class="field-textarea"
                              bind:value={editValue}
                              onkeydown={(e) => handleFieldKeydown(e, field)}
                              onblur={() => commitEdit(field)}
                            ></textarea>
                          {:else}
                            <input
                              type={field.type === 'int' || field.type === 'float' ? 'number' : 'text'}
                              step={field.type === 'float' ? '0.01' : undefined}
                              class="field-input"
                              bind:value={editValue}
                              onkeydown={(e) => handleFieldKeydown(e, field)}
                              onblur={() => commitEdit(field)}
                            />
                          {/if}
                        </div>
                      {:else if field.sensitive}
                        <!-- Sensitive: show masked, not editable inline -->
                        <span class="value-text sensitive" title="Edit in YAML tab">{formatValue(field)}</span>
                      {:else}
                        <!-- Clickable value -->
                        <button
                          class:value-btn={field.type !== 'json'}
                          class:structured-value-btn={field.type === 'json'}
                          class:dirty={isDirty(field.key)}
                          onclick={() => field.type === 'json' ? openStructuredEditor(field) : startEdit(field)}
                          title="Click to edit"
                        >
                          {#if field.type === 'json'}
                            {@const summary = structuredSummary(field)}
                            <span class="structured-main">{summary.text}</span>
                            {#if summary.preview.length > 0}
                              <span class="structured-preview">
                                {#each summary.preview as item}
                                  <span>{item}</span>
                                {/each}
                              </span>
                            {/if}
                          {:else}
                            <span class="value-text">{formatValue(field)}</span>
                          {/if}
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

    {#if tierEditorField}
      <div class="modal-backdrop" role="presentation">
        <div class="json-editor-modal tier-editor-modal" role="dialog" aria-modal="true" aria-labelledby="tier-editor-title">
          <div class="json-editor-header">
            <div>
              <div id="tier-editor-title" class="json-editor-title">{tierEditorField.label}</div>
              <div class="json-editor-path">{fieldPath(tierEditorField)}</div>
            </div>
            <button class="btn btn-ghost btn-sm" onclick={closeTierEditor}>Cancel</button>
          </div>
          {#if tierProviderOptions.length === 0}
            <div class="message message-error">Configure llm.providers before assigning tier providers.</div>
          {/if}
          <div class="tier-editor-toolbar">
            <button class="btn btn-ghost btn-sm" onclick={addTierDraft}>Add Tier</button>
          </div>
          <div class="tier-editor-list">
            {#each tierDrafts as draft (draft.id)}
              <div class="tier-row">
                <label class="tier-field tier-field-name">
                  <span>Tier</span>
                  <input
                    class:error={!!tierError(draft.id, 'name')}
                    value={draft.name}
                    oninput={(event) => updateTierDraft(draft.id, 'name', inputValue(event))}
                  />
                  {#if tierError(draft.id, 'name')}
                    <small>{tierError(draft.id, 'name')}</small>
                  {/if}
                </label>
                <label class="tier-field tier-field-provider">
                  <span>Provider</span>
                  <select
                    class:error={!!tierError(draft.id, 'provider')}
                    value={draft.provider}
                    onchange={(event) => updateTierDraft(draft.id, 'provider', inputValue(event))}
                  >
                    <option value="">Select</option>
                    {#each tierProviderChoices(draft.provider) as provider}
                      <option value={provider}>{provider}</option>
                    {/each}
                  </select>
                  {#if tierError(draft.id, 'provider')}
                    <small>{tierError(draft.id, 'provider')}</small>
                  {/if}
                </label>
                <label class="tier-field tier-field-model">
                  <span>Model</span>
                  <input
                    class:error={!!tierError(draft.id, 'model')}
                    value={draft.model}
                    oninput={(event) => updateTierDraft(draft.id, 'model', inputValue(event))}
                  />
                  {#if tierError(draft.id, 'model')}
                    <small>{tierError(draft.id, 'model')}</small>
                  {/if}
                </label>
                <label class="tier-field tier-field-effort">
                  <span>Effort</span>
                  <select
                    value={draft.reasoning_effort}
                    onchange={(event) => updateTierDraft(draft.id, 'reasoning_effort', inputValue(event))}
                  >
                    {#each tierReasoningChoices(draft.reasoning_effort) as effort}
                      <option value={effort}>{effort || 'default'}</option>
                    {/each}
                  </select>
                </label>
                <label class="tier-field tier-field-budget">
                  <span>Budget</span>
                  <input
                    type="number"
                    min="0"
                    step="1"
                    class:error={!!tierError(draft.id, 'thinking_budget')}
                    value={draft.thinking_budget}
                    oninput={(event) => updateTierDraft(draft.id, 'thinking_budget', inputValue(event))}
                  />
                  {#if tierError(draft.id, 'thinking_budget')}
                    <small>{tierError(draft.id, 'thinking_budget')}</small>
                  {/if}
                </label>
                <label class="tier-field tier-field-service">
                  <span>Service Tier</span>
                  <input
                    value={draft.service_tier}
                    oninput={(event) => updateTierDraft(draft.id, 'service_tier', inputValue(event))}
                  />
                </label>
                <button class="btn btn-ghost btn-sm tier-remove" disabled={tierDrafts.length <= 1} onclick={() => removeTierDraft(draft.id)}>Remove</button>
              </div>
            {/each}
          </div>
          <div class="json-editor-footer">
            <button class="btn btn-ghost btn-sm" onclick={resetTierEditor}>Reset</button>
            <div class="json-editor-actions">
              <button class="btn btn-ghost btn-sm" onclick={closeTierEditor}>Cancel</button>
              <button class="btn btn-primary btn-sm" onclick={applyTierEditor}>Apply</button>
            </div>
          </div>
        </div>
      </div>
    {/if}

    {#if jsonEditorField}
      <div class="modal-backdrop" role="presentation">
        <div class="json-editor-modal" role="dialog" aria-modal="true" aria-labelledby="json-editor-title">
          <div class="json-editor-header">
            <div>
              <div id="json-editor-title" class="json-editor-title">{jsonEditorField.label}</div>
              <div class="json-editor-path">{fieldPath(jsonEditorField)}</div>
            </div>
            <button class="btn btn-ghost btn-sm" onclick={closeJSONEditor}>Cancel</button>
          </div>
          <textarea
            class="json-editor-textarea"
            bind:value={jsonEditorText}
            spellcheck={false}
          ></textarea>
          {#if jsonEditorError}
            <div class="message message-error">{jsonEditorError}</div>
          {/if}
          <div class="json-editor-footer">
            <button class="btn btn-ghost btn-sm" onclick={resetJSONEditor}>Reset</button>
            <div class="json-editor-actions">
              <button class="btn btn-ghost btn-sm" onclick={closeJSONEditor}>Cancel</button>
              <button class="btn btn-primary btn-sm" onclick={applyJSONEditor}>Apply</button>
            </div>
          </div>
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
            <strong>Reset workspace</strong>
            <span>Remove sessions, cron state, agent runtime data, logs, and memory. Config is preserved.</span>
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
    background: var(--surface-elevated);
    padding: 2px var(--space-2);
    border-radius: var(--radius-sm);
  }

  .view-toggle {
    display: flex;
    background: var(--surface-elevated);
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
  .toggle-btn.active { background: var(--primary); color: #fff; }

  .loading { color: var(--text-secondary); font-size: var(--text-sm); padding: var(--space-6); }

  .empty-state { padding: var(--space-6); text-align: center; color: var(--text-secondary); font-size: var(--text-sm); }
  .empty-state code { font-family: var(--font-mono); background: var(--surface-elevated); padding: 2px var(--space-1); border-radius: var(--radius-sm); font-size: var(--text-xs); }

  .message { font-size: var(--text-sm); padding: var(--space-2) var(--space-3); border-radius: var(--radius-md); }
  .message-error { background: rgba(220, 60, 60, 0.15); color: var(--red); border: 1px solid rgba(220, 60, 60, 0.3); }
  .message-success { background: rgba(60, 180, 100, 0.15); color: var(--green); border: 1px solid rgba(60, 180, 100, 0.3); }

  /* ── Quick Start ─────────────────────────── */
  .quick-start-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  .quick-start-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-1) 0;
  }
  .quick-start-header h3 {
    margin: 0;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-lg);
    font-weight: 600;
  }
  .quick-start-kicker {
    display: block;
    margin-bottom: 2px;
    color: var(--text-ghost);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 600;
  }
  .quick-start-progress {
    min-height: 24px;
    border: 1px solid rgba(60, 180, 100, 0.28);
    border-radius: var(--radius-sm);
    padding: 3px var(--space-2);
    color: var(--green);
    background: rgba(60, 180, 100, 0.08);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    white-space: nowrap;
  }
  .quick-start-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
    gap: var(--space-3);
  }
  .quick-start-card {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--space-3);
    align-items: start;
    min-height: 132px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    padding: var(--space-3);
    background: var(--surface-elevated);
  }
  .quick-start-card.quick-attention {
    border-color: rgba(224, 145, 69, 0.3);
    background: rgba(224, 145, 69, 0.04);
  }
  .quick-start-card-main {
    display: flex;
    min-width: 0;
    flex-direction: column;
    gap: 5px;
  }
  .quick-start-title-row {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .quick-start-title {
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .quick-start-card p {
    margin: 0;
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    line-height: 1.45;
  }
  .quick-status,
  .quick-default,
  .quick-test-result {
    width: fit-content;
    max-width: 100%;
    min-height: 18px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: 1px 6px;
    font-family: var(--font-display);
    font-size: 10px;
    font-weight: 600;
    line-height: 1.4;
  }
  .quick-status.status-ready {
    border-color: rgba(60, 180, 100, 0.28);
    color: var(--green);
    background: rgba(60, 180, 100, 0.08);
  }
  .quick-status.status-attention {
    border-color: rgba(224, 145, 69, 0.35);
    color: var(--primary);
    background: rgba(224, 145, 69, 0.08);
  }
  .quick-status.status-optional {
    color: var(--text-ghost);
    background: var(--surface-inset);
  }
  .quick-status-message {
    color: var(--text-secondary);
    font-size: var(--text-xs);
    line-height: 1.4;
  }
  .quick-default {
    color: var(--text-ghost);
    background: var(--surface-inset);
    font-family: var(--font-mono);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .quick-start-control {
    display: flex;
    align-items: flex-end;
    flex-direction: column;
    gap: var(--space-2);
    max-width: 260px;
  }
  .quick-test-result {
    text-align: right;
    word-break: break-word;
  }
  .quick-test-result.test-success {
    border-color: rgba(60, 180, 100, 0.28);
    color: var(--green);
    background: rgba(60, 180, 100, 0.08);
  }
  .quick-test-result.test-error {
    border-color: rgba(220, 60, 60, 0.28);
    color: var(--red);
    background: rgba(220, 60, 60, 0.08);
  }

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
  .section-header:hover { background: var(--surface-elevated); }

  .section-header-left { display: flex; align-items: center; gap: var(--space-3); }
  .section-icon { font-size: var(--text-md); color: var(--primary); width: 20px; text-align: center; }
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
  .field-row.field-dirty { background: rgba(224, 145, 69, 0.06); border-left: 2px solid var(--primary); }

  .field-info { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
  .field-label { font-family: var(--font-display); font-size: var(--text-sm); font-weight: 500; color: var(--text-primary); }
  .field-desc { font-size: var(--text-xs); color: var(--text-tertiary); line-height: 1.4; }
  .field-key { font-family: var(--font-mono); font-size: 10px; color: var(--text-ghost); }
  .field-meta-badges {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 4px;
    margin-top: 2px;
  }
  .field-meta-badge {
    min-height: 18px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: 1px 6px;
    font-family: var(--font-display);
    font-size: 10px;
    font-weight: 600;
    line-height: 1.4;
    color: var(--text-tertiary);
    background: var(--surface-inset);
    white-space: nowrap;
  }
  .field-meta-badge.badge-default {
    color: var(--text-ghost);
  }
  .field-meta-badge.badge-modified {
    border-color: rgba(224, 145, 69, 0.35);
    color: var(--primary);
    background: rgba(224, 145, 69, 0.08);
  }
  .field-meta-badge.badge-restart {
    border-color: rgba(220, 60, 60, 0.28);
    color: var(--red);
    background: rgba(220, 60, 60, 0.08);
  }
  .field-meta-badge.badge-live {
    border-color: rgba(60, 180, 100, 0.28);
    color: var(--green);
    background: rgba(60, 180, 100, 0.08);
  }
  .field-meta-badge.badge-secret {
    border-color: rgba(120, 120, 160, 0.28);
    color: var(--text-secondary);
    background: rgba(120, 120, 160, 0.08);
  }

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
    background: var(--surface-elevated);
  }
  .value-btn.dirty .value-text {
    color: var(--primary);
    font-weight: 500;
  }

  .structured-value-btn {
    display: grid;
    gap: var(--space-1);
    justify-items: end;
    max-width: 300px;
    min-width: 150px;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    background: transparent;
    padding: var(--space-1) var(--space-2);
    color: inherit;
    cursor: pointer;
    text-align: right;
    transition: all var(--duration-fast) var(--ease-out);
  }
  .structured-value-btn:hover {
    border-color: var(--border-default);
    background: var(--surface-elevated);
  }
  .structured-value-btn.dirty {
    border-color: rgba(224, 145, 69, 0.4);
    background: rgba(224, 145, 69, 0.08);
  }
  .structured-main {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-primary);
  }
  .structured-value-btn.dirty .structured-main { color: var(--primary); }
  .structured-preview {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 4px;
    max-width: 100%;
  }
  .structured-preview span {
    max-width: 88px;
    min-height: 18px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-tertiary);
    padding: 1px 5px;
    font-family: var(--font-mono);
    font-size: 10px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
    box-shadow: 0 0 0 1px var(--primary);
  }

  /* ── Field select ─────────────────────────── */
  .field-select {
    padding: 3px var(--space-2);
    background: var(--surface-base);
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
  .field-select:focus { outline: none; border-color: var(--primary); box-shadow: 0 0 0 2px rgba(224, 145, 69, 0.3); }
  .field-select.dirty { border-color: var(--primary); color: var(--primary); font-weight: 500; }

  /* ── Field input ─────────────────────────── */
  .field-edit { display: flex; }
  .field-input {
    width: 200px;
    padding: var(--space-1) var(--space-2);
    background: var(--surface-base);
    border: 1px solid var(--primary);
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
  .field-textarea {
    width: min(420px, 42vw);
    min-height: 120px;
    padding: var(--space-2);
    background: var(--surface-base);
    border: 1px solid var(--primary);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    line-height: 1.5;
    resize: vertical;
    outline: none;
    white-space: pre;
  }
  .field-textarea:focus {
    box-shadow: 0 0 0 2px rgba(224, 145, 69, 0.3);
  }

  /* ── YAML Editor ─────────────────────────── */
  .editor-card { display: flex; flex-direction: column; }
  .card-actions { display: flex; gap: var(--space-2); }

  .config-editor {
    width: 100%;
    min-height: 500px;
    padding: var(--space-3);
    background: var(--surface-base);
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
  .config-editor:focus { outline: none; box-shadow: inset 0 0 0 1px var(--primary); }

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

  /* ── Diff panel ──────────────────────────── */
  .diff-badge { cursor: pointer; }
  .diff-panel { border-color: rgba(224, 145, 69, 0.3); }
  .diff-body { display: flex; flex-direction: column; }

  .diff-row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--space-4);
    padding: var(--space-2) var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
    font-size: var(--text-xs);
  }
  .diff-row:last-child { border-bottom: none; }

  .diff-field { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
  .diff-label { font-family: var(--font-display); font-weight: 500; color: var(--text-primary); }
  .diff-key { font-family: var(--font-mono); font-size: 10px; color: var(--text-ghost); }

  .diff-values { display: flex; align-items: center; gap: var(--space-2); flex-shrink: 0; font-family: var(--font-mono); }
  .diff-old { color: var(--red); text-decoration: line-through; max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .diff-arrow { color: var(--text-ghost); }
  .diff-new { color: var(--green); font-weight: 600; max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .diff-impact {
    flex-basis: 100%;
    margin-top: var(--space-2);
    padding: var(--space-3);
    border-radius: var(--radius-md);
    background: var(--surface-base);
  }
  .diff-impact-title {
    display: block;
    margin-bottom: var(--space-1);
    color: var(--text-primary);
    font-family: var(--font-display);
    font-weight: 600;
  }
  .diff-impact ul {
    margin: 0;
    padding-left: var(--space-4);
    color: var(--text-secondary);
  }
  .diff-impact li + li { margin-top: 2px; }

  /* ── JSON editor modal ───────────────────── */
  .modal-backdrop {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    left: var(--nav-width);
    z-index: 30;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-4);
    background: rgba(0, 0, 0, 0.62);
  }
  .json-editor-modal {
    width: min(880px, calc(100vw - var(--nav-width) - 32px));
    max-height: calc(100vh - 32px);
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-lg);
    background: var(--surface);
    box-shadow: var(--shadow-lg);
    padding: var(--space-4);
    overflow: hidden;
  }
  .json-editor-header, .json-editor-footer {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
  }
  .json-editor-title {
    font-family: var(--font-display);
    font-size: var(--text-md);
    color: var(--text-primary);
  }
  .json-editor-path {
    margin-top: 2px;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }
  .json-editor-textarea {
    width: 100%;
    min-height: clamp(280px, 54vh, 520px);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-base);
    color: var(--text-primary);
    padding: var(--space-3);
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    line-height: 1.55;
    resize: vertical;
    tab-size: 2;
    white-space: pre;
    overflow: auto;
  }
  .json-editor-textarea:focus {
    outline: none;
    border-color: var(--primary);
    box-shadow: 0 0 0 2px rgba(224, 145, 69, 0.22);
  }
  .json-editor-actions {
    display: flex;
    gap: var(--space-2);
  }

  .tier-editor-modal {
    width: min(1040px, calc(100vw - var(--nav-width) - 32px));
  }
  .tier-editor-toolbar {
    display: flex;
    justify-content: flex-end;
  }
  .tier-editor-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-height: 0;
    overflow: auto;
    padding-right: 2px;
  }
  .tier-row {
    display: grid;
    grid-template-columns: minmax(90px, 1fr) minmax(120px, 1fr) minmax(180px, 1.4fr) minmax(110px, 0.9fr) minmax(96px, 0.8fr) minmax(130px, 1fr) auto;
    gap: var(--space-2);
    align-items: start;
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-base);
  }
  .tier-field {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }
  .tier-field span {
    font-family: var(--font-display);
    font-size: 10px;
    font-weight: 600;
    color: var(--text-ghost);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .tier-field input,
  .tier-field select {
    width: 100%;
    min-width: 0;
    height: 32px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text-primary);
    padding: 0 var(--space-2);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  .tier-field input:focus,
  .tier-field select:focus {
    outline: none;
    border-color: var(--primary);
    box-shadow: 0 0 0 2px rgba(224, 145, 69, 0.22);
  }
  .tier-field input.error,
  .tier-field select.error {
    border-color: var(--red);
    box-shadow: 0 0 0 1px rgba(220, 60, 60, 0.25);
  }
  .tier-field small {
    color: var(--red);
    font-size: 10px;
    line-height: 1.25;
  }
  .tier-remove {
    align-self: end;
    white-space: nowrap;
  }

  @media (max-width: 960px) {
    .tier-row {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    .tier-field-model,
    .tier-field-service {
      grid-column: span 2;
    }
    .tier-remove {
      justify-self: end;
    }
  }

  @media (max-width: 640px) {
    .modal-backdrop {
      left: 0;
      padding: var(--space-2);
    }
    .json-editor-modal,
    .tier-editor-modal {
      width: calc(100vw - 16px);
      max-height: calc(100vh - 16px);
    }
    .tier-row {
      grid-template-columns: minmax(0, 1fr);
    }
    .tier-field-model,
    .tier-field-service {
      grid-column: auto;
    }
    .json-editor-header,
    .json-editor-footer {
      align-items: stretch;
      flex-direction: column;
    }
    .json-editor-actions {
      justify-content: flex-end;
    }
  }

  /* ── Search bar ──────────────────────────── */
  .search-bar {
    position: relative;
    display: flex;
    align-items: center;
  }

  .search-input {
    width: 100%;
    padding: var(--space-2) var(--space-3);
    padding-right: var(--space-8);
    background: var(--surface-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-family: var(--font-body);
    font-size: var(--text-sm);
  }
  .search-input:focus {
    outline: none;
    border-color: var(--primary);
    box-shadow: 0 0 0 2px rgba(224, 145, 69, 0.2);
  }
  .search-input::placeholder { color: var(--text-ghost); }

  .search-clear {
    position: absolute;
    right: var(--space-2);
    background: none;
    border: none;
    color: var(--text-ghost);
    font-size: var(--text-md);
    cursor: pointer;
    padding: 2px 6px;
    line-height: 1;
  }
  .search-clear:hover { color: var(--text-primary); }
</style>
