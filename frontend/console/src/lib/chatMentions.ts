export type ChatMentionKind = 'file' | 'directory'

export type ChatMentionCandidate = {
  kind: ChatMentionKind
  name: string
  path: string
  root: string
  root_label: string
  token: string
  size?: number
  updated_at?: string
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
