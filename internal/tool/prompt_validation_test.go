package tool

import "testing"

func TestValidateNaturalTaskPrompt_AcceptsNaturalSentence(t *testing.T) {
	if err := validateNaturalTaskPrompt("10분마다 디스크 상태를 점검해서 알려줘"); err != nil {
		t.Fatalf("expected natural prompt accepted, got %v", err)
	}
}

func TestValidateNaturalTaskPrompt_RejectsCommandLikePrompt(t *testing.T) {
	if err := validateNaturalTaskPrompt("rm -rf /tmp"); err == nil {
		t.Fatalf("expected command-like prompt rejected")
	}
}

func TestValidateNaturalTaskPrompt_RejectsTooShortPrompt(t *testing.T) {
	if err := validateNaturalTaskPrompt("ok"); err == nil {
		t.Fatalf("expected short prompt rejected")
	}
}
