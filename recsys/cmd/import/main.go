// Команда импорта и проверки наборов данных (clients.csv + sales.csv) и закрытого релиза v4.
//
//	go run ./cmd/import -clients testdata/smoke/clients.csv -sales testdata/smoke/sales.csv -version demo -dry-run
//	go run ./cmd/import -verify-v4
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"recsys/internal/contract"
	"recsys/internal/data"
	"recsys/internal/eval"
	"recsys/internal/store/postgres"
)

type dryRunWriter struct {
	version string
	batches int
}

func (w *dryRunWriter) WriteDataset(ctx context.Context, version string, batches []contract.DatasetBatch) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.version = version
	w.batches = len(batches)
	return nil
}

func main() {
	clientsPath := flag.String("clients", "", "путь к clients.csv")
	salesPath := flag.String("sales", "", "путь к sales.csv")
	version := flag.String("version", "demo-v1", "версия набора данных для DatasetWriter")
	batchSize := flag.Int("batch-size", data.DefaultBatchSize, "размер партии для contract.DatasetWriter")
	checkAnon := flag.Bool("check-anonymization", false, "проверить пороги обезличивания (>=3 покупателей, >=5 физлиц для B2C)")
	dryRun := flag.Bool("dry-run", false, "проверить CSV и сформировать партии без записи в PostgreSQL")
	verifyV4 := flag.Bool("verify-v4", false, "проверить закрытую БД v4 (recommendations.dataset_release, sales, clients)")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := run(ctx, logger, options{
		clientsPath: *clientsPath,
		salesPath:   *salesPath,
		version:     *version,
		batchSize:   *batchSize,
		checkAnon:   *checkAnon,
		dryRun:      *dryRun,
		verifyV4:    *verifyV4,
	}); err != nil {
		logger.Error("import failed", "error", err)
		fmt.Fprintln(os.Stderr, "import:", err)
		os.Exit(1)
	}
}

type options struct {
	clientsPath string
	salesPath   string
	version     string
	batchSize   int
	checkAnon   bool
	dryRun      bool
	verifyV4    bool
}

func run(ctx context.Context, logger *slog.Logger, opts options) error {
	if opts.verifyV4 {
		return verifyClosedV4(ctx, logger)
	}
	if opts.clientsPath == "" || opts.salesPath == "" {
		return errors.New("укажите -clients и -sales для CSV-импорта или -verify-v4 для проверки закрытой базы")
	}

	ds, err := data.LoadCSV(ctx, opts.clientsPath, opts.salesPath)
	if err != nil {
		return err
	}
	if opts.checkAnon {
		anon, err := data.CheckAnonymization(ds.Clients, ds.Sales)
		if err != nil {
			return err
		}
		logger.Info("anonymization verified", "items", anon.ItemsChecked, "min_buyers", anon.MinItemBuyers, "min_b2c_buyers", anon.MinB2CBuyers)
	}

	if !opts.dryRun {
		return errors.New("SQL-адаптер contract.DatasetWriter принадлежит зоне 4 (internal/store/postgres); используйте -dry-run для проверки CSV или см. tests/e2e/data/HANDOFF.md")
	}

	cf, err := os.Open(opts.clientsPath)
	if err != nil {
		return fmt.Errorf("повторно открыть %q: %w", opts.clientsPath, err)
	}
	defer cf.Close()
	sf, err := os.Open(opts.salesPath)
	if err != nil {
		return fmt.Errorf("повторно открыть %q: %w", opts.salesPath, err)
	}
	defer sf.Close()

	writer := &dryRunWriter{}
	summary, err := data.ImportCSV(ctx, opts.version, cf, sf, writer, opts.batchSize)
	if err != nil {
		return err
	}

	logger.Info("csv dry-run completed",
		"version", summary.Version,
		"clients", summary.Clients,
		"sales", summary.Sales,
		"batches", summary.Batches,
		"items", summary.DistinctItems,
		"empty_sku_rows", summary.EmptySKURows,
	)
	fmt.Printf("import dry-run ok: version=%s clients=%d sales=%d batches=%d items=%d empty_sku=%d\n",
		summary.Version, summary.Clients, summary.Sales, summary.Batches, summary.DistinctItems, summary.EmptySKURows)
	return nil
}

func verifyClosedV4(ctx context.Context, logger *slog.Logger) error {
	dsn, err := data.ResolveDatabaseURL()
	if err != nil {
		return err
	}
	logger.Info("verifying closed v4 release", "dsn", data.MaskDSN(dsn))

	store, err := postgres.Open(dsn)
	if err != nil {
		return fmt.Errorf("открыть store: %w", err)
	}
	defer store.Close()

	sha, sales, err := store.EvaluationV4(ctx)
	if err != nil {
		return fmt.Errorf("прочитать закрытую базу v4: %w", err)
	}
	summary, anon, err := eval.VerifyV4Evaluation(sha, sales)
	if err != nil {
		return err
	}

	logger.Info("closed v4 release verified",
		"sha256", sha,
		"rows", summary.Rows,
		"clients", summary.UniqueClients,
		"baskets", summary.Baskets,
		"items", summary.TotalItems,
		"train_items", summary.TrainItems,
		"cold_items", len(summary.ColdItems),
		"min_buyers", anon.MinItemBuyers,
		"min_b2c_buyers", anon.MinB2CBuyers,
	)
	fmt.Printf("v4 verified: sha256=%s rows=%d clients=%d baskets=%d items=%d train_items=%d cold_items=%d min_buyers=%d min_b2c_buyers=%d\n",
		sha, summary.Rows, summary.UniqueClients, summary.Baskets, summary.TotalItems,
		summary.TrainItems, len(summary.ColdItems), anon.MinItemBuyers, anon.MinB2CBuyers)
	return nil
}
