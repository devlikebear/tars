package assistant

type PushToTalkState struct {
	recording bool
}

func (s *PushToTalkState) HandlePressed() bool {
	if s == nil {
		return false
	}
	if s.recording {
		return false
	}
	s.recording = true
	return true
}

func (s *PushToTalkState) HandleReleased() bool {
	if s == nil {
		return false
	}
	if !s.recording {
		return false
	}
	s.recording = false
	return true
}
