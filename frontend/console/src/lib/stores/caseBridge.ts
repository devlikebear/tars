import * as api from '../api'
import { CaseBridgeStore } from './caseBridgeStore.svelte'

// One instance for the whole console: the panel is mounted once, in
// App.svelte, and CASE's bubble and the shortcut open the same one.
export const caseBridge = new CaseBridgeStore(api)
