import type { MemoryAsset } from './types'

export type MemoryAssetMetadata = {
  description: string
  filledBy: string[]
  readBy: string[]
  staleAfterDays?: number
}

const memoryMetadata: MemoryAssetMetadata = {
  description: 'Core user facts, preferences, and rules. Editable by hand.',
  filledBy: ['Manual edits', 'remember command'],
  readBy: ['Every chat turn via Prior Context prefetch', 'memory_search tool'],
}

const experienceMetadata: MemoryAssetMetadata = {
  description: 'Automatically extracted experience log filled by reflection.',
  filledBy: ['Reflection nightly memory job (02:00-05:00)', 'Experience extraction from conversations'],
  readBy: ['memory_search tool', 'semantic prefetch'],
  staleAfterDays: 7,
}

const dailyMetadata: MemoryAssetMetadata = {
  description: 'Daily activity log captured from chat turns.',
  filledBy: ['Per-turn chat memory hook', 'Daily log append'],
  readBy: ['memory_search tool', 'daily memory recall'],
}

const semanticIndexMetadata: MemoryAssetMetadata = {
  description: 'Embedding index managed automatically; not usually edited.',
  filledBy: ['Embedding/index refresh pipeline'],
  readBy: ['semantic prefetch', 'memory_search ranking'],
}

const fallbackMetadata: MemoryAssetMetadata = {
  description: 'Memory subsystem asset. Inspect before editing.',
  filledBy: ['Memory subsystem', 'manual editor updates when editable'],
  readBy: ['Memory page editor', 'memory APIs when requested'],
}

export function getMemoryAssetMetadata(asset: Pick<MemoryAsset, 'kind' | 'path'>): MemoryAssetMetadata {
  const path = asset.path.toLowerCase()
  switch (asset.kind) {
    case 'long_term_memory':
      return memoryMetadata
    case 'experience_log':
      return experienceMetadata
    case 'daily_memory':
      return dailyMetadata
    case 'semantic_index':
    case 'semantic_raw':
      return semanticIndexMetadata
    default:
      if (path.endsWith('memory.md')) return memoryMetadata
      if (path.includes('experience')) return experienceMetadata
      if (path.includes('daily')) return dailyMetadata
      if (path.includes('semantic') || path.includes('embedding')) return semanticIndexMetadata
      return fallbackMetadata
  }
}

export function isMemoryAssetStale(
  asset: Pick<MemoryAsset, 'kind' | 'path' | 'updated_at'>,
  now = new Date(),
): boolean {
  const metadata = getMemoryAssetMetadata(asset)
  if (!metadata.staleAfterDays || !asset.updated_at) return false

  const updatedAt = new Date(asset.updated_at)
  if (Number.isNaN(updatedAt.getTime()) || Number.isNaN(now.getTime())) return false

  const staleAfterMs = metadata.staleAfterDays * 24 * 60 * 60 * 1000
  return now.getTime() - updatedAt.getTime() >= staleAfterMs
}
