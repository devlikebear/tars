package tarsserver

import (
	"errors"
	"testing"
)

func TestIsRecoverableLLMInitError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "plain error", err: errors.New("boom"), want: false},
		{name: "validate_config", err: &runtimeDepsError{stage: "validate_config", err: errors.New("x")}, want: false},
		{name: "ensure_workspace", err: &runtimeDepsError{stage: "ensure_workspace", err: errors.New("x")}, want: false},
		{name: "init_usage", err: &runtimeDepsError{stage: "init_usage", err: errors.New("x")}, want: false},
		{name: "init_llm", err: &runtimeDepsError{stage: "init_llm", err: errors.New("x")}, want: true},
		{name: "init_memory_backend", err: &runtimeDepsError{stage: "init_memory_backend", err: errors.New("x")}, want: true},
		{name: "init_semantic_memory", err: &runtimeDepsError{stage: "init_semantic_memory", err: errors.New("x")}, want: true},
		{name: "wrapped init_llm", err: wrap(&runtimeDepsError{stage: "init_llm", err: errors.New("x")}), want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRecoverableLLMInitError(tc.err); got != tc.want {
				t.Fatalf("isRecoverableLLMInitError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

type wrappedErr struct {
	inner error
}

func (w *wrappedErr) Error() string { return w.inner.Error() }
func (w *wrappedErr) Unwrap() error { return w.inner }

func wrap(err error) error { return &wrappedErr{inner: err} }
