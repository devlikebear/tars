<script lang="ts">
  // On an iPhone, a pipeline's page offers to open the same pipeline in the
  // TARS app (docs/decisions/mobile-remote.md §7). A Telegram alert links to
  // the console because Telegram makes only http(s) links tappable; this line
  // is the step from there into the app. In flow, never over the page, so it
  // covers no button; closing it keeps the browser.
  import { t } from '../i18n'

  interface Props {
    link: string
    // Top of the page (the login screen): clear the status bar.
    top?: boolean
    onDismiss: () => void
  }

  let { link, top = false, onDismiss }: Props = $props()
</script>

<div class="app-handoff" class:top role="status" data-testid="app-handoff">
  <span>{$t.phones.handoffText}</span>
  <a class="btn btn-primary btn-sm" href={link}>{$t.phones.handoffOpen}</a>
  <button type="button" class="app-handoff-dismiss" aria-label={$t.phones.handoffDismissAria} onclick={onDismiss}>×</button>
</div>

<style>
  .app-handoff {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-4);
    background: rgba(var(--primary-rgb), 0.12);
    border-bottom: 1px solid var(--primary);
    color: var(--primary-text);
    font-size: 13px;
  }

  .app-handoff.top {
    padding-top: calc(env(safe-area-inset-top) + var(--space-2));
  }

  .app-handoff span {
    flex: 1;
    min-width: 0;
  }

  .app-handoff a {
    flex: none;
    min-height: 36px;
    display: inline-flex;
    align-items: center;
  }

  .app-handoff-dismiss {
    flex: none;
    min-width: 36px;
    min-height: 36px;
    background: none;
    border: none;
    color: var(--text-secondary);
    font-size: 18px;
    line-height: 1;
    cursor: pointer;
  }

  .app-handoff-dismiss:hover {
    color: var(--text-primary);
  }
</style>
