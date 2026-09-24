import { get } from 'svelte/store'
import * as api from '../api'
import { t } from '../../i18n'
import { buildSessionHealthReport, emptySessionHealthReport } from '../sessionHealth'
import { emptyTaskProgressSummary, summarizeTasks } from '../tasks'
import { ChatSessionStore } from './chatSessionStore.svelte'

export const chatSession = new ChatSessionStore(api, {
  emptyReport: () => emptySessionHealthReport(get(t).sessionHealth),
  buildReport: (input) => buildSessionHealthReport(get(t).sessionHealth, input),
  emptyTasks: emptyTaskProgressSummary,
  summarizeTasks,
  goalEventText: () => get(t).chatCommands.goalEvents,
})
