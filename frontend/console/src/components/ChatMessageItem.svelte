<script lang="ts">
  import { t } from '../i18n'
  import type { Artifact } from '../lib/artifacts'
  import type { ChatMessage } from '../lib/chatMessages'
  import {
    formatElapsedSeconds,
    formatToolInvocationPreview,
    formatToolJSON,
    toolCallTone,
  } from '../lib/toolCalls'
  import MarkdownContent from './MarkdownContent.svelte'
  import SubagentProgressCard from './SubagentProgressCard.svelte'
  import ChatStreamingStatus from './ChatStreamingStatus.svelte'
  import { buildSubagentProgress } from '../lib/subagentProgress'

  interface StreamingStatus {
    label: string
    elapsedLabel: string
    steps: readonly string[]
    currentStepIndex: number
    stepLabels: string[]
    locale: 'ko' | 'en'
    onToggleLocale: () => void
  }

  interface Props {
    message: ChatMessage
    artifacts: Artifact[]
    onArtifactOpen?: (path: string) => void
    onCopy: (text: string) => void
    onForkMessage?: (message: ChatMessage) => void
    streamingStatus?: StreamingStatus | null
  }

  let { message, artifacts, onArtifactOpen, onCopy, onForkMessage, streamingStatus }: Props = $props()

  let nowMs = $state(Date.now())

  $effect(() => {
    if (message.role !== 'tool' || message.toolDone) return
    nowMs = Date.now()
    const timer = setInterval(() => {
      nowMs = Date.now()
    }, 500)
    return () => clearInterval(timer)
  })

  let tone = $derived(message.role === 'tool' ? toolCallTone(message) : 'done')
  let elapsedLabel = $derived(formatElapsedSeconds(message.toolStartedAt, message.toolFinishedAt, nowMs))
  let invocationPreview = $derived(formatToolInvocationPreview(message.toolName, message.toolArgs))
  let argsJSON = $derived(formatToolJSON(message.toolArgs))
  let resultJSON = $derived(formatToolJSON(message.toolResult))
  let toolBadgeClass = $derived(tone === 'error' ? 'badge-error' : tone === 'running' ? 'badge-accent' : 'badge-default')
  let subagentProgress = $derived(message.role === 'tool' ? buildSubagentProgress({
    toolName: message.toolName,
    toolArgs: message.toolArgs,
    toolResult: message.toolResult,
    toolDone: message.toolDone,
    toolIsError: message.toolIsError,
  }) : null)
</script>

{#if message.role === 'tool'}
  {#if subagentProgress}
    <SubagentProgressCard progress={subagentProgress} {tone} {elapsedLabel} />
  {:else}
    <details class="chat-msg chat-tool chat-tool-{tone}" open={message.toolIsError || !message.toolDone}>
      <summary class="tool-header">
        <span class="tool-icon">{tone === 'error' ? '!' : message.toolDone ? '\u2713' : '\u27F3'}</span>
        <span class="tool-name">{invocationPreview}</span>
        {#if elapsedLabel}
          <span class="tool-elapsed">{elapsedLabel}</span>
        {/if}
        <span class="badge {toolBadgeClass} tool-badge">{tone}</span>
      </summary>
      <div class="tool-detail-grid">
        {#if argsJSON}
          <div class="tool-detail">
            <span class="tool-detail-label">args</span>
            <pre class="tool-detail-value"><code>{argsJSON}</code></pre>
          </div>
        {/if}
        {#if resultJSON}
          <div class="tool-detail">
            <span class="tool-detail-label">result</span>
            <pre class="tool-detail-value"><code>{resultJSON}</code></pre>
          </div>
        {/if}
      </div>
    </details>
  {/if}
{:else}
  <div class="chat-msg chat-{message.role}">
    <div class="chat-message-heading">
      {#if message.role === 'assistant'}
        <img class="chat-avatar" src="/console/tars-avatar.png" alt="" width="22" height="22" />
      {/if}
      <span class="chat-role">{$t.chat.message.roles[message.role as keyof typeof $t.chat.message.roles] ?? message.role}</span>
    </div>
    {#if message.role === 'assistant'}
      {#if !message.text && streamingStatus}
        <div class="chat-text">
          <ChatStreamingStatus
            label={streamingStatus.label}
            elapsedLabel={streamingStatus.elapsedLabel}
            steps={streamingStatus.steps}
            currentStepIndex={streamingStatus.currentStepIndex}
            stepLabels={streamingStatus.stepLabels}
            locale={streamingStatus.locale}
            onToggleLocale={streamingStatus.onToggleLocale}
          />
        </div>
      {:else}
        <div class="chat-text"><MarkdownContent text={message.text} {artifacts} {onArtifactOpen} /></div>
      {/if}
    {:else}
      <div class="chat-text">{message.text || '\u2026'}</div>
    {/if}
    {#if (message.role === 'assistant' || message.role === 'user') && message.text}
      <div class="chat-msg-footer">
        {#if message.usage}
          <span class="usage-badge" title={$t.chat.message.usageBadgeTitle}>{$t.chat.message.usageIn}: {message.usage.input_tokens.toLocaleString()} &middot; {$t.chat.message.usageOut}: {message.usage.output_tokens.toLocaleString()}{message.usage.cache_read_tokens ? ` \u00b7 ${$t.chat.message.usageCached}: ${message.usage.cache_read_tokens.toLocaleString()}` : ''}</span>
        {/if}
        {#if message.sourceMessageId && onForkMessage}
          <button type="button" class="msg-copy-btn" title={$t.chat.message.forkFromHereTitle} onclick={() => onForkMessage?.(message)}>{$t.chat.message.forkFromHere}</button>
        {/if}
        <button type="button" class="msg-copy-btn" title={$t.chat.message.copyTitle} onclick={() => onCopy(message.text)}>{$t.chat.message.copy}</button>
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
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-xs);
  }

  .chat-tool-running {
    background: var(--primary-muted);
    border-color: color-mix(in srgb, var(--primary) 35%, var(--border-subtle));
  }

  .chat-tool-error {
    background: var(--error-muted);
    border-color: color-mix(in srgb, var(--error) 35%, var(--border-subtle));
  }

  .tool-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    cursor: pointer;
    list-style: none;
  }

  .tool-header::-webkit-details-marker {
    display: none;
  }

  .tool-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    border-radius: 999px;
    background: var(--surface-elevated);
    color: var(--text-secondary);
    font-size: 11px;
    flex-shrink: 0;
  }

  .chat-tool-running .tool-icon {
    background: var(--primary-muted);
    color: var(--primary-text);
  }

  .chat-tool-error .tool-icon {
    background: var(--error-muted);
    color: var(--error);
  }

  .tool-name {
    font-family: var(--font-mono);
    font-weight: 600;
    color: var(--text-primary);
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .tool-elapsed {
    margin-left: auto;
    font-family: var(--font-mono);
    color: var(--text-tertiary);
  }

  .tool-badge { font-size: 10px; padding: 1px 6px; }

  .tool-detail-grid {
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }

  .tool-detail {
    display: grid;
    grid-template-columns: 48px minmax(0, 1fr);
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
    margin: 0;
    font-family: var(--font-mono);
    color: var(--text-secondary);
    white-space: pre-wrap;
    word-break: break-all;
    font-size: var(--text-xs);
    line-height: 1.55;
    background: var(--surface-inset);
    padding: var(--space-2);
    border-radius: var(--radius-sm);
    max-height: 160px;
    overflow-y: auto;
  }

  .chat-role {
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 500;
    color: var(--text-tertiary);
    display: block;
  }

  .chat-message-heading {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: var(--space-1);
    min-height: 22px;
  }

  .chat-avatar {
    width: 24px;
    height: 24px;
    padding: 2px;
    border: 1px solid rgba(224, 145, 69, 0.16);
    border-radius: var(--radius-sm);
    background: rgba(224, 145, 69, 0.08);
    box-sizing: border-box;
    object-fit: contain;
    flex-shrink: 0;
  }

  .chat-msg-footer {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
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
