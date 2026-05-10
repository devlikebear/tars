export type ToolOutputLine = {
  stream: 'stdout' | 'stderr' | string
  text: string
}

export type ChatMessage = {
  id: string
  sourceMessageId?: string
  role: 'user' | 'assistant' | 'system' | 'error' | 'tool'
  text: string
  reasoningText?: string
  toolName?: string
  toolCallId?: string
  toolArgs?: string
  toolResult?: string
  toolDone?: boolean
  toolIsError?: boolean
  toolStartedAt?: number
  toolFinishedAt?: number
  // Streaming stdout/stderr lines emitted while the tool runs.
  // Currently populated by exec via SSE `tool_output_line` events.
  toolOutputLines?: ToolOutputLine[]
  usage?: {
    input_tokens: number
    output_tokens: number
    cached_tokens: number
    cache_read_tokens: number
    cache_write_tokens: number
  }
}
