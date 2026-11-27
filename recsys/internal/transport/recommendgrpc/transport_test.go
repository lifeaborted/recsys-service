package recommendgrpc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"reflect"
	"testing"
	"time"

	recommendationv1 "recsys/api/recommendation/v1"
	"recsys/internal/config"
	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/logger"
	"recsys/internal/store/postgres"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type recommenderFunc func(context.Context, contract.RecommendationQuery) (contract.RecommendationResult, error)

func (fn recommenderFunc) Recommend(ctx context.Context, query contract.RecommendationQuery) (contract.RecommendationResult, error) {
	return fn(ctx, query)
}

func TestRecommendationRPC(t *testing.T) {
	price := 0.0
	sku, family, reason, url := "", "suite", "insufficient_eligible_products", "https://example.com/upgrade"
	want := contract.RecommendationResult{
		SnapshotID: "demo-v1", PartialReason: &reason,
		Items: []domain.Recommendation{{
			ItemID: "demo-upgrade", SKU: &sku, FamilyID: &family, Kind: domain.Upgrade,
			Title: "Апгрейд", URL: &url, Price: &price, Score: 0.75, Explanation: "Compatible",
			EvidenceIDs: []string{"edge-1"}, Status: "review",
			Variants: []domain.Variant{{ItemID: "demo-variant", Differences: []domain.Difference{
				{Field: "term", Value: 12.0, Unit: "month", Source: "feed"},
				{Field: "expires_at", Value: nil, Unit: "date", Source: "feed"},
			}}},
		}},
		Graph: domain.Graph{
			Nodes: []domain.Node{{ID: "item:demo-base", Type: "product", Label: "Base"}, {ID: "item:demo-upgrade", Type: "product", Label: "Upgrade"}},
			Edges: []domain.Evidence{{
				ID: "edge-1", From: "item:demo-base", To: "item:demo-upgrade", Type: "upgrade",
				Weight: 0.81, Reason: "Compatible", Source: "feed", SnapshotID: "demo-v1",
			}},
		},
	}
	engine := recommenderFunc(func(ctx context.Context, query contract.RecommendationQuery) (contract.RecommendationResult, error) {
		switch query.ClientID {
		case "slow":
			<-ctx.Done()
			return contract.RecommendationResult{}, ctx.Err()
		case "missing":
			return contract.RecommendationResult{}, postgres.ErrNotFound
		case "failure":
			return contract.RecommendationResult{}, errors.New("internal details")
		}
		ids := metadata.ValueFromIncomingContext(ctx, requestIDKey)
		if query != (contract.RecommendationQuery{ClientID: "demo-client", Mode: contract.Online, Limit: 36}) ||
			!reflect.DeepEqual(ids, []string{"req-123"}) {
			return contract.RecommendationResult{}, errors.New("unexpected request parameters or metadata")
		}
		return want, nil
	})

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	recommendationv1.RegisterRecommendationServiceServer(server, NewServer(engine, 250*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil))))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	client := NewClient(conn)
	got, err := client.Recommend(logger.WithRequestID(ctx, "req-123"), contract.RecommendationQuery{
		ClientID: "demo-client", Mode: contract.Online, Limit: 36,
	})
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("gRPC round trip got %+v (error: %v), want %+v", got, err, want)
	}

	direct := recommendationv1.NewRecommendationServiceClient(conn)
	for _, tc := range []struct {
		name    string
		request *recommendationv1.GetRecommendationsRequest
		code    codes.Code
	}{
		{"empty client", &recommendationv1.GetRecommendationsRequest{}, codes.InvalidArgument},
		{"oversize limit", &recommendationv1.GetRecommendationsRequest{ClientId: "demo-client", Limit: 37}, codes.InvalidArgument},
		{"missing client", &recommendationv1.GetRecommendationsRequest{ClientId: "missing"}, codes.NotFound},
		{"engine error", &recommendationv1.GetRecommendationsRequest{ClientId: "failure"}, codes.Internal},
		{"server timeout", &recommendationv1.GetRecommendationsRequest{ClientId: "slow"}, codes.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := direct.GetRecommendations(ctx, tc.request)
			if status.Code(err) != tc.code {
				t.Fatalf("got %v, want %s", err, tc.code)
			}
			if tc.code == codes.Internal && status.Convert(err).Message() != "failed to compute recommendations" {
				t.Fatalf("unexpected error details: %v", err)
			}
		})
	}
}

func TestPlaintextGRPCRequiresLoopback(t *testing.T) {
	if _, err := ServerCredentials(config.GRPCConfig{ListenAddr: "127.0.0.1:50051"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ServerCredentials(config.GRPCConfig{ListenAddr: "0.0.0.0:50051"}); err == nil {
		t.Fatal("plaintext server accepted a non-loopback listener")
	}
	if _, err := ClientCredentials(config.GRPCConfig{Target: "recommend.example.com:50051"}); err == nil {
		t.Fatal("plaintext client accepted a non-loopback target")
	}
}
