package main

import (
	"context"
	"fmt"
	"log"

	caasv1 "github.com/OpenAGX/caas/gen/go/caas/v1"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Neo4jStore manages the trust graph in Neo4j.
type Neo4jStore struct {
	driver neo4j.DriverWithContext
}

func NewNeo4jStore(ctx context.Context, uri, user, password string) (*Neo4jStore, error) {
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, password, ""))
	if err != nil {
		return nil, fmt.Errorf("create neo4j driver: %w", err)
	}

	if err := driver.VerifyConnectivity(ctx); err != nil {
		return nil, fmt.Errorf("neo4j connectivity: %w", err)
	}

	store := &Neo4jStore{driver: driver}

	// Ensure schema constraints
	if err := store.initSchema(ctx); err != nil {
		log.Printf("WARN: failed to init Neo4j schema: %v", err)
	}

	return store, nil
}

func (s *Neo4jStore) Close(ctx context.Context) {
	s.driver.Close(ctx)
}

func (s *Neo4jStore) initSchema(ctx context.Context) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer session.Close(ctx)

	constraints := []string{
		"CREATE CONSTRAINT entity_id IF NOT EXISTS FOR (e:Entity) REQUIRE e.id IS UNIQUE",
	}

	for _, c := range constraints {
		if _, err := session.Run(ctx, c, nil); err != nil {
			return err
		}
	}

	return nil
}

// SetTrustScore upserts an entity node with its trust score and dimensions.
func (s *Neo4jStore) SetTrustScore(ctx context.Context, entityID string, overall int32, dims *caasv1.TrustDimensions) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer session.Close(ctx)

	_, err := session.Run(ctx,
		`MERGE (e:Entity {id: $id})
		 SET e.trustScore = $overall,
		     e.identity_verification = $iv,
		     e.behavioral_consistency = $bc,
		     e.network_reputation = $nr,
		     e.transaction_history = $th,
		     e.compliance_adherence = $ca,
		     e.temporal_stability = $ts,
		     e.peer_endorsement = $pe,
		     e.lastUpdated = datetime()`,
		map[string]any{
			"id":      entityID,
			"overall": int64(overall),
			"iv":      int64(dims.IdentityVerification),
			"bc":      int64(dims.BehavioralConsistency),
			"nr":      int64(dims.NetworkReputation),
			"th":      int64(dims.TransactionHistory),
			"ca":      int64(dims.ComplianceAdherence),
			"ts":      int64(dims.TemporalStability),
			"pe":      int64(dims.PeerEndorsement),
		},
	)
	return err
}

// GetTrustScore reads the current trust score from Neo4j.
func (s *Neo4jStore) GetTrustScore(ctx context.Context, entityID string) (*caasv1.TrustScore, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer session.Close(ctx)

	result, err := session.Run(ctx,
		`MATCH (e:Entity {id: $id})
		 WHERE e.trustScore IS NOT NULL
		 RETURN e.trustScore, e.identity_verification, e.behavioral_consistency,
		        e.network_reputation, e.transaction_history, e.compliance_adherence,
		        e.temporal_stability, e.peer_endorsement`,
		map[string]any{"id": entityID},
	)
	if err != nil {
		return nil, err
	}

	if result.Next(ctx) {
		record := result.Record()
		return &caasv1.TrustScore{
			EntityId:     entityID,
			OverallScore: int32(record.Values[0].(int64)),
			Dimensions: &caasv1.TrustDimensions{
				IdentityVerification:  safeInt32(record.Values[1]),
				BehavioralConsistency: safeInt32(record.Values[2]),
				NetworkReputation:     safeInt32(record.Values[3]),
				TransactionHistory:    safeInt32(record.Values[4]),
				ComplianceAdherence:   safeInt32(record.Values[5]),
				TemporalStability:     safeInt32(record.Values[6]),
				PeerEndorsement:       safeInt32(record.Values[7]),
			},
			CalculationReason: "neo4j_live",
		}, nil
	}

	return nil, fmt.Errorf("no trust score found for %s", entityID)
}

// CreateTrustEdge creates a TRUSTS relationship between two entities.
func (s *Neo4jStore) CreateTrustEdge(ctx context.Context, fromID, toID, relType string, weight float64) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer session.Close(ctx)

	_, err := session.Run(ctx,
		`MERGE (a:Entity {id: $from})
		 MERGE (b:Entity {id: $to})
		 MERGE (a)-[r:TRUSTS {type: $relType}]->(b)
		 SET r.weight = $weight, r.updatedAt = datetime()`,
		map[string]any{
			"from":    fromID,
			"to":      toID,
			"relType": relType,
			"weight":  weight,
		},
	)
	return err
}

// RemoveTrustEdge removes a specific trust relationship.
func (s *Neo4jStore) RemoveTrustEdge(ctx context.Context, fromID, toID, relType string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer session.Close(ctx)

	_, err := session.Run(ctx,
		`MATCH (a:Entity {id: $from})-[r:TRUSTS {type: $relType}]->(b:Entity {id: $to})
		 DELETE r`,
		map[string]any{"from": fromID, "to": toID, "relType": relType},
	)
	return err
}

// GetTrustGraph returns nodes and edges within N hops of an entity.
func (s *Neo4jStore) GetTrustGraph(ctx context.Context, entityID string, depth int) ([]*caasv1.TrustNode, []*caasv1.TrustEdge, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer session.Close(ctx)

	// Get nodes within depth hops
	nodesResult, err := session.Run(ctx,
		fmt.Sprintf(
			`MATCH path = (start:Entity {id: $id})-[:TRUSTS*1..%d]-(connected:Entity)
			 WITH collect(DISTINCT connected) + start AS allNodes
			 UNWIND allNodes AS n
			 RETURN DISTINCT n.id AS id, n.displayName AS name, n.entityType AS type, n.trustScore AS score`, depth),
		map[string]any{"id": entityID},
	)
	if err != nil {
		return nil, nil, err
	}

	nodeMap := make(map[string]bool)
	var nodes []*caasv1.TrustNode
	for nodesResult.Next(ctx) {
		r := nodesResult.Record()
		id := safeString(r.Values[0])
		if nodeMap[id] {
			continue
		}
		nodeMap[id] = true
		nodes = append(nodes, &caasv1.TrustNode{
			EntityId:    id,
			DisplayName: safeString(r.Values[1]),
			EntityType:  safeString(r.Values[2]),
			TrustScore:  safeInt32(r.Values[3]),
		})
	}

	// Get edges between those nodes
	edgesResult, err := session.Run(ctx,
		fmt.Sprintf(
			`MATCH (start:Entity {id: $id})-[:TRUSTS*1..%d]-(connected:Entity)
			 WITH collect(DISTINCT connected) + start AS allNodes
			 UNWIND allNodes AS a
			 MATCH (a)-[r:TRUSTS]->(b)
			 WHERE b IN allNodes
			 RETURN DISTINCT a.id AS source, b.id AS target, r.weight AS weight, r.type AS relType`, depth),
		map[string]any{"id": entityID},
	)
	if err != nil {
		return nil, nil, err
	}

	var edges []*caasv1.TrustEdge
	for edgesResult.Next(ctx) {
		r := edgesResult.Record()
		edges = append(edges, &caasv1.TrustEdge{
			SourceId:         safeString(r.Values[0]),
			TargetId:         safeString(r.Values[1]),
			TrustScore:       int32(safeFloat64(r.Values[2]) * 100),
			RelationshipType: safeString(r.Values[3]),
		})
	}

	return nodes, edges, nil
}

// --- Helpers ---

func safeInt32(v any) int32 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int64:
		return int32(val)
	case float64:
		return int32(val)
	default:
		return 0
	}
}

func safeString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func safeFloat64(v any) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	default:
		return 0
	}
}
