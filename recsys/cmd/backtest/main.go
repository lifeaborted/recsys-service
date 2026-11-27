// Команда исторической оценки рекомендаций на закрытом наборе (v5/v4 БД или CSV) или синтетических CSV (demo).
//
//	go run ./cmd/backtest -dataset v5 -snapshot v5-train-20260831
//	go run ./cmd/backtest -clients testdata/public-eval/clients.csv -sales testdata/public-eval/sales.csv -label demo -format json
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"recsys/internal/contract"
	"recsys/internal/data"
	"recsys/internal/domain"
	"recsys/internal/eval"
	"recsys/internal/recommend"
	"recsys/internal/store/postgres"
)

type backtestData struct {
	Dataset               string
	WorkbookSHA256        string
	HasProductGroupColumn bool
	FeedGeneratedAt       string
	Summary               eval.DatasetSummary
	Baselines             []eval.Result
	LeakageWarnings       []string
	Hybrid                *eval.HybridReport
	HybridStatus          string
	Demo                  bool
}

func main() {
	clientsPath := flag.String("clients", "", "путь к clients.csv для автономного/demo/v5 прогона")
	salesPath := flag.String("sales", "", "путь к sales.csv (v4 6 колонок или v5 7 колонок с product_group)")
	label := flag.String("label", "", "метка в отчёте (по умолчанию версия БД или demo)")
	checkAnon := flag.Bool("check-anonymization", false, "проверить пороги обезличивания (>=3 / >=5)")
	showCold := flag.Bool("show-cold-items", false, "показать список item_id холодных товаров (только table, не для -format json)")
	format := flag.String("format", "table", "формат вывода: table или json (публичный JSON без stderr-логов)")
	snapshot := flag.String("snapshot", "", "immutable v5/v4 snapshot ID for hybrid evaluation")
	dataset := flag.String("dataset", "", "версия закрытой БД: v5 или v4 (по умолчанию v5 либо версия снимка)")
	flag.Parse()

	logWriter := io.Writer(os.Stderr)
	if *format == "json" {
		logWriter = io.Discard
	}
	logger := slog.New(slog.NewJSONHandler(logWriter, nil))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := run(ctx, logger, os.Stdout, *clientsPath, *salesPath, *label, *checkAnon, *showCold, *format, *snapshot, *dataset); err != nil {
		logger.Error("backtest failed", "error", err)
		fmt.Fprintln(os.Stderr, "backtest:", err)
		os.Exit(1)
	}
}

func run(
	ctx context.Context,
	logger *slog.Logger,
	outW io.Writer,
	clientsPath, salesPath, label string,
	checkAnon, showCold bool,
	format, snapshot, dataset string,
) error {
	if (clientsPath != "") != (salesPath != "") {
		return errors.New("для CSV-режима укажите одновременно -clients и -sales")
	}
	if format != "table" && format != "json" {
		return fmt.Errorf("неподдерживаемый формат %q (доступны table, json)", format)
	}
	if format == "json" && showCold {
		return errors.New("-show-cold-items запрещён в публичном JSON-режиме")
	}

	var res backtestData
	var err error
	if clientsPath != "" {
		if snapshot != "" {
			return fmt.Errorf("-snapshot requires PostgreSQL mode")
		}
		res, err = evaluateCSV(ctx, logger, clientsPath, salesPath, label, checkAnon, "", nil)
	} else {
		res, err = evaluateDatabase(ctx, logger, label, snapshot, dataset)
	}
	if err != nil {
		return err
	}

	return printOutput(outW, res, showCold, format)
}

func evaluateCSV(
	ctx context.Context,
	logger *slog.Logger,
	clientsPath, salesPath, label string,
	checkAnon bool,
	snapshotID string,
	rec contract.Recommender,
) (backtestData, error) {
	if strings.TrimSpace(label) == "" {
		label = "demo"
	}
	ds, err := data.LoadCSV(ctx, clientsPath, salesPath)
	if err != nil {
		return backtestData{}, err
	}
	if checkAnon {
		if _, err := data.CheckAnonymization(ds.Clients, ds.Sales); err != nil {
			return backtestData{}, err
		}
	}
	sales, err := eval.SalesFromDomain(ds.Clients, ds.Sales)
	if err != nil {
		return backtestData{}, err
	}
	pgroups := eval.ProductGroupStrings(ds.ProductGroups)
	out := backtestData{
		Dataset:               label,
		HasProductGroupColumn: ds.HasGroupCol,
		FeedGeneratedAt:       "unavailable (csv mode)",
		Summary:               eval.SummarizeDatasetWithGroups(sales, pgroups),
		Baselines:             eval.BaselinesWithGroups(sales, pgroups),
		LeakageWarnings:       eval.DefaultLeakageWarnings(),
		HybridStatus:          "baseline only; supply -snapshot for a pinned hybrid run",
		Demo:                  isDemoLabel(label),
	}
	if rec != nil && strings.TrimSpace(snapshotID) != "" {
		allowed := eval.AllowedItemsFromSales(ds.Sales)
		report, err := eval.EvaluateRecommender(ctx, rec, ds.Clients, ds.Sales, eval.BacktestConfig{
			DatasetLabel:  label,
			SnapshotID:    snapshotID,
			AllowedItems:  allowed,
			ProductGroups: ds.ProductGroups,
		})
		if err != nil {
			return backtestData{}, err
		}
		out.Hybrid = &report
		out.HybridStatus = "completed"
	}
	logger.Info("csv backtest completed",
		"dataset", out.Dataset,
		"rows", out.Summary.Rows,
		"product_groups", out.Summary.ProductGroups,
		"has_product_group_col", out.HasProductGroupColumn,
		"cohort_a", out.Summary.CohortAClients,
	)
	return out, nil
}

func evaluateDatabase(ctx context.Context, logger *slog.Logger, label, snapshot, dataset string) (backtestData, error) {
	dsn, err := data.ResolveDatabaseURL()
	if err != nil {
		return backtestData{}, err
	}
	logger.Info("connecting to closed database", "dsn", data.MaskDSN(dsn))

	store, err := postgres.OpenContext(ctx, dsn)
	if err != nil {
		return backtestData{}, fmt.Errorf("открыть БД: %w", err)
	}
	defer store.Close()

	var meta postgres.SnapshotMetadata
	if snapshot != "" {
		meta, err = store.SnapshotMetadata(ctx, snapshot)
		if err != nil {
			return backtestData{}, err
		}
		if dataset == "" {
			dataset = meta.DatasetVersion
		}
	}
	release, err := store.EvaluationDataset(ctx, dataset)
	if err != nil {
		return backtestData{}, fmt.Errorf("загрузить закрытые продажи: %w", err)
	}
	if strings.TrimSpace(label) == "" {
		label = release.Version
	}
	summary, anon, err := eval.VerifyClosedEvaluation(release.Version, release.SHA, release.Sales, release.ProductGroups)
	if err != nil {
		return backtestData{}, err
	}

	out := backtestData{
		Dataset:               label,
		WorkbookSHA256:        release.SHA,
		HasProductGroupColumn: release.Version == data.ExpectedV5Version,
		FeedGeneratedAt:       "not used in the sales-only snapshot",
		Summary:               summary,
		Baselines:             eval.BaselinesWithGroups(release.Sales, eval.ProductGroupStrings(release.ProductGroups)),
		LeakageWarnings:       eval.DefaultLeakageWarnings(),
		HybridStatus:          "baseline only; supply -snapshot for a pinned hybrid run",
		Demo:                  isDemoLabel(label),
	}
	if snapshot != "" {
		if meta.DatasetVersion != release.Version || meta.WorkbookSHA256 != release.SHA || meta.TrainThrough.Format(time.DateOnly) != eval.TrainThrough {
			return backtestData{}, fmt.Errorf("snapshot does not match %s release and train cutoff", release.Version)
		}
		batch, err := store.OfflineDataset(ctx)
		if err != nil {
			return backtestData{}, err
		}
		allowed := eval.AllowedItemsFromSales(batch.Sales)
		if len(allowed) != data.ExpectedV4Items {
			return backtestData{}, fmt.Errorf("expected %d historical items, got %d", data.ExpectedV4Items, len(allowed))
		}
		ids := make([]domain.ItemID, 0, len(allowed))
		for id := range allowed {
			ids = append(ids, id)
		}
		products, err := store.Products(ctx, snapshot, ids)
		if err != nil {
			return backtestData{}, err
		}
		families := make(map[domain.ItemID]string)
		for id, p := range products {
			if p.FamilySource != "" && p.FamilyID != "" {
				families[id] = p.FamilyID
			}
		}
		report, err := eval.EvaluateRecommender(ctx, recommend.New(store), batch.Clients, batch.Sales, eval.BacktestConfig{
			DatasetLabel:    release.Version,
			SnapshotID:      snapshot,
			AllowedItems:    allowed,
			ProductGroups:   release.ProductGroups,
			ItemFamilies:    families,
			FeedGeneratedAt: meta.FeedGeneratedAt,
		})
		if err != nil {
			return backtestData{}, err
		}
		out.Hybrid = &report
		out.HybridStatus = "completed"
		if meta.FeedGeneratedAt != nil {
			out.FeedGeneratedAt = meta.FeedGeneratedAt.Format(time.RFC3339)
		}
	}

	logger.Info("closed backtest completed",
		"version", release.Version, "sha256", release.SHA,
		"rows", summary.Rows,
		"baskets", summary.Baskets,
		"items", summary.TotalItems,
		"min_buyers", anon.MinItemBuyers,
		"min_b2c_buyers", anon.MinB2CBuyers,
	)
	return out, nil
}

func isDemoLabel(label string) bool {
	low := strings.ToLower(strings.TrimSpace(label))
	return low == "demo" || strings.HasPrefix(low, "demo-") || strings.HasPrefix(low, "synth")
}

func printOutput(w io.Writer, out backtestData, showCold bool, format string) error {
	if format == "json" {
		public := eval.BuildPublicReport(
			out.Dataset,
			out.WorkbookSHA256,
			out.FeedGeneratedAt,
			out.HasProductGroupColumn,
			out.Summary,
			out.Baselines,
			out.Hybrid,
			out.HybridStatus,
			out.LeakageWarnings,
			out.Demo,
		)
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(public)
	}

	if out.WorkbookSHA256 != "" {
		fmt.Fprintf(w, "dataset=%s sha256=%s has_product_group=%t train_through=%s test_from=%s\n",
			out.Dataset, out.WorkbookSHA256, out.HasProductGroupColumn, eval.TrainThrough, ">"+eval.TrainThrough)
	} else {
		fmt.Fprintf(w, "dataset=%s has_product_group=%t train_through=%s test_from=%s\n",
			out.Dataset, out.HasProductGroupColumn, eval.TrainThrough, ">"+eval.TrainThrough)
	}
	fmt.Fprintf(w, "summary: rows=%d clients=%d baskets=%d items=%d product_groups=%d train_items=%d cold_items=%d cohort_a=%d cohort_b=%d\n",
		out.Summary.Rows, out.Summary.UniqueClients, out.Summary.Baskets, out.Summary.TotalItems,
		out.Summary.ProductGroups, out.Summary.TrainItems, len(out.Summary.ColdItems),
		out.Summary.CohortAClients, out.Summary.CohortBClients)
	if showCold && len(out.Summary.ColdItems) > 0 {
		fmt.Fprintf(w, "cold_items: %s\n", strings.Join(out.Summary.ColdItems, ", "))
	}
	fmt.Fprintf(w, "feed_generated_at: %s\n", out.FeedGeneratedAt)
	fmt.Fprintln(w, "Cohort  Group  Clients  Pop_P@3  Pop_Fill  Pop_Full3  Pop_Hits  Pop_Var%  Co_P@3  Co_Fill  Co_Full3  Co_Hits  Co_Var%")
	for _, r := range out.Baselines {
		fmt.Fprintf(w, "%-6s  %-5s  %7d  %6.2f%%  %7.1f%%  %8.1f%%  %8d  %7.1f%%  %5.2f%%  %6.1f%%  %7.1f%%  %7d  %6.1f%%\n",
			r.Cohort, r.Group, r.Clients,
			r.Popularity.Precision, r.Popularity.FillRate, r.Popularity.Full3Rate, r.Popularity.Hits, r.Popularity.VariantShare,
			r.CoPurchase.Precision, r.CoPurchase.FillRate, r.CoPurchase.Full3Rate, r.CoPurchase.Hits, r.CoPurchase.VariantShare)
	}
	fmt.Fprintln(w, "leakage_warnings:")
	for _, warn := range out.LeakageWarnings {
		fmt.Fprintf(w, "  - %s\n", warn)
	}
	fmt.Fprintf(w, "hybrid_eval: %s\n", out.HybridStatus)
	if out.Hybrid != nil {
		fmt.Fprintf(w, "snapshot_id=%s\n", out.Hybrid.SnapshotID)
		for _, r := range out.Hybrid.Results {
			fmt.Fprintf(w, "%s %s clients=%d P@3=%.2f%% hits=%d variant=%.2f%% new=%.2f%% unknown=%.2f%%\n",
				r.Cohort, r.Group, r.Clients, r.Metrics.Precision, r.Metrics.Hits,
				r.Families.KnownFamilyShare(), r.Families.NewFamilyShare(), r.Families.UnknownFamilyShare())
		}
	}
	return nil
}
