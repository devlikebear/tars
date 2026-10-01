package focuspipeline

import (
	"reflect"
	"testing"

	"github.com/devlikebear/tars/internal/session"
)

func TestSessionTasks(t *testing.T) {
	st := SessionTasks(*testPlan(StageOrder...), t0)
	if st.Plan == nil || st.Plan.Goal != "g" || st.Plan.Status != session.PlanStatusExecuting {
		t.Fatalf("plan = %+v", st.Plan)
	}
	if st.Contract == nil || st.Contract.Status != session.ContractStatusApproved {
		t.Fatalf("contract = %+v", st.Contract)
	}
	if want := []string{"make test", "make console-e2e"}; !reflect.DeepEqual(st.Contract.VerificationCommands, want) {
		t.Fatalf("verify = %v", st.Contract.VerificationCommands)
	}
	if want := []string{"t1: d1", "t2: d2"}; !reflect.DeepEqual(st.Contract.DoneCriteria, want) {
		t.Fatalf("done = %v", st.Contract.DoneCriteria)
	}
	if len(st.Tasks) != 2 || st.Tasks[0].ID != "task-1" || st.Tasks[1].Title != "t2" || st.Tasks[0].Status != "pending" {
		t.Fatalf("tasks = %+v", st.Tasks)
	}
}
