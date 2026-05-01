<script lang="ts">
  import { onMount } from 'svelte'
  import { executeTasksAction, getSessionTasks } from '../lib/api'
  import type { SessionTasks, TaskContract } from '../lib/types'

  interface Props {
    sessionId: string
    onClose: () => void
  }

  let { sessionId, onClose }: Props = $props()

  let data: SessionTasks = $state({ tasks: [] })
  let loading = $state(true)
  let saving = $state(false)
  let error = $state('')
  let saved = $state('')
  let goal = $state('')
  let scope = $state('')
  let doneCriteria = $state('')
  let verificationCommands = $state('')
  let artifacts = $state('')

  let contract = $derived(data.contract)
  let status = $derived(contract?.status ?? 'draft')
  let hasDraft = $derived(Boolean(contract || data.plan))

  function lines(value?: string[]): string {
    return (value ?? []).join('\n')
  }

  function splitLines(value: string): string[] {
    return value
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean)
  }

  function loadDraft(next: SessionTasks) {
    const nextContract: TaskContract | undefined = next.contract
    goal = nextContract?.goal ?? next.plan?.goal ?? ''
    scope = nextContract?.scope ?? next.plan?.constraints ?? ''
    doneCriteria = lines(nextContract?.done_criteria)
    verificationCommands = lines(nextContract?.verification_commands)
    artifacts = lines(nextContract?.artifacts)
  }

  export async function load() {
    loading = true
    error = ''
    saved = ''
    try {
      data = await getSessionTasks(sessionId)
      loadDraft(data)
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load contract'
    } finally {
      loading = false
    }
  }

  async function saveContract(markApproved = false) {
    saving = true
    error = ''
    saved = ''
    try {
      await executeTasksAction(sessionId, {
        action: 'contract_update',
        goal,
        scope,
        done_criteria: splitLines(doneCriteria),
        verification_commands: splitLines(verificationCommands),
        artifacts: splitLines(artifacts),
      })
      if (markApproved) {
        await executeTasksAction(sessionId, { action: 'contract_approve' })
      }
      data = await getSessionTasks(sessionId)
      loadDraft(data)
      saved = markApproved ? 'Approved' : 'Saved'
    } catch (err) {
      error = err instanceof Error ? err.message : 'Save failed'
    } finally {
      saving = false
    }
  }

  onMount(() => { void load() })

  $effect(() => {
    void sessionId
    void load()
  })
</script>

<div class="contract-panel">
  <div class="panel-header">
    <div class="panel-title-row">
      <span class="card-title">Contract</span>
      {#if hasDraft}
        <span class="badge" class:badge-success={status === 'approved'} class:badge-warning={status !== 'approved'}>{status}</span>
      {/if}
    </div>
    <div class="panel-actions">
      <button class="btn btn-ghost btn-sm" type="button" onclick={load} title="Refresh">&#x21bb;</button>
      <button class="btn btn-ghost btn-sm" type="button" onclick={onClose} title="Close">&times;</button>
    </div>
  </div>

  {#if loading}
    <div class="empty-state">Loading contract...</div>
  {:else if error}
    <div class="error-banner">{error}</div>
  {:else if !hasDraft}
    <div class="empty-state">
      <p>No task contract yet.</p>
      <p class="hint">Start a multi-step plan to draft one.</p>
    </div>
  {:else}
    <div class="contract-form">
      <label>
        <span>Goal</span>
        <textarea bind:value={goal} rows="3" disabled={saving}></textarea>
      </label>
      <label>
        <span>Scope</span>
        <textarea bind:value={scope} rows="4" disabled={saving}></textarea>
      </label>
      <label>
        <span>Done criteria</span>
        <textarea bind:value={doneCriteria} rows="5" disabled={saving}></textarea>
      </label>
      <label>
        <span>Verification commands</span>
        <textarea bind:value={verificationCommands} rows="4" disabled={saving}></textarea>
      </label>
      <label>
        <span>Artifacts</span>
        <textarea bind:value={artifacts} rows="4" disabled={saving}></textarea>
      </label>
    </div>

    {#if saved}
      <div class="success-banner">{saved}</div>
    {/if}

    <div class="contract-actions">
      <button class="btn btn-primary btn-sm" type="button" disabled={saving} onclick={() => saveContract(false)}>
        {saving ? 'Saving...' : 'Save'}
      </button>
      <button class="btn btn-ghost btn-sm" type="button" disabled={saving} onclick={() => saveContract(true)}>
        Approve Contract
      </button>
    </div>
  {/if}
</div>

<style>
  .contract-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    height: 100%;
    overflow-y: auto;
  }

  .panel-header,
  .panel-title-row,
  .panel-actions,
  .contract-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .panel-header {
    justify-content: space-between;
  }

  .contract-form {
    display: grid;
    gap: var(--space-3);
  }

  .contract-form label {
    display: grid;
    gap: var(--space-1);
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }

  .contract-form textarea {
    width: 100%;
    resize: vertical;
    border-radius: var(--radius-md);
    border: 1px solid var(--border-subtle);
    background: var(--surface);
    color: var(--text-primary);
    padding: var(--space-2);
    font: inherit;
    line-height: 1.45;
  }

  .contract-form textarea:focus {
    outline: none;
    border-color: var(--primary);
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--primary) 20%, transparent);
  }

  .contract-actions {
    flex-wrap: wrap;
  }

  .empty-state {
    padding: var(--space-6) var(--space-4);
    text-align: center;
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .hint {
    color: var(--text-tertiary);
  }

  .success-banner {
    border: 1px solid color-mix(in srgb, var(--success) 40%, transparent);
    border-radius: var(--radius-md);
    background: color-mix(in srgb, var(--success) 10%, transparent);
    color: var(--text-primary);
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-sm);
  }
</style>
