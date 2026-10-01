package tarsserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// replyGateClient answers reply once release is closed and records the
// messages it was sent.
type replyGateClient struct {
	reply   string
	started chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	seen    [][]llm.ChatMessage
}

func newReplyGateClient(reply string) *replyGateClient {
	return &replyGateClient{reply: reply, started: make(chan struct{}), release: make(chan struct{})}
}

func (c *replyGateClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *replyGateClient) Chat(ctx context.Context, msgs []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.mu.Lock()
	c.seen = append(c.seen, msgs)
	c.mu.Unlock()
	c.once.Do(func() { close(c.started) })
	select {
	case <-c.release:
	case <-ctx.Done():
		return llm.ChatResponse{}, ctx.Err()
	}
	if opts.OnDelta != nil {
		opts.OnDelta(c.reply)
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: c.reply}}, nil
}

// testChatDeps are the deps newChatAPIHandlerWithRuntimeConfig builds, for
// tests that run turns without the HTTP handler.
func testChatDeps(t *testing.T, client llm.Client) (chatHandlerDeps, *session.Store, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(root)
	return chatHandlerDeps{
		workspaceDir:   root,
		store:          store,
		client:         client,
		logger:         zerolog.Nop(),
		maxIters:       2,
		tooling:        defaultChatToolingOptions(),
		cancelRegistry: newChatCancelRegistry(),
		turnFeeds:      newChatTurnFeeds(),
		chatActivity:   newChatActivity(store, nil),
	}, store, root
}

func TestRunServerChatTurnIsReplayedAndFeedsThePipeline(t *testing.T) {
	client := newReplyGateClient(focusPlanReply)
	deps, store, _ := testChatDeps(t, client)
	sess := focusSession(t, store, "ship it")

	type result struct {
		resp llm.ChatResponse
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := runServerChatTurn(serverauth.WithRoleContext(context.Background(), "user"), deps, sess.ID, "plan it", "card c1")
		done <- result{resp, err}
	}()
	<-client.started
	if !deps.cancelRegistry.Running(sess.ID) {
		t.Fatal("a server turn registers for POST /v1/chat/cancel")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleChatTurnStream(w, r, deps.turnFeeds)
	}))
	defer srv.Close()
	attach, err := http.Get(srv.URL + "/v1/chat/stream?session_id=" + sess.ID)
	if err != nil || attach.StatusCode != http.StatusOK {
		t.Fatalf("attach: %v %v", attach, err)
	}
	defer func() { _ = attach.Body.Close() }()
	close(client.release)
	body := readAll(t, attach)
	open, pipeline, finish := strings.Index(body, `"stream_open"`), strings.Index(body, `"type":"pipeline"`), strings.Index(body, `"type":"done"`)
	if open < 0 || pipeline < open || finish < pipeline {
		t.Fatalf("replay lacks begin…pipeline…done in order:\n%s", body)
	}

	res := <-done
	if res.err != nil || !strings.Contains(res.resp.Message.Content, "<focus-plan>") {
		t.Fatalf("resp = %+v err = %v", res.resp, res.err)
	}
	p, _, _ := focusStoreFor(store).Get(sess.ID)
	if p.OpenGate != focuspipeline.GatePlan || len(p.Cards) != 1 || p.Cards[0].Turn != 1 {
		t.Fatalf("the pipeline must advance from the server turn's block: %+v", p)
	}
	client.mu.Lock()
	last := client.seen[0][len(client.seen[0])-1].Content
	client.mu.Unlock()
	if !strings.Contains(last, "plan it") || !strings.Contains(last, "<console-context>") || !strings.Contains(last, "<focus-stage>") {
		t.Fatalf("server turn message = %q", last)
	}
	messages, _ := session.ReadMessages(store.TranscriptPath(sess.ID))
	if len(messages) != 2 || messages[1].Role != "assistant" {
		t.Fatalf("transcript = %+v", messages)
	}
	if deps.cancelRegistry.Running(sess.ID) {
		t.Fatal("the turn unregisters when done")
	}
}

func TestRunServerChatTurnCancelAndErrors(t *testing.T) {
	client := newReplyGateClient("never")
	deps, store, _ := testChatDeps(t, client)
	sess, _ := store.Create("s")

	done := make(chan error, 1)
	go func() {
		_, err := runServerChatTurn(context.Background(), deps, sess.ID, "go", "")
		done <- err
	}()
	<-client.started
	if !deps.cancelRegistry.Cancel(sess.ID) {
		t.Fatal("cancel found no turn")
	}
	if err := <-done; err == nil {
		t.Fatal("a cancelled server turn returns its error")
	}

	if _, err := runServerChatTurn(context.Background(), deps, "", "go", ""); err == nil {
		t.Fatal("an empty session id must not start a new session")
	}
	if _, err := runServerChatTurn(context.Background(), deps, "missing", "go", ""); err == nil {
		t.Fatal("unknown session")
	}

	// Shutdown: the server context bounds the turn.
	client2 := newReplyGateClient("never")
	deps.client = client2
	ctx, stop := context.WithCancel(context.Background())
	go func() {
		_, err := runServerChatTurn(ctx, deps, sess.ID, "go", "")
		done <- err
	}()
	<-client2.started
	stop()
	if err := <-done; err == nil {
		t.Fatal("server shutdown ends the turn")
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	var b strings.Builder
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			return b.String()
		}
	}
}

// R1: a session claimed by one turn refuses a second, from a console or
// from the server, before anything is prepared or written.
func TestRunChatTurnRefusesASecondTurnOnTheSession(t *testing.T) {
	client := newReplyGateClient("one")
	deps, store, root := testChatDeps(t, client)
	sess, _ := store.Create("s")
	_, release, _ := deps.cancelRegistry.Claim(sess.ID) // a turn in its prepare window
	defer release()

	if _, err := runServerChatTurn(context.Background(), deps, sess.ID, "go", ""); !errors.Is(err, errChatTurnBusy) {
		t.Fatalf("server turn: err = %v", err)
	}
	h := newChatAPIHandlerWithRuntimeConfig(root, store, client, nil, zerolog.Nop(), 2, nil, "", defaultChatToolingOptions())
	// A different registry inside h, so claim through a running turn there.
	done := make(chan struct{})
	go func() {
		defer close(done)
		focusRequest(t, h, http.MethodPost, "/v1/chat", `{"session_id":"`+sess.ID+`","message":"first"}`, false)
	}()
	<-client.started
	rec := focusRequest(t, h, http.MethodPost, "/v1/chat", `{"session_id":"`+sess.ID+`","message":"second"}`, false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second console turn = %d %s", rec.Code, rec.Body.String())
	}
	close(client.release)
	<-done
	messages, _ := session.ReadMessages(store.TranscriptPath(sess.ID))
	for _, m := range messages {
		if strings.Contains(m.Content, "second") {
			t.Fatal("the refused turn wrote to the transcript")
		}
	}
}

// A console sends its queued follow-up on the turn's last event (Stop then
// resume): by the time `cancelled` or `done` is out, the session is free.
func TestTurnReleasesTheSessionBeforeItsLastEvent(t *testing.T) {
	for _, cancel := range []bool{true, false} {
		client := newReplyGateClient("ok")
		deps, store, _ := testChatDeps(t, client)
		sess, _ := store.Create("s")
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = runServerChatTurn(context.Background(), deps, sess.ID, "go", "")
		}()
		<-client.started
		feed := deps.turnFeeds.get(sess.ID)
		if cancel {
			deps.cancelRegistry.Cancel(sess.ID)
		} else {
			close(client.release)
		}
		last := "done"
		if cancel {
			last = "cancelled"
		}
		freeAtLast := false
		for next := 0; ; {
			events, after, finished, _, changed := feed.since(next)
			for _, e := range events {
				if strings.Contains(string(e), `"type":"`+last+`"`) {
					freeAtLast = !deps.cancelRegistry.Running(sess.ID)
				}
			}
			next = after
			if finished {
				break
			}
			<-changed
		}
		<-done
		if !freeAtLast {
			t.Fatalf("cancel=%v: the session was still claimed when %q went out", cancel, last)
		}
	}
}
