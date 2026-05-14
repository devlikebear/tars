<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { listSessions, getPulseStatus, getReflectionStatus } from '../lib/api'
  import type { PulseSnapshot, ReflectionSnapshot, Session } from '../lib/types'

  interface Props {
    serverHealth?: string
    onNavigate?: (path: string) => void
  }

  let { serverHealth = 'ok', onNavigate }: Props = $props()

  const POLL_INTERVAL_MS = 15_000
  const PULSE_STALE_MS = 5 * 60 * 1000

  let pulse = $state<PulseSnapshot | null>(null)
  let reflection = $state<ReflectionSnapshot | null>(null)
  let sessions = $state<Session[]>([])
  let popoverOpen = $state(false)
  let pollTimer: ReturnType<typeof setInterval> | null = null

  let activeSession = $derived.by(() => {
    const visible = sessions.filter((s) => !s.archived_at && s.kind !== 'worker')
    if (visible.length === 0) return null
    return visible
      .slice()
      .sort((a, b) => (b.updated_at ?? '').localeCompare(a.updated_at ?? ''))[0] ?? null
  })

  let activeSessionCount = $derived(
    sessions.filter((s) => !s.archived_at && s.kind !== 'worker').length,
  )

  let serverState = $derived(stateForServer(serverHealth))
  let pulseState = $derived(stateForPulse(pulse))
  let reflectionState = $derived(stateForReflection(reflection))
  let sessionState = $derived<'ok' | 'idle'>(activeSessionCount > 0 ? 'ok' : 'idle')

  let aggregateLevel = $derived<'ok' | 'warn' | 'error'>(
    [serverState, pulseState, reflectionState].some((s) => s === 'error')
      ? 'error'
      : [serverState, pulseState, reflectionState].some((s) => s === 'warn')
        ? 'warn'
        : 'ok',
  )

  function stateForServer(value: string): 'ok' | 'warn' | 'error' {
    if (value === 'ok') return 'ok'
    if (value === 'connecting') return 'warn'
    return 'error'
  }

  function stateForPulse(snapshot: PulseSnapshot | null): 'ok' | 'warn' | 'error' | 'idle' {
    if (!snapshot) return 'idle'
    if (snapshot.last_err) return 'error'
    if (!snapshot.last_tick_at) return 'idle'
    const last = Date.parse(snapshot.last_tick_at)
    if (Number.isNaN(last)) return 'idle'
    if (Date.now() - last > PULSE_STALE_MS) return 'warn'
    return 'ok'
  }

  function stateForReflection(snapshot: ReflectionSnapshot | null): 'ok' | 'warn' | 'error' | 'idle' {
    if (!snapshot) return 'idle'
    if (snapshot.consecutive_failures >= 2) return 'error'
    if (snapshot.consecutive_failures === 1) return 'warn'
    if (!snapshot.last_run_at) return 'idle'
    return 'ok'
  }

  async function refresh() {
    const [pulseResult, reflectionResult, sessionsResult] = await Promise.allSettled([
      getPulseStatus(),
      getReflectionStatus(),
      listSessions(false, 'active'),
    ])
    if (pulseResult.status === 'fulfilled') pulse = pulseResult.value
    if (reflectionResult.status === 'fulfilled') reflection = reflectionResult.value
    if (sessionsResult.status === 'fulfilled') sessions = sessionsResult.value
  }

  function togglePopover() {
    popoverOpen = !popoverOpen
  }

  function handleClickOutside(e: MouseEvent) {
    const target = e.target as HTMLElement
    if (!target.closest('.status-pill-wrapper')) {
      popoverOpen = false
    }
  }

  function openActiveChat() {
    popoverOpen = false
    if (activeSession) {
      onNavigate?.(`/console/chat/${encodeURIComponent(activeSession.id)}`)
    } else {
      onNavigate?.('/console/chat')
    }
  }

  function jumpTo(path: string) {
    popoverOpen = false
    onNavigate?.(path)
  }

  onMount(() => {
    void refresh()
    pollTimer = setInterval(() => { void refresh() }, POLL_INTERVAL_MS)
  })

  onDestroy(() => {
    if (pollTimer) clearInterval(pollTimer)
  })
</script>

<svelte:document onclick={handleClickOutside} />

<div class="status-pill-wrapper">
  <button
    type="button"
    class="status-pill {aggregateLevel}"
    aria-label="TARS status"
    aria-expanded={popoverOpen}
    title="Server, Pulse, Reflection, sessions"
    onclick={togglePopover}
  >
    <span class="status-dot {serverState}" aria-hidden="true"></span>
    <span class="status-dot {pulseState}" aria-hidden="true"></span>
    <span class="status-dot {reflectionState}" aria-hidden="true"></span>
    <span class="status-pill-count">{activeSessionCount}</span>
  </button>

  {#if popoverOpen}
    <div class="status-popover" role="dialog" aria-label="Status detail">
      <div class="status-row">
        <span class="status-row-label">
          <span class="status-dot {serverState}" aria-hidden="true"></span>
          Server
        </span>
        <span class="status-row-value">{serverHealth === 'ok' ? 'Connected' : serverHealth === 'connecting' ? 'Connecting…' : 'Disconnected'}</span>
      </div>

      <button class="status-row clickable" type="button" onclick={() => jumpTo('/console/pulse')}>
        <span class="status-row-label">
          <span class="status-dot {pulseState}" aria-hidden="true"></span>
          Pulse
        </span>
        <span class="status-row-value">
          {pulseState === 'idle' ? '—' : pulse?.last_err ? 'Error' : pulseState === 'warn' ? 'Stale tick' : 'OK'}
        </span>
      </button>

      <button class="status-row clickable" type="button" onclick={() => jumpTo('/console/reflection')}>
        <span class="status-row-label">
          <span class="status-dot {reflectionState}" aria-hidden="true"></span>
          Reflection
        </span>
        <span class="status-row-value">
          {reflectionState === 'idle' ? '—' : (reflection?.consecutive_failures ?? 0) > 0 ? `${reflection?.consecutive_failures} fail(s)` : 'OK'}
        </span>
      </button>

      <button class="status-row clickable" type="button" onclick={openActiveChat}>
        <span class="status-row-label">
          <span class="status-dot {sessionState}" aria-hidden="true"></span>
          Sessions
        </span>
        <span class="status-row-value">
          {activeSessionCount} active{activeSession ? ` · jump to ${activeSession.title || activeSession.id}` : ''}
        </span>
      </button>

      <div class="status-popover-footer">
        <button class="btn btn-ghost btn-sm" type="button" onclick={() => jumpTo('/console/ops')}>Open Ops</button>
        <button class="btn btn-primary btn-sm" type="button" onclick={openActiveChat}>
          {activeSession ? 'Open active chat' : 'Open chat'}
        </button>
      </div>
    </div>
  {/if}
</div>

<style>
  .status-pill-wrapper {
    position: relative;
  }

  .status-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 30px;
    padding: 0 var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: 11px;
    cursor: pointer;
    transition: all var(--duration-fast) var(--ease-out);
  }

  .status-pill:hover {
    background: var(--surface-elevated);
    border-color: var(--border-default);
    color: var(--text-primary);
  }

  .status-pill.warn {
    border-color: color-mix(in srgb, var(--warning) 45%, var(--border-default));
  }

  .status-pill.error {
    border-color: color-mix(in srgb, var(--error) 55%, var(--border-default));
  }

  .status-pill-count {
    font-family: var(--font-display);
    font-size: 10px;
    font-weight: 600;
    color: var(--text-tertiary);
    padding-left: 4px;
    border-left: 1px solid var(--border-subtle);
    margin-left: 2px;
  }

  .status-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--text-ghost);
    flex-shrink: 0;
  }
  .status-dot.ok { background: var(--success); }
  .status-dot.warn { background: var(--warning); }
  .status-dot.error { background: var(--error); }
  .status-dot.idle { background: var(--text-ghost); }

  .status-popover {
    position: absolute;
    top: calc(100% + var(--space-2));
    right: 0;
    width: 280px;
    display: flex;
    flex-direction: column;
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
    overflow: hidden;
    animation: pillIn 0.12s var(--ease-out);
    z-index: 50;
  }

  @keyframes pillIn {
    from { opacity: 0; transform: translateY(-4px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .status-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    background: transparent;
    color: var(--text-secondary);
    font-size: var(--text-xs);
    text-align: left;
    width: 100%;
    border-left: 0;
    border-right: 0;
    border-top: 0;
  }

  .status-row.clickable {
    cursor: pointer;
    transition: background var(--duration-fast) var(--ease-out);
  }

  .status-row.clickable:hover {
    background: var(--surface-elevated);
  }

  .status-row-label {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-family: var(--font-display);
    font-weight: 500;
    color: var(--text-primary);
  }

  .status-row-value {
    font-family: var(--font-mono);
    color: var(--text-tertiary);
    font-size: 11px;
    max-width: 60%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .status-popover-footer {
    display: flex;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    background: var(--surface-elevated);
  }

  .status-popover-footer .btn {
    flex: 1;
  }
</style>
