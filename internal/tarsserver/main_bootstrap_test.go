package tarsserver

import (
	"testing"

	"github.com/devlikebear/tarsncase/internal/config"
)

func TestValidateAPIAuthSecurity(t *testing.T) {
	t.Run("skip when api server is disabled", func(t *testing.T) {
		err := validateAPIAuthSecurity(config.Config{APIAuthMode: "off"}, false)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("reject insecure mode without explicit opt-in", func(t *testing.T) {
		cases := []string{"off", "external-required"}
		for _, mode := range cases {
			t.Run(mode, func(t *testing.T) {
				err := validateAPIAuthSecurity(config.Config{
					APIAuthMode:               mode,
					APIAllowInsecureLocalAuth: false,
				}, true)
				if err == nil {
					t.Fatalf("expected error for insecure mode %q", mode)
				}
			})
		}
	})

	t.Run("allow insecure mode when explicitly approved", func(t *testing.T) {
		err := validateAPIAuthSecurity(config.Config{
			APIAuthMode:               "off",
			APIAllowInsecureLocalAuth: true,
		}, true)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("required mode always allowed", func(t *testing.T) {
		err := validateAPIAuthSecurity(config.Config{
			APIAuthMode:               "required",
			APIAllowInsecureLocalAuth: false,
		}, true)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})
}
