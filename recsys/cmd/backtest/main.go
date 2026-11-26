package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"recsys/internal/config"
	"recsys/internal/eval"
	"recsys/internal/store/postgres"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "backtest:", err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getenv("RECSYS_DATABASE_URL") == "" {
		return fmt.Errorf("задайте RECSYS_DATABASE_URL для закрытой базы v4")
	}
	store, err := postgres.Open(config.Load().DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	sha, sales, err := store.EvaluationV4(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("dataset=v4 sha256=%s train_through=%s\n", sha, eval.TrainThrough)
	for _, r := range eval.Baselines(sales) {
		fmt.Printf("%s %s %d popularity (%.2f, %.1f) copurchase (%.2f, %.1f)\n",
			r.Cohort, r.Group, r.Clients, r.Popularity.Precision, r.Popularity.FillRate,
			r.CoPurchase.Precision, r.CoPurchase.FillRate)
	}
	return nil
}
