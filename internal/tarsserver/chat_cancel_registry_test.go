package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// postCancel calls the cancel handler directly and decodes its answer.
func postCancel(ctx context.Context, t *testing.T, registry *chatCancelRegistry, sessionID string, wait time.Duration) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/cancel?session_id="+sessionID, nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	handleChatCancel(rec, req, registry, nil, wait)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

// A cancelled turn still winds down (end checkpoint, transcript) while it
// holds the session. Stop's answer waits for that, so a client that sends
// next on the answer — a queued message resumed after Stop — finds the
// session free instead of a 409.
func TestChatCancelAnswersOnceTheTurnLetsGoOfTheSession(t *testing.T) {
	registry := newChatCancelRegistry()
	claim, release, ok := registry.Claim("s1")
	if !ok {
		t.Fatal("claim")
	}
	turnCtx, cancelTurn := context.WithCancel(context.Background())
	claim.setCancel(cancelTurn)
	go func() {
		<-turnCtx.Done()
		time.Sleep(50 * time.Millisecond) // winding down
		release()
	}()

	code, body := postCancel(context.Background(), t, registry, "s1", 5*time.Second)
	if code != http.StatusOK || body["cancelled"] != true || body["ended"] != true {
		t.Fatalf("answer = %d %v", code, body)
	}
	if registry.Running("s1") {
		t.Fatal("the cancel answered while the turn still held the session")
	}
	if _, releaseNext, ok := registry.Claim("s1"); !ok {
		t.Fatal("the next turn could not claim the session after the cancel answered")
	} else {
		releaseNext()
	}
}

// A turn that does not stop in time does not hold the answer forever: the
// cancel is still fired, and ended says the session is not free yet.
func TestChatCancelWaitIsBounded(t *testing.T) {
	registry := newChatCancelRegistry()
	claim, release, _ := registry.Claim("s1")
	defer release()
	turnCtx, cancelTurn := context.WithCancel(context.Background())
	claim.setCancel(cancelTurn)

	code, body := postCancel(context.Background(), t, registry, "s1", 20*time.Millisecond)
	if code != http.StatusOK || body["cancelled"] != true || body["ended"] != false {
		t.Fatalf("answer = %d %v", code, body)
	}
	if turnCtx.Err() == nil {
		t.Fatal("the turn was not cancelled")
	}
}

// A client that gives up on the answer stops the wait too.
func TestChatCancelWaitEndsWithTheRequest(t *testing.T) {
	registry := newChatCancelRegistry()
	claim, release, _ := registry.Claim("s1")
	defer release()
	claim.setCancel(func() {})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		postCancel(ctx, t, registry, "s1", time.Minute)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the cancel kept waiting after its request ended")
	}
}

func TestChatCancelWithNothingRunning(t *testing.T) {
	code, _ := postCancel(context.Background(), t, newChatCancelRegistry(), "s1", time.Second)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d", code)
	}
}
