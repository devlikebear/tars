package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

func TestCheckDoctorInitiative(t *testing.T) {
	only := func(t *testing.T, r doctorReport) doctorCheck {
		t.Helper()
		if len(r.checks) != 1 || r.checks[0].name != "initiative" {
			t.Fatalf("checks = %+v", r.checks)
		}
		return r.checks[0]
	}

	var disabled doctorReport
	checkDoctorInitiative(&disabled, config.Config{})
	if c := only(t, disabled); c.status != "ok" || c.detail != "disabled" {
		t.Fatalf("disabled = %+v", c)
	}

	var noBackend doctorReport
	checkDoctorInitiative(&noBackend, config.Config{Initiative: config.InitiativeConfig{Enabled: true}})
	if c := only(t, noBackend); c.status != "warn" || !strings.Contains(c.detail, "jev.base_url") {
		t.Fatalf("no backend = %+v", c)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"answers":{"ok":{"noul":0.9}}}`))
	}))
	defer srv.Close()
	var ok doctorReport
	checkDoctorInitiative(&ok, config.Config{Initiative: config.InitiativeConfig{Enabled: true}, Jev: config.JevConfig{BaseURL: srv.URL}})
	if c := only(t, ok); c.status != "ok" || !strings.Contains(c.detail, "shadow") || !strings.Contains(c.detail, "loopback") {
		t.Fatalf("reachable = %+v", c)
	}

	srv.Close()
	var down doctorReport
	checkDoctorInitiative(&down, config.Config{Initiative: config.InitiativeConfig{Enabled: true}, Jev: config.JevConfig{BaseURL: srv.URL}})
	if c := only(t, down); c.status != "warn" || !strings.Contains(c.detail, "unreachable") {
		t.Fatalf("down = %+v", c)
	}
}
