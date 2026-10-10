<script lang="ts">
  import { onDestroy } from 'svelte'
  import { t } from '../i18n'
  import {
    companionExpression,
    companionShouldAutoClose,
    companionShouldOpen,
    companionState,
    companionWaitingKeys,
    type CompanionFailure,
    type CompanionLine,
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
    onNavigate?: (path: string) => void
    onDismissFailure?: (key: string) => void
    onAsk?: (prompt: string) => void
  }

  let {
    activity = {},
    failures = [],
    activeSessionId = null,
    routeView = 'home',
    onNavigate,
    onDismissFailure,
    onAsk,
  }: Props = $props()
  let open = $state(false)
  let draft = $state('')
  // Reactive, not a one-off Date.now(): an expiring failure must actually
  // drop out of the snapshot over time, not only when a new activity poll
  // or SSE event happens to re-run this. The effect below keeps it moving
  // for as long as a failure is on screen.
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
  // CASE's face (#1190). Cues beyond `snapshot.lines` (a just-finished turn,
  // a warning event, a long quiet stretch, a just-arrived user) are P3
  // (#1191) scope — this component does not fill them in yet.
  let expression = $derived(companionExpression(snapshot))

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
    if (companionShouldAutoClose(autoOpened, lines)) {
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

  onDestroy(() => {
    if (failureTimer) clearInterval(failureTimer)
  })

  function toggleOpen() {
    open = !open
    // Whatever opened or closed it now was the user's own click.
    autoOpened = false
  }

  function closeBubble() {
    open = false
    autoOpened = false
  }

  function goToLine(line: CompanionLine) {
    onNavigate?.(line.path)
    // Looking at a failure counts as handling it: it leaves the list
    // instead of sitting there once the user has already gone to look.
    if (line.kind === 'failure') onDismissFailure?.(line.key)
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
      {#if snapshot.lines.length === 0}
        <p class="companion-empty">{$t.companion.emptyLine}</p>
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

  <button type="button" class="companion-button" aria-label={$t.companion.buttonAria} onclick={toggleOpen}>
    <span class="companion-shadow"></span>
    <span class={`companion-body expr-${expression}`} aria-hidden="true">
      <span class="companion-antenna"></span>
      <span class="companion-brows">
        <span class="companion-brow companion-brow-left"></span>
        <span class="companion-brow companion-brow-right"></span>
      </span>
      <span class="companion-eyes">
        <span class="companion-eye companion-eye-left"></span>
        <span class="companion-eye companion-eye-right"></span>
      </span>
      <span class="companion-mouth"></span>
    </span>
    {#if snapshot.badge > 0}
      <span class="companion-badge" aria-label={$t.companion.badgeAria(snapshot.badge)}>{snapshot.badge}</span>
    {/if}
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
    animation: companionFloat 4.8s var(--ease-out) infinite;
  }

  .companion-button:focus-visible {
    outline: 2px solid var(--primary);
    outline-offset: 3px;
    border-radius: var(--radius-lg);
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

  /* neutral (idle, the default): the base shapes above as-is. The rule
     below only reaffirms the base mouth width so every expression has its
     own .expr-<name> rule to point at. */
  .expr-neutral .companion-mouth {
    width: 14px;
  }

  /* working (a turn is running): eyes glance up and to one side (a
     definite look, not a 3px nudge), the mouth closes to a small pursed
     line, and the antenna tip becomes a ring instead of a filled dot —
     three shapes differ from neutral at once. */
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
     mouth, the antenna tip dims. */
  .expr-sleepy .companion-eye {
    height: 2px;
    box-shadow: none;
    animation: none;
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

  /* Reduced motion stops every animation here. */
  @media (prefers-reduced-motion: reduce) {
    .companion-button,
    .companion-eye,
    .companion-bubble {
      animation: none;
    }
  }
</style>
