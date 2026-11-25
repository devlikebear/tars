// One vibe-mode project as runes state (docs/plans/vibe-mode-redesign units
// 4, 5 and 8): the project, its run, and the few things a person can do to
// it — answer the leader's questions, approve the blueprint or say what to
// change in it, continue after the usage limit, try again when the team is
// stuck, start over a project that could not be set up, and ask for a
// change once it is finished. The desk's workspace (EasyProject.svelte)
// and the phone's (src/mobile/MobileProject.svelte) draw it differently
// and share this. Every call goes through the EasyAPI it is given, so the
// phone aims it at one desktop. Nothing here decides a transition: after
// each answer it reads the server again.
import { APIRequestError } from '../api/client.ts'
import type { EasyAPI } from '../api/easy.ts'
import { easyErrorCode } from '../api/easy.ts'
import { easyLive, easyRetryAction, easyView, recreateEasyProject, type EasyProject, type EasyView } from '../easy.ts'
import type { FocusPipeline } from '../types.ts'

// What can be wrong with "tell the team" or a change request, as a key
// the screen turns into words: nothing written, or an error code from the
// server ('' when it gave none).
export type EasySayError = '' | 'required' | 'failed'

export type EasyProjectStoreDeps = {
  api: EasyAPI
  projectId: string
  // How often the screen reads while the project changes on its own.
  pollMs?: number
  // When something else says the project may have changed (the phone's
  // desktop event stream), the store reads at once.
  changes?: { listen(onChange: () => void): () => void }
}

export class EasyProjectStore {
  project = $state<EasyProject | null>(null)
  pipeline = $state<FocusPipeline | null>(null)
  notFound = $state(false)
  loadFailed = $state(false)
  // One action at a time; each says it is running so its button waits.
  approving = $state(false)
  continuing = $state(false)
  answering = $state('')
  saying = $state(false)
  retrying = $state(false)
  recreating = $state(false)
  changing = $state(false)
  // The last action failed (the screen says so near the bar).
  actionFailed = $state(false)
  sayError = $state<EasySayError>('')
  // A change refused: 'required', or the server's code ('' = unknown).
  changeError = $state<string | null>(null)

  view = $derived<EasyView | null>(this.project ? easyView(this.project, this.pipeline) : null)

  #deps: EasyProjectStoreDeps
  #timer: ReturnType<typeof setTimeout> | undefined
  #stopped = false
  #unlisten: (() => void) | undefined

  constructor(deps: EasyProjectStoreDeps) {
    this.#deps = deps
  }

  get api(): EasyAPI {
    return this.#deps.api
  }

  get projectId(): string {
    return this.#deps.projectId
  }

  // start reads the project and keeps reading while it changes on its own.
  start(): void {
    this.#stopped = false
    this.#unlisten = this.#deps.changes?.listen(() => void this.refresh())
    void this.#loop()
  }

  stop(): void {
    this.#stopped = true
    clearTimeout(this.#timer)
    this.#unlisten?.()
    this.#unlisten = undefined
  }

  // read fetches the project and then its run, and shows them together:
  // after a change the project names a new run, and showing it with the
  // old one (or none) would flash the wrong step.
  async read(): Promise<void> {
    const api = this.#deps.api
    try {
      const next = await api.get(this.#deps.projectId)
      const nextPipeline = next.status === 'ready' && next.session_id ? await api.pipeline(next.session_id) : null
      this.project = next
      this.pipeline = nextPipeline
      this.loadFailed = false
    } catch (err) {
      if (err instanceof APIRequestError && err.status === 404) {
        this.notFound = true
        return
      }
      this.loadFailed = true
    }
  }

  // refresh reads now and starts the timer again from here.
  async refresh(): Promise<void> {
    clearTimeout(this.#timer)
    await this.#loop()
  }

  async #loop(): Promise<void> {
    await this.read()
    if (this.#stopped || this.notFound) return
    const view = this.view
    clearTimeout(this.#timer)
    if (!view || easyLive(view) || this.loadFailed) this.#timer = setTimeout(() => void this.#loop(), this.#deps.pollMs ?? 2000)
  }

  // The plan gate's open card: the action names it so a gate the server
  // replaced since refuses it.
  #planCard(): { card_id?: string } {
    const card = this.pipeline?.cards.findLast((c) => c.kind === 'gate' && c.state !== 'decided')
    return card ? { card_id: card.id } : {}
  }

  get #session(): string | undefined {
    return this.project?.session_id
  }

  // approve takes the blueprint; the server then goes on by itself
  // (GoalAfterPlan).
  async approve(): Promise<void> {
    const session = this.#session
    if (!session || this.approving) return
    this.approving = true
    this.actionFailed = false
    try {
      await this.#deps.api.gate(session, 'plan', 'approve', this.#planCard())
      await this.refresh()
    } catch {
      this.actionFailed = true
    } finally {
      this.approving = false
    }
  }

  // continueAfterLimit sends the turn the usage limit refused again; the
  // server puts goal mode back on (GoalDueOnApproval).
  async continueAfterLimit(): Promise<void> {
    const session = this.#session
    const limit = this.view?.limit
    if (!session || !limit || this.continuing) return
    this.continuing = true
    this.actionFailed = false
    try {
      await this.#deps.api.gate(session, 'blocked', 'retry', { card_id: limit.cardId })
      await this.refresh()
    } catch {
      this.actionFailed = true
    } finally {
      this.continuing = false
    }
  }

  // answer decides one of the leader's questions with the choice's words.
  // The last answer sends them all to the leader (the server's rule).
  async answer(cardId: string, choice: string): Promise<void> {
    const session = this.#session
    if (!session || this.answering) return
    this.answering = cardId
    this.actionFailed = false
    try {
      await this.#deps.api.card(session, cardId, 'decided', choice)
      await this.refresh()
    } catch {
      this.actionFailed = true
    } finally {
      this.answering = ''
    }
  }

  // sayToTeam is the blueprint's "tell the team": the plan gate's request
  // for changes with the person's words; the leader draws it again.
  // Answers whether the words went.
  async sayToTeam(words: string): Promise<boolean> {
    const session = this.#session
    if (!session || this.saying) return false
    if (!words.trim()) {
      this.sayError = 'required'
      return false
    }
    this.saying = true
    this.sayError = ''
    try {
      await this.#deps.api.gate(session, 'plan', 'request_changes', { note: words.trim(), ...this.#planCard() })
      await this.refresh()
      return true
    } catch {
      this.sayError = 'failed'
      return false
    } finally {
      this.saying = false
    }
  }

  // retryStuck is "Try again" on a stuck project: the blocked step again,
  // with the person's words when they explained again. A gate answered
  // elsewhere meanwhile (409) only means the screen is behind. Answers
  // whether the note was used.
  async retryStuck(note: string): Promise<boolean> {
    const session = this.#session
    const retry = this.view?.retry
    if (!session || !retry || this.retrying) return false
    this.retrying = true
    this.actionFailed = false
    let sent = false
    try {
      const { action, note: words } = easyRetryAction(note)
      await this.#deps.api.gate(session, 'blocked', action, { note: words, card_id: retry.cardId })
      sent = true
    } catch (err) {
      if (!(err instanceof APIRequestError && err.status === 409)) this.actionFailed = true
    } finally {
      await this.refresh()
      this.retrying = false
    }
    return sent
  }

  // recreate starts the idea of a project that could not be set up as a
  // new project; answers the new project's id, or null.
  async recreate(): Promise<string | null> {
    const project = this.project
    if (!project || this.recreating) return null
    this.recreating = true
    this.actionFailed = false
    try {
      return await recreateEasyProject({ create: (idea) => this.#deps.api.create(idea), remove: (id) => this.#deps.api.remove(id) }, project)
    } catch {
      this.actionFailed = true
      this.recreating = false
      return null
    }
  }

  // change asks for "change it like this"; answers whether it started.
  async change(request: string): Promise<boolean> {
    const project = this.project
    if (!project || this.changing) return false
    if (!request.trim()) {
      this.changeError = 'change_required'
      return false
    }
    this.changing = true
    this.changeError = null
    try {
      await this.#deps.api.change(project.id, request)
      await this.refresh()
      return true
    } catch (err) {
      this.changeError = easyErrorCode(err)
      return false
    } finally {
      this.changing = false
    }
  }
}
