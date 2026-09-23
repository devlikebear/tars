// Sidebar navigation groups (#968): Work · Build · System.
//
// Paths and role gating come from the command palette's page table, so the
// nav, the palette, and App.svelte's route gating cannot drift apart. Pages
// that are not listed here (home, lineage, plans, channels, onboarding) stay
// reachable through ⌘K and in-context links.

import { pageEntries, type PageView } from './commands.ts'

export type NavGroupId = 'work' | 'build' | 'system'

// Label keys under `nav.items`.
export type NavItemLabel =
  | 'chat'
  | 'agentruntime'
  | 'memory'
  | 'extensions'
  | 'sysprompt'
  | 'ops'
  | 'pulse'
  | 'reflection'
  | 'cron'
  | 'logs'
  | 'analytics'
  | 'config'

export type NavItem = {
  view: PageView
  label: NavItemLabel
  path: string
  icon: string
}

export type NavGroup = {
  id: NavGroupId
  items: NavItem[]
}

const groupViews: { id: NavGroupId; items: { view: PageView; label: NavItemLabel; icon: string }[] }[] = [
  {
    id: 'work',
    items: [
      { view: 'chat', label: 'chat', icon: '◎' },
    ],
  },
  {
    id: 'build',
    items: [
      { view: 'agentruntime', label: 'agentruntime', icon: '⧉' },
      { view: 'memory', label: 'memory', icon: '◈' },
      { view: 'extensions', label: 'extensions', icon: '⊞' },
      { view: 'sysprompt', label: 'sysprompt', icon: '¶' },
    ],
  },
  {
    id: 'system',
    items: [
      { view: 'ops', label: 'ops', icon: '⚙' },
      { view: 'pulse', label: 'pulse', icon: '♡' },
      { view: 'reflection', label: 'reflection', icon: '☾' },
      { view: 'cron', label: 'cron', icon: '◷' },
      { view: 'logs', label: 'logs', icon: '≡' },
      { view: 'analytics', label: 'analytics', icon: '▤' },
      { view: 'config', label: 'config', icon: '☸' },
    ],
  },
]

function pageFor(view: PageView) {
  const page = pageEntries.find((entry) => entry.view === view)
  if (!page) throw new Error(`nav: no page entry for view ${view}`)
  return page
}

export const navGroups: NavGroup[] = groupViews.map((group) => ({
  id: group.id,
  items: group.items.map((item) => ({ ...item, path: pageFor(item.view).path })),
}))

// Groups as the given role sees them. App.svelte renders admin-only views
// only for admin-level roles, so listing them for `user` would link to a page
// that silently falls through to Home.
export function visibleNavGroups(authRole: string): NavGroup[] {
  if (authRole !== 'user') return navGroups
  return navGroups
    .map((group) => ({ ...group, items: group.items.filter((item) => !pageFor(item.view).adminOnly) }))
    .filter((group) => group.items.length > 0)
}
