package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/mcp"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/rs/zerolog"
)

func TestExtensionsAPI_HealthReportsSkillsAndMCPFailures(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	skillPath := filepath.Join(workspaceDir, "skills", "echo-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte("# Echo skill\n"), 0o600); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	mcpDir := filepath.Join(workspaceDir, "mcp-servers", "echo")
	if err := os.MkdirAll(mcpDir, 0o755); err != nil {
		t.Fatalf("mkdir mcp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mcpDir, "tars.mcp.json"), []byte(`{"schema_version":1,"server":{"name":"echo","command":"python3","args":["${MCP_DIR}/server.py"],"transport":"stdio"}}`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mcpDir, "requirements.txt"), []byte("mcp>=1.0\n"), 0o600); err != nil {
		t.Fatalf("write requirements: %v", err)
	}

	provider := &mockExtensionsProvider{
		snapshot: extensions.Snapshot{
			Skills: []skill.Definition{
				{
					Name:         "echo-skill",
					Description:  "Echo helper",
					FilePath:     skillPath,
					RequiresBins: []string{"definitely-not-a-real-tars-bin"},
				},
			},
			MCPServers: []config.MCPServer{
				{Name: "echo", Command: "python3", Args: []string{filepath.Join(mcpDir, "server.py")}, Transport: "stdio", Source: "workspace"},
			},
		},
	}
	mcpProvider := &mockMCPProvider{
		servers: []mcp.ServerStatus{
			{Name: "echo", Command: "python3", Transport: "stdio", Source: "workspace", Connected: false, Error: "No module named 'mcp'"},
		},
	}
	handler := newExtensionsAPIHandlerWithHealth(provider, zerolog.New(io.Discard), nil, nil, extensionHealthOptions{
		MCPProvider:  mcpProvider,
		WorkspaceDir: workspaceDir,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/runtime/extensions/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload extensionHealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode health payload: %v", err)
	}
	if len(payload.Skills) != 1 || payload.Skills[0].Name != "echo-skill" || payload.Skills[0].Status != extensionHealthFail {
		t.Fatalf("expected failing skill health, got %+v", payload.Skills)
	}
	if !hasHealthCheck(payload.Skills[0].Checks, "required_bins", extensionHealthFail) {
		t.Fatalf("expected failing required_bins check, got %+v", payload.Skills[0].Checks)
	}
	if len(payload.MCPServers) != 1 || payload.MCPServers[0].Name != "echo" || payload.MCPServers[0].Status != extensionHealthFail {
		t.Fatalf("expected failing mcp health, got %+v", payload.MCPServers)
	}
	if !payload.MCPServers[0].Repairable {
		t.Fatalf("expected echo mcp to be repairable, got %+v", payload.MCPServers[0])
	}
	if !hasHealthCheck(payload.MCPServers[0].Checks, "connection", extensionHealthFail) {
		t.Fatalf("expected failing connection check, got %+v", payload.MCPServers[0].Checks)
	}
}

func TestExtensionsAPI_RepairMCPPythonRequirements(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	mcpDir := filepath.Join(workspaceDir, "mcp-servers", "echo")
	if err := os.MkdirAll(mcpDir, 0o755); err != nil {
		t.Fatalf("mkdir mcp: %v", err)
	}
	manifestPath := filepath.Join(mcpDir, "tars.mcp.json")
	if err := os.WriteFile(manifestPath, []byte(`{
  "schema_version": 1,
  "server": {
    "name": "echo",
    "command": "python3",
    "args": ["${MCP_DIR}/server.py"],
    "transport": "stdio"
  }
}`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mcpDir, "requirements.txt"), []byte("mcp>=1.0\n"), 0o600); err != nil {
		t.Fatalf("write requirements: %v", err)
	}

	provider := &mockExtensionsProvider{
		snapshot: extensions.Snapshot{
			MCPServers: []config.MCPServer{
				{Name: "echo", Command: "python3", Args: []string{filepath.Join(mcpDir, "server.py")}, Transport: "stdio", Source: "workspace"},
			},
		},
	}

	oldRunner := runExtensionRepairCommand
	var gotDir string
	var gotName string
	var gotArgs []string
	runExtensionRepairCommand = func(_ context.Context, dir, name string, args ...string) (extensionRepairCommandResult, error) {
		gotDir = dir
		gotName = name
		gotArgs = append([]string(nil), args...)
		return extensionRepairCommandResult{Stdout: "installed", ExitCode: 0}, nil
	}
	t.Cleanup(func() { runExtensionRepairCommand = oldRunner })

	handler := newExtensionsAPIHandlerWithHealth(provider, zerolog.New(io.Discard), nil, nil, extensionHealthOptions{
		WorkspaceDir: workspaceDir,
	})
	reqBody := bytes.NewBufferString(`{"kind":"mcp","name":"echo"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/runtime/extensions/repair", reqBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if gotDir != mcpDir || gotName != "python3" {
		t.Fatalf("unexpected repair command dir/name: dir=%q name=%q", gotDir, gotName)
	}
	wantArgs := []string{"-m", "pip", "install", "-r", "requirements.txt", "--target", ".python", "--disable-pip-version-check"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("unexpected repair args: got %+v want %+v", gotArgs, wantArgs)
	}
	if provider.reloadCount != 1 {
		t.Fatalf("expected provider reload after repair, got %d", provider.reloadCount)
	}
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(manifest), `"PYTHONPATH": "${MCP_DIR}/.python"`) {
		t.Fatalf("expected manifest PYTHONPATH patch, got %s", string(manifest))
	}
}

func hasHealthCheck(checks []extensionHealthCheck, name string, status extensionHealthStatus) bool {
	for _, check := range checks {
		if check.Name == name && check.Status == status {
			return true
		}
	}
	return false
}
