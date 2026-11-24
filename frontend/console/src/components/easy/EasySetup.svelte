<script lang="ts">
  // Easy mode's first run (docs/decisions/vibe-coder-identity.md phase 4):
  // TARS has no AI set up yet, so instead of the setup wizard (a choice of
  // provider and tiers a beginner cannot make) the server sets Claude Code
  // up and restarts. The page then loads again and the Start screen says
  // whether Claude Code still has to be installed or signed in to.
  import { onMount } from 'svelte'
  import { t } from '../../i18n'
  import { setUpEasy } from '../../lib/api/easy'
  import { getHealthz } from '../../lib/api/system'
  import { runEasySetup, type EasySetupResult } from '../../lib/easy'
  import { consoleMode } from '../../lib/stores/consoleMode.svelte'

  let { onNavigate }: { onNavigate: (path: string) => void } = $props()

  let result = $state<EasySetupResult | 'working'>('working')

  async function run() {
    result = 'working'
    result = await runEasySetup({
      setUp: setUpEasy,
      health: getHealthz,
      sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
    })
    // The whole console starts again with TARS set up.
    if (result === 'ready') window.location.reload()
  }

  onMount(() => {
    void run()
  })
</script>

<div class="easy" data-testid="easy-root">
  <section class="setup" data-testid="easy-setup" data-state={result}>
    <h1>{$t.easy.setup.title}</h1>
    {#if result === 'working' || result === 'ready'}
      <p class="line"><span class="dot"></span>{$t.easy.setup.working}</p>
    {:else}
      <p class="error" role="alert">{result === 'timeout' ? $t.easy.setup.timeout : $t.easy.setup.failed}</p>
      <div class="actions">
        <button type="button" class="btn btn-primary" onclick={run}>{$t.easy.setup.retry}</button>
        {#if consoleMode.expert}
          <button type="button" class="btn btn-ghost" onclick={() => onNavigate('/console/onboarding')}>{$t.easy.setup.byHand}</button>
        {/if}
      </div>
    {/if}
  </section>
</div>

<style>
  .easy {
    max-width: 720px;
    margin: 0 auto;
    padding: var(--space-10) var(--space-6);
  }
  .setup {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }
  h1 {
    font-family: var(--font-display);
    font-size: var(--text-2xl);
    margin: 0;
  }
  .line {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    color: var(--text-secondary);
    margin: 0;
  }
  .dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: var(--primary);
    animation: pulse 1.4s ease-in-out infinite;
  }
  .error {
    color: var(--error);
    margin: 0;
  }
  .actions {
    display: flex;
    gap: var(--space-3);
  }
  @keyframes pulse {
    50% {
      opacity: 0.3;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .dot {
      animation: none;
    }
  }
</style>
