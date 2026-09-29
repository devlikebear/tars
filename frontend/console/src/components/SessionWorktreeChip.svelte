<script lang="ts">
  // Session worktree chip (#971). Outside a worktree it offers "Isolate"
  // when the session's folder is in a git repository; inside one it shows
  // the branch and how much changed, and its popover applies the changes to
  // the checkout, keeps them on the branch, or discards them.
  import { untrack } from 'svelte'
  import { getSessionWorktree, sessionWorktreeAction, setSessionIsolation } from '../lib/api'
  import type { SessionWorktreeAction, SessionWorktreeView } from '../lib/types'
  import { chatSession } from '../lib/stores/chatSession'
  import { t } from '../i18n'

  const shownFiles = 8

  let sessionId = $derived(chatSession.activeSessionId ?? '')
  // The session record carries the worktree; the view adds status and lease.
  let worktreeKey = $derived(chatSession.activeSession?.worktree?.path ?? '')
  let view = $state<SessionWorktreeView | null>(null)
  let open = $state(false)
  let busy = $state(false)
  let confirmDiscard = $state(false)

  async function load(id: string) {
    if (!id) {
      view = null
      return
    }
    try {
      const next = await getSessionWorktree(id)
      if (id === chatSession.activeSessionId) view = next
    } catch {
      if (id === chatSession.activeSessionId) view = null
    }
  }

  $effect(() => {
    const id = sessionId
    void worktreeKey
    void chatSession.streaming
    untrack(() => {
      open = false
      confirmDiscard = false
      void load(id)
    })
  })

  async function act(action: SessionWorktreeAction) {
    const id = sessionId
    if (!id || busy) return
    busy = true
    try {
      const { result, view: next } = await sessionWorktreeAction(id, action)
      view = next
      open = false
      confirmDiscard = false
      const strings = $t.sessionWorktree
      if (action === 'apply') chatSession.notify(strings.applied(Array.isArray(result.files) ? result.files.length : 0))
      else if (action === 'keep') chatSession.notify(strings.kept(String(result.branch ?? '')))
      else if (action === 'discard') chatSession.notify(strings.discarded)
      else chatSession.notify(strings.isolated(String(result.branch ?? '')))
      await chatSession.refreshActive()
    } catch (err) {
      chatSession.notify($t.sessionWorktree.failed(err instanceof Error ? err.message : String(err)))
    } finally {
      busy = false
    }
  }

  async function toggleAuto(event: Event) {
    const id = sessionId
    if (!id) return
    const on = (event.currentTarget as HTMLInputElement).checked
    try {
      view = await setSessionIsolation(id, on ? '' : 'off')
    } catch (err) {
      chatSession.notify($t.sessionWorktree.failed(err instanceof Error ? err.message : String(err)))
    }
  }

  function shortPath(value: string): string {
    const parts = value.split(/[\\/]/).filter(Boolean)
    return parts.length <= 3 ? value : `…/${parts.slice(-3).join('/')}`
  }

  let files = $derived(view?.status?.files ?? [])
  let disabled = $derived(busy || !!view?.running || chatSession.streaming)
</script>

{#if view?.worktree}
  <div class="wt">
    <button
      type="button"
      class="wt-chip"
      data-testid="worktree-chip"
      title={`${$t.sessionWorktree.chipTitle}: ${view.worktree.branch} · ${files.length > 0 ? $t.sessionWorktree.filesChanged(files.length) : $t.sessionWorktree.noChanges}`}
      aria-expanded={open}
      onclick={() => { open = !open; confirmDiscard = false; if (open && sessionId) void load(sessionId) }}
    >
      <span class="wt-glyph" aria-hidden="true">⑂</span>
      <span class="wt-label">{$t.sessionWorktree.chipLabel}</span>
      {#if files.length > 0}
        <span class="wt-count">· {$t.sessionWorktree.filesShort(files.length)}</span>
      {/if}
    </button>
    {#if open}
      <div class="wt-popover" role="dialog" aria-label={$t.sessionWorktree.chipTitle}>
        <strong class="wt-summary">{files.length > 0 ? $t.sessionWorktree.filesChanged(files.length) : $t.sessionWorktree.noChanges}</strong>
        <dl class="wt-facts">
          <dt>{$t.sessionWorktree.branch}</dt><dd class="mono">{view.worktree.branch}</dd>
          <dt>{$t.sessionWorktree.folder}</dt><dd class="mono" title={view.worktree.path}>{shortPath(view.worktree.path)}</dd>
          <dt>{$t.sessionWorktree.from}</dt><dd class="mono" title={view.worktree.source_dir}>{shortPath(view.worktree.source_dir)}</dd>
        </dl>
        {#if view.worktree.reason && $t.sessionWorktree.reason[view.worktree.reason]}
          <p class="wt-note">{$t.sessionWorktree.reason[view.worktree.reason]}</p>
        {/if}
        {#if files.length > 0}
          <ul class="wt-files">
            {#each files.slice(0, shownFiles) as file}
              <li class="mono">{file}</li>
            {/each}
            {#if files.length > shownFiles}
              <li class="wt-more">{$t.sessionWorktree.moreFiles(files.length - shownFiles)}</li>
            {/if}
          </ul>
        {/if}
        {#if (view.status?.commits ?? 0) > 0}
          <p class="wt-note">{$t.sessionWorktree.commits(view.status?.commits ?? 0)}</p>
        {/if}
        {#if view.running || chatSession.streaming}
          <p class="wt-note">{$t.sessionWorktree.busyRunning}</p>
        {/if}
        <div class="wt-actions">
          <button type="button" class="btn btn-primary btn-sm" disabled={disabled} title={$t.sessionWorktree.applyTitle} onclick={() => void act('apply')}>{$t.sessionWorktree.apply}</button>
          <button type="button" class="btn btn-secondary btn-sm" disabled={disabled} title={$t.sessionWorktree.keepTitle} onclick={() => void act('keep')}>{$t.sessionWorktree.keep}</button>
          {#if confirmDiscard}
            <button type="button" class="btn btn-danger btn-sm" disabled={disabled} onclick={() => void act('discard')}>{$t.sessionWorktree.discardConfirm}</button>
          {:else}
            <button type="button" class="btn btn-ghost btn-sm wt-discard" disabled={disabled} title={$t.sessionWorktree.discardTitle} onclick={() => { confirmDiscard = true }}>{$t.sessionWorktree.discard}</button>
          {/if}
        </div>
        <label class="wt-auto">
          <input type="checkbox" checked={view.isolation !== 'off'} onchange={toggleAuto} />
          <span>{$t.sessionWorktree.auto}</span>
        </label>
      </div>
    {/if}
  </div>
{:else if view?.repo_root}
  <div class="wt">
    <button
      type="button"
      class="btn btn-ghost btn-sm wt-isolate"
      data-testid="worktree-isolate"
      disabled={disabled}
      title={view.lease_holder && view.lease_holder !== sessionId ? `${$t.sessionWorktree.leaseHeld(view.lease_holder_title || view.lease_holder)}. ${$t.sessionWorktree.isolateTitle}` : $t.sessionWorktree.isolateTitle}
      onclick={() => void act('isolate')}
    >
      <span aria-hidden="true">⑂</span> {$t.sessionWorktree.isolate}
    </button>
  </div>
{/if}

<style>
  /* Shrinks with the title row so it never runs under the session actions. */
  /* The popover spans the session header (position: relative there), so
     it never runs past the chat column. */
  .wt {
    display: inline-flex;
    flex: 0 0 auto;
  }

  .wt-chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    min-width: 0;
    max-width: 240px;
    overflow: hidden;
    padding: 2px var(--space-2);
    border: 1px solid var(--info);
    border-radius: var(--radius-sm);
    background: var(--info-muted);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    cursor: pointer;
  }

  .wt-chip:hover {
    color: var(--text-primary);
  }

  .wt-glyph {
    color: var(--info);
  }

  .wt-label {
    color: var(--text-primary);
    white-space: nowrap;
  }

  .wt-count {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-tertiary);
  }

  .wt-summary {
    font-weight: 600;
    color: var(--text-primary);
  }

  .wt-isolate {
    font-size: var(--text-xs);
  }

  .wt-popover {
    position: absolute;
    top: calc(100% - var(--space-1));
    left: var(--space-4);
    right: var(--space-4);
    z-index: 30;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    max-width: 520px;
    padding: var(--space-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
    box-shadow: var(--shadow-md, 0 12px 28px rgba(0, 0, 0, 0.28));
    font-size: var(--text-sm);
  }

  .wt-facts {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 2px var(--space-2);
    margin: 0;
  }

  .wt-facts dt {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .wt-facts dd {
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-xs);
  }

  .wt-note {
    margin: 0;
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .wt-files {
    margin: 0;
    padding: var(--space-2);
    list-style: none;
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    font-size: var(--text-xs);
    max-height: 160px;
    overflow: auto;
  }

  .wt-more {
    color: var(--text-tertiary);
  }

  .wt-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }

  .wt-discard {
    color: var(--error);
  }

  .wt-auto {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }
</style>
