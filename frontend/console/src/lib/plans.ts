import type { GlobalPlanItem, SessionTask } from './types'

export type PlanTaskStatusFilter = 'in_progress' | 'pending' | 'completed'
export type PlanSummaryFilter = 'all' | 'ready_to_close' | 'stalled' | PlanTaskStatusFilter
type PlanTaskStatus = PlanTaskStatusFilter | 'cancelled'

type PlanStatusSource = {
  updated_at?: string | null
  summary?: Record<string, number> | null
  tasks?: SessionTask[] | null
  plan?: { status?: string | null } | null
  stale_completed?: boolean | null
}

function countTasksByStatus(tasks: SessionTask[] | null | undefined, status: PlanTaskStatus): number {
  return (tasks ?? []).reduce((count, task) => count + (task.status === status ? 1 : 0), 0)
}

function statusCount(item: PlanStatusSource, status: PlanTaskStatus): number {
  const summaryValue = item.summary?.[status]
  if (typeof summaryValue === 'number' && Number.isFinite(summaryValue)) {
    return Math.max(0, summaryValue)
  }
  return countTasksByStatus(item.tasks, status)
}

function totalCount(item: PlanStatusSource): number {
  const summaryValue = item.summary?.total
  if (typeof summaryValue === 'number' && Number.isFinite(summaryValue)) {
    return Math.max(0, summaryValue)
  }
  return item.tasks?.length ?? 0
}

export function planStatusCount(item: PlanStatusSource, status: PlanTaskStatusFilter): number {
  return statusCount(item, status)
}

export function aggregatePlanStatusCount(items: readonly PlanStatusSource[], status: PlanTaskStatusFilter): number {
  return items.reduce((total, item) => total + planStatusCount(item, status), 0)
}

export function isStaleCompletedPlan(item: PlanStatusSource): boolean {
  if (item.stale_completed === true) return true
  const status = item.plan?.status?.trim().toLowerCase()
  if (status === 'completed' || status === 'aborted') return false
  const total = totalCount(item)
  if (total <= 0) return false
  if (statusCount(item, 'pending') > 0 || statusCount(item, 'in_progress') > 0) return false
  return statusCount(item, 'completed') + statusCount(item, 'cancelled') >= total
}

export function aggregateStaleCompletedPlanCount(items: readonly PlanStatusSource[]): number {
  return items.reduce((total, item) => total + (isStaleCompletedPlan(item) ? 1 : 0), 0)
}

// A plan with work left that nothing has touched for this long has outlived
// its session: the session stopped (a turn ran out, the user moved on) and
// the plan was left reading "executing".
export const STALLED_PLAN_AFTER_MS = 24 * 60 * 60 * 1000

export function isStalledPlan(item: PlanStatusSource, nowMs: number = Date.now()): boolean {
  if (isStaleCompletedPlan(item)) return false
  const status = item.plan?.status?.trim().toLowerCase()
  if (status === 'completed' || status === 'aborted') return false
  if (statusCount(item, 'pending') + statusCount(item, 'in_progress') <= 0) return false
  const updatedMs = Date.parse(item.updated_at ?? '')
  if (!Number.isFinite(updatedMs)) return false
  return nowMs - updatedMs >= STALLED_PLAN_AFTER_MS
}

export function aggregateStalledPlanCount(items: readonly PlanStatusSource[], nowMs: number = Date.now()): number {
  return items.reduce((total, item) => total + (isStalledPlan(item, nowMs) ? 1 : 0), 0)
}

export function filterPlansBySummaryCard<T extends GlobalPlanItem>(items: readonly T[], filter: PlanSummaryFilter, nowMs: number = Date.now()): T[] {
  if (filter === 'all') return [...items]
  if (filter === 'ready_to_close') return items.filter(isStaleCompletedPlan)
  if (filter === 'stalled') return items.filter((item) => isStalledPlan(item, nowMs))
  return items.filter((item) => planStatusCount(item, filter) > 0)
}
