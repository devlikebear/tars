<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import {
    getHeartbeatStatus,
    runHeartbeatOnce,
    getHeartbeatConfig,
    saveHeartbeatConfig,
    getHeartbeatLog,
  } from '../lib/api'
  import { renderMarkdown } from '../lib/markdown'
  import type { HeartbeatStatus, HeartbeatRunResult } from '../lib/types'

  let status: HeartbeatStatus | null = $state(null)
  let heartbeatMd = $state('')
  let dailyLog = $state('')
  let dailyLogDate = $state('')

  let loading = $state(true)
  let error = $state('')

  let running = $state(false)
  let runResult: HeartbeatRunResult | null = $state(null)

  let editing = $state(false)
  let editContent = $state('')
  let saving = $state(false)
  let saveMsg = $state('')

  let refreshInterval: ReturnType<typeof setInterval> | null = null

  async function loadStatus() {
    try {
      status = await getHeartbeatStatus()
    } catch { /* ignore */ }
  }

  async function loadConfig() {
    try {
      const result = await getHeartbeatConfig()
      heartbeatMd = result.content
    } catch { heartbeatMd = '' }
  }

  async function loadLog() {
    try {
      const result = await getHeartbeatLog()
      dailyLog = result.content
      dailyLogDate = result.date
    } catch { dailyLog = '' }
  }

  async function loadAll() {
    loading = true
    error = ''
    try {
      await Promise.all([loadStatus(), loadConfig(), loadLog()])
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load'
    } finally {
      loading = false
    }
  }

  async function handleRun() {
    running = true
    runResult = null
    try {
      runResult = await runHeartbeatOnce()
      await loadStatus()
      await loadLog()
    } catch (e) {
      runResult = { response: e instanceof Error ? e.message : 'Run failed' }
    } finally {
      running = false
    }
  }

  function startEdit() {
    editContent = heartbeatMd
    saveMsg = ''
    editing = true
  }

  function cancelEdit() {
    editing = false
    saveMsg = ''
  }

  async function handleSave() {
    saving = true
    saveMsg = ''
    try {
      await saveHeartbeatConfig(editContent)
      heartbeatMd = editContent
      editing = false
      saveMsg = 'Saved'
      setTimeout(() => { saveMsg = '' }, 2000)
    } catch (e) {
      saveMsg = e instanceof Error ? e.message : 'Save failed'
    } finally {
      saving = false
    }
  }

  function fmtTime(value?: string): string {
    if (!value?.trim()) return '\u2014'
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function relativeTime(value?: string): string {
    if (!value?.trim()) return 'never'
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
    if (seconds < 60) return `${seconds}s ago`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
    return `${Math.floor(seconds / 86400)}d ago`
  }

  onMount(() => {
    void loadAll()
    refreshInterval = setInterval(loadStatus, 30000)
  })

  onDestroy(() => {
    if (refreshInterval) clearInterval(refreshInterval)
  })
</script>

<div class="hb">
  {#if loading}
    <div class="hb-loading">Loading heartbeat...</div>
  {:else}
    {#if error}
      <div class="error-banner" style="margin-bottom:var(--space-4)">{error}</div>
    {/if}

    <!-- Status -->
    <section class="card">
      <div class="card-header">
        <span class="card-title">Status</span>
        {#if status?.configured}
          <span class="badge badge-success">configured</span>
        {:else}
          <span class="badge badge-default">not configured</span>
        {/if}
        {#if status?.chat_busy}
          <span class="badge badge-warning">chat busy</span>
        {/if}
      </div>
      <dl class="hb-facts">
        <div><dt>Active Hours</dt><dd>{status?.active_hours || 'always'}</dd></div>
        <div><dt>Timezone</dt><dd>{status?.timezone || 'system'}</dd></div>
        <div><dt>Last Run</dt><dd>{relativeTime(status?.last_run_at)}</dd></div>
      </dl>
    </section>

    <!-- Last Result + Run Action -->
    <section class="card">
      <div class="card-header">
        <span class="card-title">Last Result</span>
        <div class="hb-badges">
          {#if status?.last_skipped}
            <span class="badge badge-warning">skipped</span>
          {/if}
          {#if status?.last_acknowledged}
            <span class="badge badge-success">acknowledged</span>
          {/if}
          {#if status?.last_logged}
            <span class="badge badge-info">logged</span>
          {/if}
          {#if status?.last_error}
            <span class="badge badge-error">error</span>
          {/if}
        </div>
      </div>
      {#if status?.last_error}
        <div class="hb-error">{status.last_error}</div>
      {/if}
      {#if status?.last_skip_reason}
        <div class="hb-skip-reason">Skip reason: {status.last_skip_reason}</div>
      {/if}
      {#if status?.last_response}
        <div class="hb-response">{status.last_response}</div>
      {:else if !status?.last_error && !status?.last_skip_reason}
        <div class="hb-empty">No heartbeat result yet.</div>
      {/if}

      <div class="hb-actions">
        <button class="btn btn-primary btn-sm" disabled={running || status?.chat_busy} onclick={handleRun}>
          {running ? 'Running...' : 'Run Now'}
        </button>
        {#if status?.chat_busy}
          <span class="hb-hint">Paused while chat is active</span>
        {/if}
      </div>

      {#if runResult}
        <div class="hb-run-result">
          <div class="hb-run-result-header">
            <strong>Run result</strong>
            <div class="hb-badges">
              {#if runResult.skipped}<span class="badge badge-warning">skipped</span>{/if}
              {#if runResult.acknowledged}<span class="badge badge-success">ack</span>{/if}
              {#if runResult.logged}<span class="badge badge-info">logged</span>{/if}
            </div>
          </div>
          {#if runResult.skip_reason}
            <div class="hb-skip-reason">{runResult.skip_reason}</div>
          {/if}
          {#if runResult.response}
            <div class="hb-response">{runResult.response}</div>
          {/if}
        </div>
      {/if}
    </section>

    <!-- HEARTBEAT.md -->
    <section class="card">
      <div class="card-header">
        <span class="card-title">HEARTBEAT.md</span>
        {#if !editing}
          <button class="btn btn-ghost btn-sm" onclick={startEdit}>Edit</button>
        {/if}
        {#if saveMsg}
          <span class="hb-save-msg">{saveMsg}</span>
        {/if}
      </div>
      {#if editing}
        <textarea class="hb-editor" bind:value={editContent} rows="12"></textarea>
        <div class="hb-editor-actions">
          <button class="btn btn-primary btn-sm" disabled={saving} onclick={handleSave}>
            {saving ? 'Saving...' : 'Save'}
          </button>
          <button class="btn btn-ghost btn-sm" onclick={cancelEdit}>Cancel</button>
        </div>
      {:else if heartbeatMd}
        <div class="hb-md-preview hb-md">{@html renderMarkdown(heartbeatMd)}</div>
      {:else}
        <div class="hb-empty">No HEARTBEAT.md configured. Click Edit to create one.</div>
      {/if}
    </section>

    <!-- Today's Log -->
    <section class="card">
      <div class="card-header">
        <span class="card-title">Daily Log</span>
        {#if dailyLogDate}
          <span class="badge badge-default">{dailyLogDate}</span>
        {/if}
        <button class="btn btn-ghost btn-sm" onclick={loadLog}>Refresh</button>
      </div>
      {#if dailyLog}
        <div class="hb-log hb-md">{@html renderMarkdown(dailyLog)}</div>
      {:else}
        <div class="hb-empty">No heartbeat entries for today.</div>
      {/if}
    </section>
  {/if}
</div>

<style>
  .hb {
    display: grid;
    gap: var(--space-4);
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .hb-loading {
    padding: var(--space-10);
    text-align: center;
    color: var(--text-tertiary);
  }

  .hb-facts {
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-3);
  }

  .hb-facts div {
    display: grid;
    grid-template-columns: 120px minmax(0, 1fr);
    gap: var(--space-3);
  }

  .hb-facts dt {
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }

  .hb-facts dd {
    margin: 0;
    font-size: var(--text-sm);
  }

  .hb-badges {
    display: flex;
    gap: var(--space-1);
  }

  .hb-response {
    padding: var(--space-3);
    background: var(--bg-base);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
    line-height: 1.6;
    white-space: pre-wrap;
    color: var(--text-secondary);
    margin-top: var(--space-2);
  }

  .hb-error {
    padding: var(--space-2) var(--space-3);
    background: var(--error-muted);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
    color: var(--error);
    margin-top: var(--space-2);
  }

  .hb-skip-reason {
    font-size: var(--text-xs);
    color: var(--text-ghost);
    margin-top: var(--space-1);
  }

  .hb-empty {
    padding: var(--space-3);
    color: var(--text-ghost);
    font-size: var(--text-sm);
  }

  .hb-actions {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-top: var(--space-3);
  }

  .hb-hint {
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  .hb-run-result {
    margin-top: var(--space-3);
    padding: var(--space-3);
    background: rgba(224, 145, 69, 0.06);
    border: 1px solid rgba(224, 145, 69, 0.12);
    border-radius: var(--radius-md);
  }

  .hb-run-result-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--space-2);
  }

  .hb-run-result-header strong {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
  }

  .hb-editor {
    width: 100%;
    min-height: 200px;
    padding: var(--space-3);
    background: var(--bg-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    line-height: 1.6;
    resize: vertical;
    margin-top: var(--space-2);
  }

  .hb-editor:focus {
    outline: none;
    border-color: var(--accent);
  }

  .hb-editor-actions {
    display: flex;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }

  .hb-save-msg {
    font-size: var(--text-xs);
    color: var(--success);
  }

  .hb-md-preview {
    padding: var(--space-3);
    font-size: var(--text-sm);
    line-height: 1.6;
    color: var(--text-secondary);
  }

  .hb-md :global(h1), .hb-md :global(h2), .hb-md :global(h3) {
    font-family: var(--font-display);
    font-weight: 600;
    color: var(--text-primary);
    margin: var(--space-3) 0 var(--space-1);
  }
  .hb-md :global(p) { margin: 0 0 var(--space-2); }
  .hb-md :global(ul), .hb-md :global(ol) { margin: var(--space-1) 0; padding-left: var(--space-5); }
  .hb-md :global(li) { margin-bottom: var(--space-1); font-size: var(--text-sm); }
  .hb-md :global(code) {
    font-family: var(--font-mono);
    font-size: 0.9em;
    background: rgba(255,255,255,0.06);
    padding: 1px 5px;
    border-radius: 3px;
  }
  .hb-md :global(pre) {
    background: var(--bg-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: var(--space-2);
    overflow-x: auto;
    margin: var(--space-2) 0;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  .hb-md :global(strong) { font-weight: 600; color: var(--text-primary); }

  .hb-log {
    padding: var(--space-3);
    font-size: var(--text-sm);
    line-height: 1.6;
    color: var(--text-secondary);
    max-height: 400px;
    overflow-y: auto;
  }
</style>
