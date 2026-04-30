<script lang="ts">
  import type { Artifact } from '../lib/artifacts'
  import type { ChatMessage } from '../lib/chatMessages'
  import MarkdownContent from './MarkdownContent.svelte'

  interface Props {
    message: ChatMessage
    artifacts: Artifact[]
    onArtifactOpen?: (path: string) => void
    onCopy: (text: string) => void
  }

  let { message, artifacts, onArtifactOpen, onCopy }: Props = $props()
</script>

{#if message.role === 'tool'}
  <div class="chat-msg chat-tool">
    <div class="tool-header">
      <span class="tool-icon">{message.toolDone ? '\u2713' : '\u27F3'}</span>
      <span class="tool-name">{message.toolName}</span>
      {#if message.toolDone}
        <span class="badge badge-success tool-badge">done</span>
      {:else}
        <span class="badge badge-accent tool-badge">running</span>
      {/if}
    </div>
    {#if message.toolArgs}
      <div class="tool-detail">
        <span class="tool-detail-label">args</span>
        <code class="tool-detail-value">{message.toolArgs}</code>
      </div>
    {/if}
    {#if message.toolResult}
      <div class="tool-detail">
        <span class="tool-detail-label">result</span>
        <code class="tool-detail-value">{message.toolResult}</code>
      </div>
    {/if}
  </div>
{:else}
  <div class="chat-msg chat-{message.role}">
    <span class="chat-role">{message.role}</span>
    {#if message.role === 'assistant'}
      <div class="chat-text"><MarkdownContent text={message.text} {artifacts} {onArtifactOpen} /></div>
    {:else}
      <div class="chat-text">{message.text || '\u2026'}</div>
    {/if}
    {#if (message.role === 'assistant' || message.role === 'user') && message.text}
      <div class="chat-msg-footer">
        {#if message.usage}
          <span class="usage-badge" title="Token usage">In: {message.usage.input_tokens.toLocaleString()} &middot; Out: {message.usage.output_tokens.toLocaleString()}{message.usage.cache_read_tokens ? ` \u00b7 Cached: ${message.usage.cache_read_tokens.toLocaleString()}` : ''}</span>
        {/if}
        <button type="button" class="msg-copy-btn" title="Copy message" onclick={() => onCopy(message.text)}>Copy</button>
      </div>
    {/if}
  </div>
{/if}

<style>
  .chat-msg {
    padding: var(--space-3);
    border-radius: var(--radius-md);
    background: var(--surface-base);
  }

  .chat-user {
    background: rgba(224, 145, 69, 0.08);
    border: 1px solid rgba(224, 145, 69, 0.12);
  }

  .chat-assistant {
    background: var(--surface-elevated);
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
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
    cursor: pointer;
    padding: 2px 10px;
    border-radius: var(--radius-sm);
    transition: all var(--duration-fast);
  }
  .msg-copy-btn:hover { color: var(--primary); border-color: var(--primary); }

  .chat-text {
    white-space: pre-wrap;
    font-size: var(--text-sm);
    line-height: 1.55;
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
</style>
