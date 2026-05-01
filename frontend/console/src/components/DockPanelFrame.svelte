<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { DockZone } from '../lib/dock/layout'

  interface Props {
    title: string
    zone: DockZone
    closeable?: boolean
    onDock: (zone: DockZone) => void
    onClose?: () => void
    children: Snippet
  }

  let {
    title,
    zone,
    closeable = true,
    onDock,
    onClose,
    children,
  }: Props = $props()

  const dockTargets: { zone: DockZone; label: string; title: string }[] = [
    { zone: 'left', label: '\u2190', title: 'Dock left' },
    { zone: 'right', label: '\u2192', title: 'Dock right' },
    { zone: 'bottom', label: '\u2193', title: 'Dock bottom' },
    { zone: 'fullscreen', label: '\u26f6', title: 'Fullscreen' },
  ]
</script>

<section class="dock-panel-frame" data-dock-zone={zone}>
  <header class="dock-panel-header">
    <strong>{title}</strong>
    <div class="dock-panel-actions">
      {#each dockTargets as target}
        <button
          type="button"
          class="dock-action"
          class:active={zone === target.zone}
          title={target.title}
          aria-label={target.title}
          onclick={() => onDock(target.zone)}
        >{target.label}</button>
      {/each}
      {#if closeable && onClose}
        <button type="button" class="dock-action dock-close" title="Close panel" aria-label="Close panel" onclick={onClose}>×</button>
      {/if}
    </div>
  </header>
  <div class="dock-panel-body">
    {@render children()}
  </div>
</section>

<style>
  .dock-panel-frame {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
    height: 100%;
    background: var(--surface);
  }

  .dock-panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    min-height: 34px;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
  }

  .dock-panel-header strong {
    min-width: 0;
    overflow: hidden;
    color: var(--text-secondary);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .dock-panel-actions {
    display: flex;
    align-items: center;
    gap: 2px;
    flex-shrink: 0;
  }

  .dock-action {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 22px;
    padding: 0;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-ghost);
    cursor: pointer;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    line-height: 1;
    transition:
      border-color var(--duration-fast) var(--ease-out),
      color var(--duration-fast) var(--ease-out),
      background var(--duration-fast) var(--ease-out);
  }

  .dock-action:hover,
  .dock-action.active {
    border-color: var(--border-default);
    background: var(--surface-elevated);
    color: var(--primary);
  }

  .dock-close:hover {
    border-color: color-mix(in srgb, var(--error) 40%, transparent);
    background: var(--error-muted);
    color: var(--error);
  }

  .dock-panel-body {
    flex: 1;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
  }
</style>
