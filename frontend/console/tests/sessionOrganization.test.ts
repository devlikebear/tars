import test from 'node:test'
import assert from 'node:assert/strict'

import {
  cleanupCandidateSessions,
  organizeSessions,
  sessionKind,
} from '../src/lib/sessionOrganization.ts'
import type { Session } from '../src/lib/types.ts'

function makeSession(id: string, overrides: Partial<Session> = {}): Session {
  return {
    id,
    title: id,
    created_at: '2026-05-01T00:00:00Z',
    updated_at: '2026-05-01T00:00:00Z',
    ...overrides,
  }
}

test('organizeSessions hides archived sessions by default and supports archived filter', () => {
  const active = makeSession('active', { updated_at: '2026-05-03T00:00:00Z' })
  const archived = makeSession('archived', {
    title: 'Archived cleanup notes',
    archived_at: '2026-05-04T00:00:00Z',
    updated_at: '2026-05-04T00:00:00Z',
  })
  const otherArchived = makeSession('other-archived', {
    title: 'Archived release notes',
    archived_at: '2026-05-04T00:00:00Z',
    updated_at: '2026-05-04T00:00:00Z',
  })

  assert.deepEqual(
    organizeSessions([archived, otherArchived, active], { filterKind: 'all', sortBy: 'updated' }).map((s) => s.id),
    ['active'],
  )
  assert.deepEqual(
    organizeSessions([archived, active], { filterKind: 'archived', sortBy: 'updated', query: 'cleanup' }).map((s) => s.id),
    ['archived'],
  )
})

test('organizeSessions keeps pinned sessions above ordinary recent sessions', () => {
  const olderPinned = makeSession('older-pinned', {
    pinned_at: '2026-05-02T00:00:00Z',
    updated_at: '2026-05-01T00:00:00Z',
  })
  const newer = makeSession('newer', { updated_at: '2026-05-04T00:00:00Z' })

  assert.deepEqual(
    organizeSessions([newer, olderPinned], { filterKind: 'all', sortBy: 'updated' }).map((s) => s.id),
    ['older-pinned', 'newer'],
  )
})

test('cleanupCandidateSessions excludes pinned archived main and worker sessions', () => {
  const staleNewChat = makeSession('stale-new-chat', {
    title: 'New Chat',
    updated_at: '2026-04-01T00:00:00Z',
  })
  const pinned = makeSession('pinned-new-chat', {
    title: 'New Chat',
    pinned_at: '2026-05-02T00:00:00Z',
    updated_at: '2026-04-01T00:00:00Z',
  })
  const archived = makeSession('archived-new-chat', {
    title: 'New Chat',
    archived_at: '2026-05-02T00:00:00Z',
    updated_at: '2026-04-01T00:00:00Z',
  })
  const main = makeSession('main', { kind: 'main', title: 'main', updated_at: '2026-04-01T00:00:00Z' })
  const worker = makeSession('worker', { hidden: true, title: 'worker', updated_at: '2026-04-01T00:00:00Z' })

  assert.equal(sessionKind(worker), 'worker')
  assert.deepEqual(
    cleanupCandidateSessions([staleNewChat, pinned, archived, main, worker], new Date('2026-05-09T00:00:00Z')).map((s) => s.id),
    ['stale-new-chat'],
  )
})
