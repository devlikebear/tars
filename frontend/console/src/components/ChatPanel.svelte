<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte'
  import { t } from '../i18n'
  import { streamChat, cancelChat, getSessionHistory, renameSession, streamEvents, listChatFileMentions, listAgentRuntimeSubagents, listSkills, listChatTools, getSessionEffectiveConfig, forkSessionFromMessage } from '../lib/api'
  import type { AgentRuntimeSubagent, ChatAttachment, ChatEvent, ChatTier, ChatTierRecommendationRequest, CommandDef, Session, SessionMessage, SkillDef } from '../lib/types'
  import { extractArtifact, extractArtifactsFromHistory, mergeArtifact, type Artifact } from '../lib/artifacts'
  import { buildTierRecommendation, tierRecommendationPayload, type TierRecommendation } from '../lib/tierRecommendation'
  import {
    applyMentionCandidate,
    buildSubagentMentionCandidates,
    filterSelectedMentionsForMessage,
    findActiveMentionTrigger,
    type ActiveMentionTrigger,
    type ChatMentionCandidate,
    type SelectedChatMention,
  } from '../lib/chatMentions'
  import {
    applySlashCandidate,
    buildSlashCandidates,
    builtinSlashCommandId,
    findActiveSlashTrigger,
    parseLeadingSlashCommand,
    type ActiveSlashTrigger,
    type SlashCommandCandidate,
  } from '../lib/slash'
  import type { ChatMessage } from '../lib/chatMessages'
  import ChatMessageItem from './ChatMessageItem.svelte'
  import SlashPopover from './SlashPopover.svelte'

  interface Props {
    sessionId?: string
    initialPrompt?: string
    autoSend?: boolean
    onSessionChange?: () => void
    onArtifactsChange?: (artifacts: Artifact[]) => void
    onContextInfo?: (info: {
      system_prompt_tokens?: number
      history_tokens?: number
      history_messages?: number
      tool_count?: number
      tool_names?: string[]
      skill_count?: number
      skill_names?: string[]
      command_count?: number
      command_names?: string[]
      memory_count?: number
      memory_tokens?: number
      compaction_trigger_tokens?: number
      compaction_keep_recent_tokens?: number
      compaction_keep_recent_fraction?: number
      compaction_last_mode?: string
      used_tool_names?: string[]
      selected_skill_name?: string
      selected_skill_reason?: string
      selected_command_name?: string
      selected_command_reason?: string
      mentioned_path_count?: number
      mentioned_paths?: string[]
      mentioned_subagent_count?: number
      mentioned_subagents?: string[]
      llm_tier?: string
      tier_recommendation?: ChatTierRecommendationRequest
    }) => void
    onToolComplete?: (toolName: string) => void
    onSessionReady?: (sessionId: string) => void
    onArtifactOpen?: (path: string) => void
    onTasksChanged?: (summary: TasksSummary) => void
    onSlashCommand?: (command: string, args: string) => void | Promise<void>
    onDraftChange?: (draft: string) => void
    onSessionForked?: (session: Session) => void
  }

  type TasksSummary = {
    total: number
    pending: number
    in_progress: number
    completed: number
    cancelled: number
    plan_goal?: string
  }

  let { sessionId, initialPrompt, autoSend, onSessionChange, onArtifactsChange, onContextInfo, onToolComplete, onSessionReady, onArtifactOpen, onTasksChanged, onSlashCommand, onDraftChange, onSessionForked }: Props = $props()

  let artifacts: Artifact[] = $state([])

  let chatInput = $state('')
  let chatBusy = $state(false)
  let chatError = $state('')
  let chatSessionId = $state('')
  let chatStatusLine = $state('')
  let chatStatusPhase = $state('')
  let chatStatusMessage = $state('')
  let chatStatusTool = $state('')
  let chatStatusSkill = $state('')
  let chatStatusPhaseStartAt = $state(0)
  let chatStatusElapsedMs = $state(0)
  let chatStatusTicker: ReturnType<typeof setInterval> | null = $state(null)
  const CHAT_STATUS_PROGRESS_STEPS = ['connecting', 'loop_start', 'before_llm', 'tool', 'after_llm', 'done'] as const
  type ChatStatusLocale = 'ko' | 'en'
  function getDefaultChatStatusLocale(): ChatStatusLocale {
    if (typeof navigator === 'undefined') return 'ko'
    return navigator.language.toLowerCase().startsWith('en') ? 'en' : 'ko'
  }
  let chatStatusLocale: ChatStatusLocale = $state(getDefaultChatStatusLocale())
  let chatMessages: ChatMessage[] = $state([])
  let autoTitled = $state(false)
  let autoSendDone = false
  let abortController: AbortController | null = $state(null)
  let contextInfo: {
    system_prompt_tokens?: number
    history_tokens?: number
    history_messages?: number
    tool_count?: number
    tool_names?: string[]
    skill_count?: number
    skill_names?: string[]
    command_count?: number
    command_names?: string[]
    memory_count?: number
    memory_tokens?: number
    compaction_trigger_tokens?: number
    compaction_keep_recent_tokens?: number
    compaction_keep_recent_fraction?: number
    compaction_last_mode?: string
    used_tool_names?: string[]
    selected_skill_name?: string
    selected_skill_reason?: string
    selected_command_name?: string
    selected_command_reason?: string
    mentioned_path_count?: number
    mentioned_paths?: string[]
    mentioned_subagent_count?: number
    mentioned_subagents?: string[]
    llm_tier?: string
    tier_recommendation?: ChatTierRecommendationRequest
  } = $state({})
  let pendingTierRecommendation: TierRecommendation | null = $state(null)
  let pendingTierMessage = $state('')

  function publishContextInfo(next: typeof contextInfo) {
    contextInfo = next
    onContextInfo?.(next)
  }

  function stopChatStatusTicker() {
    if (chatStatusTicker) {
      clearInterval(chatStatusTicker)
      chatStatusTicker = null
    }
    chatStatusElapsedMs = 0
    chatStatusPhaseStartAt = 0
  }

  function startChatStatusTicker() {
    stopChatStatusTicker()
    chatStatusPhaseStartAt = Date.now()
    chatStatusElapsedMs = 0
    chatStatusTicker = setInterval(() => {
      if (!chatBusy) {
        stopChatStatusTicker()
        return
      }
      chatStatusElapsedMs = Date.now() - chatStatusPhaseStartAt
    }, 300)
  }

  function getStatusStepPhase(phase: string): (typeof CHAT_STATUS_PROGRESS_STEPS)[number] | '' {
    if (!phase) return ''
    if (phase === 'before_tool_call' || phase === 'after_tool_call') return 'tool'
    if (phase === 'connecting' || phase === 'loop_start' || phase === 'before_llm' || phase === 'after_llm' || phase === 'done') {
      return phase
    }
    return ''
  }

  function getCurrentStatusStepIndex(): number {
    const step = getStatusStepPhase(chatStatusPhase)
    if (!step) return -1
    return CHAT_STATUS_PROGRESS_STEPS.indexOf(step)
  }

  function getStatusStepLabel(step: (typeof CHAT_STATUS_PROGRESS_STEPS)[number], toolName: string, isEn: boolean): string {
    const t = (ko: string, en: string) => (isEn ? en : ko)
    switch (step) {
      case 'connecting':
        return t('요청 전송', 'Connecting')
      case 'loop_start':
        return t('추론 시작', 'Starting')
      case 'before_llm':
        return t('LLM 추론', 'LLM Thinking')
      case 'tool':
        return toolName ? toolName : t('도구 호출', 'Tool Call')
      case 'after_llm':
        return t('응답 정리', 'Assembling')
      case 'done':
        return t('완료', 'Done')
    }
  }

  function formatStatusElapsed(ms: number): string {
    const totalSec = Math.max(0, Math.floor(ms / 1000))
    const m = Math.floor(totalSec / 60)
    const s = totalSec % 60
    if (m > 0) {
      return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
    }
    return `${String(s).padStart(2, '0')}s`
  }

  function applyChatStatus(opts: {
    phase?: string
    message?: string
    toolName?: string
    skillName?: string
  }) {
    const phase = opts.phase?.trim() ?? ''
    const previousPhase = chatStatusPhase
    chatStatusPhase = phase
    chatStatusMessage = opts.message?.trim() ?? ''
    chatStatusTool = opts.toolName?.trim() ?? ''
    chatStatusSkill = opts.skillName?.trim() ?? ''
    chatStatusLine = [phase, chatStatusMessage, chatStatusTool, chatStatusSkill].filter(Boolean).join(' · ')

    if (!phase) {
      stopChatStatusTicker()
      return
    }
    if (phase !== previousPhase) {
      if (chatBusy) {
        startChatStatusTicker()
      } else if (['connecting', 'loop_start', 'before_llm', 'before_tool_call', 'after_tool_call', 'after_llm', 'done', 'cancelled'].includes(phase)) {
        chatStatusElapsedMs = 0
        chatStatusPhaseStartAt = Date.now()
      }
    }
  }

  function getChatStatusLabel(): string {
    const phase = chatStatusPhase
    const msg = chatStatusMessage
    const toolName = chatStatusTool
    const skillName = chatStatusSkill
    const isEn = chatStatusLocale === 'en'
    const t = (ko: string, en: string) => (isEn ? en : ko)

    if (!phase) return msg || chatStatusLine

    switch (phase) {
      case 'connecting':
        return t('요청 전송 중', 'Sending request')
      case 'loop_start':
        return t('추론 시작 중', 'Starting reasoning')
      case 'before_llm':
        return t('LLM 응답 생성 중', 'Generating LLM response')
      case 'after_llm':
        return t('응답 정리 중', 'Preparing final response')
      case 'before_tool_call':
        return toolName
          ? isEn
            ? `${toolName} running`
            : `${toolName} 도구 실행 중`
          : t('도구 실행 중', 'Running tool')
      case 'after_tool_call':
        return toolName
          ? isEn
            ? `${toolName} applying result`
            : `${toolName} 도구 결과 반영 중`
          : t('도구 결과 반영 중', 'Applying tool result')
      case 'skill_selected':
        return skillName ? (isEn ? `${skillName} skill selected` : `${skillName} skill 선택`) : (msg || t('skill 선택', 'Skill selected'))
      case 'command_selected':
        return msg || t('명령 선택', 'Command selected')
      case 'compaction':
        return `${t('대화 압축 중', 'Compacting conversation')}${msg ? ` · ${msg}` : ''}`
      case 'slash':
        return chatStatusLine || msg || t('명령 실행 중', 'Running command')
      case 'done':
        return t('응답 완료', 'Response complete')
      case 'cancelled':
        return t('요청 취소됨', 'Request cancelled')
      default:
        return msg || phase
    }
  }

  function toggleChatStatusLocale() {
    chatStatusLocale = chatStatusLocale === 'ko' ? 'en' : 'ko'
  }

  let streamingAssistantId = $derived.by(() => {
    if (!chatBusy) return ''
    for (let i = chatMessages.length - 1; i >= 0; i--) {
      const m = chatMessages[i]
      if (m.role === 'assistant') {
        return m.text ? '' : m.id
      }
    }
    return ''
  })

  let streamingStatus = $derived.by(() => {
    if (!chatBusy || !chatStatusLine) return null
    return {
      label: getChatStatusLabel(),
      elapsedLabel: formatStatusElapsed(chatStatusElapsedMs),
      steps: CHAT_STATUS_PROGRESS_STEPS,
      currentStepIndex: getCurrentStatusStepIndex(),
      stepLabels: CHAT_STATUS_PROGRESS_STEPS.map((step) =>
        getStatusStepLabel(step, chatStatusTool, chatStatusLocale === 'en'),
      ),
      locale: chatStatusLocale,
      onToggleLocale: toggleChatStatusLocale,
    }
  })

  function addUsedToolName(toolName?: string) {
    const normalized = toolName?.trim()
    if (!normalized) return
    const used = new Set(contextInfo.used_tool_names ?? [])
    used.add(normalized)
    publishContextInfo({
      ...contextInfo,
      used_tool_names: [...used],
    })
  }

  // One-shot auto-send: fires once when autoSend becomes true with a prompt
  $effect(() => {
    if (autoSend && initialPrompt && !autoSendDone && !chatBusy) {
      autoSendDone = true
      chatInput = initialPrompt
      tick().then(() => submitChat({ allowPrompt: false }))
    }
  })

  $effect(() => {
    onDraftChange?.(chatInput)
  })

  let chatLogEl: HTMLDivElement | undefined = $state()
  let autoScroll = $state(true)

  function handleScroll() {
    if (!chatLogEl) return
    const threshold = 40
    autoScroll = chatLogEl.scrollTop + chatLogEl.clientHeight >= chatLogEl.scrollHeight - threshold
  }

  async function scrollToBottom() {
    if (!autoScroll || !chatLogEl) return
    await tick()
    chatLogEl.scrollTop = chatLogEl.scrollHeight
  }

  function syncSessionId(nextSessionId: string | undefined) {
    const resolved = nextSessionId?.trim()
    if (!resolved || resolved === chatSessionId) return
    chatSessionId = resolved
    onSessionReady?.(resolved)
    void reloadSlashSkillsAndCandidates()
  }

  function handleChatEvent(event: ChatEvent, assistantRef: { id: string }) {
    syncSessionId(event.session_id)

    switch (event.type) {
      case 'status':
        if (event.phase === 'before_tool_call' && event.tool_name) {
          addUsedToolName(event.tool_name)
          const toolMsg: ChatMessage = {
            id: `tool-${event.tool_call_id || Date.now()}`,
            role: 'tool',
            text: '',
            toolName: event.tool_name,
            toolCallId: event.tool_call_id,
            toolArgs: event.tool_args_preview,
            toolDone: false,
            toolStartedAt: Date.now(),
          }
          const aIdx = chatMessages.findIndex((m) => m.id === assistantRef.id)
          if (aIdx >= 0) {
            const placeholder = chatMessages[aIdx]
            const hasContent =
              (placeholder.text?.trim() || '').length > 0 ||
              (placeholder.reasoningText?.trim() || '').length > 0
            if (hasContent) {
              // Freeze the current bubble, append tool after it, and start a
              // new placeholder so subsequent text streams into a fresh bubble.
              // This preserves the chronological "text -> tool -> text" order
              // instead of collapsing all intermediate text into one final bubble.
              const newId = `assistant-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`
              chatMessages = [
                ...chatMessages.slice(0, aIdx + 1),
                toolMsg,
                { id: newId, role: 'assistant', text: '' },
                ...chatMessages.slice(aIdx + 1),
              ]
              assistantRef.id = newId
            } else {
              chatMessages.splice(aIdx, 0, toolMsg)
              chatMessages = [...chatMessages]
            }
            void scrollToBottom()
          }
        } else if (event.phase === 'after_tool_call' && event.tool_call_id) {
          addUsedToolName(event.tool_name)
          const tIdx = chatMessages.findIndex((m) => m.toolCallId === event.tool_call_id)
          if (tIdx >= 0) {
            const toolArgs = event.tool_args_preview || chatMessages[tIdx].toolArgs
            chatMessages[tIdx] = {
              ...chatMessages[tIdx],
              toolArgs,
              toolResult: event.tool_result_preview,
              toolDone: true,
              toolIsError: event.tool_is_error,
              toolFinishedAt: Date.now(),
            }
            chatMessages = [...chatMessages]
            void scrollToBottom()

            // Track artifacts
            const artifact = extractArtifact(
              event.tool_name || '',
              event.tool_call_id,
              toolArgs,
              event.tool_result_preview,
              chatSessionId || sessionId,
            )
            if (artifact) {
              artifacts = mergeArtifact(artifacts, artifact, chatSessionId || sessionId)
              onArtifactsChange?.(artifacts)
            }

            onToolComplete?.(event.tool_name || '')
            if (mentionOpen) {
              void refreshMentionCandidates()
            }
            if ((event.tool_name || '').trim() === 'project_skill') {
              void reloadSlashSkillsAndCandidates()
            }
          }
        } else if (event.phase === 'skill_selected' && event.skill_name) {
          publishContextInfo({
            ...contextInfo,
            selected_skill_name: event.skill_name,
            selected_skill_reason: event.skill_reason,
          })
          const skillMsg: ChatMessage = {
            id: `skill-${Date.now()}`,
            role: 'system',
            text: `skill selected: ${event.skill_name}`,
          }
          const aIdx = chatMessages.findIndex((m) => m.id === assistantRef.id)
          if (aIdx >= 0) {
            chatMessages.splice(aIdx, 0, skillMsg)
            chatMessages = [...chatMessages]
          }
        } else if (event.phase === 'command_selected' && event.command_name) {
          publishContextInfo({
            ...contextInfo,
            selected_command_name: event.command_name,
            selected_command_reason: event.command_reason,
          })
          const commandMsg: ChatMessage = {
            id: `command-${Date.now()}`,
            role: 'system',
            text: `command selected: ${event.command_name}`,
          }
          const aIdx = chatMessages.findIndex((m) => m.id === assistantRef.id)
          if (aIdx >= 0) {
            chatMessages.splice(aIdx, 0, commandMsg)
            chatMessages = [...chatMessages]
          }
        }
        applyChatStatus({
          phase: event.phase,
          message: event.message,
          toolName: event.tool_name,
          skillName: event.skill_name,
        })
        break
      case 'delta': {
        const chunk = event.text ?? ''
        if (!chunk) break
        const idx = chatMessages.findIndex((m) => m.id === assistantRef.id)
        if (idx >= 0) {
          chatMessages[idx] = { ...chatMessages[idx], text: chatMessages[idx].text + chunk }
          chatMessages = [...chatMessages]
          void scrollToBottom()
        }
        break
      }
      case 'reasoning_delta': {
        const chunk = event.text ?? ''
        if (!chunk) break
        const idx = chatMessages.findIndex((m) => m.id === assistantRef.id)
        if (idx >= 0) {
          const prev = chatMessages[idx].reasoningText ?? ''
          chatMessages[idx] = { ...chatMessages[idx], reasoningText: prev + chunk }
          chatMessages = [...chatMessages]
          void scrollToBottom()
        }
        break
      }
      case 'tool_output_line': {
        // Streamed stdout/stderr line from a running tool (currently exec).
        // Append to the matching tool message so the user sees progress
        // instead of a 30-second-silent freeze.
        const callId = event.tool_call_id
        if (!callId) break
        const idx = chatMessages.findIndex((m) => m.toolCallId === callId)
        if (idx >= 0) {
          const prev = chatMessages[idx].toolOutputLines ?? []
          chatMessages[idx] = {
            ...chatMessages[idx],
            toolOutputLines: [
              ...prev,
              { stream: event.stream || 'stdout', text: event.text ?? '' },
            ],
          }
          chatMessages = [...chatMessages]
          void scrollToBottom()
        }
        break
      }
      case 'context_info':
        publishContextInfo({
          ...contextInfo,
          system_prompt_tokens: event.system_prompt_tokens,
          history_tokens: event.history_tokens,
          history_messages: event.history_messages,
          tool_count: event.tool_count,
          memory_count: event.memory_count,
          memory_tokens: event.memory_tokens,
          compaction_trigger_tokens: event.compaction_trigger_tokens,
          compaction_keep_recent_tokens: event.compaction_keep_recent_tokens,
          compaction_keep_recent_fraction: event.compaction_keep_recent_fraction,
          compaction_last_mode: event.compaction_last_mode ?? contextInfo.compaction_last_mode,
          tool_names: event.tool_names,
          skill_count: event.skill_count,
          skill_names: event.skill_names,
          command_count: event.command_count,
          command_names: event.command_names,
          used_tool_names: event.used_tool_names ?? contextInfo.used_tool_names ?? [],
          selected_skill_name: event.selected_skill_name ?? contextInfo.selected_skill_name,
          selected_skill_reason: event.selected_skill_reason ?? contextInfo.selected_skill_reason,
          selected_command_name: event.selected_command_name ?? contextInfo.selected_command_name,
          selected_command_reason: event.selected_command_reason ?? contextInfo.selected_command_reason,
          mentioned_path_count: event.mentioned_path_count ?? contextInfo.mentioned_path_count,
          mentioned_paths: event.mentioned_paths ?? contextInfo.mentioned_paths,
          mentioned_subagent_count: event.mentioned_subagent_count ?? contextInfo.mentioned_subagent_count,
          mentioned_subagents: event.mentioned_subagents ?? contextInfo.mentioned_subagents,
          llm_tier: event.llm_tier ?? contextInfo.llm_tier,
          tier_recommendation: event.tier_recommendation ?? contextInfo.tier_recommendation,
        })
        break
      case 'compaction_applied':
        publishContextInfo({
          ...contextInfo,
          compaction_last_mode: event.compaction_last_mode ?? event.mode ?? contextInfo.compaction_last_mode,
          compaction_trigger_tokens: event.trigger_tokens ?? contextInfo.compaction_trigger_tokens,
        })
        applyChatStatus({
          phase: 'compaction',
          message: chatStatusLocale === 'en'
            ? `${event.mode ? `${event.mode} mode` : ''} ${event.compacted_count ?? 0} tokens compacted`.trim()
            : `${event.mode ? `${event.mode} 모드` : ''} ${event.compacted_count ?? 0}개 토큰 압축`.trim(),
        })
        break
      case 'tasks_changed':
        onTasksChanged?.({
          total: event.task_total ?? 0,
          pending: event.task_pending ?? 0,
          in_progress: event.task_in_progress ?? 0,
          completed: event.task_completed ?? 0,
          cancelled: event.task_cancelled ?? 0,
          plan_goal: event.plan_goal,
        })
        break
      case 'done': {
        chatSessionId = event.session_id?.trim() || chatSessionId
        applyChatStatus({ phase: 'done' })
        stopChatStatusTicker()
        // Attach usage to assistant message
        if (event.usage) {
          const aIdx = chatMessages.findIndex((m) => m.id === assistantRef.id)
          if (aIdx >= 0) {
            chatMessages[aIdx] = { ...chatMessages[aIdx], usage: event.usage }
            chatMessages = [...chatMessages]
          }
        }
        // Auto-title: use first user message as session title for new sessions
        if (chatSessionId && !autoTitled) {
          autoTitled = true
          const firstUser = chatMessages.find((m) => m.role === 'user')
          if (firstUser?.text) {
            const title = firstUser.text.slice(0, 60).trim() + (firstUser.text.length > 60 ? '...' : '')
            renameSession(chatSessionId, title).catch(() => {})
          }
        }
        onSessionChange?.()
        break
      }
      case 'cancelled':
        applyChatStatus({ phase: 'cancelled' })
        stopChatStatusTicker()
        onSessionChange?.()
        break
      case 'error':
        chatError = event.error?.trim() || 'Stream failed'
        break
    }
  }

  let mentionOpen = $state(false)
  let mentionLoading = $state(false)
  let mentionCandidates: ChatMentionCandidate[] = $state([])
  let mentionActiveIndex = $state(0)
  let activeMentionTrigger: ActiveMentionTrigger | null = $state(null)
  let selectedMentions: SelectedChatMention[] = $state([])
  let mentionRequestSeq = 0
  let mentionSubagents: AgentRuntimeSubagent[] = $state([])
  let activeSelectedMentions = $derived(filterSelectedMentionsForMessage(selectedMentions, chatInput))
  let slashSkills: SkillDef[] = $state([])
  let slashCommands: CommandDef[] = $state([])
  let slashOpen = $state(false)
  let slashCandidates: SlashCommandCandidate[] = $state([])
  let slashActiveIndex = $state(0)
  let activeSlashTrigger: ActiveSlashTrigger | null = $state(null)
  let slashRequestSeq = 0

  function closeMentionMenu() {
    mentionOpen = false
    mentionLoading = false
    mentionCandidates = []
    mentionActiveIndex = 0
    activeMentionTrigger = null
  }

  function closeSlashMenu() {
    slashOpen = false
    slashCandidates = []
    slashActiveIndex = 0
    activeSlashTrigger = null
  }

  function activeChatSessionId(): string {
    return (chatSessionId || sessionId || '').trim()
  }

  async function loadSlashSkills() {
    try {
      const sessionIdForSlash = activeChatSessionId() || undefined
      const [skills, toolsResp, effectiveResp] = await Promise.all([
        listSkills(sessionIdForSlash),
        listChatTools(sessionIdForSlash),
        sessionIdForSlash ? getSessionEffectiveConfig(sessionIdForSlash).catch(() => null) : Promise.resolve(null),
      ])
      const toolConfig = effectiveResp?.effective.tool_config
      slashSkills = filterSlashDefs(skills, toolConfig?.skills_custom, toolConfig?.skills_enabled)
      slashCommands = filterSlashDefs(toolsResp.commands ?? [], toolConfig?.commands_custom, toolConfig?.commands_enabled)
    } catch {
      slashSkills = []
      slashCommands = []
    }
  }

  function filterSlashDefs<T extends { name: string }>(defs: T[], custom?: boolean, enabled?: string[]): T[] {
    if (!custom && !Array.isArray(enabled)) return defs
    const allowed = new Set((enabled ?? []).map((name) => name.trim().toLowerCase()).filter(Boolean))
    return defs.filter((def) => allowed.has(def.name.trim().toLowerCase()))
  }

  async function reloadSlashSkillsAndCandidates() {
    await loadSlashSkills()
    await refreshSlashCandidates()
  }

  async function loadMentionSubagents() {
    try {
      const result = await listAgentRuntimeSubagents()
      mentionSubagents = (result.agents ?? []).filter((agent) => agent.enabled !== false)
    } catch {
      mentionSubagents = []
    }
  }

  async function refreshSlashCandidates() {
    const caret = textareaEl?.selectionStart ?? chatInput.length
    const trigger = findActiveSlashTrigger(chatInput, caret)
    activeSlashTrigger = trigger
    if (!trigger) {
      closeSlashMenu()
      return
    }
    const seq = ++slashRequestSeq
    const candidates = buildSlashCandidates(trigger.query, slashSkills, slashCommands)
    if (seq !== slashRequestSeq) return
    slashCandidates = candidates
    slashActiveIndex = 0
    slashOpen = candidates.length > 0
  }

  async function refreshMentionCandidates() {
    const caret = textareaEl?.selectionStart ?? chatInput.length
    const trigger = findActiveMentionTrigger(chatInput, caret)
    activeMentionTrigger = trigger
    if (!trigger) {
      closeMentionMenu()
      return
    }

    const seq = ++mentionRequestSeq
    mentionOpen = true
    mentionLoading = true
    try {
      const [result, subagentResult] = await Promise.all([
        listChatFileMentions(chatSessionId || sessionId, trigger.query, 30),
        mentionSubagents.length > 0
          ? Promise.resolve({ agents: mentionSubagents })
          : listAgentRuntimeSubagents().catch(() => ({ agents: [] as AgentRuntimeSubagent[] })),
      ])
      if (seq !== mentionRequestSeq) return
      mentionSubagents = (subagentResult.agents ?? []).filter((agent) => agent.enabled !== false)
      const subagentCandidates = buildSubagentMentionCandidates(trigger.query, mentionSubagents, 12)
      mentionCandidates = [...result.candidates, ...subagentCandidates].sort(compareMentionCandidates)
      mentionActiveIndex = 0
    } catch {
      if (seq !== mentionRequestSeq) return
      mentionCandidates = []
    } finally {
      if (seq === mentionRequestSeq) mentionLoading = false
    }
  }

  function selectMention(candidate: ChatMentionCandidate) {
    if (!activeMentionTrigger) return
    const applied = applyMentionCandidate(chatInput, activeMentionTrigger, candidate)
    chatInput = applied.value
    selectedMentions = [
      ...filterSelectedMentionsForMessage(selectedMentions, chatInput).filter((mention) =>
        mention.kind !== applied.mention.kind ||
        mention.root !== applied.mention.root ||
        mention.path !== applied.mention.path,
      ),
      applied.mention,
    ]
    closeMentionMenu()
    tick().then(() => {
      textareaEl?.focus()
      textareaEl?.setSelectionRange(applied.caret, applied.caret)
    })
  }

  function compareMentionCandidates(a: ChatMentionCandidate, b: ChatMentionCandidate): number {
    const rank = (kind: ChatMentionCandidate['kind']) => kind === 'directory' ? 0 : kind === 'file' ? 1 : 2
    const diff = rank(a.kind) - rank(b.kind)
    if (diff !== 0) return diff
    return a.path.toLowerCase().localeCompare(b.path.toLowerCase())
  }

  function mentionKindLabel(kind: ChatMentionCandidate['kind'] | SelectedChatMention['kind']): string {
    if (kind === 'directory') return 'DIR'
    if (kind === 'subagent') return 'AGENT'
    return 'FILE'
  }

  function mentionSectionLabel(kind: ChatMentionCandidate['kind']): string {
    if (kind === 'directory') return 'Directories'
    if (kind === 'subagent') return 'Subagents'
    return 'Files'
  }

  function mentionOptionMeta(candidate: ChatMentionCandidate): string {
    if (candidate.kind === 'subagent') {
      return [candidate.tier, candidate.model, candidate.description].filter(Boolean).join(' · ') || 'subagent'
    }
    return candidate.root_label
  }

  function isFileOrDirectoryMention(mention: SelectedChatMention): mention is SelectedChatMention & { kind: 'file' | 'directory' } {
    return mention.kind === 'file' || mention.kind === 'directory'
  }

  function selectSlashCandidate(candidate: SlashCommandCandidate) {
    if (!activeSlashTrigger) return
    const applied = applySlashCandidate(chatInput, activeSlashTrigger, candidate)
    chatInput = applied.value
    closeSlashMenu()
    tick().then(() => {
      textareaEl?.focus()
      textareaEl?.setSelectionRange(applied.caret, applied.caret)
    })
  }

  async function executeBuiltinSlashCommand(command: string, args: string): Promise<boolean> {
    const id = builtinSlashCommandId(command)
    if (!id || !onSlashCommand) return false
    chatInput = ''
    closeMentionMenu()
    closeSlashMenu()
    chatError = ''
    applyChatStatus({ phase: 'slash', message: `/${command}` })
    await onSlashCommand(id, args)
    return true
  }

  async function executeSlashCandidate(candidate: SlashCommandCandidate): Promise<boolean> {
    if (candidate.kind !== 'builtin') return false
    return executeBuiltinSlashCommand(candidate.command, '')
  }

  function removeSelectedMention(index: number) {
    const active = filterSelectedMentionsForMessage(selectedMentions, chatInput)
    active.splice(index, 1)
    selectedMentions = active
  }

  type SubmitChatOptions = {
    recommendation?: ChatTierRecommendationRequest
    allowPrompt?: boolean
  }

  function isFirstUserTurn(): boolean {
    return !chatMessages.some((msg) => msg.role === 'user' || (msg.role === 'assistant' && msg.text.trim()))
  }

  function tierLabel(tier: ChatTier): string {
    if (tier === 'heavy') return 'Heavy'
    if (tier === 'light') return 'Light'
    return 'Standard'
  }

  function tierClass(tier: ChatTier): string {
    return `tier-pill ${tier}`
  }

  async function continueWithTier(tier: ChatTier) {
    if (!pendingTierRecommendation) return
    const recommendation = tierRecommendationPayload(pendingTierRecommendation, tier)
    pendingTierRecommendation = null
    pendingTierMessage = ''
    await submitChat({ recommendation, allowPrompt: false })
  }

  async function submitChat(options: SubmitChatOptions = {}) {
    const message = chatInput.trim()
    if (!message || chatBusy) return
    const parsedSlash = parseLeadingSlashCommand(message)
    if (parsedSlash && await executeBuiltinSlashCommand(parsedSlash.command, parsedSlash.args)) {
      return
    }

    let tierRecommendation = options.recommendation
    if (!tierRecommendation && isFirstUserTurn()) {
      const recommendation = buildTierRecommendation(message)
      if (recommendation.should_prompt && options.allowPrompt !== false) {
        pendingTierRecommendation = recommendation
        pendingTierMessage = message
        applyChatStatus({ phase: '', message: '' })
        return
      }
      tierRecommendation = tierRecommendationPayload(recommendation, recommendation.recommended_tier)
    }

    chatBusy = true
    chatError = ''
    applyChatStatus({ phase: 'connecting' })
    chatInput = ''
    pendingTierRecommendation = null
    pendingTierMessage = ''
    autoScroll = true
    publishContextInfo({})

    const currentFiles = [...attachedFiles]
    const currentMentions = filterSelectedMentionsForMessage(selectedMentions, message)
    const fileMentions = currentMentions.filter(isFileOrDirectoryMention)
    const subagentMentions = currentMentions.filter((mention) => mention.kind === 'subagent')
    attachedFiles = []
    selectedMentions = []
    closeMentionMenu()
    closeSlashMenu()

    const fileLabel = currentFiles.length > 0
      ? ` [${currentFiles.map((f) => f.name).join(', ')}]`
      : ''
    const userId = `user-${Date.now()}`
    const assistantRef = { id: `assistant-${Date.now()}` }
    chatMessages = [
      ...chatMessages,
      { id: userId, role: 'user', text: message + fileLabel },
      { id: assistantRef.id, role: 'assistant', text: '' },
    ]
    void scrollToBottom()
    const ac = new AbortController()
    abortController = ac
    try {
      const chatAttachments = currentFiles.length > 0 ? await filesToAttachments(currentFiles) : undefined
      await streamChat(
        {
          message,
          session_id: chatSessionId || 'new',
          attachments: chatAttachments,
          mentions: fileMentions.map((mention) => ({
            kind: mention.kind,
            root: mention.root,
            path: mention.path,
          })),
          subagent_mentions: subagentMentions.map((mention) => ({
            name: mention.path,
            token: mention.token,
          })),
          tier_recommendation: tierRecommendation,
        },
        (event) => handleChatEvent(event, assistantRef),
        ac.signal,
      )
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        // User cancelled — no error to show
      } else {
        chatError = err instanceof Error ? err.message : 'Failed to send'
        chatMessages = [...chatMessages, { id: `error-${Date.now()}`, role: 'error', text: chatError }]
      }
    } finally {
      abortController = null
      chatBusy = false
      stopChatStatusTicker()
      void scrollToBottom()
    }
  }

  async function handleCancel() {
    if (chatSessionId) {
      await cancelChat(chatSessionId)
    }
    abortController?.abort()
  }

  // -- File attachments --
  let attachedFiles: File[] = $state([])
  let fileInputEl: HTMLInputElement | undefined = $state()
  let isDragging = $state(false)
  let filePreviews: Map<string, string> = $state(new Map()) // file name → preview URL or text

  function addFiles(files: FileList | File[]) {
    for (const file of files) {
      if (attachedFiles.length >= 5) break
      attachedFiles = [...attachedFiles, file]
      generatePreview(file)
    }
  }

  function generatePreview(file: File) {
    const key = `${file.name}-${file.size}-${file.lastModified}`
    if (file.type.startsWith('image/')) {
      const url = URL.createObjectURL(file)
      filePreviews = new Map([...filePreviews, [key, url]])
    } else if (file.type.startsWith('text/') || /\.(txt|md|json|csv|yaml|yml|ts|js|py|go)$/i.test(file.name)) {
      file.slice(0, 500).text().then((text) => {
        const lines = text.split('\n').slice(0, 3).join('\n')
        filePreviews = new Map([...filePreviews, [key, lines]])
      }).catch(() => {})
    }
  }

  function getPreviewKey(file: File): string {
    return `${file.name}-${file.size}-${file.lastModified}`
  }

  function handleFileSelect(e: Event) {
    const input = e.target as HTMLInputElement
    if (!input.files) return
    addFiles(input.files)
    input.value = ''
  }

  function removeAttachment(index: number) {
    const file = attachedFiles[index]
    const key = getPreviewKey(file)
    const preview = filePreviews.get(key)
    if (preview && preview.startsWith('blob:')) URL.revokeObjectURL(preview)
    filePreviews.delete(key)
    filePreviews = new Map(filePreviews)
    attachedFiles = attachedFiles.filter((_, i) => i !== index)
  }

  // Drag & drop
  function handleDragOver(e: DragEvent) {
    e.preventDefault()
    isDragging = true
  }

  function handleDragLeave(e: DragEvent) {
    // Only clear if leaving the panel itself (not a child)
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    if (e.clientX <= rect.left || e.clientX >= rect.right || e.clientY <= rect.top || e.clientY >= rect.bottom) {
      isDragging = false
    }
  }

  function handleDrop(e: DragEvent) {
    e.preventDefault()
    isDragging = false
    if (e.dataTransfer?.files && e.dataTransfer.files.length > 0) {
      addFiles(e.dataTransfer.files)
    }
  }

  async function filesToAttachments(files: File[]): Promise<ChatAttachment[]> {
    const results: ChatAttachment[] = []
    for (const file of files) {
      const buffer = await file.arrayBuffer()
      const bytes = new Uint8Array(buffer)
      let binary = ''
      for (let i = 0; i < bytes.byteLength; i++) {
        binary += String.fromCharCode(bytes[i])
      }
      results.push({
        name: file.name,
        mime_type: file.type || 'application/octet-stream',
        data: btoa(binary),
      })
    }
    return results
  }

  function copyMessageText(text: string) {
    navigator.clipboard.writeText(text).catch(() => {})
  }

  // Send a message programmatically — used by panels (TasksPanel
  // Approve & Run) that need to emit a follow-up turn without forcing
  // the user back into the composer.
  export async function sendMessageText(text: string): Promise<void> {
    const trimmed = text.trim()
    if (!trimmed || chatBusy) return
    chatInput = trimmed
    await submitChat()
  }

  export function clearThread() {
    chatInput = ''
    chatError = ''
    applyChatStatus({ phase: '', message: '' })
    chatMessages = []
    attachedFiles = []
    selectedMentions = []
    publishContextInfo({})
    closeMentionMenu()
    closeSlashMenu()
  }

  export function exportAsMarkdown(): string {
    const lines: string[] = []
    for (const msg of chatMessages) {
      if (msg.role === 'system') continue
      if (msg.role === 'tool') {
        lines.push(`> **Tool: ${msg.toolName}**`)
        if (msg.toolArgs) lines.push(`> Args: \`${msg.toolArgs}\``)
        if (msg.toolResult) lines.push(`> Result: \`${msg.toolResult}\``)
        lines.push('')
      } else if (msg.role === 'user') {
        lines.push(`### User\n\n${msg.text}\n`)
      } else if (msg.role === 'assistant') {
        lines.push(`### Assistant\n\n${msg.text}\n`)
      } else if (msg.role === 'error') {
        lines.push(`> **Error:** ${msg.text}\n`)
      }
    }
    return lines.join('\n')
  }

  function fmtSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  function handleKeydown(e: KeyboardEvent) {
    if (slashOpen) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        slashActiveIndex = Math.min(Math.max(0, slashCandidates.length - 1), slashActiveIndex + 1)
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        slashActiveIndex = Math.max(0, slashActiveIndex - 1)
        return
      }
      if ((e.key === 'Enter' || e.key === 'Tab') && slashCandidates[slashActiveIndex]) {
        e.preventDefault()
        const candidate = slashCandidates[slashActiveIndex]
        if (e.key === 'Enter' && candidate.kind === 'builtin') {
          void executeSlashCandidate(candidate)
        } else {
          selectSlashCandidate(candidate)
        }
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        closeSlashMenu()
        return
      }
    }
    if (mentionOpen) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        mentionActiveIndex = Math.min(Math.max(0, mentionCandidates.length - 1), mentionActiveIndex + 1)
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        mentionActiveIndex = Math.max(0, mentionActiveIndex - 1)
        return
      }
      if ((e.key === 'Enter' || e.key === 'Tab') && mentionCandidates[mentionActiveIndex]) {
        e.preventDefault()
        selectMention(mentionCandidates[mentionActiveIndex])
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        closeMentionMenu()
        return
      }
    }
    if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
      e.preventDefault()
      void submitChat()
    }
  }

  function handleChatInput() {
    if (pendingTierRecommendation && chatInput.trim() !== pendingTierMessage) {
      pendingTierRecommendation = null
      pendingTierMessage = ''
    }
    void refreshMentionCandidates()
    void refreshSlashCandidates()
  }

  function handleTextareaCursorChange() {
    void refreshMentionCandidates()
    void refreshSlashCandidates()
  }

  function handleTextareaKeyup(e: KeyboardEvent) {
    if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) {
      void refreshMentionCandidates()
      void refreshSlashCandidates()
    }
  }

  function handlePaste(e: ClipboardEvent) {
    if (!e.clipboardData) return
    const items = e.clipboardData.items

    // Check for images or files first
    for (let i = 0; i < items.length; i++) {
      const item = items[i]
      if (item.type.startsWith('image/')) {
        e.preventDefault()
        const file = item.getAsFile()
        if (file && attachedFiles.length < 5) {
          const name = `clipboard-${Date.now()}.${item.type.split('/')[1] || 'png'}`
          const renamed = new File([file], name, { type: file.type })
          addFiles([renamed])
        }
        return
      }
      if (item.kind === 'file') {
        const file = item.getAsFile()
        if (file && attachedFiles.length < 5) {
          addFiles([file])
        }
      }
    }

    // Long text paste → attach as file instead of flooding textarea
    const text = e.clipboardData.getData('text/plain')
    if (text && text.length > 500 && attachedFiles.length < 5) {
      e.preventDefault()
      const file = new File([text], `clipboard-text-${Date.now()}.txt`, { type: 'text/plain' })
      addFiles([file])
    }
    // Short text paste is handled natively by the textarea
  }

  let textareaEl: HTMLTextAreaElement | undefined = $state()
  let stopEventStream: (() => void) | null = null
  let visibilityHandler: (() => void) | null = null

  async function loadHistoryInto(targetSessionId: string) {
    const rebuilt: ChatMessage[] = [
      { id: 'system-init', role: 'system', text: `Session: ${targetSessionId.slice(0, 8)}...` },
    ]
    const history = await getSessionHistory(targetSessionId)
    for (const msg of history) {
      if (msg.role === 'system' && (msg.content.startsWith('[HEARTBEAT]') || msg.content.startsWith('[COMPACTION SUMMARY]'))) {
        continue
      }
      if (msg.role === 'tool') {
        rebuilt.push({
          id: `tool-${msg.tool_call_id || Date.now()}`,
          role: 'tool',
          text: '',
          toolName: msg.tool_name,
          toolCallId: msg.tool_call_id,
          toolArgs: msg.tool_args,
          toolResult: msg.content,
          toolDone: true,
          toolIsError: msg.tool_is_error,
        })
      } else {
        rebuilt.push({
          id: msg.id || `hist-${rebuilt.length}`,
          sourceMessageId: msg.id,
          role: msg.role as ChatMessage['role'],
          text: msg.content,
        })
      }
    }
    chatMessages = rebuilt
    artifacts = extractArtifactsFromHistory(chatMessages, targetSessionId)
    if (artifacts.length > 0) onArtifactsChange?.(artifacts)
  }

  async function handleForkMessage(message: ChatMessage) {
    const targetSessionId = (chatSessionId || sessionId || '').trim()
    const sourceMessageId = message.sourceMessageId?.trim()
    if (!targetSessionId || !sourceMessageId) return
    if (chatBusy) {
      chatError = 'Wait for the current response to finish before forking this session.'
      return
    }
    chatError = ''
    applyChatStatus({
      phase: 'slash',
      message: chatStatusLocale === 'en' ? 'Duplicating session...' : '세션 복사 중',
    })
    try {
      const child = await forkSessionFromMessage(targetSessionId, sourceMessageId, `Forked from ${message.role} message`)
      onSessionForked?.(child)
      onSessionChange?.()
    } catch (err) {
      chatError = err instanceof Error ? err.message : 'Failed to fork session'
    } finally {
      applyChatStatus({ phase: '', message: '' })
    }
  }

  onMount(async () => {
    void loadSlashSkills()
    void loadMentionSubagents()
    if (sessionId) {
      chatSessionId = sessionId
      chatMessages = [{ id: 'system-init', role: 'system', text: $t.chat.systemInit.session(sessionId.slice(0, 8)) }]
      try {
        await loadHistoryInto(sessionId)
        autoTitled = true
        void scrollToBottom()
      } catch { /* ignore */ }
    } else {
      chatMessages = [{ id: 'system-init', role: 'system', text: $t.chat.systemInit.tars }]
    }
    if (initialPrompt && !autoSend) {
      chatInput = initialPrompt
      tick().then(() => textareaEl?.focus())
    }

    // Refresh transcript when any background actor (cron, pulse auto-resume,
    // future agentruntime/chat events) touches the active session — and when
    // the SSE stream reconnects after a drop, since events emitted during the
    // gap were lost. Skip while a foreground stream is filling chatMessages.
    const refreshActiveSession = () => {
      const currentId = (chatSessionId || sessionId || '').trim()
      if (!currentId) return
      if (chatBusy) return
      void (async () => {
        try {
          await loadHistoryInto(currentId)
          void scrollToBottom()
        } catch { /* ignore */ }
      })()
    }

    stopEventStream = streamEvents(
      (event) => {
        if (!event.session_id) return
        const currentId = (chatSessionId || sessionId || '').trim()
        if (!currentId) return
        if (event.session_id.trim() !== currentId) return
        refreshActiveSession()
      },
      undefined,
      undefined,
      refreshActiveSession,
    )

    // Browsers may close the SSE connection or throttle timers when the tab is
    // hidden. On refocus, force a transcript reload so anything that arrived
    // while we weren't listening shows up.
    visibilityHandler = () => {
      if (document.visibilityState === 'visible') {
        refreshActiveSession()
      }
    }
    document.addEventListener('visibilitychange', visibilityHandler)
  })

  onDestroy(() => {
    stopEventStream?.()
    stopChatStatusTicker()
    if (visibilityHandler) {
      document.removeEventListener('visibilitychange', visibilityHandler)
      visibilityHandler = null
    }
  })
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="chat-panel" role="region" aria-label="Chat" ondragover={handleDragOver} ondragleave={handleDragLeave} ondrop={handleDrop}>
  {#if isDragging}
    <div class="drop-overlay">
      <div class="drop-label">{$t.chat.dropOverlay}</div>
    </div>
  {/if}
  <div class="chat-log" bind:this={chatLogEl} onscroll={handleScroll}>
    {#each chatMessages as msg}
      <ChatMessageItem
        message={msg}
        {artifacts}
        {onArtifactOpen}
        onCopy={copyMessageText}
        onForkMessage={handleForkMessage}
        streamingStatus={msg.id === streamingAssistantId ? streamingStatus : null}
      />
    {/each}
  </div>
  {#if chatError}
    <div class="error-banner" style="margin-bottom: var(--space-3)">{chatError}</div>
  {/if}
  {#if attachedFiles.length > 0}
    <div class="chat-attachments">
      {#each attachedFiles as file, i}
        {@const preview = filePreviews.get(getPreviewKey(file))}
        <div class="attachment-card">
          {#if file.type.startsWith('image/') && preview}
            <img class="attachment-thumb" src={preview} alt={file.name} />
          {:else if preview && !preview.startsWith('blob:')}
            <pre class="attachment-text-preview">{preview}</pre>
          {:else}
            <span class="attachment-icon-lg">{file.type.startsWith('image/') ? '\ud83d\uddbc' : file.type === 'application/pdf' ? '\ud83d\udcc3' : '\ud83d\udcc4'}</span>
          {/if}
          <div class="attachment-info">
            <span class="attachment-name">{file.name}</span>
            <span class="attachment-size">{fmtSize(file.size)}</span>
          </div>
          <button class="attachment-remove" onclick={() => removeAttachment(i)}>&times;</button>
        </div>
      {/each}
    </div>
  {/if}
  {#if activeSelectedMentions.length > 0}
    <div class="chat-mentions">
      {#each activeSelectedMentions as mention, i}
        <button type="button" class="mention-chip" title={mention.root} onclick={() => removeSelectedMention(i)}>
          <span class="mention-kind">{mentionKindLabel(mention.kind)}</span>
          <span class="mention-label">{mention.token}</span>
          <span class="mention-remove">&times;</span>
        </button>
      {/each}
    </div>
  {/if}
  {#if pendingTierRecommendation}
    <div class="tier-recommendation-card" aria-label={$t.chat.tierRecommendation.ariaLabel}>
      <div class="tier-recommendation-copy">
        <span class={tierClass(pendingTierRecommendation.recommended_tier)}>{tierLabel(pendingTierRecommendation.recommended_tier)}</span>
        <div>
          <strong>{$t.chat.tierRecommendation.headline(tierLabel(pendingTierRecommendation.recommended_tier))}</strong>
          <p>{pendingTierRecommendation.reason}</p>
        </div>
      </div>
      <div class="tier-recommendation-actions" aria-label={$t.chat.tierRecommendation.chooseTierAria}>
        <button type="button" class="btn btn-ghost btn-sm" onclick={() => continueWithTier('light')}>{$t.chat.tierRecommendation.tierLight}</button>
        <button type="button" class="btn btn-ghost btn-sm" onclick={() => continueWithTier('standard')}>{$t.chat.tierRecommendation.tierStandard}</button>
        <button type="button" class="btn btn-primary btn-sm" onclick={() => continueWithTier('heavy')}>{$t.chat.tierRecommendation.tierHeavy}</button>
      </div>
    </div>
  {/if}
  <form class="chat-form" onsubmit={(e) => { e.preventDefault(); void submitChat() }}>
    <div class="chat-input-row">
      <div class="chat-toolbar">
        <input
          type="file"
          multiple
          accept="image/*,.pdf,.txt,.md,.json,.csv,.yaml,.yml"
          bind:this={fileInputEl}
          onchange={handleFileSelect}
          class="file-input-hidden"
        />
        <button type="button" class="toolbar-btn" title={$t.chat.toolbar.attachFile} onclick={() => fileInputEl?.click()}>
          <span class="toolbar-icon">{'\ud83d\udcce'}</span>
        </button>
        <button type="button" class="toolbar-btn" title={$t.chat.toolbar.attachImage} onclick={() => { if (fileInputEl) { fileInputEl.accept = 'image/*'; fileInputEl.click(); fileInputEl.accept = 'image/*,.pdf,.txt,.md,.json,.csv,.yaml,.yml' } }}>
          <span class="toolbar-icon">{'\ud83d\uddbc'}</span>
        </button>
      </div>
      <div class="chat-input-wrap">
        <textarea
          bind:this={textareaEl}
          bind:value={chatInput}
          rows="2"
          placeholder={sessionId ? $t.chat.input.placeholderContinue : $t.chat.input.placeholderNew}
          oninput={handleChatInput}
          onclick={handleTextareaCursorChange}
          onkeyup={handleTextareaKeyup}
          onkeydown={handleKeydown}
          onpaste={handlePaste}
        ></textarea>
        {#if slashOpen}
          <SlashPopover
            candidates={slashCandidates}
            activeIndex={slashActiveIndex}
            onSelect={selectSlashCandidate}
            onExecute={(candidate) => void executeSlashCandidate(candidate)}
          />
        {/if}
        {#if mentionOpen}
          <div class="mention-menu">
            {#if mentionLoading}
              <div class="mention-empty">{$t.chat.mention.loading}</div>
            {:else if mentionCandidates.length === 0}
              <div class="mention-empty">{$t.chat.mention.noMatches}</div>
            {:else}
              {#each mentionCandidates as candidate, i}
                {#if i === 0 || mentionCandidates[i - 1]?.kind !== candidate.kind}
                  <div class="mention-section">{mentionSectionLabel(candidate.kind)}</div>
                {/if}
                <button
                  type="button"
                  class:active={i === mentionActiveIndex}
                  class="mention-option"
                  onmousedown={(e) => e.preventDefault()}
                  onclick={() => selectMention(candidate)}
                >
                  <span class="mention-option-kind">{mentionKindLabel(candidate.kind)}</span>
                  <span class="mention-option-main">{candidate.path}</span>
                  <span class="mention-option-root">{mentionOptionMeta(candidate)}</span>
                </button>
              {/each}
            {/if}
          </div>
        {/if}
      </div>
    </div>
    <div class="chat-form-actions">
      {#if chatBusy}
        <button type="button" class="btn btn-danger btn-sm" onclick={handleCancel}>{$t.chat.input.stop}</button>
      {:else}
        <button type="submit" class="btn btn-primary" disabled={!chatInput.trim()}>{$t.chat.input.send}</button>
      {/if}
    </div>
  </form>
</div>

<style>
  .chat-panel {
    display: flex;
    flex-direction: column;
    min-height: 0;
    flex: 1;
    position: relative;
  }

  .drop-overlay {
    position: absolute;
    inset: 0;
    z-index: 10;
    background: rgba(224, 145, 69, 0.08);
    border: 2px dashed var(--primary);
    border-radius: var(--radius-lg);
    display: flex;
    align-items: center;
    justify-content: center;
    pointer-events: none;
  }
  .drop-label {
    font-family: var(--font-display);
    font-size: var(--text-lg);
    font-weight: 500;
    color: var(--primary);
  }

  .chat-log {
    display: grid;
    gap: var(--space-2);
    flex: 1;
    overflow-y: auto;
    margin-bottom: var(--space-3);
    scroll-behavior: smooth;
    min-height: 0;
  }

  .tier-recommendation-card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    margin-bottom: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
  }

  .tier-recommendation-copy {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
  }

  .tier-recommendation-copy strong {
    display: block;
    color: var(--text-primary);
    font-size: var(--text-sm);
  }

  .tier-recommendation-copy p {
    margin: 2px 0 0;
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .tier-recommendation-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
  }

  .tier-pill {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 68px;
    min-height: 28px;
    padding: 0 var(--space-2);
    border-radius: var(--radius-sm);
    border: 1px solid var(--border-subtle);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    text-transform: uppercase;
  }

  .tier-pill.heavy {
    color: var(--error);
    background: var(--error-muted);
  }

  .tier-pill.standard {
    color: var(--info);
    background: var(--info-muted);
  }

  .tier-pill.light {
    color: var(--success);
    background: var(--success-muted);
  }

  /* ── Attachments ─────────────────────────────── */
  .chat-attachments {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }

  .attachment-card {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2);
    background: var(--surface-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    font-size: var(--text-xs);
    max-width: 240px;
    position: relative;
  }

  .attachment-thumb {
    width: 48px;
    height: 48px;
    object-fit: cover;
    border-radius: var(--radius-sm);
    flex-shrink: 0;
  }

  .attachment-text-preview {
    width: 48px;
    height: 48px;
    overflow: hidden;
    font-family: var(--font-mono);
    font-size: 7px;
    line-height: 1.3;
    color: var(--text-ghost);
    background: var(--surface-base);
    border-radius: var(--radius-sm);
    padding: 2px 3px;
    flex-shrink: 0;
    white-space: pre;
    margin: 0;
  }

  .attachment-icon-lg {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 48px;
    height: 48px;
    font-size: 24px;
    background: var(--surface-base);
    border-radius: var(--radius-sm);
    flex-shrink: 0;
  }

  .attachment-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .attachment-name {
    color: var(--text-primary);
    max-width: 140px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-weight: 500;
  }
  .attachment-size { color: var(--text-ghost); }
  .attachment-remove {
    position: absolute;
    top: 2px;
    right: 2px;
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: 50%;
    color: var(--text-ghost);
    cursor: pointer;
    font-size: 12px;
    width: 18px;
    height: 18px;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    line-height: 1;
    opacity: 0;
    transition: opacity var(--duration-fast);
  }
  .attachment-card:hover .attachment-remove { opacity: 1; }
  .attachment-remove:hover { color: var(--error); border-color: var(--error); }

  .chat-mentions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }

  .mention-chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    max-width: 260px;
    min-height: 26px;
    padding: 3px 8px;
    border: 1px solid rgba(96, 165, 250, 0.35);
    border-radius: var(--radius-sm);
    background: rgba(96, 165, 250, 0.08);
    color: var(--text-primary);
    cursor: pointer;
    font-size: var(--text-xs);
  }

  .mention-kind,
  .mention-option-kind {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
  }

  .mention-label {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
  }

  .mention-remove {
    color: var(--text-ghost);
  }

  /* ── Form ───────────────────────────────────── */
  .chat-form {
    display: grid;
    gap: var(--space-2);
  }

  .chat-input-row {
    display: flex;
    gap: var(--space-2);
    align-items: flex-start;
  }

  .chat-input-row textarea {
    flex: 1;
  }

  .chat-input-wrap {
    position: relative;
    flex: 1;
    min-width: 0;
  }

  .chat-input-wrap textarea {
    width: 100%;
  }

  .mention-menu {
    position: absolute;
    left: 0;
    right: 0;
    bottom: calc(100% + 6px);
    z-index: 20;
    max-height: 240px;
    overflow-y: auto;
    padding: 4px;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
    box-shadow: var(--shadow-lg);
  }

  .mention-section {
    padding: 5px 8px 3px;
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
    text-transform: uppercase;
  }

  .mention-option {
    display: grid;
    grid-template-columns: 44px minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
    min-height: 30px;
    padding: 4px 8px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
    text-align: left;
  }

  .mention-option:hover,
  .mention-option.active {
    background: rgba(224, 145, 69, 0.12);
    color: var(--text-primary);
  }

  .mention-option-main {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .mention-option-root,
  .mention-empty {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
  }

  .mention-option-root {
    max-width: 220px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mention-empty {
    padding: 8px;
  }

  .chat-toolbar {
    display: flex;
    flex-direction: column;
    gap: 2px;
    flex-shrink: 0;
    padding-top: 4px;
  }

  .toolbar-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: transparent;
    cursor: pointer;
    transition: all var(--duration-fast) var(--ease-out);
  }
  .toolbar-btn:hover {
    background: var(--surface-elevated);
    border-color: var(--border-default);
  }

  .toolbar-icon { font-size: 14px; }

  .chat-form-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .file-input-hidden {
    position: absolute;
    width: 0;
    height: 0;
    opacity: 0;
    overflow: hidden;
  }
</style>
