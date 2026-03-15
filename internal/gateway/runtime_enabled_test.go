package gateway

import (
	"context"
	"testing"
)

func TestRuntimeRequireEnabled(t *testing.T) {
	t.Parallel()

	disabledRuntime := NewRuntime(RuntimeOptions{})
	enabledRuntime := NewRuntime(RuntimeOptions{Enabled: true})
	t.Cleanup(func() {
		if disabledRuntime != nil {
			_ = disabledRuntime.Close(context.Background())
		}
		if enabledRuntime != nil {
			_ = enabledRuntime.Close(context.Background())
		}
	})

	tests := []struct {
		name    string
		runtime *Runtime
		wantErr bool
	}{
		{name: "nil runtime", runtime: nil, wantErr: true},
		{name: "disabled runtime", runtime: disabledRuntime, wantErr: true},
		{name: "enabled runtime", runtime: enabledRuntime, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.runtime.requireEnabled()
			if tt.wantErr && err == nil {
				t.Fatal("expected disabled runtime error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tt.wantErr && err.Error() != "gateway runtime is disabled" {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
