package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	caasv1 "github.com/OpenAGX/caas/gen/go/caas/v1"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PostgresStore handles trust data in PostgreSQL (endorsements + score history).
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, connString string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

// --- Trust Score History ---

func (s *PostgresStore) RecordTrustScore(ctx context.Context, entityID string, overall int32, dims *caasv1.TrustDimensions, reason string) error {
	dimsJSON, _ := json.Marshal(map[string]int32{
		"identity_verification":  dims.IdentityVerification,
		"behavioral_consistency": dims.BehavioralConsistency,
		"network_reputation":     dims.NetworkReputation,
		"transaction_history":    dims.TransactionHistory,
		"compliance_adherence":   dims.ComplianceAdherence,
		"temporal_stability":     dims.TemporalStability,
		"peer_endorsement":       dims.PeerEndorsement,
	})

	_, err := s.pool.Exec(ctx,
		`INSERT INTO trust_score_history (entity_id, overall_score, dimensions, calculation_reason)
		 VALUES ($1, $2, $3, $4)`,
		entityID, overall, dimsJSON, reason,
	)
	return err
}

func (s *PostgresStore) GetLatestTrustScore(ctx context.Context, entityID string) (*caasv1.TrustScore, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT overall_score, dimensions, calculation_reason, calculated_at
		 FROM trust_score_history
		 WHERE entity_id = $1
		 ORDER BY calculated_at DESC
		 LIMIT 1`,
		entityID,
	)

	var overall int32
	var dimsJSON []byte
	var reason string
	var calculatedAt time.Time

	if err := row.Scan(&overall, &dimsJSON, &reason, &calculatedAt); err != nil {
		return nil, err
	}

	dims := parseDimensionsJSON(dimsJSON)

	return &caasv1.TrustScore{
		EntityId:          entityID,
		OverallScore:      overall,
		Dimensions:        dims,
		CalculationReason: reason,
		LastUpdated:       timestamppb.New(calculatedAt),
	}, nil
}

func (s *PostgresStore) GetTrustHistory(ctx context.Context, entityID string, since *timestamppb.Timestamp, limit int) ([]*caasv1.TrustScore, error) {
	query := `SELECT overall_score, dimensions, calculation_reason, calculated_at
		FROM trust_score_history
		WHERE entity_id = $1`
	args := []any{entityID}

	if since != nil {
		query += ` AND calculated_at >= $2`
		args = append(args, since.AsTime())
	}

	query += ` ORDER BY calculated_at DESC LIMIT $` + fmt.Sprintf("%d", len(args)+1)
	args = append(args, limit)

	pgRows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer pgRows.Close()

	var scores []*caasv1.TrustScore
	for pgRows.Next() {
		var overall int32
		var dimsJSON []byte
		var reason string
		var calculatedAt time.Time

		if err := pgRows.Scan(&overall, &dimsJSON, &reason, &calculatedAt); err != nil {
			return nil, err
		}

		scores = append(scores, &caasv1.TrustScore{
			EntityId:          entityID,
			OverallScore:      overall,
			Dimensions:        parseDimensionsJSON(dimsJSON),
			CalculationReason: reason,
			LastUpdated:       timestamppb.New(calculatedAt),
		})
	}

	return scores, nil
}

// --- Endorsements ---

func (s *PostgresStore) CreateEndorsement(ctx context.Context, endorserID, endorsedID, endorsementType string, weight float64, evidenceHash string) (*caasv1.Endorsement, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO trust_endorsements (endorser_id, endorsed_id, endorsement_type, weight, evidence_hash)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		endorserID, endorsedID, endorsementType, weight, evidenceHash,
	)

	var id string
	var createdAt time.Time
	if err := row.Scan(&id, &createdAt); err != nil {
		return nil, err
	}

	return &caasv1.Endorsement{
		Id:              id,
		EndorserId:      endorserID,
		EndorsedId:      endorsedID,
		EndorsementType: endorsementType,
		Weight:          weight,
		EvidenceHash:    evidenceHash,
		CreatedAt:       timestamppb.New(createdAt),
	}, nil
}

func (s *PostgresStore) RevokeEndorsement(ctx context.Context, endorsementID string) (*caasv1.Endorsement, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE trust_endorsements SET revoked_at = now()
		 WHERE id = $1 AND revoked_at IS NULL
		 RETURNING id, endorser_id, endorsed_id, endorsement_type, weight, evidence_hash, created_at, revoked_at`,
		endorsementID,
	)

	var id, endorserID, endorsedID, endorsementType string
	var weight float64
	var evidenceHash *string
	var createdAt time.Time
	var revokedAt *time.Time

	if err := row.Scan(&id, &endorserID, &endorsedID, &endorsementType, &weight, &evidenceHash, &createdAt, &revokedAt); err != nil {
		return nil, err
	}

	endorsement := &caasv1.Endorsement{
		Id:              id,
		EndorserId:      endorserID,
		EndorsedId:      endorsedID,
		EndorsementType: endorsementType,
		Weight:          weight,
		CreatedAt:       timestamppb.New(createdAt),
	}
	if evidenceHash != nil {
		endorsement.EvidenceHash = *evidenceHash
	}
	if revokedAt != nil {
		endorsement.RevokedAt = timestamppb.New(*revokedAt)
	}

	return endorsement, nil
}

func (s *PostgresStore) GetEndorsementsFor(ctx context.Context, endorsedID string) ([]*caasv1.Endorsement, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, endorser_id, endorsed_id, endorsement_type, weight, evidence_hash, created_at, revoked_at
		 FROM trust_endorsements
		 WHERE endorsed_id = $1
		 ORDER BY created_at DESC`,
		endorsedID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var endorsements []*caasv1.Endorsement
	for rows.Next() {
		var id, endorserID, endorsedIDOut, endorsementType string
		var weight float64
		var evidenceHash *string
		var createdAt time.Time
		var revokedAt *time.Time

		if err := rows.Scan(&id, &endorserID, &endorsedIDOut, &endorsementType, &weight, &evidenceHash, &createdAt, &revokedAt); err != nil {
			return nil, err
		}

		e := &caasv1.Endorsement{
			Id:              id,
			EndorserId:      endorserID,
			EndorsedId:      endorsedIDOut,
			EndorsementType: endorsementType,
			Weight:          weight,
			CreatedAt:       timestamppb.New(createdAt),
		}
		if evidenceHash != nil {
			e.EvidenceHash = *evidenceHash
		}
		if revokedAt != nil {
			e.RevokedAt = timestamppb.New(*revokedAt)
		}
		endorsements = append(endorsements, e)
	}

	return endorsements, nil
}

// --- Helpers ---

func parseDimensionsJSON(data []byte) *caasv1.TrustDimensions {
	var m map[string]int32
	if err := json.Unmarshal(data, &m); err != nil {
		return &caasv1.TrustDimensions{}
	}
	return &caasv1.TrustDimensions{
		IdentityVerification:  m["identity_verification"],
		BehavioralConsistency: m["behavioral_consistency"],
		NetworkReputation:     m["network_reputation"],
		TransactionHistory:    m["transaction_history"],
		ComplianceAdherence:   m["compliance_adherence"],
		TemporalStability:     m["temporal_stability"],
		PeerEndorsement:       m["peer_endorsement"],
	}
}
