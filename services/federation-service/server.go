package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"time"

	caasv1 "github.com/OpenAGX/caas/gen/go/caas/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type FederationServer struct {
	caasv1.UnimplementedFederationServiceServer
	store  *PostgresStore
	events *EventProducer
}

func NewFederationServer(store *PostgresStore, events *EventProducer) *FederationServer {
	return &FederationServer{store: store, events: events}
}

// --- Sovereign Node Management ---

func (s *FederationServer) RegisterNode(ctx context.Context, req *caasv1.RegisterNodeRequest) (*caasv1.RegisterNodeResponse, error) {
	if req.Name == "" || req.Jurisdiction == "" || req.Endpoint == "" {
		return nil, status.Error(codes.InvalidArgument, "name, jurisdiction, and endpoint are required")
	}

	// Generate DID if not provided
	did := req.Did
	if did == "" {
		did = fmt.Sprintf("did:web:%s", req.Name)
	}

	// Generate keypair if no public key provided
	publicKey := req.PublicKey
	if publicKey == "" {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "generate keypair: %v", err)
		}
		publicKey = base64.StdEncoding.EncodeToString(pub)
	}

	node, err := s.store.CreateNode(ctx, req.Name, did, req.Jurisdiction, req.Endpoint, publicKey, req.Metadata)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create node: %v", err)
	}

	s.events.Emit("node.registered", map[string]any{
		"node_id":      node.Id,
		"name":         node.Name,
		"jurisdiction": node.Jurisdiction,
	})

	return &caasv1.RegisterNodeResponse{Node: node}, nil
}

func (s *FederationServer) GetNode(ctx context.Context, req *caasv1.GetNodeRequest) (*caasv1.GetNodeResponse, error) {
	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}

	node, err := s.store.GetNode(ctx, req.NodeId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "node not found: %v", err)
	}

	return &caasv1.GetNodeResponse{Node: node}, nil
}

func (s *FederationServer) ListNodes(ctx context.Context, req *caasv1.ListNodesRequest) (*caasv1.ListNodesResponse, error) {
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 20
	}
	page := int(req.Page)
	if page <= 0 {
		page = 1
	}

	var statusFilter *caasv1.SovereignNodeStatus
	if req.StatusFilter != caasv1.SovereignNodeStatus_SOVEREIGN_NODE_STATUS_UNSPECIFIED {
		statusFilter = &req.StatusFilter
	}

	nodes, total, err := s.store.ListNodes(ctx, statusFilter, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list nodes: %v", err)
	}

	return &caasv1.ListNodesResponse{Nodes: nodes, Total: int32(total)}, nil
}

func (s *FederationServer) UpdateNodeStatus(ctx context.Context, req *caasv1.UpdateNodeStatusRequest) (*caasv1.UpdateNodeStatusResponse, error) {
	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}

	if err := s.store.UpdateNodeStatus(ctx, req.NodeId, req.Status); err != nil {
		return nil, status.Errorf(codes.Internal, "update node status: %v", err)
	}

	node, err := s.store.GetNode(ctx, req.NodeId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get updated node: %v", err)
	}

	s.events.Emit("node.status_updated", map[string]any{
		"node_id": node.Id,
		"status":  nodeStatusToString(req.Status),
	})

	return &caasv1.UpdateNodeStatusResponse{Node: node}, nil
}

// --- Treaty Management ---

func (s *FederationServer) ProposeTreaty(ctx context.Context, req *caasv1.ProposeTreatyRequest) (*caasv1.ProposeTreatyResponse, error) {
	if req.TargetNodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "target_node_id is required")
	}

	trustWeight := req.TrustWeight
	if trustWeight <= 0 || trustWeight > 1.0 {
		trustWeight = 0.5
	}

	var validUntil *time.Time
	if req.ValidUntil != nil {
		t := req.ValidUntil.AsTime()
		validUntil = &t
	}

	treaty, err := s.store.CreateTreaty(ctx, req.TargetNodeId, trustWeight, req.AllowedOperations, validUntil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create treaty: %v", err)
	}

	s.events.Emit("treaty.proposed", map[string]any{
		"treaty_id":      treaty.Id,
		"target_node_id": req.TargetNodeId,
	})

	return &caasv1.ProposeTreatyResponse{Treaty: treaty}, nil
}

func (s *FederationServer) AcceptTreaty(ctx context.Context, req *caasv1.AcceptTreatyRequest) (*caasv1.AcceptTreatyResponse, error) {
	if req.TreatyId == "" {
		return nil, status.Error(codes.InvalidArgument, "treaty_id is required")
	}

	if err := s.store.UpdateTreatyStatus(ctx, req.TreatyId, caasv1.TreatyStatus_TREATY_STATUS_ACTIVE); err != nil {
		return nil, status.Errorf(codes.Internal, "accept treaty: %v", err)
	}

	treaty, err := s.store.GetTreaty(ctx, req.TreatyId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get treaty: %v", err)
	}

	s.events.Emit("treaty.accepted", map[string]any{"treaty_id": treaty.Id})

	return &caasv1.AcceptTreatyResponse{Treaty: treaty}, nil
}

func (s *FederationServer) GetTreaty(ctx context.Context, req *caasv1.GetTreatyRequest) (*caasv1.GetTreatyResponse, error) {
	if req.TreatyId == "" {
		return nil, status.Error(codes.InvalidArgument, "treaty_id is required")
	}

	treaty, err := s.store.GetTreaty(ctx, req.TreatyId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "treaty not found: %v", err)
	}

	return &caasv1.GetTreatyResponse{Treaty: treaty}, nil
}

func (s *FederationServer) ListTreaties(ctx context.Context, req *caasv1.ListTreatiesRequest) (*caasv1.ListTreatiesResponse, error) {
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 20
	}
	page := int(req.Page)
	if page <= 0 {
		page = 1
	}

	var statusFilter *caasv1.TreatyStatus
	if req.StatusFilter != caasv1.TreatyStatus_TREATY_STATUS_UNSPECIFIED {
		statusFilter = &req.StatusFilter
	}

	treaties, total, err := s.store.ListTreaties(ctx, req.NodeId, statusFilter, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list treaties: %v", err)
	}

	return &caasv1.ListTreatiesResponse{Treaties: treaties, Total: int32(total)}, nil
}

func (s *FederationServer) TerminateTreaty(ctx context.Context, req *caasv1.TerminateTreatyRequest) (*caasv1.TerminateTreatyResponse, error) {
	if req.TreatyId == "" {
		return nil, status.Error(codes.InvalidArgument, "treaty_id is required")
	}

	if err := s.store.UpdateTreatyStatus(ctx, req.TreatyId, caasv1.TreatyStatus_TREATY_STATUS_TERMINATED); err != nil {
		return nil, status.Errorf(codes.Internal, "terminate treaty: %v", err)
	}

	treaty, err := s.store.GetTreaty(ctx, req.TreatyId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get treaty: %v", err)
	}

	s.events.Emit("treaty.terminated", map[string]any{
		"treaty_id": treaty.Id,
		"reason":    req.Reason,
	})

	return &caasv1.TerminateTreatyResponse{Treaty: treaty}, nil
}

// --- Federated Queries ---

func (s *FederationServer) FederatedTrustQuery(ctx context.Context, req *caasv1.FederatedTrustQueryRequest) (*caasv1.FederatedTrustQueryResponse, error) {
	if req.EntityId == "" || req.RequestingNodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id and requesting_node_id are required")
	}

	// Verify active treaty exists with requesting node
	hasTreaty, treatyWeight, err := s.store.HasActiveTreaty(ctx, req.RequestingNodeId, "trust_query")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "check treaty: %v", err)
	}
	if !hasTreaty {
		return nil, status.Error(codes.PermissionDenied, "no active treaty with trust_query permission")
	}

	// Look up local trust score for entity
	score, err := s.store.GetLocalTrustScore(ctx, req.EntityId)
	if err != nil {
		log.Printf("No local trust score for entity %s: %v", req.EntityId, err)
		score = 0
	}

	// Apply treaty weight
	weightedScore := int32(float32(score) * treatyWeight)
	confidence := treatyWeight * 0.9 // Slightly discount confidence

	// Sign the result
	signature := signResult(fmt.Sprintf("%s:%d:%f", req.EntityId, weightedScore, confidence))

	result := &caasv1.FederatedTrustResult{
		EntityId:             req.EntityId,
		SourceNodeId:         "local", // Would be this node's ID in production
		TrustScore:           weightedScore,
		Confidence:           confidence,
		QueriedAt:            timestamppb.Now(),
		AttestationSignature: signature,
	}

	s.events.Emit("federation.trust_query", map[string]any{
		"entity_id":          req.EntityId,
		"requesting_node_id": req.RequestingNodeId,
		"weighted_score":     weightedScore,
	})

	return &caasv1.FederatedTrustQueryResponse{Result: result}, nil
}

func (s *FederationServer) FederatedPermissionCheck(ctx context.Context, req *caasv1.FederatedPermissionCheckRequest) (*caasv1.FederatedPermissionCheckResponse, error) {
	if req.Subject == "" || req.Permission == "" || req.Resource == "" || req.RequestingNodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "subject, permission, resource, and requesting_node_id are required")
	}

	// Verify active treaty with permission_check allowed
	hasTreaty, _, err := s.store.HasActiveTreaty(ctx, req.RequestingNodeId, "permission_check")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "check treaty: %v", err)
	}
	if !hasTreaty {
		return nil, status.Error(codes.PermissionDenied, "no active treaty with permission_check permission")
	}

	// In production, this would call the local authz-engine
	// For MVP, return a stub result
	result := &caasv1.FederatedPermissionResult{
		Allowed:      false,
		SourceNodeId: "local",
		QueriedAt:    timestamppb.Now(),
	}

	s.events.Emit("federation.permission_check", map[string]any{
		"subject":            req.Subject,
		"permission":         req.Permission,
		"resource":           req.Resource,
		"requesting_node_id": req.RequestingNodeId,
	})

	return &caasv1.FederatedPermissionCheckResponse{Result: result}, nil
}

// --- Authority Overrides ---

func (s *FederationServer) CreateAuthorityOverride(ctx context.Context, req *caasv1.CreateAuthorityOverrideRequest) (*caasv1.CreateAuthorityOverrideResponse, error) {
	if req.WorkflowId == "" || req.AuthorityEntityId == "" || req.Justification == "" || req.Outcome == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_id, authority_entity_id, justification, and outcome are required")
	}

	if req.Tier == caasv1.AuthorityTier_AUTHORITY_TIER_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "authority tier is required")
	}

	// Tier 5 (Sovereign Emergency) requires expiry
	if req.Tier == caasv1.AuthorityTier_AUTHORITY_TIER_SOVEREIGN_EMERGENCY && req.ExpiresAt == nil {
		return nil, status.Error(codes.InvalidArgument, "sovereign emergency overrides must have an expiry time")
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t := req.ExpiresAt.AsTime()
		expiresAt = &t
	}

	override, err := s.store.CreateAuthorityOverride(
		ctx, req.WorkflowId, req.AuthorityEntityId,
		int(req.Tier), req.Justification, req.LegalReference,
		req.Outcome, expiresAt,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create authority override: %v", err)
	}

	s.events.Emit("authority.override_created", map[string]any{
		"override_id": override.Id,
		"workflow_id": req.WorkflowId,
		"tier":        req.Tier.String(),
		"outcome":     req.Outcome,
	})

	return &caasv1.CreateAuthorityOverrideResponse{Override: override}, nil
}

func (s *FederationServer) GetAuthorityOverride(ctx context.Context, req *caasv1.GetAuthorityOverrideRequest) (*caasv1.GetAuthorityOverrideResponse, error) {
	if req.OverrideId == "" {
		return nil, status.Error(codes.InvalidArgument, "override_id is required")
	}

	override, err := s.store.GetAuthorityOverride(ctx, req.OverrideId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "override not found: %v", err)
	}

	return &caasv1.GetAuthorityOverrideResponse{Override: override}, nil
}

func (s *FederationServer) ListAuthorityOverrides(ctx context.Context, req *caasv1.ListAuthorityOverridesRequest) (*caasv1.ListAuthorityOverridesResponse, error) {
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 20
	}
	page := int(req.Page)
	if page <= 0 {
		page = 1
	}

	overrides, total, err := s.store.ListAuthorityOverrides(ctx, req.WorkflowId, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list overrides: %v", err)
	}

	return &caasv1.ListAuthorityOverridesResponse{Overrides: overrides, Total: int32(total)}, nil
}

// --- Verifiable Credentials ---

func (s *FederationServer) IssueCredential(ctx context.Context, req *caasv1.IssueCredentialRequest) (*caasv1.IssueCredentialResponse, error) {
	if req.EntityId == "" || req.CredentialType == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id and credential_type are required")
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t := req.ExpiresAt.AsTime()
		expiresAt = &t
	}

	// Generate proof signature from claims
	claimsJSON, _ := json.Marshal(req.Claims)
	proofSig := signResult(fmt.Sprintf("%s:%s:%s", req.EntityId, req.CredentialType, string(claimsJSON)))

	credential, err := s.store.IssueCredential(ctx, req.EntityId, req.CredentialType, req.Claims, proofSig, expiresAt)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "issue credential: %v", err)
	}

	s.events.Emit("vc.issued", map[string]any{
		"credential_id":   credential.Id,
		"entity_id":       req.EntityId,
		"credential_type": req.CredentialType,
	})

	return &caasv1.IssueCredentialResponse{Credential: credential}, nil
}

func (s *FederationServer) VerifyCredential(ctx context.Context, req *caasv1.VerifyCredentialRequest) (*caasv1.VerifyCredentialResponse, error) {
	if req.CredentialId == "" {
		return nil, status.Error(codes.InvalidArgument, "credential_id is required")
	}

	cred, err := s.store.GetCredential(ctx, req.CredentialId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "credential not found: %v", err)
	}

	// Check validity
	if cred.Revoked {
		return &caasv1.VerifyCredentialResponse{
			Valid:      false,
			Reason:     "credential has been revoked",
			Credential: cred,
		}, nil
	}

	if cred.ExpiresAt != nil && cred.ExpiresAt.AsTime().Before(time.Now()) {
		return &caasv1.VerifyCredentialResponse{
			Valid:      false,
			Reason:     "credential has expired",
			Credential: cred,
		}, nil
	}

	return &caasv1.VerifyCredentialResponse{
		Valid:      true,
		Reason:     "credential is valid",
		Credential: cred,
	}, nil
}

func (s *FederationServer) RevokeCredential(ctx context.Context, req *caasv1.RevokeCredentialRequest) (*caasv1.RevokeCredentialResponse, error) {
	if req.CredentialId == "" {
		return nil, status.Error(codes.InvalidArgument, "credential_id is required")
	}

	if err := s.store.RevokeCredential(ctx, req.CredentialId); err != nil {
		return nil, status.Errorf(codes.Internal, "revoke credential: %v", err)
	}

	cred, err := s.store.GetCredential(ctx, req.CredentialId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get credential: %v", err)
	}

	s.events.Emit("vc.revoked", map[string]any{
		"credential_id": req.CredentialId,
		"reason":        req.Reason,
	})

	return &caasv1.RevokeCredentialResponse{Credential: cred}, nil
}

func (s *FederationServer) ListCredentials(ctx context.Context, req *caasv1.ListCredentialsRequest) (*caasv1.ListCredentialsResponse, error) {
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 20
	}
	page := int(req.Page)
	if page <= 0 {
		page = 1
	}

	creds, total, err := s.store.ListCredentials(ctx, req.EntityId, req.CredentialType, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list credentials: %v", err)
	}

	return &caasv1.ListCredentialsResponse{Credentials: creds, Total: int32(total)}, nil
}

// --- Helpers ---

func signResult(data string) string {
	hash := sha256.Sum256([]byte(data))
	return base64.StdEncoding.EncodeToString(hash[:])
}
