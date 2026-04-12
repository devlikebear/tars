package gateway

import "sync"

type runEventBroker struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan RunEvent]struct{}
}

func newRunEventBroker() *runEventBroker {
	return &runEventBroker{subscribers: map[string]map[chan RunEvent]struct{}{}}
}

func (b *runEventBroker) Publish(runID string, event RunEvent) {
	if b == nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers[runID] {
		select {
		case ch <- event:
		default:
		}
	}
}

func (b *runEventBroker) Subscribe(runID string) (<-chan RunEvent, func()) {
	ch := make(chan RunEvent, 32)
	if b == nil {
		close(ch)
		return ch, func() {}
	}
	b.mu.Lock()
	if _, ok := b.subscribers[runID]; !ok {
		b.subscribers[runID] = map[chan RunEvent]struct{}{}
	}
	b.subscribers[runID][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if subs, ok := b.subscribers[runID]; ok {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(b.subscribers, runID)
			}
		}
		b.mu.Unlock()
		close(ch)
	}
}
