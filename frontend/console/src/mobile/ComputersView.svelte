<script lang="ts">
  // The Computers tab (docs/plans/vibe-mode-redesign unit 8,
  // screens/M-Pair): each paired desktop with, in plain words, whether it
  // is on, signed in to the AI and has the making tools (GET
  // /v1/easy/readiness), how many projects it holds, and pairing another
  // one. Getting a desk ready happens at the desk, so a problem only says
  // so. Keeping the desk awake is the desk's own setting, which a phone
  // cannot read; it is not shown.
  import { t } from '../i18n'
  import type { DesktopStore } from '../lib/mobile/desktopStore.svelte'
  import type { ProjectStore } from '../lib/mobile/projectStore.svelte'
  import { isDemoDesktop } from '../lib/mobile/demoDesk'
  import { deskLines } from '../lib/mobile/projects'
  import type { PairedDesktop } from '../lib/mobile/desktops'

  interface Props {
    store: DesktopStore
    projects: ProjectStore
    // Ideas kept on this phone, not sent yet.
    savedIdeas: number
    checking: boolean
    onPair: (desktop?: PairedDesktop) => void
    onDemo: () => void
    onForget: (desktop: PairedDesktop) => void
    onRefresh: () => void
    onIdeas: () => void
  }

  let { store, projects, savedIdeas, checking, onPair, onDemo, onForget, onRefresh, onIdeas }: Props = $props()

  const hasDemo = $derived(store.desktops.some(isDemoDesktop))

  function summary(desktop: PairedDesktop): string {
    const list = projects.byDesk[desktop.id] ?? []
    const working = list.filter((p) => p.board.state === 'working' || p.board.state === 'starting').length
    return $t.phone.computers.summary(list.length, working)
  }
</script>

<section class="computers" aria-labelledby="phone-computers-title">
  <div class="title-row">
    <h1 id="phone-computers-title">{$t.phone.computers.title}</h1>
    <button type="button" class="btn btn-ghost btn-sm" onclick={onRefresh} disabled={!store.loaded || checking}>
      {checking ? $t.phone.computers.checking : $t.phone.computers.refresh}
    </button>
  </div>

  {#each store.desktops as desktop (desktop.id)}
    {@const status = store.statuses[desktop.id]?.state ?? 'checking'}
    <article class="desk" data-testid="mobile-desktop" data-state={status}>
      <div class="desk-head">
        <span class="dot {status}" aria-hidden="true"></span>
        <strong>{desktop.name}</strong>
      </div>
      {#if isDemoDesktop(desktop)}
        <span class="hint" data-testid="mobile-demo-hint">{$t.phone.computers.demoHint}</span>
      {/if}

      {#if status === 'online'}
        {@const lines = deskLines(projects.readiness[desktop.id])}
        <ul class="lines" data-testid="phone-desk-lines" data-ready={lines.every((l) => l.ok) ? 'true' : 'false'}>
          {#each lines as line (line.key)}
            <li class:bad={!line.ok}><span aria-hidden="true">{line.ok ? '✓' : '!'}</span> {$t.phone.computers.lines[line.key]}</li>
          {/each}
        </ul>
        {#if lines.some((l) => !l.ok)}<p class="hint">{$t.phone.computers.setupHint}</p>{/if}
        <p class="summary">{summary(desktop)}</p>
      {:else if status === 'checking'}
        <p class="hint">{$t.phone.computers.checking}</p>
      {:else if status === 'unpaired'}
        <p class="state bad">{$t.phone.computers.unpaired}</p>
        <p class="hint">{$t.phone.computers.unpairedHint}</p>
      {:else if status === 'setup'}
        <p class="state bad">{$t.phone.computers.setup}</p>
        <p class="hint">{$t.phone.computers.setupHint}</p>
      {:else}
        <p class="state">{$t.phone.computers.off}</p>
        <p class="hint">{$t.phone.computers.offHint}</p>
        {#if savedIdeas > 0}
          <button type="button" class="link" onclick={onIdeas}>{$t.phone.computers.savedIdeas(savedIdeas)}</button>
        {/if}
      {/if}

      <div class="actions">
        {#if status === 'unpaired'}
          <button type="button" class="btn btn-primary btn-sm" onclick={() => onPair(desktop)}>{$t.phone.computers.reconnect}</button>
        {/if}
        <button type="button" class="btn btn-ghost btn-sm" onclick={() => onForget(desktop)}>{$t.phone.computers.forget}</button>
      </div>
    </article>
  {/each}

  <div class="add">
    <h2>{$t.phone.computers.addTitle}</h2>
    <ol>
      {#each $t.phone.computers.addSteps as step (step)}<li>{step}</li>{/each}
    </ol>
    <button type="button" class="btn btn-primary go" onclick={() => onPair()} data-testid="mobile-pair-open">{$t.phone.computers.add}</button>
    {#if store.loaded && !hasDemo}
      <button type="button" class="btn btn-ghost go" onclick={onDemo} data-testid="mobile-demo-open">{$t.phone.computers.demo}</button>
    {/if}
  </div>
</section>

<style>
  .computers {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  .title-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--text-xl);
  }
  h2 {
    margin: 0;
    font-size: var(--text-md);
  }
  .desk {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--surface);
    padding: var(--space-4);
  }
  .desk-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .desk-head strong {
    font-size: 17px;
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--text-tertiary);
    flex: none;
  }
  .dot.online {
    background: var(--success);
  }
  .dot.unpaired,
  .dot.setup {
    background: var(--warning);
  }
  .lines {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: var(--text-sm);
  }
  .lines li span {
    color: var(--success);
  }
  .lines li.bad,
  .lines li.bad span {
    color: var(--warning);
  }
  .state {
    margin: 0;
    font-size: var(--text-sm);
  }
  .state.bad {
    color: var(--warning);
  }
  .hint,
  .summary {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--text-secondary);
    line-height: 1.5;
  }
  .link {
    align-self: flex-start;
    border: 0;
    background: none;
    padding: 0;
    color: var(--primary-text);
    font: inherit;
    font-size: var(--text-sm);
    min-height: 36px;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
  .add {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin-top: var(--space-3);
  }
  .add ol {
    margin: 0;
    padding-left: var(--space-5);
    font-size: var(--text-sm);
    color: var(--text-secondary);
    line-height: 1.6;
  }
  .go {
    width: 100%;
    min-height: 48px;
    font-size: 16px;
  }
</style>
