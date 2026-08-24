import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const sessionsSource = readFileSync(new URL('../src/components/Sessions.svelte', import.meta.url), 'utf8')
const sessionSidebarSource = readFileSync(new URL('../src/components/SessionSidebar.svelte', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api/sessions.ts', import.meta.url), 'utf8')
const chatMessagesSource = readFileSync(new URL('../src/lib/chatMessages.ts', import.meta.url), 'utf8')
const chatPanelSource = readFileSync(new URL('../src/components/ChatPanel.svelte', import.meta.url), 'utf8')
const chatMessageItemSource = readFileSync(new URL('../src/components/ChatMessageItem.svelte', import.meta.url), 'utf8')

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

test('highlightTerms escapes HTML before marking OR query terms', async () => {
  // markdown.ts pulls in DOMPurify which needs a DOM at module load time; skip
  // under bare Node since this assertion is covered by the browser-side build.
  if (typeof document === 'undefined') return
  const { highlightTerms } = await import('../src/lib/markdown.ts')
  const html = highlightTerms('[user] Alpha <script>alert(1)</script> beta', ['alpha', 'beta'])
  assert.equal(
    html,
    '[user] <mark>Alpha</mark> &lt;script&gt;alert(1)&lt;/script&gt; <mark>beta</mark>',
  )
})

test('Session and transcript message types expose lineage identifiers', () => {
  assert.match(typesSource, /parent_session_id\?: string/)
  assert.match(typesSource, /root_session_id\?: string/)
  assert.match(typesSource, /forked_from_message_id\?: string/)
  assert.match(typesSource, /forked_from_index\?: number/)
  assert.match(typesSource, /fork_reason\?: string/)
  assert.match(typesSource, /export type SessionMessage = \{[\s\S]*id: string/)
})

test('Chat transcript messages can fork a new session from their persisted message id', () => {
  assert.match(chatMessagesSource, /sourceMessageId\?: string/)
  assert.match(apiSource, /forkSessionFromMessage/)
  assert.match(apiSource, /\/v1\/admin\/sessions\/.*\/fork/)
  assert.match(chatPanelSource, /forkSessionFromMessage/)
  assert.match(chatPanelSource, /handleForkMessage/)
  assert.match(chatPanelSource, /sourceMessageId:\s*msg\.id/)
  assert.match(chatMessageItemSource, /onForkMessage/)
  assert.match(chatMessageItemSource, /forkFromHere/)
})
