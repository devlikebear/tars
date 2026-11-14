<script lang="ts">
  // One pipeline on one desktop, in the app (docs/decisions/mobile-remote.md
  // §1): the desktop console's own pipeline screen, given an API that sends
  // every request to the desktop that owns the pipeline. What the screen
  // links to inside the console — another pipeline, or anything else —
  // opens here or in that desktop's console in the browser.
  import FocusPipeline from '../components/focus/FocusPipeline.svelte'
  import { remoteFocusApi } from '../lib/mobile/focusRemote'
  import { targetOf, type PairedDesktop } from '../lib/mobile/desktops'

  interface Props {
    desktop: PairedDesktop
    sessionId: string
    onOpen: (sessionId: string) => void
    onClose: () => void
  }

  let { desktop, sessionId, onOpen, onClose }: Props = $props()

  // MobileApp remounts this screen ({#key}) for another pipeline or desktop.
  // svelte-ignore state_referenced_locally
  const api = remoteFocusApi(targetOf(desktop), sessionId)

  function navigate(path: string) {
    if (path === '/console/focus') return onClose()
    const pipeline = /^\/console\/focus\/([^/?#]+)$/.exec(path)
    if (pipeline) return onOpen(decodeURIComponent(pipeline[1]))
    window.open(`${desktop.baseURL.replace(/\/+$/, '')}${path}`, '_blank', 'noopener')
  }

  // Links the screen renders into the console (a harness proposal's
  // pipeline, say) are relative to the desktop, not to the app.
  function onClick(event: MouseEvent) {
    const link = (event.target as Element | null)?.closest?.('a[href^="/console/"]')
    if (!link) return
    event.preventDefault()
    navigate(link.getAttribute('href') ?? '')
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="mobile-pipeline" onclick={onClick} data-testid="mobile-pipeline" data-desktop={desktop.name}>
  <div class="desk-bar"><span class="desk">{desktop.name}</span></div>
  <FocusPipeline {sessionId} onNavigate={navigate} remoteApi={api} remote />
</div>

<style>
  .mobile-pipeline {
    min-height: 100dvh;
    padding: env(safe-area-inset-top) 0 env(safe-area-inset-bottom);
    display: flex;
    flex-direction: column;
  }

  /* The console's pipeline screen at phone width. Only what a phone needs
     differently is changed here; the desktop console keeps its own styles.
     A phone has no room to spare for padding, and no keyboard for the
     deck's shortcuts. Titles wrap instead of ending in an ellipsis: a cut
     question cannot be answered. */
  .mobile-pipeline :global(.focus-screen) {
    padding: var(--space-3) var(--space-4) var(--space-8);
    width: 100%;
    box-sizing: border-box;
  }

  .mobile-pipeline :global(.focus-title h2),
  .mobile-pipeline :global(.focus-card-title) {
    white-space: normal;
    overflow-wrap: anywhere;
  }

  .mobile-pipeline :global(.focus-card-title) {
    flex-basis: 100%;
  }

  .mobile-pipeline :global(.deck-keys) {
    display: none;
  }

  .mobile-pipeline :global(button),
  .mobile-pipeline :global(.btn) {
    min-height: 40px;
  }

  .desk-bar {
    padding: var(--space-2) var(--space-4) 0;
  }

  .desk {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: rgb(var(--primary-rgb));
    background: rgba(var(--primary-rgb), 0.1);
    border-radius: var(--radius-sm);
    padding: 2px 6px;
  }
</style>
