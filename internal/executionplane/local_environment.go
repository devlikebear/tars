package executionplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/proofverifier"
	"github.com/devlikebear/tars/internal/workstore"
)

type LocalEnvironmentProvider struct {
	rootDir string
	now     func() time.Time
}

func NewLocalEnvironmentProvider(rootDir string) (*LocalEnvironmentProvider, error) {
	root, err := canonicalDirectory(rootDir)
	if err != nil {
		return nil, fmt.Errorf("executionplane: local environment root: %w", err)
	}
	return &LocalEnvironmentProvider{rootDir: root, now: time.Now}, nil
}

func (provider *LocalEnvironmentProvider) Name() string { return "local" }

func (provider *LocalEnvironmentProvider) Capabilities() EnvironmentCapabilities {
	return EnvironmentCapabilities{Recoverable: true, Snapshot: true}
}

func (provider *LocalEnvironmentProvider) Provision(_ context.Context, request ProvisionRequest) (Environment, error) {
	requested, err := canonicalDirectory(request.SourceDir)
	if err != nil {
		return Environment{}, err
	}
	if requested != provider.rootDir {
		return Environment{}, fmt.Errorf("executionplane: local source %q does not match configured root", requested)
	}
	now := provider.now().UTC()
	metadata, _ := json.Marshal(map[string]any{"provider": provider.Name(), "shared_source": true})
	return Environment{
		SchemaVersion: lifecycleSchemaVersion,
		ID:            "local:" + strings.TrimSpace(request.Execution.Claim.Attempt.ID),
		Kind:          provider.Name(),
		RootDir:       provider.rootDir,
		SourceDir:     provider.rootDir,
		MetadataJSON:  metadata,
		ProvisionedAt: now,
		UpdatedAt:     now,
	}, nil
}

func (provider *LocalEnvironmentProvider) Recover(_ context.Context, environment Environment) (Environment, error) {
	root, err := canonicalDirectory(environment.RootDir)
	if err != nil {
		return Environment{}, fmt.Errorf("executionplane: recover local environment: %w", err)
	}
	if root != provider.rootDir || environment.Kind != provider.Name() {
		return Environment{}, fmt.Errorf("executionplane: local environment does not belong to this provider")
	}
	environment.RootDir = root
	environment.UpdatedAt = provider.now().UTC()
	return environment, nil
}

func (provider *LocalEnvironmentProvider) Sync(ctx context.Context, environment Environment) (EnvironmentSnapshot, error) {
	recovered, err := provider.Recover(ctx, environment)
	if err != nil {
		return EnvironmentSnapshot{}, err
	}
	verifier, err := proofverifier.New(proofverifier.Options{
		ID: "executionplane-local-snapshot", RootDir: recovered.RootDir,
	})
	if err != nil {
		return EnvironmentSnapshot{}, err
	}
	digest, artifactDigests, err := verifier.SubjectDigest(ctx, workstore.ProofRequirement{Paths: []string{"."}})
	if err != nil {
		return EnvironmentSnapshot{}, fmt.Errorf("executionplane: snapshot local environment: %w", err)
	}
	metadata, _ := json.Marshal(map[string]any{"artifact_digests": json.RawMessage(artifactDigests)})
	now := provider.now().UTC()
	return EnvironmentSnapshot{
		ID: "snapshot:" + strings.TrimPrefix(digest, "sha256:"), Digest: digest,
		URI:          (&url.URL{Scheme: "file", Path: recovered.RootDir}).String(),
		MetadataJSON: metadata, CreatedAt: now,
	}, nil
}

func (provider *LocalEnvironmentProvider) Destroy(_ context.Context, environment Environment) error {
	if environment.Kind != provider.Name() {
		return fmt.Errorf("executionplane: refusing to destroy a foreign local environment")
	}
	// Local execution is intentionally shared with the source tree. Cleanup is
	// therefore a no-op and never deletes or rewrites user files.
	return nil
}

func canonicalDirectory(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("directory is required")
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("directory is unavailable")
	}
	return filepath.Clean(resolved), nil
}

var _ EnvironmentProvider = (*LocalEnvironmentProvider)(nil)
