// The mobile app's desktops as runes state: load them from secure storage,
// pair with a new one (a scanned tars-mobile:// link fills the form), forget one, and check
// which answer. Storage and the network come in through the constructor so
// tests run it under Node.
import { parsePairingLink, type PairingTarget } from '../devicePairing.ts'
import { APIRequestError } from '../api/client.ts'
import type { DesktopAPI } from './desktopApi.ts'
import {
  addressToBaseURL,
  desktopName,
  parseDesktops,
  targetOf,
  withDesktop,
  withoutDesktop,
  type DesktopStatus,
  type PairedDesktop,
} from './desktops.ts'
import type { SecureStore } from './secureStore.ts'

const storageKey = 'desktops'

export type PairFailure = 'invalid_address' | 'invalid_code' | 'locked' | 'unreachable' | 'failed'

export type DesktopStoreDeps = {
  storage: SecureStore
  api: DesktopAPI
  now?: () => Date
  allowLoopbackHTTP?: boolean
}

export class DesktopStore {
  desktops = $state<PairedDesktop[]>([])
  statuses = $state<Record<string, DesktopStatus>>({})
  loaded = $state(false)
  pairing = $state(false)
  pairFailure = $state<PairFailure | null>(null)

  #deps: DesktopStoreDeps

  constructor(deps: DesktopStoreDeps) {
    this.#deps = deps
  }

  async load(): Promise<void> {
    this.desktops = parseDesktops(await this.#deps.storage.get(storageKey))
    this.loaded = true
  }

  // readLink reads a scanned tars-mobile://pair link into the pairing form;
  // null when it is not one.
  readLink(link: string): PairingTarget | null {
    return parsePairingLink(link, { allowLoopbackHTTP: this.#deps.allowLoopbackHTTP })
  }

  // pair trades code at the desktop at address. A desktop already in the
  // list is replaced: pairing again is how a phone recovers after the desk
  // revoked it.
  async pair(address: string, code: string, phoneName: string): Promise<PairedDesktop | null> {
    this.pairFailure = null
    const baseURL = addressToBaseURL(address, { allowLoopbackHTTP: this.#deps.allowLoopbackHTTP })
    if (!baseURL) {
      this.pairFailure = 'invalid_address'
      return null
    }
    if (!code.trim()) {
      this.pairFailure = 'invalid_code'
      return null
    }
    const target: PairingTarget = { baseURL, code: code.trim() }
    this.pairing = true
    try {
      const answer = await this.#deps.api.pair(target, phoneName.trim() || 'iPhone')
      const desktop: PairedDesktop = {
        id: target.baseURL,
        name: desktopName(target.baseURL),
        baseURL: target.baseURL,
        token: answer.token,
        deviceID: answer.device_id,
        pairedAt: (this.#deps.now?.() ?? new Date()).toISOString(),
      }
      const next = withDesktop(this.desktops, desktop)
      await this.#save(next)
      this.desktops = next
      this.statuses = { ...this.statuses, [desktop.id]: { state: 'online' } }
      return desktop
    } catch (err) {
      this.pairFailure = pairFailureOf(err)
      return null
    } finally {
      this.pairing = false
    }
  }

  // forget removes a desktop from this phone only. The desk still lists the
  // phone until someone revokes it there.
  async forget(id: string): Promise<void> {
    const next = withoutDesktop(this.desktops, id)
    await this.#save(next)
    this.desktops = next
    this.statuses = Object.fromEntries(Object.entries(this.statuses).filter(([key]) => key !== id))
  }

  // refresh checks every desktop at once; one that sleeps does not hold up
  // the others.
  async refresh(): Promise<void> {
    const list = this.desktops
    this.statuses = Object.fromEntries(list.map((d) => [d.id, { ...this.statuses[d.id], state: 'checking' as const }]))
    await Promise.all(
      list.map(async (d) => {
        let status: DesktopStatus
        try {
          status = await this.#deps.api.status(targetOf(d))
        } catch {
          status = { state: 'unreachable' }
        }
        this.statuses = { ...this.statuses, [d.id]: status }
      }),
    )
  }

  async #save(list: PairedDesktop[]): Promise<void> {
    await this.#deps.storage.set(storageKey, JSON.stringify(list))
  }
}

function pairFailureOf(err: unknown): PairFailure {
  if (!(err instanceof APIRequestError)) return 'unreachable'
  if (err.status === 401) return 'invalid_code'
  if (err.status === 429) return 'locked'
  return 'failed'
}
