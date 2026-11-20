// Command migrate applies (or reverts) the versioned database migrations
// embedded in the binary.
//
//	migrate        apply every pending migration (same as "migrate up")
//	migrate down   revert every migration (development only)
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/RamizUnal/notification-system/internal/storage/postgres"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}

	command := "up"
	if len(args) > 0 {
		command = args[0]
	}
	switch command {
	case "up":
		version, err := postgres.MigrateUp(dsn)
		if err != nil {
			return err
		}
		fmt.Printf("database schema is at version %d\n", version)
		return nil
	case "down":
		if err := postgres.MigrateDown(dsn); err != nil {
			return err
		}
		fmt.Println("all migrations reverted")
		return nil
	default:
		return fmt.Errorf("unknown command %q (use up or down)", command)
	}
}
