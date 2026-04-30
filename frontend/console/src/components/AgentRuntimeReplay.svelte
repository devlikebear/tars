<script lang="ts">
  import { onDestroy } from 'svelte'
  import {
    deriveAgentRuntimeReplayBounds,
    deriveAgentRuntimeReplayState,
  } from '../lib/agentruntime-graph'
  import type { AgentRuntimeRunEvent } from '../lib/types'

  interface Props {
    events: AgentRuntimeRunEvent[]
    runStatus?: string
  }

  type PlaybackSpeed = 1 | 2 | 5
  type PlaybackSpeedOption = {
    value: PlaybackSpeed
    label: string
  }

  let { events, runStatus = 'pending' }: Props = $props()
  let cursorMs = $state(0)
  let live = $state(true)
  let playing = $state(false)
  let speed: PlaybackSpeed = $state(1)
  let playbackTimer: ReturnType<typeof setInterval> | null = null

  const speeds: PlaybackSpeedOption[] = [
    { value: 1, label: '1x' },
    { value: 2, label: '2x' },
    { value: 5, label: '5x' },
  ]

  let bounds = $derived(deriveAgentRuntimeReplayBounds(events))
  let replayState = $derived(deriveAgentRuntimeReplayState(events, cursorMs))
  let progressPercent = $derived.by<number>(() => {
    if (!bounds.hasTimeline || bounds.endMs <= bounds.startMs) return 100
    return Math.max(0, Math.min(100, ((cursorMs - bounds.startMs) / (bounds.endMs - bounds.startMs)) * 100))
  })
  let effectiveStatus = $derived(replayState.appliedCount === 0 ? runStatus : replayState.status)

  $effect(() => {
    if (bounds.hasTimeline && (live || cursorMs === 0)) {
      cursorMs = bounds.endMs
    }
  })

  $effect(() => {
    if (!playing || live || !bounds.hasTimeline) {
      stopPlayback()
      return
    }
    stopPlayback()
    playbackTimer = setInterval(() => {
      const nextCursor = Math.min(bounds.endMs, cursorMs + 250 * speed)
      cursorMs = nextCursor
      if (nextCursor >= bounds.endMs) {
        playing = false
      }
    }, 250)
    return stopPlayback
  })

  function toggleLive() {
    live = !live
    if (live) {
      playing = false
      cursorMs = bounds.endMs
    }
  }

  function togglePlayback() {
    if (!bounds.hasTimeline) return
    if (playing) {
      playing = false
      return
    }
    live = false
    if (cursorMs >= bounds.endMs) cursorMs = bounds.startMs
    playing = true
  }

  function scrub(event: Event) {
    const input = event.currentTarget as HTMLInputElement
    live = false
    playing = false
    cursorMs = Number(input.value)
  }

  function setSpeed(nextSpeed: PlaybackSpeed) {
    speed = nextSpeed
  }

  function stopPlayback() {
    if (!playbackTimer) return
    clearInterval(playbackTimer)
    playbackTimer = null
  }

  function fmtTime(value: number): string {
    if (!Number.isFinite(value) || value <= 0) return '--:--'
    return new Intl.DateTimeFormat('en', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    }).format(new Date(value))
  }

  onDestroy(stopPlayback)
</script>

<section class="detail-panel replay-scrubber" aria-label="Agent Runtime replay scrubber">
  <div class="replay-head">
    <div>
      <h3>Replay</h3>
      <p>{replayState.appliedCount}/{replayState.totalCount} events / {effectiveStatus}</p>
    </div>
    <div class="replay-controls" aria-label="Replay controls">
      <button type="button" class:active={live} disabled={!bounds.hasTimeline} onclick={toggleLive}>Live</button>
      <button type="button" disabled={!bounds.hasTimeline} onclick={togglePlayback}>{playing ? 'Pause' : 'Play'}</button>
      <div class="speed-controls" aria-label="Playback speed">
        {#each speeds as option}
          <button type="button" class:active={speed === option.value} onclick={() => setSpeed(option.value)}>{option.label}</button>
        {/each}
      </div>
    </div>
  </div>

  {#if !bounds.hasTimeline}
    <div class="agentruntime-empty">No timestamped events available for replay.</div>
  {:else}
    <div class="replay-timeline">
      <span>{fmtTime(bounds.startMs)}</span>
      <input
        type="range"
        min={bounds.startMs}
        max={bounds.endMs}
        step="250"
        value={cursorMs}
        disabled={live}
        aria-label="Replay cursor"
        oninput={scrub}
      />
      <span>{fmtTime(bounds.endMs)}</span>
    </div>
    <div class="replay-progress" aria-hidden="true">
      <span style={`width: ${progressPercent}%`}></span>
    </div>
    <div class="replay-state-grid">
      <div><span>Cursor</span><strong>{fmtTime(replayState.currentTimeMs)}</strong></div>
      <div><span>Status</span><strong>{effectiveStatus}</strong></div>
      <div><span>Last event</span><strong>{replayState.lastEventType}</strong></div>
      <div><span>Message</span><strong>{replayState.lastMessage || '-'}</strong></div>
    </div>
    {#if replayState.filePaths.length > 0}
      <div class="replay-file-row" aria-label="Files touched by replayed events">
        {#each replayState.filePaths as path}
          <span title={path}>{path}</span>
        {/each}
      </div>
    {/if}
  {/if}
</section>

<style>
  .replay-scrubber { display: flex; flex-direction: column; gap: var(--space-3); overflow: hidden; }
  .replay-head { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-3); }
  .replay-head h3 { margin: 0; }
  .replay-head p { margin: var(--space-1) 0 0; color: var(--text-tertiary); font-size: var(--text-xs); }
  .replay-controls { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
  .replay-controls button, .speed-controls button {
    min-height: 28px;
    border: 1px solid var(--border-subtle);
    background: var(--surface-inset);
    color: var(--text-tertiary);
    border-radius: var(--radius-sm);
    padding: 0 var(--space-2);
    font: inherit;
    font-size: var(--text-xs);
    cursor: pointer;
  }
  .replay-controls button.active, .speed-controls button.active {
    border-color: var(--primary);
    background: var(--primary-muted);
    color: var(--primary-text);
  }
  .replay-controls button:disabled {
    cursor: default;
    opacity: 0.55;
  }
  .speed-controls { display: inline-flex; gap: var(--space-1); border: 1px solid var(--border-subtle); background: var(--surface-inset); border-radius: var(--radius-md); padding: 3px; }
  .speed-controls button { border: 0; background: transparent; }
  .replay-timeline { display: grid; grid-template-columns: auto minmax(120px, 1fr) auto; gap: var(--space-2); align-items: center; color: var(--text-ghost); font-family: var(--font-mono); font-size: var(--text-xs); }
  .replay-timeline input { width: 100%; accent-color: var(--primary); }
  .replay-progress { height: 8px; overflow: hidden; border: 1px solid var(--border-subtle); background: var(--surface-inset); border-radius: var(--radius-sm); }
  .replay-progress span { display: block; height: 100%; background: var(--primary); }
  .replay-state-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--space-2); }
  .replay-state-grid div { min-width: 0; border: 1px solid var(--border-subtle); background: var(--surface-inset); border-radius: var(--radius-sm); padding: var(--space-2); }
  .replay-state-grid span { display: block; color: var(--text-ghost); font-family: var(--font-mono); font-size: 10px; text-transform: uppercase; }
  .replay-state-grid strong { display: block; min-width: 0; margin-top: 2px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-primary); font-size: var(--text-xs); }
  .replay-file-row { display: flex; flex-wrap: wrap; gap: var(--space-1); }
  .replay-file-row span { max-width: 260px; min-height: 24px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; border: 1px solid var(--border-subtle); background: var(--surface-inset); border-radius: var(--radius-sm); padding: 3px var(--space-2); color: var(--text-secondary); font-family: var(--font-mono); font-size: var(--text-xs); }
  .agentruntime-empty { color: var(--text-ghost); font-size: var(--text-sm); }
  @media (max-width: 768px) {
    .replay-head { align-items: stretch; flex-direction: column; }
    .replay-state-grid, .replay-timeline { grid-template-columns: 1fr; }
  }
</style>
