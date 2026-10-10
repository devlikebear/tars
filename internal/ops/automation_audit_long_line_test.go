package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An audit entry with a long detail (a command, a diff excerpt) must not
// make the whole audit log unreadable (#1231).
func TestListAutomationAuditReadsLongLinesAndSkipsBadOnes(t *testing.T) {
	mgr := NewManager(t.TempDir(), Options{HomeDir: t.TempDir()})
	long := strings.Repeat("d", 300*1024)
	if _, err := mgr.RecordAutomationAudit(AutomationAuditEntry{
		Actor: "test", Action: "long", SessionID: "s1", Result: "ok", Details: map[string]any{"preview": long},
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	f, err := os.OpenFile(mgr.auditPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n{not json\n" + `{"id":"other","actor":"test","action":"x","session_id":"s2","result":"ok"}`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(mgr.auditPath)); err != nil {
		t.Fatal(err)
	}

	all, err := mgr.ListAutomationAudit(AutomationAuditListOptions{})
	if err != nil || len(all) != 2 {
		t.Fatalf("list = %d entries, err=%v", len(all), err)
	}
	one, err := mgr.ListAutomationAudit(AutomationAuditListOptions{SessionID: "s1"})
	if err != nil || len(one) != 1 || one[0].Details["preview"] != long {
		t.Fatalf("filtered list = %d entries, err=%v", len(one), err)
	}
}
