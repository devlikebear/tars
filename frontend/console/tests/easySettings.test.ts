import test from 'node:test'
import assert from 'node:assert/strict'

import { chatTier, phoneAccess, phoneAccessBlocker, restarted, thinkingChoices, thinkingUpdate } from '../src/lib/easySettings.ts'
import type { RemoteAccessResponse } from '../src/lib/types.ts'

const easySetup = {
  llm_providers: { claude: { kind: 'claude-code-cli', auth_mode: 'cli' } },
  llm_tiers: {
    light: { provider: 'claude', model: 'haiku' },
    custom: { provider: 'claude', model: 'claude-opus-5-5' },
    heavy: { provider: 'claude', model: 'opus' },
    standard: { provider: 'claude', model: 'sonnet' },
    broken: { provider: 'claude' },
  },
}

test('the choices are the tiers with a model, the three known ones first', () => {
  assert.deepEqual(
    thinkingChoices(easySetup).map((c) => [c.tier, c.model, c.kind]),
    [
      ['heavy', 'opus', 'claude-code-cli'],
      ['standard', 'sonnet', 'claude-code-cli'],
      ['light', 'haiku', 'claude-code-cli'],
      ['custom', 'claude-opus-5-5', 'claude-code-cli'],
    ],
  )
  assert.deepEqual(thinkingChoices({}), [])
  assert.deepEqual(thinkingChoices({ llm_tiers: 'nope' }), [])
})

test('the team runs on the chat role tier, else the default tier, else standard', () => {
  assert.equal(chatTier({}), 'standard')
  assert.equal(chatTier({ llm_default_tier: 'light' }), 'light')
  assert.equal(chatTier({ llm_default_tier: 'light', llm_role_defaults: { chat_main: 'heavy' } }), 'heavy')
})

test('moving the team writes the default tier, or the chat role when it names one', () => {
  assert.deepEqual(thinkingUpdate({ llm_default_tier: 'standard' }, 'heavy'), { llm_default_tier: 'heavy' })
  assert.deepEqual(thinkingUpdate({ llm_role_defaults: { chat_main: 'standard', pulse_decider: 'light' } }, 'heavy'), {
    llm_role_defaults: { chat_main: 'heavy', pulse_decider: 'light' },
  })
  assert.deepEqual(thinkingUpdate({ llm_role_defaults: { pulse_decider: 'light' } }, 'heavy'), { llm_default_tier: 'heavy' })
})

const remote = (status: RemoteAccessResponse['status'], failed: string[] = []): RemoteAccessResponse => ({
  desired_enabled: false,
  desired_https_port: 443,
  target_url: '',
  status,
  checks: ['api_auth_required', 'admin_password_configured', 'user_password_configured', 'tailscale_installed', 'tailscale_logged_in', 'serve_target_available'].map(
    (key) => ({ key, ok: !failed.includes(key), message: '' }),
  ),
})

test('phone access in plain states', () => {
  assert.equal(phoneAccess(null), 'checking')
  assert.equal(phoneAccess(remote({})), 'noTailscale')
  assert.equal(phoneAccess(remote({ installed: true })), 'signedOut')
  assert.equal(phoneAccess(remote({ installed: true, logged_in: true })), 'off')
  assert.equal(phoneAccess(remote({ installed: true, logged_in: true, serve_active: true })), 'portTaken')
  assert.equal(phoneAccess(remote({ Installed: true, LoggedIn: true, ServeActive: true, OwnedByTARS: true })), 'on')
  // Renamed in Tailscale: the handler sits under the old name only.
  assert.equal(phoneAccess(remote({ installed: true, logged_in: true, stale_host: 'chang-macbookpro.tailnet.ts.net' })), 'renamed')
  // Another target under the current name still reads as taken.
  assert.equal(phoneAccess(remote({ installed: true, logged_in: true, serve_active: true, stale_host: 'old.tailnet.ts.net' })), 'portTaken')
})

test('what blocks turning phone access on, in the order to fix it', () => {
  assert.equal(phoneAccessBlocker(null), null)
  assert.equal(phoneAccessBlocker(remote({})), null)
  assert.equal(phoneAccessBlocker(remote({}, ['api_auth_required', 'tailscale_logged_in'])), 'signedOut')
  assert.equal(phoneAccessBlocker(remote({}, ['user_password_configured', 'tailscale_installed'])), 'noTailscale')
  assert.equal(phoneAccessBlocker(remote({}, ['serve_target_available', 'api_auth_required'])), 'portTaken')
  assert.equal(phoneAccessBlocker(remote({}, ['api_auth_required'])), 'needsPassword')
  assert.equal(phoneAccessBlocker(remote({}, ['admin_password_configured'])), 'needsPassword')
})

test('a restart is seen as a new start time or process', () => {
  assert.equal(restarted({ runtime_started_at: 'a' }, { runtime_started_at: 'a' }), false)
  assert.equal(restarted({ runtime_started_at: 'a' }, { runtime_started_at: 'b' }), true)
  assert.equal(restarted({ process_id: 1 }, { process_id: 2 }), true)
  assert.equal(restarted({}, {}), false)
})
