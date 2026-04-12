<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import {
    getEventsHistory,
    getPulseStatus,
    getReflectionStatus,
    streamEvents,
  } from '../lib/api'
  import type {
    PulseSnapshot,
    ReflectionSnapshot,
    NotificationMessage,
  } from '../lib/types'
  import ChatPanel from './ChatPanel.svelte'

  interface Props {
    onNavigate: (path: string) => void
    initialPrompt?: string
  }

  let { onNavigate, initialPrompt }: Props = $props()

  let pulse: PulseSnapshot | null = $state(null)
  let reflection: ReflectionSnapshot | null = $state(null)
  let notifications: NotificationMessage[] = $state([])
  let unreadCount = $state(0)

  let loading = $state(true)
  let error = $state('')
  let stopStream: (() => void) | null = null

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function compact(value?: string, max = 120): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    return text.length <= max ? text : `${text.slice(0, max - 1)}\u2026`
  }

  function relativeTime(value?: string): string {
    if (!value?.trim()) return 'never'
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    if (date.getFullYear() <= 1) return 'never'
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
    if (seconds < 60) return `${seconds}s ago`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
    return `${Math.floor(seconds / 86400)}d ago`
  }

  async function load() {
    loading = true
    error = ''
    try {
      const [h, r, e] = await Promise.allSettled([
        getPulseStatus(),
        getReflectionStatus(),
        getEventsHistory(20),
      ])
      pulse = h.status === 'fulfilled' ? h.value : null
      reflection = r.status === 'fulfilled' ? r.value : null
      if (e.status === 'fulfilled') {
        notifications = (e.value.items ?? []).slice(0, 10)
        unreadCount = e.value.unread_count ?? 0
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load overview'
    } finally {
      loading = false
    }
  }

  function startEventStream() {
    stopStream?.()
    stopStream = streamEvents(
      (event) => {
        notifications = [event, ...notifications.filter((n) => n.id !== event.id)].slice(0, 10)
        unreadCount++
        if (event.category === 'pulse') {
          void getPulseStatus().then((s) => { pulse = s }).catch(() => {})
          // A pulse reflection-failure signal means the reflection
          // snapshot likely changed too — refresh both.
          void getReflectionStatus().then((s) => { reflection = s }).catch(() => {})
        }
      },
    )
  }

  onMount(() => {
    void load()
    startEventStream()
  })

  onDestroy(() => {
    stopStream?.()
  })
</script>

<div class="home">
  <div class="home-header">
    <div>
      <h2>Welcome back</h2>
      <p class="home-subtitle">Here's what needs your attention.</p>
    </div>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  {#if loading}
    <div class="home-loading">Loading overview...</div>
  {:else}
    <!-- Pulse strip -->
    <div class="pulse-strip">
      <div class="pulse-item">
        <span class="pulse-value" class:has-attention={(pulse?.total_ticks ?? 0) > 0 && !pulse?.last_err}>
          {pulse?.total_ticks ?? 0}
        </span>
        <span class="pulse-label">Pulse ticks</span>
      </div>
      <div class="pulse-divider"></div>
      <div class="pulse-item">
        <span class="pulse-value" class:has-warning={!!pulse?.last_err}>
          {pulse?.last_tick_at ? relativeTime(pulse.last_tick_at) : 'never'}
        </span>
        <span class="pulse-label">Last tick</span>
      </div>
      <div class="pulse-divider"></div>
      <div class="pulse-item">
        <span class="pulse-value" class:has-warning={(reflection?.consecutive_failures ?? 0) > 0}>
          {reflection?.last_successful_run_at ? relativeTime(reflection.last_successful_run_at) : 'never'}
        </span>
        <span class="pulse-label">Last reflection</span>
      </div>
      <div class="pulse-divider"></div>
      <div class="pulse-item">
        <span class="pulse-value">{unreadCount}</span>
        <span class="pulse-label">Unread notifications</span>
      </div>
    </div>

    <!-- Chat (main feature) -->
    <section class="chat-main card">
      <ChatPanel {initialPrompt} />
    </section>

    <div class="home-grid">
      <!-- Pulse -->
      <section class="card home-section">
        <div class="card-header">
          <span class="card-title">Pulse</span>
          {#if (pulse?.total_ticks ?? 0) > 0}
            <span class="badge badge-success">active</span>
          {:else}
            <span class="badge badge-default">idle</span>
          {/if}
        </div>
        {#if pulse?.last_decision?.summary}
          <div class="list-items">
            <div class="list-item">
              <p class="list-item-detail">{compact(pulse.last_decision.summary, 200)}</p>
              <span class="list-item-time">{relativeTime(pulse.last_tick_at)}</span>
            </div>
          </div>
        {:else if pulse?.last_err}
          <div class="list-items">
            <div class="list-item">
              <p class="list-item-detail" style="color:var(--error)">{compact(pulse.last_err, 200)}</p>
            </div>
          </div>
        {:else}
          <div class="empty-state">
            <p>No pulse decisions yet.</p>
          </div>
        {/if}
      </section>

      <!-- Recent notifications -->
      <section class="card home-section">
        <div class="card-header">
          <span class="card-title">Recent notifications</span>
          <span class="badge badge-default">{notifications.length}</span>
        </div>
        {#if notifications.length === 0}
          <div class="empty-state">
            <p>No recent notifications.</p>
          </div>
        {:else}
          <div class="list-items">
            {#each notifications.slice(0, 5) as item}
              <div class="list-item">
                <div class="list-item-top">
                  <strong>{item.title}</strong>
                  <span class="badge badge-default">{item.category}</span>
                </div>
                <p class="list-item-detail">{compact(item.message, 120)}</p>
                <span class="list-item-time">{fmt(item.timestamp)}</span>
              </div>
            {/each}
          </div>
        {/if}
      </section>
    </div>
  {/if}
</div>

<style>
  .home {
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .home-header {
    margin-bottom: var(--space-6);
  }

  .home-header h2 {
    font-size: var(--text-2xl);
    margin-bottom: var(--space-1);
  }

  .home-subtitle {
    color: var(--text-tertiary);
    font-size: var(--text-base);
  }

  .home-loading {
    padding: var(--space-10);
    text-align: center;
    color: var(--text-tertiary);
  }

  /* ── Pulse strip ──────────────────────────────── */
  .pulse-strip {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    padding: var(--space-5) var(--space-6);
    background: var(--bg-surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    margin-bottom: var(--space-6);
  }

  .pulse-item {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .pulse-value {
    font-family: var(--font-display);
    font-size: var(--text-xl);
    font-weight: 600;
    color: var(--text-primary);
    line-height: 1;
  }

  .pulse-value.has-attention {
    color: var(--warning);
  }

  .pulse-value.has-warning {
    color: var(--error);
  }

  .pulse-label {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .pulse-divider {
    width: 1px;
    height: 32px;
    background: var(--border-subtle);
    flex-shrink: 0;
  }

  /* ── Chat main ────────────────────────────────── */
  .chat-main {
    margin-bottom: var(--space-5);
  }

  /* ── Home grid ────────────────────────────────── */
  .home-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-4);
  }

  .home-section {
    min-height: 200px;
  }

  /* ── List items ───────────────────────────────── */
  .list-items {
    display: grid;
    gap: var(--space-2);
  }

  .list-item {
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-md);
    background: var(--bg-base);
  }

  .list-item-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: var(--space-1);
  }

  .list-item-top strong {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }

  .list-item-detail {
    font-size: var(--text-sm);
    color: var(--text-secondary);
    margin-bottom: var(--space-1);
  }

  .list-item-time {
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  /* ── Responsive ───────────────────────────────── */
  @media (max-width: 900px) {
    .home-grid {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 600px) {
    .pulse-strip {
      flex-wrap: wrap;
      gap: var(--space-4);
    }

    .pulse-divider {
      display: none;
    }

    .pulse-item {
      flex: 1;
      min-width: 80px;
    }
  }
</style>
