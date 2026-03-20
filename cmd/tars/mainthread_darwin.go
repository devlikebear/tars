//go:build darwin && cgo

package main

import "golang.design/x/hotkey/mainthread"

func runOnMainThread(fn func()) {
	if fn == nil {
		return
	}
	mainthread.Init(fn)
}
