/**
 * Artifact tracking — detects file creation/modification from tool call events.
 */

export type Artifact = {
  path: string
  toolName: string
  toolCallId: string
  action: 'created' | 'modified'
  timestamp: number
}

const ARTIFACT_TOOLS = new Set(['write', 'write_file', 'edit_file', 'apply_patch'])

/**
 * Attempt to extract an artifact from a tool call event.
 * Returns null if the tool is not a file operation or the path can't be parsed.
 */
export function extractArtifact(
  toolName: string,
  toolCallId: string,
  toolArgs: string | undefined,
  toolResult: string | undefined,
): Artifact | null {
  if (!ARTIFACT_TOOLS.has(toolName)) return null
  if (!toolArgs) return null

  // Parse file path from args preview
  // Formats: {"path":"some/file.ts",...} or {"file_path":"some/file.ts",...}
  const pathMatch = toolArgs.match(/"(?:path|file_path)"\s*:\s*"([^"]+)"/)
  if (!pathMatch) return null

  const path = pathMatch[1]
  const created = toolResult ? toolResult.includes('"created":true') || toolResult.includes('created') : false
  const action = (toolName === 'write' || toolName === 'write_file') && created ? 'created' : 'modified'

  return {
    path,
    toolName,
    toolCallId,
    action,
    timestamp: Date.now(),
  }
}

/**
 * Extract artifacts from a list of historical tool messages.
 */
export function extractArtifactsFromHistory(
  messages: Array<{ role: string; toolName?: string; toolCallId?: string; toolArgs?: string; toolResult?: string }>,
): Artifact[] {
  const artifacts: Artifact[] = []
  const seen = new Set<string>()

  for (const msg of messages) {
    if (msg.role !== 'tool' || !msg.toolName) continue
    const artifact = extractArtifact(msg.toolName, msg.toolCallId || '', msg.toolArgs, msg.toolResult)
    if (artifact && !seen.has(artifact.path)) {
      seen.add(artifact.path)
      artifacts.push(artifact)
    }
  }

  return artifacts
}

/**
 * Get a file extension icon based on the file path.
 */
export function fileIcon(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase() || ''
  switch (ext) {
    case 'ts': case 'tsx': return '\ud83d\udcd8' // blue book
    case 'js': case 'jsx': return '\ud83d\udcd9' // orange book
    case 'go': return '\ud83d\udc39' // hamster (gopher-like)
    case 'py': return '\ud83d\udc0d' // snake
    case 'rs': return '\u2699' // gear
    case 'md': return '\ud83d\udcdd' // memo
    case 'json': case 'yaml': case 'yml': case 'toml': return '\u2699' // gear
    case 'css': case 'scss': return '\ud83c\udfa8' // palette
    case 'html': case 'svelte': case 'vue': return '\ud83c\udf10' // globe
    case 'sql': return '\ud83d\uddc3' // file cabinet
    case 'sh': case 'bash': return '\ud83d\udcbb' // laptop
    case 'png': case 'jpg': case 'jpeg': case 'gif': case 'svg': return '\ud83d\uddbc' // frame
    default: return '\ud83d\udcc4' // page
  }
}
