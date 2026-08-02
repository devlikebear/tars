<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { getWorkerControlPlane } from '../lib/api'
  import type { RemoteWorkerCapabilities, WorkerControlPlaneResponse } from '../lib/types'
  import {
    controlEventPresentation,
    placementStateBadge,
    workerStateBadge,
  } from '../lib/workerControlPlane'
  import { t } from '../i18n'

  const pollIntervalMS = 10_000
  let snapshot = $state<WorkerControlPlaneResponse | null>(null)
  let loading = $state(true)
  let refreshing = $state(false)
  let error = $state('')
  let pollTimer: ReturnType<typeof setInterval> | null = null

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function fmtBytes(bytes?: number): string {
    const value = Math.max(0, bytes ?? 0)
    if (value < 1024) return `${value} B`
    if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
    if (value < 1024 * 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MB`
    return `${(value / (1024 * 1024 * 1024)).toFixed(1)} GB`
  }

  function compact(value?: string, max = 28): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    return text.length <= max ? text : `${text.slice(0, max - 1)}\u2026`
  }

  function capabilityLabels(capabilities: RemoteWorkerCapabilities): string[] {
    return Object.entries(capabilities)
      .filter(([, enabled]) => enabled)
      .map(([name]) => name.replaceAll('_', ' '))
  }

  async function load(silent = false) {
    if (!silent) refreshing = true
    error = ''
    try {
      snapshot = await getWorkerControlPlane()
    } catch (err) {
      error = err instanceof Error ? err.message : $t.ops.workers.loadError
    } finally {
      loading = false
      refreshing = false
    }
  }

  onMount(() => {
    void load(true)
    pollTimer = setInterval(() => { void load(true) }, pollIntervalMS)
  })

  onDestroy(() => {
    if (pollTimer != null) clearInterval(pollTimer)
  })
</script>

<section class="card remote-workers-section" aria-label={$t.ops.workers.ariaLabel}>
  <div class="card-header remote-workers-header">
    <div>
      <span class="card-title">{$t.ops.workers.title}</span>
      <p>{$t.ops.workers.subtitle}</p>
    </div>
    <div class="header-actions">
      {#if snapshot}
        <span class="badge {snapshot.enabled ? 'badge-success' : 'badge-default'}">
          {snapshot.enabled ? $t.ops.workers.enabled : $t.ops.workers.disabled}
        </span>
        <span class="badge badge-default">{$t.ops.workers.protocol} {snapshot.protocol_version}</span>
        <span class="badge {snapshot.a2a.enabled ? 'badge-info' : 'badge-default'}">
          {$t.ops.workers.a2a} {snapshot.a2a.enabled ? $t.ops.workers.enabled : $t.ops.workers.disabled}
        </span>
      {/if}
      <button type="button" class="btn btn-ghost btn-sm" disabled={refreshing} onclick={() => { void load() }}>
        {refreshing ? $t.ops.workers.refreshing : $t.ops.workers.refresh}
      </button>
    </div>
  </div>

  {#if error}
    <div class="error-banner">{$t.ops.workers.loadError}: {error}</div>
  {/if}

  {#if loading}
    <div class="worker-empty">{$t.ops.workers.loading}</div>
  {:else if snapshot && !snapshot.enabled}
    <div class="worker-empty worker-disabled">
      <strong>{$t.ops.workers.disabledTitle}</strong>
      <span>{$t.ops.workers.disabledBody}</span>
    </div>
  {:else if snapshot}
    <div class="summary-grid" aria-label="Remote execution summary">
      <div><strong>{snapshot.summary.workers}</strong><span>{$t.ops.workers.summary.workers}</span></div>
      <div><strong>{snapshot.summary.ready_workers}</strong><span>{$t.ops.workers.summary.ready}</span></div>
      <div><strong>{snapshot.summary.lost_workers}</strong><span>{$t.ops.workers.summary.lost}</span></div>
      <div><strong>{snapshot.summary.placements}</strong><span>{$t.ops.workers.summary.placements}</span></div>
      <div><strong>{snapshot.summary.active_placements}</strong><span>{$t.ops.workers.summary.active}</span></div>
      <div><strong>{snapshot.summary.recovering_placements}</strong><span>{$t.ops.workers.summary.recovering}</span></div>
      <div><strong>{snapshot.summary.recovery_count}</strong><span>{$t.ops.workers.summary.recoveries}</span></div>
    </div>

    <div class="control-grid">
      <div class="control-group">
        <div class="group-heading">
          <h3>{$t.ops.workers.workersTitle}</h3>
          <span class="badge badge-default">{snapshot.workers.length}</span>
        </div>
        {#if snapshot.workers.length === 0}
          <div class="inline-empty">{$t.ops.workers.noWorkers}</div>
        {:else}
          <div class="worker-list">
            {#each snapshot.workers as worker (worker.id)}
              <article class="worker-card">
                <div class="entity-heading">
                  <strong class="mono" title={worker.id}>{compact(worker.id, 36)}</strong>
                  <span class="badge {workerStateBadge(worker.state)}">{worker.state}</span>
                  <span class="badge badge-default">{worker.transport}</span>
                </div>
                <dl class="detail-grid">
                  <div><dt>{$t.ops.workers.lastSeen}</dt><dd>{fmt(worker.last_seen_at)}</dd></div>
                  <div><dt>{$t.ops.workers.lease}</dt><dd>{fmt(worker.lease_expires_at)}</dd></div>
                </dl>
                <div class="capability-row">
                  <span class="detail-label">{$t.ops.workers.capabilities}</span>
                  {#each capabilityLabels(worker.capabilities) as capability}
                    <span class="badge badge-default">{capability}</span>
                  {/each}
                </div>
              </article>
            {/each}
          </div>
        {/if}
      </div>

      <div class="control-group placements-group">
        <div class="group-heading">
          <h3>{$t.ops.workers.placementsTitle}</h3>
          <span class="badge badge-default">{snapshot.placements.length}</span>
        </div>
        {#if snapshot.placements.length === 0}
          <div class="inline-empty">{$t.ops.workers.noPlacements}</div>
        {:else}
          <div class="placement-list">
            {#each snapshot.placements as placement (placement.id)}
              <article class="placement-card">
                <div class="entity-heading">
                  <strong class="mono" title={placement.id}>{compact(placement.id, 40)}</strong>
                  <span class="badge {placementStateBadge(placement.state)}">{placement.state}</span>
                  <span class="badge badge-default">{placement.worker_id}</span>
                </div>
                <dl class="placement-identities">
                  <div><dt>{$t.ops.workers.work}</dt><dd class="mono" title={placement.work_id}>{compact(placement.work_id)}</dd></div>
                  <div><dt>{$t.ops.workers.step}</dt><dd class="mono" title={placement.step_id}>{compact(placement.step_id)}</dd></div>
                  <div><dt>{$t.ops.workers.attempt}</dt><dd class="mono" title={placement.attempt_id}>{compact(placement.attempt_id)}</dd></div>
                </dl>
                <div class="placement-facts">
                  <div>
                    <span class="detail-label">{$t.ops.workers.sync}</span>
                    <strong>{placement.sync.mode}</strong>
                    <span>{placement.sync.file_count ?? 0} files · {fmtBytes(placement.sync.total_bytes)}</span>
                    {#if placement.sync.manifest_digest}<code title={placement.sync.manifest_digest}>{compact(placement.sync.manifest_digest)}</code>{/if}
                  </div>
                  <div>
                    <span class="detail-label">{$t.ops.workers.checkpoint}</span>
                    {#if placement.checkpoint}
                      <strong class="mono">{compact(placement.checkpoint.id)}</strong>
                      <code title={placement.checkpoint.digest}>{compact(placement.checkpoint.digest)}</code>
                    {:else}
                      <strong>\u2014</strong>
                    {/if}
                  </div>
                  <div>
                    <span class="detail-label">{$t.ops.workers.recovery}</span>
                    <strong>{placement.recovery_count}</strong>
                    <span>{$t.ops.workers.updated} {fmt(placement.updated_at)}</span>
                  </div>
                  <div>
                    <span class="detail-label">{$t.ops.workers.egress}</span>
                    <strong>{placement.policy.egress.mode}</strong>
                    <span>{placement.policy.egress.allow_hosts?.join(', ') || '\u2014'}</span>
                  </div>
                  <div>
                    <span class="detail-label">{$t.ops.workers.resources}</span>
                    <strong>{placement.policy.limits.cpu_seconds}s CPU · {placement.policy.limits.memory_mb} MB</strong>
                    <span>{placement.policy.limits.disk_mb} MB disk · {fmtBytes(placement.policy.limits.max_output_bytes)} output</span>
                  </div>
                </div>
              </article>
            {/each}
          </div>
        {/if}
      </div>
    </div>

    <div class="events-group">
      <div class="group-heading">
        <h3>{$t.ops.workers.eventsTitle}</h3>
        <span class="badge badge-default">{snapshot.events.length}</span>
      </div>
      {#if snapshot.events.length === 0}
        <div class="inline-empty">{$t.ops.workers.noEvents}</div>
      {:else}
        <div class="event-list">
          {#each snapshot.events.slice(0, 24) as event (event.id)}
            {@const view = controlEventPresentation(event)}
            <div class="event-row">
              <span class="event-marker" class:event-pending={!event.published}></span>
              <div>
                <strong>{view.title}</strong>
                <span>{view.detail}</span>
              </div>
              <span class="badge {event.published ? 'badge-default' : 'badge-warning'}">
                {event.published ? $t.ops.workers.published : $t.ops.workers.pendingPublish}
              </span>
              <time datetime={event.occurred_at}>{fmt(event.occurred_at)}</time>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</section>

<style>
  .remote-workers-section {
    margin-bottom: var(--space-4);
  }

  .remote-workers-header {
    align-items: flex-start;
    gap: var(--space-4);
  }

  .remote-workers-header p {
    max-width: 700px;
    margin: var(--space-1) 0 0;
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }

  .header-actions,
  .entity-heading,
  .group-heading,
  .capability-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .header-actions {
    justify-content: flex-end;
  }

  .worker-empty,
  .inline-empty {
    color: var(--text-tertiary);
    font-size: var(--text-sm);
    line-height: 1.55;
  }

  .worker-empty {
    display: grid;
    gap: var(--space-1);
    padding: var(--space-8) var(--space-4);
    text-align: center;
  }

  .worker-empty strong {
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-lg);
    font-weight: 500;
  }

  .worker-disabled {
    margin-top: var(--space-3);
    border-top: 1px solid var(--border-subtle);
  }

  .summary-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(90px, 1fr));
    gap: var(--space-2);
    margin-top: var(--space-4);
  }

  .summary-grid > div {
    display: grid;
    gap: var(--space-1);
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
  }

  .summary-grid strong {
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-xl);
    font-weight: 500;
  }

  .summary-grid span,
  .detail-label,
  dt {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .control-grid {
    display: grid;
    grid-template-columns: minmax(260px, 0.8fr) minmax(0, 1.6fr);
    gap: var(--space-4);
    margin-top: var(--space-5);
  }

  .control-group,
  .events-group {
    min-width: 0;
  }

  .events-group {
    margin-top: var(--space-5);
    padding-top: var(--space-4);
    border-top: 1px solid var(--border-subtle);
  }

  .group-heading {
    justify-content: space-between;
    margin-bottom: var(--space-3);
  }

  .group-heading h3 {
    margin: 0;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-base);
    font-weight: 500;
  }

  .worker-list,
  .placement-list,
  .event-list {
    display: grid;
    gap: var(--space-2);
  }

  .worker-card,
  .placement-card,
  .event-row {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
  }

  .worker-card,
  .placement-card {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-3);
  }

  .entity-heading strong {
    min-width: 0;
    overflow: hidden;
    color: var(--text-primary);
    font-size: var(--text-xs);
    text-overflow: ellipsis;
  }

  .detail-grid,
  .placement-identities {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
    gap: var(--space-2);
    margin: 0;
  }

  .detail-grid div,
  .placement-identities div {
    min-width: 0;
  }

  dt,
  dd {
    margin: 0;
  }

  dd {
    overflow: hidden;
    color: var(--text-secondary);
    font-size: var(--text-xs);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .capability-row .detail-label {
    width: 100%;
  }

  .placement-facts {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: var(--space-2);
  }

  .placement-facts > div {
    display: grid;
    min-width: 0;
    gap: 2px;
    padding: var(--space-2);
    border-radius: var(--radius-sm);
    background: var(--surface);
  }

  .placement-facts strong,
  .placement-facts span:not(.detail-label),
  .placement-facts code {
    overflow: hidden;
    color: var(--text-secondary);
    font-size: var(--text-xs);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .event-row {
    display: grid;
    grid-template-columns: 8px minmax(0, 1fr) auto auto;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
  }

  .event-marker {
    width: 6px;
    height: 6px;
    border-radius: var(--radius-sm);
    background: var(--success);
  }

  .event-marker.event-pending {
    background: var(--warning);
  }

  .event-row > div {
    display: grid;
    min-width: 0;
  }

  .event-row strong,
  .event-row span,
  .event-row time {
    overflow: hidden;
    font-size: var(--text-xs);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .event-row strong {
    color: var(--text-primary);
    font-weight: 500;
  }

  .event-row div span,
  .event-row time {
    color: var(--text-tertiary);
  }

  @media (max-width: 900px) {
    .control-grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }

  @media (max-width: 760px) {
    .remote-workers-header,
    .header-actions {
      align-items: flex-start;
      flex-direction: column;
    }

    .event-row {
      grid-template-columns: 8px minmax(0, 1fr);
    }

    .event-row > .badge,
    .event-row time {
      grid-column: 2;
      justify-self: start;
    }
  }
</style>
