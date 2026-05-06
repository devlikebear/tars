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
  import Login from './components/Login.svelte'
  import AgentRuntimeRunView from './components/AgentRuntimeRunView.svelte'
  import { resolveRoute, type Route } from './lib/router'
  import { APIRequestError, getAuthWhoami, getEventsHistory, getHealthz, logoutAuth, streamEvents } from './lib/api'
  import type { AuthWhoamiResponse } from './lib/types'
  import { isZenShortcut, zenMode } from './lib/zenMode.svelte'

  let currentPath = $state('/console')
  let route = $state<Route>({ view: 'home' })
  let serverHealth = $state('connecting')
  let needsSetup = $state(false)
  let unreadCount = $state(0)
  let aiPrompt = $state('')
  let authLoading = $state(true)
  let authInfo = $state<AuthWhoamiResponse | null>(null)
  let loginRequired = $state(false)
  let stopGlobalStream: (() => void) | null = null
  let authRole = $derived(authInfo?.auth_role ?? '')
  let zenActive = $derived(zenMode.active && route.view === 'chat' && !needsSetup && !loginRequired)

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
    if (needsSetup || loginRequired) return
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

  async function refreshAuth() {
    authLoading = true
    try {
      authInfo = await getAuthWhoami()
      loginRequired = authInfo.auth_mode === 'required' && !authInfo.authenticated
    } catch (err) {
      authInfo = null
      loginRequired = err instanceof APIRequestError && err.status === 401
    } finally {
      authLoading = false
    }
  }

  async function loadConsoleNotifications() {
    if (needsSetup || loginRequired) return
    getEventsHistory(1)
      .then((h) => { unreadCount = h.unread_count ?? 0 })
      .catch(() => {})
    startGlobalStream()
  }

  function handleOnboardingComplete() {
    needsSetup = false
    navigate('/console')
    void refreshAuth().then(loadConsoleNotifications)
  }

  function handleLogin(auth: AuthWhoamiResponse) {
    authInfo = auth
    loginRequired = false
    void loadConsoleNotifications()
  }

  async function handleLogout() {
    try {
      await logoutAuth()
    } catch {
      // Cookie may already be invalid; local UI state still needs to reset.
    }
    stopGlobalStream?.()
    stopGlobalStream = null
    authInfo = null
    loginRequired = true
    unreadCount = 0
    navigate('/console')
  }

  function onGlobalKeydown(event: KeyboardEvent) {
    if (route.view !== 'chat' || needsSetup || loginRequired) return
    if (isZenShortcut(event)) {
      event.preventDefault()
      zenMode.toggle()
      return
    }
    if (event.key === 'Escape' && zenMode.active) {
      const target = event.target as HTMLElement | null
      const tag = target?.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || target?.isContentEditable) return
      event.preventDefault()
      zenMode.set(false)
    }
  }

  onMount(() => {
    syncFromBrowser()
    const onPopState = () => syncFromBrowser()
    window.addEventListener('popstate', onPopState)
    window.addEventListener('keydown', onGlobalKeydown)

    void checkSetupAndMaybeRedirect()
      .then(refreshAuth)
      .then(loadConsoleNotifications)

    return () => {
      window.removeEventListener('popstate', onPopState)
      window.removeEventListener('keydown', onGlobalKeydown)
    }
  })

  onDestroy(() => {
    stopGlobalStream?.()
  })
</script>

{#if authLoading && !needsSetup}
  <div class="app-loading">Connecting to TARS...</div>
{:else if loginRequired && !(needsSetup && route.view === 'onboarding')}
  <Login onLogin={handleLogin} />
{:else}
  <Shell
    {currentPath}
    {serverHealth}
    {unreadCount}
    {needsSetup}
    {authRole}
    {zenActive}
    onNavigate={navigate}
    onUnreadChange={(count) => { unreadCount = count }}
    onLogout={handleLogout}
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
    {:else if route.view === 'ops' && authRole !== 'user'}
      <Ops />
    {:else if route.view === 'cron' && authRole !== 'user'}
      <Cron />
    {:else if route.view === 'logs' && authRole !== 'user'}
      <Logs />
    {:else if route.view === 'analytics' && authRole !== 'user'}
      <Analytics />
    {:else if route.view === 'config' && authRole !== 'user'}
      <Config onNavigate={navigate} />
    {:else if route.view === 'pulse' && authRole !== 'user'}
      <Pulse onNavigate={navigate} />
    {:else if route.view === 'reflection' && authRole !== 'user'}
      <Reflection />
    {:else if route.view === 'extensions' && authRole !== 'user'}
      <Extensions />
    {:else if route.view === 'channels' && authRole !== 'user'}
      <Channels />
    {:else}
      <Home onNavigate={navigate} />
    {/if}
  </Shell>
{/if}

<style>
  .app-loading {
    min-height: 100vh;
    display: grid;
    place-items: center;
    color: var(--text-secondary);
    background: var(--surface-base);
    font-family: var(--font-display);
  }
</style>
