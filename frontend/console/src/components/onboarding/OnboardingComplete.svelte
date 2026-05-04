<script lang="ts">
  import {
    optionalSections,
    type OnboardingFormState,
    type OptionalSection,
    type WizardMode,
  } from '../../lib/onboarding'
  import type { SetupCapabilityStatus, SetupStatusResponse } from '../../lib/types'
  import { t } from '../../i18n'

  interface Props {
    form: OnboardingFormState
    mode: WizardMode
    setupStatus: SetupStatusResponse | null
    savedSections: Set<OptionalSection>
    saveError: string
    onConfigureMore: () => void
    onRestart: () => void
    onClose: () => void
    onJumpTo: (section: OptionalSection) => void
  }
  let {
    form,
    mode,
    setupStatus,
    savedSections,
    saveError,
    onConfigureMore,
    onRestart,
    onClose,
    onJumpTo,
  }: Props = $props()

  type RowStatus = 'ok' | 'missing' | 'skipped'
  type Row = {
    id: string
    label: string
    status: RowStatus
    section: OptionalSection | 'core'
  }

  // Capabilities derive from in-memory form state when present, with the
  // setup status flags as a fallback for refresh-after-save scenarios.
  // The form is the source of truth for what the user just configured;
  // setupStatus reflects on-disk-at-fetch state.
  function caps(): SetupCapabilityStatus {
    const fromStatus = setupStatus?.capabilities
    return {
      web_search_enabled: form.tools.web_search_enabled,
      web_search_api_key_set:
        form.tools.keepWebSearchKey || form.tools.web_search_api_key.trim() !== '' ||
        Boolean(fromStatus?.web_search_api_key_set),
      web_fetch_enabled: form.tools.web_fetch_enabled,
      memory_embed_api_key_set:
        form.integrations.keepMemoryEmbedKey ||
        form.integrations.memory_embed_api_key.trim() !== '' ||
        Boolean(fromStatus?.memory_embed_api_key_set),
      telegram_enabled: form.channels.telegram_enabled,
      telegram_bot_token_set:
        form.channels.keepTelegramToken ||
        form.channels.telegram_bot_token.trim() !== '' ||
        Boolean(fromStatus?.telegram_bot_token_set),
      webhook_enabled: form.channels.webhook_enabled,
      tools_configured: false,
      integrations_configured: false,
      channels_configured: false,
    }
  }

  let rows = $derived.by((): Row[] => {
    const c = caps()
    const visited = (s: OptionalSection) => savedSections.has(s)
    const allTiersBound = setupStatus
      ? Object.values(setupStatus.tiers).every((t) => t.configured)
      : (form.tiers.heavy.model.trim() !== '' &&
         form.tiers.standard.model.trim() !== '' &&
         form.tiers.light.model.trim() !== '')
    const providersOk = setupStatus
      ? !setupStatus.providers.missing
      : form.provider.alias.trim() !== ''

    const out: Row[] = [
      {
        id: 'provider',
        label: $t.onboarding.complete.rows.provider,
        status: providersOk ? 'ok' : 'missing',
        section: 'core',
      },
      {
        id: 'tiers',
        label: $t.onboarding.complete.rows.tiers,
        status: allTiersBound ? 'ok' : 'missing',
        section: 'core',
      },
      {
        id: 'webSearch',
        label: $t.onboarding.complete.rows.webSearch,
        status: c.web_search_enabled
          ? c.web_search_api_key_set ? 'ok' : 'missing'
          : visited('tools') ? 'skipped' : 'skipped',
        section: 'tools',
      },
      {
        id: 'webFetch',
        label: $t.onboarding.complete.rows.webFetch,
        status: c.web_fetch_enabled ? 'ok' : 'skipped',
        section: 'tools',
      },
      {
        id: 'memoryEmbed',
        label: $t.onboarding.complete.rows.memoryEmbed,
        status: c.memory_embed_api_key_set
          ? 'ok'
          : form.integrations.memory_embed_provider.trim() !== ''
            ? 'missing'
            : 'skipped',
        section: 'integrations',
      },
      {
        id: 'telegram',
        label: $t.onboarding.complete.rows.telegram,
        status: c.telegram_enabled
          ? c.telegram_bot_token_set ? 'ok' : 'missing'
          : 'skipped',
        section: 'channels',
      },
      {
        id: 'webhook',
        label: $t.onboarding.complete.rows.webhook,
        status: c.webhook_enabled ? 'ok' : 'skipped',
        section: 'channels',
      },
    ]
    return out
  })

  let workersChanged = $derived(
    savedSections.has('channels') &&
      (form.channels.telegram_enabled || form.channels.webhook_enabled),
  )

  function statusGlyph(status: RowStatus): string {
    if (status === 'ok') return '✓'
    if (status === 'missing') return '✗'
    return '—'
  }
</script>

<section class="card onboarding-complete">
  <h2>{$t.onboarding.complete.title}</h2>
  <p>{mode === 'quick' ? $t.onboarding.complete.bodyQuick : $t.onboarding.complete.bodyFull}</p>

  {#if saveError}
    <div class="onboarding-errors-inline"><strong>{$t.onboarding.errors.restartFailed}</strong>: {saveError}</div>
  {/if}

  <h3>{$t.onboarding.complete.matrixHeading}</h3>
  <table class="onboarding-matrix">
    <tbody>
      {#each rows as row}
        <tr class={`status-${row.status}`}>
          <td class="onboarding-matrix-glyph" aria-hidden="true">{statusGlyph(row.status)}</td>
          <td class="onboarding-matrix-label">{row.label}</td>
          <td class="onboarding-matrix-status">{$t.onboarding.complete.status[row.status]}</td>
          <td class="onboarding-matrix-action">
            {#if row.section !== 'core' && optionalSections.includes(row.section as OptionalSection)}
              <button class="btn btn-ghost btn-sm" type="button" onclick={() => onJumpTo(row.section as OptionalSection)}>
                {$t.onboarding.complete.jumpTo(row.label)}
              </button>
            {/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>

  {#if workersChanged}
    <p class="onboarding-restart-notice">{$t.onboarding.complete.restartRequiredNotice}</p>
  {/if}

  <div class="onboarding-actions onboarding-complete-actions">
    {#if mode === 'quick'}
      <button class="btn btn-ghost" type="button" onclick={onConfigureMore}>{$t.onboarding.complete.configureMoreButton}</button>
    {/if}
    <button class="btn btn-ghost" type="button" onclick={onClose}>{$t.onboarding.complete.backToConsoleButton}</button>
    <button class="btn btn-primary" type="button" onclick={onRestart}>{$t.onboarding.complete.restartNowButton}</button>
  </div>
</section>

<style>
  .onboarding-complete {
    padding: var(--space-5) var(--space-5) var(--space-4);
  }
  .onboarding-complete h2 {
    margin: 0 0 var(--space-2);
  }
  .onboarding-complete h3 {
    margin: var(--space-4) 0 var(--space-2);
    font-size: 14px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
  }
  .onboarding-matrix {
    width: 100%;
    border-collapse: collapse;
    font-size: 14px;
  }
  .onboarding-matrix tbody tr {
    border-bottom: 1px dashed var(--border-soft);
  }
  .onboarding-matrix tbody tr:last-child {
    border-bottom: none;
  }
  .onboarding-matrix td {
    padding: 8px 6px;
    vertical-align: middle;
  }
  .onboarding-matrix-glyph {
    width: 24px;
    text-align: center;
    font-weight: 700;
  }
  tr.status-ok .onboarding-matrix-glyph {
    color: var(--accent-success, #4caf80);
  }
  tr.status-missing .onboarding-matrix-glyph {
    color: var(--accent-error, #d36b6b);
  }
  tr.status-skipped .onboarding-matrix-glyph {
    color: var(--text-muted);
  }
  .onboarding-matrix-label {
    color: var(--text-primary);
  }
  .onboarding-matrix-status {
    color: var(--text-muted);
    font-size: 13px;
    width: 130px;
  }
  .onboarding-matrix-action {
    text-align: right;
    width: 1%;
    white-space: nowrap;
  }
  .onboarding-restart-notice {
    margin-top: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border: 1px dashed var(--primary);
    border-radius: 6px;
    color: var(--text-primary);
    font-size: 13px;
  }
  .onboarding-complete-actions {
    margin-top: var(--space-4);
    flex-wrap: wrap;
    justify-content: flex-end;
  }
  .onboarding-errors-inline {
    margin-bottom: var(--space-3);
    color: var(--accent-error, #d36b6b);
    font-size: 13px;
  }
</style>
