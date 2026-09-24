<script lang="ts">
  // Icon rail on the chat workbench's right edge (#968). It replaces the
  // row of eleven text toggles and the pulse mini-dashboard that sat above
  // the chat: one icon per dock panel with a count badge where it helps,
  // and the ⌘K palette at the top for everything else.
  import { t } from '../i18n'
  import type { Translations } from '../i18n'
  import { shortcutLabel } from '../lib/shortcuts'
  import { chatSession } from '../lib/stores/chatSession'
  import { chatCommands } from '../lib/stores/chatCommandQueue.svelte'
  import { chatDock, chatDockPanelTitleKeys, type ChatDockPanelID } from '../lib/stores/chatDockStore.svelte'
  import { overlays } from '../lib/stores/overlays.svelte'

  type PanelTooltipKey = keyof Translations['chat']['panels']

  // U+FE0E keeps symbols monochrome where a platform would draw an emoji.
  const railItems: { id: ChatDockPanelID; icon: string; tooltip?: PanelTooltipKey }[] = [
    { id: 'sessions', icon: '☰︎', tooltip: 'sessionsTooltip' },
    { id: 'artifacts', icon: '▤', tooltip: 'filesTooltip' },
    { id: 'git', icon: '⎇', tooltip: 'gitTooltip' },
    { id: 'tasks', icon: '☑︎', tooltip: 'tasksTooltip' },
    { id: 'terminal', icon: '›_' },
    { id: 'context', icon: '◔', tooltip: 'contextTooltip' },
    { id: 'prior', icon: '↺', tooltip: 'priorTooltip' },
    { id: 'prompt', icon: '¶', tooltip: 'promptTooltip' },
    { id: 'config', icon: '⚙︎', tooltip: 'configTooltip' },
    { id: 'skillExtraction', icon: '✦︎', tooltip: 'skillsTooltip' },
    { id: 'cron', icon: '◷', tooltip: 'cronTooltip' },
    { id: 'health', icon: '♡︎' },
  ]

  let artifactCount = $derived(chatSession.artifacts.length)
  let tasksSummary = $derived(chatSession.tasksSummary)
  let sessionHealth = $derived(chatSession.health)
  let healthIssueCount = $derived(sessionHealth.recommendations.length)

  function panelTitle(id: ChatDockPanelID): string {
    return $t.chat.panels[chatDockPanelTitleKeys[id]]
  }

  function tooltip(item: (typeof railItems)[number]): string {
    if (item.id === 'health') return sessionHealth.summary || panelTitle('health')
    if (item.id === 'tasks' && tasksSummary.total > 0) {
      return $t.chat.panels.tasksProgressTooltip(tasksSummary.completed, tasksSummary.in_progress, tasksSummary.pending)
    }
    const key = item.tooltip
    const text = key ? $t.chat.panels[key] : ''
    return typeof text === 'string' && text ? text : panelTitle(item.id)
  }

  function badge(id: ChatDockPanelID): string {
    if (id === 'artifacts' && artifactCount > 0) return String(artifactCount)
    if (id === 'tasks' && tasksSummary.total > 0) return `${tasksSummary.completed}/${tasksSummary.total}`
    if (id === 'health' && healthIssueCount > 0) return String(healthIssueCount)
    return ''
  }

  // Pressed means on screen: a tab covered by another one is not, and
  // clicking it brings it forward rather than closing it.
  function isPanelOpen(id: ChatDockPanelID): boolean {
    return chatDock.isVisible(id)
  }

  function togglePanel(id: ChatDockPanelID) {
    // The terminal needs a tab to show; the dock host knows how to open one.
    if (id === 'terminal' && !chatDock.isOpen('terminal')) {
      chatCommands.request({ kind: 'toggle-terminal' })
      return
    }
    chatDock.toggle(id)
  }
</script>

<nav class="chat-rail" aria-label={$t.rail.label}>
  <!-- The palette leads: the floating companion pet sits over the rail's
       bottom end, and ⌘K is the way to everything the rail omits. -->
  <button
    type="button"
    class="rail-btn rail-palette"
    aria-label={$t.rail.paletteHint(shortcutLabel('palette'))}
    title={$t.rail.paletteHint(shortcutLabel('palette'))}
    onclick={() => overlays.openPalette()}
  >
    <span class="rail-icon" aria-hidden="true">⌘</span>
  </button>
  <span class="rail-divider" aria-hidden="true"></span>
  {#each railItems as item (item.id)}
    <button
      type="button"
      class="rail-btn"
      class:active={isPanelOpen(item.id)}
      class:warn={item.id === 'health' && healthIssueCount > 0}
      data-panel={item.id}
      aria-label={panelTitle(item.id)}
      aria-pressed={isPanelOpen(item.id)}
      title={tooltip(item)}
      onclick={() => togglePanel(item.id)}
    >
      <span class="rail-icon" aria-hidden="true">{item.icon}</span>
      {#if badge(item.id)}
        <span class="rail-badge">{badge(item.id)}</span>
      {/if}
    </button>
  {/each}
</nav>

<style>
  .chat-rail {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
    width: 44px;
    flex-shrink: 0;
    padding: var(--space-2) 0;
    border-left: 1px solid var(--border-subtle);
    background: var(--surface);
  }

  .rail-btn {
    position: relative;
    display: grid;
    place-items: center;
    width: 34px;
    height: 34px;
    border: 0;
    border-left: 2px solid transparent;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-tertiary);
    cursor: pointer;
    transition:
      background var(--duration-fast) var(--ease-out),
      color var(--duration-fast) var(--ease-out);
  }

  .rail-btn:hover {
    background: var(--surface-hover);
    color: var(--text-primary);
  }

  .rail-btn:focus-visible {
    outline: 1px solid var(--primary);
    outline-offset: -1px;
  }

  /* Same accent rule as the palette's selected row: surface plus a 2px edge. */
  .rail-btn.active {
    background: var(--surface-active);
    border-left-color: var(--primary);
    color: var(--primary-text);
  }

  .rail-btn.warn .rail-icon {
    color: var(--warning);
  }

  .rail-icon {
    font-family: var(--font-mono);
    font-size: 0.95rem;
    line-height: 1;
  }

  .rail-badge {
    position: absolute;
    right: 1px;
    bottom: 1px;
    min-width: 14px;
    padding: 0 3px;
    border-radius: var(--radius-sm);
    background: var(--surface-elevated);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: 9px;
    line-height: 13px;
    text-align: center;
  }

  .rail-divider {
    width: 20px;
    height: 1px;
    margin: var(--space-1) 0;
    background: var(--border-default);
  }

  .rail-palette .rail-icon {
    font-size: 0.85rem;
  }

  @media (max-width: 900px) {
    .chat-rail {
      flex-direction: row;
      width: auto;
      padding: var(--space-1) var(--space-2);
      border-left: 0;
      border-bottom: 1px solid var(--border-subtle);
      overflow-x: auto;
    }
    .rail-btn {
      flex-shrink: 0;
    }
    .rail-divider {
      width: 1px;
      height: 20px;
      margin: 0 var(--space-1);
    }
  }
</style>
