import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { getHubSkillContent } from '../src/lib/api/extensions.ts'
import { heldUpdates } from '../src/lib/hubUpdate.ts'

const extensionsSource = readFileSync(new URL('../src/components/Extensions.svelte', import.meta.url), 'utf8')

test('getHubSkillContent asks the source the skill was listed under', async () => {
  const urls: string[] = []
  const realFetch = globalThis.fetch
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    urls.push(String(input))
    return new Response(JSON.stringify({ name: 'gh review', version: '0.0.0', content: 'body' }), { status: 200 })
  }) as typeof fetch
  try {
    await getHubSkillContent('gh review', 'openclaw')
  } finally {
    globalThis.fetch = realFetch
  }
  assert.equal(urls.length, 1)
  const url = new URL(urls[0], 'http://console.test')
  assert.equal(url.pathname, '/v1/hub/skill-content')
  assert.equal(url.searchParams.get('name'), 'gh review')
  assert.equal(url.searchParams.get('source'), 'openclaw')
})

test('the Hub tab opens a skill detail per source', () => {
  assert.match(extensionsSource, /getHubSkillContent\(name, hubSource\)/)
  assert.match(extensionsSource, /toggleDetail\('skill', entry\.name, 'hub', sid\)/)
  assert.match(extensionsSource, /\{#if isDetailOpen\('skill', entry\.name, 'hub', sid\)\}/)
})

test('heldUpdates lists packages an update left for the user, not the up-to-date ones', () => {
  const empty = { updated: [], skipped: [], failed: [] }
  const held = heldUpdates({
    skill_update_result: {
      updated: ['a'],
      skipped: [
        { name: 'b', reason: 'up to date' },
        { name: 'gh-review', reason: 'upstream changed; review and apply it with `tars skill install openclaw:gh-review`' },
        { name: 'c', error: 'lookup failed' },
      ],
      failed: [],
    },
    plugin_update_result: empty,
    mcp_update_result: { updated: null, skipped: [{ name: 'm', reason: 'up to date' }], failed: [] },
    focus_template_update_result: { updated: [], skipped: [{ name: 'hotfix', reason: 'edited locally; uninstall and install again to replace your changes' }], failed: [] },
  })
  assert.deepEqual(held, [
    'gh-review: upstream changed; review and apply it with `tars skill install openclaw:gh-review`',
    'c: lookup failed',
    'hotfix: edited locally; uninstall and install again to replace your changes',
  ])
})

test('Update all reports held packages instead of "everything is up to date"', () => {
  assert.match(extensionsSource, /const held = heldUpdates\(result\)/)
  assert.match(extensionsSource, /\$t\.extensions\.updateHeld\(held\.join/)
})

test('a hub-installed skill shows its hub and offers Uninstall, not workspace Edit/Delete', () => {
  assert.match(extensionsSource, /\{#if s\.source === 'workspace' && !isHubInstalled\('skill', s\.name\)\}\s*<button[^\n]*handleEditSkill/)
  assert.match(extensionsSource, /\{#if isHubInstalled\('skill', s\.name\)\}\s*<button[^\n]*handleUninstall\('skill', s\.name\)/)
  assert.match(extensionsSource, /<span class="badge badge-default">\{skillSourceLabel\(s\)\}<\/span>/)
})
