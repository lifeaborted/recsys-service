package recommendgrpc

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	recommendationv1 "recsys/api/recommendation/v1"
	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/store/postgres"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Server exposes the online recommendation engine to trusted gRPC callers.
type Server struct {
	recommendationv1.UnimplementedRecommendationServiceServer
	engine  contract.Recommender
	timeout time.Duration
	log     *slog.Logger
}

func NewServer(engine contract.Recommender, timeout time.Duration, log *slog.Logger) *Server {
	return &Server{engine: engine, timeout: timeout, log: log}
}

func (s *Server) GetRecommendations(ctx context.Context, request *recommendationv1.GetRecommendationsRequest) (*recommendationv1.GetRecommendationsResponse, error) {
	clientID := request.GetClientId()
	if strings.TrimSpace(clientID) == "" {
		return nil, status.Error(codes.InvalidArgument, "client_id is required")
	}
	limit := request.GetLimit()
	if limit < 0 || limit > contract.MaxOnlineRecommendations {
		return nil, status.Errorf(codes.InvalidArgument, "limit must be zero or between 1 and %d", contract.MaxOnlineRecommendations)
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result, err := s.engine.Recommend(ctx, contract.RecommendationQuery{
		ClientID: domain.ClientID(clientID), Mode: contract.Online, Limit: int(limit),
	})
	if ctx.Err() != nil {
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	if errors.Is(err, postgres.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "client or active snapshot not found")
	}
	if err != nil {
		s.logFailure(ctx, err)
		return nil, status.Error(codes.Internal, "failed to compute recommendations")
	}
	response, err := encodeResult(result)
	if err != nil {
		s.logFailure(ctx, err)
		return nil, status.Error(codes.Internal, "failed to encode recommendations")
	}
	return response, nil
}

func (s *Server) logFailure(ctx context.Context, err error) {
	requestID := ""
	if ids := metadata.ValueFromIncomingContext(ctx, requestIDKey); len(ids) > 0 {
		requestID = ids[0]
	}
	s.log.ErrorContext(ctx, "gRPC recommendation failed", "request_id", requestID, "error", err)
}
