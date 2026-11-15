package focuspipeline

import (
	"path/filepath"
	"sync"
)

// Who wants to know that a pipeline changed. Stores are made per request,
// so the watchers belong to the sessions folder, like the folder lock: every
// Store over that folder tells them about each pipeline it saves or
// deletes. The phones' focus event stream (docs/decisions/mobile-remote.md)
// listens here instead of reading every pipeline file on a timer.

type changeWatchers struct {
	mu   sync.Mutex
	next int
	fns  map[int]func(sessionID string)
}

var dirWatchers sync.Map // clean dir → *changeWatchers

// WatchChanges calls fn with the session id of each pipeline saved or
// deleted under sessionsDir, through any Store over that folder, until the
// returned func is called. fn may run with the folder's lock held, so it
// must return at once — signal, never read the store from it.
func WatchChanges(sessionsDir string, fn func(sessionID string)) func() {
	if fn == nil {
		return func() {}
	}
	value, _ := dirWatchers.LoadOrStore(filepath.Clean(sessionsDir), &changeWatchers{fns: map[int]func(string){}})
	w := value.(*changeWatchers)
	w.mu.Lock()
	id := w.next
	w.next++
	w.fns[id] = fn
	w.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			w.mu.Lock()
			delete(w.fns, id)
			w.mu.Unlock()
		})
	}
}

// changed tells the folder's watchers that sessionID's pipeline changed.
func (s *Store) changed(sessionID string) {
	value, ok := dirWatchers.Load(filepath.Clean(s.dir))
	if !ok {
		return
	}
	w := value.(*changeWatchers)
	w.mu.Lock()
	fns := make([]func(string), 0, len(w.fns))
	for _, fn := range w.fns {
		fns = append(fns, fn)
	}
	w.mu.Unlock()
	for _, fn := range fns {
		fn(sessionID)
	}
}
