package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

// PromoteRequest copies a session-local skill directory into the
// shared workspace skills root.
type PromoteRequest struct {
	SourceSkillDir   string // <cwd>/.tars/skills/<name>
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
	source := strings.TrimSpace(req.SourceSkillDir)
	root := strings.TrimSpace(req.TargetSkillsRoot)
	name := strings.TrimSpace(req.Name)
	if source == "" {
		return PromoteResult{}, fmt.Errorf("source skill directory is required")
	}
	if root == "" {
		return PromoteResult{}, fmt.Errorf("target skills root is required")
	}
	if name == "" {
		return PromoteResult{}, fmt.Errorf("name is required")
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." || strings.Contains(name, "..") {
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

	srcInfo, err := os.Stat(source)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("read source: %w", err)
	}
	if !srcInfo.IsDir() {
		return PromoteResult{}, fmt.Errorf("source is not a directory: %s", source)
	}
	if _, err := os.Stat(filepath.Join(source, "SKILL.md")); err != nil {
		return PromoteResult{}, fmt.Errorf("source skill missing SKILL.md: %w", err)
	}

	targetName, action, err := resolvePromoteTargetName(root, name, policy)
	if err != nil {
		return PromoteResult{}, err
	}
	targetPath := filepath.Join(root, targetName)

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
	if _, err := os.Stat(filepath.Join(root, name)); err != nil {
		if os.IsNotExist(err) {
			return name, PromoteActionCreated, nil
		}
		return "", "", fmt.Errorf("stat target: %w", err)
	}
	switch policy {
	case PromoteOnConflictOverwrite:
		return name, PromoteActionOverwritten, nil
	case PromoteOnConflictAbort:
		return "", "", ErrPromoteConflict
	case PromoteOnConflictRename:
		for i := 2; i < 1000; i++ {
			candidate := fmt.Sprintf("%s-%d", name, i)
			if _, err := os.Stat(filepath.Join(root, candidate)); err != nil {
				if os.IsNotExist(err) {
					return candidate, PromoteActionRenamed, nil
				}
				return "", "", fmt.Errorf("stat target candidate: %w", err)
			}
		}
		return "", "", fmt.Errorf("exhausted rename suffixes for %q", name)
	default:
		return "", "", fmt.Errorf("invalid conflict policy: %q", policy)
	}
}

func copyDirContents(srcDir, dstDir string) error {
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
		target := filepath.Join(dstDir, rel)
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
