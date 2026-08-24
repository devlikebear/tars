// Console API client split by domain (#931).
//
// Each module owns one server domain; this barrel re-exports the full public
// surface so `import { … } from '../lib/api'` keeps working everywhere.

export { APIRequestError } from './client.ts'

export * from './system.ts'
export * from './config.ts'
export * from './chat.ts'
export * from './sessions.ts'
export * from './tasks.ts'
export * from './git.ts'
export * from './memory.ts'
export * from './ops.ts'
export * from './extensions.ts'
export * from './channels.ts'
export * from './agentruntime.ts'
export * from './workspace.ts'
