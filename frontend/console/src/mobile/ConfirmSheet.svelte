<script lang="ts">
  // A question the phone asks before something it cannot undo (forget a
  // computer, remove a saved idea). The system's own confirm dialog always
  // answers in English ("Cancel" / "OK") whatever the phone's language, so
  // the phone asks in its own sheet with its own words.
  import { onMount } from 'svelte'
  import { t } from '../i18n'

  interface Props {
    question: string
    // The button that does it, in its own words ("Remove", "Forget").
    confirmLabel: string
    onConfirm: () => void
    onCancel: () => void
  }

  let { question, confirmLabel, onConfirm, onCancel }: Props = $props()

  let cancelButton = $state<HTMLButtonElement | null>(null)
  // The safe answer has the focus, as the system sheet does.
  onMount(() => cancelButton?.focus())

  function onKey(event: KeyboardEvent) {
    if (event.key === 'Escape') onCancel()
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="backdrop" role="presentation" onclick={onCancel}></div>
<div class="sheet" role="alertdialog" aria-modal="true" aria-labelledby="phone-confirm-question" data-testid="phone-confirm">
  <p id="phone-confirm-question">{question}</p>
  <div class="actions">
    <button type="button" class="btn btn-ghost" bind:this={cancelButton} onclick={onCancel} data-testid="phone-confirm-cancel">{$t.phone.confirm.cancel}</button>
    <button type="button" class="btn btn-danger-solid" onclick={onConfirm} data-testid="phone-confirm-go">{confirmLabel}</button>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 20;
    background: rgb(0 0 0 / 0.45);
  }
  .sheet {
    position: fixed;
    left: var(--space-3);
    right: var(--space-3);
    bottom: calc(env(safe-area-inset-bottom) + var(--space-3));
    z-index: 21;
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 480px;
    margin: 0 auto;
    padding: var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--surface);
  }
  p {
    margin: 0;
    font-size: var(--text-md);
    line-height: 1.5;
    overflow-wrap: anywhere;
  }
  .actions {
    display: flex;
    gap: var(--space-2);
  }
  .actions .btn {
    flex: 1;
    min-height: 44px;
  }
  .btn-danger-solid {
    background: var(--error);
    color: var(--surface-base);
    font-weight: 600;
  }
</style>
