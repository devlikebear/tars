package tarsserver

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestChatTurnFeedFollowsAndTruncates(t *testing.T) {
	feed := newChatTurnFeed()
	feed.publish([]byte(`{"n":1}`))
	events, next, done, truncated, changed := feed.since(0)
	if len(events) != 1 || next != 1 || done || truncated {
		t.Fatalf("first read: %d %d %v %v", len(events), next, done, truncated)
	}
	go feed.publish([]byte(`{"n":2}`))
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("publish did not wake the follower")
	}
	events, next, _, _, _ = feed.since(next)
	if len(events) != 1 || string(events[0]) != `{"n":2}` || next != 2 {
		t.Fatalf("second read: %q %d", events, next)
	}
	feed.finish()
	feed.finish()
	feed.publish([]byte(`{"n":3}`))
	if _, _, done, _, _ := feed.since(next); !done {
		t.Fatal("finished feed")
	}
	var nilFeed *chatTurnFeed
	nilFeed.publish([]byte("x"))

	big := newChatTurnFeed()
	for i := 0; i < chatTurnFeedMaxEvents+5; i++ {
		big.publish([]byte(`{}`))
	}
	events, next, _, truncated, _ = big.since(0)
	if !truncated || len(events) != chatTurnFeedMaxEvents || next != chatTurnFeedMaxEvents+5 {
		t.Fatalf("truncated read: %d events, next %d, truncated %v", len(events), next, truncated)
	}
	if events, _, _, _, _ := big.since(next + 10); len(events) != 0 {
		t.Fatal("reading past the end")
	}
}

func TestChatTurnFeedsRegistry(t *testing.T) {
	feeds := newChatTurnFeeds()
	if feed, end := feeds.begin(""); feed != nil {
		t.Fatal("no session, no feed")
	} else {
		end()
	}
	first, endFirst := feeds.begin("s")
	second, endSecond := feeds.begin("s")
	if feeds.get("s") != second {
		t.Fatal("the newest turn owns the session's feed")
	}
	endFirst()
	if feeds.get("s") != second {
		t.Fatal("ending an older turn must not drop the newer feed")
	}
	endSecond()
	if feeds.get("s") != nil || first == nil {
		t.Fatal("ended feed still registered")
	}
	var nilFeeds *chatTurnFeeds
	if nilFeeds.get("s") != nil {
		t.Fatal("nil registry")
	}
}

func TestChatTurnStreamEndpoint(t *testing.T) {
	feeds := newChatTurnFeeds()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleChatTurnStream(w, r, feeds)
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?session_id=idle")
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("idle session: %v %v", resp, err)
	}
	resp, _ = http.Get(srv.URL)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing session id = %d", resp.StatusCode)
	}
	resp, _ = http.Post(srv.URL+"?session_id=x", "text/plain", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("post = %d", resp.StatusCode)
	}

	feed, end := feeds.begin("busy")
	feed.publish([]byte(`{"type":"delta","text":"a"}`))
	resp, err = http.Get(srv.URL + "?session_id=busy")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("busy session: %v %v", resp, err)
	}
	defer func() { _ = resp.Body.Close() }()
	lines := make(chan string, 16)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if line := scanner.Text(); strings.HasPrefix(line, "data: ") {
				lines <- strings.TrimPrefix(line, "data: ")
			}
		}
		close(lines)
	}()
	if got := <-lines; got != `{"type":"delta","text":"a"}` {
		t.Fatalf("replayed %q", got)
	}
	feed.publish([]byte(`{"type":"done"}`))
	if got := <-lines; got != `{"type":"done"}` {
		t.Fatalf("followed %q", got)
	}
	end()
	if _, open := <-lines; open {
		t.Fatal("stream should close when the turn ends")
	}
}

// gatedLLMClient answers once release is closed, so a test can drop the
// client while the turn is still running.
type gatedLLMClient struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *gatedLLMClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *gatedLLMClient) Chat(ctx context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.once.Do(func() { close(c.started) })
	select {
	case <-c.release:
	case <-ctx.Done():
		return llm.ChatResponse{}, ctx.Err()
	}
	if opts.OnDelta != nil {
		opts.OnDelta("finished in the background")
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "finished in the background"}}, nil
}

func TestChatTurnOutlivesItsClientAndCanBeReattached(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("background")
	if err != nil {
		t.Fatal(err)
	}
	client := &gatedLLMClient{started: make(chan struct{}), release: make(chan struct{})}
	srv := httptest.NewServer(newChatAPIHandlerWithRuntimeConfig(root, store, client, nil, zerolog.Nop(), 2, nil, "", defaultChatToolingOptions()))
	defer srv.Close()

	ctx, drop := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/v1/chat", strings.NewReader(`{"session_id":"`+sess.ID+`","message":"go"}`))
	req.Header.Set("Content-Type", "application/json")
	go func() {
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-client.started
	drop() // the console goes away mid-turn

	attach, err := http.Get(srv.URL + "/v1/chat/stream?session_id=" + sess.ID)
	if err != nil || attach.StatusCode != http.StatusOK {
		t.Fatalf("attach: %v %v", attach, err)
	}
	defer func() { _ = attach.Body.Close() }()
	close(client.release)
	body := new(strings.Builder)
	scanner := bufio.NewScanner(attach.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		body.WriteString(scanner.Text())
		body.WriteString("\n")
	}
	got := body.String()
	for _, want := range []string{`"stream_open"`, `finished in the background`, `"type":"done"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("attached stream lacks %s:\n%s", want, got)
		}
	}
	messages, err := session.ReadMessages(store.TranscriptPath(sess.ID))
	if err != nil {
		t.Fatal(err)
	}
	if last := messages[len(messages)-1]; last.Role != "assistant" || last.Content != "finished in the background" {
		t.Fatalf("transcript ends with %+v", last)
	}
}

func TestDetachedTurnGivesBackItsChatSlot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(root)
	first, _ := store.Create("left waiting")
	second, _ := store.Create("next")
	client := &gatedLLMClient{started: make(chan struct{}), release: make(chan struct{})}
	tooling := defaultChatToolingOptions()
	tooling.APIMaxInflightChat = 1
	srv := httptest.NewServer(newChatAPIHandlerWithRuntimeConfig(root, store, client, nil, zerolog.Nop(), 2, nil, "", tooling))
	defer srv.Close()
	// Let the held turn finish before the server waits for it to close.
	defer close(client.release)

	post := func(ctx context.Context, id string) (*http.Response, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/v1/chat", strings.NewReader(`{"session_id":"`+id+`","message":"go"}`))
		req.Header.Set("Content-Type", "application/json")
		return http.DefaultClient.Do(req)
	}
	ctx, drop := context.WithCancel(context.Background())
	go func() {
		if resp, err := post(ctx, first.ID); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-client.started
	drop()

	// The first turn still runs, but its console left: the slot is free.
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := post(context.Background(), second.ID)
		if err != nil {
			t.Fatal(err)
		}
		code := resp.StatusCode
		_ = resp.Body.Close()
		if code != http.StatusTooManyRequests {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a detached turn kept its chat slot")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
