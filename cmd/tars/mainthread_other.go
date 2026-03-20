//go:build !darwin || !cgo

package main

func runOnMainThread(fn func()) {
	if fn != nil {
		fn()
	}
}
