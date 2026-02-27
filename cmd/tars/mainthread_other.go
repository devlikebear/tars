//go:build !darwin

package main

func runOnMainThread(fn func()) {
	if fn != nil {
		fn()
	}
}
