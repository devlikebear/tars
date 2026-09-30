<script lang="ts">
  // Needs-input bar (#970): tool calls an unattended run of this session
  // (cron, Telegram, a subagent) waits to run, queued in the ops approvals
  // because the session's permission mode asks. Answering here is the same
  // as answering on the Ops page.
  import { onDestroy } from 'svelte'
  import { listApprovals, reviewApproval } from '../lib/api'
  import { pendingToolApprovals, type PendingToolApproval } from '../lib/unattendedApprovals'
  import { t } from '../i18n'

  let { sessionId }: { sessionId: string } = $props()

  const pollMs = 3000
  let pending = $state<PendingToolApproval[]>([])
  let answering = $state('')
  let error = $state('')
  let timer: ReturnType<typeof setInterval> | null = null

  async function refresh(id: string) {
    try {
      const items = await listApprovals()
      if (id === sessionId) pending = pendingToolApprovals(items, id)
    } catch {
      // The Ops page reports load errors; the bar just stays as it was.
    }
  }

  async function answer(item: PendingToolApproval, action: 'approve' | 'reject') {
    answering = item.id
    error = ''
    try {
      await reviewApproval(item.id, action)
    } catch (err) {
      error = err instanceof Error ? err.message : $t.permissionMode.unattended.error
    } finally {
      answering = ''
      await refresh(sessionId)
    }
  }

  $effect(() => {
    const id = sessionId
    pending = []
    if (timer) clearInterval(timer)
    timer = null
    if (!id) return
    void refresh(id)
    timer = setInterval(() => void refresh(id), pollMs)
  })

  onDestroy(() => {
    if (timer) clearInterval(timer)
  })
</script>

{#each pending as item (item.id)}
  <div class="needs-input-bar" role="group" aria-label={$t.permissionMode.unattended.title(item.request.source)} data-testid="needs-input">
    <div class="needs-input-text">
      <strong>{$t.permissionMode.unattended.title(item.request.source)}</strong>
      <span>{$t.permissionMode.unattended.body(item.request.tool_name)}</span>
      {#if item.request.preview}
        <code>{item.request.preview}</code>
      {/if}
    </div>
    <div class="needs-input-actions">
      <button type="button" class="btn btn-secondary btn-sm" disabled={answering === item.id} onclick={() => void answer(item, 'approve')}>{$t.permissionMode.unattended.approve}</button>
      <button type="button" class="btn btn-danger btn-sm" disabled={answering === item.id} onclick={() => void answer(item, 'reject')}>{$t.permissionMode.unattended.reject}</button>
    </div>
  </div>
{/each}
{#if error}
  <div class="needs-input-error" role="alert">{error}</div>
{/if}

<style>
  .needs-input-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    margin-bottom: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--warning);
    border-radius: var(--radius-md);
    background: var(--warning-muted);
    font-size: var(--text-sm);
  }

  .needs-input-text {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-2);
    min-width: 0;
    color: var(--text-secondary);
  }

  .needs-input-text strong {
    color: var(--warning);
    font-weight: 600;
  }

  .needs-input-text code {
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-primary);
  }

  .needs-input-actions {
    display: flex;
    flex-shrink: 0;
    gap: var(--space-1);
  }

  .needs-input-error {
    margin-bottom: var(--space-2);
    color: var(--error);
    font-size: var(--text-xs);
  }

  @media (max-width: 640px) {
    .needs-input-bar {
      flex-direction: column;
      align-items: stretch;
    }
  }
</style>
