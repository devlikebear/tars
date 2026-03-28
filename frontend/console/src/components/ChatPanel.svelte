<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte'
  import { streamChat } from '../lib/api'
  import { renderMarkdown } from '../lib/markdown'
  import type { ChatEvent } from '../lib/types'

  type ChatMessage = {
    id: string
    role: 'user' | 'assistant' | 'system' | 'error'
    text: string
  }

  interface Props {
    projectId?: string
  }

  let { projectId }: Props = $props()

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
    const userId = `user-${Date.now()}`
    const assistantId = `assistant-${Date.now()}`
    chatMessages = [
      ...chatMessages,
      { id: userId, role: 'user', text: message },
      { id: assistantId, role: 'assistant', text: '' },
    ]
    void scrollToBottom()
    try {
      await streamChat(
        {
          message,
          session_id: chatSessionId || undefined,
          project_id: projectId || undefined,
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

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      void submitChat()
    }
  }

  onMount(() => {
    const scope = projectId ? `project ${projectId}` : 'TARS'
    chatMessages = [{ id: 'system-init', role: 'system', text: `Chat scoped to ${scope}` }]
  })
</script>

<div class="chat-panel">
  <div class="chat-status">
    {chatBusy ? 'streaming' : chatStatusLine || 'idle'}
  </div>
  <div class="chat-log" bind:this={chatLogEl} onscroll={handleScroll}>
    {#each chatMessages as msg}
      <div class="chat-msg chat-{msg.role}">
        <span class="chat-role">{msg.role}</span>
        {#if msg.role === 'assistant'}
          <div class="chat-text chat-md">{@html renderMarkdown(msg.text || '\u2026')}</div>
        {:else}
          <div class="chat-text">{msg.text || '\u2026'}</div>
        {/if}
      </div>
    {/each}
  </div>
  {#if chatError}
    <div class="error-banner" style="margin-bottom: var(--space-3)">{chatError}</div>
  {/if}
  <form class="chat-form" onsubmit={(e) => { e.preventDefault(); void submitChat() }}>
    <textarea
      bind:value={chatInput}
      rows="2"
      placeholder={projectId ? 'Ask TARS about this project...' : 'Ask TARS anything...'}
      onkeydown={handleKeydown}
    ></textarea>
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

  /* ── Form ───────────────────────────────────── */
  .chat-form {
    display: grid;
    gap: var(--space-3);
  }
</style>
