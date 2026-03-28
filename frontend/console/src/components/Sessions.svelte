<script lang="ts">
  import { onMount } from 'svelte'
  import { listSessions, getSessionHistory } from '../lib/api'
  import { renderMarkdown } from '../lib/markdown'
  import type { Session, SessionMessage } from '../lib/types'

  let sessions: Session[] = $state([])
  let loading = $state(true)
  let error = $state('')
  let showHidden = $state(false)

  let selectedSession: Session | null = $state(null)
  let history: SessionMessage[] = $state([])
  let historyLoading = $state(false)
  let historyError = $state('')

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function kindLabel(session: Session): string {
    if (session.kind === 'main') return 'main'
    if (session.hidden) return 'worker'
    return session.kind || 'session'
  }

  function kindBadge(session: Session): string {
    if (session.kind === 'main') return 'badge-accent'
    if (session.hidden) return 'badge-default'
    return 'badge-info'
  }

  async function load() {
    loading = true
    error = ''
    try {
      sessions = await listSessions(showHidden)
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load sessions'
    } finally {
      loading = false
    }
  }

  async function selectSession(session: Session) {
    if (selectedSession?.id === session.id) {
      selectedSession = null
      history = []
      return
    }
    selectedSession = session
    historyLoading = true
    historyError = ''
    history = []
    try {
      history = await getSessionHistory(session.id)
    } catch (err) {
      historyError = err instanceof Error ? err.message : 'Failed to load history'
    } finally {
      historyLoading = false
    }
  }

  function toggleHidden() {
    showHidden = !showHidden
    selectedSession = null
    history = []
    void load()
  }

  onMount(() => { void load() })
</script>

<div class="sessions">
  <div class="sessions-header">
    <div>
      <h2>Sessions</h2>
      <p class="sessions-subtitle">Chat sessions and worker transcripts.</p>
    </div>
    <label class="sessions-toggle">
      <input type="checkbox" checked={showHidden} onchange={toggleHidden} />
      <span>Show worker sessions</span>
    </label>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  {#if loading}
    <div class="sessions-loading">Loading sessions...</div>
  {:else if sessions.length === 0}
    <div class="empty-state"><p>No sessions found.</p></div>
  {:else}
    <div class="sessions-layout">
      <div class="sessions-list">
        {#each sessions as session}
          <button
            type="button"
            class="session-item"
            class:active={selectedSession?.id === session.id}
            onclick={() => { void selectSession(session) }}
          >
            <div class="session-item-top">
              <strong class="session-item-title">{session.title || session.id}</strong>
              <span class="badge {kindBadge(session)}">{kindLabel(session)}</span>
            </div>
            <div class="session-item-meta">
              <span class="mono">{session.id}</span>
              {#if session.project_id}
                <span>project: {session.project_id}</span>
              {/if}
              <span>{fmt(session.updated_at)}</span>
            </div>
          </button>
        {/each}
      </div>

      {#if selectedSession}
        <div class="session-detail">
          <div class="session-detail-header">
            <h3>{selectedSession.title || selectedSession.id}</h3>
            <span class="badge {kindBadge(selectedSession)}">{kindLabel(selectedSession)}</span>
          </div>
          <dl class="session-facts">
            <div><dt>ID</dt><dd class="mono">{selectedSession.id}</dd></div>
            <div><dt>Kind</dt><dd>{selectedSession.kind || '\u2014'}</dd></div>
            {#if selectedSession.project_id}
              <div><dt>Project</dt><dd class="mono">{selectedSession.project_id}</dd></div>
            {/if}
            <div><dt>Created</dt><dd>{fmt(selectedSession.created_at)}</dd></div>
            <div><dt>Updated</dt><dd>{fmt(selectedSession.updated_at)}</dd></div>
          </dl>

          <div class="session-transcript">
            <span class="card-title">Transcript</span>
            {#if historyLoading}
              <div class="sessions-loading">Loading transcript...</div>
            {:else if historyError}
              <div class="error-banner">{historyError}</div>
            {:else if history.length === 0}
              <div class="empty-state"><p>No messages in this session.</p></div>
            {:else}
              <div class="transcript-messages">
                {#each history as msg}
                  <div class="transcript-msg transcript-{msg.role}">
                    <div class="transcript-msg-top">
                      <span class="transcript-role">{msg.role}</span>
                      <span class="transcript-time">{fmt(msg.timestamp)}</span>
                    </div>
                    {#if msg.role === 'assistant'}
                      <div class="transcript-content transcript-md">{@html renderMarkdown(msg.content || '')}</div>
                    {:else}
                      <div class="transcript-content">{msg.content || ''}</div>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .sessions {
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .sessions-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-6);
  }

  .sessions-header h2 {
    font-size: var(--text-2xl);
    margin-bottom: var(--space-1);
  }

  .sessions-subtitle {
    color: var(--text-tertiary);
  }

  .sessions-toggle {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    color: var(--text-secondary);
    cursor: pointer;
    flex-shrink: 0;
  }

  .sessions-toggle input {
    accent-color: var(--accent);
  }

  .sessions-loading {
    padding: var(--space-10);
    text-align: center;
    color: var(--text-tertiary);
  }

  /* ── Layout ────────────────────────────────── */
  .sessions-layout {
    display: grid;
    grid-template-columns: 360px minmax(0, 1fr);
    gap: var(--space-4);
    align-items: start;
  }

  /* ── List ──────────────────────────────────── */
  .sessions-list {
    display: grid;
    gap: var(--space-2);
    max-height: calc(100vh - 200px);
    overflow-y: auto;
  }

  .session-item {
    display: block;
    width: 100%;
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--bg-surface);
    text-align: left;
    cursor: pointer;
    transition:
      border-color var(--duration-fast) var(--ease-out),
      background var(--duration-fast) var(--ease-out);
  }

  .session-item:hover {
    border-color: var(--border-default);
    background: var(--bg-elevated);
  }

  .session-item.active {
    border-color: var(--accent);
    background: var(--accent-muted);
  }

  .session-item-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: var(--space-1);
  }

  .session-item-title {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }

  .session-item-meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-3);
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  /* ── Detail ────────────────────────────────── */
  .session-detail {
    background: var(--bg-surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    padding: var(--space-5);
  }

  .session-detail-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }

  .session-detail-header h3 {
    font-size: var(--text-lg);
    font-weight: 500;
  }

  .session-facts {
    display: grid;
    gap: var(--space-2);
    margin-bottom: var(--space-5);
    padding-bottom: var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
  }

  .session-facts div {
    display: grid;
    grid-template-columns: 80px minmax(0, 1fr);
    gap: var(--space-3);
  }

  .session-facts dt {
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }

  .session-facts dd {
    margin: 0;
    font-size: var(--text-sm);
    word-break: break-word;
  }

  /* ── Transcript ────────────────────────────── */
  .session-transcript {
    margin-top: var(--space-3);
  }

  .transcript-messages {
    display: grid;
    gap: var(--space-2);
    max-height: 600px;
    overflow-y: auto;
    margin-top: var(--space-3);
  }

  .transcript-msg {
    padding: var(--space-3);
    border-radius: var(--radius-md);
    background: var(--bg-base);
  }

  .transcript-user {
    background: rgba(224, 145, 69, 0.08);
    border: 1px solid rgba(224, 145, 69, 0.12);
  }

  .transcript-assistant {
    background: var(--bg-elevated);
  }

  .transcript-system {
    background: transparent;
    opacity: 0.6;
    padding: var(--space-2) var(--space-3);
  }

  .transcript-msg-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: var(--space-1);
  }

  .transcript-role {
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 500;
    color: var(--text-tertiary);
  }

  .transcript-time {
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  .transcript-content {
    white-space: pre-wrap;
    font-size: var(--text-sm);
    line-height: 1.55;
  }

  .transcript-md {
    white-space: normal;
  }

  .transcript-md :global(p) {
    margin: 0 0 var(--space-2);
  }

  .transcript-md :global(p:last-child) {
    margin-bottom: 0;
  }

  .transcript-md :global(pre) {
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

  .transcript-md :global(code) {
    font-family: var(--font-mono);
    font-size: 0.9em;
    background: rgba(255, 255, 255, 0.06);
    padding: 1px 5px;
    border-radius: 3px;
  }

  .transcript-md :global(pre code) {
    background: none;
    padding: 0;
  }

  .transcript-md :global(strong) {
    font-weight: 600;
    color: var(--text-primary);
  }

  .transcript-md :global(ul),
  .transcript-md :global(ol) {
    margin: var(--space-2) 0;
    padding-left: var(--space-5);
  }

  .transcript-md :global(li) {
    margin-bottom: var(--space-1);
    font-size: var(--text-sm);
  }

  /* ── Responsive ────────────────────────────── */
  @media (max-width: 900px) {
    .sessions-layout {
      grid-template-columns: 1fr;
    }
  }
</style>
