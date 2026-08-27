<script lang="ts">
  import type { OnboardingFormState } from '../../lib/onboarding'
  import { t } from '../../i18n'
  import FormField from './FormField.svelte'

  interface Props {
    form: OnboardingFormState
    errors: string[]
    onBack: () => void
    onNext: () => void
    onSkip: () => void
  }
  let { form = $bindable(), errors, onBack, onNext, onSkip }: Props = $props()

  function handleTokenInput(value: string) {
    form.channels.telegram_bot_token = value
    if (value.trim() !== '') form.channels.keepTelegramToken = false
  }

  // Disabling the Telegram channel must also clear polling. The polling
  // checkbox is rendered conditionally on telegram_enabled, so without
  // this auto-clear a user who turned off Telegram (with polling=true
  // prefilled from disk) couldn't reach the polling checkbox to unset
  // it — and validateChannelsStep would then block save with
  // "polling requires the Telegram channel to be enabled".
  function handleTelegramEnabledChange(checked: boolean) {
    form.channels.telegram_enabled = checked
    if (!checked) {
      form.channels.telegram_polling_enabled = false
    }
  }
</script>

<section class="card">
  <div class="card-header">
    <span class="card-title">{$t.onboarding.channels.cardTitle}</span>
    <span class="card-meta">{$t.onboarding.channels.cardMeta}</span>
  </div>

  <fieldset class="onboarding-subsection">
    <legend>{$t.onboarding.channels.telegramHeading}</legend>
    <label class="onboarding-checkrow">
      <input
        type="checkbox"
        checked={form.channels.telegram_enabled}
        onchange={(e) => handleTelegramEnabledChange((e.currentTarget as HTMLInputElement).checked)}
      />
      <span>
        <strong>{$t.onboarding.channels.telegramEnableLabel}</strong>
        <em>{$t.onboarding.channels.telegramEnableHint}</em>
      </span>
    </label>
    {#if form.channels.telegram_enabled}
      <FormField
        label={$t.onboarding.channels.telegramTokenLabel}
        hint={form.channels.keepTelegramToken ? $t.onboarding.channels.telegramTokenHint : undefined}
      >
        <input
          type="password"
          value={form.channels.keepTelegramToken ? '' : form.channels.telegram_bot_token}
          oninput={(e) => handleTokenInput((e.currentTarget as HTMLInputElement).value)}
          autocomplete="new-password"
          placeholder={form.channels.keepTelegramToken ? $t.onboarding.channels.telegramTokenPlaceholderKeep : $t.onboarding.channels.telegramTokenPlaceholderNew}
        />
      </FormField>
      <label class="onboarding-checkrow">
        <input type="checkbox" bind:checked={form.channels.telegram_polling_enabled} />
        <span>
          <strong>{$t.onboarding.channels.telegramPollingLabel}</strong>
          <em>{$t.onboarding.channels.telegramPollingHint}</em>
        </span>
      </label>
    {/if}
  </fieldset>

  <fieldset class="onboarding-subsection">
    <legend>{$t.onboarding.channels.webhookHeading}</legend>
    <label class="onboarding-checkrow">
      <input type="checkbox" bind:checked={form.channels.webhook_enabled} />
      <span>
        <strong>{$t.onboarding.channels.webhookEnableLabel}</strong>
        <em>{$t.onboarding.channels.webhookEnableHint}</em>
      </span>
    </label>
  </fieldset>

  <p class="onboarding-hint">{$t.onboarding.channels.restartHint}</p>

  {#if errors.length > 0}
    <div class="onboarding-errors-inline" aria-live="polite">
      <ul>{#each errors as err}<li>{err}</li>{/each}</ul>
    </div>
  {/if}

  <div class="onboarding-actions">
    <button class="btn btn-ghost" type="button" onclick={onBack}>{$t.onboarding.channels.backButton}</button>
    <button class="btn btn-ghost" type="button" onclick={onSkip}>{$t.onboarding.channels.skipButton}</button>
    <button class="btn btn-primary" type="button" onclick={onNext}>{$t.onboarding.channels.nextButton}</button>
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
  .onboarding-hint {
    margin: var(--space-3) 0 0;
    padding: var(--space-2) var(--space-3);
    border: 1px dashed var(--border-soft);
    border-radius: 6px;
    color: var(--text-muted);
    font-size: 13px;
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
