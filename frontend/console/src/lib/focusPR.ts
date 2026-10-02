// Focus mode's PR stages (docs/decisions/focus-mode.md §4, P4): pure
// helpers for the stepper's CI chips, the G3 draft gate, the G4 merge
// summary, the "gh unavailable" manual pass and the finished state. The
// facts come from the server's read-only gh probe. Tested under Node.
import type {
  FocusCard,
  FocusCICounts,
  FocusMergeSummary,
  FocusPipeline,
  FocusPRCheck,
  FocusPRDraft,
  FocusStageId,
  FocusWorktreeEnd,
} from './types.ts'

// The notice card the server raises when gh cannot run (pr.go
// NoticeGHUnavailable).
export const ghUnavailableTitle = 'gh unavailable'

const prStages = new Set<FocusStageId>(['pr', 'pr_review', 'merge'])

// ciCounts counts checks by state; skipped counts as passed, as the server
// does.
export function ciCounts(checks: FocusPRCheck[] | undefined): FocusCICounts {
  const counts = { passed: 0, failed: 0, pending: 0 }
  for (const c of checks ?? []) {
    if (c.state === 'fail') counts.failed++
    else if (c.state === 'pending') counts.pending++
    else counts.passed++
  }
  return counts
}

export type PRStageChip = { number: number; url: string; ci: FocusCICounts | null }

// prStageChip is what a PR stage of the stepper shows: the PR number once
// known, and on pr_review and merge the CI counts. Stages not reached yet
// show nothing.
export function prStageChip(p: FocusPipeline, stage: FocusStageId): PRStageChip | null {
  if (!prStages.has(stage) || !p.pr || p.pr.number <= 0) return null
  const status = p.stages.find((s) => s.id === stage)?.status
  if (status !== 'active' && status !== 'done' && status !== 'blocked') return null
  const ci = stage !== 'pr' && p.pr.checks?.length ? ciCounts(p.pr.checks) : null
  return { number: p.pr.number, url: p.pr.url, ci }
}

// ghUnavailable is the latest probe's error when the current stage cannot
// get its facts from gh — the developer passes it by hand — else null. It
// follows the latest probe (pr_unavailable), not the stage's notice card,
// so one transient failure does not leave the banner up. A gate open or a
// turn still owed decides first.
export function ghUnavailable(p: FocusPipeline): string | null {
  // A turn the pipeline owes (the write turn a gate approved, a fix) runs
  // first: the server refuses a pass by hand meanwhile (#1082).
  if (p.open_gate || !p.pr_unavailable || p.pending_turn) return null
  const stage = p.stages.find((s) => s.id === p.current)
  return stage?.status === 'active' ? p.pr_unavailable : null
}

export type FinishOutcome = {
  merged: boolean
  // What the server did with the session worktree (null: not recorded).
  worktree: FocusWorktreeEnd['action'] | null
  reason: string
}

// finishOutcome says how a finished pipeline with a merge stage ended: was
// the PR merged, and what happened to the session worktree, as the server
// recorded it. null while running, and for plans without merge.
export function finishOutcome(p: FocusPipeline): FinishOutcome | null {
  if (!p.plan?.stages.includes('merge')) return null
  if (p.stages.some((s) => s.status === 'active' || s.status === 'pending' || s.status === 'blocked')) return null
  if (p.stages.find((s) => s.id === 'merge')?.status !== 'done') return null
  return {
    merged: p.pr?.state === 'MERGED',
    worktree: p.worktree_end?.action ?? null,
    reason: p.worktree_end?.reason ?? '',
  }
}

export type FinishedStrings = {
  finished: string
  finishedMerged: string
  finishedMergedOnly: string
  finishedKept: string
  finishedKeptBecause: (reason: string) => string
  finishedLeft: (reason: string) => string
}

// finishedText is the finished banner: never claims a worktree outcome the
// server did not record.
export function finishedText(o: FinishOutcome | null, text: FinishedStrings): string {
  const lead = o?.merged ? text.finishedMergedOnly : text.finished
  switch (o?.worktree) {
    case 'discard':
      return o.merged ? text.finishedMerged : lead
    case 'keep':
      if (!o.merged && !o.reason) return text.finishedKept
      return `${lead} ${text.finishedKeptBecause(o.reason)}`
    case 'left':
      return `${lead} ${text.finishedLeft(o.reason)}`
    default:
      return lead
  }
}

// prDraftOf is the draft of a G3 gate card, else null.
export function prDraftOf(card: FocusCard): FocusPRDraft | null {
  if (card.kind !== 'gate' || card.stage !== 'pr') return null
  const p = card.payload as Partial<FocusPRDraft> | undefined
  if (!p || typeof p.title !== 'string') return null
  return { title: p.title, body: typeof p.body === 'string' ? p.body : '' }
}

// mergeSummary is the summary of a G4 gate card, else null.
export function mergeSummary(card: FocusCard): FocusMergeSummary | null {
  if (card.kind !== 'gate' || card.stage !== 'merge') return null
  const p = card.payload as Partial<FocusMergeSummary> | undefined
  if (!p || typeof p.checks !== 'object' || p.checks === null) return null
  return {
    pr: p.pr,
    title: p.title,
    checks: { passed: p.checks.passed ?? 0, failed: p.checks.failed ?? 0, pending: p.checks.pending ?? 0 },
    fixed: p.fixed ?? 0,
    dismissed: p.dismissed ?? 0,
    undecided: p.undecided ?? 0,
    merge_state: p.merge_state,
  }
}
