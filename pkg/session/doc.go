// Package session exposes tars' file-backed chat session and transcript
// persistence for external agent applications. It is a thin alias layer over
// internal/session; the on-disk format (sessions/sessions.json index plus one
// sessions/{id}.jsonl transcript per session) is unchanged. Construct a Store
// with NewStore(dir); obtain a session's transcript path via
// (*Store).TranscriptPath(id); then append/read/load messages with the
// transcript helpers.
package session
