<script lang="ts">
  import { onMount } from 'svelte'
  import { getGlobalPlans } from '../lib/api'
  import type { GlobalPlanItem } from '../lib/types'
  import { t } from '../i18n'

  interface Props {
    onNavigate: (path: string) => void
  }

  let { onNavigate }: Props = $props()
  let plans: GlobalPlanItem[] = $state([])
  let loading = $state(true)
  let error = $state('')

  async function load() {
    loading = true
    error = ''
    try {
      plans = (await getGlobalPlans(true)).items
    } catch (err) {
      error = err instanceof Error ? err.message : $t.plans.failedLoad
    } finally {
      loading = false
    }
  }

  function progressPercent(item: GlobalPlanItem): number {
    const total = Math.max(0, item.summary?.total ?? item.tasks.length)
    if (total === 0) return 0
    const finished = (item.summary?.completed ?? 0) + (item.summary?.cancelled ?? 0)
    return Math.max(0, Math.min(100, Math.round((finished / total) * 100)))
  }

  function finishedCount(item: GlobalPlanItem): number {
    return (item.summary?.completed ?? 0) + (item.summary?.cancelled ?? 0)
  }

  function sessionKind(item: GlobalPlanItem): string {
    if (item.session.kind?.trim()) return item.session.kind
    return item.session.hidden ? $t.plans.sessionKindWorker : $t.plans.sessionKindSession
  }

  function formatTime(value?: string): string {
    if (!value) return $t.plans.timeNever
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    return new Intl.DateTimeFormat(undefined, {
      month: 'short',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    }).format(date)
  }

  function openSession(sessionId: string) {
    onNavigate(`/console/chat?session=${encodeURIComponent(sessionId)}`)
  }

  onMount(() => {
    void load()
  })
</script>

<div class="plans-page">
  <section class="plans-header">
    <div>
      <span class="plans-kicker">{$t.plans.kicker}</span>
      <h2>{$t.plans.title}</h2>
    </div>
    <button class="btn btn-ghost btn-sm" type="button" onclick={load} disabled={loading}>{$t.plans.refresh}</button>
  </section>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  {#if loading && plans.length === 0}
    <div class="empty-state">{$t.plans.loadingPlans}</div>
  {:else if plans.length === 0}
    <section class="empty-state plans-empty">
      <strong>{$t.plans.emptyTitle}</strong>
      <p>{$t.plans.emptyBody}</p>
      <button class="btn btn-primary btn-sm" type="button" onclick={() => onNavigate('/console/chat')}>{$t.plans.openChat}</button>
    </section>
  {:else}
    <section class="plans-summary" aria-label={$t.plans.summaryAriaLabel}>
      <div class="summary-card card">
        <span>{$t.plans.activePlans}</span>
        <strong>{plans.length}</strong>
      </div>
      <div class="summary-card card">
        <span>{$t.plans.inProgress}</span>
        <strong>{plans.reduce((total, item) => total + (item.summary?.in_progress ?? 0), 0)}</strong>
      </div>
      <div class="summary-card card">
        <span>{$t.plans.pending}</span>
        <strong>{plans.reduce((total, item) => total + (item.summary?.pending ?? 0), 0)}</strong>
      </div>
      <div class="summary-card card">
        <span>{$t.plans.completed}</span>
        <strong>{plans.reduce((total, item) => total + (item.summary?.completed ?? 0), 0)}</strong>
      </div>
    </section>

    <section class="plan-grid" aria-label={$t.plans.plansAriaLabel}>
      {#each plans as item (item.session.id)}
        {@const percent = progressPercent(item)}
        <button class="plan-card card" type="button" onclick={() => openSession(item.session.id)}>
          <span class="card-topline">
            <span class="session-title">{item.session.title}</span>
            <span class="badge badge-default">{sessionKind(item)}</span>
          </span>
          <strong class="plan-goal">{item.plan.goal}</strong>
          <span class="plan-meta">
            <span>{item.plan.status ?? $t.plans.statusFallback}</span>
            <span>{$t.plans.updatedAt(formatTime(item.updated_at))}</span>
          </span>
          <span class="progress-track" aria-label={$t.plans.progressAria(percent)}>
            <span class="progress-fill" style={`width: ${percent}%`}></span>
          </span>
          <span class="plan-stats">
            <span>{$t.plans.doneSuffix(finishedCount(item), item.summary?.total ?? item.tasks.length)}</span>
            <span>{$t.plans.activeSuffix(item.summary?.in_progress ?? 0)}</span>
            <span>{$t.plans.pendingSuffix(item.summary?.pending ?? 0)}</span>
          </span>
        </button>
      {/each}
    </section>
  {/if}
</div>

<style>
  .plans-page {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .plans-header {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .plans-kicker {
    color: var(--text-tertiary);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0;
  }

  .plans-header h2 {
    margin: var(--space-1) 0 0;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-3xl);
    line-height: 1.1;
  }

  .plans-summary {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: var(--space-3);
  }

  .summary-card {
    display: flex;
    min-height: 88px;
    flex-direction: column;
    justify-content: center;
    gap: var(--space-1);
  }

  .summary-card span {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .summary-card strong {
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-2xl);
    line-height: 1;
  }

  .plan-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
    gap: var(--space-3);
  }

  .plan-card {
    display: flex;
    min-height: 220px;
    flex-direction: column;
    justify-content: space-between;
    gap: var(--space-3);
    width: 100%;
    border: 1px solid var(--border-subtle);
    background: var(--surface-card);
    color: var(--text-primary);
    cursor: pointer;
    text-align: left;
    transition:
      border-color var(--duration-fast) var(--ease-out),
      transform var(--duration-fast) var(--ease-out);
  }

  .plan-card:hover {
    border-color: var(--primary);
    transform: translateY(-1px);
  }

  .card-topline,
  .plan-meta,
  .plan-stats {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .session-title {
    min-width: 0;
    overflow: hidden;
    color: var(--text-secondary);
    font-size: var(--text-sm);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .plan-goal {
    display: -webkit-box;
    overflow: hidden;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-lg);
    font-weight: 600;
    line-height: 1.25;
    line-clamp: 3;
    -webkit-line-clamp: 3;
    -webkit-box-orient: vertical;
  }

  .plan-meta,
  .plan-stats {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .plan-stats {
    flex-wrap: wrap;
    justify-content: flex-start;
  }

  .progress-track {
    display: block;
    width: 100%;
    height: 8px;
    overflow: hidden;
    border-radius: 999px;
    background: var(--surface-inset);
  }

  .progress-fill {
    display: block;
    height: 100%;
    border-radius: inherit;
    background: var(--primary);
  }

  .plans-empty {
    display: flex;
    min-height: 220px;
    flex-direction: column;
    align-items: flex-start;
    justify-content: center;
    gap: var(--space-2);
  }

  .plans-empty strong {
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-lg);
  }

  .plans-empty p {
    margin: 0;
    color: var(--text-secondary);
  }

  @media (max-width: 720px) {
    .plans-header {
      align-items: flex-start;
      flex-direction: column;
    }

    .plan-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
