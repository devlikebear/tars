import { requestJSON } from './client.ts'

// --- CASE's Bridge (case-companion phase 2, docs/decisions/case-bridge.md) ---
//
// A question runs as a turn in CASE's own hidden session; the console
// streams it from GET /v1/chat/stream like any turn, then reads the answer
// without its card block and the cards the server checked.

export type CaseBridgeScope = 'all' | 'screen'

export type CaseBridgeView = {
  route: string
  session_id?: string
  title?: string
}

export type CaseBridgeCardKind = 'focus' | 'session' | 'cron' | 'approvals' | 'usage' | 'pulse'

export type CaseBridgeCard = {
  kind: CaseBridgeCardKind
  id?: string
  title: string
  sub?: string
  severity: 'block' | 'info'
  href: string
}

export type CaseBridgeAnswer = {
  session_id: string
  busy: boolean
  question: string
  answer: string
  cards: CaseBridgeCard[]
  error?: string
}

export function askCaseBridge(question: string, scope: CaseBridgeScope, view: CaseBridgeView): Promise<{ session_id: string }> {
  return requestJSON('/v1/case/ask', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ question, scope, view }),
  })
}

export function getCaseBridgeAnswer(): Promise<CaseBridgeAnswer> {
  return requestJSON<CaseBridgeAnswer>('/v1/case/answer')
}
