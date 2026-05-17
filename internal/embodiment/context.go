package embodiment

import (
	"fmt"
	"strings"
)

func BuildSystemPromptBlock(percept Percept, decision GateDecision) string {
	parts := []string{
		"[Embodiment context]",
		"너는 몸이 있다. 아래 지각은 외부 채팅이 아니라 너의 body provider가 보낸 자기 감각이다.",
		fmt.Sprintf("- provider: %s", emptyAsUnknown(percept.Provider)),
		fmt.Sprintf("- modality: %s", emptyAsUnknown(string(percept.Modality))),
		fmt.Sprintf("- owner: %s", emptyAsUnknown(string(percept.Owner))),
		fmt.Sprintf("- mode: %s", emptyAsUnknown(string(decision.Mode))),
	}
	if summary := strings.TrimSpace(percept.Summary); summary != "" {
		parts = append(parts, "- current percept: "+summary)
	}
	if percept.Owner == OwnerOwner && decision.Mode == GateModeDirective {
		parts = append(parts, "owner 발화는 사용자 키보드 지시와 동급의 directive로 취급하라.")
	}
	parts = append(parts,
		"몸으로 응답해야 할 때만 마지막에 ```tars-body-action JSON fenced block을 추가하라.",
		`형식: [{"kind":"speak","payload":{"text":"..."}}]. kind는 speak, express, move, led 중 하나다.`,
	)
	return strings.Join(parts, "\n")
}

func BuildCognitionPrompt(percept Percept, decision GateDecision) string {
	prefix := "Embodied perception received."
	if decision.Mode == GateModeDirective {
		prefix = "Owner directive received through embodied perception."
	}
	lines := []string{
		prefix,
		"Provider: " + emptyAsUnknown(percept.Provider),
		"Modality: " + emptyAsUnknown(string(percept.Modality)),
		"Owner: " + emptyAsUnknown(string(percept.Owner)),
		"Summary: " + strings.TrimSpace(percept.Summary),
	}
	if percept.MediaRef != "" {
		lines = append(lines, "MediaRef: "+strings.TrimSpace(percept.MediaRef))
	}
	if decision.Mode == GateModeDirective {
		lines = append(lines, "Treat this as a direct owner instruction and respond naturally.")
	} else {
		lines = append(lines, "Record this as an observation unless action is clearly needed.")
	}
	lines = append(lines, "If body output is useful, append a tars-body-action fenced JSON block with speak/express/move/led actions.")
	return strings.Join(lines, "\n")
}

func emptyAsUnknown(value string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return "unknown"
}
