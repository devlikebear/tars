// The phone's vibe-mode screens (docs/plans/vibe-mode-redesign unit 8,
// screens/M-*): projects from every paired desktop that answers, in one
// list by the server's board state, and which tabs a project's screen
// shows at phone width. Pure: the stores and screens only render this.
import { boardBefore, columnOf, type EasyColumn, type EasyListed } from '../easyBoard.ts'
import type { EasyReadiness, EasyView } from '../easy.ts'

// A project as the phone lists it: the desk's record and board, and which
// paired desktop it lives on.
export type PhoneProject = EasyListed & { desktopID: string; desktopName: string }

export function phoneProjectKey(p: Pick<PhoneProject, 'desktopID' | 'id'>): string {
  return `${p.desktopID}\n${p.id}`
}

// The chips above the list (screens/M-Projects): everything, then the three
// a person looks for. Stopped projects show under everything only.
export const PHONE_FILTERS = ['all', 'your_turn', 'working', 'done'] as const
export type PhoneFilter = (typeof PHONE_FILTERS)[number]

// The list's sections, in the order a person reads them: what waits on
// them first (the Waiting inbox folded in), then what the team makes, what
// stopped, and what is finished.
export const PHONE_SECTIONS = ['your_turn', 'working', 'stopped', 'done'] as const
export type PhoneSection = (typeof PHONE_SECTIONS)[number]

function sectionOf(p: PhoneProject): PhoneSection {
  const column: EasyColumn = columnOf(p.board.state)
  // Online (published, unit 9) reads as finished on the phone.
  return column === 'published' ? 'done' : column
}

function inFilter(section: PhoneSection, filter: PhoneFilter): boolean {
  return filter === 'all' || filter === section
}

// phoneSections answers the non-empty sections a filter shows, newest change
// first in each.
export function phoneSections(projects: PhoneProject[], filter: PhoneFilter): Array<{ section: PhoneSection; projects: PhoneProject[] }> {
  const sorted = [...projects].sort((a, b) => (boardBefore(a, b) ? -1 : boardBefore(b, a) ? 1 : 0))
  return PHONE_SECTIONS.filter((s) => inFilter(s, filter))
    .map((section) => ({ section, projects: sorted.filter((p) => sectionOf(p) === section) }))
    .filter((s) => s.projects.length > 0)
}

export function filterCounts(projects: PhoneProject[]): Record<PhoneFilter, number> {
  const counts: Record<PhoneFilter, number> = { all: projects.length, your_turn: 0, working: 0, done: 0 }
  for (const p of projects) {
    const section = sectionOf(p)
    if (section !== 'stopped') counts[section]++
  }
  return counts
}

// How many projects wait on the person: the tab's badge.
export function waitingCount(projects: PhoneProject[]): number {
  return projects.filter((p) => p.board.state === 'your_turn').length
}

// The tabs of a project's screen (screens/M-Plan, M-Progress, M-Result,
// M-Versions). A phone shows one pane at a time where the desk shows them
// side by side.
//   progress  — the step's own content: making, checking, or the state
//               the project is in (getting ready, paused, stuck)
//   team      — the team's conversation, with the leader's questions
//   blueprint — the plan to approve, or the one being made
//   app       — the finished app (on the desk for now: unit 7)
//   versions  — saved versions and going back
//   checks    — the finished version's check results
export type PhoneTab = 'progress' | 'team' | 'blueprint' | 'app' | 'versions' | 'checks'

export function phoneTabs(view: Pick<EasyView, 'phase' | 'blueprint'>): PhoneTab[] {
  switch (view.phase) {
    case 'creating':
    case 'failed':
      return ['progress', 'team']
    case 'planning':
    case 'questions':
      return ['team']
    case 'approve':
      return ['blueprint', 'team']
    case 'finished':
      return ['app', 'team', 'versions', 'checks']
    default:
      return view.blueprint ? ['progress', 'team', 'blueprint'] : ['progress', 'team']
  }
}

// The tab a project opens on: the first, which holds the one thing to do
// or to look at.
export function defaultPhoneTab(view: Pick<EasyView, 'phase' | 'blueprint'>): PhoneTab {
  return phoneTabs(view)[0]
}

// One line of the Computers tab (screens/M-Pair), in plain words: whether
// the desk answers, whether its AI is installed and signed in, and whether
// the tools a project needs are there. Keeping the desk awake is the
// desk's own setting, which a phone cannot read.
export type DeskLine = { key: 'on' | 'ai' | 'aiMissing' | 'aiSignedOut' | 'aiNotSet' | 'tools' | 'toolsMissing'; ok: boolean }

export function deskLines(readiness: EasyReadiness | null | undefined): DeskLine[] {
  const lines: DeskLine[] = [{ key: 'on', ok: true }]
  if (!readiness) return lines
  switch (readiness.engine) {
    case 'ok':
      lines.push({ key: 'ai', ok: true })
      break
    case 'signed_out':
      lines.push({ key: 'aiSignedOut', ok: false })
      break
    case 'missing':
      lines.push({ key: 'aiMissing', ok: false })
      break
    default:
      lines.push({ key: 'aiNotSet', ok: false })
  }
  lines.push(readiness.tools === 'ok' ? { key: 'tools', ok: true } : { key: 'toolsMissing', ok: false })
  return lines
}

// Whether a desk can take a new idea now: it answers and its readiness says
// a project can be made. A desk too old to say is given the benefit of the
// doubt; the desk refuses the idea itself if it cannot.
export function deskTakesIdeas(online: boolean, readiness: EasyReadiness | null | undefined): boolean {
  return online && (readiness ? readiness.ready : true)
}
