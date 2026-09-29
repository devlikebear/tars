<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { t } from '../i18n'
  import { getSessionBoard } from '../lib/api'
  import {
    boardCounts,
    boardStatusOrder,
    formatAge,
    formatCost,
    groupBoard,
    type BoardCard,
    type BoardFilter,
    type BoardSession,
    type BoardSort,
  } from '../lib/sessionBoard'
  import { sessionActivity } from '../lib/stores/sessionActivity'

  let { onNavigate, onNewChat }: { onNavigate: (path: string) => void; onNewChat: () => void } = $props()

  const filterKey = 'tars.sessionBoard.filter'
  const sortKey = 'tars.sessionBoard.sort'

  function stored<T extends string>(key: string, allowed: readonly T[], fallback: T): T {
    try {
      const value = localStorage.getItem(key) as T | null
      return value && allowed.includes(value) ? value : fallback
    } catch {
      return fallback
    }
  }

  function remember(key: string, value: string) {
    try {
      localStorage.setItem(key, value)
    } catch {
      // Blocked storage: the choice lasts until the page reloads.
    }
  }

  const filters: BoardFilter[] = ['all', ...boardStatusOrder]
  const sorts: BoardSort[] = ['recent', 'status', 'title', 'cost']

  let sessions = $state<BoardSession[]>([])
  let loading = $state(true)
  let error = $state('')
  let filter = $state<BoardFilter>(stored(filterKey, filters, 'all'))
  let sort = $state<BoardSort>(stored(sortKey, sorts, 'recent'))
  let query = $state('')
  let now = $state(Date.now())
  let timer: ReturnType<typeof setInterval> | null = null
  let clock: ReturnType<typeof setInterval> | null = null
  let inflight = false

  let groups = $derived(groupBoard(sessions, sessionActivity.seen, { filter, sort, query }))
  let counts = $derived(boardCounts(sessions, sessionActivity.seen))

  async function load(showLoading = false) {
    if (inflight) return
    inflight = true
    if (showLoading) loading = true
    try {
      const resp = await getSessionBoard()
      sessions = resp.sessions ?? []
      error = ''
    } catch (err) {
      if (!sessions.length) error = err instanceof Error ? err.message : $t.sessionBoard.loadFailed
    } finally {
      loading = false
      inflight = false
    }
  }

  // The activity store polls cheaply; when it sees a change, refresh the
  // board so status and last turn follow within a poll.
  let lastVersion = -1
  $effect(() => {
    const version = sessionActivity.version
    if (lastVersion >= 0 && version !== lastVersion) void load()
    lastVersion = version
  })

  onMount(() => {
    void load(true)
    timer = setInterval(() => void load(), 15_000)
    clock = setInterval(() => (now = Date.now()), 15_000)
  })

  onDestroy(() => {
    if (timer) clearInterval(timer)
    if (clock) clearInterval(clock)
  })

  function setFilter(next: BoardFilter) {
    filter = next
    remember(filterKey, next)
  }

  function setSort(next: BoardSort) {
    sort = next
    remember(sortKey, next)
  }

  function open(card: BoardCard) {
    onNavigate(`/console/chat/${encodeURIComponent(card.id)}`)
  }

  function age(iso: string | undefined): string {
    const at = Date.parse(iso ?? '')
    return formatAge(now - at, $t.sessionBoard.age)
  }

  // "now" stands alone: not "now ago" or "running now".
  function timeFact(card: BoardCard): string {
    const since = card.running_since ?? card.last_turn_at ?? card.updated_at
    const text = age(since)
    if (text === $t.sessionBoard.age.now) return text
    return card.running_since ? $t.sessionBoard.runningFor(text) : $t.sessionBoard.updated(text)
  }

  function statusBadge(card: BoardCard): string {
    switch (card.boardStatus) {
      case 'needs_input':
        return 'badge-warning'
      case 'running':
        return 'badge-accent'
      case 'done_unread':
        return 'badge-info'
    }
    return 'badge-default'
  }

  async function toggleNotifications() {
    if (sessionActivity.notifyEnabled) sessionActivity.disableNotifications()
    else await sessionActivity.enableNotifications()
  }
</script>

<div class="board">
  <div class="board-header">
    <div>
      <h2>{$t.sessionBoard.title}</h2>
      <p class="board-subtitle">{$t.sessionBoard.subtitle}</p>
    </div>
    <div class="board-actions">
      {#if sessionActivity.permission !== 'unsupported'}
        {#if sessionActivity.permission === 'denied'}
          <span class="notify-note" title={$t.sessionBoard.notifications.hint}>{$t.sessionBoard.notifications.blocked}</span>
        {:else}
          <button
            class="btn btn-ghost btn-sm"
            class:notify-on={sessionActivity.notifyEnabled}
            title={sessionActivity.notifyEnabled ? $t.sessionBoard.notifications.disable : $t.sessionBoard.notifications.hint}
            onclick={toggleNotifications}
          >
            {sessionActivity.notifyEnabled ? $t.sessionBoard.notifications.enabled : $t.sessionBoard.notifications.enable}
          </button>
        {/if}
      {/if}
      <button class="btn btn-primary btn-sm" onclick={onNewChat}>{$t.sessionBoard.newChat}</button>
    </div>
  </div>

  <div class="board-controls">
    <div class="filter-chips" role="group" aria-label={$t.sessionBoard.filter.label}>
      {#each filters as option (option)}
        <button
          class="chip"
          class:active={filter === option}
          aria-pressed={filter === option}
          onclick={() => setFilter(option)}
        >
          {$t.sessionBoard.filter[option]}
          {#if option !== 'all'}<span class="chip-count">{counts[option]}</span>{/if}
        </button>
      {/each}
    </div>
    <div class="board-tools">
      <input class="search" type="search" placeholder={$t.sessionBoard.search} aria-label={$t.sessionBoard.search} bind:value={query} />
      <label class="sort">
        <span>{$t.sessionBoard.sort.label}</span>
        <select value={sort} onchange={(event) => setSort((event.currentTarget as HTMLSelectElement).value as BoardSort)}>
          {#each sorts as option (option)}
            <option value={option}>{$t.sessionBoard.sort[option]}</option>
          {/each}
        </select>
      </label>
    </div>
  </div>

  {#if loading}
    <div class="board-empty">{$t.sessionBoard.loading}</div>
  {:else if error}
    <div class="board-empty">
      <p>{$t.sessionBoard.loadFailed}</p>
      <p class="error-detail">{error}</p>
      <button class="btn btn-secondary btn-sm" onclick={() => load(true)}>{$t.sessionBoard.retry}</button>
    </div>
  {:else if sessions.length === 0}
    <div class="board-empty">{$t.sessionBoard.empty}</div>
  {:else if groups.length === 0}
    <div class="board-empty">{$t.sessionBoard.emptyFilter}</div>
  {:else}
    {#each groups as group (group.key)}
      <section class="repo-group" aria-label={group.name || $t.sessionBoard.noFolder}>
        <header class="repo-header">
          <h3 title={group.key}>{group.name || $t.sessionBoard.noFolder}</h3>
          <span class="repo-count">{$t.sessionBoard.groupCount(group.cards.length)}</span>
        </header>
        <div class="cards">
          {#each group.cards as card (card.id)}
            <button class="session-card" class:needs-input={card.boardStatus === 'needs_input'} data-session-id={card.id} onclick={() => open(card)}>
              <div class="card-top">
                <span class="card-title-text">{card.title || card.id}</span>
                <span class="badge {statusBadge(card)}">{$t.sessionBoard.status[card.boardStatus]}</span>
              </div>
              <div class="card-meta">
                {#if card.branch}<span class="branch" title={card.cwd}>⎇ {card.branch}</span>{/if}
                {#if card.pinned_at}<span class="pinned">{$t.sessionBoard.pinned}</span>{/if}
              </div>
              <div class="card-facts">
                {#if card.pending_approvals > 0}
                  <span class="fact attention">{$t.sessionBoard.pending(card.pending_approvals)}</span>
                {/if}
                {#if card.last_change}
                  <span class="fact change" title={$t.sessionBoard.changeTitle}>
                    {$t.sessionBoard.change(card.last_change.files)}
                    <span class="plus">+{card.last_change.additions}</span>
                    <span class="minus">−{card.last_change.deletions}</span>
                  </span>
                {/if}
                <span class="fact">{timeFact(card)}</span>
                {#if card.cost_usd > 0}<span class="fact">{$t.sessionBoard.cost(formatCost(card.cost_usd))}</span>{/if}
              </div>
            </button>
          {/each}
        </div>
      </section>
    {/each}
  {/if}
</div>

<style>
  .board {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .board-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .board-header h2 {
    margin: 0;
    font-size: var(--text-2xl);
    color: var(--text-primary);
  }

  .board-subtitle {
    margin: var(--space-1) 0 0;
    color: var(--text-tertiary);
  }

  .board-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
  }

  .notify-on {
    color: var(--primary-text);
  }

  .notify-note {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .board-controls {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .filter-chips {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: 4px 10px;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--text-secondary);
    font-size: var(--text-xs);
    cursor: pointer;
  }

  .chip:hover {
    border-color: var(--border-strong);
    color: var(--text-primary);
  }

  .chip.active {
    border-color: var(--primary);
    background: var(--primary-muted);
    color: var(--primary-text);
  }

  .chip-count {
    font-family: var(--font-mono);
    color: var(--text-tertiary);
  }

  .chip.active .chip-count {
    color: var(--primary-text);
  }

  .board-tools {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .search {
    width: 260px;
    max-width: 100%;
    padding: 5px 10px;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    color: var(--text-primary);
    font-size: var(--text-sm);
  }

  .search:focus,
  .sort select:focus {
    outline: none;
    border-color: var(--primary);
  }

  .sort {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .sort select {
    padding: 4px 6px;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    color: var(--text-primary);
    font-size: var(--text-xs);
  }

  .board-empty {
    padding: var(--space-10);
    text-align: center;
    color: var(--text-tertiary);
  }

  .error-detail {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .repo-group {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .repo-header {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
  }

  .repo-header h3 {
    margin: 0;
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-primary);
  }

  .repo-count {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: var(--space-3);
  }

  .session-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
    padding: var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--surface);
    color: inherit;
    text-align: left;
    cursor: pointer;
    transition:
      border-color var(--duration-fast) var(--ease-out),
      background var(--duration-fast) var(--ease-out);
  }

  .session-card:hover {
    border-color: var(--border-strong);
    background: var(--surface-elevated);
  }

  .session-card:focus-visible {
    outline: none;
    border-color: var(--primary);
  }

  .session-card.needs-input {
    border-color: var(--warning);
  }

  .card-top {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .card-title-text {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 500;
    color: var(--text-primary);
  }

  .card-meta,
  .card-facts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .card-meta:empty {
    display: none;
  }

  .branch {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 100%;
    color: var(--text-secondary);
  }

  .attention {
    color: var(--warning);
  }

  .plus {
    color: var(--success);
  }

  .minus {
    color: var(--error);
  }

  @media (max-width: 768px) {
    .board-header,
    .board-controls {
      flex-direction: column;
      align-items: stretch;
    }

    .search {
      width: 100%;
    }
  }
</style>
