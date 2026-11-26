<script lang="ts">
  // The TARS iPhone app (docs/decisions/mobile-remote.md, docs/plans/vibe-
  // mode-redesign unit 8): vibe mode on every paired desktop, driven from
  // the phone. Three tabs — Projects (what waits on the person first), +
  // New (an idea to a desk's team, or kept here while no desk is on) and
  // Computers (each desk in plain words, and pairing). A project opens its
  // own screen (MobileProject). The earlier Waiting, Ideas and Chat tabs
  // are folded into these: what waits leads Projects, ideas are what + New
  // keeps, and a project's team conversation replaces the chat.
  //
  // Scanning a desk's QR code with the camera opens a tars-mobile://pair
  // link, which lands here and opens the pairing sheet filled in. A
  // tars-mobile://focus link (an alert's "open in app") opens the project
  // whose run it names, or that run's own screen when it is not a vibe
  // project.
  import { onDestroy, onMount } from 'svelte'
  import { t } from '../i18n'
  import type { PairingTarget } from '../lib/devicePairing'
  import type { DesktopStore } from '../lib/mobile/desktopStore.svelte'
  import type { ProjectStore } from '../lib/mobile/projectStore.svelte'
  import type { IdeaStore } from '../lib/mobile/ideaStore.svelte'
  import { isDemoDesktop } from '../lib/mobile/demoDesk'
  import type { AppLinks } from '../lib/mobile/appLinks'
  import type { DesktopChange, DesktopEvents } from '../lib/mobile/desktopEvents'
  import type { PairedDesktop } from '../lib/mobile/desktops'
  import { waitingCount } from '../lib/mobile/projects'
  import MobileProject from './MobileProject.svelte'
  import MobilePipeline from './MobilePipeline.svelte'
  import PairSheet from './PairSheet.svelte'
  import ConfirmSheet from './ConfirmSheet.svelte'
  import ProjectsView from './ProjectsView.svelte'
  import NewView from './NewView.svelte'
  import ComputersView from './ComputersView.svelte'

  interface Props {
    store: DesktopStore
    projects: ProjectStore
    ideas: IdeaStore
    // The desktops' event streams; optional so a screen built without them
    // still reads on its timer.
    events?: DesktopEvents
    links: AppLinks
  }

  let { store, projects, ideas, events, links }: Props = $props()

  // While the app is open it reads again this often: for desktops that wake
  // or sleep, and for desktops too old for the event stream. A desktop
  // whose stream is open says at once when a project changes.
  const pollMs = 30_000

  let tab = $state<'projects' | 'new' | 'computers'>('projects')
  // What is open on screen: a project, or (from a link) a run that is not
  // a vibe project.
  let opened = $state<{ desktopID: string; projectID?: string; sessionID?: string } | null>(null)
  const openedDesktop = $derived(opened ? store.desktops.find((d) => d.id === opened!.desktopID) ?? null : null)
  let checking = $state(false)
  let pairOpen = $state(false)
  let pairInitial = $state<PairingTarget | null>(null)
  let notice = $state('')
  let noticeWarns = $state(false)
  const stops: Array<() => void> = []

  const isOnline = (d: PairedDesktop) => store.statuses[d.id]?.state === 'online'
  const onlineDesktops = $derived(store.desktops.filter(isOnline))
  const waiting = $derived(waitingCount(projects.projects))
  const unsentIdeas = $derived(ideas.ideas.filter((i) => !i.sent).length)

  function say(text: string, warns = false) {
    notice = text
    noticeWarns = warns
  }

  function openPair(desktop?: PairedDesktop, target: PairingTarget | null = null) {
    store.pairFailure = null
    pairInitial = target ?? (desktop ? { baseURL: desktop.baseURL, code: '' } : null)
    pairOpen = true
  }

  function onPaired(name: string) {
    pairOpen = false
    say($t.phone.computers.paired(name))
    void refresh()
  }

  // tryDemo adds the desk that lives inside the app, for someone with no
  // desktop paired yet (and for App Store review).
  async function tryDemo() {
    await store.addDemo()
    say($t.phone.computers.demoAdded)
    tab = 'new'
    void refresh()
  }

  // Forgetting a computer asks first, in the phone's own sheet.
  let forgetting = $state<PairedDesktop | null>(null)
  async function forget() {
    const desktop = forgetting
    forgetting = null
    if (!desktop) return
    await store.forget(desktop.id)
    void refresh()
  }

  // refresh checks which desktops answer, then reads their projects.
  async function refresh() {
    if (checking) return
    checking = true
    try {
      await store.refresh()
      const online = store.desktops.filter(isOnline)
      events?.watch(online)
      await projects.refresh(online)
    } finally {
      checking = false
    }
  }

  // The desktops whose projects the list cannot show now.
  const missing = $derived(
    store.desktops
      .filter((d) => {
        const state = store.statuses[d.id]?.state
        return (state && state !== 'online' && state !== 'checking') || projects.failed.includes(d.id)
      })
      .map((d) => d.name),
  )

  function openProject(desktopID: string, projectID: string) {
    notice = ''
    opened = { desktopID, projectID }
    window.scrollTo(0, 0)
  }

  // A desktop's stream said something changed: read that desktop's
  // projects again. A project on screen reads its own state.
  function onDesktopChanged(change: DesktopChange) {
    if (opened) return
    const desktop = store.desktops.find((d) => d.id === change.desktopID)
    if (desktop) void projects.refreshDesktop(desktop)
  }

  // Back from a project goes to the Projects list, wherever the project was
  // opened from (+ New hands an idea to a team and opens its project; back
  // from there is the list it now sits in, not an emptied idea form). Its
  // state has likely changed, so the list reads again.
  function close() {
    opened = null
    tab = 'projects'
    void refresh()
  }

  function hostOf(baseURL: string): string {
    try {
      return new URL(baseURL).host
    } catch {
      return baseURL
    }
  }

  // openLink acts on a link the app was opened with.
  async function openLink(url: string, loading: Promise<void>) {
    const pairing = store.readLink(url)
    if (pairing) {
      openPair(undefined, pairing)
      return
    }
    await loading
    const focus = store.readFocusLink(url)
    if (!focus) return
    if (!focus.desktop) {
      // A desk this phone is not paired with: say so where pairing is.
      opened = null
      tab = 'computers'
      say($t.phone.computers.notPaired(hostOf(focus.target.baseURL)), true)
      return
    }
    const desktop = focus.desktop
    if (isOnline(desktop) && !projects.byDesk[desktop.id]) await projects.refreshDesktop(desktop)
    const project = projects.findBySession(desktop.id, focus.target.sessionID)
    opened = project ? { desktopID: desktop.id, projectID: project.id } : { desktopID: desktop.id, sessionID: focus.target.sessionID }
  }

  onMount(() => {
    const loading = store.load()
    const first = loading.then(() => refresh())
    void ideas.load()
    const timer = setInterval(() => {
      if (document.visibilityState === 'visible' && !opened) void refresh()
    }, pollMs)
    stops.push(
      links.listen((url) => void openLink(url, first)),
      links.onResume(() => {
        events?.reconnect()
        if (!opened) void refresh()
      }),
      () => clearInterval(timer),
    )
    if (events) stops.push(events.listen(onDesktopChanged), () => events.close())
  })

  onDestroy(() => stops.forEach((stop) => stop()))
</script>

{#if opened && openedDesktop && opened.projectID}
  {#key `${opened.desktopID}\n${opened.projectID}`}
    <MobileProject
      desktop={openedDesktop}
      projectId={opened.projectID}
      {events}
      onOpen={(projectID) => openProject(openedDesktop.id, projectID)}
      onClose={close}
    />
  {/key}
{:else if opened && openedDesktop && opened.sessionID}
  {#key `${opened.desktopID}\n${opened.sessionID}`}
    <MobilePipeline
      desktop={openedDesktop}
      sessionId={opened.sessionID}
      {events}
      onOpen={(sessionID) => (opened = { desktopID: openedDesktop.id, sessionID })}
      onClose={close}
    />
  {/key}
{:else}
  <main class="mobile">
    {#if notice}
      <p class="notice" class:warns={noticeWarns} role="status" data-testid="mobile-notice">{notice}</p>
    {/if}

    {#if tab === 'projects'}
      <ProjectsView {projects} desktops={store.desktops} {missing} onOpen={openProject} onNotice={(text) => say(text)} />
      {#if store.loaded && store.desktops.length === 0}
        <button type="button" class="btn btn-primary first" onclick={() => (tab = 'computers')}>{$t.phone.computers.add}</button>
      {/if}
    {:else if tab === 'new'}
      <NewView {projects} {ideas} desktops={store.desktops} online={isOnline} onOpen={openProject} onNotice={(text) => say(text)} />
    {:else}
      <ComputersView
        {store}
        {projects}
        savedIdeas={unsentIdeas}
        {checking}
        onPair={(desktop) => openPair(desktop)}
        onDemo={tryDemo}
        onForget={(desktop) => (forgetting = desktop)}
        onRefresh={() => void refresh()}
        onIdeas={() => (tab = 'new')}
      />
    {/if}
  </main>

  <nav class="tabbar" aria-label={$t.phone.tabsLabel}>
    <button type="button" class:active={tab === 'projects'} aria-current={tab === 'projects' ? 'page' : undefined} onclick={() => (tab = 'projects')} data-testid="mobile-tab-projects">
      {$t.phone.tabs.projects}
      {#if waiting > 0}<span class="badge" data-testid="mobile-waiting-count">{waiting}</span>{/if}
    </button>
    <button type="button" class:active={tab === 'new'} aria-current={tab === 'new' ? 'page' : undefined} onclick={() => (tab = 'new')} data-testid="mobile-tab-new">
      {$t.phone.tabs.new}
    </button>
    <button type="button" class:active={tab === 'computers'} aria-current={tab === 'computers' ? 'page' : undefined} onclick={() => (tab = 'computers')} data-testid="mobile-tab-computers">
      {$t.phone.tabs.computers}
      {#if onlineDesktops.length === 0 && store.loaded}<span class="dot-warn" aria-hidden="true"></span>{/if}
    </button>
  </nav>
{/if}

{#if forgetting}
  <ConfirmSheet
    question={isDemoDesktop(forgetting) ? $t.phone.computers.confirmForgetDemo : $t.phone.computers.confirmForget(forgetting.name)}
    confirmLabel={$t.phone.computers.forgetGo}
    onConfirm={() => void forget()}
    onCancel={() => (forgetting = null)}
  />
{/if}

{#if pairOpen}
  <PairSheet {store} initial={pairInitial} onDone={onPaired} onCancel={() => (pairOpen = false)} />
{/if}

<style>
  .mobile {
    min-height: 100dvh;
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    box-sizing: border-box;
    padding: calc(env(safe-area-inset-top) + var(--space-4)) var(--space-4) calc(env(safe-area-inset-bottom) + 72px);
    max-width: 640px;
    margin: 0 auto;
  }

  .notice {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--success);
    line-height: 1.5;
  }

  .notice.warns {
    color: var(--warning);
  }

  .first {
    width: 100%;
    min-height: 48px;
  }

  /* The three tabs sit at the bottom, where a thumb reaches
     (screens/M-Projects). */
  .tabbar {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    display: flex;
    padding: var(--space-1) var(--space-2) calc(env(safe-area-inset-bottom) + var(--space-1));
    border-top: 1px solid var(--border-subtle);
    background: var(--surface);
  }

  .tabbar button {
    flex: 1;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    min-height: 52px;
    border: 0;
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--text-sm);
  }

  .tabbar button.active {
    color: var(--primary-text);
    font-weight: 600;
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

  .dot-warn {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--warning);
  }
</style>
