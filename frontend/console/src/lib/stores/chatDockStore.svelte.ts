// Dock layout for the chat workbench (#968): which panel is open in which
// zone, and how big each zone is.
//
// Shared by the toolbar toggles, the session header, and ChatDockHost so
// they read and change one layout instead of passing callbacks around.
// Titles are display text and stay in the components (they are i18n);
// the layout logic never reads them.

import {
  closeDockPanel,
  createDockLayout,
  moveDockPanel,
  normalizeDockLayout,
  openDockPanel,
  panelIsOpen,
  resizeDock,
  serializeDockLayout,
  type DockLayoutState,
  type DockPanelDefinition,
  type DockSizes,
  type DockZone,
} from '../dock/layout'

export type ChatDockPanelID = 'sessions' | 'artifacts' | 'config' | 'context' | 'prompt' | 'prior' | 'tasks' | 'git' | 'skillExtraction' | 'cron' | 'health' | 'terminal'
export type ToolDockPanelID = Exclude<ChatDockPanelID, 'sessions'>

export const chatDockPanels: DockPanelDefinition[] = [
  { id: 'sessions', title: 'sessions', defaultZone: 'left', closeable: false },
  { id: 'artifacts', title: 'artifacts', defaultZone: 'right' },
  { id: 'config', title: 'config', defaultZone: 'right' },
  { id: 'context', title: 'context', defaultZone: 'right' },
  { id: 'prompt', title: 'prompt', defaultZone: 'right' },
  { id: 'prior', title: 'prior', defaultZone: 'right' },
  { id: 'tasks', title: 'tasks', defaultZone: 'right' },
  { id: 'git', title: 'git', defaultZone: 'right' },
  { id: 'skillExtraction', title: 'skillExtraction', defaultZone: 'right' },
  { id: 'cron', title: 'cron', defaultZone: 'right' },
  { id: 'health', title: 'health', defaultZone: 'right' },
  { id: 'terminal', title: 'terminal', defaultZone: 'bottom' },
]

const toolPanels: ToolDockPanelID[] = ['artifacts', 'config', 'context', 'prompt', 'prior', 'tasks', 'git', 'skillExtraction', 'cron', 'health', 'terminal']
// Panels that count as "the user is looking at a tool" (the terminal does not).
const inspectorPanels: ToolDockPanelID[] = toolPanels.filter((id) => id !== 'terminal')

export const chatDockStorageKey = 'tars.console.chat.dockLayout.v1'

// Below this width the chat layout collapses; dock panels render as fullscreen overlays.
export const MOBILE_LAYOUT_MAX_WIDTH = 900

export function isMobileLayout(): boolean {
  return typeof window !== 'undefined' && window.matchMedia(`(max-width: ${MOBILE_LAYOUT_MAX_WIDTH}px)`).matches
}

type StorageLike = Pick<Storage, 'getItem' | 'setItem'>

export class ChatDockStore {
  layout = $state<DockLayoutState>(createDockLayout(chatDockPanels))

  get left(): ChatDockPanelID | undefined { return this.layout.active.left as ChatDockPanelID | undefined }
  get right(): ChatDockPanelID | undefined { return this.layout.active.right as ChatDockPanelID | undefined }
  get bottom(): ChatDockPanelID | undefined { return this.layout.active.bottom as ChatDockPanelID | undefined }
  get fullscreen(): ChatDockPanelID | undefined { return this.layout.active.fullscreen as ChatDockPanelID | undefined }

  isOpen(panelID: ChatDockPanelID): boolean {
    return panelIsOpen(this.layout, panelID)
  }

  // True when any inspector panel (not the session list or terminal) is open.
  get anyToolPanelOpen(): boolean {
    return inspectorPanels.some((id) => panelIsOpen(this.layout, id))
  }

  open(panelID: ChatDockPanelID): void {
    this.layout = openDockPanel(this.layout, chatDockPanels, panelID)
  }

  close(panelID: ChatDockPanelID): void {
    this.layout = closeDockPanel(this.layout, panelID)
  }

  toggle(panelID: ChatDockPanelID): void {
    if (this.isOpen(panelID)) {
      this.close(panelID)
    } else {
      this.open(panelID)
    }
  }

  move(panelID: ChatDockPanelID, zone: DockZone): void {
    this.layout = moveDockPanel(this.layout, chatDockPanels, panelID, zone)
  }

  resize(zone: keyof DockSizes, size: number): void {
    this.layout = resizeDock(this.layout, zone, size)
  }

  closeToolPanels(): void {
    let next = this.layout
    for (const panelID of toolPanels) next = closeDockPanel(next, panelID)
    this.layout = next
  }

  // Which zone a panel is currently shown in, if any.
  zoneOf(panelID: ChatDockPanelID): DockZone | null {
    if (this.fullscreen === panelID) return 'fullscreen'
    if (this.bottom === panelID) return 'bottom'
    if (this.left === panelID) return 'left'
    if (this.right === panelID) return 'right'
    return null
  }

  restore(storage: StorageLike | undefined): void {
    try {
      const stored = storage?.getItem(chatDockStorageKey)
      if (stored) this.layout = normalizeDockLayout(JSON.parse(stored), chatDockPanels)
    } catch {
      this.layout = createDockLayout(chatDockPanels)
    }
  }

  persist(storage: StorageLike | undefined): void {
    try {
      storage?.setItem(chatDockStorageKey, JSON.stringify(serializeDockLayout(this.layout)))
    } catch { /* quota or privacy mode */ }
  }
}

export const chatDock = new ChatDockStore()
