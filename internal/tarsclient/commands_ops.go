package tarsclient

import (
	"fmt"
	"strings"
)

func cmdOps(c commandContext) (bool, string, error) {
	if c.fields[0] != "/ops" {
		return false, c.session, nil
	}
	if len(c.fields) < 2 {
		return true, c.session, fmt.Errorf("usage: /ops {status|cleanup plan}")
	}
	sub := strings.ToLower(strings.TrimSpace(c.fields[1]))
	switch sub {
	case "status":
		status, err := c.runtime.opsStatus(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > ops status disk_used=%.2f%% process_count=%d free_bytes=%d\n", status.DiskUsedPercent, status.ProcessCount, status.DiskFreeBytes)
		return true, c.session, nil
	case "cleanup":
		if len(c.fields) < 3 || strings.ToLower(strings.TrimSpace(c.fields[2])) != "plan" {
			return true, c.session, fmt.Errorf("usage: /ops cleanup plan")
		}
		plan, err := c.runtime.opsCleanupPlan(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > cleanup plan approval_id=%s candidates=%d total_bytes=%d\n", plan.ApprovalID, len(plan.Candidates), plan.TotalBytes)
		return true, c.session, nil
	default:
		return true, c.session, fmt.Errorf("usage: /ops {status|cleanup plan}")
	}
}
