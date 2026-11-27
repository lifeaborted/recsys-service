package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"recsys/internal/domain"
	"recsys/internal/enrichment"
	"recsys/internal/explain"
	"recsys/internal/store/postgres"
	_ "time/tzdata"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "enrich:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output, diagnostics io.Writer) error {
	flags := flag.NewFlagSet("enrich", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	path := flags.String("feed", "", "path to YML or gzipped YML")
	itemsPath := flags.String("items", "", "path to item JSON")
	reviewPath := flags.String("sources", "", "path to reviewed source and rule JSON")
	version := flags.String("catalog-version", "", "immutable catalog version")
	sourceURL := flags.String("feed-url", "", "source URL for the feed")
	snapshot := flags.String("snapshot", "", "recommendation snapshot ID")
	locationName := flags.String("feed-timezone", "Asia/Yekaterinburg", "timezone for YML dates without offset")
	cutoffRaw := flags.String("cutoff", "", "optional RFC3339 cutoff instant")
	card := flags.String("card", "", "optional item ID for explanation")
	publicPath := flags.String("public-text", "", "optional approved public excerpt JSON for chat ordering")
	searchPath := flags.String("semantic-search", "", "optional JSON approving feed names and per-item queries for embeddings")
	vectorIndex := flags.Bool("vector-index", false, "read catalog vectors already stored in DATABASE_URL")
	proposalPath := flags.String("propose-relation", "", "extract a relation proposal from one approved source JSON")
	requestsPath := flags.String("relation-requests", "", "path to relation requests approving local source excerpts for extraction")
	discover := flags.Bool("discover-online", false, "discover relation drafts from a pinned v5-online snapshot")
	statePath := flags.String("relation-state", "", "local JSON file storing completed online discoveries")
	maxPairs := flags.Int("max-pairs", 20, "maximum new relation requests per background run")
	neighbors := flags.Int("neighbors", 8, "number of existing vector neighbors used to prioritize discovery")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if *discover {
		var invalid string
		flags.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "discover-online", "snapshot", "sources", "relation-state", "max-pairs", "neighbors":
			default:
				invalid = f.Name
			}
		})
		if flags.NArg() != 0 || invalid != "" || *snapshot == "" || *reviewPath == "" || *statePath == "" ||
			*maxPairs < 1 || *maxPairs > 200 || *neighbors < 1 || *neighbors > 50 {
			return fmt.Errorf("discover-online requires -snapshot, -sources and -relation-state (max-pairs 1–200, neighbors 1–50)")
		}
		return discoverOnline(ctx, *snapshot, *reviewPath, *statePath, *maxPairs, *neighbors, output)
	}
	if *statePath != "" || *maxPairs != 20 || *neighbors != 8 {
		return fmt.Errorf("relation-state, max-pairs and neighbors require discover-online")
	}
	if *proposalPath != "" {
		onlyProposal := flags.NFlag() == 1 && flags.NArg() == 0
		if !onlyProposal {
			return fmt.Errorf("propose-relation runs on its own")
		}
		var input explain.RelationInput
		if err := readJSON(*proposalPath, &input); err != nil {
			return err
		}
		proposal, err := explain.ChatFromEnv().ProposeRelation(ctx, input)
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		return encoder.Encode(proposal)
	}
	if flags.NArg() != 0 || *path == "" || *itemsPath == "" || *version == "" || *sourceURL == "" || *snapshot == "" {
		flags.Usage()
		return fmt.Errorf("feed, items, catalog-version, feed-url and snapshot are required")
	}
	if strings.EqualFold(strings.TrimSpace(*snapshot), "v5-train-20260831") {
		return fmt.Errorf("snapshot v5-train-20260831 is reserved for historical evaluation")
	}
	if *publicPath != "" && *card == "" {
		return fmt.Errorf("public-text requires card")
	}
	location, err := time.LoadLocation(*locationName)
	if err != nil {
		return err
	}
	var cutoff *time.Time
	if *cutoffRaw != "" {
		parsed, err := time.Parse(time.RFC3339, *cutoffRaw)
		if err != nil {
			return fmt.Errorf("invalid cutoff: %w", err)
		}
		cutoff = &parsed
	}
	file, err := os.Open(*path)
	if err != nil {
		return err
	}
	defer file.Close()
	var offers []enrichment.Offer
	feed, err := enrichment.ReadFeed(file, *version, *sourceURL, location, func(_ enrichment.FeedVersion, offer enrichment.Offer) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		offers = append(offers, offer)
		return nil
	})
	if err != nil {
		return err
	}
	var items []enrichment.Item
	if err := readJSON(*itemsPath, &items); err != nil {
		return err
	}
	var review enrichment.Review
	if *reviewPath != "" {
		if err := readJSON(*reviewPath, &review); err != nil {
			return err
		}
	}
	var search enrichment.Searcher
	if *searchPath != "" {
		var approval explain.SearchApproval
		if err := readJSON(*searchPath, &approval); err != nil {
			return err
		}
		provider, err := explain.EmbeddingsFromEnv()
		if err != nil {
			return err
		}
		if *vectorIndex {
			if os.Getenv("DATABASE_URL") == "" {
				return fmt.Errorf("vector index requires DATABASE_URL")
			}
			openCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			store, err := postgres.OpenContext(openCtx, os.Getenv("DATABASE_URL"))
			cancel()
			if err != nil {
				return err
			}
			defer store.Close()
			model := os.Getenv("EMBED_MODEL")
			search, err = explain.NewStoredSearch(provider, feed, approval, func(ctx context.Context, vector []float64, limit int) ([]enrichment.SearchHit, error) {
				rows, err := store.NearestOffers(ctx, model, vector, limit)
				if err != nil {
					return nil, err
				}
				hits := make([]enrichment.SearchHit, len(rows))
				for i, row := range rows {
					hits[i] = enrichment.SearchHit{OfferID: row.OfferID, Basis: fmt.Sprintf("stored offer cosine similarity %.6f (model %s)", row.Score, model)}
				}
				return hits, nil
			})
		} else {
			search, err = explain.NewCatalogSearch(provider, feed, offers, approval)
		}
		if err != nil {
			return err
		}
	}
	result, err := enrichment.Build(ctx, feed, *snapshot, cutoff, offers, items, review, search)
	if err != nil {
		return err
	}
	response := struct {
		enrichment.Result
		Proposals   []relationDraft   `json:"relation_proposals,omitempty"`
		Explanation *explain.Rendered `json:"explanation,omitempty"`
	}{Result: result}
	if *requestsPath != "" {
		var requests []relationRequest
		if err := readJSON(*requestsPath, &requests); err != nil {
			return err
		}
		drafts, rejections, err := extractRelations(ctx, requests, review.Documents, result)
		if err != nil {
			return err
		}
		response.Proposals = drafts
		response.Rejected = append(response.Rejected, rejections...)
	}
	if *card != "" {
		var variants []domain.Variant
		for _, fact := range result.Families {
			if fact.ItemID == domain.ItemID(*card) && fact.Family.Value != nil {
				variants = result.Variants[*fact.Family.Value]
				break
			}
		}
		var chat *explain.Chat
		var public *explain.PublicText
		if *publicPath != "" {
			public = new(explain.PublicText)
			if err := readJSON(*publicPath, public); err != nil {
				return err
			}
			chat = explain.ChatFromEnv()
		}
		rendered := explain.Render(ctx, chat, public, explain.EdgesFor(domain.ItemID(*card), result.Relations), variants)
		response.Explanation = &rendered
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(response)
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
