<script lang="ts">
  // The TARS mobile app's first screen (docs/decisions/mobile-remote.md §2):
  // the desktops this phone is paired with and whether each answers now.
  // Scanning a desk's QR code with the camera opens a tars-mobile://pair
  // link, which lands here and opens the pairing sheet filled in.
  import { onDestroy, onMount } from 'svelte'
  import { t } from '../i18n'
  import type { PairingTarget } from '../lib/devicePairing'
  import type { DesktopStore } from '../lib/mobile/desktopStore.svelte'
  import type { PairedDesktop } from '../lib/mobile/desktops'
  import type { AppLinks } from '../lib/mobile/appLinks'
  import PairSheet from './PairSheet.svelte'

  interface Props {
    store: DesktopStore
    links: AppLinks
  }

  let { store, links }: Props = $props()

  let pairOpen = $state(false)
  let pairInitial = $state<PairingTarget | null>(null)
  let notice = $state('')
  const stops: Array<() => void> = []

  function openPair(target: PairingTarget | null = null) {
    store.pairFailure = null
    pairInitial = target
    pairOpen = true
  }

  function onPaired(name: string) {
    pairOpen = false
    notice = $t.mobileApp.pairedWith(name)
  }

  async function forget(desktop: PairedDesktop) {
    if (!confirm($t.mobileApp.confirmForget(desktop.name))) return
    await store.forget(desktop.id)
  }

  function host(desktop: PairedDesktop): string {
    try {
      return new URL(desktop.baseURL).host
    } catch {
      return desktop.baseURL
    }
  }

  onMount(() => {
    void store.load().then(() => store.refresh())
    stops.push(
      links.listen((url) => {
        const target = store.readLink(url)
        if (target) openPair(target)
      }),
      links.onResume(() => void store.refresh()),
    )
  })

  onDestroy(() => stops.forEach((stop) => stop()))
</script>

<main class="mobile">
  <header>
    <h1>{$t.mobileApp.title}</h1>
    <button type="button" class="btn btn-ghost btn-sm" onclick={() => store.refresh()} disabled={!store.loaded}>
      {$t.mobileApp.refresh}
    </button>
  </header>

  {#if notice}
    <p class="notice" role="status">{notice}</p>
  {/if}

  <section aria-label={$t.mobileApp.desktopsTitle}>
    <h2>{$t.mobileApp.desktopsTitle}</h2>
    {#if store.loaded && store.desktops.length === 0}
      <p class="empty" data-testid="mobile-empty">{$t.mobileApp.empty}</p>
    {/if}
    {#each store.desktops as desktop (desktop.id)}
      {@const status = store.statuses[desktop.id]?.state ?? 'checking'}
      <article class="desktop" data-testid="mobile-desktop" data-state={status}>
        <div class="desktop-head">
          <span class="dot {status}" aria-hidden="true"></span>
          <strong>{desktop.name}</strong>
          <span class="state">{$t.mobileApp.states[status]}</span>
        </div>
        <span class="host">{host(desktop)}</span>
        {#if $t.mobileApp.stateHints[status]}
          <span class="state-hint">{$t.mobileApp.stateHints[status]}</span>
        {/if}
        <div class="desktop-actions">
          {#if status === 'unpaired'}
            <button type="button" class="btn btn-primary btn-sm" onclick={() => openPair({ baseURL: desktop.baseURL, code: '' })}>
              {$t.mobileApp.pair}
            </button>
          {/if}
          <button type="button" class="btn btn-ghost btn-sm" onclick={() => forget(desktop)}>{$t.mobileApp.forget}</button>
        </div>
      </article>
    {/each}
  </section>

  <footer>
    <button type="button" class="btn btn-primary" onclick={() => openPair()} data-testid="mobile-pair-open">
      {$t.mobileApp.pairDesktop}
    </button>
  </footer>
</main>

{#if pairOpen}
  <PairSheet {store} initial={pairInitial} onDone={onPaired} onCancel={() => (pairOpen = false)} />
{/if}

<style>
  .mobile {
    min-height: 100dvh;
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    padding: calc(env(safe-area-inset-top) + var(--space-4)) var(--space-4) calc(env(safe-area-inset-bottom) + var(--space-4));
    max-width: 640px;
    margin: 0 auto;
  }

  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  h1 {
    margin: 0;
    font-size: var(--text-xl);
    letter-spacing: 0.04em;
  }

  h2 {
    margin: 0 0 var(--space-2);
    font-size: var(--text-sm);
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  section {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    flex: 1;
  }

  .empty,
  .notice {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--text-secondary);
    line-height: 1.5;
  }

  .notice {
    color: var(--success);
  }

  .desktop {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--surface);
    padding: var(--space-4);
  }

  .desktop-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .desktop-head strong {
    font-size: 17px;
    color: var(--text-primary);
  }

  .state {
    margin-left: auto;
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--text-tertiary);
    flex: none;
  }

  .dot.online {
    background: var(--success);
  }

  .dot.unpaired,
  .dot.setup {
    background: var(--warning);
  }

  .dot.unreachable {
    background: var(--error);
  }

  .host {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    overflow-wrap: anywhere;
  }

  .state-hint {
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }

  .desktop-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }

  footer .btn {
    width: 100%;
    min-height: 48px;
    font-size: 16px;
  }
</style>
