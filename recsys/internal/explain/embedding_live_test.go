package explain

import (
	"math"
	"os"
	"testing"
)

// Run against a configured local provider with RECSYS_TEST_EMBED=1.
func TestLiveCatalogEmbedding(t *testing.T) {
	if os.Getenv("RECSYS_TEST_EMBED") != "1" {
		t.Skip("set RECSYS_TEST_EMBED=1 to query the embedding server")
	}
	provider, err := EmbeddingsFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	texts := []string{
		"Instruct: Retrieve the software offer matching the requested product and license term.\nQuery: Антивирус для рабочих станций, продление на один год",
		"Антивирус для рабочих станций — продление лицензии на 12 месяцев",
		"Антивирус для рабочих станций — новая лицензия на 24 месяца",
		"Сервер резервного копирования — лицензия для центра обработки данных",
	}
	var inputs []PublicText
	for _, text := range texts {
		inputs = append(inputs, PublicText{Text: text, Approved: true, Version: "demo-v1", SourceURL: "https://example.com/demo/catalog"})
	}
	vectors, err := provider.Embed(t.Context(), inputs)
	if err != nil {
		t.Fatal(err)
	}
	scores := make([]float64, len(texts)-1)
	for i, offer := range vectors[1:] {
		var dot, qnorm, dnorm float64
		for j, d := range offer.Vector {
			q := vectors[0].Vector[j]
			dot += q * d
			qnorm += q * q
			dnorm += d * d
		}
		scores[i] = dot / math.Sqrt(qnorm*dnorm)
		t.Logf("%s: %.4f (%d dimensions)", texts[i+1], scores[i], offer.Dimensions)
	}
	if !(scores[0] > scores[1] && scores[0] > scores[2]) {
		t.Fatal("expected the matching renewal to rank first")
	}
}
