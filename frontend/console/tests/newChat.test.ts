import test from 'node:test'
import assert from 'node:assert/strict'
import {
  currentProjectFolder,
  newChatRequest,
  parseNewChatArgs,
} from '../src/lib/newChat.ts'

test('parseNewChatArgs reads a path and the isolate flag in any order', () => {
  assert.deepEqual(parseNewChatArgs(''), { path: '', isolate: false })
  assert.deepEqual(parseNewChatArgs('--isolate'), { path: '', isolate: true })
  assert.deepEqual(parseNewChatArgs('~/code/app'), { path: '~/code/app', isolate: false })
  assert.deepEqual(parseNewChatArgs('~/code/app --isolate'), { path: '~/code/app', isolate: true })
  assert.deepEqual(parseNewChatArgs('-i ~/code/app'), { path: '~/code/app', isolate: true })
  // Folders with spaces: bare or quoted.
  assert.deepEqual(parseNewChatArgs('~/My Projects/app'), { path: '~/My Projects/app', isolate: false })
  assert.deepEqual(parseNewChatArgs('"~/My Projects/app" --isolate'), { path: '~/My Projects/app', isolate: true })
})

test('parseNewChatArgs rejects unknown flags', () => {
  assert.deepEqual(parseNewChatArgs('--worktree ~/app'), { path: '', isolate: false, unknownFlag: '--worktree' })
})

const artifacts = 'workspace/artifacts/s1'
const repo = 'code/app'

test('currentProjectFolder is the folder the session works in, not its own artifacts', () => {
  assert.equal(currentProjectFolder(null, null), '')
  assert.equal(currentProjectFolder({ worktree: null }, { current: artifacts, eligible: [artifacts] }), '')
  assert.equal(currentProjectFolder({ worktree: null }, { current: repo, eligible: [artifacts, repo] }), repo)
  // An isolated session: a new chat starts from the checkout, not from
  // this session's worktree.
  const isolated = { worktree: { source_dir: repo, dir: 'wt/app' } }
  assert.equal(currentProjectFolder(isolated, { current: 'wt/app', eligible: [artifacts, 'wt/app'] }), repo)
})

test('newChatRequest sends a folder only when there is one', () => {
  assert.deepEqual(newChatRequest('', true), {})
  assert.deepEqual(newChatRequest('  ', false), {})
  assert.deepEqual(newChatRequest(repo, false), { cwd: repo })
  assert.deepEqual(newChatRequest(` ${repo} `, true), { cwd: repo, isolate: true })
})
