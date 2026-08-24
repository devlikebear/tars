import { requestJSON } from './client.ts'
import type { TelegramAllowedUser, TelegramPairingsResponse } from '../types'

// --- Channels / Telegram ---

export async function getTelegramPairings(): Promise<TelegramPairingsResponse> {
  return requestJSON<TelegramPairingsResponse>('/v1/channels/telegram/pairings')
}

export async function approveTelegramPairing(code: string): Promise<{ approved: TelegramAllowedUser }> {
  return requestJSON<{ approved: TelegramAllowedUser }>('/v1/channels/telegram/pairings/approve', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code: code.trim() }),
  })
}

export async function revokeTelegramPairing(userId: number): Promise<{ revoked: boolean }> {
  return requestJSON<{ revoked: boolean }>('/v1/channels/telegram/pairings/revoke', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user_id: userId }),
  })
}
