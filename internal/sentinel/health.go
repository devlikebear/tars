package sentinel

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func probeHealth(client *http.Client, url string, timeout time.Duration) error {
	trimmed := strings.TrimSpace(url)
	if trimmed == "" {
		return nil
	}
	if client == nil {
		client = &http.Client{}
	}
	if timeout > 0 {
		client = &http.Client{Timeout: timeout}
	}
	resp, err := client.Get(trimmed)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
