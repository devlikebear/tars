import test from 'node:test'
import assert from 'node:assert/strict'

import { diffFocusTemplateStages, focusTemplateDraftHasChanges, focusTemplateDraftSummary, focusTemplateShareCommands } from '../src/lib/focusTemplateEdit.ts'
import type { FocusTemplate, FocusTemplateDraftResponse } from '../src/lib/types.ts'

function template(id: string, stages: FocusTemplate['stages'], extra: Partial<FocusTemplate> = {}): FocusTemplate {
  return { id, name: id, stages, builtin: false, ...extra }
}

test('diffFocusTemplateStages: a brand new template marks every stage added', () => {
  const after = template('blog', [{ id: 'plan' }, { id: 'draft', kind: 'build', instructions: 'write' }])
  const diffs = diffFocusTemplateStages(null, after)
  assert.deepEqual(diffs.map((d) => [d.id, d.change]), [
    ['plan', 'added'],
    ['draft', 'added'],
  ])
})

test('diffFocusTemplateStages: unchanged, changed, added and removed stages', () => {
  const before = template('blog', [
    { id: 'plan' },
    { id: 'draft', kind: 'build', instructions: 'write' },
    { id: 'edit', kind: 'review' },
  ])
  const after = template('blog', [
    { id: 'plan' },
    { id: 'draft', kind: 'build', instructions: 'write it well' },
    { id: 'illustrate', kind: 'build' },
  ])
  const diffs = diffFocusTemplateStages(before, after)
  assert.deepEqual(diffs.map((d) => [d.id, d.change]), [
    ['plan', 'unchanged'],
    ['draft', 'changed'],
    ['illustrate', 'added'],
    ['edit', 'removed'],
  ])
  const draftDiff = diffs.find((d) => d.id === 'draft')
  assert.equal(draftDiff?.before?.instructions, 'write')
  assert.equal(draftDiff?.after?.instructions, 'write it well')
})

test('diffFocusTemplateStages: a label-only change still counts as changed', () => {
  const before = template('blog', [{ id: 'plan', label: 'Outline' }])
  const after = template('blog', [{ id: 'plan', label: 'Scope' }])
  assert.equal(diffFocusTemplateStages(before, after)[0].change, 'changed')
})

test('focusTemplateDraftHasChanges: false only when a save draft repeats the existing template', () => {
  const before = template('blog', [{ id: 'plan' }, { id: 'draft', kind: 'build' }])
  const same: FocusTemplateDraftResponse = { action: 'save', template: template('blog', [{ id: 'plan' }, { id: 'draft', kind: 'build' }]) }
  const changed: FocusTemplateDraftResponse = { action: 'save', template: template('blog', [{ id: 'plan' }, { id: 'draft', kind: 'build', instructions: 'new' }]) }
  assert.equal(focusTemplateDraftHasChanges(before, same), false)
  assert.equal(focusTemplateDraftHasChanges(before, changed), true)
  assert.equal(focusTemplateDraftHasChanges(null, same), true)
})

test('focusTemplateDraftHasChanges: a delete draft is always a change', () => {
  const draft: FocusTemplateDraftResponse = { action: 'delete', original_id: 'blog' }
  assert.equal(focusTemplateDraftHasChanges(template('blog', [{ id: 'plan' }]), draft), true)
})

test('focusTemplateDraftSummary: the model summary, else a fallback per action', () => {
  assert.equal(focusTemplateDraftSummary({ action: 'save', template: template('blog', []), summary: 'Added an illustration stage.' }), 'Added an illustration stage.')
  assert.equal(focusTemplateDraftSummary({ action: 'save', template: template('blog', []) }), 'Save the "blog" template.')
  assert.equal(focusTemplateDraftSummary({ action: 'delete', original_id: 'blog' }), 'Delete the "blog" template.')
})

test('focusTemplateShareCommands: publishes the template file through the registry script', () => {
  const commands = focusTemplateShareCommands('/home/me/.tars/workspace/focus-templates/', { id: 'hotfix', source: 'hotfix.yaml' })
  assert.match(commands, /^git clone https:\/\/github\.com\/devlikebear\/tars-skills\.git$/m)
  assert.match(commands, /^git switch -c add-hotfix-focus-template$/m)
  assert.match(commands, /^python3 scripts\/registry\.py add-focus-template '\/home\/me\/\.tars\/workspace\/focus-templates\/hotfix\.yaml'$/m)
  assert.match(commands, /^gh pr create --fill$/m)
})

test('focusTemplateShareCommands: quotes paths and keeps Windows separators', () => {
  const commands = focusTemplateShareCommands("C:\\Users\\o'neil\\.tars\\workspace\\focus-templates", { id: 'notes' })
  assert.match(commands, /add-focus-template 'C:\\Users\\o'\\''neil\\\.tars\\workspace\\focus-templates\\notes\.yaml'$/m)
})
