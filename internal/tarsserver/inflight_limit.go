package tarsserver

type inflightLimiter struct {
	sem chan struct{}
}

func newInflightLimiter(limit int, fallback int) *inflightLimiter {
	resolved := limit
	if resolved <= 0 {
		resolved = fallback
	}
	if resolved <= 0 {
		resolved = 1
	}
	return &inflightLimiter{
		sem: make(chan struct{}, resolved),
	}
}

func (l *inflightLimiter) tryAcquire() (func(), bool) {
	if l == nil || l.sem == nil {
		return func() {}, true
	}
	select {
	case l.sem <- struct{}{}:
		return func() {
			<-l.sem
		}, true
	default:
		return nil, false
	}
}
