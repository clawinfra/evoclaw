// Command init-turso initializes the Turso database schema for EvoClaw cloud sync
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/clawinfra/evoclaw/internal/cloudsync"
)

func main() {
	dbURL := flag.String("db", "", "Turso database URL")
	authToken := flag.String("token", "", "Turso auth token")
	flag.Parse()

	if *dbURL == "" {
		*dbURL = os.Getenv("TURSO_DATABASE_URL")
	}
	if *authToken == "" {
		*authToken = os.Getenv("TURSO_AUTH_TOKEN")
	}

	if *dbURL == "" || *authToken == "" {
		fmt.Println("Usage: init-turso -db <database-url> -token <auth-token>")
		fmt.Println("Or set TURSO_DATABASE_URL and TURSO_AUTH_TOKEN environment variables")
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	client := cloudsync.NewClient(*dbURL, *authToken, logger)

	ctx := context.Background()
	log.Println("Creating Turso database schema...")

	if err := client.InitSchema(ctx); err != nil {
		log.Fatalf("Failed to initialize schema: %v", err)
	}

	log.Println("✓ Schema created successfully!")
	log.Println("✓ Tables: agents, core_memory, warm_memory, evolution_log, action_log, devices, sync_state")
}
