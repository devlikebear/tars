<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import Shell from './components/Shell.svelte'
  import Home from './components/Home.svelte'
  import Chat from './components/Chat.svelte'
  import SessionLineageGraph from './components/SessionLineageGraph.svelte'
  import Plans from './components/Plans.svelte'
  import MemoryCenter from './components/MemoryCenter.svelte'
  import SyspromptCenter from './components/SyspromptCenter.svelte'
  import Ops from './components/Ops.svelte'
  import Cron from './components/Cron.svelte'
  import Logs from './components/Logs.svelte'
  import Analytics from './components/Analytics.svelte'
  import Config from './components/Config.svelte'
  import Extensions from './components/Extensions.svelte'
  import Pulse from './components/Pulse.svelte'
  import Reflection from './components/Reflection.svelte'
  import Channels from './components/Channels.svelte'
  import Onboarding from './components/Onboarding.svelte'
  import AgentRuntimeRunView from './components/AgentRuntimeRunView.svelte'
  import { resolveRoute, type Route } from './lib/router'
  import { getEventsHistory, getHealthz, streamEvents } from './lib/api'

  let currentPath = $state('/console')
  let route = $state<Route>({ view: 'home' })
  let serverHealth = $state('connecting')
  let needsSetup = $state(false)
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
    currentPath = window.location.pathname + window.location.search
    route = resolveRoute(currentPath)
  }

  function startGlobalStream() {
    stopGlobalStream?.()
    stopGlobalStream = streamEvents(
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

  async function checkSetupAndMaybeRedirect() {
    try {
      const health = await getHealthz()
      needsSetup = !!health.needs_setup
      if (needsSetup && route.view !== 'onboarding') {
        navigate('/console/onboarding')
      }
    } catch {
      // healthz fetch failed — let the SSE stream surface the disconnect
    }
  }

  function handleOnboardingComplete() {
    needsSetup = false
    navigate('/console')
  }

  onMount(() => {
    syncFromBrowser()
    const onPopState = () => syncFromBrowser()
    window.addEventListener('popstate', onPopState)

    void checkSetupAndMaybeRedirect()

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
  {needsSetup}
  onNavigate={navigate}
  onUnreadChange={(count) => { unreadCount = count }}
>
  {#if route.view === 'onboarding'}
    <Onboarding onComplete={handleOnboardingComplete} reentry={route.reentry === true} />
  {:else if route.view === 'home'}
    <Home onNavigate={navigate} />
  {:else if route.view === 'chat'}
    {#key aiPrompt}
      <Chat sessionId={route.sessionId} onNavigate={navigate} initialPrompt={aiPrompt} />
    {/key}
  {:else if route.view === 'session-lineage'}
    <SessionLineageGraph onNavigate={navigate} />
  {:else if route.view === 'tasks'}
    <Plans onNavigate={navigate} />
  {:else if route.view === 'agentruntime'}
    <AgentRuntimeRunView runId={route.runId} tab={route.tab} onNavigate={navigate} />
  {:else if route.view === 'memory'}
    <MemoryCenter onAskAI={navigateWithPrompt} />
  {:else if route.view === 'sysprompt'}
    <SyspromptCenter />
  {:else if route.view === 'ops'}
    <Ops />
  {:else if route.view === 'cron'}
    <Cron />
  {:else if route.view === 'logs'}
    <Logs />
  {:else if route.view === 'analytics'}
    <Analytics />
  {:else if route.view === 'config'}
    <Config />
  {:else if route.view === 'pulse'}
    <Pulse onNavigate={navigate} />
  {:else if route.view === 'reflection'}
    <Reflection />
  {:else if route.view === 'extensions'}
    <Extensions />
  {:else if route.view === 'channels'}
    <Channels />
  {/if}
</Shell>
