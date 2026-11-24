// The AI's readiness as the vibe shell shows it (EasyShell.svelte's dot).
// The home screen already reads GET /v1/easy/readiness for its Start
// screen, so there it publishes what it read here and the shell does not
// read it a second time; on every other screen the shell reads it itself.
export type ShellAI = 'ready' | 'notReady' | 'unknown'

export const easyReadiness = $state<{ ai: ShellAI }>({ ai: 'unknown' })
