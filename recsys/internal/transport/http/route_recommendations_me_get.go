package transporthttp

import (
	"net/http"
	"strconv"

	"recsys/internal/auth"
	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/transport/http/respond"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) handleGetRecommendationsMe(w http.ResponseWriter, r *http.Request) {
	profile, ok := auth.Current(r.Context())
	if !ok {
		respond.Error(w, r, http.StatusUnauthorized, "unauthorized", "Missing or invalid session")
		return
	}
	result, ok := s.recommendationsFor(w, r, profile.ClientID)
	if ok {
		respond.JSON(w, r, http.StatusOK, result)
	}
}

func (s *Server) handleGetGuestRecommendations(w http.ResponseWriter, r *http.Request) {
	result, ok := s.recommendationsFor(w, r, "DEMO-GUEST")
	if ok {
		respond.JSON(w, r, http.StatusOK, result)
	}
}

func (s *Server) recommendationsFor(w http.ResponseWriter, r *http.Request, clientID domain.ClientID) (contract.RecommendationResponse, bool) {
	var zero contract.RecommendationResponse
	if s.recommender == nil {
		respond.Error(w, r, http.StatusServiceUnavailable, "model_unavailable", "Recommendation engine unavailable")
		return zero, false
	}

	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > contract.MaxOnlineRecommendations {
			respond.Error(w, r, http.StatusBadRequest, "invalid_limit", "limit must be an integer between 1 and 36")
			return zero, false
		}
		limit = parsed
	}

	res, err := s.recommender.Recommend(r.Context(), contract.RecommendationQuery{
		ClientID: clientID,
		Mode:     contract.Online,
		Limit:    limit,
	})
	if err != nil {
		switch status.Code(err) {
		case codes.NotFound:
			respond.Error(w, r, http.StatusNotFound, "recommendation_not_found", "Recommendation profile or snapshot not found")
		case codes.Unavailable:
			respond.Error(w, r, http.StatusServiceUnavailable, "model_unavailable", "Recommendation service unavailable")
		case codes.DeadlineExceeded:
			respond.Error(w, r, http.StatusGatewayTimeout, "recommendation_timeout", "Recommendation service timed out")
		default:
			respond.Error(w, r, http.StatusInternalServerError, "recommendation_failed", "Failed to compute recommendations")
		}
		return zero, false
	}

	normalizeRecommendationResult(&res)
	return contract.RecommendationResponse{
		RequestID:            respond.RequestID(w, r),
		RecommendationResult: res,
	}, true
}

func normalizeRecommendationResult(res *contract.RecommendationResult) {
	if res.Items == nil {
		res.Items = []domain.Recommendation{}
	}
	for i := range res.Items {
		if res.Items[i].Variants == nil {
			res.Items[i].Variants = []domain.Variant{}
		}
		for j := range res.Items[i].Variants {
			if res.Items[i].Variants[j].Differences == nil {
				res.Items[i].Variants[j].Differences = []domain.Difference{}
			}
		}
		if res.Items[i].EvidenceIDs == nil {
			res.Items[i].EvidenceIDs = []string{}
		}
	}
	if res.Graph.Nodes == nil {
		res.Graph.Nodes = []domain.Node{}
	}
	if res.Graph.Edges == nil {
		res.Graph.Edges = []domain.Evidence{}
	}
}
