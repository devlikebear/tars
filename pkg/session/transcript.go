package session

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AppendMessage appends a single message as one JSON line to the JSONL file at path.
func AppendMessage(path string, msg Message) error {
	unlock := lockPath(path)
	defer unlock()
	var err error
	msg, err = ensurePersistedMessageID(msg)
	if err != nil {
		return err
	}
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
	unlock := lockPath(path)
	defer unlock()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open transcript %s: %w", path, err)
	}
	defer f.Close()

	for _, msg := range messages {
		var err error
		msg, err = ensurePersistedMessageID(msg)
		if err != nil {
			return err
		}
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
	unlock := lockPath(path)
	defer unlock()
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
		if strings.TrimSpace(msg.ID) == "" {
			msg.ID = virtualMessageID(path, len(messages), msg)
		} else {
			msg.ID = strings.TrimSpace(msg.ID)
		}
		messages = append(messages, msg)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan transcript: %w", err)
	}
	return messages, nil
}

func ensurePersistedMessageID(msg Message) (Message, error) {
	msg.ID = strings.TrimSpace(msg.ID)
	if msg.ID != "" {
		return msg, nil
	}
	id, err := newUUIDv7(msg.Timestamp)
	if err != nil {
		return Message{}, fmt.Errorf("generate message id: %w", err)
	}
	msg.ID = id
	return msg, nil
}

func newUUIDv7(at time.Time) (string, error) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	ms := uint64(at.UTC().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func virtualMessageID(path string, index int, msg Message) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(
		h,
		"%s\x00%d\x00%s\x00%s\x00%s\x00%s\x00%s\x00%t",
		filepath.ToSlash(filepath.Clean(path)),
		index,
		strings.TrimSpace(msg.Role),
		strings.TrimSpace(msg.Content),
		msg.Timestamp.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(msg.ToolName),
		strings.TrimSpace(msg.ToolCallID),
		msg.ToolIsError,
	)
	sum := h.Sum(nil)
	return "legacy_" + hex.EncodeToString(sum)[:24]
}

// HistorySnapshot captures the portion of transcript loaded into model context.
type HistorySnapshot struct {
	Messages       []Message
	Tokens         int
	CompactionUsed bool
}

// LoadHistory reads messages from a JSONL file, returning only the most recent
// messages that fit within the given token budget. Tokens are estimated as
// len(content)/4. Messages are returned in chronological order (oldest first).
// Returns an empty slice if the file does not exist.
func LoadHistory(path string, maxTokens int) ([]Message, error) {
	snapshot, err := LoadHistorySnapshot(path, maxTokens)
	if err != nil {
		return nil, err
	}
	return snapshot.Messages, nil
}

// LoadHistorySnapshot reads transcript history and returns the loaded messages
// together with token and compaction-boundary metadata.
func LoadHistorySnapshot(path string, maxTokens int) (HistorySnapshot, error) {
	all, err := ReadMessages(path)
	if err != nil {
		return HistorySnapshot{}, err
	}
	if len(all) == 0 {
		return HistorySnapshot{}, nil
	}

	// Walk backwards from the most recent message, accumulating token cost
	tokens := 0
	startIdx := len(all)
	for i := len(all) - 1; i >= 0; i-- {
		cost := messageTokenCost(all[i])
		if tokens+cost > maxTokens {
			break
		}
		tokens += cost
		startIdx = i
	}
	history := all[startIdx:]
	if len(history) == 0 {
		return HistorySnapshot{}, nil
	}
	compactionUsed := false
	if summaryIdx := latestCompactionSummaryIndex(all); summaryIdx >= 0 {
		switch {
		case startIdx > summaryIdx:
			// Always include the latest compaction boundary summary so model context
			// keeps the compacted past when only recent messages fit by token budget.
			prefix := []Message{all[summaryIdx]}
			for i := summaryIdx + 1; i < startIdx && i < len(all); i++ {
				if !IsTasksInjectionMessage(all[i]) {
					break
				}
				prefix = append(prefix, all[i])
			}
			history = append(prefix, history...)
			compactionUsed = true
		case startIdx < summaryIdx:
			history = all[summaryIdx:]
			compactionUsed = true
		case startIdx == summaryIdx:
			compactionUsed = true
		}
	}
	return HistorySnapshot{
		Messages:       history,
		Tokens:         historyTokenCost(history),
		CompactionUsed: compactionUsed,
	}, nil
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

func historyTokenCost(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += messageTokenCost(msg)
	}
	return total
}

func messageTokenCost(msg Message) int {
	return estimateMessageTokenCost(msg)
}
