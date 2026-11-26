<script lang="ts">
  // The + New tab (docs/plans/vibe-mode-redesign unit 8, screens/M-New):
  // the idea, the desktop it goes to, and one button that hands it to the
  // team. With no desktop that can take it, the same button keeps the idea
  // on this phone (the Ideas tab folded in), and the saved ideas wait below
  // until a desk is on.
  import { t } from '../i18n'
  import type { PairedDesktop } from '../lib/mobile/desktops'
  import type { EasyReadiness } from '../lib/easy'
  import type { IdeaStore } from '../lib/mobile/ideaStore.svelte'
  import type { ProjectStore } from '../lib/mobile/projectStore.svelte'
  import { ideaTitle, sortedIdeas, type Idea } from '../lib/mobile/ideas'
  import { chosenDesk, deskChoices } from '../lib/mobile/projects'
  import ConfirmSheet from './ConfirmSheet.svelte'

  interface Props {
    projects: ProjectStore
    ideas: IdeaStore
    desktops: PairedDesktop[]
    // Which desktops answer now.
    online: (desktop: PairedDesktop) => boolean
    onOpen: (desktopID: string, projectID: string) => void
    onNotice: (text: string) => void
  }

  let { projects, ideas, desktops, online, onOpen, onNotice }: Props = $props()

  const readiness = (d: PairedDesktop): EasyReadiness | null | undefined => projects.readiness[d.id]
  // Every paired computer that answers; the idea goes to the one picked,
  // else the first that can take it.
  const choices = $derived(deskChoices(desktops, online, readiness))
  let chosenID = $state('')
  const chosen = $derived(chosenDesk(choices, chosenID))
  const onlineButNotReady = $derived(!chosen && choices.length > 0)
  let choosing = $state(false)
  function pick(id: string) {
    chosenID = id
    choosing = false
  }

  let idea = $state('')
  let sending = $state(false)
  let error = $state('')

  function failureText(code: string): string {
    if (code === 'unpaired') return $t.phone.computers.unpairedHint
    if (code === 'unreachable') return $t.phone.failures.unreachable
    return $t.easy.errors[code] ?? $t.easy.errors.unknown
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    if (sending) return
    error = ''
    if (!idea.trim()) {
      error = $t.easy.errors.idea_required
      return
    }
    sending = true
    try {
      if (!chosen) {
        await ideas.add(idea)
        idea = ''
        onNotice($t.phone.new.saved)
        return
      }
      const answer = await projects.create(chosen, idea)
      if ('failure' in answer) {
        error = failureText(answer.failure)
        return
      }
      idea = ''
      onOpen(chosen.id, answer.project.id)
    } finally {
      sending = false
    }
  }

  const saved = $derived(sortedIdeas(ideas.ideas))

  async function send(item: Idea) {
    if (!chosen) return
    const sent = await ideas.send(chosen, item.id)
    if (sent?.projectID) onOpen(sent.desktopID, sent.projectID)
  }

  // Removing a saved idea asks first, in the phone's own sheet.
  let removing = $state<Idea | null>(null)
  async function remove() {
    const item = removing
    removing = null
    if (item) await ideas.remove(item.id)
  }

  function sentProject(item: Idea): { desktopID: string; projectID: string } | null {
    const s = item.sent
    if (!s?.projectID || !desktops.some((d) => d.id === s.desktopID)) return null
    return { desktopID: s.desktopID, projectID: s.projectID }
  }
</script>

<section class="new" aria-labelledby="phone-new-title">
  <form onsubmit={submit} class="idea-form">
    <h1 id="phone-new-title">{$t.easy.home.title}</h1>
    <p class="sub">{$t.easy.home.subtitle}</p>
    <label class="sr-only" for="phone-idea">{$t.easy.home.title}</label>
    <textarea id="phone-idea" rows="4" data-testid="phone-idea" placeholder={$t.easy.home.placeholder} bind:value={idea} disabled={sending}></textarea>

    {#if chosen}
      <div class="desk" data-testid="phone-new-desk">
        <span class="dot" aria-hidden="true"></span>
        <span class="desk-text">
          <b>{$t.phone.new.deskLabel} {chosen.name}</b>
          <span>{$t.phone.new.deskReady}</span>
        </span>
        {#if choices.length > 1}
          <button type="button" class="btn btn-ghost btn-sm" aria-expanded={choosing} onclick={() => (choosing = !choosing)} data-testid="phone-new-desk-change">{$t.phone.new.change}</button>
        {/if}
      </div>
      {#if choosing}
        <div class="choices" role="radiogroup" aria-label={$t.phone.new.choose} data-testid="phone-new-desk-choose">
          {#each choices as c (c.desktop.id)}
            <button
              type="button"
              role="radio"
              class="choice"
              aria-checked={c.desktop.id === chosen.id}
              disabled={!c.ready}
              onclick={() => pick(c.desktop.id)}
              data-testid="phone-new-desk-option"
              data-desktop={c.desktop.name}
            >
              <span class="choice-name">{c.desktop.name}</span>
              <span class="choice-state">{c.ready ? $t.phone.new.deskReady : $t.phone.new.notReady}</span>
            </button>
          {/each}
        </div>
      {/if}
    {:else}
      <p class="hint" data-testid="phone-new-nodesk">{onlineButNotReady ? $t.phone.new.deskNotReady : $t.phone.new.noDesk}</p>
    {/if}

    <h2>{$t.easy.home.examplesTitle}</h2>
    <div class="examples">
      {#each $t.easy.home.examples as example (example.idea)}
        <button type="button" class="example" onclick={() => (idea = example.idea)} data-testid="phone-example">{example.idea}</button>
      {/each}
    </div>

    {#if error}<p class="error" role="alert" data-testid="phone-new-error">{error}</p>{/if}
    <button type="submit" class="btn btn-primary go" data-testid="phone-new-submit" disabled={sending}>
      {sending ? $t.easy.home.starting : chosen ? $t.easy.home.start : $t.phone.new.save}
    </button>
    <p class="scope">{$t.easy.home.scope}</p>
  </form>

  {#if saved.length > 0}
    <div class="saved" data-testid="phone-saved-ideas">
      <h2>{$t.phone.new.savedTitle}</h2>
      {#each saved as item (item.id)}
        {@const opened = sentProject(item)}
        <article class="saved-idea" data-testid="phone-saved-idea" data-sent={item.sent ? 'true' : 'false'}>
          <span class="saved-text">{ideaTitle(item)}</span>
          {#if item.sent}
            <span class="saved-meta">{$t.phone.new.sentTo(item.sent.desktopName)}</span>
          {/if}
          <div class="saved-actions">
            {#if opened}
              <button type="button" class="btn btn-secondary btn-sm" onclick={() => onOpen(opened.desktopID, opened.projectID)}>{$t.phone.new.open}</button>
            {:else if !item.sent && chosen}
              <button type="button" class="btn btn-primary btn-sm" data-testid="phone-saved-send" disabled={ideas.sending !== ''} onclick={() => send(item)}>
                {ideas.sending === item.id ? $t.phone.new.sending : $t.phone.new.send(chosen.name)}
              </button>
            {/if}
            <button type="button" class="btn btn-ghost btn-sm" onclick={() => (removing = item)}>{$t.phone.new.remove}</button>
          </div>
        </article>
      {/each}
      {#if ideas.sendFailure}<p class="error" role="alert">{failureText(ideas.sendFailure)}</p>{/if}
    </div>
  {/if}
</section>

{#if removing}
  <ConfirmSheet question={$t.phone.new.confirmRemove} confirmLabel={$t.phone.new.confirmRemoveGo} onConfirm={() => void remove()} onCancel={() => (removing = null)} />
{/if}

<style>
  .new {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }
  .idea-form {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--text-xl);
  }
  h2 {
    margin: var(--space-2) 0 0;
    font-size: var(--text-xs);
    font-weight: 600;
    color: var(--text-tertiary);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .sub,
  .hint,
  .scope {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
    line-height: 1.5;
  }
  .scope {
    text-align: center;
    font-size: var(--text-xs);
  }
  textarea {
    width: 100%;
    box-sizing: border-box;
    padding: var(--space-3);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    color: var(--text-primary);
    font: inherit;
    font-size: 16px;
    resize: vertical;
  }
  .desk {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--success);
    flex: none;
  }
  .desk-text {
    flex: 1;
    display: flex;
    flex-direction: column;
    font-size: var(--text-sm);
  }
  .desk-text span {
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }
  .choices {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .choice {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    min-height: 44px;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--text-sm);
    text-align: left;
  }
  .choice[aria-checked='true'] {
    border-color: var(--primary);
  }
  .choice:disabled {
    opacity: 0.55;
  }
  .choice-state {
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }
  .examples {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .example {
    text-align: left;
    min-height: 44px;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--text-sm);
  }
  .go {
    width: 100%;
    min-height: 48px;
    font-size: var(--text-md);
  }
  .error {
    margin: 0;
    color: var(--error);
    font-size: var(--text-sm);
  }
  .saved {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .saved-idea {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
  }
  .saved-text {
    overflow-wrap: anywhere;
  }
  .saved-meta {
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }
  .saved-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
  }
</style>
