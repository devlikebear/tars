<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import Shell from './components/Shell.svelte'
  import Home from './components/Home.svelte'
  import ProjectView from './components/ProjectView.svelte'
  import Sessions from './components/Sessions.svelte'
  import Ops from './components/Ops.svelte'
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
    <Sessions />
  {:else if route.view === 'ops'}
    <Ops />
  {/if}
</Shell>

