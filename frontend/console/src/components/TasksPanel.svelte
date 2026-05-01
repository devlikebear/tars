<script lang="ts">
  import { onMount } from 'svelte'
  import { t } from '../i18n'
  import { getSessionPlanArchive, getSessionTasks, executeTasksAction, cancelChat } from '../lib/api'
  import { planProgressPercent, summarizeTasks } from '../lib/tasks'
  import type { PlanArchiveItem, SessionTask, SessionTasks, TaskEvidence } from '../lib/types'

  interface Props {
    sessionId: string
    onClose: () => void
    onSendMessage?: (text: string) => Promise<void>
  }

  let { sessionId, onClose, onSendMessage }: Props = $props()

  let data: SessionTasks = $state({ tasks: [] })
  let loading = $state(true)
  let error = $state('')
  let planExpanded = $state(true)
  let archiveItems: PlanArchiveItem[] = $state([])
  let archiveExpanded = $state(false)
  let archiveError = $state('')
  let expandedArchiveId = $state('')
  let taskList = $derived(Array.isArray(data.tasks) ? data.tasks : [])
  let planStatus = $derived(data.plan?.status ?? '')

  // Edit/CTA state for the propose/approve workflow.
  let editing = $state(false)
  let editDrafts: Array<{ id: string; title: string; description: string }> = $state([])
  let actionBusy = $state(false)
  let actionError = $state('')
  let evidenceDraftTaskId = $state('')
  let evidenceType = $state('test_result')
  let evidenceTitle = $state('')
  let evidenceSummary = $state('')
  let evidenceURL = $state('')
  let evidenceCommand = $state('')

  const evidenceTypeOptions = [
    { value: 'test_result', label: 'Test result' },
    { value: 'image', label: 'Image' },
    { value: 'log_excerpt', label: 'Log excerpt' },
    { value: 'pr_link', label: 'PR link' },
    { value: 'release_tag', label: 'Release tag' },
    { value: 'command_output_summary', label: 'Command output' },
  ]

  export async function load() {
    loading = true
    error = ''
    archiveError = ''
    try {
      data = await getSessionTasks(sessionId)
      try {
        archiveItems = (await getSessionPlanArchive(sessionId)).items
      } catch (err) {
        archiveItems = []
        archiveError = err instanceof Error ? err.message : 'Failed to load archived plans'
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load tasks'
    } finally {
      loading = false
    }
  }

  async function runAction(payload: Record<string, unknown>) {
    actionBusy = true
    actionError = ''
    try {
      await executeTasksAction(sessionId, payload)
      await load()
    } catch (err) {
      actionError = err instanceof Error ? err.message : 'Action failed'
      throw err
    } finally {
      actionBusy = false
    }
  }

  async function handleApprove() {
    try {
      await runAction({ action: 'plan_approve' })
    } catch {
      return
    }
    // Auto-send "go" through the host chat panel so the message lands
    // in the conversation transcript and the LLM picks up the next turn
    // immediately. The send is best-effort: the plan is already approved
    // on the backend, so a silent failure is acceptable.
    if (onSendMessage) {
      try {
        await onSendMessage('go')
      } catch {
        // ignore — user can type "go" manually as a fallback
      }
    }
    await load()
  }

  async function handleDiscard() {
    if (!confirm($t.tasks.confirm.discard)) return
    await runAction({ action: 'clear' })
  }

  // Pause cancels any in-flight chat turn first so the LLM stops producing
  // tokens immediately, then flips the plan status to paused. Resume just
  // flips status back; the user (or LLM on its next turn) drives forward
  // progress from here.
  async function handlePause() {
    actionBusy = true
    actionError = ''
    try {
      await cancelChat(sessionId)
    } catch {
      // best-effort — keep going to set paused state
    } finally {
      actionBusy = false
    }
    await runAction({ action: 'plan_pause' })
  }

  async function handleResume() {
    try {
      await runAction({ action: 'plan_resume' })
    } catch {
      return
    }
    if (onSendMessage) {
      try {
        await onSendMessage('continue')
      } catch {
        // user can type "continue" manually as fallback
      }
    }
    await load()
  }

  async function handleAbort() {
    if (!confirm($t.tasks.confirm.abort)) return
    actionBusy = true
    actionError = ''
    try {
      await cancelChat(sessionId)
    } catch {
      // best-effort
    } finally {
      actionBusy = false
    }
    await runAction({ action: 'plan_abort' })
  }

  // Skipping a single task flips it to cancelled and lets the LLM know
  // via a short user message. The auto-completion rule (CON-051) will
  // mark the plan completed automatically if this was the last open
  // task.
  async function handleSkipTask(taskId: string, taskTitle: string) {
    try {
      await runAction({ action: 'update', id: taskId, status: 'cancelled' })
    } catch {
      return
    }
    if (onSendMessage) {
      try {
        await onSendMessage(`Skip task ${taskId} (${taskTitle.trim()}) and continue with the next pending task.`)
      } catch {
        // ignore
      }
    }
    await load()
  }

  function startEdit() {
    editDrafts = taskList.map((t) => ({
      id: t.id,
      title: t.title,
      description: t.description ?? '',
    }))
    editing = true
  }

  function cancelEdit() {
    editing = false
    editDrafts = []
    actionError = ''
  }

  function addDraft() {
    editDrafts = [
      ...editDrafts,
      { id: `__new_${Date.now()}_${editDrafts.length}`, title: '', description: '' },
    ]
  }

  function evidenceForTask(task: SessionTask): TaskEvidence[] {
    return Array.isArray(task.evidence) ? task.evidence : []
  }

  function evidenceTypeLabel(type: string): string {
    return evidenceTypeOptions.find((option) => option.value === type)?.label ?? 'Evidence'
  }

  function evidenceLabel(evidence: TaskEvidence): string {
    return evidence.title?.trim() || evidenceTypeLabel(evidence.type)
  }

  function evidenceMeta(evidence: TaskEvidence): string {
    return [evidence.status, evidence.command, evidence.path].filter(Boolean).join(' · ')
  }

  function startEvidence(taskId: string) {
    evidenceDraftTaskId = taskId
    evidenceType = 'test_result'
    evidenceTitle = ''
    evidenceSummary = ''
    evidenceURL = ''
    evidenceCommand = ''
    actionError = ''
  }

  function cancelEvidence() {
    evidenceDraftTaskId = ''
    evidenceTitle = ''
    evidenceSummary = ''
    evidenceURL = ''
    evidenceCommand = ''
  }

  async function saveEvidence(taskId: string) {
    try {
      await runAction({
        action: 'evidence_add',
        task_id: taskId,
        type: evidenceType,
        title: evidenceTitle,
        summary: evidenceSummary,
        url: evidenceURL,
        command: evidenceCommand,
      })
      cancelEvidence()
    } catch {
      return
    }
  }

  async function removeEvidence(taskId: string, evidenceId: string) {
    try {
      await runAction({ action: 'evidence_remove', task_id: taskId, evidence_id: evidenceId })
    } catch {
      return
    }
  }

  function removeDraft(idx: number) {
    editDrafts = editDrafts.filter((_, i) => i !== idx)
  }

  async function saveEdits() {
    actionBusy = true
    actionError = ''
    try {
      const originalById = new Map(taskList.map((t) => [t.id, t]))
      const draftIds = new Set<string>()

      for (const draft of editDrafts) {
        const title = draft.title.trim()
        if (!title) continue
        const description = draft.description.trim()
        const existing = originalById.get(draft.id)
        if (existing) {
          draftIds.add(draft.id)
          // Update only if something actually changed — avoids redundant
          // round-trips on a no-op save.
          if (existing.title !== title || (existing.description ?? '') !== description) {
            await executeTasksAction(sessionId, {
              action: 'update',
              id: draft.id,
              title,
              description,
            })
          }
        } else {
          // Newly inserted draft — backend assigns the real id.
          await executeTasksAction(sessionId, {
            action: 'add',
            title,
            description,
          })
        }
      }

      // Remove any tasks the user dropped from the list.
      for (const original of taskList) {
        if (!draftIds.has(original.id)) {
          await executeTasksAction(sessionId, { action: 'remove', id: original.id })
        }
      }

      editing = false
      editDrafts = []
      await load()
    } catch (err) {
      actionError = err instanceof Error ? err.message : 'Save failed'
    } finally {
      actionBusy = false
    }
  }

  function statusIcon(status: string): string {
    switch (status) {
      case 'completed': return '\u2714'
      case 'in_progress': return '\u25b6'
      case 'cancelled': return '\u2716'
      default: return '\u25cb'
    }
  }

  function statusClass(status: string): string {
    switch (status) {
      case 'completed': return 'badge-success'
      case 'in_progress': return 'badge-accent'
      case 'cancelled': return 'badge-error'
      default: return 'badge-default'
    }
  }

  function formatArchiveDate(value?: string): string {
    if (!value) return ''
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    return new Intl.DateTimeFormat(undefined, {
      month: 'short',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    }).format(date)
  }

  let summary = $derived(summarizeTasks(taskList))

  let progress = $derived(planProgressPercent(summary))

  onMount(() => { void load() })

  // Reload when sessionId changes
  $effect(() => {
    void sessionId
    void load()
  })
</script>

<div class="tasks-panel">
  <div class="panel-header">
    <div class="panel-title-row">
      <span class="card-title">{$t.tasks.title}</span>
      {#if summary.total > 0}
        <span class="badge badge-accent">{summary.completed}/{summary.total}</span>
      {/if}
    </div>
    <div class="panel-actions">
      <button class="btn btn-ghost btn-sm" type="button" onclick={load} title={$t.common.actions.refresh}>&#x21bb;</button>
      <button class="btn btn-ghost btn-sm" type="button" onclick={onClose} title={$t.common.actions.close}>&times;</button>
    </div>
  </div>

  {#if loading}
    <div class="empty-state">{$t.tasks.loading}</div>
  {:else if error}
    <div class="error-banner">{error}</div>
  {:else if !data.plan && taskList.length === 0}
    <div class="empty-state">
      <p>{$t.tasks.empty}</p>
      <p class="hint">{$t.tasks.emptyHint}</p>
    </div>
  {:else}
    {#if planStatus === 'proposed' && !editing}
      <div class="propose-banner">
        <p class="propose-title">{$t.tasks.planReady}</p>
        <p class="propose-hint">{$t.tasks.planReadyHint(summary.total)}</p>
        {#if actionError}
          <p class="error-banner">{actionError}</p>
        {/if}
        <div class="propose-actions">
          <button class="btn btn-primary btn-sm" type="button" disabled={actionBusy} onclick={handleApprove}>
            {actionBusy ? $t.common.actions.working : `\u2713 ${$t.tasks.approveRun}`}
          </button>
          <button class="btn btn-ghost btn-sm" type="button" disabled={actionBusy} onclick={startEdit}>
            {'\u270e'} {$t.tasks.editPlan}
          </button>
          <button class="btn btn-danger btn-sm" type="button" disabled={actionBusy} onclick={handleDiscard}>
            {'\u2717'} {$t.tasks.discard}
          </button>
        </div>
      </div>
    {/if}

    {#if data.plan}
      <div class="plan-section">
        <button class="plan-header" type="button" onclick={() => planExpanded = !planExpanded}>
          <span class="plan-toggle">{planExpanded ? '\u25be' : '\u25b8'}</span>
          <span class="plan-label">{$t.tasks.plan}</span>
          {#if planStatus}
            <span class="plan-status badge {planStatus === 'proposed' ? 'badge-warning' : planStatus === 'executing' ? 'badge-accent' : planStatus === 'completed' ? 'badge-success' : planStatus === 'aborted' ? 'badge-error' : 'badge-default'}">{planStatus}</span>
          {/if}
        </button>
        {#if planExpanded}
          <div class="plan-body">
            <p class="plan-goal">{data.plan.goal}</p>
            {#if data.plan.constraints}
              <p class="plan-constraints">{data.plan.constraints}</p>
            {/if}
          </div>
        {/if}
      </div>
    {/if}

    {#if !editing && (planStatus === 'executing' || planStatus === 'paused')}
      <div class="runtime-actions">
        {#if planStatus === 'executing'}
          <button class="btn btn-ghost btn-sm" type="button" disabled={actionBusy} onclick={handlePause} title="Pause execution: cancels the in-flight LLM turn and waits for instructions.">
            {'\u23f8'} {$t.tasks.pause}
          </button>
        {:else}
          <button class="btn btn-primary btn-sm" type="button" disabled={actionBusy} onclick={handleResume} title="Resume execution: flips status back to executing and sends 'continue' to the chat.">
            {'\u25b6'} {$t.tasks.resume}
          </button>
        {/if}
        <button class="btn btn-ghost btn-sm" type="button" disabled={actionBusy} onclick={startEdit} title="Edit plan in place: reorder, retitle, add or remove tasks.">
          {'\u270e'} {$t.tasks.editPlan}
        </button>
        <button class="btn btn-danger btn-sm" type="button" disabled={actionBusy} onclick={handleAbort} title="Abort plan: cancels execution and marks the plan aborted permanently.">
          {'\u2298'} {$t.tasks.abort}
        </button>
        {#if actionError}
          <p class="error-banner runtime-error">{actionError}</p>
        {/if}
      </div>
    {/if}

    {#if summary.total > 0}
      <div class="progress-section">
        <div class="progress-bar">
          <div class="progress-fill" style="width: {progress}%"></div>
        </div>
        <div class="progress-stats">
          {#if summary.in_progress > 0}<span class="stat-chip accent">{summary.in_progress} {$t.tasks.stats.active}</span>{/if}
          {#if summary.pending > 0}<span class="stat-chip">{summary.pending} {$t.tasks.stats.pending}</span>{/if}
          {#if summary.completed > 0}<span class="stat-chip success">{summary.completed} {$t.tasks.stats.done}</span>{/if}
          {#if summary.cancelled > 0}<span class="stat-chip error">{summary.cancelled} {$t.tasks.stats.skipped}</span>{/if}
        </div>
      </div>
    {/if}

    {#if editing}
      <div class="tasks-list">
        {#each editDrafts as draft, idx (draft.id)}
          <div class="task-edit-card">
            <div class="task-edit-row">
              <input
                class="task-edit-title"
                type="text"
                placeholder={$t.tasks.taskTitle}
                bind:value={editDrafts[idx].title}
                disabled={actionBusy}
              />
              <button
                class="btn btn-ghost btn-sm"
                type="button"
                title={$t.tasks.removeTask}
                disabled={actionBusy}
                onclick={() => removeDraft(idx)}
              >×</button>
            </div>
            <textarea
              class="task-edit-desc"
              placeholder={$t.tasks.descriptionOptional}
              rows="2"
              bind:value={editDrafts[idx].description}
              disabled={actionBusy}
            ></textarea>
          </div>
        {/each}
        <button class="btn btn-ghost btn-sm task-add-btn" type="button" disabled={actionBusy} onclick={addDraft}>
          + {$t.tasks.addTask}
        </button>
      </div>
      {#if actionError}
        <p class="error-banner">{actionError}</p>
      {/if}
      <div class="edit-actions">
        <button class="btn btn-primary btn-sm" type="button" disabled={actionBusy} onclick={saveEdits}>
          {actionBusy ? $t.common.actions.saving : `\u2713 ${$t.tasks.saveChanges}`}
        </button>
        <button class="btn btn-ghost btn-sm" type="button" disabled={actionBusy} onclick={cancelEdit}>
          {$t.common.actions.cancel}
        </button>
      </div>
    {:else}
      <div class="tasks-list">
        {#each taskList as task (task.id)}
          <div class="task-card" class:completed={task.status === 'completed'} class:cancelled={task.status === 'cancelled'} class:active={task.status === 'in_progress'}>
            <span class="task-status-icon">{statusIcon(task.status)}</span>
            <div class="task-content">
              <span class="task-title">{task.title}</span>
              {#if task.description}
                <span class="task-desc">{task.description}</span>
              {/if}
              {#if evidenceForTask(task).length > 0}
                <div class="task-evidence-list" aria-label={`Evidence for ${task.title}`}>
                  {#each evidenceForTask(task) as evidence (evidence.id)}
                    <article class="task-evidence-card">
                      <div class="task-evidence-head">
                        <span>{evidenceLabel(evidence)}</span>
                        <span class="evidence-type">{evidenceTypeLabel(evidence.type)}</span>
                      </div>
                      {#if evidence.summary}
                        <p>{evidence.summary}</p>
                      {/if}
                      {#if evidenceMeta(evidence)}
                        <small>{evidenceMeta(evidence)}</small>
                      {/if}
                      {#if evidence.url}
                        <a href={evidence.url} target="_blank" rel="noreferrer">{evidence.url}</a>
                      {/if}
                      <button
                        class="btn btn-ghost btn-sm evidence-remove-btn"
                        type="button"
                        disabled={actionBusy}
                        onclick={() => removeEvidence(task.id, evidence.id)}
                      >
                        Remove Evidence
                      </button>
                    </article>
                  {/each}
                </div>
              {/if}
              {#if evidenceDraftTaskId === task.id}
                <div class="evidence-form">
                  <select bind:value={evidenceType} disabled={actionBusy} aria-label="Evidence type">
                    {#each evidenceTypeOptions as option}
                      <option value={option.value}>{option.label}</option>
                    {/each}
                  </select>
                  <input bind:value={evidenceTitle} disabled={actionBusy} placeholder="Evidence title" />
                  <textarea bind:value={evidenceSummary} disabled={actionBusy} rows="2" placeholder="Short summary"></textarea>
                  <input bind:value={evidenceCommand} disabled={actionBusy} placeholder="Command or release tag" />
                  <input bind:value={evidenceURL} disabled={actionBusy} placeholder="URL or image path" />
                  <div class="evidence-actions">
                    <button class="btn btn-primary btn-sm" type="button" disabled={actionBusy} onclick={() => saveEvidence(task.id)}>
                      {actionBusy ? 'Saving...' : 'Attach Evidence'}
                    </button>
                    <button class="btn btn-ghost btn-sm" type="button" disabled={actionBusy} onclick={cancelEvidence}>Cancel</button>
                  </div>
                </div>
              {/if}
            </div>
            <span class="badge {statusClass(task.status)}">{task.status.replace('_', ' ')}</span>
            <button
              class="btn btn-ghost btn-sm evidence-add-btn"
              type="button"
              disabled={actionBusy}
              onclick={() => startEvidence(task.id)}
            >
              + Evidence
            </button>
            {#if (planStatus === 'executing' || planStatus === 'paused') && (task.status === 'pending' || task.status === 'in_progress')}
              <button
                class="btn btn-ghost btn-sm task-skip-btn"
                type="button"
                disabled={actionBusy}
                title={$t.tasks.skipTask}
                onclick={() => handleSkipTask(task.id, task.title)}
              >{'⏭'}</button>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  {/if}

  {#if !loading && !error}
    <section class="archive-section">
      <button class="archive-header" type="button" onclick={() => archiveExpanded = !archiveExpanded}>
        <span class="plan-toggle">{archiveExpanded ? '\u25be' : '\u25b8'}</span>
        <span>{$t.tasks.archive.pastPlans(archiveItems.length)}</span>
      </button>
      {#if archiveExpanded}
        {#if archiveError}
          <p class="error-banner archive-error">{archiveError}</p>
        {:else if archiveItems.length === 0}
          <div class="empty-state archive-empty">{$t.tasks.archive.empty}</div>
        {:else}
          <div class="archive-items">
            {#each archiveItems as item (item.id)}
              <article class="archive-card">
                <button class="archive-card-header" type="button" onclick={() => expandedArchiveId = expandedArchiveId === item.id ? '' : item.id}>
                  <span>
                    <strong>{item.goal}</strong>
                    <small>{$t.tasks.archive.archivedAt} {formatArchiveDate(item.archived_at)}</small>
                    {#if item.created_at}
                      <small>{$t.tasks.archive.createdAt(formatArchiveDate(item.created_at))}</small>
                    {/if}
                  </span>
                  <span class="plan-toggle">{expandedArchiveId === item.id ? '\u25be' : '\u25b8'}</span>
                </button>
                {#if expandedArchiveId === item.id}
                  <pre>{item.summary}</pre>
                {/if}
              </article>
            {/each}
          </div>
        {/if}
      {/if}
    </section>
  {/if}
</div>

<style>
  .tasks-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    height: 100%;
    overflow-y: auto;
  }

  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .panel-title-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .panel-actions {
    display: flex;
    gap: var(--space-1);
  }

  .empty-state {
    padding: var(--space-6) var(--space-4);
    text-align: center;
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .hint {
    margin-top: var(--space-2);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .propose-banner {
    padding: var(--space-3);
    background: color-mix(in srgb, var(--warning) 8%, var(--surface-elevated));
    border: 1px solid color-mix(in srgb, var(--warning) 30%, var(--border-default));
    border-radius: var(--radius-md, 0.375rem);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .propose-title {
    margin: 0;
    font-weight: 600;
    color: var(--text-primary);
  }

  .propose-hint {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .propose-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-top: var(--space-1);
  }

  .plan-status {
    margin-left: auto;
  }

  .task-edit-card {
    padding: var(--space-2);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm, 0.25rem);
    background: var(--surface-inset);
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .task-edit-row {
    display: flex;
    gap: var(--space-1);
    align-items: center;
  }

  .task-edit-title {
    flex: 1;
    padding: var(--space-1) var(--space-2);
    background: var(--surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm, 0.25rem);
    color: var(--text-primary);
    font-size: var(--text-sm);
  }

  .task-edit-desc {
    width: 100%;
    padding: var(--space-1) var(--space-2);
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm, 0.25rem);
    color: var(--text-secondary);
    font-size: var(--text-xs);
    resize: vertical;
    font-family: inherit;
  }

  .task-add-btn {
    align-self: flex-start;
  }

  .edit-actions {
    display: flex;
    gap: var(--space-2);
  }

  .runtime-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    padding: var(--space-2);
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
  }

  .runtime-error {
    flex-basis: 100%;
    margin: var(--space-1) 0 0;
  }

  .task-skip-btn {
    padding: 0 var(--space-2);
    min-width: 1.75rem;
    margin-left: var(--space-1);
  }

  .plan-section {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    overflow: hidden;
  }

  .archive-section {
    border-top: 1px solid var(--border-subtle);
    padding-top: var(--space-3);
  }

  .archive-header,
  .archive-card-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
    border: none;
    background: transparent;
    color: var(--text-primary);
    cursor: pointer;
    text-align: left;
  }

  .archive-header {
    padding: 0;
    font-family: var(--font-display);
    font-size: var(--text-xs);
  }

  .archive-items {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }

  .archive-card {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
  }

  .archive-card-header {
    justify-content: space-between;
    padding: var(--space-3);
  }

  .archive-card-header > span:first-child {
    display: flex;
    min-width: 0;
    flex-direction: column;
    gap: 2px;
  }

  .archive-card strong {
    overflow: hidden;
    color: var(--text-primary);
    font-size: var(--text-sm);
    font-weight: 500;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .archive-card small {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .archive-card pre {
    margin: 0;
    padding: 0 var(--space-3) var(--space-3);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    line-height: 1.5;
    white-space: pre-wrap;
  }

  .archive-error,
  .archive-empty {
    margin-top: var(--space-2);
  }

  .plan-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
    padding: var(--space-3);
    background: none;
    border: none;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    cursor: pointer;
  }

  .plan-toggle {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .plan-body {
    padding: 0 var(--space-3) var(--space-3);
  }

  .plan-goal {
    font-size: var(--text-sm);
    color: var(--text-primary);
    line-height: 1.5;
  }

  .plan-constraints {
    margin-top: var(--space-2);
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .progress-section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .progress-bar {
    height: 4px;
    background: var(--surface-inset);
    border-radius: 2px;
    overflow: hidden;
  }

  .progress-fill {
    height: 100%;
    background: var(--primary);
    border-radius: 2px;
    transition: width 0.3s var(--ease-out);
  }

  .progress-stats {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }

  .stat-chip {
    padding: 2px var(--space-2);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    color: var(--text-secondary);
    background: var(--surface-inset);
  }

  .stat-chip.accent { color: var(--primary-text); background: rgba(224, 145, 69, 0.12); }
  .stat-chip.success { color: var(--success); background: rgba(74, 222, 128, 0.12); }
  .stat-chip.error { color: var(--error); background: rgba(239, 68, 68, 0.12); }

  .tasks-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .task-card {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    transition: border-color var(--duration-fast) var(--ease-out);
  }

  .task-card.active {
    border-color: var(--primary);
    background: rgba(224, 145, 69, 0.06);
  }

  .task-card.completed {
    opacity: 0.6;
  }

  .task-card.cancelled {
    opacity: 0.4;
  }

  .task-status-icon {
    flex-shrink: 0;
    width: 18px;
    text-align: center;
    font-size: var(--text-sm);
    line-height: 1.4;
  }

  .task-card.active .task-status-icon { color: var(--primary); }
  .task-card.completed .task-status-icon { color: var(--success); }
  .task-card.cancelled .task-status-icon { color: var(--text-tertiary); }

  .task-content {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .task-title {
    font-size: var(--text-sm);
    color: var(--text-primary);
    line-height: 1.4;
  }

  .task-desc {
    font-size: var(--text-xs);
    color: var(--text-secondary);
    line-height: 1.4;
  }

  .task-evidence-list {
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }

  .task-evidence-card,
  .evidence-form {
    display: grid;
    gap: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface);
    padding: var(--space-2);
  }

  .task-evidence-head,
  .evidence-actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .task-evidence-head span:first-child {
    color: var(--text-primary);
    font-size: var(--text-xs);
    font-weight: 600;
  }

  .evidence-type {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    background: var(--surface-inset);
    padding: 2px var(--space-2);
    font-size: 10px;
    text-transform: uppercase;
  }

  .task-evidence-card p,
  .task-evidence-card small,
  .task-evidence-card a {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--text-xs);
    overflow-wrap: anywhere;
  }

  .task-evidence-card small {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
  }

  .evidence-form input,
  .evidence-form select,
  .evidence-form textarea {
    width: 100%;
    min-width: 0;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-primary);
    padding: var(--space-2);
    font: inherit;
    font-size: var(--text-xs);
  }

  .evidence-form textarea {
    resize: vertical;
  }

  .evidence-add-btn,
  .evidence-remove-btn {
    white-space: nowrap;
  }
</style>
