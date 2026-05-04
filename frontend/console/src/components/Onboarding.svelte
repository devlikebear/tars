<script lang="ts">
  import { onMount } from 'svelte'
  import {
    getConfigSchema,
    getHealthz,
    getProviderModels,
    getSetupStatus,
    patchConfigValues,
    restartServer,
  } from '../lib/api'
  import {
    SNAPSHOT_DATE,
    popularModelsForKind,
  } from '../lib/llm-catalog'
  import {
    allAliasesFromConfigValues,
    availableAuthModesForKind,
    buildConfigPayload,
    defaultBaseURLForKind,
    emptyOnboardingForm,
    formFromConfigValues,
    propagateAliasToTiers,
    providerFromConfigValues,
    providerKinds,
    resetTierModelsForKindChange,
    suggestedAuthModeForKind,
    validateForm,
    validateProviderStep,
    validateTiersStep,
    type AuthMode,
    type OnboardingFormState,
    type ProviderKind,
  } from '../lib/onboarding'
  import type { SetupStatusResponse } from '../lib/types'
  import { t } from '../i18n'

  interface Props {
    onComplete?: () => void
    reentry?: boolean
  }
  let { onComplete, reentry = false }: Props = $props()

  type StepId = 'provider' | 'tiers' | 'review' | 'restarting' | 'saved'

  let step = $state<StepId>('provider')
  let form = $state<OnboardingFormState>(emptyOnboardingForm())
  let availableAuthModes = $derived(availableAuthModesForKind(form.provider.kind))
  // All provider aliases loaded from config on re-entry. Used to populate
  // the provider selector in step 1 and the alias dropdown in step 2.
  let configValues = $state<Record<string, unknown>>({})
  let existingAliases = $derived(allAliasesFromConfigValues(configValues))
  // Union of existing aliases + the alias being configured in step 1.
  let allKnownAliases = $derived(
    [...new Set([...existingAliases, form.provider.alias.trim()].filter(Boolean))].sort()
  )
  // Live model list pulled from /v1/models when the user clicks Refresh
  // (reentry / normal mode only — the setup-only path has no on-disk
  // credentials yet so /v1/models has nothing to call).
  let liveModels = $state<string[]>([])
  let liveRefreshError = $state<string>('')
  let liveRefreshing = $state<boolean>(false)
  let modelSuggestions = $derived(
    liveModels.length > 0 ? liveModels : popularModelsForKind(form.provider.kind),
  )
  let setupStatus = $state<SetupStatusResponse | null>(null)
  let stepErrors = $state<string[]>([])
  let saveError = $state<string>('')
  let restartPhase = $state<'idle' | 'patching' | 'restarting' | 'polling' | 'ready' | 'timeout'>('idle')
  let pollDeadline = $state<number>(0)

  onMount(async () => {
    try {
      setupStatus = await getSetupStatus()
    } catch (err) {
      // status fetch is optional — wizard still works on empty fields
      console.warn('setup status fetch failed', err)
    }
    if (reentry) {
      try {
        const schema = await getConfigSchema()
        configValues = schema.values || {}
        form = formFromConfigValues(configValues)
      } catch (err) {
        console.warn('reentry prefill failed', err)
      }
    }
  })

  const stepOrder: StepId[] = ['provider', 'tiers', 'review', 'restarting']
  let stepLabels = $derived<Record<StepId, string>>({
    provider: $t.onboarding.steps.provider,
    tiers: $t.onboarding.steps.tiers,
    review: $t.onboarding.steps.review,
    restarting: $t.onboarding.steps.restart,
    saved: $t.onboarding.steps.saved,
  })

  function progressIndex(current: StepId): number {
    if (current === 'saved') return stepOrder.length - 1
    return stepOrder.indexOf(current)
  }

  function handleApiKeyInput(value: string) {
    form.provider.api_key = value
    if (value.trim() !== '') {
      // user typed a fresh value — drop the keep-existing pin so the
      // payload includes the new key
      form.provider.keepExistingApiKey = false
    }
  }

  function handleKindChange(value: string) {
    const kind = value as ProviderKind | ''
    const previousKind = form.provider.kind
    form.provider.kind = kind
    // Replace base_url when it's empty OR matches the previous kind's
    // canonical default. The user may have customized base_url; only
    // the boilerplate default gets swapped.
    const previousDefault = defaultBaseURLForKind(previousKind)
    const currentBaseURL = form.provider.base_url.trim()
    if (currentBaseURL === '' || currentBaseURL === previousDefault) {
      form.provider.base_url = defaultBaseURLForKind(kind)
    }
    // Reset auth_mode to a value valid for the picked kind. If the
    // user had selected api-key for an api-key kind and switches to
    // claude-code-cli (cli-only), keeping the stale value would let
    // them advance with an invalid combo.
    const valid = availableAuthModesForKind(kind)
    if (!valid.includes(form.provider.auth_mode)) {
      form.provider.auth_mode = suggestedAuthModeForKind(kind)
    }
    if (form.provider.alias.trim() === '' && kind !== '') {
      form.provider.alias = kind
    }
    // Models are kind-specific (gpt-5.4 makes no sense once kind flips
    // to anthropic). Clear any previous selection so step 2 prompts
    // the user with the new per-kind suggestion list.
    if (previousKind !== '' && previousKind !== kind) {
      resetTierModelsForKindChange(form)
    }
  }

  function syncTiersToWizardAlias() {
    propagateAliasToTiers(
      form,
      form.provider.previousAlias || '',
      form.provider.alias,
    )
  }

  function goToTiers() {
    stepErrors = validateProviderStep(form)
    if (stepErrors.length === 0) {
      syncTiersToWizardAlias()
      step = 'tiers'
    }
  }

  function handleSelectProviderForEdit(alias: string) {
    if (alias === '__new__') {
      form.provider = emptyOnboardingForm().provider
    } else {
      form.provider = providerFromConfigValues(configValues, alias)
    }
  }

  function goToReview() {
    stepErrors = validateTiersStep(form, allKnownAliases)
    if (stepErrors.length === 0) {
      step = 'review'
    }
  }

  function goBack(target: StepId) {
    stepErrors = []
    saveError = ''
    step = target
  }

  async function refreshLiveModels() {
    liveRefreshError = ''
    liveRefreshing = true
    try {
      const info = await getProviderModels()
      const models = Array.isArray(info?.models) ? info.models.filter((m) => typeof m === 'string' && m.trim() !== '') : []
      if (models.length === 0) {
        liveRefreshError = $t.onboarding.step2.refreshErrorEmpty
        return
      }
      liveModels = models
    } catch (err) {
      liveRefreshError = (err as Error).message || 'failed to fetch models'
    } finally {
      liveRefreshing = false
    }
  }

  async function handleSave(restart: boolean) {
    const formErrors = validateForm(form, allKnownAliases)
    if (formErrors.length > 0) {
      stepErrors = formErrors
      return
    }
    stepErrors = []
    saveError = ''
    step = 'restarting'
    restartPhase = 'patching'
    try {
      const existingProviders =
        (configValues.llm_providers as Record<string, unknown>) || {}
      await patchConfigValues(buildConfigPayload(form, existingProviders))
    } catch (err) {
      saveError = (err as Error).message || 'failed to save config'
      restartPhase = 'idle'
      step = 'review'
      return
    }
    if (!restart) {
      restartPhase = 'idle'
      step = 'saved'
      return
    }
    restartPhase = 'restarting'
    try {
      await restartServer()
    } catch (err) {
      saveError = (err as Error).message || 'failed to trigger restart'
      restartPhase = 'idle'
      step = 'review'
      return
    }
    restartPhase = 'polling'
    pollDeadline = Date.now() + 30_000
    pollUntilReady()
  }

  async function handleManualRestart() {
    saveError = ''
    step = 'restarting'
    restartPhase = 'restarting'
    try {
      await restartServer()
    } catch (err) {
      saveError = (err as Error).message || 'failed to trigger restart'
      restartPhase = 'idle'
      step = 'saved'
      return
    }
    restartPhase = 'polling'
    pollDeadline = Date.now() + 30_000
    pollUntilReady()
  }

  async function pollUntilReady() {
    while (Date.now() < pollDeadline) {
      await new Promise((resolve) => setTimeout(resolve, 1000))
      try {
        const health = await getHealthz()
        if (!health.needs_setup) {
          restartPhase = 'ready'
          if (onComplete) onComplete()
          return
        }
      } catch {
        // server is restarting — connection refused is expected
      }
    }
    restartPhase = 'timeout'
  }

  function maskedKey(key: string): string {
    const trimmed = key.trim()
    if (trimmed.length === 0) return '(none)'
    if (trimmed.length <= 8) return '*'.repeat(trimmed.length)
    return trimmed.slice(0, 4) + '*'.repeat(trimmed.length - 8) + trimmed.slice(-4)
  }
</script>

<section class="onboarding">
  <header class="onboarding-header">
    <span class="onboarding-kicker">{reentry ? $t.onboarding.kicker.reentry : $t.onboarding.kicker.firstRun}</span>
    <h1>{$t.onboarding.title}</h1>
    <p>{reentry ? $t.onboarding.subtitleReentry : $t.onboarding.subtitleFirstRun}</p>
  </header>

  <ol class="onboarding-progress">
    {#each stepOrder as stepKey, idx}
      <li class:active={step === stepKey} class:done={idx < progressIndex(step)}>
        <span class="onboarding-progress-dot">{idx + 1}</span>
        <span class="onboarding-progress-label">{stepLabels[stepKey]}</span>
      </li>
    {/each}
  </ol>

  {#if stepErrors.length > 0 && step !== 'restarting'}
    <div class="onboarding-errors">
      <strong>{$t.onboarding.errors.inputCheck}</strong>
      <ul>
        {#each stepErrors as err}
          <li>{err}</li>
        {/each}
      </ul>
    </div>
  {/if}

  {#if step === 'provider'}
    <section class="card">
      <div class="card-header">
        <span class="card-title">{$t.onboarding.step1.cardTitle}</span>
      </div>
      {#if reentry && existingAliases.length > 0}
        <div class="onboarding-provider-selector">
          <label class="onboarding-field">
            <span>{$t.onboarding.step1.selectProviderLabel}</span>
            <select
              value={form.provider.alias}
              onchange={(e) => handleSelectProviderForEdit((e.currentTarget as HTMLSelectElement).value)}
            >
              {#each existingAliases as alias}
                <option value={alias}>{alias}</option>
              {/each}
              <option value="__new__">{$t.onboarding.step1.addNewProviderOption}</option>
            </select>
          </label>
        </div>
      {/if}
      <div class="onboarding-grid">
        <label class="onboarding-field">
          <span>{$t.onboarding.step1.kindLabel} <em>{$t.onboarding.step1.kindHint}</em></span>
          <select value={form.provider.kind} onchange={(e) => handleKindChange((e.currentTarget as HTMLSelectElement).value)}>
            <option value="">{$t.onboarding.step1.kindPlaceholder}</option>
            {#each providerKinds as kind}
              <option value={kind}>{kind}</option>
            {/each}
          </select>
        </label>

        <label class="onboarding-field">
          <span>{$t.onboarding.step1.aliasLabel} <em>{$t.onboarding.step1.aliasHint}</em></span>
          <input type="text" bind:value={form.provider.alias} placeholder={$t.onboarding.step1.aliasPlaceholder} />
        </label>

        <label class="onboarding-field">
          <span>{$t.onboarding.step1.authModeLabel} <em>{$t.onboarding.step1.authModeHint}</em></span>
          <select bind:value={form.provider.auth_mode} disabled={availableAuthModes.length <= 1 && form.provider.kind !== ''}>
            {#each availableAuthModes as mode}
              <option value={mode}>{mode}</option>
            {/each}
          </select>
        </label>

        {#if form.provider.auth_mode === 'api-key'}
          <label class="onboarding-field">
            <span>{$t.onboarding.step1.apiKeyLabel} {#if form.provider.keepExistingApiKey}<em>{$t.onboarding.step1.apiKeyKeepHint}</em>{/if}</span>
            <input
              type="password"
              value={form.provider.keepExistingApiKey ? '' : form.provider.api_key}
              oninput={(e) => handleApiKeyInput((e.currentTarget as HTMLInputElement).value)}
              autocomplete="new-password"
              placeholder={form.provider.keepExistingApiKey ? $t.onboarding.step1.apiKeyPlaceholderKeep : $t.onboarding.step1.apiKeyPlaceholderNew}
            />
          </label>
        {/if}

        <label class="onboarding-field">
          <span>{$t.onboarding.step1.baseUrlLabel} <em>{$t.onboarding.step1.baseUrlHint}</em></span>
          <input type="url" bind:value={form.provider.base_url} placeholder={defaultBaseURLForKind(form.provider.kind)} />
        </label>
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

      <div class="onboarding-actions">
        <span class="onboarding-spacer"></span>
        <button class="btn btn-primary" type="button" onclick={goToTiers}>{$t.onboarding.step1.nextButton}</button>
      </div>
    </section>
  {:else if step === 'tiers'}
    <section class="card">
      <div class="card-header">
        <span class="card-title">{$t.onboarding.step2.cardTitle}</span>
        <span class="card-meta">{$t.onboarding.step2.cardMeta}</span>
      </div>

      <div class="onboarding-models-source">
        <div class="onboarding-models-source-text">
          {#if liveModels.length > 0}
            {$t.onboarding.step2.modelsSourceLive(liveModels.length)}
          {:else if modelSuggestions.length > 0}
            {$t.onboarding.step2.modelsSourceStatic(SNAPSHOT_DATE, modelSuggestions.length)}
          {:else}
            {$t.onboarding.step2.modelsSourceStaticEmpty(SNAPSHOT_DATE)}
          {/if}
        </div>
        {#if reentry}
          <button class="btn btn-ghost btn-sm" type="button" onclick={refreshLiveModels} disabled={liveRefreshing}>
            {liveRefreshing ? $t.onboarding.step2.refreshing : $t.onboarding.step2.refreshButton}
          </button>
        {/if}
      </div>
      {#if liveRefreshError}
        <div class="onboarding-errors"><strong>{$t.onboarding.errors.refreshFailed}</strong><div>{liveRefreshError}</div></div>
      {/if}

      <datalist id="onboarding-model-suggestions">
        {#each modelSuggestions as model}
          <option value={model}></option>
        {/each}
      </datalist>

      <div class="onboarding-tier-grid">
        {#each ['heavy', 'standard', 'light'] as const as tier}
          <fieldset class="onboarding-tier">
            <legend>{tier}</legend>
            <label class="onboarding-field">
              <span>{$t.onboarding.step2.providerAliasLabel}</span>
              {#if allKnownAliases.length > 1}
                <select bind:value={form.tiers[tier].provider}>
                  {#each allKnownAliases as alias}
                    <option value={alias}>{alias}</option>
                  {/each}
                </select>
              {:else}
                <input type="text" bind:value={form.tiers[tier].provider} />
              {/if}
            </label>
            <label class="onboarding-field">
              <span>{$t.onboarding.step2.modelLabel}</span>
              <input
                type="text"
                bind:value={form.tiers[tier].model}
                placeholder={tier === 'light' ? 'e.g. gpt-5.4-mini' : 'e.g. gpt-5.4'}
                list="onboarding-model-suggestions"
                autocomplete="off"
              />
            </label>
            <label class="onboarding-field">
              <span>{$t.onboarding.step2.reasoningLabel} <em>{$t.onboarding.step2.reasoningHint}</em></span>
              <select bind:value={form.tiers[tier].reasoning_effort}>
                <option value="">{$t.onboarding.step2.reasoningDefault}</option>
                <option value="minimal">minimal</option>
                <option value="low">low</option>
                <option value="medium">medium</option>
                <option value="high">high</option>
              </select>
            </label>
          </fieldset>
        {/each}
      </div>

      <div class="onboarding-actions">
        <button class="btn btn-ghost" type="button" onclick={() => goBack('provider')}>{$t.onboarding.step2.backButton}</button>
        <button class="btn btn-primary" type="button" onclick={goToReview}>{$t.onboarding.step2.nextButton}</button>
      </div>
    </section>
  {:else if step === 'review'}
    <section class="card">
      <div class="card-header">
        <span class="card-title">{$t.onboarding.step3.cardTitle}</span>
        {#if setupStatus?.config_path}
          <span class="card-meta">{$t.onboarding.step3.saveLocation}: <code>{setupStatus.config_path}</code></span>
        {/if}
      </div>

      {#if saveError}
        <div class="onboarding-errors"><strong>{$t.onboarding.errors.saveFailed}</strong><div>{saveError}</div></div>
      {/if}

      {#if reentry && form.provider.previousAlias && form.provider.previousAlias !== form.provider.alias.trim() && form.provider.alias.trim() !== ''}
        <p class="onboarding-hint">
          {$t.onboarding.step3.renameNotice(form.provider.previousAlias, form.provider.alias.trim())}
        </p>
      {/if}

      <div class="onboarding-review">
        <div class="onboarding-review-section">
          <h3>{$t.onboarding.step3.providerHeading}</h3>
          <dl>
            <div><dt>{$t.onboarding.step3.aliasField}</dt><dd>{form.provider.alias}</dd></div>
            <div><dt>{$t.onboarding.step3.kindField}</dt><dd>{form.provider.kind}</dd></div>
            <div><dt>{$t.onboarding.step3.authModeField}</dt><dd>{form.provider.auth_mode}</dd></div>
            {#if form.provider.auth_mode === 'api-key'}
              <div>
                <dt>{$t.onboarding.step3.apiKeyField}</dt>
                <dd>
                  {#if form.provider.keepExistingApiKey}
                    <code>{$t.onboarding.step3.apiKeyKept}</code>
                  {:else}
                    <code>{maskedKey(form.provider.api_key)}</code>
                  {/if}
                </dd>
              </div>
            {/if}
            <div><dt>{$t.onboarding.step3.baseUrlField}</dt><dd>{form.provider.base_url || defaultBaseURLForKind(form.provider.kind) || $t.onboarding.step3.none}</dd></div>
          </dl>
        </div>

        <div class="onboarding-review-section">
          <h3>{$t.onboarding.step3.tiersHeading}</h3>
          <table>
            <thead><tr><th>{$t.onboarding.step3.tierField}</th><th>{$t.onboarding.step3.providerField}</th><th>{$t.onboarding.step3.modelField}</th><th>{$t.onboarding.step3.reasoningField}</th></tr></thead>
            <tbody>
              {#each ['heavy', 'standard', 'light'] as const as tier}
                <tr>
                  <td><strong>{tier}</strong></td>
                  <td>{form.tiers[tier].provider}</td>
                  <td>{form.tiers[tier].model}</td>
                  <td>{form.tiers[tier].reasoning_effort || $t.onboarding.step3.defaultLabel}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>

      <div class="onboarding-actions">
        <button class="btn btn-ghost" type="button" onclick={() => goBack('tiers')}>{$t.onboarding.step3.backButton}</button>
        {#if reentry}
          <button class="btn btn-ghost" type="button" onclick={() => handleSave(false)}>{$t.onboarding.step3.saveOnlyButton}</button>
          <button class="btn btn-primary" type="button" onclick={() => handleSave(true)}>{$t.onboarding.step3.saveAndRestartButton}</button>
        {:else}
          <button class="btn btn-primary" type="button" onclick={() => handleSave(true)}>{$t.onboarding.step3.saveAndRestartButton}</button>
        {/if}
      </div>
    </section>
  {:else if step === 'saved'}
    <section class="card onboarding-restart">
      <h2>{$t.onboarding.saved.title}</h2>
      <p>{$t.onboarding.saved.body}</p>
      {#if saveError}
        <div class="onboarding-errors"><strong>{$t.onboarding.errors.restartFailed}</strong><div>{saveError}</div></div>
      {/if}
      <div class="onboarding-actions onboarding-saved-actions">
        <button class="btn btn-ghost" type="button" onclick={() => onComplete && onComplete()}>{$t.onboarding.saved.laterButton}</button>
        <button class="btn btn-primary" type="button" onclick={handleManualRestart}>{$t.onboarding.saved.restartNowButton}</button>
      </div>
    </section>
  {:else}
    <section class="card onboarding-restart">
      {#if restartPhase === 'patching'}
        <h2>{$t.onboarding.restart.patchingTitle}</h2>
        <p>{$t.onboarding.restart.patchingBody}</p>
      {:else if restartPhase === 'restarting'}
        <h2>{$t.onboarding.restart.restartingTitle}</h2>
        <p>{$t.onboarding.restart.restartingBody}</p>
      {:else if restartPhase === 'polling'}
        <h2>{$t.onboarding.restart.pollingTitle}</h2>
        <p>{$t.onboarding.restart.pollingBody}</p>
        <div class="onboarding-spinner" aria-hidden="true"></div>
      {:else if restartPhase === 'ready'}
        <h2>{$t.onboarding.restart.readyTitle}</h2>
        <p>{$t.onboarding.restart.readyBody}</p>
      {:else if restartPhase === 'timeout'}
        <h2>{$t.onboarding.restart.timeoutTitle}</h2>
        <p>{$t.onboarding.restart.timeoutBody}</p>
        <button class="btn btn-primary" type="button" onclick={() => window.location.reload()}>{$t.onboarding.restart.refreshButton}</button>
      {/if}
    </section>
  {/if}
</section>

<style>
  .onboarding {
    max-width: 760px;
    margin: 0 auto;
    padding: var(--space-6) var(--space-4);
    color: var(--text-primary);
  }
  .onboarding-header {
    margin-bottom: var(--space-5);
  }
  .onboarding-kicker {
    display: inline-block;
    padding: 2px 8px;
    background: var(--surface-2);
    border: 1px solid var(--border-soft);
    border-radius: 999px;
    font-size: 12px;
    color: var(--primary);
    letter-spacing: 0.05em;
    text-transform: uppercase;
  }
  .onboarding-header h1 {
    margin: var(--space-2) 0 var(--space-2);
    color: var(--text-primary);
  }
  .onboarding-header p {
    color: var(--text-muted);
    margin: 0;
  }
  .onboarding-progress {
    list-style: none;
    padding: 0;
    margin: 0 0 var(--space-5);
    display: flex;
    gap: var(--space-3);
    flex-wrap: wrap;
  }
  .onboarding-progress li {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-muted);
  }
  .onboarding-progress-dot {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: var(--surface-2);
    border: 1px solid var(--border-soft);
    color: var(--text-muted);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-weight: 600;
  }
  .onboarding-progress li.active .onboarding-progress-dot {
    background: var(--primary);
    color: var(--surface-base);
    border-color: var(--primary);
  }
  .onboarding-progress li.active .onboarding-progress-label {
    color: var(--text-primary);
    font-weight: 600;
  }
  .onboarding-progress li.done .onboarding-progress-dot {
    background: var(--surface-3);
    border-color: var(--primary);
    color: var(--primary);
  }
  .onboarding-errors {
    margin-bottom: var(--space-4);
    padding: var(--space-3) var(--space-4);
    border-radius: 8px;
    border: 1px solid var(--border-error, #6c2a2a);
    background: rgba(204, 64, 64, 0.08);
    color: var(--text-primary);
  }
  .onboarding-errors strong {
    display: block;
    margin-bottom: var(--space-2);
    color: var(--accent-error, #d36b6b);
  }
  .onboarding-errors ul {
    margin: 0;
    padding-left: 1.2em;
  }
  .onboarding-provider-selector {
    margin-bottom: var(--space-4);
    padding-bottom: var(--space-4);
    border-bottom: 1px solid var(--border-soft);
  }
  .onboarding-provider-selector .onboarding-field {
    max-width: 340px;
  }
  .onboarding-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-3) var(--space-4);
  }
  .onboarding-field {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    font-size: 14px;
  }
  .onboarding-field span {
    color: var(--text-muted);
    font-weight: 500;
  }
  .onboarding-field span em {
    color: var(--text-muted);
    font-weight: 400;
    font-style: normal;
    margin-left: 4px;
    font-size: 12px;
  }
  .onboarding-field input,
  .onboarding-field select {
    padding: 8px 10px;
    border: 1px solid var(--border-soft);
    border-radius: 6px;
    background: var(--surface-1);
    color: var(--text-primary);
    font-family: inherit;
    font-size: 14px;
  }
  .onboarding-field input:focus,
  .onboarding-field select:focus {
    outline: 2px solid var(--primary);
    outline-offset: 1px;
  }
  .onboarding-tier-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: var(--space-4);
  }
  .onboarding-tier {
    border: 1px solid var(--border-soft);
    border-radius: 8px;
    padding: var(--space-3);
    background: var(--surface-1);
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  .onboarding-tier legend {
    padding: 0 6px;
    color: var(--primary);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    font-size: 12px;
  }
  .onboarding-actions {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-3);
    margin-top: var(--space-4);
  }
  .onboarding-spacer { flex: 1; }
  .onboarding-models-source {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border: 1px dashed var(--border-soft);
    border-radius: 6px;
    background: var(--surface-1);
    font-size: 13px;
    color: var(--text-muted);
  }
  .onboarding-hint {
    margin: var(--space-4) 0 0;
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border-soft);
    border-left: 3px solid var(--primary);
    border-radius: 6px;
    background: var(--surface-1);
    color: var(--text-muted);
    font-size: 13px;
    line-height: 1.55;
  }
  .onboarding-hint strong {
    color: var(--text-primary);
    margin-right: 4px;
  }
  .onboarding-review {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }
  .onboarding-review-section h3 {
    margin: 0 0 var(--space-2);
    color: var(--text-primary);
  }
  .onboarding-review dl {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: var(--space-2) var(--space-4);
    margin: 0;
  }
  .onboarding-review dt {
    color: var(--text-muted);
    font-size: 13px;
  }
  .onboarding-review dd { margin: 0; color: var(--text-primary); font-size: 14px; }
  .onboarding-review code {
    background: var(--surface-2);
    padding: 2px 6px;
    border-radius: 4px;
    font-size: 12px;
  }
  .onboarding-review table {
    width: 100%;
    border-collapse: collapse;
    font-size: 14px;
  }
  .onboarding-review thead th {
    text-align: left;
    color: var(--text-muted);
    border-bottom: 1px solid var(--border-soft);
    padding: 6px 8px;
    font-weight: 500;
    font-size: 12px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .onboarding-review tbody td {
    padding: 8px;
    border-bottom: 1px dashed var(--border-soft);
  }
  .onboarding-restart {
    text-align: center;
    padding: var(--space-6);
  }
  .onboarding-saved-actions {
    justify-content: center;
    margin-top: var(--space-5);
  }
  .onboarding-spinner {
    width: 40px;
    height: 40px;
    border-radius: 50%;
    border: 3px solid var(--border-soft);
    border-top-color: var(--primary);
    margin: var(--space-4) auto 0;
    animation: onboarding-spin 1s linear infinite;
  }
  @keyframes onboarding-spin {
    to { transform: rotate(360deg); }
  }
</style>
