<script lang="ts">
  // G3 (ADR §4, §7): the PR draft the agent wrote. The developer edits the
  // title and body and approves; the server then has the agent push and run
  // `gh pr create` with exactly this text, and probes until the PR exists.
  import { untrack } from 'svelte'
  import { t } from '../../i18n'
  import type { FocusPRDraft } from '../../lib/types'

  interface Props {
    // A new card (a revised draft) resets the edits; the same card re-read
    // from the server keeps them.
    cardId: string
    draft: FocusPRDraft
    // The gate is open and the card undecided: the draft can be edited.
    open: boolean
    busy: boolean
    onApprove: (draft: FocusPRDraft) => void
    onRequestChanges: (note: string) => void
    onStop: () => void
  }

  let { cardId, draft, open, busy, onApprove, onRequestChanges, onStop }: Props = $props()

  let title = $state('')
  let body = $state('')
  let asking = $state(false)
  let note = $state('')
  let seededFor: string | null = null

  $effect(() => {
    if (cardId === seededFor) return
    seededFor = cardId
    untrack(() => {
      title = draft.title
      body = draft.body
      asking = false
      note = ''
    })
  })

  function approve() {
    if (!title.trim()) return
    onApprove({ title: title.trim(), body })
  }

  function sendChanges() {
    const text = note.trim()
    if (text) onRequestChanges(text)
  }
</script>

<div class="pr-gate" data-testid="focus-pr-gate">
  {#if open}
    <label class="label" for={`focus-pr-title-${cardId}`}>{$t.focus.pr.title}</label>
    <input id={`focus-pr-title-${cardId}`} type="text" bind:value={title} disabled={busy} data-testid="focus-pr-title" />
    {#if !title.trim()}<p class="hint warn">{$t.focus.pr.noTitle}</p>{/if}
    <label class="label" for={`focus-pr-body-${cardId}`}>{$t.focus.pr.body}</label>
    <textarea id={`focus-pr-body-${cardId}`} rows={Math.min(14, Math.max(4, body.split('\n').length))} bind:value={body} disabled={busy} data-testid="focus-pr-body"></textarea>
    <p class="hint">{$t.focus.pr.approveHint}</p>
    {#if asking}
      <textarea rows="3" bind:value={note} placeholder={$t.focus.gate.notePlaceholder} disabled={busy}></textarea>
      <div class="actions">
        <button type="button" class="btn btn-secondary btn-sm" disabled={busy || !note.trim()} onclick={sendChanges}>{$t.focus.gate.sendChanges}</button>
        <button type="button" class="btn btn-ghost btn-sm" disabled={busy} onclick={() => { asking = false }}>{$t.focus.gate.cancel}</button>
      </div>
    {:else}
      <div class="actions">
        <button type="button" class="btn btn-primary" disabled={busy || !title.trim()} onclick={approve} data-testid="focus-pr-approve">{$t.focus.pr.approve}</button>
        <button type="button" class="btn btn-secondary" disabled={busy} onclick={() => { asking = true }}>{$t.focus.gate.requestChanges}</button>
        <button type="button" class="btn btn-danger" disabled={busy} onclick={onStop}>{$t.focus.gate.stop}</button>
      </div>
    {/if}
  {:else}
    <h4 class="label">{$t.focus.pr.title}</h4>
    <p class="prose" data-content>{draft.title}</p>
    <h4 class="label">{$t.focus.pr.body}</h4>
    <pre data-content>{draft.body}</pre>
  {/if}
</div>

<style>
  .pr-gate {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  h4,
  .prose {
    margin: 0;
  }

  input,
  textarea {
    width: 100%;
    box-sizing: border-box;
    padding: var(--space-2);
    background: var(--surface-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-sm);
  }

  textarea {
    resize: vertical;
  }

  input:focus,
  textarea:focus {
    outline: none;
    border-color: var(--primary);
  }

  pre {
    margin: 0;
    padding: var(--space-2);
    background: var(--surface-inset);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    white-space: pre-wrap;
  }

  .hint {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .hint.warn {
    color: var(--warning);
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
</style>
