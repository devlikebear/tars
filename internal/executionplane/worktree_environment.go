package executionplane

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/atomicwrite"
	"github.com/devlikebear/tars/internal/fileuri"
	"github.com/devlikebear/tars/internal/proofverifier"
	"github.com/devlikebear/tars/internal/workstore"
)

const worktreeMarkerRelativePath = ".tars/execution-environment.json"

type ManagedWorktreeProvider struct {
	sourceDir   string
	managedRoot string
	now         func() time.Time
}

type worktreeMarker struct {
	SchemaVersion int    `json:"schema_version"`
	EnvironmentID string `json:"environment_id"`
	SourceDir     string `json:"source_dir"`
	Head          string `json:"head"`
}

func NewManagedWorktreeProvider(sourceDir, managedRoot string) (*ManagedWorktreeProvider, error) {
	source, err := canonicalDirectory(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("executionplane: managed worktree source: %w", err)
	}
	root, err := ensurePrivateDirectory(managedRoot)
	if err != nil {
		return nil, err
	}
	if sameOrWithin(source, root) || sameOrWithin(root, source) {
		return nil, fmt.Errorf("executionplane: managed root and source repository must not overlap")
	}
	if _, err := runGitCommand(context.Background(), source, "rev-parse", "--show-toplevel"); err != nil {
		return nil, fmt.Errorf("executionplane: source is not a git repository: %w", err)
	}
	return &ManagedWorktreeProvider{sourceDir: source, managedRoot: root, now: time.Now}, nil
}

func (provider *ManagedWorktreeProvider) Name() string { return "managed-worktree" }

func (provider *ManagedWorktreeProvider) Capabilities() EnvironmentCapabilities {
	return EnvironmentCapabilities{
		Recoverable: true, Snapshot: true, Cleanup: true, FilesystemIsolation: true,
	}
}

func (provider *ManagedWorktreeProvider) Provision(ctx context.Context, request ProvisionRequest) (Environment, error) {
	source, err := canonicalDirectory(request.SourceDir)
	if err != nil {
		return Environment{}, err
	}
	if source != provider.sourceDir {
		return Environment{}, fmt.Errorf("executionplane: worktree source does not match configured repository")
	}
	attemptID := strings.TrimSpace(request.Execution.Claim.Attempt.ID)
	if !safeStateID.MatchString(attemptID) {
		return Environment{}, fmt.Errorf("executionplane: invalid attempt id for managed worktree")
	}
	target := filepath.Join(provider.managedRoot, attemptID)
	environmentID := "worktree:" + attemptID
	if _, statErr := os.Lstat(target); statErr == nil {
		existing := Environment{SchemaVersion: lifecycleSchemaVersion, ID: environmentID, Kind: provider.Name(), RootDir: target, SourceDir: source}
		return provider.Recover(ctx, existing)
	} else if !os.IsNotExist(statErr) {
		return Environment{}, fmt.Errorf("executionplane: inspect managed worktree target: %w", statErr)
	}
	headRaw, err := runGitCommand(ctx, source, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return Environment{}, err
	}
	head := strings.TrimSpace(string(headRaw))
	status, err := runGitCommand(ctx, source, "status", "--porcelain=v1", "-z")
	if err != nil {
		return Environment{}, err
	}
	if _, err := runGitCommand(ctx, source, "worktree", "add", "--detach", target, head); err != nil {
		return Environment{}, fmt.Errorf("executionplane: add managed worktree: %w", err)
	}
	marker := worktreeMarker{SchemaVersion: lifecycleSchemaVersion, EnvironmentID: environmentID, SourceDir: source, Head: head}
	if err := writeWorktreeMarker(target, marker); err != nil {
		_ = provider.removeRegisteredWorktree(context.Background(), target)
		return Environment{}, err
	}
	metadata, _ := json.Marshal(map[string]any{
		"provider": provider.Name(), "head": head,
		"source_dirty": len(status) > 0, "source_dirty_digest": digestRaw(status),
		"source_dirty_entries": bytes.Count(status, []byte{0}),
	})
	now := provider.now().UTC()
	return Environment{
		SchemaVersion: lifecycleSchemaVersion, ID: environmentID, Kind: provider.Name(),
		RootDir: target, SourceDir: source, MetadataJSON: metadata,
		ProvisionedAt: now, UpdatedAt: now,
	}, nil
}

func (provider *ManagedWorktreeProvider) Recover(ctx context.Context, environment Environment) (Environment, error) {
	if environment.Kind != provider.Name() || !sameOrWithin(provider.managedRoot, environment.RootDir) || filepath.Clean(environment.RootDir) == provider.managedRoot {
		return Environment{}, fmt.Errorf("executionplane: managed environment does not belong to this provider")
	}
	root, err := canonicalDirectory(environment.RootDir)
	if err != nil {
		return Environment{}, fmt.Errorf("executionplane: recover managed worktree: %w", err)
	}
	marker, err := readWorktreeMarker(root)
	if err != nil {
		return Environment{}, err
	}
	if marker.SchemaVersion != lifecycleSchemaVersion || marker.EnvironmentID != environment.ID || marker.SourceDir != provider.sourceDir {
		return Environment{}, fmt.Errorf("executionplane: managed worktree ownership marker mismatch")
	}
	if _, err := runGitCommand(ctx, root, "rev-parse", "--is-inside-work-tree"); err != nil {
		return Environment{}, fmt.Errorf("executionplane: managed worktree is not recoverable: %w", err)
	}
	environment.RootDir = root
	environment.SourceDir = provider.sourceDir
	environment.UpdatedAt = provider.now().UTC()
	return environment, nil
}

func (provider *ManagedWorktreeProvider) Sync(ctx context.Context, environment Environment) (EnvironmentSnapshot, error) {
	recovered, err := provider.Recover(ctx, environment)
	if err != nil {
		return EnvironmentSnapshot{}, err
	}
	verifier, err := proofverifier.New(proofverifier.Options{ID: "executionplane-worktree-snapshot", RootDir: recovered.RootDir})
	if err != nil {
		return EnvironmentSnapshot{}, err
	}
	digest, artifactDigests, err := verifier.SubjectDigest(ctx, workstore.ProofRequirement{Paths: []string{"."}})
	if err != nil {
		return EnvironmentSnapshot{}, fmt.Errorf("executionplane: snapshot managed worktree: %w", err)
	}
	status, statusErr := runGitCommand(ctx, recovered.RootDir, "status", "--porcelain=v1", "-z")
	if statusErr != nil {
		return EnvironmentSnapshot{}, statusErr
	}
	diff, diffErr := runGitCommand(ctx, recovered.RootDir, "diff", "--binary", "HEAD")
	if diffErr != nil {
		return EnvironmentSnapshot{}, diffErr
	}
	metadata, _ := json.Marshal(map[string]any{
		"artifact_digests": json.RawMessage(artifactDigests), "git_status_digest": digestRaw(status),
		"git_diff_digest": digestRaw(diff), "git_status_entries": bytes.Count(status, []byte{0}),
	})
	now := provider.now().UTC()
	uri, err := fileuri.New(recovered.RootDir)
	if err != nil {
		return EnvironmentSnapshot{}, err
	}
	return EnvironmentSnapshot{
		ID: "snapshot:" + strings.TrimPrefix(digest, "sha256:"), Digest: digest,
		URI:          uri,
		MetadataJSON: metadata, CreatedAt: now,
	}, nil
}

func (provider *ManagedWorktreeProvider) Destroy(ctx context.Context, environment Environment) error {
	if environment.Kind != provider.Name() || !sameOrWithin(provider.managedRoot, environment.RootDir) || filepath.Clean(environment.RootDir) == provider.managedRoot {
		return fmt.Errorf("executionplane: refusing to destroy a foreign worktree")
	}
	if _, err := os.Lstat(environment.RootDir); os.IsNotExist(err) {
		_, pruneErr := runGitCommand(ctx, provider.sourceDir, "worktree", "prune")
		return pruneErr
	} else if err != nil {
		return err
	}
	marker, err := readWorktreeMarker(environment.RootDir)
	if err != nil || marker.EnvironmentID != environment.ID || marker.SourceDir != provider.sourceDir {
		return fmt.Errorf("executionplane: refusing to destroy an unowned worktree")
	}
	if err := provider.removeRegisteredWorktree(ctx, environment.RootDir); err != nil {
		return err
	}
	_, err = runGitCommand(ctx, provider.sourceDir, "worktree", "prune")
	return err
}

func (provider *ManagedWorktreeProvider) removeRegisteredWorktree(ctx context.Context, target string) error {
	if _, err := runGitCommand(ctx, provider.sourceDir, "worktree", "remove", "--force", target); err != nil {
		return fmt.Errorf("executionplane: remove managed worktree: %w", err)
	}
	return nil
}

func writeWorktreeMarker(root string, marker worktreeMarker) error {
	payload, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	path := filepath.Join(root, filepath.FromSlash(worktreeMarkerRelativePath))
	if err := atomicwrite.Write(path, append(payload, '\n')); err != nil {
		return fmt.Errorf("executionplane: write worktree ownership marker: %w", err)
	}
	return os.Chmod(path, 0o600)
}

func readWorktreeMarker(root string) (worktreeMarker, error) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(worktreeMarkerRelativePath)))
	if err != nil {
		return worktreeMarker{}, fmt.Errorf("executionplane: read worktree ownership marker: %w", err)
	}
	var marker worktreeMarker
	if err := json.Unmarshal(raw, &marker); err != nil {
		return worktreeMarker{}, fmt.Errorf("executionplane: decode worktree ownership marker: %w", err)
	}
	return marker, nil
}

func ensurePrivateDirectory(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("executionplane: managed root is required")
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return "", err
	}
	return canonicalDirectory(absolute)
}

func sameOrWithin(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func runGitCommand(ctx context.Context, root string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"-C", root}, args...)
	cmd := exec.CommandContext(ctx, "git", commandArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func digestRaw(raw []byte) string {
	digest := sha256.Sum256(raw)
	return fmt.Sprintf("sha256:%x", digest[:])
}

var _ EnvironmentProvider = (*ManagedWorktreeProvider)(nil)
