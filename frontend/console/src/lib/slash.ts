import type { SkillDef } from './types'

export type SlashCandidateKind = 'builtin' | 'skill'

export type SlashCommandCandidate = {
  kind: SlashCandidateKind
  command: string
  title: string
  description: string
  source?: string
  id?: string
  skillName?: string
  aliasOf?: string
}

export type ActiveSlashTrigger = {
  start: number
  end: number
  query: string
}

export type ParsedSlashCommand = {
  command: string
  args: string
}

const BUILTIN_SLASH_COMMANDS: SlashCommandCandidate[] = [
  {
    kind: 'builtin',
    id: 'compact',
    command: 'compact',
    title: 'Compact',
    description: 'Compact the current session transcript.',
  },
  {
    kind: 'builtin',
    id: 'tasks',
    command: 'tasks',
    title: 'Tasks',
    description: 'Open the session Tasks panel.',
  },
  {
    kind: 'builtin',
    id: 'config',
    command: 'config',
    title: 'Config',
    description: 'Open session tool and skill settings.',
  },
  {
    kind: 'builtin',
    id: 'context',
    command: 'context',
    title: 'Context',
    description: 'Open the LLM-facing context preview.',
  },
  {
    kind: 'builtin',
    id: 'prompt',
    command: 'prompt',
    title: 'Prompt',
    description: 'Open the session prompt editor.',
  },
  {
    kind: 'builtin',
    id: 'prompt',
    command: 'sysprompt',
    title: 'System Prompt',
    description: 'Open the session prompt editor.',
    aliasOf: 'prompt',
  },
  {
    kind: 'builtin',
    id: 'files',
    command: 'files',
    title: 'Files',
    description: 'Open the session Files panel.',
  },
  {
    kind: 'builtin',
    id: 'cron',
    command: 'cron',
    title: 'Cron',
    description: 'Open session cron jobs.',
  },
  {
    kind: 'builtin',
    id: 'memory',
    command: 'memory',
    title: 'Memory',
    description: 'Open Memory search and assets.',
  },
]

export function builtinSlashCommandId(command: string): string {
  const key = normalizeSlashCommand(command)
  return BUILTIN_SLASH_COMMANDS.find((candidate) => candidate.command === key)?.id || ''
}

export function findActiveSlashTrigger(value: string, caret = value.length): ActiveSlashTrigger | null {
  const safeCaret = Math.max(0, Math.min(caret, value.length))
  const beforeCaret = value.slice(0, safeCaret)
  const firstNonSpace = beforeCaret.search(/\S/)
  if (firstNonSpace < 0) return null
  if (beforeCaret[firstNonSpace] !== '/') return null

  const token = beforeCaret.slice(firstNonSpace + 1)
  if (/\s/.test(token)) return null
  if (!/^[\w.-]*$/.test(token)) return null

  const afterCaret = value.slice(safeCaret)
  const rest = afterCaret.match(/^[\w.-]*/)
  const tokenEnd = safeCaret + (rest?.[0]?.length ?? 0)
  return { start: firstNonSpace, end: tokenEnd, query: token }
}

export function parseLeadingSlashCommand(value: string): ParsedSlashCommand | null {
  const trimmed = value.trimStart()
  if (!trimmed.startsWith('/')) return null
  const match = trimmed.match(/^\/([\w.-]+)(?:\s+([\s\S]*))?$/)
  if (!match) return null
  return {
    command: normalizeSlashCommand(match[1]),
    args: (match[2] ?? '').trim(),
  }
}

export function buildSlashCandidates(query: string, skills: SkillDef[] = []): SlashCommandCandidate[] {
  const normalizedQuery = normalizeSlashCommand(query)
  const reserved = new Set(BUILTIN_SLASH_COMMANDS.map((candidate) => candidate.command))
  const candidates = [
    ...BUILTIN_SLASH_COMMANDS,
    ...skillSlashCandidates(skills, reserved),
  ]

  const scored = candidates
    .map((candidate, index) => ({ candidate, index, score: slashMatchScore(normalizedQuery, candidate) }))
    .filter((item) => item.score >= 0)
    .sort((a, b) => a.score - b.score || kindRank(a.candidate.kind) - kindRank(b.candidate.kind) || a.index - b.index)

  return scored.map((item) => item.candidate)
}

export function applySlashCandidate(
  value: string,
  trigger: ActiveSlashTrigger,
  candidate: SlashCommandCandidate,
): { value: string; caret: number } {
  const insertText = `/${candidate.command}`
  const nextChar = value[trigger.end]
  const needsSpace = nextChar && !/\s/.test(nextChar) ? ' ' : ''
  const nextValue = `${value.slice(0, trigger.start)}${insertText}${needsSpace}${value.slice(trigger.end)}`
  return {
    value: nextValue,
    caret: trigger.start + insertText.length + needsSpace.length,
  }
}

function skillSlashCandidates(skills: SkillDef[], reserved: Set<string>): SlashCommandCandidate[] {
  const out: SlashCommandCandidate[] = []
  const seen = new Set<string>()
  for (const skill of skills) {
    if (skill.user_invocable === false) continue
    const name = normalizeSlashCommand(skill.name)
    if (!name) continue
    const primary = normalizeSlashCommand(skill.slash || skill.name)
    const commands = uniqueCommands([primary, ...(skill.aliases ?? [])])
    for (const command of commands) {
      if (!command || reserved.has(command) || seen.has(command)) continue
      seen.add(command)
      out.push({
        kind: 'skill',
        command,
        title: `/${command}`,
        description: skill.description || 'No description provided.',
        source: skill.source,
        skillName: skill.name,
        aliasOf: command === primary ? undefined : primary,
      })
    }
  }
  return out
}

function uniqueCommands(values: string[]): string[] {
  const out: string[] = []
  const seen = new Set<string>()
  for (const value of values) {
    const command = normalizeSlashCommand(value)
    if (!command || seen.has(command)) continue
    seen.add(command)
    out.push(command)
  }
  return out
}

function slashMatchScore(query: string, candidate: SlashCommandCandidate): number {
  if (!query) return candidate.kind === 'builtin' ? 0 : 10
  const haystack = [
    candidate.command,
    candidate.title,
    candidate.description,
    candidate.source,
    candidate.skillName,
    candidate.aliasOf,
  ].filter(Boolean).join(' ').toLowerCase()
  const command = candidate.command.toLowerCase()
  if (command === query) return 0
  if (command.startsWith(query)) return 1
  if (command.includes(query)) return 2
  if (fuzzyIncludes(command, query)) return 3
  if (query.length >= 4 && haystack.includes(query)) return 4
  return -1
}

function fuzzyIncludes(value: string, query: string): boolean {
  let idx = 0
  for (const ch of query) {
    idx = value.indexOf(ch, idx)
    if (idx < 0) return false
    idx++
  }
  return true
}

function normalizeSlashCommand(value: string | undefined): string {
  return (value ?? '').trim().replace(/^\/+/, '').toLowerCase()
}

function kindRank(kind: SlashCandidateKind): number {
  return kind === 'builtin' ? 0 : 1
}
