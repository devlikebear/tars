<script lang="ts">
  import { onMount } from 'svelte'
  import { locale, t } from '../i18n'
  import {
    extractSkillsFromSession,
    listSessionLocalSkills,
    listSkillExtractions,
    promoteSessionLocalSkills,
    reviewSkillExtractionCandidate,
  } from '../lib/api'
  import type {
    CapabilityOutcome,
    CapabilityVersion,
    EvaluationRun,
    SessionLocalSkillItem,
    SessionLocalSkillPromoteConflict,
    SessionLocalSkillPromoteMode,
    SkillExtractionCandidate,
    SkillExtractionCandidateAction,
  } from '../lib/types'

  interface Props {
    sessionId: string
    onClose?: () => void
    onApproved?: (path: string) => void
  }

  let { sessionId, onClose, onApproved }: Props = $props()

  type InboxTab = 'extracted' | 'session'

  let activeTab: InboxTab = $state('extracted')
  let candidates: SkillExtractionCandidate[] = $state([])
  let capabilities: CapabilityVersion[] = $state([])
  let evaluations: EvaluationRun[] = $state([])
  let outcomes: CapabilityOutcome[] = $state([])
  let localSkills: SessionLocalSkillItem[] = $state([])
  let cwd = $state('')
  let loading = $state(false)
  let loadingLocal = $state(false)
  let extracting = $state(false)
  let reviewing = $state('')
  let promoting = $state(false)
  let error = $state('')
  let success = $state('')

  let mode: SessionLocalSkillPromoteMode = $state('copy')
  let selected: Set<string> = $state(new Set())

  let conflictDialogOpen = $state(false)
  let conflictNames: string[] = $state([])
  let conflictChoice: SessionLocalSkillPromoteConflict = $state('rename')
  let pendingPromoteNames: string[] = $state([])

  let pendingExtracted = $derived(candidates.filter((candidate) => candidate.status === 'pending').length)
  let pendingLocal = $derived(localSkills.length)

  function fmtDate(value?: string): string {
    if (!value) return ''
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    return date.toLocaleString($locale)
  }

  function tools(candidate: SkillExtractionCandidate): string {
    return (candidate.recommended_tools ?? []).join(', ')
  }

  function capabilityFor(candidate: SkillExtractionCandidate): CapabilityVersion | undefined {
    return capabilities.find((version) => version.candidate_id === candidate.id)
  }

  function evaluationsFor(version?: CapabilityVersion): EvaluationRun[] {
    if (!version) return []
    return evaluations.filter((run) => run.capability_version_id === version.id)
  }

  function latestEvaluation(version?: CapabilityVersion): EvaluationRun | undefined {
    const runs = evaluationsFor(version)
    return runs.length > 0 ? runs[runs.length - 1] : undefined
  }

  function outcomesFor(version?: CapabilityVersion): CapabilityOutcome[] {
    if (!version) return []
    return outcomes.filter((outcome) => outcome.capability_version_id === version.id)
  }

  function formatDelta(value?: number, suffix = ''): string {
    if (value === undefined || !Number.isFinite(value)) return '—'
    const prefix = value > 0 ? '+' : ''
    return `${prefix}${value.toFixed(suffix === ' ms' ? 0 : 3)}${suffix}`
  }

  async function loadExtracted() {
    loading = true
    error = ''
    try {
      const res = await listSkillExtractions('all')
      candidates = res.candidates ?? []
      capabilities = res.capabilities ?? []
      evaluations = res.evaluations ?? []
      outcomes = res.outcomes ?? []
    } catch (err) {
      error = err instanceof Error ? err.message : $t.skillInbox.errors.loadInbox
    } finally {
      loading = false
    }
  }

  async function loadLocal() {
    if (!sessionId) {
      localSkills = []
      cwd = ''
      return
    }
    loadingLocal = true
    error = ''
    try {
      const res = await listSessionLocalSkills(sessionId)
      localSkills = res.items ?? []
      cwd = res.cwd ?? ''
      // Drop any stale selections that no longer exist.
      const live = new Set(localSkills.filter((s) => s.kind === 'skill').map((s) => s.name))
      selected = new Set([...selected].filter((name) => live.has(name)))
    } catch (err) {
      error = err instanceof Error ? err.message : $t.skillInbox.errors.loadSessionSkills
    } finally {
      loadingLocal = false
    }
  }

  async function reload() {
    if (activeTab === 'extracted') {
      await loadExtracted()
    } else {
      await loadLocal()
    }
  }

  async function extract() {
    if (!sessionId || extracting) return
    extracting = true
    error = ''
    success = ''
    try {
      const res = await extractSkillsFromSession(sessionId, 5)
      candidates = res.candidates ?? []
      capabilities = res.capabilities ?? []
      evaluations = res.evaluations ?? []
      outcomes = res.outcomes ?? []
      success = res.count > 0 ? $t.skillInbox.feedback.queued(res.count) : $t.skillInbox.feedback.noCandidatesFound
    } catch (err) {
      error = err instanceof Error ? err.message : $t.skillInbox.errors.extract
    } finally {
      extracting = false
    }
  }

  async function review(candidate: SkillExtractionCandidate, action: SkillExtractionCandidateAction) {
    if (!candidate.id || reviewing) return
    reviewing = candidate.id
    error = ''
    success = ''
    try {
      const res = await reviewSkillExtractionCandidate(candidate.id, action)
      switch (action) {
        case 'evaluate':
          success = $t.skillInbox.feedback.evaluated(res.capability.capability_name)
          break
        case 'approve':
          success = $t.skillInbox.feedback.approved(res.capability.capability_name)
          break
        case 'promote':
          success = $t.skillInbox.feedback.promoted(res.capability.capability_name)
          onApproved?.(res.saved?.path ?? '')
          break
        case 'rollback':
          success = $t.skillInbox.feedback.rolledBack(res.capability.capability_name)
          break
        case 'reject':
          success = $t.skillInbox.feedback.rejected(candidate.name)
          break
      }
      await loadExtracted()
    } catch (err) {
      error = err instanceof Error ? err.message : $t.skillInbox.errors.review
    } finally {
      reviewing = ''
    }
  }

  function setTab(next: InboxTab) {
    activeTab = next
    error = ''
    success = ''
    if (next === 'session' && localSkills.length === 0 && !loadingLocal) {
      void loadLocal()
    }
  }

  function toggleSelect(name: string) {
    const next = new Set(selected)
    if (next.has(name)) next.delete(name)
    else next.add(name)
    selected = next
  }

  function selectAll() {
    selected = new Set(localSkills.filter((item) => item.kind === 'skill').map((item) => item.name))
  }

  function clearSelection() {
    selected = new Set()
  }

  function attemptPromote(names: string[]) {
    if (names.length === 0) return
    const collisionSet = new Set(
      localSkills.filter((item) => item.has_workspace_collision).map((item) => item.name),
    )
    const collisions = names.filter((name) => collisionSet.has(name))
    if (collisions.length > 0) {
      pendingPromoteNames = names
      conflictNames = collisions
      conflictChoice = 'rename'
      conflictDialogOpen = true
      return
    }
    void doPromote(names, 'rename')
  }

  function confirmConflictDialog() {
    if (conflictChoice === 'abort') {
      conflictDialogOpen = false
      pendingPromoteNames = []
      conflictNames = []
      return
    }
    const names = pendingPromoteNames
    const choice = conflictChoice
    conflictDialogOpen = false
    pendingPromoteNames = []
    conflictNames = []
    void doPromote(names, choice)
  }

  function cancelConflictDialog() {
    conflictDialogOpen = false
    pendingPromoteNames = []
    conflictNames = []
  }

  async function doPromote(names: string[], onConflict: SessionLocalSkillPromoteConflict) {
    if (!sessionId || promoting || names.length === 0) return
    promoting = true
    error = ''
    success = ''
    try {
      const res = await promoteSessionLocalSkills(sessionId, {
        items: names.map((name) => ({ name })),
        mode,
        on_conflict: onConflict,
      })
      const promotedCount = res.promoted?.length ?? 0
      const failedCount = res.failed?.length ?? 0
      if (promotedCount > 0 && failedCount === 0) {
        success = $t.skillInbox.feedback.promotedToWorkspace(promotedCount)
      } else if (promotedCount > 0 && failedCount > 0) {
        success = $t.skillInbox.feedback.promotedPartially(promotedCount, failedCount)
      } else if (failedCount > 0) {
        error = res.failed[0]?.error || $t.skillInbox.errors.promotion
      }
      selected = new Set()
      await loadLocal()
    } catch (err) {
      error = err instanceof Error ? err.message : $t.skillInbox.errors.promote
    } finally {
      promoting = false
    }
  }

  function promoteSelected() {
    attemptPromote([...selected])
  }

  function promoteOne(name: string) {
    attemptPromote([name])
  }

  onMount(() => {
    void loadExtracted()
    if (sessionId) void loadLocal()
  })
</script>

<div class="skill-extraction-panel">
  <header class="panel-header">
    <div>
      <strong>{$t.skillInbox.title}</strong>
      <span>
        {activeTab === 'extracted' ? $t.skillInbox.pendingCount(pendingExtracted) : $t.skillInbox.sessionSkillCount(pendingLocal)}
      </span>
    </div>
    <button class="btn btn-ghost btn-sm" type="button" onclick={() => onClose?.()}>{$t.skillInbox.close}</button>
  </header>

  <nav class="tab-bar" aria-label={$t.skillInbox.tabsAriaLabel}>
    <button
      class="tab"
      class:tab-active={activeTab === 'extracted'}
      type="button"
      onclick={() => setTab('extracted')}
    >
      {$t.skillInbox.tabs.extracted}
      <span class="tab-count">{pendingExtracted}</span>
    </button>
    <button
      class="tab"
      class:tab-active={activeTab === 'session'}
      type="button"
      onclick={() => setTab('session')}
    >
      {$t.skillInbox.tabs.session}
      <span class="tab-count">{pendingLocal}</span>
    </button>
  </nav>

  {#if error}
    <div class="message message-error">{error}</div>
  {/if}
  {#if success}
    <div class="message message-success">{success}</div>
  {/if}

  {#if activeTab === 'extracted'}
    <div class="panel-actions">
      <button class="btn btn-primary btn-sm" type="button" disabled={extracting || !sessionId} onclick={extract}>
        {extracting ? $t.skillInbox.extracting : $t.skillInbox.extract}
      </button>
      <button class="btn btn-ghost btn-sm" type="button" disabled={loading} onclick={loadExtracted}>
        {loading ? $t.skillInbox.loading : $t.skillInbox.reload}
      </button>
    </div>

    {#if loading && candidates.length === 0}
      <div class="empty-state">{$t.skillInbox.loadingCandidates}</div>
    {:else if candidates.length === 0}
      <div class="empty-state">{$t.skillInbox.noCandidates}</div>
    {:else}
      <div class="candidate-list">
        {#each candidates as candidate}
          {@const capability = capabilityFor(candidate)}
          {@const capabilityEvaluations = evaluationsFor(capability)}
          {@const capabilityOutcomes = outcomesFor(capability)}
          {@const latest = latestEvaluation(capability)}
          <article
            class="candidate-card"
            class:approved={candidate.status === 'approved'}
            class:rejected={candidate.status === 'rejected'}
          >
            <div class="candidate-main">
              <div>
                <strong>{candidate.title || candidate.name}</strong>
                <span class="candidate-name">{candidate.name}</span>
              </div>
              <div class="badges">
                {#if capability}
                  <span class="badge badge-soft">v{capability.version}</span>
                  <span class="badge {capability.state === 'promoted' ? 'badge-success' : capability.state === 'rejected' || capability.state === 'rolled_back' ? 'badge-error' : 'badge-default'}">{capability.state}</span>
                  {#if capability.rollout?.review_required}<span class="badge badge-warn">{$t.skillInbox.candidate.regressionReview}</span>{/if}
                {:else}
                  <span class="badge {candidate.status === 'approved' ? 'badge-success' : candidate.status === 'rejected' ? 'badge-error' : 'badge-default'}">{candidate.status}</span>
                {/if}
              </div>
            </div>
            <p>{candidate.summary}</p>
            <div class="candidate-meta">
              {#if candidate.trigger}<span>{candidate.trigger}</span>{/if}
              {#if tools(candidate)}<span>{$t.skillInbox.candidate.tools(tools(candidate))}</span>{/if}
              {#if candidate.repeated_count}<span>{$t.skillInbox.candidate.evidenceCount(candidate.repeated_count)}</span>{/if}
              {#if candidate.signals?.length}<span>{candidate.signals.map((signal) => signal.kind).join(' · ')}</span>{/if}
              {#if candidate.message_range}<span>{candidate.message_range}</span>{/if}
              {#if candidate.updated_at}<span>{fmtDate(candidate.updated_at)}</span>{/if}
            </div>
            {#if candidate.evidence?.length}
              <details class="evidence">
                <summary>{$t.skillInbox.candidate.evidence}</summary>
                <div class="evidence-list">
                  {#each candidate.evidence as evidence}
                    <div class="evidence-row">
                      <span>{evidence.role}</span>
                      <p>{evidence.snippet}</p>
                    </div>
                  {/each}
                </div>
              </details>
            {/if}
            {#if capability}
              <details class="evaluation" open={capability.state === 'shadow' || capability.state === 'canary'}>
                <summary>{$t.skillInbox.review.summary}</summary>
                <div class="evaluation-grid">
                  <div>
                    <span>{$t.skillInbox.review.provenance}</span>
                    <strong>{candidate.provenance?.source ?? $t.skillInbox.review.defaultSource} · {candidate.source_session ?? $t.skillInbox.review.unknownSession}</strong>
                  </div>
                  <div>
                    <span>{$t.skillInbox.review.rollout}</span>
                    <strong>{capability.rollout?.mode ?? $t.skillInbox.review.none} · {capability.rollout?.percent ?? 0}%</strong>
                  </div>
                  <div>
                    <span>{$t.skillInbox.review.rollbackTarget}</span>
                    <strong>{capability.rollback_target_id ?? $t.skillInbox.review.removeFirstVersion}</strong>
                  </div>
                  <div>
                    <span>{$t.skillInbox.review.permissionExpansion}</span>
                    <strong>{latest?.report?.permission_expansion?.join(', ') || $t.skillInbox.review.none}</strong>
                  </div>
                </div>
                {#if latest?.report?.content_diff}
                  <pre class="capability-diff">{latest.report.content_diff}</pre>
                {/if}
                {#if capabilityEvaluations.length > 0}
                  <div class="evaluation-list" aria-label={$t.skillInbox.review.evaluationsAriaLabel}>
                    {#each capabilityEvaluations as evaluation}
                      <div class="evaluation-row">
                        <span class="badge {evaluation.status === 'passed' ? 'badge-success' : evaluation.status === 'failed' ? 'badge-error' : 'badge-default'}">{evaluation.stage}: {evaluation.status}</span>
                        <span>{evaluation.metrics?.latency_ms ?? 0} ms</span>
                        {#if evaluation.proof_id}<span>{$t.skillInbox.review.proof(evaluation.proof_id)}</span>{/if}
                      </div>
                    {/each}
                  </div>
                {/if}
                <div class="evaluation-delta">
                  <span>{$t.skillInbox.review.evaluationDelta}</span>
                  <span>{$t.skillInbox.review.delta.success} {formatDelta(latest?.delta?.success_rate)}</span>
                  <span>{$t.skillInbox.review.delta.verification} {formatDelta(latest?.delta?.verification_rate)}</span>
                  <span>{$t.skillInbox.review.delta.cost} {formatDelta(latest?.delta?.cost_usd, ' USD')}</span>
                  <span>{$t.skillInbox.review.delta.latency} {formatDelta(latest?.delta?.latency_ms, ' ms')}</span>
                </div>
                {#if capabilityOutcomes.length > 0}
                  <div class="evaluation-list" aria-label={$t.skillInbox.review.observedOutcomes}>
                    <span>{$t.skillInbox.review.observedOutcomes}</span>
                    {#each capabilityOutcomes as outcome}
                      <div class="evaluation-row">
                        <span class="badge {outcome.status === 'succeeded' && (outcome.verifier_status === 'passed' || outcome.verifier_status === 'reported') ? 'badge-success' : outcome.status === 'failed' || outcome.verifier_status === 'failed' || outcome.verifier_status === 'stale' ? 'badge-error' : 'badge-default'}">
                          {outcome.status} · {outcome.verifier_status}
                        </span>
                        <span>{$t.skillInbox.review.work(outcome.work_id)}</span>
                        <span>{outcome.cost_usd.toFixed(3)} USD · {outcome.latency_ms} ms</span>
                        <span>{fmtDate(outcome.created_at)}</span>
                      </div>
                    {/each}
                  </div>
                {/if}
              </details>
            {/if}
            {#if candidate.draft_path}
              <div class="draft-path">{candidate.draft_path}</div>
            {/if}
            {#if capability && capability.state !== 'rejected' && capability.state !== 'rolled_back'}
              <div class="candidate-actions">
                {#if capability.state === 'candidate' || capability.state === 'draft' || capability.state === 'sandbox' || capability.state === 'offline_eval'}
                  <button class="btn btn-primary btn-sm" type="button" disabled={reviewing === candidate.id} onclick={() => review(candidate, 'evaluate')}>
                    {reviewing === candidate.id ? $t.skillInbox.actions.evaluating : $t.skillInbox.actions.evaluate}
                  </button>
                {:else if capability.state === 'shadow'}
                  <button class="btn btn-primary btn-sm" type="button" disabled={reviewing === candidate.id} onclick={() => review(candidate, 'approve')}>
                    {reviewing === candidate.id ? $t.skillInbox.actions.approving : $t.skillInbox.actions.approve}
                  </button>
                {:else if capability.state === 'canary'}
                  <button class="btn btn-primary btn-sm" type="button" disabled={reviewing === candidate.id || latest?.status !== 'passed'} onclick={() => review(candidate, 'promote')}>
                    {reviewing === candidate.id ? $t.skillInbox.actions.promoting : $t.skillInbox.actions.promote}
                  </button>
                {:else if capability.state === 'promoted'}
                  <button class="btn btn-ghost btn-sm" type="button" disabled={reviewing === candidate.id} onclick={() => review(candidate, 'rollback')}>{$t.skillInbox.actions.rollback}</button>
                {/if}
                {#if capability.state !== 'promoted'}
                  <button class="btn btn-ghost btn-sm" type="button" disabled={reviewing === candidate.id} onclick={() => review(candidate, 'reject')}>{$t.skillInbox.actions.reject}</button>
                {/if}
              </div>
            {/if}
          </article>
        {/each}
      </div>
    {/if}
  {:else}
    <div class="panel-actions session-actions">
      <button class="btn btn-ghost btn-sm" type="button" disabled={loadingLocal} onclick={loadLocal}>
        {loadingLocal ? $t.skillInbox.loading : $t.skillInbox.reload}
      </button>
      <div class="mode-toggle" role="group" aria-label={$t.skillInbox.session.modeAriaLabel}>
        <label>
          <input type="radio" name="promote-mode" value="copy" checked={mode === 'copy'} onchange={() => (mode = 'copy')} />
          {$t.skillInbox.session.modeCopy}
        </label>
        <label>
          <input type="radio" name="promote-mode" value="move" checked={mode === 'move'} onchange={() => (mode = 'move')} />
          {$t.skillInbox.session.modeMove}
        </label>
      </div>
      <div class="bulk-actions">
        <button class="btn btn-ghost btn-sm" type="button" onclick={selectAll} disabled={pendingLocal === 0}>{$t.skillInbox.session.selectAll}</button>
        <button class="btn btn-ghost btn-sm" type="button" onclick={clearSelection} disabled={selected.size === 0}>{$t.skillInbox.session.clearSelection}</button>
        <button
          class="btn btn-primary btn-sm"
          type="button"
          onclick={promoteSelected}
          disabled={promoting || selected.size === 0}
        >
          {promoting ? $t.skillInbox.session.promoting : $t.skillInbox.session.promoteSelected(selected.size)}
        </button>
      </div>
    </div>

    {#if cwd}
      <div class="cwd-strip" title={cwd}>cwd: <code>{cwd}</code></div>
    {/if}

    {#if loadingLocal && localSkills.length === 0}
      <div class="empty-state">{$t.skillInbox.session.loading}</div>
    {:else if localSkills.length === 0}
      <div class="empty-state">{$t.skillInbox.session.empty.before}<code>.tars/skills/</code>{$t.skillInbox.session.empty.after}</div>
    {:else}
      <div class="candidate-list">
        {#each localSkills as item (item.kind + ':' + item.name)}
          <article class="candidate-card session-card" class:command={item.kind === 'command'}>
            <div class="candidate-main">
              <div class="session-header">
                {#if item.kind === 'skill'}
                  <input
                    type="checkbox"
                    aria-label={$t.skillInbox.session.selectItem(item.name)}
                    checked={selected.has(item.name)}
                    onchange={() => toggleSelect(item.name)}
                  />
                {/if}
                <strong>{item.name}</strong>
                {#if item.slash}<span class="candidate-name">/{item.slash}</span>{/if}
              </div>
              <div class="badges">
                <span class="badge {item.kind === 'skill' ? 'badge-default' : 'badge-soft'}">{item.kind}</span>
                {#if item.has_workspace_collision}
                  <span class="badge badge-warn" title={$t.skillInbox.session.collisionTitle}>{$t.skillInbox.session.collision}</span>
                {/if}
              </div>
            </div>
            {#if item.description}<p>{item.description}</p>{/if}
            <div class="candidate-meta">
              <span class="draft-path">{item.file_path}</span>
            </div>
            {#if item.kind === 'skill'}
              <div class="candidate-actions">
                <button
                  class="btn btn-primary btn-sm"
                  type="button"
                  disabled={promoting}
                  onclick={() => promoteOne(item.name)}
                >
                  {$t.skillInbox.session.promote}
                </button>
              </div>
            {/if}
          </article>
        {/each}
      </div>
    {/if}
  {/if}

  {#if conflictDialogOpen}
    <div class="modal-backdrop" role="presentation" onclick={cancelConflictDialog}></div>
    <div class="modal-card" role="dialog" aria-modal="true" aria-labelledby="conflict-title">
      <h3 id="conflict-title">{$t.skillInbox.conflict.title}</h3>
      <p>
        {$t.skillInbox.conflict.body(conflictNames.length)}
      </p>
      <ul class="conflict-list">
        {#each conflictNames as name}
          <li><code>{name}</code></li>
        {/each}
      </ul>
      <div class="conflict-options" role="radiogroup" aria-label={$t.skillInbox.conflict.ariaLabel}>
        <label>
          <input type="radio" name="conflict" value="rename" checked={conflictChoice === 'rename'} onchange={() => (conflictChoice = 'rename')} />
          {$t.skillInbox.conflict.rename}
        </label>
        <label>
          <input type="radio" name="conflict" value="overwrite" checked={conflictChoice === 'overwrite'} onchange={() => (conflictChoice = 'overwrite')} />
          {$t.skillInbox.conflict.overwrite}
        </label>
        <label>
          <input type="radio" name="conflict" value="abort" checked={conflictChoice === 'abort'} onchange={() => (conflictChoice = 'abort')} />
          {$t.skillInbox.conflict.abortBatch}
        </label>
      </div>
      <p class="apply-to-all">{$t.skillInbox.conflict.appliesToAll}</p>
      <div class="modal-actions">
        <button class="btn btn-ghost btn-sm" type="button" onclick={cancelConflictDialog}>{$t.skillInbox.conflict.cancel}</button>
        <button class="btn btn-primary btn-sm" type="button" onclick={confirmConflictDialog}>
          {conflictChoice === 'abort' ? $t.skillInbox.conflict.abort : $t.skillInbox.conflict.apply}
        </button>
      </div>
    </div>
  {/if}
</div>

<style>
  .skill-extraction-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    height: 100%;
    padding: var(--space-3);
    overflow: auto;
    position: relative;
  }

  .panel-header,
  .panel-actions,
  .candidate-main,
  .candidate-actions,
  .candidate-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .panel-header {
    justify-content: space-between;
    border-bottom: 1px solid var(--border-subtle);
    padding-bottom: var(--space-2);
  }

  .panel-header > div {
    display: grid;
    gap: 2px;
  }

  .panel-header span,
  .candidate-name,
  .candidate-meta,
  .draft-path {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .tab-bar {
    display: flex;
    gap: var(--space-1);
    border-bottom: 1px solid var(--border-subtle);
  }

  .tab {
    background: transparent;
    border: 0;
    padding: var(--space-2) var(--space-3);
    color: var(--text-secondary);
    font: inherit;
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    border-bottom: 2px solid transparent;
    cursor: pointer;
  }

  .tab:hover {
    color: var(--text-primary);
  }

  .tab-active {
    color: var(--text-primary);
    border-bottom-color: var(--accent-amber);
  }

  .tab-count {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    background: var(--surface-inset);
    border-radius: 999px;
    padding: 0 var(--space-2);
    min-width: 22px;
    text-align: center;
  }

  .session-actions {
    justify-content: space-between;
  }

  .mode-toggle,
  .bulk-actions {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }

  .mode-toggle label {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .cwd-strip {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .cwd-strip code,
  .conflict-list code {
    font-family: var(--font-mono);
    color: var(--text-secondary);
  }

  .message {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
  }

  .message-error {
    border: 1px solid rgba(239, 68, 68, 0.25);
    background: rgba(239, 68, 68, 0.08);
    color: var(--error);
  }

  .message-success {
    border: 1px solid rgba(34, 197, 94, 0.25);
    background: rgba(34, 197, 94, 0.08);
    color: var(--green);
  }

  .candidate-list {
    display: grid;
    gap: var(--space-3);
  }

  .candidate-card {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-raised);
  }

  .candidate-card.approved {
    border-color: rgba(34, 197, 94, 0.28);
  }

  .candidate-card.rejected {
    opacity: 0.72;
  }

  .session-card.command {
    opacity: 0.85;
  }

  .session-header {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .badges {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
  }

  .badge-warn {
    border-color: rgba(var(--primary-rgb), 0.4);
    background: rgba(var(--primary-rgb), 0.12);
    color: var(--accent-amber, var(--primary));
  }

  .badge-soft {
    background: var(--surface-inset);
    color: var(--text-tertiary);
  }

  .candidate-main {
    justify-content: space-between;
  }

  .candidate-main > div {
    display: grid;
    gap: 2px;
    min-width: 0;
  }

  .candidate-card p {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
    line-height: 1.45;
  }

  .evidence {
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }

  .evaluation {
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }

  .evaluation-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: var(--space-2);
    margin-top: var(--space-2);
  }

  .evaluation-grid > div {
    display: grid;
    gap: 2px;
    min-width: 0;
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
  }

  .evaluation-grid span,
  .evaluation-row span:not(.badge),
  .evaluation-delta span:first-child {
    color: var(--text-tertiary);
  }

  .evaluation-grid strong {
    overflow-wrap: anywhere;
    font-family: var(--font-mono);
    font-weight: 500;
  }

  .capability-diff {
    max-height: 120px;
    margin: var(--space-2) 0 0;
    padding: var(--space-2);
    overflow: auto;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-secondary);
    font: var(--text-xs)/1.45 var(--font-mono);
    white-space: pre-wrap;
  }

  .evaluation-list {
    display: grid;
    gap: var(--space-1);
    margin-top: var(--space-2);
  }

  .evaluation-row,
  .evaluation-delta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .evaluation-delta {
    margin-top: var(--space-2);
    padding-top: var(--space-2);
    border-top: 1px solid var(--border-subtle);
    font-family: var(--font-mono);
  }

  .evidence-list {
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }

  .evidence-row {
    display: grid;
    gap: 2px;
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
  }

  .evidence-row span {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
  }

  .draft-path {
    font-family: var(--font-mono);
    overflow-wrap: anywhere;
  }

  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    z-index: 50;
  }

  .modal-card {
    position: fixed;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    padding: var(--space-4);
    width: min(420px, 90vw);
    z-index: 51;
    display: grid;
    gap: var(--space-3);
  }

  .modal-card h3 {
    margin: 0;
    font-size: var(--text-md);
  }

  .modal-card p {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .conflict-list {
    margin: 0;
    padding: 0 0 0 var(--space-4);
    color: var(--text-secondary);
    font-size: var(--text-xs);
    max-height: 120px;
    overflow: auto;
  }

  .conflict-options {
    display: grid;
    gap: var(--space-1);
  }

  .conflict-options label {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .apply-to-all {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .modal-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
</style>
