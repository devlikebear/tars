<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import Shell from './components/Shell.svelte'
  import CompanionPet from './components/CompanionPet.svelte'
  import Home from './components/Home.svelte'
  import Onboarding from './components/Onboarding.svelte'
  import Login from './components/Login.svelte'
  import { resolveRoute, type Route } from './lib/router'
  import { loadRouteComponent } from './lib/routeComponents'
  import { APIRequestError, getAuthWhoami, getConfigSchema, getEventsHistory, getHealthz, logoutAuth, requestCompanionFeedback, streamEvents } from './lib/api'
  import type { AuthWhoamiResponse } from './lib/types'
  import {
    companionAskHandoffReaction,
    companionEnabledFromConfigValues,
    companionPromptForAsk,
    companionReactionForStimulus,
    companionReactionFromEvent,
    shouldShowCompanion,
    type CompanionReaction,
    type CompanionStimulus,
  } from './lib/companion'
  import { locale } from './i18n'
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
  let companionEnabled = $state(false)
  let companionReaction = $state<CompanionReaction | null>(null)
  let stopGlobalStream: (() => void) | null = null
  let companionReactionTimer: ReturnType<typeof setTimeout> | null = null
  let companionFeedbackRequestID = 0
  let authRole = $derived(authInfo?.auth_role ?? '')
  let zenActive = $derived(zenMode.active && route.view === 'chat' && !needsSetup && !loginRequired)
  let showCompanion = $derived(shouldShowCompanion({ enabled: companionEnabled, needsSetup, loginRequired, zenActive }))

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
      (event) => {
        if (!event.coalesced) unreadCount++
        const reaction = companionReactionFromEvent(event, $locale)
        if (reaction) showCompanionReaction(reaction)
      },
      () => {
        serverHealth = 'disconnected'
      },
      () => {
        serverHealth = 'ok'
      },
    )
  }

  function showCompanionReaction(reaction: CompanionReaction) {
    if (companionReactionTimer) {
      clearTimeout(companionReactionTimer)
      companionReactionTimer = null
    }
    companionReaction = reaction
    companionReactionTimer = setTimeout(() => {
      companionReaction = null
      companionReactionTimer = null
    }, 9000)
  }

  async function handleCompanionStimulus(stimulus: CompanionStimulus) {
    const requestID = ++companionFeedbackRequestID
    const fallback = companionReactionForStimulus(stimulus, route.view, $locale)
    try {
      const response = await requestCompanionFeedback({
        stimulus,
        route_view: route.view,
        locale: $locale,
        fallback_message: fallback.message,
        fallback_detail: fallback.detail,
      })
      if (requestID !== companionFeedbackRequestID) return
      showCompanionReaction({
        mood: response.mood,
        message: response.message || fallback.message,
        detail: response.detail || fallback.detail,
      })
    } catch {
      // The pet already showed the local fallback instantly; failed micro-feedback stays quiet.
    }
  }

  function handleCompanionAsk(prompt: string) {
    companionFeedbackRequestID += 1
    showCompanionReaction(companionAskHandoffReaction($locale))
    navigateWithPrompt(companionPromptForAsk(prompt, route.view, $locale))
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
    void loadCompanionSetting()
    getEventsHistory(1)
      .then((h) => { unreadCount = h.unread_count ?? 0 })
      .catch(() => {})
    startGlobalStream()
  }

  async function loadCompanionSetting() {
    if (needsSetup || loginRequired) {
      companionEnabled = false
      return
    }
    try {
      const config = await getConfigSchema()
      companionEnabled = companionEnabledFromConfigValues(config.effective_values || config.values)
    } catch {
      companionEnabled = false
    }
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
    if (companionReactionTimer) clearTimeout(companionReactionTimer)
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
      {#await loadRouteComponent('chat')}
        <div class="route-loading">Loading...</div>
      {:then module}
        {@const ChatRoute = module.default}
        {#key aiPrompt}
          <ChatRoute sessionId={route.sessionId} onNavigate={navigate} initialPrompt={aiPrompt} />
        {/key}
      {:catch}
        <div class="route-error">Could not load console page.</div>
      {/await}
    {:else if route.view === 'session-lineage'}
      {#await loadRouteComponent('session-lineage')}
        <div class="route-loading">Loading...</div>
      {:then module}
        {@const SessionLineageRoute = module.default}
        <SessionLineageRoute onNavigate={navigate} />
      {:catch}
        <div class="route-error">Could not load console page.</div>
      {/await}
    {:else if route.view === 'tasks'}
      {#await loadRouteComponent('tasks')}
        <div class="route-loading">Loading...</div>
      {:then module}
        {@const PlansRoute = module.default}
        <PlansRoute onNavigate={navigate} />
      {:catch}
        <div class="route-error">Could not load console page.</div>
      {/await}
    {:else if route.view === 'agentruntime'}
      {#await loadRouteComponent('agentruntime')}
        <div class="route-loading">Loading...</div>
      {:then module}
        {@const AgentRuntimeRoute = module.default}
        <AgentRuntimeRoute runId={route.runId} tab={route.tab} onNavigate={navigate} />
      {:catch}
        <div class="route-error">Could not load console page.</div>
      {/await}
    {:else if route.view === 'memory'}
      {#await loadRouteComponent('memory')}
        <div class="route-loading">Loading...</div>
      {:then module}
        {@const MemoryRoute = module.default}
        <MemoryRoute onAskAI={navigateWithPrompt} />
      {:catch}
        <div class="route-error">Could not load console page.</div>
      {/await}
    {:else if route.view === 'sysprompt'}
      {#await loadRouteComponent('sysprompt')}
        <div class="route-loading">Loading...</div>
      {:then module}
        {@const SyspromptRoute = module.default}
        <SyspromptRoute />
      {:catch}
        <div class="route-error">Could not load console page.</div>
      {/await}
    {:else if route.view === 'ops' && authRole !== 'user'}
      {#await loadRouteComponent('ops')}
        <div class="route-loading">Loading...</div>
      {:then module}
        {@const OpsRoute = module.default}
        <OpsRoute />
      {:catch}
        <div class="route-error">Could not load console page.</div>
      {/await}
    {:else if route.view === 'cron' && authRole !== 'user'}
      {#await loadRouteComponent('cron')}
        <div class="route-loading">Loading...</div>
      {:then module}
        {@const CronRoute = module.default}
        <CronRoute />
      {:catch}
        <div class="route-error">Could not load console page.</div>
      {/await}
    {:else if route.view === 'logs' && authRole !== 'user'}
      {#await loadRouteComponent('logs')}
        <div class="route-loading">Loading...</div>
      {:then module}
        {@const LogsRoute = module.default}
        <LogsRoute />
      {:catch}
        <div class="route-error">Could not load console page.</div>
      {/await}
    {:else if route.view === 'analytics' && authRole !== 'user'}
      {#await loadRouteComponent('analytics')}
        <div class="route-loading">Loading...</div>
      {:then module}
        {@const AnalyticsRoute = module.default}
        <AnalyticsRoute />
      {:catch}
        <div class="route-error">Could not load console page.</div>
      {/await}
    {:else if route.view === 'config' && authRole !== 'user'}
      {#await loadRouteComponent('config')}
        <div class="route-loading">Loading...</div>
      {:then module}
        {@const ConfigRoute = module.default}
        <ConfigRoute onNavigate={navigate} />
      {:catch}
        <div class="route-error">Could not load console page.</div>
      {/await}
    {:else if route.view === 'pulse' && authRole !== 'user'}
      {#await loadRouteComponent('pulse')}
        <div class="route-loading">Loading...</div>
      {:then module}
        {@const PulseRoute = module.default}
        <PulseRoute onNavigate={navigate} />
      {:catch}
        <div class="route-error">Could not load console page.</div>
      {/await}
    {:else if route.view === 'reflection' && authRole !== 'user'}
      {#await loadRouteComponent('reflection')}
        <div class="route-loading">Loading...</div>
      {:then module}
        {@const ReflectionRoute = module.default}
        <ReflectionRoute />
      {:catch}
        <div class="route-error">Could not load console page.</div>
      {/await}
    {:else if route.view === 'extensions' && authRole !== 'user'}
      {#await loadRouteComponent('extensions')}
        <div class="route-loading">Loading...</div>
      {:then module}
        {@const ExtensionsRoute = module.default}
        <ExtensionsRoute />
      {:catch}
        <div class="route-error">Could not load console page.</div>
      {/await}
    {:else if route.view === 'channels' && authRole !== 'user'}
      {#await loadRouteComponent('channels')}
        <div class="route-loading">Loading...</div>
      {:then module}
        {@const ChannelsRoute = module.default}
        <ChannelsRoute />
      {:catch}
        <div class="route-error">Could not load console page.</div>
      {/await}
    {:else}
      <Home onNavigate={navigate} />
    {/if}
    {#if showCompanion}
      <CompanionPet reaction={companionReaction} routeView={route.view} locale={$locale} onStimulus={handleCompanionStimulus} onAsk={handleCompanionAsk} />
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

  .route-loading,
  .route-error {
    min-height: min(420px, calc(100vh - 180px));
    display: grid;
    place-items: center;
    color: var(--text-secondary);
    font-family: var(--font-display);
  }

  .route-error {
    color: var(--danger);
  }
</style>
