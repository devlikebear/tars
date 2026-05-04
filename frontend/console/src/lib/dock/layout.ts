export type DockZone = 'left' | 'right' | 'bottom' | 'fullscreen'

export type DockPanelDefinition = {
  id: string
  title: string
  defaultZone: DockZone
  closeable?: boolean
}

export type DockSizes = {
  left: number
  right: number
  bottom: number
}

export type DockLayoutState = {
  placements: Record<string, DockZone>
  active: Partial<Record<DockZone, string>>
  sizes: DockSizes
}

export type SerializedDockLayout = {
  placements?: Record<string, unknown>
  active?: Record<string, unknown>
  sizes?: Partial<Record<keyof DockSizes, unknown>>
}

export const defaultDockSizes: DockSizes = {
  left: 280,
  right: 320,
  bottom: 300,
}

const dockSizeLimits: Record<keyof DockSizes, { min: number; max: number }> = {
  left: { min: 220, max: 520 },
  right: { min: 320, max: 520 },
  bottom: { min: 180, max: 520 },
}

const zones: DockZone[] = ['left', 'right', 'bottom', 'fullscreen']

function isDockZone(value: unknown): value is DockZone {
  return typeof value === 'string' && zones.includes(value as DockZone)
}

function panelByID(panels: DockPanelDefinition[]): Map<string, DockPanelDefinition> {
  return new Map(panels.map((panel) => [panel.id, panel]))
}

function clampSize(zone: keyof DockSizes, value: unknown): number {
  const numeric = typeof value === 'number' && Number.isFinite(value) ? value : defaultDockSizes[zone]
  const limits = dockSizeLimits[zone]
  return Math.min(limits.max, Math.max(limits.min, Math.round(numeric)))
}

function defaultPlacements(panels: DockPanelDefinition[]): Record<string, DockZone> {
  const placements: Record<string, DockZone> = {}
  for (const panel of panels) {
    placements[panel.id] = panel.defaultZone
  }
  return placements
}

function defaultActive(panels: DockPanelDefinition[], placements: Record<string, DockZone>): Partial<Record<DockZone, string>> {
  const active: Partial<Record<DockZone, string>> = {}
  for (const panel of panels) {
    if (panel.closeable === false) {
      active[placements[panel.id] ?? panel.defaultZone] = panel.id
    }
  }
  return active
}

function removeActivePanel(active: Partial<Record<DockZone, string>>, panelID: string): Partial<Record<DockZone, string>> {
  const next = { ...active }
  for (const zone of zones) {
    if (next[zone] === panelID) {
      delete next[zone]
    }
  }
  return next
}

export function createDockLayout(panels: DockPanelDefinition[]): DockLayoutState {
  const placements = defaultPlacements(panels)
  return {
    placements,
    active: defaultActive(panels, placements),
    sizes: { ...defaultDockSizes },
  }
}

export function normalizeDockLayout(raw: unknown, panels: DockPanelDefinition[]): DockLayoutState {
  const byID = panelByID(panels)
  const fallback = createDockLayout(panels)
  const input = raw && typeof raw === 'object' ? raw as SerializedDockLayout : {}
  const rawPlacements = input.placements && typeof input.placements === 'object' ? input.placements : {}
  const rawActive = input.active && typeof input.active === 'object' ? input.active : {}
  const rawSizes = input.sizes && typeof input.sizes === 'object' ? input.sizes : {}

  const placements = { ...fallback.placements }
  for (const [panelID, zone] of Object.entries(rawPlacements)) {
    if (!byID.has(panelID) || !isDockZone(zone)) continue
    placements[panelID] = zone
  }

  const active = defaultActive(panels, placements)
  for (const [zone, panelID] of Object.entries(rawActive)) {
    if (!isDockZone(zone) || typeof panelID !== 'string' || !byID.has(panelID)) continue
    if (placements[panelID] !== zone) {
      placements[panelID] = zone
    }
    active[zone] = panelID
  }

  return {
    placements,
    active,
    sizes: {
      left: clampSize('left', rawSizes.left),
      right: clampSize('right', rawSizes.right),
      bottom: clampSize('bottom', rawSizes.bottom),
    },
  }
}

export function serializeDockLayout(layout: DockLayoutState): SerializedDockLayout {
  return {
    placements: { ...layout.placements },
    active: { ...layout.active },
    sizes: { ...layout.sizes },
  }
}

export function openDockPanel(
  layout: DockLayoutState,
  panels: DockPanelDefinition[],
  panelID: string,
): DockLayoutState {
  const panel = panelByID(panels).get(panelID)
  if (!panel) return layout
  const zone = layout.placements[panelID] ?? panel.defaultZone
  const active = removeActivePanel(layout.active, panelID)
  active[zone] = panelID
  return {
    ...layout,
    active,
    placements: {
      ...layout.placements,
      [panelID]: zone,
    },
  }
}

export function closeDockPanel(layout: DockLayoutState, panelID: string): DockLayoutState {
  return {
    ...layout,
    active: removeActivePanel(layout.active, panelID),
  }
}

export function moveDockPanel(
  layout: DockLayoutState,
  panels: DockPanelDefinition[],
  panelID: string,
  zone: DockZone,
): DockLayoutState {
  if (!panelByID(panels).has(panelID)) return layout
  const active = removeActivePanel(layout.active, panelID)
  active[zone] = panelID
  return {
    ...layout,
    active,
    placements: {
      ...layout.placements,
      [panelID]: zone,
    },
  }
}

export function resizeDock(layout: DockLayoutState, zone: keyof DockSizes, size: number): DockLayoutState {
  return {
    ...layout,
    sizes: {
      ...layout.sizes,
      [zone]: clampSize(zone, size),
    },
  }
}

export function panelIsOpen(layout: DockLayoutState, panelID: string): boolean {
  return zones.some((zone) => layout.active[zone] === panelID)
}

export function activePanelZone(layout: DockLayoutState, panelID: string): DockZone | undefined {
  return zones.find((zone) => layout.active[zone] === panelID)
}
