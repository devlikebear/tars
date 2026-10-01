<script lang="ts">
  // "New task" on the focus home: a folder (recent ones, or a path the
  // server checks as it will on create — the same picker as New chat in a
  // folder), a one-line goal, and, in a git repository, isolation in a
  // worktree. Creating it starts a session with a pipeline; the pipeline
  // screen then sends the goal as the first turn.
  import { onDestroy, onMount } from 'svelte'
  import { t } from '../../i18n'
  import { APIRequestError, checkSessionFolder, createFocusPipeline, listRecentSessionFolders, type SessionFolder } from '../../lib/api'
  import { shortCwdLabel } from '../../lib/sessionLabels'

  interface Props {
    onCreated: (sessionId: string) => void
    onCancel: () => void
  }

  let { onCreated, onCancel }: Props = $props()

  type Check =
    | { state: 'idle' }
    | { state: 'checking' }
    | { state: 'ok'; folder: SessionFolder }
    | { state: 'error'; message: string }

  let recent = $state<SessionFolder[]>([])
  let path = $state('')
  let goal = $state('')
  let isolate = $state(false)
  let check = $state<Check>({ state: 'idle' })
  let busy = $state(false)
  let error = $state('')
  let timer: ReturnType<typeof setTimeout> | null = null
  let checkSeq = 0

  let folder = $derived(check.state === 'ok' ? check.folder : null)
  let canStart = $derived(!!folder && !!goal.trim() && !busy)

  onMount(async () => {
    try {
      recent = await listRecentSessionFolders()
    } catch {
      recent = []
    }
  })

  onDestroy(() => {
    if (timer) clearTimeout(timer)
  })

  function checkError(err: unknown): string {
    if (err instanceof APIRequestError && err.status === 404) return $t.focus.newTask.notFound
    if (err instanceof APIRequestError && err.status === 400) return $t.focus.newTask.notAFolder
    return err instanceof Error ? err.message : String(err)
  }

  async function runCheck(value: string) {
    const seq = ++checkSeq
    check = { state: 'checking' }
    try {
      const found = await checkSessionFolder(value)
      if (seq !== checkSeq) return
      check = { state: 'ok', folder: found }
      if (!found.repo_root) isolate = false
    } catch (err) {
      if (seq === checkSeq) check = { state: 'error', message: checkError(err) }
    }
  }

  function onPathInput() {
    if (timer) clearTimeout(timer)
    checkSeq++
    error = ''
    const value = path.trim()
    if (!value) {
      check = { state: 'idle' }
      return
    }
    check = { state: 'checking' }
    timer = setTimeout(() => void runCheck(value), 250)
  }

  function pick(item: SessionFolder) {
    if (timer) clearTimeout(timer)
    checkSeq++
    path = item.path
    check = { state: 'ok', folder: item }
    if (!item.repo_root) isolate = false
  }

  async function start() {
    if (!canStart || !folder) return
    busy = true
    error = ''
    try {
      const created = await createFocusPipeline({ goal: goal.trim(), cwd: folder.path, isolate: isolate && !!folder.repo_root })
      onCreated(created.session_id)
    } catch (err) {
      error = $t.focus.newTask.failed(err instanceof Error ? err.message : String(err))
    } finally {
      busy = false
    }
  }
</script>

<form class="new-task" aria-label={$t.focus.newTask.title} data-testid="focus-new-task" onsubmit={(e) => { e.preventDefault(); void start() }}>
  <h3>{$t.focus.newTask.title}</h3>

  {#if recent.length > 0}
    <div class="field">
      <span class="label">{$t.focus.newTask.recent}</span>
      <div class="recent">
        {#each recent as item (item.path)}
          <button type="button" class="folder" class:selected={folder?.path === item.path} title={item.path} onclick={() => pick(item)}>
            <span class="mono" data-content>{shortCwdLabel(item.path)}</span>
            {#if item.repo_root}<span class="tag">git</span>{/if}
          </button>
        {/each}
      </div>
    </div>
  {/if}

  <div class="field">
    <label class="label" for="focus-new-folder">{$t.focus.newTask.folder}</label>
    <input id="focus-new-folder" class="mono" type="text" bind:value={path} oninput={onPathInput} placeholder={$t.focus.newTask.folderPlaceholder} data-testid="focus-new-folder" autocomplete="off" />
    <p class="status" class:error={check.state === 'error'} data-testid="focus-new-folder-status">
      {#if check.state === 'checking'}{$t.focus.newTask.checking}
      {:else if check.state === 'error'}{check.message}
      {:else if folder}{folder.repo_root ? $t.focus.newTask.inRepo : $t.focus.newTask.notRepo}
      {/if}
    </p>
  </div>

  <div class="field">
    <label class="label" for="focus-new-goal">{$t.focus.newTask.goal}</label>
    <input id="focus-new-goal" type="text" bind:value={goal} placeholder={$t.focus.newTask.goalPlaceholder} data-testid="focus-new-goal" />
  </div>

  {#if folder?.repo_root}
    <label class="check">
      <input type="checkbox" bind:checked={isolate} data-testid="focus-new-isolate" />
      {$t.focus.newTask.isolate}
    </label>
  {/if}

  {#if error}<p class="status error">{error}</p>{/if}

  <div class="actions">
    <button type="submit" class="btn btn-primary" disabled={!canStart} data-testid="focus-new-start">{busy ? $t.focus.newTask.starting : $t.focus.newTask.start}</button>
    <button type="button" class="btn btn-ghost" onclick={onCancel}>{$t.focus.newTask.cancel}</button>
  </div>
</form>

<style>
  .new-task {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-5);
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
  }

  h3 {
    margin: 0;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  input[type='text'] {
    padding: var(--space-2) var(--space-3);
    background: var(--surface-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-base);
  }

  input[type='text']:focus {
    outline: none;
    border-color: var(--primary);
  }

  .recent {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .folder {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-2);
    background: transparent;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    cursor: pointer;
  }

  .folder.selected {
    border-color: var(--primary);
    background: var(--primary-muted);
  }

  .tag {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .status {
    min-height: 1em;
    margin: 0;
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .status.error {
    color: var(--error);
  }

  .check {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
  }

  .actions {
    display: flex;
    gap: var(--space-2);
  }
</style>
