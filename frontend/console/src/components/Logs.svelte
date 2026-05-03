<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { getLogs } from '../lib/api'
  import type { LogFileOption, LogLineView, LogsResponse } from '../lib/types'
  import { t } from '../i18n'

  type LevelFilter = 'all' | 'debug' | 'info' | 'warn' | 'error'

  let levelOptions = $derived<{ value: LevelFilter; label: string }[]>([
    { value: 'all', label: $t.logs.levelOptions.all },
    { value: 'debug', label: $t.logs.levelOptions.debug },
    { value: 'info', label: $t.logs.levelOptions.info },
    { value: 'warn', label: $t.logs.levelOptions.warn },
    { value: 'error', label: $t.logs.levelOptions.error },
  ])
  const lineOptions = [50, 100, 200, 500]

  let snapshot = $state<LogsResponse | null>(null)
  let selectedFile = $state('runtime')
  let selectedLevel: LevelFilter = $state('all')
  let selectedComponent = $state('')
  let lineCount = $state(100)
  let autoRefresh = $state(false)
  let loading = $state(true)
  let error = $state('')
  let refreshTimer: ReturnType<typeof setInterval> | null = null

  let files: LogFileOption[] = $derived.by(() => snapshot?.files ?? [])
  let logLines: LogLineView[] = $derived.by(() => snapshot?.lines ?? [])
  let selectedOption: LogFileOption | undefined = $derived.by(() => files.find((file: LogFileOption) => file.id === selectedFile))

  export async function loadLogs() {
    loading = true
    error = ''
    try {
      const next = await getLogs({
        file: selectedFile,
        level: selectedLevel,
        component: selectedComponent,
        lines: lineCount,
      })
      snapshot = next
      if (next.selected_file) {
        selectedFile = next.selected_file
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load logs'
    } finally {
      loading = false
    }
  }

  function startAutoRefresh() {
    stopAutoRefresh()
    refreshTimer = setInterval(() => {
      void loadLogs()
    }, 5000)
  }

  function stopAutoRefresh() {
    if (refreshTimer) clearInterval(refreshTimer)
    refreshTimer = null
  }

  function handleAutoRefreshChange(event: Event) {
    autoRefresh = (event.currentTarget as HTMLInputElement).checked
    if (autoRefresh) {
      startAutoRefresh()
    } else {
      stopAutoRefresh()
    }
  }

  function handleComponentKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      void loadLogs()
    }
  }

  function selectFile(file: LogFileOption) {
    selectedFile = file.id
    void loadLogs()
  }

  function levelClass(level?: string): string {
    switch ((level || '').toLowerCase()) {
      case 'error':
        return 'level-error'
      case 'warn':
        return 'level-warn'
      case 'debug':
        return 'level-debug'
      case 'info':
        return 'level-info'
      default:
        return 'level-raw'
    }
  }

  function badgeClass(level?: string): string {
    switch ((level || '').toLowerCase()) {
      case 'error':
        return 'badge-error'
      case 'warn':
        return 'badge-warning'
      case 'debug':
        return 'badge-default'
      case 'info':
        return 'badge-info'
      default:
        return 'badge-default'
    }
  }

  function levelLabel(line: LogLineView): string {
    return (line.level || 'raw').toUpperCase()
  }

  function lineText(line: LogLineView): string {
    return line.message?.trim() || line.raw
  }

  function fmtTime(value?: string): string {
    if (!value?.trim()) return ''
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    return new Intl.DateTimeFormat('en', {
      month: 'short',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    }).format(date)
  }

  function fmtSize(bytes?: number): string {
    if (!bytes || bytes <= 0) return '0 B'
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  function fileStatus(file: LogFileOption): string {
    return file.exists ? fmtSize(file.size_bytes) : $t.logs.fileMissing
  }

  onMount(() => {
    void loadLogs()
  })

  onDestroy(() => {
    stopAutoRefresh()
  })
</script>

<div class="logs-page">
  <section class="logs-header">
    <div>
      <span class="logs-kicker">{$t.logs.kicker}</span>
      <h2>{$t.logs.title}</h2>
    </div>
    <div class="logs-actions">
      <label class="auto-toggle">
        <input type="checkbox" checked={autoRefresh} onchange={handleAutoRefreshChange} />
        <span>{$t.logs.autoToggle}</span>
      </label>
      <button class="btn btn-primary btn-sm" type="button" disabled={loading} onclick={() => void loadLogs()}>
        {loading ? $t.logs.refreshing : $t.logs.refresh}
      </button>
    </div>
  </section>

  <section class="logs-toolbar card">
    <label>
      <span>{$t.logs.file}</span>
      <select bind:value={selectedFile} onchange={() => void loadLogs()}>
        {#if files.length === 0}
          <option value="runtime">{$t.logs.runtimeLog}</option>
        {:else}
          {#each files as file}
            <option value={file.id}>{file.label}</option>
          {/each}
        {/if}
      </select>
    </label>

    <label>
      <span>{$t.logs.level}</span>
      <select bind:value={selectedLevel} onchange={() => void loadLogs()}>
        {#each levelOptions as option}
          <option value={option.value}>{option.label}</option>
        {/each}
      </select>
    </label>

    <label>
      <span>{$t.logs.component}</span>
      <input
        type="search"
        placeholder={$t.logs.componentPlaceholder}
        bind:value={selectedComponent}
        onkeydown={handleComponentKeydown}
      />
    </label>

    <label>
      <span>{$t.logs.lines}</span>
      <select bind:value={lineCount} onchange={() => void loadLogs()}>
        {#each lineOptions as count}
          <option value={count}>{count}</option>
        {/each}
      </select>
    </label>
  </section>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  <div class="logs-layout">
    <section class="logs-stream card">
      <div class="card-header">
        <span class="card-title">{$t.logs.streamTitle}</span>
        <div class="logs-meta">
          <span class="badge badge-default">{$t.logs.linesSuffix(snapshot?.count ?? 0)}</span>
          {#if selectedOption}
            <span class="badge" class:badge-success={selectedOption.exists} class:badge-warning={!selectedOption.exists}>
              {fileStatus(selectedOption)}
            </span>
          {/if}
        </div>
      </div>

      {#if loading && !snapshot}
        <div class="empty-state">{$t.logs.loadingLogs}</div>
      {:else if logLines.length === 0}
        <div class="empty-state">{$t.logs.noLines}</div>
      {:else}
        <ol class="log-lines">
          {#each logLines as line}
            <li class="log-line {levelClass(line.level)}">
              <div class="log-row-meta">
                <span class="badge {badgeClass(line.level)}">{levelLabel(line)}</span>
                {#if line.component}
                  <code>{line.component}</code>
                {/if}
                {#if line.time}
                  <span>{fmtTime(line.time)}</span>
                {/if}
              </div>
              <pre>{lineText(line)}</pre>
              {#if line.message && line.raw}
                <details>
                  <summary>{$t.logs.rawSummary}</summary>
                  <code>{line.raw}</code>
                </details>
              {/if}
            </li>
          {/each}
        </ol>
      {/if}
    </section>

    <aside class="logs-files card">
      <div class="card-header">
        <span class="card-title">{$t.logs.filesTitle}</span>
        <span class="badge badge-default">{files.length}</span>
      </div>
      <div class="file-list">
        {#each files as file}
          <button
            type="button"
            class="file-row"
            class:active={file.id === selectedFile}
            onclick={() => selectFile(file)}
          >
            <span>
              <strong>{file.label}</strong>
              <code>{file.path}</code>
            </span>
            <span class="badge" class:badge-success={file.exists} class:badge-warning={!file.exists}>
              {fileStatus(file)}
            </span>
          </button>
        {/each}
      </div>
    </aside>
  </div>
</div>

<style>
  .logs-page {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .logs-header {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .logs-kicker {
    display: block;
    margin-bottom: var(--space-1);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }

  .logs-actions,
  .logs-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .auto-toggle {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-secondary);
    font-size: var(--text-sm);
    white-space: nowrap;
  }

  .auto-toggle input {
    accent-color: var(--primary);
  }

  .logs-toolbar {
    display: grid;
    grid-template-columns: minmax(180px, 1.2fr) minmax(120px, 0.6fr) minmax(180px, 1fr) minmax(110px, 0.5fr);
    gap: var(--space-3);
    align-items: end;
  }

  .logs-toolbar label {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .logs-toolbar label span {
    font-family: var(--font-display);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }

  select {
    width: 100%;
    height: 40px;
    padding: 0 var(--space-3);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    color: var(--text-primary);
  }

  select:focus {
    outline: none;
    border-color: var(--primary);
  }

  .logs-layout {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(260px, 320px);
    gap: var(--space-4);
    align-items: start;
  }

  .logs-stream,
  .logs-files {
    min-width: 0;
  }

  .log-lines {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    list-style: none;
  }

  .log-line {
    border: 1px solid var(--border-subtle);
    border-left-width: 3px;
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    padding: var(--space-3);
  }

  .log-line.level-error {
    border-left-color: var(--error);
    background: var(--error-muted);
  }

  .log-line.level-warn {
    border-left-color: var(--warning);
    background: var(--warning-muted);
  }

  .log-line.level-debug {
    border-left-color: var(--text-tertiary);
  }

  .log-line.level-info {
    border-left-color: var(--info);
  }

  .log-line.level-raw {
    border-left-color: var(--border-strong);
  }

  .log-row-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    flex-wrap: wrap;
  }

  pre {
    white-space: pre-wrap;
    word-break: break-word;
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  details {
    margin-top: var(--space-2);
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }

  details code {
    display: block;
    margin-top: var(--space-1);
    white-space: pre-wrap;
    word-break: break-word;
  }

  .file-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .file-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    width: 100%;
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    color: var(--text-primary);
    text-align: left;
    cursor: pointer;
  }

  .file-row:hover,
  .file-row.active {
    border-color: var(--primary);
    background: var(--primary-muted);
  }

  .file-row > span:first-child {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .file-row strong,
  .file-row code {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .file-row code {
    max-width: 210px;
    color: var(--text-tertiary);
  }

  @media (max-width: 980px) {
    .logs-toolbar,
    .logs-layout {
      grid-template-columns: 1fr;
    }

    .logs-header {
      align-items: flex-start;
      flex-direction: column;
    }
  }
</style>
