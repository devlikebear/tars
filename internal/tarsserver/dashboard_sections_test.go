package tarsserver

import (
	"reflect"
	"testing"
)

func TestProjectDashboardSectionRegistryPreservesRefreshOrder(t *testing.T) {
	got := projectDashboardRefreshSectionIDs()
	want := []string{
		"autopilot-section",
		"board-section",
		"activity-section",
		"github-flow-section",
		"reports-section",
		"blockers-section",
		"decisions-section",
		"replans-section",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected refresh ids: got=%v want=%v", got, want)
	}
	if len(projectDashboardSectionRegistry) != len(want) {
		t.Fatalf("expected %d dashboard section specs, got %d", len(want), len(projectDashboardSectionRegistry))
	}
	if projectDashboardSectionRegistry[0].ID != "autopilot-section" {
		t.Fatalf("expected first dashboard section to stay autopilot, got %+v", projectDashboardSectionRegistry[0])
	}
	if projectDashboardSectionRegistry[len(projectDashboardSectionRegistry)-1].ID != "replans-section" {
		t.Fatalf("expected last dashboard section to stay replans, got %+v", projectDashboardSectionRegistry[len(projectDashboardSectionRegistry)-1])
	}
}
