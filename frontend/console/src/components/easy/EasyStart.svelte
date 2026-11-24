<script lang="ts">
  // Easy mode's Start screen (docs/decisions/vibe-coder-identity.md §4,
  // phase 4; redrawn in docs/plans/vibe-mode-redesign unit 5, screens/
  // D-Start): shown in place of the make screen while this computer lacks
  // what a project needs. The three steps sit on the left, done, now or
  // still to come (startSteps); the right explains only the one to do now
  // (GET /v1/easy/readiness decides which). EasyHome reads the answer again
  // every few seconds, so finishing a step outside TARS is enough to move on.
  import { t } from '../../i18n'
  import { START_COMMANDS, START_STEPS, engineName, startSteps, type EasyReadiness } from '../../lib/easy'
  import { consoleMode } from '../../lib/stores/consoleMode.svelte'
  import EasyStartFrame from './EasyStartFrame.svelte'

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

  // Where the plans are, for someone who has no paid Claude plan yet.
  const plansURL = 'https://claude.ai/upgrade'

  const name = $derived(engineName(readiness.engine_kind))
  const claude = $derived((readiness.engine_kind ?? 'claude-code-cli') === 'claude-code-cli')
  const progress = $derived(startSteps(readiness))
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
    <button type="button" class="btn btn-secondary btn-sm" onclick={() => copy(text)}>
      {copied === text ? $t.easy.start.copied : $t.easy.start.copy}
    </button>
  </div>
{/snippet}

{#snippet numbered(steps: string[], withCommand?: { at: number; text: string })}
  <ol class="card">
    {#each steps as step, i (step)}
      <li>
        <span class="n" aria-hidden="true">{i + 1}</span>
        <div class="step">
          {step}
          {#if withCommand && withCommand.at === i}{@render command(withCommand.text)}{/if}
        </div>
      </li>
    {/each}
  </ol>
{/snippet}

{#snippet foot()}
  {#if consoleMode.expert}
    <button type="button" class="link" onclick={() => onNavigate('/console/onboarding?reentry=1')}>{$t.easy.start.expert}</button>
  {/if}
{/snippet}

<section class="start" data-testid="easy-start-screen">
  <EasyStartFrame steps={progress.steps} {foot}>
    <div class="next" data-testid="easy-start-next" data-next={readiness.next}>
      {#if progress.at}
        <span class="eyebrow">{$t.easy.start.now(progress.at, START_STEPS.length)}</span>
      {/if}
      {#if readiness.next === 'install_engine'}
        <h2>{$t.easy.start.install_engine.title(name)}</h2>
        {#if claude}
          <p class="body">{$t.easy.start.install_engine.body}</p>
          {@render paid()}
          {@render numbered($t.easy.start.install_engine.steps, { at: 1, text: START_COMMANDS.installClaude })}
        {:else}
          <p class="body">{$t.easy.start.install_engine.other(name)}</p>
        {/if}
      {:else if readiness.next === 'sign_in'}
        <h2>{$t.easy.start.sign_in.title(name)}</h2>
        <p class="body">{$t.easy.start.sign_in.body}</p>
        {#if claude}{@render paid()}{/if}
        {@render numbered($t.easy.start.sign_in.steps)}
        <details class="help">
          <summary>{$t.easy.start.help}</summary>
          <p>{$t.easy.start.sign_in.command}</p>
          {@render command(START_COMMANDS.signIn)}
        </details>
      {:else if readiness.next === 'install_tools'}
        <h2>{$t.easy.start.install_tools.title}</h2>
        <p class="body">{$t.easy.start.install_tools.body}</p>
        {@render numbered($t.easy.start.install_tools.steps)}
        <a class="btn btn-primary go" href="https://nodejs.org/" target="_blank" rel="noopener noreferrer">{$t.easy.start.install_tools.link}</a>
      {:else if readiness.next === 'choose_engine'}
        <h2>{$t.easy.start.choose_engine.title}</h2>
        <p class="body">{$t.easy.start.choose_engine.body}</p>
        <button type="button" class="btn btn-primary go" onclick={() => onNavigate('/console/onboarding?reentry=1')}>
          {$t.easy.start.choose_engine.open}
        </button>
      {/if}
    </div>

    <footer>
      <span class="dot" aria-hidden="true"></span>
      <span class="muted">{$t.easy.start.watching}</span>
      <button type="button" class="btn btn-secondary btn-sm" data-testid="easy-start-check" disabled={checking} onclick={onCheck}>
        {checking ? $t.easy.start.checking : $t.easy.start.checkNow}
      </button>
    </footer>
  </EasyStartFrame>
</section>

{#snippet paid()}
  <div class="paid" data-testid="easy-start-paid">
    <span>{$t.easy.start.paid}</span>
    <a href={plansURL} target="_blank" rel="noopener noreferrer">{$t.easy.start.paidLink}</a>
  </div>
{/snippet}

<style>
  .next {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 640px;
  }
  .eyebrow {
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: 0.08em;
    color: var(--primary-text);
  }
  h2 {
    margin: 0;
    font-size: var(--text-2xl);
    font-weight: 600;
  }
  .body {
    margin: 0;
    color: var(--text-secondary);
    line-height: 1.65;
  }
  .paid {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    background: var(--warning-muted);
    border: 1px solid var(--warning);
    border-radius: var(--radius-md);
    line-height: 1.6;
  }
  .paid span {
    flex: 1;
  }
  .paid a {
    color: var(--warning);
    white-space: nowrap;
  }
  .card {
    list-style: none;
    margin: 0;
    padding: var(--space-5);
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    background: var(--surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
  }
  .card li {
    display: flex;
    gap: var(--space-3);
    align-items: flex-start;
  }
  .n {
    width: 26px;
    height: 26px;
    flex: none;
    border-radius: 50%;
    background: var(--surface-active);
    display: grid;
    place-items: center;
    font-family: var(--font-mono);
    font-size: var(--text-sm);
  }
  .step {
    flex: 1;
    min-width: 0;
    padding-top: 2px;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  .command {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
  }
  code {
    flex: 1;
    min-width: 0;
    font-family: var(--font-mono);
    overflow-x: auto;
    white-space: nowrap;
  }
  .help {
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }
  .help p {
    margin: var(--space-3) 0 var(--space-2);
  }
  .go {
    align-self: flex-start;
    min-height: 44px;
  }
  footer {
    margin-top: auto;
    max-width: 640px;
    display: flex;
    align-items: center;
    gap: var(--space-3);
    font-size: var(--text-sm);
  }
  footer .btn {
    margin-left: auto;
  }
  .dot {
    width: 7px;
    height: 7px;
    flex: none;
    border-radius: 50%;
    background: var(--primary);
  }
  .muted {
    color: var(--text-secondary);
  }
  .link {
    font: inherit;
    color: var(--text-secondary);
    background: transparent;
    border: 0;
    padding: 0;
    cursor: pointer;
    text-align: left;
  }
  .link:hover {
    color: var(--text-primary);
  }
</style>
