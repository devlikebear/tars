type ToolCallState = {
  toolDone?: boolean
  toolIsError?: boolean
}

export function toolCallTone(state: ToolCallState): 'running' | 'done' | 'error' {
  if (state.toolIsError) return 'error'
  if (state.toolDone) return 'done'
  return 'running'
}

export function formatElapsedSeconds(startMs?: number, endMs?: number, nowMs: number = Date.now()): string {
  if (!Number.isFinite(startMs) || !startMs) return ''
  const end = Number.isFinite(endMs) && endMs ? endMs : nowMs
  const seconds = Math.max(0, (end - startMs) / 1000)
  return `${seconds.toFixed(1)}s`
}

export function formatToolJSON(raw?: string): string {
  const trimmed = raw?.trim()
  if (!trimmed) return ''
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2)
  } catch {
    return trimmed
  }
}

export function formatToolInvocationPreview(toolName?: string, rawArgs?: string): string {
  const name = toolName?.trim() || 'tool'
  const entries = previewArgEntries(rawArgs)
  if (entries.length === 0) return `${name}()`
  return `${name}(${entries.map(([key, value]) => `${key}=${formatArgValue(value)}`).join(', ')})`
}

function previewArgEntries(rawArgs?: string): Array<[string, unknown]> {
  const trimmed = rawArgs?.trim()
  if (!trimmed) return []
  try {
    const parsed = JSON.parse(trimmed) as unknown
    if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
      return [['value', parsed]]
    }
    const record = parsed as Record<string, unknown>
    const priority = ['path', 'file', 'command', 'action', 'title', 'query', 'pattern', 'id', 'name']
    const ordered = [
      ...priority.filter((key) => Object.prototype.hasOwnProperty.call(record, key)),
      ...Object.keys(record).filter((key) => !priority.includes(key)),
    ]
    return ordered.slice(0, 2).map((key) => [key, record[key]])
  } catch {
    return [['args', truncateOneLine(trimmed, 64)]]
  }
}

function formatArgValue(value: unknown): string {
  if (typeof value === 'string') {
    if (value.includes('/') && !value.includes('"')) return truncateOneLine(value, 48)
    return JSON.stringify(truncateOneLine(value, 48))
  }
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (value == null) return 'null'
  return truncateOneLine(JSON.stringify(value), 48)
}

function truncateOneLine(value: string, max: number): string {
  const normalized = value.replace(/\s+/g, ' ').trim()
  if (normalized.length <= max) return normalized
  return `${normalized.slice(0, Math.max(0, max - 3))}...`
}
