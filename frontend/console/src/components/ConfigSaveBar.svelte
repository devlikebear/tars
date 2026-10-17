<script lang="ts">
  // Sticky save bar (#1238): the single place a restart notice appears now
  // ("requires restart" used to repeat on every one of 181 field cards).
  // It also carries the post-"Save and apply" restart confirmation
  // (restartConfirm) — the dedicated restart button only lives on the
  // Advanced tab, but a save can happen from any tab, so the confirmation
  // has to be visible everywhere too; this bar already is.
  import { t } from '../i18n'

  interface Props {
    changedCount: number
    saving?: boolean
    restartConfirm?: boolean
    restartBusy?: boolean
    onDiscard: () => void
    onSave: () => void
    onSaveAndRestart: () => void
    onViewChanges: () => void
    onConfirmRestart: () => void
  }

  let {
    changedCount,
    saving = false,
    restartConfirm = false,
    restartBusy = false,
    onDiscard,
    onSave,
    onSaveAndRestart,
    onViewChanges,
    onConfirmRestart,
  }: Props = $props()
</script>

{#if restartConfirm}
  <div class="config-save-bar" role="region" aria-label={$t.config.savedPending}>
    <span class="config-save-bar-notice">{$t.config.savedPending}</span>
    <div class="config-save-bar-actions">
      <button type="button" class="btn btn-primary btn-sm" disabled={restartBusy} onclick={onConfirmRestart}>
        {restartBusy ? $t.config.restart.restarting : $t.config.confirmRestart}
      </button>
    </div>
  </div>
{:else if changedCount > 0}
  <div class="config-save-bar" role="region" aria-label={$t.config.saveBar.changedCount(changedCount)}>
    <button type="button" class="config-save-bar-summary" onclick={onViewChanges}>
      {$t.config.saveBar.changedCount(changedCount)}
    </button>
    <span class="config-save-bar-notice">{$t.config.saveBar.restartNotice}</span>
    <div class="config-save-bar-actions">
      <button type="button" class="btn btn-ghost btn-sm" onclick={onDiscard} disabled={saving}>{$t.config.saveBar.discard}</button>
      <button type="button" class="btn btn-secondary btn-sm" onclick={onSaveAndRestart} disabled={saving}>{$t.config.saveApply}</button>
      <button type="button" class="btn btn-primary btn-sm" onclick={onSave} disabled={saving}>
        {saving ? $t.config.saveBar.saving : $t.config.saveBar.save}
      </button>
    </div>
  </div>
{/if}

<style>
  .config-save-bar {
    position: sticky;
    bottom: 0;
    z-index: 5;
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-wrap: wrap;
    padding: var(--space-3) var(--space-4);
    /* CASE (CompanionPet.svelte) floats fixed at bottom:var(--space-5)
       right:var(--space-5), a 74x86px button, on every route — settings
       included, unlike the chat route which shifts CASE aside for its own
       rail. Reserve that corner so this bar's own right-aligned buttons are
       never the thing CASE sits on top of and swallows the click for. */
    padding-right: calc(var(--case-button-width) + var(--space-5) + var(--space-3));
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: var(--surface-elevated);
    box-shadow: 0 -4px 16px rgba(0, 0, 0, 0.28);
  }

  .config-save-bar-summary {
    background: none;
    border: none;
    padding: 0;
    color: var(--primary);
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 600;
    cursor: pointer;
    text-decoration: underline;
  }

  .config-save-bar-notice {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    flex: 1;
    min-width: 0;
  }

  .config-save-bar-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-left: auto;
  }

  @media (max-width: 640px) {
    .config-save-bar {
      flex-direction: column;
      align-items: stretch;
    }

    .config-save-bar-actions {
      margin-left: 0;
      justify-content: flex-end;
    }
  }
</style>
