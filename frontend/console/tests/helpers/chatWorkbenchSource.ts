// The chat workbench spans several files since Chat.svelte was split (#968).
// Source-level tests that ask "does the chat surface wire X" read all of
// them, so they keep passing when markup moves between these files.

import { readFileSync } from 'node:fs'

export const chatWorkbenchFiles = [
  '../../src/components/Chat.svelte',
  '../../src/components/ChatRail.svelte',
  '../../src/components/ChatStatusBar.svelte',
  '../../src/components/ChatSessionHeader.svelte',
  '../../src/components/ChatDockHost.svelte',
  '../../src/lib/stores/chatDockStore.svelte.ts',
] as const

export function readChatWorkbenchFile(relative: (typeof chatWorkbenchFiles)[number]): string {
  return readFileSync(new URL(relative, import.meta.url), 'utf8')
}

export const chatWorkbenchSource = chatWorkbenchFiles.map(readChatWorkbenchFile).join('\n')
