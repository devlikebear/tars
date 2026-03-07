package tarsserver

import "strings"

func telegramHelpText() string {
	return strings.TrimSpace(`SYSTEM > telegram commands
/help
/sessions
/status
/health
/providers
/models
/model list
/cron {list|runs {job_id} [limit]}
/gateway status
/channels
/new [title]        (per-user scope only)
/resume main        (all scopes)
/resume {id|latest} (per-user scope only)`)
}

func blockedCommandMessage(reason string) string {
	msg := strings.TrimSpace(reason)
	if msg == "" {
		msg = "command is not supported on telegram."
	}
	return "SYSTEM > " + msg
}

func blockInMainSessionMessage() string {
	return blockedCommandMessage("main session mode does not support session switching. use per-user mode.")
}

func splitTelegramMessage(text string, maxLen int) []string {
	body := strings.TrimSpace(text)
	if body == "" {
		return []string{"done."}
	}
	if maxLen <= 0 {
		maxLen = telegramMaxMessageLength
	}
	if len(body) <= maxLen {
		return []string{body}
	}

	lines := strings.SplitAfter(body, "\n")
	chunks := make([]string, 0, len(lines))
	var buf strings.Builder
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		chunks = append(chunks, buf.String())
		buf.Reset()
	}
	for _, line := range lines {
		if len(line) > maxLen {
			flush()
			start := 0
			for start < len(line) {
				end := start + maxLen
				if end > len(line) {
					end = len(line)
				}
				chunks = append(chunks, line[start:end])
				start = end
			}
			continue
		}
		if buf.Len()+len(line) > maxLen {
			flush()
		}
		buf.WriteString(line)
	}
	flush()
	if len(chunks) <= 1 {
		return chunks
	}
	last := chunks[len(chunks)-1]
	prev := chunks[len(chunks)-2]
	if len(last) < maxLen/8 && len(prev)+len(last) <= maxLen {
		chunks[len(chunks)-2] = prev + last
		chunks = chunks[:len(chunks)-1]
	}
	return chunks
}
