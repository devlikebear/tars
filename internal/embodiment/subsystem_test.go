package embodiment

import (
	"context"
	"io"
	"runtime"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
)

func TestSubsystemNoop(t *testing.T) {
	t.Run("disabled config does not start", func(t *testing.T) {
		before := runtime.NumGoroutine()
		subsystem := New(config.EmbodimentConfig{}, zerolog.New(io.Discard))
		if err := subsystem.Start(context.Background()); err != nil {
			t.Fatalf("start disabled subsystem: %v", err)
		}
		subsystem.Stop()
		after := runtime.NumGoroutine()
		if after > before {
			t.Fatalf("disabled subsystem started goroutines: before=%d after=%d", before, after)
		}
		if status := subsystem.Status(); status.Enabled {
			t.Fatalf("disabled subsystem status = %+v", status)
		}
	})

	t.Run("enabled config without enabled providers stays inactive", func(t *testing.T) {
		subsystem := New(config.EmbodimentConfig{
			Enabled: true,
			Providers: []config.EmbodimentProviderConfig{{
				Name:         "host",
				Enabled:      false,
				Transport:    "webhook",
				Capabilities: []string{"hearing"},
			}},
		}, zerolog.New(io.Discard))
		if err := subsystem.Start(context.Background()); err != nil {
			t.Fatalf("start empty subsystem: %v", err)
		}
		defer subsystem.Stop()
		if status := subsystem.Status(); status.Enabled {
			t.Fatalf("empty subsystem status = %+v", status)
		}
	})
}
