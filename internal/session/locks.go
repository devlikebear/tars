package session

import "sync"

var pathLocks sync.Map

func lockPath(path string) func() {
	if path == "" {
		return func() {}
	}
	value, _ := pathLocks.LoadOrStore(path, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}
