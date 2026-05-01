import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { resolveRoute } from '../src/lib/router.ts'
import { buildSessionLineageRows, forkPreviewFromHistory } from '../src/lib/sessionLineage.ts'
import type { Session, SessionMessage } from '../src/lib/types.ts'

const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')
const graphSource = readFileSync(new URL('../src/components/SessionLineageGraph.svelte', import.meta.url), 'utf8')

const rootSession: Session = {
  id: 'root',
  title: 'Root',
  root_session_id: 'root',
  created_at: '2026-05-01T00:00:00Z',
  updated_at: '2026-05-01T00:00:00Z',
}

const childSession: Session = {
  id: 'child',
  title: 'Child',
  parent_session_id: 'root',
  root_session_id: 'root',
  forked_from_message_id: 'm2',
  forked_from_index: 1,
  fork_reason: 'Alternative path',
  created_at: '2026-05-01T00:01:00Z',
  updated_at: '2026-05-01T00:02:00Z',
}

test('session lineage graph route is first-class console view', () => {
  assert.deepEqual(resolveRoute('/console/sessions/graph'), { view: 'session-lineage' })
  assert.match(appSource, /<SessionLineageGraph onNavigate=\{navigate\} \/>/)
  assert.match(navSource, /id: 'lineage'[\s\S]*path: '\/console\/sessions\/graph'/)
})

test('session lineage rows order roots before forked children with depth metadata', () => {
  const rows = buildSessionLineageRows([childSession, rootSession], {
    child: {
      role: 'assistant',
      content: 'workspace ready',
      message_id: 'm2',
      index: 1,
    },
  })

  assert.equal(rows.length, 2)
  assert.equal(rows[0].session.id, 'root')
  assert.equal(rows[0].depth, 0)
  assert.equal(rows[0].kind, 'root')
  assert.equal(rows[1].session.id, 'child')
  assert.equal(rows[1].depth, 1)
  assert.equal(rows[1].kind, 'fork')
  assert.equal(rows[1].parent?.id, 'root')
  assert.equal(rows[1].forkPreview?.content, 'workspace ready')
})

test('fork preview resolves by stable message id with index fallback', () => {
  const history: SessionMessage[] = [
    { id: 'm1', role: 'user', content: 'setup', timestamp: '2026-05-01T00:00:00Z' },
    { id: 'm2', role: 'assistant', content: 'branch point content that should be shown', timestamp: '2026-05-01T00:00:01Z' },
  ]

  assert.deepEqual(forkPreviewFromHistory(childSession, history), {
    message_id: 'm2',
    index: 1,
    role: 'assistant',
    content: 'branch point content that should be shown',
  })

  const legacyChild = { ...childSession, forked_from_message_id: 'missing', forked_from_index: 0 }
  assert.equal(forkPreviewFromHistory(legacyChild, history)?.content, 'setup')
})

test('session lineage graph page loads sessions, parent history previews, and chat navigation', () => {
  assert.match(graphSource, /listSessions\(true\)/)
  assert.match(graphSource, /getSessionHistory\(parentId\)/)
  assert.match(graphSource, /buildSessionLineageRows/)
  assert.match(graphSource, /forkPreviewFromHistory/)
  assert.match(graphSource, /onNavigate\(`\/console\/chat\/\$\{encodeURIComponent\(row\.session\.id\)\}`\)/)
  assert.match(graphSource, /class="lineage-graph"/)
  assert.match(graphSource, /Fork point/)
})
