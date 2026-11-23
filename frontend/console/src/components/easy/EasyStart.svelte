<script lang="ts">
  // Easy mode's Start screen (docs/decisions/vibe-coder-identity.md §4,
  // phase 4): shown in place of the make screen while this computer lacks
  // what a project needs. It lists the two things and explains only the
  // next one (GET /v1/easy/readiness decides which); EasyHome reads the
  // answer again every few seconds, so finishing a step outside TARS is
  // enough to move on.
  import { t } from '../../i18n'
  import { START_COMMANDS, engineName, type EasyReadiness } from '../../lib/easy'

  let {
    readiness,
    checking,
    onCheck,
    onNavigate,
  }: {
    readiness: EasyReadiness
    checking: boolean
    onCheck: () => void
    onNavigate: (path: string) => void
  } = $props()

  const name = $derived(engineName(readiness.engine_kind))
  const claude = $derived((readiness.engine_kind ?? 'claude-code-cli') === 'claude-code-cli')
  let copied = $state('')

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text)
      copied = text
      setTimeout(() => {
        if (copied === text) copied = ''
      }, 2000)
    } catch {
      // The command is on screen to copy by hand.
    }
  }
</script>

{#snippet command(text: string)}
  <div class="command">
    <code data-testid="easy-start-command">{text}</code>
    <button type="button" class="btn btn-ghost btn-sm" onclick={() => copy(text)}>
      {copied === text ? $t.easy.start.copied : $t.easy.start.copy}
    </button>
  </div>
{/snippet}

<section class="start" data-testid="easy-start-screen">
  <h1>{$t.easy.start.title}</h1>
  <p class="subtitle">{$t.easy.start.subtitle}</p>

  <ul class="checks">
    <li data-testid="easy-start-engine" data-state={readiness.engine}>
      <span class="dot" class:ok={readiness.engine === 'ok'}></span>
      <span class="label">{$t.easy.start.checks.engine(name)}</span>
      <span class="state">{$t.easy.start.state[readiness.engine] ?? ''}</span>
    </li>
    <li data-testid="easy-start-tools" data-state={readiness.tools}>
      <span class="dot" class:ok={readiness.tools === 'ok'}></span>
      <span class="label">{$t.easy.start.checks.tools}</span>
      <span class="state">{$t.easy.start.state[readiness.tools] ?? ''}</span>
    </li>
  </ul>

  <div class="next" data-testid="easy-start-next" data-next={readiness.next}>
    <span class="eyebrow">{$t.easy.start.next}</span>
    {#if readiness.next === 'install_engine'}
      <h2>{$t.easy.start.install_engine.title(name)}</h2>
      {#if claude}
        <p>{$t.easy.start.install_engine.body}</p>
        <ol>
          <li>{$t.easy.start.install_engine.steps[0]}</li>
          <li>{$t.easy.start.install_engine.steps[1]}{@render command(START_COMMANDS.installClaude)}</li>
          <li>{$t.easy.start.install_engine.steps[2]}</li>
        </ol>
      {:else}
        <p>{$t.easy.start.install_engine.other(name)}</p>
      {/if}
    {:else if readiness.next === 'sign_in'}
      <h2>{$t.easy.start.sign_in.title(name)}</h2>
      <p>{$t.easy.start.sign_in.body}</p>
      <ol>
        <li>{$t.easy.start.sign_in.steps[0]}{@render command(START_COMMANDS.signIn)}</li>
        <li>{$t.easy.start.sign_in.steps[1]}</li>
        <li>{$t.easy.start.sign_in.steps[2]}</li>
      </ol>
    {:else if readiness.next === 'install_tools'}
      <h2>{$t.easy.start.install_tools.title}</h2>
      <p>{$t.easy.start.install_tools.body}</p>
      <ol>
        {#each $t.easy.start.install_tools.steps as step (step)}
          <li>{step}</li>
        {/each}
      </ol>
      <a class="btn btn-primary" href="https://nodejs.org/" target="_blank" rel="noopener noreferrer">{$t.easy.start.install_tools.link}</a>
    {:else if readiness.next === 'choose_engine'}
      <h2>{$t.easy.start.choose_engine.title}</h2>
      <p>{$t.easy.start.choose_engine.body}</p>
      <button type="button" class="btn btn-primary" onclick={() => onNavigate('/console/onboarding?reentry=1')}>
        {$t.easy.start.choose_engine.open}
      </button>
    {/if}
  </div>

  <footer>
    <span class="muted">{$t.easy.start.watching}</span>
    <button type="button" class="btn btn-ghost btn-sm" data-testid="easy-start-check" disabled={checking} onclick={onCheck}>
      {checking ? $t.easy.start.checking : $t.easy.start.checkNow}
    </button>
  </footer>
</section>

<style>
  .start {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }
  h1 {
    font-family: var(--font-display);
    font-size: var(--text-2xl);
    margin: 0;
  }
  h2 {
    font-size: var(--text-lg);
    margin: var(--space-1) 0 var(--space-2);
  }
  .subtitle,
  .muted {
    color: var(--text-secondary);
    margin: 0;
  }
  .checks {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .checks li {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
  }
  .dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: var(--warning);
    flex: none;
  }
  .dot.ok {
    background: var(--primary);
  }
  .label {
    flex: 1;
  }
  .state {
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }
  .next {
    padding: var(--space-5);
    border: 1px solid rgba(var(--primary-rgb), 0.4);
    border-radius: var(--radius-lg);
    background: rgba(var(--primary-rgb), 0.05);
  }
  .eyebrow {
    color: var(--primary);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .next p {
    margin: 0 0 var(--space-3);
  }
  ol {
    margin: 0 0 var(--space-4);
    padding-left: var(--space-5);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .command {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }
  code {
    font-family: var(--font-mono);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    overflow-x: auto;
  }
  footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }
</style>
