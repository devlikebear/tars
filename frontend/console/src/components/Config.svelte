<script lang="ts">
  // Settings page (#1238/#1239/#1241/#1242): section tabs with a
  // basic/advanced split per tab, a cross-tab search, and a sticky save bar
  // replace the old single "Quick Start" grid of all 181 fields. See
  // frontend/console/DESIGN.md's "Settings" section for the layout this
  // implements.
  import { onMount } from 'svelte'
  import {
    getConfigSchema,
    patchConfigValues,
    resetWorkspace,
    restartServer,
    testProviders,
  } from '../lib/api'
  import { describeProviderProbe, type ProviderProbeRow } from '../lib/providerProbe'
  import { buildConfigImpactPreview } from '../lib/configImpact'
  import {
    advancedFields,
    basicFields,
    defaultConfigTabId,
    groupFieldsByTab,
    isConfigTabId,
    isFieldShown,
  } from '../lib/configSections'
  import type { ConfigTabId } from '../lib/configSections'
  import { searchConfigFields, type ConfigQuickFilter } from '../lib/configSearch'
  import { localizeConfigField } from '../lib/configI18n'
  import {
    configValuesEqual,
    formatConfigDisplayValue,
    stringifyConfigValue,
  } from '../lib/configStructured'
  import type { ConfigEnvOverride, ConfigFieldMeta, ConfigSchema } from '../lib/types'
  import ConfigPendingChanges from './ConfigPendingChanges.svelte'
  import ConfigSaveBar from './ConfigSaveBar.svelte'
  import ConfigTabs from './ConfigTabs.svelte'
  import ConfigFieldRow from './ConfigFieldRow.svelte'
  import ConfigLLMEditor from './ConfigLLMEditor.svelte'
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
  let pendingKeys: string[] = $state([])
  let runtimeStartedAt = $state('')
  let processID = $state(0)
  let loading = $state(true)
  let error = $state('')
  let success = $state('')

  let dirtyFields: Record<string, unknown> = $state({})
  let fieldSaving = $state(false)
  let llmTestBusy = $state(false)
  let llmTestRows: ProviderProbeRow[] | null = $state(null)
  let llmTestError = $state('')
  let showDiff = $state(false)

  let hasDirtyFields = $derived(Object.keys(dirtyFields).length > 0)
  // Merges pending edits over saved values, so show_when and search both
  // react immediately to an unsaved toggle.
  let displayValues = $derived({ ...values, ...dirtyFields })

  // -- Tabs --
  function initialTab(): ConfigTabId {
    if (typeof window === 'undefined') return defaultConfigTabId()
    const match = window.location.pathname.match(/\/console\/config\/([a-zA-Z]+)/)
    const candidate = match?.[1]
    return isConfigTabId(candidate) ? candidate : defaultConfigTabId()
  }
  let activeTab: ConfigTabId = $state(initialTab())
  let expandedAdvanced: Set<ConfigTabId> = $state(new Set())

  function selectTab(id: ConfigTabId) {
    activeTab = id
    onNavigate?.(`/console/config/${id}`)
  }

  // RemoteAccessCard is the one editor for these two fields (shown on the
  // Channels & Remote tab); keep them out of the generic field-row list and
  // out of search results too, or a result would jump to a row that no
  // longer renders and the generic row would race the card's own save.
  const REMOTE_ACCESS_FIELD_KEYS = new Set(['remote_access_tailscale_serve_enabled', 'remote_access_tailscale_serve_https_port'])
  // ConfigLLMEditor (#1286) is the one editor for the LLM pool fields —
  // same reasoning as RemoteAccessCard above. llm_role_defaults is
  // 'advanced'-tier in the schema; pulling it out here means the LLM tab's
  // "Advanced (N)" toggle never gets a row it would render empty.
  const LLM_POOL_FIELD_KEYS = new Set(['llm_providers', 'llm_tiers', 'llm_role_defaults'])
  function isGenericRow(field: ConfigFieldMeta): boolean {
    return !REMOTE_ACCESS_FIELD_KEYS.has(field.key) && !LLM_POOL_FIELD_KEYS.has(field.key)
  }

  let tabFields = $derived(groupFieldsByTab(schema, displayValues))
  let currentTabFields = $derived((tabFields.find((t2) => t2.id === activeTab)?.fields ?? []).filter(isGenericRow))
  let currentBasic = $derived(basicFields(currentTabFields))
  let currentAdvanced = $derived(advancedFields(currentTabFields))
  let advancedOpen = $derived(expandedAdvanced.has(activeTab))

  // ConfigLLMEditor needs the ConfigFieldMeta for each pool field to route
  // its edits through the same setFieldValue/dirtyFields path as every
  // other field (masked-key stripping, diff, save, revert).
  let llmProvidersField = $derived(schema.find((f) => f.key === 'llm_providers'))
  let llmTiersField = $derived(schema.find((f) => f.key === 'llm_tiers'))
  let llmRoleDefaultsField = $derived(schema.find((f) => f.key === 'llm_role_defaults'))

  function toggleAdvanced() {
    const next = new Set(expandedAdvanced)
    if (next.has(activeTab)) next.delete(activeTab)
    else next.add(activeTab)
    expandedAdvanced = next
  }

  // -- Search --
  let search = $state('')
  let activeFilters: Set<ConfigQuickFilter> = $state(new Set())

  function toggleFilter(filter: ConfigQuickFilter) {
    const next = new Set(activeFilters)
    if (next.has(filter)) next.delete(filter)
    else next.add(filter)
    activeFilters = next
  }

  // searchConfigFields only hides file/remove-tier fields on its own; a
  // field still hidden by its own show_when (e.g. the Telegram session
  // scope while the Telegram channel is off) has no row to jump to, so
  // filter those out here before searching, same as isGenericRow above.
  let searchableFields = $derived(schema.filter((f) => isFieldShown(f, displayValues, schema) && isGenericRow(f)))

  let searchActive = $derived(search.trim().length > 0 || activeFilters.size > 0)
  let searchResult = $derived(
    searchActive
      ? searchConfigFields({
          fields: searchableFields,
          query: search,
          localize: (field) => localizeConfigField(field, $t.config),
          values: displayValues,
          dirtyFields,
          envOverrides,
          filters: [...activeFilters],
        })
      : null,
  )

  function clearSearch() {
    search = ''
    activeFilters = new Set()
  }

  function jumpToField(field: ConfigFieldMeta, tabId: ConfigTabId) {
    clearSearch()
    activeTab = tabId
    // An advanced-tier field's row does not exist in the DOM at all while
    // its tab's "Advanced (N)" section is collapsed (the default) — expand
    // it first, or scrollIntoView has nothing to find.
    if (field.tier === 'advanced' && !expandedAdvanced.has(tabId)) {
      expandedAdvanced = new Set(expandedAdvanced).add(tabId)
    }
    onNavigate?.(`/console/config/${tabId}`)
    setTimeout(() => {
      const el = document.getElementById(`config-field-${field.key}`)
      el?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    }, 0)
  }

  function handleSearchKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') clearSearch()
  }

  function handleGlobalKeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === 's') {
      e.preventDefault()
      if (hasDirtyFields && !fieldSaving) handleSaveFields()
      return
    }
    if (e.key === '/' && document.activeElement?.tagName !== 'INPUT' && document.activeElement?.tagName !== 'TEXTAREA') {
      e.preventDefault()
      document.getElementById('config-search-input')?.focus()
    }
  }

  // -- Loading / values --
  async function load() {
    loading = true
    error = ''
    try {
      const schemaResp = await getConfigSchema()
      applySchema(schemaResp)
      dirtyFields = {}
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load config'
    } finally {
      loading = false
    }
  }

  function applySchema(schemaResp: ConfigSchema) {
    configPath = schemaResp.path
    schemaUpdatedAt = schemaResp.updated_at || ''
    schema = schemaResp.fields
    values = schemaResp.values
    effectiveValues = schemaResp.effective_values || {}
    envOverrides = schemaResp.env_overrides || {}
    pendingKeys = schemaResp.pending_restart_keys || []
    runtimeStartedAt = schemaResp.runtime_started_at || ''
    processID = schemaResp.process_id || 0
  }

  function getDisplayValue(field: ConfigFieldMeta): unknown {
    return dirtyFields[field.key] !== undefined ? dirtyFields[field.key] : values[field.key]
  }

  function isDirty(key: string): boolean {
    return dirtyFields[key] !== undefined
  }

  // llm_providers shows each provider's api_key already masked with `*`s
  // (the server never sends a real secret back). Editing the JSON blob
  // without touching that field must not round-trip the mask back as the
  // new "secret" and overwrite the real key — strip any masked api_key
  // before it reaches dirtyFields, same as the old per-field JSON editor.
  function stripMaskedProviderKeys(field: ConfigFieldMeta, parsed: unknown): unknown {
    if (field.key !== 'llm_providers' || !parsed || typeof parsed !== 'object') return parsed
    for (const provider of Object.values(parsed as Record<string, Record<string, unknown>>)) {
      if (typeof provider.api_key === 'string' && provider.api_key.includes('*')) delete provider.api_key
    }
    return parsed
  }

  function setFieldValue(field: ConfigFieldMeta, rawParsed: unknown) {
    const parsed = stripMaskedProviderKeys(field, rawParsed)
    const original = values[field.key]
    if (configValuesEqual(parsed, original)) {
      delete dirtyFields[field.key]
    } else {
      dirtyFields[field.key] = parsed
    }
    dirtyFields = { ...dirtyFields }
  }

  function revertField(field: ConfigFieldMeta) {
    if (isDirty(field.key)) {
      delete dirtyFields[field.key]
      dirtyFields = { ...dirtyFields }
      return
    }
    setFieldValue(field, field.default_value)
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

  function searchHitValueText(field: ConfigFieldMeta): string {
    const v = getDisplayValue(field)
    if (field.sensitive && typeof v === 'string' && v.length > 0) {
      return v.includes('*') ? v : '••••••••'
    }
    return formatConfigDisplayValue(v, $t.config.values).text
  }

  function envOverrideActiveSummary(field: ConfigFieldMeta): string | undefined {
    const override = envOverrideFor(field)
    if (!override) return undefined
    return configValuesEqual(effectiveValueFor(field), getDisplayValue(field)) ? undefined : effectiveValueSummary(field)
  }

  let diffEntries = $derived.by(() => {
    return Object.entries(dirtyFields).map(([key, newVal]) => {
      const field = schema.find((f) => f.key === key)
      return {
        key,
        label: field ? localizeConfigField(field, $t.config).label : key,
        path: field?.path || key,
        oldVal: field?.sensitive || key === 'llm_providers' ? '••••' : stringifyConfigValue(values[key]),
        newVal: field?.sensitive || key === 'llm_providers' ? '••••' : stringifyConfigValue(newVal),
        impact: buildConfigImpactPreview(field, values[key], newVal).items,
      }
    })
  })

  // -- Restart --
  let saveAndApply = $state(false)
  let restartBusy = $state(false)
  let restartConfirm = $state(false)

  async function handleRestart() {
    if (!restartConfirm) { restartConfirm = true; return }
    restartBusy = true
    error = ''
    success = ''
    try {
      const previousRuntime = runtimeStartedAt
      await restartServer()
      restartConfirm = false
      success = $t.config.reconnecting
      let recovered = false
      for (let attempt = 0; attempt < 30; attempt++) {
        await new Promise((resolve) => setTimeout(resolve, 1000))
        try {
          const status = await getConfigSchema(AbortSignal.timeout(2000))
          if (status.runtime_started_at && status.runtime_started_at !== previousRuntime) {
            applySchema(status)
            success = pendingKeys.length ? $t.config.savedPending : $t.config.applied
            recovered = true
            break
          }
        } catch { /* Retry while the server is restarting. */ }
      }
      if (!recovered) throw new Error($t.config.reconnectFailed)
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to restart server'
    } finally {
      restartBusy = false
    }
  }

  // -- Reset / danger zone --
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

  // Tests every provider alias in the pool, the default tier's first. CLI
  // providers have no model list, so the server checks install, version and
  // sign-in instead of reporting them as broken.
  async function testLLMConnection() {
    llmTestBusy = true
    llmTestError = ''
    try {
      const response = await testProviders()
      const strings = $t.providerTest
      llmTestRows = (response.results || []).map((result) => describeProviderProbe(result, strings))
    } catch (e) {
      llmTestRows = null
      llmTestError = $t.providerTest.failed(e instanceof Error ? e.message : String(e))
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
      success = $t.config.savedPending
      dirtyFields = {}
      const schemaResp = await getConfigSchema()
      applySchema(schemaResp)
      if (saveAndApply) restartConfirm = true
      saveAndApply = false
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

  onMount(() => { load() })
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="config-page" onkeydown={handleGlobalKeydown}>
  <div class="page-header">
    <div class="page-header-left">
      <h2 class="page-title">{$t.config.pageTitle}</h2>
      {#if configPath}
        <span class="config-path">{configPath}</span>
      {/if}
    </div>
  </div>

  {#if onNavigate}
    <div class="wizard-entry-card" role="region" aria-label="Onboarding wizard entry">
      <div>
        <span class="wizard-entry-kicker">{$t.configWizardCard.kicker}</span>
        <p class="wizard-entry-text">{$t.configWizardCard.body}</p>
      </div>
      <button type="button" class="btn btn-primary btn-sm" onclick={() => onNavigate?.('/console/onboarding?reentry=1')}>
        {$t.configWizardCard.button}
      </button>
    </div>
  {/if}

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

    <div class="config-search-bar">
      <div class="config-search-input-wrap">
        <input
          id="config-search-input"
          class="config-search-input config-control-input"
          type="search"
          aria-label={$t.config.search.label}
          placeholder={$t.config.search.placeholder}
          bind:value={search}
          onkeydown={handleSearchKeydown}
        />
        {#if search}
          <button type="button" class="config-search-clear" onclick={clearSearch}>{$t.config.search.clear}</button>
        {/if}
      </div>
      <div class="config-search-filters" role="group" aria-label={$t.config.search.label}>
        <button type="button" class="config-filter-chip" class:active={activeFilters.has('changedFromDefault')} onclick={() => toggleFilter('changedFromDefault')}>
          {$t.config.search.filters.changedFromDefault}
        </button>
        <button type="button" class="config-filter-chip" class:active={activeFilters.has('envOverridden')} onclick={() => toggleFilter('envOverridden')}>
          {$t.config.search.filters.envOverridden}
        </button>
        <button type="button" class="config-filter-chip" class:active={activeFilters.has('unsaved')} onclick={() => toggleFilter('unsaved')}>
          {$t.config.search.filters.unsaved}
        </button>
      </div>
    </div>

    {#if searchActive && searchResult}
      <div class="config-search-results">
        <p class="config-search-count">{$t.config.search.resultsCount(searchResult.totalCount)}</p>
        {#if searchResult.totalCount === 0}
          <p class="config-empty-state">{$t.config.search.noResults}</p>
        {:else}
          {#each searchResult.groups as group}
            <div class="config-search-group">
              <h4 class="config-search-group-title">{group.sectionLabel} <span class="config-search-group-count">{group.hits.length}</span></h4>
              {#each group.hits as hit}
                <button type="button" class="config-search-hit" onclick={() => jumpToField(hit.field, hit.tabId)} title={$t.config.search.jumpTo}>
                  <span class="config-search-hit-label">
                    {#each hit.labelSegments as seg}{#if seg.matched}<mark>{seg.text}</mark>{:else}{seg.text}{/if}{/each}
                  </span>
                  <span class="config-search-hit-description">
                    {#each hit.descriptionSegments as seg}{#if seg.matched}<mark>{seg.text}</mark>{:else}{seg.text}{/if}{/each}
                  </span>
                  <span class="config-search-hit-meta">
                    <span class="config-search-hit-path">{hit.field.path}</span>
                    <span class="config-search-hit-value">{searchHitValueText(hit.field)}</span>
                  </span>
                </button>
              {/each}
            </div>
          {/each}
        {/if}
      </div>
    {:else}
      <ConfigTabs active={activeTab} onSelect={selectTab} />

      {#if activeTab === 'channelsRemote'}
        <RemoteAccessCard />
      {/if}

      {#if activeTab === 'llm' && llmProvidersField && llmTiersField && llmRoleDefaultsField}
        <div class="config-llm-editor-card card">
          <ConfigLLMEditor
            providersValue={getDisplayValue(llmProvidersField)}
            tiersValue={getDisplayValue(llmTiersField)}
            roleDefaultsValue={getDisplayValue(llmRoleDefaultsField)}
            onProvidersChange={(v) => setFieldValue(llmProvidersField, v)}
            onTiersChange={(v) => setFieldValue(llmTiersField, v)}
            onRoleDefaultsChange={(v) => setFieldValue(llmRoleDefaultsField, v)}
            text={$t.config.llm}
          />
          <div class="config-llm-hint">
            <button type="button" class="btn btn-ghost btn-sm" disabled={llmTestBusy} onclick={testLLMConnection}>
              {llmTestBusy ? $t.providerTest.testing : $t.providerTest.button}
            </button>
          </div>
          {#if llmTestRows !== null || llmTestError}
            <div class="provider-test" role="status" aria-label={$t.providerTest.resultsLabel}>
              {#if llmTestError}
                <p class="provider-test-error">{llmTestError}</p>
              {:else if llmTestRows && llmTestRows.length === 0}
                <p class="provider-test-empty">{$t.providerTest.noProviders}</p>
              {:else if llmTestRows}
                <ul class="provider-test-list">
                  {#each llmTestRows as row (row.alias)}
                    <li class={`provider-test-row tone-${row.tone}`}>
                      <div class="provider-test-head">
                        <span class="provider-test-alias">{row.alias}</span>
                        <span class="provider-test-kind">{row.kind}</span>
                        {#if row.isDefault}
                          <span class="badge badge-default">{$t.providerTest.defaultBadge}</span>
                        {/if}
                        <span class={`badge badge-${row.tone} provider-test-status`}>{row.statusLabel}</span>
                      </div>
                      {#if row.summary}<p class="provider-test-summary">{row.summary}</p>{/if}
                      {#if row.detail}<p class="provider-test-detail">{row.detail}</p>{/if}
                    </li>
                  {/each}
                </ul>
              {/if}
            </div>
          {/if}
        </div>
      {/if}

      <div class="config-tab-body card">
        {#if currentBasic.length === 0 && currentAdvanced.length === 0}
          <p class="config-empty-state">{$t.config.emptyTab}</p>
        {:else}
          {#if currentBasic.length > 0}
            <div class="config-field-list">
              {#each currentBasic as field (field.key)}
                <ConfigFieldRow
                  {field}
                  loc={localizeConfigField(field, $t.config)}
                  value={getDisplayValue(field)}
                  dirty={isDirty(field.key)}
                  envOverride={envOverrideFor(field)}
                  envOverrideActiveValue={envOverrideActiveSummary(field)}
                  onChange={(v) => setFieldValue(field, v)}
                  onRevert={() => revertField(field)}
                />
              {/each}
            </div>
          {/if}

          {#if currentAdvanced.length > 0}
            <button type="button" class="config-advanced-toggle" onclick={toggleAdvanced} aria-expanded={advancedOpen}>
              {$t.config.advancedHeading(currentAdvanced.length)}
            </button>
            {#if advancedOpen}
              <div class="config-field-list">
                {#each currentAdvanced as field (field.key)}
                  <ConfigFieldRow
                    {field}
                    loc={localizeConfigField(field, $t.config)}
                    value={getDisplayValue(field)}
                    dirty={isDirty(field.key)}
                    envOverride={envOverrideFor(field)}
                    envOverrideActiveValue={envOverrideActiveSummary(field)}
                    onChange={(v) => setFieldValue(field, v)}
                    onRevert={() => revertField(field)}
                  />
                {/each}
              </div>
            {/if}
          {/if}
        {/if}
      </div>

      {#if activeTab === 'advanced'}
        <div class="restart-section card">
          <div class="card-header">
            <span class="card-title">{$t.config.restart.title}</span>
          </div>
          <div class="danger-body">
            <div class="danger-row">
              <div class="danger-info">
                <strong>{$t.config.restart.label}</strong>
                <span>{$t.config.restart.description}</span>
              </div>
              <button class="btn btn-primary btn-sm" disabled={restartBusy} onclick={handleRestart}>
                {restartConfirm ? $t.config.restart.confirm : restartBusy ? $t.config.restart.restarting : $t.config.restart.label}
              </button>
            </div>
          </div>
        </div>

        <div class="danger-zone card">
          <div class="card-header">
            <span class="card-title danger-title">{$t.config.dangerZone.title}</span>
          </div>
          <div class="danger-body">
            <div class="danger-row">
              <div class="danger-info">
                <strong>{$t.config.dangerZone.resetLabel}</strong>
                <span>{$t.config.dangerZone.resetDescription}</span>
              </div>
              <button class="btn btn-danger btn-sm" disabled={resetWsBusy} onclick={handleResetWorkspace}>
                {resetWsConfirm ? $t.config.dangerZone.confirm : resetWsBusy ? $t.config.dangerZone.resetting : $t.config.dangerZone.resetLabel}
              </button>
            </div>
          </div>
        </div>
      {/if}
    {/if}
  {/if}
</div>

<ConfigSaveBar
  changedCount={Object.keys(dirtyFields).length}
  saving={fieldSaving}
  restartConfirm={restartConfirm}
  restartBusy={restartBusy}
  onDiscard={handleDiscardFields}
  onSave={() => { saveAndApply = false; handleSaveFields() }}
  onSaveAndRestart={() => { saveAndApply = true; handleSaveFields() }}
  onViewChanges={() => { showDiff = !showDiff }}
  onConfirmRestart={handleRestart}
  onCancelRestart={() => { restartConfirm = false }}
/>

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

  .page-header-left {
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
    flex-wrap: wrap;
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
    background: rgba(var(--primary-rgb), 0.12);
    border: 1px solid var(--primary);
    color: var(--primary);
    border-radius: 999px;
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  .wizard-entry-text {
    margin: var(--space-2) 0 0;
    color: var(--text-muted);
    font-size: var(--text-sm);
  }

  .config-search-bar {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .config-search-input-wrap {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  /* .config-control-input (app.css, #1242) gives the search box the same
     control look as every field input; only the layout here is local. */
  .config-search-input {
    flex: 1;
    height: 36px;
  }

  .config-search-clear {
    background: none;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    padding: var(--space-1) var(--space-2);
    color: var(--text-secondary);
    font-size: var(--text-xs);
    cursor: pointer;
  }

  .config-search-filters {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .config-filter-chip {
    border: 1px solid var(--border-subtle);
    border-radius: 999px;
    background: var(--surface-inset);
    color: var(--text-secondary);
    font-size: var(--text-xs);
    padding: 3px 10px;
    cursor: pointer;
  }

  .config-filter-chip.active {
    border-color: var(--primary);
    color: var(--primary);
    background: rgba(var(--primary-rgb), 0.1);
  }

  .config-search-results {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .config-search-count {
    margin: 0;
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .config-search-group-title {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0 0 var(--space-1);
    font-family: var(--font-display);
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }

  .config-search-group-count {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .config-search-hit {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 100%;
    text-align: left;
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    margin-bottom: var(--space-1);
    cursor: pointer;
    padding: var(--space-2) var(--space-3);
  }

  .config-search-hit:hover {
    border-color: var(--primary);
  }

  .config-search-hit-label {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .config-search-hit-description {
    font-family: var(--font-body);
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .config-search-hit-meta {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .config-search-hit-path {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-ghost);
    overflow-wrap: anywhere;
  }

  .config-search-hit-value {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    flex-shrink: 0;
  }

  .config-search-hit mark {
    background: rgba(var(--primary-rgb), 0.28);
    color: var(--text-primary);
    border-radius: 2px;
  }

  .config-empty-state {
    color: var(--text-tertiary);
    font-size: var(--text-sm);
    padding: var(--space-4);
    text-align: center;
  }

  .config-tab-body {
    padding: 0;
  }

  .config-llm-editor-card {
    padding: var(--space-3);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .config-field-list {
    display: flex;
    flex-direction: column;
  }

  .config-advanced-toggle {
    width: 100%;
    text-align: left;
    background: var(--surface-inset);
    border: none;
    border-top: 1px solid var(--border-subtle);
    padding: var(--space-2) var(--space-3);
    color: var(--text-secondary);
    font-family: var(--font-display);
    font-size: var(--text-sm);
    cursor: pointer;
  }

  .config-advanced-toggle:hover {
    color: var(--primary);
  }

  .config-llm-hint {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    border-bottom: 1px solid var(--border-subtle);
    flex-wrap: wrap;
  }

  .provider-test {
    padding: var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
  }

  .provider-test-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .provider-test-row {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    padding: var(--space-2);
  }

  .provider-test-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .provider-test-alias {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-primary);
  }

  .provider-test-kind {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .provider-test-summary,
  .provider-test-detail,
  .provider-test-error,
  .provider-test-empty {
    margin: var(--space-1) 0 0;
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .restart-section,
  .danger-zone {
    padding: var(--space-4);
  }

  .danger-title {
    color: var(--error);
  }

  .danger-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    flex-wrap: wrap;
  }

  .danger-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .danger-info span {
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }

  .loading,
  .empty-state {
    padding: var(--space-5);
    text-align: center;
    color: var(--text-tertiary);
  }

  .message {
    padding: var(--space-3);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
  }

  .message-error {
    color: var(--error);
    background: var(--error-muted);
  }

  .message-success {
    color: var(--success);
    background: var(--success-muted);
  }

  @media (max-width: 640px) {
    .config-search-input-wrap {
      flex-direction: column;
      align-items: stretch;
    }

    .danger-row {
      flex-direction: column;
      align-items: stretch;
    }
  }
</style>
