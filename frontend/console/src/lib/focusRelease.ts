// The release train (docs/decisions/focus-mode.md §9 P5): "Start release"
// starts an ordinary focus pipeline whose kickoff goal asks the agent to
// propose one fixed plan. The plan still goes through the plan gate; nothing
// here bypasses it.
//
// The goal is an instruction to the agent, not UI text, so it is English in
// every locale (like the stage guidance the server appends).
import type { FocusCreateRequest } from './api/focus.ts'
import type { FocusStageId, ReleaseTrainGroup, ReleaseTrainItem } from './types.ts'

// A release skips the self-review stage: the merged pipelines were reviewed.
export const RELEASE_STAGES: FocusStageId[] = ['plan', 'build', 'pr', 'pr_review', 'merge']

// The longest goal line quoted per item.
const goalLineMax = 160

// releaseItemLabel names a merged pipeline: its PR when it has one.
export function releaseItemLabel(item: ReleaseTrainItem): string {
  return item.pr ? `#${item.pr.number} ${item.title}` : item.title
}

function firstLine(text: string): string {
  const line = text.split('\n', 1)[0]?.trim() ?? ''
  return line.length > goalLineMax ? `${line.slice(0, goalLineMax - 1).trimEnd()}…` : line
}

function itemLine(item: ReleaseTrainItem): string {
  const link = item.pr?.url ? ` (${item.pr.url})` : ''
  const goal = firstLine(item.goal)
  return `- ${releaseItemLabel(item)}${link}${goal ? ` — ${goal}` : ''}`
}

// releaseGoal is the release pipeline's goal: one line, since the stage
// guidance repeats the goal on every turn.
export function releaseGoal(group: ReleaseTrainGroup): string {
  return `Release: ship the ${group.items.length} change(s) merged ${sinceText(group)}.`
}

function sinceText(group: ReleaseTrainGroup): string {
  return group.last_tag ? `since ${group.last_tag}` : 'since the start of the repository'
}

// releaseKickoff is the first turn of a release pipeline: the goal, the
// fixed plan, and the merged list, sent once.
export function releaseKickoff(group: ReleaseTrainGroup): string {
  return [
    releaseGoal(group),
    '',
    'Work from the remote: run `git fetch origin` and start the release branch from `origin/<default branch>` (find it with `git symbolic-ref refs/remotes/origin/HEAD`), not from the local HEAD.',
    '',
    'Propose exactly this plan and nothing else:',
    '1. Bump VERSION.txt to the next version.',
    '2. Update CHANGELOG.md with one entry per merged change listed below.',
    '3. Open the release pull request.',
    '4. Merge it once its checks pass.',
    `Use the stages ${JSON.stringify(RELEASE_STAGES)}.`,
    '',
    `Merged ${sinceText(group)}:`,
    ...group.items.map(itemLine),
  ].join('\n')
}

// releaseRequest starts the release pipeline in an isolated worktree of the
// repository's main checkout, marked as a release so the next release train
// does not list it. It records the sessions it ships and the cut-off the list started from,
// which the train treats as released once this release finishes.
export function releaseRequest(group: ReleaseTrainGroup, title: string): FocusCreateRequest {
  const request: FocusCreateRequest = {
    goal: releaseGoal(group),
    kickoff: releaseKickoff(group),
    kind: 'release',
    cwd: group.repo,
    isolate: true,
    title,
    release_items: group.items.map((item) => item.session_id),
  }
  if (group.since) request.release_since = group.since
  return request
}
