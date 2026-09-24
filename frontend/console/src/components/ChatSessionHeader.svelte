<script lang="ts">
  // Session header for the chat column: title and rename, health badge,
  // goal chip, the session menu, the feedback line, and the work strip
  // (plan progress plus workbench jumps). The cwd chip lives in the status
  // bar under the composer. Session data comes from the shared store;
  // actions that other surfaces also trigger (compact, goal status,
  // workbench jumps) are delegated back to Chat.
  import { untrack } from 'svelte'
  import { deleteSession } from '../lib/api'
  import { t } from '../i18n'
  import { planProgressPercent } from '../lib/tasks'
  import { buildWorkbenchActions, type WorkbenchAction } from '../lib/workbenchActions'
  import { displaySessionTitle, shortGoalLabel } from '../lib/sessionLabels'
  import { chatSession } from '../lib/stores/chatSession'
  import { chatDock, type ChatDockPanelID } from '../lib/stores/chatDockStore.svelte'
  import { zenMode } from '../lib/zenMode.svelte'

  interface Props {
    onNewSession: () => Promise<void>
    onCompact: () => Promise<void>
    onGoalStatus: () => Promise<void>
    onWorkbenchAction: (action: WorkbenchAction) => Promise<void>
    onCopy: () => void
    onDownload: () => void
  }

  let { onNewSession, onCompact, onGoalStatus, onWorkbenchAction, onCopy, onDownload }: Props = $props()

  let selectedSessionId = $derived(chatSession.activeSessionId)
  let selectedSession = $derived(chatSession.activeSession)
  let sessionHealth = $derived(chatSession.health)
  let sessionGoal = $derived(chatSession.goal)
  let actionFeedback = $derived(chatSession.feedback)
  let tasksSummary = $derived(chatSession.tasksSummary)
  let planStripProgress = $derived(planProgressPercent(tasksSummary))
  let hasPlanStrip = $derived(!!tasksSummary.plan_goal?.trim())
  let workbenchActions = $derived(buildWorkbenchActions({
    sessionId: selectedSessionId,
    hasPlan: hasPlanStrip,
    activeTaskTitle: tasksSummary.active_task_title,
  }, $t.chatCommands.workbench))

  let renaming = $state(false)
  let renameValue = $state('')
  let actionBusy = $state(false)
  let deleteConfirm = $state(false)
  let sessionMenuOpen = $state(false)

  // Menus, rename, and delete confirmation belong to one session.
  $effect(() => {
    void chatSession.activeSessionId
    untrack(() => {
      renaming = false
      deleteConfirm = false
      sessionMenuOpen = false
    })
  })

  function isPanelOpen(panelID: ChatDockPanelID): boolean {
    return chatDock.isVisible(panelID)
  }

  function openPanel(panelID: ChatDockPanelID) {
    chatDock.open(panelID)
  }

  function togglePanel(panelID: ChatDockPanelID) {
    chatDock.toggle(panelID)
  }

  function isMainSession(): boolean {
    return selectedSession?.kind === 'main'
  }

  function startRename() {
    if (!selectedSession || selectedSession.kind === 'main') return
    renaming = true
    renameValue = selectedSession.title || selectedSession.id.slice(0, 12)
  }

  function closeSessionMenu() {
    sessionMenuOpen = false
  }

  async function commitRename() {
    if (!selectedSessionId || !renameValue.trim()) { renaming = false; return }
    actionBusy = true
    try {
      await chatSession.rename(renameValue)
    } catch { /* ignore */ }
    renaming = false
    actionBusy = false
  }

  async function handleAutoTitle() {
    if (!selectedSessionId || !selectedSession) return
    actionBusy = true
    try {
      await chatSession.autoTitle()
    } catch { /* ignore */ }
    actionBusy = false
  }

  async function handleCompact() {
    actionBusy = true
    try {
      await onCompact()
    } finally {
      actionBusy = false
    }
  }

  async function handleDelete() {
    if (!selectedSessionId) return
    if (!deleteConfirm) { deleteConfirm = true; return }
    actionBusy = true
    try {
      await deleteSession(selectedSessionId)
      await onNewSession()
    } catch { /* ignore */ }
    actionBusy = false
    deleteConfirm = false
  }

  function handleCopyChat() {
    onCopy()
  }

  function handleDownloadChat() {
    onDownload()
  }

  async function handleGoalSlashCommand(_args: 'status') {
    await onGoalStatus()
  }

  async function handleWorkbenchAction(action: WorkbenchAction) {
    await onWorkbenchAction(action)
  }
</script>

<!-- Session header with actions -->
{#if selectedSession}
  <div class="session-header">
    <div class="session-title-row">
      {#if renaming}
        <!-- svelte-ignore a11y_autofocus -->
        <input
          class="session-rename-input"
          bind:value={renameValue}
          autofocus
          onkeydown={(e) => { if (e.key === 'Enter') commitRename(); if (e.key === 'Escape') { renaming = false } }}
          onblur={() => commitRename()}
        />
      {:else}
        <h3 class="session-title">{displaySessionTitle(selectedSession.title, $t.chat.session.newChat) || selectedSession.id.slice(0, 12)}</h3>
      {/if}
      <button
        type="button"
        class={`session-health-badge health-${sessionHealth.status}`}
        onclick={() => openPanel('health')}
        title={sessionHealth.summary}
      >
        <span>{$t.chat.session.healthBadge}</span>
        <strong>{sessionHealth.badgeLabel}</strong>
      </button>
      {#if sessionGoal}
        <button
          type="button"
          class="goal-chip"
          class:satisfied={sessionGoal.status === 'satisfied'}
          class:exhausted={sessionGoal.status === 'exhausted'}
          title={$t.chatCommands.goal.chipTitle(
            $t.chatCommands.goal.statuses[sessionGoal.status] ?? sessionGoal.status,
            sessionGoal.description,
            sessionGoal.auto_continue_count,
            sessionGoal.max_auto_continues,
          )}
          onclick={() => void handleGoalSlashCommand('status')}
        >
          <span class="goal-chip-label">{$t.chatCommands.goal.chipLabel}</span>
          <strong>{shortGoalLabel(sessionGoal.description)}</strong>
          <span class="goal-chip-counter">{sessionGoal.auto_continue_count}/{sessionGoal.max_auto_continues}</span>
        </button>
      {/if}
    </div>
    <div class="session-actions">
      <button
        type="button"
        class="btn btn-ghost btn-sm zen-toggle"
        class:active={zenMode.active}
        aria-pressed={zenMode.active}
        onclick={() => zenMode.toggle()}
        title={zenMode.active ? $t.chat.session.actions.zenExitTooltip : $t.chat.session.actions.zenEnterTooltip}
      >
        {zenMode.active ? $t.chat.session.actions.zenExit : $t.chat.session.actions.zenEnter}
      </button>
      {#if !zenMode.active}
        <div class="session-menu">
          <button
            type="button"
            class="btn btn-ghost btn-sm session-menu-trigger"
            aria-haspopup="menu"
            aria-expanded={sessionMenuOpen}
            onclick={() => { sessionMenuOpen = !sessionMenuOpen }}
          >
            {$t.sessions.actions.more}
          </button>
          {#if sessionMenuOpen}
            <div class="session-menu-popover" role="menu">
              {#if !isMainSession()}
                <button type="button" role="menuitem" disabled={actionBusy} onclick={() => { closeSessionMenu(); startRename() }}>{$t.chat.session.actions.rename}</button>
                <button type="button" role="menuitem" disabled={actionBusy} onclick={() => { closeSessionMenu(); void handleAutoTitle() }} title={$t.chat.session.actions.aiTitleTooltip}>{$t.chat.session.actions.aiTitle}</button>
              {/if}
              <button type="button" role="menuitem" disabled={actionBusy} onclick={() => { closeSessionMenu(); void handleCompact() }} title={$t.chat.session.actions.compactTooltip}>{$t.chat.session.actions.compact}</button>
              <button type="button" role="menuitem" onclick={() => { closeSessionMenu(); void handleCopyChat() }} title={$t.chat.session.actions.copyAllTooltip}>{$t.chat.session.actions.copyAll}</button>
              <button type="button" role="menuitem" onclick={() => { closeSessionMenu(); handleDownloadChat() }} title={$t.chat.session.actions.downloadTooltip}>{$t.chat.session.actions.download}</button>
              <button type="button" role="menuitem" disabled={actionBusy} onclick={() => { closeSessionMenu(); openPanel('skillExtraction') }} title={$t.chat.session.actions.extractSkillTooltip}>{$t.chat.session.actions.extractSkill}</button>
              {#if !isMainSession()}
                <span class="session-menu-divider"></span>
                <button type="button" role="menuitem" class="danger" disabled={actionBusy} onclick={() => { closeSessionMenu(); void handleDelete() }}>
                  {deleteConfirm ? $t.chat.session.actions.confirmDelete : $t.chat.session.actions.delete}
                </button>
              {/if}
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>
{:else}
  <div class="session-header">
    <h3 class="session-title new-chat-title">{$t.chat.session.newChat}</h3>
    <div class="session-actions">
      <button
        type="button"
        class="btn btn-ghost btn-sm zen-toggle"
        class:active={zenMode.active}
        aria-pressed={zenMode.active}
        onclick={() => zenMode.toggle()}
        title={zenMode.active ? $t.chat.session.actions.zenExitTooltip : $t.chat.session.actions.zenEnterTooltip}
      >
        {zenMode.active ? $t.chat.session.actions.zenExit : $t.chat.session.actions.zenEnter}
      </button>
    </div>
  </div>
{/if}

{#if actionFeedback}
  <div class="action-feedback" class:multiline={actionFeedback.includes('\n')}>{actionFeedback}</div>
{/if}

<!-- Plan progress and workbench jumps share one row (#968). -->
{#if hasPlanStrip || workbenchActions.length > 0}
<div class="work-strip">
{#if hasPlanStrip}
  <button
    type="button"
    class="plan-progress-strip"
    class:active={isPanelOpen('tasks')}
    onclick={() => togglePanel('tasks')}
    title={$t.chat.planStrip.openTitle}
  >
    <span class="plan-strip-goal">
      <span class="plan-strip-label">{$t.chat.planStrip.label}</span>
      <strong>{tasksSummary.plan_goal}</strong>
      {#if tasksSummary.active_task_title}
        <span class="plan-strip-active" title={$t.chat.planStrip.activeTaskTooltip(tasksSummary.active_task_title)}>
          <span class="plan-strip-active-dot" aria-hidden="true"></span>
          {tasksSummary.active_task_title}
        </span>
      {/if}
    </span>
    <span class="plan-strip-progress">
      <span class="plan-strip-bar" aria-label={$t.chat.planStrip.progressAria(planStripProgress)}>
        <span class="plan-strip-fill" style={`width: ${planStripProgress}%`}></span>
      </span>
      <span class="plan-strip-count">{$t.chat.planStrip.tasksSuffix(tasksSummary.completed, tasksSummary.total)}</span>
    </span>
  </button>
{/if}

{#if workbenchActions.length > 0}
  <div class="workbench-action-strip" aria-label={$t.chatCommands.workbench.ariaLabel}>
    {#each workbenchActions as action}
      <button
        type="button"
        class="workbench-action"
        onclick={() => { void handleWorkbenchAction(action) }}
        title={action.title}
      >
        {action.label}
      </button>
    {/each}
  </div>
{/if}
</div>
{/if}

<style>
  .action-feedback {
    padding: 6px 14px;
    font-size: 0.82rem;
    color: var(--primary);
    background: color-mix(in srgb, var(--primary) 10%, transparent);
    border-bottom: 1px solid color-mix(in srgb, var(--primary) 25%, transparent);
    text-align: center;
  }
  .action-feedback.multiline {
    white-space: pre-wrap;
    text-align: left;
    font-family: var(--font-mono), ui-monospace, monospace;
    line-height: 1.5;
  }
  /* Session header */
  .session-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4) var(--space-2);
    flex-shrink: 0;
    min-height: 44px;
    border-bottom: 1px solid color-mix(in srgb, var(--border-subtle) 72%, transparent);
  }

  .session-title-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 1;
    min-width: 0;
  }

  .session-title {
    flex: 0 1 auto;
    min-width: 80px;
    max-width: min(36vw, 420px);
    font-family: var(--font-display);
    font-size: var(--text-base);
    font-weight: 500;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    margin: 0;
  }

  .session-health-badge {
    display: inline-flex;
    flex-shrink: 0;
    align-items: center;
    gap: var(--space-1);
    max-width: 190px;
    padding: 3px var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-base);
    color: var(--text-secondary);
    cursor: pointer;
    font-size: var(--text-xs);
    transition:
      background var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out);
  }

  .session-health-badge:hover {
    border-color: var(--border-default);
    background: var(--surface-elevated);
  }

  .session-health-badge span {
    color: var(--text-tertiary);
  }

  .session-health-badge strong {
    min-width: 0;
    overflow: hidden;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .session-health-badge.health-watch {
    border-color: color-mix(in srgb, var(--warning) 45%, var(--border-subtle));
    color: var(--warning);
  }

  .session-health-badge.health-attention,
  .session-health-badge.health-critical {
    border-color: color-mix(in srgb, var(--error) 50%, var(--border-subtle));
    color: var(--error);
  }

  /* Session goal chip — compact mono chip, tinted with the accent
     by default to flag that an autonomous goal is steering the session.
     Switches to muted styling when the goal is satisfied or exhausted. */
  .goal-chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    max-width: 280px;
    padding: 3px var(--space-2);
    border: 1px solid color-mix(in srgb, var(--primary) 55%, var(--border-subtle));
    border-radius: var(--radius-sm);
    background: color-mix(in srgb, var(--primary) 14%, var(--surface-base));
    color: var(--primary);
    cursor: pointer;
    font-size: var(--text-xs);
    font-family: var(--font-mono);
    transition:
      background var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out);
  }

  .goal-chip:hover {
    background: color-mix(in srgb, var(--primary) 22%, var(--surface-base));
  }

  .goal-chip-label {
    color: color-mix(in srgb, var(--primary) 75%, var(--text-tertiary));
    font-family: var(--font-display);
  }

  .goal-chip strong {
    min-width: 0;
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .goal-chip-counter {
    color: color-mix(in srgb, var(--primary) 70%, var(--text-tertiary));
    font-size: 10px;
  }

  .goal-chip.satisfied {
    border-color: var(--border-subtle);
    background: var(--surface-base);
    color: var(--text-tertiary);
  }

  .goal-chip.exhausted {
    border-color: color-mix(in srgb, var(--warning, var(--primary)) 50%, var(--border-subtle));
    background: color-mix(in srgb, var(--warning, var(--primary)) 10%, var(--surface-base));
    color: var(--warning, var(--primary));
  }


  .new-chat-title {
    color: var(--text-tertiary);
  }

  .session-rename-input {
    width: 100%;
    padding: var(--space-1) var(--space-2);
    font-size: var(--text-base);
    font-family: var(--font-display);
    background: var(--surface-base);
    border: 1px solid var(--primary);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    outline: none;
  }

  .session-actions {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    flex-shrink: 0;
  }

  .session-menu {
    position: relative;
    display: inline-flex;
  }

  .session-menu-trigger {
    min-width: 64px;
  }

  .session-menu-popover {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
    z-index: 55;
    display: flex;
    flex-direction: column;
    min-width: 190px;
    padding: var(--space-1);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
    box-shadow: var(--shadow-md, 0 12px 28px rgba(0, 0, 0, 0.28));
  }

  .session-menu-popover button {
    display: flex;
    align-items: center;
    width: 100%;
    min-height: 30px;
    padding: var(--space-1) var(--space-2);
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
    font-family: var(--font-display);
    font-size: var(--text-xs);
    text-align: left;
  }

  .session-menu-popover button:hover:not(:disabled) {
    background: var(--surface-base);
    color: var(--text-primary);
  }

  .session-menu-popover button:disabled {
    cursor: not-allowed;
    opacity: 0.55;
  }

  .session-menu-popover button.danger {
    color: var(--error);
  }

  .session-menu-divider {
    height: 1px;
    margin: var(--space-1) 0;
    background: var(--border-subtle);
  }

  .zen-toggle.active {
    color: var(--primary);
    border-color: var(--primary);
  }

  .work-strip {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: var(--space-2) var(--space-4);
  }

  .plan-progress-strip {
    display: flex;
    flex: 1;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    min-width: 0;
    min-height: 32px;
    padding: var(--space-1) var(--space-3);
    border: 1px solid color-mix(in srgb, var(--border-subtle) 78%, transparent);
    border-radius: var(--radius-md);
    background: color-mix(in srgb, var(--surface) 70%, transparent);
    color: var(--text-primary);
    cursor: pointer;
    text-align: left;
    transition:
      background var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out);
  }

  .plan-progress-strip:hover,
  .plan-progress-strip.active {
    border-color: var(--primary);
    background: color-mix(in srgb, var(--primary) 8%, var(--surface));
  }

  .plan-strip-goal {
    display: flex;
    min-width: 0;
    align-items: center;
    gap: var(--space-2);
  }

  .plan-strip-label {
    flex-shrink: 0;
    padding: 2px var(--space-2);
    border-radius: var(--radius-sm);
    background: var(--primary-muted);
    color: var(--primary-text);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 600;
  }

  .plan-strip-goal strong {
    min-width: 0;
    overflow: hidden;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .plan-strip-active {
    display: inline-flex;
    flex-shrink: 1;
    align-items: center;
    gap: var(--space-1);
    min-width: 0;
    overflow: hidden;
    color: var(--text-secondary);
    font-size: var(--text-xs);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .plan-strip-active-dot {
    display: inline-block;
    flex-shrink: 0;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent);
    animation: plan-strip-active-pulse 1.4s ease-in-out infinite;
  }

  @keyframes plan-strip-active-pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }

  .plan-strip-progress {
    display: flex;
    flex-shrink: 0;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .plan-strip-bar {
    display: block;
    width: 86px;
    height: 6px;
    overflow: hidden;
    border-radius: 999px;
    background: var(--surface-inset);
  }

  .plan-strip-fill {
    display: block;
    height: 100%;
    border-radius: inherit;
    background: var(--primary);
    transition: width 0.3s var(--ease-out);
  }

  .plan-strip-count {
    min-width: 56px;
    font-family: var(--font-mono);
    text-align: right;
  }

  .workbench-action-strip {
    display: flex;
    flex: 0 1 auto;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: var(--space-1);
    margin-left: auto;
  }

  .workbench-action {
    min-height: 26px;
    padding: 0 var(--space-2);
    border: 1px solid color-mix(in srgb, var(--border-subtle) 76%, transparent);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-tertiary);
    cursor: pointer;
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 600;
    transition:
      background var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out),
      color var(--duration-fast) var(--ease-out);
  }

  .workbench-action:hover {
    border-color: var(--primary);
    background: color-mix(in srgb, var(--primary) 7%, var(--surface-elevated));
    color: var(--text-primary);
  }

  @media (max-width: 900px) {
    .session-header {
      flex-wrap: wrap;
      align-items: flex-start;
    }
    .session-title-row {
      flex-basis: 1px;
      min-width: 0;
    }
    .session-title {
      max-width: none;
    }
    .session-actions {
      flex-wrap: nowrap;
      justify-content: flex-end;
    }
    .session-menu-popover {
      right: 0;
      max-width: calc(100vw - var(--space-6));
    }
    .work-strip {
      flex-wrap: wrap;
    }
    .plan-progress-strip {
      align-items: flex-start;
      flex-direction: column;
    }
    .plan-strip-progress {
      width: 100%;
    }
    .plan-strip-bar {
      flex: 1;
      width: auto;
    }
  }
</style>
