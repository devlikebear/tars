<script lang="ts">
  import { getProviderModels } from '../../lib/api'
  import { SNAPSHOT_DATE, popularModelsForKind } from '../../lib/llm-catalog'
  import type { OnboardingFormState } from '../../lib/onboarding'
  import { t } from '../../i18n'
  import FormField from './FormField.svelte'

  interface Props {
    form: OnboardingFormState
    reentry: boolean
    allKnownAliases: string[]
    errors: string[]
    onBack: () => void
    onNext: () => void
  }
  let {
    form = $bindable(),
    reentry,
    allKnownAliases,
    errors,
    onBack,
    onNext,
  }: Props = $props()

  let liveModels = $state<string[]>([])
  let liveRefreshError = $state<string>('')
  let liveRefreshing = $state<boolean>(false)
  let modelSuggestions = $derived(
    liveModels.length > 0 ? liveModels : popularModelsForKind(form.provider.kind),
  )

  async function refreshLiveModels() {
    liveRefreshError = ''
    liveRefreshing = true
    try {
      const info = await getProviderModels()
      const models = Array.isArray(info?.models)
        ? info.models.filter((m) => typeof m === 'string' && m.trim() !== '')
        : []
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
</script>

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
    <div class="onboarding-errors-inline">
      <strong>{$t.onboarding.errors.refreshFailed}</strong>: {liveRefreshError}
    </div>
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
        <FormField label={$t.onboarding.step2.providerAliasLabel}>
          {#if allKnownAliases.length > 1}
            <select bind:value={form.tiers[tier].provider}>
              {#each allKnownAliases as alias}
                <option value={alias}>{alias}</option>
              {/each}
            </select>
          {:else}
            <input type="text" bind:value={form.tiers[tier].provider} />
          {/if}
        </FormField>
        <FormField label={$t.onboarding.step2.modelLabel}>
          <input
            type="text"
            bind:value={form.tiers[tier].model}
            placeholder={tier === 'light' ? 'e.g. gpt-5.4-mini' : 'e.g. gpt-5.4'}
            list="onboarding-model-suggestions"
            autocomplete="off"
          />
        </FormField>
        <FormField label={$t.onboarding.step2.reasoningLabel} hint={$t.onboarding.step2.reasoningHint}>
          <select bind:value={form.tiers[tier].reasoning_effort}>
            <option value="">{$t.onboarding.step2.reasoningDefault}</option>
            <option value="minimal">minimal</option>
            <option value="low">low</option>
            <option value="medium">medium</option>
            <option value="high">high</option>
          </select>
        </FormField>
      </fieldset>
    {/each}
  </div>

  {#if errors.length > 0}
    <div class="onboarding-errors-inline" aria-live="polite">
      <ul>{#each errors as err}<li>{err}</li>{/each}</ul>
    </div>
  {/if}

  <div class="onboarding-actions">
    <button class="btn btn-ghost" type="button" onclick={onBack}>{$t.onboarding.step2.backButton}</button>
    <button class="btn btn-primary" type="button" onclick={onNext}>{$t.onboarding.step2.nextButton}</button>
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
