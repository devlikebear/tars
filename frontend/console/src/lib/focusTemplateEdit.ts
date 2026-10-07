// AI-assisted focus template editing (ADR §4.1 follow-up): pure helpers for
// the editing screen's preview, built around POST /v1/focus/templates/draft
// (lib/api/focus.ts draftFocusTemplate). The server never writes a file for
// a draft; this module only turns the draft response into what the preview
// shows — a per-stage before/after diff and a one-line status — so the
// component stays thin. Tested under Node (tests/focusTemplateEdit.test.ts).
import type { FocusTemplate, FocusTemplateDraftResponse, FocusTemplateStage } from './types'

export type FocusTemplateStageChange = 'added' | 'removed' | 'changed' | 'unchanged'

export type FocusTemplateStageDiff = {
  id: string
  change: FocusTemplateStageChange
  before?: FocusTemplateStage
  after?: FocusTemplateStage
}

function stagesEqual(a: FocusTemplateStage, b: FocusTemplateStage): boolean {
  return (
    (a.kind ?? '') === (b.kind ?? '') &&
    (a.label ?? '') === (b.label ?? '') &&
    (a.instructions ?? '') === (b.instructions ?? '') &&
    (a.fix_instructions ?? '') === (b.fix_instructions ?? '')
  )
}

// diffFocusTemplateStages compares before's stages (the template being
// edited, or none for a new template) against after's (the draft), stage
// by id. Stages kept in the draft are listed in the draft's order (an
// added or reordered stage shows where it now sits); a stage the draft
// dropped is appended at the end, since there is no "after" position to
// place it at.
export function diffFocusTemplateStages(before: FocusTemplate | null | undefined, after: FocusTemplate | null | undefined): FocusTemplateStageDiff[] {
  const beforeStages = before?.stages ?? []
  const afterStages = after?.stages ?? []
  const beforeById = new Map(beforeStages.map((s) => [s.id, s]))
  const afterIds = new Set(afterStages.map((s) => s.id))

  const diffs: FocusTemplateStageDiff[] = afterStages.map((stage) => {
    const prev = beforeById.get(stage.id)
    if (!prev) return { id: stage.id, change: 'added', after: stage }
    return { id: stage.id, change: stagesEqual(prev, stage) ? 'unchanged' : 'changed', before: prev, after: stage }
  })
  for (const stage of beforeStages) {
    if (!afterIds.has(stage.id)) diffs.push({ id: stage.id, change: 'removed', before: stage })
  }
  return diffs
}

// focusTemplateDraftHasChanges reports whether a "save" draft actually
// differs from the template it replaces (false for a brand new template,
// which has nothing to differ from but is still a change worth showing).
export function focusTemplateDraftHasChanges(before: FocusTemplate | null | undefined, draft: FocusTemplateDraftResponse): boolean {
  if (draft.action === 'delete') return true
  if (!draft.template) return false
  if (!before) return true
  if (before.name !== draft.template.name || (before.description ?? '') !== (draft.template.description ?? '')) return true
  return diffFocusTemplateStages(before, draft.template).some((d) => d.change !== 'unchanged')
}

// focusTemplateDraftSummary is the one line shown above the diff: the
// model's own summary when it gave one, else a generic fallback that still
// says what is about to happen.
export function focusTemplateDraftSummary(draft: FocusTemplateDraftResponse): string {
  const summary = draft.summary?.trim()
  if (summary) return summary
  if (draft.action === 'delete') return `Delete the "${draft.original_id}" template.`
  return `Save the "${draft.template?.id ?? ''}" template.`
}
