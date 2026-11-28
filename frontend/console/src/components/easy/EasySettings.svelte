<script lang="ts">
  // Vibe mode's settings (/console/easy/settings): the one screen a
  // vibe-mode person has for what the expert Settings screen holds. It
  // reuses that screen's parts and routes, not copies of them:
  //   - Your phone: Tailscale remote access in plain words (the same
  //     /v1/admin/remote-access routes as RemoteAccessCard) and the
  //     PhonesCard itself, in its vibe variant.
  //   - The AI: which tier the team's turns run on, written through the
  //     config API (PATCH /v1/admin/config/values) — llm_default_tier, or
  //     the chat role's tier when one is set (lib/easySettings.ts).
  //   - Expert mode: console_expert_mode, the same way.
  // A change that needs a restart offers one and never restarts by itself
  // (the Local embeddings panel's rule). Every route here is admin-only; a
  // console without admin gets a sentence saying so.
  import { onMount } from 'svelte'
  import { t } from '../../i18n'
  import {
    APIRequestError,
    disableRemoteAccess,
    enableRemoteAccess,
    getConfigSchema,
    getRemoteAccessStatus,
    patchConfigValues,
    restartServer,
  } from '../../lib/api'
  import { engineName } from '../../lib/easy'
  import { chatTier, phoneAccess, phoneAccessBlocker, restarted, thinkingChoices, thinkingUpdate } from '../../lib/easySettings'
  import { consoleMode } from '../../lib/stores/consoleMode.svelte'
  import type { ConfigSchema, RemoteAccessResponse } from '../../lib/types'
  import PhonesCard from '../PhonesCard.svelte'

  let { onNavigate }: { onNavigate: (path: string) => void } = $props()

  let phase = $state<'loading' | 'ready' | 'failed' | 'forbidden'>('loading')
  let schema = $state<ConfigSchema | null>(null)
  let remote = $state<RemoteAccessResponse | null>(null)
  let remoteBusy = $state(false)
  let remoteFailed = $state(false)
  // Bumped when phone access changes, so the phone card reads the address again.
  let phoneCardKey = $state(0)

  let picked = $state('')
  let aiBusy = $state(false)
  let aiMessage = $state<'' | 'saved' | 'saveFailed'>('')
  let expertBusy = $state(false)
  let expertMessage = $state<'' | 'saved' | 'saveFailed'>('')

  let confirming = $state(false)
  let restarting = $state(false)
  let restartFailed = $state(false)

  const values = $derived<Record<string, unknown>>(schema ? { ...schema.values, ...(schema.effective_values ?? {}) } : {})
  const choices = $derived(thinkingChoices(values))
  const current = $derived(chatTier(values))
  const currentChoice = $derived(choices.find((c) => c.tier === current))
  const access = $derived(phoneAccess(remote))
  const blocker = $derived(phoneAccessBlocker(remote))
  const pending = $derived((schema?.pending_restart_keys ?? []).length > 0)
  // What the file says now, so a saved but not yet applied change shows.
  const expertSaved = $derived(schema?.values?.console_expert_mode === true)

  function forbidden(err: unknown): boolean {
    return err instanceof APIRequestError && (err.status === 401 || err.status === 403)
  }

  async function readSchema() {
    schema = await getConfigSchema()
    if (!picked) picked = chatTier({ ...schema.values, ...(schema.effective_values ?? {}) })
  }

  async function readRemote() {
    try {
      remote = await getRemoteAccessStatus()
    } catch {
      remote = null
      remoteFailed = true
    }
  }

  async function load() {
    phase = 'loading'
    try {
      await readSchema()
    } catch (err) {
      phase = forbidden(err) ? 'forbidden' : 'failed'
      return
    }
    phase = 'ready'
    await readRemote()
  }

  async function checkAgain() {
    remoteFailed = false
    await readRemote()
    phoneCardKey++
  }

  async function setPhoneAccess(on: boolean) {
    if (remoteBusy) return
    remoteBusy = true
    remoteFailed = false
    try {
      remote = on ? await enableRemoteAccess() : await disableRemoteAccess()
    } catch (err) {
      // A refused turn-on carries the checks that failed; the blocker
      // sentence comes from them.
      if (err instanceof APIRequestError && Array.isArray(err.payload?.checks) && remote) {
        remote = { ...remote, checks: err.payload.checks as RemoteAccessResponse['checks'] }
      }
      if (!phoneAccessBlocker(remote)) remoteFailed = true
    } finally {
      remoteBusy = false
      phoneCardKey++
    }
  }

  async function saveThinking() {
    if (aiBusy || !picked) return
    aiBusy = true
    aiMessage = ''
    try {
      await patchConfigValues(thinkingUpdate(values, picked))
      await readSchema()
      aiMessage = 'saved'
    } catch {
      aiMessage = 'saveFailed'
    } finally {
      aiBusy = false
    }
  }

  async function setExpert(on: boolean) {
    if (expertBusy) return
    expertBusy = true
    expertMessage = ''
    try {
      await patchConfigValues({ console_expert_mode: on })
      await readSchema()
      expertMessage = 'saved'
    } catch {
      expertMessage = 'saveFailed'
    } finally {
      expertBusy = false
    }
  }

  async function restart() {
    if (!confirming) {
      confirming = true
      return
    }
    confirming = false
    restarting = true
    restartFailed = false
    const before = { runtime_started_at: schema?.runtime_started_at, process_id: schema?.process_id }
    try {
      await restartServer()
      for (let attempt = 0; attempt < 60; attempt++) {
        await new Promise((resolve) => setTimeout(resolve, 1000))
        try {
          const now = await getConfigSchema()
          if (restarted(before, now)) {
            // The whole console starts again: expert mode is read before
            // the first screen is drawn.
            window.location.reload()
            return
          }
        } catch {
          // The server is on its way back.
        }
      }
      restartFailed = true
    } catch {
      restartFailed = true
    } finally {
      restarting = false
    }
  }

  function go(event: MouseEvent, path: string) {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0) return
    event.preventDefault()
    onNavigate(path)
  }

  onMount(() => {
    void load()
  })
</script>

<div class="settings-page" data-testid="easy-settings" data-state={phase}>
  <header>
    <h1>{$t.easy.settings.title}</h1>
    <p class="muted">{$t.easy.settings.subtitle}</p>
  </header>

  {#if phase === 'loading'}
    <p class="muted">{$t.easy.settings.loading}</p>
  {:else if phase === 'forbidden'}
    <p class="note" role="status" data-testid="easy-settings-not-owner">{$t.easy.settings.notOwner}</p>
  {:else if phase === 'failed'}
    <p class="error" role="alert">{$t.easy.settings.loadFailed}</p>
    <div><button type="button" class="btn btn-secondary btn-sm" onclick={load}>{$t.easy.settings.retry}</button></div>
  {:else}
    {#if pending}
      <div class="restart" role="status" data-testid="easy-settings-restart">
        <span>{restarting ? $t.easy.settings.restart.restarting : restartFailed ? $t.easy.settings.restart.failed : $t.easy.settings.restart.needed}</span>
        <button type="button" class="btn btn-primary btn-sm" disabled={restarting} onclick={restart} data-testid="easy-settings-restart-button">
          {confirming ? $t.easy.settings.restart.confirm : $t.easy.settings.restart.button}
        </button>
      </div>
    {/if}

    <section class="block" data-testid="easy-settings-phone">
      <h2>{$t.easy.settings.phone.title}</h2>
      <p class="muted">{$t.easy.settings.phone.body}</p>
      <div class="access" data-testid="easy-settings-access" data-access={access}>
        <span class="dot {access === 'on' ? 'on' : access === 'checking' ? '' : 'warn'}" aria-hidden="true"></span>
        <div class="access-text">
          <span>
            {#if remoteFailed}
              {$t.easy.settings.phone.access.failed}
            {:else if (access === 'off' || access === 'renamed') && blocker}
              {$t.easy.settings.phone.access[blocker]}
            {:else}
              {$t.easy.settings.phone.access[access]}
            {/if}
          </span>
          {#if access === 'on' && remote?.url}
            <span class="muted small">{$t.easy.settings.phone.address(remote.url)}</span>
          {/if}
        </div>
      </div>
      <div class="actions">
        {#if access === 'noTailscale'}
          <a class="btn btn-secondary btn-sm" href="https://tailscale.com/download" target="_blank" rel="noreferrer">{$t.easy.settings.phone.getTailscale}</a>
        {/if}
        {#if access === 'off' && !blocker}
          <button type="button" class="btn btn-primary btn-sm" disabled={remoteBusy} onclick={() => setPhoneAccess(true)} data-testid="easy-settings-access-on">
            {remoteBusy ? $t.easy.settings.phone.working : $t.easy.settings.phone.turnOn}
          </button>
        {/if}
        {#if access === 'renamed' && !blocker}
          <button type="button" class="btn btn-primary btn-sm" disabled={remoteBusy} onclick={() => setPhoneAccess(true)} data-testid="easy-settings-access-fix">
            {remoteBusy ? $t.easy.settings.phone.working : $t.easy.settings.phone.fix}
          </button>
        {/if}
        {#if access === 'on' && remote?.desired_enabled}
          <button type="button" class="btn btn-ghost btn-sm" disabled={remoteBusy} onclick={() => setPhoneAccess(false)}>
            {remoteBusy ? $t.easy.settings.phone.working : $t.easy.settings.phone.turnOff}
          </button>
        {/if}
        {#if access !== 'checking'}
          <button type="button" class="btn btn-ghost btn-sm" disabled={remoteBusy} onclick={checkAgain}>{$t.easy.settings.phone.checkAgain}</button>
        {/if}
      </div>
      {#key phoneCardKey}
        <PhonesCard vibe />
      {/key}
    </section>

    <section class="block" data-testid="easy-settings-ai">
      <h2>{$t.easy.settings.ai.title}</h2>
      <p class="muted" data-testid="easy-settings-engine">
        {currentChoice ? $t.easy.settings.ai.engine(engineName(currentChoice.kind)) : $t.easy.settings.ai.engineUnknown}
      </p>
      {#if choices.length === 0}
        <p class="muted">{$t.easy.settings.ai.none}</p>
      {:else}
        <fieldset class="choices">
          <legend>{$t.easy.settings.ai.choose}</legend>
          {#each choices as choice (choice.tier)}
            <label class="choice" class:picked={picked === choice.tier} data-testid="easy-settings-choice" data-tier={choice.tier}>
              <input type="radio" name="thinking" value={choice.tier} bind:group={picked} />
              <span class="choice-text">
                <strong>{$t.easy.settings.ai.kinds[choice.tier] ?? choice.tier}</strong>
                {#if $t.easy.settings.ai.kindHints[choice.tier]}<span class="muted small">{$t.easy.settings.ai.kindHints[choice.tier]}</span>{/if}
                <span class="muted small mono">{$t.easy.settings.ai.model(choice.model)}{choice.kind && choice.kind !== currentChoice?.kind ? ` · ${engineName(choice.kind)}` : ''}</span>
              </span>
            </label>
          {/each}
        </fieldset>
        <div class="actions">
          <button
            type="button"
            class="btn btn-primary btn-sm"
            disabled={aiBusy || picked === current}
            onclick={saveThinking}
            data-testid="easy-settings-ai-save"
          >
            {aiBusy ? $t.easy.settings.ai.saving : $t.easy.settings.ai.save}
          </button>
          {#if aiMessage}
            <span class={aiMessage === 'saved' ? 'muted' : 'error'} role="status">{aiMessage === 'saved' ? $t.easy.settings.ai.saved : $t.easy.settings.ai.saveFailed}</span>
          {/if}
        </div>
      {/if}
      <p class="muted small">{$t.easy.settings.ai.more}</p>
    </section>

    <section class="block" data-testid="easy-settings-expert">
      <h2>{$t.easy.settings.expert.title}</h2>
      <p class="muted">{consoleMode.expert ? $t.easy.settings.expert.on : $t.easy.settings.expert.body}</p>
      <div class="actions">
        {#if consoleMode.expert}
          <a class="btn btn-secondary btn-sm" href="/console" onclick={(e) => go(e, '/console')}>{$t.easy.settings.expert.open}</a>
        {/if}
        {#if expertSaved}
          <button type="button" class="btn btn-ghost btn-sm" disabled={expertBusy} onclick={() => setExpert(false)} data-testid="easy-settings-expert-off">
            {$t.easy.settings.expert.turnOff}
          </button>
        {:else}
          <button type="button" class="btn btn-secondary btn-sm" disabled={expertBusy} onclick={() => setExpert(true)} data-testid="easy-settings-expert-on">
            {$t.easy.settings.expert.turnOn}
          </button>
        {/if}
        {#if expertMessage}
          <span class={expertMessage === 'saved' ? 'muted' : 'error'} role="status">
            {expertMessage === 'saved' ? $t.easy.settings.expert.saved : $t.easy.settings.ai.saveFailed}
          </span>
        {/if}
      </div>
    </section>
  {/if}
</div>

<style>
  .settings-page {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    max-width: 760px;
    margin: 0 auto;
    padding: var(--space-6) var(--space-6) var(--space-8);
    box-sizing: border-box;
  }
  h1 {
    margin: 0;
    font-size: var(--text-xl);
    font-weight: 600;
  }
  h2 {
    margin: 0;
    font-size: var(--text-lg);
    font-weight: 600;
  }
  header {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }
  p {
    margin: 0;
  }
  .muted {
    color: var(--text-secondary);
  }
  .small {
    font-size: var(--text-sm);
  }
  .mono {
    font-family: var(--font-mono);
  }
  .error {
    color: var(--error);
  }
  .note {
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    color: var(--text-secondary);
  }
  .block {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-5);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--surface);
  }
  .restart {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border: 1px solid color-mix(in srgb, var(--warning) 45%, var(--border-default));
    border-radius: var(--radius-md);
    background: var(--warning-muted);
    color: var(--text-primary);
  }
  .access {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
  }
  .access-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .dot {
    flex: none;
    width: 8px;
    height: 8px;
    margin-top: 7px;
    border-radius: 50%;
    background: var(--text-ghost);
  }
  .dot.on {
    background: var(--primary);
  }
  .dot.warn {
    background: var(--warning);
  }
  .actions {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .choices {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    border: 0;
  }
  legend {
    margin-bottom: var(--space-2);
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }
  .choice {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    cursor: pointer;
  }
  .choice.picked {
    border-color: var(--primary);
    background: rgba(var(--primary-rgb), 0.06);
  }
  .choice input {
    margin-top: 3px;
  }
  .choice-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .block :global(.phones-card) {
    margin-top: var(--space-2);
  }
  @media (max-width: 640px) {
    .settings-page {
      padding: var(--space-4);
    }
    .block {
      padding: var(--space-4);
    }
  }
</style>
