import * as api from '../api'
import { buildSessionHealthReport, emptySessionHealthReport } from '../sessionHealth'
import { emptyTaskProgressSummary, summarizeTasks } from '../tasks'
import { ChatSessionStore } from './chatSessionStore.svelte'

export const chatSession = new ChatSessionStore(api, {
  emptyReport: () => emptySessionHealthReport(),
  buildReport: buildSessionHealthReport,
  emptyTasks: emptyTaskProgressSummary,
  summarizeTasks,
})
