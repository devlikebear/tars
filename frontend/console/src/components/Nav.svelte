<script lang="ts">
  import { onMount } from 'svelte'
  import { t } from '../i18n'
  import type { Translations } from '../i18n'
  import { getServerStatus } from '../lib/api'

  type NavGroupId = keyof Translations['nav']['groups']
  type NavItemId = keyof Translations['nav']['items']

  interface NavItem {
    id: NavItemId
    path: string
    icon: string
  }

  interface NavGroup {
    id: NavGroupId
    items: NavItem[]
  }

  interface Props {
    currentPath: string
    onNavigate: (path: string) => void
  }

  let { currentPath, onNavigate }: Props = $props()
  let version = $state('')

  onMount(async () => {
    try {
      const status = await getServerStatus()
      version = status.version ?? ''
    } catch {
      version = ''
    }
  })

  const groups: NavGroup[] = [
    {
      id: 'work',
      items: [
        { id: 'chat', path: '/console/chat', icon: '\u25ce' },
        { id: 'memory', path: '/console/memory', icon: '\u22c8' },
        { id: 'sysprompt', path: '/console/sysprompt', icon: '\u2691' },
        { id: 'extensions', path: '/console/extensions', icon: '\u2756' },
      ],
    },
    {
      id: 'operate',
      items: [
        { id: 'agentruntime', path: '/console/agentruntime', icon: '\u25c8' },
        { id: 'ops', path: '/console/approvals', icon: '\u2699' },
        { id: 'pulse', path: '/console/pulse', icon: '\u2661' },
        { id: 'reflection', path: '/console/reflection', icon: '\u263e' },
      ],
    },
    {
      id: 'setup',
      items: [
        { id: 'config', path: '/console/config', icon: '\u2638' },
      ],
    },
  ]

  function isActive(itemPath: string, current: string): boolean {
    if (itemPath === '/console/chat') {
      return current.startsWith('/console/chat') || current.startsWith('/console/sessions')
    }
    if (itemPath === '/console/approvals') {
      return current.startsWith('/console/approvals') || current.startsWith('/console/ops')
    }
    return current.startsWith(itemPath)
  }

  function handleClick(event: MouseEvent, path: string) {
    event.preventDefault()
    onNavigate(path)
  }

  function groupLabel(id: NavGroupId): string {
    return $t.nav.groups[id]
  }

  function itemLabel(id: NavItemId): string {
    return $t.nav.items[id]
  }
</script>

<nav class="nav" aria-label={$t.nav.mainNavigation}>
  <div class="nav-brand">
    <button type="button" class="nav-logo" onclick={(e: MouseEvent) => handleClick(e, '/console')}>
      <span class="nav-logo-mark">T</span>
      <span class="nav-logo-text">TARS</span>
    </button>
  </div>

  <div class="nav-items">
    {#each groups as group}
      <section class="nav-group" aria-label={`${groupLabel(group.id)} ${$t.nav.navigationSuffix}`}>
        <div class="nav-group-label">{groupLabel(group.id)}</div>
        {#each group.items as item}
          <a
            href={item.path}
            class="nav-item"
            class:active={isActive(item.path, currentPath)}
            onclick={(e: MouseEvent) => handleClick(e, item.path)}
          >
            <span class="nav-icon">{item.icon}</span>
            <span class="nav-label">{itemLabel(item.id)}</span>
          </a>
        {/each}
      </section>
    {/each}
  </div>

  <div class="nav-footer">
    <div class="nav-version">{version ? `v${version}` : ''}</div>
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
    background: var(--surface);
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
    background: var(--surface-elevated);
  }

  .nav-logo-mark {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border-radius: var(--radius-md);
    background: var(--primary);
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
    gap: var(--space-4);
  }

  .nav-group {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .nav-group-label {
    padding: 0 var(--space-3) var(--space-1);
    color: var(--text-ghost);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0;
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
    background: var(--surface-elevated);
    color: var(--text-primary);
    text-decoration: none;
  }
  .nav-item.active {
    background: var(--primary-muted);
    color: var(--primary-text);
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
