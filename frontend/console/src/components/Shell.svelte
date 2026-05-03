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
</script>

<div class="shell" class:setup-only={needsSetup}>
  {#if !needsSetup}
    <Nav {currentPath} {onNavigate} />
  {/if}
  <div class="shell-main" class:no-nav={needsSetup}>
    <Header {serverHealth} {unreadCount} {onUnreadChange} {onNavigate} />
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

  @media (max-width: 900px) {
    .shell-main {
      margin-left: 0;
    }
    .shell-content {
      padding: var(--space-4);
    }
  }
</style>
