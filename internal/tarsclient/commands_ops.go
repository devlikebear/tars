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

func cmdApprove(c commandContext) (bool, string, error) {
	if c.fields[0] != "/approve" {
		return false, c.session, nil
	}
	if len(c.fields) < 2 {
		return true, c.session, fmt.Errorf("usage: /approve {list|run|reject}")
	}
	sub := strings.ToLower(strings.TrimSpace(c.fields[1]))
	switch sub {
	case "list":
		items, err := c.runtime.listApprovals(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		if len(items) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no approvals)")
			return true, c.session, nil
		}
		fmt.Fprintln(c.stdout, "SYSTEM > approvals")
		for _, item := range items {
			fmt.Fprintf(c.stdout, "- %s type=%s status=%s candidates=%d\n", item.ID, item.Type, item.Status, len(item.Plan.Candidates))
		}
		return true, c.session, nil
	case "run":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /approve run {id}")
		}
		approvalID := strings.TrimSpace(c.fields[2])
		if err := c.runtime.approveCleanup(c.ctx, approvalID); err != nil {
			return true, c.session, err
		}
		result, err := c.runtime.opsCleanupApply(c.ctx, approvalID)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > cleanup applied approval_id=%s deleted=%d bytes=%d\n", result.ApprovalID, result.DeletedCount, result.DeletedBytes)
		return true, c.session, nil
	case "reject":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /approve reject {id}")
		}
		approvalID := strings.TrimSpace(c.fields[2])
		if err := c.runtime.rejectCleanup(c.ctx, approvalID); err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > approval rejected %s\n", approvalID)
		return true, c.session, nil
	default:
		return true, c.session, fmt.Errorf("usage: /approve {list|run|reject}")
	}
}
