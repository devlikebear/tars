<script lang="ts">
  // ⌘K command palette (#968). Mounted once in App so it works on every
  // route. It only filters and runs the commands it is given; App builds the
  // list from pages, dock panels, sessions, and slash commands.
  import { tick } from 'svelte'
  import { t } from '../i18n'
  import { filterCommands, type CommandGroup, type PaletteCommand } from '../lib/commands'
  import { shortcutLabel } from '../lib/shortcuts'

  interface Props {
    commands: PaletteCommand[]
    onClose: () => void
  }

  let { commands, onClose }: Props = $props()

  let query = $state('')
  let activeIndex = $state(0)
  let inputEl: HTMLInputElement | undefined = $state()
  let listEl: HTMLUListElement | undefined = $state()
  const returnFocusTo = typeof document === 'undefined' ? null : (document.activeElement as HTMLElement | null)

  let results = $derived(filterCommands(commands, query))

  // Keep the highlighted row valid as results change.
  $effect(() => {
    void query
    activeIndex = 0
  })

  $effect(() => {
    void tick().then(() => inputEl?.focus())
    return () => {
      // Hand focus back to where the user was, unless the command moved it.
      if (returnFocusTo?.isConnected && document.activeElement === document.body) returnFocusTo.focus()
    }
  })

  function groupLabel(group: CommandGroup): string {
    return $t.palette.groups[group]
  }

  // Show a group heading on the first row of each group.
  function startsGroup(index: number): boolean {
    return index === 0 || results[index - 1]?.group !== results[index]?.group
  }

  async function runAt(index: number) {
    const command = results[index]
    if (!command) return
    onClose()
    await command.run()
  }

  function move(delta: number) {
    if (results.length === 0) return
    activeIndex = (activeIndex + delta + results.length) % results.length
    void tick().then(() => {
      listEl?.querySelector<HTMLElement>(`[data-index="${activeIndex}"]`)?.scrollIntoView({ block: 'nearest' })
    })
  }

  function handleKeydown(event: KeyboardEvent) {
    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault()
        move(1)
        return
      case 'ArrowUp':
        event.preventDefault()
        move(-1)
        return
      case 'Enter':
        event.preventDefault()
        void runAt(activeIndex)
        return
      case 'Escape':
        event.preventDefault()
        onClose()
        return
    }
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="palette-backdrop" onclick={onClose}>
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div
    class="palette"
    role="dialog"
    aria-modal="true"
    aria-label={$t.palette.title}
    tabindex="-1"
    onclick={(event) => event.stopPropagation()}
    onkeydown={handleKeydown}
  >
    <input
      bind:this={inputEl}
      bind:value={query}
      class="palette-input"
      type="text"
      role="combobox"
      aria-expanded="true"
      aria-controls="command-palette-results"
      aria-activedescendant={results[activeIndex] ? `palette-item-${activeIndex}` : undefined}
      placeholder={$t.palette.placeholder}
      autocomplete="off"
      spellcheck="false"
    />
    {#if results.length === 0}
      <div class="palette-empty">{$t.palette.empty}</div>
    {:else}
      <ul bind:this={listEl} id="command-palette-results" class="palette-list" role="listbox" aria-label={$t.palette.title}>
        {#each results as command, index (command.id)}
          {#if startsGroup(index)}
            <li class="palette-group" role="presentation">{groupLabel(command.group)}</li>
          {/if}
          <li
            id={`palette-item-${index}`}
            data-index={index}
            data-group={command.group}
            class="palette-item"
            class:active={index === activeIndex}
            role="option"
            aria-selected={index === activeIndex}
            onmousemove={() => { activeIndex = index }}
            onclick={() => void runAt(index)}
          >
            <span class="palette-item-title">{command.title}</span>
            {#if command.subtitle}
              <span class="palette-item-subtitle">{command.subtitle}</span>
            {/if}
            {#if command.shortcut}
              <kbd class="palette-item-shortcut">{shortcutLabel(command.shortcut)}</kbd>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
    <div class="palette-footer">{$t.palette.footer}</div>
  </div>
</div>

<style>
  .palette-backdrop {
    position: fixed;
    inset: 0;
    z-index: 200;
    display: flex;
    justify-content: center;
    align-items: flex-start;
    padding: 12vh var(--space-4) var(--space-4);
    background: rgba(0, 0, 0, 0.45);
  }

  /* DESIGN.md: surface-elevated, border-default, no shadow. */
  .palette {
    width: min(640px, 100%);
    max-height: 70vh;
    display: flex;
    flex-direction: column;
    background: var(--surface-elevated);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-lg);
    overflow: hidden;
  }

  .palette-input {
    padding: var(--space-3) var(--space-4);
    background: transparent;
    border: 0;
    border-bottom: 1px solid var(--border-subtle);
    color: var(--text-primary);
    font-family: var(--font-body);
    font-size: var(--text-md);
    outline: none;
  }

  .palette-input::placeholder {
    color: var(--text-tertiary);
  }

  .palette-list {
    list-style: none;
    margin: 0;
    padding: var(--space-1) 0;
    overflow-y: auto;
  }

  .palette-group {
    padding: var(--space-2) var(--space-4) var(--space-1);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 500;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--text-tertiary);
  }

  .palette-item {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-4);
    border-left: 2px solid transparent;
    cursor: pointer;
    color: var(--text-primary);
    font-size: var(--text-sm);
  }

  /* DESIGN.md: selected row takes surface-active plus a 2px primary edge. */
  .palette-item.active {
    background: var(--surface-active);
    border-left-color: var(--primary);
  }

  .palette-item-title {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .palette-item-subtitle {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    white-space: nowrap;
  }

  .palette-item-shortcut {
    padding: 1px var(--space-2);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .palette-empty {
    padding: var(--space-6) var(--space-4);
    color: var(--text-tertiary);
    font-size: var(--text-sm);
    text-align: center;
  }

  .palette-footer {
    padding: var(--space-2) var(--space-4);
    border-top: 1px solid var(--border-subtle);
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }
</style>
