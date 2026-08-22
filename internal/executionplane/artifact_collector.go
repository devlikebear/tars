package executionplane

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devlikebear/tars/internal/atomicwrite"
	"github.com/devlikebear/tars/internal/fileuri"
)

const (
	defaultArtifactMaxFiles = 128
	defaultArtifactMaxBytes = 32 << 20
)

type ArtifactCollectorOptions struct {
	RootDir           string
	Paths             []string
	IncludeTranscript bool
	IncludeGitPatch   bool
	MaxFiles          int
	MaxBytes          int64
}

type FileArtifactCollector struct {
	rootDir           string
	paths             []string
	includeTranscript bool
	includeGitPatch   bool
	maxFiles          int
	maxBytes          int64
}

func NewFileArtifactCollector(opts ArtifactCollectorOptions) (*FileArtifactCollector, error) {
	root, err := ensurePrivateDirectory(opts.RootDir)
	if err != nil {
		return nil, fmt.Errorf("executionplane: artifact root: %w", err)
	}
	paths := make([]string, 0, len(opts.Paths))
	for _, rawPath := range opts.Paths {
		path := filepath.Clean(strings.TrimSpace(rawPath))
		if path == "" || filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("executionplane: artifact path %q escapes the environment", rawPath)
		}
		paths = append(paths, path)
	}
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = defaultArtifactMaxFiles
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = defaultArtifactMaxBytes
	}
	return &FileArtifactCollector{
		rootDir: root, paths: paths, includeTranscript: opts.IncludeTranscript, includeGitPatch: opts.IncludeGitPatch,
		maxFiles: opts.MaxFiles, maxBytes: opts.MaxBytes,
	}, nil
}

func (collector *FileArtifactCollector) Collect(ctx context.Context, request CollectRequest) ([]CollectedArtifact, error) {
	root, err := canonicalDirectory(request.Environment.RootDir)
	if err != nil {
		return nil, fmt.Errorf("executionplane: collect artifact environment: %w", err)
	}
	if sameOrWithin(root, collector.rootDir) || sameOrWithin(collector.rootDir, root) {
		return nil, fmt.Errorf("executionplane: artifact destination and environment overlap")
	}
	workID := strings.TrimSpace(request.Execution.Work.ID)
	attemptID := strings.TrimSpace(request.Execution.Claim.Attempt.ID)
	if !safeStateID.MatchString(workID) || !safeStateID.MatchString(attemptID) {
		return nil, fmt.Errorf("executionplane: invalid durable identity for artifact collection")
	}
	destination := filepath.Join(collector.rootDir, workID, attemptID)
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return nil, fmt.Errorf("executionplane: create artifact destination: %w", err)
	}
	if err := os.Chmod(destination, 0o700); err != nil {
		return nil, err
	}

	candidates, err := collector.candidateFiles(ctx, root)
	if err != nil {
		return nil, err
	}
	artifacts := make([]CollectedArtifact, 0, len(candidates)+1)
	var totalBytes int64
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(artifacts) >= collector.maxFiles {
			return nil, fmt.Errorf("executionplane: artifact count exceeds %d", collector.maxFiles)
		}
		raw, err := os.ReadFile(candidate.absolute)
		if err != nil {
			return nil, fmt.Errorf("executionplane: read artifact %q: %w", candidate.relative, err)
		}
		raw = redactArtifactBytes(raw, request.RedactValues)
		totalBytes += int64(len(raw))
		if totalBytes > collector.maxBytes {
			return nil, fmt.Errorf("executionplane: artifact bytes exceed %d", collector.maxBytes)
		}
		artifact, err := writeCollectedArtifact(destination, "file", candidate.relative, raw)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	if collector.includeGitPatch {
		patch, err := collectGitPatch(ctx, root)
		if err != nil {
			return nil, err
		}
		if len(patch) > 0 {
			if len(artifacts) >= collector.maxFiles {
				return nil, fmt.Errorf("executionplane: artifact count exceeds %d", collector.maxFiles)
			}
			patch = redactArtifactBytes(patch, request.RedactValues)
			totalBytes += int64(len(patch))
			if totalBytes > collector.maxBytes {
				return nil, fmt.Errorf("executionplane: artifact bytes exceed %d", collector.maxBytes)
			}
			artifact, err := writeCollectedArtifact(destination, "patch", "changes.patch", patch)
			if err != nil {
				return nil, err
			}
			artifact.MediaType = "text/x-diff"
			artifacts = append(artifacts, artifact)
		}
	}
	if collector.includeTranscript && len(request.Worker.Transcript) > 0 {
		if len(artifacts) >= collector.maxFiles {
			return nil, fmt.Errorf("executionplane: artifact count exceeds %d", collector.maxFiles)
		}
		transcript, err := encodeTranscript(request.Worker.Transcript, request.RedactValues)
		if err != nil {
			return nil, err
		}
		totalBytes += int64(len(transcript))
		if totalBytes > collector.maxBytes {
			return nil, fmt.Errorf("executionplane: artifact bytes exceed %d", collector.maxBytes)
		}
		artifact, err := writeCollectedArtifact(destination, "transcript", "transcript.jsonl", transcript)
		if err != nil {
			return nil, err
		}
		artifact.MediaType = "application/x-ndjson"
		artifacts = append(artifacts, artifact)
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Name < artifacts[j].Name })
	return artifacts, nil
}

func collectGitPatch(ctx context.Context, root string) ([]byte, error) {
	const privatePathspec = ":(exclude).tars/**"
	if _, err := runGitCommand(ctx, root, "add", "--intent-to-add", "--", ".", privatePathspec); err != nil {
		return nil, fmt.Errorf("executionplane: include untracked files in Git patch: %w", err)
	}
	patch, err := runGitCommand(ctx, root, "diff", "--binary", "--no-ext-diff", "--no-textconv", "HEAD", "--", ".", privatePathspec)
	if err != nil {
		return nil, fmt.Errorf("executionplane: collect Git patch: %w", err)
	}
	if len(bytes.TrimSpace(patch)) == 0 {
		return nil, nil
	}
	return patch, nil
}

type artifactCandidate struct {
	absolute string
	relative string
}

func (collector *FileArtifactCollector) candidateFiles(ctx context.Context, root string) ([]artifactCandidate, error) {
	byRelative := map[string]artifactCandidate{}
	for _, requested := range collector.paths {
		matches := []string{filepath.Join(root, requested)}
		if strings.ContainsAny(requested, "*?[") {
			var err error
			matches, err = filepath.Glob(filepath.Join(root, requested))
			if err != nil {
				return nil, fmt.Errorf("executionplane: invalid artifact glob %q: %w", requested, err)
			}
		}
		for _, match := range matches {
			if err := collector.walkCandidate(ctx, root, match, byRelative); err != nil {
				return nil, err
			}
		}
	}
	result := make([]artifactCandidate, 0, len(byRelative))
	for _, candidate := range byRelative {
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].relative < result[j].relative })
	return result, nil
}

func (collector *FileArtifactCollector) walkCandidate(ctx context.Context, root, match string, byRelative map[string]artifactCandidate) error {
	clean := filepath.Clean(match)
	if !sameOrWithin(root, clean) {
		return fmt.Errorf("executionplane: artifact path escapes the environment")
	}
	info, err := os.Lstat(clean)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return addArtifactCandidate(root, clean, info, byRelative)
	}
	return filepath.WalkDir(clean, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path != clean && entry.IsDir() && excludedArtifactDirectory(entry.Name()) {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return addArtifactCandidate(root, path, info, byRelative)
	})
}

func addArtifactCandidate(root, path string, info os.FileInfo, byRelative map[string]artifactCandidate) error {
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("executionplane: artifact path escapes the environment")
	}
	relative = filepath.ToSlash(relative)
	if sensitiveArtifactPath(relative) {
		return nil
	}
	byRelative[relative] = artifactCandidate{absolute: path, relative: relative}
	return nil
}

func excludedArtifactDirectory(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case ".git", ".tars", "node_modules":
		return true
	default:
		return false
	}
}

func sensitiveArtifactPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case ".env", "id_rsa", "id_ed25519", "credentials", "credentials.json":
		return true
	}
	if strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") {
		return true
	}
	return strings.Contains(base, "credential") || strings.Contains(base, "secret") || strings.Contains(base, "token")
}

func redactArtifactBytes(raw []byte, values []string) []byte {
	redacted := append([]byte(nil), raw...)
	for _, value := range values {
		if value == "" {
			continue
		}
		redacted = bytes.ReplaceAll(redacted, []byte(value), []byte("[REDACTED]"))
	}
	return redacted
}

func encodeTranscript(entries []TranscriptEntry, redactions []string) ([]byte, error) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)
	for _, entry := range entries {
		raw, err := json.Marshal(entry)
		if err != nil {
			return nil, fmt.Errorf("executionplane: encode transcript: %w", err)
		}
		raw = redactArtifactBytes(raw, redactions)
		if _, err := writer.Write(append(raw, '\n')); err != nil {
			return nil, err
		}
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func writeCollectedArtifact(root, kind, name string, raw []byte) (CollectedArtifact, error) {
	path := filepath.Join(root, filepath.FromSlash(name))
	if !sameOrWithin(root, path) {
		return CollectedArtifact{}, fmt.Errorf("executionplane: artifact destination escapes root")
	}
	if err := atomicwrite.Write(path, raw); err != nil {
		return CollectedArtifact{}, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return CollectedArtifact{}, err
	}
	digest := sha256.Sum256(raw)
	mediaType := mime.TypeByExtension(filepath.Ext(name))
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	uri, err := fileuri.New(path)
	if err != nil {
		return CollectedArtifact{}, err
	}
	return CollectedArtifact{
		Kind: kind, Name: filepath.ToSlash(name), URI: uri,
		Digest: fmt.Sprintf("sha256:%x", digest[:]), MediaType: mediaType, SizeBytes: int64(len(raw)),
	}, nil
}

func filepathFromURI(raw string) (string, error) {
	path, err := fileuri.Path(raw)
	if err != nil {
		return "", fmt.Errorf("executionplane: invalid file artifact URI: %w", err)
	}
	return path, nil
}

var _ ArtifactCollector = (*FileArtifactCollector)(nil)
