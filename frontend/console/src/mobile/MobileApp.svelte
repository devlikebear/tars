<script lang="ts">
  // The TARS mobile app's screen (docs/decisions/mobile-remote.md §2, §6):
  // what waits on you across every paired desktop, ideas kept on this phone
  // until sent to a desktop as a task, and the desktops themselves with
  // whether each answers now. Scanning a desk's QR code with the camera
  // opens a tars-mobile://pair link, which lands here and opens the pairing
  // sheet filled in.
  import { onDestroy, onMount } from 'svelte'
  import { t } from '../i18n'
  import type { PairingTarget } from '../lib/devicePairing'
  import type { DesktopStore } from '../lib/mobile/desktopStore.svelte'
  import type { InboxStore } from '../lib/mobile/inboxStore.svelte'
  import type { PairedDesktop } from '../lib/mobile/desktops'
  import type { AppLinks } from '../lib/mobile/appLinks'
  import { stageLabel } from '../lib/focus'
  import { runningPipelines, waitingOf } from '../lib/mobile/inbox'
  import type { FocusListItem } from '../lib/types'
  import InboxView from './InboxView.svelte'
  import MobilePipeline from './MobilePipeline.svelte'
  import PairSheet from './PairSheet.svelte'
  import IdeasView from './IdeasView.svelte'
  import SendIdeaSheet from './SendIdeaSheet.svelte'
  import type { IdeaStore } from '../lib/mobile/ideaStore.svelte'
  import type { Idea, SentIdea } from '../lib/mobile/ideas'

  interface Props {
    store: DesktopStore
    inbox: InboxStore
    ideas: IdeaStore
    links: AppLinks
  }

  let { store, inbox, ideas, links }: Props = $props()

  // The idea being sent, while the send sheet is open.
  let sending = $state<Idea | null>(null)
  const onlineDesktops = $derived(store.desktops.filter((d) => store.statuses[d.id]?.state === 'online'))

  function openSend(idea: Idea) {
    ideas.sendFailure = null
    sending = idea
  }

  function onSent(sent: SentIdea) {
    sending = null
    notice = $t.mobileApp.sentNotice(sent.desktopName)
    void refresh()
  }

  // While the app is open it reads again this often; there is no event
  // stream for phones yet.
  const pollMs = 30_000

  let tab = $state<'inbox' | 'ideas' | 'desktops'>('inbox')
  // The pipeline open on screen, on the desktop that owns it.
  let opened = $state<{ desktopID: string; sessionID: string } | null>(null)
  const openedDesktop = $derived(opened ? store.desktops.find((d) => d.id === opened!.desktopID) ?? null : null)
  let checking = $state(false)
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
    void refresh()
  }

  async function forget(desktop: PairedDesktop) {
    if (!confirm($t.mobileApp.confirmForget(desktop.name))) return
    await store.forget(desktop.id)
    void refresh()
  }

  // refresh checks which desktops answer, then asks those for what waits.
  async function refresh() {
    if (checking) return
    checking = true
    try {
      await store.refresh()
      await inbox.refresh(store.desktops.filter((d) => store.statuses[d.id]?.state === 'online'))
    } finally {
      checking = false
    }
  }

  // The desktops whose pipelines the inbox cannot show now.
  const missing = $derived(
    store.desktops
      .filter((d) => {
        const state = store.statuses[d.id]?.state
        return (state && state !== 'online' && state !== 'checking') || inbox.failed.includes(d.id)
      })
      .map((d) => d.name),
  )

  const waitingCount = $derived(inbox.entries.length)

  function openPipeline(desktopID: string, sessionID: string) {
    opened = { desktopID, sessionID }
    window.scrollTo(0, 0)
  }

  // Back from a pipeline: what waits has likely changed.
  function closePipeline() {
    opened = null
    void refresh()
  }

  function stageOf(item: FocusListItem): string {
    return stageLabel({ template: item.template, stages: [{ id: item.current, label: item.current_label ?? '' }] }, item.current, $t.focus)
  }

  function host(desktop: PairedDesktop): string {
    try {
      return new URL(desktop.baseURL).host
    } catch {
      return desktop.baseURL
    }
  }

  onMount(() => {
    void store.load().then(() => refresh())
    void ideas.load()
    // A pipeline on screen reads its own state; the lists wait for back.
    const timer = setInterval(() => {
      if (document.visibilityState === 'visible' && !opened) void refresh()
    }, pollMs)
    stops.push(
      links.listen((url) => {
        const target = store.readLink(url)
        if (target) openPair(target)
      }),
      links.onResume(() => {
        if (!opened) void refresh()
      }),
      () => clearInterval(timer),
    )
  })

  onDestroy(() => stops.forEach((stop) => stop()))
</script>

{#if opened && openedDesktop}
  {#key `${opened.desktopID}\n${opened.sessionID}`}
    <MobilePipeline
      desktop={openedDesktop}
      sessionId={opened.sessionID}
      onOpen={(sessionID) => openPipeline(openedDesktop.id, sessionID)}
      onClose={closePipeline}
    />
  {/key}
{:else}
<main class="mobile">
  <header>
    <h1>{$t.mobileApp.title}</h1>
    <button type="button" class="btn btn-ghost btn-sm" onclick={() => refresh()} disabled={!store.loaded || checking}>
      {checking ? $t.mobileApp.refreshing : $t.mobileApp.refresh}
    </button>
  </header>

  <div class="tabs" role="tablist">
    <button
      type="button"
      role="tab"
      aria-selected={tab === 'inbox'}
      class:active={tab === 'inbox'}
      onclick={() => (tab = 'inbox')}
      data-testid="mobile-tab-inbox"
    >
      {$t.mobileApp.tabs.inbox}
      {#if waitingCount > 0}
        <span class="badge" data-testid="mobile-inbox-count">{waitingCount}</span>
      {/if}
    </button>
    <button
      type="button"
      role="tab"
      aria-selected={tab === 'ideas'}
      class:active={tab === 'ideas'}
      onclick={() => (tab = 'ideas')}
      data-testid="mobile-tab-ideas"
    >
      {$t.mobileApp.tabs.ideas}
    </button>
    <button
      type="button"
      role="tab"
      aria-selected={tab === 'desktops'}
      class:active={tab === 'desktops'}
      onclick={() => (tab = 'desktops')}
      data-testid="mobile-tab-desktops"
    >
      {$t.mobileApp.tabs.desktops}
    </button>
  </div>

  {#if notice}
    <p class="notice" role="status">{notice}</p>
  {/if}

  {#if tab === 'inbox'}
    <InboxView {inbox} desktops={store.desktops} {missing} {checking} onOpen={openPipeline} />
  {:else if tab === 'ideas'}
    <IdeasView {ideas} onSend={openSend} onOpen={openPipeline} paired={(id) => store.desktops.some((d) => d.id === id)} />
  {:else}
  <section aria-label={$t.mobileApp.desktopsTitle}>
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
        {#if status === 'online' && inbox.pipelines[desktop.id]}
          {@const running = runningPipelines(inbox.pipelines[desktop.id])}
          <div class="pipelines" data-testid="mobile-desktop-pipelines">
            <span class="pipelines-title">{$t.mobileApp.pipelinesTitle}</span>
            {#if running.length === 0}
              <span class="state-hint">{$t.mobileApp.noPipelines}</span>
            {/if}
            {#each running as item (item.session_id)}
              {@const waits = waitingOf(item)}
              <button type="button" class="pipeline-row" onclick={() => openPipeline(desktop.id, item.session_id)} data-testid="mobile-desktop-pipeline">
                <span class="pipeline-title">{item.title || item.goal}</span>
                <span class="pipeline-meta">
                  {stageOf(item)}
                  {#if waits.gate || waits.cards > 0}<span class="pipeline-waits">· {$t.mobileApp.waitsOnYou}</span>{/if}
                  {#if item.goal_mode}<span>· {$t.focus.screen.goal}</span>{/if}
                </span>
              </button>
            {/each}
          </div>
        {/if}
      </article>
    {/each}
  </section>
  {/if}

  <!-- Pairing belongs to the Desktops tab, or to any tab while there is
       nothing paired yet. -->
  {#if tab === 'desktops' || (store.loaded && store.desktops.length === 0)}
    <footer>
      <button type="button" class="btn btn-primary" onclick={() => openPair()} data-testid="mobile-pair-open">
        {$t.mobileApp.pairDesktop}
      </button>
    </footer>
  {/if}
</main>

{/if}

{#if pairOpen}
  <PairSheet {store} initial={pairInitial} onDone={onPaired} onCancel={() => (pairOpen = false)} />
{/if}

{#if sending}
  <SendIdeaSheet {ideas} idea={sending} desktops={onlineDesktops} {onSent} onCancel={() => (sending = null)} />
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

  .pipelines {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin-top: var(--space-3);
    padding-top: var(--space-3);
    border-top: 1px solid var(--border-subtle);
  }

  .pipelines-title {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  .pipeline-row {
    display: flex;
    flex-direction: column;
    gap: 2px;
    align-items: flex-start;
    text-align: left;
    min-height: 44px;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    color: var(--text-primary);
    font: inherit;
  }

  .pipeline-title {
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
  }

  .pipeline-meta {
    display: flex;
    gap: var(--space-1);
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .pipeline-waits {
    color: var(--warning);
  }

  .tabs {
    display: flex;
    gap: var(--space-1);
    padding: 3px;
    border-radius: var(--radius-md);
    background: var(--surface-inset);
  }

  .tabs button {
    flex: 1;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    min-height: 36px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--text-sm);
  }

  .tabs button.active {
    background: var(--surface-elevated);
    color: var(--text-primary);
  }

  .badge {
    min-width: 20px;
    padding: 0 6px;
    border-radius: 10px;
    background: var(--warning);
    color: var(--surface-base);
    font-size: var(--text-xs);
    font-weight: 600;
    line-height: 20px;
  }

  footer {
    margin-top: auto;
  }

  footer .btn {
    width: 100%;
    min-height: 48px;
    font-size: 16px;
  }
</style>
