// Hearing a desktop's focus pipelines change as they change
// (docs/decisions/mobile-remote.md §4): one GET /v1/focus/events stream per
// desktop that answers. The stream says only which pipeline changed; the
// app reads that desktop's lists again, which are already scoped to what a
// phone may see. The 30-second read stays for desktops that come and go
// and for desktops too old to have the stream.
import { APIRequestError, type APITarget } from '../api/client.ts'
import { targetOf, type PairedDesktop } from './desktops.ts'

// One message on the stream: "connected" first, then "pipeline" with the
// session that changed, and "keepalive" while nothing happens.
export type FocusStreamEvent = { type: string; session_id?: string }

export type EventsAPI = {
  // open resolves when the desktop ends the stream and rejects when it
  // cannot be opened or breaks; a refusal is an APIRequestError.
  open(target: APITarget, onEvent: (event: FocusStreamEvent) => void, signal: AbortSignal): Promise<void>
}

// Waits before opening a dropped stream again, longer each time; the last
// one repeats.
const defaultRetryMs = [1000, 2000, 5000, 10_000, 30_000]
// Gathers one change's events (a turn ends: activity, then the pipeline)
// into one read of the desktop.
const defaultSettleMs = 250

type Watched = {
  desktop: PairedDesktop
  key: string
  controller: AbortController | null
  retry: ReturnType<typeof setTimeout> | null
  settle: ReturnType<typeof setTimeout> | null
  attempt: number
  // The stream broke or could not open since it last connected: a change
  // may have been missed, so the next "connected" reads the desktop again.
  dropped: boolean
  // The desktop refused the stream (too old, or the token is gone); it is
  // not asked again until it is watched afresh.
  refused: boolean
  // The stream said "connected" and has not ended since.
  live: boolean
  // What changed since the last call back: pipeline sessions, or every
  // pipeline (a reconnect after a drop, when changes may have been missed).
  changedSessions: Set<string>
  changedAll: boolean
}

// What a call back says changed on a desktop: the pipeline sessions the
// stream named, or all of them ("all": changes may have been missed).
export type DesktopChange = { desktopID: string; sessions: string[]; all: boolean }

// touches reports whether change may concern the pipeline sessionID.
export function touches(change: DesktopChange, desktopID: string, sessionID: string): boolean {
  return change.desktopID === desktopID && (change.all || change.sessions.includes(sessionID))
}

// What a screen showing one pipeline needs from the stream: a call back
// when that pipeline may have changed, and whether the stream is open now.
export type PipelineChanges = {
  listen(onChange: () => void): () => void
  live(): boolean
}

// pipelineChanges narrows events to one pipeline on one desktop.
export function pipelineChanges(events: Pick<DesktopEvents, 'listen' | 'isLive'>, desktopID: string, sessionID: string): PipelineChanges {
  return {
    listen: (onChange) => events.listen((change) => {
      if (touches(change, desktopID, sessionID)) onChange()
    }),
    live: () => events.isLive(desktopID),
  }
}

export class DesktopEvents {
  #api: EventsAPI
  #retryMs: number[]
  #settleMs: number
  #watched = new Map<string, Watched>()
  #listeners = new Set<(change: DesktopChange) => void>()

  constructor(deps: { api: EventsAPI; retryMs?: number[]; settleMs?: number }) {
    this.#api = deps.api
    this.#retryMs = deps.retryMs?.length ? deps.retryMs : defaultRetryMs
    this.#settleMs = deps.settleMs ?? defaultSettleMs
  }

  // listen calls back when a desktop's pipelines may have changed; returns
  // a function that stops listening.
  listen(onChange: (change: DesktopChange) => void): () => void {
    this.#listeners.add(onChange)
    return () => this.#listeners.delete(onChange)
  }

  // isLive reports whether the desktop's stream is open now, so a screen
  // can rely on it and read on its timer less often.
  isLive(desktopID: string): boolean {
    return this.#watched.get(desktopID)?.live ?? false
  }

  // watch keeps a stream open to each desktop given and closes the rest. A
  // desktop already watched at the same address and token is left alone.
  watch(desktops: PairedDesktop[]): void {
    const keep = new Set<string>()
    for (const desktop of desktops) {
      keep.add(desktop.id)
      const key = `${desktop.baseURL}\n${desktop.token}`
      const current = this.#watched.get(desktop.id)
      if (current && current.key === key) {
        current.desktop = desktop
        continue
      }
      if (current) this.#stop(current)
      const w: Watched = {
        desktop, key, controller: null, retry: null, settle: null, attempt: 0, dropped: false, refused: false,
        live: false, changedSessions: new Set(), changedAll: false,
      }
      this.#watched.set(desktop.id, w)
      this.#open(w)
    }
    for (const [id, w] of this.#watched) {
      if (!keep.has(id)) {
        this.#stop(w)
        this.#watched.delete(id)
      }
    }
  }

  // reconnect opens every watched stream again now: back from the
  // background, a stream that looks open may have died with the app
  // suspended, and one waiting to retry should not wait longer.
  reconnect(): void {
    for (const w of this.#watched.values()) {
      if (w.refused) continue
      this.#stop(w)
      w.attempt = 0
      w.dropped = false
      this.#open(w)
    }
  }

  // close ends every stream.
  close(): void {
    for (const w of this.#watched.values()) this.#stop(w)
    this.#watched.clear()
  }

  #open(w: Watched): void {
    const controller = new AbortController()
    w.controller = controller
    const onEvent = (event: FocusStreamEvent) => {
      if (w.controller !== controller) return
      if (event.type === 'connected') {
        w.attempt = 0
        w.live = true
        if (w.dropped) {
          w.dropped = false
          this.#changed(w, null)
        }
      } else if (event.type === 'pipeline') {
        this.#changed(w, event.session_id ?? null)
      }
    }
    this.#api.open(targetOf(w.desktop), onEvent, controller.signal).then(
      () => this.#ended(w, controller, null),
      (err: unknown) => this.#ended(w, controller, err),
    )
  }

  #ended(w: Watched, controller: AbortController, err: unknown): void {
    if (w.controller !== controller || controller.signal.aborted) return
    w.controller = null
    w.live = false
    if (err instanceof APIRequestError && err.status >= 400 && err.status < 500) {
      // 404: a desk too old for the stream; 401/403: the phone is no longer
      // paired there. The 30-second read shows the desk as it is.
      w.refused = true
      return
    }
    const delay = this.#retryMs[Math.min(w.attempt, this.#retryMs.length - 1)]
    w.attempt++
    w.dropped = true
    w.retry = setTimeout(() => {
      w.retry = null
      if (this.#watched.get(w.desktop.id) === w) this.#open(w)
    }, delay)
  }

  // #changed notes what changed — a session, or null for everything — and
  // calls back once the burst settles.
  #changed(w: Watched, sessionID: string | null): void {
    if (sessionID) w.changedSessions.add(sessionID)
    else w.changedAll = true
    if (w.settle) return
    w.settle = setTimeout(() => {
      w.settle = null
      if (this.#watched.get(w.desktop.id) !== w) return
      const change: DesktopChange = { desktopID: w.desktop.id, sessions: [...w.changedSessions].sort(), all: w.changedAll }
      w.changedSessions.clear()
      w.changedAll = false
      for (const listener of this.#listeners) listener(change)
    }, this.#settleMs)
  }

  #stop(w: Watched): void {
    w.controller?.abort()
    w.controller = null
    w.live = false
    w.changedSessions.clear()
    w.changedAll = false
    if (w.retry) clearTimeout(w.retry)
    if (w.settle) clearTimeout(w.settle)
    w.retry = null
    w.settle = null
  }
}
