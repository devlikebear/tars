// The phone's saved ideas (docs/decisions/mobile-remote.md §6, vibe
// redesign unit 8): an idea written while no desktop answers is kept on
// the phone, so capture works with every desktop asleep or offline. It
// becomes a vibe project when the person sends it to a desktop that
// answers; the leader then asks its questions and draws the blueprint.

// Where an idea went. Ideas sent before vibe mode became the app started a
// focus task and carry its session; newer ones carry the project.
export type SentIdea = {
  desktopID: string
  desktopName: string
  projectID?: string
  sessionID?: string
  at: string
}

export type Idea = {
  id: string
  text: string
  createdAt: string
  // Where the idea went, once sent. A sent idea stays (with a way to open
  // its project) until the person deletes it.
  sent?: SentIdea
}

// ideaTitle is an idea's first line, the way a pipeline's title is its
// goal's.
export function ideaTitle(idea: Pick<Idea, 'text'>): string {
  return idea.text.trim().split('\n', 1)[0].trim()
}

// sortedIdeas puts ideas not yet sent first, then the rest; newest first in
// each group.
export function sortedIdeas(list: Idea[]): Idea[] {
  return [...list].sort((a, b) => {
    const sent = Number(!!a.sent) - Number(!!b.sent)
    if (sent) return sent
    return Date.parse(b.createdAt) - Date.parse(a.createdAt)
  })
}

export function withIdea(list: Idea[], idea: Idea): Idea[] {
  const at = list.findIndex((i) => i.id === idea.id)
  if (at < 0) return [...list, idea]
  return list.map((i, n) => (n === at ? idea : i))
}

export function withoutIdea(list: Idea[], id: string): Idea[] {
  return list.filter((i) => i.id !== id)
}

// parseIdeas reads the stored list, dropping any entry that is not whole.
export function parseIdeas(raw: string | null | undefined): Idea[] {
  if (!raw) return []
  let value: unknown
  try {
    value = JSON.parse(raw)
  } catch {
    return []
  }
  if (!Array.isArray(value)) return []
  const out: Idea[] = []
  for (const item of value) {
    if (!item || typeof item !== 'object') continue
    const d = item as Record<string, unknown>
    if (typeof d.id !== 'string' || !d.id || typeof d.text !== 'string' || !d.text.trim() || typeof d.createdAt !== 'string') continue
    const idea: Idea = { id: d.id, text: d.text, createdAt: d.createdAt }
    const s = d.sent as Record<string, unknown> | undefined
    const text = (k: string) => typeof s?.[k] === 'string' && s[k] !== ''
    if (s && typeof s === 'object' && ['desktopID', 'desktopName', 'at'].every(text) && (text('projectID') || text('sessionID'))) {
      const sent: SentIdea = { desktopID: s.desktopID as string, desktopName: s.desktopName as string, at: s.at as string }
      if (text('projectID')) sent.projectID = s.projectID as string
      if (text('sessionID')) sent.sessionID = s.sessionID as string
      idea.sent = sent
    }
    out.push(idea)
  }
  return out
}
