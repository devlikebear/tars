const STORAGE_KEY = 'tars.console.chatZen'

function readInitial(): boolean {
  if (typeof localStorage === 'undefined') return false
  try {
    return localStorage.getItem(STORAGE_KEY) === 'true'
  } catch {
    return false
  }
}

function persist(value: boolean): void {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(STORAGE_KEY, value ? 'true' : 'false')
  } catch {
    // ignore quota / privacy mode failures
  }
}

class ZenModeStore {
  active = $state(readInitial())

  toggle(): void {
    this.set(!this.active)
  }

  set(value: boolean): void {
    if (this.active === value) return
    this.active = value
    persist(value)
  }
}

// The Mod+. toggle is matched in lib/shortcuts.ts with the other shortcuts.
export const zenMode = new ZenModeStore()
