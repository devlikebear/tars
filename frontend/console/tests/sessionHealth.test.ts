import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { buildSessionHealthReport } from '../src/lib/sessionHealth.ts'
import type { ChatToolInfo } from '../src/lib/api/chat.ts'
import type { SessionToolConfig } from '../src/lib/api/sessions.ts'
import type { Session, SessionMessage, SessionTasks } from '../src/lib/types.ts'

const chatSource = readFileSync(new URL('../src/components/Chat.svelte', import.meta.url), 'utf8')
const panelSource = readFileSync(new URL('../src/components/SessionHealthPanel.svelte', import.meta.url), 'utf8')

const tools: ChatToolInfo[] = [
  { name: 'read_file', description: 'read files', high_risk: false, group: 'files' },
  { name: 'write_file', description: 'write files', high_risk: true, group: 'files' },
  { name: 'edit_file', description: 'edit files', high_risk: true, group: 'files' },
  { name: 'exec', description: 'run shell commands', high_risk: true, group: 'shell' },
  { name: 'git_status', description: 'read git state', high_risk: false, group: 'git' },
]

function session(overrides: Partial<Session> = {}): Session {
  return {
    id: 'sess-health',
    title: 'Long coding session',
    kind: 'chat',
    created_at: '2026-04-20T10:00:00Z',
    updated_at: '2026-04-28T10:00:00Z',
    ...overrides,
  }
}

function messages(count: number): SessionMessage[] {
  return Array.from({ length: count }, (_, index) => ({
    id: `m-${index}`,
    role: index % 2 === 0 ? 'user' : 'assistant',
    content: `message ${index}`,
    timestamp: '2026-04-28T10:00:00Z',
  }))
}

test('session health recommends compacting and splitting long sessions', () => {
  const report = buildSessionHealthReport({
    session: session(),
    messages: messages(190),
    tasks: { tasks: [] },
    config: {},
    tools,
    now: new Date('2026-05-01T10:00:00Z'),
  })

  assert.equal(report.status, 'critical')
  assert.equal(report.badgeLabel, 'Critical')
  assert.ok(report.recommendations.some((item) => item.action === 'compact'))
  assert.ok(report.recommendations.some((item) => item.action === 'review_fork_points'))
  assert.ok(report.signals.some((item) => item.kind === 'long_context'))
})

test('session health detects stale plans with open tasks', () => {
  const tasks: SessionTasks = {
    plan: {
      goal: 'Ship contract mode',
      status: 'executing',
      created_at: '2026-04-21T09:00:00Z',
      updated_at: '2026-04-24T09:00:00Z',
    },
    tasks: [
      { id: 't1', title: 'Wire panel', status: 'pending' },
      { id: 't2', title: 'Verify browser', status: 'in_progress' },
    ],
  }

  const report = buildSessionHealthReport({
    session: session(),
    messages: messages(12),
    tasks,
    config: {},
    tools,
    now: new Date('2026-05-01T10:00:00Z'),
  })

  assert.equal(report.status, 'attention')
  assert.ok(report.recommendations.some((item) => item.action === 'open_tasks'))
  assert.ok(report.signals.some((item) => item.kind === 'stale_plan'))
})

test('session health warns when high risk tools are broadly enabled', () => {
  const config: SessionToolConfig = {
    tools_custom: true,
    tools_enabled: ['read_file', 'write_file', 'edit_file', 'exec', 'git_status'],
  }

  const report = buildSessionHealthReport({
    session: session(),
    messages: messages(8),
    tasks: { tasks: [] },
    config,
    tools,
    now: new Date('2026-05-01T10:00:00Z'),
  })

  assert.equal(report.status, 'attention')
  assert.ok(report.recommendations.some((item) => item.action === 'open_config'))
  assert.ok(report.signals.some((item) => item.kind === 'broad_permissions'))
})

test('session health does not flag permissions on an empty new session', () => {
  const report = buildSessionHealthReport({
    session: session(),
    messages: [],
    tasks: { tasks: [] },
    config: {},
    tools,
    now: new Date('2026-05-01T10:00:00Z'),
  })

  assert.equal(report.status, 'healthy')
  assert.equal(report.recommendations.length, 0)
})

test('chat renders a session health badge and recommendation panel', () => {
  assert.match(chatSource, /SessionHealthPanel/)
  assert.match(chatSource, /session-health-badge/)
  assert.match(chatSource, /Health/)
  assert.match(panelSource, /Recommendation/)
  assert.match(panelSource, /Open Tasks/)
  assert.match(panelSource, /Compact/)
  assert.match(panelSource, /Open Config/)
})
