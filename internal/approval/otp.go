package approval

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type OTPManager struct {
	mu      sync.Mutex
	nowFn   func() time.Time
	pending map[string]*pendingOTP
}

type pendingOTP struct {
	expiresAt time.Time
	result    chan string
}

func NewOTPManager(nowFn func() time.Time) *OTPManager {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &OTPManager{
		nowFn:   nowFn,
		pending: map[string]*pendingOTP{},
	}
}

func (m *OTPManager) Request(ctx context.Context, chatID string, timeout time.Duration) (string, error) {
	if m == nil {
		return "", fmt.Errorf("otp manager is nil")
	}
	target := strings.TrimSpace(chatID)
	if target == "" {
		return "", fmt.Errorf("chat id is required")
	}
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	deadline := m.nowFn().UTC().Add(timeout)
	pending := &pendingOTP{
		expiresAt: deadline,
		result:    make(chan string, 1),
	}

	m.mu.Lock()
	m.gcLocked()
	m.pending[target] = pending
	m.mu.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		m.mu.Lock()
		if existing, ok := m.pending[target]; ok && existing == pending {
			delete(m.pending, target)
		}
		m.mu.Unlock()
		return "", ctx.Err()
	case <-timer.C:
		m.mu.Lock()
		if existing, ok := m.pending[target]; ok && existing == pending {
			delete(m.pending, target)
		}
		m.mu.Unlock()
		return "", fmt.Errorf("otp timed out")
	case code := <-pending.result:
		return strings.TrimSpace(code), nil
	}
}

func (m *OTPManager) Consume(chatID string, text string) bool {
	if m == nil {
		return false
	}
	target := strings.TrimSpace(chatID)
	code := strings.TrimSpace(text)
	if target == "" || code == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.gcLocked()
	pending, ok := m.pending[target]
	if !ok {
		return false
	}
	delete(m.pending, target)
	select {
	case pending.result <- code:
	default:
	}
	close(pending.result)
	return true
}

func (m *OTPManager) HasPending(chatID string) bool {
	if m == nil {
		return false
	}
	target := strings.TrimSpace(chatID)
	if target == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gcLocked()
	_, ok := m.pending[target]
	return ok
}

func (m *OTPManager) gcLocked() {
	now := m.nowFn().UTC()
	for chatID, item := range m.pending {
		if item == nil {
			delete(m.pending, chatID)
			continue
		}
		if !item.expiresAt.IsZero() && now.After(item.expiresAt) {
			delete(m.pending, chatID)
		}
	}
}
