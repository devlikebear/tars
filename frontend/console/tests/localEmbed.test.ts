import test from 'node:test'
import assert from 'node:assert/strict'

import { awaitingServer, canInstall, canReindex, foreignServer, formatBytes, jobPercent, jobRunning } from '../src/lib/localEmbed.ts'
import type { LocalEmbedState } from '../src/lib/types.ts'

function state(over: Partial<LocalEmbedState> = {}, status: Partial<LocalEmbedState['status']> = {}): LocalEmbedState {
  return {
    enabled: false,
    restart_required: false,
    reindex_pending: 0,
    ...over,
    status: {
      model: 'kakaocorp/kanana-2-1.3b-embedding',
      base_url: 'http://127.0.0.1:43181/v1',
      dir: '/w/_shared/local-embed',
      env_ready: false,
      model_installed: false,
      model_bytes: 0,
      running: false,
      ...status,
    },
  }
}

test('formatBytes', () => {
  assert.equal(formatBytes(12), '12 B')
  assert.equal(formatBytes(2048), '2.0 KB')
  assert.equal(formatBytes(2582995040), '2.4 GB')
})

test('canInstall needs uv unless the environment exists, and no running job', () => {
  assert.equal(canInstall(null), false)
  assert.equal(canInstall(state()), false)
  assert.equal(canInstall(state({}, { uv_path: '/bin/uv' })), true)
  assert.equal(canInstall(state({}, { env_ready: true })), true)
  assert.equal(canInstall(state({ job: { action: 'install', running: true } }, { uv_path: '/bin/uv' })), false)
})

test('canReindex needs a backlog, a running server and no pending restart', () => {
  assert.equal(canReindex(state({ reindex_pending: 3 })), false)
  assert.equal(canReindex(state({ reindex_pending: 3 }, { running: true })), true)
  assert.equal(canReindex(state({ reindex_pending: 0 }, { running: true })), false)
  assert.equal(canReindex(state({ reindex_pending: 3, restart_required: true }, { running: true })), false)
  assert.equal(canReindex(state({ reindex_pending: 3, job: { action: 'reindex', running: true } }, { running: true })), false)
})

test('foreignServer names a server that is not ours', () => {
  assert.equal(foreignServer(state({}, { running_model: 'someone/else' })), 'someone/else')
  assert.equal(foreignServer(state({}, { running: true, running_model: 'x/y' })), '')
  assert.equal(foreignServer(null), '')
})

test('jobPercent and jobRunning', () => {
  assert.equal(jobPercent(state({ job: { action: 'reindex', running: true, done: 1, total: 4 } })), 25)
  assert.equal(jobPercent(state({ job: { action: 'install', running: true } })), null)
  assert.equal(jobRunning(state({ job: { action: 'install', running: false } })), false)
})

test('awaitingServer is true only while an enabled, installed model is loading', () => {
  assert.equal(awaitingServer(state({ enabled: true }, { model_installed: true })), true)
  assert.equal(awaitingServer(state({ enabled: true }, { model_installed: true, running: true })), false)
  assert.equal(awaitingServer(state({ enabled: false }, { model_installed: true })), false)
  assert.equal(awaitingServer(state({ enabled: true })), false)
  assert.equal(awaitingServer(state({ enabled: true, restart_required: true }, { model_installed: true })), false)
  assert.equal(awaitingServer(state({ enabled: true }, { model_installed: true, running_model: 'x/y' })), false)
})
