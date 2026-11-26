package transporthttp

import (
	"net/http"

	"recsys/internal/auth"
	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/transport/http/respond"
)

func (s *Server) handleGetRecommendationsMe(w http.ResponseWriter, r *http.Request) {
	profile, ok := auth.Current(r.Context())
	if !ok {
		respond.Error(w, r, http.StatusUnauthorized, "unauthorized", "Missing or invalid session")
		return
	}
	if s.recommender == nil {
		respond.Error(w, r, http.StatusServiceUnavailable, "model_unavailable", "Recommendation engine unavailable")
		return
	}

	res, err := s.recommender.Recommend(r.Context(), contract.RecommendationQuery{
		ClientID: profile.ClientID,
		Mode:     contract.Online,
	})
	if err != nil {
		respond.Error(w, r, http.StatusInternalServerError, "recommendation_failed", "Failed to compute recommendations")
		return
	}

	normalizeRecommendationResult(&res)
	respond.JSON(w, r, http.StatusOK, contract.RecommendationResponse{
		RequestID:            respond.RequestID(w, r),
		RecommendationResult: res,
	})
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
