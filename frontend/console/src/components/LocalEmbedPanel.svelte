<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { t } from '../i18n'
  import { getLocalEmbed, restartServer, runLocalEmbedAction } from '../lib/api'
  import type { LocalEmbedAction, LocalEmbedState } from '../lib/types'
  import { awaitingServer, canInstall, canReindex, foreignServer, formatBytes, jobPercent, jobRunning } from '../lib/localEmbed'

  const POLL_MS = 1500
  // A sidecar that never comes up should not be polled for as long as the tab
  // stays open.
  const MAX_SERVER_POLLS = 40

  let embed = $state<LocalEmbedState | null>(null)
  let error = $state('')
  let busy = $state(false)
  let confirming = $state<'remove' | 'remove-all' | 'restart' | ''>('')
  let restarting = $state(false)
  let timer: ReturnType<typeof setTimeout> | null = null
  let serverPolls = 0
  let destroyed = false

  async function refresh() {
    try {
      embed = await getLocalEmbed()
      error = ''
    } catch (err) {
      error = $t.localEmbed.load(err instanceof Error ? err.message : String(err))
    }
    schedule()
  }

  // Only a running job or a loading model changes on its own; polling stops
  // when that settles.
  function schedule() {
    if (timer) clearTimeout(timer)
    timer = null
    if (destroyed) return
    const waiting = awaitingServer(embed) && serverPolls < MAX_SERVER_POLLS
    serverPolls = waiting ? serverPolls + 1 : 0
    if (jobRunning(embed) || waiting) timer = setTimeout(refresh, POLL_MS)
  }

  async function act(action: LocalEmbedAction) {
    busy = true
    error = ''
    confirming = ''
    try {
      if (action === 'cancel') {
        await runLocalEmbedAction(action)
        await refresh()
      } else {
        embed = await runLocalEmbedAction(action)
        schedule()
      }
    } catch (err) {
      error = $t.localEmbed.actionFailed(err instanceof Error ? err.message : String(err))
    } finally {
      busy = false
    }
  }

  function confirmThen(kind: 'remove' | 'remove-all') {
    if (confirming !== kind) {
      confirming = kind
      return
    }
    void act(kind)
  }

  async function restart() {
    if (confirming !== 'restart') {
      confirming = 'restart'
      return
    }
    confirming = ''
    restarting = true
    error = ''
    try {
      await restartServer()
      for (let attempt = 0; attempt < 30 && !destroyed; attempt++) {
        await new Promise((resolve) => setTimeout(resolve, 1000))
        try {
          const next = await getLocalEmbed(AbortSignal.timeout(2000))
          if (!next.restart_required) {
            embed = next
            schedule()
            return
          }
        } catch {
          // The server is on its way back.
        }
      }
      error = $t.localEmbed.restartFailed
    } catch (err) {
      error = $t.localEmbed.actionFailed(err instanceof Error ? err.message : String(err))
    } finally {
      restarting = false
    }
  }

  onMount(refresh)
  onDestroy(() => {
    destroyed = true
    if (timer) clearTimeout(timer)
  })

  let status = $derived(embed?.status)
  let job = $derived(embed?.job)
  let running = $derived(jobRunning(embed))
  let percent = $derived(jobPercent(embed))
  let foreign = $derived(foreignServer(embed))
  let disabled = $derived(busy || restarting || running)
</script>

<section class="card local-embed">
  <div class="le-head">
    <div>
      <h3>{$t.localEmbed.title}</h3>
      <div class="le-subtitle">{$t.localEmbed.subtitle}</div>
    </div>
    {#if embed}
      <button
        class={embed.enabled ? 'btn btn-secondary' : 'btn btn-primary'}
        type="button"
        role="switch"
        aria-checked={embed.enabled}
        disabled={disabled}
        onclick={() => act(embed?.enabled ? 'disable' : 'enable')}
      >{embed.enabled ? $t.localEmbed.disable : $t.localEmbed.enable}</button>
    {/if}
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  {#if embed && status}
    {#if embed.restart_required}
      <div class="le-notice">
        <span>{restarting ? $t.localEmbed.restarting : $t.localEmbed.restartNeeded}</span>
        <button class="btn btn-secondary btn-sm" type="button" disabled={disabled} onclick={restart}>
          {confirming === 'restart' ? $t.localEmbed.confirmRestart : $t.localEmbed.restart}
        </button>
      </div>
    {/if}

    <dl class="le-facts">
      <dt>{$t.localEmbed.labels.power}</dt>
      <dd>{embed.enabled ? $t.localEmbed.state.on : $t.localEmbed.state.off}</dd>
      <dt>{$t.localEmbed.labels.model}</dt>
      <dd class="mono">{status.model}</dd>
      <dt>{$t.localEmbed.labels.disk}</dt>
      <dd>
        {status.model_installed
          ? `${$t.localEmbed.state.installed} · ${formatBytes(status.model_bytes)}`
          : $t.localEmbed.state.notInstalled}
      </dd>
      <dt>{$t.localEmbed.labels.python}</dt>
      <dd>{status.env_ready ? $t.localEmbed.ready : $t.localEmbed.notSetUp}</dd>
      <dt>{$t.localEmbed.labels.server}</dt>
      <dd>
        {status.running ? $t.localEmbed.state.running : $t.localEmbed.state.stopped}
        <span class="mono le-dim">{status.base_url}</span>
      </dd>
      <dt>{$t.localEmbed.labels.folder}</dt>
      <dd class="mono le-dim">{status.dir}</dd>
    </dl>

    {#if foreign}
      <div class="le-notice warn">{$t.localEmbed.foreignServer(foreign)}</div>
    {/if}
    {#if !status.env_ready && !status.uv_path}
      <div class="le-notice warn">{$t.localEmbed.needsUv}</div>
    {/if}
    {#if embed.enabled && !status.model_installed && !running}
      <div class="le-notice">{$t.localEmbed.enableFirst}</div>
    {/if}

    {#if job}
      <div class="le-job" aria-live="polite">
        <div class="le-job-head">
          <strong>{job.action === 'reindex' ? $t.localEmbed.job.reindex : $t.localEmbed.job.install}</strong>
          {#if job.running}
            {#if job.total}<span class="mono">{$t.localEmbed.job.progress(job.done ?? 0, job.total)}</span>{/if}
            <button class="btn btn-ghost btn-sm" type="button" onclick={() => act('cancel')}>{$t.localEmbed.cancel}</button>
          {:else if job.error}
            <span class="le-error">{$t.localEmbed.job.failed(job.error)}</span>
          {:else}
            <span>{$t.localEmbed.job.finished}</span>
          {/if}
        </div>
        {#if percent !== null}
          <div class="le-bar"><span style:width="{percent}%"></span></div>
        {/if}
        {#if job.log?.length}
          <pre class="le-log">{job.log.join('\n')}</pre>
        {/if}
      </div>
    {/if}

    {#if embed.reindex_pending > 0}
      <div class="le-notice">
        <span>{$t.localEmbed.reindex.body(embed.reindex_pending)}</span>
        <button class="btn btn-secondary btn-sm" type="button" disabled={!canReindex(embed) || busy} onclick={() => act('reindex')}>
          {$t.localEmbed.reindex.run}
        </button>
      </div>
    {/if}

    <div class="le-actions">
      <button class="btn btn-primary btn-sm" type="button" disabled={!canInstall(embed) || busy || restarting} onclick={() => act('install')}>
        {status.model_installed ? $t.localEmbed.reinstall : $t.localEmbed.install}
      </button>
      {#if status.model_installed}
        <button class="btn btn-danger btn-sm" type="button" disabled={disabled} onclick={() => confirmThen('remove')}>
          {confirming === 'remove' ? $t.localEmbed.confirmDelete : $t.localEmbed.remove}
        </button>
      {/if}
      {#if status.env_ready}
        <button class="btn btn-danger btn-sm" type="button" disabled={disabled} onclick={() => confirmThen('remove-all')}>
          {confirming === 'remove-all' ? $t.localEmbed.confirmDelete : $t.localEmbed.removeAll}
        </button>
      {/if}
    </div>
  {/if}
</section>

<style>
  .local-embed { display: flex; flex-direction: column; gap: 16px; }
  .le-head { display: flex; justify-content: space-between; gap: 16px; align-items: flex-start; }
  .le-head h3 { margin: 0; }
  .le-subtitle { color: var(--text-secondary); font-size: var(--text-sm); margin-top: 4px; }
  .le-facts { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 6px 16px; margin: 0; font-size: var(--text-sm); }
  .le-facts dt { color: var(--text-tertiary); }
  .le-facts dd { margin: 0; overflow-wrap: anywhere; }
  .mono { font-family: var(--font-mono); font-size: var(--text-xs); }
  .le-dim { color: var(--text-tertiary); }
  .le-notice { display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 8px 12px; border-radius: 8px; background: var(--surface-elevated); font-size: var(--text-sm); }
  .le-notice.warn { background: var(--warning-muted); color: var(--warning); }
  .le-job { display: flex; flex-direction: column; gap: 8px; font-size: var(--text-sm); }
  .le-job-head { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
  .le-error { color: var(--error); overflow-wrap: anywhere; }
  .le-bar { height: 4px; border-radius: 2px; background: var(--surface-elevated); overflow: hidden; }
  .le-bar span { display: block; height: 100%; background: var(--primary); }
  .le-log { margin: 0; max-height: 160px; overflow: auto; padding: 8px 12px; border-radius: 8px; background: var(--surface-inset); font-family: var(--font-mono); font-size: var(--text-xs); color: var(--text-secondary); white-space: pre-wrap; overflow-wrap: anywhere; }
  .le-actions { display: flex; gap: 8px; flex-wrap: wrap; }
</style>
