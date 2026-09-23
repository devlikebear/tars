// Requests from outside the chat route (command palette, global shortcuts)
// for things only the mounted chat workbench can do: run a slash command,
// open a new session, toggle the terminal, jump to the Nth listed session.
//
// App enqueues and navigates to the chat route if needed; Chat drains the
// queue whenever it is mounted. Nothing here reaches into components.

export type ChatCommand =
  | { kind: 'slash'; command: string; args?: string }
  | { kind: 'new-session' }
  | { kind: 'toggle-terminal' }
  | { kind: 'switch-session'; index: number }

export class ChatCommandQueue {
  pending = $state<ChatCommand[]>([])

  // Session ids in the order the sidebar shows them, published by the
  // sidebar so "session N" means what the user sees.
  visibleSessionOrder = $state<string[]>([])

  request(command: ChatCommand): void {
    this.pending = [...this.pending, command]
  }

  // Remove and return everything queued, oldest first.
  take(): ChatCommand[] {
    const commands = this.pending
    if (commands.length) this.pending = []
    return commands
  }

  // The id of the Nth visible session, if there is one.
  sessionAt(index: number, fallback: string[] = []): string | undefined {
    const order = this.visibleSessionOrder.length ? this.visibleSessionOrder : fallback
    return order[index]
  }
}

export const chatCommands = new ChatCommandQueue()
