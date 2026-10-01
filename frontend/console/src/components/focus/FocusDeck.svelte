<script lang="ts">
  // The card deck (ADR §3, §7): exactly one card at a time, ←/→ to move,
  // `n / m`, 1–9 to choose a decision option, `o` to view the source turn.
  // Informational cards can be acknowledged together. The deck keeps
  // showing the same card while the list changes under it; a card that
  // leaves the deck hands over to the first one.
  import { untrack } from 'svelte'
  import { t } from '../../i18n'
  import { acknowledgeable, deckCursor, deckOrder, mustHandle } from '../../lib/focus'
  import type { FocusCard as Card, FocusDecision, FocusGateAction, FocusPlan, SessionMessage } from '../../lib/types'
  import FocusCard from './FocusCard.svelte'
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
  }

  let { cards, history, openGate, busy, running, baseDirs = [], onGate, onDecide, onSeen, onAcknowledgeRest }: Props = $props()

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
      currentId = deckCursor(known, order, currentId)
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
      <span class="deck-spacer"></span>
      {#if rest.length > 1 && !mustHandle(current)}
        <button type="button" class="btn btn-ghost btn-sm" disabled={busy} onclick={() => onAcknowledgeRest(rest)} data-testid="focus-ack-rest">{$t.focus.deck.acknowledgeRest(rest.length)}</button>
      {/if}
      <button type="button" class="btn btn-ghost btn-sm" aria-pressed={showRaw} onclick={() => { showRaw = !showRaw }} data-testid="focus-view-raw">
        {showRaw ? $t.focus.deck.hideRaw : $t.focus.deck.viewRaw}
      </button>
    </div>

    {#key current.id}
      <FocusCard card={current} {openGate} {busy} {onGate} onDecide={decide} />
    {/key}

    {#if showRaw}
      <FocusRawSlice {history} turn={current.turn} {baseDirs} />
    {/if}

    <p class="deck-keys mono">{$t.focus.deck.keys}</p>
  {/if}
</section>

<style>
  .focus-deck {
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
