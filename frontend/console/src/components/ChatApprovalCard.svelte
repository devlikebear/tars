<script lang="ts">
  // A tool call waiting on the user's decision, inline in the thread (#970).
  // Claude Code blocks the tool until the card is answered; the card settles
  // from the server's permission_resolved event, not from the click, so it
  // also closes when the turn ends without an answer.
  import { onMount } from 'svelte'
  import { t } from '../i18n'
  import { answerChatPermission } from '../lib/api'
  import {
    approvalPreview,
    decisionForKey,
    type ChatApproval,
    type ChatApprovalDecision,
  } from '../lib/chatApproval'

  interface Props {
    approval: ChatApproval
    onChange: (next: ChatApproval) => void
  }

  let { approval, onChange }: Props = $props()

  let card: HTMLElement | undefined = $state()
  let preview = $derived(approvalPreview(approval))
  let pending = $derived(approval.state === 'pending')

  async function answer(decision: ChatApprovalDecision) {
    if (approval.state !== 'pending') return
    onChange({ ...approval, state: 'sending', error: undefined })
    try {
      await answerChatPermission(approval.requestId, approval.sessionId, decision)
    } catch {
      onChange({ ...approval, state: 'pending', error: $t.chatApproval.sendFailed })
    }
  }

  // y / s / n answer the card while focus is inside it, which it takes when
  // it appears; typing in the composer never reaches it.
  function onKeydown(event: KeyboardEvent) {
    if (!card?.contains(document.activeElement)) return
    if (event.metaKey || event.ctrlKey || event.altKey) return
    const decision = decisionForKey(event.key, approval)
    if (!decision) return
    event.preventDefault()
    void answer(decision)
  }

  onMount(() => {
    if (approval.state === 'pending') card?.focus()
  })

  function settledText(): string {
    switch (approval.state) {
      case 'sending':
        return $t.chatApproval.state.sending
      case 'allowed':
        return $t.chatApproval.state.allowed
      case 'allowed_session':
        return $t.chatApproval.state.allowedSession(approval.sessionRule ?? approval.toolName)
      case 'denied':
        return $t.chatApproval.state.denied
      case 'withdrawn':
        return $t.chatApproval.state.withdrawn
    }
    return ''
  }
</script>

<svelte:window onkeydown={onKeydown} />

<section
  bind:this={card}
  class="approval"
  class:settled={!pending}
  aria-label={$t.chatApproval.label}
  tabindex="-1"
>
  <header class="approval-head">
    <span class="approval-title">{approval.title ?? $t.chatApproval.heading(approval.toolName)}</span>
    {#if approval.agentId}
      <span class="badge badge-info">{$t.chatApproval.subagent}</span>
    {/if}
  </header>

  {#if preview.text}
    <div class="approval-preview">
      <span class="approval-label">{$t.chatApproval.preview[preview.kind]}</span>
      <pre class="approval-code">{preview.text}</pre>
    </div>
  {/if}
  {#if approval.description && approval.description !== preview.text}
    <p class="approval-note">{approval.description}</p>
  {/if}
  {#if approval.reason}
    <p class="approval-note"><span class="approval-label">{$t.chatApproval.reason}</span> {approval.reason}</p>
  {/if}

  {#if pending}
    <div class="approval-actions">
      <button type="button" class="btn btn-secondary btn-sm" onclick={() => answer('allow_once')}>{$t.chatApproval.allowOnce}</button>
      {#if approval.sessionRule}
        <button type="button" class="btn btn-ghost btn-sm" onclick={() => answer('allow_session')}>
          {$t.chatApproval.allowSession(approval.sessionRule)}
        </button>
      {/if}
      <button type="button" class="btn btn-danger btn-sm" onclick={() => answer('deny')}>{$t.chatApproval.deny}</button>
      <span class="approval-keys" aria-hidden="true">
        {approval.sessionRule ? $t.chatApproval.keysHint : $t.chatApproval.keysHintNoSession}
      </span>
    </div>
    {#if approval.error}
      <p class="approval-error" role="alert">{approval.error}</p>
    {/if}
  {:else}
    <p class="approval-outcome" data-state={approval.state}>{settledText()}</p>
  {/if}
</section>

<style>
  .approval {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--warning);
    border-radius: var(--radius-md);
    background: var(--warning-muted);
    font-size: var(--text-sm);
    min-width: 0;
    outline: none;
  }

  .approval:focus-visible {
    box-shadow: 0 0 0 2px var(--border-strong);
  }

  .approval.settled {
    border-color: var(--border-subtle);
    background: var(--surface);
  }

  .approval-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .approval-title {
    color: var(--text-primary);
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .approval-label {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .approval-preview {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .approval-code {
    margin: 0;
    padding: var(--space-2);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    max-height: 240px;
    overflow: auto;
  }

  .approval-note {
    margin: 0;
    color: var(--text-secondary);
    overflow-wrap: anywhere;
  }

  .approval-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .approval-keys {
    margin-left: auto;
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .approval-error {
    margin: 0;
    color: var(--error);
    font-size: var(--text-xs);
  }

  .approval-outcome {
    margin: 0;
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .approval-outcome[data-state='denied'] {
    color: var(--error);
  }

  .approval-outcome[data-state='allowed'],
  .approval-outcome[data-state='allowed_session'] {
    color: var(--success);
  }
</style>
