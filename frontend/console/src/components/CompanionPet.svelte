<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { t } from '../i18n'
  import {
    companionActionFor,
    companionCueDismissServer,
    companionCueTick,
    companionCuesFor,
    companionExpression,
    companionFailedSessionIds,
    companionGazeApplies,
    companionGazeOffset,
    companionIdleEligible,
    companionIsLongQuiet,
    companionJustArrived,
    companionNextIdle,
    companionShouldAutoClose,
    companionShouldOpen,
    companionShouldRecordInput,
    companionState,
    companionTurnsJustFinished,
    companionWaitingKeys,
    COMPANION_CUE_NONE,
    type CompanionAction,
    type CompanionCueState,
    type CompanionExpression,
    type CompanionFailure,
    type CompanionIdleKind,
    type CompanionLine,
    type CompanionServerCue,
  } from '../lib/companion'
  import type { ActivityMap } from '../lib/sessionBoard'

  interface Props {
    activity?: ActivityMap
    failures?: CompanionFailure[]
    // The session the chat route currently has on screen — its own
    // pending approval and running turn are already visible there, so
    // CASE leaves them out (queued approvals still show; they have no
    // card in any thread).
    activeSessionId?: string | null
    routeView?: string
    // The timestamp of the latest warn-severity SSE event App.svelte saw
    // (companionWarningFromEvent), or null/undefined for none yet. Only the
    // value *changing* counts as a new signal — see the $effect below.
    warningAt?: number | null
    // The latest validated category "companion" SSE event
    // (companionServerCueFromEvent), or null/undefined for none yet
    // (#1192). `serverCueAt` is the edge marker — same pattern as
    // `warningAt` above — since `serverCue` itself is a fresh object on
    // every SSE event and so cannot be compared by identity across renders.
    serverCue?: CompanionServerCue | null
    serverCueAt?: number | null
    onNavigate?: (path: string) => void
    onDismissFailure?: (key: string) => void
    onAsk?: (prompt: string) => void
  }

  let {
    activity = {},
    failures = [],
    activeSessionId = null,
    routeView = 'home',
    warningAt = null,
    serverCue = null,
    serverCueAt = null,
    onNavigate,
    onDismissFailure,
    onAsk,
  }: Props = $props()
  let open = $state(false)
  let draft = $state('')
  // Reactive, not a one-off Date.now(): an expiring failure must actually
  // drop out of the snapshot over time, not only when a new activity poll
  // or SSE event happens to re-run this. The effect below keeps it moving
  // for as long as a failure is on screen, and the cue timer below keeps
  // it moving for as long as a cue or the long-quiet check needs it.
  let now = $state(Date.now())
  // Plain variables, not $state: they are bookkeeping for the effects
  // below, not something the template reads, so updating them must not
  // re-trigger the effect that reads them.
  let prevWaitingKeys = new Set<string>()
  // False until the first evaluation has run once: whatever is already
  // waiting the first time CASE renders (a reload, leaving zen mode, a
  // fresh login) is already-there, not new, so it must not pop the bubble
  // open — only a wait that appears after that counts as new.
  let primed = false
  // Whether the *current* open=true came from the bubble opening itself,
  // not from the user clicking the button. Only a self-opened bubble ever
  // closes itself; the user's own open is left alone either way.
  let autoOpened = false
  let failureTimer: ReturnType<typeof setInterval> | null = null
  let snapshot = $derived(companionState(activity, failures, now, $t.companion.lines, activeSessionId))
  let hasFailureLines = $derived(snapshot.lines.some((line) => line.kind === 'failure'))

  // --- Cues (#1191): companionCueTick/companionCuesFor turn real events
  // (a turn finishing, a warning, a long-hidden tab becoming visible, a
  // long quiet stretch) into the cues companionExpression reads when
  // `snapshot.lines` has nothing stronger to say.
  let cueState = $state<CompanionCueState>(COMPANION_CUE_NONE)
  let lastInputAt = $state(Date.now())
  let hasWaitingLines = $derived(snapshot.lines.length > 0)
  let longQuiet = $derived(companionIsLongQuiet(now, lastInputAt, hasWaitingLines))
  // `cueState` only changes when a new edge fires (justFinished/warning/
  // justArrived, below) — nothing re-runs companionCueTick on every tick
  // of `now` to notice its own expiry passing. Reading the expiry against
  // the ticking `now` here, instead of trusting `cueState.cue` as already
  // cleared, is what makes a cue actually fade back to neutral (or
  // whatever `longQuiet` says) on its own once its few seconds are up.
  let liveCue = $derived(
    cueState.expiresAt !== null && now >= cueState.expiresAt ? null : cueState.cue,
  )
  // `cueState.server` fades along with `liveCue` once `now` passes
  // `expiresAt` — without this, the server line/expression would keep
  // reading from a cueState the ticking clock already considers expired.
  let liveServer = $derived(
    cueState.expiresAt !== null && now >= cueState.expiresAt ? undefined : cueState.server,
  )
  let cues = $derived(companionCuesFor({ cue: liveCue, expiresAt: cueState.expiresAt, server: liveServer }, longQuiet))
  let expression = $derived(companionExpression(snapshot, cues))
  // The bubble's server line (#1192): only set while the cue is live and
  // the event actually carried a line (body_only cues are expression-only
  // and never show here).
  let serverLine = $derived(liveCue === 'server' && liveServer?.line ? liveServer : undefined)
  let hasServerLine = $derived(!!serverLine)

  // Starts empty, not a read of the `activity` prop: an empty map has no
  // `running` entries to compare against, so the first effect run below
  // never finds a false "just finished" transition at mount.
  let prevActivityForCue: ActivityMap = {}
  let lastSeenWarningAt: number | null = null
  let lastSeenServerCueAt: number | null = null
  let hiddenAt: number | null = null
  let cueTimer: ReturnType<typeof setInterval> | null = null

  // --- Motion (#1191): one-shot actions on an expression change, idle
  // play while neutral and idle, and the cursor-gaze eye offset. All three
  // are off under `prefers-reduced-motion: reduce` — their timers/listeners
  // are never even started, not merely visually suppressed.
  let prefersReducedMotion = $state(false)
  let action = $state<CompanionAction | null>(null)
  let idlePlay = $state<CompanionIdleKind | null>(null)
  let gazeOffset = $state<{ x: number; y: number }>({ x: 0, y: 0 })
  let previousExpression: CompanionExpression | null = null
  let figureEl: HTMLSpanElement | null = null
  let idleTimer: ReturnType<typeof setTimeout> | null = null
  let idleFireAt: number | null = null
  let rafHandle: number | null = null
  let pendingPointer: { x: number; y: number } | null = null

  const ACTION_ANIMATION_NAMES = new Set([
    'companionActBounce',
    'companionActNod',
    'companionActShake',
    'companionActTilt',
    'companionAntennaWave',
  ])
  const IDLE_ANIMATION_NAMES: Record<string, CompanionIdleKind> = {
    companionLookAround: 'look',
    companionYawn: 'yawn',
    companionWink: 'wink',
  }

  // The bubble opens by itself only for a new approval wait or a new
  // failure — never for a running turn or one finishing — and closes
  // itself again once the thing that opened it is gone (answered,
  // navigated to, dismissed, or expired).
  $effect(() => {
    const lines = snapshot.lines
    if (companionShouldOpen(primed, prevWaitingKeys, lines)) {
      open = true
      autoOpened = true
    }
    if (companionShouldAutoClose(autoOpened, lines, hasServerLine)) {
      open = false
      autoOpened = false
    }
    prevWaitingKeys = companionWaitingKeys(lines)
    primed = true
  })

  // A failure's 30-minute window only closes when something re-evaluates
  // the snapshot. Run a timer that refreshes `now` while a failure is
  // showing, so an untouched failure actually disappears from the badge
  // and the bubble once it expires, instead of waiting on the next
  // unrelated activity poll or event. The timer stops itself once there is
  // nothing left to expire, and never starts at all when nothing failed.
  $effect(() => {
    if (hasFailureLines) {
      if (!failureTimer) failureTimer = setInterval(() => { now = Date.now() }, 60_000)
    } else if (failureTimer) {
      clearInterval(failureTimer)
      failureTimer = null
    }
  })

  // A running turn (in some other session) disappearing from `activity`
  // without landing in a recent failure is a finished turn — companionCueTick
  // fires `justFinished` once for that edge.
  $effect(() => {
    const finished = companionTurnsJustFinished(
      prevActivityForCue,
      activity,
      activeSessionId,
      companionFailedSessionIds(failures, Date.now()),
    )
    prevActivityForCue = activity
    if (finished) cueState = companionCueTick(cueState, Date.now(), { justFinished: true })
  })

  // `warningAt` is a timestamp, not a boolean: only the value *changing* to
  // something new is a fresh warning signal — the same timestamp re-read
  // on an unrelated re-render must not replay the cue.
  $effect(() => {
    if (warningAt != null && warningAt !== lastSeenWarningAt) {
      lastSeenWarningAt = warningAt
      cueState = companionCueTick(cueState, Date.now(), { warning: true })
    }
  })

  // `serverCueAt` is the edge marker for `serverCue` (#1192), same pattern
  // as `warningAt` above. A line opens the bubble by itself, same as a new
  // approval wait or failure; a body_only cue (no line) only changes the
  // face.
  $effect(() => {
    if (serverCueAt != null && serverCueAt !== lastSeenServerCueAt) {
      lastSeenServerCueAt = serverCueAt
      if (serverCue) {
        cueState = companionCueTick(cueState, Date.now(), { server: serverCue })
        if (serverCue.line) {
          open = true
          autoOpened = true
        }
      }
    }
  })

  // A one-shot action plays exactly once per expression *change* into one
  // of the five action expressions, and not again while that expression
  // holds (companionActionFor returns null for a no-op transition).
  $effect(() => {
    const current = expression
    const next = companionActionFor(previousExpression, current)
    previousExpression = current
    if (next && !prefersReducedMotion) action = next
  })

  // Idle play is only ever started while `expression === 'neutral'`
  // (companionIdleEligible, in fireIdle below), but a real event can move
  // `expression` off neutral while a look/yawn/wink is still mid-flight —
  // unlike the one-shot actions above (whose keyframes never touch a
  // property an expr-* rule also sets), look/yawn/wink animate the same
  // eye/mouth width, height and border-radius an expression's own shape
  // depends on, so a stale idlePlay must clear immediately here, not wait
  // for its own `animationend` — which will not come anyway, since the CSS
  // for these classes is itself scoped to `.expr-neutral` and losing that
  // class cancels the animation (`animationcancel`) instead of finishing
  // it. Without this, CASE's face would show a look/yawn/wink shape
  // fighting a real alert/upset/etc. expression for the rest of its
  // ~0.5-1.4s, and the stale class could even replay unprompted the next
  // time CASE returns to neutral.
  $effect(() => {
    if (expression !== 'neutral' && idlePlay) idlePlay = null
  })

  function tickNow() {
    now = Date.now()
  }

  function startCueTimer() {
    if (cueTimer) return
    cueTimer = setInterval(tickNow, 1_000)
  }

  function stopCueTimer() {
    if (cueTimer) {
      clearInterval(cueTimer)
      cueTimer = null
    }
  }

  function scheduleIdle(delayMs: number) {
    if (idleTimer) clearTimeout(idleTimer)
    idleFireAt = Date.now() + delayMs
    idleTimer = setTimeout(fireIdle, delayMs)
  }

  function fireIdle() {
    idleTimer = null
    idleFireAt = null
    const next = companionNextIdle(Math.random)
    if (!prefersReducedMotion && companionIdleEligible(expression, snapshot.lines.length, open)) {
      idlePlay = next.kind
    }
    scheduleIdle(next.delayMs)
  }

  function pauseIdle() {
    if (idleTimer) {
      clearTimeout(idleTimer)
      idleTimer = null
    }
  }

  // Resumes the *same* schedule from wherever it left off (the absolute
  // `idleFireAt` survives the pause), rather than rolling a fresh delay —
  // pausing on a hidden tab must not reset how soon CASE plays next.
  function resumeIdle() {
    if (prefersReducedMotion || idleTimer) return
    if (idleFireAt === null) {
      scheduleIdle(companionNextIdle(Math.random).delayMs)
      return
    }
    idleTimer = setTimeout(fireIdle, Math.max(0, idleFireAt - Date.now()))
  }

  function applyGaze() {
    rafHandle = null
    if (!pendingPointer || !figureEl) return
    if (!companionGazeApplies(expression)) {
      gazeOffset = { x: 0, y: 0 }
      return
    }
    const rect = figureEl.getBoundingClientRect()
    gazeOffset = companionGazeOffset({ x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 }, pendingPointer)
  }

  function onPointerMove(event: PointerEvent) {
    pendingPointer = { x: event.clientX, y: event.clientY }
    if (rafHandle !== null) return
    rafHandle = requestAnimationFrame(applyGaze)
  }

  function startGaze() {
    window.addEventListener('pointermove', onPointerMove, { passive: true })
  }

  function stopGaze() {
    window.removeEventListener('pointermove', onPointerMove)
    if (rafHandle !== null) {
      cancelAnimationFrame(rafHandle)
      rafHandle = null
    }
    pendingPointer = null
    gazeOffset = { x: 0, y: 0 }
  }

  function startDecorativeMotion() {
    if (prefersReducedMotion) return
    resumeIdle()
    startGaze()
  }

  function stopDecorativeMotion() {
    pauseIdle()
    stopGaze()
  }

  function handleBodyAnimationEnd(event: AnimationEvent) {
    if (ACTION_ANIMATION_NAMES.has(event.animationName)) action = null
    const idleKind = IDLE_ANIMATION_NAMES[event.animationName]
    if (idleKind && idlePlay === idleKind) idlePlay = null
  }

  function handleVisibilityChange() {
    if (document.hidden) {
      hiddenAt = Date.now()
      stopCueTimer()
      stopDecorativeMotion()
      stopInputTracking()
      return
    }
    const wasHiddenForMs = hiddenAt !== null ? Date.now() - hiddenAt : 0
    hiddenAt = null
    now = Date.now()
    if (companionJustArrived(wasHiddenForMs)) {
      cueState = companionCueTick(cueState, now, { justArrived: true })
    }
    startCueTimer()
    startDecorativeMotion()
    startInputTracking()
  }

  // "Input" for longQuiet/sleepy is any sign of a present reader, not only
  // clicks and keystrokes: a drifting pointer or a scroll counts too — a
  // reader who is only looking, not clicking, must not have CASE doze off
  // under them. pointermove/wheel fire far more than once a second, so
  // companionShouldRecordInput throttles the $state write to at most once
  // per second (pure, tested); the very first event after a long quiet
  // stretch always passes, which is what wakes a sleeping CASE immediately.
  function handleInput() {
    const eventNow = Date.now()
    if (companionShouldRecordInput(lastInputAt, eventNow)) lastInputAt = eventNow
  }

  // This tracking runs regardless of reduced motion — longQuiet/sleepy is
  // an expression change (shape only), not an action, so it does not lean
  // on the cursor-gaze pointermove listener (startGaze/stopGaze), which is
  // off under reduced motion. It still stops while the tab is hidden, same
  // as every other listener here.
  function startInputTracking() {
    window.addEventListener('pointerdown', handleInput, { passive: true })
    window.addEventListener('keydown', handleInput)
    window.addEventListener('pointermove', handleInput, { passive: true })
    window.addEventListener('wheel', handleInput, { passive: true })
  }

  function stopInputTracking() {
    window.removeEventListener('pointerdown', handleInput)
    window.removeEventListener('keydown', handleInput)
    window.removeEventListener('pointermove', handleInput)
    window.removeEventListener('wheel', handleInput)
  }

  function handleReducedMotionChange(event: MediaQueryListEvent) {
    prefersReducedMotion = event.matches
    if (prefersReducedMotion) {
      stopDecorativeMotion()
      action = null
      idlePlay = null
    } else if (!document.hidden) {
      startDecorativeMotion()
    }
  }

  onMount(() => {
    const media = window.matchMedia('(prefers-reduced-motion: reduce)')
    prefersReducedMotion = media.matches
    media.addEventListener('change', handleReducedMotionChange)
    document.addEventListener('visibilitychange', handleVisibilityChange)
    startInputTracking()

    // Opening the console is itself an arrival.
    cueState = companionCueTick(cueState, Date.now(), { justArrived: true })
    startCueTimer()
    startDecorativeMotion()

    return () => {
      media.removeEventListener('change', handleReducedMotionChange)
      document.removeEventListener('visibilitychange', handleVisibilityChange)
      stopInputTracking()
      stopCueTimer()
      stopDecorativeMotion()
    }
  })

  onDestroy(() => {
    if (failureTimer) clearInterval(failureTimer)
    if (idleTimer) clearTimeout(idleTimer)
    if (rafHandle !== null) cancelAnimationFrame(rafHandle)
  })

  function toggleOpen() {
    open = !open
    // Whatever opened or closed it now was the user's own click.
    autoOpened = false
  }

  function closeBubble() {
    open = false
    autoOpened = false
    // The user closing the bubble dismisses a showing server line too
    // (#1192) — a no-op when the active cue is not 'server', so this never
    // cuts a warning/justFinished/justArrived cue short.
    cueState = companionCueDismissServer(cueState)
  }

  function goToLine(line: CompanionLine) {
    onNavigate?.(line.path)
    // Looking at a failure counts as handling it: it leaves the list
    // instead of sitting there once the user has already gone to look.
    if (line.kind === 'failure') onDismissFailure?.(line.key)
    open = false
    autoOpened = false
  }

  // The server line (#1192) only ever has a click action when the event
  // carried a session_id — otherwise it is dismissed by closing the
  // bubble only, same as the component props' doc comment promises.
  function goToServerLine() {
    if (serverLine?.sessionId) onNavigate?.(`/console/chat/${serverLine.sessionId}`)
    cueState = companionCueDismissServer(cueState)
    open = false
    autoOpened = false
  }

  function dismissFailure(key: string) {
    onDismissFailure?.(key)
  }

  function submitAsk() {
    const text = draft.trim()
    if (!text) return
    onAsk?.(text)
    draft = ''
  }
</script>

<div class="companion-pet" class:beside-rail={routeView === 'chat'}>
  {#if open}
    <section class="companion-bubble" aria-live="polite">
      <div class="companion-bubble-header">
        <span class="companion-state">{$t.companion.header}</span>
        <button type="button" class="companion-close" aria-label={$t.companion.closeAria} onclick={closeBubble}>&times;</button>
      </div>
      {#if serverLine}
        <!-- The server-requested line (#1192): visually distinct from the
             "waiting on you" lines below — it is CASE saying something, not
             a queue item — and shown above them. A session_id makes it a
             button that navigates there and clears the line; without one
             it only goes away when the bubble is closed. -->
        <div class="companion-server-line-row">
          {#if serverLine.sessionId}
            <!-- No aria-label: the button's own text (the server's
                 message) is the accessible name, same as every other
                 .companion-line button in this list — a label here would
                 replace that text for assistive tech instead of adding to
                 it. -->
            <button type="button" class="companion-line companion-line-server" onclick={goToServerLine}>
              {serverLine.line}
            </button>
          {:else}
            <p class="companion-line companion-line-server companion-line-server-static">{serverLine.line}</p>
          {/if}
        </div>
      {/if}
      {#if snapshot.lines.length === 0}
        {#if !serverLine}
          <p class="companion-empty">{$t.companion.emptyLine}</p>
        {/if}
      {:else}
        <ul class="companion-lines">
          {#each snapshot.lines as line (line.key)}
            <li class="companion-line-row">
              <button type="button" class={`companion-line companion-line-${line.kind}`} onclick={() => goToLine(line)}>
                {line.label}
              </button>
              {#if line.kind === 'failure'}
                <button
                  type="button"
                  class="companion-line-dismiss"
                  aria-label={$t.companion.dismissFailureAria}
                  onclick={() => dismissFailure(line.key)}
                >&times;</button>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
      <form class="companion-ask" onsubmit={(event) => { event.preventDefault(); submitAsk() }}>
        <input
          type="text"
          bind:value={draft}
          placeholder={$t.companion.inputPlaceholder}
          aria-label={$t.companion.inputAria}
        />
        <button type="submit" disabled={!draft.trim()} aria-label={$t.companion.sendAria}>{$t.companion.send}</button>
      </form>
    </section>
  {/if}

  <!-- The hit area: fixed size and position, no animation or transform of
       its own, so an action or idle play never grows or moves the clickable
       region over a neighbouring button (#1194 was exactly this, for the
       bubble). Everything that moves lives inside .companion-figure, which
       is pointer-events: none. -->
  <button type="button" class="companion-button" aria-label={$t.companion.buttonAria} onclick={toggleOpen}>
    <span class="companion-figure" bind:this={figureEl}>
      <span class="companion-shadow"></span>
      <span
        class={`companion-body expr-${expression}${action ? ` act-${action}` : ''}${idlePlay ? ` play-${idlePlay}` : ''}`}
        aria-hidden="true"
        onanimationend={handleBodyAnimationEnd}
      >
        <span class="companion-antenna"></span>
        <span class="companion-brows">
          <span class="companion-brow companion-brow-left"></span>
          <span class="companion-brow companion-brow-right"></span>
        </span>
        <span class="companion-eyes" style={`--eye-x: ${gazeOffset.x}px; --eye-y: ${gazeOffset.y}px;`}>
          <span class="companion-eye companion-eye-left"></span>
          <span class="companion-eye companion-eye-right"></span>
        </span>
        <span class="companion-mouth"></span>
        <span class="companion-zzz" aria-hidden="true">z</span>
      </span>
      {#if snapshot.badge > 0}
        <span class="companion-badge" aria-label={$t.companion.badgeAria(snapshot.badge)}>{snapshot.badge}</span>
      {/if}
    </span>
  </button>
</div>

<style>
  .companion-pet {
    position: fixed;
    right: var(--space-5);
    bottom: var(--space-5);
    z-index: 90;
    pointer-events: auto;
    display: grid;
    justify-items: end;
    gap: var(--space-2);
  }

  /* On the chat workbench the panel rail runs down the right edge, inside the
     shell's padding. Keep the pet and its bubble left of it: the bubble opens
     by itself on a new approval wait or failure and would cover the rail's
     lower icons until it closes. Below 900px the rail is a row above the chat. */
  @media (min-width: 901px) {
    .companion-pet.beside-rail {
      right: calc(var(--space-6) + var(--chat-rail-width) + var(--space-2));
    }
  }

  /* Everything that moves lives in .companion-figure below, not here: the
     float, every one-shot action and idle play, and the antenna/breathing
     loops. .companion-button keeps a fixed size and no animation or
     transform of its own, so it is always what actually receives clicks
     (#1194 was a bubble covering a neighbouring button; this is the same
     hazard for motion). */
  .companion-button {
    position: relative;
    width: 74px;
    height: 86px;
    border: 0;
    background: transparent;
    color: inherit;
    cursor: pointer;
    display: grid;
    place-items: end center;
  }

  .companion-button:focus-visible {
    outline: 2px solid var(--primary);
    outline-offset: 3px;
    border-radius: var(--radius-lg);
  }

  .companion-figure {
    /* pointer-events: none keeps the whole floating/animated figure out
       of hit-testing, so .companion-button's own unanimated box is always
       what receives clicks. */
    position: relative;
    width: 100%;
    height: 100%;
    display: grid;
    place-items: end center;
    pointer-events: none;
    animation: companionFloat 4.8s var(--ease-out) infinite;
  }

  .companion-body {
    position: relative;
    width: 64px;
    height: 58px;
    border: 1px solid rgba(var(--primary-rgb), 0.45);
    border-radius: var(--radius-lg);
    background:
      linear-gradient(180deg, rgba(var(--primary-rgb), 0.18), rgba(var(--primary-rgb), 0.04)),
      var(--surface-elevated);
    box-shadow:
      0 12px 28px rgba(0, 0, 0, 0.42),
      inset 0 1px 0 rgba(255, 255, 255, 0.06);
  }

  .companion-antenna {
    position: absolute;
    top: -16px;
    left: 50%;
    width: 2px;
    height: 14px;
    transform: translateX(-50%);
    background: var(--primary-text);
  }

  .companion-antenna::after {
    content: '';
    position: absolute;
    top: -6px;
    left: 50%;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    transform: translateX(-50%);
    background: var(--primary);
    box-shadow: 0 0 14px rgba(var(--primary-rgb), 0.7);
  }

  /* CASE's face (#1190): eyebrows, eyes, mouth and the antenna tip are
     always in the DOM — only the `.expr-<name>` class on `.companion-body`
     (from lib/companion.ts `companionExpression`) changes, and every block
     below changes shape, never only colour, so the eight expressions read
     as different faces rather than one face with a different light on it.
     When a state colour is useful it rides only on the antenna tip
     (`.companion-antenna::after`) — nowhere else on the face carries
     colour as its only difference, so there is no second "mouth" made of
     a colour chip sitting next to the real one. */

  .companion-brows {
    position: absolute;
    top: 12px;
    left: 50%;
    display: flex;
    width: 38px;
    justify-content: space-between;
    transform: translateX(-50%);
  }

  .companion-brow {
    width: 9px;
    height: 2px;
    border-radius: 999px;
    background: var(--primary-text);
    /* Hidden by default: a brow only means something on alert/upset/wary
       below. Everywhere else it is a faint line sitting above the eyes
       for no reason, so it stays invisible instead of being noise. */
    opacity: 0;
  }

  .companion-eyes {
    position: absolute;
    top: 20px;
    left: 50%;
    display: flex;
    width: 38px;
    justify-content: space-between;
    transform: translateX(-50%);
  }

  .companion-eye {
    position: relative;
    width: 9px;
    height: 12px;
    border-radius: 999px;
    background: var(--primary-text);
    box-shadow: 0 0 12px color-mix(in srgb, var(--primary-text) 75%, transparent);
    animation: companionBlink 5.6s infinite;
  }

  /* Cursor gaze (#1191): the eyes lean a few px toward a nearby pointer.
     `translate` is a separate property from `transform`, so it composes
     with whatever an expression's own `transform` already does (working's
     glance, upset's pinch) instead of replacing it — the component sets
     `--eye-x`/`--eye-y` on .companion-eyes (lib/companion.ts
     `companionGazeOffset`), already clamped and zeroed past 200px.
     Scoped to the four expressions whose eyes are pupil-shaped: this
     selector, not just the component's `companionGazeApplies` check, is
     what actually keeps a stale non-zero offset from applying to
     happy/upset/sleepy/greeting's curve/line/star eyes. */
  .companion-body:not(.expr-happy):not(.expr-upset):not(.expr-sleepy):not(.expr-greeting) .companion-eye {
    translate: var(--eye-x, 0px) var(--eye-y, 0px);
  }

  .companion-mouth {
    position: absolute;
    top: 38px;
    left: 50%;
    width: 14px;
    height: 2px;
    border-radius: 999px;
    background: var(--primary-text);
    opacity: 0.85;
    transform: translateX(-50%);
  }

  /* The sleepy 'z' — hidden everywhere else, shown only by .expr-sleepy
     below. pointer-events: none like the rest of .companion-figure's
     contents; it is decoration, never a hit target. */
  .companion-zzz {
    position: absolute;
    top: 0;
    right: -8px;
    font-family: var(--font-mono);
    font-size: 10px;
    line-height: 1;
    color: var(--primary-text);
    opacity: 0;
    pointer-events: none;
  }

  /* neutral (idle, the default): the base shapes above as-is. The rule
     below only reaffirms the base mouth width so every expression has its
     own .expr-<name> rule to point at. */
  .expr-neutral .companion-mouth {
    width: 14px;
  }

  /* working (a turn is running): eyes glance up and to one side (a
     definite look, not a 3px nudge), the mouth closes to a small pursed
     line, and the antenna tip becomes a ring instead of a filled dot —
     three shapes differ from neutral at once. The tip also pulses slowly
     for as long as the turn runs (#1191) — the one continuous, repeating
     motion among the eight expressions, as opposed to the one-shot
     actions the other five play once on arrival. */
  .expr-working .companion-eye {
    transform: translate(4px, -3px);
  }

  .expr-working .companion-mouth {
    width: 6px;
  }

  .expr-working .companion-antenna::after {
    box-sizing: border-box;
    background: transparent;
    border: 2px solid var(--primary-text);
    animation: companionAntennaPulse 1.8s ease-in-out infinite;
  }

  /* alert (something is waiting on you): eyes open wide and stop
     blinking, both brows lift, the mouth is a round "oh", the antenna
     tip turns warning-coloured and brightens. */
  .expr-alert .companion-eye {
    width: 12px;
    height: 14px;
    animation: none;
  }

  .expr-alert .companion-brow {
    opacity: 0.75;
    transform: translateY(-1px);
  }

  .expr-alert .companion-mouth {
    top: 34px;
    width: 10px;
    height: 10px;
    background: transparent;
    border: 2px solid var(--primary-text);
    border-radius: 50%;
    opacity: 1;
  }

  .expr-alert .companion-antenna::after {
    background: var(--warning);
    box-shadow: 0 0 18px rgba(245, 197, 66, 0.9);
  }

  /* happy (a turn just finished): eyes curve upward, a wide smile, the
     antenna tip turns the same colour as a finished-OK state. */
  .expr-happy .companion-eye {
    width: 10px;
    height: 6px;
    background: transparent;
    box-shadow: none;
    border-radius: 0 0 999px 999px;
    border-bottom: 2px solid var(--primary-text);
    animation: none;
  }

  .expr-happy .companion-mouth {
    top: 33px;
    width: 18px;
    height: 9px;
    background: transparent;
    border-bottom: 2px solid var(--primary-text);
    border-radius: 0 0 999px 999px;
    opacity: 1;
  }

  .expr-happy .companion-antenna::after {
    background: var(--success);
    box-shadow: 0 0 14px rgba(63, 212, 180, 0.6);
  }

  /* upset (a failure): eyes pinch inward and flatten, both brows furrow
     down, the mouth curves into a frown, the antenna tip turns
     error-coloured. */
  .expr-upset .companion-eye {
    width: 10px;
    height: 4px;
    border-radius: 2px;
    box-shadow: none;
    animation: none;
  }

  .expr-upset .companion-eye-left {
    transform: rotate(-14deg);
  }

  .expr-upset .companion-eye-right {
    transform: rotate(14deg);
  }

  .expr-upset .companion-brow-left {
    opacity: 0.85;
    transform: rotate(18deg) translate(1px, 1px);
  }

  .expr-upset .companion-brow-right {
    opacity: 0.85;
    transform: rotate(-18deg) translate(-1px, 1px);
  }

  .expr-upset .companion-mouth {
    top: 36px;
    width: 16px;
    height: 7px;
    background: transparent;
    border-top: 2px solid var(--primary-text);
    border-radius: 999px 999px 0 0;
    opacity: 1;
  }

  .expr-upset .companion-antenna::after {
    background: var(--error);
    box-shadow: 0 0 12px rgba(255, 93, 93, 0.55);
  }

  /* wary (a warning): one eyebrow lifts — thick, long and sharply angled,
     not a faint tilt — while that same side's eye narrows to a squint.
     The other eye and the mouth stay close to neutral on purpose: a
     symmetric change reads as "surprised", not "suspicious". */
  .expr-wary .companion-eye-right {
    height: 2px;
    box-shadow: none;
    animation: none;
  }

  .expr-wary .companion-brow-left {
    width: 13px;
    height: 3px;
    opacity: 1;
    transform: rotate(-26deg) translate(1px, -3px);
  }

  .expr-wary .companion-mouth {
    width: 12px;
    transform: translateX(-50%) rotate(6deg);
  }

  .expr-wary .companion-antenna::after {
    background: var(--warning);
    box-shadow: 0 0 14px rgba(245, 197, 66, 0.6);
  }

  /* sleepy (quiet for a while): both eyes close to a thin line, a tiny
     mouth, the antenna tip dims. A slow breathing scale on the body and a
     drifting 'z' (#1191) are the other continuous expression, alongside
     working's antenna pulse. */
  .expr-sleepy .companion-eye {
    height: 2px;
    box-shadow: none;
    animation: none;
  }

  .expr-sleepy.companion-body {
    animation: companionBreathe 3.6s ease-in-out infinite;
  }

  .expr-sleepy .companion-mouth {
    top: 37px;
    width: 4px;
    height: 4px;
    border-radius: 50%;
    opacity: 0.6;
  }

  .expr-sleepy .companion-antenna::after {
    opacity: 0.4;
    box-shadow: none;
  }

  .expr-sleepy .companion-zzz {
    opacity: 0.75;
    animation: companionZzzFloat 2.4s ease-in-out infinite;
  }

  /* greeting (returning, or saying hello): each eye becomes a four-point
     sparkle — a clip-path star, not a "+" (that one read as a broken eye,
     not a glint) — a wide smile, the antenna tip brightens. */
  .expr-greeting .companion-eye {
    width: 11px;
    height: 11px;
    background: var(--primary-text);
    box-shadow: none;
    border-radius: 0;
    animation: none;
    clip-path: polygon(50% 0%, 63% 37%, 100% 50%, 63% 63%, 50% 100%, 37% 63%, 0% 50%, 37% 37%);
    filter: drop-shadow(0 0 5px color-mix(in srgb, var(--primary-text) 70%, transparent));
  }

  .expr-greeting .companion-mouth {
    top: 33px;
    width: 18px;
    height: 8px;
    background: transparent;
    border-bottom: 2px solid var(--primary-text);
    border-radius: 0 0 999px 999px;
    opacity: 1;
  }

  .expr-greeting .companion-antenna::after {
    background: var(--success);
    box-shadow: 0 0 20px rgba(63, 212, 180, 0.9);
  }

  .companion-body.act-bounce {
    /* Actions (#1191): a one-shot class the component (lib/companion.ts
       `companionActionFor`) adds exactly once per transition into its
       expression, and clears on its own `animationend`. Each plays on
       .companion-body except wave below, which moves the antenna instead
       — that one reads as CASE's own gesture, not the body nudging it. */
    animation: companionActBounce 0.5s var(--ease-out);
  }

  .companion-body.act-nod {
    animation: companionActNod 0.6s var(--ease-out);
  }

  .companion-body.act-shake {
    animation: companionActShake 0.5s var(--ease-out);
  }

  .companion-body.act-tilt {
    animation: companionActTilt 0.6s var(--ease-out);
  }

  .companion-body.act-wave .companion-antenna {
    animation: companionAntennaWave 0.6s var(--ease-out);
  }

  .companion-body.expr-neutral.play-look .companion-eye {
    /* Idle play (#1191): look/yawn/wink, a few seconds apart while CASE
       is neutral and idle (lib/companion.ts `companionNextIdle`).
       Decoration only — it never touches the expression or the badge.
       The component only ever adds these classes alongside `expr-neutral`
       (companionIdleEligible), but `idlePlay` is JS state that does not
       clear itself the instant a real event moves `expression` on while
       the idle animation is still running — requiring `.expr-neutral`
       here too, not just on `idlePlay`, is what actually stops a stale
       look/yawn/wink from fighting a real expression's own mouth/eye
       shape for the rest of its ~0.5-1.4s. Same guarantee gaze gets from
       its own `:not(.expr-*)` selector below. */
    animation: companionLookAround 1.4s ease-in-out;
  }

  .companion-body.expr-neutral.play-yawn .companion-mouth {
    animation: companionYawn 1.2s ease-in-out;
  }

  .companion-body.expr-neutral.play-wink .companion-eye-right {
    animation: companionWink 0.5s ease-in-out;
  }

  .companion-shadow {
    position: absolute;
    bottom: 0;
    width: 54px;
    height: 10px;
    border-radius: 50%;
    background: rgba(0, 0, 0, 0.36);
    filter: blur(2px);
  }

  .companion-badge {
    position: absolute;
    top: -4px;
    right: 2px;
    min-width: 18px;
    height: 18px;
    padding: 0 4px;
    border-radius: 999px;
    background: var(--danger);
    color: var(--on-danger, #fff);
    font-size: 0.68rem;
    line-height: 18px;
    text-align: center;
    box-shadow: 0 0 0 2px var(--surface-elevated);
  }

  .companion-bubble {
    width: min(300px, calc(100vw - 32px));
    border: 1px solid rgba(var(--primary-rgb), 0.32);
    border-radius: var(--radius-lg);
    background: color-mix(in srgb, var(--surface-elevated) 94%, black);
    box-shadow: 0 18px 44px rgba(0, 0, 0, 0.42);
    padding: var(--space-3);
    animation: companionBubbleIn 170ms var(--ease-out);
  }

  .companion-bubble-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }

  .companion-state {
    color: var(--muted-text);
    font-size: 0.68rem;
    letter-spacing: 0;
    text-transform: uppercase;
  }

  .companion-close {
    width: 24px;
    height: 24px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-muted);
    color: var(--secondary-text);
    cursor: pointer;
  }

  .companion-empty {
    margin: 0;
    color: var(--muted-text);
    font-size: 0.82rem;
    line-height: 1.4;
  }

  .companion-lines {
    display: grid;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .companion-line-row {
    display: flex;
    align-items: stretch;
    gap: var(--space-1);
  }

  .companion-line {
    flex: 1;
    min-width: 0;
    text-align: left;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-muted);
    color: var(--primary-text);
    cursor: pointer;
    font-size: 0.78rem;
    line-height: 1.3;
    padding: var(--space-1) var(--space-2);
  }

  .companion-line:hover {
    border-color: rgba(var(--primary-rgb), 0.42);
  }

  .companion-line-failure {
    border-color: rgba(229, 62, 62, 0.4);
  }

  /* The server-requested line (#1192): CASE talking, not a queue item —
     tinted with the brand colour instead of the neutral/failure borders
     the waiting lines use, and sits above them with its own gap. */
  .companion-server-line-row {
    margin-bottom: var(--space-1);
  }

  .companion-line-server {
    border-color: rgba(var(--primary-rgb), 0.5);
    background: color-mix(in srgb, var(--primary) 14%, var(--surface-muted));
  }

  .companion-line-server-static {
    margin: 0;
    cursor: default;
  }

  .companion-line-dismiss {
    flex: 0 0 auto;
    width: 24px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-muted);
    color: var(--secondary-text);
    cursor: pointer;
    font-size: 0.78rem;
    line-height: 1;
  }

  .companion-line-dismiss:hover {
    border-color: rgba(229, 62, 62, 0.4);
    color: var(--primary-text);
  }

  .companion-ask {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 34px;
    gap: var(--space-1);
    margin-top: var(--space-2);
  }

  .companion-ask input {
    min-width: 0;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-base);
    color: var(--primary-text);
    padding: 0 var(--space-2);
    font-size: 0.78rem;
  }

  .companion-ask button {
    min-height: 30px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-muted);
    color: var(--secondary-text);
    cursor: pointer;
    font-size: 0.75rem;
  }

  .companion-ask button:hover:not(:disabled) {
    border-color: rgba(var(--primary-rgb), 0.42);
    color: var(--primary-text);
  }

  .companion-ask button:disabled {
    cursor: default;
    opacity: 0.5;
  }

  @keyframes companionFloat {
    0%, 100% { transform: translateY(0); }
    50% { transform: translateY(-7px); }
  }

  @keyframes companionBubbleIn {
    from {
      opacity: 0;
      transform: translateY(6px) scale(0.98);
    }
    to {
      opacity: 1;
      transform: translateY(0) scale(1);
    }
  }

  @keyframes companionBlink {
    0%, 44%, 50%, 100% { transform: scaleY(1); }
    47% { transform: scaleY(0.12); }
  }

  /* working's continuous antenna pulse. */
  @keyframes companionAntennaPulse {
    0%, 100% { opacity: 1; transform: translateX(-50%) scale(1); }
    50% { opacity: 0.55; transform: translateX(-50%) scale(1.25); }
  }

  /* sleepy's continuous breathing + drifting 'z'. */
  @keyframes companionBreathe {
    0%, 100% { transform: scale(1); }
    50% { transform: scale(1.035); }
  }

  @keyframes companionZzzFloat {
    0% { opacity: 0; transform: translateY(0); }
    30% { opacity: 0.75; }
    100% { opacity: 0; transform: translateY(-10px); }
  }

  /* One-shot actions (#1191), one per expression that has one: alert
     bounces, happy nods, upset shakes, wary tilts, greeting's antenna
     waves. */
  @keyframes companionActBounce {
    0%, 100% { transform: translateY(0); }
    30% { transform: translateY(-10px); }
    55% { transform: translateY(0); }
    75% { transform: translateY(-4px); }
  }

  @keyframes companionActNod {
    0%, 100% { transform: rotate(0deg); }
    35% { transform: rotate(8deg); }
    70% { transform: rotate(-4deg); }
  }

  @keyframes companionActShake {
    0%, 100% { transform: translateX(0); }
    20% { transform: translateX(-5px); }
    40% { transform: translateX(5px); }
    60% { transform: translateX(-3px); }
    80% { transform: translateX(3px); }
  }

  @keyframes companionActTilt {
    0%, 100% { transform: rotate(0deg); }
    40% { transform: rotate(-9deg); }
    75% { transform: rotate(4deg); }
  }

  @keyframes companionAntennaWave {
    0%, 100% { transform: translateX(-50%) rotate(0deg); }
    25% { transform: translateX(-50%) rotate(18deg); }
    50% { transform: translateX(-50%) rotate(-14deg); }
    75% { transform: translateX(-50%) rotate(10deg); }
  }

  /* Idle play (#1191): look/yawn/wink, each a few hundred ms to a second
     and a half, purely decorative. */
  @keyframes companionLookAround {
    0%, 100% { transform: translateX(0); }
    25% { transform: translateX(-3px); }
    50% { transform: translateX(0); }
    75% { transform: translateX(3px); }
  }

  @keyframes companionYawn {
    0%, 100% { height: 2px; width: 14px; border-radius: 999px; }
    50% { height: 10px; width: 10px; border-radius: 50%; }
  }

  @keyframes companionWink {
    0%, 100% { transform: scaleY(1); }
    50% { transform: scaleY(0.1); }
  }

  @media (max-width: 700px) {
    .companion-pet {
      right: var(--space-3);
      bottom: var(--space-3);
    }

    .companion-button {
      transform: scale(0.88);
      transform-origin: right bottom;
    }
  }

  /* Reduced motion stops every animation here — every selector listed
     above with a repeating or one-shot animation gets its own `animation:
     none` entry here, named exactly (tests/companionPet.test.ts checks
     this one-for-one). */
  @media (prefers-reduced-motion: reduce) {
    .companion-figure,
    .companion-eye,
    .companion-bubble,
    .expr-working .companion-antenna::after,
    .expr-sleepy.companion-body,
    .expr-sleepy .companion-zzz,
    .companion-body.act-bounce,
    .companion-body.act-nod,
    .companion-body.act-shake,
    .companion-body.act-tilt,
    .companion-body.act-wave .companion-antenna,
    .companion-body.expr-neutral.play-look .companion-eye,
    .companion-body.expr-neutral.play-yawn .companion-mouth,
    .companion-body.expr-neutral.play-wink .companion-eye-right {
      animation: none;
    }
  }
</style>
