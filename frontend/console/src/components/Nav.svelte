<script lang="ts">
  import { onMount } from 'svelte'
  import { t } from '../i18n'
  import type { Translations } from '../i18n'
  import { getServerStatus } from '../lib/api'
  import StatusStrip from './StatusStrip.svelte'

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
    navOpen?: boolean
    onClose?: () => void
  }

  let { currentPath, onNavigate, navOpen = false, onClose }: Props = $props()
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
        { id: 'lineage', path: '/console/sessions/graph', icon: '\u257f' },
        { id: 'plans', path: '/console/tasks', icon: '\u2637' },
        { id: 'memory', path: '/console/memory', icon: '\u22c8' },
        { id: 'sysprompt', path: '/console/sysprompt', icon: '\u2691' },
        { id: 'extensions', path: '/console/extensions', icon: '\u2756' },
      ],
    },
    {
      id: 'operate',
      items: [
        { id: 'agentruntime', path: '/console/agentruntime', icon: '\u25c8' },
        { id: 'channels', path: '/console/channels', icon: '\u2709' },
        { id: 'ops', path: '/console/approvals', icon: '\u2699' },
        { id: 'cron', path: '/console/cron', icon: '\u23f2' },
        { id: 'logs', path: '/console/logs', icon: '\u2261' },
        { id: 'analytics', path: '/console/analytics', icon: '\u25b1' },
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
      return current.startsWith('/console/chat') || (current.startsWith('/console/sessions') && !current.startsWith('/console/sessions/graph'))
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

  function handleClose(event: MouseEvent) {
    event.preventDefault()
    onClose?.()
  }

  function groupLabel(id: NavGroupId): string {
    return $t.nav.groups[id]
  }

  function itemLabel(id: NavItemId): string {
    return $t.nav.items[id]
  }
</script>

<nav class="nav" style={navOpen ? 'transform: translateX(0)' : undefined} aria-label={$t.nav.mainNavigation}>
  <div class="nav-brand">
    <button type="button" class="nav-logo" onclick={(e: MouseEvent) => handleClick(e, '/console')}>
      <span class="nav-logo-frame" aria-hidden="true">
        <img class="nav-logo-mark" src="/console/tars-icon.png" alt="" width="28" height="28" />
      </span>
      <span class="nav-logo-text">TARS</span>
    </button>
    <button type="button" class="nav-close-btn" aria-label="Close navigation" onclick={handleClose}>✕</button>
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
    <StatusStrip {onNavigate} />
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

  .nav-logo-frame {
    width: 34px;
    height: 34px;
    padding: 3px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--radius-md);
    border: 1px solid rgba(224, 145, 69, 0.28);
    background: rgba(224, 145, 69, 0.16);
    box-sizing: border-box;
    flex-shrink: 0;
  }

  .nav-logo-mark {
    width: 28px;
    height: 28px;
    object-fit: contain;
    filter: brightness(1.35) contrast(1.05);
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
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-4);
    border-top: 1px solid var(--border-subtle);
  }

  .nav-version {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  .nav-close-btn {
    display: none;
  }

  @media (max-width: 900px) {
    .nav {
      /* Mobile drawer: use explicit width because :root overrides --nav-width to 0 on mobile */
      width: min(280px, 80vw);
      transform: translateX(-100%);
      transition: transform var(--duration-normal) var(--ease-out), box-shadow var(--duration-normal) var(--ease-out);
    }
    .nav[style*='translateX(0)'] {
      box-shadow: 8px 0 32px rgba(0, 0, 0, 0.5);
    }
    .nav-brand {
      display: flex;
      align-items: center;
      justify-content: space-between;
    }
    .nav-close-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 28px;
      height: 28px;
      border: none;
      border-radius: var(--radius-md);
      background: transparent;
      color: var(--text-tertiary);
      font-size: var(--text-base);
      cursor: pointer;
      transition: background var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out);
      flex-shrink: 0;
    }
    .nav-close-btn:hover {
      background: var(--surface-elevated);
      color: var(--text-primary);
    }
  }
</style>
