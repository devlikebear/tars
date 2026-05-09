import type { GlobalPlanItem, SessionTask } from './types'

export type PlanTaskStatusFilter = 'in_progress' | 'pending' | 'completed'
export type PlanSummaryFilter = 'all' | 'ready_to_close' | PlanTaskStatusFilter
type PlanTaskStatus = PlanTaskStatusFilter | 'cancelled'

type PlanStatusSource = {
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

export function filterPlansBySummaryCard<T extends GlobalPlanItem>(items: readonly T[], filter: PlanSummaryFilter): T[] {
  if (filter === 'all') return [...items]
  if (filter === 'ready_to_close') return items.filter(isStaleCompletedPlan)
  return items.filter((item) => planStatusCount(item, filter) > 0)
}
