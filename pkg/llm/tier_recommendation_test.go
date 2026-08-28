package llm

import "testing"

func TestRecommendTierForTaskClassifiesCommonWork(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		wantTier Tier
		wantType string
	}{
		{
			name:     "code and release work wants heavy",
			message:  "Implement the next GitHub issue, update tests, open a PR, and verify the release.",
			wantTier: TierHeavy,
			wantType: "coding",
		},
		{
			name:     "short rewrite wants light",
			message:  "Summarize this paragraph in one sentence.",
			wantTier: TierLight,
			wantType: "light_transform",
		},
		{
			name:     "open ended product discussion stays standard",
			message:  "Let's brainstorm a few UX ideas for the workspace dashboard.",
			wantTier: TierStandard,
			wantType: "general",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := RecommendTierForTask(tt.message)
			if rec.RecommendedTier != tt.wantTier {
				t.Fatalf("tier = %q, want %q (rec=%+v)", rec.RecommendedTier, tt.wantTier, rec)
			}
			if rec.TaskType != tt.wantType {
				t.Fatalf("task type = %q, want %q (rec=%+v)", rec.TaskType, tt.wantType, rec)
			}
			if rec.Confidence <= 0 || rec.Reason == "" {
				t.Fatalf("recommendation should include confidence and reason: %+v", rec)
			}
		})
	}
}
