import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import {
  getMemoryAssetMetadata,
  isMemoryAssetStale,
} from '../src/lib/memoryAssetMetadata.ts'
import { en } from '../src/i18n/en.ts'
import type { MemoryAsset } from '../src/lib/types.ts'

const memorySource = readFileSync(new URL('../src/components/MemoryCenter.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api/memory.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

function asset(partial: Partial<MemoryAsset>): MemoryAsset {
  return {
    path: partial.path || 'MEMORY.md',
    kind: partial.kind || 'long_term_memory',
    editable: partial.editable ?? true,
    size_bytes: partial.size_bytes ?? 0,
    updated_at: partial.updated_at,
  }
}

test('memory asset metadata explains who fills and reads each durable asset', () => {
  const memory = getMemoryAssetMetadata(asset({ path: 'MEMORY.md', kind: 'long_term_memory' }))
  assert.match(memory.description, /Core user facts/)
  assert.match(memory.filledBy.join(' '), /Manual edits/)
  assert.match(memory.filledBy.join(' '), /remember/)
  assert.match(memory.readBy.join(' '), /Prior Context prefetch/)
  assert.match(memory.readBy.join(' '), /memory_search/)

  const experiences = getMemoryAssetMetadata(asset({ path: 'experiences.jsonl', kind: 'experience_log' }))
  assert.match(experiences.description, /Approved experience log/)
  assert.match(experiences.filledBy.join(' '), /Memory Inbox/)
  assert.match(experiences.readBy.join(' '), /semantic prefetch/)
  assert.equal(experiences.staleAfterDays, 7)

  const inbox = getMemoryAssetMetadata(asset({ path: 'inbox.jsonl', kind: 'memory_inbox' }))
  assert.match(inbox.description, /Review queue/)
  assert.match(inbox.filledBy.join(' '), /Reflection nightly memory job/)
  assert.match(inbox.readBy.join(' '), /Memory Inbox review/)

  const daily = getMemoryAssetMetadata(asset({ path: 'daily.jsonl', kind: 'daily_memory' }))
  assert.match(daily.description, /Daily activity log/)

  const semantic = getMemoryAssetMetadata(asset({ path: 'semantic.json', kind: 'semantic_index' }))
  assert.match(semantic.description, /Embedding index/)
})

test('experience logs become stale after seven quiet days', () => {
  const now = new Date('2026-04-30T00:00:00Z')
  assert.equal(
    isMemoryAssetStale(asset({ path: 'experiences.jsonl', kind: 'experience_log', updated_at: '2026-04-20T00:00:00Z' }), now),
    true,
  )
  assert.equal(
    isMemoryAssetStale(asset({ path: 'experiences.jsonl', kind: 'experience_log', updated_at: '2026-04-29T00:00:00Z' }), now),
    false,
  )
  assert.equal(
    isMemoryAssetStale(asset({ path: 'MEMORY.md', kind: 'long_term_memory', updated_at: '2026-04-01T00:00:00Z' }), now),
    false,
  )
})

test('Memory page renders filled/read metadata and stale badges on asset cards', () => {
  assert.match(memorySource, /getMemoryAssetMetadata/)
  assert.match(memorySource, /asset-flow-line/)
  assert.match(memorySource, /\$t\.memory\.filledBy/)
  assert.match(memorySource, /\$t\.memory\.readBy/)
  assert.match(memorySource, /\$t\.memory\.stale/)
})

test('Memory page uses friendly tab labels and asset descriptions', () => {
  assert.equal(en.memory.tabs.storedKnowledge, 'Stored Knowledge')
  assert.equal(en.memory.tabs.trySearch, 'Try a Search')
  assert.match(memorySource, /\$t\.memory\.tabs\.storedKnowledge/)
  assert.match(memorySource, /\$t\.memory\.tabs\.trySearch/)
  assert.doesNotMatch(memorySource, />Durable Memory</)
  assert.doesNotMatch(memorySource, />Search Test</)
  assert.match(memorySource, /asset-description/)
  assert.match(memorySource, /title=\{metadata\.description\}/)
})

test('Memory page introduces memory assets before editing', () => {
  assert.match(memorySource, /memoryIntroDismissed/)
  assert.match(memorySource, /memory-intro-card/)
  assert.match(memorySource, /\$t\.common\.actions\.dismiss/)
  assert.match(memorySource, /\$t\.memory\.introTitle/)
  assert.equal(en.memory.introHeadings.memory, 'MEMORY.md')
  assert.equal(en.memory.introHeadings.experiences, 'Experiences')
  assert.equal(en.memory.introHeadings.dailyLogs, 'Daily Logs')
  assert.equal(en.memory.introHeadings.semanticIndex, 'Semantic Index')
})

test('Memory search offers Tool path and Prefetch path debug modes', () => {
  assert.match(memorySource, /type MemorySearchMode = 'tool' \| 'prefetch'/)
  assert.match(memorySource, /searchMode = \$state<MemorySearchMode>\('tool'\)/)
  assert.match(memorySource, /\$t\.memory\.search\.toolPath/)
  assert.match(memorySource, /\$t\.memory\.search\.prefetchPath/)
  assert.match(memorySource, /runMemoryPrefetch/)
  assert.match(memorySource, /prefetchResult/)
  assert.match(memorySource, /prefetch-section/)
  assert.match(apiSource, /runMemoryPrefetch/)
  assert.match(apiSource, /\/v1\/memory\/prefetch/)
  assert.match(typesSource, /export type MemoryPrefetchResult/)
  assert.match(typesSource, /source_tag/)
  assert.match(typesSource, /budget_percent/)
})

test('Memory page exposes review-before-store inbox controls', () => {
  assert.equal(en.memory.tabs.inbox, 'Inbox')
  assert.match(memorySource, /type MemoryTab = 'inbox' \| 'durable' \| 'search'/)
  assert.match(memorySource, /loadMemoryInbox/)
  assert.match(memorySource, /reviewMemoryCandidate/)
  assert.match(memorySource, /\$t\.memory\.inbox\.approve/)
  assert.match(memorySource, /\$t\.memory\.inbox\.reject/)
  assert.match(memorySource, /\$t\.memory\.inbox\.merge/)
  assert.match(apiSource, /listMemoryInbox/)
  assert.match(apiSource, /\/v1\/memory\/inbox/)
  assert.match(apiSource, /reviewMemoryCandidate/)
  assert.match(apiSource, /\/v1\/memory\/inbox\/review/)
  assert.match(typesSource, /export type MemoryCandidate/)
  assert.match(typesSource, /MemoryCandidateHint/)
  assert.match(typesSource, /MemoryCandidateProvenance/)
})
