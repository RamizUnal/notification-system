// Command mockprovider runs a local stand-in for the external notification
// provider, with runtime-adjustable latency, failure rate and outages.
// See internal/mockprovider.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/RamizUnal/notification-system/internal/mockprovider"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "mockprovider:", err)
		os.Exit(1)
	}
}

func run() error {
	settings := mockprovider.Settings{
		FailureRate:  envFloat("MOCK_FAILURE_RATE", 0.03),
		MinLatencyMS: envInt("MOCK_MIN_LATENCY_MS", 30),
		MaxLatencyMS: envInt("MOCK_MAX_LATENCY_MS", 150),
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8090"
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	server := &http.Server{Addr: addr, Handler: mockprovider.New(settings).Handler(), ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	logger.Info("mock provider listening", slog.String("addr", addr), slog.Any("settings", settings))

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func envFloat(key string, fallback float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return fallback
}
