<script lang="ts">
  // CASE's Bridge (case-companion phase 2): a panel on the right of any
  // screen to ask CASE about TARS as a whole. Opened with Mod+/ , the
  // palette, or CASE's bubble. Answers are plain text, never HTML;
  // cards are links the server checked and built. CASE cannot change
  // anything — the cards only take the user to the screen that can.
  import { onDestroy, tick } from 'svelte'
  import CaseFigure from './CaseFigure.svelte'
  import { t } from '../i18n'
  import { canScopeToScreen, caseBridgeFollowUps, caseBridgeView, type CaseBridgeRoute } from '../lib/caseBridge'
  import { caseBridge } from '../lib/stores/caseBridge'
  import type { CaseBridgeCard } from '../lib/api/caseBridge'
  import type { CompanionTalk } from '../lib/companion'

  interface Props {
    route: CaseBridgeRoute
    onNavigate?: (path: string) => void
  }

  let { route, onNavigate }: Props = $props()

  let draft = $state('')
  let inputEl: HTMLInputElement | null = $state(null)
  let listEl: HTMLDivElement | null = $state(null)

  let view = $derived(caseBridgeView(route))
  let screenScope = $derived(canScopeToScreen(view))
  let last = $derived(caseBridge.entries.at(-1))
  let followUps = $derived(last && last.status !== 'streaming' ? caseBridgeFollowUps(last.cards) : caseBridgeFollowUps([]))
  let talk = $derived<CompanionTalk | null>(last?.status === 'streaming' ? (last.answer ? 'speaking' : 'thinking') : null)
  let where = $derived($t.caseBridge.routes[view.route] ?? $t.caseBridge.routes.other)

  // Focus the input whenever the panel opens; keep the newest answer in
  // view as it streams.
  $effect(() => {
    if (caseBridge.open) void tick().then(() => inputEl?.focus())
  })
  $effect(() => {
    void caseBridge.entries
    if (caseBridge.open) void tick().then(() => listEl?.scrollTo({ top: listEl.scrollHeight }))
  })

  onDestroy(() => caseBridge.dispose())

  async function submit(text = draft) {
    const question = text.trim()
    if (!question || caseBridge.busy) return
    draft = ''
    const sent = await caseBridge.ask(question, view)
    if (!sent && !draft) draft = question
  }

  function go(card: CaseBridgeCard) {
    caseBridge.close()
    onNavigate?.(card.href)
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.preventDefault()
      caseBridge.close()
    }
  }
</script>

{#if caseBridge.open}
  <div class="case-bridge" role="dialog" aria-label={$t.caseBridge.title} tabindex="-1" onkeydown={onKeydown}>
    <header class="case-bridge-header">
      <span class="case-bridge-face" aria-hidden="true">
        <CaseFigure expression={talk ? 'working' : 'neutral'} {talk} />
      </span>
      <span class="case-bridge-titles">
        <strong>{$t.caseBridge.title}</strong>
        <span>{$t.caseBridge.subtitle}</span>
      </span>
      <button type="button" class="case-bridge-close" aria-label={$t.caseBridge.closeAria} onclick={() => caseBridge.close()}>&times;</button>
    </header>

    <div class="case-bridge-context">
      <span class="case-bridge-where">{$t.caseBridge.looking(where)}</span>
      <div class="case-bridge-scope" role="radiogroup" aria-label={$t.caseBridge.scope.label}>
        <button
          type="button"
          role="radio"
          aria-checked={caseBridge.scope === 'all' || !screenScope}
          class:active={caseBridge.scope === 'all' || !screenScope}
          onclick={() => (caseBridge.scope = 'all')}
        >{$t.caseBridge.scope.all}</button>
        <button
          type="button"
          role="radio"
          aria-checked={caseBridge.scope === 'screen' && screenScope}
          class:active={caseBridge.scope === 'screen' && screenScope}
          disabled={!screenScope}
          onclick={() => (caseBridge.scope = 'screen')}
        >{$t.caseBridge.scope.screen}</button>
      </div>
    </div>

    <div class="case-bridge-log" bind:this={listEl} aria-live="polite">
      {#if caseBridge.entries.length === 0}
        <p class="case-bridge-empty">{$t.caseBridge.empty}</p>
      {/if}
      {#each caseBridge.entries as entry (entry.id)}
        <section class="case-bridge-entry" data-status={entry.status}>
          <p class="case-bridge-question">{entry.question}</p>
          {#if entry.answer}
            <p class="case-bridge-answer">{entry.answer}</p>
          {:else if entry.status === 'streaming'}
            <p class="case-bridge-thinking">{$t.caseBridge.thinking}</p>
          {/if}
          {#if entry.error}
            <p class="case-bridge-error">{$t.caseBridge.errors[entry.error]}</p>
          {/if}
          {#if entry.cards.length > 0}
            <ul class="case-bridge-cards" aria-label={$t.caseBridge.cardsAria}>
              {#each entry.cards as card (card.kind + ':' + (card.id ?? ''))}
                <li>
                  <button type="button" class="case-bridge-card" data-severity={card.severity} onclick={() => go(card)}>
                    <span class="case-bridge-card-severity">{$t.caseBridge.severity[card.severity]}</span>
                    <span class="case-bridge-card-title">{card.title}</span>
                    {#if card.sub}<span class="case-bridge-card-sub">{card.sub}</span>{/if}
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </section>
      {/each}
    </div>

    {#if !caseBridge.busy}
      <div class="case-bridge-chips">
        {#each followUps as key (key)}
          <button type="button" class="case-bridge-chip" onclick={() => submit($t.caseBridge.followUps[key])}>{$t.caseBridge.followUps[key]}</button>
        {/each}
      </div>
    {/if}

    <form class="case-bridge-ask" onsubmit={(event) => { event.preventDefault(); void submit() }}>
      <input
        bind:this={inputEl}
        bind:value={draft}
        placeholder={$t.caseBridge.inputPlaceholder}
        aria-label={$t.caseBridge.inputAria}
        autocomplete="off"
      />
      <button type="submit" disabled={caseBridge.busy || !draft.trim()}>{$t.caseBridge.send}</button>
    </form>
  </div>
{/if}

<style>
  /* A panel on the right edge of any screen, above the page and CASE
     (z-index 90) but below the palette and help overlays. Full width
     below 900px. */
  .case-bridge {
    position: fixed;
    top: var(--space-4);
    right: var(--space-4);
    bottom: var(--space-4);
    z-index: 95;
    width: min(480px, calc(100vw - 2 * var(--space-4)));
    display: grid;
    grid-template-rows: auto auto 1fr auto auto;
    gap: var(--space-3);
    padding: var(--space-4);
    border: 1px solid rgba(var(--primary-rgb), 0.32);
    border-radius: var(--radius-lg);
    background: color-mix(in srgb, var(--surface-elevated) 96%, black);
    box-shadow: 0 18px 44px rgba(0, 0, 0, 0.42);
    color: var(--text-primary);
    outline: none;
  }

  @media (max-width: 900px) {
    .case-bridge {
      top: 0;
      right: 0;
      bottom: 0;
      width: 100vw;
      border-radius: 0;
    }
  }

  .case-bridge-header {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .case-bridge-face {
    flex: none;
    width: 64px;
    height: 64px;
  }

  .case-bridge-titles {
    display: grid;
    gap: 2px;
    min-width: 0;
    flex: 1;
  }

  .case-bridge-titles strong {
    font-size: 0.95rem;
  }

  .case-bridge-titles span {
    color: var(--text-tertiary);
    font-size: 0.8rem;
  }

  .case-bridge-close {
    align-self: flex-start;
    width: 28px;
    height: 28px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
  }

  .case-bridge-context {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    font-size: 0.8rem;
    color: var(--text-tertiary);
  }

  .case-bridge-scope {
    display: inline-flex;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .case-bridge-scope button {
    padding: 4px var(--space-2);
    border: 0;
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    cursor: pointer;
  }

  .case-bridge-scope button.active {
    background: rgba(var(--primary-rgb), 0.16);
    color: var(--primary-text);
  }

  .case-bridge-scope button:disabled {
    opacity: 0.45;
    cursor: default;
  }

  .case-bridge-log {
    min-height: 0;
    overflow-y: auto;
    display: grid;
    align-content: start;
    gap: var(--space-3);
  }

  .case-bridge-empty {
    margin: 0;
    color: var(--text-tertiary);
    font-size: 0.85rem;
    line-height: 1.5;
  }

  .case-bridge-entry {
    display: grid;
    gap: var(--space-2);
  }

  .case-bridge-question {
    margin: 0;
    color: var(--text-tertiary);
    font-size: 0.8rem;
    font-weight: 600;
  }

  .case-bridge-answer {
    margin: 0;
    font-size: 0.9rem;
    line-height: 1.55;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .case-bridge-thinking {
    margin: 0;
    color: var(--text-tertiary);
    font-size: 0.85rem;
  }

  .case-bridge-error {
    margin: 0;
    color: var(--error);
    font-size: 0.85rem;
  }

  .case-bridge-cards {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: var(--space-1);
  }

  .case-bridge-card {
    width: 100%;
    display: grid;
    grid-template-columns: auto 1fr;
    column-gap: var(--space-2);
    align-items: baseline;
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    font-size: 0.85rem;
    text-align: left;
    cursor: pointer;
  }

  .case-bridge-card:hover,
  .case-bridge-card:focus-visible {
    border-color: rgba(var(--primary-rgb), 0.5);
  }

  .case-bridge-card[data-severity='block'] {
    border-color: color-mix(in srgb, var(--error) 55%, transparent);
  }

  .case-bridge-card-severity {
    grid-row: span 2;
    font-family: var(--font-mono);
    font-size: 0.68rem;
    text-transform: uppercase;
    color: var(--text-tertiary);
  }

  .case-bridge-card[data-severity='block'] .case-bridge-card-severity {
    color: var(--error);
  }

  .case-bridge-card-title {
    font-weight: 600;
  }

  .case-bridge-card-sub {
    color: var(--text-tertiary);
    font-size: 0.8rem;
  }

  .case-bridge-chips {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }

  .case-bridge-chip {
    padding: 4px var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: 999px;
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-size: 0.78rem;
    cursor: pointer;
  }

  .case-bridge-chip:hover,
  .case-bridge-chip:focus-visible {
    border-color: rgba(var(--primary-rgb), 0.5);
    color: var(--primary-text);
  }

  .case-bridge-ask {
    display: flex;
    gap: var(--space-2);
  }

  .case-bridge-ask input {
    flex: 1;
    min-width: 0;
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-base);
    color: var(--text-primary);
    font: inherit;
  }

  .case-bridge-ask button {
    padding: var(--space-2) var(--space-3);
    border: 1px solid rgba(var(--primary-rgb), 0.5);
    border-radius: var(--radius-sm);
    background: rgba(var(--primary-rgb), 0.16);
    color: var(--primary-text);
    font: inherit;
    cursor: pointer;
  }

  .case-bridge-ask button:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
