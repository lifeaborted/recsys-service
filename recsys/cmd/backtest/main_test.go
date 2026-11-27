package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/eval"
)

type stubHybridRecommender struct {
	responses map[domain.ClientID]contract.RecommendationResult
}

func (s *stubHybridRecommender) Recommend(_ context.Context, query contract.RecommendationQuery) (contract.RecommendationResult, error) {
	if res, ok := s.responses[query.ClientID]; ok {
		return res, nil
	}
	return contract.RecommendationResult{SnapshotID: query.SnapshotID}, nil
}

func assertNoSensitivePublicJSONLeaks(t *testing.T, body string) {
	t.Helper()
	forbidden := []string{
		"SYN-A1",
		"SYN-B1",
		"SYN-B2",
		"SYN-P1",
		"I-COLD",
		"cold_items\"",
		"\"cold_items\"",
		"secret explanation",
		"Base Suite",
		"Cold Tool",
	}
	for _, token := range forbidden {
		if strings.Contains(body, token) {
			t.Fatalf("public JSON leaked forbidden token %q", token)
		}
	}
}

func TestBacktestPublicJSONGoldenCLI(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "backtest-test-bin")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-mod=readonly", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build cmd/backtest failed: %v\n%s", err, out)
	}

	fixtureDir := filepath.Join("..", "..", "testdata", "public-eval")
	cmd := exec.Command(
		bin,
		"-clients", filepath.Join(fixtureDir, "clients.csv"),
		"-sales", filepath.Join(fixtureDir, "sales.csv"),
		"-label", "demo",
		"-format", "json",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("backtest CLI failed: %v\nstderr=%s", err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("public JSON mode must emit zero bytes to stderr, got:\n%s", stderr.String())
	}

	wantBytes, err := os.ReadFile(filepath.Join(fixtureDir, "expected-cli-public.json"))
	if err != nil {
		t.Fatalf("read golden CLI JSON: %v", err)
	}
	got := strings.ReplaceAll(stdout.String(), "\r\n", "\n")
	want := strings.ReplaceAll(string(wantBytes), "\r\n", "\n")
	if got != want {
		t.Fatalf("CLI public JSON mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	assertNoSensitivePublicJSONLeaks(t, got)
}

func TestBacktestPublicJSONGoldenWithHybrid(t *testing.T) {
	ctx := context.Background()
	fixtureDir := filepath.Join("..", "..", "testdata", "public-eval")
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	rec := &stubHybridRecommender{
		responses: map[domain.ClientID]contract.RecommendationResult{
			"SYN-A1": {
				SnapshotID: "demo-snap-v1",
				Items: []domain.Recommendation{
					{ItemID: "I-VAR", Title: "Base Edition Upgrade", Explanation: "secret explanation must never appear"},
					{ItemID: "feed:outside-sales", Title: "Outside feed offer", Explanation: "secret explanation must never appear"},
					{ItemID: "I-NEW", Title: "New Cloud Backup", Explanation: "secret explanation must never appear"},
					{ItemID: "I-EXTRA", Title: "Extra 4th card truncated", Explanation: "secret explanation must never appear"},
				},
			},
			"SYN-B1": {
				SnapshotID: "demo-snap-v1",
				Items: []domain.Recommendation{
					{ItemID: "I-VAR", Explanation: "secret explanation must never appear"},
					{ItemID: "I-EXTRA", Explanation: "secret explanation must never appear"},
				},
			},
			"SYN-B2": {
				SnapshotID: "demo-snap-v1",
				Items: []domain.Recommendation{
					{ItemID: "I-COLD", Explanation: "secret explanation must never appear"},
				},
			},
		},
	}

	res, err := evaluateCSV(
		ctx,
		logger,
		filepath.Join(fixtureDir, "clients.csv"),
		filepath.Join(fixtureDir, "sales.csv"),
		"demo",
		false,
		"demo-snap-v1",
		rec,
	)
	if err != nil {
		t.Fatalf("evaluateCSV with hybrid stub failed: %v", err)
	}

	var buf bytes.Buffer
	if err := printOutput(&buf, res, false, "json"); err != nil {
		t.Fatalf("printOutput json failed: %v", err)
	}

	wantBytes, err := os.ReadFile(filepath.Join(fixtureDir, "expected-hybrid-public.json"))
	if err != nil {
		t.Fatalf("read golden hybrid JSON: %v", err)
	}
	got := strings.ReplaceAll(buf.String(), "\r\n", "\n")
	want := strings.ReplaceAll(string(wantBytes), "\r\n", "\n")
	if got != want {
		t.Fatalf("hybrid public JSON mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	assertNoSensitivePublicJSONLeaks(t, got)
}

func TestBacktestShowColdRejectedInPublicJSON(t *testing.T) {
	fixtureDir := filepath.Join("..", "..", "testdata", "public-eval")
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	var buf bytes.Buffer
	err := run(
		context.Background(),
		logger,
		&buf,
		filepath.Join(fixtureDir, "clients.csv"),
		filepath.Join(fixtureDir, "sales.csv"),
		"demo",
		false,
		true,
		"json",
		"",
		"",
	)
	if err == nil {
		t.Fatalf("expected error when -show-cold-items is combined with -format json")
	}
	if buf.Len() != 0 {
		t.Fatalf("expected empty stdout on error, got %q", buf.String())
	}
}

func TestPublicMetricsDocumentHelperCompile(t *testing.T) {
	// Sanity-check eval.V5PublicAggregateDocument accepts a synthetic public report without mutating its cells.
	doc := eval.V5PublicAggregateDocument(nil)
	if err := eval.ValidateV5PublicAggregateDocument(doc); err != nil {
		t.Fatalf("V5PublicAggregateDocument invalid: %v", err)
	}
}
