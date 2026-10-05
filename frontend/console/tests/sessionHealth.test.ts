import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { buildSessionHealthReport } from '../src/lib/sessionHealth.ts'
import { sessionHealthEn, sessionHealthKo } from '../src/i18n/sections/sessionHealth.ts'
import type { ChatToolInfo } from '../src/lib/api/chat.ts'
import type { SessionToolConfig } from '../src/lib/api/sessions.ts'
import type { Session, SessionMessage, SessionTasks } from '../src/lib/types.ts'
import { chatWorkbenchSource } from './helpers/chatWorkbenchSource.ts'

const chatSource = chatWorkbenchSource
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
  const report = buildSessionHealthReport(sessionHealthEn, {
    session: session(),
    messages: messages(190),
    tasks: { tasks: [] },
    config: {},
    tools,
    now: new Date('2026-05-01T10:00:00Z'),
  })

  assert.equal(report.status, 'critical')
  assert.equal(report.badgeLabel, 'Critical')
  assert.equal(report.summary, '1 critical session issue(s) need action before continuing.')
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

  const report = buildSessionHealthReport(sessionHealthEn, {
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

for (const permissionMode of ['manual', 'accept_edits', 'plan', '', undefined, 'unknown']) {
  test(`native ${permissionMode} mode does not warn about enabled high-risk tools`, () => {
    const report = buildSessionHealthReport(sessionHealthEn, {
      messages: messages(8), tools, provider: { kind: 'anthropic', permissionMode },
    })
    assert.equal(report.status, 'healthy')
    assert.ok(!report.signals.some((item) => item.kind === 'broad_permissions'))
  })
}

for (const strings of [sessionHealthEn, sessionHealthKo]) {
  for (const toolCount of [1, 3]) {
    test(`native auto mode warns for ${toolCount} high-risk tools (${strings.status.healthy})`, () => {
      const report = buildSessionHealthReport(strings, {
        messages: messages(8), tools: tools.filter((tool) => tool.high_risk).slice(0, toolCount),
        provider: { kind: 'anthropic', permissionMode: 'auto' },
      })
      assert.equal(report.status, 'watch')
      const signal = report.signals.find((item) => item.kind === 'broad_permissions')
      assert.equal(signal?.severity, 'warning')
      assert.equal(signal?.detail, strings.signals.broadPermissions.detail(toolCount))
      assert.ok(report.recommendations.some((item) => item.action === 'choose_permission_mode' && item.severity === 'warning'))
    })
  }

  for (const [count, percent, severity] of [
    [83, 5.7, undefined], [160, 5.7, undefined], [80, undefined, 'warning'],
    [160, undefined, 'critical'], [160, 75, 'warning'], [8, 94, 'warning'], [8, 95, 'critical'],
  ] as const) {
    test(`context ${count} messages at ${percent}% (${strings.status.healthy})`, () => {
      const report = buildSessionHealthReport(strings, {
        messages: messages(count),
        contextInfo: percent === undefined ? {} : { history_tokens: percent * 100, compaction_trigger_tokens: 10000 },
      })
      const signal = report.signals.find((item) => item.kind === 'long_context')
      assert.equal(signal?.severity, severity)
      if (severity) {
        const text = severity === 'critical' ? strings.signals.contextSaturated : strings.signals.contextLong
        assert.equal(signal?.detail, percent === undefined ? text.detail(count) : text.detailPercent(percent))
        if (percent !== undefined) assert.ok(signal?.detail.includes(`${percent}%`))
      } else {
        assert.equal(report.status, 'healthy')
      }
    })
  }
}

test('session health does not flag permissions on an empty new session', () => {
  const report = buildSessionHealthReport(sessionHealthEn, {
    provider: { kind: 'anthropic', permissionMode: 'auto' },
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

test('session health report text follows the strings it is given', () => {
  const report = buildSessionHealthReport(sessionHealthKo, {
    session: session(),
    messages: messages(190),
    tasks: { tasks: [] },
    config: {},
    tools,
    now: new Date('2026-05-01T10:00:00Z'),
  })

  assert.equal(report.badgeLabel, sessionHealthKo.status.critical)
  assert.equal(report.summary, sessionHealthKo.summary.critical(report.signals.length))
  const compact = report.recommendations.find((item) => item.action === 'compact')
  assert.equal(compact?.actionLabel, sessionHealthKo.actions.compact)
})

test('chat renders a session health badge and recommendation panel', () => {
  assert.match(chatSource, /SessionHealthPanel/)
  assert.match(chatSource, /session-health-badge/)
  assert.match(chatSource, /Health/)
  assert.match(panelSource, /\$t\.sessionHealth\.panel\.recommendations\}/)
  assert.equal(sessionHealthEn.panel.recommendations, 'Recommendations')
  assert.match(panelSource, /\$t\.sessionHealth\.actions\[recommendation\.action\]/)
  assert.equal(sessionHealthEn.actions.open_tasks, 'Open Tasks')
  assert.equal(sessionHealthEn.actions.compact, 'Compact')
  assert.equal(sessionHealthEn.actions.open_config, 'Open Config')
})

// claude-code-cli resumes its own upstream session (#857): TARS does not
// re-send the transcript, and Claude Code compacts its own context.
const highRiskConfig: SessionToolConfig = {
  tools_custom: true,
  tools_enabled: ['read_file', 'write_file', 'edit_file', 'exec', 'git_status'],
}

test('a resumed claude-code-cli session gets no transcript-length compaction advice', () => {
  const report = buildSessionHealthReport(sessionHealthKo, {
    session: session({ upstream_session_id: 'upstream-1' }),
    messages: messages(104),
    tasks: { tasks: [] },
    config: {},
    tools: [],
    contextInfo: { history_tokens: 90_000, compaction_trigger_tokens: 100_000 },
    provider: { kind: 'claude-code-cli', claudeCodeFlag: 'auto' },
    now: new Date('2026-04-28T11:00:00Z'),
  })

  assert.equal(report.status, 'healthy')
  assert.ok(!report.signals.some((item) => item.kind === 'long_context'))
  assert.ok(!report.recommendations.some((item) => item.action === 'compact'))
  assert.equal(report.metrics.messageCount, 104)
  assert.equal(report.metrics.contextTokenPercent, undefined)
  const note = report.notes.find((item) => item.kind === 'provider_context')
  assert.equal(note?.title, sessionHealthKo.notes.providerContext.title('Claude Code'))
  assert.equal(note?.detail, sessionHealthKo.notes.providerContext.detail(104))
})

test('a claude-code-cli session that has not resumed yet still warns about length', () => {
  // No upstream session: the next turn starts a fresh CLI session from the
  // whole transcript, so its length matters as it does for native providers.
  const report = buildSessionHealthReport(sessionHealthEn, {
    session: session(),
    messages: messages(104),
    tasks: { tasks: [] },
    config: {},
    tools: [],
    provider: { kind: 'claude-code-cli' },
    now: new Date('2026-04-28T11:00:00Z'),
  })

  assert.ok(report.signals.some((item) => item.kind === 'long_context'))
  assert.ok(report.recommendations.some((item) => item.action === 'compact'))
  assert.ok(!report.notes.some((item) => item.kind === 'provider_context'))
})

test('a native session keeps its length warning even with a stale upstream id', () => {
  const report = buildSessionHealthReport(sessionHealthEn, {
    session: session({ upstream_session_id: 'upstream-1' }),
    messages: messages(104),
    tasks: { tasks: [] },
    config: highRiskConfig,
    tools,
    provider: { kind: 'anthropic' },
    now: new Date('2026-04-28T11:00:00Z'),
  })

  assert.ok(report.signals.some((item) => item.kind === 'long_context'))
  assert.ok(!report.signals.some((item) => item.kind === 'broad_permissions'))
  assert.equal(report.metrics.highRiskToolCount, 3)
  assert.equal(report.metrics.cliPermissionMode, undefined)
  assert.deepEqual(report.notes, [])
})

test('a claude-code-cli session is judged by its permission mode, not TARS tools', () => {
  const report = buildSessionHealthReport(sessionHealthEn, {
    session: session({ upstream_session_id: 'upstream-1' }),
    messages: messages(8),
    tasks: { tasks: [] },
    config: highRiskConfig,
    tools,
    provider: { kind: 'claude-code-cli', claudeCodeFlag: 'acceptEdits' },
    now: new Date('2026-04-28T11:00:00Z'),
  })

  assert.equal(report.status, 'healthy')
  assert.ok(!report.signals.some((item) => item.kind === 'broad_permissions'))
  assert.ok(!report.recommendations.some((item) => item.action === 'open_config'))
  assert.equal(report.metrics.cliPermissionMode, 'acceptEdits')
  const note = report.notes.find((item) => item.kind === 'provider_permissions')
  assert.equal(note?.detail, sessionHealthEn.notes.providerPermissions.detail('Claude Code', 'acceptEdits'))
})

test('claude-code-cli running with bypassPermissions is flagged', () => {
  const report = buildSessionHealthReport(sessionHealthEn, {
    session: session({ upstream_session_id: 'upstream-1' }),
    messages: messages(8),
    tasks: { tasks: [] },
    config: {},
    tools,
    provider: { kind: 'claude-code-cli', claudeCodeFlag: 'bypassPermissions' },
    now: new Date('2026-04-28T11:00:00Z'),
  })

  assert.equal(report.status, 'attention')
  const signal = report.signals.find((item) => item.kind === 'broad_permissions')
  assert.equal(signal?.title, sessionHealthEn.signals.cliBypassPermissions.title)
  const recommendation = report.recommendations.find((item) => item.action === 'choose_permission_mode')
  assert.equal(recommendation?.actionLabel, sessionHealthEn.actions.choose_permission_mode)
})

test('bypassPermissions is not flagged on an empty new session', () => {
  const report = buildSessionHealthReport(sessionHealthEn, {
    session: session(),
    messages: [],
    config: {},
    tools,
    provider: { kind: 'claude-code-cli', claudeCodeFlag: 'bypassPermissions' },
  })

  assert.equal(report.status, 'healthy')
})

test('a resumed antigravity-cli session is left to the CLI for context and tools', () => {
  const report = buildSessionHealthReport(sessionHealthEn, {
    session: session({ upstream_session_id: 'conversation-1' }),
    messages: messages(170),
    tasks: { tasks: [] },
    config: highRiskConfig,
    tools,
    provider: { kind: 'antigravity-cli' },
    now: new Date('2026-04-28T11:00:00Z'),
  })

  assert.equal(report.status, 'healthy')
  assert.equal(report.notes.find((item) => item.kind === 'provider_context')?.title, sessionHealthEn.notes.providerContext.title('Antigravity'))
  assert.equal(report.notes.find((item) => item.kind === 'provider_permissions')?.detail, sessionHealthEn.notes.providerPermissions.detail('Antigravity', ''))
  assert.equal(report.metrics.cliPermissionMode, '')
})

test('the health panel shows provider notes and the CLI permission mode', () => {
  assert.match(panelSource, /report\.notes/)
  assert.match(panelSource, /cliPermissionMode/)
  assert.match(chatSource, /case 'choose_permission_mode'/)
})
