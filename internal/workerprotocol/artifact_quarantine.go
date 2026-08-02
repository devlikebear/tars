package workerprotocol

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/devlikebear/tars/internal/atomicwrite"
	"github.com/devlikebear/tars/internal/secrets"
)

type ArtifactQuarantineLimits struct {
	MaxArtifacts int   `json:"max_artifacts"`
	MaxFileBytes int64 `json:"max_file_bytes"`
	MaxBytes     int64 `json:"max_bytes"`
}

func DefaultArtifactQuarantineLimits() ArtifactQuarantineLimits {
	return ArtifactQuarantineLimits{MaxArtifacts: 128, MaxFileBytes: 32 << 20, MaxBytes: 64 << 20}
}

func (limits ArtifactQuarantineLimits) Validate() error {
	if limits.MaxArtifacts <= 0 || limits.MaxFileBytes <= 0 || limits.MaxBytes <= 0 || limits.MaxFileBytes > limits.MaxBytes {
		return ErrBundleLimit
	}
	return nil
}

type ArtifactScanner interface {
	Scan(name, mediaType string, raw []byte, forbiddenValues []string) error
}

type ArtifactQuarantineOptions struct {
	RootDir string
	Limits  ArtifactQuarantineLimits
	Scanner ArtifactScanner
}

type ReleasedArtifact struct {
	Name      string `json:"name"`
	URI       string `json:"uri"`
	Digest    string `json:"digest"`
	MediaType string `json:"media_type,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
}

type RejectedArtifact struct {
	Name   string `json:"name"`
	Digest string `json:"digest,omitempty"`
	Reason string `json:"reason"`
}

type ArtifactQuarantineReport struct {
	Accepted []ReleasedArtifact `json:"accepted"`
	Rejected []RejectedArtifact `json:"rejected"`
}

type ArtifactQuarantine struct {
	mu             sync.Mutex
	quarantineRoot string
	acceptedRoot   string
	limits         ArtifactQuarantineLimits
	scanner        ArtifactScanner
}

func NewArtifactQuarantine(opts ArtifactQuarantineOptions) (*ArtifactQuarantine, error) {
	root := strings.TrimSpace(opts.RootDir)
	if root == "" {
		return nil, fmt.Errorf("workerprotocol: artifact quarantine root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("workerprotocol: resolve artifact quarantine root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("workerprotocol: create artifact quarantine root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("workerprotocol: resolve artifact quarantine root: %w", err)
	}
	limits := opts.Limits
	if limits == (ArtifactQuarantineLimits{}) {
		limits = DefaultArtifactQuarantineLimits()
	}
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	if opts.Scanner == nil {
		opts.Scanner = DefaultArtifactScanner{}
	}
	quarantineRoot := filepath.Join(canonical, "quarantine")
	acceptedRoot := filepath.Join(canonical, "accepted")
	for _, path := range []string{canonical, quarantineRoot, acceptedRoot} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return nil, fmt.Errorf("workerprotocol: create artifact quarantine directory: %w", err)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return nil, fmt.Errorf("workerprotocol: secure artifact quarantine directory: %w", err)
		}
	}
	return &ArtifactQuarantine{
		quarantineRoot: quarantineRoot, acceptedRoot: acceptedRoot, limits: limits, scanner: opts.Scanner,
	}, nil
}

func (quarantine *ArtifactQuarantine) InspectAndRelease(ctx context.Context, placementID string, artifacts []WireArtifact, forbiddenValues []string) (ArtifactQuarantineReport, error) {
	report := ArtifactQuarantineReport{Accepted: []ReleasedArtifact{}, Rejected: []RejectedArtifact{}}
	if quarantine == nil || quarantine.scanner == nil || !validProtocolIdentifier(placementID) {
		return report, fmt.Errorf("workerprotocol: invalid artifact quarantine request")
	}
	if len(artifacts) > quarantine.limits.MaxArtifacts {
		return report, fmt.Errorf("%w: artifacts %d exceed %d", ErrBundleLimit, len(artifacts), quarantine.limits.MaxArtifacts)
	}
	quarantine.mu.Lock()
	defer quarantine.mu.Unlock()
	seen := make(map[string]struct{}, len(artifacts))
	var totalBytes int64
	for _, artifact := range artifacts {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		rejection := validateWireArtifact(artifact, quarantine.limits, &totalBytes, seen)
		if rejection == "" {
			if err := quarantine.scanner.Scan(artifact.Name, artifact.MediaType, artifact.Data, forbiddenValues); err != nil {
				rejection = "secret_detected"
			}
		}
		if rejection != "" {
			report.Rejected = append(report.Rejected, RejectedArtifact{Name: artifact.Name, Digest: artifact.Digest, Reason: rejection})
			continue
		}
		released, err := quarantine.release(ctx, placementID, artifact)
		if err != nil {
			return report, err
		}
		report.Accepted = append(report.Accepted, released)
	}
	return report, nil
}

func validateWireArtifact(artifact WireArtifact, limits ArtifactQuarantineLimits, totalBytes *int64, seen map[string]struct{}) string {
	if !safeWorkspaceRelativePath(artifact.Name) {
		return "invalid_path"
	}
	if sensitiveWorkspacePath(artifact.Name) || excludedWorkspaceDirectoryInPath(artifact.Name) {
		return "sensitive_path"
	}
	if _, ok := seen[artifact.Name]; ok {
		return "duplicate_name"
	}
	seen[artifact.Name] = struct{}{}
	size := int64(len(artifact.Data))
	*totalBytes += size
	if size > limits.MaxFileBytes || *totalBytes > limits.MaxBytes {
		return "limit_exceeded"
	}
	if artifact.Digest != digestBytes(artifact.Data) {
		return "digest_mismatch"
	}
	return ""
}

func (quarantine *ArtifactQuarantine) release(ctx context.Context, placementID string, artifact WireArtifact) (ReleasedArtifact, error) {
	if err := ctx.Err(); err != nil {
		return ReleasedArtifact{}, err
	}
	quarantineDir := filepath.Join(quarantine.quarantineRoot, placementID)
	acceptedDir := filepath.Join(quarantine.acceptedRoot, placementID)
	for _, path := range []string{quarantineDir, acceptedDir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return ReleasedArtifact{}, fmt.Errorf("workerprotocol: create artifact placement directory: %w", err)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return ReleasedArtifact{}, fmt.Errorf("workerprotocol: secure artifact placement directory: %w", err)
		}
	}
	destination := filepath.Join(acceptedDir, filepath.FromSlash(artifact.Name))
	if !sameOrWithinWorkspace(acceptedDir, destination) {
		return ReleasedArtifact{}, ErrUnsafeWorkspace
	}
	if existing, err := os.ReadFile(destination); err == nil {
		if digestBytes(existing) != artifact.Digest {
			return ReleasedArtifact{}, fmt.Errorf("%w: accepted artifact %q changed digest", ErrManifestMismatch, artifact.Name)
		}
		return releasedArtifact(destination, artifact), nil
	} else if !os.IsNotExist(err) {
		return ReleasedArtifact{}, fmt.Errorf("workerprotocol: inspect accepted artifact: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return ReleasedArtifact{}, fmt.Errorf("workerprotocol: create accepted artifact directory: %w", err)
	}
	pending := filepath.Join(quarantineDir, strings.TrimPrefix(artifact.Digest, "sha256:")+".pending")
	if err := atomicwrite.Write(pending, artifact.Data); err != nil {
		return ReleasedArtifact{}, fmt.Errorf("workerprotocol: stage quarantined artifact: %w", err)
	}
	if err := os.Chmod(pending, 0o600); err != nil {
		_ = os.Remove(pending)
		return ReleasedArtifact{}, fmt.Errorf("workerprotocol: secure quarantined artifact: %w", err)
	}
	if err := os.Rename(pending, destination); err != nil {
		_ = os.Remove(pending)
		return ReleasedArtifact{}, fmt.Errorf("workerprotocol: release quarantined artifact: %w", err)
	}
	if err := os.Chmod(destination, 0o600); err != nil {
		return ReleasedArtifact{}, fmt.Errorf("workerprotocol: secure accepted artifact: %w", err)
	}
	return releasedArtifact(destination, artifact), nil
}

func releasedArtifact(path string, artifact WireArtifact) ReleasedArtifact {
	return ReleasedArtifact{
		Name: artifact.Name, URI: (&url.URL{Scheme: "file", Path: path}).String(),
		Digest: artifact.Digest, MediaType: artifact.MediaType, SizeBytes: int64(len(artifact.Data)),
	}
}

type DefaultArtifactScanner struct{}

func (DefaultArtifactScanner) Scan(_ string, mediaType string, raw []byte, forbiddenValues []string) error {
	for _, value := range forbiddenValues {
		if value != "" && bytes.Contains(raw, []byte(value)) {
			return errors.New("workerprotocol: artifact contains a task credential")
		}
	}
	if !artifactLooksTextual(mediaType, raw) {
		return nil
	}
	text := string(raw)
	if secrets.RedactText(text) != text {
		return errors.New("workerprotocol: artifact contains secret-shaped content")
	}
	return nil
}

func artifactLooksTextual(_ string, raw []byte) bool {
	// Media types are supplied by an untrusted worker and therefore cannot be
	// used to opt out of scanning. Any valid UTF-8 payload without NUL bytes is
	// treated as text, including artifacts with an empty or binary-looking type.
	return utf8.Valid(raw) && !bytes.ContainsRune(raw, '\x00')
}

func artifactFilePath(rawURI string) (string, error) {
	parsed, err := url.Parse(rawURI)
	if err != nil || parsed.Scheme != "file" || parsed.Host != "" || parsed.Path == "" {
		return "", fmt.Errorf("workerprotocol: invalid artifact file URI")
	}
	return filepath.FromSlash(parsed.Path), nil
}

var _ ArtifactScanner = DefaultArtifactScanner{}
