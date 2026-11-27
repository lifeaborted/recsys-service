// Команда исторической оценки рекомендаций на закрытом наборе (v4/v5 БД или CSV) или синтетических CSV (demo).
//
//	go run ./cmd/backtest
//	go run ./cmd/backtest -clients testdata/smoke/clients.csv -sales testdata/smoke/sales.csv -label demo
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"recsys/internal/data"
	"recsys/internal/domain"
	"recsys/internal/eval"
	"recsys/internal/recommend"
	"recsys/internal/store/postgres"
)

type backtestOutput struct {
	Dataset               string              `json:"dataset"`
	WorkbookSHA256        string              `json:"workbook_sha256,omitempty"`
	HasProductGroupColumn bool                `json:"has_product_group_column"`
	TrainThrough          string              `json:"train_through"`
	TestFrom              string              `json:"test_from"`
	FeedGeneratedAt       string              `json:"feed_generated_at"`
	Summary               eval.DatasetSummary `json:"summary"`
	Baselines             []eval.Result       `json:"baselines"`
	LeakageWarnings       []string            `json:"leakage_warnings"`
	Hybrid                *eval.HybridReport  `json:"hybrid,omitempty"`
	HybridStatus          string              `json:"hybrid_status"`
}

func main() {
	clientsPath := flag.String("clients", "", "путь к clients.csv для автономного/demo/v5 прогона")
	salesPath := flag.String("sales", "", "путь к sales.csv (v4 6 колонок или v5 7 колонок с product_group)")
	label := flag.String("label", "", "метка в отчёте (по умолчанию версия БД или demo)")
	checkAnon := flag.Bool("check-anonymization", false, "проверить пороги обезличивания (>=3 / >=5)")
	showCold := flag.Bool("show-cold-items", false, "показать список item_id холодных товаров (не для публичного отчёта)")
	format := flag.String("format", "table", "формат вывода: table или json")
	snapshot := flag.String("snapshot", "", "immutable v4/v5 snapshot ID for hybrid evaluation")
	dataset := flag.String("dataset", "", "версия закрытой БД: v4 или v5 (по умолчанию v5 либо версия снимка)")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := run(ctx, logger, *clientsPath, *salesPath, *label, *checkAnon, *showCold, *format, *snapshot, *dataset); err != nil {
		logger.Error("backtest failed", "error", err)
		fmt.Fprintln(os.Stderr, "backtest:", err)
		os.Exit(1)
	}
}

func run(
	ctx context.Context,
	logger *slog.Logger,
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

	var out backtestOutput
	out.TrainThrough = eval.TrainThrough
	out.TestFrom = ">" + eval.TrainThrough
	out.LeakageWarnings = eval.DefaultLeakageWarnings()
	out.HybridStatus = "baseline only; supply -snapshot for a pinned hybrid run"

	if clientsPath != "" {
		if snapshot != "" {
			return fmt.Errorf("-snapshot requires PostgreSQL mode")
		}
		if strings.TrimSpace(label) == "" {
			label = "demo"
		}
		ds, err := data.LoadCSV(ctx, clientsPath, salesPath)
		if err != nil {
			return err
		}
		if checkAnon {
			if _, err := data.CheckAnonymization(ds.Clients, ds.Sales); err != nil {
				return err
			}
		}
		sales, err := eval.SalesFromDomain(ds.Clients, ds.Sales)
		if err != nil {
			return err
		}
		pgroups := eval.ProductGroupStrings(ds.ProductGroups)
		out.Dataset = label
		out.HasProductGroupColumn = ds.HasGroupCol
		out.FeedGeneratedAt = "unavailable (csv mode)"
		out.Summary = eval.SummarizeDatasetWithGroups(sales, pgroups)
		out.Baselines = eval.BaselinesWithGroups(sales, pgroups)
		logger.Info("csv backtest completed",
			"dataset", out.Dataset,
			"rows", out.Summary.Rows,
			"product_groups", out.Summary.ProductGroups,
			"has_product_group_col", out.HasProductGroupColumn,
			"cohort_a", out.Summary.CohortAClients,
		)
	} else {
		dsn, err := data.ResolveDatabaseURL()
		if err != nil {
			return err
		}
		logger.Info("connecting to closed database", "dsn", data.MaskDSN(dsn))

		store, err := postgres.OpenContext(ctx, dsn)
		if err != nil {
			return fmt.Errorf("открыть БД: %w", err)
		}
		defer store.Close()

		var meta postgres.SnapshotMetadata
		if snapshot != "" {
			meta, err = store.SnapshotMetadata(ctx, snapshot)
			if err != nil {
				return err
			}
			if dataset == "" {
				dataset = meta.DatasetVersion
			}
		}
		release, err := store.EvaluationDataset(ctx, dataset)
		if err != nil {
			return fmt.Errorf("загрузить закрытые продажи: %w", err)
		}
		if strings.TrimSpace(label) == "" {
			label = release.Version
		}
		summary, anon, err := eval.VerifyClosedEvaluation(release.Version, release.SHA, release.Sales, release.ProductGroups)
		if err != nil {
			return err
		}
		out.Dataset = label
		out.WorkbookSHA256 = release.SHA
		out.HasProductGroupColumn = release.Version == data.ExpectedV5Version
		out.FeedGeneratedAt = "not used in the sales-only snapshot"
		out.Summary = summary
		out.Baselines = eval.BaselinesWithGroups(release.Sales, eval.ProductGroupStrings(release.ProductGroups))
		if snapshot != "" {
			if meta.DatasetVersion != release.Version || meta.WorkbookSHA256 != release.SHA || meta.TrainThrough.Format(time.DateOnly) != eval.TrainThrough {
				return fmt.Errorf("snapshot does not match %s release and train cutoff", release.Version)
			}
			batch, err := store.OfflineDataset(ctx)
			if err != nil {
				return err
			}
			allowed := eval.AllowedItemsFromSales(batch.Sales)
			if len(allowed) != 817 {
				return fmt.Errorf("expected 817 historical items, got %d", len(allowed))
			}
			ids := make([]domain.ItemID, 0, len(allowed))
			for id := range allowed {
				ids = append(ids, id)
			}
			products, err := store.Products(ctx, snapshot, ids)
			if err != nil {
				return err
			}
			families := make(map[domain.ItemID]string)
			for id, p := range products {
				if p.FamilySource != "" && p.FamilyID != "" {
					families[id] = p.FamilyID
				}
			}
			report, err := eval.EvaluateRecommender(ctx, recommend.New(store), batch.Clients, batch.Sales, eval.BacktestConfig{
				DatasetLabel: release.Version, SnapshotID: snapshot, AllowedItems: allowed, ProductGroups: release.ProductGroups, ItemFamilies: families, FeedGeneratedAt: meta.FeedGeneratedAt,
			})
			if err != nil {
				return err
			}
			if !showCold {
				report.Cold.ColdItems = nil
				report.Summary.ColdItems = nil
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
	}

	return printOutput(out, showCold, format)
}

func printOutput(out backtestOutput, showCold bool, format string) error {
	if format == "json" {
		if !showCold {
			out.Summary.ColdItems = nil
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	if out.WorkbookSHA256 != "" {
		fmt.Printf("dataset=%s sha256=%s has_product_group=%t train_through=%s test_from=%s\n",
			out.Dataset, out.WorkbookSHA256, out.HasProductGroupColumn, out.TrainThrough, out.TestFrom)
	} else {
		fmt.Printf("dataset=%s has_product_group=%t train_through=%s test_from=%s\n",
			out.Dataset, out.HasProductGroupColumn, out.TrainThrough, out.TestFrom)
	}
	fmt.Printf("summary: rows=%d clients=%d baskets=%d items=%d product_groups=%d train_items=%d cold_items=%d cohort_a=%d cohort_b=%d\n",
		out.Summary.Rows, out.Summary.UniqueClients, out.Summary.Baskets, out.Summary.TotalItems,
		out.Summary.ProductGroups, out.Summary.TrainItems, len(out.Summary.ColdItems),
		out.Summary.CohortAClients, out.Summary.CohortBClients)
	if showCold && len(out.Summary.ColdItems) > 0 {
		fmt.Printf("cold_items: %s\n", strings.Join(out.Summary.ColdItems, ", "))
	}
	fmt.Printf("feed_generated_at: %s\n", out.FeedGeneratedAt)
	fmt.Println("Cohort  Group  Clients  Pop_P@3  Pop_Fill  Pop_Full3  Pop_Hits  Pop_Var%  Co_P@3  Co_Fill  Co_Full3  Co_Hits  Co_Var%")
	for _, r := range out.Baselines {
		fmt.Printf("%-6s  %-5s  %7d  %6.2f%%  %7.1f%%  %8.1f%%  %8d  %7.1f%%  %5.2f%%  %6.1f%%  %7.1f%%  %7d  %6.1f%%\n",
			r.Cohort, r.Group, r.Clients,
			r.Popularity.Precision, r.Popularity.FillRate, r.Popularity.Full3Rate, r.Popularity.Hits, r.Popularity.VariantShare,
			r.CoPurchase.Precision, r.CoPurchase.FillRate, r.CoPurchase.Full3Rate, r.CoPurchase.Hits, r.CoPurchase.VariantShare)
	}
	fmt.Println("leakage_warnings:")
	for _, w := range out.LeakageWarnings {
		fmt.Printf("  - %s\n", w)
	}
	fmt.Printf("hybrid_eval: %s\n", out.HybridStatus)
	if out.Hybrid != nil {
		fmt.Printf("snapshot_id=%s\n", out.Hybrid.SnapshotID)
		for _, r := range out.Hybrid.Results {
			fmt.Printf("%s %s clients=%d P@3=%.2f%% hits=%d variant=%.2f%% new=%.2f%% unknown=%.2f%%\n", r.Cohort, r.Group, r.Clients, r.Metrics.Precision, r.Metrics.Hits,
				r.Families.KnownFamilyShare(), r.Families.NewFamilyShare(), r.Families.UnknownFamilyShare())
		}
	}
	return nil
}
