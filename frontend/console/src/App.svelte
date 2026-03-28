<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import Shell from './components/Shell.svelte'
  import Home from './components/Home.svelte'
  import ProjectView from './components/ProjectView.svelte'
  import { resolveRoute, type Route } from './lib/router'
  import { getEventsHistory, streamEvents } from './lib/api'

  let currentPath = $state('/console')
  let route: Route = $state({ view: 'home' })
  let serverHealth = $state('connecting')
  let unreadCount = $state(0)
  let stopGlobalStream: (() => void) | null = null

  function navigate(path: string) {
    if (path === currentPath) return
    window.history.pushState(null, '', path)
    currentPath = path
    route = resolveRoute(path)
  }

  function syncFromBrowser() {
    currentPath = window.location.pathname
    route = resolveRoute(currentPath)
  }

  function startGlobalStream() {
    stopGlobalStream?.()
    stopGlobalStream = streamEvents(
      undefined,
      () => {
        unreadCount++
      },
      () => {
        serverHealth = 'disconnected'
      },
      () => {
        serverHealth = 'ok'
      },
    )
  }

  onMount(() => {
    syncFromBrowser()
    const onPopState = () => syncFromBrowser()
    window.addEventListener('popstate', onPopState)

    getEventsHistory(1)
      .then((h) => { unreadCount = h.unread_count ?? 0 })
      .catch(() => {})

    startGlobalStream()

    return () => window.removeEventListener('popstate', onPopState)
  })

  onDestroy(() => {
    stopGlobalStream?.()
  })
</script>

<Shell
  {currentPath}
  {serverHealth}
  {unreadCount}
  onNavigate={navigate}
>
  {#if route.view === 'home'}
    <Home onNavigate={navigate} />
  {:else if route.view === 'project'}
    {#key route.projectId}
      <ProjectView projectId={route.projectId} />
    {/key}
  {:else if route.view === 'projects'}
    <Home onNavigate={navigate} />
  {:else if route.view === 'sessions'}
    <div class="placeholder-view">
      <h2>Sessions</h2>
      <p>Chat history and session management. Coming soon.</p>
    </div>
  {:else if route.view === 'ops'}
    <div class="placeholder-view">
      <h2>Operations</h2>
      <p>Approvals, cron jobs, and system operations. Coming soon.</p>
    </div>
  {/if}
</Shell>

<style>
  .placeholder-view {
    padding: var(--space-10);
    text-align: center;
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  .placeholder-view h2 {
    margin-bottom: var(--space-2);
    font-size: var(--text-2xl);
  }

  .placeholder-view p {
    color: var(--text-tertiary);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }
</style>
