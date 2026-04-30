import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { highlightTerms } from '../src/lib/markdown.ts'

const sessionsSource = readFileSync(new URL('../src/components/Sessions.svelte', import.meta.url), 'utf8')
const sessionSidebarSource = readFileSync(new URL('../src/components/SessionSidebar.svelte', import.meta.url), 'utf8')

test('Sessions search reuses memory session search for transcript snippets', () => {
  assert.match(sessionsSource, /runMemorySearch/)
  assert.match(sessionsSource, /include_memory:\s*false/)
  assert.match(sessionsSource, /include_daily:\s*false/)
  assert.match(sessionsSource, /include_sessions:\s*true/)
  assert.match(sessionsSource, /source\.startsWith\('session:'\)/)
  assert.match(sessionsSource, /sessionSearchSnippets/)
  assert.match(sessionsSource, /session-snippet-list/)
})

test('Chat session sidebar exposes transcript snippets in the active session picker', () => {
  assert.match(sessionSidebarSource, /runMemorySearch/)
  assert.match(sessionSidebarSource, /include_memory:\s*false/)
  assert.match(sessionSidebarSource, /include_daily:\s*false/)
  assert.match(sessionSidebarSource, /include_sessions:\s*true/)
  assert.match(sessionSidebarSource, /source\.startsWith\('session:'\)/)
  assert.match(sessionSidebarSource, /sessionSearchSnippets/)
  assert.match(sessionSidebarSource, /sidebar-snippet-list/)
})

test('highlightTerms escapes HTML before marking OR query terms', () => {
  const html = highlightTerms('[user] Alpha <script>alert(1)</script> beta', ['alpha', 'beta'])
  assert.equal(
    html,
    '[user] <mark>Alpha</mark> &lt;script&gt;alert(1)&lt;/script&gt; <mark>beta</mark>',
  )
})
