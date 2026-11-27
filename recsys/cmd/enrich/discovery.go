package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"recsys/internal/domain"
	"recsys/internal/enrichment"
	"recsys/internal/explain"
	"recsys/internal/store/postgres"
)

type discovery struct {
	From domain.ItemID `json:"from_item_id"`
	To   domain.ItemID `json:"to_item_id"`
	explain.RelationProposal
}

type discoveryTask struct {
	key      string
	from, to postgres.OnlineRelationItem
	source   explain.PublicText
	priority int
}

func discoverOnline(ctx context.Context, snapshot, sourcesPath, statePath string, limit, neighbors int, output io.Writer) error {
	inputPath, err := filepath.Abs(sourcesPath)
	if err != nil {
		return err
	}
	outputPath, err := filepath.Abs(statePath)
	if err != nil {
		return err
	}
	if strings.EqualFold(inputPath, outputPath) {
		return fmt.Errorf("relation-state must be separate from sources")
	}

	var approved struct {
		enrichment.Review
		ProductNames map[domain.ItemID]string `json:"product_names"`
	}
	if err := readJSON(sourcesPath, &approved); err != nil {
		return err
	}
	if len(approved.Documents) == 0 {
		return fmt.Errorf("sources must contain approved public excerpts")
	}
	for _, doc := range approved.Documents {
		if doc.Source.ObservedAt.IsZero() {
			return fmt.Errorf("every source requires observed_at")
		}
	}
	var state map[string]discovery
	if err := readJSON(statePath, &state); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if state == nil {
		state = make(map[string]discovery)
	}

	chat := explain.ChatFromEnv()
	if strings.TrimSpace(chat.Model) == "" || strings.TrimSpace(chat.BaseURL) == "" {
		return fmt.Errorf("LLM_MODEL and LLM_BASE_URL are required")
	}
	openCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	store, err := postgres.OpenContext(openCtx, os.Getenv("DATABASE_URL"), postgres.WithOnlineSources(snapshot, os.Getenv("EMBED_MODEL")))
	cancel()
	if err != nil {
		return err
	}
	defer store.Close()
	items, err := store.OnlineRelationItems(ctx, snapshot)
	if err != nil {
		return err
	}
	for i := range items {
		if name := strings.TrimSpace(approved.ProductNames[items[i].ID]); name != "" {
			items[i].Title = name
		}
	}
	tasks := planDiscoveries(items, approved.Documents, chat.Model, state)
	if os.Getenv("EMBED_MODEL") != "" {
		vectors := make(map[domain.ItemID]map[domain.ItemID]bool)
		for i := range tasks {
			task := &tasks[i]
			related, loaded := vectors[task.from.ID]
			if !loaded {
				rows, err := store.SimilarItems(ctx, snapshot, []domain.ItemID{task.from.ID}, neighbors)
				if err != nil {
					return fmt.Errorf("read discovery neighbors: %w", err)
				}
				related = make(map[domain.ItemID]bool, len(rows))
				for _, row := range rows {
					related[row.ItemID] = true
				}
				vectors[task.from.ID] = related
			}
			if related[task.to.ID] {
				task.priority++
			}
		}
	}
	slices.SortFunc(tasks, func(a, b discoveryTask) int {
		if n := cmp.Compare(b.priority, a.priority); n != 0 {
			return n
		}
		return cmp.Compare(a.key, b.key)
	})

	var completed int
	var failures []error
	for _, task := range tasks[:min(limit, len(tasks))] {
		if err := ctx.Err(); err != nil {
			return err
		}
		proposal, err := chat.ProposeRelation(ctx, explain.RelationInput{
			From: task.from.Title, To: task.to.Title, Source: task.source,
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("relation draft %.12s: %w", task.key, err))
			continue
		}
		state[task.key] = discovery{From: task.from.ID, To: task.to.ID, RelationProposal: proposal}
		if err := saveDiscoveryState(statePath, state); err != nil {
			return err
		}
		completed++
	}
	_, err = fmt.Fprintf(output, "snapshot=%s pending=%d completed=%d failed=%d\n", snapshot, len(tasks)-completed, completed, len(failures))
	return errors.Join(append(failures, err)...)
}

func planDiscoveries(items []postgres.OnlineRelationItem, documents []enrichment.Document, model string, state map[string]discovery) []discoveryTask {
	var result []discoveryTask
	seen := make(map[string]bool)
	for _, doc := range documents {
		text := strings.ToLower(doc.Text)
		var mentioned []postgres.OnlineRelationItem
		for _, item := range items {
			if title := strings.TrimSpace(item.Title); title != "" && strings.Contains(text, strings.ToLower(title)) {
				mentioned = append(mentioned, item)
			}
		}
		source := explain.PublicText{Text: doc.Text, SourceURL: doc.Source.URL, Version: doc.Source.Version, Approved: true}
		for _, from := range mentioned {
			for _, to := range mentioned {
				if from.ID == to.ID || from.OfferID == to.OfferID || strings.EqualFold(from.Title, to.Title) ||
					!explain.HasRelationEndpoints(doc.Text, from.Title, to.Title) {
					continue
				}
				identity, _ := json.Marshal([]any{from, to, doc.Source, explain.InputHash(doc.Text), model, explain.RelationPromptVersion})
				key := explain.InputHash(string(identity))
				if _, done := state[key]; done || seen[key] {
					continue
				}
				seen[key] = true
				priority := 0
				if from.Family != "" && from.Family == to.Family {
					priority += 2
				}
				fromSKU, toSKU := skuStem(from.SKU), skuStem(to.SKU)
				if fromSKU != "" && fromSKU == toSKU {
					priority++
				}
				result = append(result, discoveryTask{key: key, from: from, to: to, source: source, priority: priority})
			}
		}
	}
	return result
}

func skuStem(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if i := strings.LastIndexAny(value, "-_/"); i >= 4 {
		return value[:i]
	}
	return value
}

func saveDiscoveryState(path string, state map[string]discovery) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".relations-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	writeErr := encoder.Encode(state)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
