package reflection

import (
	"context"
	"time"
)

// Job is the interface each reflection task implements. Jobs are
// registered in order on the Runtime and executed sequentially during a
// reflection run. A failing job does not stop subsequent jobs from
// running — each job is expected to be independent so that partial
// success is still meaningful.
type Job interface {
	// Name returns the job's stable identifier, used in logs, state
	// snapshots, and HTTP responses.
	Name() string

	// Run performs the job. It must not panic; any error becomes a
	// JobResult with Success=false. The context is the run context
	// derived from the runtime's parent ctx; jobs should respect
	// ctx.Done() for cooperative cancellation.
	Run(ctx context.Context) (JobResult, error)
}

// runJob executes a single Job, catching panics, timing it, and
// normalizing the output into a JobResult. The returned JobResult
// always has its Name and Duration set.
func runJob(ctx context.Context, job Job) JobResult {
	start := time.Now()
	name := "unknown"
	if job != nil {
		name = job.Name()
	}

	var result JobResult
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = panicError{value: r}
			}
		}()
		if job == nil {
			err = errNilJob
			return
		}
		result, err = job.Run(ctx)
	}()

	if result.Name == "" {
		result.Name = name
	}
	result.Duration = time.Since(start)
	if err != nil {
		result.Success = false
		if result.Err == "" {
			result.Err = err.Error()
		}
		return result
	}
	if result.Err != "" {
		result.Success = false
		return result
	}
	result.Success = true
	return result
}

type panicError struct{ value any }

func (p panicError) Error() string {
	return "reflection job panicked"
}

type sentinelError string

func (e sentinelError) Error() string { return string(e) }

const errNilJob sentinelError = "reflection job is nil"
