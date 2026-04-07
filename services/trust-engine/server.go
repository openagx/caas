package main

import (
	"context"
	"log"

	caasv1 "github.com/OpenAGX/caas/gen/go/caas/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TrustServer implements caasv1.TrustServiceServer.
type TrustServer struct {
	caasv1.UnimplementedTrustServiceServer
	pg       *PostgresStore
	graph    *Neo4jStore
	events   *EventProducer
	mlClient *MLClient
}

func NewTrustServer(pg *PostgresStore, graph *Neo4jStore, events *EventProducer, mlClient *MLClient) *TrustServer {
	return &TrustServer{pg: pg, graph: graph, events: events, mlClient: mlClient}
}

func (s *TrustServer) GetTrustScore(ctx context.Context, req *caasv1.GetTrustScoreRequest) (*caasv1.GetTrustScoreResponse, error) {
	if req.EntityId == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}

	// Try to get from Neo4j graph first (real-time score)
	score, err := s.graph.GetTrustScore(ctx, req.EntityId)
	if err != nil {
		// Fall back to PostgreSQL history (latest)
		score, err = s.pg.GetLatestTrustScore(ctx, req.EntityId)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "no trust score for entity %s: %v", req.EntityId, err)
		}
	}

	return &caasv1.GetTrustScoreResponse{Score: score}, nil
}

func (s *TrustServer) GetTrustHistory(ctx context.Context, req *caasv1.GetTrustHistoryRequest) (*caasv1.GetTrustHistoryResponse, error) {
	if req.EntityId == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}

	var since *timestamppb.Timestamp
	if req.Since != nil {
		since = req.Since
	}

	scores, err := s.pg.GetTrustHistory(ctx, req.EntityId, since, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get trust history: %v", err)
	}

	return &caasv1.GetTrustHistoryResponse{Scores: scores}, nil
}

func (s *TrustServer) CreateEndorsement(ctx context.Context, req *caasv1.CreateEndorsementRequest) (*caasv1.CreateEndorsementResponse, error) {
	if req.EndorserId == "" || req.EndorsedId == "" {
		return nil, status.Error(codes.InvalidArgument, "endorser_id and endorsed_id are required")
	}
	if req.EndorsementType == "" {
		return nil, status.Error(codes.InvalidArgument, "endorsement_type is required")
	}
	if req.Weight <= 0 {
		req.Weight = 1.0
	}

	// Store endorsement in PostgreSQL
	endorsement, err := s.pg.CreateEndorsement(ctx, req.EndorserId, req.EndorsedId, req.EndorsementType, req.Weight, req.EvidenceHash)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create endorsement: %v", err)
	}

	// Create trust edge in Neo4j
	if err := s.graph.CreateTrustEdge(ctx, req.EndorserId, req.EndorsedId, req.EndorsementType, req.Weight); err != nil {
		log.Printf("WARN: failed to create Neo4j trust edge: %v", err)
	}

	// Recalculate trust score for the endorsed entity
	go s.recalculateTrust(context.Background(), req.EndorsedId, "endorsement_received")

	log.Printf("ENDORSEMENT created: %s -> %s (type=%s, weight=%.2f)", req.EndorserId, req.EndorsedId, req.EndorsementType, req.Weight)

	s.events.Emit("endorsement_created", map[string]any{
		"endorser_id":      req.EndorserId,
		"endorsed_id":      req.EndorsedId,
		"endorsement_type": req.EndorsementType,
	})

	return &caasv1.CreateEndorsementResponse{Endorsement: endorsement}, nil
}

func (s *TrustServer) RevokeEndorsement(ctx context.Context, req *caasv1.RevokeEndorsementRequest) (*caasv1.RevokeEndorsementResponse, error) {
	if req.EndorsementId == "" {
		return nil, status.Error(codes.InvalidArgument, "endorsement_id is required")
	}

	endorsement, err := s.pg.RevokeEndorsement(ctx, req.EndorsementId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "revoke endorsement: %v", err)
	}

	// Remove trust edge from Neo4j
	if err := s.graph.RemoveTrustEdge(ctx, endorsement.EndorserId, endorsement.EndorsedId, endorsement.EndorsementType); err != nil {
		log.Printf("WARN: failed to remove Neo4j trust edge: %v", err)
	}

	// Recalculate trust score
	go s.recalculateTrust(context.Background(), endorsement.EndorsedId, "endorsement_revoked")

	log.Printf("ENDORSEMENT revoked: %s", req.EndorsementId)

	s.events.Emit("endorsement_revoked", map[string]any{
		"endorsement_id": req.EndorsementId,
		"endorsed_id":    endorsement.EndorsedId,
	})

	return &caasv1.RevokeEndorsementResponse{Endorsement: endorsement}, nil
}

func (s *TrustServer) GetTrustGraph(ctx context.Context, req *caasv1.GetTrustGraphRequest) (*caasv1.GetTrustGraphResponse, error) {
	if req.EntityId == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}

	depth := int(req.Depth)
	if depth <= 0 {
		depth = 2
	}
	if depth > 5 {
		depth = 5
	}

	nodes, edges, err := s.graph.GetTrustGraph(ctx, req.EntityId, depth)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get trust graph: %v", err)
	}

	return &caasv1.GetTrustGraphResponse{Nodes: nodes, Edges: edges}, nil
}

func (s *TrustServer) VerifyTrust(ctx context.Context, req *caasv1.VerifyTrustRequest) (*caasv1.VerifyTrustResponse, error) {
	if req.EntityId == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}

	// Get current trust score
	scoreResp, err := s.GetTrustScore(ctx, &caasv1.GetTrustScoreRequest{EntityId: req.EntityId})
	if err != nil {
		return nil, err
	}

	score := scoreResp.Score
	var failedDimensions []string
	meets := true

	// Check overall minimum
	if req.MinimumScore > 0 && score.OverallScore < req.MinimumScore {
		meets = false
	}

	// Check per-dimension minimums
	if score.Dimensions != nil {
		dimMap := map[string]int32{
			"identity_verification":  score.Dimensions.IdentityVerification,
			"behavioral_consistency": score.Dimensions.BehavioralConsistency,
			"network_reputation":     score.Dimensions.NetworkReputation,
			"transaction_history":    score.Dimensions.TransactionHistory,
			"compliance_adherence":   score.Dimensions.ComplianceAdherence,
			"temporal_stability":     score.Dimensions.TemporalStability,
			"peer_endorsement":       score.Dimensions.PeerEndorsement,
		}

		for dimName, minVal := range req.MinimumDimensions {
			actual, ok := dimMap[dimName]
			if !ok || actual < minVal {
				meets = false
				failedDimensions = append(failedDimensions, dimName)
			}
		}
	}

	s.events.Emit("trust_verified", map[string]any{
		"entity_id":         req.EntityId,
		"meets_requirements": meets,
		"failed_dimensions": failedDimensions,
	})

	return &caasv1.VerifyTrustResponse{
		MeetsRequirements: meets,
		CurrentScore:      score,
		FailedDimensions:  failedDimensions,
	}, nil
}

// recalculateTrust recomputes the trust score for an entity based on
// endorsements, graph position, and ML scoring.
func (s *TrustServer) recalculateTrust(ctx context.Context, entityID, reason string) {
	// Get all active endorsements for this entity
	endorsements, err := s.pg.GetEndorsementsFor(ctx, entityID)
	if err != nil {
		log.Printf("ERROR: recalculate trust for %s: %v", entityID, err)
		return
	}

	// Calculate base score from endorsements
	dims := calculateDimensionsFromEndorsements(endorsements)

	// Try ML sidecar for enhanced scoring
	mlDims, err := s.mlClient.ScoreTrust(ctx, entityID)
	if err == nil && mlDims != nil {
		// Blend ML scores with endorsement-based scores (60% ML, 40% endorsement)
		dims = blendDimensions(dims, mlDims, 0.4, 0.6)
	}

	overall := computeOverallScore(dims)

	// Store in Neo4j
	if err := s.graph.SetTrustScore(ctx, entityID, overall, dims); err != nil {
		log.Printf("WARN: failed to set Neo4j trust score: %v", err)
	}

	// Store in PostgreSQL history
	if err := s.pg.RecordTrustScore(ctx, entityID, overall, dims, reason); err != nil {
		log.Printf("WARN: failed to record trust history: %v", err)
	}

	log.Printf("TRUST recalculated: entity=%s overall=%d reason=%s", entityID, overall, reason)

	s.events.Emit("trust_recalculated", map[string]any{
		"entity_id":     entityID,
		"overall_score": overall,
		"reason":        reason,
	})
}

// calculateDimensionsFromEndorsements computes dimension scores from endorsements.
func calculateDimensionsFromEndorsements(endorsements []*caasv1.Endorsement) *caasv1.TrustDimensions {
	dims := &caasv1.TrustDimensions{}

	// Map endorsement types to dimensions with their max values
	typeToMax := map[string]struct {
		field string
		max   int32
	}{
		"identity":    {"identity_verification", 150},
		"behavior":    {"behavioral_consistency", 200},
		"reputation":  {"network_reputation", 150},
		"transaction": {"transaction_history", 150},
		"compliance":  {"compliance_adherence", 100},
		"temporal":    {"temporal_stability", 100},
		"peer":        {"peer_endorsement", 150},
	}

	// Accumulate weighted endorsements per dimension
	dimScores := make(map[string]float64)
	dimCounts := make(map[string]int)

	for _, e := range endorsements {
		if e.RevokedAt != nil {
			continue
		}
		if info, ok := typeToMax[e.EndorsementType]; ok {
			dimScores[info.field] += e.Weight * float64(info.max)
			dimCounts[info.field]++
		} else {
			// Generic endorsement boosts peer_endorsement
			dimScores["peer_endorsement"] += e.Weight * 50
			dimCounts["peer_endorsement"]++
		}
	}

	// Average and clamp
	for field, total := range dimScores {
		count := dimCounts[field]
		if count == 0 {
			continue
		}
		avg := int32(total / float64(count))
		switch field {
		case "identity_verification":
			dims.IdentityVerification = clamp(avg, 0, 150)
		case "behavioral_consistency":
			dims.BehavioralConsistency = clamp(avg, 0, 200)
		case "network_reputation":
			dims.NetworkReputation = clamp(avg, 0, 150)
		case "transaction_history":
			dims.TransactionHistory = clamp(avg, 0, 150)
		case "compliance_adherence":
			dims.ComplianceAdherence = clamp(avg, 0, 100)
		case "temporal_stability":
			dims.TemporalStability = clamp(avg, 0, 100)
		case "peer_endorsement":
			dims.PeerEndorsement = clamp(avg, 0, 150)
		}
	}

	return dims
}

// computeOverallScore sums all dimensions (max 1000).
func computeOverallScore(dims *caasv1.TrustDimensions) int32 {
	if dims == nil {
		return 0
	}
	total := dims.IdentityVerification +
		dims.BehavioralConsistency +
		dims.NetworkReputation +
		dims.TransactionHistory +
		dims.ComplianceAdherence +
		dims.TemporalStability +
		dims.PeerEndorsement
	return clamp(total, 0, 1000)
}

// blendDimensions combines two dimension sets with weights.
func blendDimensions(a, b *caasv1.TrustDimensions, wA, wB float64) *caasv1.TrustDimensions {
	blend := func(va, vb int32) int32 {
		return int32(float64(va)*wA + float64(vb)*wB)
	}
	return &caasv1.TrustDimensions{
		IdentityVerification:  clamp(blend(a.IdentityVerification, b.IdentityVerification), 0, 150),
		BehavioralConsistency: clamp(blend(a.BehavioralConsistency, b.BehavioralConsistency), 0, 200),
		NetworkReputation:     clamp(blend(a.NetworkReputation, b.NetworkReputation), 0, 150),
		TransactionHistory:    clamp(blend(a.TransactionHistory, b.TransactionHistory), 0, 150),
		ComplianceAdherence:   clamp(blend(a.ComplianceAdherence, b.ComplianceAdherence), 0, 100),
		TemporalStability:     clamp(blend(a.TemporalStability, b.TemporalStability), 0, 100),
		PeerEndorsement:       clamp(blend(a.PeerEndorsement, b.PeerEndorsement), 0, 150),
	}
}

func clamp(v, min, max int32) int32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
