import test from 'node:test'
import assert from 'node:assert/strict'

import { RELEASE_STAGES, releaseGoal, releaseItemLabel, releaseKickoff, releaseRequest } from '../src/lib/focusRelease.ts'
import type { ReleaseTrainGroup } from '../src/lib/types.ts'

function group(extra: Partial<ReleaseTrainGroup> = {}): ReleaseTrainGroup {
  return {
    repo: 'repo/tars',
    last_tag: 'v0.1.0',
    since: '2026-01-01T00:00:00Z',
    items: [
      { session_id: 's1', title: 'Pipeline graph', goal: 'Draw the pipeline\nwith xyflow', finished_at: '2026-02-01T00:00:00Z', updated_at: '2026-02-01T00:00:00Z' },
      {
        session_id: 's2',
        title: 'Release train',
        goal: 'List merged pipelines',
        pr: { number: 42, url: 'https://example.com/pr/42', state: 'merged' },
        finished_at: '2026-02-02T00:00:00Z',
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

test('releaseKickoff asks for exactly the fixed release plan', () => {
  const goal = releaseKickoff(group())
  assert.match(goal.split('\n')[0], /^Release: /)
  assert.match(goal, /VERSION\.txt/)
  assert.match(goal, /CHANGELOG\.md/)
  assert.match(goal, /pull request/i)
  assert.match(goal, /[Mm]erge/)
  assert.ok(goal.includes(JSON.stringify(RELEASE_STAGES)), 'names the stages exactly')
  assert.deepEqual(RELEASE_STAGES, ['plan', 'build', 'pr', 'pr_review', 'merge'])
  assert.match(goal, /since v0\.1\.0/)
})

test('releaseKickoff lists every merged item with its PR link and the first line of its goal', () => {
  const goal = releaseKickoff(group())
  assert.ok(goal.includes('- Pipeline graph — Draw the pipeline\n'))
  assert.ok(goal.includes('- #42 Release train (https://example.com/pr/42) — List merged pipelines'))
  assert.ok(!goal.includes('with xyflow'), 'only the first line of a goal')
})

test('releaseKickoff without a tag lists everything since the first release', () => {
  const goal = releaseKickoff(group({ last_tag: undefined, since: undefined }))
  assert.match(goal, /since the start of the repository/)
})

test('releaseGoal is one line, so stage guidance never repeats the merged list', () => {
  const goal = releaseGoal(group())
  assert.equal(goal, 'Release: ship the 2 change(s) merged since v0.1.0.')
  assert.equal(releaseKickoff(group()).split('\n')[0], goal, 'the kickoff opens with the goal')
})

test('releaseRequest starts an isolated release pipeline that records what it ships', () => {
  const g = group()
  assert.deepEqual(releaseRequest(g, 'Release (2)'), {
    goal: releaseGoal(g),
    kickoff: releaseKickoff(g),
    kind: 'release',
    cwd: 'repo/tars',
    isolate: true,
    title: 'Release (2)',
    release_items: ['s1', 's2'],
    release_since: '2026-01-01T00:00:00Z',
  })
  // No tag yet: no cut-off to record.
  assert.equal('release_since' in releaseRequest(group({ last_tag: undefined, since: undefined }), 't'), false)
})

test('releaseKickoff works from the remote default branch, not the local HEAD', () => {
  const text = releaseKickoff(group())
  assert.match(text, /git fetch origin/)
  assert.match(text, /origin\/<default branch>/)
  assert.match(text, /not .*local HEAD/)
})

test('an item without a PR has no PR reference; a PR without a URL has no empty link', () => {
  const text = releaseKickoff(group({
    items: [
      { session_id: 's1', title: 'No PR', goal: 'g1', finished_at: '2026-02-01T00:00:00Z', updated_at: '2026-02-01T00:00:00Z' },
      { session_id: 's2', title: 'No URL', goal: 'g2', pr: { number: 7, url: '', state: 'merged' }, finished_at: '2026-02-02T00:00:00Z', updated_at: '2026-02-02T00:00:00Z' },
    ],
  }))
  assert.ok(text.includes('- No PR — g1\n'))
  assert.ok(text.includes('- #7 No URL — g2'))
  assert.ok(!text.includes('()'), 'no empty link')
  assert.ok(!/#undefined|#0 /.test(text))
})
