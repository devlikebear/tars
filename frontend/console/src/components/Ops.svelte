<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import {
    createCleanupPlan,
    listAutomationAudit,
    listApprovals,
    reviewApproval,
    streamEvents,
  } from '../lib/api'
  import type { Approval, AutomationAuditEntry } from '../lib/types'

  type ApprovalGuideStep = {
    title: string
    detail: string
  }

  type ApprovalTriggerGuide = {
    title: string
    detail: string
    state: string
  }

  const cleanupPlanTooltip = 'Scans unused temporary files and empty sessions from the last 30 days. Actual deletion waits for approval.'

  const approvalTriggerGuide: ApprovalTriggerGuide[] = [
    {
      title: 'New cleanup plan',
      detail: 'Scans unused temporary files, stale workspace scratch data, and empty sessions so you can review cleanup candidates first.',
      state: 'available',
    },
    {
      title: 'future Pulse signals',
      detail: 'Pulse findings that are not safe to autofix can later land here for human review.',
      state: 'planned',
    },
  ]

  const approvalGuideSteps: ApprovalGuideStep[] = [
    {
      title: 'Review the candidate list',
      detail: 'Each request shows file paths, size, and reason before anything changes.',
    },
    {
      title: 'Choose Approve or Reject',
      detail: 'Approve applies the plan; Reject discards it without touching the workspace.',
    },
    {
      title: 'Read the result log',
      detail: 'Applied approvals keep a result log so you can see what changed after the action runs.',
    },
  ]

  let approvals: Approval[] = $state([])
  let loading = $state(true)
  let error = $state('')
  let reviewingId = $state('')
  let planCreating = $state(false)
  let auditEntries: AutomationAuditEntry[] = $state([])
  let auditLoading = $state(false)
  let stopStream: (() => void) | null = null

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function fmtBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`
  }

  function compact(value?: string, max = 120): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    return text.length <= max ? text : `${text.slice(0, max - 1)}\u2026`
  }

  function statusBadge(s: string): string {
    switch (s) {
      case 'pending': return 'badge-warning'
      case 'approved': return 'badge-success'
      case 'rejected': return 'badge-error'
      case 'applied': return 'badge-info'
      default: return 'badge-default'
    }
  }

  async function load() {
    loading = true
    error = ''
    try {
      const [approvalList, auditList] = await Promise.all([
        listApprovals(),
        listAutomationAudit(25),
      ])
      approvals = approvalList
      auditEntries = auditList.items ?? []
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load approvals'
    } finally {
      loading = false
    }
  }

  async function handleReview(approvalId: string, action: 'approve' | 'reject') {
    reviewingId = approvalId
    try {
      await reviewApproval(approvalId, action)
      const [approvalList, auditList] = await Promise.all([listApprovals(), listAutomationAudit(25)])
      approvals = approvalList
      auditEntries = auditList.items ?? []
    } catch (err) {
      error = err instanceof Error ? err.message : 'Review failed'
    } finally {
      reviewingId = ''
    }
  }

  async function refreshAudit() {
    auditLoading = true
    try {
      const res = await listAutomationAudit(25)
      auditEntries = res.items ?? []
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load automation audit'
    } finally {
      auditLoading = false
    }
  }

  async function handleCreatePlan() {
    planCreating = true
    error = ''
    try {
      await createCleanupPlan()
      approvals = await listApprovals()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to create cleanup plan'
    } finally {
      planCreating = false
    }
  }

  onMount(() => {
    void load()
    stopStream = streamEvents(
      (event) => {
        if (event.category === 'ops') {
          void listApprovals().then((list) => { approvals = list })
          void refreshAudit()
        }
      },
    )
  })

  onDestroy(() => {
    stopStream?.()
  })
</script>

<div class="ops">
  <div class="ops-header">
    <div>
      <h2>Approvals</h2>
      <p class="ops-subtitle">Review risky cleanup plans before TARS applies them.</p>
    </div>
    <button type="button" class="btn btn-ghost btn-sm" onclick={() => { void load() }}>
      Refresh
    </button>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  {#if loading}
    <div class="ops-loading">Loading approvals...</div>
  {:else}
    <section class="card approvals-section">
      <div class="card-header">
        <span class="card-title">Approvals</span>
        <div class="card-header-actions">
          {#if approvals.filter((a) => a.status === 'pending').length > 0}
            <span class="badge badge-warning">{approvals.filter((a) => a.status === 'pending').length} pending</span>
          {/if}
          <button
            type="button"
            class="btn btn-ghost btn-sm"
            title={cleanupPlanTooltip}
            disabled={planCreating}
            onclick={handleCreatePlan}
          >
            {planCreating ? 'Creating...' : 'New cleanup plan'}
          </button>
        </div>
      </div>

      {#if approvals.length === 0}
        <div class="approval-empty-guide">
          <div class="approval-empty-intro">
            <span class="approval-empty-kicker">Approvals</span>
            <h3>Approvals review queue</h3>
            <p>
              Approvals are where riskier workspace changes wait for your review before TARS applies them.
            </p>
          </div>

          <div class="approval-empty-grid">
            <div>
              <div class="approval-empty-label">Current triggers</div>
              <ul class="approval-empty-list">
                {#each approvalTriggerGuide as trigger}
                  <li>
                    <div>
                      <strong>{trigger.title}</strong>
                      <span class="badge badge-default">{trigger.state}</span>
                    </div>
                    <span>{trigger.detail}</span>
                  </li>
                {/each}
              </ul>
            </div>

            <div>
              <div class="approval-empty-label">How each approval works</div>
              <ol class="approval-step-list">
                {#each approvalGuideSteps as step}
                  <li>
                    <strong>{step.title}</strong>
                    <span>{step.detail}</span>
                  </li>
                {/each}
              </ol>
            </div>
          </div>
        </div>
      {:else}
        <div class="approval-list">
          {#each approvals as approval}
            <div class="approval-item" class:approval-pending={approval.status === 'pending'}>
              <div class="approval-top">
                <div class="approval-info">
                  <strong class="mono">{approval.id}</strong>
                  <span class="badge {statusBadge(approval.status)}">{approval.status}</span>
                </div>
                <span class="approval-time">{fmt(approval.requested_at)}</span>
              </div>

              <div class="approval-detail">
                <span>{approval.plan.candidates.length} candidates</span>
                <span class="approval-dot"></span>
                <span>{fmtBytes(approval.plan.total_bytes)}</span>
                {#if approval.note}
                  <span class="approval-dot"></span>
                  <span class="approval-note" class:approval-result={approval.status === 'applied'}>{compact(approval.note, 120)}</span>
                {/if}
              </div>

              {#if approval.status === 'pending'}
                <div class="approval-actions">
                  <button
                    type="button"
                    class="btn btn-sm btn-primary"
                    disabled={reviewingId === approval.id}
                    onclick={() => { void handleReview(approval.id, 'approve') }}
                  >
                    Approve
                  </button>
                  <button
                    type="button"
                    class="btn btn-sm btn-danger"
                    disabled={reviewingId === approval.id}
                    onclick={() => { void handleReview(approval.id, 'reject') }}
                  >
                    Reject
                  </button>
                </div>
              {/if}

              {#if approval.plan.candidates.length > 0}
                <details class="approval-candidates">
                  <summary>{approval.plan.candidates.length} cleanup candidates</summary>
                  <div class="candidate-list">
                    {#each approval.plan.candidates as candidate}
                      <div class="candidate-row">
                        <span class="mono candidate-path">{candidate.path}</span>
                        <span class="candidate-size">{fmtBytes(candidate.size_bytes)}</span>
                        {#if candidate.reason}
                          <span class="candidate-reason">{candidate.reason}</span>
                        {/if}
                      </div>
                    {/each}
                  </div>
                </details>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </section>
    <section class="card audit-section">
      <div class="card-header">
        <span class="card-title">Automation Audit</span>
        <div class="card-header-actions">
          <span class="badge badge-default">{auditEntries.length} events</span>
          <button type="button" class="btn btn-ghost btn-sm" disabled={auditLoading} onclick={() => { void refreshAudit() }}>
            {auditLoading ? 'Refreshing...' : 'Refresh'}
          </button>
        </div>
      </div>
      {#if auditEntries.length === 0}
        <div class="ops-loading">No automation events yet.</div>
      {:else}
        <div class="audit-list">
          {#each auditEntries as entry}
            <div class="audit-item">
              <div class="audit-top">
                <strong>{entry.action}</strong>
                <span class="badge {entry.result === 'success' ? 'badge-success' : entry.result === 'blocked' ? 'badge-warning' : 'badge-default'}">{entry.result}</span>
                <span class="audit-time">{fmt(entry.timestamp)}</span>
              </div>
              <div class="audit-detail">
                <span>{entry.actor}</span>
                {#if entry.session_id}
                  <span class="approval-dot"></span>
                  <span>{entry.session_id}</span>
                {/if}
                {#if entry.cwd}
                  <span class="approval-dot"></span>
                  <span class="mono audit-cwd">{compact(entry.cwd, 90)}</span>
                {/if}
              </div>
              {#if entry.reason}
                <div class="audit-reason">{entry.reason}</div>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </section>
  {/if}
</div>

<style>
  .ops {
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .ops-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-6);
  }

  .ops-header h2 {
    font-size: var(--text-2xl);
    margin-bottom: var(--space-1);
  }

  .ops-subtitle {
    color: var(--text-tertiary);
  }

  .ops-loading {
    padding: var(--space-10);
    text-align: center;
    color: var(--text-tertiary);
  }

  .approvals-section {
    min-height: 200px;
  }

  .audit-section {
    margin-top: var(--space-4);
  }

  .card-header-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .approval-empty-guide {
    display: grid;
    gap: var(--space-4);
    margin-top: var(--space-3);
    padding-top: var(--space-3);
    border-top: 1px solid var(--border-subtle);
  }

  .approval-empty-intro {
    display: grid;
    gap: var(--space-1);
  }

  .approval-empty-kicker,
  .approval-empty-label {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: 0;
    text-transform: uppercase;
  }

  .approval-empty-intro h3 {
    margin: 0;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-lg);
    font-weight: 600;
  }

  .approval-empty-intro p {
    max-width: 720px;
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
    line-height: 1.55;
  }

  .approval-empty-grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: var(--space-4);
  }

  .approval-empty-list,
  .approval-step-list {
    display: grid;
    gap: var(--space-2);
    margin: var(--space-2) 0 0;
    padding: 0;
    list-style: none;
  }

  .approval-empty-list li,
  .approval-step-list li {
    display: grid;
    gap: var(--space-1);
    padding-top: var(--space-2);
    border-top: 1px solid var(--border-subtle);
  }

  .approval-empty-list li:first-child,
  .approval-step-list li:first-child {
    padding-top: 0;
    border-top: 0;
  }

  .approval-empty-list li > div {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .approval-empty-list strong,
  .approval-step-list strong {
    color: var(--text-primary);
    font-size: var(--text-sm);
    font-weight: 500;
  }

  .approval-empty-list span:not(.badge),
  .approval-step-list span {
    color: var(--text-secondary);
    font-size: var(--text-sm);
    line-height: 1.5;
  }

  .approval-list {
    display: grid;
    gap: var(--space-2);
  }

  .approval-item {
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-md);
    background: var(--surface-base);
    border: 1px solid transparent;
  }

  .approval-pending {
    border-color: rgba(251, 191, 36, 0.2);
    background: rgba(251, 191, 36, 0.04);
  }

  .approval-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: var(--space-1);
  }

  .approval-info {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .approval-info strong {
    font-size: var(--text-xs);
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .approval-time {
    font-size: var(--text-xs);
    color: var(--text-ghost);
    flex-shrink: 0;
  }

  .approval-detail {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    color: var(--text-secondary);
    margin-bottom: var(--space-2);
  }

  .approval-dot {
    width: 3px;
    height: 3px;
    border-radius: 50%;
    background: var(--text-ghost);
    flex-shrink: 0;
  }

  .approval-note {
    color: var(--text-tertiary);
  }

  .approval-note.approval-result {
    color: var(--green);
    font-weight: 500;
  }

  .approval-actions {
    display: flex;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }

  .approval-candidates {
    margin-top: var(--space-2);
  }

  .approval-candidates summary {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    cursor: pointer;
    user-select: none;
  }

  .approval-candidates summary:hover {
    color: var(--text-secondary);
  }

  .candidate-list {
    display: grid;
    gap: var(--space-1);
    margin-top: var(--space-2);
    padding: var(--space-2) var(--space-3);
    background: var(--surface);
    border-radius: var(--radius-sm);
    max-height: 200px;
    overflow-y: auto;
  }

  .candidate-row {
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
    font-size: var(--text-xs);
  }

  .candidate-path {
    color: var(--text-secondary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
    flex: 1;
  }

  .candidate-size {
    color: var(--text-tertiary);
    flex-shrink: 0;
  }

  .candidate-reason {
    color: var(--text-ghost);
    flex-shrink: 0;
  }

  .audit-list {
    display: grid;
    gap: var(--space-2);
  }

  .audit-item {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
  }

  .audit-top,
  .audit-detail {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
    flex-wrap: wrap;
  }

  .audit-top strong {
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .audit-time,
  .audit-detail,
  .audit-reason {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .audit-cwd {
    color: var(--text-secondary);
  }

  @media (max-width: 760px) {
    .ops-header,
    .card-header-actions,
    .approval-top,
    .approval-detail,
    .audit-top,
    .audit-detail,
    .candidate-row {
      align-items: flex-start;
      flex-direction: column;
    }

    .approval-empty-grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
