import type { ChatCommandsTranslations } from '../i18n/sections/chatCommands'

export type WorkbenchActionID = 'tasks' | 'evidence' | 'agentruntime' | 'git'
export type WorkbenchPanelID = 'tasks' | 'git'
export type WorkbenchActionTab = 'evidence'

// Labels and titles for the jumps. Callers pass `$t.chatCommands.workbench`.
export type WorkbenchActionText = ChatCommandsTranslations['workbench']

export interface WorkbenchAction {
  id: WorkbenchActionID
  label: string
  title: string
  panel?: WorkbenchPanelID
  tab?: WorkbenchActionTab
  href?: string
}

export interface WorkbenchActionInput {
  sessionId?: string | null
  hasPlan: boolean
  activeTaskTitle?: string
}

export function buildWorkbenchActions(input: WorkbenchActionInput, text: WorkbenchActionText): WorkbenchAction[] {
  if (!input.sessionId || !input.hasPlan) return []

  const activeTask = input.activeTaskTitle?.trim() ?? ''

  return [
    {
      id: 'tasks',
      label: text.tasks,
      title: text.tasksTitle(activeTask),
      panel: 'tasks',
    },
    {
      id: 'evidence',
      label: text.evidence,
      title: text.evidenceTitle(activeTask),
      panel: 'tasks',
      tab: 'evidence',
    },
    {
      id: 'agentruntime',
      label: text.agentRuntime,
      title: text.agentRuntimeTitle,
      href: '/console/agentruntime',
    },
    {
      id: 'git',
      label: text.git,
      title: text.gitTitle,
      panel: 'git',
    },
  ]
}
