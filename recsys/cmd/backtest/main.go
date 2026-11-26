// Команда исторической оценки рекомендаций на закрытом наборе (v4 БД / v5 CSV) или синтетических CSV (demo).
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
	"recsys/internal/eval"
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
	HybridStatus          string              `json:"hybrid_status"`
}

func main() {
	clientsPath := flag.String("clients", "", "путь к clients.csv для автономного/demo/v5 прогона")
	salesPath := flag.String("sales", "", "путь к sales.csv (v4 6 колонок или v5 7 колонок с product_group)")
	label := flag.String("label", "", "метка набора данных (по умолчанию v4 для БД и demo для CSV)")
	checkAnon := flag.Bool("check-anonymization", false, "проверить пороги обезличивания (>=3 / >=5)")
	showCold := flag.Bool("show-cold-items", false, "показать список item_id холодных товаров (не для публичного отчёта)")
	format := flag.String("format", "table", "формат вывода: table или json")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := run(ctx, logger, *clientsPath, *salesPath, *label, *checkAnon, *showCold, *format); err != nil {
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
	format string,
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
	out.HybridStatus = "not_wired: требуется реализация contract.Recommender (зона 2) и метод чтения снимка/каталога в store/postgres (зона 4); логика оценки проверена в TestDataModule"

	if clientsPath != "" {
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
		if strings.TrimSpace(label) == "" {
			label = data.ExpectedV4Version
		}
		dsn, err := data.ResolveDatabaseURL()
		if err != nil {
			return err
		}
		logger.Info("connecting to v4 database", "dsn", data.MaskDSN(dsn))

		store, err := postgres.Open(dsn)
		if err != nil {
			return fmt.Errorf("открыть БД: %w", err)
		}
		defer store.Close()

		sha, sales, err := store.EvaluationV4(ctx)
		if err != nil {
			return fmt.Errorf("загрузить продажи v4: %w", err)
		}
		summary, anon, err := eval.VerifyV4Evaluation(sha, sales)
		if err != nil {
			return err
		}
		out.Dataset = label
		out.WorkbookSHA256 = sha
		out.HasProductGroupColumn = false
		out.FeedGeneratedAt = "unavailable via EvaluationV4 (обновлённой БД v5 с product_group пока нет; см. tests/e2e/data/README.md)"
		out.Summary = summary
		out.Baselines = eval.Baselines(sales)
		logger.Info("v4 backtest completed",
			"sha256", sha,
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
	return nil
}
