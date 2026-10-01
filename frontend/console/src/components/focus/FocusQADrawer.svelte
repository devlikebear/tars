<script lang="ts">
  // The Q&A drawer (ADR §8): questions about the card on screen, threaded
  // by card. Answers come from the pipeline's hidden plan-mode session, so
  // they can read code but never change it, and they never reach the main
  // agent unless promoted — "Promote to instruction" puts an answer into
  // the stage input as a draft to edit.
  import { t } from '../../i18n'
  import type { QAEntry } from '../../lib/focus'

  interface Props {
    cardId: string
    thread: QAEntry[]
    // The card's question being answered now.
    answering: boolean
    error: string
    open: boolean
    onToggle: () => void
    onAsk: (question: string) => Promise<boolean>
    onPromote: (entry: QAEntry) => void
    // Bound so the deck's `?` key can focus the question box.
    input?: HTMLTextAreaElement | null
  }

  let { cardId, thread, answering, error, open, onToggle, onAsk, onPromote, input = $bindable(null) }: Props = $props()

  let question = $state('')
  let sending = $state(false)

  let errorText = $derived(error === 'busy' ? $t.focus.qa.busy : error ? $t.focus.qa.failed(error) : '')

  async function ask() {
    const text = question.trim()
    if (!text || sending) return
    sending = true
    try {
      if (await onAsk(text)) question = ''
    } finally {
      sending = false
    }
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
      event.preventDefault()
      void ask()
    }
  }
</script>

<section class="qa" data-testid="focus-qa" data-card-id={cardId}>
  <button
    type="button"
    class="btn btn-ghost btn-sm qa-toggle"
    aria-expanded={open}
    data-testid="focus-qa-toggle"
    onclick={onToggle}
  >
    {open ? $t.focus.qa.close : $t.focus.qa.open}
    {#if thread.length && !open}<span class="badge badge-default mono">{thread.length}</span>{/if}
  </button>

  {#if open}
    <div class="qa-body">
      <h4 class="label">{$t.focus.qa.title}</h4>
      {#if thread.length === 0}
        <p class="muted qa-empty">{$t.focus.qa.empty}</p>
      {:else}
        <ol class="qa-thread">
          {#each thread as entry (entry.turn)}
            <li class="qa-entry" data-testid="focus-qa-entry">
              <p class="qa-q"><span class="label">{$t.focus.qa.you}</span> <span data-content>{entry.question}</span></p>
              {#if entry.answer}
                <div class="qa-a">
                  <span class="label">{$t.focus.qa.answer}</span>
                  <p class="prose" data-content data-testid="focus-qa-answer">{entry.answer}</p>
                  <button type="button" class="btn btn-ghost btn-sm" data-testid="focus-qa-promote" onclick={() => onPromote(entry)}>{$t.focus.qa.promote}</button>
                </div>
              {:else}
                <p class="muted qa-a" role="status">{$t.focus.qa.answering}</p>
              {/if}
            </li>
          {/each}
        </ol>
      {/if}
      <div class="qa-ask">
        <textarea
          rows="2"
          bind:this={input}
          bind:value={question}
          placeholder={$t.focus.qa.placeholder}
          aria-label={$t.focus.qa.title}
          disabled={sending}
          onkeydown={onKeydown}
          data-testid="focus-qa-input"
        ></textarea>
        <button type="button" class="btn btn-secondary btn-sm" disabled={sending || answering || !question.trim()} onclick={() => void ask()} data-testid="focus-qa-ask">{$t.focus.qa.ask}</button>
      </div>
      {#if errorText}<p class="qa-error" role="alert">{errorText}</p>{/if}
    </div>
  {/if}
</section>

<style>
  .qa {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
  }

  .qa-toggle {
    align-self: flex-start;
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }

  .qa-body {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    background: var(--surface-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
  }

  .qa-body h4,
  .qa-empty {
    margin: 0;
  }

  .qa-thread {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    max-height: 320px;
    overflow-y: auto;
  }

  .qa-entry p {
    margin: 0;
  }

  .qa-q {
    color: var(--text-primary);
  }

  .qa-a {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-1);
    margin-top: var(--space-1);
    padding-left: var(--space-3);
    border-left: 2px solid rgba(var(--primary-rgb), 0.35);
  }

  .qa-a .prose {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .qa-ask {
    display: flex;
    gap: var(--space-2);
    align-items: flex-end;
  }

  .qa-ask textarea {
    flex: 1;
    min-width: 0;
    padding: var(--space-2) var(--space-3);
    background: var(--surface-base);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-sm);
    resize: vertical;
  }

  .qa-ask textarea:focus {
    outline: none;
    border-color: var(--primary);
  }

  .qa-error {
    margin: 0;
    color: var(--error);
    font-size: var(--text-sm);
  }
</style>
