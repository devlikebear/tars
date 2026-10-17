// Cross-tab config search (#1239): matches on the current-language label,
// the English (server) label/description, the YAML path, the internal key,
// the section name and the overriding environment variable name. Grouped by
// section with counts, plus three quick filters that need no text query.
import { tabForSection, isFieldConsoleVisible } from './configSections.ts'
import type { ConfigTabId } from './configSections'
import { configValuesEqual } from './configStructured.ts'
import type { ConfigEnvOverride, ConfigFieldMeta } from './types'

export type ConfigQuickFilter = 'changedFromDefault' | 'envOverridden' | 'unsaved'

export interface ConfigFieldLocalization {
  label: string
  description: string
  sectionLabel: string
}

export type ConfigLocalize = (field: ConfigFieldMeta) => ConfigFieldLocalization

// A highlighted text segment: `matched` spans are the parts of `text` that
// the query hit (case-insensitive); render with <mark> or similar.
export interface ConfigTextSegment {
  text: string
  matched: boolean
}

export interface ConfigSearchHit {
  field: ConfigFieldMeta
  tabId: ConfigTabId
  label: string
  description: string
  labelSegments: ConfigTextSegment[]
  descriptionSegments: ConfigTextSegment[]
}

export interface ConfigSearchSectionGroup {
  section: string
  sectionLabel: string
  tabId: ConfigTabId
  hits: ConfigSearchHit[]
}

export interface ConfigSearchParams {
  fields: ConfigFieldMeta[]
  query: string
  localize: ConfigLocalize
  values: Record<string, unknown>
  dirtyFields: Record<string, unknown>
  envOverrides: Record<string, ConfigEnvOverride>
  filters?: ConfigQuickFilter[]
}

export interface ConfigSearchResult {
  query: string
  groups: ConfigSearchSectionGroup[]
  totalCount: number
}

function matchesFilters(
  field: ConfigFieldMeta,
  values: Record<string, unknown>,
  dirtyFields: Record<string, unknown>,
  envOverrides: Record<string, ConfigEnvOverride>,
  filters: ConfigQuickFilter[],
): boolean {
  return filters.every((filter) => {
    switch (filter) {
      case 'changedFromDefault':
        return !configValuesEqual(values[field.key], field.default_value)
      case 'envOverridden':
        return Object.prototype.hasOwnProperty.call(envOverrides, field.key)
      case 'unsaved':
        return Object.prototype.hasOwnProperty.call(dirtyFields, field.key)
      default:
        return true
    }
  })
}

function normalizedHaystacks(
  field: ConfigFieldMeta,
  loc: ConfigFieldLocalization,
  envOverrides: Record<string, ConfigEnvOverride>,
): string[] {
  const haystacks = [
    loc.label,
    field.label,
    loc.description,
    field.description,
    field.path,
    field.key,
    loc.sectionLabel,
    field.section,
  ]
  const env = envOverrides[field.key]?.env_key
  if (env) haystacks.push(env)
  return haystacks.filter(Boolean)
}

function fieldMatchesQuery(haystacks: string[], needle: string): boolean {
  if (!needle) return true
  return haystacks.some((h) => h.toLowerCase().includes(needle))
}

// Splits `text` into segments, marking every (case-insensitive,
// non-overlapping) occurrence of `query`. An empty query marks nothing.
export function highlightConfigText(text: string, query: string): ConfigTextSegment[] {
  if (!query || !text) return [{ text, matched: false }]
  const lower = text.toLowerCase()
  const needle = query.toLowerCase()
  const segments: ConfigTextSegment[] = []
  let cursor = 0
  while (cursor < text.length) {
    const idx = lower.indexOf(needle, cursor)
    if (idx === -1) {
      segments.push({ text: text.slice(cursor), matched: false })
      break
    }
    if (idx > cursor) segments.push({ text: text.slice(cursor, idx), matched: false })
    segments.push({ text: text.slice(idx, idx + needle.length), matched: true })
    cursor = idx + needle.length
  }
  return segments
}

// Searches across every tab's console-visible fields (callers pass the
// already show_when-filtered field list), groups matches by section in
// schema order, and reports a result count per group.
export function searchConfigFields(params: ConfigSearchParams): ConfigSearchResult {
  const { fields, localize, values, dirtyFields, envOverrides } = params
  const query = params.query.trim()
  const needle = query.toLowerCase()
  const filters = params.filters ?? []

  const groups: ConfigSearchSectionGroup[] = []
  const groupBySection = new Map<string, ConfigSearchSectionGroup>()
  let totalCount = 0

  for (const field of fields) {
    if (!isFieldConsoleVisible(field)) continue
    if (!matchesFilters(field, values, dirtyFields, envOverrides, filters)) continue
    const loc = localize(field)
    const haystacks = normalizedHaystacks(field, loc, envOverrides)
    if (!fieldMatchesQuery(haystacks, needle)) continue

    let group = groupBySection.get(field.section)
    if (!group) {
      group = { section: field.section, sectionLabel: loc.sectionLabel, tabId: tabForSection(field.section), hits: [] }
      groupBySection.set(field.section, group)
      groups.push(group)
    }
    group.hits.push({
      field,
      tabId: group.tabId,
      label: loc.label,
      description: loc.description,
      labelSegments: highlightConfigText(loc.label, query),
      descriptionSegments: highlightConfigText(loc.description, query),
    })
    totalCount += 1
  }

  return { query, groups, totalCount }
}
