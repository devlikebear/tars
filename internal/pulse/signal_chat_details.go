package pulse

type chatSignalCandidate interface {
	chatSignalDetails() chatSignalCandidateDetails
	canAutoResumeCandidate() bool
}

type chatSignalCandidateDetails struct {
	sessionID         string
	title             string
	lastMessageID     string
	ageMinutes        int
	canAutoResume     bool
	autoResumeEnabled bool
	blockReason       string
}

func (candidate StalledChatCandidate) chatSignalDetails() chatSignalCandidateDetails {
	return chatSignalCandidateDetails{
		sessionID:         candidate.SessionID,
		title:             candidate.Title,
		lastMessageID:     candidate.LastMessageID,
		ageMinutes:        candidate.AgeMinutes,
		canAutoResume:     candidate.CanAutoResume,
		autoResumeEnabled: candidate.AutoResumeEnabled,
		blockReason:       candidate.BlockReason,
	}
}

func (candidate StalledChatCandidate) canAutoResumeCandidate() bool {
	return candidate.CanAutoResume
}

func (candidate FailedChatCandidate) chatSignalDetails() chatSignalCandidateDetails {
	return chatSignalCandidateDetails{
		sessionID:         candidate.SessionID,
		title:             candidate.Title,
		lastMessageID:     candidate.LastMessageID,
		ageMinutes:        candidate.AgeMinutes,
		canAutoResume:     candidate.CanAutoResume,
		autoResumeEnabled: candidate.AutoResumeEnabled,
		blockReason:       candidate.BlockReason,
	}
}

func (candidate FailedChatCandidate) canAutoResumeCandidate() bool {
	return candidate.CanAutoResume
}

func newChatSignalDetails[T chatSignalCandidate](
	countKey string,
	candidates []T,
	autofixCandidate string,
	extra map[string]any,
) map[string]any {
	if len(candidates) == 0 {
		return nil
	}
	primary := candidates[0].chatSignalDetails()
	details := map[string]any{
		countKey:              len(candidates),
		"session_id":          primary.sessionID,
		"session_title":       primary.title,
		"last_message_id":     primary.lastMessageID,
		"age_minutes":         primary.ageMinutes,
		"can_auto_resume":     primary.canAutoResume,
		"auto_resume_enabled": primary.autoResumeEnabled,
		"block_reason":        primary.blockReason,
		"autofix_candidate":   autofixCandidate,
		"sessions":            candidates,
	}
	for _, candidate := range candidates {
		if candidate.canAutoResumeCandidate() {
			details["has_auto_resume_candidate"] = true
			break
		}
	}
	for key, value := range extra {
		details[key] = value
	}
	return details
}
