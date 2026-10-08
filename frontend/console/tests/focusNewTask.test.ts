import test from 'node:test'
import assert from 'node:assert/strict'

import { createFocusPipeline } from '../src/lib/api/focus.ts'

// FocusNewTask.svelte sends extra_dirs (more folders the session can
// reach) alongside the primary cwd, which stays the only active/isolated
// one. This checks the request shape createFocusPipeline actually sends,
// without a folder the component only sends what it was given.
test('createFocusPipeline sends extra_dirs alongside cwd when the task has more folders', async () => {
  const originalFetch = globalThis.fetch
  const requests: Array<{ url: string; body: unknown }> = []
  globalThis.fetch = async (input, init) => {
    requests.push({ url: String(input), body: init?.body ? JSON.parse(String(init.body)) : undefined })
    return new Response(JSON.stringify({ session_id: 's1', pipeline: {} }), {
      status: 201,
      headers: { 'Content-Type': 'application/json' },
    })
  }
  try {
    await createFocusPipeline({
      goal: 'g',
      cwd: '/repo',
      isolate: true,
      extra_dirs: ['/repo', '/other', '/other', '/third'],
    })
  } finally {
    globalThis.fetch = originalFetch
  }

  assert.equal(requests.length, 1)
  assert.match(requests[0].url, /\/v1\/focus\/pipelines$/)
  // The client sends exactly what it was given; de-duplication against the
  // primary and the cap are the server's job (chat_new_session.go).
  assert.deepEqual(requests[0].body, {
    goal: 'g',
    cwd: '/repo',
    isolate: true,
    extra_dirs: ['/repo', '/other', '/other', '/third'],
  })
})

test('createFocusPipeline omits extra_dirs when the task has no extra folders', async () => {
  const originalFetch = globalThis.fetch
  const requests: Array<{ body: unknown }> = []
  globalThis.fetch = async (_input, init) => {
    requests.push({ body: init?.body ? JSON.parse(String(init.body)) : undefined })
    return new Response(JSON.stringify({ session_id: 's1', pipeline: {} }), {
      status: 201,
      headers: { 'Content-Type': 'application/json' },
    })
  }
  try {
    await createFocusPipeline({ goal: 'g', cwd: '/repo' })
  } finally {
    globalThis.fetch = originalFetch
  }

  assert.deepEqual(requests[0].body, { goal: 'g', cwd: '/repo' })
})
