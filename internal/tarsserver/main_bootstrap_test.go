package tarsserver

import (
	"testing"

	"github.com/devlikebear/tars/internal/config"
)

func TestValidateAPIAuthSecurity(t *testing.T) {
	t.Run("reject insecure mode without explicit opt-in", func(t *testing.T) {
		cases := []string{"off", "external-required"}
		for _, mode := range cases {
			t.Run(mode, func(t *testing.T) {
				err := validateAPIAuthSecurity(config.Config{
					APIConfig: config.APIConfig{
						APIAuthMode:               mode,
						APIAllowInsecureLocalAuth: false,
					},
				})
				if err == nil {
					t.Fatalf("expected error for insecure mode %q", mode)
				}
			})
		}
	})

	t.Run("allow insecure mode when explicitly approved", func(t *testing.T) {
		err := validateAPIAuthSecurity(config.Config{
			APIConfig: config.APIConfig{
				APIAuthMode:               "off",
				APIAllowInsecureLocalAuth: true,
			},
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("required mode always allowed", func(t *testing.T) {
		err := validateAPIAuthSecurity(config.Config{
			APIConfig: config.APIConfig{
				APIAuthMode:               "required",
				APIAllowInsecureLocalAuth: false,
			},
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})
}
