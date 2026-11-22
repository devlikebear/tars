<script lang="ts">
  // The result screen's preview (docs/decisions/vibe-coder-identity.md §4,
  // phase 3): the person's finished app, running on this computer and shown
  // in place. The server runs it and stops it when nobody reads it; this
  // component starts it when it appears and keeps reading while it shows.
  import { onDestroy, onMount } from 'svelte'
  import { t } from '../../i18n'
  import { getEasyPreview, startEasyPreview } from '../../lib/api/easy'
  import { previewPollMs, previewReachable, type EasyPreview } from '../../lib/easy'

  let { projectId }: { projectId: string } = $props()

  const reachable = previewReachable(window.location.hostname)

  let preview = $state<EasyPreview | null>(null)
  let requestFailed = $state(false)
  // Bumped by Reload so the frame loads the app again.
  let frameKey = $state(0)
  let timer: ReturnType<typeof setTimeout> | undefined
  let destroyed = false

  function schedule() {
    clearTimeout(timer)
    if (destroyed || !preview) return
    const ms = previewPollMs(preview)
    if (ms !== null) timer = setTimeout(read, ms)
  }

  async function read() {
    try {
      preview = await getEasyPreview(projectId)
      requestFailed = false
    } catch {
      requestFailed = true
    }
    schedule()
  }

  async function start() {
    requestFailed = false
    try {
      preview = await startEasyPreview(projectId)
    } catch {
      requestFailed = true
    }
    schedule()
  }

  onMount(() => {
    if (reachable) start()
  })
  onDestroy(() => {
    destroyed = true
    clearTimeout(timer)
  })
</script>

<section class="result" data-testid="easy-result">
  <div class="head">
    <h2>{$t.easy.result.title}</h2>
    {#if preview?.state === 'running' && preview.url}
      <div class="actions">
        <button type="button" class="btn btn-ghost btn-sm" data-testid="easy-preview-reload" onclick={() => (frameKey += 1)}>
          {$t.easy.result.reload}
        </button>
        <a class="btn btn-ghost btn-sm" href={preview.url} target="_blank" rel="noopener noreferrer">{$t.easy.result.newTab}</a>
      </div>
    {/if}
  </div>

  {#if !reachable}
    <p class="muted" data-testid="easy-preview-elsewhere">{$t.easy.result.elsewhere}</p>
  {:else if preview?.state === 'running' && preview.url}
    {#key frameKey}
      <iframe
        class="frame"
        data-testid="easy-preview-frame"
        title={$t.easy.result.frameTitle}
        src={preview.url}
        sandbox="allow-scripts allow-forms allow-same-origin allow-popups allow-modals"
      ></iframe>
    {/key}
  {:else if preview?.state === 'failed'}
    <p class="error" data-testid="easy-preview-failed">
      {preview.error_code === 'node_missing' ? $t.easy.result.nodeMissing : $t.easy.result.failed}
    </p>
    <button type="button" class="btn btn-secondary btn-sm" onclick={start}>{$t.easy.result.retry}</button>
  {:else if preview?.state === 'stopped'}
    <p class="muted">{$t.easy.result.stopped}</p>
    <button type="button" class="btn btn-secondary btn-sm" data-testid="easy-preview-start" onclick={start}>{$t.easy.result.start}</button>
  {:else if requestFailed}
    <p class="error">{$t.easy.project.actionFailed}</p>
    <button type="button" class="btn btn-secondary btn-sm" onclick={start}>{$t.easy.result.retry}</button>
  {:else}
    <p class="muted" data-testid="easy-preview-starting">{$t.easy.result.starting}</p>
  {/if}
</section>

<style>
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    margin-bottom: var(--space-3);
  }
  h2 {
    font-size: var(--text-lg);
    margin: 0;
  }
  .actions {
    display: flex;
    gap: var(--space-2);
  }
  .frame {
    display: block;
    width: 100%;
    height: 520px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    /* The app paints its own page; white is what an unstyled page expects. */
    background: #fff;
  }
  .muted {
    color: var(--text-secondary);
    margin: 0 0 var(--space-3);
  }
  .error {
    color: var(--error);
    margin: 0 0 var(--space-3);
  }
</style>
