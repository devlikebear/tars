// Where the mobile app keeps its desktops and their device tokens: the iOS
// Keychain, through the app's own TarsKeychain plugin
// (mobile/ios/App/App/KeychainPlugin.swift). In a desktop browser — the Vite
// dev server, tests — there is no Keychain, so a development build falls
// back to localStorage; a native build never does.
import { Capacitor, registerPlugin } from '@capacitor/core'

export type SecureStore = {
  get(key: string): Promise<string | null>
  set(key: string, value: string): Promise<void>
  remove(key: string): Promise<void>
}

type KeychainPlugin = {
  get(options: { key: string }): Promise<{ value: string | null }>
  set(options: { key: string; value: string }): Promise<void>
  remove(options: { key: string }): Promise<void>
}

export function keychainStore(plugin: KeychainPlugin): SecureStore {
  return {
    async get(key) {
      return (await plugin.get({ key })).value ?? null
    },
    set: (key, value) => plugin.set({ key, value }),
    remove: (key) => plugin.remove({ key }),
  }
}

export function memoryStore(backing: Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>, prefix = 'tars-mobile:'): SecureStore {
  return {
    async get(key) {
      return backing.getItem(prefix + key)
    },
    async set(key, value) {
      backing.setItem(prefix + key, value)
    },
    async remove(key) {
      backing.removeItem(prefix + key)
    },
  }
}

export function appSecureStore(): SecureStore {
  if (Capacitor.isNativePlatform()) return keychainStore(registerPlugin<KeychainPlugin>('TarsKeychain'))
  if (import.meta.env.DEV) return memoryStore(window.localStorage)
  throw new Error('no secure storage outside the app')
}
