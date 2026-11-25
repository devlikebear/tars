<script lang="ts">
  // One project on the home screen or the board: its name, the one line
  // its server state gives (lib/easyBoard.ts cardLine) and, on the board,
  // the menu — open, rename, archive or bring back, delete. The card only
  // opens the project's own route; what the project shows is unit 4's.
  import { t, locale } from '../../i18n'
  import { easyProjectPath } from '../../lib/easy'
  import { cardLine, columnOf, manageable, type CardLine, type EasyListed } from '../../lib/easyBoard'

  let {
    project,
    variant = 'board',
    menu = false,
    archived = false,
    onNavigate,
    onRename,
    onArchive,
    onDelete,
  }: {
    project: EasyListed
    variant?: 'home' | 'board'
    menu?: boolean
    archived?: boolean
    onNavigate: (path: string) => void
    // Resolves to an error sentence, or '' when the name was saved.
    onRename?: (name: string) => Promise<string>
    onArchive?: (archived: boolean) => void
    onDelete?: () => void
  } = $props()

  let menuOpen = $state(false)
  let renaming = $state(false)
  let newName = $state('')
  let renameError = $state('')
  let saving = $state(false)
  let menuButton = $state<HTMLButtonElement | null>(null)
  let nameInput = $state<HTMLInputElement | null>(null)

  const shown = $derived(cardLine(project.board, $locale))
  const path = $derived(easyProjectPath(project.id))
  const canMove = $derived(manageable(project.board.state))

  function lineText(line: CardLine): string {
    const words = $t.easy.board.line
    switch (line.key) {
      case 'paused':
        return words.paused(line.when)
      case 'done':
        return words.done(line.ago)
      default:
        return words[line.key]
    }
  }

  function open(event: MouseEvent) {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0) return
    event.preventDefault()
    onNavigate(path)
  }

  function startRename() {
    menuOpen = false
    newName = project.name
    renameError = ''
    renaming = true
    queueMicrotask(() => nameInput?.select())
  }

  async function saveRename() {
    if (saving) return
    saving = true
    renameError = (await onRename?.(newName)) ?? ''
    saving = false
    if (!renameError) renaming = false
  }

  function cancelRename() {
    renaming = false
    renameError = ''
    menuButton?.focus()
  }

  function onWindowClick(event: MouseEvent) {
    if (menuOpen && !(event.target as HTMLElement).closest(`[data-card="${project.id}"] .menu-area`)) menuOpen = false
  }

  function onMenuKey(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      menuOpen = false
      menuButton?.focus()
    }
  }
</script>

<svelte:window onclick={onWindowClick} />

<div
  class="card {variant} tone-{shown.tone}"
  class:turn={shown.tone === 'turn' && variant === 'home'}
  data-card={project.id}
  data-testid="easy-project-item"
  data-state={project.board.state}
>
  {#if renaming}
    <form
      class="rename"
      onsubmit={(e) => {
        e.preventDefault()
        void saveRename()
      }}
    >
      <label class="sr-only" for="rename-{project.id}">{$t.easy.board.rename.label}</label>
      <input
        id="rename-{project.id}"
        data-testid="easy-rename-input"
        maxlength="60"
        bind:value={newName}
        bind:this={nameInput}
        onkeydown={(e) => {
          if (e.key === 'Escape') cancelRename()
        }}
      />
      {#if renameError}
        <p class="error" role="alert" data-testid="easy-rename-error">{renameError}</p>
      {/if}
      <div class="rename-actions">
        <button type="submit" class="btn btn-primary btn-sm" data-testid="easy-rename-save" disabled={saving}>{$t.easy.board.rename.save}</button>
        <button type="button" class="btn btn-ghost btn-sm" onclick={cancelRename}>{$t.easy.board.rename.cancel}</button>
      </div>
    </form>
  {:else}
    <div class="head">
      <a class="name" href={path} onclick={open} data-testid="easy-project-open">{project.name}</a>
      {#if variant === 'home'}
        <span class="pill tone-{shown.tone}">{$t.easy.board.pill[columnOf(project.board.state)]}</span>
      {/if}
    </div>
    <span class="line" data-testid="easy-project-line">{lineText(shown.line)}</span>
    {#if menu}
      <div class="menu-area" class:open={menuOpen}>
        <button
          type="button"
          class="menu-button"
          aria-label={$t.easy.board.menu.label(project.name)}
          aria-haspopup="menu"
          aria-expanded={menuOpen}
          data-testid="easy-card-menu"
          bind:this={menuButton}
          onclick={() => (menuOpen = !menuOpen)}
        >⋯</button>
        {#if menuOpen}
          <!-- svelte-ignore a11y_interactive_supports_focus -->
          <div class="menu" role="menu" tabindex="-1" onkeydown={onMenuKey}>
            <a role="menuitem" href={path} onclick={open} data-testid="easy-menu-open">{$t.easy.board.menu.open}</a>
            <button type="button" role="menuitem" onclick={startRename} data-testid="easy-menu-rename">{$t.easy.board.menu.rename}</button>
            {#if archived}
              <button type="button" role="menuitem" class="sep" onclick={() => { menuOpen = false; onArchive?.(false) }} data-testid="easy-menu-restore">{$t.easy.board.menu.restore}</button>
            {:else}
              <button
                type="button"
                role="menuitem"
                class="sep"
                disabled={!canMove}
                title={canMove ? undefined : $t.easy.board.menu.busyHint}
                onclick={() => { menuOpen = false; onArchive?.(true) }}
                data-testid="easy-menu-archive">{$t.easy.board.menu.archive}</button>
            {/if}
            <!-- A paired phone cannot delete a project: it passes no onDelete. -->
            {#if onDelete}
              <button
                type="button"
                role="menuitem"
                class="danger"
                disabled={!canMove}
                title={canMove ? undefined : $t.easy.board.menu.busyHint}
                onclick={() => { menuOpen = false; onDelete?.() }}
                data-testid="easy-menu-delete">{$t.easy.board.menu.delete}</button>
            {/if}
          </div>
        {/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .card {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: var(--space-3);
    background: var(--surface-elevated);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    min-width: 0;
  }
  .card.home {
    background: var(--surface);
    border-radius: var(--radius-lg);
    padding: 14px var(--space-4);
    gap: var(--space-2);
  }
  .card.turn {
    border-color: var(--warning);
  }
  .card:hover {
    border-color: var(--border-strong);
  }
  .card.turn:hover {
    border-color: var(--warning);
  }
  .head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }
  .name {
    color: var(--text-primary);
    text-decoration: none;
    font-weight: 600;
    font-size: var(--text-base);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    padding-right: 28px;
  }
  .card.home .name {
    font-size: 15px;
    padding-right: 0;
  }
  /* The whole card opens the project; the menu sits above this. */
  .name::after {
    content: '';
    position: absolute;
    inset: 0;
  }
  .name:focus-visible {
    outline: none;
  }
  .card:has(.name:focus-visible) {
    outline: 2px solid var(--primary);
    outline-offset: 2px;
  }
  .pill {
    margin-left: auto;
    flex-shrink: 0;
    font-size: var(--text-xs);
    border-radius: 999px;
    padding: 2px 9px;
    background: var(--surface-hover);
    color: var(--text-primary);
  }
  .pill.tone-turn {
    background: var(--warning-muted);
    color: var(--warning);
  }
  .pill.tone-work {
    background: var(--primary-muted);
    color: var(--primary-text);
  }
  .pill.tone-problem {
    background: var(--error-muted);
    color: var(--error);
  }
  .line {
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }
  .card.home .line {
    font-size: var(--text-sm);
  }
  .card.board.tone-turn .line {
    color: var(--warning);
  }
  .card.board.tone-work .line {
    color: var(--primary-text);
  }
  .card.board.tone-problem .line {
    color: var(--error);
  }
  .menu-area {
    position: absolute;
    right: var(--space-2);
    top: var(--space-2);
    z-index: 1;
  }
  /* An open menu sits above the cards after it. */
  .menu-area.open {
    z-index: 10;
  }
  .menu-button {
    width: 28px;
    height: 24px;
    font: inherit;
    line-height: 1;
    color: var(--text-secondary);
    border: 1px solid transparent;
    background: transparent;
    border-radius: var(--radius-md);
    cursor: pointer;
  }
  .menu-button:hover,
  .menu-button[aria-expanded='true'] {
    color: var(--text-primary);
    border-color: var(--border-strong);
    background: var(--surface-active);
  }
  .menu {
    position: absolute;
    right: 0;
    top: 30px;
    width: 160px;
    background: var(--surface-elevated);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    padding: var(--space-1);
    display: flex;
    flex-direction: column;
    box-shadow: 0 12px 28px rgba(0, 0, 0, 0.5);
    z-index: 5;
  }
  .menu > a,
  .menu > button {
    font: inherit;
    font-size: var(--text-sm);
    text-align: left;
    color: var(--text-primary);
    background: transparent;
    border: 0;
    border-radius: var(--radius-md);
    padding: 7px 10px;
    text-decoration: none;
    cursor: pointer;
  }
  .menu > a:hover,
  .menu > button:hover:not(:disabled) {
    background: var(--surface-hover);
  }
  .menu > button:disabled {
    color: var(--text-ghost);
    cursor: not-allowed;
  }
  .menu .sep {
    border-top: 1px solid var(--border-default);
    border-radius: 0;
  }
  .menu .danger:not(:disabled) {
    color: var(--error);
  }
  .rename {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .rename input {
    font: inherit;
    font-size: var(--text-base);
    color: var(--text-primary);
    background: var(--surface-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    padding: 6px 8px;
  }
  .rename-actions {
    display: flex;
    gap: var(--space-2);
  }
  .error {
    margin: 0;
    color: var(--error);
    font-size: var(--text-xs);
  }
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
    white-space: nowrap;
  }
</style>
