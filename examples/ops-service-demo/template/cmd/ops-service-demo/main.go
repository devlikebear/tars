package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"example.com/ops-service-demo/internal/app"
)

func main() {
	logger := log.New(os.Stderr, "", 0)
	port := firstNonEmpty(strings.TrimSpace(os.Getenv("PORT")), "18080")
	interval := parseInterval(strings.TrimSpace(os.Getenv("SYNTHETIC_CHECK_INTERVAL")))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	service := app.New(os.Stdout, time.Now)
	service.RunSyntheticCheck()

	go runSyntheticChecks(ctx, service, interval)

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           service.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Printf("ops-service-demo listening on :%s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("listen and serve: %v", err)
	}
}

func runSyntheticChecks(ctx context.Context, service *app.Service, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			service.RunSyntheticCheck()
		}
	}
}

func parseInterval(raw string) time.Duration {
	if raw == "" {
		return 5 * time.Second
	}
	interval, err := time.ParseDuration(raw)
	if err != nil || interval <= 0 {
		return 5 * time.Second
	}
	return interval
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
