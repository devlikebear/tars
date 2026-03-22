package skillhub

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/config"
)

const (
	hubMCPDir             = "mcp-servers"
	defaultMCPManifest    = "tars.mcp.json"
	mcpDirPlaceholder     = "${MCP_DIR}"
	defaultMCPManifestVer = 1
)

type MCPManifest struct {
	SchemaVersion int              `json:"schema_version,omitempty"`
	Server        config.MCPServer `json:"server"`
}

func parseMCPManifest(data []byte, fallbackName string) (MCPManifest, error) {
	var manifest MCPManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return MCPManifest{}, fmt.Errorf("decode mcp manifest: %w", err)
	}
	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = defaultMCPManifestVer
	}
	if manifest.SchemaVersion != defaultMCPManifestVer {
		return MCPManifest{}, fmt.Errorf("unsupported mcp manifest schema_version %d", manifest.SchemaVersion)
	}
	manifest.Server = normalizeMCPServer(manifest.Server, fallbackName)
	if strings.TrimSpace(manifest.Server.Name) == "" {
		return MCPManifest{}, fmt.Errorf("mcp server name is required")
	}
	if strings.TrimSpace(manifest.Server.Command) == "" {
		return MCPManifest{}, fmt.Errorf("mcp server command is required")
	}
	return manifest, nil
}

func normalizeMCPServer(server config.MCPServer, fallbackName string) config.MCPServer {
	out := config.NormalizeMCPServer(server)
	if out.Name == "" {
		out.Name = strings.TrimSpace(fallbackName)
	}
	return out
}

func expandMCPServer(server config.MCPServer, mcpDir string) config.MCPServer {
	out := config.MCPServer{
		Name:          server.Name,
		Command:       expandMCPPlaceholder(server.Command, mcpDir),
		Transport:     server.Transport,
		URL:           expandMCPPlaceholder(server.URL, mcpDir),
		AuthMode:      server.AuthMode,
		AuthTokenEnv:  server.AuthTokenEnv,
		OAuthProvider: server.OAuthProvider,
		Source:        server.Source,
		Args:          make([]string, 0, len(server.Args)),
	}
	for _, arg := range server.Args {
		out.Args = append(out.Args, expandMCPPlaceholder(arg, mcpDir))
	}
	if len(server.Env) > 0 {
		out.Env = make(map[string]string, len(server.Env))
		for key, value := range server.Env {
			out.Env[key] = expandMCPPlaceholder(value, mcpDir)
		}
	}
	if len(server.Headers) > 0 {
		out.Headers = make(map[string]string, len(server.Headers))
		for key, value := range server.Headers {
			out.Headers[key] = expandMCPPlaceholder(value, mcpDir)
		}
	}
	return out
}

func expandMCPPlaceholder(value string, mcpDir string) string {
	return strings.ReplaceAll(value, mcpDirPlaceholder, mcpDir)
}

func cleanRegistryRelativePath(relPath string) (string, error) {
	normalized := strings.TrimSpace(strings.ReplaceAll(relPath, "\\", "/"))
	if normalized == "" {
		return "", fmt.Errorf("path is required")
	}
	cleaned := path.Clean(normalized)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("path traversal is not allowed")
	}
	return cleaned, nil
}

func verifyFileChecksum(content []byte, expected string) error {
	expected = strings.ToLower(strings.TrimSpace(expected))
	if expected == "" {
		return fmt.Errorf("sha256 checksum is required")
	}
	sum := sha256.Sum256(content)
	actual := hex.EncodeToString(sum[:])
	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected %s got %s", expected, actual)
	}
	return nil
}

func materializePackageFiles(dstDir string, files map[string][]byte) error {
	parentDir := filepath.Dir(dstDir)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("create package parent dir: %w", err)
	}
	tmpDir := dstDir + ".tmp"
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return fmt.Errorf("create temp package dir: %w", err)
	}
	for relPath, content := range files {
		dst := filepath.Join(tmpDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			_ = os.RemoveAll(tmpDir)
			return fmt.Errorf("create package file dir: %w", err)
		}
		if err := os.WriteFile(dst, content, 0o644); err != nil {
			_ = os.RemoveAll(tmpDir)
			return fmt.Errorf("write package file: %w", err)
		}
	}
	_ = os.RemoveAll(dstDir)
	if err := os.Rename(tmpDir, dstDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("activate package dir: %w", err)
	}
	return nil
}

func LoadInstalledMCPServers(workspaceDir string) ([]config.MCPServer, []string) {
	inst := NewInstaller(workspaceDir)
	db, err := inst.loadDB()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []string{fmt.Sprintf("%s: %v", inst.dbPath(), err)}
	}

	servers := make([]config.MCPServer, 0, len(db.MCPs))
	diagnostics := make([]string, 0)
	for _, installed := range db.MCPs {
		manifestPath := filepath.Join(installed.Dir, filepath.FromSlash(installed.manifestPath()))
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			diagnostics = append(diagnostics, fmt.Sprintf("%s: %v", manifestPath, err))
			continue
		}
		manifest, err := parseMCPManifest(data, installed.Name)
		if err != nil {
			diagnostics = append(diagnostics, fmt.Sprintf("%s: %v", manifestPath, err))
			continue
		}
		servers = append(servers, expandMCPServer(manifest.Server, installed.Dir))
	}
	return servers, diagnostics
}

func (installed InstalledMCP) manifestPath() string {
	if strings.TrimSpace(installed.Manifest) == "" {
		return defaultMCPManifest
	}
	return installed.Manifest
}
