<script lang="ts">
  interface NavItem {
    id: string
    label: string
    path: string
    icon: string
  }

  interface Props {
    currentPath: string
    onNavigate: (path: string) => void
  }

  let { currentPath, onNavigate }: Props = $props()

  const items: NavItem[] = [
    { id: 'chat', label: 'Chat', path: '/console/chat', icon: '\u25ce' },
    { id: 'projects', label: 'Projects', path: '/console/projects', icon: '\u25eb' },
    { id: 'memory', label: 'Memory', path: '/console/memory', icon: '\u22c8' },
    { id: 'sysprompt', label: 'System Prompt', path: '/console/sysprompt', icon: '\u2691' },
    { id: 'ops', label: 'Operations', path: '/console/ops', icon: '\u2699' },
    { id: 'heartbeat', label: 'Heartbeat', path: '/console/heartbeat', icon: '\u2661' },
    { id: 'extensions', label: 'Extensions', path: '/console/extensions', icon: '\u2756' },
    { id: 'config', label: 'Settings', path: '/console/config', icon: '\u2638' },
  ]

  function isActive(itemPath: string, current: string): boolean {
    if (itemPath === '/console/chat') {
      return current === '/console' || current === '/console/' || current.startsWith('/console/chat') || current.startsWith('/console/sessions')
    }
    return current.startsWith(itemPath)
  }

  function handleClick(event: MouseEvent, path: string) {
    event.preventDefault()
    onNavigate(path)
  }
</script>

<nav class="nav" aria-label="Main navigation">
  <div class="nav-brand">
    <button type="button" class="nav-logo" onclick={(e: MouseEvent) => handleClick(e, '/console')}>
      <span class="nav-logo-mark">T</span>
      <span class="nav-logo-text">TARS</span>
    </button>
  </div>

  <div class="nav-items">
    {#each items as item}
      <a
        href={item.path}
        class="nav-item"
        class:active={isActive(item.path, currentPath)}
        onclick={(e: MouseEvent) => handleClick(e, item.path)}
      >
        <span class="nav-icon">{item.icon}</span>
        <span class="nav-label">{item.label}</span>
      </a>
    {/each}
  </div>

  <div class="nav-footer">
    <div class="nav-version">v0.13</div>
  </div>
</nav>

<style>
  .nav {
    position: fixed;
    top: 0;
    left: 0;
    width: var(--nav-width);
    height: 100vh;
    display: flex;
    flex-direction: column;
    background: var(--bg-surface);
    border-right: 1px solid var(--border-subtle);
    z-index: 40;
    overflow-y: auto;
  }

  .nav-brand {
    padding: var(--space-4) var(--space-4) var(--space-3);
  }

  .nav-logo {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    background: none;
    border: none;
    cursor: pointer;
    padding: var(--space-2);
    border-radius: var(--radius-md);
    transition: background var(--duration-fast) var(--ease-out);
  }
  .nav-logo:hover {
    background: var(--bg-elevated);
  }

  .nav-logo-mark {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border-radius: var(--radius-md);
    background: var(--accent);
    color: #fff;
    font-family: var(--font-display);
    font-weight: 600;
    font-size: var(--text-sm);
  }

  .nav-logo-text {
    font-family: var(--font-display);
    font-weight: 600;
    font-size: var(--text-md);
    color: var(--text-primary);
    letter-spacing: 0.02em;
  }

  .nav-items {
    flex: 1;
    padding: var(--space-2) var(--space-3);
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .nav-item {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: 8px var(--space-3);
    border-radius: var(--radius-md);
    color: var(--text-secondary);
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
    text-decoration: none;
    transition:
      background var(--duration-fast) var(--ease-out),
      color var(--duration-fast) var(--ease-out);
  }
  .nav-item:hover {
    background: var(--bg-elevated);
    color: var(--text-primary);
    text-decoration: none;
  }
  .nav-item.active {
    background: var(--accent-muted);
    color: var(--accent-text);
  }

  .nav-icon {
    font-size: var(--text-md);
    width: 20px;
    text-align: center;
    flex-shrink: 0;
  }

  .nav-label {
    white-space: nowrap;
  }

  .nav-footer {
    padding: var(--space-4);
    border-top: 1px solid var(--border-subtle);
  }

  .nav-version {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  @media (max-width: 768px) {
    .nav { display: none; }
  }
</style>
