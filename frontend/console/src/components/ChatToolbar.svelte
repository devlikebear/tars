<script lang="ts">
  // Chat top bar: pulse mini-stats plus one toggle per dock panel. The
  // header cleanup in #968 replaces this with an icon rail and the command
  // palette; until then it is moved here unchanged from Chat.svelte.
  import { onMount } from 'svelte'
  import { getEventsHistory, getPulseStatus } from '../lib/api'
  import { t } from '../i18n'
  import type { PulseSnapshot } from '../lib/types'
  import { chatSession } from '../lib/stores/chatSession'
  import { chatDock, type ChatDockPanelID } from '../lib/stores/chatDockStore.svelte'

  let pulse: PulseSnapshot | null = $state(null)
  let unreadCount = $state(0)

  let chatArtifacts = $derived(chatSession.artifacts)
  let tasksSummary = $derived(chatSession.tasksSummary)
  let sessionHealth = $derived(chatSession.health)
  let healthIssueCount = $derived(sessionHealth.recommendations.length)

  function isPanelOpen(panelID: ChatDockPanelID): boolean {
    return chatDock.isOpen(panelID)
  }

  function togglePanel(panelID: ChatDockPanelID) {
    chatDock.toggle(panelID)
  }

  function relativeTime(value?: string): string {
    if (!value?.trim()) return $t.chat.statusStrip.neverTick
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    if (date.getFullYear() <= 1) return $t.chat.statusStrip.neverTick
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
    const labels = $t.sessions.relativeTime
    if (seconds < 60) return labels.secondsAgo(seconds)
    if (seconds < 3600) return labels.minutesAgo(Math.floor(seconds / 60))
    if (seconds < 86400) return labels.hoursAgo(Math.floor(seconds / 3600))
    return labels.daysAgo(Math.floor(seconds / 86400))
  }

  async function loadDashboard() {
    const [h, e] = await Promise.allSettled([
      getPulseStatus(),
      getEventsHistory(1),
    ])
    pulse = h.status === 'fulfilled' ? h.value : null
    if (e.status === 'fulfilled') {
      unreadCount = e.value.unread_count ?? 0
    }
  }

  onMount(() => {
    void loadDashboard()
  })
</script>

<!-- Mini dashboard pulse -->
<div class="chat-pulse">
  <div class="chat-pulse-stats">
  <div class="pulse-item">
    <span class="pulse-val" class:warn={!!pulse?.last_err}>
      {pulse?.total_ticks ?? 0}
    </span>
    <span class="pulse-lbl">{$t.chat.statusStrip.pulseTicks}</span>
  </div>
  <div class="pulse-sep"></div>
  <div class="pulse-item">
    <span class="pulse-val">{pulse?.last_tick_at ? relativeTime(pulse.last_tick_at) : $t.chat.statusStrip.neverTick}</span>
    <span class="pulse-lbl">{$t.chat.statusStrip.lastTick}</span>
  </div>
  <div class="pulse-sep"></div>
  <div class="pulse-item">
    <span class="pulse-val">{unreadCount}</span>
    <span class="pulse-lbl">{$t.chat.statusStrip.unread}</span>
  </div>
  <div class="pulse-sep"></div>
  </div>
  <div class="pulse-panel-toggles">
    <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('sessions')} onclick={() => togglePanel('sessions')} title={$t.chat.panels.sessionsTooltip}>{$t.chat.panels.sessions}</button>
    <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('artifacts')} onclick={() => togglePanel('artifacts')} title={$t.chat.panels.filesTooltip}>{$t.chat.panels.files}{#if chatArtifacts.length > 0}{$t.chat.panels.filesCount(chatArtifacts.length)}{/if}</button>
    <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('config')} onclick={() => togglePanel('config')} title={$t.chat.panels.configTooltip}>{$t.chat.panels.config}</button>
    <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('context')} onclick={() => togglePanel('context')} title={$t.chat.panels.contextTooltip}>{$t.chat.panels.context}</button>
    <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('prompt')} onclick={() => togglePanel('prompt')} title={$t.chat.panels.promptTooltip}>{$t.chat.panels.prompt}</button>
    <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('prior')} onclick={() => togglePanel('prior')} title={$t.chat.panels.priorTooltip}>{$t.chat.panels.prior}</button>
    <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('tasks')} onclick={() => togglePanel('tasks')} title={tasksSummary.total > 0 ? $t.chat.panels.tasksProgressTooltip(tasksSummary.completed, tasksSummary.in_progress, tasksSummary.pending) : $t.chat.panels.tasksTooltip}>{$t.chat.panels.tasks}{#if tasksSummary.total > 0}{$t.chat.panels.tasksCount(tasksSummary.completed, tasksSummary.total)}{/if}</button>
    <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('git')} onclick={() => togglePanel('git')} title={$t.chat.panels.gitTooltip}>{$t.chat.panels.git}</button>
    <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('skillExtraction')} onclick={() => togglePanel('skillExtraction')} title={$t.chat.panels.skillsTooltip}>{$t.chat.panels.skills}</button>
    <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('cron')} onclick={() => togglePanel('cron')} title={$t.chat.panels.cronTooltip}>{$t.chat.panels.cron}</button>
    <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('health')} onclick={() => togglePanel('health')} title={sessionHealth.summary}>{$t.chat.panels.health}{#if healthIssueCount > 0}{$t.chat.panels.healthCount(healthIssueCount)}{/if}</button>
  </div>
</div>

<style>
  /* Mini dashboard */
  .chat-pulse {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-2) var(--space-4);
    background: var(--surface);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
    position: sticky;
    top: 0;
    z-index: 10;
    min-width: 0;
    overflow: hidden;
  }

  .chat-pulse-stats {
    display: contents;
  }

  .pulse-item {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .pulse-val {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-primary);
  }
  .pulse-val.warn { color: var(--error); }
  .pulse-lbl {
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }
  .pulse-sep {
    width: 1px;
    height: 16px;
    background: var(--border-subtle);
    flex-shrink: 0;
  }

  .pulse-panel-toggles {
    display: flex;
    gap: 2px;
    flex-shrink: 0;
  }

  .pulse-toggle-btn {
    background: none;
    border: 1px solid var(--border-subtle);
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
    cursor: pointer;
    padding: 2px 8px;
    border-radius: var(--radius-sm);
    transition: all var(--duration-fast);
  }
  .pulse-toggle-btn:hover {
    color: var(--text-primary);
    border-color: var(--border-default);
  }
  .pulse-toggle-btn.active {
    color: var(--primary);
    border-color: var(--primary);
    background: rgba(224, 145, 69, 0.08);
  }

  @media (max-width: 900px) {
    .chat-pulse {
      flex-wrap: nowrap;
      gap: 0;
      padding: 0;
      flex-direction: column;
      align-items: stretch;
      overflow: visible;
    }
    .chat-pulse-stats {
      display: flex;
      align-items: center;
      gap: var(--space-3);
      padding: var(--space-1) var(--space-3);
      overflow-x: auto;
    }
    .pulse-sep { display: none; }
    .pulse-panel-toggles {
      flex-wrap: wrap;
      gap: 4px;
      padding: var(--space-1) var(--space-3) var(--space-2);
      border-top: 1px solid var(--border-subtle);
      flex-shrink: 0;
    }
  }
</style>
