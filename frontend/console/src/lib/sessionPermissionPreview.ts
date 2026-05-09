import type { ChatToolInfo, SessionToolConfig } from './api'
import { sortStrings } from './sort.js'

export type PermissionRisk = 'low' | 'medium' | 'high'

export type SessionPermissionPreview = {
  summary: string
  risk: PermissionRisk
  capabilities: string[]
  gainedTools: string[]
  lostTools: string[]
  gainedHighRiskTools: string[]
  gainedGroups: string[]
  lostGroups: string[]
  gainedSkills: string[]
  lostSkills: string[]
  gainedCommands: string[]
  lostCommands: string[]
  gainedMCPServers: string[]
  lostMCPServers: string[]
}

export type SessionPermissionPreviewCatalog = {
  tools: ChatToolInfo[]
  skills?: string[]
  commands?: string[]
  mcpServers?: string[]
}

export function buildSessionPermissionPreview(
  before: SessionToolConfig,
  after: SessionToolConfig,
  catalog: SessionPermissionPreviewCatalog,
): SessionPermissionPreview {
  const tools = catalog.tools ?? []
  const beforeTools = effectiveToolNames(before, tools)
  const afterTools = effectiveToolNames(after, tools)
  const beforeSkills = effectiveNamedSet(before.skills_custom, before.skills_enabled, catalog.skills ?? [])
  const afterSkills = effectiveNamedSet(after.skills_custom, after.skills_enabled, catalog.skills ?? [])
  const beforeCommands = effectiveNamedSet(before.commands_custom, before.commands_enabled, catalog.commands ?? [])
  const afterCommands = effectiveNamedSet(after.commands_custom, after.commands_enabled, catalog.commands ?? [])
  const beforeMCP = effectiveNamedSet(before.mcp_custom || Array.isArray(before.mcp_enabled), before.mcp_enabled, catalog.mcpServers ?? [])
  const afterMCP = effectiveNamedSet(after.mcp_custom || Array.isArray(after.mcp_enabled), after.mcp_enabled, catalog.mcpServers ?? [])

  const gainedTools = diffSet(afterTools, beforeTools)
  const lostTools = diffSet(beforeTools, afterTools)
  const gainedSkills = diffSet(afterSkills, beforeSkills)
  const lostSkills = diffSet(beforeSkills, afterSkills)
  const gainedCommands = diffSet(afterCommands, beforeCommands)
  const lostCommands = diffSet(beforeCommands, afterCommands)
  const gainedMCPServers = diffSet(afterMCP, beforeMCP)
  const lostMCPServers = diffSet(beforeMCP, afterMCP)
  const beforeGroups = groupsForTools([...beforeTools], tools)
  const afterGroups = groupsForTools([...afterTools], tools)
  const gainedGroups = diffSet(afterGroups, beforeGroups)
  const lostGroups = diffSet(beforeGroups, afterGroups)
  const gainedHighRiskTools = gainedTools.filter((name) => tools.find((tool) => tool.name === name)?.high_risk)
  const capabilities = capabilitiesForTools(gainedTools, tools)
  const risk = determineRisk(gainedHighRiskTools, capabilities, gainedTools, gainedSkills, gainedCommands, gainedMCPServers)

  return {
    summary: buildPermissionSummary(gainedTools, lostTools, gainedSkills, lostSkills, gainedCommands, lostCommands, gainedMCPServers, lostMCPServers),
    risk,
    capabilities,
    gainedTools,
    lostTools,
    gainedHighRiskTools,
    gainedGroups,
    lostGroups,
    gainedSkills,
    lostSkills,
    gainedCommands,
    lostCommands,
    gainedMCPServers,
    lostMCPServers,
  }
}

function effectiveToolNames(config: SessionToolConfig, tools: ChatToolInfo[]): Set<string> {
  const toolNames = tools.map((tool) => tool.name)
  const allowGroups = new Set(config.tools_allow_groups ?? [])
  const denyGroups = new Set(config.tools_deny_groups ?? [])
  const disabled = new Set(config.tools_disabled ?? [])
  const useAllowGroups = allowGroups.size > 0
  const useAllowTools = (config.tools_enabled?.length ?? 0) > 0 || (!!config.tools_custom && !useAllowGroups)
  let names: string[]

  if (useAllowGroups) {
    names = tools.filter((tool) => tool.group && allowGroups.has(tool.group)).map((tool) => tool.name)
  } else if (useAllowTools) {
    names = [...(config.tools_enabled ?? [])]
  } else {
    names = [...toolNames]
  }

  const byName = new Map(tools.map((tool) => [tool.name, tool]))
  return new Set(
    names
      .filter((name) => toolNames.includes(name))
      .filter((name) => !disabled.has(name))
      .filter((name) => {
        const group = byName.get(name)?.group
        return !group || !denyGroups.has(group)
      }),
  )
}

function effectiveNamedSet(custom: boolean | undefined, enabled: string[] | undefined, all: string[]): Set<string> {
  if (custom || Array.isArray(enabled)) {
    return new Set((enabled ?? []).filter((name) => name.trim()))
  }
  return new Set(all)
}

function diffSet(left: Set<string>, right: Set<string>): string[] {
  return [...left].filter((item) => !right.has(item))
}

function groupsForTools(names: string[], tools: ChatToolInfo[]): Set<string> {
  const byName = new Map(tools.map((tool) => [tool.name, tool]))
  return new Set(
    names
      .map((name) => byName.get(name)?.group ?? '')
      .filter(Boolean),
  )
}

function capabilitiesForTools(names: string[], tools: ChatToolInfo[]): string[] {
  const byName = new Map(tools.map((tool) => [tool.name, tool]))
  const capabilities = new Set<string>()
  for (const name of names) {
    const group = byName.get(name)?.group ?? ''
    const lower = `${name} ${group}`.toLowerCase()
    if (lower.includes('shell') || lower.includes('exec') || lower.includes('process')) capabilities.add('shell')
    if (lower.includes('file') || lower.includes('workspace')) capabilities.add('files')
    if (lower.includes('git')) capabilities.add('git')
    if (lower.includes('web') || lower.includes('network') || lower.includes('telegram')) capabilities.add('network')
  }
  return sortStrings(capabilities)
}

function determineRisk(
  gainedHighRiskTools: string[],
  capabilities: string[],
  gainedTools: string[],
  gainedSkills: string[],
  gainedCommands: string[],
  gainedMCPServers: string[],
): PermissionRisk {
  if (gainedHighRiskTools.length > 0 || capabilities.includes('shell')) return 'high'
  if (gainedTools.length > 0 || gainedSkills.length > 0 || gainedCommands.length > 0 || gainedMCPServers.length > 0) return 'medium'
  return 'low'
}

function buildPermissionSummary(
  gainedTools: string[],
  lostTools: string[],
  gainedSkills: string[],
  lostSkills: string[],
  gainedCommands: string[],
  lostCommands: string[],
  gainedMCPServers: string[],
  lostMCPServers: string[],
): string {
  const parts = [
    countLabel(gainedTools.length, 'tool', 'enabled'),
    countLabel(lostTools.length, 'tool', 'disabled'),
    countLabel(gainedSkills.length, 'skill', 'enabled'),
    countLabel(lostSkills.length, 'skill', 'disabled'),
    countLabel(gainedCommands.length, 'command', 'enabled'),
    countLabel(lostCommands.length, 'command', 'disabled'),
    countLabel(gainedMCPServers.length, 'MCP server', 'enabled'),
    countLabel(lostMCPServers.length, 'MCP server', 'disabled'),
  ].filter(Boolean)
  return parts.length > 0 ? parts.join(', ') : 'No effective permission change'
}

function countLabel(count: number, noun: string, action: string): string {
  if (count === 0) return ''
  return `${count} ${noun}${count === 1 ? '' : 's'} ${action}`
}
