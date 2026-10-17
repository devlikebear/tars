<script lang="ts">
  import { onMount } from 'svelte'
  import { t } from '../i18n'
  import { APIRequestError, changeBrowserPassword, disableRemoteAccess, enableRemoteAccess, getRemoteAccessStatus } from '../lib/api'
  import type { RemoteAccessCheck, RemoteAccessResponse, RemoteAccessStatus } from '../lib/types'

  interface Props {
    compact?: boolean
    onChanged?: (status: RemoteAccessResponse) => void
  }

  let { compact = false, onChanged }: Props = $props()

  let loading = $state(true)
  let busy = $state(false)
  let error = $state('')
  let success = $state('')
  let data = $state<RemoteAccessResponse | null>(null)
  let httpsPort = $state(443)
  let adminPassword = $state('')
  let userPassword = $state('')

  let checks = $derived<RemoteAccessCheck[]>(data?.checks ?? [])
  let status = $derived<RemoteAccessStatus | null>(data?.status ?? null)
  let installed = $derived(Boolean(status?.installed ?? status?.Installed))
  let loggedIn = $derived(Boolean(status?.logged_in ?? status?.LoggedIn))
  let serving = $derived(Boolean(status?.serve_active ?? status?.ServeActive))
  let ownedByTARS = $derived(Boolean(status?.owned_by_tars ?? status?.OwnedByTARS))
  let hostName = $derived(String(status?.host_name ?? status?.HostName ?? ''))
  let tailnetURL = $derived(String(status?.tailnet_url ?? status?.TailnetURL ?? ''))
  let servePort = $derived(Number(status?.serve_port ?? status?.ServePort ?? data?.desired_https_port ?? httpsPort))
  let failedChecks = $derived(checks.filter((check) => !check.ok))
  let needsAdminPassword = $derived(failedChecks.some((check) => check.key === 'admin_password_configured'))
  let needsUserPassword = $derived(failedChecks.some((check) => check.key === 'user_password_configured'))

  function checkMessage(check: RemoteAccessCheck): string {
    return $t.remoteAccess.checks[check.key] ?? check.message
  }

  async function load() {
    loading = true
    error = ''
    try {
      const next = await getRemoteAccessStatus()
      applyData(next)
    } catch (err) {
      error = messageForError(err)
    } finally {
      loading = false
    }
  }

  function applyData(next: RemoteAccessResponse) {
    data = next
    httpsPort = next.desired_https_port || 443
    onChanged?.(next)
  }

  function messageForError(err: unknown): string {
    if (err instanceof APIRequestError) return err.message
    return err instanceof Error ? err.message : $t.remoteAccess.genericError
  }

  async function handleEnable() {
    busy = true
    error = ''
    success = ''
    try {
      const next = await enableRemoteAccess(httpsPort)
      applyData(next)
      success = $t.remoteAccess.enabled
    } catch (err) {
      error = messageForError(err)
      if (err instanceof APIRequestError && Array.isArray(err.payload?.checks)) {
        data = {
          ...(data ?? { desired_enabled: false, desired_https_port: httpsPort, target_url: '', status: {}, checks: [] }),
          checks: err.payload.checks as RemoteAccessCheck[],
        }
      }
    } finally {
      busy = false
    }
  }

  async function handleDisable() {
    busy = true
    error = ''
    success = ''
    try {
      const next = await disableRemoteAccess(httpsPort)
      applyData(next)
      success = $t.remoteAccess.disabled
    } catch (err) {
      error = messageForError(err)
    } finally {
      busy = false
    }
  }

  async function copyURL() {
    const url = data?.url?.trim()
    if (!url || typeof navigator === 'undefined') return
    await navigator.clipboard?.writeText(url)
    success = $t.remoteAccess.urlCopied
  }

  async function savePassword(role: 'admin' | 'user') {
    const password = role === 'admin' ? adminPassword : userPassword
    if (!password.trim() || busy) return
    busy = true
    error = ''
    success = ''
    try {
      await changeBrowserPassword(role, { new_password: password })
      if (role === 'admin') adminPassword = ''
      if (role === 'user') userPassword = ''
      success = $t.remoteAccess.passwordSaved(role)
      await load()
    } catch (err) {
      error = messageForError(err)
    } finally {
      busy = false
    }
  }

  onMount(() => { void load() })
</script>

<section class="remote-card" class:compact aria-label={$t.remoteAccess.ariaLabel}>
  <div class="remote-card-head">
    <div>
      <span class="remote-kicker">{$t.remoteAccess.kicker}</span>
      <h3>{$t.remoteAccess.title}</h3>
    </div>
    <span class="remote-status" class:on={serving && ownedByTARS} class:warn={serving && !ownedByTARS}>
      {#if loading}
        {$t.remoteAccess.status.checking}
      {:else if serving && ownedByTARS}
        {$t.remoteAccess.status.serving}
      {:else if serving}
        {$t.remoteAccess.status.portInUse}
      {:else if installed && loggedIn}
        {$t.remoteAccess.status.idle}
      {:else}
        {$t.remoteAccess.status.notReady}
      {/if}
    </span>
  </div>

  {#if loading}
    <div class="remote-muted">{$t.remoteAccess.checkingTailscale}</div>
  {:else}
    <div class="remote-facts">
      <span>{installed ? (loggedIn ? $t.remoteAccess.tailscaleLoggedIn : $t.remoteAccess.tailscaleLoggedOut) : $t.remoteAccess.tailscaleNotInstalled}</span>
      {#if hostName}<span>{$t.remoteAccess.device(hostName)}</span>{/if}
      {#if tailnetURL}<span>{$t.remoteAccess.url(tailnetURL)}</span>{/if}
      {#if serving}<span>{$t.remoteAccess.https(servePort)}</span>{/if}
    </div>

    {#if data?.url}
      <div class="remote-url-row">
        <code>{data.url}</code>
        <button type="button" class="btn btn-ghost btn-sm" onclick={copyURL}>{$t.remoteAccess.copy}</button>
      </div>
    {/if}

    <div class="remote-controls">
      <label>
        <span>{$t.remoteAccess.httpsPortLabel}</span>
        <input class="config-control-input" type="number" min="1" max="65535" bind:value={httpsPort} />
      </label>
      {#if data?.desired_enabled}
        <button type="button" class="btn btn-ghost btn-sm" disabled={busy} onclick={handleDisable}>
          {busy ? $t.remoteAccess.working : $t.remoteAccess.disable}
        </button>
      {:else}
        <button type="button" class="btn btn-primary btn-sm" disabled={busy} onclick={handleEnable}>
          {busy ? $t.remoteAccess.working : $t.remoteAccess.enable}
        </button>
      {/if}
      <button type="button" class="btn btn-ghost btn-sm" disabled={busy} onclick={load}>{$t.remoteAccess.refresh}</button>
    </div>

    {#if failedChecks.length > 0}
      <div class="remote-checks" role="status">
        {#each failedChecks as check}
          <div class="remote-check">
            <strong class="remote-check-code">{check.key.replaceAll('_', ' ')}</strong>
            <span>{checkMessage(check)}</span>
          </div>
        {/each}
      </div>
    {/if}

    {#if needsAdminPassword || needsUserPassword}
      <div class="remote-passwords">
        {#if needsAdminPassword}
          <label>
            <span>{$t.remoteAccess.adminPasswordLabel}</span>
            <input class="config-control-input" type="password" bind:value={adminPassword} autocomplete="new-password" />
            <button type="button" class="btn btn-ghost btn-sm" disabled={busy || !adminPassword.trim()} onclick={() => savePassword('admin')}>{$t.remoteAccess.save}</button>
          </label>
        {/if}
        {#if needsUserPassword}
          <label>
            <span>{$t.remoteAccess.userPasswordLabel}</span>
            <input class="config-control-input" type="password" bind:value={userPassword} autocomplete="new-password" />
            <button type="button" class="btn btn-ghost btn-sm" disabled={busy || !userPassword.trim()} onclick={() => savePassword('user')}>{$t.remoteAccess.save}</button>
          </label>
        {/if}
      </div>
    {/if}

    {#if error}
      <div class="remote-message error" role="alert">{error}</div>
    {:else if success}
      <div class="remote-message success">{success}</div>
    {/if}
  {/if}
</section>

<style>
  .remote-card {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--surface);
    padding: var(--space-5);
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .remote-card.compact {
    padding: var(--space-4);
  }

  .remote-card-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .remote-kicker {
    color: var(--primary);
    font-size: var(--text-xs);
    font-weight: 700;
    text-transform: uppercase;
  }

  h3 {
    margin: 2px 0 0;
    font-size: var(--text-lg);
    letter-spacing: 0;
  }

  .remote-status {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    padding: 4px 8px;
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    white-space: nowrap;
  }

  .remote-status.on {
    border-color: color-mix(in srgb, var(--success) 45%, var(--border-default));
    color: var(--success);
  }

  .remote-status.warn {
    border-color: color-mix(in srgb, var(--warning) 45%, var(--border-default));
    color: var(--warning);
  }

  .remote-muted,
  .remote-facts {
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .remote-facts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .remote-facts span {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    padding: 3px 8px;
    background: var(--surface-inset);
  }

  .remote-url-row,
  .remote-controls {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .remote-url-row code {
    min-width: 0;
    flex: 1;
    color: var(--text-primary);
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    padding: 7px 9px;
    overflow-wrap: anywhere;
  }

  label {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  /* .config-control-input (ConfigControl.svelte) gives the port/password
     inputs here the same height, font and border as every other settings
     control (#1242). */
  .remote-controls .config-control-input {
    width: 92px;
  }

  .remote-checks {
    display: grid;
    gap: var(--space-2);
  }

  .remote-passwords {
    display: grid;
    gap: var(--space-2);
  }

  .remote-passwords label {
    display: grid;
    grid-template-columns: minmax(110px, auto) minmax(0, 1fr) auto;
    align-items: center;
  }

  .remote-passwords .config-control-input {
    width: 100%;
  }

  .remote-check {
    display: flex;
    flex-direction: column;
    gap: 2px;
    border: 1px solid rgba(251, 191, 36, 0.28);
    border-radius: var(--radius-md);
    background: var(--warning-muted);
    padding: var(--space-3);
    font-size: var(--text-sm);
  }

  .remote-check strong {
    color: var(--warning);
    font-size: var(--text-xs);
    text-transform: uppercase;
  }

  .remote-message {
    border-radius: var(--radius-md);
    padding: var(--space-3);
    font-size: var(--text-sm);
  }

  .remote-message.error {
    color: var(--error);
    background: var(--error-muted);
    border: 1px solid rgba(248, 113, 113, 0.28);
  }

  .remote-message.success {
    color: var(--success);
    background: var(--success-muted);
    border: 1px solid rgba(74, 222, 128, 0.28);
  }

  @media (max-width: 640px) {
    .remote-card-head,
    .remote-controls,
    .remote-url-row {
      align-items: stretch;
      flex-direction: column;
    }

    .remote-controls .config-control-input {
      width: 100%;
    }

    .remote-passwords label {
      grid-template-columns: 1fr;
      align-items: stretch;
    }
  }
</style>
