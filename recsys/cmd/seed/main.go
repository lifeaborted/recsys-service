package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"recsys/internal/config"
	"recsys/internal/store/postgres"
)

func main() {
	migrateFirst := flag.Bool("migrate", false, "apply migrations before seeding demo fixtures")
	migrationsDir := flag.String("migrations-dir", "", "override migrations directory")
	allowOnV4 := flag.Bool("allow-on-v4", false, "allow seeding onto a database containing dataset v4")
	flag.Parse()

	if err := run(*migrateFirst, *migrationsDir, *allowOnV4); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run(migrateFirst bool, migrationsDirOverride string, allowOnV4 bool) error {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store, err := postgres.OpenContext(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	migDir := cfg.MigrationsDir
	if migrationsDirOverride != "" {
		migDir = migrationsDirOverride
	}
	if migrateFirst {
		ver, err := store.Migrate(ctx, migDir)
		if err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		fmt.Fprintf(os.Stdout, "migrations ready (version=%s)\n", ver)
	}

	if err := store.SeedDemo(ctx, postgres.SeedOptions{
		B2BLogin:    os.Getenv("DEMO_B2B_LOGIN"),
		B2BPassword: os.Getenv("DEMO_B2B_PASSWORD"),
		B2CLogin:    os.Getenv("DEMO_B2C_LOGIN"),
		B2CPassword: os.Getenv("DEMO_B2C_PASSWORD"),
		BcryptCost:  cfg.BcryptCost,
		AllowOnV4:   allowOnV4,
	}); err != nil {
		return err
	}

	fmt.Fprintln(os.Stdout, "synthetic demo fixtures seeded (version=demo-v1)")
	return nil
}
