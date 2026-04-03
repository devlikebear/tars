<script lang="ts">
  import { browseFilesystem, type FilesystemBrowseResult } from '../lib/api'

  let { value = $bindable(''), placeholder = 'Select directory...' }: { value: string; placeholder?: string } = $props()

  let open = $state(false)
  let loading = $state(false)
  let error = $state('')
  let result: FilesystemBrowseResult | null = $state(null)
  let manualInput = $state('')

  async function browse(path?: string) {
    loading = true
    error = ''
    try {
      result = await browseFilesystem(path)
      manualInput = result.path
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to browse'
    } finally {
      loading = false
    }
  }

  function openPicker() {
    open = true
    browse(value || undefined)
  }

  function selectDir(name: string) {
    if (!result) return
    browse(result.path + '/' + name)
  }

  function goUp() {
    if (!result?.parent) return
    browse(result.parent)
  }

  function goToManualPath() {
    const p = manualInput.trim()
    if (p) browse(p)
  }

  function confirm() {
    if (result) {
      value = result.path
    }
    open = false
  }

  function cancel() {
    open = false
  }
</script>

<div class="dp-field">
  <input
    type="text"
    {placeholder}
    bind:value
    class="form-input"
    style="flex:1"
  />
  <button type="button" class="btn btn-ghost btn-sm" onclick={openPicker} title="Browse directories">
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M1 3C1 2.44772 1.44772 2 2 2H6L8 4H14C14.5523 4 15 4.44772 15 5V13C15 13.5523 14.5523 14 14 14H2C1.44772 14 1 13.5523 1 13V3Z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
    </svg>
  </button>
</div>

{#if open}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="dp-overlay" onkeydown={(e) => e.key === 'Escape' && cancel()} onclick={cancel}>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="dp-dialog" onclick={(e) => e.stopPropagation()}>
      <div class="dp-header">
        <span class="dp-title">Select Directory</span>
        <button class="btn btn-ghost btn-sm" onclick={cancel}>&times;</button>
      </div>

      <div class="dp-pathbar">
        <input
          type="text"
          class="form-input dp-path-input"
          bind:value={manualInput}
          onkeydown={(e) => e.key === 'Enter' && goToManualPath()}
        />
        <button class="btn btn-ghost btn-sm" onclick={goToManualPath}>Go</button>
      </div>

      {#if error}
        <div class="dp-error">{error}</div>
      {/if}

      <div class="dp-list">
        {#if loading}
          <div class="dp-loading">Loading...</div>
        {:else if result}
          {#if result.parent}
            <button class="dp-entry" onclick={goUp}>
              <span class="dp-icon">..</span>
              <span class="dp-name">(parent)</span>
            </button>
          {/if}
          {#each result.entries as entry}
            <button class="dp-entry" ondblclick={() => selectDir(entry.name)} onclick={() => selectDir(entry.name)}>
              <span class="dp-icon">{entry.is_git ? '\u{1F4C2}' : '\u{1F4C1}'}</span>
              <span class="dp-name">{entry.name}</span>
              {#if entry.is_git}
                <span class="badge badge-accent" style="font-size:0.65rem">git</span>
              {/if}
            </button>
          {/each}
          {#if result.entries.length === 0}
            <div class="dp-empty">No subdirectories</div>
          {/if}
        {/if}
      </div>

      <div class="dp-footer">
        <span class="dp-selected mono">{result?.path || ''}</span>
        <div class="dp-actions">
          <button class="btn btn-ghost btn-sm" onclick={cancel}>Cancel</button>
          <button class="btn btn-primary btn-sm" onclick={confirm} disabled={!result?.path}>Select</button>
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  .dp-field {
    display: flex;
    gap: var(--space-1);
    align-items: center;
  }
  .dp-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    z-index: 1000;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .dp-dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    width: min(560px, 90vw);
    max-height: 70vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
  }
  .dp-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border);
  }
  .dp-title {
    font-weight: 600;
    font-size: 0.95rem;
  }
  .dp-pathbar {
    display: flex;
    gap: var(--space-1);
    padding: var(--space-2) var(--space-4);
    border-bottom: 1px solid var(--border);
  }
  .dp-path-input {
    flex: 1;
    font-size: 0.8rem;
    font-family: var(--font-mono);
  }
  .dp-error {
    padding: var(--space-2) var(--space-4);
    color: var(--fg-error);
    font-size: 0.8rem;
  }
  .dp-list {
    flex: 1;
    overflow-y: auto;
    min-height: 200px;
    max-height: 400px;
    padding: var(--space-1) 0;
  }
  .dp-entry {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-4);
    width: 100%;
    background: none;
    border: none;
    color: var(--fg);
    cursor: pointer;
    text-align: left;
    font-size: 0.85rem;
  }
  .dp-entry:hover {
    background: var(--bg-hover);
  }
  .dp-icon {
    width: 1.2em;
    text-align: center;
    flex-shrink: 0;
  }
  .dp-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .dp-empty, .dp-loading {
    padding: var(--space-4);
    text-align: center;
    color: var(--fg-muted);
    font-size: 0.85rem;
  }
  .dp-footer {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--space-2) var(--space-4);
    border-top: 1px solid var(--border);
    gap: var(--space-2);
  }
  .dp-selected {
    font-size: 0.75rem;
    color: var(--fg-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }
  .dp-actions {
    display: flex;
    gap: var(--space-1);
    flex-shrink: 0;
  }
</style>
