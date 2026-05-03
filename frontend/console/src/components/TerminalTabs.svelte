<script lang="ts">
  import IntegratedTerminal from './IntegratedTerminal.svelte'

  export interface TerminalTab {
    id: string
    cwd: string
    label: string
  }

  interface TabStatus {
    connected: boolean
    status: string
    error: string
  }

  interface Props {
    sessionId: string
    tabs: TerminalTab[]
    activeId: string | null
    onActivate: (id: string) => void
    onCloseTab: (id: string) => void
    onAddTab?: (cwd: string, label: string) => void
  }

  let { sessionId, tabs, activeId, onActivate, onCloseTab, onAddTab }: Props = $props()

  let statuses: Record<string, TabStatus> = $state({})

  function activeTab(): TerminalTab | undefined {
    return tabs.find((t) => t.id === activeId)
  }

  function shortLabel(tab: TerminalTab): string {
    if (!tab.label) return tab.cwd
    return tab.label
  }

  function tabConnected(id: string): boolean {
    return statuses[id]?.connected ?? false
  }

  function tabHasError(id: string): boolean {
    return !!statuses[id]?.error
  }

  function onCloseClick(e: MouseEvent, id: string) {
    e.stopPropagation()
    onCloseTab(id)
    delete statuses[id]
  }

  function onAddClick() {
    const current = activeTab()
    if (!current) return
    onAddTab?.(current.cwd, current.label)
  }
</script>

<div class="terminal-tabs">
  <div class="tab-strip" role="tablist">
    {#each tabs as tab (tab.id)}
      <button
        type="button"
        role="tab"
        class="tab"
        class:active={tab.id === activeId}
        aria-selected={tab.id === activeId}
        title={tab.cwd}
        onclick={() => onActivate(tab.id)}
      >
        <span
          class="tab-dot"
          class:connected={tabConnected(tab.id)}
          class:errored={tabHasError(tab.id)}
        ></span>
        <span class="tab-label">{shortLabel(tab)}</span>
        <span
          class="tab-close"
          role="button"
          tabindex="-1"
          aria-label="Close tab"
          onclick={(e) => onCloseClick(e, tab.id)}
          onkeydown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              onCloseTab(tab.id)
              delete statuses[tab.id]
            }
          }}
        >×</span>
      </button>
    {/each}
    {#if onAddTab && tabs.length > 0}
      <button
        type="button"
        class="tab-add"
        onclick={onAddClick}
        title="New shell in same directory"
        aria-label="New tab"
      >+</button>
    {/if}
  </div>

  <div class="tab-panes">
    {#each tabs as tab (tab.id)}
      <div class="tab-pane" class:active={tab.id === activeId} aria-hidden={tab.id !== activeId}>
        <IntegratedTerminal
          {sessionId}
          cwd={tab.cwd}
          label={tab.label}
          visible={tab.id === activeId}
          hideLabel
          onClose={() => onCloseTab(tab.id)}
          onStatusChange={(s) => {
            statuses = { ...statuses, [tab.id]: s }
          }}
        />
      </div>
    {/each}
  </div>
</div>

<style>
  .terminal-tabs {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    background: var(--surface-inset);
  }

  .tab-strip {
    display: flex;
    align-items: stretch;
    gap: 1px;
    padding: 4px 4px 0;
    border-bottom: 1px solid var(--border-subtle);
    background: var(--surface);
    overflow-x: auto;
    flex-shrink: 0;
  }

  .tab {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    background: transparent;
    border: 1px solid transparent;
    border-bottom: none;
    border-radius: var(--radius-sm) var(--radius-sm) 0 0;
    color: var(--text-ghost);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 500;
    padding: 6px 10px;
    cursor: pointer;
    max-width: 220px;
    min-width: 0;
    flex-shrink: 0;
  }

  .tab:hover {
    color: var(--text-primary);
    background: var(--surface-inset);
  }

  .tab.active {
    background: var(--surface-inset);
    border-color: var(--border-subtle);
    color: var(--text-primary);
    position: relative;
    top: 1px;
  }

  .tab-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--text-ghost);
    flex-shrink: 0;
  }

  .tab-dot.connected {
    background: var(--success);
  }

  .tab-dot.errored {
    background: var(--danger, #e06c75);
  }

  .tab-label {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }

  .tab-close {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    color: var(--text-ghost);
    font-size: 14px;
    line-height: 1;
    cursor: pointer;
    flex-shrink: 0;
  }

  .tab-close:hover {
    background: var(--border-subtle);
    color: var(--text-primary);
  }

  .tab-add {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    align-self: center;
    margin-left: 4px;
    background: transparent;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    color: var(--text-ghost);
    font-size: 16px;
    line-height: 1;
    cursor: pointer;
    flex-shrink: 0;
  }

  .tab-add:hover {
    color: var(--accent, #e09145);
    border-color: var(--accent, #e09145);
  }

  .tab-panes {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
  }

  .tab-pane {
    display: none;
    flex: 1;
    flex-direction: column;
    min-height: 0;
  }

  .tab-pane.active {
    display: flex;
  }
</style>
