<script lang="ts">
  import type { Snippet } from 'svelte'
  import Nav from './Nav.svelte'
  import Header from './Header.svelte'
  import { t } from '../i18n'

  interface Props {
    currentPath: string
    serverHealth?: string
    unreadCount?: number
    needsSetup?: boolean
    onNavigate: (path: string) => void
    onUnreadChange?: (count: number) => void
    children: Snippet
  }

  let {
    currentPath,
    serverHealth = 'ok',
    unreadCount = 0,
    needsSetup = false,
    onNavigate,
    onUnreadChange,
    children,
  }: Props = $props()

  let navOpen = $state(false)
  let navOpenedAt = 0

  function toggleNav() {
    navOpen = !navOpen
    if (navOpen) navOpenedAt = Date.now()
  }

  function closeNav() {
    // Guard against ghost clicks that fire immediately after opening on touch devices
    if (Date.now() - navOpenedAt < 350) return
    navOpen = false
  }

  function handleNavigate(path: string) {
    navOpen = false
    onNavigate(path)
  }
</script>

<div class="shell" class:setup-only={needsSetup}>
  {#if !needsSetup}
    <Nav {currentPath} onNavigate={handleNavigate} {navOpen} onClose={closeNav} />
    {#if navOpen}
      <div class="nav-overlay" role="presentation" onclick={closeNav}></div>
    {/if}
  {/if}
  <div class="shell-main" class:no-nav={needsSetup}>
    <Header {serverHealth} {unreadCount} {onUnreadChange} {onNavigate} {navOpen} onToggleNav={toggleNav} />
    {#if needsSetup}
      <div class="setup-only-banner" role="alert">
        <strong>{$t.shell.setupOnlyKicker}</strong>
        <span>{$t.shell.setupOnlyBody}</span>
      </div>
    {/if}
    <main class="shell-content">
      {@render children()}
    </main>
  </div>
</div>

<style>
  .shell {
    display: flex;
    min-height: 100vh;
  }

  .shell-main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    margin-left: var(--nav-width);
  }

  .shell-main.no-nav {
    margin-left: 0;
  }

  .setup-only-banner {
    background: rgba(224, 145, 69, 0.12);
    border-bottom: 1px solid var(--primary);
    color: var(--text-primary);
    padding: var(--space-2) var(--space-4);
    font-size: 13px;
    display: flex;
    gap: var(--space-3);
    align-items: center;
  }
  .setup-only-banner strong {
    color: var(--primary);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    font-size: 11px;
  }

  .shell-content {
    flex: 1;
    padding: var(--space-6);
    width: 100%;
  }

  .nav-overlay {
    display: none;
  }

  @media (max-width: 900px) {
    .shell-main {
      margin-left: 0;
    }
    .shell-content {
      padding: var(--space-4);
    }
    .nav-overlay {
      display: block;
      position: fixed;
      inset: 0;
      background: rgba(0, 0, 0, 0.55);
      z-index: 39;
      animation: overlayIn var(--duration-normal) var(--ease-out);
    }
  }

  @keyframes overlayIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }
</style>
