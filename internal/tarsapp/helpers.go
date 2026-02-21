package tarsapp

// helpers.go — shared utilities and HTTP middleware.
// Domain-specific helpers have been extracted to:
//   - helpers_runtime.go   — runtimeActivity, heartbeat state/runner/policy
//   - helpers_chat.go      — compaction, memory writes, keyword detection
//   - helpers_agent.go     — agent prompt runners, tool registry
//   - helpers_cron.go      — cron runner, delivery, target resolution
//   - helpers_build.go     — builder functions (automation, extensions, gateway)
