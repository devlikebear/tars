<script lang="ts">
  // Pairing with a desktop: its Tailscale address, the one-time code the
  // desk shows, and this phone's name for the desk's list. A scanned
  // tars-mobile://pair link arrives as `initial` and fills the first two.
  import { t } from '../i18n'
  import type { DesktopStore } from '../lib/mobile/desktopStore.svelte'
  import type { PairingTarget } from '../lib/devicePairing'

  interface Props {
    store: DesktopStore
    initial?: PairingTarget | null
    onDone: (name: string) => void
    onCancel: () => void
  }

  let { store, initial = null, onDone, onCancel }: Props = $props()

  let address = $state('')
  let code = $state('')
  let phoneName = $state('')

  $effect(() => {
    if (initial) {
      address = initial.baseURL
      code = initial.code
    }
  })

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    const desktop = await store.pair(address, code, phoneName || $t.mobileApp.defaultPhoneName)
    if (desktop) onDone(desktop.name)
  }
</script>

<div class="sheet" role="dialog" aria-modal="true" aria-label={$t.mobileApp.pairTitle} data-testid="mobile-pair-sheet">
  <form onsubmit={submit}>
    <h2>{$t.mobileApp.pairTitle}</h2>
    <p class="hint">{$t.mobileApp.pairHint}</p>

    <label>
      <span>{$t.mobileApp.addressLabel}</span>
      <input
        type="url"
        inputmode="url"
        autocapitalize="off"
        autocomplete="off"
        spellcheck="false"
        placeholder={$t.mobileApp.addressPlaceholder}
        bind:value={address}
        data-testid="mobile-pair-address"
      />
    </label>
    <label>
      <span>{$t.mobileApp.codeLabel}</span>
      <input
        class="code"
        inputmode="numeric"
        autocomplete="one-time-code"
        autocapitalize="off"
        bind:value={code}
        data-testid="mobile-pair-code"
      />
    </label>
    <label>
      <span>{$t.mobileApp.phoneNameLabel}</span>
      <input placeholder={$t.mobileApp.defaultPhoneName} bind:value={phoneName} maxlength="64" data-testid="mobile-pair-name" />
      <small>{$t.mobileApp.phoneNameHint}</small>
    </label>

    {#if store.pairFailure}
      <p class="error" role="alert">{$t.mobileApp.failures[store.pairFailure]}</p>
    {/if}

    <div class="actions">
      <button type="submit" class="btn btn-primary" disabled={store.pairing} data-testid="mobile-pair-submit">
        {store.pairing ? $t.mobileApp.pairing : $t.mobileApp.pair}
      </button>
      <button type="button" class="btn btn-ghost" onclick={onCancel}>{$t.mobileApp.cancel}</button>
    </div>
  </form>
</div>

<style>
  .sheet {
    position: fixed;
    inset: 0;
    z-index: 10;
    background: var(--surface-base);
    overflow-y: auto;
    padding: calc(env(safe-area-inset-top) + var(--space-5)) var(--space-4) calc(env(safe-area-inset-bottom) + var(--space-5));
  }

  form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 520px;
    margin: 0 auto;
  }

  h2 {
    margin: 0;
    font-size: var(--text-xl);
  }

  .hint,
  small {
    color: var(--text-secondary);
    font-size: var(--text-sm);
    margin: 0;
  }

  label {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  input {
    font: inherit;
    font-size: 17px; /* 16px or more keeps iOS from zooming into the field */
    color: var(--text-primary);
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    padding: 12px;
  }

  input:focus {
    outline: none;
    border-color: var(--primary);
  }

  .code {
    font-family: var(--font-mono);
    letter-spacing: 0.12em;
  }

  .error {
    margin: 0;
    color: var(--error);
    background: var(--error-muted);
    border-radius: var(--radius-md);
    padding: var(--space-3);
    font-size: var(--text-sm);
  }

  .actions {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .actions .btn {
    min-height: 44px;
    font-size: 16px;
  }
</style>
