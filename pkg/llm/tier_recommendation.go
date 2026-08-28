package llm

import (
	"strings"
	"unicode/utf8"
)

type TierRecommendation struct {
	TaskType        string  `json:"task_type"`
	RecommendedTier Tier    `json:"recommended_tier"`
	Reason          string  `json:"reason"`
	Confidence      float64 `json:"confidence"`
	ShouldPrompt    bool    `json:"should_prompt"`
}

func RecommendTierForTask(message string) TierRecommendation {
	text := strings.ToLower(strings.TrimSpace(message))
	length := utf8.RuneCountInString(text)
	lines := strings.Count(text, "\n") + 1

	if text == "" {
		return TierRecommendation{
			TaskType:        "general",
			RecommendedTier: TierStandard,
			Reason:          "Empty or whitespace-only prompts fall back to the standard tier.",
			Confidence:      0.4,
			ShouldPrompt:    false,
		}
	}

	if hasAny(text, heavySignals...) || length > 900 || lines >= 8 {
		return TierRecommendation{
			TaskType:        "coding",
			RecommendedTier: TierHeavy,
			Reason:          "This looks like coding, repository, release, or deep reasoning work where a stronger tier reduces rework.",
			Confidence:      0.82,
			ShouldPrompt:    true,
		}
	}

	if hasAny(text, lightSignals...) && length <= 320 && lines <= 3 {
		return TierRecommendation{
			TaskType:        "light_transform",
			RecommendedTier: TierLight,
			Reason:          "This looks like a short classification, rewrite, translation, or summary task that should stay cheap.",
			Confidence:      0.78,
			ShouldPrompt:    true,
		}
	}

	return TierRecommendation{
		TaskType:        "general",
		RecommendedTier: TierStandard,
		Reason:          "This looks like open-ended chat or planning that fits the standard tier.",
		Confidence:      0.62,
		ShouldPrompt:    false,
	}
}

func hasAny(text string, signals ...string) bool {
	for _, signal := range signals {
		if strings.Contains(text, signal) {
			return true
		}
	}
	return false
}

var heavySignals = []string{
	"implement",
	"refactor",
	"debug",
	"fix",
	"failing test",
	"test",
	"codebase",
	"architecture",
	"security",
	"threat model",
	"pull request",
	"pr",
	"github issue",
	"release",
	"merge",
	"git",
	"diff",
	"migration",
	"schema",
	"api",
	"구현",
	"개발",
	"리팩터",
	"리팩토",
	"버그",
	"테스트",
	"코드",
	"코드베이스",
	"아키텍처",
	"보안",
	"깃헙",
	"깃허브",
	"이슈",
	"릴리즈",
	"머지",
}

var lightSignals = []string{
	"summarize",
	"summary",
	"translate",
	"rewrite",
	"rephrase",
	"classify",
	"extract",
	"one sentence",
	"short answer",
	"요약",
	"번역",
	"다듬",
	"고쳐",
	"분류",
	"추출",
	"한 문장",
}
