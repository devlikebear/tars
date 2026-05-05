<script lang="ts">
  import { draftMCPServer, saveLocalMCPServer, submitMCPServerDraftPR, testMCPServerDraft } from '../lib/api'
  import type {
    MCPServerCreatorConversationMessage,
    MCPServerCreatorDraftResponse,
    MCPServerCreatorFile,
    MCPServerCreatorSaveResponse,
    MCPServerCreatorTestResponse,
    MCPServerCreatorToolSpec,
  } from '../lib/types'

  type Language = 'python' | 'node'

  interface Props {
    onclose?: () => void
    onsaved?: (result: MCPServerCreatorSaveResponse) => void
  }

  let { onclose, onsaved }: Props = $props()

  let naturalPrompt = $state('')
  let useLLM = $state(true)
  let conversation: MCPServerCreatorConversationMessage[] = $state([])
  let name = $state('')
  let description = $state('')
  let language: Language = $state('python')
  let useCase = $state('')
  let toolsJSON = $state('')
  let draft: MCPServerCreatorDraftResponse | null = $state(null)
  let selectedFilePath = $state('tars.mcp.json')
  let busy = $state(false)
  let saving = $state(false)
  let testing = $state(false)
  let submitting = $state(false)
  let draftAttempted = $state(false)
  let error = $state('')
  let message = $state('')
  let testResult: MCPServerCreatorTestResponse | null = $state(null)

  const languages: { value: Language; label: string }[] = [
    { value: 'python', label: 'Python FastMCP' },
    { value: 'node', label: 'Node MCP SDK' },
  ]
  const draftNamePattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/
  const stdioProbeSteps = ['tools/list', 'tools/call']
  const toolPlaceholder = '[{"name":"get_time","description":"Return current time"}]'

  let files: MCPServerCreatorFile[] = $derived.by(() => draft ? draft.files : [])
  let selectedFile: MCPServerCreatorFile | undefined = $derived.by(() => files.find((file: MCPServerCreatorFile) => file.path === selectedFilePath) ?? files[0])
  let draftBlockedReason: string = $derived.by(() => validateDraftInput())
  let showDraftHint: boolean = $derived.by(() => {
    if (!draftBlockedReason) return false
    if (draftAttempted) return true
    return name.trim() !== '' && draftBlockedReason !== 'Name is required.'
  })

  function validateDraftInput(): string {
    if (naturalPrompt.trim()) return ''
    const trimmedName = name.trim()
    if (!trimmedName) return 'Name is required.'
    if (!draftNamePattern.test(trimmedName)) {
      return 'Name must be kebab-case using lowercase letters, numbers, and dashes.'
    }
    if (!description.trim()) return 'Description is required.'
    if (!useCase.trim()) return 'Use case is required.'
    return ''
  }

  function parseToolSpecs(): MCPServerCreatorToolSpec[] {
    const raw = toolsJSON.trim()
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) throw new Error('Tool signatures must be a JSON array.')
    return parsed.map((item) => ({
      name: String(item?.name ?? '').trim(),
      description: String(item?.description ?? '').trim(),
      input_schema: item?.input_schema,
      output_schema: item?.output_schema,
    })).filter((tool) => tool.name)
  }

  function appendConversation(role: 'user' | 'assistant', content: string): MCPServerCreatorConversationMessage[] {
    const trimmed = content.trim()
    if (!trimmed) return conversation
    conversation = [...conversation, { role, content: trimmed }]
    return conversation
  }

  function draftStatusMessage(draft: MCPServerCreatorDraftResponse): string {
    const source = draft.draft_source ? `${draft.draft_source} draft ready.` : 'Draft ready.'
    const warnings = draft.warnings?.length ? ` ${draft.warnings.join(' ')}` : ''
    return `${source}${warnings}`
  }

  async function generateDraft() {
    draftAttempted = true
    const blockReason = validateDraftInput()
    if (blockReason) {
      error = blockReason
      message = ''
      return
    }
    busy = true
    error = ''
    message = ''
    try {
      const tools = parseToolSpecs()
      const requestConversation = naturalPrompt.trim()
        ? appendConversation('user', naturalPrompt.trim())
        : conversation
      draft = await draftMCPServer({
        prompt: naturalPrompt.trim(),
        conversation: requestConversation,
        use_llm: useLLM,
        name: name.trim(),
        description: description.trim(),
        language,
        use_case: useCase.trim(),
        ...(tools.length ? { tools } : {}),
      })
      name = draft.name
      description = draft.description
      language = draft.language
      useCase = draft.use_case
      selectedFilePath = draft.files[0]?.path ?? 'tars.mcp.json'
      toolsJSON = JSON.stringify(draft.tools, null, 2)
      testResult = null
      if (draft.assistant_message) {
        appendConversation('assistant', draft.assistant_message)
      }
      naturalPrompt = ''
      message = draftStatusMessage(draft)
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to draft MCP server'
    } finally {
      busy = false
    }
  }

  function updateSelectedFile(content: string) {
    if (!draft || !selectedFile) return
    draft = {
      ...draft,
      files: draft.files.map((file: MCPServerCreatorFile) => (
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
      const saved = await saveLocalMCPServer(draft)
      message = `Saved to ${saved.path}`
      onsaved?.(saved)
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to save MCP server'
    } finally {
      saving = false
    }
  }

  async function runStdioValidation() {
    if (!draft) return
    testing = true
    error = ''
    message = ''
    testResult = null
    try {
      testResult = await testMCPServerDraft(draft)
      if (testResult.success) {
        message = 'stdio validation passed.'
      } else {
        error = 'stdio validation failed.'
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to test MCP server'
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
      const result = await submitMCPServerDraftPR(draft.name)
      message = result.message
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to prepare draft PR'
    } finally {
      submitting = false
    }
  }
</script>

<div class="creator-backdrop" role="presentation">
  <div class="creator-modal" role="dialog" aria-modal="true" aria-labelledby="mcp-creator-title">
    <header class="creator-header">
      <div>
        <h3 id="mcp-creator-title">Create MCP Server</h3>
        <span class="creator-subtitle">tars.mcp.json + stdio server stub</span>
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
          <label class="prompt-field">
            <span>Builder Prompt</span>
            <textarea
              bind:value={naturalPrompt}
              rows="4"
              placeholder="예: 로컬 시간을 안전한 JSON으로 반환하는 get_time MCP 서버를 만들어줘."
            ></textarea>
          </label>

          <div class="builder-options">
            <label class="toggle-row">
              <input type="checkbox" bind:checked={useLLM} />
              <span>Generate with LLM</span>
            </label>
          </div>

          {#if conversation.length}
            <div class="conversation-log" aria-label="MCP builder conversation">
              {#each conversation as item}
                <div class="conversation-item {item.role}">
                  <span>{item.role}</span>
                  <p>{item.content}</p>
                </div>
              {/each}
            </div>
          {/if}

          <label>
            <span>Name</span>
            <input
              bind:value={name}
              placeholder="safe-time"
              autocomplete="off"
              aria-invalid={showDraftHint ? 'true' : 'false'}
              aria-describedby={showDraftHint ? 'mcp-creator-name-hint' : undefined}
            />
            {#if showDraftHint}
              <small id="mcp-creator-name-hint" class="field-hint">{draftBlockedReason}</small>
            {/if}
          </label>
          <label>
            <span>Description</span>
            <input bind:value={description} placeholder="Expose safe local time utilities" />
          </label>

          <div class="field-group">
            <span>Language</span>
            <div class="segmented">
              {#each languages as option}
                <button type="button" class:active={language === option.value} onclick={() => { language = option.value }}>{option.label}</button>
              {/each}
            </div>
          </div>

          <label>
            <span>Use Case</span>
            <textarea bind:value={useCase} rows="4" placeholder="Return the current local time in a safe structured format."></textarea>
          </label>

          <label>
            <span>Tool Signatures</span>
            <textarea
              bind:value={toolsJSON}
              rows="7"
              spellcheck="false"
              placeholder={toolPlaceholder}
            ></textarea>
          </label>

          <div class="creator-actions">
            <button class="btn btn-primary btn-sm" type="submit" disabled={busy}>
              {busy ? 'Drafting...' : useLLM ? 'Generate with LLM' : 'Draft'}
            </button>
            <button class="btn btn-ghost btn-sm" type="button" disabled={!draft || saving} onclick={saveDraft}>
              {saving ? 'Saving...' : 'Save Local'}
            </button>
            <button class="btn btn-ghost btn-sm" type="button" disabled={!draft || testing} onclick={runStdioValidation}>
              {testing ? 'Testing...' : 'Test stdio'}
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
              <div class="protocol-row">
                {#each (testResult.protocol_steps.length ? testResult.protocol_steps : stdioProbeSteps) as step}
                  <code>{step}</code>
                {/each}
              </div>
              <div class="test-output-grid">
                <div class="test-output">
                  <span>Tools</span>
                  <pre>{testResult.tools.length ? testResult.tools.join('\n') : '(none)'}</pre>
                </div>
                <div class="test-output">
                  <span>Call Result</span>
                  <pre>{testResult.call_result || testResult.stderr || '(empty)'}</pre>
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

  .prompt-field textarea {
    min-height: 108px;
  }

  .builder-options {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .toggle-row {
    flex-direction: row;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-primary);
  }

  .toggle-row input {
    width: auto;
    accent-color: var(--primary);
  }

  .conversation-log {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    max-height: 160px;
    overflow: auto;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    padding: var(--space-2);
    background: rgba(255, 255, 255, 0.02);
  }

  .conversation-item {
    display: grid;
    grid-template-columns: 64px 1fr;
    gap: var(--space-2);
    align-items: start;
  }

  .conversation-item span {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    text-transform: uppercase;
  }

  .conversation-item p {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--text-xs);
    line-height: 1.45;
    overflow-wrap: anywhere;
  }

  .conversation-item.assistant p {
    color: var(--text-primary);
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

  .field-hint {
    color: var(--error);
    font-size: var(--text-xs);
    line-height: 1.35;
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
  .test-summary,
  .protocol-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }
  .protocol-row code {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: var(--space-1) var(--space-2);
    background: var(--surface);
    color: var(--text-primary);
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
