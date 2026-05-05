import type { GlobalPlanItem, SessionTask } from './types'

export type PlanTaskStatusFilter = 'in_progress' | 'pending' | 'completed'
export type PlanSummaryFilter = 'all' | PlanTaskStatusFilter

type PlanStatusSource = {
  summary?: Record<string, number> | null
  tasks?: SessionTask[] | null
}

function countTasksByStatus(tasks: SessionTask[] | null | undefined, status: PlanTaskStatusFilter): number {
  return (tasks ?? []).reduce((count, task) => count + (task.status === status ? 1 : 0), 0)
}

export function planStatusCount(item: PlanStatusSource, status: PlanTaskStatusFilter): number {
  const summaryValue = item.summary?.[status]
  if (typeof summaryValue === 'number' && Number.isFinite(summaryValue)) {
    return Math.max(0, summaryValue)
  }
  return countTasksByStatus(item.tasks, status)
}

export function aggregatePlanStatusCount(items: readonly PlanStatusSource[], status: PlanTaskStatusFilter): number {
  return items.reduce((total, item) => total + planStatusCount(item, status), 0)
}

export function filterPlansBySummaryCard<T extends GlobalPlanItem>(items: readonly T[], filter: PlanSummaryFilter): T[] {
  if (filter === 'all') return [...items]
  return items.filter((item) => planStatusCount(item, filter) > 0)
}
