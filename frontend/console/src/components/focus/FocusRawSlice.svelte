<script lang="ts">
  // "View raw" (o): the turn a card came from, read-only, drawn by the chat
  // thread's own message renderer. Hidden blocks fold away there as they do
  // in Advanced.
  import { t } from '../../i18n'
  import { turnSlice } from '../../lib/focus'
  import { transcriptChatMessages } from '../../lib/transcriptMessages'
  import type { SessionMessage } from '../../lib/types'
  import ChatMessageItem from '../ChatMessageItem.svelte'

  interface Props {
    history: SessionMessage[]
    turn: number
    baseDirs?: string[]
  }

  let { history, turn, baseDirs = [] }: Props = $props()

  let messages = $derived(transcriptChatMessages(turnSlice(history, turn)))

  function copy(text: string) {
    void navigator.clipboard?.writeText(text).catch(() => {})
  }
</script>

<section class="raw-slice" aria-label={$t.focus.deck.rawTitle} data-testid="focus-raw">
  <h4 class="label">{$t.focus.deck.rawTitle} · {$t.focus.card.turn(turn)}</h4>
  {#if turn < 1}
    <p class="muted">{$t.focus.deck.rawEmpty}</p>
  {:else if messages.length === 0}
    <p class="muted">{$t.focus.deck.rawLoading}</p>
  {:else}
    <div class="raw-messages">
      {#each messages as message (message.id)}
        <ChatMessageItem {message} artifacts={[]} onCopy={copy} toolBaseDir={baseDirs} />
      {/each}
    </div>
  {/if}
</section>

<style>
  /* The transcript stays inside the card column: long lines wrap, and tool
     cards and code blocks scroll on their own instead of widening the page. */
  .raw-slice {
    min-width: 0;
    max-width: 100%;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding-top: var(--space-3);
    border-top: 1px solid var(--border-subtle);
  }

  h4 {
    margin: 0;
  }

  .raw-messages {
    min-width: 0;
    overflow-x: hidden;
    overflow-wrap: anywhere;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    max-height: 480px;
    overflow: auto;
  }

  .raw-messages > :global(*) {
    min-width: 0;
    max-width: 100%;
  }

  .raw-messages :global(pre) {
    max-width: 100%;
    overflow-x: auto;
  }

  /* A table sizes to its cells; as a block it scrolls inside the slice. */
  .raw-messages :global(table) {
    display: block;
    max-width: 100%;
    overflow-x: auto;
  }

  .muted {
    margin: 0;
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }
</style>
