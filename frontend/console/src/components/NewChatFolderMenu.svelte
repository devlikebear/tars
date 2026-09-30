<script lang="ts">
  // "New chat in a folder": the caret beside the board's New chat and the
  // sidebar's + New Chat. It offers the folders recent sessions worked in and
  // a path field the server checks as it will on create; in a git repository
  // a checkbox isolates the chat in a worktree. The session is created here
  // in one call (lib/newChat.ts), so a failure stays in the panel.
  import { onDestroy } from 'svelte'
  import { t } from '../i18n'
  import { APIRequestError, checkSessionFolder, createSession, listRecentSessionFolders, type SessionFolder } from '../lib/api'
  import { newChatRequest } from '../lib/newChat'
  import { shortCwdLabel } from '../lib/sessionLabels'
  import type { Session } from '../lib/types'

  interface Props {
    onCreated: (session: Session) => void
    // right: a popover under the button's row, right-aligned (board);
    // stretch: spans the positioned parent (sidebar header).
    align?: 'right' | 'stretch'
    buttonClass?: string
  }

  let { onCreated, align = 'right', buttonClass = 'btn-secondary' }: Props = $props()

  type Check =
    | { state: 'idle' }
    | { state: 'checking' }
    | { state: 'ok'; folder: SessionFolder }
    | { state: 'error'; message: string }

  let open = $state(false)
  let recent = $state<SessionFolder[]>([])
  let recentLoaded = $state(false)
  let path = $state('')
  let check = $state<Check>({ state: 'idle' })
  let isolate = $state(false)
  let busy = $state(false)
  let error = $state('')
  let root: HTMLElement | undefined = $state()
  let input: HTMLInputElement | undefined = $state()
  let timer: ReturnType<typeof setTimeout> | null = null
  let checkSeq = 0

  let folder = $derived(check.state === 'ok' ? check.folder : null)
  let canIsolate = $derived(!!folder?.repo_root)

  function clearTimer() {
    if (timer) clearTimeout(timer)
    timer = null
  }

  onDestroy(clearTimer)

  async function toggle() {
    if (open) {
      close()
      return
    }
    open = true
    error = ''
    queueMicrotask(() => input?.focus())
    try {
      recent = await listRecentSessionFolders()
    } catch {
      recent = []
    } finally {
      recentLoaded = true
    }
  }

  function close() {
    open = false
    clearTimer()
    checkSeq++
    path = ''
    check = { state: 'idle' }
    isolate = false
    error = ''
  }

  function checkError(err: unknown): string {
    if (err instanceof APIRequestError && err.status === 404) return $t.newChatFolder.notFound
    if (err instanceof APIRequestError && err.status === 400) return $t.newChatFolder.notAFolder
    return err instanceof Error ? err.message : String(err)
  }

  async function runCheck(value: string): Promise<SessionFolder | null> {
    const seq = ++checkSeq
    check = { state: 'checking' }
    try {
      const found = await checkSessionFolder(value)
      if (seq !== checkSeq) return null
      check = { state: 'ok', folder: found }
      if (!found.repo_root) isolate = false
      return found
    } catch (err) {
      if (seq === checkSeq) check = { state: 'error', message: checkError(err) }
      return null
    }
  }

  function onPathInput() {
    error = ''
    clearTimer()
    checkSeq++
    const value = path.trim()
    if (!value) {
      check = { state: 'idle' }
      return
    }
    check = { state: 'checking' }
    timer = setTimeout(() => void runCheck(value), 250)
  }

  function pick(item: SessionFolder) {
    clearTimer()
    checkSeq++
    path = item.path
    error = ''
    check = { state: 'ok', folder: item }
    if (!item.repo_root) isolate = false
  }

  async function start() {
    if (busy) return
    let target = folder
    if (!target || target.path !== path.trim()) {
      const value = path.trim()
      if (!value) return
      clearTimer()
      target = await runCheck(value)
      if (!target) return
    }
    busy = true
    error = ''
    try {
      const session = await createSession(undefined, newChatRequest(target.path, isolate && !!target.repo_root))
      close()
      onCreated(session)
    } catch (err) {
      error = $t.newChatFolder.failed(err instanceof Error ? err.message : String(err))
    } finally {
      busy = false
    }
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.stopPropagation()
      close()
    } else if (event.key === 'Enter' && event.target === input && !event.isComposing) {
      event.preventDefault()
      void start()
    }
  }

  $effect(() => {
    if (!open) return
    function onDocumentClick(event: MouseEvent) {
      if (root && !root.contains(event.target as Node)) close()
    }
    document.addEventListener('click', onDocumentClick)
    return () => document.removeEventListener('click', onDocumentClick)
  })
</script>

<div class="ncf" class:stretch={align === 'stretch'} bind:this={root}>
  <button
    type="button"
    class="btn btn-sm ncf-toggle {buttonClass}"
    aria-label={$t.newChatFolder.open}
    title={$t.newChatFolder.open}
    aria-haspopup="dialog"
    aria-expanded={open}
    data-testid="new-chat-folder"
    onclick={(event) => { event.stopPropagation(); void toggle() }}
  >▾</button>

  {#if open}
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div class="ncf-panel" role="dialog" aria-label={$t.newChatFolder.title} tabindex="-1" onkeydown={onKeydown}>
      <strong class="ncf-title">{$t.newChatFolder.title}</strong>

      <div class="ncf-section">
        <span class="ncf-label">{$t.newChatFolder.recent}</span>
        {#if recentLoaded && recent.length === 0}
          <p class="ncf-note">{$t.newChatFolder.noRecent}</p>
        {:else}
          <ul class="ncf-recent">
            {#each recent as item (item.path)}
              <li>
                <button
                  type="button"
                  class="ncf-folder"
                  class:selected={folder?.path === item.path}
                  title={item.path}
                  onclick={() => pick(item)}
                >
                  <span class="mono ncf-path">{shortCwdLabel(item.path)}</span>
                  {#if item.repo_root}<span class="ncf-tag">git</span>{/if}
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>

      <label class="ncf-section">
        <span class="ncf-label">{$t.newChatFolder.path}</span>
        <input
          bind:this={input}
          bind:value={path}
          oninput={onPathInput}
          class="ncf-input mono"
          type="text"
          spellcheck="false"
          autocomplete="off"
          placeholder={$t.newChatFolder.pathPlaceholder}
          data-testid="new-chat-folder-path"
        />
      </label>

      <p class="ncf-status" data-testid="new-chat-folder-status" aria-live="polite">
        {#if check.state === 'checking'}
          <span class="ncf-note">{$t.newChatFolder.checking}</span>
        {:else if check.state === 'error'}
          <span class="ncf-error">{check.message}</span>
        {:else if folder?.repo_root}
          <span class="ncf-tag">git</span>
          <span class="mono ncf-path" title={folder.repo_root}>{$t.newChatFolder.inRepo} · {shortCwdLabel(folder.repo_root)}</span>
        {:else if folder}
          <span class="ncf-note">{$t.newChatFolder.notRepo}</span>
        {/if}
      </p>

      {#if canIsolate}
        <label class="ncf-isolate" title={$t.newChatFolder.isolateHint}>
          <input type="checkbox" bind:checked={isolate} data-testid="new-chat-folder-isolate" />
          <span>⑂ {$t.newChatFolder.isolate}</span>
        </label>
        {#if isolate}<p class="ncf-note">{$t.newChatFolder.isolateHint}</p>{/if}
      {/if}

      {#if error}<p class="ncf-error" role="alert">{error}</p>{/if}

      <div class="ncf-actions">
        <button type="button" class="btn btn-ghost btn-sm" onclick={close}>{$t.newChatFolder.cancel}</button>
        <button
          type="button"
          class="btn btn-primary btn-sm"
          disabled={busy || !path.trim() || check.state === 'error'}
          onclick={() => void start()}
          data-testid="new-chat-folder-start"
        >{busy ? $t.newChatFolder.starting : $t.newChatFolder.start}</button>
      </div>
    </div>
  {/if}
</div>

<style>
  .ncf {
    display: inline-flex;
  }

  .ncf-toggle {
    padding-left: var(--space-2);
    padding-right: var(--space-2);
  }

  .ncf-panel {
    position: absolute;
    top: calc(100% + var(--space-1));
    right: 0;
    z-index: 40;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    width: min(380px, calc(100vw - 2 * var(--space-4)));
    padding: var(--space-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
    box-shadow: var(--shadow-md, 0 12px 28px rgba(0, 0, 0, 0.28));
    text-align: left;
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .stretch .ncf-panel {
    left: var(--space-2);
    right: var(--space-2);
    width: auto;
  }

  .ncf-title {
    font-size: var(--text-sm);
  }

  .ncf-section {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .ncf-label {
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .ncf-recent {
    list-style: none;
    margin: 0;
    padding: var(--space-1);
    max-height: 200px;
    overflow-y: auto;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
  }

  .ncf-folder {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
    padding: var(--space-1) var(--space-2);
    border: 0;
    border-left: 2px solid transparent;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-primary);
    text-align: left;
    cursor: pointer;
  }

  .ncf-folder:hover {
    background: var(--surface-hover);
  }

  .ncf-folder.selected {
    background: var(--surface-active);
    border-left-color: var(--primary);
  }

  .ncf-path {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-xs);
  }

  .ncf-tag {
    flex: none;
    padding: 0 var(--space-1);
    border: 1px solid var(--info);
    border-radius: var(--radius-sm);
    background: var(--info-muted);
    color: var(--info);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .ncf-input {
    width: 100%;
    padding: var(--space-1) var(--space-2);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    color: var(--text-primary);
    font-size: var(--text-xs);
  }

  .ncf-input:focus {
    outline: none;
    border-color: var(--primary);
  }

  .ncf-status {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 1.25em;
    margin: 0;
    font-size: var(--text-xs);
  }

  .ncf-note {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .ncf-error {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--error);
  }

  .ncf-isolate {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    cursor: pointer;
  }

  .ncf-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }

  .mono {
    font-family: var(--font-mono);
  }
</style>
