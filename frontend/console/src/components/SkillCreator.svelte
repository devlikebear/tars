<script lang="ts">
  import { draftSkill, saveLocalSkill, submitSkillDraftPR, testSkillDraft } from '../lib/api'
  import type {
    SkillCreatorDraftResponse,
    SkillCreatorFile,
    SkillCreatorSaveResponse,
    SkillCreatorTestResponse,
  } from '../lib/types'

  type Language = 'python' | 'typescript' | 'shell'
  type Layout = 'single_file' | 'directory'

  interface Props {
    onclose?: () => void
    onsaved?: (result: SkillCreatorSaveResponse) => void
  }

  let { onclose, onsaved }: Props = $props()

  let name = $state('')
  let description = $state('')
  let category = $state('')
  let language: Language = $state('python')
  let layout: Layout = $state('single_file')
  let useCase = $state('')
  let toolText = $state('bash')
  let draft: SkillCreatorDraftResponse | null = $state(null)
  let selectedFilePath = $state('SKILL.md')
  let busy = $state(false)
  let saving = $state(false)
  let testing = $state(false)
  let submitting = $state(false)
  let error = $state('')
  let message = $state('')
  let testResult: SkillCreatorTestResponse | null = $state(null)

  const languages: { value: Language; label: string }[] = [
    { value: 'python', label: 'Python' },
    { value: 'typescript', label: 'TypeScript' },
    { value: 'shell', label: 'Shell' },
  ]
  const layouts: { value: Layout; label: string }[] = [
    { value: 'single_file', label: 'Single file' },
    { value: 'directory', label: 'Directory' },
  ]
  const toolOptions = ['bash', 'web_fetch', 'memory_search']

  let files: SkillCreatorFile[] = $derived.by(() => draft ? draft.files : [])
  let selectedFile: SkillCreatorFile | undefined = $derived.by(() => files.find((file: SkillCreatorFile) => file.path === selectedFilePath) ?? files[0])

  function setTool(tool: string, checked: boolean) {
    const tools = new Set(parseTools())
    if (checked) tools.add(tool)
    else tools.delete(tool)
    toolText = [...tools].join(', ')
  }

  function parseTools(): string[] {
    return toolText.split(/[\n,]/).map((tool) => tool.trim()).filter(Boolean)
  }

  async function generateDraft() {
    busy = true
    error = ''
    message = ''
    try {
      draft = await draftSkill({
        name: name.trim(),
        description: description.trim(),
        category: category.trim(),
        language,
        layout,
        use_case: useCase.trim(),
        recommended_tools: parseTools(),
      })
      selectedFilePath = draft.files[0]?.path ?? 'SKILL.md'
      toolText = draft.recommended_tools.join(', ')
      testResult = null
      message = 'Draft ready.'
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to draft skill'
    } finally {
      busy = false
    }
  }

  function updateSelectedFile(content: string) {
    if (!draft || !selectedFile) return
    draft = {
      ...draft,
      files: draft.files.map((file: SkillCreatorFile) => (
        file.path === selectedFile.path ? { ...file, content } : file
      )),
    }
  }

  async function saveDraft() {
    if (!draft) return
    saving = true
    error = ''
    message = ''
    try {
      const saved = await saveLocalSkill(draft)
      message = `Saved to ${saved.path}`
      onsaved?.(saved)
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to save skill'
    } finally {
      saving = false
    }
  }

  async function runSandboxTest() {
    if (!draft) return
    testing = true
    error = ''
    message = ''
    testResult = null
    try {
      testResult = await testSkillDraft(draft)
      message = testResult.success ? 'Sandbox test passed.' : 'Sandbox test failed.'
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to test skill'
    } finally {
      testing = false
    }
  }

  async function submitDraft() {
    if (!draft) return
    submitting = true
    error = ''
    message = ''
    try {
      const result = await submitSkillDraftPR(draft.name)
      message = result.message
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to prepare draft PR'
    } finally {
      submitting = false
    }
  }
</script>

<div class="creator-backdrop" role="presentation">
  <div class="creator-modal" role="dialog" aria-modal="true" aria-labelledby="skill-creator-title">
    <header class="creator-header">
      <div>
        <h3 id="skill-creator-title">Create Skill</h3>
        <span class="creator-subtitle">SKILL.md + companion CLI</span>
      </div>
      <button class="creator-close" type="button" aria-label="Close" onclick={onclose}>&times;</button>
    </header>

    {#if error}
      <div class="message message-error">{error}</div>
    {/if}
    {#if message}
      <div class="message message-success">{message}</div>
    {/if}

    <div class="creator-body">
      <section class="creator-form-panel">
        <form class="creator-form" onsubmit={(event) => { event.preventDefault(); void generateDraft() }}>
          <label>
            <span>Name</span>
            <input bind:value={name} placeholder="docker-log-shipper" autocomplete="off" />
          </label>
          <label>
            <span>Description</span>
            <input bind:value={description} placeholder="Collect recent Docker ERROR logs" />
          </label>
          <label>
            <span>Category</span>
            <input bind:value={category} placeholder="system" />
          </label>

          <div class="field-group">
            <span>Language</span>
            <div class="segmented">
              {#each languages as option}
                <button type="button" class:active={language === option.value} onclick={() => { language = option.value }}>{option.label}</button>
              {/each}
            </div>
          </div>

          <div class="field-group">
            <span>Layout</span>
            <div class="segmented">
              {#each layouts as option}
                <button type="button" class:active={layout === option.value} onclick={() => { layout = option.value }}>{option.label}</button>
              {/each}
            </div>
          </div>

          <label>
            <span>Use Case</span>
            <textarea bind:value={useCase} rows="4" placeholder="Extract ERROR logs from Docker containers and send them to Slack."></textarea>
          </label>

          <div class="field-group">
            <span>Recommended Tools</span>
            <div class="tool-row">
              {#each toolOptions as tool}
                <label class="tool-check">
                  <input type="checkbox" checked={parseTools().includes(tool)} onchange={(event) => setTool(tool, event.currentTarget.checked)} />
                  <span>{tool}</span>
                </label>
              {/each}
            </div>
            <input bind:value={toolText} placeholder="bash, web_fetch" />
          </div>

          <div class="creator-actions">
            <button class="btn btn-primary btn-sm" type="submit" disabled={busy}>
              {busy ? 'Drafting...' : 'Draft'}
            </button>
            <button class="btn btn-ghost btn-sm" type="button" disabled={!draft || saving} onclick={saveDraft}>
              {saving ? 'Saving...' : 'Save Local'}
            </button>
            <button class="btn btn-ghost btn-sm" type="button" disabled={!draft || testing} onclick={runSandboxTest}>
              {testing ? 'Testing...' : 'Test'}
            </button>
            <button class="btn btn-ghost btn-sm" type="button" disabled={!draft || submitting} onclick={submitDraft}>
              {submitting ? 'Preparing...' : 'Submit Draft PR'}
            </button>
          </div>
        </form>
      </section>

      <section class="creator-preview-panel">
        <div class="creator-preview">
          <div class="preview-tabs">
            {#if files.length === 0}
              <span class="preview-empty">No draft yet</span>
            {:else}
              {#each files as file}
                <button type="button" class:active={selectedFilePath === file.path} onclick={() => { selectedFilePath = file.path }}>
                  {file.path}
                </button>
              {/each}
            {/if}
          </div>
          {#if selectedFile}
            <textarea
              class="preview-editor"
              value={selectedFile.content}
              oninput={(event) => updateSelectedFile(event.currentTarget.value)}
              spellcheck="false"
            ></textarea>
          {:else}
            <div class="preview-placeholder">Draft output appears here.</div>
          {/if}
          {#if testResult}
            <div class="test-result" class:failed={!testResult.success}>
              <div class="test-summary">
                <span class="badge {testResult.success ? 'badge-success' : 'badge-error'}">{testResult.success ? 'pass' : 'fail'}</span>
                <span>exit {testResult.exit_code}</span>
                <span>{testResult.duration_ms}ms</span>
                <span>{testResult.session_kind}{testResult.hidden ? ' hidden' : ''}</span>
              </div>
              <div class="test-output-grid">
                <div class="test-output">
                  <span>stdout</span>
                  <pre>{testResult.stdout || '(empty)'}</pre>
                </div>
                <div class="test-output">
                  <span>stderr</span>
                  <pre>{testResult.stderr || '(empty)'}</pre>
                </div>
              </div>
              <div class="tool-trail">
                <span class="trail-title">Tool Trail</span>
                {#each testResult.tool_trail as item}
                  <code>{item.tool}: {item.command}</code>
                {/each}
              </div>
            </div>
          {/if}
        </div>
      </section>
    </div>
  </div>
</div>

<style>
  .creator-backdrop {
    position: fixed;
    inset: 0;
    z-index: 50;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-8) var(--space-4);
    background: rgba(10, 10, 10, 0.62);
    overflow: auto;
  }

  .creator-modal {
    width: min(1080px, 100%);
    height: min(760px, calc(100vh - var(--space-8) * 2));
    max-height: calc(100vh - var(--space-8) * 2);
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--surface-elevated);
    box-shadow: var(--shadow-lg);
    overflow: hidden;
  }

  .creator-header {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
    align-items: flex-start;
  }

  .creator-header h3 {
    margin: 0;
    font-size: var(--text-xl);
  }

  .creator-subtitle {
    display: block;
    margin-top: 2px;
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }

  .creator-close {
    border: none;
    background: transparent;
    color: var(--text-ghost);
    cursor: pointer;
    font-size: 22px;
    line-height: 1;
  }
  .creator-close:hover { color: var(--text-primary); }

  .creator-body {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: minmax(300px, 380px) minmax(420px, 1fr);
    gap: var(--space-4);
    overflow: hidden;
  }

  .creator-form-panel,
  .creator-preview-panel {
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .creator-form,
  .creator-preview {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .creator-form {
    overflow-y: auto;
    padding-right: var(--space-1);
  }

  .creator-preview {
    overflow: hidden;
  }

  label,
  .field-group {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }

  input,
  textarea {
    width: 100%;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
    color: var(--text-primary);
    padding: var(--space-2);
    font: inherit;
  }

  textarea {
    resize: vertical;
    line-height: 1.45;
  }

  .segmented {
    display: flex;
    flex-wrap: wrap;
    gap: 2px;
    padding: 2px;
    border-radius: var(--radius-md);
    background: var(--surface);
  }

  .segmented button,
  .preview-tabs button {
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
    font-size: var(--text-xs);
    padding: var(--space-1) var(--space-2);
  }

  .segmented button.active,
  .preview-tabs button.active {
    background: var(--primary);
    color: #fff;
  }

  .tool-row {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .tool-check {
    flex-direction: row;
    align-items: center;
    gap: var(--space-1);
  }

  .tool-check input {
    width: auto;
  }

  .creator-actions {
    position: sticky;
    bottom: 0;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    padding-top: var(--space-2);
    background: var(--surface-elevated);
  }

  .preview-tabs {
    display: flex;
    min-height: 32px;
    align-items: center;
    gap: var(--space-1);
    overflow-x: auto;
  }

  .preview-empty,
  .preview-placeholder {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .preview-editor,
  .preview-placeholder {
    min-height: 0;
    flex: 1;
  }

  .preview-editor {
    resize: none;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    white-space: pre;
    overflow: auto;
  }

  .preview-placeholder {
    display: grid;
    place-items: center;
    border: 1px dashed var(--border-subtle);
    border-radius: var(--radius-md);
  }

  .test-result {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: rgba(255, 255, 255, 0.02);
  }
  .test-result.failed { border-color: rgba(220, 60, 60, 0.4); }
  .test-summary {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }
  .test-output-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-2);
  }
  .test-output-grid pre {
    min-height: 72px;
    max-height: 160px;
    margin: 0;
    overflow: auto;
    white-space: pre-wrap;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: var(--space-2);
    background: var(--surface);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  .tool-trail {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }
  .trail-title { color: var(--text-primary); font-weight: 600; }
  .tool-trail code {
    overflow-wrap: anywhere;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: var(--space-1) var(--space-2);
    background: var(--surface);
  }

  .message {
    font-size: var(--text-sm);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
  }
  .message-error { background: rgba(220, 60, 60, 0.15); color: var(--red); border: 1px solid rgba(220, 60, 60, 0.3); }
  .message-success { background: rgba(60, 180, 100, 0.15); color: var(--green); border: 1px solid rgba(60, 180, 100, 0.3); }

  @media (max-width: 860px) {
    .creator-backdrop { align-items: flex-start; padding: var(--space-3); }
    .creator-modal { height: auto; max-height: none; }
    .creator-body { grid-template-columns: 1fr; overflow: visible; }
    .creator-form-panel,
    .creator-preview-panel { overflow: visible; }
    .creator-form { overflow: visible; padding-right: 0; }
    .test-output-grid { grid-template-columns: 1fr; }
    .preview-editor,
    .preview-placeholder { min-height: 300px; }
  }
</style>
