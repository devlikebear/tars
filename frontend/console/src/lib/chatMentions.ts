export type ChatMentionKind = 'file' | 'directory' | 'subagent'

export type ChatMentionCandidate = {
  kind: ChatMentionKind
  name: string
  path: string
  root: string
  root_label: string
  token: string
  size?: number
  updated_at?: string
  description?: string
  tier?: string
  model?: string
}

export type SelectedChatMention = {
  kind: ChatMentionKind
  root: string
  path: string
  label: string
  token: string
}

export type ActiveMentionTrigger = {
  start: number
  end: number
  query: string
}

export type ChatMentionSubagentSource = {
  name: string
  description?: string
  enabled?: boolean
  default_tier?: string
  effective_tier?: string
  resolved_model?: string
  provider_override?: {
    model?: string
  }
}

export function findActiveMentionTrigger(value: string, caret: number): ActiveMentionTrigger | null {
  const safeCaret = Math.max(0, Math.min(caret, value.length))
  const beforeCaret = value.slice(0, safeCaret)
  const at = beforeCaret.lastIndexOf('@')
  if (at < 0) return null
  if (at > 0 && !/\s/.test(value[at - 1])) return null

  const token = beforeCaret.slice(at + 1)
  if (/\s/.test(token)) return null
  if (!/^[\w./-]*$/.test(token)) return null

  const afterCaret = value.slice(safeCaret)
  const next = afterCaret.match(/^\S*/)
  const tokenEnd = safeCaret + (next?.[0]?.length ?? 0)
  return { start: at, end: tokenEnd, query: token }
}

export function buildSubagentMentionCandidates(
  rawQuery: string,
  subagents: ChatMentionSubagentSource[],
  limit = 12,
): ChatMentionCandidate[] {
  const query = rawQuery.trim().replace(/^@/, '').toLowerCase()
  if (query.includes('/')) return []

  return subagents
    .filter((agent) => agent.enabled !== false)
    .map((agent) => {
      const name = agent.name.trim()
      const description = agent.description?.trim() || undefined
      const tier = agent.effective_tier?.trim() || agent.default_tier?.trim() || undefined
      const model = agent.resolved_model?.trim() || agent.provider_override?.model?.trim() || undefined
      return { name, description, tier, model }
    })
    .filter((agent) => agent.name)
    .filter((agent) => {
      if (!query) return true
      return agent.name.toLowerCase().includes(query) || (agent.description?.toLowerCase().includes(query) ?? false)
    })
    .sort((a, b) => {
      const aName = a.name.toLowerCase()
      const bName = b.name.toLowerCase()
      if (query) {
        const aPrefix = aName.startsWith(query)
        const bPrefix = bName.startsWith(query)
        if (aPrefix !== bPrefix) return aPrefix ? -1 : 1
      }
      return aName.localeCompare(bName)
    })
    .slice(0, Math.max(0, limit))
    .map((agent) => ({
      kind: 'subagent',
      name: agent.name,
      path: agent.name,
      root: 'agentruntime',
      root_label: 'subagent',
      token: `@${agent.name}`,
      description: agent.description,
      tier: agent.tier,
      model: agent.model,
    }))
}

export function applyMentionCandidate(
  value: string,
  trigger: ActiveMentionTrigger,
  candidate: ChatMentionCandidate,
): { value: string; caret: number; mention: SelectedChatMention } {
  const insertText = candidate.token || `@${candidate.path}${candidate.kind === 'directory' ? '/' : ''}`
  const needsSpace = value[trigger.end] && !/\s/.test(value[trigger.end]) ? ' ' : ''
  const nextValue = `${value.slice(0, trigger.start)}${insertText}${needsSpace}${value.slice(trigger.end)}`
  const caret = trigger.start + insertText.length + needsSpace.length
  return {
    value: nextValue,
    caret,
    mention: {
      kind: candidate.kind,
      root: candidate.root,
      path: candidate.path,
      label: candidate.path,
      token: insertText,
    },
  }
}

export function filterSelectedMentionsForMessage(
  mentions: SelectedChatMention[],
  message: string,
): SelectedChatMention[] {
  const seen = new Set<string>()
  const active: SelectedChatMention[] = []
  for (const mention of mentions) {
    if (!mention.token || !message.includes(mention.token)) continue
    const key = `${mention.kind}\u0000${mention.root}\u0000${mention.path}`
    if (seen.has(key)) continue
    seen.add(key)
    active.push(mention)
  }
  return active
}
