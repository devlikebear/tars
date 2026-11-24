// consoleMode holds whether expert mode is on (console_expert_mode from
// /v1/healthz, read by App.svelte before the first screen is drawn). Vibe
// mode's screens read it to leave out every link into the workbench and
// focus mode when it is off (vibe-coder-identity.md, 2026-11-24).
export const consoleMode = $state({ expert: false })
