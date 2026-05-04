<script lang="ts">
  import {
    formatPrivateHostAllowlistInput,
    parsePrivateHostAllowlistInput,
    type OnboardingFormState,
  } from '../../lib/onboarding'
  import { t } from '../../i18n'

  interface Props {
    form: OnboardingFormState
    errors: string[]
    onBack: () => void
    onNext: () => void
    onSkip: () => void
  }
  let { form = $bindable(), errors, onBack, onNext, onSkip }: Props = $props()

  // Local textarea string mirrored to the typed allowlist on input. This
  // avoids losing the user's in-progress newlines / formatting while they
  // type before parsing into the structured array.
  let allowlistInput = $state(formatPrivateHostAllowlistInput(form.tools.web_fetch_private_host_allowlist))

  function handleAllowlistInput(value: string) {
    allowlistInput = value
    form.tools.web_fetch_private_host_allowlist = parsePrivateHostAllowlistInput(value)
  }

  function handleApiKeyInput(value: string) {
    form.tools.web_search_api_key = value
    if (value.trim() !== '') form.tools.keepWebSearchKey = false
  }
</script>

<section class="card">
  <div class="card-header">
    <span class="card-title">{$t.onboarding.tools.cardTitle}</span>
    <span class="card-meta">{$t.onboarding.tools.cardMeta}</span>
  </div>

  <fieldset class="onboarding-subsection">
    <legend>{$t.onboarding.tools.webSearchHeading}</legend>
    <label class="onboarding-checkrow">
      <input type="checkbox" bind:checked={form.tools.web_search_enabled} />
      <span>
        <strong>{$t.onboarding.tools.webSearchEnableLabel}</strong>
        <em>{$t.onboarding.tools.webSearchEnableHint}</em>
      </span>
    </label>
    {#if form.tools.web_search_enabled}
      <div class="onboarding-grid">
        <label class="onboarding-field">
          <span>{$t.onboarding.tools.webSearchProviderLabel}</span>
          <input type="text" bind:value={form.tools.web_search_provider} placeholder={$t.onboarding.tools.webSearchProviderPlaceholder} />
        </label>
        <label class="onboarding-field">
          <span>{$t.onboarding.tools.webSearchApiKeyLabel} {#if form.tools.keepWebSearchKey}<em>{$t.onboarding.tools.webSearchApiKeyHint}</em>{/if}</span>
          <input
            type="password"
            value={form.tools.keepWebSearchKey ? '' : form.tools.web_search_api_key}
            oninput={(e) => handleApiKeyInput((e.currentTarget as HTMLInputElement).value)}
            autocomplete="new-password"
            placeholder={form.tools.keepWebSearchKey ? $t.onboarding.tools.webSearchApiKeyPlaceholderKeep : $t.onboarding.tools.webSearchApiKeyPlaceholderNew}
          />
        </label>
      </div>
    {/if}
  </fieldset>

  <fieldset class="onboarding-subsection">
    <legend>{$t.onboarding.tools.webFetchHeading}</legend>
    <label class="onboarding-checkrow">
      <input type="checkbox" bind:checked={form.tools.web_fetch_enabled} />
      <span>
        <strong>{$t.onboarding.tools.webFetchEnableLabel}</strong>
        <em>{$t.onboarding.tools.webFetchEnableHint}</em>
      </span>
    </label>
    <label class="onboarding-checkrow">
      <input type="checkbox" bind:checked={form.tools.web_fetch_allow_private_hosts} />
      <span>
        <strong>{$t.onboarding.tools.webFetchPrivateHostsLabel}</strong>
        <em>{$t.onboarding.tools.webFetchPrivateHostsHint}</em>
      </span>
    </label>
    <label class="onboarding-field">
      <span>{$t.onboarding.tools.webFetchAllowlistLabel} <em>{$t.onboarding.tools.webFetchAllowlistHint}</em></span>
      <textarea
        rows="4"
        value={allowlistInput}
        oninput={(e) => handleAllowlistInput((e.currentTarget as HTMLTextAreaElement).value)}
        placeholder={$t.onboarding.tools.webFetchAllowlistPlaceholder}
      ></textarea>
    </label>
  </fieldset>

  <fieldset class="onboarding-subsection">
    <legend>{$t.onboarding.tools.permissionsHeading}</legend>
    <label class="onboarding-checkrow danger">
      <input type="checkbox" bind:checked={form.tools.allow_high_risk_user} />
      <span>
        <strong>{$t.onboarding.tools.highRiskUserLabel}</strong>
        <em class="warning">{$t.onboarding.tools.highRiskUserWarning}</em>
      </span>
    </label>
  </fieldset>

  {#if errors.length > 0}
    <div class="onboarding-errors-inline" aria-live="polite">
      <ul>{#each errors as err}<li>{err}</li>{/each}</ul>
    </div>
  {/if}

  <div class="onboarding-actions">
    <button class="btn btn-ghost" type="button" onclick={onBack}>{$t.onboarding.tools.backButton}</button>
    <button class="btn btn-ghost" type="button" onclick={onSkip}>{$t.onboarding.tools.skipButton}</button>
    <button class="btn btn-primary" type="button" onclick={onNext}>{$t.onboarding.tools.nextButton}</button>
  </div>
</section>

<style>
  .onboarding-subsection {
    border: 1px solid var(--border-soft);
    border-radius: 8px;
    padding: var(--space-3) var(--space-4);
    margin-bottom: var(--space-4);
    background: var(--surface-1);
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  .onboarding-subsection legend {
    padding: 0 6px;
    color: var(--primary);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    font-size: 12px;
  }
  .onboarding-checkrow {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
    cursor: pointer;
    font-size: 14px;
    color: var(--text-primary);
  }
  .onboarding-checkrow input[type='checkbox'] {
    margin-top: 4px;
  }
  .onboarding-checkrow span {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .onboarding-checkrow em {
    color: var(--text-muted);
    font-style: normal;
    font-size: 13px;
  }
  .onboarding-checkrow em.warning {
    color: var(--accent-error, #d36b6b);
  }
  .onboarding-checkrow.danger strong {
    color: var(--accent-error, #d36b6b);
  }
  .onboarding-field textarea {
    padding: 8px 10px;
    border: 1px solid var(--border-soft);
    border-radius: 6px;
    background: var(--surface-1);
    color: var(--text-primary);
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 13px;
    line-height: 1.5;
    resize: vertical;
  }
  .onboarding-field textarea:focus {
    outline: 2px solid var(--primary);
    outline-offset: 1px;
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
