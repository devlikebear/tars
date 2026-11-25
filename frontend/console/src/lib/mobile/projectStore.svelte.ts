// The phone's projects as runes state (docs/plans/vibe-mode-redesign unit
// 8): every vibe project on every paired desktop that answers, with the
// desk's board state, each desk's readiness, and the few things the list
// itself does — rename, archive, and hand a new idea to a desk. The
// network comes in through the constructor so tests run it under Node.
import { APIRequestError } from '../api/client.ts'
import { easyErrorCode, type EasyAPI } from '../api/easy.ts'
import type { EasyProject, EasyReadiness } from '../easy.ts'
import { targetOf, type PairedDesktop } from './desktops.ts'
import { remoteEasyApi } from './easyRemote.ts'
import { phoneProjectKey, type PhoneProject } from './projects.ts'

export type ProjectStoreDeps = {
  // The easy calls aimed at one desktop.
  apiFor?: (desktop: PairedDesktop) => EasyAPI
}

// Why a request from the list did not go through, as a key into the
// strings: a server code (busy, name_required, idea_required, …), the desk
// revoked this phone, or no answer.
export type ProjectFailure = string

// A read of every project bounds each desk this long.
const readTimeoutMs = 8000

export function failureOf(err: unknown): ProjectFailure {
  if (!(err instanceof APIRequestError)) return 'unreachable'
  if (err.status === 401) return 'unpaired'
  return easyErrorCode(err) || 'unknown'
}

export class ProjectStore {
  // Per desktop, so one desk read again leaves the others as they are.
  byDesk = $state<Record<string, PhoneProject[]>>({})
  readiness = $state<Record<string, EasyReadiness | null>>({})
  // Desktops that answer their health check but not the project list.
  failed = $state<string[]>([])
  loaded = $state(false)
  reading = $state(false)

  projects = $derived(Object.values(this.byDesk).flat())

  #apiFor: (desktop: PairedDesktop) => EasyAPI

  constructor(deps: ProjectStoreDeps = {}) {
    this.#apiFor = deps.apiFor ?? ((d) => remoteEasyApi(targetOf(d), { timeoutMs: readTimeoutMs }))
  }

  api(desktop: PairedDesktop): EasyAPI {
    return this.#apiFor(desktop)
  }

  // refresh reads every desktop that answers; the projects of a desk that
  // stopped answering leave the list (the Computers tab says why).
  async refresh(online: PairedDesktop[]): Promise<void> {
    this.reading = true
    try {
      const ids = new Set(online.map((d) => d.id))
      const keep = (r: Record<string, unknown>) => Object.fromEntries(Object.entries(r).filter(([id]) => ids.has(id)))
      this.byDesk = keep(this.byDesk) as Record<string, PhoneProject[]>
      this.readiness = keep(this.readiness) as Record<string, EasyReadiness | null>
      this.failed = this.failed.filter((id) => ids.has(id))
      await Promise.all(online.map((d) => this.refreshDesktop(d)))
    } finally {
      this.reading = false
      this.loaded = true
    }
  }

  async refreshDesktop(desktop: PairedDesktop): Promise<void> {
    const api = this.#apiFor(desktop)
    const [board, readiness] = await Promise.allSettled([api.board(), api.readiness()])
    if (board.status === 'fulfilled') {
      this.byDesk = {
        ...this.byDesk,
        [desktop.id]: board.value.projects.map((p) => ({ ...p, desktopID: desktop.id, desktopName: desktop.name })),
      }
      this.failed = this.failed.filter((id) => id !== desktop.id)
    } else if (!this.failed.includes(desktop.id)) {
      this.failed = [...this.failed, desktop.id]
    }
    // A desk too old to say keeps no answer; the screens read that as "no
    // problem known".
    this.readiness = { ...this.readiness, [desktop.id]: readiness.status === 'fulfilled' ? readiness.value : null }
  }

  find(desktopID: string, projectID: string): PhoneProject | undefined {
    return this.byDesk[desktopID]?.find((p) => p.id === projectID)
  }

  // The project whose current run is this session, for a link that names
  // the run (an alert's /console/focus/<session>).
  findBySession(desktopID: string, sessionID: string): PhoneProject | undefined {
    return this.byDesk[desktopID]?.find((p) => p.session_id === sessionID || p.changes?.some((c) => c.session_id === sessionID))
  }

  #replace(project: PhoneProject) {
    const list = this.byDesk[project.desktopID] ?? []
    this.byDesk = { ...this.byDesk, [project.desktopID]: list.map((p) => (phoneProjectKey(p) === phoneProjectKey(project) ? project : p)) }
  }

  #drop(project: PhoneProject) {
    const list = this.byDesk[project.desktopID] ?? []
    this.byDesk = { ...this.byDesk, [project.desktopID]: list.filter((p) => p.id !== project.id) }
  }

  // rename answers '' when it went through, else why not.
  async rename(desktop: PairedDesktop, project: PhoneProject, name: string): Promise<ProjectFailure> {
    if (!name.trim()) return 'name_required'
    try {
      const listed = await this.#apiFor(desktop).update(project.id, { name: name.trim() })
      this.#replace({ ...listed, desktopID: project.desktopID, desktopName: project.desktopName })
      return ''
    } catch (err) {
      return failureOf(err)
    }
  }

  // archive takes a project off the list; the desk keeps it under Archive.
  async archive(desktop: PairedDesktop, project: PhoneProject): Promise<ProjectFailure> {
    try {
      await this.#apiFor(desktop).update(project.id, { archived: true })
      this.#drop(project)
      return ''
    } catch (err) {
      return failureOf(err)
    }
  }

  // create hands an idea to a desk's team; answers the new project or why
  // not.
  async create(desktop: PairedDesktop, idea: string): Promise<{ project: EasyProject } | { failure: ProjectFailure }> {
    if (!idea.trim()) return { failure: 'idea_required' }
    try {
      const { project } = await this.#apiFor(desktop).create(idea.trim())
      void this.refreshDesktop(desktop)
      return { project }
    } catch (err) {
      return { failure: failureOf(err) }
    }
  }
}
