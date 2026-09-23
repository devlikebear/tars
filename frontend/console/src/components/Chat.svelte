<script lang="ts">
  // Chat route: wires the session store to the route and composes the
  // workbench — toolbar, dock host, and the conversation column (session
  // header plus ChatPanel). Slash commands and the actions several surfaces
  // share (compact, goal, cwd, session switches) live here.
  import { untrack } from 'svelte'
  import {
    getCodexUsage,
    createSession,
    listChatTools, getSessionEffectiveConfig, updateSessionLocalConfig,
    getSessionGoal, setSessionGoal, clearSessionGoal,
    type SessionToolConfig,
  } from '../lib/api'
  import { formatCodexStatusLines } from '../lib/codexStatus'
  import { t } from '../i18n'
  import type { SessionHealthAction } from '../lib/sessionHealth'
  import type { WorkbenchAction } from '../lib/workbenchActions'
  import type { Session } from '../lib/types'
  import { shortCwdLabel } from '../lib/sessionLabels'
  import { isArchived } from '../lib/sessionOrganization'
  import { chatSession } from '../lib/stores/chatSession'
  import { chatDock, isMobileLayout, type ChatDockPanelID } from '../lib/stores/chatDockStore.svelte'
  import { chatCommands, type ChatCommand } from '../lib/stores/chatCommandQueue.svelte'
  import { loadChatComponent } from '../lib/chatComponents'
  import ChatRail from './ChatRail.svelte'
  import ChatStatusBar from './ChatStatusBar.svelte'
  import ChatSessionHeader from './ChatSessionHeader.svelte'
  import ChatDockHost from './ChatDockHost.svelte'

  interface Props {
    sessionId?: string
    onNavigate: (path: string) => void
    initialPrompt?: string
  }

  let { sessionId, onNavigate, initialPrompt }: Props = $props()

  // The route owns which session is active; the shared store owns its state.
  $effect(() => {
    const sid = sessionId
    untrack(() => chatSession.setActive(sid))
  })

  let selectedSessionId = $derived(chatSession.activeSessionId)
  let selectedSession = $derived(chatSession.activeSession)
  let cwdState = $derived(chatSession.cwd)
  let cwdBusy = $derived(chatSession.cwdBusy)

  type ChatPanelHandle = {
    sendMessageText: (text: string) => Promise<void>
    clearThread: () => void
    exportAsMarkdown: () => string
  }
  let chatPanelRef: ChatPanelHandle | undefined = $state()
  let dockHost: { notifyToolComplete: (toolName: string) => void; openArtifact: (path: string) => Promise<void>; openEvidence: () => Promise<void>; toggleTerminal: () => void } | undefined = $state()

  function openPanel(panelID: ChatDockPanelID) {
    chatDock.open(panelID)
  }

  function closePanel(panelID: ChatDockPanelID) {
    chatDock.close(panelID)
  }

  function showFeedback(msg: string, ms = 4000) {
    chatSession.notify(msg, ms)
  }

  function refreshSessionHealth(): Promise<void> {
    return chatSession.refreshHealth()
  }

  // Resolves true when the switch succeeded.
  async function transitionCwd(target: string): Promise<boolean> {
    if (!selectedSessionId) {
      showFeedback('Select a session first')
      return false
    }
    if (cwdBusy) return false
    try {
      await chatSession.setCwd(target)
      showFeedback(`cwd → ${shortCwdLabel(target)}`)
      return true
    } catch (err) {
      showFeedback(`cwd transition failed: ${err instanceof Error ? err.message : String(err)}`)
      return false
    }
  }

  // Leaving the current session closes its tool panels (and, on mobile, the
  // session list overlay). The store and header reset their own per-session
  // state when the route change lands.
  function resetSessionChrome() {
    chatDock.closeToolPanels()
    if (isMobileLayout()) {
      closePanel('sessions')
    }
  }

  function handleSelectSession(session: Session) {
    resetSessionChrome()
    onNavigate(`/console/chat/${encodeURIComponent(session.id)}`)
  }

  async function handleNewSession() {
    let created: Session | null = null
    try {
      created = await createSession()
    } catch {
      created = null
      // The route may not change, so reload the thread explicitly.
      chatSession.remountThread()
    }
    resetSessionChrome()
    void chatSession.refreshSessions()
    onNavigate(created ? `/console/chat/${encodeURIComponent(created.id)}` : '/console/chat')
  }

  function openSessionById(id: string) {
    if (id === selectedSessionId) return
    resetSessionChrome()
    onNavigate(`/console/chat/${encodeURIComponent(id)}`)
  }

  // Requests queued by the command palette and global shortcuts (App.svelte).
  async function runChatCommand(command: ChatCommand) {
    switch (command.kind) {
      case 'slash':
        await handleSlashCommand(command.command, command.args ?? '')
        return
      case 'new-session':
        await handleNewSession()
        return
      case 'toggle-terminal':
        dockHost?.toggleTerminal()
        return
      case 'switch-session': {
        const fallback = chatSession.sessions.filter((session) => !isArchived(session)).map((session) => session.id)
        const id = chatCommands.sessionAt(command.index, fallback)
        if (id) openSessionById(id)
        return
      }
    }
  }

  $effect(() => {
    if (chatCommands.pending.length === 0) return
    const commands = untrack(() => chatCommands.take())
    void (async () => {
      for (const command of commands) await runChatCommand(command)
    })()
  })

  function handleSessionForked(session: Session) {
    resetSessionChrome()
    void chatSession.refreshSessions()
    showFeedback(`Forked session: ${session.title || session.id.slice(0, 12)}`)
    onNavigate(`/console/chat/${encodeURIComponent(session.id)}`)
  }

  // Surface the Files panel the first time a turn produces artifacts, unless
  // the user already has a tool panel open.
  $effect(() => {
    const artifacts = chatSession.artifacts
    untrack(() => {
      if (artifacts.length > 0 && !chatDock.anyToolPanelOpen) openPanel('artifacts')
    })
  })

  async function handleCompact() {
    if (!selectedSessionId) return
    try {
      const r = await chatSession.compact()
      if (r?.compacted) {
        const saved = r.tokens_before - r.tokens_after
        const pct = r.tokens_before > 0 ? Math.round((saved / r.tokens_before) * 100) : 0
        showFeedback($t.chat.feedback.compacted(r.compacted_count, r.original_count, r.final_count, pct))
      } else if (r) {
        showFeedback(r.reason || $t.chat.feedback.nothingToCompact)
      }
    } catch (e) {
      showFeedback(e instanceof Error ? e.message : $t.chat.feedback.compactFailed)
    }
  }

  function handleToolComplete(toolName: string) {
    dockHost?.notifyToolComplete(toolName)
  }

  async function handleArtifactOpen(path: string) {
    await dockHost?.openArtifact(path)
  }

  async function handleWorkbenchAction(action: WorkbenchAction) {
    if (action.id === 'agentruntime') {
      onNavigate('/console/agentruntime')
      return
    }
    if (action.id === 'git') {
      openPanel('git')
      return
    }
    if (action.id === 'evidence') {
      await dockHost?.openEvidence()
      return
    }
    openPanel('tasks')
  }

  async function handleHealthAction(action: SessionHealthAction) {
    switch (action) {
      case 'compact':
        await handleCompact()
        return
      case 'open_tasks':
        openPanel('tasks')
        return
      case 'open_config':
        openPanel('config')
        return
      case 'open_prior':
        openPanel('prior')
        return
      case 'open_skill_extraction':
        openPanel('skillExtraction')
        return
      case 'review_fork_points':
        closePanel('health')
        return
    }
  }

  async function handleSlashCommand(command: string, _args: string) {
    const args = _args.trim()
    switch (command) {
      case 'clear':
        chatPanelRef?.clearThread()
        showFeedback('Chat view cleared')
        return
      case 'compact':
        if (!selectedSessionId) {
          showFeedback('Select a session first')
          return
        }
        await handleCompact()
        return
      case 'tasks':
        if (!selectedSessionId) {
          showFeedback('Select a session first')
          return
        }
        openPanel('tasks')
        return
      case 'config':
        if (!selectedSessionId) {
          showFeedback('Select a session first')
          return
        }
        openPanel('config')
        return
      case 'context':
        openPanel('context')
        return
      case 'prior':
        openPanel('prior')
        return
      case 'prompt':
        openPanel('prompt')
        return
      case 'files':
        openPanel('artifacts')
        return
      case 'cron':
        if (!selectedSessionId) {
          showFeedback('Select a session first')
          return
        }
        openPanel('cron')
        return
      case 'memory':
        {
          const query = memorySearchQueryFromSlashArgs(args)
          if (query) {
            onNavigate(`/console/memory?tab=search&q=${encodeURIComponent(query)}`)
            return
          }
          onNavigate('/console/memory')
        }
        return
      case 'skill':
        await toggleSessionSkill(args)
        return
      case 'extract-skill':
        if (!selectedSessionId) {
          showFeedback('Select a session first')
          return
        }
        openPanel('skillExtraction')
        return
      case 'status': {
        try {
          const res = await getCodexUsage()
          const lines = formatCodexStatusLines(res.tiers ?? [])
          showFeedback(lines.join('\n'), 10_000)
        } catch (err) {
          showFeedback(`status: ${err instanceof Error ? err.message : 'failed to load codex quota'}`)
        }
        return
      }
      case 'cwd':
        if (!selectedSessionId) {
          showFeedback('Select a session first')
          return
        }
        if (args === '' || args.toLowerCase() === 'list') {
          await chatSession.refreshCwd()
          if (!cwdState) {
            showFeedback('cwd: no eligible directories')
            return
          }
          const eligible = cwdState.eligible.length
            ? cwdState.eligible.map((p, i) => `  ${i + 1}. ${shortCwdLabel(p)}${p === cwdState!.current ? ' (active)' : ''}`).join('\n')
            : '  (none)'
          showFeedback(`cwd active: ${shortCwdLabel(cwdState.current)}\n${eligible}`)
          return
        }
        await transitionCwd(args)
        return
      case 'goal':
        if (!selectedSessionId) {
          showFeedback('Select a session first')
          return
        }
        await handleGoalSlashCommand(args)
        return
    }
  }

  async function handleGoalSlashCommand(args: string) {
    if (!selectedSessionId) return
    const trimmed = args.trim()
    const lower = trimmed.toLowerCase()
    try {
      if (trimmed === '' || lower === 'status' || lower === 'show') {
        const resp = await getSessionGoal(selectedSessionId)
        chatSession.setGoal(resp.goal)
        if (!resp.goal) {
          showFeedback('goal: (none) — usage: /goal <description> | /goal clear')
          return
        }
        const remaining = Math.max(resp.goal.max_auto_continues - resp.goal.auto_continue_count, 0)
        showFeedback(
          `goal [${resp.goal.status}]: ${resp.goal.description}\n  auto-continues remaining: ${remaining}/${resp.goal.max_auto_continues}`,
        )
        return
      }
      if (lower === 'clear' || lower === 'cancel') {
        const resp = await clearSessionGoal(selectedSessionId)
        chatSession.setGoal(resp.goal)
        showFeedback('goal cleared')
        return
      }
      const resp = await setSessionGoal(selectedSessionId, trimmed)
      chatSession.setGoal(resp.goal)
      if (resp.goal) {
        showFeedback(`goal set: ${resp.goal.description}`)
      } else {
        showFeedback('goal cleared (empty description)')
      }
    } catch (err) {
      showFeedback(`goal: ${err instanceof Error ? err.message : 'failed'}`)
    }
  }

  function memorySearchQueryFromSlashArgs(args: string): string {
    const trimmed = args.trim()
    if (!trimmed) return ''
    const searchPrefix = trimmed.match(/^search\s+([\s\S]+)$/i)
    return (searchPrefix?.[1] ?? trimmed).trim()
  }

  async function toggleSessionSkill(args: string) {
    if (!selectedSessionId) {
      showFeedback('Select a session first')
      return
    }
    const requested = args.trim()
    if (!requested) {
      openPanel('config')
      showFeedback('Usage: /skill <name>')
      return
    }
    try {
      const [toolsResp, config] = await Promise.all([
        listChatTools(selectedSessionId),
        getSessionEffectiveConfig(selectedSessionId),
      ])
      const skills = toolsResp.skills ?? []
      const match = skills.find((skill) => skill.toLowerCase() === requested.toLowerCase())
      if (!match) {
        showFeedback(`Skill not found: ${requested}`)
        return
      }
      const effectiveToolConfig = config.effective.tool_config
      const useCustomSkills = effectiveToolConfig.skills_custom || Array.isArray(effectiveToolConfig.skills_enabled)
      const enabledSkills = new Set(useCustomSkills ? (effectiveToolConfig.skills_enabled ?? []) : skills)
      const wasEnabled = enabledSkills.has(match)
      if (wasEnabled) {
        enabledSkills.delete(match)
      } else {
        enabledSkills.add(match)
      }
      const nextConfig: SessionToolConfig = {
        ...effectiveToolConfig,
        skills_custom: true,
        skills_enabled: [...enabledSkills],
      }
      await updateSessionLocalConfig(selectedSessionId, nextConfig)
      showFeedback(`Skill ${match} ${wasEnabled ? 'disabled' : 'enabled'}`)
      openPanel('config')
      await refreshSessionHealth()
    } catch (err) {
      showFeedback(err instanceof Error ? err.message : 'Skill toggle failed')
    }
  }

  function handleCopyChat() {
    const md = chatPanelRef?.exportAsMarkdown()
    if (md) navigator.clipboard.writeText(md).catch(() => {})
  }

  function handleDownloadChat() {
    const md = chatPanelRef?.exportAsMarkdown()
    if (!md) return
    const title = selectedSession?.title || 'chat'
    const blob = new Blob([md], { type: 'text/markdown' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${title.replace(/[^a-zA-Z0-9가-힣-_ ]/g, '').slice(0, 50)}.md`
    a.click()
    URL.revokeObjectURL(url)
  }
</script>

<div class="chat-page">

  <ChatDockHost
    bind:this={dockHost}
    onSelectSession={handleSelectSession}
    onNewSession={handleNewSession}
    onSendMessage={async (text) => { await chatPanelRef?.sendMessageText(text) }}
    onHealthAction={handleHealthAction}
  >
    <main class="chat-main">
      <ChatSessionHeader
        onNewSession={handleNewSession}
        onCompact={handleCompact}
        onGoalStatus={() => handleGoalSlashCommand('status')}
        onWorkbenchAction={handleWorkbenchAction}
        onCopy={handleCopyChat}
        onDownload={handleDownloadChat}
      />

      {#key chatSession.threadVersion}
        {#await loadChatComponent('chat-panel')}
          <div class="chat-panel-loading">Loading...</div>
        {:then module}
          {@const ChatPanelRoute = module.default}
          <ChatPanelRoute
            bind:this={chatPanelRef}
            sessionId={selectedSessionId || undefined}
            {initialPrompt}
            onToolComplete={handleToolComplete}
            onSlashCommand={handleSlashCommand}
            onSessionForked={handleSessionForked}
            onArtifactOpen={handleArtifactOpen}
          />
        {:catch}
          <div class="chat-panel-loading">Could not load chat panel.</div>
        {/await}
      {/key}

      <ChatStatusBar onCwdSelect={transitionCwd} />
    </main>
  </ChatDockHost>
  <ChatRail />
</div>

<style>
  .chat-panel-loading {
    flex: 1;
    min-height: 260px;
    display: grid;
    place-items: center;
    color: var(--text-secondary);
    font-family: var(--font-display);
  }
  /* Dock host on the left, the panel rail on the right edge. */
  .chat-page {
    display: flex;
    flex-direction: row;
    flex: 1;
    min-height: 0;
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .chat-main {
    grid-area: main;
    display: flex;
    flex-direction: column;
    min-height: 0;
    min-width: 0;
    padding: var(--space-4);
    padding-top: 0;
    overflow: hidden;
  }

  @media (max-width: 900px) {
    /* The rail turns into a row above the chat. */
    .chat-page {
      flex-direction: column-reverse;
    }
  }
</style>
