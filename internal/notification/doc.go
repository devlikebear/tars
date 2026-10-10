// Package notification is how the server tells people something happened:
// an Event, a Broker that fans events out to the console's SSE stream, a
// Store that keeps the recent ones with per-role read state, a desktop
// notifier, and the Dispatcher that sends one event to all three. It also
// serves /v1/events/stream, /v1/events/history and /v1/events/read.
//
// It knows nothing about chat, sessions or pipelines; those packages build
// an Event and call Dispatcher.Emit. It was split out of internal/tarsserver
// (#1204), where five other groups of files depended on it.
package notification
