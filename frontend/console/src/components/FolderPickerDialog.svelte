<script lang="ts">
  // Shared folder browser: a typed or pasted path, a filter, folded dot
  // folders and "New folder" over GET/POST /v1/filesystem/browse. Pulled out
  // of ArtifactPanel's "+" directory picker so Focus mode's new-task form can
  // put the same browser in a modal, without a second server surface.
  //
  // This component renders the browser's content only (header, path input,
  // toolbar, list) — it fills whatever container it's placed in. ArtifactPanel
  // keeps it inline (fills the panel), FocusNewTask wraps it with a modal
  // backdrop.
  import { onMount } from 'svelte'
  import { t } from '../i18n'
  import { browseFilesystem, createFilesystemDirectory, APIRequestError, type WorkspaceFileEntry } from '../lib/api'
  import { PickerRequests, arrangePickerEntries, browseErrorReason, resolvePickerPath } from '../lib/folderPicker'

  interface Props {
    // A folder was chosen with "Select Here".
    onSelect: (path: string) => void
    onCancel: () => void
    // Opens here instead of the server's home folder. The home folder is
    // still fetched in the background so a typed `~` keeps working.
    initialPath?: string
    // Hides "New folder" for callers that only want to pick an existing one.
    allowCreate?: boolean
  }

  let { onSelect, onCancel, initialPath, allowCreate = true }: Props = $props()

  let pickPath = $state('')
  let pickParent = $state('')
  let pickFiles: WorkspaceFileEntry[] = $state([])
  let pickLoading = $state(false)
  let pickActionError = $state('')
  let pickActionBusy = $state(false)
  let pickCreatingFolder = $state(false)
  let pickNewFolderName = $state('')
  // Typed/pasted path, filter, and the collapsed dot-folder group (#picker).
  let pickHome = $state('')
  let pickPathInput = $state('')
  let pickFilter = $state('')
  let pickShowHidden = $state(false)
  const pickRequests = new PickerRequests()
  let pickArranged = $derived(arrangePickerEntries(pickFiles, pickFilter))
  let pickFiltering = $derived(pickFilter.trim() !== '')

  function joinFilesystemPath(parent: string, name: string): string {
    if (parent === '/') return `/${name}`
    return `${parent.replace(/\/+$/, '')}/${name}`
  }

  // A failed browse keeps the current listing and says why, so a mistyped
  // path or an unreadable folder doesn't lose the user's place.
  async function browsePick(path: string | undefined) {
    const request = pickRequests.start(pickPathInput)
    pickLoading = true
    pickActionError = ''
    try {
      const result = await browseFilesystem(path)
      const settled = pickRequests.settle(request, pickPathInput, result.path)
      if (!settled.current) return
      pickFiles = result.entries.filter(e => e.is_dir).map(e => ({
        name: e.name,
        path: joinFilesystemPath(result.path, e.name),
        is_dir: true,
      }))
      if (path === undefined) pickHome = result.path
      pickPath = result.path
      pickParent = result.parent
      pickPathInput = settled.pathInput
      pickFilter = ''
    } catch (err) {
      if (!pickRequests.isCurrent(request)) return
      const reason = browseErrorReason(err instanceof APIRequestError ? err.status : undefined)
      pickActionError = $t.artifactPanel.picker.errors[reason](path ?? '~')
    } finally {
      if (pickRequests.isCurrent(request)) pickLoading = false
    }
  }

  async function submitPickPath() {
    const resolved = resolvePickerPath(pickPathInput, pickHome)
    if (!resolved.ok) {
      if (resolved.reason === 'empty') {
        pickPathInput = pickPath
        pickActionError = ''
      } else {
        pickActionError = $t.artifactPanel.picker.errors.relative
      }
      return
    }
    await browsePick(resolved.path)
  }

  function onPickPathKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault()
      void submitPickPath()
    } else if (e.key === 'Escape') {
      // Reverts the typed edit — must not also bubble to an ancestor's
      // Escape-to-close (FocusNewTask's modal wraps this dialog), or
      // reverting a typo would close the whole picker.
      e.preventDefault()
      e.stopPropagation()
      pickPathInput = pickPath
      pickActionError = ''
    }
  }

  function onPickFilterKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault()
      const first = pickArranged.shown[0] ?? pickArranged.hidden[0]
      if (first) void browsePick(first.path)
    } else if (e.key === 'Escape' && pickFilter) {
      // Same reasoning as the path field: clearing the filter is this
      // keypress's job, not closing an ancestor modal.
      e.preventDefault()
      e.stopPropagation()
      pickFilter = ''
    }
  }

  function selectHere() {
    if (!pickPath) return
    onSelect(pickPath)
  }

  function beginPickCreateFolder() {
    pickActionError = ''
    pickCreatingFolder = true
    pickNewFolderName = ''
  }

  function cancelPickCreateFolder() {
    pickCreatingFolder = false
    pickNewFolderName = ''
  }

  async function submitPickCreateFolder() {
    const name = pickNewFolderName.trim()
    if (!name || !pickPath || pickActionBusy) return
    pickActionBusy = true
    pickActionError = ''
    try {
      const created = await createFilesystemDirectory(pickPath, name)
      pickCreatingFolder = false
      pickNewFolderName = ''
      await browsePick(created.path)
    } catch (err) {
      pickActionError = err instanceof Error ? err.message : $t.artifactPanel.errors.createFolder
    } finally {
      pickActionBusy = false
    }
  }

  onMount(() => {
    if (initialPath) {
      void browsePick(initialPath)
      // The home folder for `~` expansion, fetched in the background —
      // opening at initialPath must not wait on a second round trip.
      void browseFilesystem(undefined).then((result) => { pickHome = pickHome || result.path }).catch(() => {})
    } else {
      void browsePick(undefined)
    }
  })
</script>

{#snippet pickEntry(entry: WorkspaceFileEntry)}
  <button type="button" class="artifact-item" onclick={() => browsePick(entry.path)}>
    <span class="artifact-icon">&#x1f4c1;</span>
    <span class="artifact-name">{entry.name}</span>
  </button>
{/snippet}

<div class="pick-overlay">
  <div class="pick-header">
    <span class="pick-title">{$t.artifactPanel.picker.title}</span>
    <div class="pick-actions">
      <button type="button" class="btn btn-primary btn-sm" disabled={pickLoading || pickActionBusy || !pickPath} onclick={selectHere}>{$t.artifactPanel.picker.selectHere}</button>
      <button type="button" class="btn btn-ghost btn-sm" disabled={pickActionBusy} onclick={onCancel}>{$t.artifactPanel.actions.cancel}</button>
    </div>
  </div>
  <input
    class="pick-current"
    type="text"
    spellcheck="false"
    autocomplete="off"
    aria-label={$t.artifactPanel.picker.pathLabel}
    title={$t.artifactPanel.picker.pathTitle}
    bind:value={pickPathInput}
    onkeydown={onPickPathKeydown}
  />
  {#if allowCreate}
    <div class="pick-toolbar">
      {#if pickCreatingFolder}
        <div class="ws-inline-form">
          <span class="artifact-icon">&#x1f4c1;</span>
          <input
            class="ws-inline-input"
            bind:value={pickNewFolderName}
            placeholder={$t.artifactPanel.folder.newFolderPlaceholder}
            onkeydown={(e) => {
              if (e.key === 'Enter') submitPickCreateFolder()
              if (e.key === 'Escape') {
                // Cancels just this inline prompt — must not also bubble to
                // an ancestor's Escape-to-close (FocusNewTask's modal wraps
                // this dialog), same reasoning as the path/filter fields.
                e.stopPropagation()
                cancelPickCreateFolder()
              }
            }}
          />
          <button type="button" class="btn btn-primary btn-sm" disabled={pickActionBusy || !pickNewFolderName.trim()} onclick={submitPickCreateFolder}>{$t.artifactPanel.actions.create}</button>
          <button type="button" class="btn btn-ghost btn-sm" disabled={pickActionBusy} onclick={cancelPickCreateFolder}>{$t.artifactPanel.actions.cancel}</button>
        </div>
      {:else}
        <button type="button" class="btn btn-ghost btn-sm" disabled={pickLoading || pickActionBusy || !pickPath} onclick={beginPickCreateFolder}>{$t.artifactPanel.folder.newFolder}</button>
      {/if}
    </div>
  {/if}
  {#if pickActionError}
    <div class="pick-error" role="alert">{pickActionError}</div>
  {/if}
  {#if pickFiles.length > 0}
    <div class="pick-filter">
      <input
        class="ws-inline-input"
        type="search"
        spellcheck="false"
        autocomplete="off"
        aria-label={$t.artifactPanel.picker.filterLabel}
        placeholder={$t.artifactPanel.picker.filterPlaceholder}
        bind:value={pickFilter}
        onkeydown={onPickFilterKeydown}
      />
    </div>
  {/if}
  <div class="pick-list">
    {#if pickLoading}
      <div class="artifact-empty">{$t.artifactPanel.loading}</div>
    {:else}
      {#if pickParent}
        <button type="button" class="artifact-item" onclick={() => browsePick(pickParent)}>
          <span class="artifact-icon">&#x2191;</span>
          <span class="artifact-name">..</span>
        </button>
      {/if}
      {#each pickArranged.shown as entry (entry.path)}
        {@render pickEntry(entry)}
      {/each}
      {#if pickArranged.hidden.length > 0}
        {#if pickFiltering}
          {#each pickArranged.hidden as entry (entry.path)}
            {@render pickEntry(entry)}
          {/each}
        {:else}
          <button
            type="button"
            class="pick-hidden-toggle"
            aria-expanded={pickShowHidden}
            onclick={() => (pickShowHidden = !pickShowHidden)}
          >
            <span class="pick-hidden-caret" class:open={pickShowHidden}>&#x25B8;</span>
            {$t.artifactPanel.picker.hiddenToggle(pickArranged.hidden.length)}
          </button>
          {#if pickShowHidden}
            {#each pickArranged.hidden as entry (entry.path)}
              {@render pickEntry(entry)}
            {/each}
          {/if}
        {/if}
      {/if}
      {#if pickFiles.length === 0 && pickParent}
        <div class="artifact-empty">{$t.artifactPanel.picker.noSubdirectories}</div>
      {:else if pickFiltering && pickArranged.shown.length === 0 && pickArranged.hidden.length === 0}
        <div class="artifact-empty">{$t.artifactPanel.picker.noMatches(pickFilter.trim())}</div>
      {/if}
    {/if}
  </div>
</div>

<style>
  .pick-overlay {
    display: flex;
    flex-direction: column;
    flex: 1;
    overflow: hidden;
    border-top: 1px solid var(--primary);
    background: var(--surface);
  }

  .pick-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
  }

  .pick-title {
    font-family: var(--font-display);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--primary-text);
  }

  .pick-actions {
    display: flex;
    gap: var(--space-1);
  }

  .pick-current {
    flex-shrink: 0;
    width: 100%;
    min-width: 0;
    padding: var(--space-1) var(--space-3);
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-primary);
    background: var(--surface-inset);
    border: 1px solid transparent;
    border-bottom-color: var(--border-subtle);
    border-radius: 0;
    outline: none;
  }

  .pick-current:focus {
    border-color: var(--primary);
  }

  .pick-filter {
    padding: var(--space-2) var(--space-3) 0;
    flex-shrink: 0;
  }

  .pick-filter .ws-inline-input {
    width: 100%;
  }

  .pick-filter .ws-inline-input:focus {
    outline: none;
    border-color: var(--primary);
  }

  .pick-hidden-toggle {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    margin-top: var(--space-2);
    padding: 4px 8px;
    border: none;
    border-top: 1px solid var(--border-subtle);
    background: transparent;
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: 10px;
    text-align: left;
    cursor: pointer;
  }

  .pick-hidden-toggle:hover {
    color: var(--text-primary);
  }

  .pick-hidden-caret {
    display: inline-block;
    transition: transform var(--duration-fast) var(--ease-out);
  }

  .pick-hidden-caret.open {
    transform: rotate(90deg);
  }

  .pick-toolbar {
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
  }

  .pick-error {
    padding: 0 var(--space-3) var(--space-2);
    color: var(--error);
    font-size: 10px;
    flex-shrink: 0;
  }

  .pick-list {
    flex: 1;
    overflow-y: auto;
    padding: var(--space-2);
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .artifact-empty {
    padding: var(--space-4);
    text-align: center;
    color: var(--text-ghost);
    font-size: var(--text-xs);
  }

  .ws-inline-form {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .ws-inline-input {
    flex: 1;
    min-width: 0;
    padding: 6px 8px;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: 11px;
  }

  .artifact-item {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2);
    border-radius: var(--radius-sm);
    background: none;
    border: none;
    width: 100%;
    text-align: left;
    cursor: pointer;
    color: var(--text-primary);
    transition: background var(--duration-fast) var(--ease-out);
  }
  .artifact-item:hover { background: var(--surface-hover); }

  .artifact-icon {
    font-size: var(--text-md);
    flex-shrink: 0;
    width: 20px;
    text-align: center;
  }

  .artifact-name {
    display: block;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
