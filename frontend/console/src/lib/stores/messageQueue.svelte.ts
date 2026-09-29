// Follow-up messages for a session whose turn is still running (#971).
//
// While a turn runs, the composer queues instead of sending. When the turn
// ends on its own, the chat sends the next queued message. Queues are per
// session and live in memory for the page's lifetime; files and mentions
// ride along with the text so a queued message sends exactly as typed.
//
// A queue pauses when there is reason to stop and look: the user pressed
// Stop, the turn failed, or the chat was reopened with messages still
// queued (the turn they were waiting on may have ended unseen). A paused
// queue sends nothing until resumed.

export type QueuedMessage<F = unknown, M = unknown> = {
  id: string
  text: string
  files: F[]
  mentions: M[]
}

export type SessionQueue<F = unknown, M = unknown> = {
  items: QueuedMessage<F, M>[]
  paused: boolean
}

export class MessageQueueStore<F = unknown, M = unknown> {
  queues = $state<Record<string, SessionQueue<F, M>>>({})
  private seq = 0

  private get(sessionId: string): SessionQueue<F, M> {
    return this.queues[sessionId] ?? { items: [], paused: false }
  }

  private put(sessionId: string, queue: SessionQueue<F, M>): void {
    if (queue.items.length === 0) {
      const { [sessionId]: _dropped, ...rest } = this.queues
      this.queues = rest
      return
    }
    this.queues = { ...this.queues, [sessionId]: queue }
  }

  items(sessionId: string): QueuedMessage<F, M>[] {
    return this.get(sessionId).items
  }

  paused(sessionId: string): boolean {
    return this.get(sessionId).paused
  }

  enqueue(sessionId: string, text: string, files: F[] = [], mentions: M[] = []): QueuedMessage<F, M> | null {
    const trimmed = text.trim()
    if (!sessionId || !trimmed) return null
    this.seq++
    const item: QueuedMessage<F, M> = { id: `q${this.seq}`, text: trimmed, files: [...files], mentions: [...mentions] }
    const queue = this.get(sessionId)
    this.put(sessionId, { ...queue, items: [...queue.items, item] })
    return item
  }

  remove(sessionId: string, id: string): QueuedMessage<F, M> | null {
    const queue = this.get(sessionId)
    const item = queue.items.find((q) => q.id === id) ?? null
    if (item) this.put(sessionId, { ...queue, items: queue.items.filter((q) => q.id !== id) })
    return item
  }

  update(sessionId: string, id: string, text: string): void {
    const trimmed = text.trim()
    if (!trimmed) {
      this.remove(sessionId, id)
      return
    }
    const queue = this.get(sessionId)
    this.put(sessionId, { ...queue, items: queue.items.map((q) => (q.id === id ? { ...q, text: trimmed } : q)) })
  }

  // moveToFront puts a message next in line, for "send now".
  moveToFront(sessionId: string, id: string): void {
    const queue = this.get(sessionId)
    const item = queue.items.find((q) => q.id === id)
    if (!item) return
    this.put(sessionId, { ...queue, items: [item, ...queue.items.filter((q) => q.id !== id)] })
  }

  // take removes and returns the next message, unless the queue is paused.
  take(sessionId: string): QueuedMessage<F, M> | null {
    const queue = this.get(sessionId)
    if (queue.paused || queue.items.length === 0) return null
    const [next, ...rest] = queue.items
    this.put(sessionId, { ...queue, items: rest })
    return next
  }

  pause(sessionId: string): void {
    const queue = this.get(sessionId)
    if (queue.items.length) this.put(sessionId, { ...queue, paused: true })
  }

  resume(sessionId: string): void {
    const queue = this.get(sessionId)
    if (queue.items.length) this.put(sessionId, { ...queue, paused: false })
  }

  clear(sessionId: string): void {
    this.put(sessionId, { items: [], paused: false })
  }

  // rekey moves a queue to a new key: a new chat's queue is kept under a
  // placeholder until the server assigns the session its ID.
  rekey(from: string, to: string): void {
    if (!from || !to || from === to) return
    const moving = this.get(from)
    if (moving.items.length === 0) return
    const target = this.get(to)
    const { [from]: _moved, ...rest } = this.queues
    this.queues = { ...rest, [to]: { items: [...target.items, ...moving.items], paused: target.paused || moving.paused } }
  }
}

export const messageQueue = new MessageQueueStore()
