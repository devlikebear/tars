<script lang="ts">
  // Sending an idea as a focus task (docs/decisions/mobile-remote.md §6):
  // a desktop that answers, a folder its owner registered for phones, a
  // template, and the goal (the idea, editable). The desk drafts the plan and
  // stops at G1; goal mode is never turned on from here.
  import { t } from '../i18n'
  import { templateText } from '../lib/focus'
  import type { IdeaStore } from '../lib/mobile/ideaStore.svelte'
  import type { Idea, SentIdea } from '../lib/mobile/ideas'
  import type { PairedDesktop } from '../lib/mobile/desktops'
  import type { FocusTemplate } from '../lib/types'

  interface Props {
    ideas: IdeaStore
    idea: Idea
    // The desktops that answer now.
    desktops: PairedDesktop[]
    onSent: (sent: SentIdea) => void
    onCancel: () => void
  }

  let { ideas, idea, desktops, onSent, onCancel }: Props = $props()

  // svelte-ignore state_referenced_locally
  let goal = $state(idea.text)
  let desktopID = $state('')
  let cwd = $state('')
  let template = $state('')

  const desktop = $derived(desktops.find((d) => d.id === desktopID) ?? null)
  const options = $derived(ideas.options && ideas.options.desktopID === desktopID ? ideas.options : null)

  function pick(id: string) {
    desktopID = id
    cwd = ''
    template = ''
    const d = desktops.find((x) => x.id === id)
    if (d) void ideas.prepare(d)
  }

  // One desktop: nothing to choose.
  $effect(() => {
    if (!desktopID && desktops.length === 1) pick(desktops[0].id)
  })

  // One folder: nothing to choose either.
  $effect(() => {
    if (options && !cwd && options.folders.length === 1) cwd = options.folders[0].path
  })

  function templateName(tpl: FocusTemplate): string {
    return (tpl.builtin ? templateText(tpl.id, $t.focus.templates)?.name : '') || tpl.name || tpl.id
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    if (!desktop) return
    const sent = await ideas.send(desktop, idea.id, goal, cwd, template)
    if (sent) onSent(sent)
  }
</script>

<div class="sheet" role="dialog" aria-modal="true" aria-label={$t.mobileApp.sendTitle} data-testid="mobile-send-sheet">
  <form onsubmit={submit}>
    <h2>{$t.mobileApp.sendTitle}</h2>
    <p class="hint">{$t.mobileApp.sendHint}</p>

    <label>
      <span>{$t.mobileApp.goalLabel}</span>
      <textarea rows="5" bind:value={goal} data-testid="mobile-send-goal"></textarea>
    </label>

    <fieldset>
      <legend>{$t.mobileApp.desktopLabel}</legend>
      {#if desktops.length === 0}
        <p class="hint" data-testid="mobile-send-no-desktop">{$t.mobileApp.noDesktopOnline}</p>
      {/if}
      {#each desktops as d (d.id)}
        <label class="choice">
          <input type="radio" name="desktop" value={d.id} checked={desktopID === d.id} onchange={() => pick(d.id)} data-testid="mobile-send-desktop" />
          <span>{d.name}</span>
        </label>
      {/each}
    </fieldset>

    {#if desktop}
      {#if ideas.loadingOptions}
        <p class="hint">{$t.mobileApp.loadingFolders}</p>
      {:else if ideas.optionsFailure}
        <p class="error" role="alert">{$t.mobileApp.sendFailures[ideas.optionsFailure]}</p>
      {:else if options}
        <fieldset>
          <legend>{$t.mobileApp.folderLabel}</legend>
          {#if options.folders.length === 0}
            <p class="hint" data-testid="mobile-send-no-folder">{$t.mobileApp.noFolders}</p>
          {/if}
          {#each options.folders as folder (folder.path)}
            <label class="choice">
              <input type="radio" name="folder" value={folder.path} bind:group={cwd} data-testid="mobile-send-folder" />
              <span class="folder">
                <strong>{folder.name}</strong>
                <small class="mono">{folder.path}</small>
              </span>
            </label>
          {/each}
        </fieldset>

        {#if options.templates.length > 1}
          <label>
            <span>{$t.mobileApp.templateLabel}</span>
            <select bind:value={template} data-testid="mobile-send-template">
              <option value="">{templateName(options.templates.find((tpl) => tpl.id === 'dev') ?? options.templates[0])}</option>
              {#each options.templates.filter((tpl) => tpl.id !== 'dev') as tpl (tpl.id)}
                <option value={tpl.id}>{templateName(tpl)}</option>
              {/each}
            </select>
          </label>
        {/if}
      {/if}
    {/if}

    {#if ideas.sendFailure}
      <p class="error" role="alert">{$t.mobileApp.sendFailures[ideas.sendFailure]}</p>
    {/if}

    <div class="actions">
      <button type="submit" class="btn btn-primary" disabled={ideas.sending || !desktop || !cwd || !goal.trim()} data-testid="mobile-send-submit">
        {ideas.sending ? $t.mobileApp.sending : $t.mobileApp.send}
      </button>
      <button type="button" class="btn btn-ghost" onclick={onCancel}>{$t.mobileApp.cancel}</button>
    </div>
  </form>
</div>

<style>
  .sheet {
    position: fixed;
    inset: 0;
    z-index: 10;
    background: var(--surface-base);
    overflow-y: auto;
    padding: calc(env(safe-area-inset-top) + var(--space-5)) var(--space-4) calc(env(safe-area-inset-bottom) + var(--space-5));
  }

  form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 520px;
    margin: 0 auto;
  }

  h2 {
    margin: 0;
    font-size: var(--text-xl);
  }

  .hint {
    color: var(--text-secondary);
    font-size: var(--text-sm);
    margin: 0;
    line-height: 1.5;
  }

  label,
  fieldset {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  fieldset {
    border: 0;
    margin: 0;
    padding: 0;
  }

  legend {
    padding: 0;
    margin-bottom: var(--space-2);
  }

  textarea,
  select {
    font: inherit;
    font-size: 17px; /* 16px or more keeps iOS from zooming into the field */
    color: var(--text-primary);
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    padding: 12px;
  }

  textarea {
    resize: vertical;
    line-height: 1.45;
  }

  .choice {
    flex-direction: row;
    align-items: center;
    gap: var(--space-3);
    min-height: 44px;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
    color: var(--text-primary);
    font-size: 16px;
  }

  .choice input {
    width: 20px;
    height: 20px;
    flex: none;
    accent-color: rgb(var(--primary-rgb));
  }

  .folder {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .folder small {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    overflow-wrap: anywhere;
  }

  .error {
    margin: 0;
    color: var(--error);
    background: var(--error-muted);
    border-radius: var(--radius-md);
    padding: var(--space-3);
    font-size: var(--text-sm);
  }

  .actions {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .actions .btn {
    min-height: 44px;
    font-size: 16px;
  }
</style>
