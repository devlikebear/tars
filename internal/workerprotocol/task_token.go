package workerprotocol

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	taskTokenPrefix  = "tars-task-v1"
	taskTokenVersion = 1
)

var (
	ErrTaskTokenInvalid   = errors.New("workerprotocol: task token is invalid")
	ErrTaskTokenSignature = errors.New("workerprotocol: task token signature is invalid")
	ErrTaskTokenBinding   = errors.New("workerprotocol: task token binding does not match")
	ErrTaskTokenScope     = errors.New("workerprotocol: task token scope is not allowed")
	ErrTaskTokenExpired   = errors.New("workerprotocol: task token is expired")
	ErrTaskTokenTTL       = errors.New("workerprotocol: task token TTL is invalid")
)

type TaskScope string

const (
	TaskScopeExecute    TaskScope = "execute"
	TaskScopeStream     TaskScope = "stream"
	TaskScopeCheckpoint TaskScope = "checkpoint"
	TaskScopeCollect    TaskScope = "collect"
	TaskScopeDestroy    TaskScope = "destroy"
)

type TaskTokenBinding struct {
	WorkspaceID string `json:"workspace_id"`
	WorkID      string `json:"work_id"`
	StepID      string `json:"step_id"`
	AttemptID   string `json:"attempt_id"`
	PlacementID string `json:"placement_id"`
	WorkerID    string `json:"worker_id"`
}

type TaskTokenClaims struct {
	Version       int         `json:"version"`
	KeyID         string      `json:"key_id"`
	TokenID       string      `json:"token_id"`
	WorkspaceID   string      `json:"workspace_id"`
	WorkID        string      `json:"work_id"`
	StepID        string      `json:"step_id"`
	AttemptID     string      `json:"attempt_id"`
	PlacementID   string      `json:"placement_id"`
	WorkerID      string      `json:"worker_id"`
	Scopes        []TaskScope `json:"scopes"`
	IssuedAtUnix  int64       `json:"issued_at_unix"`
	ExpiresAtUnix int64       `json:"expires_at_unix"`
}

type TaskTokenIssuerOptions struct {
	PrivateKey ed25519.PrivateKey
	MaxTTL     time.Duration
	Now        func() time.Time
}

type TaskTokenIssuer struct {
	privateKey ed25519.PrivateKey
	verifier   *TaskTokenVerifier
	maxTTL     time.Duration
	now        func() time.Time
}

type TaskTokenVerifier struct {
	PublicKey ed25519.PublicKey `json:"public_key"`
	KeyID     string            `json:"key_id"`
	maxTTL    time.Duration
	now       func() time.Time
}

func NewTaskTokenIssuer(opts TaskTokenIssuerOptions) (*TaskTokenIssuer, error) {
	if len(opts.PrivateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: Ed25519 private key is required", ErrTaskTokenInvalid)
	}
	if opts.MaxTTL <= 0 {
		return nil, fmt.Errorf("%w: positive maximum TTL is required", ErrTaskTokenTTL)
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	privateKey := append(ed25519.PrivateKey(nil), opts.PrivateKey...)
	publicKey := append(ed25519.PublicKey(nil), privateKey.Public().(ed25519.PublicKey)...)
	verifier, err := NewTaskTokenVerifier(publicKey, opts.MaxTTL, opts.Now)
	if err != nil {
		return nil, err
	}
	return &TaskTokenIssuer{privateKey: privateKey, verifier: verifier, maxTTL: opts.MaxTTL, now: opts.Now}, nil
}

func NewTaskTokenVerifier(publicKey ed25519.PublicKey, maxTTL time.Duration, now func() time.Time) (*TaskTokenVerifier, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: Ed25519 public key is required", ErrTaskTokenInvalid)
	}
	if maxTTL <= 0 {
		return nil, fmt.Errorf("%w: positive maximum TTL is required", ErrTaskTokenTTL)
	}
	if now == nil {
		now = time.Now
	}
	publicCopy := append(ed25519.PublicKey(nil), publicKey...)
	return &TaskTokenVerifier{
		PublicKey: publicCopy, KeyID: taskTokenKeyID(publicCopy), maxTTL: maxTTL, now: now,
	}, nil
}

func (issuer *TaskTokenIssuer) KeyID() string {
	if issuer == nil || issuer.verifier == nil {
		return ""
	}
	return issuer.verifier.KeyID
}

func (issuer *TaskTokenIssuer) PublicVerifier() *TaskTokenVerifier {
	if issuer == nil || issuer.verifier == nil {
		return nil
	}
	copy := *issuer.verifier
	copy.PublicKey = append(ed25519.PublicKey(nil), issuer.verifier.PublicKey...)
	return &copy
}

func (issuer *TaskTokenIssuer) Issue(binding TaskTokenBinding, scopes []TaskScope, ttl time.Duration) (string, TaskTokenClaims, error) {
	if issuer == nil || len(issuer.privateKey) != ed25519.PrivateKeySize || issuer.verifier == nil {
		return "", TaskTokenClaims{}, ErrTaskTokenInvalid
	}
	if err := validateTaskTokenBinding(binding); err != nil {
		return "", TaskTokenClaims{}, err
	}
	if ttl <= 0 || ttl > issuer.maxTTL {
		return "", TaskTokenClaims{}, ErrTaskTokenTTL
	}
	normalizedScopes, err := normalizeTaskScopes(scopes)
	if err != nil {
		return "", TaskTokenClaims{}, err
	}
	tokenID, err := newTaskTokenID()
	if err != nil {
		return "", TaskTokenClaims{}, fmt.Errorf("workerprotocol: generate task token id: %w", err)
	}
	now := issuer.now().UTC()
	claims := TaskTokenClaims{
		Version: taskTokenVersion, KeyID: issuer.verifier.KeyID, TokenID: tokenID,
		WorkspaceID: binding.WorkspaceID, WorkID: binding.WorkID, StepID: binding.StepID,
		AttemptID: binding.AttemptID, PlacementID: binding.PlacementID, WorkerID: binding.WorkerID,
		Scopes: normalizedScopes, IssuedAtUnix: now.Unix(), ExpiresAtUnix: now.Add(ttl).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", TaskTokenClaims{}, fmt.Errorf("workerprotocol: encode task token: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := taskTokenPrefix + "." + encodedPayload
	signature := ed25519.Sign(issuer.privateKey, []byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), claims, nil
}

func (verifier *TaskTokenVerifier) Verify(token string, binding TaskTokenBinding, requiredScopes ...TaskScope) (TaskTokenClaims, error) {
	if verifier == nil || len(verifier.PublicKey) != ed25519.PublicKeySize || strings.TrimSpace(verifier.KeyID) == "" || verifier.now == nil {
		return TaskTokenClaims{}, ErrTaskTokenInvalid
	}
	if err := validateTaskTokenBinding(binding); err != nil {
		return TaskTokenClaims{}, err
	}
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 || parts[0] != taskTokenPrefix {
		return TaskTokenClaims{}, ErrTaskTokenInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return TaskTokenClaims{}, ErrTaskTokenInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) != ed25519.SignatureSize || !ed25519.Verify(verifier.PublicKey, []byte(parts[0]+"."+parts[1]), signature) {
		return TaskTokenClaims{}, ErrTaskTokenSignature
	}
	var claims TaskTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return TaskTokenClaims{}, ErrTaskTokenInvalid
	}
	if claims.Version != taskTokenVersion || claims.KeyID != verifier.KeyID || !validProtocolIdentifier(claims.TokenID) {
		return TaskTokenClaims{}, ErrTaskTokenInvalid
	}
	if !claimsMatchesBinding(claims, binding) {
		return TaskTokenClaims{}, ErrTaskTokenBinding
	}
	issuedAt := time.Unix(claims.IssuedAtUnix, 0).UTC()
	expiresAt := time.Unix(claims.ExpiresAtUnix, 0).UTC()
	now := verifier.now().UTC()
	if !expiresAt.After(issuedAt) || expiresAt.Sub(issuedAt) > verifier.maxTTL {
		return TaskTokenClaims{}, ErrTaskTokenTTL
	}
	if !expiresAt.After(now) {
		return TaskTokenClaims{}, ErrTaskTokenExpired
	}
	if issuedAt.After(now.Add(30 * time.Second)) {
		return TaskTokenClaims{}, ErrTaskTokenInvalid
	}
	normalizedScopes, err := normalizeTaskScopes(claims.Scopes)
	if err != nil || len(normalizedScopes) != len(claims.Scopes) {
		return TaskTokenClaims{}, ErrTaskTokenInvalid
	}
	granted := make(map[TaskScope]struct{}, len(normalizedScopes))
	for _, scope := range normalizedScopes {
		granted[scope] = struct{}{}
	}
	for _, required := range requiredScopes {
		if !validTaskScope(required) {
			return TaskTokenClaims{}, ErrTaskTokenScope
		}
		if _, ok := granted[required]; !ok {
			return TaskTokenClaims{}, ErrTaskTokenScope
		}
	}
	claims.Scopes = normalizedScopes
	return claims, nil
}

func validateTaskTokenBinding(binding TaskTokenBinding) error {
	if !validProtocolIdentifier(binding.WorkspaceID) || !validProtocolIdentifier(binding.WorkID) ||
		!validProtocolIdentifier(binding.StepID) || !validProtocolIdentifier(binding.AttemptID) ||
		!validProtocolIdentifier(binding.PlacementID) || !validProtocolIdentifier(binding.WorkerID) {
		return ErrTaskTokenBinding
	}
	return nil
}

func claimsMatchesBinding(claims TaskTokenClaims, binding TaskTokenBinding) bool {
	return claims.WorkspaceID == binding.WorkspaceID && claims.WorkID == binding.WorkID &&
		claims.StepID == binding.StepID && claims.AttemptID == binding.AttemptID &&
		claims.PlacementID == binding.PlacementID && claims.WorkerID == binding.WorkerID
}

func normalizeTaskScopes(scopes []TaskScope) ([]TaskScope, error) {
	if len(scopes) == 0 {
		return nil, ErrTaskTokenScope
	}
	seen := make(map[TaskScope]struct{}, len(scopes))
	for _, scope := range scopes {
		if !validTaskScope(scope) {
			return nil, ErrTaskTokenScope
		}
		seen[scope] = struct{}{}
	}
	normalized := make([]TaskScope, 0, len(seen))
	for scope := range seen {
		normalized = append(normalized, scope)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i] < normalized[j] })
	return normalized, nil
}

func validTaskScope(scope TaskScope) bool {
	switch scope {
	case TaskScopeExecute, TaskScopeStream, TaskScopeCheckpoint, TaskScopeCollect, TaskScopeDestroy:
		return true
	default:
		return false
	}
}

func taskTokenKeyID(publicKey ed25519.PublicKey) string {
	digest := sha256.Sum256(publicKey)
	return "ed25519:" + hex.EncodeToString(digest[:8])
}

func newTaskTokenID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
