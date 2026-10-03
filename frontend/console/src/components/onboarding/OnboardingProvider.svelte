<script lang="ts">
  import { onMount } from 'svelte'
  import { discoverSetupProviders } from '../../lib/api'
  import { autoApplyDiscovery, applyDiscoveredProvider, type SetupCandidate } from '../../lib/setupDiscovery'

  import {
    availableAuthModesForKind,
    defaultBaseURLForKind,
    emptyOnboardingForm,
    propagateAliasToTiers,
    providerFromConfigValues,
    providerKinds,
    resetTierModelsForKindChange,
    suggestedAuthModeForKind,
    type OnboardingFormState,
    type ProviderKind,
  } from '../../lib/onboarding'
  import { t } from '../../i18n'
  import FormField from './FormField.svelte'

  interface Props {
    candidates?: SetupCandidate[]
    form: OnboardingFormState
    reentry: boolean
    existingAliases: string[]
    configValues: Record<string, unknown>
    errors: string[]
    onNext: () => void
  }
  let {
    candidates = $bindable([]),
    form = $bindable(),
    reentry,
    existingAliases,
    configValues,
    errors,
    onNext,
  }: Props = $props()

  let scanning = $state(false)
  let discoveryFailed = $state(false)
  let automatic = $state(false)
  let scanGeneration = 0
  let readyCount = $derived(candidates.filter(candidate => candidate.ready).length)
  async function scan() {
    const generation = ++scanGeneration
    scanning = true
    discoveryFailed = false
    try {
      const result = await discoverSetupProviders()
      if (generation !== scanGeneration) return
      candidates = result.candidates
      automatic = autoApplyDiscovery(form, candidates, reentry)
    } catch {
      if (generation === scanGeneration) discoveryFailed = true
    } finally {
      if (generation === scanGeneration) scanning = false
    }
  }
  onMount(() => {
    void scan()
    return () => { scanGeneration++ }
  })
  function selectCandidate(candidate: SetupCandidate) {
    applyDiscoveredProvider(form, candidate)
    automatic = false
  }

  let availableAuthModes = $derived(availableAuthModesForKind(form.provider.kind))

  function handleApiKeyInput(value: string) {
    form.provider.api_key = value
    if (value.trim() !== '') {
      form.provider.keepExistingApiKey = false
    }
  }

  function handleKindChange(value: string) {
    const kind = value as ProviderKind | ''
    const previousKind = form.provider.kind
    form.provider.kind = kind
    const previousDefault = defaultBaseURLForKind(previousKind)
    const currentBaseURL = form.provider.base_url.trim()
    if (currentBaseURL === '' || currentBaseURL === previousDefault) {
      form.provider.base_url = defaultBaseURLForKind(kind)
    }
    const valid = availableAuthModesForKind(kind)
    if (!valid.includes(form.provider.auth_mode)) {
      form.provider.auth_mode = suggestedAuthModeForKind(kind)
    }
    if (form.provider.alias.trim() === '' && kind !== '') {
      form.provider.alias = kind
    }
    if (previousKind !== '' && previousKind !== kind) {
      resetTierModelsForKindChange(form)
    }
  }

  function handleSelectProviderForEdit(alias: string) {
    if (alias === '__new__') {
      form.provider = emptyOnboardingForm().provider
    } else {
      form.provider = providerFromConfigValues(configValues, alias)
    }
  }

  function handleNext() {
    propagateAliasToTiers(form, form.provider.previousAlias || '', form.provider.alias)
    onNext()
  }
</script>

<section class="card">
  <div class="card-header">
    <span class="card-title">{$t.onboarding.step1.cardTitle}</span>
  </div>
  <div class="setup-discovery" aria-live="polite">
    <div class="setup-discovery-heading">
      <strong>{$t.setupDiscovery.title}</strong>
      <button class="btn btn-ghost btn-sm" type="button" onclick={scan} disabled={scanning}>{$t.setupDiscovery.rescan}</button>
    </div>
    {#if scanning}<p>{$t.setupDiscovery.scanning}</p>{/if}
    {#if discoveryFailed}<p>{$t.setupDiscovery.failed}</p>{/if}
    {#if automatic}<p>{$t.setupDiscovery.automatic}</p>{:else if readyCount > 1}<p>{$t.setupDiscovery.choose}</p>{/if}
    {#each candidates as candidate}
      <div class="setup-discovery-candidate">
        <div><strong>{candidate.kind === 'claude-code-cli' ? 'Claude Code' : 'Codex'}</strong>
          <span>{candidate.ready ? $t.setupDiscovery.ready : !candidate.installed ? $t.setupDiscovery.missing : $t.setupDiscovery.login}</span>
          {#if candidate.version}<small>{candidate.version}</small>{/if}
        </div>
        {#if candidate.ready}
          <button class="btn btn-ghost btn-sm" type="button" onclick={() => selectCandidate(candidate)}>
            {form.provider.kind === candidate.kind ? $t.setupDiscovery.selected : $t.setupDiscovery.select}
          </button>
        {:else if !candidate.installed}
          <a class="btn btn-ghost btn-sm" href={candidate.kind === 'claude-code-cli' ? 'https://code.claude.com/docs/en/setup' : 'https://developers.openai.com/codex/quickstart'} target="_blank" rel="noopener noreferrer">{$t.setupDiscovery.install}</a>
        {:else}
          <p>{candidate.problem === 'auth_unknown' ? $t.setupDiscovery.unknown : candidate.kind === 'claude-code-cli' ? $t.setupDiscovery.claudeLogin : $t.setupDiscovery.codexLogin}</p>
          <code>{candidate.kind === 'claude-code-cli' ? 'claude auth login' : 'codex -c cli_auth_credentials_store="file" login'}</code>
          <a href={candidate.kind === 'claude-code-cli' ? 'https://code.claude.com/docs/en/authentication' : 'https://developers.openai.com/codex/auth'} target="_blank" rel="noopener noreferrer">{$t.setupDiscovery.signIn}</a>
        {/if}
      </div>
    {/each}
  </div>
  {#if reentry && existingAliases.length > 0}
    <div class="onboarding-provider-selector">
      <FormField label={$t.onboarding.step1.selectProviderLabel}>
        <select
          value={form.provider.alias}
          onchange={(e) => handleSelectProviderForEdit((e.currentTarget as HTMLSelectElement).value)}
        >
          {#each existingAliases as alias}
            <option value={alias}>{alias}</option>
          {/each}
          <option value="__new__">{$t.onboarding.step1.addNewProviderOption}</option>
        </select>
      </FormField>
    </div>
  {/if}
  <div class="onboarding-grid">
    <FormField label={$t.onboarding.step1.kindLabel} hint={$t.onboarding.step1.kindHint}>
      <select value={form.provider.kind} onchange={(e) => handleKindChange((e.currentTarget as HTMLSelectElement).value)}>
        <option value="">{$t.onboarding.step1.kindPlaceholder}</option>
        {#each providerKinds as kind}
          <option value={kind}>{kind}</option>
        {/each}
      </select>
    </FormField>

    <FormField label={$t.onboarding.step1.aliasLabel} hint={$t.onboarding.step1.aliasHint}>
      <input type="text" bind:value={form.provider.alias} placeholder={$t.onboarding.step1.aliasPlaceholder} />
    </FormField>

    <FormField label={$t.onboarding.step1.authModeLabel} hint={$t.onboarding.step1.authModeHint}>
      <select bind:value={form.provider.auth_mode} disabled={availableAuthModes.length <= 1 && form.provider.kind !== ''}>
        {#each availableAuthModes as mode}
          <option value={mode}>{mode}</option>
        {/each}
      </select>
    </FormField>

    {#if form.provider.auth_mode === 'api-key'}
      <FormField
        label={$t.onboarding.step1.apiKeyLabel}
        hint={form.provider.keepExistingApiKey ? $t.onboarding.step1.apiKeyKeepHint : undefined}
      >
        <input
          type="password"
          value={form.provider.keepExistingApiKey ? '' : form.provider.api_key}
          oninput={(e) => handleApiKeyInput((e.currentTarget as HTMLInputElement).value)}
          autocomplete="new-password"
          placeholder={form.provider.keepExistingApiKey ? $t.onboarding.step1.apiKeyPlaceholderKeep : $t.onboarding.step1.apiKeyPlaceholderNew}
        />
      </FormField>
    {/if}

    <FormField label={$t.onboarding.step1.baseUrlLabel} hint={$t.onboarding.step1.baseUrlHint}>
      <input type="url" bind:value={form.provider.base_url} placeholder={defaultBaseURLForKind(form.provider.kind)} />
    </FormField>
  </div>

  {#if form.provider.auth_mode === 'oauth'}
    <p class="onboarding-hint">
      <strong>{$t.onboarding.step1.hintOauthTitle}</strong> · {$t.onboarding.step1.hintOauthBody}
    </p>
  {:else if form.provider.auth_mode === 'cli'}
    <p class="onboarding-hint">
      <strong>{$t.onboarding.step1.hintCliTitle}</strong> · {$t.onboarding.step1.hintCliBody}
    </p>
  {/if}

  {#if errors.length > 0}
    <div class="onboarding-errors-inline" aria-live="polite">
      <ul>
        {#each errors as err}<li>{err}</li>{/each}
      </ul>
    </div>
  {/if}

  <div class="onboarding-actions">
    <span class="onboarding-spacer"></span>
    <button class="btn btn-primary" type="button" onclick={handleNext}>{$t.onboarding.step1.nextButton}</button>
  </div>
</section>

<style>
  .onboarding-errors-inline {
    margin-top: var(--space-3);
    color: var(--accent-error, #d36b6b);
    font-size: 13px;
  }
  .onboarding-errors-inline ul {
    margin: 0;
    padding-left: 1.2em;
  }

  .setup-discovery { margin-bottom: 1.5rem; padding: 1rem; border: 1px solid var(--border, #333); border-radius: 8px; }
  .setup-discovery-heading, .setup-discovery-candidate { display: flex; align-items: center; justify-content: space-between; gap: 1rem; flex-wrap: wrap; }
  .setup-discovery-candidate { padding-top: .75rem; }
  .setup-discovery-candidate span, .setup-discovery-candidate small { margin-left: .75rem; }
  .setup-discovery p { margin: .5rem 0; }
</style>
