package assistant

import "testing"

func TestPushToTalkStateTransitions(t *testing.T) {
	var state PushToTalkState

	if !state.HandlePressed() {
		t.Fatalf("expected first pressed event to start recording")
	}
	if !state.Recording() {
		t.Fatalf("expected recording=true after pressed")
	}
	if state.HandlePressed() {
		t.Fatalf("expected duplicate pressed event to be ignored")
	}
	if !state.HandleReleased() {
		t.Fatalf("expected released event to stop recording")
	}
	if state.Recording() {
		t.Fatalf("expected recording=false after released")
	}
	if state.HandleReleased() {
		t.Fatalf("expected duplicate released event to be ignored")
	}
}
