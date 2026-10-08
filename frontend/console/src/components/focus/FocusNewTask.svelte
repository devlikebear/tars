<script lang="ts">
  // "New task" on the focus home: a folder (recent ones, or a path the
  // server checks as it will on create — the same picker as New chat in a
  // folder), a one-line goal, and, in a git repository, isolation in a
  // worktree. A template picks the pipeline's stages (development unless
  // another is chosen), and goal mode lets the server decide every gate.
  // Creating it starts a session with a pipeline; the pipeline screen then
  // sends the goal as the first turn.
  import { onDestroy, onMount, tick } from 'svelte'
  import { t } from '../../i18n'
  import { APIRequestError, checkSessionFolder, createFocusPipeline, listFocusTemplates, listRecentSessionFolders, type SessionFolder } from '../../lib/api'
  import { stageLabel, templateText } from '../../lib/focus'
  import { shortCwdLabel } from '../../lib/sessionLabels'
  import { clipboardImageFiles, filesToAttachments } from '../../lib/chatAttachments'
  import { stashKickoffAttachments } from '../../lib/focusKickoff'
  import type { FocusTemplate } from '../../lib/types'
  import FolderPickerDialog from '../FolderPickerDialog.svelte'

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
  let templates = $state<FocusTemplate[]>([])
  let templatesSkipped = $state(0)
  let templateId = $state('dev')
  let goalMode = $state(false)
  let check = $state<Check>({ state: 'idle' })
  let busy = $state(false)
  let error = $state('')
  let timer: ReturnType<typeof setTimeout> | null = null
  let checkSeq = 0
  // The folder picker dialog (#picker), opened by "Browse…" next to the
  // folder field — the same server-backed browser as the Files panel's "+",
  // shared via FolderPickerDialog.
  let showBrowser = $state(false)

  // Images pasted into the goal field (#1097): held here until start()
  // converts them and hands them to the pipeline's first turn — the
  // session the server will attach them to doesn't exist yet.
  let images = $state<File[]>([])
  let imagePreviews = $state(new Map<string, string>())

  // A per-File identity, not a name/size/lastModified string: two paste
  // events in the same millisecond can otherwise produce two
  // clipboard-<ts>.<ext> Files with identical name, size and lastModified,
  // colliding on one key (one blob URL overwriting the other, and a keyed
  // {#each} seeing a duplicate key). A WeakMap keyed on the File object
  // itself can't collide this way, however fast two pastes land.
  let imageIds = new WeakMap<File, string>()
  let nextImageId = 0

  function imageKey(file: File): string {
    let id = imageIds.get(file)
    if (id === undefined) {
      id = `img-${nextImageId++}`
      imageIds.set(file, id)
    }
    return id
  }

  function addImages(files: File[]) {
    for (const file of files) {
      images = [...images, file]
      imagePreviews = new Map([...imagePreviews, [imageKey(file), URL.createObjectURL(file)]])
    }
  }

  function removeImage(index: number) {
    const file = images[index]
    const key = imageKey(file)
    const preview = imagePreviews.get(key)
    if (preview) URL.revokeObjectURL(preview)
    imagePreviews.delete(key)
    imagePreviews = new Map(imagePreviews)
    images = images.filter((_, i) => i !== index)
  }

  function onGoalPaste(e: ClipboardEvent) {
    if (!e.clipboardData) return
    const files = clipboardImageFiles(e.clipboardData.items, images.length)
    if (files.length === 0) return
    e.preventDefault()
    addImages(files)
  }

  let folder = $derived(check.state === 'ok' ? check.folder : null)
  let canStart = $derived(!!folder && !!goal.trim() && !busy)
  let template = $derived(templates.find((item) => item.id === templateId) ?? null)

  // A built-in template is shown in the console's language; a workspace
  // one as written.
  function templateName(item: FocusTemplate): string {
    return (item.builtin && templateText(item.id, $t.focus.templates)?.name) || item.name
  }

  function templateDescription(item: FocusTemplate): string {
    return (item.builtin && templateText(item.id, $t.focus.templates)?.description) || item.description || ''
  }

  function templateStages(item: FocusTemplate): string {
    return item.stages.map((s) => stageLabel({ template: item.builtin ? item.id : undefined, stages: item.stages }, s.id, $t.focus)).join(' → ')
  }

  onMount(async () => {
    void listRecentSessionFolders().then((list) => { recent = list }).catch(() => { recent = [] })
    try {
      const listed = await listFocusTemplates()
      templates = listed.templates
      templatesSkipped = listed.diagnostics.length
    } catch {
      // An older server: only the development pipeline.
      templates = []
    }
  })

  onDestroy(() => {
    if (timer) clearTimeout(timer)
    for (const url of imagePreviews.values()) URL.revokeObjectURL(url)
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

  function openBrowser() {
    showBrowser = true
  }

  function closeBrowser() {
    showBrowser = false
  }

  // The dialog hands back an absolute, server-verified folder path — run the
  // same check a typed path gets, right away rather than debounced.
  function onBrowsePicked(picked: string) {
    showBrowser = false
    if (timer) clearTimeout(timer)
    error = ''
    path = picked
    void runCheck(picked)
  }

  function onBrowserKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      closeBrowser()
    }
  }

  let browserModalEl: HTMLDivElement | undefined = $state()

  $effect(() => {
    if (showBrowser) void tick().then(() => browserModalEl?.focus())
  })

  async function start() {
    if (!canStart || !folder) return
    busy = true
    error = ''
    try {
      // Converted before the session exists: a conversion failure (a Blob
      // read error) must not leave an orphaned pipeline on the server with
      // no way back to it.
      const attachments = images.length > 0 ? await filesToAttachments(images) : null
      const created = await createFocusPipeline({
        goal: goal.trim(),
        cwd: folder.path,
        isolate: isolate && !!folder.repo_root,
        ...(template && template.id !== 'dev' ? { template: template.id } : {}),
        ...(goalMode ? { goal_mode: true } : {}),
      })
      if (attachments) stashKickoffAttachments(created.session_id, attachments)
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
    <div class="folder-row">
      <input id="focus-new-folder" class="mono" type="text" bind:value={path} oninput={onPathInput} placeholder={$t.focus.newTask.folderPlaceholder} data-testid="focus-new-folder" autocomplete="off" />
      <button type="button" class="btn btn-ghost btn-sm" title={$t.focus.newTask.browseTitle} onclick={openBrowser} data-testid="focus-new-folder-browse">{$t.focus.newTask.browse}</button>
    </div>
    <p class="status" class:error={check.state === 'error'} data-testid="focus-new-folder-status">
      {#if check.state === 'checking'}{$t.focus.newTask.checking}
      {:else if check.state === 'error'}{check.message}
      {:else if folder}{folder.repo_root ? $t.focus.newTask.inRepo : $t.focus.newTask.notRepo}
      {/if}
    </p>
  </div>

  <div class="field">
    <label class="label" for="focus-new-goal">{$t.focus.newTask.goal}</label>
    <input id="focus-new-goal" type="text" bind:value={goal} onpaste={onGoalPaste} placeholder={$t.focus.newTask.goalPlaceholder} data-testid="focus-new-goal" />
    {#if images.length > 0}
      <div class="images" data-testid="focus-new-images">
        {#each images as file, i (imageKey(file))}
          <div class="image-card" data-testid="focus-new-image">
            <img class="image-thumb" src={imagePreviews.get(imageKey(file))} alt={file.name} />
            <button type="button" class="image-remove" aria-label={$t.focus.newTask.removeImage} title={$t.focus.newTask.removeImage} data-testid="focus-new-image-remove" onclick={() => removeImage(i)}>&times;</button>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  {#if templates.length > 1}
    <div class="field">
      <label class="label" for="focus-new-template">{$t.focus.newTask.template}</label>
      <select id="focus-new-template" bind:value={templateId} data-testid="focus-new-template">
        {#each templates as item (item.id)}
          <option value={item.id}>{templateName(item)}{item.builtin ? '' : ` · ${$t.focus.newTask.templateCustom}`}</option>
        {/each}
      </select>
      {#if template}
        <p class="status" data-testid="focus-new-template-stages">
          {#if templateDescription(template)}<span data-content>{templateDescription(template)}</span> · {/if}{$t.focus.newTask.templateStages(templateStages(template))}
        </p>
      {/if}
      {#if templatesSkipped > 0}<p class="status error">{$t.focus.newTask.templateSkipped(templatesSkipped)}</p>{/if}
    </div>
  {/if}

  {#if folder?.repo_root}
    <label class="check">
      <input type="checkbox" bind:checked={isolate} data-testid="focus-new-isolate" />
      {$t.focus.newTask.isolate}
    </label>
  {/if}

  <div class="field">
    <label class="check">
      <input type="checkbox" bind:checked={goalMode} data-testid="focus-new-goal-mode" />
      {$t.focus.newTask.goalMode}
    </label>
    {#if goalMode}<p class="status" data-testid="focus-new-goal-hint">{$t.focus.newTask.goalModeHint}</p>{/if}
  </div>

  {#if error}<p class="status error">{error}</p>{/if}

  <div class="actions">
    <button type="submit" class="btn btn-primary" disabled={!canStart} data-testid="focus-new-start">{busy ? $t.focus.newTask.starting : $t.focus.newTask.start}</button>
    <button type="button" class="btn btn-ghost" onclick={onCancel}>{$t.focus.newTask.cancel}</button>
  </div>
</form>

{#if showBrowser}
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="browse-backdrop" onclick={closeBrowser}>
    <div
      class="browse-modal"
      role="dialog"
      aria-modal="true"
      aria-label={$t.focus.newTask.browseTitle}
      tabindex="-1"
      data-testid="focus-new-folder-dialog"
      bind:this={browserModalEl}
      onclick={(e) => e.stopPropagation()}
      onkeydown={onBrowserKeydown}
    >
      <FolderPickerDialog initialPath={folder?.path} onSelect={onBrowsePicked} onCancel={closeBrowser} />
    </div>
  </div>
{/if}

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

  .folder-row {
    display: flex;
    gap: var(--space-2);
    align-items: stretch;
  }

  .folder-row input[type='text'] {
    flex: 1;
    min-width: 0;
  }

  input[type='text'] {
    padding: var(--space-2) var(--space-3);
    background: var(--surface-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-base);
  }

  .browse-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    padding: var(--space-4);
  }

  .browse-modal {
    background: var(--surface-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    width: 100%;
    max-width: 560px;
    height: 70vh;
    max-height: 640px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  input[type='text']:focus,
  select:focus {
    outline: none;
    border-color: var(--primary);
  }

  select {
    padding: var(--space-2) var(--space-3);
    background: var(--surface-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-base);
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

  .images {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-top: var(--space-1);
  }

  .image-card {
    position: relative;
    width: 48px;
    height: 48px;
    flex-shrink: 0;
  }

  .image-thumb {
    width: 48px;
    height: 48px;
    object-fit: cover;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border-subtle);
  }

  .image-remove {
    position: absolute;
    top: -6px;
    right: -6px;
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: 50%;
    color: var(--text-ghost);
    cursor: pointer;
    font-size: 12px;
    width: 18px;
    height: 18px;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    line-height: 1;
  }

  .image-remove:hover {
    color: var(--error);
    border-color: var(--error);
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
