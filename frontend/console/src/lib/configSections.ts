// Groups the config schema's 19 server sections into a smaller set of
// console tabs (#1238), and decides which fields show inside a tab:
// `tier` hides file/remove fields entirely, and `show_when` hides a field
// until the condition it names holds (and that named field is itself
// shown) — see internal/config/schema_tier.go for the source of truth.
import type { ConfigFieldMeta } from './types'

export type ConfigTabId =
  | 'general'
  | 'llm'
  | 'chatTools'
  | 'automation'
  | 'channelsRemote'
  | 'agents'
  | 'extensions'
  | 'advanced'

export interface ConfigTabDef {
  id: ConfigTabId
  sections: string[]
}

// Tab order as shown in the console. "advanced" carries no schema sections
// of its own — it hosts server restart / danger zone actions, not fields.
export const CONFIG_TABS: ConfigTabDef[] = [
  { id: 'general', sections: ['Runtime', 'API', 'Console'] },
  { id: 'llm', sections: ['LLM'] },
  { id: 'chatTools', sections: ['Tools', 'Memory', 'Compaction'] },
  { id: 'automation', sections: ['Automation', 'Work Scheduler', 'Work Ledger', 'Usage', 'Initiative'] },
  { id: 'channelsRemote', sections: ['Channels', 'Remote Access'] },
  { id: 'agents', sections: ['Agent Runtime', 'Companion', 'Embodiment'] },
  { id: 'extensions', sections: ['Extensions', 'MCP'] },
  { id: 'advanced', sections: [] },
]

const SECTION_TO_TAB: Record<string, ConfigTabId> = (() => {
  const map: Record<string, ConfigTabId> = {}
  for (const tab of CONFIG_TABS) {
    for (const section of tab.sections) map[section] = tab.id
  }
  return map
})()

export function tabForSection(section: string): ConfigTabId {
  return SECTION_TO_TAB[section] ?? 'general'
}

export function defaultConfigTabId(): ConfigTabId {
  return CONFIG_TABS[0].id
}

export function isConfigTabId(value: string | null | undefined): value is ConfigTabId {
  return !!value && CONFIG_TABS.some((tab) => tab.id === value)
}

// Fields the console shows at all: file/remove tiers are YAML-only or on
// their way out (#1240) and never rendered, regardless of show_when.
export function isFieldConsoleVisible(field: ConfigFieldMeta): boolean {
  return field.tier === 'basic' || field.tier === 'advanced'
}

// Evaluates one field's show_when against the current (possibly dirty)
// values, string-compared like the server does (a bool is "true"/"false").
// A field with no show_when is always shown. Several conditions are OR'd.
export function isFieldShown(
  field: ConfigFieldMeta,
  values: Record<string, unknown>,
  allFields: ConfigFieldMeta[],
): boolean {
  if (!isFieldConsoleVisible(field)) return false
  const conditions = field.show_when
  if (!conditions || conditions.length === 0) return true
  return conditions.some((cond) => {
    const target = allFields.find((f) => f.key === cond.key)
    // The field naming the condition must itself be shown, or showWhen
    // never resolves (matches the server's recursive note in schema_tier.go).
    if (target && !isFieldShown(target, values, allFields)) return false
    const raw = values[cond.key]
    const asString = typeof raw === 'boolean' ? String(raw) : String(raw ?? '')
    return cond.values.includes(asString)
  })
}

export interface ConfigTabFields {
  id: ConfigTabId
  fields: ConfigFieldMeta[]
}

// Splits the schema into tabs, each holding only its console-visible,
// show_when-satisfied fields, in schema order.
export function groupFieldsByTab(
  fields: ConfigFieldMeta[],
  values: Record<string, unknown>,
): ConfigTabFields[] {
  const visible = fields.filter((f) => isFieldConsoleVisible(f) && isFieldShown(f, values, fields))
  const byTab = new Map<ConfigTabId, ConfigFieldMeta[]>()
  for (const tab of CONFIG_TABS) byTab.set(tab.id, [])
  for (const field of visible) {
    const tabId = tabForSection(field.section)
    byTab.get(tabId)?.push(field)
  }
  return CONFIG_TABS.map((tab) => ({ id: tab.id, fields: byTab.get(tab.id) ?? [] }))
}

export function basicFields(fields: ConfigFieldMeta[]): ConfigFieldMeta[] {
  return fields.filter((f) => f.tier === 'basic')
}

export function advancedFields(fields: ConfigFieldMeta[]): ConfigFieldMeta[] {
  return fields.filter((f) => f.tier === 'advanced')
}
