export type SubagentProgressStatus = 'pending' | 'running' | 'completed' | 'failed'

export type SubagentProgressTask = {
  title: string
  status: SubagentProgressStatus
  runId?: string
  sessionId?: string
  href?: string
  agent?: string
  tier?: string
  summary?: string
  error?: string
}

export type SubagentProgress = {
  agent?: string
  mode?: string
  count: number
  complete: boolean
  completed: number
  failed: number
  running: number
  pending: number
  tasks: SubagentProgressTask[]
}

export type SubagentProgressInput = {
  toolName?: string
  toolArgs?: string
  toolResult?: string
  toolDone?: boolean
  toolIsError?: boolean
}

type JSONRecord = Record<string, unknown>

export function buildSubagentProgress(input: SubagentProgressInput): SubagentProgress | null {
  if (input.toolName?.trim() !== 'subagents_run') return null

  const args = parseRecord(input.toolArgs)
  const result = parseRecord(input.toolResult)
  const complete = input.toolDone === true
  const resultTasks = result ? subagentResultTasks(result) : []
  const argTasks = args ? subagentArgTasks(args, complete, input.toolIsError === true) : []

  const tasks = resultTasks.length > 0 ? resultTasks : argTasks
  if (tasks.length === 0) return null

  const count = numericField(result?.count) || numericField(args?.count) || tasks.length
  return {
    agent: stringField(result?.agent) || stringField(args?.agent),
    mode: stringField(args?.mode) || 'parallel',
    count,
    complete,
    completed: tasks.filter((task) => task.status === 'completed').length,
    failed: tasks.filter((task) => task.status === 'failed').length,
    running: tasks.filter((task) => task.status === 'running').length,
    pending: tasks.filter((task) => task.status === 'pending').length,
    tasks,
  }
}

export function agentRuntimeRunHref(runId: string): string {
  return `/console/agentruntime/runs/${encodeURIComponent(runId)}`
}

export function shortRunID(runId?: string): string {
  const value = runId?.trim() ?? ''
  if (value.length <= 13) return value
  return `${value.slice(0, 10)}...`
}

function parseRecord(raw?: string): JSONRecord | null {
  const trimmed = raw?.trim()
  if (!trimmed) return null
  try {
    const parsed = JSON.parse(trimmed) as unknown
    if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') return null
    return parsed as JSONRecord
  } catch {
    return null
  }
}

function subagentArgTasks(args: JSONRecord, complete: boolean, errored: boolean): SubagentProgressTask[] {
  const tasks = arrayField(args.tasks)
  const fallbackStatus: SubagentProgressStatus = complete ? (errored ? 'failed' : 'completed') : 'running'
  const out: SubagentProgressTask[] = []
  tasks.forEach((item, index) => {
    const record = objectField(item)
    if (!record) return
    out.push({
      title: stringField(record.title) || `Subagent ${index + 1}`,
      status: fallbackStatus,
      tier: stringField(record.tier),
    })
  })
  return out
}

function subagentResultTasks(result: JSONRecord): SubagentProgressTask[] {
  const out: SubagentProgressTask[] = []
  arrayField(result.subagents).forEach((item, index) => {
    const record = objectField(item)
    if (!record) return
    const runId = stringField(record.run_id)
    out.push({
      title: stringField(record.title) || `Subagent ${index + 1}`,
      status: normalizeStatus(stringField(record.status)),
      runId,
      sessionId: stringField(record.session_id),
      href: runId ? agentRuntimeRunHref(runId) : undefined,
      agent: stringField(record.agent),
      tier: stringField(record.tier),
      summary: stringField(record.summary),
      error: stringField(record.error),
    })
  })
  return out
}

function normalizeStatus(status?: string): SubagentProgressStatus {
  switch (status?.trim().toLowerCase()) {
    case 'completed':
    case 'complete':
    case 'done':
    case 'success':
      return 'completed'
    case 'failed':
    case 'failure':
    case 'error':
    case 'cancelled':
    case 'canceled':
    case 'timed_out':
      return 'failed'
    case 'running':
    case 'in_progress':
      return 'running'
    default:
      return 'pending'
  }
}

function objectField(value: unknown): JSONRecord | null {
  if (!value || Array.isArray(value) || typeof value !== 'object') return null
  return value as JSONRecord
}

function arrayField(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

function stringField(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const trimmed = value.trim()
  return trimmed || undefined
}

function numericField(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? Math.floor(value) : 0
}
