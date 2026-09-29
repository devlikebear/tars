import { requestJSON } from './client.ts'
import type { ActivitySnapshot, BoardResponse } from '../sessionBoard.ts'

// --- Session board and live activity (#971) ---

// Every visible chat session with its status, repository, branch, latest
// change, and this month's cost.
export function getSessionBoard(): Promise<BoardResponse> {
  return requestJSON<BoardResponse>('/v1/chat/board')
}

// Turns running now and tool approvals waiting, across all sessions.
export function getChatActivity(): Promise<ActivitySnapshot> {
  return requestJSON<ActivitySnapshot>('/v1/chat/activity')
}
