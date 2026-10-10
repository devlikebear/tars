// Package apihandlers holds the HTTP route groups that stand on their own:
// each one needs a domain package (git, remoteaccess, usage, config) and the
// httpapi helpers, and nothing from the server that mounts it.
//
// One file is one route group, and the groups do not call each other. A
// handler belongs here only while that stays true: one that needs chat turns,
// the session API or the server runtime stays in internal/tarsserver. The
// compiler holds the line, since this package cannot import the server.
//
// The groups were split out of internal/tarsserver (#1204).
package apihandlers
