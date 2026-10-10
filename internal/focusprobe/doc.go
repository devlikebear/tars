// Package focusprobe reads the facts a focus pipeline decides on from outside
// the process: git in the session's folder, `gh pr view`, the diff around a
// review finding, the failing part of a verification log.
//
// Everything here is read-only and knows nothing about chat turns, sessions
// or HTTP. The server (internal/tarsserver) calls it and feeds the results to
// internal/focuspipeline, which decides; nothing in this package asks a model.
// It was split out of internal/tarsserver as the first step of #1204.
package focusprobe
