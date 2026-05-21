// Package tools exposes the reusable tool contracts and safe local tool
// implementations used by TARS agent loops. The package is additive: TARS'
// server-only session, cron, and runtime wiring remain internal, while small
// agent applications can compose registries, file tools, shell tools, web
// tools, and memory tools directly.
package tools
