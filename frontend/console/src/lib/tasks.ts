import type { SessionTask } from './types'

export type TaskProgressSummary = {
  total: number
  pending: number
  in_progress: number
  completed: number
  cancelled: number
  /** Title of the first in_progress task, if any — surfaced in the chat plan strip
   * so the user can see at a glance which task the session is actively working on. */
  active_task_title?: string
}

export function emptyTaskProgressSummary(): TaskProgressSummary {
  return {
    total: 0,
    pending: 0,
    in_progress: 0,
    completed: 0,
    cancelled: 0,
  }
}

export function summarizeTasks(tasks: SessionTask[] | null | undefined): TaskProgressSummary {
  const summary = emptyTaskProgressSummary()
  for (const task of tasks ?? []) {
    summary.total++
    switch (task.status) {
      case 'completed':
        summary.completed++
        break
      case 'in_progress':
        summary.in_progress++
        if (!summary.active_task_title) {
          const title = task.title?.trim()
          if (title) summary.active_task_title = title
        }
        break
      case 'cancelled':
        summary.cancelled++
        break
      case 'pending':
      default:
        summary.pending++
        break
    }
  }
  return summary
}

export function planProgressPercent(summary: Pick<TaskProgressSummary, 'total' | 'completed'>): number {
  if (summary.total <= 0) return 0
  return Math.max(0, Math.min(100, Math.round((summary.completed / summary.total) * 100)))
}
