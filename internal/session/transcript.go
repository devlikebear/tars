package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// AppendMessage appends a single message as one JSON line to the JSONL file at path.
func AppendMessage(path string, msg Message) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open transcript %s: %w", path, err)
	}
	defer f.Close()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}

// RewriteMessages replaces the transcript contents with the provided messages.
func RewriteMessages(path string, messages []Message) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open transcript %s: %w", path, err)
	}
	defer f.Close()

	for _, msg := range messages {
		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("marshal message: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("write message: %w", err)
		}
	}
	return nil
}

// ReadMessages reads all messages from a JSONL file.
// Returns an empty slice if the file does not exist or is empty.
func ReadMessages(path string) ([]Message, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open transcript %s: %w", path, err)
	}
	defer f.Close()

	var messages []Message
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("unmarshal message: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan transcript: %w", err)
	}
	return messages, nil
}

// LoadHistory reads messages from a JSONL file, returning only the most recent
// messages that fit within the given token budget. Tokens are estimated as
// len(content)/4. Messages are returned in chronological order (oldest first).
// Returns an empty slice if the file does not exist.
func LoadHistory(path string, maxTokens int) ([]Message, error) {
	all, err := ReadMessages(path)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, nil
	}

	// Walk backwards from the most recent message, accumulating token cost
	tokens := 0
	startIdx := len(all)
	for i := len(all) - 1; i >= 0; i-- {
		cost := len(all[i].Content) / 4
		if cost < 1 {
			cost = 1
		}
		if tokens+cost > maxTokens {
			break
		}
		tokens += cost
		startIdx = i
	}
	history := all[startIdx:]
	if len(history) == 0 {
		return nil, nil
	}
	if summaryIdx := latestCompactionSummaryIndex(all); summaryIdx >= 0 {
		switch {
		case startIdx > summaryIdx:
			// Always include the latest compaction boundary summary so model context
			// keeps the compacted past when only recent messages fit by token budget.
			history = append([]Message{all[summaryIdx]}, history...)
		case startIdx < summaryIdx:
			history = all[summaryIdx:]
		}
	}
	return history, nil
}

func latestCompactionSummaryIndex(messages []Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "system" {
			continue
		}
		if strings.Contains(messages[i].Content, "[COMPACTION SUMMARY]") {
			return i
		}
	}
	return -1
}
