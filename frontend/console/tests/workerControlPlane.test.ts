import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { getWorkerControlPlane } from '../src/lib/api/ops.ts'
import {
  controlEventPresentation,
  placementStateBadge,
  workerStateBadge,
} from '../src/lib/workerControlPlane.ts'

const opsSource = readFileSync(new URL('../src/components/Ops.svelte', import.meta.url), 'utf8')
const componentSource = readFileSync(new URL('../src/components/RemoteWorkers.svelte', import.meta.url), 'utf8')

test('remote worker control plane maps health and recovery states for operators', () => {
  assert.equal(workerStateBadge('ready'), 'badge-success')
  assert.equal(workerStateBadge('lost'), 'badge-error')
  assert.equal(placementStateBadge('rehydrating'), 'badge-warning')
  assert.equal(placementStateBadge('completed'), 'badge-success')

  assert.deepEqual(controlEventPresentation({
    id: 'event-1',
    type: 'rehydrate',
    entity: 'placement',
    worker_id: 'worker-b',
    placement_id: 'placement-a',
    sequence: 9,
    from_state: 'lost',
    to_state: 'rehydrating',
    published: true,
    occurred_at: '2026-08-02T10:00:00Z',
  }), {
    title: 'lost → rehydrating',
    detail: 'rehydrate · placement-a · worker-b',
  })
})

test('remote worker control plane API and Ops surface expose only sanitized views', async () => {
  const originalFetch = globalThis.fetch
  const requests: string[] = []
  globalThis.fetch = async (input) => {
    requests.push(String(input))
    return new Response(JSON.stringify({
      enabled: true,
      protocol_version: '1.0',
      a2a: { enabled: true, adapter: 'a2a-http-json', protocol_version: '1.0' },
      summary: {
        workers: 1,
        ready_workers: 1,
        lost_workers: 0,
        placements: 1,
        active_placements: 1,
        recovering_placements: 0,
        recovery_count: 0,
      },
      workers: [],
      placements: [],
      events: [],
    }), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }

  try {
    const result = await getWorkerControlPlane()
    assert.equal(result.a2a.enabled, true)
    assert.deepEqual(requests, ['/v1/admin/workers'])
  } finally {
    globalThis.fetch = originalFetch
  }

  assert.match(opsSource, /<RemoteWorkers/)
  assert.match(componentSource, /summary\.recovering_placements/)
  assert.match(componentSource, /sync\.file_count/)
  assert.match(componentSource, /checkpoint/)
  assert.doesNotMatch(componentSource, /worker\.endpoint/)
  assert.doesNotMatch(componentSource, /event\.payload/)
})
