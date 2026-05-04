<script lang="ts">
  import { t } from '../../i18n'

  type Phase = 'patching' | 'restarting' | 'polling' | 'ready' | 'timeout'

  interface Props {
    phase: Phase
  }
  let { phase }: Props = $props()
</script>

<section class="card onboarding-restart">
  {#if phase === 'patching'}
    <h2>{$t.onboarding.restart.patchingTitle}</h2>
    <p>{$t.onboarding.restart.patchingBody}</p>
  {:else if phase === 'restarting'}
    <h2>{$t.onboarding.restart.restartingTitle}</h2>
    <p>{$t.onboarding.restart.restartingBody}</p>
  {:else if phase === 'polling'}
    <h2>{$t.onboarding.restart.pollingTitle}</h2>
    <p>{$t.onboarding.restart.pollingBody}</p>
    <div class="onboarding-spinner" aria-hidden="true"></div>
  {:else if phase === 'ready'}
    <h2>{$t.onboarding.restart.readyTitle}</h2>
    <p>{$t.onboarding.restart.readyBody}</p>
  {:else if phase === 'timeout'}
    <h2>{$t.onboarding.restart.timeoutTitle}</h2>
    <p>{$t.onboarding.restart.timeoutBody}</p>
    <button class="btn btn-primary" type="button" onclick={() => window.location.reload()}>{$t.onboarding.restart.refreshButton}</button>
  {/if}
</section>

<style>
  .onboarding-restart {
    text-align: center;
    padding: var(--space-6);
  }
  .onboarding-spinner {
    width: 40px;
    height: 40px;
    border-radius: 50%;
    border: 3px solid var(--border-soft);
    border-top-color: var(--primary);
    margin: var(--space-4) auto 0;
    animation: onboarding-spin 1s linear infinite;
  }
  @keyframes onboarding-spin {
    to { transform: rotate(360deg); }
  }
</style>
