<script lang="ts">
  // The idea inbox (docs/decisions/mobile-remote.md §6): write an idea down
  // now, kept on this phone, and send it to a desktop as a focus task when
  // ready. Nothing here needs a desktop until the send.
  import { t } from '../i18n'
  import type { IdeaStore } from '../lib/mobile/ideaStore.svelte'
  import { sortedIdeas, type Idea } from '../lib/mobile/ideas'

  interface Props {
    ideas: IdeaStore
    onSend: (idea: Idea) => void
    onOpen: (desktopID: string, sessionID: string) => void
    // Whether the desktop an idea went to is still paired.
    paired: (desktopID: string) => boolean
  }

  let { ideas, onSend, onOpen, paired }: Props = $props()

  let draft = $state('')
  let editing = $state<string | null>(null)
  let editText = $state('')

  async function save(e: SubmitEvent) {
    e.preventDefault()
    if (await ideas.add(draft)) draft = ''
  }

  function startEdit(idea: Idea) {
    editing = idea.id
    editText = idea.text
  }

  async function saveEdit() {
    if (!editing) return
    await ideas.edit(editing, editText)
    editing = null
  }

  async function remove(idea: Idea) {
    if (!confirm($t.mobileApp.confirmDeleteIdea)) return
    await ideas.remove(idea.id)
  }

  function when(at: string): string {
    const d = new Date(at)
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  }
</script>

<section class="ideas" aria-label={$t.mobileApp.tabs.ideas}>
  <form class="composer" onsubmit={save}>
    <textarea rows="3" placeholder={$t.mobileApp.ideaPlaceholder} bind:value={draft} data-testid="mobile-idea-input"></textarea>
    <button type="submit" class="btn btn-primary" disabled={!draft.trim()} data-testid="mobile-idea-save">{$t.mobileApp.saveIdea}</button>
  </form>

  {#if ideas.loaded && ideas.ideas.length === 0}
    <p class="empty">{$t.mobileApp.ideasEmpty}</p>
  {/if}

  {#each sortedIdeas(ideas.ideas) as idea (idea.id)}
    <article class="idea" class:sent={!!idea.sent} data-testid="mobile-idea" data-sent={idea.sent ? 'true' : 'false'}>
      {#if editing === idea.id}
        <textarea rows="4" bind:value={editText} data-testid="mobile-idea-edit"></textarea>
        <div class="idea-actions">
          <button type="button" class="btn btn-ghost btn-sm" onclick={() => (editing = null)}>{$t.mobileApp.cancel}</button>
          <button type="button" class="btn btn-primary btn-sm" disabled={!editText.trim()} onclick={saveEdit}>{$t.mobileApp.saveIdea}</button>
        </div>
      {:else}
        <p class="text">{idea.text}</p>
        <span class="meta">
          {#if idea.sent}
            {$t.mobileApp.sentTo(idea.sent.desktopName, when(idea.sent.at))}
          {:else}
            {when(idea.createdAt)}
          {/if}
        </span>
        <div class="idea-actions">
          <button type="button" class="btn btn-ghost btn-sm" onclick={() => remove(idea)}>{$t.mobileApp.deleteIdea}</button>
          {#if idea.sent}
            {#if paired(idea.sent.desktopID)}
              <button type="button" class="btn btn-secondary btn-sm" onclick={() => onOpen(idea.sent!.desktopID, idea.sent!.sessionID)} data-testid="mobile-idea-open">{$t.mobileApp.open}</button>
            {/if}
          {:else}
            <button type="button" class="btn btn-ghost btn-sm" onclick={() => startEdit(idea)}>{$t.mobileApp.editIdea}</button>
            <button type="button" class="btn btn-primary btn-sm" onclick={() => onSend(idea)} data-testid="mobile-idea-send">{$t.mobileApp.sendIdea}</button>
          {/if}
        </div>
      {/if}
    </article>
  {/each}
</section>

<style>
  .ideas {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .composer {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  textarea {
    font: inherit;
    font-size: 17px; /* 16px or more keeps iOS from zooming into the field */
    line-height: 1.45;
    color: var(--text-primary);
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    padding: 12px;
    resize: vertical;
  }

  .composer .btn {
    align-self: flex-end;
    min-height: 40px;
    min-width: 96px;
  }

  .empty {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--text-secondary);
    line-height: 1.5;
  }

  .idea {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--surface);
    padding: var(--space-4);
  }

  .idea.sent {
    opacity: 0.75;
  }

  .text {
    margin: 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    color: var(--text-primary);
    font-size: 16px;
    line-height: 1.45;
  }

  .meta {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .idea-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }

  .idea-actions .btn {
    min-height: 40px;
  }
</style>
