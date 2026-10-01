import test from 'node:test'
import assert from 'node:assert/strict'

import { RELEASE_STAGES, releaseGoal, releaseItemLabel, releaseRequest } from '../src/lib/focusRelease.ts'
import type { ReleaseTrainGroup } from '../src/lib/types.ts'

function group(extra: Partial<ReleaseTrainGroup> = {}): ReleaseTrainGroup {
  return {
    repo: 'repo/tars',
    last_tag: 'v0.1.0',
    since: '2026-01-01T00:00:00Z',
    items: [
      { session_id: 's1', title: 'Pipeline graph', goal: 'Draw the pipeline\nwith xyflow', updated_at: '2026-02-01T00:00:00Z' },
      {
        session_id: 's2',
        title: 'Release train',
        goal: 'List merged pipelines',
        pr: { number: 42, url: 'https://example.com/pr/42', state: 'merged' },
        updated_at: '2026-02-02T00:00:00Z',
      },
    ],
    ...extra,
  }
}

test('releaseItemLabel uses the PR number when there is one, else the session title', () => {
  const g = group()
  assert.equal(releaseItemLabel(g.items[0]), 'Pipeline graph')
  assert.equal(releaseItemLabel(g.items[1]), '#42 Release train')
})

test('releaseGoal asks for exactly the fixed release plan', () => {
  const goal = releaseGoal(group())
  assert.match(goal.split('\n')[0], /^Release: /)
  assert.match(goal, /VERSION\.txt/)
  assert.match(goal, /CHANGELOG\.md/)
  assert.match(goal, /pull request/i)
  assert.match(goal, /[Mm]erge/)
  assert.ok(goal.includes(JSON.stringify(RELEASE_STAGES)), 'names the stages exactly')
  assert.deepEqual(RELEASE_STAGES, ['plan', 'build', 'pr', 'pr_review', 'merge'])
  assert.match(goal, /since v0\.1\.0/)
})

test('releaseGoal lists every merged item with its PR link and the first line of its goal', () => {
  const goal = releaseGoal(group())
  assert.ok(goal.includes('- Pipeline graph — Draw the pipeline\n'))
  assert.ok(goal.includes('- #42 Release train (https://example.com/pr/42) — List merged pipelines'))
  assert.ok(!goal.includes('with xyflow'), 'only the first line of a goal')
})

test('releaseGoal without a tag lists everything since the first release', () => {
  const goal = releaseGoal(group({ last_tag: undefined, since: undefined }))
  assert.match(goal, /since the start of the repository/)
})

test('releaseRequest starts an isolated pipeline in the repository root', () => {
  const g = group()
  assert.deepEqual(releaseRequest(g, 'Release (2)'), { goal: releaseGoal(g), cwd: 'repo/tars', isolate: true, title: 'Release (2)' })
})
