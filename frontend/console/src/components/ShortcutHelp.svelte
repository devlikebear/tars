<script lang="ts">
  // `?` overlay listing the global shortcuts (#968).
  import { tick } from 'svelte'
  import { t } from '../i18n'
  import { formatShortcutKeys, shortcutDocs } from '../lib/shortcuts'

  interface Props {
    onClose: () => void
  }

  let { onClose }: Props = $props()
  let closeEl: HTMLButtonElement | undefined = $state()

  $effect(() => {
    void tick().then(() => closeEl?.focus())
  })

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' || event.key === '?') {
      event.preventDefault()
      onClose()
    }
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="help-backdrop" onclick={onClose}>
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div
    class="help"
    role="dialog"
    aria-modal="true"
    aria-labelledby="shortcut-help-title"
    tabindex="-1"
    onclick={(event) => event.stopPropagation()}
    onkeydown={handleKeydown}
  >
    <header class="help-header">
      <h2 id="shortcut-help-title">{$t.shortcuts.title}</h2>
      <button bind:this={closeEl} type="button" class="btn btn-ghost btn-sm" onclick={onClose}>{$t.shortcuts.close}</button>
    </header>
    <dl class="help-list">
      {#each shortcutDocs as doc (doc.action)}
        <div class="help-row">
          <dt>{$t.shortcuts.actions[doc.action]}</dt>
          <dd><kbd>{formatShortcutKeys(doc.keys)}</kbd></dd>
        </div>
      {/each}
    </dl>
    <p class="help-note">{$t.shortcuts.note}</p>
  </div>
</div>

<style>
  .help-backdrop {
    position: fixed;
    inset: 0;
    z-index: 200;
    display: grid;
    place-items: center;
    padding: var(--space-4);
    background: rgba(0, 0, 0, 0.45);
  }

  .help {
    width: min(480px, 100%);
    background: var(--surface-elevated);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-lg);
    padding: var(--space-4);
  }

  .help-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--space-3);
  }

  .help-header h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--text-lg);
    font-weight: 500;
  }

  .help-list {
    margin: 0;
    display: grid;
    gap: var(--space-2);
  }

  .help-row {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
    font-size: var(--text-sm);
  }

  .help-row dt {
    color: var(--text-primary);
  }

  .help-row dd {
    margin: 0;
  }

  kbd {
    padding: 1px var(--space-2);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .help-note {
    margin: var(--space-4) 0 0;
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    line-height: 1.5;
  }
</style>
