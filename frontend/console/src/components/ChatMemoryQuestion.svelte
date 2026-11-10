<script lang="ts">
  // The remember / don't-remember buttons under a confirm_memory question
  // (tars#1001). The button is the answer: nothing reads the user's reply to
  // decide, so the server never asks a model to judge it.
  import { t } from '../i18n'
  import { answerInitiativeMemory, getInitiativeMemoryAnswer, APIRequestError } from '../lib/api'
  import { memoryQuestionState, type MemoryQuestionState } from '../lib/memoryQuestion'

  interface Props {
    entryId: string
  }

  let { entryId }: Props = $props()

  let view = $state<MemoryQuestionState>('loading')
  let busy = $state(false)

  $effect(() => {
    const id = entryId
    view = 'loading'
    getInitiativeMemoryAnswer(id)
      .then((res) => {
        if (id === entryId) view = memoryQuestionState(res.status)
      })
      .catch(() => {
        if (id === entryId) view = 'failed'
      })
  })

  async function answer(action: 'approve' | 'reject') {
    if (busy) return
    busy = true
    try {
      const res = await answerInitiativeMemory(entryId, action)
      view = memoryQuestionState(res.status)
    } catch (err) {
      // 409: already reviewed elsewhere — the body still carries the status.
      view = err instanceof APIRequestError && err.status === 409 ? 'handled' : 'failed'
    } finally {
      busy = false
    }
  }
</script>

{#if view === 'open' || view === 'failed'}
  <div class="memory-question" role="group">
    {#if view === 'failed'}
      <span class="memory-question-note">{$t.chatThread.memoryQuestion.failed}</span>
    {/if}
    <button type="button" class="memory-question-btn primary" disabled={busy} onclick={() => answer('approve')}>{$t.chatThread.memoryQuestion.remember}</button>
    <button type="button" class="memory-question-btn" disabled={busy} onclick={() => answer('reject')}>{$t.chatThread.memoryQuestion.dontRemember}</button>
  </div>
{:else if view !== 'loading'}
  <div class="memory-question">
    <span class="memory-question-note">
      {view === 'saved' ? $t.chatThread.memoryQuestion.saved : view === 'skipped' ? $t.chatThread.memoryQuestion.skipped : $t.chatThread.memoryQuestion.handled}
    </span>
  </div>
{/if}

<style>
  .memory-question {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }

  .memory-question-note {
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .memory-question-btn {
    font-size: var(--text-xs);
    padding: 2px 10px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border-default);
    background: var(--surface-elevated);
    color: var(--text-primary);
    cursor: pointer;
  }

  .memory-question-btn.primary {
    color: rgba(var(--primary-rgb), 0.95);
    border-color: rgba(var(--primary-rgb), 0.4);
    background: rgba(var(--primary-rgb), 0.12);
  }

  .memory-question-btn:disabled {
    opacity: 0.6;
    cursor: default;
  }
</style>
