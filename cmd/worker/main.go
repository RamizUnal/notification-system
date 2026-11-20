// Command worker claims due notifications from the queue and delivers them
// through the external provider, honouring rate limits, priorities, retries
// and the circuit breaker.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/RamizUnal/notification-system/internal/app"
	"github.com/RamizUnal/notification-system/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "worker:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadWorker()
	if err != nil {
		return err
	}
	logger, err := app.NewLogger(cfg.Common)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return app.RunWorker(ctx, cfg, logger)
}
