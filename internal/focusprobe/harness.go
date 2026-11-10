package focusprobe

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/pkg/skill"
)

const (
	// harnessMaxHeadings caps the headings read from one guide.
	harnessMaxHeadings = 60
	// harnessMaxSkills caps the skills listed.
	harnessMaxSkills = 40
	// harnessMaxSkillFiles caps the files listed for one skill.
	harnessMaxSkillFiles = 20
	// harnessMaxReadBytes caps how much of one file is read.
	harnessMaxReadBytes = 256 << 10
)

// ReadHarnessFiles reads what the agent harness of the repository at repo
// holds now, for a harness proposal's prompt (focuspipeline.HarnessFiles):
// the section headings of its top-level CLAUDE.md and AGENTS.md, and each
// .tars/skills/<name> with its SKILL.md description and files. Symbolic
// links are not followed. A missing piece is simply left out.
func ReadHarnessFiles(repo string) focuspipeline.HarnessFiles {
	var out focuspipeline.HarnessFiles
	if strings.TrimSpace(repo) == "" {
		return out
	}
	for _, name := range focuspipeline.HarnessGuides {
		text, ok := readRegular(filepath.Join(repo, name))
		if !ok {
			continue
		}
		out.Guides = append(out.Guides, focuspipeline.HarnessGuide{Path: name, Headings: markdownHeadings(text)})
	}
	skillsDir := filepath.Join(repo, ".tars", "skills")
	if info, err := os.Lstat(skillsDir); err != nil || !info.IsDir() {
		return out
	}
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if len(out.Skills) >= harnessMaxSkills {
			break
		}
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out.Skills = append(out.Skills, readSkill(filepath.Join(skillsDir, e.Name()), e.Name()))
	}
	return out
}

func readSkill(dir, name string) focuspipeline.HarnessSkill {
	s := focuspipeline.HarnessSkill{Name: name}
	if text, ok := readRegular(filepath.Join(dir, "SKILL.md")); ok {
		if fm, _, err := skill.ParseFrontmatter(text); err == nil {
			s.Description = strings.TrimSpace(fm.Description)
		}
	}
	for _, sub := range []string{"", "scripts"} {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Type().IsRegular() && len(s.Files) < harnessMaxSkillFiles {
				s.Files = append(s.Files, filepath.ToSlash(filepath.Join(sub, e.Name())))
			}
		}
	}
	sort.Strings(s.Files)
	return s
}

// readRegular reads a regular file (never through a symbolic link), at most
// harnessMaxReadBytes of it.
func readRegular(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, harnessMaxReadBytes))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// markdownHeadings are the ATX headings of a markdown text outside code
// fences, as written ("## Build").
func markdownHeadings(text string) []string {
	var out []string
	fenced := false
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64<<10), harnessMaxReadBytes)
	for sc.Scan() && len(out) < harnessMaxHeadings {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced || !strings.HasPrefix(line, "#") {
			continue
		}
		level := len(line) - len(strings.TrimLeft(line, "#"))
		if level > 6 || len(line) == level || line[level] != ' ' {
			continue
		}
		out = append(out, line)
	}
	return out
}
