import type { ChatCommandsTranslations } from '../i18n/sections/chatCommands'
import type { CommandDef, SkillDef } from './types'

// Titles and descriptions of the builtins, plus the fallback description
// for skills and commands that have none. Callers pass `$t.chatCommands.slash`.
export type SlashText = ChatCommandsTranslations['slash']

export type SlashCandidateKind = 'builtin' | 'skill' | 'command'

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

type BuiltinSlashCommand = {
  id: string
  command: string
  // Key of the command's title and description in SlashText['builtins'].
  text: keyof SlashText['builtins']
  aliasOf?: string
}

const BUILTIN_SLASH_COMMANDS: BuiltinSlashCommand[] = [
  { id: 'clear', command: 'clear', text: 'clear' },
  { id: 'status', command: 'status', text: 'status' },
  { id: 'compact', command: 'compact', text: 'compact' },
  { id: 'tasks', command: 'tasks', text: 'tasks' },
  { id: 'config', command: 'config', text: 'config' },
  { id: 'context', command: 'context', text: 'context' },
  { id: 'prior', command: 'prior', text: 'prior' },
  { id: 'prompt', command: 'prompt', text: 'prompt' },
  { id: 'prompt', command: 'sysprompt', text: 'sysprompt', aliasOf: 'prompt' },
  { id: 'files', command: 'files', text: 'files' },
  { id: 'cron', command: 'cron', text: 'cron' },
  { id: 'memory', command: 'memory', text: 'memory' },
  { id: 'skill', command: 'skill', text: 'skill' },
  { id: 'extract-skill', command: 'extract-skill', text: 'extractSkill' },
  { id: 'cwd', command: 'cwd', text: 'cwd' },
  { id: 'goal', command: 'goal', text: 'goal' },
]

function builtinSlashCandidates(text: SlashText): SlashCommandCandidate[] {
  return BUILTIN_SLASH_COMMANDS.map(({ id, command, text: key, aliasOf }): SlashCommandCandidate => ({
    kind: 'builtin',
    id,
    command,
    title: text.builtins[key].title,
    description: text.builtins[key].description,
    ...(aliasOf ? { aliasOf } : {}),
  }))
}

// Builtin slash commands, one per id (the command palette lists these).
export function builtinSlashCommands(text: SlashText): SlashCommandCandidate[] {
  const seen = new Set<string>()
  return builtinSlashCandidates(text).filter((candidate) => {
    const id = candidate.id ?? candidate.command
    if (seen.has(id)) return false
    seen.add(id)
    return true
  })
}

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
  if (token.includes('/')) return null

  const afterCaret = value.slice(safeCaret)
  const rest = afterCaret.match(/^[^\s/]*/)
  const tokenEnd = safeCaret + (rest?.[0]?.length ?? 0)
  return { start: firstNonSpace, end: tokenEnd, query: token }
}

export function parseLeadingSlashCommand(value: string): ParsedSlashCommand | null {
  const trimmed = value.trimStart()
  if (!trimmed.startsWith('/')) return null
  const body = trimmed.slice(1)
  const firstArg = firstSlashCommandWhitespace(body)
  const rawCommand = firstArg < 0 ? body : body.slice(0, firstArg)
  if (!rawCommand || rawCommand.includes('/')) return null

  return {
    command: normalizeSlashCommand(rawCommand),
    args: (firstArg < 0 ? '' : body.slice(firstArg)).trim(),
  }
}

export function buildSlashCandidates(
  query: string,
  skills: SkillDef[],
  commands: CommandDef[],
  text: SlashText,
): SlashCommandCandidate[] {
  const normalizedQuery = normalizeSlashCommand(query)
  const reserved = new Set(BUILTIN_SLASH_COMMANDS.map((candidate) => candidate.command))
  const candidates = [
    ...builtinSlashCandidates(text),
    ...commandSlashCandidates(commands, reserved, text),
    ...skillSlashCandidates(skills, reserved, text),
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

function commandSlashCandidates(commands: CommandDef[], reserved: Set<string>, text: SlashText): SlashCommandCandidate[] {
  const out: SlashCommandCandidate[] = []
  const seen = new Set<string>()
  for (const commandDef of commands) {
    if (commandDef.user_invocable === false) continue
    const name = normalizeSlashCommand(commandDef.name)
    if (!name) continue
    const primary = normalizeSlashCommand(commandDef.slash || commandDef.name)
    const commands = uniqueCommands([primary, ...(commandDef.aliases ?? [])])
    for (const command of commands) {
      if (!command || reserved.has(command) || seen.has(command)) continue
      seen.add(command)
      out.push({
        kind: 'command',
        command,
        title: `/${command}`,
        description: commandDef.description || text.noDescription,
        source: commandDef.source,
        skillName: commandDef.name,
        aliasOf: command === primary ? undefined : primary,
      })
    }
  }
  return out
}

function skillSlashCandidates(skills: SkillDef[], reserved: Set<string>, text: SlashText): SlashCommandCandidate[] {
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
        description: skill.description || text.noDescription,
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

function firstSlashCommandWhitespace(value: string): number {
  for (let i = 0; i < value.length; i++) {
    if (isSlashCommandWhitespace(value.charCodeAt(i))) return i
  }
  return -1
}

function isSlashCommandWhitespace(code: number): boolean {
  return code === 9 || code === 10 || code === 11 || code === 12 || code === 13 || code === 32 || code === 160
}

function kindRank(kind: SlashCandidateKind): number {
  if (kind === 'builtin') return 0
  if (kind === 'command') return 1
  return 2
}
