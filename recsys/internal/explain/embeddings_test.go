package explain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

func TestEmbeddingBatchesRetryAndValidation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string
			Input []string
		}
		if r.URL.Path != "/v1/embeddings" || r.Header.Get("Authorization") != "Bearer test" ||
			json.NewDecoder(r.Body).Decode(&body) != nil || body.Model != "versioned-model" || len(body.Input) > 2 {
			t.Error("unexpected embedding request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch calls.Add(1) {
		case 1:
			fmt.Fprint(w, `{"model":"versioned-model","data":[{"index":1,"embedding":[2,0]},{"index":0,"embedding":[1,0]}]}`)
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 3:
			fmt.Fprint(w, `{"model":"versioned-model","data":[{"index":0,"embedding":[3,0]}]}`)
		default:
			t.Error("completed batches should be reused")
		}
	}))
	defer server.Close()
	makeInput := func(value string) PublicText {
		return PublicText{Text: value, SourceURL: "https://example.com/catalog", Version: "v1", Approved: true}
	}
	inputs := []PublicText{makeInput("one"), makeInput("two"), makeInput("three"), makeInput("one")}
	adapter, err := NewEmbeddings(Config{BaseURL: server.URL + "/v1", APIKey: "test", Model: "versioned-model"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Embed(context.Background(), inputs); err == nil {
		t.Fatal("expected provider failure")
	}
	result, err := adapter.Embed(context.Background(), inputs)
	if err != nil {
		t.Fatal(err)
	}
	for i, number := range []float64{1, 2, 3, 1} {
		got := result[i]
		if !slices.Equal(got.Vector, []float64{number, 0}) || got.InputHash != InputHash(inputs[i].Text) ||
			got.Model != "versioned-model" || got.Dimensions != 2 {
			t.Fatalf("bad result %+v", got)
		}
	}
	result[0].Vector[0] = 9
	repeat, err := adapter.Embed(context.Background(), inputs)
	if err != nil || repeat[0].Vector[0] != 1 || calls.Load() != 3 {
		t.Fatalf("bad cache hit: %v, calls=%d", err, calls.Load())
	}
	inputs[3].Approved = false
	if _, err := adapter.Embed(context.Background(), inputs); err == nil || calls.Load() != 3 {
		t.Fatal("unapproved input should never be sent")
	}
}

func TestChatDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(250 * time.Millisecond):
			fmt.Fprint(w, `{"choices":[{"message":{"content":"1"}}]}`)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	chat := &Chat{Config: Config{BaseURL: server.URL, Model: "test"}}
	_, err := chat.Complete(ctx, PublicText{Text: "public", SourceURL: "https://example.com", Version: "v1", Approved: true})
	if err != context.DeadlineExceeded {
		t.Fatalf("got %v", err)
	}
}
