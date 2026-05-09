export type WorkbenchActionID = 'tasks' | 'evidence' | 'agentruntime' | 'git'
export type WorkbenchPanelID = 'tasks' | 'git'
export type WorkbenchActionTab = 'evidence'

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

export function buildWorkbenchActions(input: WorkbenchActionInput): WorkbenchAction[] {
  if (!input.sessionId || !input.hasPlan) return []

  const activeSuffix = input.activeTaskTitle?.trim()
    ? ` for ${input.activeTaskTitle.trim()}`
    : ''

  return [
    {
      id: 'tasks',
      label: 'Tasks',
      title: `Open active plan tasks${activeSuffix}`,
      panel: 'tasks',
    },
    {
      id: 'evidence',
      label: 'Evidence',
      title: `Open plan evidence${activeSuffix}`,
      panel: 'tasks',
      tab: 'evidence',
    },
    {
      id: 'agentruntime',
      label: 'Agent Runtime',
      title: 'Open Agent Runtime runs',
      href: '/console/agentruntime',
    },
    {
      id: 'git',
      label: 'Git',
      title: 'Open Git Inspector',
      panel: 'git',
    },
  ]
}
