<script lang="ts">
  // Section tabs (#1238): 19 schema sections grouped into 8 tabs
  // (lib/configSections.ts). The parent owns the active tab and URL sync.
  import { t } from '../i18n'
  import { CONFIG_TABS } from '../lib/configSections'
  import type { ConfigTabId } from '../lib/configSections'

  interface Props {
    active: ConfigTabId
    onSelect: (id: ConfigTabId) => void
  }

  let { active, onSelect }: Props = $props()
</script>

<nav class="config-tabs" aria-label={$t.config.pageTitle}>
  {#each CONFIG_TABS as tab}
    <button
      type="button"
      class="config-tab"
      class:active={tab.id === active}
      aria-current={tab.id === active ? 'page' : undefined}
      onclick={() => onSelect(tab.id)}
    >
      {$t.config.tabs[tab.id]}
    </button>
  {/each}
</nav>

<style>
  .config-tabs {
    display: flex;
    gap: var(--space-1);
    flex-wrap: wrap;
    border-bottom: 1px solid var(--border-subtle);
    padding-bottom: var(--space-1);
  }

  .config-tab {
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    padding: var(--space-2) var(--space-3);
    color: var(--text-secondary);
    font-family: var(--font-display);
    font-size: var(--text-sm);
    cursor: pointer;
    white-space: nowrap;
  }

  .config-tab:hover {
    color: var(--text-primary);
  }

  .config-tab.active {
    color: var(--primary);
    border-bottom-color: var(--primary);
  }

  @media (max-width: 640px) {
    .config-tabs {
      overflow-x: auto;
      flex-wrap: nowrap;
    }
  }
</style>
