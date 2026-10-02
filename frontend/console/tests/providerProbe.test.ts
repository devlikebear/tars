import { test } from 'node:test'
import assert from 'node:assert/strict'
import { describeProviderProbe } from '../src/lib/providerProbe.ts'
import { providerTestEn, providerTestKo } from '../src/i18n/sections/providerTest.ts'

test('a signed-in claude-code-cli reads as OK with its sign-in and version, not as an error', () => {
  const row = describeProviderProbe(
    { alias: 'claude', kind: 'claude-code-cli', default: true, status: 'ok', version: '2.1.283', auth_method: 'claude.ai', cli_path: '/usr/local/bin/claude' },
    providerTestEn,
  )
  assert.equal(row.tone, 'success')
  assert.equal(row.statusLabel, 'OK')
  assert.equal(row.isDefault, true)
  assert.equal(row.summary, 'Signed in (claude.ai) · CLI 2.1.283')
  assert.equal(row.detail, '/usr/local/bin/claude')
})

test('an unverifiable CLI sign-in is informational, a missing sign-in is an error', () => {
  const unknown = describeProviderProbe(
    { alias: 'claude', kind: 'claude-code-cli', default: false, status: 'info', problem: 'auth_unknown', cli_path: '/bin/claude' },
    providerTestEn,
  )
  assert.equal(unknown.tone, 'info')
  assert.match(unknown.summary, /was not checked/)

  const loggedOut = describeProviderProbe(
    { alias: 'claude', kind: 'claude-code-cli', default: false, status: 'error', problem: 'not_logged_in' },
    providerTestKo,
  )
  assert.equal(loggedOut.tone, 'error')
  assert.equal(loggedOut.statusLabel, '실패')
  assert.match(loggedOut.summary, /로그인/)
})

test('an old agy names its version and the floor', () => {
  const row = describeProviderProbe(
    { alias: 'agy', kind: 'antigravity-cli', default: false, status: 'warn', problem: 'version_old', version: '1.1.9', min_version: '1.1.12', model_count: 3 },
    providerTestEn,
  )
  assert.equal(row.tone, 'warning')
  assert.match(row.summary, /agy 1\.1\.9 is older than 1\.1\.12/)
})

test('HTTP providers report their model count, and failures keep the provider error as detail', () => {
  const ok = describeProviderProbe({ alias: 'codex', kind: 'openai-codex', default: false, status: 'ok', model_count: 7 }, providerTestEn)
  assert.equal(ok.summary, '7 models available')
  assert.equal(ok.detail, '')

  const failed = describeProviderProbe(
    { alias: 'codex', kind: 'openai-codex', default: false, status: 'error', problem: 'models_failed', detail: 'openai-codex: 401 unauthorized' },
    providerTestEn,
  )
  assert.equal(failed.summary, 'Model listing failed.')
  assert.equal(failed.detail, 'openai-codex: 401 unauthorized')
})
