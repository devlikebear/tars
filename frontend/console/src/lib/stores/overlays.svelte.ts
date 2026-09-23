// App-level overlays (#968): the ⌘K command palette and the ? shortcut list.
// App mounts them; anything (a shortcut, the chat rail) can open them.

export class OverlayStore {
  paletteOpen = $state(false)
  helpOpen = $state(false)

  openPalette(): void {
    this.helpOpen = false
    this.paletteOpen = true
  }

  togglePalette(): void {
    if (this.paletteOpen) this.paletteOpen = false
    else this.openPalette()
  }

  openHelp(): void {
    this.paletteOpen = false
    this.helpOpen = true
  }

  closeAll(): void {
    this.paletteOpen = false
    this.helpOpen = false
  }

  get anyOpen(): boolean {
    return this.paletteOpen || this.helpOpen
  }
}

export const overlays = new OverlayStore()
