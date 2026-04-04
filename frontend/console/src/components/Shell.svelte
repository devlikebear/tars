<script lang="ts">
  import type { Snippet } from 'svelte'
  import Nav from './Nav.svelte'
  import Header from './Header.svelte'

  interface Props {
    currentPath: string
    serverHealth?: string
    activeProject?: string
    unreadCount?: number
    onNavigate: (path: string) => void
    onUnreadChange?: (count: number) => void
    children: Snippet
  }

  let {
    currentPath,
    serverHealth = 'ok',
    activeProject = '',
    unreadCount = 0,
    onNavigate,
    onUnreadChange,
    children,
  }: Props = $props()
</script>

<div class="shell">
  <Nav {currentPath} {onNavigate} />
  <div class="shell-main">
    <Header {serverHealth} {activeProject} {unreadCount} {onUnreadChange} />
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

  .shell-content {
    flex: 1;
    padding: var(--space-6);
    width: 100%;
  }

  @media (max-width: 768px) {
    .shell-main {
      margin-left: 0;
    }
    .shell-content {
      padding: var(--space-4);
    }
  }
</style>
