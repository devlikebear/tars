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
    availableAuthModesForKind,
    buildConfigPayload,
    defaultBaseURLForKind,
    emptyOnboardingForm,
    formFromConfigValues,
    providerKinds,
    suggestedAuthModeForKind,
    validateForm,
    validateProviderStep,
    validateTiersStep,
    type AuthMode,
    type OnboardingFormState,
    type ProviderKind,
  } from '../lib/onboarding'
  import type { SetupStatusResponse } from '../lib/types'

  interface Props {
    onComplete?: () => void
    reentry?: boolean
  }
  let { onComplete, reentry = false }: Props = $props()

  type StepId = 'provider' | 'tiers' | 'review' | 'restarting' | 'saved'

  let step = $state<StepId>('provider')
  let form = $state<OnboardingFormState>(emptyOnboardingForm())
  let availableAuthModes = $derived(availableAuthModesForKind(form.provider.kind))
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
        form = formFromConfigValues(schema.values || {})
      } catch (err) {
        console.warn('reentry prefill failed', err)
      }
    }
  })

  const stepOrder: StepId[] = ['provider', 'tiers', 'review', 'restarting']
  const stepLabels: Record<StepId, string> = {
    provider: 'Provider',
    tiers: 'Tiers',
    review: 'Review',
    restarting: 'Restart',
    saved: 'Saved',
  }

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
    form.provider.kind = kind
    if (form.provider.base_url.trim() === '') {
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
  }

  function syncTiersToWizardAlias() {
    const alias = form.provider.alias.trim()
    if (alias === '') return
    for (const tier of ['heavy', 'standard', 'light'] as const) {
      if (form.tiers[tier].provider.trim() === '') {
        form.tiers[tier].provider = alias
      }
    }
  }

  function goToTiers() {
    stepErrors = validateProviderStep(form)
    if (stepErrors.length === 0) {
      syncTiersToWizardAlias()
      step = 'tiers'
    }
  }

  function goToReview() {
    stepErrors = validateTiersStep(form)
    if (stepErrors.length === 0) {
      step = 'review'
    }
  }

  function goBack(target: StepId) {
    stepErrors = []
    saveError = ''
    step = target
  }

  async function handleSave(restart: boolean) {
    const formErrors = validateForm(form)
    if (formErrors.length > 0) {
      stepErrors = formErrors
      return
    }
    stepErrors = []
    saveError = ''
    step = 'restarting'
    restartPhase = 'patching'
    try {
      await patchConfigValues(buildConfigPayload(form))
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
    <span class="onboarding-kicker">{reentry ? 'Reconfigure' : 'First-run setup'}</span>
    <h1>TARS 설정 마법사</h1>
    {#if reentry}
      <p>이미 설정된 값이 prefill되어 있습니다. 변경하지 않은 항목은 그대로 유지됩니다.</p>
    {:else}
      <p>최소 LLM provider 1개와 3개 tier(heavy/standard/light) 바인딩만 입력하면 콘솔이 활성화됩니다.</p>
    {/if}
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
      <strong>입력을 확인해주세요</strong>
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
        <span class="card-title">Step 1 · Provider 등록</span>
      </div>
      <div class="onboarding-grid">
        <label class="onboarding-field">
          <span>Provider 종류 <em>Kind</em></span>
          <select value={form.provider.kind} onchange={(e) => handleKindChange((e.currentTarget as HTMLSelectElement).value)}>
            <option value="">선택하세요…</option>
            {#each providerKinds as kind}
              <option value={kind}>{kind}</option>
            {/each}
          </select>
        </label>

        <label class="onboarding-field">
          <span>Alias <em>llm_providers의 키</em></span>
          <input type="text" bind:value={form.provider.alias} placeholder="openai, codex, kimi 등" />
        </label>

        <label class="onboarding-field">
          <span>인증 모드 <em>auth_mode</em></span>
          <select bind:value={form.provider.auth_mode} disabled={availableAuthModes.length <= 1 && form.provider.kind !== ''}>
            {#each availableAuthModes as mode}
              <option value={mode}>{mode}</option>
            {/each}
          </select>
        </label>

        {#if form.provider.auth_mode === 'api-key'}
          <label class="onboarding-field">
            <span>API Key {#if form.provider.keepExistingApiKey}<em>변경하지 않으면 기존 값 유지</em>{/if}</span>
            <input
              type="password"
              value={form.provider.keepExistingApiKey ? '' : form.provider.api_key}
              oninput={(e) => handleApiKeyInput((e.currentTarget as HTMLInputElement).value)}
              autocomplete="new-password"
              placeholder={form.provider.keepExistingApiKey ? '••••••• (현재 값 유지)' : 'sk-…'}
            />
          </label>
        {/if}

        <label class="onboarding-field">
          <span>Base URL <em>비우면 kind 기본값 사용</em></span>
          <input type="url" bind:value={form.provider.base_url} placeholder={defaultBaseURLForKind(form.provider.kind)} />
        </label>
      </div>

      {#if form.provider.auth_mode === 'oauth'}
        <p class="onboarding-hint">
          <strong>OAuth 인증</strong> · 환경변수 또는 OAuth 핸드셰이크에서 자동으로 토큰을 받습니다. 선택한 kind에 맞는 OAuth provider가 자동 적용됩니다 (별도 입력 불필요).
        </p>
      {:else if form.provider.auth_mode === 'cli'}
        <p class="onboarding-hint">
          <strong>CLI 인증</strong> · 로컬에 설치된 <code>claude-code</code> CLI를 통해 인증합니다. 추가 키 입력 없이도 동작합니다 (CLI 자체 로그인 상태가 사용됩니다).
        </p>
      {/if}

      <div class="onboarding-actions">
        <span class="onboarding-spacer"></span>
        <button class="btn btn-primary" type="button" onclick={goToTiers}>다음: Tier 설정 →</button>
      </div>
    </section>
  {:else if step === 'tiers'}
    <section class="card">
      <div class="card-header">
        <span class="card-title">Step 2 · Tier 바인딩</span>
        <span class="card-meta">heavy / standard / light 모두 모델을 지정해야 합니다.</span>
      </div>
      <div class="onboarding-tier-grid">
        {#each ['heavy', 'standard', 'light'] as const as tier}
          <fieldset class="onboarding-tier">
            <legend>{tier}</legend>
            <label class="onboarding-field">
              <span>Provider alias</span>
              <input type="text" bind:value={form.tiers[tier].provider} />
            </label>
            <label class="onboarding-field">
              <span>Model</span>
              <input type="text" bind:value={form.tiers[tier].model} placeholder={tier === 'light' ? 'e.g. gpt-5.4-mini' : 'e.g. gpt-5.4'} />
            </label>
            <label class="onboarding-field">
              <span>Reasoning effort <em>선택</em></span>
              <select bind:value={form.tiers[tier].reasoning_effort}>
                <option value="">기본값</option>
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
        <button class="btn btn-ghost" type="button" onclick={() => goBack('provider')}>← 이전</button>
        <button class="btn btn-primary" type="button" onclick={goToReview}>다음: 검토 →</button>
      </div>
    </section>
  {:else if step === 'review'}
    <section class="card">
      <div class="card-header">
        <span class="card-title">Step 3 · 검토 및 저장</span>
        {#if setupStatus?.config_path}
          <span class="card-meta">저장 위치: <code>{setupStatus.config_path}</code></span>
        {/if}
      </div>

      {#if saveError}
        <div class="onboarding-errors"><strong>저장 실패</strong><div>{saveError}</div></div>
      {/if}

      <div class="onboarding-review">
        <div class="onboarding-review-section">
          <h3>Provider</h3>
          <dl>
            <div><dt>Alias</dt><dd>{form.provider.alias}</dd></div>
            <div><dt>Kind</dt><dd>{form.provider.kind}</dd></div>
            <div><dt>Auth mode</dt><dd>{form.provider.auth_mode}</dd></div>
            {#if form.provider.auth_mode === 'api-key'}
              <div>
                <dt>API Key</dt>
                <dd>
                  {#if form.provider.keepExistingApiKey}
                    <code>(현재 값 유지)</code>
                  {:else}
                    <code>{maskedKey(form.provider.api_key)}</code>
                  {/if}
                </dd>
              </div>
            {/if}
            <div><dt>Base URL</dt><dd>{form.provider.base_url || defaultBaseURLForKind(form.provider.kind) || '(none)'}</dd></div>
          </dl>
        </div>

        <div class="onboarding-review-section">
          <h3>Tier 바인딩</h3>
          <table>
            <thead><tr><th>Tier</th><th>Provider</th><th>Model</th><th>Reasoning</th></tr></thead>
            <tbody>
              {#each ['heavy', 'standard', 'light'] as const as tier}
                <tr>
                  <td><strong>{tier}</strong></td>
                  <td>{form.tiers[tier].provider}</td>
                  <td>{form.tiers[tier].model}</td>
                  <td>{form.tiers[tier].reasoning_effort || '(default)'}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>

      <div class="onboarding-actions">
        <button class="btn btn-ghost" type="button" onclick={() => goBack('tiers')}>← 이전</button>
        {#if reentry}
          <button class="btn btn-ghost" type="button" onclick={() => handleSave(false)}>저장만 (재시작 없이)</button>
          <button class="btn btn-primary" type="button" onclick={() => handleSave(true)}>저장하고 재시작</button>
        {:else}
          <button class="btn btn-primary" type="button" onclick={() => handleSave(true)}>저장하고 재시작</button>
        {/if}
      </div>
    </section>
  {:else if step === 'saved'}
    <section class="card onboarding-restart">
      <h2>저장 완료</h2>
      <p>변경사항이 config 파일에 기록되었습니다. 적용하려면 서버를 재시작해주세요.</p>
      {#if saveError}
        <div class="onboarding-errors"><strong>재시작 실패</strong><div>{saveError}</div></div>
      {/if}
      <div class="onboarding-actions onboarding-saved-actions">
        <button class="btn btn-ghost" type="button" onclick={() => onComplete && onComplete()}>나중에 (콘솔로 이동)</button>
        <button class="btn btn-primary" type="button" onclick={handleManualRestart}>지금 재시작</button>
      </div>
    </section>
  {:else}
    <section class="card onboarding-restart">
      {#if restartPhase === 'patching'}
        <h2>설정 저장 중…</h2>
        <p>입력한 provider와 tier 바인딩을 config 파일에 기록하고 있습니다.</p>
      {:else if restartPhase === 'restarting'}
        <h2>서버 재시작 요청…</h2>
        <p>잠시 후 healthz를 확인합니다.</p>
      {:else if restartPhase === 'polling'}
        <h2>서버 응답 대기 중…</h2>
        <p>최대 30초까지 healthz를 폴링합니다. 정상 모드 진입이 확인되면 자동으로 콘솔 홈으로 이동합니다.</p>
        <div class="onboarding-spinner" aria-hidden="true"></div>
      {:else if restartPhase === 'ready'}
        <h2>완료</h2>
        <p>정상 모드로 진입했습니다. 잠시 후 콘솔 홈으로 이동합니다.</p>
      {:else if restartPhase === 'timeout'}
        <h2>응답 지연</h2>
        <p>30초 이내에 정상 모드 진입을 확인하지 못했습니다. 페이지를 새로고침해 상태를 다시 확인해주세요.</p>
        <button class="btn btn-primary" type="button" onclick={() => window.location.reload()}>새로고침</button>
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
  .onboarding-hint code {
    background: var(--surface-2);
    padding: 1px 5px;
    border-radius: 3px;
    font-size: 12px;
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
