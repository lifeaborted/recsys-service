package explain

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
)

type Embedding struct {
	Model      string    `json:"model"`
	Dimensions int       `json:"dimensions"`
	InputHash  string    `json:"input_sha256"`
	Vector     []float64 `json:"vector"`
}

type Embeddings struct {
	config     Config
	batchSize  int
	mutex      sync.Mutex
	cache      map[string]Embedding
	dimensions int
}

func EmbeddingsFromEnv() (*Embeddings, error) {
	size := 32
	if raw := os.Getenv("EMBED_BATCH_SIZE"); raw != "" {
		var err error
		size, err = strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid EMBED_BATCH_SIZE")
		}
	}
	return NewEmbeddings(configFromEnv("EMBED"), size)
}

func NewEmbeddings(config Config, batchSize int) (*Embeddings, error) {
	if batchSize < 1 || strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("embedding model and positive batch size required")
	}
	return &Embeddings{config: config, batchSize: batchSize, cache: make(map[string]Embedding)}, nil
}

// Embed batches approved texts and returns vectors in input order. Completed
// batches can be reused on retry through the same adapter.
func (e *Embeddings) Embed(ctx context.Context, inputs []PublicText) ([]Embedding, error) {
	result := make([]Embedding, len(inputs))
	position := make(map[string][]int)
	pending := []string{}
	for i, input := range inputs {
		if err := input.validate(); err != nil {
			return nil, err
		}
		if _, exists := position[input.Text]; !exists {
			pending = append(pending, input.Text)
		}
		position[input.Text] = append(position[input.Text], i)
	}
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	missing := make([]string, 0, len(pending))
	for _, value := range pending {
		if _, exists := e.cache[InputHash(value)]; !exists {
			missing = append(missing, value)
		}
	}
	for start := 0; start < len(missing); start += e.batchSize {
		batch := missing[start:min(start+e.batchSize, len(missing))]
		vectors, err := e.embedBatch(ctx, batch)
		if err != nil {
			return nil, err
		}
		dimensions := len(vectors[0])
		if e.dimensions != 0 && dimensions != e.dimensions {
			return nil, fmt.Errorf("embedding dimensions changed")
		}
		e.dimensions = dimensions
		for i, value := range batch {
			hash := InputHash(value)
			e.cache[hash] = Embedding{Model: e.config.Model, Dimensions: dimensions, InputHash: hash, Vector: vectors[i]}
		}
	}
	for _, value := range pending {
		embedding := e.cache[InputHash(value)]
		for _, i := range position[value] {
			result[i] = embedding
			result[i].Vector = slices.Clone(embedding.Vector)
		}
	}
	return result, nil
}

func (e *Embeddings) embedBatch(ctx context.Context, batch []string) ([][]float64, error) {
	var response struct {
		Model string `json:"model"`
		Data  []struct {
			Index  *int      `json:"index"`
			Vector []float64 `json:"embedding"`
		} `json:"data"`
	}
	payload := struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}{e.config.Model, batch}
	if err := request(ctx, e.config, "/embeddings", payload, &response); err != nil {
		return nil, err
	}
	if (response.Model != "" && response.Model != e.config.Model) || len(response.Data) != len(batch) {
		return nil, fmt.Errorf("invalid embedding model or vector count")
	}
	vectors := make([][]float64, len(batch))
	dimensions := len(response.Data[0].Vector)
	for _, entry := range response.Data {
		if entry.Index == nil || *entry.Index < 0 || *entry.Index >= len(vectors) ||
			vectors[*entry.Index] != nil || dimensions == 0 || len(entry.Vector) != dimensions {
			return nil, fmt.Errorf("invalid embedding index or dimensions")
		}
		vectors[*entry.Index] = entry.Vector
	}
	return vectors, nil
}
