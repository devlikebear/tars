package tarsapp

import (
	"os"
	"strings"

	"github.com/devlikebear/tarsncase/internal/secrets"
	"github.com/joho/godotenv"
)

// loadRuntimeEnvFiles applies environment values with precedence:
// OS env > .env.secret > .env.
func loadRuntimeEnvFiles(envPath, secretPath string) {
	load := func(path string, forcedSecret bool) {
		cleanPath := strings.TrimSpace(path)
		if cleanPath == "" {
			return
		}
		values, err := godotenv.Read(cleanPath)
		if err != nil {
			return
		}
		for key, value := range values {
			if _, exists := os.LookupEnv(key); !exists {
				_ = os.Setenv(key, value)
			}
		}
		if forcedSecret {
			secrets.RegisterMapForced(values)
			return
		}
		secrets.RegisterMapNamed(values)
	}

	load(secretPath, true)
	load(envPath, false)
	secrets.RegisterOSEnv()
}
