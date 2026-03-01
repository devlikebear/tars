//go:build darwin && cgo

package assistant

import (
	"context"
	"fmt"

	"golang.design/x/hotkey"
)

type darwinHotkeyListener struct {
	hk *hotkey.Hotkey
}

func newGlobalHotkeyListener(raw string) (hotkeyListener, error) {
	spec, err := parseHotkeySpec(raw)
	if err != nil {
		return nil, err
	}
	mods, key, err := toDarwinHotkey(spec)
	if err != nil {
		return nil, err
	}
	hk := hotkey.New(mods, key)
	if err := hk.Register(); err != nil {
		return nil, fmt.Errorf("register global hotkey failed: %w", err)
	}
	return &darwinHotkeyListener{hk: hk}, nil
}

func (l *darwinHotkeyListener) WaitPress(ctx context.Context) error {
	if l == nil || l.hk == nil {
		return fmt.Errorf("hotkey listener is not initialized")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.hk.Keydown():
		return nil
	}
}

func (l *darwinHotkeyListener) WaitRelease(ctx context.Context) error {
	if l == nil || l.hk == nil {
		return fmt.Errorf("hotkey listener is not initialized")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.hk.Keyup():
		return nil
	}
}

func (l *darwinHotkeyListener) Close() error {
	if l == nil || l.hk == nil {
		return nil
	}
	if err := l.hk.Unregister(); err != nil {
		if err.Error() == "hotkey is not registered" {
			return nil
		}
		return err
	}
	return nil
}

func toDarwinHotkey(spec hotkeySpec) ([]hotkey.Modifier, hotkey.Key, error) {
	mods := make([]hotkey.Modifier, 0, len(spec.Modifiers))
	for _, mod := range spec.Modifiers {
		switch mod {
		case "ctrl":
			mods = append(mods, hotkey.ModCtrl)
		case "option":
			mods = append(mods, hotkey.ModOption)
		case "shift":
			mods = append(mods, hotkey.ModShift)
		case "cmd":
			mods = append(mods, hotkey.ModCmd)
		default:
			return nil, 0, fmt.Errorf("unsupported modifier: %s", mod)
		}
	}
	key, err := toDarwinKey(spec.Key)
	if err != nil {
		return nil, 0, err
	}
	return mods, key, nil
}

func toDarwinKey(key string) (hotkey.Key, error) {
	switch key {
	case "space":
		return hotkey.KeySpace, nil
	case "enter", "return":
		return hotkey.KeyReturn, nil
	case "esc", "escape":
		return hotkey.KeyEscape, nil
	case "tab":
		return hotkey.KeyTab, nil
	case "up":
		return hotkey.KeyUp, nil
	case "down":
		return hotkey.KeyDown, nil
	case "left":
		return hotkey.KeyLeft, nil
	case "right":
		return hotkey.KeyRight, nil
	case "0":
		return hotkey.Key0, nil
	case "1":
		return hotkey.Key1, nil
	case "2":
		return hotkey.Key2, nil
	case "3":
		return hotkey.Key3, nil
	case "4":
		return hotkey.Key4, nil
	case "5":
		return hotkey.Key5, nil
	case "6":
		return hotkey.Key6, nil
	case "7":
		return hotkey.Key7, nil
	case "8":
		return hotkey.Key8, nil
	case "9":
		return hotkey.Key9, nil
	case "a":
		return hotkey.KeyA, nil
	case "b":
		return hotkey.KeyB, nil
	case "c":
		return hotkey.KeyC, nil
	case "d":
		return hotkey.KeyD, nil
	case "e":
		return hotkey.KeyE, nil
	case "f":
		return hotkey.KeyF, nil
	case "g":
		return hotkey.KeyG, nil
	case "h":
		return hotkey.KeyH, nil
	case "i":
		return hotkey.KeyI, nil
	case "j":
		return hotkey.KeyJ, nil
	case "k":
		return hotkey.KeyK, nil
	case "l":
		return hotkey.KeyL, nil
	case "m":
		return hotkey.KeyM, nil
	case "n":
		return hotkey.KeyN, nil
	case "o":
		return hotkey.KeyO, nil
	case "p":
		return hotkey.KeyP, nil
	case "q":
		return hotkey.KeyQ, nil
	case "r":
		return hotkey.KeyR, nil
	case "s":
		return hotkey.KeyS, nil
	case "t":
		return hotkey.KeyT, nil
	case "u":
		return hotkey.KeyU, nil
	case "v":
		return hotkey.KeyV, nil
	case "w":
		return hotkey.KeyW, nil
	case "x":
		return hotkey.KeyX, nil
	case "y":
		return hotkey.KeyY, nil
	case "z":
		return hotkey.KeyZ, nil
	case "f1":
		return hotkey.KeyF1, nil
	case "f2":
		return hotkey.KeyF2, nil
	case "f3":
		return hotkey.KeyF3, nil
	case "f4":
		return hotkey.KeyF4, nil
	case "f5":
		return hotkey.KeyF5, nil
	case "f6":
		return hotkey.KeyF6, nil
	case "f7":
		return hotkey.KeyF7, nil
	case "f8":
		return hotkey.KeyF8, nil
	case "f9":
		return hotkey.KeyF9, nil
	case "f10":
		return hotkey.KeyF10, nil
	case "f11":
		return hotkey.KeyF11, nil
	case "f12":
		return hotkey.KeyF12, nil
	case "f13":
		return hotkey.KeyF13, nil
	case "f14":
		return hotkey.KeyF14, nil
	case "f15":
		return hotkey.KeyF15, nil
	case "f16":
		return hotkey.KeyF16, nil
	case "f17":
		return hotkey.KeyF17, nil
	case "f18":
		return hotkey.KeyF18, nil
	case "f19":
		return hotkey.KeyF19, nil
	case "f20":
		return hotkey.KeyF20, nil
	default:
		return 0, fmt.Errorf("unsupported key: %s", key)
	}
}
