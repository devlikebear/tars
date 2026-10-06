package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

func TestCheckDoctorComputerUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"answers":{"ok":{"noul":0.9}}}`))
	}))
	defer srv.Close()

	found := func(string) (string, error) { return "/opt/bin/cua-driver", nil }
	missing := func(string) (string, error) { return "", errors.New("not found") }
	up := func(context.Context, string) error { return nil }
	down := func(context.Context, string) error { return errors.New("daemon not running") }
	enabled := func(base string) config.Config {
		return config.Config{ToolConfig: config.ToolConfig{ToolsComputerUseEnabled: true, ToolsComputerUseBackend: "jev"}, Jev: config.JevConfig{BaseURL: base}}
	}

	cases := []struct {
		name       string
		cfg        config.Config
		probe      doctorComputerUseProbe
		wantStatus []string
		wantDetail []string
	}{
		{"disabled", config.Config{}, doctorComputerUseProbe{missing, down}, []string{"ok"}, []string{"disabled"}},
		{"ready", enabled(srv.URL), doctorComputerUseProbe{found, up}, []string{"ok"}, []string{"loopback"}},
		{"no binary", enabled(srv.URL), doctorComputerUseProbe{missing, up}, []string{"warn"}, []string{"binary not found"}},
		{"daemon down", enabled(srv.URL), doctorComputerUseProbe{found, down}, []string{"warn"}, []string{"not answering"}},
		{"no system one", enabled(""), doctorComputerUseProbe{found, up}, []string{"warn"}, []string{"jev.base_url"}},
		{"nothing works", enabled(""), doctorComputerUseProbe{missing, up}, []string{"warn", "warn"}, []string{"binary not found", "jev.base_url"}},
		{"system one down", enabled("http://127.0.0.1:1"), doctorComputerUseProbe{found, up}, []string{"warn"}, []string{"unreachable"}},
	}
	for _, tc := range cases {
		var report doctorReport
		checkDoctorComputerUseWith(&report, tc.cfg, tc.probe)
		if len(report.checks) != len(tc.wantStatus) {
			t.Errorf("%s: checks = %+v", tc.name, report.checks)
			continue
		}
		for i, c := range report.checks {
			if c.name != "computer_use" || c.status != tc.wantStatus[i] || !strings.Contains(c.detail, tc.wantDetail[i]) {
				t.Errorf("%s: check %d = %+v, want %s containing %q", tc.name, i, c, tc.wantStatus[i], tc.wantDetail[i])
			}
		}
	}
}

// The default probe must not need a daemon to say the binary is missing.
func TestCheckDoctorComputerUse_DefaultProbeReportsMissingBinary(t *testing.T) {
	t.Setenv("CUA_DRIVER_PATH", "")
	cfg := config.Config{ToolConfig: config.ToolConfig{ToolsComputerUseEnabled: true, ToolsComputerUseCuaDriverPath: t.TempDir() + "/no-such-cua-driver"}}
	var report doctorReport
	checkDoctorComputerUse(&report, cfg)
	if len(report.checks) == 0 || report.checks[0].status != "warn" || !strings.Contains(report.checks[0].detail, "binary not found") {
		t.Fatalf("checks = %+v", report.checks)
	}
}

// TestCheckDoctorComputerUse_NeverExecsRealDriver proves TestMain's hermetic
// probe override (network_guard_test.go) is actually wired in: it puts a
// working "cua-driver" stub on PATH that records a marker file the instant
// it runs, points CUA_DRIVER_PATH and the configured path at nothing so the
// default probe's PATH fallback is what would find the stub, and asserts the
// marker is never written. If checkDoctorComputerUse ever went back to
// calling defaultDoctorComputerUseProbe() directly, the stub would run and
// this test would fail.
func TestCheckDoctorComputerUse_NeverExecsRealDriver(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell stub; Windows cua-driver lookup is covered in internal/computeruse")
	}
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	stub := filepath.Join(binDir, "cua-driver")
	script := "#!/bin/sh\necho ran >> " + marker + "\nexit 0\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatalf("write cua-driver stub: %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("CUA_DRIVER_PATH", "")

	cfg := config.Config{ToolConfig: config.ToolConfig{ToolsComputerUseEnabled: true}}
	var report doctorReport
	checkDoctorComputerUse(&report, cfg)

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("cua-driver stub was executed (marker stat err = %v); test hermeticity is broken", err)
	}
	if len(report.checks) == 0 || report.checks[0].status != "warn" || !strings.Contains(report.checks[0].detail, "binary not found") {
		t.Fatalf("checks = %+v, want warn containing %q", report.checks, "binary not found")
	}
}

func TestCheckDoctorComputerUseLLMWithoutJev(t *testing.T) {
	cfg := config.Default()
	cfg.LLMProviders = map[string]config.LLMProviderSettings{"test": {Kind: "openai", APIKey: "test-only"}}
	cfg.LLMTiers = map[string]config.LLMTierBinding{"light": {Provider: "test", Model: "light-test"}}
	probe := doctorComputerUseProbe{
		findDriver: func(string) (string, error) { return "/opt/bin/cua-driver", nil },
		pingDriver: func(context.Context, string) error { return nil },
	}
	var report doctorReport
	checkDoctorComputerUseWith(&report, cfg, probe)
	if len(report.checks) != 1 || report.checks[0].status != "ok" || !strings.Contains(report.checks[0].detail, "light-test") {
		t.Fatalf("checks: %+v", report.checks)
	}
}
