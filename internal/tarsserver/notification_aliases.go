package tarsserver

import "github.com/devlikebear/tars/internal/notification"

// Notifications live in internal/notification (#1204). These names stay for
// the code that is still in this package, the way the httpapi wrappers do;
// code that moves out uses the notification package directly.

type notificationEvent = notification.Event

type eventBroker = notification.Broker

func newNotificationEvent(category, severity, title, message string) notificationEvent {
	return notification.NewEvent(category, severity, title, message)
}

func newEventBroker() *eventBroker {
	return notification.NewBroker()
}
