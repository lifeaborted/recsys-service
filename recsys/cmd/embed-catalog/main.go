package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"recsys/internal/explain"
	"recsys/internal/store/postgres"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stderr); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "embed-catalog:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, diagnostics io.Writer) error {
	flags := flag.NewFlagSet("embed-catalog", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	dsn := flags.String("dsn", os.Getenv("DATABASE_URL"), "pgvector database")
	source := flags.String("source", "", "catalog database; defaults to -dsn")
	limit := flags.Int("limit", 0, "maximum new offers to embed; 0 means the whole priced catalog")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *source == "" {
		*source = *dsn
	}
	if *dsn == "" || *source == "" {
		return fmt.Errorf("database DSN is required")
	}
	provider, err := explain.EmbeddingsFromEnv()
	if err != nil {
		return err
	}
	openCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	target, err := postgres.OpenContext(openCtx, *dsn)
	cancel()
	if err != nil {
		return err
	}
	defer target.Close()
	catalog := target
	if *source != *dsn {
		openCtx, cancel = context.WithTimeout(ctx, 15*time.Second)
		catalog, err = postgres.OpenContext(openCtx, *source)
		cancel()
		if err != nil {
			return err
		}
		defer catalog.Close()
	}
	if err := target.EnsureOfferEmbeddings(ctx); err != nil {
		return err
	}
	offers, err := catalog.AvailableOfferNames(ctx)
	if err != nil {
		return err
	}
	model := os.Getenv("EMBED_MODEL")
	stored, dimensions, err := target.EmbeddedOffers(ctx, model)
	if err != nil {
		return err
	}
	pending := make([]postgres.OfferName, 0, len(offers))
	for _, offer := range offers {
		if stored[offer.ID] != explain.InputHash(strings.TrimSpace(offer.Name)) {
			pending = append(pending, offer)
		}
	}
	if *limit > 0 && len(pending) > *limit {
		pending = pending[:*limit]
	}
	fmt.Fprintf(diagnostics, "offers %d stored %d pending %d dimensions %d\n", len(offers), len(stored), len(pending), dimensions)
	const batch = 32
	written := 0
	for start := 0; start < len(pending); start += batch {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk := pending[start:min(start+batch, len(pending))]
		names := make([]string, len(chunk))
		for i, offer := range chunk {
			names[i] = offer.Name
		}
		vectors, err := provider.EmbedNames(ctx, names)
		if err != nil {
			return err
		}
		rows := make([]postgres.EmbeddingRow, len(chunk))
		for i, offer := range chunk {
			if dimensions == 0 {
				dimensions = vectors[i].Dimensions
			} else if vectors[i].Dimensions != dimensions {
				return fmt.Errorf("embedding dimensions changed from %d to %d", dimensions, vectors[i].Dimensions)
			}
			rows[i] = postgres.EmbeddingRow{OfferID: offer.ID, TextSHA256: vectors[i].InputHash, Vector: vectors[i].Vector}
		}
		if err := target.SaveOfferEmbeddings(ctx, model, rows); err != nil {
			return err
		}
		written += len(rows)
		fmt.Fprintf(diagnostics, "embedded %d/%d\n", written, len(pending))
	}
	if written > 0 || len(stored) > 0 {
		if err := target.IndexOfferEmbeddings(ctx, model); err != nil {
			fmt.Fprintf(diagnostics, "index skipped: %v\n", err)
		}
	}
	fmt.Fprintf(diagnostics, "done stored %d\n", len(stored)+written)
	return nil
}
