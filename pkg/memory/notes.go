package memory

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var memoryNoteLine = regexp.MustCompile(`^- ([0-9]{4}-[0-9]{2}-[0-9]{2}T[^ ]+) (.*)$`)

type MemoryNote struct {
	Timestamp time.Time `json:"timestamp"`
	Text      string    `json:"text"`
}

func ListMemoryNotesByPrefix(root string, prefix string, limit int) ([]MemoryNote, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return []MemoryNote{}, nil
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	path := filepath.Join(root, "MEMORY.md")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []MemoryNote{}, nil
		}
		return nil, fmt.Errorf("open memory notes: %w", err)
	}
	defer file.Close()

	var notes []MemoryNote
	var current *MemoryNote
	flush := func() {
		if current == nil {
			return
		}
		current.Text = strings.TrimSpace(current.Text)
		if strings.HasPrefix(current.Text, prefix) {
			notes = append(notes, *current)
		}
		current = nil
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		matches := memoryNoteLine.FindStringSubmatch(line)
		if len(matches) == 3 {
			flush()
			timestamp, err := time.Parse(time.RFC3339, matches[1])
			if err != nil {
				continue
			}
			current = &MemoryNote{
				Timestamp: timestamp.UTC(),
				Text:      strings.TrimSpace(matches[2]),
			}
			continue
		}
		if current != nil {
			current.Text += "\n" + line
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan memory notes: %w", err)
	}

	sort.Slice(notes, func(i, j int) bool {
		return notes[i].Timestamp.After(notes[j].Timestamp)
	})
	if len(notes) > limit {
		notes = notes[:limit]
	}
	if notes == nil {
		return []MemoryNote{}, nil
	}
	return notes, nil
}
