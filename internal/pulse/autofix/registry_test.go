package autofix

import (
	"context"
	"errors"
	"testing"
)

type stubFixer struct {
	name   string
	result Result
	err    error
}

func (s *stubFixer) Name() string { return s.name }
func (s *stubFixer) Run(ctx context.Context) (Result, error) {
	return s.result, s.err
}

func TestRegistry_RegisterAndHas(t *testing.T) {
	r := NewRegistry()
	if r.Has("x") {
		t.Error("Has should be false for unregistered")
	}
	r.Register(&stubFixer{name: "x"})
	if !r.Has("x") {
		t.Error("Has should be true after Register")
	}
}

func TestRegistry_RegisterNilIsNoop(t *testing.T) {
	r := NewRegistry()
	r.Register(nil)
	if len(r.Names()) != 0 {
		t.Errorf("Names() = %v, want empty", r.Names())
	}
}

func TestRegistry_NamesSorted(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubFixer{name: "zebra"})
	r.Register(&stubFixer{name: "apple"})
	r.Register(&stubFixer{name: "mango"})
	got := r.Names()
	want := []string{"apple", "mango", "zebra"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Names[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRegistry_RunSuccess(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubFixer{name: "cleanup", result: Result{Name: "cleanup", Changed: true, Summary: "done"}})
	got, err := r.Run(context.Background(), "cleanup")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Changed || got.Summary != "done" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestRegistry_RunUnknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Run(context.Background(), "ghost")
	var ue ErrUnknown
	if !errors.As(err, &ue) {
		t.Fatalf("want ErrUnknown, got %T %v", err, err)
	}
	if ue.Name != "ghost" {
		t.Errorf("ErrUnknown.Name = %q", ue.Name)
	}
}

func TestRegistry_RunPropagatesError(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubFixer{name: "boom", err: errors.New("kaboom")})
	_, err := r.Run(context.Background(), "boom")
	if err == nil || err.Error() != "kaboom" {
		t.Errorf("want kaboom, got %v", err)
	}
}

func TestRegistry_AllowedIntersection(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubFixer{name: "compress_old_logs"})
	r.Register(&stubFixer{name: "cleanup_stale_tmp"})

	cases := []struct {
		name       string
		configured []string
		want       []string
	}{
		{"empty config", nil, nil},
		{"all registered", []string{"compress_old_logs", "cleanup_stale_tmp"}, []string{"cleanup_stale_tmp", "compress_old_logs"}},
		{"config has extras", []string{"compress_old_logs", "drop_all_tables"}, []string{"compress_old_logs"}},
		{"dedupe", []string{"compress_old_logs", "compress_old_logs"}, []string{"compress_old_logs"}},
		{"none match", []string{"foo", "bar"}, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.AllowedIntersection(c.configured)
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestRegistry_NilReceiverSafe(t *testing.T) {
	var r *Registry
	if r.Has("x") {
		t.Error("nil Has should be false")
	}
	if r.Names() != nil {
		t.Error("nil Names should be nil")
	}
	_, err := r.Run(context.Background(), "x")
	if err == nil {
		t.Error("nil Run should error")
	}
}
