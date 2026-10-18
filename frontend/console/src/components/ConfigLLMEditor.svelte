<script lang="ts">
  // Settings → LLM tab: row-based editing for llm_providers / llm_tiers /
  // llm_role_defaults instead of three raw JSON textareas. One instance
  // handles all three fields together because tiers reference provider
  // aliases and a provider rename must propagate to the tiers that used the
  // old alias — splitting this into three independent ConfigFieldRow
  // instances would mean threading that cross-field llmState back out through
  // Config.svelte for no benefit.
  //
  // Each section keeps its own "Edit as JSON" fallback (lib/llmConfigEditor.ts
  // does the heavy lifting either way), so a shape this UI doesn't expose
  // (a stray key, a future LLMTierBinding field) is never silently dropped.
  //
  // onProvidersChange/onTiersChange/onRoleDefaultsChange are called only
  // with a value that currently validates with zero errors for that
  // section — Config.svelte's dirtyFields only ever holds something this
  // module is prepared to round-trip back into rows.
  import { untrack } from 'svelte'
  import {
    buildLLMEditorValues,
    buildLLMProvidersValue,
    buildLLMRoleDefaultsValue,
    buildLLMTiersValue,
    createLLMProviderRow,
    createLLMRoleRow,
    llmEditorStateFromValues,
    llmKnownRoles,
    llmProviderKinds,
    llmProviderRowsFromValue,
    llmRoleRowsFromValue,
    llmTierNames,
    llmTierRowsFromValue,
    renameLLMProviderAlias,
    type LLMEditorState,
    type LLMProviderKind,
    type LLMProviderRow,
    type LLMRoleRow,
    type LLMTierRow,
  } from '../lib/llmConfigEditor'
  import { availableAuthModesForKind, defaultBaseURLForKind, suggestedAuthModeForKind, type AuthMode } from '../lib/onboarding'
  import { popularModelsForKind } from '../lib/llm-catalog'
  import { configValuesEqual } from '../lib/configStructured'

  export interface ConfigLLMEditorText {
    providersTitle: string
    providersHint: string
    addProvider: string
    removeProvider: string
    aliasLabel: string
    aliasPlaceholder: string
    kindLabel: string
    kindPlaceholder: string
    authModeLabel: string
    apiKeyLabel: string
    apiKeyKeepHint: string
    baseUrlLabel: string
    noProviders: string
    tiersTitle: string
    tiersHint: string
    tierProviderLabel: string
    tierProviderPlaceholder: string
    tierModelLabel: string
    tierReasoningLabel: string
    tierReasoningDefault: string
    tierMaxTokensLabel: string
    tierMaxTokensPlaceholder: string
    rolesTitle: string
    rolesHint: string
    addRole: string
    removeRole: string
    roleLabel: string
    rolePlaceholder: string
    roleTierLabel: string
    roleTierPlaceholder: string
    noRoles: string
    editAsJson: string
    editAsTable: string
    jsonInvalid: string
  }

  interface Props {
    providersValue: unknown
    tiersValue: unknown
    roleDefaultsValue: unknown
    onProvidersChange: (value: Record<string, unknown>) => void
    onTiersChange: (value: Record<string, unknown>) => void
    onRoleDefaultsChange: (value: Record<string, unknown>) => void
    text: ConfigLLMEditorText
  }

  let { providersValue, tiersValue, roleDefaultsValue, onProvidersChange, onTiersChange, onRoleDefaultsChange, text }: Props = $props()

  let llmState: LLMEditorState = $state(
    untrack(() => llmEditorStateFromValues(providersValue, tiersValue, roleDefaultsValue)),
  )

  // Two different baselines per section, not one:
  //   - lastSeenXProp: the exact prop value last observed, used only to
  //     detect an *external* change (Discard, a save-reload, the JSON
  //     fallback on a sibling field) that should resync rows from props.
  //   - lastXValue: what emitIfValid last considered "the current truth"
  //     for that section — the *rebuilt* shape (buildLLM*Value's output),
  //     not the raw prop. Providers' rebuilt shape omits api_key for a row
  //     with keepExistingApiKey still true (see lib/llmConfigEditor.ts), so
  //     comparing a fresh rebuild against the raw prop (which still carries
  //     a masked api_key key from GET /schema) would read as "changed" on
  //     every keystroke in an unrelated field via the shared `built`
  //     derived below — comparing against the rebuilt baseline instead
  //     keeps an untouched section looking untouched.
  // Both comparisons use configValuesEqual (key-order-insensitive), not
  // JSON.stringify: the rebuilt value and the server's can list the same
  // keys in a different order and still be the same value.
  let lastSeenProvidersProp: unknown = untrack(() => providersValue ?? {})
  let lastSeenTiersProp: unknown = untrack(() => tiersValue ?? {})
  let lastSeenRolesProp: unknown = untrack(() => roleDefaultsValue ?? {})
  let lastProvidersValue: unknown = untrack(() => buildLLMProvidersValue(llmState.providers).value)
  let lastTiersValue: unknown = untrack(() =>
    buildLLMTiersValue(llmState.tiers, new Set(llmState.providers.map((p) => p.alias.trim()))).value,
  )
  let lastRolesValue: unknown = untrack(() => buildLLMRoleDefaultsValue(llmState.roles).value)

  $effect(() => {
    if (!configValuesEqual(providersValue ?? {}, lastSeenProvidersProp)) {
      lastSeenProvidersProp = providersValue ?? {}
      llmState.providers = llmProviderRowsFromValue(providersValue)
      lastProvidersValue = buildLLMProvidersValue(llmState.providers).value
    }
  })
  $effect(() => {
    if (!configValuesEqual(tiersValue ?? {}, lastSeenTiersProp)) {
      lastSeenTiersProp = tiersValue ?? {}
      llmState.tiers = llmTierRowsFromValue(tiersValue)
      lastTiersValue = buildLLMTiersValue(llmState.tiers, new Set(llmState.providers.map((p) => p.alias.trim()))).value
    }
  })
  $effect(() => {
    if (!configValuesEqual(roleDefaultsValue ?? {}, lastSeenRolesProp)) {
      lastSeenRolesProp = roleDefaultsValue ?? {}
      llmState.roles = llmRoleRowsFromValue(roleDefaultsValue)
      lastRolesValue = buildLLMRoleDefaultsValue(llmState.roles).value
    }
  })

  let built = $derived(buildLLMEditorValues(llmState))

  // Only a section with zero errors right now is pushed up — an in-progress
  // edit with a validation error stays local until it is fixed, matching
  // ConfigControl's "do not emit on invalid input" behavior.
  function emitIfValid() {
    const result = buildLLMEditorValues(llmState)
    if (result.providers.errors.length === 0) {
      if (!configValuesEqual(result.providers.value, lastProvidersValue)) {
        lastProvidersValue = result.providers.value
        // The parent's prop echoes this exact value back on its next
        // render (Config.svelte's dirtyFields -> displayValues) — record
        // it as already-seen so that echo does not itself look like an
        // external change and trigger a resync.
        lastSeenProvidersProp = result.providers.value
        onProvidersChange(result.providers.value)
      }
    }
    if (result.tiers.errors.length === 0) {
      if (!configValuesEqual(result.tiers.value, lastTiersValue)) {
        lastTiersValue = result.tiers.value
        lastSeenTiersProp = result.tiers.value
        onTiersChange(result.tiers.value)
      }
    }
    if (result.roles.errors.length === 0) {
      if (!configValuesEqual(result.roles.value, lastRolesValue)) {
        lastRolesValue = result.roles.value
        lastSeenRolesProp = result.roles.value
        onRoleDefaultsChange(result.roles.value)
      }
    }
  }

  // -- Providers --------------------------------------------------------

  function addProviderRow() {
    llmState.providers = [...llmState.providers, createLLMProviderRow()]
  }

  function removeProviderRow(row: LLMProviderRow) {
    llmState.providers = llmState.providers.filter((r) => r.id !== row.id)
    const removedAlias = row.alias.trim()
    if (removedAlias !== '') {
      // Clear (rather than leave dangling) so a tier that pointed at the
      // removed provider surfaces as "provider is required", not a stale
      // reference to an alias that no longer exists anywhere in the UI.
      llmState.tiers = renameLLMProviderAlias(llmState.tiers, removedAlias, '')
    }
    emitIfValid()
  }

  function handleAliasBlur(row: LLMProviderRow) {
    const prev = row.previousAlias.trim()
    const next = row.alias.trim()
    if (prev !== next) {
      llmState.tiers = renameLLMProviderAlias(llmState.tiers, prev, next)
      row.previousAlias = next
    }
    emitIfValid()
  }

  function handleKindChange(row: LLMProviderRow, value: string) {
    const kind = value as LLMProviderKind | ''
    const previousKind = row.kind
    row.kind = kind
    const previousDefault = defaultBaseURLForKind(previousKind)
    const currentBaseURL = row.base_url.trim()
    if (currentBaseURL === '' || currentBaseURL === previousDefault) {
      row.base_url = defaultBaseURLForKind(kind)
    }
    const validModes = availableAuthModesForKind(kind)
    if (!validModes.includes(row.auth_mode)) {
      row.auth_mode = suggestedAuthModeForKind(kind)
    }
    emitIfValid()
  }

  function handleAuthModeChange(row: LLMProviderRow, value: string) {
    row.auth_mode = value as AuthMode
    emitIfValid()
  }

  // Mirrors onboarding.ts's OnboardingProvider.svelte handleApiKeyInput: a
  // loaded row's control shows a blank field over its masked value
  // (keepExistingApiKey, see lib/llmConfigEditor.ts) rather than ever
  // rendering or resubmitting the "****"-shaped text GET /schema returns.
  // Typing anything commits to the new value from then on.
  function handleApiKeyInput(row: LLMProviderRow, value: string) {
    row.api_key = value
    if (value.trim() !== '') row.keepExistingApiKey = false
  }

  // -- Tiers --------------------------------------------------------------

  let providerAliasOptions = $derived(
    [...new Set(llmState.providers.map((p) => p.alias.trim()).filter(Boolean))],
  )

  function handleTierChange(row: LLMTierRow, patch: Partial<LLMTierRow>) {
    llmState.tiers = llmState.tiers.map((r) => (r.name === row.name ? { ...r, ...patch } : r))
    emitIfValid()
  }

  // -- Role defaults --------------------------------------------------------

  function addRoleRow() {
    llmState.roles = [...llmState.roles, createLLMRoleRow()]
  }

  function removeRoleRow(row: LLMRoleRow) {
    llmState.roles = llmState.roles.filter((r) => r.id !== row.id)
    emitIfValid()
  }

  function handleRoleChange(row: LLMRoleRow, patch: Partial<LLMRoleRow>) {
    llmState.roles = llmState.roles.map((r) => (r.id === row.id ? { ...r, ...patch } : r))
    emitIfValid()
  }

  // -- JSON fallback (one toggle + textarea per section) --------------------

  let providersJsonMode = $state(false)
  let providersDraft = $state('')
  let providersJsonError = $state('')

  function toggleProvidersJson() {
    if (!providersJsonMode) providersDraft = JSON.stringify(built.providers.value, null, 2)
    providersJsonError = ''
    providersJsonMode = !providersJsonMode
  }

  function commitProvidersJson() {
    try {
      const parsed = providersDraft.trim() === '' ? {} : JSON.parse(providersDraft)
      llmState.providers = llmProviderRowsFromValue(parsed)
      providersJsonError = ''
      providersJsonMode = false
      emitIfValid()
    } catch {
      providersJsonError = text.jsonInvalid
    }
  }

  let tiersJsonMode = $state(false)
  let tiersDraft = $state('')
  let tiersJsonError = $state('')

  function toggleTiersJson() {
    if (!tiersJsonMode) tiersDraft = JSON.stringify(built.tiers.value, null, 2)
    tiersJsonError = ''
    tiersJsonMode = !tiersJsonMode
  }

  function commitTiersJson() {
    try {
      const parsed = tiersDraft.trim() === '' ? {} : JSON.parse(tiersDraft)
      llmState.tiers = llmTierRowsFromValue(parsed)
      tiersJsonError = ''
      tiersJsonMode = false
      emitIfValid()
    } catch {
      tiersJsonError = text.jsonInvalid
    }
  }

  let rolesJsonMode = $state(false)
  let rolesDraft = $state('')
  let rolesJsonError = $state('')

  function toggleRolesJson() {
    if (!rolesJsonMode) rolesDraft = JSON.stringify(built.roles.value, null, 2)
    rolesJsonError = ''
    rolesJsonMode = !rolesJsonMode
  }

  function commitRolesJson() {
    try {
      const parsed = rolesDraft.trim() === '' ? {} : JSON.parse(rolesDraft)
      llmState.roles = llmRoleRowsFromValue(parsed)
      rolesJsonError = ''
      rolesJsonMode = false
      emitIfValid()
    } catch {
      rolesJsonError = text.jsonInvalid
    }
  }
</script>

<div class="llm-editor">
  <section class="llm-section" id="config-field-llm_providers">
    <div class="llm-section-header">
      <div>
        <h4>{text.providersTitle}</h4>
        <p>{text.providersHint}</p>
      </div>
      <button type="button" class="btn btn-ghost btn-sm" onclick={toggleProvidersJson}>
        {providersJsonMode ? text.editAsTable : text.editAsJson}
      </button>
    </div>

    {#if providersJsonMode}
      <textarea
        class="config-control-input config-control-textarea llm-json"
        class:invalid={!!providersJsonError}
        bind:value={providersDraft}
        onblur={commitProvidersJson}
      ></textarea>
      {#if providersJsonError}<span class="llm-error">{providersJsonError}</span>{/if}
    {:else}
      {#if llmState.providers.length === 0}
        <p class="llm-empty">{text.noProviders}</p>
      {/if}
      {#each llmState.providers as row (row.id)}
        <div class="llm-row llm-provider-row">
          <label class="llm-field">
            <span>{text.aliasLabel}</span>
            <input
              type="text"
              class="config-control-input"
              placeholder={text.aliasPlaceholder}
              bind:value={row.alias}
              onblur={() => handleAliasBlur(row)}
            />
          </label>
          <label class="llm-field">
            <span>{text.kindLabel}</span>
            <select
              class="config-control-input config-control-select"
              value={row.kind}
              onchange={(e) => handleKindChange(row, (e.currentTarget as HTMLSelectElement).value)}
            >
              <option value="">{text.kindPlaceholder}</option>
              {#each llmProviderKinds as kind}
                <option value={kind}>{kind}</option>
              {/each}
            </select>
          </label>
          <label class="llm-field">
            <span>{text.authModeLabel}</span>
            <select
              class="config-control-input config-control-select"
              value={row.auth_mode}
              onchange={(e) => handleAuthModeChange(row, (e.currentTarget as HTMLSelectElement).value)}
            >
              {#each availableAuthModesForKind(row.kind) as mode}
                <option value={mode}>{mode}</option>
              {/each}
            </select>
          </label>
          {#if row.auth_mode === 'api-key'}
            <label class="llm-field">
              <span>{text.apiKeyLabel}{#if row.keepExistingApiKey}&nbsp;<em>{text.apiKeyKeepHint}</em>{/if}</span>
              <input
                type="password"
                class="config-control-input"
                autocomplete="off"
                placeholder={row.keepExistingApiKey ? text.apiKeyKeepHint : ''}
                value={row.keepExistingApiKey ? '' : row.api_key}
                oninput={(e) => handleApiKeyInput(row, (e.currentTarget as HTMLInputElement).value)}
                onblur={emitIfValid}
              />
            </label>
          {/if}
          <label class="llm-field">
            <span>{text.baseUrlLabel}</span>
            <input
              type="text"
              class="config-control-input"
              placeholder={defaultBaseURLForKind(row.kind)}
              bind:value={row.base_url}
              onblur={emitIfValid}
            />
          </label>
          <button type="button" class="btn btn-ghost btn-sm llm-remove" onclick={() => removeProviderRow(row)}>
            {text.removeProvider}
          </button>
        </div>
      {/each}
      <button type="button" class="btn btn-secondary btn-sm" onclick={addProviderRow}>{text.addProvider}</button>
      {#if built.providers.errors.length > 0}
        <ul class="llm-section-errors">
          {#each built.providers.errors as err}<li>{err}</li>{/each}
        </ul>
      {/if}
    {/if}
  </section>

  <section class="llm-section" id="config-field-llm_tiers">
    <div class="llm-section-header">
      <div>
        <h4>{text.tiersTitle}</h4>
        <p>{text.tiersHint}</p>
      </div>
      <button type="button" class="btn btn-ghost btn-sm" onclick={toggleTiersJson}>
        {tiersJsonMode ? text.editAsTable : text.editAsJson}
      </button>
    </div>

    {#if tiersJsonMode}
      <textarea
        class="config-control-input config-control-textarea llm-json"
        class:invalid={!!tiersJsonError}
        bind:value={tiersDraft}
        onblur={commitTiersJson}
      ></textarea>
      {#if tiersJsonError}<span class="llm-error">{tiersJsonError}</span>{/if}
    {:else}
      <datalist id="config-llm-model-suggestions">
        {#each llmState.providers as row}
          {#each popularModelsForKind(row.kind) as model}
            <option value={model}></option>
          {/each}
        {/each}
      </datalist>
      {#each llmState.tiers as row (row.name)}
        <div class="llm-row llm-tier-row">
          <span class="llm-tier-name">{row.name}</span>
          <label class="llm-field">
            <span>{text.tierProviderLabel}</span>
            {#if providerAliasOptions.length > 0}
              <select
                class="config-control-input config-control-select"
                value={row.provider}
                onchange={(e) => handleTierChange(row, { provider: (e.currentTarget as HTMLSelectElement).value })}
              >
                <option value="">{text.tierProviderPlaceholder}</option>
                {#each providerAliasOptions as alias}
                  <option value={alias}>{alias}</option>
                {/each}
              </select>
            {:else}
              <input
                type="text"
                class="config-control-input"
                placeholder={text.tierProviderPlaceholder}
                bind:value={row.provider}
                onblur={emitIfValid}
              />
            {/if}
          </label>
          <label class="llm-field">
            <span>{text.tierModelLabel}</span>
            <input
              type="text"
              class="config-control-input"
              list="config-llm-model-suggestions"
              autocomplete="off"
              bind:value={row.model}
              onblur={emitIfValid}
            />
          </label>
          <label class="llm-field">
            <span>{text.tierReasoningLabel}</span>
            <select
              class="config-control-input config-control-select"
              value={row.reasoning_effort}
              onchange={(e) => handleTierChange(row, { reasoning_effort: (e.currentTarget as HTMLSelectElement).value })}
            >
              <option value="">{text.tierReasoningDefault}</option>
              <option value="minimal">minimal</option>
              <option value="low">low</option>
              <option value="medium">medium</option>
              <option value="high">high</option>
            </select>
          </label>
          <label class="llm-field">
            <span>{text.tierMaxTokensLabel}</span>
            <input
              type="text"
              inputmode="numeric"
              class="config-control-input"
              placeholder={text.tierMaxTokensPlaceholder}
              bind:value={row.max_tokens}
              onblur={emitIfValid}
            />
          </label>
        </div>
      {/each}
      {#if built.tiers.errors.length > 0}
        <ul class="llm-section-errors">
          {#each built.tiers.errors as err}<li>{err}</li>{/each}
        </ul>
      {/if}
    {/if}
  </section>

  <section class="llm-section" id="config-field-llm_role_defaults">
    <div class="llm-section-header">
      <div>
        <h4>{text.rolesTitle}</h4>
        <p>{text.rolesHint}</p>
      </div>
      <button type="button" class="btn btn-ghost btn-sm" onclick={toggleRolesJson}>
        {rolesJsonMode ? text.editAsTable : text.editAsJson}
      </button>
    </div>

    {#if rolesJsonMode}
      <textarea
        class="config-control-input config-control-textarea llm-json"
        class:invalid={!!rolesJsonError}
        bind:value={rolesDraft}
        onblur={commitRolesJson}
      ></textarea>
      {#if rolesJsonError}<span class="llm-error">{rolesJsonError}</span>{/if}
    {:else}
      <datalist id="config-llm-role-suggestions">
        {#each llmKnownRoles as role}
          <option value={role}></option>
        {/each}
      </datalist>
      {#if llmState.roles.length === 0}
        <p class="llm-empty">{text.noRoles}</p>
      {/if}
      {#each llmState.roles as row (row.id)}
        <div class="llm-row llm-role-row">
          <label class="llm-field">
            <span>{text.roleLabel}</span>
            <input
              type="text"
              class="config-control-input"
              list="config-llm-role-suggestions"
              placeholder={text.rolePlaceholder}
              bind:value={row.role}
              onblur={emitIfValid}
            />
          </label>
          <label class="llm-field">
            <span>{text.roleTierLabel}</span>
            <select
              class="config-control-input config-control-select"
              value={row.tier}
              onchange={(e) => handleRoleChange(row, { tier: (e.currentTarget as HTMLSelectElement).value })}
            >
              <option value="">{text.roleTierPlaceholder}</option>
              {#each llmTierNames as tier}
                <option value={tier}>{tier}</option>
              {/each}
            </select>
          </label>
          <button type="button" class="btn btn-ghost btn-sm llm-remove" onclick={() => removeRoleRow(row)}>
            {text.removeRole}
          </button>
        </div>
      {/each}
      <button type="button" class="btn btn-secondary btn-sm" onclick={addRoleRow}>{text.addRole}</button>
      {#if built.roles.errors.length > 0}
        <ul class="llm-section-errors">
          {#each built.roles.errors as err}<li>{err}</li>{/each}
        </ul>
      {/if}
    {/if}
  </section>
</div>

<style>
  .llm-editor {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .llm-section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .llm-section-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .llm-section-header h4 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .llm-section-header p {
    margin: 2px 0 0;
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .llm-row {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: var(--space-2);
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
  }

  .llm-tier-name {
    align-self: center;
    min-width: 64px;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    text-transform: uppercase;
  }

  .llm-field {
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: var(--text-xs);
    min-width: 120px;
  }

  .llm-field span {
    color: var(--text-secondary);
  }

  .llm-remove {
    align-self: center;
  }

  .llm-section-errors {
    margin: 0;
    padding-left: 1.2em;
    color: var(--error);
    font-size: var(--text-xs);
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .llm-empty {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    margin: 0;
  }

  .llm-error {
    color: var(--error);
    font-size: var(--text-xs);
  }

  .llm-json {
    min-height: 160px;
  }
</style>
