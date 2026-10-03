package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
		return config.Config{ToolConfig: config.ToolConfig{ToolsComputerUseEnabled: true}, Jev: config.JevConfig{BaseURL: base}}
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
