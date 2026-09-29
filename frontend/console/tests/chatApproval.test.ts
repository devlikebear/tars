import test from 'node:test'
import assert from 'node:assert/strict'

import {
  approvalFromEvent,
  approvalPreview,
  decisionForKey,
  resolveApproval,
  withdrawPendingApprovals,
  type ChatApproval,
} from '../src/lib/chatApproval.ts'
import type { ChatMessage } from '../src/lib/chatMessages.ts'

const request = {
  type: 'permission_request',
  session_id: 's1',
  request_id: 'r1',
  tool_name: 'Bash',
  tool_use_id: 'toolu_1',
  input: { command: 'touch hello.txt', description: 'Create hello.txt' },
  reason: 'writes a file',
  agent_id: '',
  session_rule: 'Bash(touch:*)',
}

test('approvalFromEvent reads a permission_request', () => {
  const approval = approvalFromEvent(request)
  assert.ok(approval)
  assert.equal(approval.requestId, 'r1')
  assert.equal(approval.sessionId, 's1')
  assert.equal(approval.toolName, 'Bash')
  assert.equal(approval.sessionRule, 'Bash(touch:*)')
  assert.equal(approval.reason, 'writes a file')
  assert.equal(approval.agentId, undefined)
  assert.equal(approval.state, 'pending')
})

test('approvalFromEvent ignores events it cannot answer', () => {
  assert.equal(approvalFromEvent({ type: 'permission_request', session_id: 's1' }), null)
  assert.equal(approvalFromEvent({ ...request, request_id: '  ' }), null)
  assert.equal(approvalFromEvent({ ...request, type: 'status' }), null)
})

test('approvalPreview shows what the tool will touch', () => {
  const base = approvalFromEvent(request) as ChatApproval
  assert.deepEqual(approvalPreview(base), { kind: 'command', text: 'touch hello.txt' })
  assert.deepEqual(
    approvalPreview({ ...base, toolName: 'Edit', input: { file_path: '/repo/a.go', old_string: 'x', new_string: 'y' } }),
    { kind: 'file', text: '/repo/a.go' },
  )
  assert.deepEqual(approvalPreview({ ...base, toolName: 'WebFetch', input: { url: 'https://example.com' } }), {
    kind: 'url',
    text: 'https://example.com',
  })
  assert.deepEqual(approvalPreview({ ...base, toolName: 'mcp__x__y', input: { a: 1 } }), {
    kind: 'input',
    text: '{\n  "a": 1\n}',
  })
  assert.deepEqual(approvalPreview({ ...base, toolName: 'mcp__x__y', input: undefined }), { kind: 'input', text: '' })
})

test('resolveApproval settles the matching card only', () => {
  const approval = approvalFromEvent(request) as ChatApproval
  const messages: ChatMessage[] = [
    { id: 'u', role: 'user', text: 'hi' },
    { id: 'approval-r1', role: 'approval', text: '', approval },
    { id: 'a', role: 'assistant', text: '' },
  ]
  const next = resolveApproval(messages, 'r1', 'allowed_session')
  assert.notEqual(next, messages)
  assert.equal(next[1].approval?.state, 'allowed_session')
  assert.equal(messages[1].approval?.state, 'pending', 'the input list is not mutated')

  assert.equal(resolveApproval(messages, 'other', 'denied'), messages)
  assert.equal(resolveApproval(messages, 'r1', 'bogus')[1].approval?.state, 'withdrawn')
})

test('decisionForKey maps y/s/n and only offers s with a session rule', () => {
  const approval = approvalFromEvent(request) as ChatApproval
  assert.equal(decisionForKey('y', approval), 'allow_once')
  assert.equal(decisionForKey('S', approval), 'allow_session')
  assert.equal(decisionForKey('n', approval), 'deny')
  assert.equal(decisionForKey('x', approval), null)
  assert.equal(decisionForKey('s', { ...approval, sessionRule: undefined }), null)
  assert.equal(decisionForKey('y', { ...approval, state: 'sending' }), null)
})

test('withdrawPendingApprovals closes cards the stream left open', () => {
  const approval = approvalFromEvent(request) as ChatApproval
  const messages: ChatMessage[] = [
    { id: 'approval-r1', role: 'approval', text: '', approval },
    { id: 'approval-r2', role: 'approval', text: '', approval: { ...approval, requestId: 'r2', state: 'denied' } },
    { id: 'approval-r3', role: 'approval', text: '', approval: { ...approval, requestId: 'r3', state: 'sending' } },
  ]
  const next = withdrawPendingApprovals(messages)
  assert.deepEqual(next.map((m) => m.approval?.state), ['withdrawn', 'denied', 'withdrawn'])
  const settled = [messages[1]]
  assert.equal(withdrawPendingApprovals(settled), settled, 'nothing open, same array')
})
