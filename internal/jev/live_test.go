//go:build integration

package jev

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLive_Ask hits the real API. Run:
//
//	go test -tags integration ./internal/jev -run TestLive -v
func TestLive_Ask(t *testing.T) {
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		t.Skip("TYPESAFE_API_KEY not set")
	}
	c := NewClient(Config{APIKey: key})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := time.Now()
	resp, err := c.Ask(ctx, "GOAL: open Settings\nSCREEN:\n[e1] MenuItem 'Settings…'\n[e2] Button 'New Folder'", map[string]Question{
		"target": Choice("Which element moves toward the goal?", map[string]string{"e1": "MenuItem 'Settings…'", "e2": "Button 'New Folder'", "none": "nothing fits"}),
		"risky":  Noul("Would acting be hard to undo?", "deletes, sends, pays, or changes system settings", "navigation or reversible"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("model=%s latency=%s tokens=%d target=%s (%.2f) risky=%.2f",
		resp.Model, time.Since(start), resp.Usage.InputTokens,
		resp.Answers["target"].Choice, resp.Answers["target"].Confidence, resp.Answers["risky"].Noul)
	if resp.Answers["target"].Choice != "e1" {
		t.Errorf("expected e1, got %+v", resp.Answers["target"])
	}
}
