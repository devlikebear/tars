<script lang="ts">
  import {
    defaultBaseURLForKind,
    type OnboardingFormState,
    type WizardMode,
  } from '../../lib/onboarding'
  import type { SetupStatusResponse } from '../../lib/types'
  import { t } from '../../i18n'

  interface Props {
    form: OnboardingFormState
    reentry: boolean
    mode: WizardMode
    setupStatus: SetupStatusResponse | null
    saveError: string
    onBack: () => void
    onSave: (restart: boolean) => void
  }
  let {
    form,
    reentry,
    mode,
    setupStatus,
    saveError,
    onBack,
    onSave,
  }: Props = $props()

  function maskedKey(key: string): string {
    const trimmed = key.trim()
    if (trimmed.length === 0) return '(none)'
    if (trimmed.length <= 8) return '*'.repeat(trimmed.length)
    return trimmed.slice(0, 4) + '*'.repeat(trimmed.length - 8) + trimmed.slice(-4)
  }
</script>

<section class="card">
  <div class="card-header">
    <span class="card-title">{$t.onboarding.step3.cardTitle}</span>
    {#if setupStatus?.config_path}
      <span class="card-meta">{$t.onboarding.step3.saveLocation}: <code>{setupStatus.config_path}</code></span>
    {/if}
  </div>

  {#if saveError}
    <div class="onboarding-errors-inline"><strong>{$t.onboarding.errors.saveFailed}</strong>: {saveError}</div>
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

    {#if mode === 'full'}
      <div class="onboarding-review-section">
        <h3>{$t.onboarding.tools.cardTitle}</h3>
        <dl>
          <div><dt>web_search</dt><dd>{form.tools.web_search_enabled ? '✓' : '—'}</dd></div>
          <div><dt>web_fetch</dt><dd>{form.tools.web_fetch_enabled ? '✓' : '—'}</dd></div>
          <div><dt>high-risk user</dt><dd>{form.tools.allow_high_risk_user ? '✓' : '—'}</dd></div>
        </dl>
      </div>
      <div class="onboarding-review-section">
        <h3>{$t.onboarding.integrations.cardTitle}</h3>
        <dl>
          <div><dt>memory_embed.provider</dt><dd>{form.integrations.memory_embed_provider || $t.onboarding.step3.none}</dd></div>
          <div>
            <dt>memory_embed.api_key</dt>
            <dd>
              {#if form.integrations.keepMemoryEmbedKey}
                <code>{$t.onboarding.step3.apiKeyKept}</code>
              {:else}
                <code>{maskedKey(form.integrations.memory_embed_api_key)}</code>
              {/if}
            </dd>
          </div>
        </dl>
      </div>
      <div class="onboarding-review-section">
        <h3>{$t.onboarding.channels.cardTitle}</h3>
        <dl>
          <div><dt>telegram</dt><dd>{form.channels.telegram_enabled ? '✓' : '—'}</dd></div>
          <div><dt>telegram polling</dt><dd>{form.channels.telegram_polling_enabled ? '✓' : '—'}</dd></div>
          <div><dt>webhook</dt><dd>{form.channels.webhook_enabled ? '✓' : '—'}</dd></div>
        </dl>
      </div>
    {/if}
  </div>

  <div class="onboarding-actions">
    <button class="btn btn-ghost" type="button" onclick={onBack}>{$t.onboarding.step3.backButton}</button>
    {#if reentry}
      <button class="btn btn-ghost" type="button" onclick={() => onSave(false)}>{$t.onboarding.step3.saveOnlyButton}</button>
      <button class="btn btn-primary" type="button" onclick={() => onSave(true)}>{$t.onboarding.step3.saveAndRestartButton}</button>
    {:else}
      <button class="btn btn-primary" type="button" onclick={() => onSave(true)}>{$t.onboarding.step3.saveAndRestartButton}</button>
    {/if}
  </div>
</section>

<style>
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
  .onboarding-review dt { color: var(--text-muted); font-size: 13px; }
  .onboarding-review dd { margin: 0; color: var(--text-primary); font-size: 14px; }
  .onboarding-review code {
    background: var(--surface-2);
    padding: 2px 6px;
    border-radius: 4px;
    font-size: 12px;
  }
  .onboarding-review table { width: 100%; border-collapse: collapse; font-size: 14px; }
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
  .onboarding-errors-inline {
    margin-bottom: var(--space-3);
    color: var(--accent-error, #d36b6b);
    font-size: 13px;
  }
</style>
