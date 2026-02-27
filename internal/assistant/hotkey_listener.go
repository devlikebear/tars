package assistant

import "context"

type hotkeyListener interface {
	WaitPress(ctx context.Context) error
	WaitRelease(ctx context.Context) error
	Close() error
}

const (
	runtimeModeHotkey   = "hotkey"
	runtimeModeFallback = "fallback"
)

func resolveRuntimeMode(listener hotkeyListener, hotkey string, listenerErr string) (mode string, warning string) {
	_ = hotkey
	if listener != nil {
		return runtimeModeHotkey, ""
	}
	if listenerErr == "" {
		listenerErr = "global hotkey is unavailable"
	}
	return runtimeModeFallback, "global hotkey disabled, fallback to enter mode: " + listenerErr
}
