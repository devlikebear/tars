package workerprotocol

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

const workspaceManifestSchemaVersion = 1

var (
	ErrUnsafeWorkspace  = errors.New("workerprotocol: workspace is unsafe to sync")
	ErrManifestMismatch = errors.New("workerprotocol: workspace manifest does not match content")
	ErrBundleLimit      = errors.New("workerprotocol: workspace bundle exceeds limits")
)

type WorkspaceBundleLimits struct {
	MaxFiles     int   `json:"max_files"`
	MaxFileBytes int64 `json:"max_file_bytes"`
	MaxBytes     int64 `json:"max_bytes"`
}

func DefaultWorkspaceBundleLimits() WorkspaceBundleLimits {
	return WorkspaceBundleLimits{MaxFiles: 4096, MaxFileBytes: 32 << 20, MaxBytes: 256 << 20}
}

func (limits WorkspaceBundleLimits) Validate() error {
	if limits.MaxFiles <= 0 || limits.MaxFileBytes <= 0 || limits.MaxBytes <= 0 || limits.MaxFileBytes > limits.MaxBytes {
		return ErrBundleLimit
	}
	return nil
}

type WorkspaceManifestEntry struct {
	Path      string `json:"path"`
	Digest    string `json:"digest"`
	SizeBytes int64  `json:"size_bytes"`
	Mode      uint32 `json:"mode"`
}

type WorkspaceManifest struct {
	SchemaVersion  int                      `json:"schema_version"`
	Mode           SyncMode                 `json:"mode"`
	Revision       string                   `json:"revision,omitempty"`
	Dirty          bool                     `json:"dirty,omitempty"`
	SourceOwner    Ownership                `json:"source_owner"`
	WorkspaceOwner Ownership                `json:"workspace_owner"`
	ArtifactOwner  Ownership                `json:"artifact_owner"`
	FileCount      int                      `json:"file_count"`
	TotalBytes     int64                    `json:"total_bytes"`
	ExcludedPaths  []string                 `json:"excluded_paths,omitempty"`
	Entries        []WorkspaceManifestEntry `json:"entries"`
	Digest         string                   `json:"digest"`
}

type WorkspaceFile struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Mode   uint32 `json:"mode"`
	Data   []byte `json:"data"`
}

type WorkspaceBundle struct {
	Manifest WorkspaceManifest `json:"manifest"`
	Files    []WorkspaceFile   `json:"files"`
}

type WorkspaceBundleOptions struct {
	RootDir string
	Mode    SyncMode
	GitPath string
	Limits  WorkspaceBundleLimits
}

func BuildWorkspaceBundle(ctx context.Context, opts WorkspaceBundleOptions) (WorkspaceBundle, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceBundle{}, err
	}
	root, err := canonicalWorkspaceRoot(opts.RootDir)
	if err != nil {
		return WorkspaceBundle{}, err
	}
	limits := opts.Limits
	if limits == (WorkspaceBundleLimits{}) {
		limits = DefaultWorkspaceBundleLimits()
	}
	if err := limits.Validate(); err != nil {
		return WorkspaceBundle{}, err
	}
	manifest := WorkspaceManifest{
		SchemaVersion: workspaceManifestSchemaVersion, Mode: opts.Mode,
		SourceOwner: OwnerGateway, WorkspaceOwner: OwnerWorker, ArtifactOwner: OwnerGateway,
		Entries: []WorkspaceManifestEntry{}, ExcludedPaths: []string{},
	}
	var paths []string
	switch opts.Mode {
	case SyncModeDirectory:
		paths, manifest.ExcludedPaths, err = directoryWorkspacePaths(ctx, root)
	case SyncModeGit:
		paths, manifest.Revision, manifest.Dirty, manifest.ExcludedPaths, err = gitWorkspacePaths(ctx, root, opts.GitPath)
	default:
		err = fmt.Errorf("%w: unsupported sync mode %q", ErrUnsafeWorkspace, opts.Mode)
	}
	if err != nil {
		return WorkspaceBundle{}, err
	}
	if len(paths) > limits.MaxFiles {
		return WorkspaceBundle{}, fmt.Errorf("%w: files %d exceed %d", ErrBundleLimit, len(paths), limits.MaxFiles)
	}
	bundle := WorkspaceBundle{Manifest: manifest, Files: make([]WorkspaceFile, 0, len(paths))}
	for _, relative := range paths {
		if err := ctx.Err(); err != nil {
			return WorkspaceBundle{}, err
		}
		file, entry, err := readWorkspaceFile(root, relative, limits)
		if err != nil {
			return WorkspaceBundle{}, err
		}
		bundle.Manifest.TotalBytes += entry.SizeBytes
		if bundle.Manifest.TotalBytes > limits.MaxBytes {
			return WorkspaceBundle{}, fmt.Errorf("%w: bytes %d exceed %d", ErrBundleLimit, bundle.Manifest.TotalBytes, limits.MaxBytes)
		}
		bundle.Files = append(bundle.Files, file)
		bundle.Manifest.Entries = append(bundle.Manifest.Entries, entry)
	}
	bundle.Manifest.FileCount = len(bundle.Files)
	sort.Strings(bundle.Manifest.ExcludedPaths)
	bundle.Manifest.ExcludedPaths = compactSortedStrings(bundle.Manifest.ExcludedPaths)
	digest, err := workspaceManifestDigest(bundle.Manifest)
	if err != nil {
		return WorkspaceBundle{}, err
	}
	bundle.Manifest.Digest = digest
	return bundle, nil
}

func VerifyWorkspaceBundle(bundle WorkspaceBundle, limits WorkspaceBundleLimits) error {
	if limits == (WorkspaceBundleLimits{}) {
		limits = DefaultWorkspaceBundleLimits()
	}
	if err := limits.Validate(); err != nil {
		return err
	}
	manifest := bundle.Manifest
	if manifest.SchemaVersion != workspaceManifestSchemaVersion ||
		(manifest.Mode != SyncModeDirectory && manifest.Mode != SyncModeGit) ||
		manifest.SourceOwner != OwnerGateway || manifest.WorkspaceOwner != OwnerWorker || manifest.ArtifactOwner != OwnerGateway {
		return ErrManifestMismatch
	}
	if manifest.FileCount != len(bundle.Files) || len(bundle.Files) != len(manifest.Entries) || len(bundle.Files) > limits.MaxFiles {
		return ErrManifestMismatch
	}
	var total int64
	previousPath := ""
	for index, file := range bundle.Files {
		if !safeWorkspaceRelativePath(file.Path) {
			return ErrUnsafeWorkspace
		}
		if sensitiveWorkspacePath(file.Path) || excludedWorkspaceDirectoryInPath(file.Path) {
			return ErrUnsafeWorkspace
		}
		if previousPath != "" && file.Path <= previousPath {
			return ErrManifestMismatch
		}
		previousPath = file.Path
		if int64(len(file.Data)) > limits.MaxFileBytes {
			return ErrBundleLimit
		}
		digest := digestBytes(file.Data)
		entry := manifest.Entries[index]
		if entry.Path != file.Path || entry.Digest != digest || file.Digest != digest ||
			entry.SizeBytes != int64(len(file.Data)) || entry.Mode != file.Mode || file.Mode&^0o777 != 0 {
			return ErrManifestMismatch
		}
		total += int64(len(file.Data))
		if total > limits.MaxBytes {
			return ErrBundleLimit
		}
	}
	if total != manifest.TotalBytes {
		return ErrManifestMismatch
	}
	digest, err := workspaceManifestDigest(manifest)
	if err != nil || digest != manifest.Digest {
		return ErrManifestMismatch
	}
	return nil
}

func ApplyWorkspaceBundle(ctx context.Context, destination string, bundle WorkspaceBundle, limits WorkspaceBundleLimits) error {
	if err := VerifyWorkspaceBundle(bundle, limits); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := ensureEmptyWorkspaceDestination(destination)
	if err != nil {
		return err
	}
	for _, file := range bundle.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(root, filepath.FromSlash(file.Path))
		if !sameOrWithinWorkspace(root, path) {
			return ErrUnsafeWorkspace
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("workerprotocol: create workspace directory: %w", err)
		}
		if err := atomicwrite.Write(path, file.Data); err != nil {
			return fmt.Errorf("workerprotocol: write workspace file: %w", err)
		}
		mode := os.FileMode(0o600 | (file.Mode & 0o111))
		if err := os.Chmod(path, mode); err != nil {
			return fmt.Errorf("workerprotocol: secure workspace file: %w", err)
		}
	}
	return nil
}

func directoryWorkspacePaths(ctx context.Context, root string) ([]string, []string, error) {
	paths := []string{}
	excluded := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symbolic link %q", ErrUnsafeWorkspace, relative)
		}
		if entry.IsDir() {
			if excludedWorkspaceDirectory(entry.Name()) {
				excluded = append(excluded, relative+"/")
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("%w: non-regular file %q", ErrUnsafeWorkspace, relative)
		}
		if sensitiveWorkspacePath(relative) {
			excluded = append(excluded, relative)
			return nil
		}
		paths = append(paths, relative)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(paths)
	return paths, excluded, nil
}

func gitWorkspacePaths(ctx context.Context, root, gitPath string) ([]string, string, bool, []string, error) {
	if strings.TrimSpace(gitPath) == "" {
		var err error
		gitPath, err = exec.LookPath("git")
		if err != nil {
			return nil, "", false, nil, fmt.Errorf("workerprotocol: locate git: %w", err)
		}
	}
	topLevel, err := runWorkspaceGit(ctx, gitPath, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, "", false, nil, err
	}
	canonicalTop, err := canonicalWorkspaceRoot(strings.TrimSpace(string(topLevel)))
	if err != nil || canonicalTop != root {
		return nil, "", false, nil, fmt.Errorf("%w: git root does not match workspace root", ErrUnsafeWorkspace)
	}
	revisionRaw, err := runWorkspaceGit(ctx, gitPath, root, "rev-parse", "HEAD")
	if err != nil {
		return nil, "", false, nil, err
	}
	revision := strings.TrimSpace(string(revisionRaw))
	if len(revision) != 40 && len(revision) != 64 {
		return nil, "", false, nil, fmt.Errorf("%w: invalid git revision", ErrUnsafeWorkspace)
	}
	status, err := runWorkspaceGit(ctx, gitPath, root, "status", "--porcelain=v1", "--untracked-files=no")
	if err != nil {
		return nil, "", false, nil, err
	}
	tracked, err := runWorkspaceGit(ctx, gitPath, root, "ls-files", "-z", "--cached")
	if err != nil {
		return nil, "", false, nil, err
	}
	paths := []string{}
	excluded := []string{}
	for _, rawPath := range strings.Split(string(tracked), "\x00") {
		if rawPath == "" {
			continue
		}
		relative := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rawPath)))
		if !safeWorkspaceRelativePath(relative) {
			return nil, "", false, nil, fmt.Errorf("%w: tracked path %q", ErrUnsafeWorkspace, rawPath)
		}
		if sensitiveWorkspacePath(relative) || excludedWorkspaceDirectoryInPath(relative) {
			excluded = append(excluded, relative)
			continue
		}
		paths = append(paths, relative)
	}
	sort.Strings(paths)
	if len(paths) != len(compactSortedStrings(paths)) {
		return nil, "", false, nil, fmt.Errorf("%w: duplicate tracked paths", ErrUnsafeWorkspace)
	}
	return paths, revision, len(bytesTrimSpace(status)) > 0, excluded, nil
}

func runWorkspaceGit(ctx context.Context, gitPath, root string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"-C", root}, args...)
	command := exec.CommandContext(ctx, gitPath, commandArgs...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_OPTIONAL_LOCKS=0")
	output, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("workerprotocol: git %s: %s", args[0], strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("workerprotocol: git %s: %w", args[0], err)
	}
	return output, nil
}

func readWorkspaceFile(root, relative string, limits WorkspaceBundleLimits) (WorkspaceFile, WorkspaceManifestEntry, error) {
	if !safeWorkspaceRelativePath(relative) {
		return WorkspaceFile{}, WorkspaceManifestEntry{}, ErrUnsafeWorkspace
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	if !sameOrWithinWorkspace(root, path) {
		return WorkspaceFile{}, WorkspaceManifestEntry{}, ErrUnsafeWorkspace
	}
	info, err := os.Lstat(path)
	if err != nil {
		return WorkspaceFile{}, WorkspaceManifestEntry{}, fmt.Errorf("workerprotocol: inspect workspace file %q: %w", relative, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return WorkspaceFile{}, WorkspaceManifestEntry{}, fmt.Errorf("%w: file %q is not regular", ErrUnsafeWorkspace, relative)
	}
	if info.Size() > limits.MaxFileBytes {
		return WorkspaceFile{}, WorkspaceManifestEntry{}, fmt.Errorf("%w: file %q exceeds %d bytes", ErrBundleLimit, relative, limits.MaxFileBytes)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return WorkspaceFile{}, WorkspaceManifestEntry{}, fmt.Errorf("workerprotocol: read workspace file %q: %w", relative, err)
	}
	digest := digestBytes(raw)
	mode := uint32(info.Mode().Perm())
	file := WorkspaceFile{Path: relative, Digest: digest, Mode: mode, Data: raw}
	entry := WorkspaceManifestEntry{Path: relative, Digest: digest, SizeBytes: int64(len(raw)), Mode: mode}
	return file, entry, nil
}

func workspaceManifestDigest(manifest WorkspaceManifest) (string, error) {
	manifest.Digest = ""
	raw, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("workerprotocol: encode workspace manifest: %w", err)
	}
	return digestBytes(raw), nil
}

func digestBytes(raw []byte) string {
	digest := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func canonicalWorkspaceRoot(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: workspace root is required", ErrUnsafeWorkspace)
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: resolve workspace root: %v", ErrUnsafeWorkspace, err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("%w: resolve workspace root: %v", ErrUnsafeWorkspace, err)
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: workspace root is not a directory", ErrUnsafeWorkspace)
	}
	return filepath.Clean(canonical), nil
}

func ensureEmptyWorkspaceDestination(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: destination is required", ErrUnsafeWorkspace)
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: resolve destination", ErrUnsafeWorkspace)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return "", fmt.Errorf("workerprotocol: create workspace destination: %w", err)
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return "", fmt.Errorf("workerprotocol: secure workspace destination: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("%w: resolve destination", ErrUnsafeWorkspace)
	}
	entries, err := os.ReadDir(canonical)
	if err != nil {
		return "", fmt.Errorf("workerprotocol: inspect workspace destination: %w", err)
	}
	if len(entries) != 0 {
		return "", fmt.Errorf("%w: destination must be empty", ErrUnsafeWorkspace)
	}
	return canonical, nil
}

func safeWorkspaceRelativePath(path string) bool {
	if path == "" || filepath.IsAbs(filepath.FromSlash(path)) || strings.ContainsRune(path, '\x00') || strings.Contains(path, "\\") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	return clean == path && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func excludedWorkspaceDirectory(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case ".git", ".tars", "node_modules":
		return true
	default:
		return false
	}
}

func excludedWorkspaceDirectoryInPath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if excludedWorkspaceDirectory(part) {
			return true
		}
	}
	return false
}

func sensitiveWorkspacePath(path string) bool {
	base := strings.ToLower(filepath.Base(filepath.FromSlash(path)))
	if base == ".ds_store" {
		return true
	}
	if base == ".tars-result.json" {
		return true
	}
	if base == ".env" || (strings.HasPrefix(base, ".env.") && !strings.HasSuffix(base, ".example")) {
		return true
	}
	switch base {
	case "id_rsa", "id_ed25519", "credentials", "credentials.json", ".netrc", ".npmrc", ".pypirc":
		return true
	}
	if strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") || strings.HasSuffix(base, ".p12") || strings.HasSuffix(base, ".pfx") {
		return true
	}
	return strings.Contains(base, "credential") || strings.Contains(base, "private-key")
}

func sameOrWithinWorkspace(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func compactSortedStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func bytesTrimSpace(raw []byte) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}
