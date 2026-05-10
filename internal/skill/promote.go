package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PromoteMode controls whether the source skill directory is preserved
// after a successful copy.
type PromoteMode string

const (
	PromoteModeCopy PromoteMode = "copy"
	PromoteModeMove PromoteMode = "move"
)

// PromoteConflictPolicy decides what happens when a workspace skill with
// the same name already exists at the target location.
type PromoteConflictPolicy string

const (
	PromoteOnConflictRename    PromoteConflictPolicy = "rename"
	PromoteOnConflictOverwrite PromoteConflictPolicy = "overwrite"
	PromoteOnConflictAbort     PromoteConflictPolicy = "abort"
)

// PromoteAction describes the outcome relative to the requested name.
type PromoteAction string

const (
	PromoteActionCreated     PromoteAction = "created"
	PromoteActionOverwritten PromoteAction = "overwritten"
	PromoteActionRenamed     PromoteAction = "renamed"
)

// ErrPromoteConflict is returned when OnConflict=abort and a workspace
// skill of the same name already exists.
var ErrPromoteConflict = errors.New("workspace skill already exists")

// promoteNamePattern restricts skill names to a narrow lexical set that
// can never collapse into a path traversal. This is defense-in-depth on
// top of the IsLocal + Rel checks in safeJoinUnder.
var promoteNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)

// safeJoinUnder joins root and name, returning an error unless the result
// stays strictly under root. The defensive checks are redundant with the
// regex/IsLocal validation above but explicitly satisfy static-analysis
// dataflow rules (CodeQL go/path-injection).
func safeJoinUnder(root, name string) (string, error) {
	if !filepath.IsLocal(name) {
		return "", fmt.Errorf("invalid skill name (not local): %q", name)
	}
	joined := filepath.Join(root, name)
	rel, err := filepath.Rel(root, joined)
	if err != nil {
		return "", fmt.Errorf("invalid skill path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("skill path escapes root: %q", name)
	}
	return joined, nil
}

// PromoteRequest copies a session-local skill directory into the
// shared workspace skills root. The on-disk source is computed as
// safeJoinUnder(SourceSkillsRoot, Name) — the caller does not get to
// supply the full source path, which prevents path-traversal attacks
// from flowing through `Name` into filesystem operations.
type PromoteRequest struct {
	SourceSkillsRoot string // <cwd>/.tars/skills
	TargetSkillsRoot string // ${workspaceDir}/skills
	Name             string
	Mode             PromoteMode
	OnConflict       PromoteConflictPolicy
}

// PromoteResult captures the chosen target name + final on-disk action.
type PromoteResult struct {
	RequestedName string        `json:"requested_name"`
	TargetName    string        `json:"target_name"`
	TargetPath    string        `json:"target_path"`
	Action        PromoteAction `json:"action"`
	SourceDeleted bool          `json:"source_deleted"`
}

// Promote copies (or moves) a session-local skill directory into the
// workspace skills root, applying the requested conflict policy.
func Promote(req PromoteRequest) (PromoteResult, error) {
	sourceRoot := strings.TrimSpace(req.SourceSkillsRoot)
	root := strings.TrimSpace(req.TargetSkillsRoot)
	name := strings.TrimSpace(req.Name)
	if sourceRoot == "" {
		return PromoteResult{}, fmt.Errorf("source skills root is required")
	}
	if root == "" {
		return PromoteResult{}, fmt.Errorf("target skills root is required")
	}
	if name == "" {
		return PromoteResult{}, fmt.Errorf("name is required")
	}
	if !promoteNamePattern.MatchString(name) {
		return PromoteResult{}, fmt.Errorf("invalid skill name: %q", name)
	}

	mode := req.Mode
	if mode == "" {
		mode = PromoteModeCopy
	}
	if mode != PromoteModeCopy && mode != PromoteModeMove {
		return PromoteResult{}, fmt.Errorf("invalid mode: %q", req.Mode)
	}

	policy := req.OnConflict
	if policy == "" {
		policy = PromoteOnConflictRename
	}
	if policy != PromoteOnConflictRename && policy != PromoteOnConflictOverwrite && policy != PromoteOnConflictAbort {
		return PromoteResult{}, fmt.Errorf("invalid conflict policy: %q", req.OnConflict)
	}

	sourceRoot = filepath.Clean(sourceRoot)
	root = filepath.Clean(root)

	// `name` is the only attacker-influenced path component; it has already
	// been validated by the regex and is re-checked by safeJoinUnder.
	source, err := safeJoinUnder(sourceRoot, name)
	if err != nil {
		return PromoteResult{}, err
	}
	srcInfo, err := os.Stat(source)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("read source: %w", err)
	}
	if !srcInfo.IsDir() {
		return PromoteResult{}, fmt.Errorf("source is not a directory: %s", source)
	}
	skillFile, err := safeJoinUnder(source, "SKILL.md")
	if err != nil {
		return PromoteResult{}, err
	}
	if _, err := os.Stat(skillFile); err != nil {
		return PromoteResult{}, fmt.Errorf("source skill missing SKILL.md: %w", err)
	}

	targetName, action, err := resolvePromoteTargetName(root, name, policy)
	if err != nil {
		return PromoteResult{}, err
	}
	targetPath, err := safeJoinUnder(root, targetName)
	if err != nil {
		return PromoteResult{}, err
	}

	if action == PromoteActionOverwritten {
		if err := os.RemoveAll(targetPath); err != nil {
			return PromoteResult{}, fmt.Errorf("remove existing target: %w", err)
		}
	}

	if err := copyDirContents(source, targetPath); err != nil {
		return PromoteResult{}, fmt.Errorf("copy skill files: %w", err)
	}

	result := PromoteResult{
		RequestedName: name,
		TargetName:    targetName,
		TargetPath:    targetPath,
		Action:        action,
	}
	if mode == PromoteModeMove {
		if err := os.RemoveAll(source); err != nil {
			return result, fmt.Errorf("remove source after move: %w", err)
		}
		result.SourceDeleted = true
	}
	return result, nil
}

func resolvePromoteTargetName(root, name string, policy PromoteConflictPolicy) (string, PromoteAction, error) {
	statTarget := func(candidate string) (bool, error) {
		joined, err := safeJoinUnder(root, candidate)
		if err != nil {
			return false, err
		}
		if _, err := os.Stat(joined); err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("stat target: %w", err)
		}
		return true, nil
	}
	exists, err := statTarget(name)
	if err != nil {
		return "", "", err
	}
	if !exists {
		return name, PromoteActionCreated, nil
	}
	switch policy {
	case PromoteOnConflictOverwrite:
		return name, PromoteActionOverwritten, nil
	case PromoteOnConflictAbort:
		return "", "", ErrPromoteConflict
	case PromoteOnConflictRename:
		for i := 2; i < 1000; i++ {
			candidate := fmt.Sprintf("%s-%d", name, i)
			if !promoteNamePattern.MatchString(candidate) {
				return "", "", fmt.Errorf("rename candidate failed validation: %q", candidate)
			}
			candidateExists, err := statTarget(candidate)
			if err != nil {
				return "", "", err
			}
			if !candidateExists {
				return candidate, PromoteActionRenamed, nil
			}
		}
		return "", "", fmt.Errorf("exhausted rename suffixes for %q", name)
	default:
		return "", "", fmt.Errorf("invalid conflict policy: %q", policy)
	}
}

func copyDirContents(srcDir, dstDir string) error {
	srcDir = filepath.Clean(srcDir)
	dstDir = filepath.Clean(dstDir)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("create target dir: %w", err)
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target, err := safeJoinUnder(dstDir, rel)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink not supported in skill source: %s", path)
		}
		return copyRegularFile(path, target, info.Mode().Perm())
	})
}

func copyRegularFile(src, dst string, mode fs.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}
