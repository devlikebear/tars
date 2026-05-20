<script lang="ts">
  import { onDestroy } from 'svelte'
  import {
    companionReactionForStimulus,
    companionUiText,
    type CompanionReaction,
    type CompanionStimulus,
  } from '../lib/companion'

  interface Props {
    reaction?: CompanionReaction | null
    routeView?: string
    locale?: string
    onStimulus?: (stimulus: CompanionStimulus) => void
    onAsk?: (prompt: string) => void
  }

  let { reaction = null, routeView = 'home', locale = 'en', onStimulus, onAsk }: Props = $props()
  let open = $state(false)
  let draft = $state('')
  let localReaction = $state<CompanionReaction>(companionReactionForStimulus('poke', 'home', 'en'))
  let dismissedReaction = $state<CompanionReaction | null>(null)
  let manualPriority = $state(false)
  let activeAction = $state<CompanionStimulus | null>(null)
  let feedbackTick = $state(0)
  let priorityTimer: ReturnType<typeof setTimeout> | null = null
  let labels = $derived(companionUiText(locale))
  let activeReaction = $derived((reaction && reaction !== dismissedReaction) ? reaction : localReaction)
  let eventReactionVisible = $derived(reaction !== null && reaction !== dismissedReaction)
  let bubbleVisible = $derived(open || eventReactionVisible || manualPriority)

  $effect(() => {
    if (!manualPriority && !reaction) {
      localReaction = companionReactionForStimulus('poke', routeView, locale)
    }
  })

  function toggleOpen() {
    open = !open
  }

  function trigger(stimulus: CompanionStimulus) {
    localReaction = companionReactionForStimulus(stimulus, routeView, locale)
    if (reaction) dismissedReaction = reaction
    manualPriority = true
    activeAction = stimulus
    feedbackTick += 1
    open = true
    if (priorityTimer) clearTimeout(priorityTimer)
    priorityTimer = setTimeout(() => {
      manualPriority = false
      activeAction = null
      priorityTimer = null
    }, 9000)
    onStimulus?.(stimulus)
  }

  function closeBubble() {
    open = false
    if (reaction) dismissedReaction = reaction
    manualPriority = false
    activeAction = null
  }

  function submitAsk() {
    const text = draft.trim()
    if (!text) return
    localReaction = {
      mood: 'focus',
      message: locale?.toLowerCase().startsWith('ko') ? '그 자극을 채팅으로 넘길게요.' : 'Opening chat with that stimulus.',
      detail: locale?.toLowerCase().startsWith('ko')
        ? '일반 TARS 대화 경로에 현재 맥락을 붙여서 가져갑니다.'
        : 'I will carry this into the normal TARS conversation path.',
    }
    manualPriority = true
    activeAction = null
    feedbackTick += 1
    onAsk?.(text)
    draft = ''
  }

  onDestroy(() => {
    if (priorityTimer) clearTimeout(priorityTimer)
  })
</script>

<div class={`companion-pet mood-${activeReaction.mood}`} class:companion-reacting={manualPriority}>
  {#if bubbleVisible}
    {#key feedbackTick}
      <section class="companion-bubble" aria-live="polite">
        <div class="companion-bubble-header">
          <span class="companion-state">{labels.moods[activeReaction.mood]}</span>
          <button type="button" class="companion-close" aria-label={labels.closeAria} onclick={closeBubble}>&times;</button>
        </div>
        <p>{activeReaction.message}</p>
        {#if activeReaction.detail}
          <small>{activeReaction.detail}</small>
        {/if}
        {#if activeAction}
          <span class="companion-feedback-strip">{labels.feedbackAck(labels.actions[activeAction])}</span>
        {/if}
        <div class="companion-actions">
          <button type="button" class:active={activeAction === 'poke'} aria-pressed={activeAction === 'poke'} onclick={() => trigger('poke')}>{labels.actions.poke}</button>
          <button type="button" class:active={activeAction === 'suggest'} aria-pressed={activeAction === 'suggest'} onclick={() => trigger('suggest')}>{labels.actions.suggest}</button>
          <button type="button" class:active={activeAction === 'feedback'} aria-pressed={activeAction === 'feedback'} onclick={() => trigger('feedback')}>{labels.actions.feedback}</button>
        </div>
        <form class="companion-ask" onsubmit={(event) => { event.preventDefault(); submitAsk() }}>
          <input
            type="text"
            bind:value={draft}
            placeholder={labels.inputPlaceholder}
            aria-label={labels.inputAria}
          />
          <button type="submit" disabled={!draft.trim()} aria-label={labels.sendAria}>{labels.send}</button>
        </form>
      </section>
    {/key}
  {/if}

  <button type="button" class="companion-button" aria-label={labels.buttonAria} onclick={toggleOpen}>
    <span class="companion-shadow"></span>
    <span class="companion-body" aria-hidden="true">
      <span class="companion-antenna"></span>
      <span class="companion-face">
        <span class="companion-eye"></span>
        <span class="companion-eye"></span>
      </span>
      <span class="companion-glow"></span>
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

  .companion-reacting .companion-button {
    animation:
      companionNod 520ms var(--ease-out),
      companionFloat 4.8s var(--ease-out) 520ms infinite;
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
    border: 1px solid rgba(224, 145, 69, 0.45);
    border-radius: var(--radius-lg);
    background:
      linear-gradient(180deg, rgba(224, 145, 69, 0.18), rgba(224, 145, 69, 0.04)),
      var(--surface-elevated);
    box-shadow:
      0 12px 28px rgba(0, 0, 0, 0.42),
      inset 0 1px 0 rgba(255, 255, 255, 0.06);
  }

  .companion-reacting .companion-body {
    border-color: rgba(224, 145, 69, 0.68);
    box-shadow:
      0 16px 34px rgba(0, 0, 0, 0.46),
      0 0 0 3px rgba(224, 145, 69, 0.12),
      inset 0 1px 0 rgba(255, 255, 255, 0.08);
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
    box-shadow: 0 0 14px rgba(224, 145, 69, 0.7);
  }

  .companion-face {
    position: absolute;
    top: 17px;
    left: 50%;
    display: flex;
    width: 38px;
    justify-content: space-between;
    transform: translateX(-50%);
  }

  .companion-eye {
    width: 9px;
    height: 12px;
    border-radius: 999px;
    background: var(--primary-text);
    box-shadow: 0 0 12px rgba(240, 184, 120, 0.75);
    animation: companionBlink 5.6s infinite;
  }

  .companion-glow {
    position: absolute;
    right: 11px;
    bottom: 10px;
    width: 12px;
    height: 3px;
    border-radius: 999px;
    background: var(--success);
    opacity: 0.75;
  }

  .mood-warn .companion-glow {
    background: var(--warning);
  }

  .mood-error .companion-glow {
    background: var(--danger);
    box-shadow: 0 0 12px rgba(245, 101, 101, 0.55);
  }

  .mood-focus .companion-glow,
  .mood-spark .companion-glow {
    background: var(--primary);
    box-shadow: 0 0 12px rgba(224, 145, 69, 0.65);
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

  .companion-bubble {
    width: min(300px, calc(100vw - 32px));
    border: 1px solid rgba(224, 145, 69, 0.32);
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

  .companion-bubble p {
    margin: 0;
    color: var(--primary-text);
    font-size: 0.86rem;
    line-height: 1.45;
  }

  .companion-bubble small {
    display: block;
    margin-top: var(--space-1);
    color: var(--muted-text);
    font-size: 0.73rem;
    line-height: 1.35;
  }

  .companion-feedback-strip {
    display: block;
    margin-top: var(--space-2);
    border: 1px solid rgba(224, 145, 69, 0.24);
    border-radius: var(--radius-sm);
    background: rgba(224, 145, 69, 0.1);
    color: var(--primary-text);
    font-size: 0.72rem;
    line-height: 1.25;
    padding: 5px var(--space-2);
    animation: companionPulse 680ms var(--ease-out);
  }

  .companion-actions {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--space-1);
    margin-top: var(--space-3);
  }

  .companion-actions button,
  .companion-ask button {
    min-height: 30px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-muted);
    color: var(--secondary-text);
    cursor: pointer;
    font-size: 0.75rem;
  }

  .companion-actions button:hover,
  .companion-actions button.active,
  .companion-ask button:hover:not(:disabled) {
    border-color: rgba(224, 145, 69, 0.42);
    color: var(--primary-text);
  }

  .companion-actions button.active {
    background: rgba(224, 145, 69, 0.16);
    box-shadow: inset 0 0 0 1px rgba(224, 145, 69, 0.2);
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

  .companion-ask button:disabled {
    cursor: default;
    opacity: 0.5;
  }

  @keyframes companionFloat {
    0%, 100% { transform: translateY(0); }
    50% { transform: translateY(-7px); }
  }

  @keyframes companionNod {
    0% { transform: translateY(0) rotate(0deg) scale(1); }
    35% { transform: translateY(-6px) rotate(-2deg) scale(1.04); }
    70% { transform: translateY(1px) rotate(2deg) scale(0.99); }
    100% { transform: translateY(0) rotate(0deg) scale(1); }
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

  @keyframes companionPulse {
    0% {
      opacity: 0.35;
      transform: translateY(3px);
    }
    100% {
      opacity: 1;
      transform: translateY(0);
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

  @media (prefers-reduced-motion: reduce) {
    .companion-button,
    .companion-eye,
    .companion-bubble,
    .companion-feedback-strip {
      animation: none;
    }
  }
</style>
