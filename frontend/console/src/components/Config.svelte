<script lang="ts">
  import { onMount } from 'svelte'
  import {
    getConfigSchema,
    getProviderModels,
    patchConfigValues,
    resetWorkspace,
    restartServer,
  } from '../lib/api'
  import { buildConfigImpactPreview } from '../lib/configImpact'
  import { buildConfigMetaBadges } from '../lib/configMetaBadges'
  import { buildQuickStartItems, quickStartProgress } from '../lib/quickStartFields'
  import {
    configValuesEqual,
    formatConfigDisplayValue,
    stringifyConfigValue,
    type ConfigDisplaySummary,
  } from '../lib/configStructured'
  import type { ConfigEnvOverride, ConfigFieldMeta, ConfigSchema } from '../lib/types'
  import ConfigPendingChanges from './ConfigPendingChanges.svelte'
  import RemoteAccessCard from './RemoteAccessCard.svelte'
  import { t } from '../i18n'

  interface Props {
    onNavigate?: (path: string) => void
  }
  let { onNavigate }: Props = $props()

  let configPath = $state('')
  let schemaUpdatedAt = $state('')
  let schema: ConfigFieldMeta[] = $state([])
  let values: Record<string, unknown> = $state({})
  let effectiveValues: Record<string, unknown> = $state({})
  let envOverrides: Record<string, ConfigEnvOverride> = $state({})
  let loading = $state(true)
  let error = $state('')
  let success = $state('')

  // -- Quick Start field editing. Since the #931 freeze this page is Quick
  //    Start checks only: the full field inspector and the YAML read view are
  //    gone, and everything outside Quick Start is documented file-first in
  //    config/tars.config.example.yaml. --
  let editingKey: string | null = $state(null)
  let editValue: string = $state('')
  let editBool: boolean = $state(false)
  let fieldSaving = $state(false)
  let dirtyFields: Record<string, unknown> = $state({})
  let llmTestBusy = $state(false)
  let llmTestResult = $state('')
  let llmTestKind: 'success' | 'error' | '' = $state('')

  let hasDirtyFields = $derived(Object.keys(dirtyFields).length > 0)
  let quickStartItems = $derived(buildQuickStartItems(schema, values, dirtyFields))
  let quickStartStats = $derived(quickStartProgress(quickStartItems))
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
      success = $t.config.workspaceResetSuccess(result.removed)
      resetWsConfirm = false
    } catch (e) {
      error = e instanceof Error ? e.message : $t.config.failedResetWorkspace
    } finally {
      resetWsBusy = false
    }
  }

  async function load() {
    loading = true
    error = ''
    try {
      const schemaResp = await getConfigSchema()
      configPath = schemaResp.path
      schemaUpdatedAt = schemaResp.updated_at || ''
      schema = schemaResp.fields
      values = schemaResp.values
      effectiveValues = schemaResp.effective_values || {}
      envOverrides = schemaResp.env_overrides || {}
      dirtyFields = {}
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load config'
    } finally {
      loading = false
    }
  }

  function startEdit(field: ConfigFieldMeta) {
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
    } else {
      // Sensitive fields start empty so the masked value is not exposed
      editValue = field.sensitive ? '' : (current !== undefined && current !== null ? String(current) : '')
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

  function envOverrideFor(field: ConfigFieldMeta): ConfigEnvOverride | null {
    return envOverrides[field.key] || null
  }

  function effectiveValueFor(field: ConfigFieldMeta): unknown {
    if (!Object.prototype.hasOwnProperty.call(effectiveValues, field.key)) return getDisplayValue(field)
    return effectiveValues[field.key]
  }

  function effectiveValueSummary(field: ConfigFieldMeta): string {
    const value = effectiveValueFor(field)
    if (field.sensitive && typeof value === 'string' && value.length > 0) {
      return value.includes('*') ? value : '********'
    }
    return stringifyConfigValue(value)
  }

  function envOverrideTitle(field: ConfigFieldMeta): string {
    const override = envOverrideFor(field)
    const active = effectiveValueSummary(field)
    if (!override) return ''
    return configValuesEqual(effectiveValueFor(field), getDisplayValue(field))
      ? `${override.env_key} is set and controls the runtime value.`
      : `${override.env_key} is set. Runtime value is ${active}; YAML/editor value is ${formatValue(field)}.`
  }

  function envOverrideDiffers(field: ConfigFieldMeta): boolean {
    return !!envOverrideFor(field) && !configValuesEqual(effectiveValueFor(field), getDisplayValue(field))
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
      // Reload to get fresh values
      const schemaResp = await getConfigSchema()
      schemaUpdatedAt = schemaResp.updated_at || ''
      values = schemaResp.values
      effectiveValues = schemaResp.effective_values || {}
      envOverrides = schemaResp.env_overrides || {}
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to save config'
    } finally {
      fieldSaving = false
    }
  }

  function handleDiscardFields() {
    dirtyFields = {}
    cancelEdit()
    success = ''
    error = ''
  }

  function handleKeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === 's') {
      e.preventDefault()
      if (hasDirtyFields && !fieldSaving) handleSaveFields()
    }
  }

  function handleFieldKeydown(e: KeyboardEvent, field: ConfigFieldMeta) {
    const multiline = field.type === 'string_list'
    if (e.key === 'Enter' && (!multiline || e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      commitEdit(field)
    } else if (e.key === 'Escape') {
      cancelEdit()
    }
  }

  function formatValue(field: ConfigFieldMeta): string {
    const v = getDisplayValue(field)
    if (field.sensitive && typeof v === 'string' && v.length > 0) {
      return v.includes('*') ? v : '\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022'
    }
    const summary = formatConfigDisplayValue(v)
    if (field.type === 'json' || summary.kind === 'array' || summary.kind === 'object') return summary.text
    return summary.raw
  }

  function fieldPath(field: ConfigFieldMeta): string {
    return field.path || field.key
  }

  // Deep link for structured fields that keep a UI editor elsewhere.
  // Provider/tier editing lives in the onboarding wizard reentry; every
  // other structured field is documented file-first (DESIGN.md #931).
  function jsonWizardLink(key: string): string | null {
    if (key === 'llm_providers') return '/console/onboarding?reentry=1&section=provider'
    if (key === 'llm_tiers') return '/console/onboarding?reentry=1&section=tiers'
    return null
  }

  function openJSONWizard(key: string) {
    const link = jsonWizardLink(key)
    if (link) onNavigate?.(link)
  }

  onMount(() => { load() })
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="config-page" onkeydown={handleKeydown}>
  <div class="page-header">
    <div class="page-header-left">
      <h2 class="page-title">{$t.config.pageTitle}</h2>
      {#if configPath}
        <span class="config-path">{configPath}</span>
      {/if}
    </div>
    <div class="page-header-right">
      {#if hasDirtyFields}
        <button class="badge badge-warning diff-badge" onclick={() => { showDiff = !showDiff }} title={$t.config.viewChangesTooltip}>{$t.config.changedSuffix(Object.keys(dirtyFields).length)}</button>
        <button class="btn btn-ghost btn-sm" onclick={handleDiscardFields}>{$t.config.discard}</button>
        <button class="btn btn-primary btn-sm" disabled={fieldSaving} onclick={handleSaveFields}>
          {fieldSaving ? $t.config.saving : $t.config.save}
        </button>
      {/if}
    </div>
  </div>

  {#if onNavigate}
    <div class="wizard-entry-card" role="region" aria-label="Onboarding wizard entry">
      <div>
        <span class="wizard-entry-kicker">{$t.configWizardCard.kicker}</span>
        <p class="wizard-entry-text">{$t.configWizardCard.body}</p>
      </div>
      <button
        type="button"
        class="btn btn-primary btn-sm"
        onclick={() => onNavigate?.('/console/onboarding?reentry=1')}
      >{$t.configWizardCard.button}</button>
    </div>
  {/if}

  <RemoteAccessCard />

  {#if loading}
    <div class="loading">{$t.config.loading}</div>
  {:else if !configPath}
    <div class="card empty-state">
      <p>{$t.config.noConfigFile}</p>
    </div>
  {:else}
    {#if error}
      <div class="message message-error">{error}</div>
    {/if}
    {#if success}
      <div class="message message-success">{success}</div>
    {/if}

    {#if showDiff && hasDirtyFields}
      <ConfigPendingChanges entries={diffEntries} onClose={() => { showDiff = false }} />
    {/if}

    <div class="quick-start-panel">
      <div class="quick-start-header">
        <div>
          <span class="quick-start-kicker">{$t.config.quickStartKicker}</span>
          <h3>{$t.config.quickStartTitle}</h3>
        </div>
        <span class="quick-start-progress">{$t.config.quickStartReady(quickStartStats.ready, quickStartStats.total)}</span>
      </div>
      <div class="quick-start-grid">
        {#each quickStartItems as item}
          {@const field = item.field}
          {@const metaBadges = buildConfigMetaBadges(field, item.value, item.dirty, schemaUpdatedAt)}
          {@const envOverride = envOverrideFor(field)}
          <div class="quick-start-card" class:quick-attention={item.status.kind === 'attention'}>
            <div class="quick-start-card-main">
              <div class="quick-start-title-row">
                <span class="quick-start-title">{item.title}</span>
                <span class={`quick-status status-${item.status.kind}`} title={item.status.message}>{item.status.label}</span>
              </div>
              <p>{item.description}</p>
              <span class="quick-status-message">{item.status.message}</span>
              {#if defaultValueSummary(field)}
                <span class="quick-default">{$t.config.defaultPrefix(defaultValueSummary(field))}</span>
              {/if}
              {#if metaBadges.length > 0}
                <div class="field-meta-badges" aria-label={`${field.label} metadata`}>
                  {#each metaBadges as badge}
                    <span class={`field-meta-badge badge-${badge.tone}`} title={badge.title}>{badge.label}</span>
                  {/each}
                </div>
              {/if}
              {#if envOverride}
                <div class="field-meta-badges" aria-label={`${field.label} environment override`}>
                  <span class="field-meta-badge badge-env" title={envOverrideTitle(field)}>ENV {envOverride.env_key}</span>
                  {#if envOverrideDiffers(field)}
                    <span class="field-meta-badge badge-env-active" title={envOverrideTitle(field)}>Active: {effectiveValueSummary(field)}</span>
                  {/if}
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
                  title={$t.config.clickToToggle}
                >
                  {getDisplayValue(field) ? $t.config.on : $t.config.off}
                </button>
              {:else if field.type === 'select' && field.options}
                <select
                  class="field-select"
                  class:dirty={isDirty(field.key)}
                  value={String(getDisplayValue(field) ?? '')}
                  onchange={(e) => handleSelectChange(field, e)}
                >
                  {#each field.options as opt}
                    <option value={opt}>{opt || $t.config.selectNone}</option>
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
                      type={field.sensitive ? 'password' : (field.type === 'int' || field.type === 'float' ? 'number' : 'text')}
                      step={field.type === 'float' ? '0.01' : undefined}
                      class="field-input"
                      bind:value={editValue}
                      onkeydown={(e) => handleFieldKeydown(e, field)}
                      onblur={() => commitEdit(field)}
                    />
                  {/if}
                </div>
              {:else if field.sensitive}
                <button
                  class="value-btn"
                  class:dirty={isDirty(field.key)}
                  onclick={() => startEdit(field)}
                  title={$t.config.clickToEdit}
                >
                  <span class="value-text sensitive">{formatValue(field)}</span>
                </button>
              {:else if field.type === 'json'}
                {@const summary = structuredSummary(field)}
                <div class="structured-readonly">
                  <span class="structured-main">{summary.text}</span>
                  {#if summary.preview.length > 0}
                    <span class="structured-preview">
                      {#each summary.preview as preview}
                        <span>{preview}</span>
                      {/each}
                    </span>
                  {/if}
                  {#if jsonWizardLink(item.key)}
                    <button class="btn btn-secondary btn-sm" onclick={() => openJSONWizard(item.key)}>
                      Edit in wizard
                    </button>
                  {:else}
                    <span class="yaml-key-hint" title="Documented in config/tars.config.example.yaml">YAML: {fieldPath(field)}</span>
                  {/if}
                </div>
              {:else}
                <button
                  class="value-btn"
                  class:dirty={isDirty(field.key)}
                  onclick={() => startEdit(field)}
                  title={$t.config.clickToEdit}
                >
                  <span class="value-text">{formatValue(field)}</span>
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
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    overflow-y: auto;
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

  .wizard-entry-card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border-soft);
    border-radius: 8px;
    background: var(--surface-2);
  }
  .wizard-entry-kicker {
    display: inline-block;
    padding: 2px 8px;
    background: rgba(224, 145, 69, 0.12);
    border: 1px solid var(--primary);
    color: var(--primary);
    border-radius: 999px;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  .wizard-entry-text {
    margin: var(--space-2) 0 0;
    color: var(--text-muted);
    font-size: 13px;
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

  .loading { color: var(--text-secondary); font-size: var(--text-sm); padding: var(--space-6); }

  .empty-state { padding: var(--space-6); text-align: center; color: var(--text-secondary); font-size: var(--text-sm); }

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

  /* ── Field metadata ──────────────────────── */
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
  .field-meta-badge.badge-env {
    border-color: rgba(90, 135, 220, 0.34);
    color: #8fb5ff;
    background: rgba(90, 135, 220, 0.1);
  }
  .field-meta-badge.badge-env-active {
    border-color: rgba(224, 145, 69, 0.42);
    color: var(--primary);
    background: rgba(224, 145, 69, 0.11);
  }

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

  .structured-readonly {
    display: grid;
    gap: var(--space-1);
    justify-items: end;
    max-width: 320px;
    min-width: 150px;
    text-align: right;
  }
  .structured-main {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-primary);
  }
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


  .yaml-key-hint {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
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
</style>
