<script lang="ts">
  // My projects (docs/plans/vibe-mode-redesign unit 2, screens/D-Projects):
  // five columns by the server's board state (GET /v1/easy/projects, one
  // state per request, five cards at a time), search by name, the card
  // menu, and the archive (?archived=1). A project changes column only when
  // the server says its state changed: the board reads again every few
  // seconds and after every rename, archive or delete.
  import { onMount } from 'svelte'
  import { t } from '../../i18n'
  import { deleteEasyProject, easyErrorCode, listEasyBoard, updateEasyProject } from '../../lib/api/easy'
  import {
    BOARD_COLUMNS,
    COLUMN_PAGE,
    boardTotal,
    columnCount,
    columnNeeds,
    columnView,
    easyArchivePath,
    easyProjectsPath,
    emptyColumn,
    groupByColumn,
    manageErrorKey,
    matchesName,
    renamedIn,
    shellPollMs,
    showMore,
    withPage,
    type ColumnFeed,
    type EasyColumn,
    type EasyListPage,
    type EasyListed,
  } from '../../lib/easyBoard'
  import EasyProjectCard from './EasyProjectCard.svelte'

  let {
    archive = false,
    onNavigate,
    onChanged,
  }: { archive?: boolean; onNavigate: (path: string) => void; onChanged?: () => void } = $props()

  let columns = $state<Record<EasyColumn, ColumnFeed>>(freshColumns())
  let counts = $state<EasyListPage['counts']>({})
  let archivedCount = $state(0)
  let loaded = $state(false)
  let loadFailed = $state(false)
  let errorText = $state('')

  // Search reads every project of the side shown once and filters by name.
  let query = $state('')
  let searchAll = $state<EasyListed[] | null>(null)

  // The archive is one list, newest first, read a page at a time.
  let archived = $state<EasyListed[]>([])
  let archivedNext = $state<string | undefined>(undefined)

  let pendingDelete = $state<EasyListed | null>(null)
  let deleting = $state(false)
  let deleteButton = $state<HTMLButtonElement | null>(null)

  let generation = 0
  let pollTimer: ReturnType<typeof setTimeout> | undefined
  let searchTimer: ReturnType<typeof setTimeout> | undefined

  const ARCHIVE_PAGE = 20

  function freshColumns(shown?: Record<EasyColumn, ColumnFeed>): Record<EasyColumn, ColumnFeed> {
    const out = {} as Record<EasyColumn, ColumnFeed>
    for (const c of BOARD_COLUMNS) out[c] = { ...emptyColumn(c), shown: shown?.[c].shown ?? COLUMN_PAGE }
    return out
  }

  function takeCounts(page: EasyListPage) {
    counts = page.counts
    archivedCount = page.archived
  }

  // Reads what a column still needs to show its cards in order.
  async function filled(start: ColumnFeed, gen: number): Promise<ColumnFeed | null> {
    let feed = start
    for (let guard = 0; guard < 20; guard++) {
      const needs = columnNeeds(feed)
      if (!needs.length) break
      const pages = await Promise.all(needs.map((s) => listEasyBoard({ state: s, limit: COLUMN_PAGE, cursor: feed.feeds[s]?.next })))
      if (gen !== generation) return null
      needs.forEach((s, i) => (feed = withPage(feed, s, pages[i])))
      takeCounts(pages[0])
    }
    return feed
  }

  // Reads the whole board again, keeping how far each column was opened,
  // and shows it in one step.
  async function loadBoard() {
    const gen = ++generation
    const start = freshColumns(columns)
    const feeds = await Promise.all(BOARD_COLUMNS.map((c) => filled(start[c], gen)))
    if (gen !== generation || feeds.some((f) => !f)) return
    const next = {} as Record<EasyColumn, ColumnFeed>
    BOARD_COLUMNS.forEach((c, i) => (next[c] = feeds[i]!))
    columns = next
  }

  async function loadArchive() {
    const gen = ++generation
    const page = await listEasyBoard({ archived: true, limit: Math.max(ARCHIVE_PAGE, archived.length) })
    if (gen !== generation) return
    archived = page.projects
    archivedNext = page.next_cursor
    takeCounts(page)
  }

  async function loadSearch() {
    const gen = ++generation
    const page = await listEasyBoard({ archived: archive })
    if (gen !== generation) return
    searchAll = page.projects.filter((p) => matchesName(p, query))
    takeCounts(page)
  }

  async function load() {
    clearTimeout(pollTimer)
    try {
      if (query.trim()) await loadSearch()
      else if (archive) await loadArchive()
      else await loadBoard()
      loadFailed = false
    } catch {
      loadFailed = true
    } finally {
      loaded = true
      pollTimer = setTimeout(load, shellPollMs)
    }
  }

  async function more(column: EasyColumn) {
    // A read already on its way would put the column back as it was.
    const gen = ++generation
    columns[column] = showMore(columns[column])
    try {
      const feed = await filled(columns[column], gen)
      if (feed && gen === generation) columns[column] = feed
    } catch {
      errorText = $t.easy.board.loadFailed
    }
  }

  async function moreArchived() {
    if (!archivedNext) return
    try {
      const page = await listEasyBoard({ archived: true, limit: ARCHIVE_PAGE, cursor: archivedNext })
      const seen = new Set(archived.map((p) => p.id))
      archived = [...archived, ...page.projects.filter((p) => !seen.has(p.id))]
      archivedNext = page.next_cursor
    } catch {
      errorText = $t.easy.board.loadFailed
    }
  }

  function refusal(err: unknown): string {
    return $t.easy.board.errors[manageErrorKey(easyErrorCode(err))]
  }

  async function rename(project: EasyListed, name: string): Promise<string> {
    errorText = ''
    try {
      const saved = await updateEasyProject(project.id, { name })
      for (const c of BOARD_COLUMNS) columns[c] = renamedIn(columns[c], project.id, saved.name)
      archived = archived.map((p) => (p.id === project.id ? { ...p, name: saved.name } : p))
      if (searchAll) searchAll = searchAll.map((p) => (p.id === project.id ? { ...p, name: saved.name } : p))
      onChanged?.()
      return ''
    } catch (err) {
      return refusal(err)
    }
  }

  async function setArchived(project: EasyListed, value: boolean) {
    errorText = ''
    try {
      await updateEasyProject(project.id, { archived: value })
      onChanged?.()
    } catch (err) {
      errorText = refusal(err)
    }
    await load()
  }

  async function confirmDelete() {
    if (!pendingDelete || deleting) return
    deleting = true
    errorText = ''
    try {
      await deleteEasyProject(pendingDelete.id)
      onChanged?.()
    } catch (err) {
      errorText = refusal(err)
    }
    deleting = false
    pendingDelete = null
    await load()
  }

  function askDelete(project: EasyListed) {
    pendingDelete = project
    queueMicrotask(() => deleteButton?.focus())
  }

  function onSearchInput() {
    clearTimeout(searchTimer)
    searchTimer = setTimeout(() => {
      if (!query.trim()) searchAll = null
      void load()
    }, 250)
  }

  function onVisible() {
    if (document.visibilityState === 'visible') void load()
  }

  function go(event: MouseEvent, path: string) {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0) return
    event.preventDefault()
    onNavigate(path)
  }

  onMount(() => {
    void load()
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      generation++
      clearTimeout(pollTimer)
      clearTimeout(searchTimer)
      document.removeEventListener('visibilitychange', onVisible)
    }
  })

  const searching = $derived(searchAll !== null && query.trim() !== '')
  const grouped = $derived(searchAll ? groupByColumn(searchAll) : null)
  const total = $derived(boardTotal(counts))
</script>

<div class="board-page" data-testid="easy-root">
  <div class="toolbar">
    {#if archive}
      <a class="back" href={easyProjectsPath} onclick={(e) => go(e, easyProjectsPath)} data-testid="easy-board-back">{$t.easy.board.backToBoard}</a>
      <h1>{$t.easy.board.archiveTitle}</h1>
      <span class="count" data-testid="easy-board-total">{archivedCount}</span>
    {:else}
      <h1>{$t.easy.board.title}</h1>
      <span class="count" data-testid="easy-board-total">{total}</span>
    {/if}
    <label class="sr-only" for="easy-board-search">{$t.easy.board.searchLabel}</label>
    <input
      id="easy-board-search"
      class="search"
      type="search"
      placeholder={$t.easy.board.searchPlaceholder}
      data-testid="easy-board-search"
      bind:value={query}
      oninput={onSearchInput}
    />
    {#if !archive}
      <a class="archive-link" href={easyArchivePath} onclick={(e) => go(e, easyArchivePath)} data-testid="easy-board-archive">{$t.easy.board.archive(archivedCount)}</a>
    {/if}
  </div>

  {#if errorText}
    <p class="banner" role="alert" data-testid="easy-board-error">{errorText}</p>
  {/if}

  {#if !loaded}
    <p class="muted">{$t.easy.board.loading}</p>
  {:else if loadFailed}
    <p class="banner" role="alert">
      {$t.easy.board.loadFailed}
      <button type="button" class="btn btn-ghost btn-sm" onclick={() => void load()}>{$t.easy.board.retry}</button>
    </p>
  {:else if archive}
    {@const list = searching ? (searchAll ?? []) : archived}
    {#if list.length === 0}
      <p class="empty wide" data-testid="easy-archive-empty">{searching ? $t.easy.board.noMatch : $t.easy.board.archiveEmpty}</p>
    {:else}
      <div class="archive-grid" data-testid="easy-archive-list">
        {#each list as project (project.id)}
          <EasyProjectCard
            {project}
            menu
            archived
            {onNavigate}
            onRename={(name) => rename(project, name)}
            onArchive={(value) => void setArchived(project, value)}
            onDelete={() => askDelete(project)}
          />
        {/each}
      </div>
      {#if !searching && archivedNext}
        <button type="button" class="more" data-testid="easy-archive-more" onclick={() => void moreArchived()}>{$t.easy.board.more}</button>
      {/if}
    {/if}
  {:else}
    {#if searching && searchAll?.length === 0}
      <p class="empty wide" data-testid="easy-board-nomatch">{$t.easy.board.noMatch}</p>
    {/if}
    <div class="columns">
      {#each BOARD_COLUMNS as column (column)}
        {@const view = grouped ? { cards: grouped[column], more: false } : columnView(columns[column])}
        {@const n = grouped ? grouped[column].length : columnCount(column, counts)}
        <section class="column col-{column}" class:lit={column === 'your_turn' && n > 0} data-testid="easy-column-{column}" aria-labelledby="col-title-{column}">
          <div class="column-head">
            <span class="mark mark-{column}" aria-hidden="true"></span>
            <h2 id="col-title-{column}">{$t.easy.board.columns[column]}</h2>
            <span class="column-count" data-testid="easy-column-count">{n}</span>
          </div>
          {#each view.cards as project (project.id)}
            <EasyProjectCard
              {project}
              menu
              {onNavigate}
              onRename={(name) => rename(project, name)}
              onArchive={(value) => void setArchived(project, value)}
              onDelete={() => askDelete(project)}
            />
          {:else}
            {#if !searching}
              <p class="empty" data-testid="easy-column-empty">{$t.easy.board.empty[column]}</p>
            {/if}
          {/each}
          {#if view.more}
            <button type="button" class="more" data-testid="easy-column-more" onclick={() => void more(column)}>{$t.easy.board.more}</button>
          {/if}
        </section>
      {/each}
    </div>
    <p class="footer">{$t.easy.board.footer}</p>
  {/if}
</div>

{#if pendingDelete}
  <div class="overlay" role="presentation" onclick={() => !deleting && (pendingDelete = null)}></div>
  <div
    class="dialog"
    role="alertdialog"
    aria-modal="true"
    aria-labelledby="easy-delete-title"
    aria-describedby="easy-delete-body"
    data-testid="easy-delete-dialog"
    tabindex="-1"
    onkeydown={(e) => {
      if (e.key === 'Escape' && !deleting) pendingDelete = null
    }}
  >
    <h2 id="easy-delete-title">{$t.easy.board.remove.title(pendingDelete.name)}</h2>
    <p id="easy-delete-body">{$t.easy.board.remove.body}</p>
    <div class="dialog-actions">
      <button type="button" class="btn btn-ghost" onclick={() => (pendingDelete = null)} disabled={deleting} data-testid="easy-delete-cancel">{$t.easy.board.remove.cancel}</button>
      <button type="button" class="btn btn-danger-solid" bind:this={deleteButton} onclick={() => void confirmDelete()} disabled={deleting} data-testid="easy-delete-confirm">{$t.easy.board.remove.confirm}</button>
    </div>
  </div>
{/if}

<style>
  .board-page {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    padding: var(--space-6) var(--space-6) var(--space-4);
    box-sizing: border-box;
  }
  .toolbar {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-3);
  }
  h1 {
    margin: 0;
    font-size: var(--text-xl);
    font-weight: 600;
  }
  .count,
  .column-count {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }
  .back {
    color: var(--text-secondary);
    text-decoration: none;
    font-size: var(--text-sm);
  }
  .back:hover {
    color: var(--text-primary);
  }
  .search {
    margin-left: var(--space-3);
    font: inherit;
    font-size: var(--text-base);
    width: 240px;
    max-width: 100%;
    color: var(--text-primary);
    background: var(--surface-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    padding: var(--space-2) 10px;
  }
  .archive-link {
    margin-left: auto;
    font-size: var(--text-sm);
    color: var(--text-secondary);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    padding: 7px var(--space-3);
    text-decoration: none;
  }
  .archive-link:hover {
    color: var(--text-primary);
    border-color: var(--border-strong);
  }
  .columns {
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    gap: var(--space-3);
    align-items: start;
  }
  .column {
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    padding: var(--space-3);
    display: flex;
    flex-direction: column;
    gap: 10px;
    min-width: 0;
  }
  .column.lit {
    border-color: var(--warning);
  }
  .column-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: 2px 2px 6px;
  }
  .column-head h2 {
    margin: 0;
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .column-count {
    margin-left: auto;
    font-size: var(--text-xs);
  }
  .mark {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
    box-sizing: border-box;
  }
  .mark-your_turn {
    background: var(--warning);
  }
  .mark-working {
    background: var(--primary);
  }
  .mark-stopped {
    border: 2px solid var(--text-secondary);
  }
  .mark-done {
    border-radius: 2px;
    background: var(--text-primary);
  }
  .mark-published {
    border-radius: 2px;
    background: var(--primary);
  }
  .empty {
    margin: 0;
    border: 1px dashed var(--border-default);
    border-radius: var(--radius-md);
    padding: 18px var(--space-3);
    font-size: var(--text-sm);
    color: var(--text-secondary);
    line-height: 1.6;
  }
  .empty.wide {
    max-width: 560px;
  }
  .more {
    font: inherit;
    font-size: var(--text-xs);
    color: var(--text-secondary);
    border: 1px dashed var(--border-strong);
    background: transparent;
    border-radius: var(--radius-md);
    padding: 9px;
    cursor: pointer;
  }
  .more:hover {
    color: var(--text-primary);
  }
  .archive-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
    gap: var(--space-3);
  }
  .footer {
    margin: 0;
    border-top: 1px solid var(--border-subtle);
    padding-top: var(--space-3);
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }
  .muted {
    color: var(--text-secondary);
  }
  .banner {
    margin: 0;
    padding: var(--space-3) var(--space-4);
    background: var(--error-muted);
    border: 1px solid var(--error);
    border-radius: var(--radius-md);
    color: var(--text-primary);
    font-size: var(--text-sm);
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.55);
    z-index: 50;
  }
  .dialog {
    position: fixed;
    left: 50%;
    top: 30%;
    transform: translate(-50%, -30%);
    width: min(440px, calc(100vw - 32px));
    box-sizing: border-box;
    background: var(--surface-elevated);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    padding: var(--space-5);
    z-index: 51;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  .dialog h2 {
    margin: 0;
    font-size: var(--text-lg);
  }
  .dialog p {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
    line-height: 1.6;
  }
  .dialog-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
  .btn-danger-solid {
    background: var(--error);
    color: var(--surface-base);
    font-weight: 600;
  }
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
    white-space: nowrap;
  }
  @media (max-width: 1100px) {
    .columns {
      grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
    }
  }
  @media (max-width: 640px) {
    .board-page {
      padding: var(--space-4);
    }
    .search {
      margin-left: 0;
      width: 100%;
    }
  }
</style>
