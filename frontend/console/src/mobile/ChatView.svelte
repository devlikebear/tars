<script lang="ts">
  // The app's Chat tab (docs/decisions/mobile-remote.md §8): a plain
  // question to one desktop, through the device's own chat session — never
  // the chat workbench. No session list, no switching sessions, no history
  // beyond this one session.
  import { onDestroy, tick } from 'svelte'
  import { t } from '../i18n'
  import type { ChatApproval, ChatApprovalState } from '../lib/chatApproval'
  import { stripFocusBlocks } from '../lib/focus'
  import type { PairedDesktop } from '../lib/mobile/desktops'
  import type { RemoteChatStore } from '../lib/mobile/remoteChatStore.svelte'
  import MarkdownContent from '../components/MarkdownContent.svelte'

  interface Props {
    chat: RemoteChatStore
    // Desktops this screen may chat with: online ones, the same set the
    // idea sheet offers.
    desktops: PairedDesktop[]
  }

  let { chat, desktops }: Props = $props()

  let desktopID = $state('')
  let draft = $state('')
  let logEl: HTMLElement | undefined = $state()
  const lastAssistantId = $derived(chat.messages.findLast((m) => m.role === 'assistant')?.id ?? '')

  async function scrollToEnd() {
    await tick()
    if (logEl) logEl.scrollTop = logEl.scrollHeight
  }

  function pick(id: string) {
    desktopID = id
    const desktop = desktops.find((d) => d.id === id)
    if (!desktop) return
    void chat.connect(desktop).then(scrollToEnd)
  }

  async function send() {
    const text = draft.trim()
    if (!text || chat.running) return
    draft = ''
    await chat.send(text)
    await scrollToEnd()
  }

  function approvalStateText(state: ChatApprovalState): string {
    switch (state) {
      case 'sending':
        return $t.mobileApp.chat.permission.sending
      case 'allowed':
        return $t.mobileApp.chat.permission.allowed
      case 'allowed_session':
        return $t.mobileApp.chat.permission.allowedSession
      case 'denied':
        return $t.mobileApp.chat.permission.denied
      default:
        return ''
    }
  }

  function answer(approval: ChatApproval, decision: 'allow_once' | 'allow_session' | 'deny') {
    void chat.answerPermission(approval, decision)
  }

  // A lone desktop is picked automatically; more than one waits for a tap.
  let autoPicked = false
  $effect(() => {
    if (!autoPicked && !desktopID && desktops.length === 1) {
      autoPicked = true
      pick(desktops[0].id)
    }
  })

  // Switching away from this tab stops watching the turn feed; the turn
  // itself, if any, keeps running at the desk (same rule as a side
  // session). Coming back to the tab remounts this screen and re-attaches.
  onDestroy(() => chat.disconnect())
</script>

<section class="chat" aria-label={$t.mobileApp.tabs.chat}>
  {#if desktops.length === 0}
    <p class="empty" data-testid="mobile-chat-empty">{$t.mobileApp.chat.noDesktop}</p>
  {:else}
    <select
      class="pick"
      aria-label={$t.mobileApp.chat.pickDesktop}
      value={desktopID}
      onchange={(e) => pick((e.currentTarget as HTMLSelectElement).value)}
      data-testid="mobile-chat-pick"
    >
      <option value="" disabled>{$t.mobileApp.chat.pickDesktop}</option>
      {#each desktops as d (d.id)}
        <option value={d.id}>{d.name}</option>
      {/each}
    </select>

    {#if !desktopID}
      <p class="empty">{$t.mobileApp.chat.empty}</p>
    {:else if chat.loading}
      <p class="state-hint" data-testid="mobile-chat-connecting">{$t.mobileApp.chat.connecting}</p>
    {:else if chat.errorKind}
      <p class="chat-error" role="alert" data-testid="mobile-chat-error">
        {$t.mobileApp.chat.failures[chat.errorKind] ?? chat.error}
      </p>
    {:else}
      <div class="log" bind:this={logEl} data-testid="mobile-chat-log">
        {#if chat.messages.length === 0}
          <p class="empty">{$t.mobileApp.chat.empty}</p>
        {/if}
        {#each chat.messages as message (message.id)}
          {#if message.role === 'approval' && message.approval}
            <div class="approval" data-testid="mobile-chat-approval">
              <p class="approval-tool">{message.approval.toolName}</p>
              {#if message.approval.state === 'pending' || message.approval.state === 'sending'}
                <div class="approval-actions">
                  <button
                    type="button"
                    class="btn btn-sm"
                    disabled={message.approval.state === 'sending'}
                    onclick={() => answer(message.approval as ChatApproval, 'allow_once')}
                  >{$t.mobileApp.chat.permission.allowOnce}</button>
                  <button
                    type="button"
                    class="btn btn-sm"
                    disabled={message.approval.state === 'sending'}
                    onclick={() => answer(message.approval as ChatApproval, 'allow_session')}
                  >{$t.mobileApp.chat.permission.allowSession}</button>
                  <button
                    type="button"
                    class="btn btn-sm btn-ghost"
                    disabled={message.approval.state === 'sending'}
                    onclick={() => answer(message.approval as ChatApproval, 'deny')}
                  >{$t.mobileApp.chat.permission.deny}</button>
                </div>
                {#if message.approval.state === 'sending'}
                  <p class="approval-state">{approvalStateText('sending')}</p>
                {/if}
              {:else}
                <p class="approval-state">{approvalStateText(message.approval.state)}</p>
              {/if}
              {#if message.approval.error}
                <p class="approval-error">{$t.mobileApp.chat.permission.failed}</p>
              {/if}
            </div>
          {:else if message.role === 'error'}
            <div class="msg-error">{message.text}</div>
          {:else}
            <div class={`msg msg-${message.role}`}>
              {#if message.role === 'assistant'}
                <MarkdownContent text={stripFocusBlocks(message.text, { streaming: chat.running && message.id === lastAssistantId })} />
              {:else}
                {message.text}
              {/if}
            </div>
          {/if}
        {/each}
        {#if chat.running}
          <div class="running" data-testid="mobile-chat-running">{$t.mobileApp.chat.running}</div>
        {/if}
      </div>
      {#if chat.error && !chat.errorKind}
        <p class="chat-error" role="alert">{chat.error}</p>
      {/if}
      <form
        class="compose"
        onsubmit={(e) => {
          e.preventDefault()
          void send()
        }}
      >
        <textarea
          rows="2"
          placeholder={$t.mobileApp.chat.placeholder}
          bind:value={draft}
          disabled={chat.running}
          data-testid="mobile-chat-input"
          onkeydown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
              e.preventDefault()
              void send()
            }
          }}
        ></textarea>
        {#if chat.running}
          <button type="button" class="btn btn-ghost btn-sm" onclick={() => chat.cancel()} data-testid="mobile-chat-stop">
            {$t.mobileApp.chat.stop}
          </button>
        {:else}
          <button type="submit" class="btn btn-primary btn-sm" disabled={!draft.trim()} data-testid="mobile-chat-send">
            {$t.mobileApp.chat.send}
          </button>
        {/if}
      </form>
    {/if}
  {/if}
</section>

<style>
  .chat {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    flex: 1;
    min-height: 0;
  }

  .pick {
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    color: var(--text-primary);
    font-size: var(--text-sm);
  }

  .empty,
  .state-hint {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--text-secondary);
    line-height: 1.5;
  }

  .chat-error {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--error);
  }

  .log {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: var(--space-2);
    min-height: 0;
    overflow-y: auto;
  }

  .msg {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
  }

  .msg-user {
    align-self: flex-end;
    max-width: 90%;
    background: var(--surface-active);
    color: var(--text-primary);
    white-space: pre-wrap;
  }

  .msg-assistant {
    background: var(--surface-inset);
  }

  .msg-error {
    color: var(--error);
    font-size: var(--text-xs);
  }

  .running {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .approval {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
  }

  .approval-tool {
    margin: 0;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .approval-actions {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .approval-state {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .approval-error {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--error);
  }

  .compose {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-1);
  }

  .compose textarea {
    width: 100%;
    resize: vertical;
    font-size: var(--text-sm);
  }
</style>
