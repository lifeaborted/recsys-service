package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"recsys/internal/domain"
	"recsys/internal/enrichment"
)

// PublishOnlineRelations creates a derived online snapshot without switching the
// active historical snapshot or overwriting its base.
func (s *Store) PublishOnlineRelations(ctx context.Context, base, target string, reviewed enrichment.Result) error {
	if !strings.HasPrefix(base, "v5-online-") || !strings.HasPrefix(target, "v5-online-") ||
		base == target || reviewed.SnapshotID != target || reviewed.Feed.ID == "" ||
		len(reviewed.Relations) == 0 || len(reviewed.Rejected) != 0 {
		return fmt.Errorf("supply distinct v5-online-* snapshot IDs and a verified artifact for the target with no rejected rules")
	}
	items, err := s.OnlineRelationItems(ctx, base)
	if err != nil {
		return err
	}
	available := make(map[domain.ItemID]string, len(items))
	for _, item := range items {
		available[item.ID] = item.OfferID
	}
	selected := make(map[domain.ItemID]string)
	for _, match := range reviewed.Matches {
		if match.Status == "selected" {
			if _, found := selected[match.ItemID]; found {
				return fmt.Errorf("duplicate evidence match for item %s", match.ItemID)
			}
			selected[match.ItemID] = match.OfferID
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		INSERT INTO recommendations.model_snapshots
		    (snapshot_id, dataset_version, workbook_sha256, catalog_version, feature_version, train_through, is_active)
		SELECT $2, dataset_version, workbook_sha256, catalog_version, feature_version, train_through, false
		FROM recommendations.model_snapshots
		WHERE snapshot_id=$1 AND dataset_version='v5' AND NOT is_active AND catalog_version=$3
	`, base, target, reviewed.Feed.ID)
	if err != nil {
		return fmt.Errorf("create online snapshot: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("evidence catalog version must match the base snapshot")
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO recommendations.evidence_records
		    (snapshot_id, evidence_id, from_node, to_node, signal_type, weight, reason, source,
		     source_url, source_quote, source_version, observed_at, constraints)
		SELECT $2, evidence_id, from_node, to_node, signal_type, weight, reason, source,
		       source_url, source_quote, source_version, observed_at, constraints
		FROM recommendations.evidence_records WHERE snapshot_id=$1
	`, base, target); err != nil {
		return fmt.Errorf("copy base evidence: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO recommendations.snapshot_candidates
		    (snapshot_id, signal, source_item_id, client_group, client_type, industry, segment,
		     rank, item_id, recommendation_kind, score, evidence_ids)
		SELECT $2, signal, source_item_id, client_group, client_type, industry, segment,
		       rank, item_id, recommendation_kind, score, evidence_ids
		FROM recommendations.snapshot_candidates WHERE snapshot_id=$1
	`, base, target); err != nil {
		return fmt.Errorf("copy base candidates: %w", err)
	}

	for _, relation := range reviewed.Relations {
		edge, source := relation.Evidence, relation.Source
		from, hasFrom := strings.CutPrefix(edge.From, "item:")
		to, hasTo := strings.CutPrefix(edge.To, "item:")
		fromOffer, toOffer := available[domain.ItemID(from)], available[domain.ItemID(to)]
		kind := domain.RecommendationKind(edge.Type)
		if relation.Status != "confirmed" || edge.SnapshotID != target || strings.TrimSpace(edge.ID) == "" ||
			!hasFrom || !hasTo || fromOffer == "" || toOffer == "" || fromOffer == toOffer ||
			selected[domain.ItemID(from)] != fromOffer || selected[domain.ItemID(to)] != toOffer ||
			source.URL == "" || source.Version == "" || source.ObservedAt.IsZero() ||
			edge.Source == "" || source.Quote == "" || edge.Reason != source.Quote ||
			edge.Weight != 1 || (kind != domain.Upgrade && kind != domain.Complement && kind != domain.Renewal) {
			return fmt.Errorf("invalid confirmed online relation %q", edge.ID)
		}
		id := "reviewed/" + edge.ID
		constraints, err := json.Marshal(relation.Constraints)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO recommendations.evidence_records
			    (snapshot_id, evidence_id, from_node, to_node, signal_type, weight, reason, source,
			     source_url, source_quote, source_version, observed_at, constraints)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		`, target, id, edge.From, edge.To, edge.Type, edge.Weight, edge.Reason, edge.Source,
			source.URL, source.Quote, source.Version, source.ObservedAt, constraints); err != nil {
			return fmt.Errorf("insert reviewed evidence %q: %w", edge.ID, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO recommendations.snapshot_candidates AS candidate
			    (snapshot_id, signal, source_item_id, rank, item_id, recommendation_kind, score, evidence_ids)
			VALUES ($1,$2,$3,1,$4,$2,$5,jsonb_build_array($6::text))
			ON CONFLICT (snapshot_id, signal, source_item_id, client_group, client_type, industry, segment, item_id)
			DO UPDATE SET score=greatest(candidate.score, EXCLUDED.score),
			              evidence_ids=candidate.evidence_ids || EXCLUDED.evidence_ids
		`, target, edge.Type, from, to, edge.Weight, id); err != nil {
			return fmt.Errorf("insert reviewed candidate %q: %w", edge.ID, err)
		}
	}
	if _, err := tx.Exec(ctx, `
		WITH ranked AS (
		    SELECT snapshot_id, signal, source_item_id, item_id,
		           row_number() OVER (PARTITION BY signal, source_item_id ORDER BY score DESC, item_id COLLATE "C") AS position
		    FROM recommendations.snapshot_candidates
		    WHERE snapshot_id=$1 AND signal IN ('upgrade','complement','renewal')
		      AND client_group='' AND client_type='' AND industry='' AND segment=''
		)
		UPDATE recommendations.snapshot_candidates AS c SET rank=r.position
		FROM ranked r
		WHERE c.snapshot_id=r.snapshot_id AND c.signal=r.signal AND c.source_item_id=r.source_item_id AND c.item_id=r.item_id
		  AND c.client_group='' AND c.client_type='' AND c.industry='' AND c.segment='' AND c.rank<>r.position
	`, target); err != nil {
		return fmt.Errorf("rank online relations: %w", err)
	}
	return tx.Commit(ctx)
}
