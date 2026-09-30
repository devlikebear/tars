import { test } from 'node:test'
import assert from 'node:assert/strict'
import { pendingToolApprovals } from '../src/lib/unattendedApprovals.ts'
import type { Approval } from '../src/lib/types.ts'

function approval(id: string, over: Partial<Approval>): Approval {
  return { id, type: 'tool_permission', status: 'pending', requested_at: '2026-09-29T10:00:00Z', updated_at: '', ...over }
}

test('picks the pending tool questions of one session, oldest first', () => {
  const items: Approval[] = [
    approval('late', { requested_at: '2026-09-29T10:05:00Z', tool_permission: { session_id: 's1', source: 'cron', tool_name: 'exec' } }),
    approval('early', { tool_permission: { session_id: 's1', source: 'telegram', tool_name: 'write_file' } }),
    approval('other', { tool_permission: { session_id: 's2', source: 'cron', tool_name: 'exec' } }),
    approval('done', { status: 'approved', tool_permission: { session_id: 's1', source: 'cron', tool_name: 'exec' } }),
    approval('cleanup', { type: 'cleanup' }),
  ]
  assert.deepEqual(pendingToolApprovals(items, ' s1 ').map((p) => p.id), ['early', 'late'])
  assert.equal(pendingToolApprovals(items, 's1')[0].request.tool_name, 'write_file')
})

test('no session, no questions', () => {
  assert.deepEqual(pendingToolApprovals([approval('a', { tool_permission: { session_id: '', source: 'cron', tool_name: 'exec' } })], ''), [])
})
