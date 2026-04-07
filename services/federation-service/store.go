package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	caasv1 "github.com/OpenAGX/caas/gen/go/caas/v1"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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

// --- Sovereign Nodes ---

func (s *PostgresStore) CreateNode(
	ctx context.Context,
	name, did, jurisdiction, endpoint, publicKey string,
	metadata *structpb.Struct,
) (*caasv1.SovereignNode, error) {
	var metaJSON []byte
	if metadata != nil {
		metaJSON, _ = metadata.MarshalJSON()
	} else {
		metaJSON = []byte("{}")
	}

	row := s.pool.QueryRow(ctx,
		`INSERT INTO sovereign_nodes (name, did, jurisdiction, endpoint, public_key, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at`,
		name, did, jurisdiction, endpoint, publicKey, metaJSON,
	)

	var id string
	var createdAt time.Time
	if err := row.Scan(&id, &createdAt); err != nil {
		return nil, err
	}

	return &caasv1.SovereignNode{
		Id:           id,
		Name:         name,
		Did:          did,
		Jurisdiction: jurisdiction,
		Endpoint:     endpoint,
		PublicKey:     publicKey,
		Status:       caasv1.SovereignNodeStatus_SOVEREIGN_NODE_STATUS_ACTIVE,
		Metadata:     metadata,
		CreatedAt:    timestamppb.New(createdAt),
	}, nil
}

func (s *PostgresStore) GetNode(ctx context.Context, id string) (*caasv1.SovereignNode, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, did, jurisdiction, endpoint, public_key, status, metadata, created_at, updated_at
		 FROM sovereign_nodes WHERE id = $1`, id,
	)

	var node caasv1.SovereignNode
	var statusStr string
	var metaJSON []byte
	var createdAt, updatedAt time.Time

	if err := row.Scan(
		&node.Id, &node.Name, &node.Did, &node.Jurisdiction,
		&node.Endpoint, &node.PublicKey, &statusStr, &metaJSON,
		&createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}

	node.Status = nodeStatusFromString(statusStr)
	node.CreatedAt = timestamppb.New(createdAt)
	node.UpdatedAt = timestamppb.New(updatedAt)

	if len(metaJSON) > 0 {
		st := &structpb.Struct{}
		if st.UnmarshalJSON(metaJSON) == nil {
			node.Metadata = st
		}
	}

	return &node, nil
}

func (s *PostgresStore) ListNodes(ctx context.Context, statusFilter *caasv1.SovereignNodeStatus, limit, offset int) ([]*caasv1.SovereignNode, int, error) {
	query := `SELECT id FROM sovereign_nodes WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM sovereign_nodes WHERE 1=1`
	args := []any{}
	argIdx := 1

	if statusFilter != nil {
		statusStr := nodeStatusToString(*statusFilter)
		query += fmt.Sprintf(` AND status = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND status = $%d`, argIdx)
		args = append(args, statusStr)
		argIdx++
	}

	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var nodes []*caasv1.SovereignNode
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		node, err := s.GetNode(ctx, id)
		if err != nil {
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes, total, nil
}

func (s *PostgresStore) UpdateNodeStatus(ctx context.Context, id string, status caasv1.SovereignNodeStatus) error {
	statusStr := nodeStatusToString(status)
	_, err := s.pool.Exec(ctx,
		`UPDATE sovereign_nodes SET status = $2, updated_at = now() WHERE id = $1`,
		id, statusStr,
	)
	return err
}

// --- Treaties ---

func (s *PostgresStore) CreateTreaty(
	ctx context.Context,
	targetNodeID string,
	trustWeight float32,
	allowedOps []string,
	validUntil *time.Time,
) (*caasv1.Treaty, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO treaties (source_node_id, target_node_id, trust_weight, allowed_operations, valid_until)
		 VALUES ((SELECT id FROM sovereign_nodes LIMIT 1), $1, $2, $3, $4)
		 RETURNING id, source_node_id, created_at`,
		targetNodeID, trustWeight, allowedOps, validUntil,
	)

	var id, sourceNodeID string
	var createdAt time.Time
	if err := row.Scan(&id, &sourceNodeID, &createdAt); err != nil {
		return nil, err
	}

	treaty := &caasv1.Treaty{
		Id:                id,
		SourceNodeId:      sourceNodeID,
		TargetNodeId:      targetNodeID,
		Status:            caasv1.TreatyStatus_TREATY_STATUS_PROPOSED,
		TrustWeight:       trustWeight,
		AllowedOperations: allowedOps,
		ValidFrom:         timestamppb.New(createdAt),
		CreatedAt:         timestamppb.New(createdAt),
	}
	if validUntil != nil {
		treaty.ValidUntil = timestamppb.New(*validUntil)
	}

	return treaty, nil
}

func (s *PostgresStore) GetTreaty(ctx context.Context, id string) (*caasv1.Treaty, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, source_node_id, target_node_id, status, trust_weight,
		        allowed_operations, valid_from, valid_until, created_at
		 FROM treaties WHERE id = $1`, id,
	)

	var treaty caasv1.Treaty
	var statusStr string
	var allowedOps []string
	var validFrom time.Time
	var validUntil *time.Time
	var createdAt time.Time

	if err := row.Scan(
		&treaty.Id, &treaty.SourceNodeId, &treaty.TargetNodeId,
		&statusStr, &treaty.TrustWeight, &allowedOps,
		&validFrom, &validUntil, &createdAt,
	); err != nil {
		return nil, err
	}

	treaty.Status = treatyStatusFromString(statusStr)
	treaty.AllowedOperations = allowedOps
	treaty.ValidFrom = timestamppb.New(validFrom)
	treaty.CreatedAt = timestamppb.New(createdAt)
	if validUntil != nil {
		treaty.ValidUntil = timestamppb.New(*validUntil)
	}

	return &treaty, nil
}

func (s *PostgresStore) ListTreaties(ctx context.Context, nodeID string, statusFilter *caasv1.TreatyStatus, limit, offset int) ([]*caasv1.Treaty, int, error) {
	query := `SELECT id FROM treaties WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM treaties WHERE 1=1`
	args := []any{}
	argIdx := 1

	if nodeID != "" {
		query += fmt.Sprintf(` AND (source_node_id = $%d OR target_node_id = $%d)`, argIdx, argIdx)
		countQuery += fmt.Sprintf(` AND (source_node_id = $%d OR target_node_id = $%d)`, argIdx, argIdx)
		args = append(args, nodeID)
		argIdx++
	}

	if statusFilter != nil {
		statusStr := treatyStatusToString(*statusFilter)
		query += fmt.Sprintf(` AND status = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND status = $%d`, argIdx)
		args = append(args, statusStr)
		argIdx++
	}

	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var treaties []*caasv1.Treaty
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		t, err := s.GetTreaty(ctx, id)
		if err != nil {
			continue
		}
		treaties = append(treaties, t)
	}

	return treaties, total, nil
}

func (s *PostgresStore) UpdateTreatyStatus(ctx context.Context, id string, status caasv1.TreatyStatus) error {
	statusStr := treatyStatusToString(status)
	_, err := s.pool.Exec(ctx,
		`UPDATE treaties SET status = $2 WHERE id = $1`,
		id, statusStr,
	)
	return err
}

func (s *PostgresStore) HasActiveTreaty(ctx context.Context, nodeID, operation string) (bool, float32, error) {
	var trustWeight float32
	err := s.pool.QueryRow(ctx,
		`SELECT trust_weight FROM treaties
		 WHERE status = 'active'
		   AND (source_node_id = $1 OR target_node_id = $1)
		   AND $2 = ANY(allowed_operations)
		   AND (valid_until IS NULL OR valid_until > now())
		 LIMIT 1`,
		nodeID, operation,
	).Scan(&trustWeight)
	if err != nil {
		return false, 0, nil // No active treaty
	}
	return true, trustWeight, nil
}

// --- Authority Overrides ---

func (s *PostgresStore) CreateAuthorityOverride(
	ctx context.Context,
	workflowID, authorityEntityID string,
	tier int,
	justification, legalReference, outcome string,
	expiresAt *time.Time,
) (*caasv1.AuthorityOverride, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO authority_overrides
		 (workflow_id, authority_entity_id, tier, justification, legal_reference, outcome, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, created_at`,
		workflowID, authorityEntityID, tier, justification, legalReference, outcome, expiresAt,
	)

	var id string
	var createdAt time.Time
	if err := row.Scan(&id, &createdAt); err != nil {
		return nil, err
	}

	override := &caasv1.AuthorityOverride{
		Id:                id,
		WorkflowId:        workflowID,
		AuthorityEntityId: authorityEntityID,
		Tier:              caasv1.AuthorityTier(tier),
		Justification:     justification,
		LegalReference:    legalReference,
		Outcome:           outcome,
		CreatedAt:         timestamppb.New(createdAt),
	}
	if expiresAt != nil {
		override.ExpiresAt = timestamppb.New(*expiresAt)
	}

	return override, nil
}

func (s *PostgresStore) GetAuthorityOverride(ctx context.Context, id string) (*caasv1.AuthorityOverride, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, workflow_id, authority_entity_id, tier, justification,
		        legal_reference, outcome, expires_at, created_at
		 FROM authority_overrides WHERE id = $1`, id,
	)

	var override caasv1.AuthorityOverride
	var tier int
	var legalRef *string
	var expiresAt *time.Time
	var createdAt time.Time

	if err := row.Scan(
		&override.Id, &override.WorkflowId, &override.AuthorityEntityId,
		&tier, &override.Justification, &legalRef,
		&override.Outcome, &expiresAt, &createdAt,
	); err != nil {
		return nil, err
	}

	override.Tier = caasv1.AuthorityTier(tier)
	override.CreatedAt = timestamppb.New(createdAt)
	if legalRef != nil {
		override.LegalReference = *legalRef
	}
	if expiresAt != nil {
		override.ExpiresAt = timestamppb.New(*expiresAt)
	}

	return &override, nil
}

func (s *PostgresStore) ListAuthorityOverrides(ctx context.Context, workflowID string, limit, offset int) ([]*caasv1.AuthorityOverride, int, error) {
	query := `SELECT id FROM authority_overrides WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM authority_overrides WHERE 1=1`
	args := []any{}
	argIdx := 1

	if workflowID != "" {
		query += fmt.Sprintf(` AND workflow_id = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND workflow_id = $%d`, argIdx)
		args = append(args, workflowID)
		argIdx++
	}

	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var overrides []*caasv1.AuthorityOverride
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		o, err := s.GetAuthorityOverride(ctx, id)
		if err != nil {
			continue
		}
		overrides = append(overrides, o)
	}

	return overrides, total, nil
}

// --- Verifiable Credentials ---

func (s *PostgresStore) IssueCredential(
	ctx context.Context,
	entityID, credentialType string,
	claims *structpb.Struct,
	proofSignature string,
	expiresAt *time.Time,
) (*caasv1.VerifiableCredential, error) {
	var claimsJSON []byte
	if claims != nil {
		claimsJSON, _ = claims.MarshalJSON()
	} else {
		claimsJSON = []byte("{}")
	}

	// Get issuer DID (for MVP, derive from entity)
	issuerDID := fmt.Sprintf("did:web:caas.local:issuer")

	row := s.pool.QueryRow(ctx,
		`INSERT INTO verifiable_credentials
		 (entity_id, issuer_did, credential_type, claims, proof_signature, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, issued_at`,
		entityID, issuerDID, credentialType, claimsJSON, proofSignature, expiresAt,
	)

	var id string
	var issuedAt time.Time
	if err := row.Scan(&id, &issuedAt); err != nil {
		return nil, err
	}

	cred := &caasv1.VerifiableCredential{
		Id:             id,
		EntityId:       entityID,
		IssuerDid:      issuerDID,
		CredentialType: credentialType,
		Claims:         claims,
		ProofSignature: proofSignature,
		IssuedAt:       timestamppb.New(issuedAt),
	}
	if expiresAt != nil {
		cred.ExpiresAt = timestamppb.New(*expiresAt)
	}

	return cred, nil
}

func (s *PostgresStore) GetCredential(ctx context.Context, id string) (*caasv1.VerifiableCredential, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, entity_id, issuer_did, credential_type, claims,
		        proof_signature, issued_at, expires_at, revoked
		 FROM verifiable_credentials WHERE id = $1`, id,
	)

	var cred caasv1.VerifiableCredential
	var claimsJSON []byte
	var issuedAt time.Time
	var expiresAt *time.Time

	if err := row.Scan(
		&cred.Id, &cred.EntityId, &cred.IssuerDid, &cred.CredentialType,
		&claimsJSON, &cred.ProofSignature, &issuedAt, &expiresAt, &cred.Revoked,
	); err != nil {
		return nil, err
	}

	cred.IssuedAt = timestamppb.New(issuedAt)
	if expiresAt != nil {
		cred.ExpiresAt = timestamppb.New(*expiresAt)
	}
	if len(claimsJSON) > 0 {
		st := &structpb.Struct{}
		if st.UnmarshalJSON(claimsJSON) == nil {
			cred.Claims = st
		}
	}

	return &cred, nil
}

func (s *PostgresStore) RevokeCredential(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE verifiable_credentials SET revoked = true WHERE id = $1`, id,
	)
	return err
}

func (s *PostgresStore) ListCredentials(ctx context.Context, entityID, credentialType string, limit, offset int) ([]*caasv1.VerifiableCredential, int, error) {
	query := `SELECT id FROM verifiable_credentials WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM verifiable_credentials WHERE 1=1`
	args := []any{}
	argIdx := 1

	if entityID != "" {
		query += fmt.Sprintf(` AND entity_id = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND entity_id = $%d`, argIdx)
		args = append(args, entityID)
		argIdx++
	}
	if credentialType != "" {
		query += fmt.Sprintf(` AND credential_type = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND credential_type = $%d`, argIdx)
		args = append(args, credentialType)
		argIdx++
	}

	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query += fmt.Sprintf(` ORDER BY issued_at DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var creds []*caasv1.VerifiableCredential
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		c, err := s.GetCredential(ctx, id)
		if err != nil {
			continue
		}
		creds = append(creds, c)
	}

	return creds, total, nil
}

// --- Local trust score lookup (for federated queries) ---

func (s *PostgresStore) GetLocalTrustScore(ctx context.Context, entityID string) (int32, error) {
	var score int32
	err := s.pool.QueryRow(ctx,
		`SELECT overall_score FROM trust_score_history
		 WHERE entity_id = $1
		 ORDER BY calculated_at DESC LIMIT 1`, entityID,
	).Scan(&score)
	return score, err
}

// --- Converters ---

func nodeStatusToString(s caasv1.SovereignNodeStatus) string {
	switch s {
	case caasv1.SovereignNodeStatus_SOVEREIGN_NODE_STATUS_ACTIVE:
		return "active"
	case caasv1.SovereignNodeStatus_SOVEREIGN_NODE_STATUS_SUSPENDED:
		return "suspended"
	case caasv1.SovereignNodeStatus_SOVEREIGN_NODE_STATUS_REVOKED:
		return "revoked"
	default:
		return "active"
	}
}

func nodeStatusFromString(s string) caasv1.SovereignNodeStatus {
	switch s {
	case "active":
		return caasv1.SovereignNodeStatus_SOVEREIGN_NODE_STATUS_ACTIVE
	case "suspended":
		return caasv1.SovereignNodeStatus_SOVEREIGN_NODE_STATUS_SUSPENDED
	case "revoked":
		return caasv1.SovereignNodeStatus_SOVEREIGN_NODE_STATUS_REVOKED
	default:
		return caasv1.SovereignNodeStatus_SOVEREIGN_NODE_STATUS_UNSPECIFIED
	}
}

func treatyStatusToString(s caasv1.TreatyStatus) string {
	switch s {
	case caasv1.TreatyStatus_TREATY_STATUS_PROPOSED:
		return "proposed"
	case caasv1.TreatyStatus_TREATY_STATUS_ACTIVE:
		return "active"
	case caasv1.TreatyStatus_TREATY_STATUS_SUSPENDED:
		return "suspended"
	case caasv1.TreatyStatus_TREATY_STATUS_TERMINATED:
		return "terminated"
	default:
		return "proposed"
	}
}

func treatyStatusFromString(s string) caasv1.TreatyStatus {
	switch s {
	case "proposed":
		return caasv1.TreatyStatus_TREATY_STATUS_PROPOSED
	case "active":
		return caasv1.TreatyStatus_TREATY_STATUS_ACTIVE
	case "suspended":
		return caasv1.TreatyStatus_TREATY_STATUS_SUSPENDED
	case "terminated":
		return caasv1.TreatyStatus_TREATY_STATUS_TERMINATED
	default:
		return caasv1.TreatyStatus_TREATY_STATUS_UNSPECIFIED
	}
}

// Keep json import alive
var _ = json.Marshal
