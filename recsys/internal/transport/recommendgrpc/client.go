package recommendgrpc

import (
	"context"
	"fmt"

	recommendationv1 "recsys/api/recommendation/v1"
	"recsys/internal/contract"
	"recsys/internal/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const requestIDKey = "x-request-id"

// Client implements the HTTP server's recommendation dependency over a shared gRPC connection.
type Client struct {
	rpc recommendationv1.RecommendationServiceClient
}

var _ contract.Recommender = (*Client)(nil)

func NewClient(conn grpc.ClientConnInterface) *Client {
	return &Client{rpc: recommendationv1.NewRecommendationServiceClient(conn)}
}

func (c *Client) Recommend(ctx context.Context, query contract.RecommendationQuery) (contract.RecommendationResult, error) {
	if query.Mode != contract.Online || query.SnapshotID != "" {
		return contract.RecommendationResult{}, fmt.Errorf("gRPC serves online recommendations from the active snapshot only")
	}
	if query.Limit < 0 || query.Limit > contract.MaxOnlineRecommendations {
		return contract.RecommendationResult{}, fmt.Errorf("invalid online recommendation limit: %d", query.Limit)
	}
	if requestID := logger.RequestIDFromContext(ctx); requestID != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, requestIDKey, requestID)
	}
	response, err := c.rpc.GetRecommendations(ctx, &recommendationv1.GetRecommendationsRequest{
		ClientId: string(query.ClientID), Limit: int32(query.Limit),
	})
	if err != nil {
		return contract.RecommendationResult{}, err
	}
	return decodeResult(response), nil
}
