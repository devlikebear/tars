// Live activity of every chat session, for everything outside the session
// being streamed: sidebar badges, the session board, and browser
// notifications when a background session needs input or finishes (#971).
//
// It polls GET /v1/chat/activity (cheap: two in-memory maps on the server)
// and polls again at once when the event stream announces an approval. What
// has been looked at is remembered per browser in localStorage.

import {
  activityChanges,
  activityMap,
  seenBaselineKey,
  type ActivityChange,
  type ActivityMap,
  type ActivitySnapshot,
  type Seen,
} from '../sessionBoard.ts'

export type SessionActivityApi = {
  getChatActivity: () => Promise<ActivitySnapshot>
}

// The parts of the browser the store uses, injected so tests can run it
// under Node.
export type SessionActivityEnv = {
  now: () => number
  storage?: Pick<Storage, 'getItem' | 'setItem'> | null
  // The browser Notification API, or null where there is none.
  notifications?: {
    permission: () => NotificationPermission
    request: () => Promise<NotificationPermission>
    show: (title: string, body: string, tag: string, onClick: () => void) => void
  } | null
  // Whether the page is in front of the user.
  visible: () => boolean
}

const seenKey = 'tars.sessionBoard.seen'
const notifyKey = 'tars.sessionBoard.notify'

export type NotificationText = {
  needsInput: (title: string) => string
  done: (title: string) => string
  body: { needsInput: string; done: string }
}

export class SessionActivityStore {
  activity = $state<ActivityMap>({})
  seen = $state<Seen>({})
  // Whether the user turned browser notifications on here.
  notifyEnabled = $state(false)
  permission = $state<NotificationPermission | 'unsupported'>('unsupported')
  // The session the user is looking at, which never notifies.
  viewing = $state<string | null>(null)
  // Bumped after every successful poll so the board can refresh with it.
  version = $state(0)

  private api: SessionActivityApi
  private env: SessionActivityEnv
  private timer: ReturnType<typeof setInterval> | null = null
  private polled = false
  private inflight: Promise<void> | null = null
  text: NotificationText | null = null
  onOpenSession: ((id: string) => void) | null = null

  constructor(api: SessionActivityApi, env: SessionActivityEnv) {
    this.api = api
    this.env = env
    this.seen = this.loadSeen()
    this.notifyEnabled = this.read(notifyKey) === '1'
    this.permission = env.notifications ? env.notifications.permission() : 'unsupported'
  }

  private read(key: string): string | null {
    try {
      return this.env.storage?.getItem(key) ?? null
    } catch {
      return null
    }
  }

  private write(key: string, value: string): void {
    try {
      this.env.storage?.setItem(key, value)
    } catch {
      // Private windows and blocked storage: keep it in memory only.
    }
  }

  private loadSeen(): Seen {
    let seen: Seen = {}
    try {
      const parsed = JSON.parse(this.read(seenKey) ?? '{}')
      if (parsed && typeof parsed === 'object') seen = parsed as Seen
    } catch {
      seen = {}
    }
    if (seen[seenBaselineKey] === undefined) {
      seen = { ...seen, [seenBaselineKey]: this.env.now() }
      this.write(seenKey, JSON.stringify(seen))
    }
    return seen
  }

  // markSeen records that the user looked at a session now.
  markSeen(id: string | null | undefined): void {
    if (!id) return
    this.seen = { ...this.seen, [id]: this.env.now() }
    this.write(seenKey, JSON.stringify(this.seen))
  }

  // setViewing tells the store which session is on screen; it counts as
  // seen for as long as it is.
  setViewing(id: string | null): void {
    this.viewing = id
    this.markSeen(id)
  }

  running(id: string): boolean {
    return Boolean(this.activity[id]?.running)
  }

  pending(id: string): number {
    return this.activity[id]?.pending ?? 0
  }

  async poll(): Promise<void> {
    if (this.inflight) return this.inflight
    this.inflight = (async () => {
      try {
        const next = activityMap(await this.api.getChatActivity())
        const changes = this.polled ? activityChanges(this.activity, next) : []
        this.activity = next
        this.polled = true
        this.version++
        if (this.viewing && !next[this.viewing] && this.env.visible()) this.markSeen(this.viewing)
        for (const change of changes) this.announce(change)
      } catch {
        // The server is down or the user signed out; keep the last state.
      } finally {
        this.inflight = null
      }
    })()
    return this.inflight
  }

  start(everyMs = 4000): void {
    if (this.timer) return
    void this.poll()
    this.timer = setInterval(() => void this.poll(), everyMs)
  }

  stop(): void {
    if (this.timer) clearInterval(this.timer)
    this.timer = null
  }

  async enableNotifications(): Promise<boolean> {
    const n = this.env.notifications
    if (!n) return false
    let permission = n.permission()
    if (permission === 'default') permission = await n.request()
    this.permission = permission
    this.notifyEnabled = permission === 'granted'
    this.write(notifyKey, this.notifyEnabled ? '1' : '0')
    return this.notifyEnabled
  }

  disableNotifications(): void {
    this.notifyEnabled = false
    this.write(notifyKey, '0')
  }

  private announce(change: ActivityChange): void {
    const n = this.env.notifications
    if (!n || !this.notifyEnabled || !this.text || n.permission() !== 'granted') return
    // The user is already looking at it.
    if (change.sessionId === this.viewing && this.env.visible()) return
    const title = change.title || change.sessionId
    const heading = change.kind === 'needs_input' ? this.text.needsInput(title) : this.text.done(title)
    const body = change.kind === 'needs_input' ? this.text.body.needsInput : this.text.body.done
    n.show(heading, body, `tars-${change.kind}-${change.sessionId}`, () => this.onOpenSession?.(change.sessionId))
  }
}
