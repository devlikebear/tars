<script lang="ts">
  import { onMount, onDestroy, untrack } from 'svelte'
  import Shell from './components/Shell.svelte'
  import CompanionPet from './components/CompanionPet.svelte'
  import CaseBridge from './components/CaseBridge.svelte'
  import { caseBridge } from './lib/stores/caseBridge'
  import Home from './components/Home.svelte'
  import SessionBoard from './components/SessionBoard.svelte'
  import Onboarding from './components/Onboarding.svelte'
  import Login from './components/Login.svelte'
  import { resolveRoute, type Route } from './lib/router'
  import { loadRouteComponent } from './lib/routeComponents'
  import { canSignOut, loginRequiredFor } from './lib/authGate'
  import { APIRequestError, getAuthWhoami, getConfigSchema, getEventsHistory, getHealthz, logoutAuth, patchConfigValues, streamEvents } from './lib/api'
  import { defaultModeRedirect, focusChromeHidden, focusOwnsShortcut, onboardingModeUpdate } from './lib/focus'
  import { getOwnConsoleBuild, shouldShowUpdateNotice } from './lib/consoleUpdate'
  import type { AuthWhoamiResponse } from './lib/types'
  import {
    companionEnabledFromConfigValues,
    companionFailureFromEvent,
    companionServerCueFromEvent,
    companionWarningFromEvent,
    pruneFailures,
    shouldShowCompanion,
    type CompanionFailure,
    type CompanionServerCue,
  } from './lib/companion'
  import { zenMode } from './lib/zenMode.svelte'
  import { startConsolePresence, stopConsolePresence } from './lib/presence'
  import { t } from './i18n'
  import CommandPalette from './components/CommandPalette.svelte'
  import ShortcutHelp from './components/ShortcutHelp.svelte'
  import { pageCommands, type PageView, type PaletteCommand } from './lib/commands'
  import { chatOnlyActions, matchShortcut, type ShortcutMatch } from './lib/shortcuts'
  import { builtinSlashCommands } from './lib/slash'
  import { isArchived } from './lib/sessionOrganization'
  import { displaySessionTitle } from './lib/sessionLabels'
  import { chatSession } from './lib/stores/chatSession'
  import { chatDock, chatDockPanels, chatDockPanelTitleKeys, type ChatDockPanelID } from './lib/stores/chatDockStore.svelte'
  import { chatCommands, type ChatCommand } from './lib/stores/chatCommandQueue.svelte'
  import { overlays } from './lib/stores/overlays.svelte'
  import { sessionActivity } from './lib/stores/sessionActivity'
  import { focusViewedSession } from './lib/stores/focusViewedSession.svelte'

  let currentPath = $state('/console')
  let route = $state<Route>({ view: 'board' })
  let serverHealth = $state('connecting')
  let needsSetup = $state(false)
  let unreadCount = $state(0)
  let aiPrompt = $state('')
  let authLoading = $state(true)
  let authInfo = $state<AuthWhoamiResponse | null>(null)
  let loginRequired = $state(false)
  let companionEnabled = $state(false)
  // True until the user navigates: only the landing on /console follows
  // console_default_mode, so the board stays reachable from the nav.
  let landing = true
  let companionFailures = $state<CompanionFailure[]>([])
  // The timestamp of the latest warn-severity SSE event — a new value
  // (not a boolean) so CompanionPet can tell a fresh warning apart from
  // the same prop re-read on an unrelated re-render (#1191).
  let companionWarningAt = $state<number | null>(null)
  // The latest validated category "companion" SSE event
  // (companionServerCueFromEvent), with its own timestamp token
  // (`companionServerCueAt`) for the same reason `companionWarningAt`
  // above is a timestamp, not the event itself: CompanionPet must tell a
  // fresh companion event apart from the same prop re-read on an
  // unrelated re-render (#1192).
  let companionServerCue = $state<CompanionServerCue | null>(null)
  let companionServerCueAt = $state<number | null>(null)
  let stopGlobalStream: (() => void) | null = null
  // Update notice (tars#1257): this tab's own console_build (read once
  // from the meta tag the server injected into this tab's index.html —
  // never changes for the life of the tab), the build value currently
  // shown in the notice (null = hidden), and the build the user last
  // dismissed the notice for (so the same new build doesn't come back).
  let ownConsoleBuild = ''
  let consoleUpdateBuild = $state<string | null>(null)
  let dismissedConsoleBuild: string | undefined = undefined
  // Whether the tab is currently hidden, tracked (same pattern as
  // lib/presence.ts's hiddenAt, #1249) so the update-notice check only
  // runs on a real hidden->visible transition, not on every
  // visibilitychange event that fires while the tab was already visible
  // — a multi-page browser context (e.g. a test opening a second tab)
  // can emit one of those without an actual transition.
  let consoleBuildTabHidden = false
  let authRole = $derived(authInfo?.auth_role ?? '')
  let zenActive = $derived(zenMode.active && route.view === 'chat' && !needsSetup && !loginRequired)
  let focusChrome = $derived(focusChromeHidden(route))
  // CASE shows on focus routes too (tars#1247) — only the sidebar stays
  // hidden there (hideNav, below). Settings off, onboarding, login and zen
  // still hide it, same as before.
  let showCompanion = $derived(shouldShowCompanion({ enabled: companionEnabled, needsSetup, loginRequired, zenActive }))
  // The session the chat route has on screen right now — same signal
  // sessionActivity.setViewing uses to mark a session as seen (#971).
  // CASE leaves this session's own pending approval and running turn out
  // of its lines/badge: its thread already shows them (#1194).
  let activeChatSessionId = $derived(route.view === 'chat' ? chatSession.activeSessionId : null)
  // The focus pipeline screen's own session plus its Q&A session
  // (tars#1247), written by FocusPipeline.svelte via focusViewedSession —
  // same "already visible there" treatment as activeChatSessionId above,
  // widened to companionState's set-shaped parameter since a pipeline can
  // have two sessions on screen at once.
  let focusViewedSessionIds = $derived.by(() => {
    if (route.view !== 'focus') return null
    const ids = [focusViewedSession.sessionId, focusViewedSession.qaSessionId].filter((id): id is string => !!id)
    return ids.length ? new Set(ids) : null
  })
  // What CASE is told is "on screen" right now, across every route it
  // shows on: a single chat session, the focus pipeline's session set, or
  // nothing (the board, and every other route).
  let companionViewedSessionIds = $derived(
    route.view === 'chat' ? activeChatSessionId : route.view === 'focus' ? focusViewedSessionIds : null,
  )

  function navigate(path: string) {
    landing = false
    if (path === currentPath) return
    window.history.pushState(null, '', path)
    currentPath = path
    route = resolveRoute(path)
  }

  // Used by MemoryRoute's "Ask AI" to open the chat with a prompt
  // prefilled (tars#1236 removed the companion handoff, the only other
  // caller, which also carried a hidden console_context — CASE no longer
  // navigates anywhere to ask a question, see CompanionPet.svelte).
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
        // A category "companion" event (#1192) is CASE's own face/bubble,
        // not a general alert: it never reaches the notification
        // list/unread count (the server never stores it either — see
        // notify.go's Emit), never becomes a failure/warning line
        // (companionFailureFromEvent/companionWarningFromEvent already
        // guard this too), and never triggers an activity re-poll.
        const isCompanionEvent = (event.category || '').trim().toLowerCase() === 'companion'
        if (!isCompanionEvent) {
          if (!event.coalesced) unreadCount++
          // A new approval question or a cron run ending: refresh live
          // activity now, not on the next poll, so badges and CASE's bubble
          // follow at once. An unattended run's question and its review
          // arrive as ops events.
          if (event.category === 'approval' || event.category === 'ops' || event.category === 'cron') void sessionActivity.poll()
          const failure = companionFailureFromEvent(event, Date.now())
          if (failure) companionFailures = pruneFailures([...companionFailures, failure], Date.now())
          // CASE's `wary` cue (#1191): a warning that is not (yet) a failure.
          if (companionWarningFromEvent(event)) companionWarningAt = Date.now()
          return
        }
        const serverCue = companionServerCueFromEvent(event, Date.now(), companionViewedSessionIds)
        if (serverCue) {
          companionServerCue = serverCue
          companionServerCueAt = Date.now()
        }
      },
      () => {
        serverHealth = 'disconnected'
      },
      () => {
        serverHealth = 'ok'
      },
      () => {
        // The stream reconnecting (not its first open) is one of the two
        // check points the issue calls for (tars#1257) — a server
        // restart always drops this connection, so this is how a tab
        // notices without polling.
        void checkConsoleBuild()
      },
    )
  }

  // Looking at a failure (clicking its line) or using its × both count as
  // handling it: it leaves CASE's list instead of sitting there forever.
  function handleDismissCompanionFailure(key: string) {
    companionFailures = companionFailures.filter((failure) => failure.key !== key)
  }

  // checkConsoleBuild is the other half of the update notice (tars#1257):
  // shouldShowUpdateNotice owns the decision, this just supplies its
  // inputs and fetch. Called on stream reconnect and on the tab
  // becoming visible again (onMount below) — never on a timer; a server
  // restart (the only real cause of a build change) always trips one of
  // those two.
  async function checkConsoleBuild() {
    try {
      const health = await getHealthz()
      const served = health.console_build ?? ''
      if (shouldShowUpdateNotice(ownConsoleBuild, served, dismissedConsoleBuild, import.meta.env.DEV)) {
        consoleUpdateBuild = served
      }
    } catch {
      // healthz fetch failed — the SSE stream already surfaces the
      // disconnect; nothing more to show here.
    }
  }

  function handleReloadConsole() {
    window.location.reload()
  }

  function handleDismissConsoleUpdate() {
    if (consoleUpdateBuild) dismissedConsoleBuild = consoleUpdateBuild
    consoleUpdateBuild = null
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
      loginRequired = loginRequiredFor(authInfo)
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
    sessionActivity.start()
    // Independent of companionEnabled (tars#1249): the presence signal
    // must go even with CASE turned off in config, so it starts here
    // rather than inside CompanionPet.
    startConsolePresence()
  }

  // Console settings from the server config: the companion, and the mode
  // /console lands in (console_default_mode; focus sends it to the focus
  // home). A user-role console cannot read the config and stays advanced.
  async function loadCompanionSetting() {
    if (needsSetup || loginRequired) {
      companionEnabled = false
      return
    }
    try {
      const config = await getConfigSchema()
      const values = config.effective_values || config.values
      companionEnabled = companionEnabledFromConfigValues(values)
      const redirect = landing ? defaultModeRedirect(values?.console_default_mode, route) : null
      if (redirect) {
        window.history.replaceState(null, '', redirect)
        syncFromBrowser()
      }
    } catch {
      companionEnabled = false
    }
  }

  // Finishing the wizard makes focus the default mode (ADR §2) when no mode
  // is set yet, and opens the focus home then. A mode already chosen —
  // advanced included — is kept, and so is re-entry's (changing providers
  // later) landing.
  async function handleOnboardingComplete() {
    const firstSetup = !(route.view === 'onboarding' && route.reentry)
    needsSetup = false
    let toFocus = false
    if (firstSetup) {
      try {
        const config = await getConfigSchema()
        const values = config.effective_values || config.values || {}
        const update = onboardingModeUpdate(values)
        if (update) await patchConfigValues(update)
        toFocus = (update?.console_default_mode ?? values.console_default_mode) === 'focus'
      } catch {
        // Unknown mode: write nothing; the focus home is one click away.
      }
    }
    navigate(toFocus ? '/console/focus' : '/console')
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
    // Ask the server who we are now rather than assuming the login form:
    // with auth off the caller is still the admin and no password exists,
    // so the form could never be passed.
    let after: AuthWhoamiResponse | null = null
    try {
      after = await getAuthWhoami()
    } catch {
      // Unreachable or 401: treat as signed out below.
    }
    if (after && !loginRequiredFor(after)) {
      authInfo = after
      return
    }
    stopGlobalStream?.()
    stopGlobalStream = null
    sessionActivity.stop()
    stopConsolePresence()
    authInfo = null
    loginRequired = true
    unreadCount = 0
    navigate('/console')
  }

  // ⌘K palette and ? help (#968). Both are App-level so they work on every
  // route; chat-only work goes through the chat command queue.
  // Open state lives in the overlay store so the chat rail can open the
  // palette too.
  let paletteOpen = $derived(overlays.paletteOpen)
  let helpOpen = $derived(overlays.helpOpen)

  function requestChat(command: ChatCommand) {
    chatCommands.request(command)
    if (route.view !== 'chat') navigate('/console/chat')
  }

  function openOnChat(action: () => void) {
    if (route.view !== 'chat') navigate('/console/chat')
    action()
  }

  // Session activity (#971): notifications speak the current language and
  // open the session they are about; the chat on screen counts as seen.
  $effect(() => {
    sessionActivity.text = $t.sessionBoard.notifications
  })
  sessionActivity.onOpenSession = (id) => navigate(`/console/chat/${encodeURIComponent(id)}`)
  $effect(() => {
    // The focus pipeline screen counts as seen too (tars#1247) — its own
    // session, same as the chat route's. The hidden Q&A session is not a
    // board/sidebar entry, so it has nothing to mark seen here.
    const viewing = route.view === 'chat' ? activeChatSessionId : route.view === 'focus' ? focusViewedSession.sessionId : null
    untrack(() => sessionActivity.setViewing(viewing))
  })

  // The palette lists sessions, so make sure the list is loaded whenever it opens.
  $effect(() => {
    if (!overlays.paletteOpen) return
    untrack(() => {
      if (chatSession.sessions.length === 0) void chatSession.refreshSessions()
    })
  })

  function buildPaletteCommands(): PaletteCommand[] {
    const tr = $t
    const pageTitles: Record<PageView, string> = {
      board: tr.nav.items.board,
      home: tr.palette.pages.home,
      chat: tr.nav.items.chat,
      focus: tr.nav.items.focus,
      tasks: tr.nav.items.plans,
      agentruntime: tr.nav.items.agentruntime,
      memory: tr.nav.items.memory,
      sysprompt: tr.nav.items.sysprompt,
      ops: tr.nav.items.ops,
      cron: tr.nav.items.cron,
      logs: tr.nav.items.logs,
      analytics: tr.nav.items.analytics,
      config: tr.nav.items.config,
      extensions: tr.nav.items.extensions,
      pulse: tr.nav.items.pulse,
      reflection: tr.nav.items.reflection,
      channels: tr.nav.items.channels,
      onboarding: tr.palette.pages.onboarding,
    }
    const actions: PaletteCommand[] = [
      { id: 'action:new-session', group: 'action', title: tr.palette.actions.newSession, shortcut: 'new-session', run: () => requestChat({ kind: 'new-session' }) },
      { id: 'action:toggle-sidebar', group: 'action', title: tr.palette.actions.toggleSidebar, shortcut: 'toggle-sidebar', run: () => openOnChat(() => chatDock.toggle('sessions')) },
      { id: 'action:toggle-terminal', group: 'action', title: tr.palette.actions.toggleTerminal, shortcut: 'toggle-terminal', run: () => requestChat({ kind: 'toggle-terminal' }) },
      { id: 'action:toggle-zen', group: 'action', title: tr.palette.actions.toggleZen, shortcut: 'toggle-zen', run: () => openOnChat(() => zenMode.toggle()) },
      ...(showCompanion ? [{ id: 'action:case-bridge', group: 'action' as const, title: tr.palette.actions.caseBridge, shortcut: 'case-bridge' as const, run: () => caseBridge.show() }] : []),
      { id: 'action:shortcuts', group: 'action', title: tr.palette.actions.shortcuts, shortcut: 'help', run: () => overlays.openHelp() },
    ]
    // The terminal needs a tab to show, which the toggle action handles.
    const panels: PaletteCommand[] = chatDockPanels
      .filter((panel) => panel.id !== 'terminal' && panel.id !== 'sessions')
      .map((panel) => {
        const id = panel.id as ChatDockPanelID
        return {
          id: `panel:${id}`,
          group: 'panel',
          title: tr.palette.panel(tr.chat.panels[chatDockPanelTitleKeys[id]]),
          keywords: [id],
          run: () => openOnChat(() => chatDock.toggle(id)),
        }
      })
    const sessions: PaletteCommand[] = chatSession.sessions
      .filter((session) => !isArchived(session))
      .map((session) => ({
        id: `session:${session.id}`,
        group: 'session',
        title: displaySessionTitle(session.title, tr.chat.session.newChat) || tr.palette.untitledSession,
        subtitle: session.id.slice(0, 8),
        keywords: [session.id],
        run: () => navigate(`/console/chat/${encodeURIComponent(session.id)}`),
      }))
    // Slash commands run against the open session, so only offer them there.
    const slash: PaletteCommand[] = route.view !== 'chat' ? [] : builtinSlashCommands(tr.chatCommands.slash).map((candidate) => ({
      id: `slash:${candidate.id ?? candidate.command}`,
      group: 'slash',
      title: `/${candidate.command}`,
      subtitle: candidate.title,
      keywords: [candidate.title, candidate.description],
      run: () => requestChat({ kind: 'slash', command: candidate.id ?? candidate.command }),
    }))
    return [...actions, ...pageCommands(pageTitles, authRole, navigate), ...panels, ...sessions, ...slash]
  }

  let paletteCommands = $derived(paletteOpen ? buildPaletteCommands() : [])

  function runShortcut(match: ShortcutMatch) {
    switch (match.action) {
      case 'palette':
        overlays.togglePalette()
        return
      case 'help':
        overlays.openHelp()
        return
      case 'new-session':
        requestChat({ kind: 'new-session' })
        return
      case 'switch-session':
        requestChat({ kind: 'switch-session', index: match.index ?? 0 })
        return
      case 'toggle-terminal':
        requestChat({ kind: 'toggle-terminal' })
        return
      case 'toggle-sidebar':
        chatDock.toggle('sessions')
        return
      case 'toggle-zen':
        zenMode.toggle()
        return
      case 'case-bridge':
        // CASE's Bridge goes with CASE: off when the companion is off.
        if (showCompanion) caseBridge.toggle()
        return
    }
  }

  function onGlobalKeydown(event: KeyboardEvent) {
    if (needsSetup || loginRequired) return
    const match = matchShortcut(event)
    if (match) {
      // While an overlay is open it owns the keyboard, except ⌘K to close.
      if ((paletteOpen || helpOpen) && match.action !== 'palette') return
      // Chat-only shortcuts stay inert on other routes.
      if (chatOnlyActions.has(match.action) && route.view !== 'chat') return
      // The focus pipeline screen uses `?` for its Q&A drawer.
      if (focusOwnsShortcut(match.action, route)) return
      event.preventDefault()
      runShortcut(match)
      return
    }
    if (route.view === 'chat') onChatKeydown(event)
  }

  function onChatKeydown(event: KeyboardEvent) {
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
    // Read once: the meta tag the server injected into this exact page
    // load never changes for the life of the tab (tars#1257).
    ownConsoleBuild = getOwnConsoleBuild()
    // Starting hidden (presence.ts's pendingFirstVisible case — a
    // background tab restored by the browser, or a desktop shell window
    // created hidden at login) means the first visibilitychange this tab
    // ever sees is itself the hidden->visible transition.
    consoleBuildTabHidden = document.visibilityState === 'hidden'
    const onPopState = () => syncFromBrowser()
    window.addEventListener('popstate', onPopState)
    window.addEventListener('keydown', onGlobalKeydown)
    // Coming back to a tab that shows a chat is looking at it.
    const onVisible = () => {
      if (document.visibilityState !== 'visible') {
        consoleBuildTabHidden = true
        return
      }
      sessionActivity.markSeen(sessionActivity.viewing)
      // The tab becoming visible again is the update notice's other
      // check point (tars#1257) — a tab left open overnight through a
      // server restart has no reconnect to catch it if the stream never
      // dropped. Gated on an actual hidden->visible transition (like
      // lib/presence.ts's hiddenAt tracking, #1249) — not every
      // visibilitychange event that fires while already visible, which
      // a multi-page browser context can emit without a real transition.
      if (consoleBuildTabHidden) {
        consoleBuildTabHidden = false
        void checkConsoleBuild()
      }
    }
    document.addEventListener('visibilitychange', onVisible)

    void checkSetupAndMaybeRedirect()
      .then(refreshAuth)
      .then(loadConsoleNotifications)

    return () => {
      window.removeEventListener('popstate', onPopState)
      window.removeEventListener('keydown', onGlobalKeydown)
      document.removeEventListener('visibilitychange', onVisible)
    }
  })

  onDestroy(() => {
    stopGlobalStream?.()
    sessionActivity.stop()
    stopConsolePresence()
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
    hideNav={focusChrome}
    onNavigate={navigate}
    onUnreadChange={(count) => { unreadCount = count }}
    onLogout={canSignOut(authInfo) ? handleLogout : undefined}
    consoleUpdateAvailable={consoleUpdateBuild !== null}
    onReloadConsole={handleReloadConsole}
    onDismissConsoleUpdate={handleDismissConsoleUpdate}
  >
    {#if route.view === 'onboarding'}
      <Onboarding onComplete={handleOnboardingComplete} reentry={route.reentry === true} />
    {:else if route.view === 'board'}
      <SessionBoard onNavigate={navigate} onNewChat={() => requestChat({ kind: 'new-session' })} />
    {:else if route.view === 'home'}
      <Home onNavigate={navigate} />
    {:else if route.view === 'focus'}
      <!-- Every focus screen (home/new task, pipeline, release train,
           templates) centers an 880px column with `margin: 0 auto` inside
           whatever width it is given — this wrapper reserves only as much
           of CASE's own corner on the right (tars#1247) as the centered
           column would otherwise land under, so a wide window keeps the
           column exactly centered (CASE already clears it on its own) and
           only a narrower one nudges it left. Below ~1216px even the
           minimum non-zero gutter would squeeze the column under 880px,
           so there the reservation switches to just the button's own
           footprint and an open bubble free-floats over content instead,
           same as it already does on every other route (see the CSS
           below for the exact numbers and DESIGN.md's Focus "Chrome" item
           for which buttons that can cover there, and why that is fine).
           The reservation depends only on viewport width and whether CASE
           shows at all, never on whether the bubble is currently open, so
           a self-opened bubble never shifts a button out from under a
           click in flight. -->
      <div class="focus-companion-space" class:case-visible={showCompanion}>
        {#await loadRouteComponent('focus')}
          <div class="route-loading">Loading...</div>
        {:then module}
          {@const FocusRoute = module.default}
          <FocusRoute sessionId={route.sessionId} release={route.release === true} templates={route.templates === true} onNavigate={navigate} />
        {:catch}
          <div class="route-error">Could not load console page.</div>
        {/await}
      </div>
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
        <OpsRoute onNavigate={navigate} />
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
      <SessionBoard onNavigate={navigate} onNewChat={() => requestChat({ kind: 'new-session' })} />
    {/if}
    {#if showCompanion}
      <CompanionPet
        activity={sessionActivity.activity}
        failures={companionFailures}
        viewedSessionIds={companionViewedSessionIds}
        routeView={route.view}
        warningAt={companionWarningAt}
        serverCue={companionServerCue}
        serverCueAt={companionServerCueAt}
        onNavigate={navigate}
        onDismissFailure={handleDismissCompanionFailure}
      />
      <CaseBridge route={route} onNavigate={navigate} />
    {/if}
  </Shell>
  {#if paletteOpen}
    <CommandPalette commands={paletteCommands} onClose={() => overlays.closeAll()} />
  {/if}
  {#if helpOpen}
    <ShortcutHelp onClose={() => overlays.closeAll()} />
  {/if}
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

  /* Reserves CASE's own corner on every focus screen (tars#1247) — see
     the comment at the focus route above. Two pieces, both derived from
     CompanionPet.svelte's actual geometry via app.css's shared
     `--case-button-width`/`--case-bubble-width` tokens (never a number
     copied by hand, so this cannot quietly drift out of sync with it):

     `--case-bubble-share` is the bubble's full footprint at and above
     901px — CASE's right offset (--space-5) + its bubble width + a gap —
     the same geometry the chat workbench's own `.beside-rail` reserves.
     The focus column already centers itself with `margin: 0 auto`, so
     the *only* padding actually needed is enough to pull a centered
     880px column's own right edge left of that share: solving for the
     viewport width V where the column's centered right edge, after
     adding padding P, lands exactly at the bubble share
     (`(V - P + 880) / 2 = V - share` ⟺ `P = 880 + 2*share - V`) gives the
     formula below. A wide enough window (>= 880 + 2*share, currently
     ~1552px) already centers clear on its own, hence `max(0px, …)`: at
     1440px wide that is ~112px of padding — half that (56px) of actual
     leftward shift, not the full bubble share.

     Below the viewport width where that same formula would squeeze the
     centered column under 880px itself (solving `V - P(V) = 880` with
     the formula above gives V ≈ 1216px) there is no point reserving the
     bubble's full width any more — continuing to would only narrow an
     already-narrow column further for no reason, since a column that
     thin no longer reads as "centered" either. Below that width the
     reservation becomes just `--case-button-share` (the button's own
     footprint), and an open bubble free-floats over whatever is under
     it there, exactly like it already does on every non-focus route —
     DESIGN.md's Focus "Chrome" item names which buttons that can cover
     and why that is an accepted tradeoff (closing the bubble clears
     them). */
  .focus-companion-space {
    box-sizing: border-box;
    width: 100%;
    min-height: 100%;
    --case-bubble-share: calc(var(--space-5) + var(--case-bubble-width) + var(--space-4));
    --case-button-share: calc(var(--space-5) + var(--case-button-width) + var(--space-2));
  }

  .focus-companion-space.case-visible {
    padding-right: max(0px, calc(880px + (2 * var(--case-bubble-share)) - 100vw));
  }

  @media (max-width: 1215px) {
    .focus-companion-space.case-visible {
      padding-right: var(--case-button-share);
    }
  }
</style>
