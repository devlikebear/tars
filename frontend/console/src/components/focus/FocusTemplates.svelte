<script lang="ts">
  // Natural-language focus template editing (ADR §4.1 follow-up): list the
  // built-in and workspace templates, then let the user describe a change
  // in plain language — the server (POST /v1/focus/templates/draft) asks an
  // LLM for a proposal and validates it, but never writes anything. The
  // preview here is the only place a save or delete actually happens
  // (PUT/DELETE /v1/focus/templates/{id}).
  import { onMount } from 'svelte'
  import { t } from '../../i18n'
  import { deleteFocusTemplate, draftFocusTemplate, listFocusTemplates, saveFocusTemplate } from '../../lib/api'
  import { diffFocusTemplateStages, focusTemplateDraftHasChanges, focusTemplateDraftSummary } from '../../lib/focusTemplateEdit'
  import { stageLabel, templateText } from '../../lib/focus'
  import type { FocusTemplate, FocusTemplateDraftResponse } from '../../lib/types'

  interface Props {
    onNavigate: (path: string) => void
  }

  let { onNavigate }: Props = $props()

  let templates = $state<FocusTemplate[]>([])
  let diagnosticsCount = $state(0)
  let loaded = $state(false)
  let loadError = $state('')
  let status = $state('')

  // The editor panel: 'closed', drafting a new template, or editing/deleting
  // an existing one (baseId names it).
  let mode = $state<'closed' | 'create' | 'edit'>('closed')
  let baseId = $state('')
  let request = $state('')
  let drafting = $state(false)
  let draftError = $state('')
  let draft = $state<FocusTemplateDraftResponse | null>(null)
  let saving = $state(false)
  let saveError = $state('')

  // A quick delete from the list, without going through natural language —
  // an inline two-step confirm rather than a native dialog, so it is as
  // testable as everything else here.
  let confirmingDeleteId = $state('')
  let deleting = $state(false)
  let deleteError = $state('')

  let baseTemplate = $derived(templates.find((item) => item.id === baseId) ?? null)
  let stageDiff = $derived.by(() => (draft?.action === 'save' ? diffFocusTemplateStages(baseTemplate, draft.template ?? null) : []))
  let hasChanges = $derived(draft ? focusTemplateDraftHasChanges(baseTemplate, draft) : false)

  function templateName(item: FocusTemplate): string {
    return (item.builtin && templateText(item.id, $t.focus.templates)?.name) || item.name
  }

  function templateDescription(item: FocusTemplate): string {
    return (item.builtin && templateText(item.id, $t.focus.templates)?.description) || item.description || ''
  }

  function templateStageSummary(item: FocusTemplate): string {
    return item.stages.map((s) => stageLabel({ template: item.builtin ? item.id : undefined, stages: item.stages }, s.id, $t.focus)).join(' → ')
  }

  async function refresh() {
    try {
      const listed = await listFocusTemplates()
      templates = listed.templates
      diagnosticsCount = listed.diagnostics.length
      loadError = ''
    } catch (err) {
      loadError = err instanceof Error ? err.message : String(err)
    } finally {
      loaded = true
    }
  }

  onMount(() => {
    void refresh()
  })

  function openCreate() {
    mode = 'create'
    baseId = ''
    request = ''
    draft = null
    draftError = ''
    status = ''
    confirmingDeleteId = ''
  }

  function openEdit(id: string) {
    mode = 'edit'
    baseId = id
    request = ''
    draft = null
    draftError = ''
    status = ''
    confirmingDeleteId = ''
  }

  function closeEditor() {
    mode = 'closed'
    baseId = ''
    request = ''
    draft = null
    draftError = ''
  }

  async function runDraft() {
    const text = request.trim()
    if (!text || drafting) return
    drafting = true
    draftError = ''
    try {
      draft = await draftFocusTemplate({
        request: text,
        ...(baseId ? { base_id: baseId } : {}),
        ...(draft?.template ? { draft: draft.template } : {}),
      })
      request = ''
    } catch (err) {
      draftError = err instanceof Error ? err.message : String(err)
    } finally {
      drafting = false
    }
  }

  async function confirmDraft() {
    if (!draft || saving) return
    saving = true
    saveError = ''
    try {
      if (draft.action === 'delete' && draft.original_id) {
        await deleteFocusTemplate(draft.original_id)
        status = $t.focus.templateEditor.deleted(draft.original_id)
      } else if (draft.action === 'save' && draft.template) {
        const saved = await saveFocusTemplate(draft.template, draft.original_id)
        status = $t.focus.templateEditor.saved(templateName(saved))
      }
      closeEditor()
      await refresh()
    } catch (err) {
      saveError = err instanceof Error ? err.message : String(err)
    } finally {
      saving = false
    }
  }

  function discardDraft() {
    draft = null
    draftError = ''
  }

  async function deleteRow(item: FocusTemplate) {
    if (deleting) return
    deleting = true
    deleteError = ''
    try {
      await deleteFocusTemplate(item.id)
      status = $t.focus.templateEditor.deleted(templateName(item))
      confirmingDeleteId = ''
      await refresh()
    } catch (err) {
      deleteError = err instanceof Error ? err.message : String(err)
    } finally {
      deleting = false
    }
  }
</script>

<div class="focus-templates" data-testid="focus-templates">
  <header class="templates-header">
    <div>
      <h2>{$t.focus.templateEditor.title}</h2>
      <p class="subtitle">{$t.focus.templateEditor.subtitle}</p>
    </div>
    <div class="header-actions">
      <button type="button" class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/focus')} data-testid="focus-templates-back">{$t.focus.templateEditor.back}</button>
      {#if mode === 'closed'}
        <button type="button" class="btn btn-primary btn-sm" onclick={openCreate} data-testid="focus-templates-new">{$t.focus.templateEditor.newTemplate}</button>
      {/if}
    </div>
  </header>

  {#if loadError}<p class="banner error" data-testid="focus-templates-error">{$t.focus.templateEditor.loadFailed(loadError)}</p>{/if}
  {#if status}<p class="banner success" data-testid="focus-templates-status">{status}</p>{/if}
  {#if deleteError}<p class="banner error" data-testid="focus-templates-delete-error">{$t.focus.templateEditor.deleteFailed(deleteError)}</p>{/if}

  {#if mode !== 'closed'}
    <section class="editor" data-testid="focus-templates-editor">
      <h3>{mode === 'edit' && baseTemplate ? $t.focus.templateEditor.editing(templateName(baseTemplate)) : $t.focus.templateEditor.newTemplate}</h3>

      {#if !draft}
        <label class="label" for="focus-template-request">{$t.focus.templateEditor.requestLabel}</label>
        <textarea
          id="focus-template-request"
          rows="3"
          bind:value={request}
          placeholder={mode === 'edit' ? $t.focus.templateEditor.requestPlaceholder : $t.focus.templateEditor.requestPlaceholderNew}
          data-testid="focus-templates-request"
        ></textarea>
        {#if draftError}<p class="banner error" data-testid="focus-templates-draft-error">{$t.focus.templateEditor.draftFailed(draftError)}</p>{/if}
        <div class="editor-actions">
          <button type="button" class="btn btn-ghost btn-sm" onclick={closeEditor} disabled={drafting}>{$t.focus.templateEditor.cancel}</button>
          <button type="button" class="btn btn-primary btn-sm" disabled={!request.trim() || drafting} onclick={() => void runDraft()} data-testid="focus-templates-draft">
            {drafting ? $t.focus.templateEditor.drafting : $t.focus.templateEditor.draft}
          </button>
        </div>
      {:else}
        <div class="preview" data-testid="focus-templates-preview">
          <strong>{$t.focus.templateEditor.previewTitle}</strong>
          <p data-testid="focus-templates-preview-summary" data-content>{focusTemplateDraftSummary(draft)}</p>
          {#if draft.warnings?.length}
            {#each draft.warnings as warning (warning)}
              <p class="banner warning" data-testid="focus-templates-preview-warning" data-content>{warning}</p>
            {/each}
          {/if}

          {#if draft.action === 'delete'}
            <p class="delete-title">{$t.focus.templateEditor.previewDeleteTitle} <code>{draft.original_id}</code></p>
          {:else if draft.template}
            <ul class="stage-diff" data-testid="focus-templates-preview-stages">
              {#each stageDiff as item (item.id)}
                <li data-testid="focus-templates-preview-stage" data-change={item.change}>
                  <span class="badge badge-{item.change === 'added' ? 'success' : item.change === 'removed' ? 'error' : item.change === 'changed' ? 'warning' : 'default'}">
                    {item.change === 'added' ? $t.focus.templateEditor.stageAdded
                      : item.change === 'removed' ? $t.focus.templateEditor.stageRemoved
                      : item.change === 'changed' ? $t.focus.templateEditor.stageChanged
                      : $t.focus.templateEditor.stageUnchanged}
                  </span>
                  <span class="mono" data-content>{item.after?.label || item.before?.label || item.id}</span>
                </li>
              {/each}
            </ul>
            {#if !hasChanges}<p class="hint">{$t.focus.templateEditor.stageUnchanged}</p>{/if}
          {/if}

          <label class="label" for="focus-template-refine">{$t.focus.templateEditor.requestLabel}</label>
          <textarea
            id="focus-template-refine"
            rows="2"
            bind:value={request}
            placeholder={$t.focus.templateEditor.refinePlaceholder}
            data-testid="focus-templates-refine-request"
          ></textarea>
          {#if draftError}<p class="banner error" data-testid="focus-templates-draft-error">{$t.focus.templateEditor.draftFailed(draftError)}</p>{/if}
          {#if saveError}<p class="banner error" data-testid="focus-templates-save-error">{$t.focus.templateEditor.saveFailed(saveError)}</p>{/if}

          <div class="editor-actions">
            <button type="button" class="btn btn-ghost btn-sm" onclick={discardDraft} disabled={saving}>{$t.focus.templateEditor.discard}</button>
            <button type="button" class="btn btn-secondary btn-sm" disabled={!request.trim() || drafting} onclick={() => void runDraft()} data-testid="focus-templates-refine">
              {drafting ? $t.focus.templateEditor.drafting : $t.focus.templateEditor.refine}
            </button>
            <button type="button" class="btn btn-primary btn-sm" disabled={saving} onclick={() => void confirmDraft()} data-testid="focus-templates-save">
              {saving ? $t.focus.templateEditor.saving : draft.action === 'delete' ? $t.focus.templateEditor.confirmDelete : $t.focus.templateEditor.save}
            </button>
          </div>
        </div>
      {/if}
    </section>
  {/if}

  {#if loaded && templates.length === 0}
    <p class="empty" data-testid="focus-templates-empty">{$t.focus.templateEditor.empty}</p>
  {/if}

  <ul class="templates-list" data-testid="focus-templates-list">
    {#each templates as item (item.id)}
      <li class="template-row" data-testid="focus-templates-row" data-template-id={item.id}>
        <div class="template-main">
          <span class="template-name" data-content>{templateName(item)}</span>
          <span class="badge badge-{item.builtin ? 'default' : 'accent'}">{item.builtin ? $t.focus.templateEditor.builtin : $t.focus.templateEditor.custom}</span>
        </div>
        {#if templateDescription(item)}<p class="template-description" data-content>{templateDescription(item)}</p>{/if}
        <p class="template-stages mono" data-content>{$t.focus.templateEditor.stages(templateStageSummary(item))}</p>
        <div class="template-actions">
          <button type="button" class="btn btn-ghost btn-sm" onclick={() => openEdit(item.id)} data-testid="focus-templates-edit">{$t.focus.templateEditor.edit}</button>
          {#if item.builtin}
            <span class="hint" title={$t.focus.templateEditor.cannotDeleteBuiltin}>{$t.focus.templateEditor.cannotDeleteBuiltin}</span>
          {:else if confirmingDeleteId === item.id}
            <span class="confirm-delete">
              <span class="hint">{$t.focus.templateEditor.deleteConfirm(templateName(item))}</span>
              <button type="button" class="btn btn-ghost btn-sm" onclick={() => { confirmingDeleteId = '' }} disabled={deleting}>{$t.focus.templateEditor.cancel}</button>
              <button type="button" class="btn btn-danger btn-sm" onclick={() => void deleteRow(item)} disabled={deleting} data-testid="focus-templates-confirm-delete">
                {$t.focus.templateEditor.confirmDelete}
              </button>
            </span>
          {:else}
            <button type="button" class="btn btn-ghost btn-sm" onclick={() => { confirmingDeleteId = item.id; deleteError = '' }} data-testid="focus-templates-delete">
              {$t.focus.templateEditor.delete}
            </button>
          {/if}
        </div>
      </li>
    {/each}
  </ul>
</div>

<style>
  .focus-templates {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 880px;
    margin: 0 auto;
    padding: var(--space-6) var(--space-6) var(--space-10);
  }

  .templates-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .header-actions {
    display: flex;
    gap: var(--space-2);
  }

  h2 {
    margin: 0;
  }

  .subtitle {
    margin: var(--space-1) 0 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .editor {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-4);
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
  }

  .editor h3 {
    margin: 0 0 var(--space-1);
  }

  .label {
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  textarea {
    width: 100%;
    resize: vertical;
    font: inherit;
  }

  .editor-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }

  .preview {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--primary);
    border-radius: var(--radius-md);
    background: var(--primary-muted);
  }

  .preview p {
    margin: 0;
  }

  .delete-title {
    color: var(--text-secondary);
  }

  .stage-diff {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .stage-diff li {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .hint {
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .templates-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .template-row {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding: var(--space-3) var(--space-4);
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
  }

  .template-main {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .template-name {
    font-weight: 600;
  }

  .template-description,
  .template-stages {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }

  .template-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-1);
  }

  .confirm-delete {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .empty {
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .banner {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .banner.error {
    background: var(--error-muted);
    color: var(--error);
  }

  .banner.warning {
    background: var(--warning-muted);
    color: var(--warning);
  }

  .banner.success {
    background: var(--success-muted);
    color: var(--success);
  }
</style>
