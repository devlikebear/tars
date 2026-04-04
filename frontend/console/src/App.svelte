<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import Shell from './components/Shell.svelte'
  import Chat from './components/Chat.svelte'
  import MemoryCenter from './components/MemoryCenter.svelte'
  import SyspromptCenter from './components/SyspromptCenter.svelte'
  import ProjectView from './components/ProjectView.svelte'
  import Projects from './components/Projects.svelte'
  import Ops from './components/Ops.svelte'
  import Config from './components/Config.svelte'
  import Extensions from './components/Extensions.svelte'
  import Heartbeat from './components/Heartbeat.svelte'
  import { resolveRoute, type Route } from './lib/router'
  import { getEventsHistory, streamEvents } from './lib/api'

  let currentPath = $state('/console')
  let route: Route = $state({ view: 'chat' })
  let serverHealth = $state('connecting')
  let unreadCount = $state(0)
  let aiPrompt = $state('')
  let stopGlobalStream: (() => void) | null = null

  function navigate(path: string) {
    if (path === currentPath) return
    window.history.pushState(null, '', path)
    currentPath = path
    route = resolveRoute(path)
  }

  function navigateWithPrompt(prompt: string) {
    aiPrompt = prompt
    navigate('/console/chat')
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
  onUnreadChange={(count) => { unreadCount = count }}
>
  {#if route.view === 'chat'}
    {#key aiPrompt}
      <Chat sessionId={route.sessionId} onNavigate={navigate} initialPrompt={aiPrompt} />
    {/key}
  {:else if route.view === 'project'}
    {#key route.projectId}
      <ProjectView projectId={route.projectId} />
    {/key}
  {:else if route.view === 'projects'}
    <Projects onNavigate={navigate} onAskAI={navigateWithPrompt} />
  {:else if route.view === 'memory'}
    <MemoryCenter onAskAI={navigateWithPrompt} />
  {:else if route.view === 'sysprompt'}
    <SyspromptCenter />
  {:else if route.view === 'ops'}
    <Ops onAskAI={navigateWithPrompt} />
  {:else if route.view === 'config'}
    <Config />
  {:else if route.view === 'heartbeat'}
    <Heartbeat />
  {:else if route.view === 'extensions'}
    <Extensions />
  {/if}
</Shell>
