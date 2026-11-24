<script lang="ts">
  // The vibe-mode shell (docs/plans/vibe-mode-redesign unit 2): the bar
  // above every easy screen. **New** opens the make screen, **My projects**
  // the board with the count of projects waiting on the person (the
  // server's your_turn count), and the dot says whether the AI is ready
  // (GET /v1/easy/readiness). Both are read again every few seconds, when
  // the tab comes back into view, and whenever a screen below says it
  // changed something (tick).
  import { onMount, type Snippet } from 'svelte'
  import { t } from '../../i18n'
  import { getEasyReadiness, listEasyBoard } from '../../lib/api/easy'
  import { easyBase } from '../../lib/easy'
  import { easyArchivePath, easyProjectsPath, shellPollMs } from '../../lib/easyBoard'
  import { consoleMode } from '../../lib/stores/consoleMode.svelte'
  import { easyReadiness } from '../../lib/stores/easyReadiness.svelte'

  let {
    active,
    tick = 0,
    onNavigate,
    children,
  }: { active: 'make' | 'projects' | 'project'; tick?: number; onNavigate: (path: string) => void; children: Snippet } = $props()

  let waiting = $state(0)
  const ai = $derived(easyReadiness.ai)
  let timer: ReturnType<typeof setTimeout> | undefined

  async function read() {
    clearTimeout(timer)
    // The home screen reads readiness itself and publishes it.
    const ownReadiness = active !== 'make'
    const [board, readiness] = await Promise.allSettled([listEasyBoard({ limit: 1 }), ownReadiness ? getEasyReadiness() : Promise.resolve(null)])
    if (board.status === 'fulfilled') waiting = board.value.counts.your_turn ?? 0
    if (ownReadiness) easyReadiness.ai = readiness.status === 'fulfilled' && readiness.value ? (readiness.value.ready ? 'ready' : 'notReady') : 'unknown'
    timer = setTimeout(read, shellPollMs)
  }

  function onVisible() {
    if (document.visibilityState === 'visible') void read()
  }

  onMount(() => {
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      clearTimeout(timer)
      document.removeEventListener('visibilitychange', onVisible)
    }
  })

  $effect(() => {
    void tick
    void active
    void read()
  })

  function go(event: MouseEvent, path: string) {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0) return
    event.preventDefault()
    onNavigate(path)
  }
</script>

<div class="vibe-shell">
  <div class="vibe-bar">
    <nav aria-label={$t.easy.shell.nav} class="vibe-nav">
      <a href={easyBase} class:current={active === 'make'} aria-current={active === 'make' ? 'page' : undefined} data-testid="vibe-nav-make" onclick={(e) => go(e, easyBase)}>
        {$t.easy.shell.make}
      </a>
      <a
        href={easyProjectsPath}
        class:current={active === 'projects'}
        aria-current={active === 'projects' ? 'page' : undefined}
        data-testid="vibe-nav-projects"
        onclick={(e) => go(e, easyProjectsPath)}
      >
        {$t.easy.shell.projects}
        {#if waiting > 0}
          <span class="waiting" data-testid="vibe-waiting-count" aria-label={$t.easy.shell.waitingCount(waiting)}>{waiting}</span>
        {/if}
      </a>
    </nav>
    <div class="vibe-right">
      <span class="ai" data-testid="vibe-ai" data-state={ai}>
        <span class="dot {ai}" aria-hidden="true"></span>{$t.easy.shell.ai[ai]}
      </span>
      {#if consoleMode.expert}
        <a href="/console" class="expert" data-testid="vibe-expert" onclick={(e) => go(e, '/console')}>{$t.easy.shell.expert}</a>
      {/if}
    </div>
  </div>
  <div class="vibe-body">
    {@render children()}
  </div>
</div>

<style>
  /* The bar runs edge to edge under the header: the shell's content
     padding (Shell.svelte .shell-content) is cancelled on three sides. */
  .vibe-shell {
    --vibe-bleed: var(--space-6);
    display: flex;
    flex-direction: column;
    min-height: 100%;
    margin: calc(-1 * var(--vibe-bleed)) calc(-1 * var(--vibe-bleed)) 0;
  }
  .vibe-bar {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-2) var(--space-6);
    border-bottom: 1px solid var(--border-subtle);
    background: var(--surface);
  }
  .vibe-nav {
    display: flex;
    gap: var(--space-1);
  }
  .vibe-nav a {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: 6px var(--space-3);
    border-radius: var(--radius-md);
    color: var(--text-secondary);
    text-decoration: none;
    font-size: var(--text-base);
    min-height: 32px;
    box-sizing: border-box;
  }
  .vibe-nav a:hover {
    color: var(--text-primary);
  }
  .vibe-nav a.current {
    color: var(--text-primary);
    background: var(--surface-hover);
  }
  .waiting {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    background: var(--warning-muted);
    color: var(--warning);
    border-radius: 999px;
    padding: 1px 7px;
  }
  .vibe-right {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: var(--space-4);
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }
  .ai {
    display: inline-flex;
    align-items: center;
    gap: 7px;
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--text-ghost);
  }
  .dot.ready {
    background: var(--primary);
  }
  .dot.notReady {
    background: var(--warning);
  }
  .expert {
    color: var(--text-secondary);
    text-decoration: none;
  }
  .expert:hover {
    color: var(--text-primary);
  }
  .vibe-body {
    flex: 1;
    min-height: 0;
  }
  @media (max-width: 900px) {
    .vibe-shell {
      --vibe-bleed: var(--space-4);
    }
  }
  @media (max-width: 640px) {
    .vibe-bar {
      padding: var(--space-2) var(--space-4);
      gap: var(--space-2);
    }
    .ai {
      font-size: 0;
      gap: 0;
    }
  }
</style>
