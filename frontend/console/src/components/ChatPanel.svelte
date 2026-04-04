<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { streamChat, cancelChat, listSessions, getSessionHistory, renameSession } from '../lib/api'
  import type { ChatAttachment, ChatEvent, SessionMessage } from '../lib/types'
  import { extractArtifact, extractArtifactsFromHistory, type Artifact } from '../lib/artifacts'
  import MarkdownContent from './MarkdownContent.svelte'

  type ChatMessage = {
    id: string
    role: 'user' | 'assistant' | 'system' | 'error' | 'tool'
    text: string
    toolName?: string
    toolCallId?: string
    toolArgs?: string
    toolResult?: string
    toolDone?: boolean
    usage?: { input_tokens: number; output_tokens: number; cached_tokens: number; cache_read_tokens: number; cache_write_tokens: number }
  }

  interface Props {
    projectId?: string
    sessionId?: string
    initialPrompt?: string
    autoSend?: boolean
    onSessionChange?: () => void
    onArtifactsChange?: (artifacts: Artifact[]) => void
  }

  let { projectId, sessionId, initialPrompt, autoSend, onSessionChange, onArtifactsChange }: Props = $props()

  let artifacts: Artifact[] = $state([])

  let chatInput = $state('')
  let chatBusy = $state(false)
  let chatError = $state('')
  let chatSessionId = $state('')
  let chatStatusLine = $state('')
  let chatMessages: ChatMessage[] = $state([])
  let autoTitled = $state(false)
  let autoSendDone = false
  let abortController: AbortController | null = $state(null)
  let contextInfo: { system_prompt_tokens?: number; history_tokens?: number; history_messages?: number; tool_count?: number; memory_count?: number; memory_tokens?: number; tool_names?: string[] } = $state({})

  // One-shot auto-send: fires once when autoSend becomes true with a prompt
  $effect(() => {
    if (autoSend && initialPrompt && !autoSendDone && !chatBusy) {
      autoSendDone = true
      chatInput = initialPrompt
      tick().then(() => submitChat())
    }
  })

  let chatLogEl: HTMLDivElement | undefined = $state()
  let autoScroll = $state(true)

  async function loadProjectSession() {
    try {
      const all = await listSessions()
      const projectSession = projectId ? all.find((s) => s.project_id === projectId) : null
      if (projectSession) {
        chatSessionId = projectSession.id
        chatMessages = [{ id: 'system-init', role: 'system', text: `Session: ${projectSession.id.slice(0, 8)}...` }]
        const history = await getSessionHistory(projectSession.id)
        for (const msg of history) {
          if (msg.role === 'system' && (msg.content.startsWith('[HEARTBEAT]') || msg.content.startsWith('[COMPACTION SUMMARY]') || msg.content.startsWith('[CRON]'))) {
            continue
          }
          if (msg.role === 'tool') {
            chatMessages.push({
              id: `tool-${msg.tool_call_id || Date.now()}`,
              role: 'tool',
              text: '',
              toolName: msg.tool_name,
              toolCallId: msg.tool_call_id,
              toolArgs: msg.tool_args,
              toolResult: msg.content,
              toolDone: true,
            })
          } else {
            chatMessages.push({
              id: `hist-${chatMessages.length}`,
              role: msg.role as ChatMessage['role'],
              text: msg.content,
            })
          }
        }
        chatMessages = [...chatMessages]
        artifacts = extractArtifactsFromHistory(chatMessages)
        if (artifacts.length > 0) onArtifactsChange?.(artifacts)
        autoTitled = true
        void scrollToBottom()
      }
    } catch { /* ignore */ }
  }

  function handleScroll() {
    if (!chatLogEl) return
    const threshold = 40
    autoScroll = chatLogEl.scrollTop + chatLogEl.clientHeight >= chatLogEl.scrollHeight - threshold
  }

  async function scrollToBottom() {
    if (!autoScroll || !chatLogEl) return
    await tick()
    chatLogEl.scrollTop = chatLogEl.scrollHeight
  }

  function handleChatEvent(event: ChatEvent, assistantId: string) {
    switch (event.type) {
      case 'status':
        if (event.phase === 'before_tool_call' && event.tool_name) {
          const toolMsg: ChatMessage = {
            id: `tool-${event.tool_call_id || Date.now()}`,
            role: 'tool',
            text: '',
            toolName: event.tool_name,
            toolCallId: event.tool_call_id,
            toolArgs: event.tool_args_preview,
            toolDone: false,
          }
          const aIdx = chatMessages.findIndex((m) => m.id === assistantId)
          if (aIdx >= 0) {
            chatMessages.splice(aIdx, 0, toolMsg)
            chatMessages = [...chatMessages]
            void scrollToBottom()
          }
        } else if (event.phase === 'after_tool_call' && event.tool_call_id) {
          const tIdx = chatMessages.findIndex((m) => m.toolCallId === event.tool_call_id)
          if (tIdx >= 0) {
            chatMessages[tIdx] = {
              ...chatMessages[tIdx],
              toolResult: event.tool_result_preview,
              toolDone: true,
            }
            chatMessages = [...chatMessages]
            void scrollToBottom()

            // Track artifacts
            const artifact = extractArtifact(
              event.tool_name || '',
              event.tool_call_id,
              event.tool_args_preview,
              event.tool_result_preview,
            )
            if (artifact) {
              // Update existing or add new
              const existing = artifacts.findIndex((a) => a.path === artifact.path)
              if (existing >= 0) {
                artifacts[existing] = artifact
              } else {
                artifacts = [...artifacts, artifact]
              }
              onArtifactsChange?.(artifacts)
            }
          }
        } else if (event.phase === 'skill_selected' && event.skill_name) {
          const skillMsg: ChatMessage = {
            id: `skill-${Date.now()}`,
            role: 'system',
            text: `skill selected: ${event.skill_name}`,
          }
          const aIdx = chatMessages.findIndex((m) => m.id === assistantId)
          if (aIdx >= 0) {
            chatMessages.splice(aIdx, 0, skillMsg)
            chatMessages = [...chatMessages]
          }
        }
        chatStatusLine = [event.phase, event.message, event.tool_name, event.skill_name]
          .filter(Boolean).join(' \u00b7 ')
        break
      case 'delta': {
        const chunk = event.text ?? ''
        if (!chunk) break
        const idx = chatMessages.findIndex((m) => m.id === assistantId)
        if (idx >= 0) {
          chatMessages[idx] = { ...chatMessages[idx], text: chatMessages[idx].text + chunk }
          chatMessages = [...chatMessages]
          void scrollToBottom()
        }
        break
      }
      case 'context_info':
        contextInfo = {
          system_prompt_tokens: event.system_prompt_tokens,
          history_tokens: event.history_tokens,
          history_messages: event.history_messages,
          tool_count: event.tool_count,
          memory_count: event.memory_count,
          memory_tokens: event.memory_tokens,
          tool_names: event.tool_names,
        }
        break
      case 'done': {
        chatSessionId = event.session_id?.trim() || chatSessionId
        chatStatusLine = 'done'
        // Attach usage to assistant message
        if (event.usage) {
          const aIdx = chatMessages.findIndex((m) => m.id === assistantId)
          if (aIdx >= 0) {
            chatMessages[aIdx] = { ...chatMessages[aIdx], usage: event.usage }
            chatMessages = [...chatMessages]
          }
        }
        // Auto-title: use first user message as session title for new sessions
        if (chatSessionId && !autoTitled) {
          autoTitled = true
          const firstUser = chatMessages.find((m) => m.role === 'user')
          if (firstUser?.text) {
            const title = firstUser.text.slice(0, 60).trim() + (firstUser.text.length > 60 ? '...' : '')
            renameSession(chatSessionId, title).catch(() => {})
          }
        }
        onSessionChange?.()
        break
      }
      case 'cancelled':
        chatStatusLine = 'cancelled'
        onSessionChange?.()
        break
      case 'error':
        chatError = event.error?.trim() || 'Stream failed'
        break
    }
  }

  async function submitChat() {
    const message = chatInput.trim()
    if (!message || chatBusy) return
    chatBusy = true
    chatError = ''
    chatStatusLine = 'connecting'
    chatInput = ''
    autoScroll = true

    const currentFiles = [...attachedFiles]
    attachedFiles = []

    const fileLabel = currentFiles.length > 0
      ? ` [${currentFiles.map((f) => f.name).join(', ')}]`
      : ''
    const userId = `user-${Date.now()}`
    const assistantId = `assistant-${Date.now()}`
    chatMessages = [
      ...chatMessages,
      { id: userId, role: 'user', text: message + fileLabel },
      { id: assistantId, role: 'assistant', text: '' },
    ]
    void scrollToBottom()
    const ac = new AbortController()
    abortController = ac
    try {
      const chatAttachments = currentFiles.length > 0 ? await filesToAttachments(currentFiles) : undefined
      await streamChat(
        {
          message,
          session_id: chatSessionId || 'new',
          project_id: projectId || undefined,
          attachments: chatAttachments,
        },
        (event) => handleChatEvent(event, assistantId),
        ac.signal,
      )
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        // User cancelled — no error to show
      } else {
        chatError = err instanceof Error ? err.message : 'Failed to send'
        chatMessages = [...chatMessages, { id: `error-${Date.now()}`, role: 'error', text: chatError }]
      }
    } finally {
      abortController = null
      chatBusy = false
      void scrollToBottom()
    }
  }

  async function handleCancel() {
    if (chatSessionId) {
      await cancelChat(chatSessionId)
    }
    abortController?.abort()
  }

  // -- File attachments --
  let attachedFiles: File[] = $state([])
  let fileInputEl: HTMLInputElement | undefined = $state()
  let isDragging = $state(false)
  let filePreviews: Map<string, string> = $state(new Map()) // file name → preview URL or text

  function addFiles(files: FileList | File[]) {
    for (const file of files) {
      if (attachedFiles.length >= 5) break
      attachedFiles = [...attachedFiles, file]
      generatePreview(file)
    }
  }

  function generatePreview(file: File) {
    const key = `${file.name}-${file.size}-${file.lastModified}`
    if (file.type.startsWith('image/')) {
      const url = URL.createObjectURL(file)
      filePreviews = new Map([...filePreviews, [key, url]])
    } else if (file.type.startsWith('text/') || /\.(txt|md|json|csv|yaml|yml|ts|js|py|go)$/i.test(file.name)) {
      file.slice(0, 500).text().then((text) => {
        const lines = text.split('\n').slice(0, 3).join('\n')
        filePreviews = new Map([...filePreviews, [key, lines]])
      }).catch(() => {})
    }
  }

  function getPreviewKey(file: File): string {
    return `${file.name}-${file.size}-${file.lastModified}`
  }

  function handleFileSelect(e: Event) {
    const input = e.target as HTMLInputElement
    if (!input.files) return
    addFiles(input.files)
    input.value = ''
  }

  function removeAttachment(index: number) {
    const file = attachedFiles[index]
    const key = getPreviewKey(file)
    const preview = filePreviews.get(key)
    if (preview && preview.startsWith('blob:')) URL.revokeObjectURL(preview)
    filePreviews.delete(key)
    filePreviews = new Map(filePreviews)
    attachedFiles = attachedFiles.filter((_, i) => i !== index)
  }

  // Drag & drop
  function handleDragOver(e: DragEvent) {
    e.preventDefault()
    isDragging = true
  }

  function handleDragLeave(e: DragEvent) {
    // Only clear if leaving the panel itself (not a child)
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    if (e.clientX <= rect.left || e.clientX >= rect.right || e.clientY <= rect.top || e.clientY >= rect.bottom) {
      isDragging = false
    }
  }

  function handleDrop(e: DragEvent) {
    e.preventDefault()
    isDragging = false
    if (e.dataTransfer?.files && e.dataTransfer.files.length > 0) {
      addFiles(e.dataTransfer.files)
    }
  }

  async function filesToAttachments(files: File[]): Promise<ChatAttachment[]> {
    const results: ChatAttachment[] = []
    for (const file of files) {
      const buffer = await file.arrayBuffer()
      const bytes = new Uint8Array(buffer)
      let binary = ''
      for (let i = 0; i < bytes.byteLength; i++) {
        binary += String.fromCharCode(bytes[i])
      }
      results.push({
        name: file.name,
        mime_type: file.type || 'application/octet-stream',
        data: btoa(binary),
      })
    }
    return results
  }

  function copyMessageText(text: string) {
    navigator.clipboard.writeText(text).catch(() => {})
  }

  export function exportAsMarkdown(): string {
    const lines: string[] = []
    for (const msg of chatMessages) {
      if (msg.role === 'system') continue
      if (msg.role === 'tool') {
        lines.push(`> **Tool: ${msg.toolName}**`)
        if (msg.toolArgs) lines.push(`> Args: \`${msg.toolArgs}\``)
        if (msg.toolResult) lines.push(`> Result: \`${msg.toolResult}\``)
        lines.push('')
      } else if (msg.role === 'user') {
        lines.push(`### User\n\n${msg.text}\n`)
      } else if (msg.role === 'assistant') {
        lines.push(`### Assistant\n\n${msg.text}\n`)
      } else if (msg.role === 'error') {
        lines.push(`> **Error:** ${msg.text}\n`)
      }
    }
    return lines.join('\n')
  }

  function fmtSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
      e.preventDefault()
      void submitChat()
    }
  }

  function handlePaste(e: ClipboardEvent) {
    if (!e.clipboardData) return
    const items = e.clipboardData.items

    // Check for images or files first
    for (let i = 0; i < items.length; i++) {
      const item = items[i]
      if (item.type.startsWith('image/')) {
        e.preventDefault()
        const file = item.getAsFile()
        if (file && attachedFiles.length < 5) {
          const name = `clipboard-${Date.now()}.${item.type.split('/')[1] || 'png'}`
          const renamed = new File([file], name, { type: file.type })
          addFiles([renamed])
        }
        return
      }
      if (item.kind === 'file') {
        const file = item.getAsFile()
        if (file && attachedFiles.length < 5) {
          addFiles([file])
        }
      }
    }

    // Long text paste → attach as file instead of flooding textarea
    const text = e.clipboardData.getData('text/plain')
    if (text && text.length > 500 && attachedFiles.length < 5) {
      e.preventDefault()
      const file = new File([text], `clipboard-text-${Date.now()}.txt`, { type: 'text/plain' })
      addFiles([file])
    }
    // Short text paste is handled natively by the textarea
  }

  let textareaEl: HTMLTextAreaElement | undefined = $state()

  onMount(async () => {
    if (sessionId) {
      chatSessionId = sessionId
      chatMessages = [{ id: 'system-init', role: 'system', text: `Session: ${sessionId.slice(0, 8)}...` }]
      try {
        const history = await getSessionHistory(sessionId)
        for (const msg of history) {
          if (msg.role === 'system' && (msg.content.startsWith('[HEARTBEAT]') || msg.content.startsWith('[COMPACTION SUMMARY]') || msg.content.startsWith('[CRON]'))) {
            continue
          }
          if (msg.role === 'tool') {
            chatMessages.push({
              id: `tool-${msg.tool_call_id || Date.now()}`,
              role: 'tool',
              text: '',
              toolName: msg.tool_name,
              toolCallId: msg.tool_call_id,
              toolArgs: msg.tool_args,
              toolResult: msg.content,
              toolDone: true,
            })
          } else {
            chatMessages.push({
              id: `hist-${chatMessages.length}`,
              role: msg.role as ChatMessage['role'],
              text: msg.content,
            })
          }
        }
        chatMessages = [...chatMessages]
        // Extract artifacts from history
        artifacts = extractArtifactsFromHistory(chatMessages)
        if (artifacts.length > 0) onArtifactsChange?.(artifacts)
        autoTitled = true
        void scrollToBottom()
      } catch { /* ignore */ }
    } else if (projectId) {
      await loadProjectSession()
    } else {
      chatMessages = [{ id: 'system-init', role: 'system', text: 'TARS' }]
    }
    if (initialPrompt && !autoSend) {
      chatInput = initialPrompt
      tick().then(() => textareaEl?.focus())
    }
  })
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="chat-panel" role="region" aria-label="Chat" ondragover={handleDragOver} ondragleave={handleDragLeave} ondrop={handleDrop}>
  {#if isDragging}
    <div class="drop-overlay">
      <div class="drop-label">Drop files here</div>
    </div>
  {/if}
  <div class="chat-log" bind:this={chatLogEl} onscroll={handleScroll}>
    {#each chatMessages as msg}
      {#if msg.role === 'tool'}
        <div class="chat-msg chat-tool">
          <div class="tool-header">
            <span class="tool-icon">{msg.toolDone ? '\u2713' : '\u27F3'}</span>
            <span class="tool-name">{msg.toolName}</span>
            {#if msg.toolDone}
              <span class="badge badge-success tool-badge">done</span>
            {:else}
              <span class="badge badge-accent tool-badge">running</span>
            {/if}
          </div>
          {#if msg.toolArgs}
            <div class="tool-detail">
              <span class="tool-detail-label">args</span>
              <code class="tool-detail-value">{msg.toolArgs}</code>
            </div>
          {/if}
          {#if msg.toolResult}
            <div class="tool-detail">
              <span class="tool-detail-label">result</span>
              <code class="tool-detail-value">{msg.toolResult}</code>
            </div>
          {/if}
        </div>
      {:else}
        <div class="chat-msg chat-{msg.role}">
          <span class="chat-role">{msg.role}</span>
          {#if msg.role === 'assistant'}
            <div class="chat-text"><MarkdownContent text={msg.text} /></div>
          {:else}
            <div class="chat-text">{msg.text || '\u2026'}</div>
          {/if}
          {#if (msg.role === 'assistant' || msg.role === 'user') && msg.text}
            <div class="chat-msg-footer">
              {#if msg.usage}
                <span class="usage-badge" title="Token usage">In: {msg.usage.input_tokens.toLocaleString()} &middot; Out: {msg.usage.output_tokens.toLocaleString()}{msg.usage.cache_read_tokens ? ` \u00b7 Cached: ${msg.usage.cache_read_tokens.toLocaleString()}` : ''}</span>
              {/if}
              <button type="button" class="msg-copy-btn" title="Copy message" onclick={() => copyMessageText(msg.text)}>Copy</button>
            </div>
          {/if}
        </div>
      {/if}
    {/each}
  </div>
  {#if chatError}
    <div class="error-banner" style="margin-bottom: var(--space-3)">{chatError}</div>
  {/if}
  {#if attachedFiles.length > 0}
    <div class="chat-attachments">
      {#each attachedFiles as file, i}
        {@const preview = filePreviews.get(getPreviewKey(file))}
        <div class="attachment-card">
          {#if file.type.startsWith('image/') && preview}
            <img class="attachment-thumb" src={preview} alt={file.name} />
          {:else if preview && !preview.startsWith('blob:')}
            <pre class="attachment-text-preview">{preview}</pre>
          {:else}
            <span class="attachment-icon-lg">{file.type.startsWith('image/') ? '\ud83d\uddbc' : file.type === 'application/pdf' ? '\ud83d\udcc3' : '\ud83d\udcc4'}</span>
          {/if}
          <div class="attachment-info">
            <span class="attachment-name">{file.name}</span>
            <span class="attachment-size">{fmtSize(file.size)}</span>
          </div>
          <button class="attachment-remove" onclick={() => removeAttachment(i)}>&times;</button>
        </div>
      {/each}
    </div>
  {/if}
  <form class="chat-form" onsubmit={(e) => { e.preventDefault(); void submitChat() }}>
    <div class="chat-input-row">
      <div class="chat-toolbar">
        <input
          type="file"
          multiple
          accept="image/*,.pdf,.txt,.md,.json,.csv,.yaml,.yml"
          bind:this={fileInputEl}
          onchange={handleFileSelect}
          class="file-input-hidden"
        />
        <button type="button" class="toolbar-btn" title="Attach file" onclick={() => fileInputEl?.click()}>
          <span class="toolbar-icon">{'\ud83d\udcce'}</span>
        </button>
        <button type="button" class="toolbar-btn" title="Attach image" onclick={() => { if (fileInputEl) { fileInputEl.accept = 'image/*'; fileInputEl.click(); fileInputEl.accept = 'image/*,.pdf,.txt,.md,.json,.csv,.yaml,.yml' } }}>
          <span class="toolbar-icon">{'\ud83d\uddbc'}</span>
        </button>
      </div>
      <textarea
        bind:this={textareaEl}
        bind:value={chatInput}
        rows="2"
        placeholder={sessionId ? 'Continue this session...' : projectId ? 'Ask TARS about this project...' : 'Ask TARS anything... (paste images with Ctrl+V)'}
        onkeydown={handleKeydown}
        onpaste={handlePaste}
      ></textarea>
    </div>
    <div class="chat-form-actions">
      {#if chatBusy}
        <button type="button" class="btn btn-danger btn-sm" onclick={handleCancel}>Stop</button>
      {:else}
        <button type="submit" class="btn btn-primary" disabled={!chatInput.trim()}>Send</button>
      {/if}
      {#if chatStatusLine && chatBusy}
        <span class="chat-status-line">{chatStatusLine}</span>
      {/if}
    </div>
  </form>
</div>

<style>
  .chat-panel {
    display: flex;
    flex-direction: column;
    min-height: 0;
    flex: 1;
    position: relative;
  }

  .drop-overlay {
    position: absolute;
    inset: 0;
    z-index: 10;
    background: rgba(224, 145, 69, 0.08);
    border: 2px dashed var(--accent);
    border-radius: var(--radius-lg);
    display: flex;
    align-items: center;
    justify-content: center;
    pointer-events: none;
  }
  .drop-label {
    font-family: var(--font-display);
    font-size: var(--text-lg);
    font-weight: 500;
    color: var(--accent);
  }

  .chat-log {
    display: grid;
    gap: var(--space-2);
    flex: 1;
    overflow-y: auto;
    margin-bottom: var(--space-3);
    scroll-behavior: smooth;
    min-height: 0;
  }

  .chat-msg {
    padding: var(--space-3);
    border-radius: var(--radius-md);
    background: var(--bg-base);
  }

  .chat-user {
    background: rgba(224, 145, 69, 0.08);
    border: 1px solid rgba(224, 145, 69, 0.12);
  }

  .chat-assistant {
    background: var(--bg-elevated);
  }

  .chat-system {
    background: transparent;
    padding: var(--space-2) var(--space-3);
    opacity: 0.6;
  }

  .chat-error {
    background: var(--error-muted);
    border: 1px solid rgba(248, 113, 113, 0.15);
  }

  .chat-tool {
    background: rgba(139, 92, 246, 0.06);
    border: 1px solid rgba(139, 92, 246, 0.12);
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-xs);
  }

  .tool-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .tool-icon { font-size: var(--text-sm); }

  .tool-name {
    font-family: var(--font-mono);
    font-weight: 600;
    color: var(--text-primary);
  }

  .tool-badge { font-size: 10px; padding: 1px 6px; }

  .tool-detail {
    margin-top: var(--space-1);
    display: flex;
    gap: var(--space-2);
    align-items: flex-start;
  }

  .tool-detail-label {
    font-family: var(--font-mono);
    color: var(--text-ghost);
    flex-shrink: 0;
    min-width: 36px;
  }

  .tool-detail-value {
    font-family: var(--font-mono);
    color: var(--text-secondary);
    white-space: pre-wrap;
    word-break: break-all;
    font-size: var(--text-xs);
    background: rgba(255, 255, 255, 0.04);
    padding: 2px 6px;
    border-radius: 3px;
    max-height: 120px;
    overflow-y: auto;
  }

  .chat-role {
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 500;
    color: var(--text-tertiary);
    margin-bottom: var(--space-1);
    display: block;
  }

  .chat-msg-footer {
    display: flex;
    justify-content: flex-end;
    margin-top: var(--space-2);
    opacity: 0;
    transition: opacity var(--duration-fast);
  }
  .chat-msg:hover .chat-msg-footer { opacity: 1; }

  .msg-copy-btn {
    background: var(--bg-base);
    border: 1px solid var(--border-subtle);
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
    cursor: pointer;
    padding: 2px 10px;
    border-radius: var(--radius-sm);
    transition: all var(--duration-fast);
  }
  .msg-copy-btn:hover { color: var(--accent); border-color: var(--accent); }

  .chat-text {
    white-space: pre-wrap;
    font-size: var(--text-sm);
    line-height: 1.55;
  }

  /* ── Attachments ─────────────────────────────── */
  .chat-attachments {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }

  .attachment-card {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2);
    background: var(--bg-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    font-size: var(--text-xs);
    max-width: 240px;
    position: relative;
  }

  .attachment-thumb {
    width: 48px;
    height: 48px;
    object-fit: cover;
    border-radius: var(--radius-sm);
    flex-shrink: 0;
  }

  .attachment-text-preview {
    width: 48px;
    height: 48px;
    overflow: hidden;
    font-family: var(--font-mono);
    font-size: 7px;
    line-height: 1.3;
    color: var(--text-ghost);
    background: var(--bg-base);
    border-radius: var(--radius-sm);
    padding: 2px 3px;
    flex-shrink: 0;
    white-space: pre;
    margin: 0;
  }

  .attachment-icon-lg {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 48px;
    height: 48px;
    font-size: 24px;
    background: var(--bg-base);
    border-radius: var(--radius-sm);
    flex-shrink: 0;
  }

  .attachment-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .attachment-name {
    color: var(--text-primary);
    max-width: 140px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-weight: 500;
  }
  .attachment-size { color: var(--text-ghost); }
  .attachment-remove {
    position: absolute;
    top: 2px;
    right: 2px;
    background: var(--bg-base);
    border: 1px solid var(--border-subtle);
    border-radius: 50%;
    color: var(--text-ghost);
    cursor: pointer;
    font-size: 12px;
    width: 18px;
    height: 18px;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    line-height: 1;
    opacity: 0;
    transition: opacity var(--duration-fast);
  }
  .attachment-card:hover .attachment-remove { opacity: 1; }
  .attachment-remove:hover { color: var(--error); border-color: var(--error); }

  /* ── Form ───────────────────────────────────── */
  .chat-form {
    display: grid;
    gap: var(--space-2);
  }

  .chat-input-row {
    display: flex;
    gap: var(--space-2);
    align-items: flex-start;
  }

  .chat-input-row textarea {
    flex: 1;
  }

  .chat-toolbar {
    display: flex;
    flex-direction: column;
    gap: 2px;
    flex-shrink: 0;
    padding-top: 4px;
  }

  .toolbar-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: transparent;
    cursor: pointer;
    transition: all var(--duration-fast) var(--ease-out);
  }
  .toolbar-btn:hover {
    background: var(--bg-elevated);
    border-color: var(--border-default);
  }

  .toolbar-icon { font-size: 14px; }

  .chat-form-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .chat-status-line {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-ghost);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .usage-badge {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
    background: rgba(255, 255, 255, 0.04);
    padding: 1px 6px;
    border-radius: var(--radius-sm);
    margin-right: auto;
  }

  .file-input-hidden {
    position: absolute;
    width: 0;
    height: 0;
    opacity: 0;
    overflow: hidden;
  }
</style>
