package focuspipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

// The harness library (harness.go): per repository, under
// <workspace>/_shared/harness-library/<repo>/,
//
//	lessons.json   every lesson proposed for the repository: the edit, the
//	               evidence it cited, the pipeline it came from, the
//	               proposal pipeline, and — once that pull request merged —
//	               adopted with its URL
//	EXPERIENCE.md  the adopted lessons for people to read: situation,
//	               lesson, cited evidence, merged pull request
//
// A lesson proposed once is never proposed again, adopted or not: a
// proposal nobody merged is a person's answer too.

// HarnessLibraryDirName is the library's folder under <workspace>/_shared.
const HarnessLibraryDirName = "harness-library"

// Harness lesson statuses (HarnessLessonRecord.Status).
const (
	HarnessLessonProposed = "proposed"
	HarnessLessonAdopted  = "adopted"
)

const (
	harnessLessonsFile    = "lessons.json"
	harnessExperienceFile = "EXPERIENCE.md"
)

// ErrInvalidHarnessRepo is a repository path the library cannot key.
var ErrInvalidHarnessRepo = errors.New("invalid harness library repository")

// harnessLibraryLocks serializes read-modify-write cycles per library folder.
var harnessLibraryLocks sync.Map // dir → *sync.Mutex

// HarnessLibrary is one repository's harness library.
type HarnessLibrary struct {
	dir string
}

// HarnessLessonRecord is one lesson in the library.
type HarnessLessonRecord struct {
	ID      string `json:"id"`
	Lesson  string `json:"lesson"`
	Path    string `json:"path"`
	Section string `json:"section,omitempty"`
	Kind    string `json:"kind"`
	Content string `json:"content"`
	// Evidence are the cited items as they were recorded.
	Evidence      []EvidenceItem `json:"evidence"`
	SourceSession string         `json:"source_session"`
	SourceGoal    string         `json:"source_goal,omitempty"`
	SourcePR      string         `json:"source_pr,omitempty"`
	// ProposalSession is the proposal pipeline that carries the edit.
	ProposalSession string     `json:"proposal_session"`
	Status          string     `json:"status"`
	ProposedAt      time.Time  `json:"proposed_at"`
	AdoptedAt       *time.Time `json:"adopted_at,omitempty"`
	// PRURL is the merged proposal pull request (adopted only).
	PRURL string `json:"pr_url,omitempty"`
}

var harnessKeyUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// HarnessLibraryKey names a repository's library folder: its folder name
// made path-safe, and a short hash of its full path so two checkouts with
// the same name stay apart.
func HarnessLibraryKey(repo string) string {
	clean := filepath.Clean(repo)
	sum := sha256.Sum256([]byte(filepath.ToSlash(clean)))
	name := strings.Trim(harnessKeyUnsafe.ReplaceAllString(filepath.Base(clean), "-"), ".-")
	if name == "" {
		name = "repo"
	}
	if len(name) > 48 {
		name = name[:48]
	}
	return name + "-" + hex.EncodeToString(sum[:4])
}

// OpenHarnessLibrary is the library of repo (its main checkout, an absolute
// path) under sharedDir (<workspace>/_shared). Nothing is written until a
// lesson is recorded.
func OpenHarnessLibrary(sharedDir, repo string) (HarnessLibrary, error) {
	repo = strings.TrimSpace(repo)
	if strings.TrimSpace(sharedDir) == "" || repo == "" || !filepath.IsAbs(repo) {
		return HarnessLibrary{}, ErrInvalidHarnessRepo
	}
	return HarnessLibrary{dir: filepath.Join(sharedDir, HarnessLibraryDirName, HarnessLibraryKey(repo))}, nil
}

// Dir is the library's folder.
func (l HarnessLibrary) Dir() string { return l.dir }

func (l HarnessLibrary) lock() func() {
	mu, _ := harnessLibraryLocks.LoadOrStore(filepath.Clean(l.dir), &sync.Mutex{})
	m := mu.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}

// Lessons reads every lesson the library holds, oldest first; none when the
// library was never written.
func (l HarnessLibrary) Lessons() ([]HarnessLessonRecord, error) {
	unlock := l.lock()
	defer unlock()
	return l.read()
}

func (l HarnessLibrary) read() ([]HarnessLessonRecord, error) {
	raw, err := os.ReadFile(filepath.Join(l.dir, harnessLessonsFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []HarnessLessonRecord
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("read harness library: %w", err)
	}
	return out, nil
}

func (l HarnessLibrary) write(lessons []HarnessLessonRecord) error {
	raw, err := json.MarshalIndent(lessons, "", "  ")
	if err != nil {
		return err
	}
	return atomicwrite.Write(filepath.Join(l.dir, harnessLessonsFile), append(raw, '\n'))
}

// Known is the library as a proposal sees it: the HarnessLessonKey of every
// lesson (for ValidateHarnessLessons) and one line per lesson (for
// HarnessLessonPrompt).
func (l HarnessLibrary) Known() (map[string]bool, []string, error) {
	lessons, err := l.Lessons()
	if err != nil {
		return nil, nil, err
	}
	keys := make(map[string]bool, len(lessons))
	lines := make([]string, 0, len(lessons))
	for _, r := range lessons {
		keys[HarnessLessonKey(r.Lesson)] = true
		lines = append(lines, fmt.Sprintf("[%s] %s (%s)", r.Status, r.Lesson, r.Path))
	}
	return keys, lines, nil
}

// RecordProposed adds the edits a proposal pipeline carries as proposed
// lessons and returns their ids.
func (l HarnessLibrary) RecordProposed(ev HarnessEvidence, proposalSession string, edits []HarnessEdit, now time.Time) ([]string, error) {
	unlock := l.lock()
	defer unlock()
	lessons, err := l.read()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(edits))
	for _, e := range edits {
		id := fmt.Sprintf("L%d", len(lessons)+1)
		var cited []EvidenceItem
		for _, ref := range e.Evidence {
			if it, ok := ev.Item(ref); ok {
				cited = append(cited, it)
			}
		}
		lessons = append(lessons, HarnessLessonRecord{
			ID: id, Lesson: e.Lesson, Path: e.Path, Section: e.Section, Kind: e.Kind, Content: e.Content,
			Evidence: cited, SourceSession: ev.SessionID, SourceGoal: ev.Goal, SourcePR: ev.PRURL,
			ProposalSession: proposalSession, Status: HarnessLessonProposed, ProposedAt: now.UTC(),
		})
		ids = append(ids, id)
	}
	if err := l.write(lessons); err != nil {
		return nil, err
	}
	return ids, nil
}

// Adopt marks the proposed lessons of a proposal pipeline adopted with the
// merged pull request, and appends them to EXPERIENCE.md. It returns how
// many it adopted; lessons already adopted are left alone.
func (l HarnessLibrary) Adopt(proposalSession, prURL string, now time.Time) (int, error) {
	unlock := l.lock()
	defer unlock()
	lessons, err := l.read()
	if err != nil {
		return 0, err
	}
	at := now.UTC()
	var adopted []HarnessLessonRecord
	for i := range lessons {
		r := &lessons[i]
		if r.ProposalSession != proposalSession || r.Status != HarnessLessonProposed {
			continue
		}
		r.Status, r.AdoptedAt, r.PRURL = HarnessLessonAdopted, &at, strings.TrimSpace(prURL)
		adopted = append(adopted, *r)
	}
	if len(adopted) == 0 {
		return 0, nil
	}
	if err := l.write(lessons); err != nil {
		return 0, err
	}
	return len(adopted), l.appendExperience(adopted)
}

func (l HarnessLibrary) appendExperience(adopted []HarnessLessonRecord) error {
	path := filepath.Join(l.dir, harnessExperienceFile)
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var b strings.Builder
	if len(existing) == 0 {
		b.WriteString("# Harness experience\n\nLessons TARS drew from finished focus pipelines in this repository, adopted when a person merged the pull request that proposed them.\n")
	} else {
		b.Write(existing)
	}
	for _, r := range adopted {
		fmt.Fprintf(&b, "\n## %s — %s\n\n", r.AdoptedAt.Format("2006-01-02"), oneLine(r.Lesson))
		situation := orNone(oneLine(r.SourceGoal))
		if r.SourcePR != "" {
			situation += " (" + r.SourcePR + ")"
		}
		fmt.Fprintf(&b, "- Situation: %s\n", situation)
		change := "`" + r.Path + "`"
		if r.Section != "" {
			change += fmt.Sprintf(" section %q", r.Section)
		}
		fmt.Fprintf(&b, "- Change: %s (%s)\n", change, r.Kind)
		b.WriteString("- Evidence:\n")
		for _, it := range r.Evidence {
			fmt.Fprintf(&b, "  - %s\n", HarnessEvidenceLine(it))
		}
		fmt.Fprintf(&b, "- Merged: %s\n", orNone(r.PRURL))
	}
	return atomicwrite.Write(path, []byte(b.String()))
}
