// Command palette registry and matching (#968).
//
// The palette is the universal way into the console: every route, including
// ones the nav does not list, must be reachable from ⌘K in at most two
// keystrokes. Command lists are built from the route table, the dock panels,
// the session list, and the builtin slash commands, so a new route or panel
// is covered by adding it to its source list.

import type { Route } from './router'
import type { ShortcutAction } from './shortcuts'

export type CommandGroup = 'action' | 'page' | 'panel' | 'session' | 'slash'

export type PaletteCommand = {
  id: string
  group: CommandGroup
  title: string
  subtitle?: string
  keywords?: string[]
  shortcut?: ShortcutAction
  run: () => void | Promise<void>
}

export type PageView = Route['view']

export type PageEntry = {
  view: PageView
  path: string
  // The route renders only for admin-level roles (App.svelte gates it).
  adminOnly: boolean
  keywords: string[]
}

// One entry per routable view. App.svelte gates the admin-only views behind
// `authRole !== 'user'`; keep the two lists in step.
export const pageEntries: PageEntry[] = [
  { view: 'home', path: '/console', adminOnly: false, keywords: ['dashboard', 'overview'] },
  { view: 'chat', path: '/console/chat', adminOnly: false, keywords: ['conversation', 'session'] },
  { view: 'session-lineage', path: '/console/sessions/graph', adminOnly: false, keywords: ['fork', 'graph', 'history'] },
  { view: 'tasks', path: '/console/tasks', adminOnly: false, keywords: ['plans', 'work', 'contracts'] },
  { view: 'agentruntime', path: '/console/agentruntime', adminOnly: false, keywords: ['runs', 'subagents', 'agents'] },
  { view: 'memory', path: '/console/memory', adminOnly: false, keywords: ['knowledge', 'inbox'] },
  { view: 'sysprompt', path: '/console/sysprompt', adminOnly: false, keywords: ['system prompt', 'workspace'] },
  { view: 'ops', path: '/console/approvals', adminOnly: true, keywords: ['approvals', 'ops', 'cleanup'] },
  { view: 'cron', path: '/console/cron', adminOnly: true, keywords: ['schedule', 'jobs'] },
  { view: 'logs', path: '/console/logs', adminOnly: true, keywords: ['log', 'tail'] },
  { view: 'analytics', path: '/console/analytics', adminOnly: true, keywords: ['usage', 'cost'] },
  { view: 'config', path: '/console/config', adminOnly: true, keywords: ['settings', 'quick start'] },
  { view: 'extensions', path: '/console/extensions', adminOnly: true, keywords: ['skills', 'plugins', 'mcp'] },
  { view: 'pulse', path: '/console/pulse', adminOnly: true, keywords: ['watchdog', 'health'] },
  { view: 'reflection', path: '/console/reflection', adminOnly: true, keywords: ['nightly', 'batch'] },
  { view: 'channels', path: '/console/channels', adminOnly: true, keywords: ['telegram', 'pairing'] },
  { view: 'onboarding', path: '/console/onboarding?reentry=1', adminOnly: true, keywords: ['setup', 'wizard', 'provider', 'tiers'] },
]

export function pageCommands(
  titles: Record<PageView, string>,
  authRole: string,
  navigate: (path: string) => void,
): PaletteCommand[] {
  return pageEntries
    .filter((page) => !page.adminOnly || authRole !== 'user')
    .map((page) => ({
      id: `page:${page.view}`,
      group: 'page' as const,
      title: titles[page.view] ?? page.view,
      subtitle: page.path.split('?')[0],
      keywords: page.keywords,
      run: () => navigate(page.path),
    }))
}

// Rank for `query` against a command, or 0 when it does not match. Prefix
// matches on the title beat word-prefix, substring, and finally subsequence
// matches, so a two-letter query lands on the obvious page.
export function scoreCommand(command: Pick<PaletteCommand, 'title' | 'keywords' | 'subtitle'>, query: string): number {
  const q = query.trim().toLowerCase()
  if (!q) return 1
  const title = command.title.toLowerCase()
  let best = scoreText(title, q, 1)
  for (const keyword of command.keywords ?? []) best = Math.max(best, scoreText(keyword.toLowerCase(), q, 0.6))
  if (command.subtitle) best = Math.max(best, scoreText(command.subtitle.toLowerCase(), q, 0.4))
  return best
}

function scoreText(text: string, q: string, weight: number): number {
  if (!text) return 0
  if (text === q) return 120 * weight
  if (text.startsWith(q)) return 100 * weight
  if (text.split(/[\s/_-]+/).some((word) => word.startsWith(q))) return 80 * weight
  if (text.includes(q)) return 60 * weight
  // Subsequence: every query character appears in order. Fewer gaps rank higher.
  let from = 0
  let gaps = 0
  for (const ch of q) {
    const at = text.indexOf(ch, from)
    if (at < 0) return 0
    gaps += at - from
    from = at + 1
  }
  return Math.max(1, 40 - gaps) * weight
}

const groupOrder: CommandGroup[] = ['action', 'page', 'panel', 'session', 'slash']

// Matching commands with each group kept together, so the palette shows one
// heading per group. Groups are ordered by their best match, so the top row
// is still the best match overall; rows within a group are ordered by score.
// Ties fall back to group order, then source order.
export function filterCommands(commands: PaletteCommand[], query: string, limit = 60): PaletteCommand[] {
  const matched = commands
    .map((command, index) => ({ command, index, score: scoreCommand(command, query) }))
    .filter((entry) => entry.score > 0)
  const groupBest = new Map<CommandGroup, number>()
  for (const entry of matched) {
    groupBest.set(entry.command.group, Math.max(groupBest.get(entry.command.group) ?? 0, entry.score))
  }
  return matched
    .sort((a, b) =>
      (groupBest.get(b.command.group) ?? 0) - (groupBest.get(a.command.group) ?? 0) ||
      groupOrder.indexOf(a.command.group) - groupOrder.indexOf(b.command.group) ||
      b.score - a.score ||
      a.index - b.index)
    .slice(0, limit)
    .map((entry) => entry.command)
}
