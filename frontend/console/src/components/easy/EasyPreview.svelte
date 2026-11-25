<script lang="ts">
  // The result screen's preview (docs/decisions/vibe-coder-identity.md §4,
  // phase 3): the person's finished app, running on this computer and shown
  // in place. The server runs it and stops it when nobody reads it; this
  // component starts it when it appears and keeps reading while it shows.
  import { onDestroy, onMount } from 'svelte'
  import { t } from '../../i18n'
  import { getEasyPreview, startEasyPreview } from '../../lib/api/easy'
  import { previewPollMs, previewReachable, type EasyPreview } from '../../lib/easy'
  import { consoleMode } from '../../lib/stores/consoleMode.svelte'

  let { projectId, version }: { projectId: string; version?: number } = $props()

  const reachable = previewReachable(window.location.hostname)

  let preview = $state<EasyPreview | null>(null)
  let requestFailed = $state(false)
  // Bumped by Reload so the frame loads the app again.
  let frameKey = $state(0)
  // The width the app is shown at: the column's, or a phone's.
  let width = $state<'pc' | 'phone'>('pc')
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
    {#if version}<span class="version" data-testid="easy-result-version">{$t.easy.result.version(version)}</span>{/if}
    {#if preview?.state === 'running' && preview.url}
      <div class="actions">
        <div class="widths" role="group" aria-label={$t.easy.result.widthLabel}>
          {#each ['pc', 'phone'] as const as w (w)}
            <button type="button" class:on={width === w} aria-pressed={width === w} data-testid="easy-preview-width-{w}" onclick={() => (width = w)}>
              {$t.easy.result.width[w]}
            </button>
          {/each}
        </div>
        <button type="button" class="btn btn-ghost btn-sm" data-testid="easy-preview-reload" onclick={() => (frameKey += 1)}>
          {$t.easy.result.reload}
        </button>
        <a class="btn btn-ghost btn-sm" data-testid="easy-preview-open" href={preview.url} target="_blank" rel="noopener noreferrer">{$t.easy.result.newTab}</a>
      </div>
    {/if}
  </div>

  {#if !reachable}
    <p class="muted" data-testid="easy-preview-elsewhere">{$t.easy.result.elsewhere}</p>
  {:else if preview?.state === 'running' && preview.url}
    {#key frameKey}
      <iframe
        class="frame"
        class:phone={width === 'phone'}
        data-width={width}
        data-testid="easy-preview-frame"
        title={$t.easy.result.frameTitle}
        src={preview.url}
        sandbox="allow-scripts allow-forms allow-same-origin allow-popups allow-modals"
      ></iframe>
    {/key}
  {:else if preview?.state === 'failed'}
    <p class="error" data-testid="easy-preview-failed">
      {preview.error_code === 'node_missing' ? $t.easy.result.nodeMissing : consoleMode.expert ? $t.easy.result.failed : $t.easy.result.failedAlone}
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
    flex-wrap: wrap;
    gap: var(--space-3);
    margin-bottom: var(--space-3);
  }
  h2 {
    font-size: var(--text-xl);
    font-weight: 600;
    margin: 0;
  }
  .version {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    border: 1px solid var(--border-strong);
    border-radius: 999px;
    padding: 2px var(--space-3);
  }
  .actions {
    margin-left: auto;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .widths {
    display: flex;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }
  .widths button {
    font: inherit;
    font-size: var(--text-sm);
    color: var(--text-secondary);
    background: transparent;
    border: 0;
    padding: var(--space-1) var(--space-3);
    cursor: pointer;
  }
  .widths button + button {
    border-left: 1px solid var(--border-default);
  }
  .widths button.on {
    color: var(--text-primary);
    background: var(--surface-active);
  }
  .frame {
    display: block;
    width: 100%;
    margin: 0 auto;
    height: 560px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    /* The app paints its own page; white is what an unstyled page expects. */
    background: #fff;
  }
  .frame.phone {
    width: 390px;
    max-width: 100%;
    height: 720px;
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
