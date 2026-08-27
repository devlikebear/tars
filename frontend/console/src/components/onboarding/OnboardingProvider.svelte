<script lang="ts">
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
    form: OnboardingFormState
    reentry: boolean
    existingAliases: string[]
    configValues: Record<string, unknown>
    errors: string[]
    onNext: () => void
  }
  let {
    form = $bindable(),
    reentry,
    existingAliases,
    configValues,
    errors,
    onNext,
  }: Props = $props()

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
</style>
