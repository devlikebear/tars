<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte'
  import { streamChat } from '../lib/api'
  import { renderMarkdown } from '../lib/markdown'
  import type { ChatAttachment, ChatEvent } from '../lib/types'

  type ChatMessage = {
    id: string
    role: 'user' | 'assistant' | 'system' | 'error' | 'tool'
    text: string
    toolName?: string
    toolCallId?: string
    toolArgs?: string
    toolResult?: string
    toolDone?: boolean
  }

  interface Props {
    projectId?: string
    sessionId?: string
    initialPrompt?: string
  }

  let { projectId, sessionId, initialPrompt }: Props = $props()

  let chatInput = $state('')
  let chatBusy = $state(false)
  let chatError = $state('')
  let chatSessionId = $state('')
  let chatStatusLine = $state('')
  let chatMessages: ChatMessage[] = $state([])

  let chatLogEl: HTMLDivElement | undefined = $state()
  let autoScroll = $state(true)

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
      case 'done':
        chatSessionId = event.session_id?.trim() || chatSessionId
        chatStatusLine = 'done'
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
    try {
      const chatAttachments = currentFiles.length > 0 ? await filesToAttachments(currentFiles) : undefined
      await streamChat(
        {
          message,
          session_id: chatSessionId || undefined,
          project_id: projectId || undefined,
          attachments: chatAttachments,
        },
        (event) => handleChatEvent(event, assistantId),
      )
    } catch (err) {
      chatError = err instanceof Error ? err.message : 'Failed to send'
      chatMessages = [...chatMessages, { id: `error-${Date.now()}`, role: 'error', text: chatError }]
    } finally {
      chatBusy = false
      void scrollToBottom()
    }
  }

  // -- File attachments --
  let attachedFiles: File[] = $state([])
  let fileInputEl: HTMLInputElement | undefined = $state()

  function handleFileSelect(e: Event) {
    const input = e.target as HTMLInputElement
    if (!input.files) return
    for (const file of input.files) {
      if (attachedFiles.length >= 5) break
      attachedFiles = [...attachedFiles, file]
    }
    input.value = ''
  }

  function removeAttachment(index: number) {
    attachedFiles = attachedFiles.filter((_, i) => i !== index)
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

  function fmtSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      void submitChat()
    }
  }

  function handlePaste(e: ClipboardEvent) {
    if (!e.clipboardData) return
    const items = e.clipboardData.items
    for (let i = 0; i < items.length; i++) {
      const item = items[i]
      // Image from clipboard (screenshot, copy-paste)
      if (item.type.startsWith('image/')) {
        e.preventDefault()
        const file = item.getAsFile()
        if (file && attachedFiles.length < 5) {
          const name = `clipboard-${Date.now()}.${item.type.split('/')[1] || 'png'}`
          const renamed = new File([file], name, { type: file.type })
          attachedFiles = [...attachedFiles, renamed]
        }
        return
      }
      // File from clipboard (some browsers support this)
      if (item.kind === 'file') {
        const file = item.getAsFile()
        if (file && attachedFiles.length < 5) {
          attachedFiles = [...attachedFiles, file]
        }
      }
    }
    // Text paste is handled natively by the textarea
  }

  let textareaEl: HTMLTextAreaElement | undefined = $state()

  onMount(() => {
    if (sessionId) chatSessionId = sessionId
    const scope = sessionId ? `session ${sessionId}` : projectId ? `project ${projectId}` : 'TARS'
    chatMessages = [{ id: 'system-init', role: 'system', text: `Chat scoped to ${scope}` }]
    if (initialPrompt) {
      chatInput = initialPrompt
      tick().then(() => textareaEl?.focus())
    }
  })
</script>

<div class="chat-panel">
  <div class="chat-status">
    {chatBusy ? 'streaming' : chatStatusLine || 'idle'}
  </div>
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
            <div class="chat-text chat-md">{@html renderMarkdown(msg.text || '\u2026')}</div>
          {:else}
            <div class="chat-text">{msg.text || '\u2026'}</div>
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
        <div class="attachment-chip">
          <span class="attachment-icon">{file.type.startsWith('image/') ? '\ud83d\uddbc' : '\ud83d\udcc4'}</span>
          <span class="attachment-name">{file.name}</span>
          <span class="attachment-size">{fmtSize(file.size)}</span>
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
        <button type="button" class="toolbar-btn" title="Attach file (coming soon)" onclick={() => fileInputEl?.click()}>
          <span class="toolbar-icon">{'\ud83d\udcce'}</span>
        </button>
        <button type="button" class="toolbar-btn" title="Attach image (coming soon)" onclick={() => { if (fileInputEl) { fileInputEl.accept = 'image/*'; fileInputEl.click(); fileInputEl.accept = 'image/*,.pdf,.txt,.md,.json,.csv,.yaml,.yml' } }}>
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
    <button type="submit" class="btn btn-primary" disabled={chatBusy || !chatInput.trim()}>
      {chatBusy ? 'Streaming...' : 'Send'}
    </button>
  </form>
</div>

<style>
  .chat-panel {
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .chat-status {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    margin-bottom: var(--space-3);
  }

  .chat-log {
    display: grid;
    gap: var(--space-2);
    max-height: 480px;
    overflow-y: auto;
    margin-bottom: var(--space-3);
    scroll-behavior: smooth;
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

  .chat-text {
    white-space: pre-wrap;
    font-size: var(--text-sm);
    line-height: 1.55;
  }

  /* ── Markdown rendered content ─────────────── */
  .chat-md {
    white-space: normal;
  }

  .chat-md :global(p) {
    margin: 0 0 var(--space-2);
  }

  .chat-md :global(p:last-child) {
    margin-bottom: 0;
  }

  .chat-md :global(pre) {
    background: var(--bg-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: var(--space-3);
    overflow-x: auto;
    margin: var(--space-2) 0;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    line-height: 1.5;
  }

  .chat-md :global(code) {
    font-family: var(--font-mono);
    font-size: 0.9em;
    background: rgba(255, 255, 255, 0.06);
    padding: 1px 5px;
    border-radius: 3px;
  }

  .chat-md :global(pre code) {
    background: none;
    padding: 0;
    border-radius: 0;
    font-size: inherit;
  }

  .chat-md :global(strong) {
    font-weight: 600;
    color: var(--text-primary);
  }

  .chat-md :global(em) {
    font-style: italic;
  }

  .chat-md :global(a) {
    color: var(--accent);
    text-decoration: underline;
    text-underline-offset: 2px;
  }

  .chat-md :global(a:hover) {
    color: var(--accent-hover);
  }

  .chat-md :global(ul),
  .chat-md :global(ol) {
    margin: var(--space-2) 0;
    padding-left: var(--space-5);
  }

  .chat-md :global(li) {
    margin-bottom: var(--space-1);
    font-size: var(--text-sm);
    line-height: 1.55;
  }

  .chat-md :global(h3),
  .chat-md :global(h4),
  .chat-md :global(h5),
  .chat-md :global(h6) {
    font-family: var(--font-display);
    font-weight: 600;
    margin: var(--space-3) 0 var(--space-1);
    color: var(--text-primary);
  }

  .chat-md :global(h3) { font-size: var(--text-base); }
  .chat-md :global(h4) { font-size: var(--text-sm); }

  /* ── Attachments ─────────────────────────────── */
  .chat-attachments {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
    margin-bottom: var(--space-2);
  }

  .attachment-chip {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    padding: 2px var(--space-2);
    background: var(--bg-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
  }

  .attachment-icon { font-size: var(--text-sm); }
  .attachment-name {
    color: var(--text-primary);
    max-width: 120px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .attachment-size { color: var(--text-ghost); }
  .attachment-remove {
    background: none;
    border: none;
    color: var(--text-ghost);
    cursor: pointer;
    font-size: var(--text-sm);
    padding: 0 2px;
    line-height: 1;
  }
  .attachment-remove:hover { color: var(--error); }

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

  .file-input-hidden {
    position: absolute;
    width: 0;
    height: 0;
    opacity: 0;
    overflow: hidden;
  }
</style>
