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
  usage?: {
    input_tokens: number
    output_tokens: number
    cached_tokens: number
    cache_read_tokens: number
    cache_write_tokens: number
  }
}
