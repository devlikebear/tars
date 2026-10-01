<script lang="ts">
  // The card deck (ADR §3, §7): exactly one card at a time, ←/→ to move,
  // `n / m`, 1–9 to choose a decision option, `o` to view the source turn,
  // `?` to ask about the card in its Q&A drawer (§8).
  // Informational cards can be acknowledged together. The deck keeps
  // showing the same card while the list changes under it; a card that
  // leaves the deck hands over to the first one.
  import { untrack } from 'svelte'
  import { t } from '../../i18n'
  import { acknowledgeable, deckCursor, deckOrder, mustHandle, type QAEntry } from '../../lib/focus'
  import type { FocusCard as Card, FocusDecision, FocusGateAction, FocusPlan, SessionMessage } from '../../lib/types'
  import FocusCard from './FocusCard.svelte'
  import FocusQADrawer from './FocusQADrawer.svelte'
  import FocusRawSlice from './FocusRawSlice.svelte'

  interface Props {
    cards: Card[]
    history: SessionMessage[]
    openGate: string
    busy: boolean
    running: boolean
    baseDirs?: string[]
    onGate: (gate: string, action: FocusGateAction, note?: string, edits?: FocusPlan) => void
    onDecide: (card: Card, decision: string) => void
    onSeen: (card: Card) => void
    onAcknowledgeRest: (cards: Card[]) => void
    // Q&A about the card on screen; absent hides the drawer.
    qaThread?: (cardId: string) => QAEntry[]
    qaAnswering?: string | null
    qaError?: string
    onAsk?: (cardId: string, question: string) => Promise<boolean>
    onPromote?: (card: Card, entry: QAEntry) => void
    // The open triage gate's progress (P3), null when none is open.
    triage?: { decided: number; total: number } | null
  }

  let {
    cards,
    history,
    openGate,
    busy,
    running,
    baseDirs = [],
    onGate,
    onDecide,
    onSeen,
    onAcknowledgeRest,
    qaThread,
    qaAnswering = null,
    qaError = '',
    onAsk,
    triage = null,
    onPromote,
  }: Props = $props()

  // The drawer stays open across cards once opened; `?` opens and focuses it.
  let qaOpen = $state(false)
  let qaInput = $state<HTMLTextAreaElement | null>(null)

  let currentId = $state<string | null>(null)
  let showRaw = $state(false)
  // The order on screen (lib/focus deckOrder): held while the same cards
  // stay, re-sorted when cards arrive or leave. The ids last seen tell an
  // arrival, which moves the deck to its first card (deckCursor).
  let order = $state<string[]>([])
  let known = new Set<string>()

  $effect(() => {
    const ids = cards.map((c) => c.id)
    untrack(() => {
      order = deckOrder(order, ids)
      const byId = new Map(cards.map((c) => [c.id, c]))
      currentId = deckCursor(known, order.map((id) => byId.get(id)!).filter(Boolean), currentId)
      known = new Set(ids)
    })
  })

  // The cards in the order on screen; until the effect has run for a new
  // deck, the sorted order.
  let shown = $derived.by(() => {
    const byId = new Map(cards.map((c) => [c.id, c]))
    const held = order.map((id) => byId.get(id)).filter((c): c is Card => !!c)
    return held.length === cards.length ? held : cards
  })
  let index = $derived.by(() => {
    const at = currentId ? shown.findIndex((c) => c.id === currentId) : -1
    return at >= 0 ? at : 0
  })
  let current = $derived(shown[index] ?? null)
  let rest = $derived(acknowledgeable(cards))

  // The card on screen is seen.
  $effect(() => {
    const card = current
    if (card) untrack(() => onSeen(card))
  })

  // A different card closes the raw view.
  $effect(() => {
    void current?.id
    untrack(() => { showRaw = false })
  })

  function move(step: number) {
    if (shown.length === 0) return
    const next = (index + step + shown.length) % shown.length
    currentId = shown[next].id
  }

  function decide(card: Card, decision: string) {
    // A handled card hands over to the next one on screen; cards the
    // decision brings (the answer turn's) move the deck to its first card.
    const at = shown.findIndex((c) => c.id === card.id)
    if (at >= 0 && at + 1 < shown.length) currentId = shown[at + 1].id
    onDecide(card, decision)
  }

  function openAsk() {
    qaOpen = true
    // The drawer renders its box on open; focus it on the next frame.
    requestAnimationFrame(() => qaInput?.focus())
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.metaKey || event.ctrlKey || event.altKey) return
    const target = event.target as HTMLElement | null
    if (target?.closest('input, textarea, select, [contenteditable="true"]')) return
    if (event.key === 'ArrowLeft') {
      event.preventDefault()
      move(-1)
    } else if (event.key === 'ArrowRight') {
      event.preventDefault()
      move(1)
    } else if (event.key === 'o' && current) {
      event.preventDefault()
      showRaw = !showRaw
    } else if (event.key === '?' && current && onAsk) {
      event.preventDefault()
      openAsk()
    } else if (/^[1-9]$/.test(event.key) && current?.kind === 'decision' && current.state !== 'decided' && !busy) {
      const option = (current.payload as FocusDecision | undefined)?.options?.[Number(event.key) - 1]
      if (option) {
        event.preventDefault()
        decide(current, option)
      }
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<section class="focus-deck" aria-label={$t.focus.deck.label}>
  {#if !current}
    <p class="deck-empty" data-testid="focus-deck-empty">{running ? $t.focus.screen.emptyRunning : $t.focus.screen.empty}</p>
  {:else}
    <div class="deck-nav">
      <button type="button" class="btn btn-ghost btn-sm" aria-label={$t.focus.deck.prev} title={$t.focus.deck.prev} disabled={shown.length < 2} onclick={() => move(-1)} data-testid="focus-deck-prev">←</button>
      <span class="mono deck-pos" data-testid="focus-deck-position">{$t.focus.deck.position(index + 1, shown.length)}</span>
      <button type="button" class="btn btn-ghost btn-sm" aria-label={$t.focus.deck.next} title={$t.focus.deck.next} disabled={shown.length < 2} onclick={() => move(1)} data-testid="focus-deck-next">→</button>
      {#if triage}
        <span class="mono triage" data-testid="focus-triage-progress">{$t.focus.finding.triage(triage.decided, triage.total)}</span>
      {/if}
      <span class="deck-spacer"></span>
      {#if rest.length > 1 && !mustHandle(current)}
        <button type="button" class="btn btn-ghost btn-sm" disabled={busy} onclick={() => onAcknowledgeRest(rest)} data-testid="focus-ack-rest">{$t.focus.deck.acknowledgeRest(rest.length)}</button>
      {/if}
      <button type="button" class="btn btn-ghost btn-sm" aria-pressed={showRaw} onclick={() => { showRaw = !showRaw }} data-testid="focus-view-raw">
        {showRaw ? $t.focus.deck.hideRaw : $t.focus.deck.viewRaw}
      </button>
    </div>

    {#key current.id}
      <FocusCard card={current} {openGate} {busy} {onGate} onDecide={decide} onAsk={onAsk && qaThread ? openAsk : undefined} />
    {/key}

    {#if showRaw}
      <FocusRawSlice {history} turn={current.turn} {baseDirs} />
    {/if}

    {#if onAsk && qaThread}
      {@const card = current}
      <FocusQADrawer
        cardId={card.id}
        thread={qaThread(card.id)}
        answering={qaAnswering === card.id}
        error={qaError}
        open={qaOpen}
        bind:input={qaInput}
        onToggle={() => { qaOpen = !qaOpen }}
        onAsk={(question) => onAsk(card.id, question)}
        onPromote={(entry) => onPromote?.(card, entry)}
      />
    {/if}

    <p class="deck-keys mono">{$t.focus.deck.keys}</p>
  {/if}
</section>

<style>
  .focus-deck {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .deck-nav {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .deck-pos {
    min-width: 3.5em;
    text-align: center;
    font-size: var(--text-xs);
  }

  .triage {
    font-size: var(--text-xs);
    color: var(--warning);
  }

  .deck-spacer {
    flex: 1;
  }

  .deck-empty {
    margin: 0;
    padding: var(--space-8) var(--space-4);
    text-align: center;
    color: var(--text-tertiary);
    border: 1px dashed var(--border-default);
    border-radius: var(--radius-lg);
  }

  .deck-keys {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }
</style>
