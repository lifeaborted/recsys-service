package explain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"recsys/internal/domain"
)

func TestRenderUsesOnlyApprovedExcerpt(t *testing.T) {
	const publicText = "Approved catalog description."
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string
			Messages []struct{ Role, Content string }
		}
		if r.URL.Path != "/v1/chat/completions" || json.NewDecoder(r.Body).Decode(&body) != nil ||
			body.Model != "requested-model" || len(body.Messages) != 2 || body.Messages[1].Content != publicText {
			t.Error("only the approved public excerpt should appear in the user message")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"model":"actual-version","choices":[{"message":{"content":"2"}}]}`)
	}))
	defer server.Close()
	chat := &Chat{Config: Config{BaseURL: server.URL + "/v1", Model: "requested-model"}}
	input := PublicText{Text: publicText, SourceURL: "https://example.com/document", Version: "public-v1", Approved: true}
	edges := []domain.Evidence{{Reason: "Local verified relation.", Source: "local-review"}}
	variants := []domain.Variant{{ItemID: "local-item", Differences: []domain.Difference{{Field: "term", Value: 12, Unit: "month", Source: "local-feed"}}}}
	result := Render(context.Background(), chat, &input, edges, variants)
	if result.Provider != "actual-version" || result.InputHash != InputHash(publicText) || result.PromptVersion != ChatPromptVersion ||
		!strings.HasPrefix(result.Text, "local-item: 12") || !strings.HasSuffix(result.Text, "Local verified relation.") {
		t.Fatalf("unexpected rendering: %+v", result)
	}
}
