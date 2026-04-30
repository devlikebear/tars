<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte'
  import { streamChat, cancelChat, getSessionHistory, renameSession, streamEvents, listChatFileMentions, listAgentRuntimeSubagents, listSkills } from '../lib/api'
  import type { AgentRuntimeSubagent, ChatAttachment, ChatEvent, SessionMessage, SkillDef } from '../lib/types'
  import { extractArtifact, extractArtifactsFromHistory, mergeArtifact, type Artifact } from '../lib/artifacts'
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
      memory_count?: number
      memory_tokens?: number
      compaction_trigger_tokens?: number
      compaction_keep_recent_tokens?: number
      compaction_keep_recent_fraction?: number
      compaction_last_mode?: string
      used_tool_names?: string[]
      selected_skill_name?: string
      selected_skill_reason?: string
      mentioned_path_count?: number
      mentioned_paths?: string[]
      mentioned_subagent_count?: number
      mentioned_subagents?: string[]
    }) => void
    onToolComplete?: (toolName: string) => void
    onSessionReady?: (sessionId: string) => void
    onArtifactOpen?: (path: string) => void
    onTasksChanged?: (summary: TasksSummary) => void
    onSlashCommand?: (command: string, args: string) => void | Promise<void>
    onDraftChange?: (draft: string) => void
  }

  type TasksSummary = {
    total: number
    pending: number
    in_progress: number
    completed: number
    cancelled: number
    plan_goal?: string
  }

  let { sessionId, initialPrompt, autoSend, onSessionChange, onArtifactsChange, onContextInfo, onToolComplete, onSessionReady, onArtifactOpen, onTasksChanged, onSlashCommand, onDraftChange }: Props = $props()

  let artifacts: Artifact[] = $state([])

  let chatInput = $state('')
  let chatBusy = $state(false)
  let chatError = $state('')
  let chatSessionId = $state('')
  let chatStatusLine = $state('')
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
    memory_count?: number
    memory_tokens?: number
    compaction_trigger_tokens?: number
    compaction_keep_recent_tokens?: number
    compaction_keep_recent_fraction?: number
    compaction_last_mode?: string
    used_tool_names?: string[]
    selected_skill_name?: string
    selected_skill_reason?: string
    mentioned_path_count?: number
    mentioned_paths?: string[]
    mentioned_subagent_count?: number
    mentioned_subagents?: string[]
  } = $state({})

  function publishContextInfo(next: typeof contextInfo) {
    contextInfo = next
    onContextInfo?.(next)
  }

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
      tick().then(() => submitChat())
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
  }

  function handleChatEvent(event: ChatEvent, assistantId: string) {
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
          const aIdx = chatMessages.findIndex((m) => m.id === assistantId)
          if (aIdx >= 0) {
            chatMessages.splice(aIdx, 0, toolMsg)
            chatMessages = [...chatMessages]
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
          const aIdx = chatMessages.findIndex((m) => m.id === assistantId)
          if (aIdx >= 0) {
            chatMessages.splice(aIdx, 0, skillMsg)
            chatMessages = [...chatMessages]
          }
        }
        chatStatusLine = [event.phase, event.message, event.tool_name, event.skill_name]
          .filter(Boolean).join(' \u00b7 ')
        break
      case 'delta': {
        const chunk = event.text ?? ''
        if (!chunk) break
        const idx = chatMessages.findIndex((m) => m.id === assistantId)
        if (idx >= 0) {
          chatMessages[idx] = { ...chatMessages[idx], text: chatMessages[idx].text + chunk }
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
          used_tool_names: event.used_tool_names ?? contextInfo.used_tool_names ?? [],
          selected_skill_name: event.selected_skill_name ?? contextInfo.selected_skill_name,
          selected_skill_reason: event.selected_skill_reason ?? contextInfo.selected_skill_reason,
          mentioned_path_count: event.mentioned_path_count ?? contextInfo.mentioned_path_count,
          mentioned_paths: event.mentioned_paths ?? contextInfo.mentioned_paths,
          mentioned_subagent_count: event.mentioned_subagent_count ?? contextInfo.mentioned_subagent_count,
          mentioned_subagents: event.mentioned_subagents ?? contextInfo.mentioned_subagents,
        })
        break
      case 'compaction_applied':
        publishContextInfo({
          ...contextInfo,
          compaction_last_mode: event.compaction_last_mode ?? event.mode ?? contextInfo.compaction_last_mode,
          compaction_trigger_tokens: event.trigger_tokens ?? contextInfo.compaction_trigger_tokens,
        })
        chatStatusLine = ['compaction', event.mode, `${event.compacted_count ?? 0} compacted`]
          .filter(Boolean).join(' · ')
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
        chatStatusLine = 'done'
        // Attach usage to assistant message
        if (event.usage) {
          const aIdx = chatMessages.findIndex((m) => m.id === assistantId)
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
        chatStatusLine = 'cancelled'
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

  async function loadSlashSkills() {
    try {
      slashSkills = await listSkills()
    } catch {
      slashSkills = []
    }
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
    const candidates = buildSlashCandidates(trigger.query, slashSkills)
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
    chatStatusLine = `/${command}`
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

  async function submitChat() {
    const message = chatInput.trim()
    if (!message || chatBusy) return
    const parsedSlash = parseLeadingSlashCommand(message)
    if (parsedSlash && await executeBuiltinSlashCommand(parsedSlash.command, parsedSlash.args)) {
      return
    }
    chatBusy = true
    chatError = ''
    chatStatusLine = 'connecting'
    chatInput = ''
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
    const assistantId = `assistant-${Date.now()}`
    chatMessages = [
      ...chatMessages,
      { id: userId, role: 'user', text: message + fileLabel },
      { id: assistantId, role: 'assistant', text: '' },
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
        },
        (event) => handleChatEvent(event, assistantId),
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
          id: `hist-${rebuilt.length}`,
          role: msg.role as ChatMessage['role'],
          text: msg.content,
        })
      }
    }
    chatMessages = rebuilt
    artifacts = extractArtifactsFromHistory(chatMessages, targetSessionId)
    if (artifacts.length > 0) onArtifactsChange?.(artifacts)
  }

  onMount(async () => {
    void loadSlashSkills()
    void loadMentionSubagents()
    if (sessionId) {
      chatSessionId = sessionId
      chatMessages = [{ id: 'system-init', role: 'system', text: `Session: ${sessionId.slice(0, 8)}...` }]
      try {
        await loadHistoryInto(sessionId)
        autoTitled = true
        void scrollToBottom()
      } catch { /* ignore */ }
    } else {
      chatMessages = [{ id: 'system-init', role: 'system', text: 'TARS' }]
    }
    if (initialPrompt && !autoSend) {
      chatInput = initialPrompt
      tick().then(() => textareaEl?.focus())
    }

    // Auto-refresh chat when a background cron job delivers a message to this
    // session (session-bound cron, or main-bound cron delivering to main).
    stopEventStream = streamEvents((event) => {
      if (event.category !== 'cron') return
      const currentId = (chatSessionId || sessionId || '').trim()
      if (!currentId) return
      if ((event.session_id || '').trim() !== currentId) return
      if (chatBusy) return
      void (async () => {
        try {
          await loadHistoryInto(currentId)
          void scrollToBottom()
        } catch { /* ignore */ }
      })()
    })
  })

  onDestroy(() => {
    stopEventStream?.()
  })
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="chat-panel" role="region" aria-label="Chat" ondragover={handleDragOver} ondragleave={handleDragLeave} ondrop={handleDrop}>
  {#if isDragging}
    <div class="drop-overlay">
      <div class="drop-label">Drop files here</div>
    </div>
  {/if}
  <div class="chat-log" bind:this={chatLogEl} onscroll={handleScroll}>
    {#each chatMessages as msg}
      <ChatMessageItem message={msg} {artifacts} {onArtifactOpen} onCopy={copyMessageText} />
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
        <button type="button" class="toolbar-btn" title="Attach file" onclick={() => fileInputEl?.click()}>
          <span class="toolbar-icon">{'\ud83d\udcce'}</span>
        </button>
        <button type="button" class="toolbar-btn" title="Attach image" onclick={() => { if (fileInputEl) { fileInputEl.accept = 'image/*'; fileInputEl.click(); fileInputEl.accept = 'image/*,.pdf,.txt,.md,.json,.csv,.yaml,.yml' } }}>
          <span class="toolbar-icon">{'\ud83d\uddbc'}</span>
        </button>
      </div>
      <div class="chat-input-wrap">
        <textarea
          bind:this={textareaEl}
          bind:value={chatInput}
          rows="2"
          placeholder={sessionId ? 'Continue this session...' : 'Ask TARS anything... (paste images with Ctrl+V)'}
          oninput={handleChatInput}
          onclick={handleTextareaCursorChange}
          onkeyup={handleTextareaKeyup}
          onkeydown={handleKeydown}
          onpaste={handlePaste}
        ></textarea>
        {#if slashOpen}
          <div class="mention-menu slash-menu">
            {#each slashCandidates as candidate, i}
              {#if i === 0 || slashCandidates[i - 1]?.kind !== candidate.kind}
                <div class="slash-section">{candidate.kind === 'builtin' ? 'Built-in' : 'Skills'}</div>
              {/if}
              <button
                type="button"
                class:active={i === slashActiveIndex}
                class="mention-option slash-option"
                onmousedown={(e) => e.preventDefault()}
                onclick={() => { candidate.kind === 'builtin' ? void executeSlashCandidate(candidate) : selectSlashCandidate(candidate) }}
              >
                <span class="mention-option-kind">{candidate.kind === 'skill' ? 'SKILL' : 'CMD'}</span>
                <span class="mention-option-main">
                  /{candidate.command}
                  {#if candidate.aliasOf}<span class="slash-alias">/{candidate.aliasOf}</span>{/if}
                </span>
                <span class="mention-option-root">{candidate.kind === 'skill' ? (candidate.source || 'skill') : 'built-in'}</span>
                <span class="slash-description">{candidate.description}</span>
              </button>
            {/each}
          </div>
        {/if}
        {#if mentionOpen}
          <div class="mention-menu">
            {#if mentionLoading}
              <div class="mention-empty">Loading...</div>
            {:else if mentionCandidates.length === 0}
              <div class="mention-empty">No matches</div>
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
        <button type="button" class="btn btn-danger btn-sm" onclick={handleCancel}>Stop</button>
      {:else}
        <button type="submit" class="btn btn-primary" disabled={!chatInput.trim()}>Send</button>
      {/if}
      {#if chatStatusLine && chatBusy}
        <span class="chat-status-line">{chatStatusLine}</span>
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

  .slash-menu {
    max-height: 300px;
  }

  .slash-section,
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

  .slash-option {
    grid-template-columns: 52px minmax(0, 1fr) auto;
    grid-template-rows: auto auto;
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

  .slash-alias {
    margin-left: var(--space-2);
    color: var(--text-ghost);
    font-size: 10px;
  }

  .slash-description {
    grid-column: 2 / 4;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-ghost);
    font-size: 10px;
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

  .chat-status-line {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-ghost);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .file-input-hidden {
    position: absolute;
    width: 0;
    height: 0;
    opacity: 0;
    overflow: hidden;
  }
</style>
