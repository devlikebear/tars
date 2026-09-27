import * as api from '../api'
import { ChangesStore } from './changes.svelte'

// The one Changes store the chat workbench shares.
export const changes = new ChangesStore(api)
