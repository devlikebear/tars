<script lang="ts">
  // The "waiting on you" inbox (docs/decisions/mobile-remote.md §2): every
  // focus pipeline that waits on its developer, across all paired desktops,
  // labeled by desktop. A tool call that waits is answered here; a gate or a
  // card opens in that desktop's console until the app has focus screens.
  import { t } from '../i18n'
  import { stageLabel } from '../lib/focus'
  import { consoleURL, type InboxApproval, type InboxEntry } from '../lib/mobile/inbox'
  import type { InboxStore } from '../lib/mobile/inboxStore.svelte'
  import type { PairedDesktop } from '../lib/mobile/desktops'

  interface Props {
    inbox: InboxStore
    desktops: PairedDesktop[]
    // Names of the desktops whose pipelines are not in the list now.
    missing: string[]
    checking: boolean
  }

  let { inbox, desktops, missing, checking }: Props = $props()

  function desktopOf(entry: InboxEntry): PairedDesktop | undefined {
    return desktops.find((d) => d.id === entry.desktopID)
  }

  function stage(entry: InboxEntry): string {
    if (!entry.current) return ''
    return stageLabel({ template: entry.template, stages: [{ id: entry.current, label: entry.currentLabel }] }, entry.current, $t.focus)
  }

  function answer(entry: InboxEntry, approval: InboxApproval, allow: boolean) {
    const desktop = desktopOf(entry)
    if (desktop) void inbox.answer(desktop, approval, allow)
  }
</script>

<section class="inbox" aria-label={$t.mobileApp.tabs.inbox}>
  {#if desktops.length === 0}
    <p class="empty" data-testid="mobile-inbox-no-desktops">{$t.mobileApp.inboxNoDesktops}</p>
  {:else if !inbox.loaded && checking}
    <p class="empty">{$t.mobileApp.inboxChecking}</p>
  {:else if inbox.entries.length === 0}
    <div class="empty" data-testid="mobile-inbox-empty">
      <strong>{$t.mobileApp.inboxEmpty}</strong>
      <span>{$t.mobileApp.inboxEmptyHint}</span>
    </div>
  {/if}

  {#each inbox.entries as entry (entry.key)}
    {@const desktop = desktopOf(entry)}
    <article class="entry" data-testid="mobile-inbox-entry" data-session={entry.sessionID}>
      <div class="entry-head">
        <span class="desk">{entry.desktopName}</span>
        {#if stage(entry)}
          <span class="stage">{stage(entry)}</span>
        {/if}
      </div>
      <strong class="title">{entry.title}</strong>
      {#if entry.gate}
        <span class="waits gate">{$t.mobileApp.gates[entry.gate] ?? entry.gate}</span>
      {/if}
      {#if entry.cards > 0}
        <span class="waits">{$t.mobileApp.cardsToDecide(entry.cards)}</span>
      {/if}

      {#each entry.approvals as approval (approval.id)}
        {@const busy = inbox.answering[approval.id]}
        {@const failure = inbox.answerFailures[approval.id]}
        <div class="call" data-testid="mobile-inbox-call" data-kind={approval.kind}>
          <span class="call-label">{$t.mobileApp.toolCall}</span>
          {#if approval.toolName}
            <code class="tool">{approval.toolName}</code>
          {/if}
          {#if approval.preview}
            <pre class="preview">{approval.preview}</pre>
          {/if}
          {#if failure}
            <span class="call-failure" role="alert">{$t.mobileApp.answerFailures[failure]}</span>
          {/if}
          <div class="call-actions">
            <button type="button" class="btn btn-ghost btn-sm" disabled={busy} onclick={() => answer(entry, approval, false)}>
              {approval.kind === 'queued' ? $t.mobileApp.reject : $t.mobileApp.deny}
            </button>
            <button type="button" class="btn btn-primary btn-sm" disabled={busy} onclick={() => answer(entry, approval, true)}>
              {approval.kind === 'queued' ? $t.mobileApp.approve : $t.mobileApp.allowOnce}
            </button>
          </div>
        </div>
      {/each}

      {#if desktop}
        <a class="open" href={consoleURL(desktop.baseURL, entry.sessionID)} target="_blank" rel="noopener noreferrer">
          {$t.mobileApp.openOnDesktop}
        </a>
      {/if}
    </article>
  {/each}

  {#if missing.length > 0}
    <p class="missing" data-testid="mobile-inbox-missing">{$t.mobileApp.notShown(missing.join(', '))}</p>
  {/if}
</section>

<style>
  .inbox {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .empty {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    font-size: var(--text-sm);
    color: var(--text-secondary);
    line-height: 1.5;
  }

  .empty strong {
    color: var(--text-primary);
    font-size: 16px;
  }

  .entry {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--surface);
    padding: var(--space-4);
  }

  .entry-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-xs);
  }

  .desk {
    font-family: var(--font-mono);
    color: rgb(var(--primary-rgb));
    background: rgba(var(--primary-rgb), 0.1);
    border-radius: var(--radius-sm);
    padding: 2px 6px;
  }

  .stage {
    margin-left: auto;
    color: var(--text-secondary);
  }

  .title {
    font-size: 16px;
    color: var(--text-primary);
    line-height: 1.35;
    overflow-wrap: anywhere;
  }

  .waits {
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }

  .waits.gate {
    color: var(--warning);
  }

  .call {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin-top: var(--space-2);
    padding: var(--space-3);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
  }

  .call-label {
    font-size: var(--text-xs);
    color: var(--warning);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  .tool {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .preview {
    margin: 0;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-secondary);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    max-height: 8.5em;
    overflow: hidden;
  }

  .call-failure {
    font-size: var(--text-sm);
    color: var(--error);
  }

  .call-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    margin-top: var(--space-1);
  }

  .call-actions .btn {
    min-height: 40px;
    min-width: 88px;
  }

  .open {
    align-self: flex-start;
    margin-top: var(--space-2);
    font-size: var(--text-sm);
    color: rgb(var(--primary-rgb));
  }

  .missing {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }
</style>
