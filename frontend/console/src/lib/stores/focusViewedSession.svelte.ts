// The session(s) the focus pipeline screen currently has on screen
// (tars#1247) — the pipeline's own session, and its hidden Q&A session
// (lib/stores/focusStore.svelte.ts's `qa_session_id`) while the drawer is
// following a question. App.svelte reads this (only while
// route.view === 'focus') the same way it reads the chat route's own
// active session, so CASE leaves both out of its lines/cues: the pipeline
// deck already shows the pipeline session's progress/gate state, and the
// Q&A drawer already shows "answering" for its own turn.
//
// FocusPipeline.svelte is the only writer: it sets this on mount and
// whenever the pipeline/Q&A session changes, and clears it on unmount —
// leaving the screen must not leave a stale exclusion behind.

export class FocusViewedSessionStore {
  sessionId = $state<string | null>(null)
  qaSessionId = $state<string | null>(null)

  set(sessionId: string | null, qaSessionId: string | null = null): void {
    this.sessionId = sessionId
    this.qaSessionId = qaSessionId
  }

  clear(): void {
    this.sessionId = null
    this.qaSessionId = null
  }
}

export const focusViewedSession = new FocusViewedSessionStore()
