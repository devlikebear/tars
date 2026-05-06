<script lang="ts">
  import { onMount } from 'svelte'
  import {
    getConfigSchema,
    getHealthz,
    getSetupStatus,
    patchConfigValues,
    restartServer,
  } from '../lib/api'
  import {
    allAliasesFromConfigValues,
    buildConfigPayload,
    buildSectionPayload,
    emptyOnboardingForm,
    formFromConfigValues,
    optionalSections,
    validateForm,
    validateProviderStep,
    validateSectionStep,
    validateTiersStep,
    type OnboardingFormState,
    type OptionalSection,
    type WizardMode,
  } from '../lib/onboarding'
  import type { SetupStatusResponse } from '../lib/types'
  import { t } from '../i18n'

  import OnboardingProvider from './onboarding/OnboardingProvider.svelte'
  import OnboardingTiers from './onboarding/OnboardingTiers.svelte'
  import OnboardingTools from './onboarding/OnboardingTools.svelte'
  import OnboardingIntegrations from './onboarding/OnboardingIntegrations.svelte'
  import OnboardingChannels from './onboarding/OnboardingChannels.svelte'
  import OnboardingReview from './onboarding/OnboardingReview.svelte'
  import OnboardingComplete from './onboarding/OnboardingComplete.svelte'
  import OnboardingRestart from './onboarding/OnboardingRestart.svelte'
  import RemoteAccessCard from './RemoteAccessCard.svelte'

  interface Props {
    onComplete?: () => void
    reentry?: boolean
  }
  let { onComplete, reentry = false }: Props = $props()

  // Sections drive both the progress bar and the if/else dispatch in
  // the template. 'restarting' is a terminal phase rendered by
  // OnboardingRestart; 'complete' shows the capability matrix.
  type SectionId =
    | 'provider'
    | 'tiers'
    | 'tools'
    | 'integrations'
    | 'channels'
    | 'remote'
    | 'review'
    | 'restarting'
    | 'complete'

  // Quick mode = LLM-only (the original 4-step wizard); Full mode adds
  // the optional Tools/Integrations/Channels sections between Tiers and
  // Review. Reentry defaults to Full so users can deep-link any section.
  // mode starts as Quick; the deep-link handler in onMount switches it
  // to Full when reentry=true (or the URL pins a specific section). We
  // avoid initializing from `reentry` directly here to keep $state from
  // pinning to the initial prop value (Svelte 5 warns about that).
  let mode = $state<WizardMode>('quick')
  let step = $state<SectionId>('provider')
  let form = $state<OnboardingFormState>(emptyOnboardingForm())
  let configValues = $state<Record<string, unknown>>({})
  let existingAliases = $derived(allAliasesFromConfigValues(configValues))
  let allKnownAliases = $derived(
    [...new Set([...existingAliases, form.provider.alias.trim()].filter(Boolean))].sort(),
  )
  let setupStatus = $state<SetupStatusResponse | null>(null)
  let stepErrors = $state<string[]>([])
  let saveError = $state<string>('')
  let restartPhase = $state<'idle' | 'patching' | 'restarting' | 'polling' | 'ready' | 'timeout'>('idle')
  let pollDeadline = $state<number>(0)
  // Sections the user has saved (or skipped) in the current wizard run.
  // Drives the completion matrix's "configured vs not yet visited"
  // distinction and the restart-required hint when channels changed.
  let savedSections = $state<Set<OptionalSection>>(new Set())
  // Set when a section save mutated worker-backed channels (telegram/
  // webhook). The matrix surfaces this so the user knows a restart is
  // required to actually start the worker.
  let workerRestartPending = $state<boolean>(false)

  onMount(async () => {
    if (reentry) mode = 'full'
    try {
      setupStatus = await getSetupStatus()
    } catch (err) {
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
    // Honour ?section=<name> deep-link for reentry. Refuses to land on
    // a non-LLM section when no provider is configured (would write a
    // useless partial config) and falls back to provider step.
    if (typeof window !== 'undefined') {
      const params = new URLSearchParams(window.location.search)
      const target = params.get('section') as SectionId | null
      if (target && reentry) {
        const aliases = allAliasesFromConfigValues(configValues)
        const isOptional = (optionalSections as readonly string[]).includes(target)
        if (isOptional && aliases.length === 0) {
          step = 'provider'
        } else if (
          target === 'provider' ||
          target === 'tiers' ||
          target === 'tools' ||
          target === 'integrations' ||
          target === 'channels' ||
          target === 'remote' ||
          target === 'review'
        ) {
          mode = 'full'
          step = target
        }
      }
    }
  })

  // Derived ordering keeps the progress bar in sync with the active
  // mode. Quick excludes the optional sections; restarting/complete
  // are rendered separately and not tracked in stepOrder.
  let stepOrder = $derived<SectionId[]>(
    mode === 'quick'
      ? ['provider', 'tiers', 'remote', 'review']
      : ['provider', 'tiers', 'tools', 'integrations', 'channels', 'remote', 'review'],
  )

  let stepLabels = $derived<Record<SectionId, string>>({
    provider: $t.onboarding.steps.provider,
    tiers: $t.onboarding.steps.tiers,
    tools: $t.onboarding.steps.tools,
    integrations: $t.onboarding.steps.integrations,
    channels: $t.onboarding.steps.channels,
    remote: 'Remote',
    review: $t.onboarding.steps.review,
    restarting: $t.onboarding.steps.restart,
    complete: $t.onboarding.steps.saved,
  })

  function progressIndex(current: SectionId): number {
    if (current === 'complete' || current === 'restarting') {
      return stepOrder.length - 1
    }
    return stepOrder.indexOf(current)
  }

  function goToTiers() {
    stepErrors = validateProviderStep(form)
    if (stepErrors.length === 0) step = 'tiers'
  }

  function advanceFromTiers() {
    stepErrors = validateTiersStep(form, allKnownAliases)
    if (stepErrors.length > 0) return
    step = mode === 'full' ? 'tools' : 'remote'
  }

  // saveOptionalSection patches just the keys for a given section and
  // marks the section as visited. Errors surface inline; on success
  // the wizard moves to the next section in the active mode.
  async function saveOptionalSection(section: OptionalSection, next: SectionId) {
    const errs = validateSectionStep(section, form)
    if (errs.length > 0) {
      stepErrors = errs
      return
    }
    stepErrors = []
    saveError = ''
    try {
      await patchConfigValues(buildSectionPayload(section, form))
      savedSections.add(section)
      // Trigger reactivity for the Set — Svelte tracks the reference,
      // not internal mutations.
      savedSections = new Set(savedSections)
      if (
        section === 'channels' &&
        (form.channels.telegram_enabled || form.channels.webhook_enabled)
      ) {
        workerRestartPending = true
      }
    } catch (err) {
      stepErrors = [(err as Error).message || 'failed to save section']
      return
    }
    step = next
  }

  function skipOptionalSection(next: SectionId) {
    stepErrors = []
    step = next
  }

  function backToSection(target: SectionId) {
    stepErrors = []
    saveError = ''
    step = target
  }

  // handleSave commits the LLM provider+tier bindings via the existing
  // alias-replace payload (preserves other providers on disk). Optional
  // sections were already saved one-by-one if Full mode was used.
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
      step = 'complete'
      return
    }
    await triggerRestart('review')
  }

  async function triggerRestart(failbackStep: SectionId) {
    restartPhase = 'restarting'
    step = 'restarting'
    try {
      await restartServer()
    } catch (err) {
      saveError = (err as Error).message || 'failed to trigger restart'
      restartPhase = 'idle'
      step = failbackStep
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

  function configureMore() {
    // Switch from Quick → Full so the user can fill in Tools/Integrations/
    // Channels without re-running the LLM steps.
    mode = 'full'
    step = 'tools'
    stepErrors = []
  }

  function jumpToOptionalSection(section: OptionalSection) {
    mode = 'full'
    step = section
    stepErrors = []
  }

  function closeWizard() {
    if (onComplete) onComplete()
  }
</script>

<section class="onboarding">
  <header class="onboarding-header">
    <span class="onboarding-kicker">{reentry ? $t.onboarding.kicker.reentry : $t.onboarding.kicker.firstRun}</span>
    <h1>{$t.onboarding.title}</h1>
    <p>{reentry ? $t.onboarding.subtitleReentry : $t.onboarding.subtitleFirstRun}</p>
    <div class="onboarding-mode-row">
      <span class="onboarding-mode-badge">{mode === 'quick' ? $t.onboarding.mode.quick : $t.onboarding.mode.full}</span>
      <span class="onboarding-mode-hint">{mode === 'quick' ? $t.onboarding.mode.quickHint : $t.onboarding.mode.fullHint}</span>
    </div>
  </header>

  <ol class="onboarding-progress">
    {#each stepOrder as stepKey, idx}
      <li class:active={step === stepKey} class:done={idx < progressIndex(step)}>
        <span class="onboarding-progress-dot">{idx + 1}</span>
        <span class="onboarding-progress-label">{stepLabels[stepKey]}</span>
      </li>
    {/each}
  </ol>

  {#if stepErrors.length > 0 && step !== 'restarting' && step !== 'complete'}
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
    <OnboardingProvider
      bind:form
      {reentry}
      {existingAliases}
      {configValues}
      errors={stepErrors}
      onNext={goToTiers}
    />
  {:else if step === 'tiers'}
    <OnboardingTiers
      bind:form
      {reentry}
      {allKnownAliases}
      errors={stepErrors}
      onBack={() => backToSection('provider')}
      onNext={advanceFromTiers}
    />
  {:else if step === 'tools'}
    <OnboardingTools
      bind:form
      errors={stepErrors}
      onBack={() => backToSection('tiers')}
      onNext={() => saveOptionalSection('tools', 'integrations')}
      onSkip={() => skipOptionalSection('integrations')}
    />
  {:else if step === 'integrations'}
    <OnboardingIntegrations
      bind:form
      errors={stepErrors}
      onBack={() => backToSection('tools')}
      onNext={() => saveOptionalSection('integrations', 'channels')}
      onSkip={() => skipOptionalSection('channels')}
    />
  {:else if step === 'channels'}
    <OnboardingChannels
      bind:form
      errors={stepErrors}
      onBack={() => backToSection('integrations')}
      onNext={() => saveOptionalSection('channels', 'remote')}
      onSkip={() => skipOptionalSection('remote')}
    />
  {:else if step === 'remote'}
    <div class="onboarding-step">
      <RemoteAccessCard compact />
      <div class="onboarding-actions">
        <button type="button" class="btn btn-ghost" onclick={() => backToSection(mode === 'full' ? 'channels' : 'tiers')}>Back</button>
        <button type="button" class="btn btn-primary" onclick={() => backToSection('review')}>Continue</button>
      </div>
    </div>
  {:else if step === 'review'}
    <OnboardingReview
      {form}
      {reentry}
      {mode}
      {setupStatus}
      {saveError}
      onBack={() => backToSection('remote')}
      onSave={handleSave}
    />
  {:else if step === 'complete'}
    <OnboardingComplete
      {form}
      {mode}
      {setupStatus}
      {savedSections}
      {saveError}
      onConfigureMore={configureMore}
      onRestart={() => triggerRestart('complete')}
      onClose={closeWizard}
      onJumpTo={jumpToOptionalSection}
    />
    {#if workerRestartPending}
      <p class="onboarding-footnote">{$t.onboarding.complete.restartRequiredNotice}</p>
    {/if}
  {:else}
    <OnboardingRestart phase={restartPhase === 'idle' ? 'patching' : restartPhase} />
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
  .onboarding-mode-row {
    margin-top: var(--space-2);
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .onboarding-mode-badge {
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--surface-2);
    border: 1px solid var(--border-soft);
    color: var(--primary);
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  .onboarding-mode-hint {
    color: var(--text-muted);
    font-size: 12px;
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
  .onboarding-footnote {
    margin: var(--space-3) 0 0;
    color: var(--text-muted);
    font-size: 12px;
    text-align: center;
  }
  /* Shared field styles still used by extracted children. Kept here so
     scoped styles in child .svelte files can reference them via the
     same class names (Svelte 5 scoping is per-component but global
     CSS custom properties / tag-level rules cascade). */
  :global(.onboarding-grid) {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-3) var(--space-4);
  }
  :global(.onboarding-field) {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    font-size: 14px;
  }
  :global(.onboarding-field span) {
    color: var(--text-muted);
    font-weight: 500;
  }
  :global(.onboarding-field span em) {
    color: var(--text-muted);
    font-weight: 400;
    font-style: normal;
    margin-left: 4px;
    font-size: 12px;
  }
  :global(.onboarding-field input),
  :global(.onboarding-field select),
  :global(.onboarding-field textarea) {
    padding: 8px 10px;
    border: 1px solid var(--border-soft);
    border-radius: 6px;
    background: var(--surface-1);
    color: var(--text-primary);
    font-family: inherit;
    font-size: 14px;
  }
  :global(.onboarding-field input:focus),
  :global(.onboarding-field select:focus),
  :global(.onboarding-field textarea:focus) {
    outline: 2px solid var(--primary);
    outline-offset: 1px;
  }
  :global(.onboarding-tier-grid) {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: var(--space-4);
  }
  :global(.onboarding-tier) {
    border: 1px solid var(--border-soft);
    border-radius: 8px;
    padding: var(--space-3);
    background: var(--surface-1);
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  :global(.onboarding-tier legend) {
    padding: 0 6px;
    color: var(--primary);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    font-size: 12px;
  }
  :global(.onboarding-actions) {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-3);
    margin-top: var(--space-4);
  }
  :global(.onboarding-spacer) {
    flex: 1;
  }
  :global(.onboarding-models-source) {
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
  :global(.onboarding-provider-selector) {
    margin-bottom: var(--space-4);
    padding-bottom: var(--space-4);
    border-bottom: 1px solid var(--border-soft);
  }
  :global(.onboarding-provider-selector .onboarding-field) {
    max-width: 340px;
  }
</style>
