<script lang="ts">
  // Side session dock panel (#971): another session next to the active one.
  // It shows that session's conversation, follows its running turn through
  // the turn feed, lets approval cards be answered, and sends messages, so
  // two sessions can be worked side by side.
  import { onDestroy, tick } from 'svelte'
  import { attachChatStream, getSessionCwd, getSessionHistory, streamChat, streamEvents } from '../lib/api'
  import type { ChatApproval } from '../lib/chatApproval'
  import { withdrawPendingApprovals } from '../lib/chatApproval'
  import type { ChatMessage } from '../lib/chatMessages'
  import { applySideEvent, historyMessages } from '../lib/sideSession'
  import { displaySessionTitle } from '../lib/sessionLabels'
  import { formatToolInvocationPreview } from '../lib/toolCalls'
  import { toolBaseDirs } from '../lib/cliToolLabels'
  import { chatSession } from '../lib/stores/chatSession'
  import type { Session } from '../lib/types'
  import { t } from '../i18n'
  import ChatApprovalCard from './ChatApprovalCard.svelte'
  import MarkdownContent from './MarkdownContent.svelte'
  import { stripFocusBlocks } from '../lib/focus'

  interface Props {
    activeSessionId: string | null
    onOpen: (session: Session) => void
    onClose: () => void
  }

  let { activeSessionId, onOpen, onClose }: Props = $props()

  const storageKey = 'tars.console.chat.sideSession'

  function stored(): string {
    try {
      return localStorage.getItem(storageKey) ?? ''
    } catch {
      return ''
    }
  }

  let sideId = $state(stored())
  let messages = $state<ChatMessage[]>([])
  // The side session's working folder: tool paths inside it show relative.
  let sideCwd = $state<string | undefined>(undefined)
  let running = $state(false)
  let draft = $state('')
  let error = $state('')
  let logEl: HTMLElement | undefined = $state()
  let controller: AbortController | null = null
  let loadToken = 0

  let candidates = $derived(chatSession.sessions.filter((s) => s.id !== activeSessionId && !s.archived_at))
  let sideSession = $derived(chatSession.sessions.find((s) => s.id === sideId) ?? null)

  function remember(id: string) {
    try {
      if (id) localStorage.setItem(storageKey, id)
      else localStorage.removeItem(storageKey)
    } catch {
      // Private windows: the choice lasts for this page only.
    }
  }

  async function scrollToEnd() {
    await tick()
    if (logEl) logEl.scrollTop = logEl.scrollHeight
  }

  async function load(id: string) {
    const token = ++loadToken
    controller?.abort()
    controller = null
    running = false
    error = ''
    sideCwd = undefined
    if (!id) {
      messages = []
      return
    }
    void getSessionCwd(id)
      .then((cwd) => {
        if (token === loadToken) sideCwd = cwd.current || undefined
      })
      .catch(() => {
        // Absolute paths then; the label still names the file.
      })
    try {
      const history = await getSessionHistory(id)
      if (token !== loadToken) return
      messages = historyMessages(history)
      void scrollToEnd()
      void follow(id, token)
    } catch (err) {
      if (token === loadToken) error = err instanceof Error ? err.message : String(err)
    }
  }

  // follow attaches to the session's running turn, if any.
  async function follow(id: string, token: number) {
    if (running) return
    const ac = new AbortController()
    controller = ac
    const replyId = `side-reply-${Date.now()}`
    try {
      await attachChatStream(id, (event) => {
        if (token !== loadToken) return
        running = true
        messages = applySideEvent(messages, event, replyId)
        void scrollToEnd()
      }, ac.signal)
    } catch {
      // Aborted, or the turn ended as we attached; the reload below settles it.
    } finally {
      if (controller === ac) controller = null
      if (token === loadToken && running) {
        running = false
        messages = withdrawPendingApprovals(messages)
        void reloadHistory(id, token)
      }
    }
  }

  async function reloadHistory(id: string, token: number) {
    try {
      const history = await getSessionHistory(id)
      if (token === loadToken) {
        messages = historyMessages(history)
        void scrollToEnd()
      }
    } catch {
      // Keep what the stream showed.
    }
  }

  async function send() {
    const text = draft.trim()
    const id = sideId
    if (!text || !id || running) return
    draft = ''
    error = ''
    const token = loadToken
    running = true
    messages = [...messages, { id: `side-user-${Date.now()}`, role: 'user', text }]
    const replyId = `side-reply-${Date.now()}`
    const ac = new AbortController()
    controller = ac
    try {
      await streamChat({ message: text, session_id: id, interactive_permissions: true }, (event) => {
        if (token !== loadToken) return
        messages = applySideEvent(messages, event, replyId)
        void scrollToEnd()
      }, ac.signal)
    } catch (err) {
      if (!(err instanceof DOMException && err.name === 'AbortError')) error = err instanceof Error ? err.message : String(err)
    } finally {
      if (controller === ac) controller = null
      if (token === loadToken) {
        running = false
        messages = withdrawPendingApprovals(messages)
        void reloadHistory(id, token)
      }
    }
  }

  function choose(id: string) {
    sideId = id
    remember(id)
    void load(id)
  }

  function updateApproval(next: ChatApproval) {
    messages = messages.map((m) => (m.approval?.requestId === next.requestId ? { ...m, approval: next } : m))
  }

  // The side session is never the active one; picking it as the main
  // session clears the side.
  $effect(() => {
    if (sideId && sideId === activeSessionId) choose('')
  })

  let started = false
  $effect(() => {
    if (started) return
    started = true
    void load(sideId)
  })

  // A turn started elsewhere (another tab, cron) shows up when the session
  // changes; attach to it then.
  const stopEvents = streamEvents((event) => {
    const id = sideId
    if (!id || event.session_id?.trim() !== id || running) return
    void reloadHistory(id, loadToken).then(() => follow(id, loadToken))
  })

  onDestroy(() => {
    // Only this panel stops listening; the side session's turn runs on.
    controller?.abort()
    stopEvents()
  })
</script>

<div class="side" data-testid="side-session">
  <div class="side-head">
    <select
      class="side-pick"
      aria-label={$t.sideSession.pick}
      value={sideId}
      onchange={(e) => choose((e.currentTarget as HTMLSelectElement).value)}
    >
      <option value="">{$t.sideSession.none}</option>
      {#each candidates as session (session.id)}
        <option value={session.id}>{displaySessionTitle(session.title, $t.chat.session.newChat) || session.id.slice(0, 8)}</option>
      {/each}
    </select>
    {#if sideSession}
      <button type="button" class="btn btn-ghost btn-sm" title={$t.sideSession.openTitle} onclick={() => sideSession && onOpen(sideSession)}>{$t.sideSession.open}</button>
    {/if}
    <button type="button" class="btn btn-ghost btn-sm" aria-label={$t.sideSession.close} onclick={onClose}>×</button>
  </div>

  {#if !sideId}
    <p class="side-empty">{$t.sideSession.hint}</p>
  {:else}
    <div class="side-log" bind:this={logEl}>
      {#each messages as message (message.id)}
        {#if message.role === 'approval' && message.approval}
          <ChatApprovalCard approval={message.approval} onChange={updateApproval} />
        {:else if message.role === 'tool'}
          <div class="side-tool" class:error={message.toolIsError} title={message.toolArgs}>⚙ {formatToolInvocationPreview(message.toolName, message.toolArgs, toolBaseDirs(sideSession, sideCwd))}</div>
        {:else if message.role === 'error'}
          <div class="side-error">{message.text}</div>
        {:else}
          <div class={`side-msg side-${message.role}`}>
            {#if message.role === 'assistant'}
              <MarkdownContent text={stripFocusBlocks(message.text)} />
            {:else}
              {message.text}
            {/if}
          </div>
        {/if}
      {/each}
      {#if running}
        <div class="side-running">{$t.sideSession.running}</div>
      {/if}
    </div>
    {#if error}
      <div class="side-error" role="alert">{error}</div>
    {/if}
    <form class="side-compose" onsubmit={(e) => { e.preventDefault(); void send() }}>
      <textarea
        rows="2"
        placeholder={$t.sideSession.placeholder}
        bind:value={draft}
        disabled={running}
        onkeydown={(e) => {
          if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
            e.preventDefault()
            void send()
          }
        }}
      ></textarea>
      <button type="submit" class="btn btn-primary btn-sm" disabled={running || !draft.trim()}>{$t.sideSession.send}</button>
    </form>
  {/if}
</div>

<style>
  .side {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    height: 100%;
    min-height: 0;
    padding: var(--space-2);
  }

  .side-head {
    display: flex;
    align-items: center;
    gap: var(--space-1);
  }

  .side-pick {
    flex: 1;
    min-width: 0;
    padding: 3px var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-primary);
    font-size: var(--text-sm);
    cursor: pointer;
  }

  .side-pick:focus-visible {
    outline: none;
    border-color: var(--primary);
  }

  .side-empty {
    margin: 0;
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }

  .side-log {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: var(--space-2);
    min-height: 0;
    overflow-y: auto;
  }

  .side-msg {
    padding: var(--space-2);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
  }

  .side-user {
    align-self: flex-end;
    max-width: 90%;
    background: var(--surface-active);
    color: var(--text-primary);
    white-space: pre-wrap;
  }

  .side-assistant {
    background: var(--surface-inset);
  }

  .side-tool {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .side-tool.error {
    color: var(--error);
  }

  .side-error {
    color: var(--error);
    font-size: var(--text-xs);
  }

  .side-running {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  /* Send sits under the box on the left: the companion pet floats over
     the panel's bottom-right corner. */
  .side-compose {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-1);
  }

  .side-compose textarea {
    width: 100%;
    resize: vertical;
    font-size: var(--text-sm);
  }
</style>
