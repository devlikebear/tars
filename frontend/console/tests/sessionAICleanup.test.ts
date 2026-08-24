import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const sidebarSource = readFileSync(new URL('../src/components/SessionSidebar.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api/sessions.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('session cleanup API exposes AI archive and delete suggestion modes', () => {
  assert.match(typesSource, /export type SessionCleanupMode = 'archive' \| 'delete'/)
  assert.match(typesSource, /export type SessionCleanupSuggestion = \{[\s\S]*session_id: string/)
  assert.match(typesSource, /export type SessionCleanupSuggestionResponse = \{[\s\S]*suggestions: SessionCleanupSuggestion\[\]/)
  assert.match(apiSource, /recommendSessionCleanup/)
  assert.match(apiSource, /\/v1\/admin\/sessions\/cleanup-suggestions/)
})

test('session sidebar routes AI suggestions to archive for active sessions and delete for archived sessions', () => {
  assert.match(sidebarSource, /aiCleanupMode/)
  assert.match(sidebarSource, /function sessionCleanupModeForFilter/)
  assert.match(sidebarSource, /kind === 'archived' \? 'delete' : 'archive'/)
  assert.match(sidebarSource, /recommendSessionCleanup\(\{ mode: aiCleanupMode/)
  assert.match(sidebarSource, /setSessionArchived\(suggestion\.session_id, true\)/)
  assert.match(sidebarSource, /deleteSession\(suggestion\.session_id\)/)
  assert.match(sidebarSource, /ai-delete-confirm/)
})
