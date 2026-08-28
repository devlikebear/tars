package llm

import (
	"fmt"
	"strings"
)

func requireConfiguredValue(provider, field, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%s %s is required", provider, field)
	}
	return trimmed, nil
}
