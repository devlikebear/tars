<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { getPulseStatus, getReflectionStatus, getServerStatus, listSessions } from '../lib/api'
  import {
    countActiveSessions,
    derivePulseTone,
    deriveReflectionTone,
    formatRelativeStatusTime,
  } from '../lib/statusStrip'
  import type { PulseSnapshot, ReflectionSnapshot } from '../lib/types'
  import type { StatusTone } from '../lib/statusStrip'

  interface Props {
    onNavigate: (path: string) => void
  }

  let { onNavigate }: Props = $props()

  let serverTone = $state<StatusTone>('warn')
  let serverVersion = $state('')
  let pulse = $state<PulseSnapshot | null>(null)
  let reflection = $state<ReflectionSnapshot | null>(null)
  let sessionCount = $state(0)
  let pollTimer: ReturnType<typeof setInterval> | null = null

  let pulseTone = $derived(derivePulseTone(pulse))
  let reflectionTone = $derived(deriveReflectionTone(reflection))
  let pulseLabel = $derived(pulseTone === 'error' ? 'error' : pulseTone === 'ok' ? 'healthy' : 'waiting')
  let reflectionLabel = $derived(formatRelativeStatusTime(reflection?.last_run_at || reflection?.last_successful_run_at))
  let serverLabel = $derived(serverTone === 'ok' ? 'running' : 'offline')

  function navigate(path: string) {
    onNavigate(path)
  }

  async function loadStatus() {
    const [serverResult, pulseResult, reflectionResult, sessionsResult] = await Promise.allSettled([
      getServerStatus(),
      getPulseStatus(),
      getReflectionStatus(),
      listSessions(false),
    ])

    if (serverResult.status === 'fulfilled') {
      serverTone = 'ok'
      serverVersion = serverResult.value.version ?? ''
    } else {
      serverTone = 'error'
      serverVersion = ''
    }
    pulse = pulseResult.status === 'fulfilled' ? pulseResult.value : null
    reflection = reflectionResult.status === 'fulfilled' ? reflectionResult.value : null
    sessionCount = sessionsResult.status === 'fulfilled' ? countActiveSessions(sessionsResult.value) : 0
  }

  onMount(() => {
    void loadStatus()
    pollTimer = setInterval(loadStatus, 30_000)
  })

  onDestroy(() => {
    if (pollTimer) clearInterval(pollTimer)
  })
</script>

<div class="status-strip" aria-label="Sidebar system status">
  <button type="button" class="status-row" onclick={() => navigate('/console')} aria-label="SERVER status">
    <span class="status-label">SERVER</span>
    <span class={`status-dot ${serverTone}`} aria-hidden="true"></span>
    <span class="status-value">{serverLabel}</span>
    {#if serverVersion}
      <span class="status-extra">v{serverVersion}</span>
    {/if}
  </button>

  <button type="button" class="status-row" onclick={() => navigate('/console/pulse')} aria-label="PULSE status">
    <span class="status-label">PULSE</span>
    <span class={`status-dot ${pulseTone}`} aria-hidden="true"></span>
    <span class="status-value">{pulseLabel}</span>
  </button>

  <button type="button" class="status-row" onclick={() => navigate('/console/reflection')} aria-label="REFLECT status">
    <span class="status-label">REFLECT</span>
    <span class={`status-dot ${reflectionTone}`} aria-hidden="true"></span>
    <span class="status-value">{reflectionLabel}</span>
  </button>

  <button type="button" class="status-row" onclick={() => navigate('/console/chat')} aria-label="SESSIONS status">
    <span class="status-label">SESSIONS</span>
    <span class="status-value session-count">{sessionCount} active</span>
  </button>
</div>

<style>
  .status-strip {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 100%;
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
  }

  .status-row {
    display: grid;
    grid-template-columns: minmax(54px, 0.78fr) 8px minmax(0, 1fr);
    align-items: center;
    gap: var(--space-2);
    min-height: 24px;
    width: 100%;
    padding: 2px var(--space-1);
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
    font: inherit;
    text-align: left;
  }

  .status-row:hover {
    background: var(--surface-elevated);
    color: var(--text-primary);
  }

  .status-label {
    min-width: 0;
    overflow: hidden;
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0;
    text-overflow: ellipsis;
  }

  .status-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--warning);
  }
  .status-dot.ok { background: var(--success); }
  .status-dot.warn { background: var(--warning); }
  .status-dot.error { background: var(--error); }

  .status-value,
  .status-extra {
    min-width: 0;
    overflow: hidden;
    font-size: 11px;
    line-height: 1.2;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .status-extra {
    grid-column: 3;
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
  }

  .session-count {
    grid-column: 2 / 4;
  }
</style>
