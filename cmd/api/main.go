// Command api serves the notification REST API, the WebSocket event stream,
// health checks, metrics and the OpenAPI documentation.
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
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadAPI()
	if err != nil {
		return err
	}
	logger, err := app.NewLogger(cfg.Common)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return app.RunAPI(ctx, cfg, logger)
}
