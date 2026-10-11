import * as api from '../api'
import { CompanionAskStore } from './companionAskStore.svelte'

// One instance for the whole console (CompanionPet is mounted once, in
// App.svelte's Shell) — see lib/stores/chatSession.ts for the same
// wiring pattern.
export const companionAsk = new CompanionAskStore(api)
