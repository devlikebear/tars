<script lang="ts">
  interface Props {
    serverHealth?: string
    activeProject?: string
    unreadCount?: number
  }

  let {
    serverHealth = 'ok',
    activeProject = '',
    unreadCount = 0,
  }: Props = $props()
</script>

<header class="header">
  <div class="header-left">
    <h1 class="header-title">Console</h1>
  </div>

  <div class="header-right">
    {#if activeProject}
      <div class="header-meta">
        <span class="label">Project</span>
        <span class="header-meta-value">{activeProject}</span>
      </div>
    {/if}

    <div class="header-indicator" class:healthy={serverHealth === 'ok'}>
      <span class="header-dot"></span>
      <span class="header-indicator-label">{serverHealth === 'ok' ? 'Connected' : 'Disconnected'}</span>
    </div>

    {#if unreadCount > 0}
      <div class="header-badge">{unreadCount}</div>
    {/if}
  </div>
</header>

<style>
  .header {
    position: sticky;
    top: 0;
    z-index: 30;
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: var(--header-height);
    padding: 0 var(--space-6);
    background: var(--bg-base);
    border-bottom: 1px solid var(--border-subtle);
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: var(--space-4);
  }

  .header-title {
    font-size: var(--text-md);
    font-weight: 500;
    color: var(--text-secondary);
  }

  .header-right {
    display: flex;
    align-items: center;
    gap: var(--space-5);
  }

  .header-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .header-meta-value {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .header-indicator {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .header-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--error);
    flex-shrink: 0;
  }
  .header-indicator.healthy .header-dot {
    background: var(--success);
  }

  .header-indicator-label {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .header-badge {
    display: flex;
    align-items: center;
    justify-content: center;
    min-width: 20px;
    height: 20px;
    padding: 0 6px;
    border-radius: 10px;
    background: var(--accent);
    color: #fff;
    font-family: var(--font-display);
    font-size: 11px;
    font-weight: 600;
  }

  @media (max-width: 768px) {
    .header { padding: 0 var(--space-4); }
    .header-meta { display: none; }
  }
</style>
