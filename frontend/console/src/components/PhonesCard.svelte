<script lang="ts">
  // Paired phones (docs/decisions/mobile-remote.md §4): show a one-time
  // pairing QR code, list and revoke paired phones, and keep the folders a
  // phone may start a focus task in. The QR code carries this desktop's
  // Tailscale address, so pairing waits until remote access serves TARS.
  import { onDestroy, onMount } from 'svelte'
  import { t } from '../i18n'
  import {
    APIRequestError,
    createDevicePairingCode,
    getDeviceFolders,
    getRemoteAccessStatus,
    listPairedDevices,
    revokePairedDevice,
    setDeviceFolders,
    type DeviceFolder,
    type DevicePairingCode,
    type PairedDevice,
  } from '../lib/api'
  import {
    formatCountdown,
    newlyPaired,
    pairingBaseURL,
    pairingLink,
    qrPicture,
    secondsLeft,
    withFolder,
    withoutFolder,
  } from '../lib/devicePairing'
  import FolderPickerDialog from './FolderPickerDialog.svelte'

  let loading = $state(true)
  let busy = $state(false)
  let error = $state('')
  let baseURL = $state('')
  let devices = $state<PairedDevice[]>([])
  let folders = $state<DeviceFolder[]>([])
  let code = $state<DevicePairingCode | null>(null)
  let knownIDs: string[] = []
  let now = $state(Date.now())
  let picking = $state(false)
  let timer: ReturnType<typeof setInterval> | undefined
  let polls = 0

  let link = $derived(code ? pairingLink(baseURL, code.code) : '')
  let qr = $derived(link ? qrPicture(link) : null)
  let left = $derived(code ? secondsLeft(code.expires_at, now) : 0)

  function messageFor(err: unknown): string {
    if (err instanceof APIRequestError) return err.message
    return err instanceof Error ? err.message : $t.phones.genericError
  }

  async function load() {
    loading = true
    error = ''
    try {
      const [remote, list, dirs] = await Promise.all([getRemoteAccessStatus(), listPairedDevices(), getDeviceFolders()])
      const status = remote.status ?? {}
      baseURL = pairingBaseURL({
        url: remote.url,
        serving: Boolean(status.serve_active ?? status.ServeActive),
        ownedByTARS: Boolean(status.owned_by_tars ?? status.OwnedByTARS),
      })
      devices = list
      folders = dirs
    } catch (err) {
      error = messageFor(err)
    } finally {
      loading = false
    }
  }

  function stopTimer() {
    if (timer) clearInterval(timer)
    timer = undefined
  }

  // While a code is on screen, tick the countdown every second and look for
  // the phone that used it every third second.
  function startTimer() {
    stopTimer()
    polls = 0
    timer = setInterval(async () => {
      now = Date.now()
      if (!code || left <= 0) return
      if (++polls % 3 !== 0) return
      try {
        const list = await listPairedDevices()
        devices = list
        if (newlyPaired(knownIDs, list)) {
          code = null
          stopTimer()
        }
      } catch {
        // The next poll tries again; the code stays on screen.
      }
    }, 1000)
  }

  async function pair() {
    if (busy) return
    busy = true
    error = ''
    try {
      knownIDs = devices.map((d) => d.id)
      code = await createDevicePairingCode()
      now = Date.now()
      startTimer()
    } catch (err) {
      error = messageFor(err)
    } finally {
      busy = false
    }
  }

  async function revoke(device: PairedDevice) {
    if (busy || !confirm($t.phones.confirmRevoke(device.name))) return
    busy = true
    error = ''
    try {
      await revokePairedDevice(device.id)
      devices = await listPairedDevices()
    } catch (err) {
      error = messageFor(err)
    } finally {
      busy = false
    }
  }

  async function saveFolders(next: string[]) {
    busy = true
    error = ''
    try {
      folders = await setDeviceFolders(next)
    } catch (err) {
      error = messageFor(err)
    } finally {
      busy = false
    }
  }

  function addFolder(path: string) {
    picking = false
    const paths = folders.map((f) => f.path)
    const next = withFolder(paths, path)
    if (next !== paths) void saveFolders(next)
  }

  function removeFolder(path: string) {
    void saveFolders(withoutFolder(folders.map((f) => f.path), path))
  }

  function onKeydown(e: KeyboardEvent) {
    if (!picking || e.key !== 'Escape') return
    e.preventDefault()
    picking = false
  }

  function formatDate(value: string): string {
    const at = new Date(value)
    return Number.isNaN(at.getTime()) ? value : at.toLocaleDateString()
  }

  onMount(() => { void load() })
  onDestroy(stopTimer)
</script>

<svelte:window onkeydown={onKeydown} />

<section class="phones-card" aria-label={$t.phones.ariaLabel} data-testid="phones-card">
  <div class="phones-head">
    <div>
      <span class="phones-kicker">{$t.phones.kicker}</span>
      <h3>{$t.phones.title}</h3>
    </div>
    <button
      type="button"
      class="btn btn-primary btn-sm"
      disabled={loading || busy || !baseURL}
      onclick={pair}
      data-testid="phones-pair"
    >
      {busy ? $t.phones.working : code ? $t.phones.newCode : $t.phones.pair}
    </button>
  </div>

  <p class="phones-muted">{$t.phones.intro}</p>

  {#if !loading && !baseURL}
    <div class="phones-note" role="status">{$t.phones.needsAddress}</div>
  {/if}

  {#if code && qr}
    <div class="phones-pairing" data-testid="phones-pairing">
      <svg
        class="phones-qr"
        viewBox="0 0 {qr.size} {qr.size}"
        role="img"
        aria-label={$t.phones.qrLabel}
        shape-rendering="crispEdges"
      >
        <rect width={qr.size} height={qr.size} fill="#fff" />
        <path d={qr.path} fill="#000" />
      </svg>
      <div class="phones-pairing-text">
        <span class="phones-muted">{$t.phones.codeLabel}</span>
        <code class="phones-code" data-testid="phones-code">{code.code}</code>
        <span class="phones-muted">{baseURL}</span>
        {#if left > 0}
          <span class="phones-muted">{$t.phones.expiresIn(formatCountdown(left))}</span>
          <span class="phones-muted">{$t.phones.scanHint}</span>
        {:else}
          <span class="phones-warn">{$t.phones.expired}</span>
        {/if}
      </div>
    </div>
  {/if}

  <div class="phones-section">
    <h4>{$t.phones.devicesTitle}</h4>
    {#if !loading && devices.length === 0}
      <span class="phones-muted">{$t.phones.noDevices}</span>
    {/if}
    {#each devices as device (device.id)}
      <div class="phones-row" class:revoked={!device.active} data-testid="phones-device">
        <span class="phones-name">{device.name}</span>
        <span class="phones-muted">{device.active ? $t.phones.pairedOn(formatDate(device.created_at)) : $t.phones.revoked}</span>
        {#if device.active}
          <button type="button" class="btn btn-ghost btn-sm" disabled={busy} onclick={() => revoke(device)}>{$t.phones.revoke}</button>
        {/if}
      </div>
    {/each}
  </div>

  <div class="phones-section">
    <div class="phones-section-head">
      <h4>{$t.phones.foldersTitle}</h4>
      <button type="button" class="btn btn-ghost btn-sm" disabled={loading || busy} onclick={() => (picking = true)} data-testid="phones-add-folder">
        {$t.phones.addFolder}
      </button>
    </div>
    <span class="phones-muted">{$t.phones.foldersHint}</span>
    {#if !loading && folders.length === 0}
      <span class="phones-muted">{$t.phones.noFolders}</span>
    {/if}
    {#each folders as folder (folder.path)}
      <div class="phones-row" data-testid="phones-folder">
        <span class="phones-name">{folder.name}</span>
        <code class="phones-path">{folder.path}</code>
        {#if !folder.exists}<span class="phones-warn">{$t.phones.missing}</span>{/if}
        <button type="button" class="btn btn-ghost btn-sm" disabled={busy} onclick={() => removeFolder(folder.path)}>{$t.phones.remove}</button>
      </div>
    {/each}
  </div>

  {#if error}
    <div class="phones-error" role="alert">{error}</div>
  {/if}
</section>

{#if picking}
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="phones-backdrop" onclick={() => (picking = false)}>
    <div
      class="phones-modal"
      role="dialog"
      aria-modal="true"
      aria-label={$t.phones.addFolder}
      tabindex="-1"
      onclick={(e) => e.stopPropagation()}
    >
      <FolderPickerDialog allowCreate={false} onSelect={addFolder} onCancel={() => (picking = false)} />
    </div>
  </div>
{/if}

<style>
  .phones-card {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--surface);
    padding: var(--space-5);
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .phones-head,
  .phones-section-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .phones-kicker {
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

  h4 {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .phones-muted {
    color: var(--text-secondary);
    font-size: var(--text-sm);
    margin: 0;
  }

  .phones-warn {
    color: var(--warning);
    font-size: var(--text-sm);
  }

  .phones-note {
    border: 1px solid rgba(251, 191, 36, 0.28);
    border-radius: var(--radius-md);
    background: var(--warning-muted);
    padding: var(--space-3);
    font-size: var(--text-sm);
  }

  .phones-pairing {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    flex-wrap: wrap;
  }

  /* The code stays black on white in both themes: phone cameras read dark
     modules on a light ground most reliably. */
  .phones-qr {
    width: 196px;
    height: 196px;
    border-radius: var(--radius-md);
    flex: none;
  }

  .phones-pairing-text {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .phones-code {
    font-family: var(--font-mono);
    font-size: var(--text-xl);
    color: var(--text-primary);
    letter-spacing: 0.08em;
  }

  .phones-section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .phones-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    padding: var(--space-2) var(--space-3);
    flex-wrap: wrap;
  }

  .phones-row.revoked {
    opacity: 0.6;
  }

  .phones-row .btn {
    margin-left: auto;
  }

  .phones-name {
    color: var(--text-primary);
    font-size: var(--text-sm);
    font-weight: 600;
  }

  .phones-path {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    overflow-wrap: anywhere;
    min-width: 0;
  }

  .phones-error {
    border-radius: var(--radius-md);
    padding: var(--space-3);
    font-size: var(--text-sm);
    color: var(--error);
    background: var(--error-muted);
    border: 1px solid rgba(248, 113, 113, 0.28);
  }

  .phones-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    padding: var(--space-4);
  }

  .phones-modal {
    background: var(--surface-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    width: 100%;
    max-width: 560px;
    height: 70vh;
    max-height: 640px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  @media (max-width: 640px) {
    .phones-head,
    .phones-section-head {
      flex-direction: column;
      align-items: stretch;
    }
  }
</style>
