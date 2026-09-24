export type DockZone = 'left' | 'right' | 'bottom' | 'fullscreen'

export type DockPanelDefinition = {
  id: string
  title: string
  defaultZone: DockZone
  closeable?: boolean
}

// One entry of a zone's tab strip, as the panel frame renders it.
export type DockTab = {
  id: string
  title: string
  closeable: boolean
}

export type DockSizes = {
  left: number
  right: number
  bottom: number
}

// A zone holds an ordered stack of open panels shown as tabs; `active` is
// the tab on top. `placements` remembers each panel's zone while it is
// closed so it reopens where the user left it.
export type DockLayoutState = {
  placements: Record<string, DockZone>
  open: Partial<Record<DockZone, string[]>>
  active: Partial<Record<DockZone, string>>
  sizes: DockSizes
}

// Layouts stored before tabs (#968) have no `open`; normalize seeds each
// zone's stack from its active panel.
export type SerializedDockLayout = {
  placements?: Record<string, unknown>
  open?: Record<string, unknown>
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

function objectOrEmpty(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? value as Record<string, unknown> : {}
}

function defaultPlacements(panels: DockPanelDefinition[]): Record<string, DockZone> {
  const placements: Record<string, DockZone> = {}
  for (const panel of panels) {
    placements[panel.id] = panel.defaultZone
  }
  return placements
}

// Removes a panel from whichever stack holds it. When it was the active
// tab, its neighbor takes over (the next tab, else the previous one).
function detachPanel(layout: DockLayoutState, panelID: string): Pick<DockLayoutState, 'open' | 'active'> {
  const open = { ...layout.open }
  const active = { ...layout.active }
  for (const zone of zones) {
    const stack = open[zone] ?? []
    const index = stack.indexOf(panelID)
    if (index === -1) continue
    const rest = stack.filter((id) => id !== panelID)
    if (rest.length > 0) {
      open[zone] = rest
    } else {
      delete open[zone]
    }
    if (active[zone] !== panelID) continue
    if (rest.length > 0) {
      active[zone] = rest[Math.min(index, rest.length - 1)]
    } else {
      delete active[zone]
    }
  }
  return { open, active }
}

// Puts a panel on top of a zone's stack, taking it out of any other zone.
function attachPanel(layout: DockLayoutState, panelID: string, zone: DockZone): DockLayoutState {
  const { open, active } = detachPanel(layout, panelID)
  open[zone] = [...(open[zone] ?? []), panelID]
  active[zone] = panelID
  return {
    ...layout,
    open,
    active,
    placements: {
      ...layout.placements,
      [panelID]: zone,
    },
  }
}

export function createDockLayout(panels: DockPanelDefinition[]): DockLayoutState {
  let layout: DockLayoutState = {
    placements: defaultPlacements(panels),
    open: {},
    active: {},
    sizes: { ...defaultDockSizes },
  }
  for (const panel of panels) {
    if (panel.closeable === false) layout = attachPanel(layout, panel.id, panel.defaultZone)
  }
  return layout
}

function normalizePlacements(
  raw: Record<string, unknown>,
  byID: Map<string, DockPanelDefinition>,
  fallback: Record<string, DockZone>,
): Record<string, DockZone> {
  const placements = { ...fallback }
  for (const [panelID, zone] of Object.entries(raw)) {
    if (byID.has(panelID) && isDockZone(zone)) placements[panelID] = zone
  }
  return placements
}

// Stored stacks, or for a pre-tabs layout the single active panel per zone.
function storedStacks(input: SerializedDockLayout): Array<[DockZone, string[]]> {
  const source = input.open
    ? objectOrEmpty(input.open)
    : Object.fromEntries(Object.entries(objectOrEmpty(input.active)).map(([zone, id]) => [zone, [id]]))
  const stacks: Array<[DockZone, string[]]> = []
  for (const [zone, ids] of Object.entries(source)) {
    if (!isDockZone(zone) || !Array.isArray(ids)) continue
    stacks.push([zone, ids.filter((id): id is string => typeof id === 'string')])
  }
  return stacks
}

// Brings back the stored top tab of each zone, when it is still in that stack.
function restoreActiveTabs(layout: DockLayoutState, rawActive: Record<string, unknown>): Partial<Record<DockZone, string>> {
  const active = { ...layout.active }
  for (const [zone, panelID] of Object.entries(rawActive)) {
    if (isDockZone(zone) && typeof panelID === 'string' && layout.open[zone]?.includes(panelID)) {
      active[zone] = panelID
    }
  }
  return active
}

export function normalizeDockLayout(raw: unknown, panels: DockPanelDefinition[]): DockLayoutState {
  const byID = panelByID(panels)
  const fallback = createDockLayout(panels)
  const input = objectOrEmpty(raw) as SerializedDockLayout
  const rawSizes = objectOrEmpty(input.sizes)

  // Unknown panels drop out; a panel listed twice keeps its last position.
  // Stored stacks are authoritative (a hidden session list stays hidden);
  // a pre-tabs layout keeps the old behavior of reopening fixed panels.
  let layout: DockLayoutState = {
    ...fallback,
    ...(input.open ? { open: {}, active: {} } : {}),
    placements: normalizePlacements(objectOrEmpty(input.placements), byID, fallback.placements),
  }
  for (const [zone, ids] of storedStacks(input)) {
    for (const panelID of ids) {
      if (byID.has(panelID)) layout = attachPanel(layout, panelID, zone)
    }
  }

  return {
    ...layout,
    active: restoreActiveTabs(layout, objectOrEmpty(input.active)),
    sizes: {
      left: clampSize('left', rawSizes.left),
      right: clampSize('right', rawSizes.right),
      bottom: clampSize('bottom', rawSizes.bottom),
    },
  }
}

export function serializeDockLayout(layout: DockLayoutState): SerializedDockLayout {
  const open: Record<string, string[]> = {}
  for (const zone of zones) {
    const stack = layout.open[zone]
    if (stack && stack.length > 0) open[zone] = [...stack]
  }
  return {
    placements: { ...layout.placements },
    open,
    active: { ...layout.active },
    sizes: { ...layout.sizes },
  }
}

// Opens a panel as the top tab of its remembered zone. A panel that is
// already open just comes to the front where it is.
export function openDockPanel(
  layout: DockLayoutState,
  panels: DockPanelDefinition[],
  panelID: string,
): DockLayoutState {
  const panel = panelByID(panels).get(panelID)
  if (!panel) return layout
  const openZone = activePanelZone(layout, panelID)
  if (openZone) {
    return { ...layout, active: { ...layout.active, [openZone]: panelID } }
  }
  return attachPanel(layout, panelID, layout.placements[panelID] ?? panel.defaultZone)
}

export function closeDockPanel(layout: DockLayoutState, panelID: string): DockLayoutState {
  return {
    ...layout,
    ...detachPanel(layout, panelID),
  }
}

export function moveDockPanel(
  layout: DockLayoutState,
  panels: DockPanelDefinition[],
  panelID: string,
  zone: DockZone,
): DockLayoutState {
  if (!panelByID(panels).has(panelID)) return layout
  return attachPanel(layout, panelID, zone)
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

// Open means the panel has a tab somewhere, even if another tab covers it.
export function panelIsOpen(layout: DockLayoutState, panelID: string): boolean {
  return activePanelZone(layout, panelID) !== undefined
}

// Visible means the panel is the top tab of its zone.
export function panelIsVisible(layout: DockLayoutState, panelID: string): boolean {
  return zones.some((zone) => layout.active[zone] === panelID)
}

// The zone whose stack holds the panel, whether or not it is on top.
export function activePanelZone(layout: DockLayoutState, panelID: string): DockZone | undefined {
  return zones.find((zone) => layout.open[zone]?.includes(panelID))
}

export function dockZoneTabs(layout: DockLayoutState, zone: DockZone): string[] {
  return layout.open[zone] ?? []
}
