<script lang="ts">
  import type { OnboardingFormState } from '../../lib/onboarding'
  import { t } from '../../i18n'

  interface Props {
    form: OnboardingFormState
    errors: string[]
    onBack: () => void
    onNext: () => void
    onSkip: () => void
  }
  let { form = $bindable(), errors, onBack, onNext, onSkip }: Props = $props()

  function handleApiKeyInput(value: string) {
    form.integrations.memory_embed_api_key = value
    if (value.trim() !== '') form.integrations.keepMemoryEmbedKey = false
  }

  function handleDimensionsInput(raw: string) {
    const trimmed = raw.trim()
    if (trimmed === '') {
      form.integrations.memory_embed_dimensions = null
      return
    }
    const n = Number(trimmed)
    form.integrations.memory_embed_dimensions = Number.isFinite(n) && n > 0 ? n : null
  }
</script>

<section class="card">
  <div class="card-header">
    <span class="card-title">{$t.onboarding.integrations.cardTitle}</span>
    <span class="card-meta">{$t.onboarding.integrations.cardMeta}</span>
  </div>

  <fieldset class="onboarding-subsection">
    <legend>{$t.onboarding.integrations.memoryHeading}</legend>
    <div class="onboarding-grid">
      <label class="onboarding-field">
        <span>{$t.onboarding.integrations.memoryProviderLabel} <em>{$t.onboarding.integrations.memoryProviderHint}</em></span>
        <input type="text" bind:value={form.integrations.memory_embed_provider} placeholder={$t.onboarding.integrations.memoryProviderPlaceholder} />
      </label>
      <label class="onboarding-field">
        <span>{$t.onboarding.integrations.memoryApiKeyLabel} {#if form.integrations.keepMemoryEmbedKey}<em>{$t.onboarding.integrations.memoryApiKeyHint}</em>{/if}</span>
        <input
          type="password"
          value={form.integrations.keepMemoryEmbedKey ? '' : form.integrations.memory_embed_api_key}
          oninput={(e) => handleApiKeyInput((e.currentTarget as HTMLInputElement).value)}
          autocomplete="new-password"
          placeholder={form.integrations.keepMemoryEmbedKey ? $t.onboarding.integrations.memoryApiKeyPlaceholderKeep : $t.onboarding.integrations.memoryApiKeyPlaceholderNew}
        />
      </label>
      <label class="onboarding-field">
        <span>{$t.onboarding.integrations.memoryModelLabel}</span>
        <input type="text" bind:value={form.integrations.memory_embed_model} placeholder={$t.onboarding.integrations.memoryModelPlaceholder} />
      </label>
      <label class="onboarding-field">
        <span>{$t.onboarding.integrations.memoryBaseUrlLabel}</span>
        <input type="url" bind:value={form.integrations.memory_embed_base_url} placeholder={$t.onboarding.integrations.memoryBaseUrlPlaceholder} />
      </label>
      <label class="onboarding-field">
        <span>{$t.onboarding.integrations.memoryDimensionsLabel} <em>{$t.onboarding.integrations.memoryDimensionsHint}</em></span>
        <input
          type="number"
          min="1"
          step="1"
          value={form.integrations.memory_embed_dimensions ?? ''}
          oninput={(e) => handleDimensionsInput((e.currentTarget as HTMLInputElement).value)}
        />
      </label>
    </div>
  </fieldset>

  {#if errors.length > 0}
    <div class="onboarding-errors-inline" aria-live="polite">
      <ul>{#each errors as err}<li>{err}</li>{/each}</ul>
    </div>
  {/if}

  <div class="onboarding-actions">
    <button class="btn btn-ghost" type="button" onclick={onBack}>{$t.onboarding.integrations.backButton}</button>
    <button class="btn btn-ghost" type="button" onclick={onSkip}>{$t.onboarding.integrations.skipButton}</button>
    <button class="btn btn-primary" type="button" onclick={onNext}>{$t.onboarding.integrations.nextButton}</button>
  </div>
</section>

<style>
  .onboarding-subsection {
    border: 1px solid var(--border-soft);
    border-radius: 8px;
    padding: var(--space-3) var(--space-4);
    margin-bottom: var(--space-4);
    background: var(--surface-1);
  }
  .onboarding-subsection legend {
    padding: 0 6px;
    color: var(--primary);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    font-size: 12px;
  }
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
