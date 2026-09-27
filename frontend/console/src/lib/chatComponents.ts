export type LazyChatComponent =
  | 'chat-panel'
  | 'artifact-panel'
  | 'terminal-tabs'
  | 'changes-panel'

type ChatComponentModule = { default: any }
type ChatComponentLoader = () => Promise<ChatComponentModule>

function memoizeChatLoader(loader: ChatComponentLoader): ChatComponentLoader {
  let promise: Promise<ChatComponentModule> | null = null
  return () => {
    promise ??= loader()
    return promise
  }
}

export const chatComponentLoaders = {
  'chat-panel': memoizeChatLoader(() => import('../components/ChatPanel.svelte')),
  'artifact-panel': memoizeChatLoader(() => import('../components/ArtifactPanel.svelte')),
  'terminal-tabs': memoizeChatLoader(() => import('../components/TerminalTabs.svelte')),
  'changes-panel': memoizeChatLoader(() => import('../components/ChangesPanel.svelte')),
} satisfies Record<LazyChatComponent, ChatComponentLoader>

export function loadChatComponent(component: LazyChatComponent): Promise<ChatComponentModule> {
  return chatComponentLoaders[component]()
}
