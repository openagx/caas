package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"

	caasv1 "github.com/OpenAGX/caas/gen/go/caas/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// DecisionServer implements caasv1.DecisionServiceServer.
type DecisionServer struct {
	caasv1.UnimplementedDecisionServiceServer
	store  *PostgresStore
	events *EventProducer
}

func NewDecisionServer(store *PostgresStore, events *EventProducer) *DecisionServer {
	return &DecisionServer{store: store, events: events}
}

func (s *DecisionServer) CreateWorkflow(ctx context.Context, req *caasv1.CreateWorkflowRequest) (*caasv1.CreateWorkflowResponse, error) {
	if req.WorkflowType == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_type is required")
	}
	if req.SubjectEntityId == "" {
		return nil, status.Error(codes.InvalidArgument, "subject_entity_id is required")
	}

	requiredApprovals := req.RequiredApprovals
	if requiredApprovals <= 0 {
		requiredApprovals = 2
	}
	totalReviewers := req.TotalReviewers
	if totalReviewers <= 0 {
		totalReviewers = 3
	}
	if totalReviewers < requiredApprovals {
		totalReviewers = requiredApprovals
	}
	coolOffHours := req.CoolOffHours
	if coolOffHours <= 0 {
		coolOffHours = 168 // 7 days
	}

	workflow, err := s.store.CreateWorkflow(ctx,
		req.WorkflowType,
		req.SubjectEntityId,
		req.CaseData,
		requiredApprovals,
		totalReviewers,
		req.BlindReview,
		coolOffHours,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create workflow: %v", err)
	}

	log.Printf("WORKFLOW created: %s (type=%s, subject=%s, M=%d/N=%d)",
		workflow.Id, workflow.WorkflowType, workflow.SubjectEntityId,
		workflow.RequiredApprovals, workflow.TotalReviewers)

	s.events.Emit("workflow_created", map[string]any{
		"workflow_id":       workflow.Id,
		"workflow_type":     workflow.WorkflowType,
		"subject_entity_id": workflow.SubjectEntityId,
	})

	return &caasv1.CreateWorkflowResponse{Workflow: workflow}, nil
}

func (s *DecisionServer) GetWorkflow(ctx context.Context, req *caasv1.GetWorkflowRequest) (*caasv1.GetWorkflowResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	workflow, err := s.store.GetWorkflow(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "workflow %s not found: %v", req.Id, err)
	}

	return &caasv1.GetWorkflowResponse{Workflow: workflow}, nil
}

func (s *DecisionServer) ListWorkflows(ctx context.Context, req *caasv1.ListWorkflowsRequest) (*caasv1.ListWorkflowsResponse, error) {
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}

	offset := 0
	if req.PageToken != "" {
		fmt.Sscanf(req.PageToken, "%d", &offset)
	}

	var statusFilter *caasv1.WorkflowStatus
	if req.Status != caasv1.WorkflowStatus_WORKFLOW_STATUS_UNSPECIFIED {
		statusFilter = &req.Status
	}

	workflows, total, err := s.store.ListWorkflows(ctx, statusFilter, req.SubjectEntityId, pageSize, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list workflows: %v", err)
	}

	nextToken := ""
	if offset+pageSize < total {
		nextToken = fmt.Sprintf("%d", offset+pageSize)
	}

	return &caasv1.ListWorkflowsResponse{
		Workflows:     workflows,
		NextPageToken: nextToken,
	}, nil
}

func (s *DecisionServer) SubmitVote(ctx context.Context, req *caasv1.SubmitVoteRequest) (*caasv1.SubmitVoteResponse, error) {
	if req.WorkflowId == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_id is required")
	}
	if req.ReviewerId == "" {
		return nil, status.Error(codes.InvalidArgument, "reviewer_id is required")
	}
	if req.Vote == caasv1.VoteType_VOTE_TYPE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "vote is required")
	}

	// Get workflow to check state
	workflow, err := s.store.GetWorkflow(ctx, req.WorkflowId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "workflow not found: %v", err)
	}

	// Check workflow is still accepting votes
	if workflow.Status != caasv1.WorkflowStatus_WORKFLOW_STATUS_PENDING &&
		workflow.Status != caasv1.WorkflowStatus_WORKFLOW_STATUS_IN_REVIEW {
		return nil, status.Error(codes.FailedPrecondition, "workflow is no longer accepting votes")
	}

	// Check cool-off period
	if workflow.BlindReview {
		cooledOff, err := s.store.CheckCoolOff(ctx, req.ReviewerId, workflow.SubjectEntityId, workflow.CoolOffHours)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "cool-off check: %v", err)
		}
		if !cooledOff {
			return nil, status.Error(codes.FailedPrecondition, "reviewer is within cool-off period for this entity")
		}
	}

	// Check reviewer hasn't already voted
	hasVoted, err := s.store.HasVoted(ctx, req.WorkflowId, req.ReviewerId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "vote check: %v", err)
	}
	if hasVoted {
		return nil, status.Error(codes.AlreadyExists, "reviewer has already voted on this workflow")
	}

	// Generate blind case ID
	blindCaseID := generateBlindCaseID(req.WorkflowId, req.ReviewerId)

	// Record vote
	vote, err := s.store.SubmitVote(ctx,
		req.WorkflowId,
		req.ReviewerId,
		req.Vote,
		req.Reasoning,
		req.Confidence,
		req.TimeSpentSeconds,
		blindCaseID,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "submit vote: %v", err)
	}

	// Update reviewer integrity stats
	if err := s.store.UpdateReviewerStats(ctx, req.ReviewerId, req.Vote, req.TimeSpentSeconds); err != nil {
		log.Printf("WARN: failed to update reviewer stats: %v", err)
	}

	// Check if workflow should be resolved
	updatedWorkflow, err := s.resolveWorkflow(ctx, workflow)
	if err != nil {
		log.Printf("WARN: failed to resolve workflow: %v", err)
		updatedWorkflow = workflow
	}

	log.Printf("VOTE submitted: workflow=%s reviewer=%s vote=%s blind=%s",
		req.WorkflowId, req.ReviewerId, req.Vote, blindCaseID)

	s.events.Emit("vote_submitted", map[string]any{
		"workflow_id": req.WorkflowId,
		"reviewer_id": req.ReviewerId,
		"vote":        req.Vote.String(),
	})

	return &caasv1.SubmitVoteResponse{Vote: vote, Workflow: updatedWorkflow}, nil
}

func (s *DecisionServer) GetBlindCase(ctx context.Context, req *caasv1.GetBlindCaseRequest) (*caasv1.GetBlindCaseResponse, error) {
	if req.WorkflowId == "" || req.ReviewerId == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_id and reviewer_id are required")
	}

	workflow, err := s.store.GetWorkflow(ctx, req.WorkflowId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "workflow not found: %v", err)
	}

	blindCaseID := generateBlindCaseID(req.WorkflowId, req.ReviewerId)

	// Redact PII from case data
	redacted := redactCaseData(workflow.CaseData)

	// Count existing votes
	voteCount := len(workflow.Votes)

	return &caasv1.GetBlindCaseResponse{
		BlindCaseId:       blindCaseID,
		RedactedCaseData:  redacted,
		WorkflowType:      workflow.WorkflowType,
		RequiredApprovals: workflow.RequiredApprovals,
		VotesReceived:     int32(voteCount),
	}, nil
}

func (s *DecisionServer) GetReviewerIntegrity(ctx context.Context, req *caasv1.GetReviewerIntegrityRequest) (*caasv1.GetReviewerIntegrityResponse, error) {
	if req.ReviewerId == "" {
		return nil, status.Error(codes.InvalidArgument, "reviewer_id is required")
	}

	integrity, err := s.store.GetReviewerIntegrity(ctx, req.ReviewerId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "reviewer integrity not found: %v", err)
	}

	return &caasv1.GetReviewerIntegrityResponse{Integrity: integrity}, nil
}

// resolveWorkflow checks vote counts and resolves the workflow if threshold is met.
func (s *DecisionServer) resolveWorkflow(ctx context.Context, workflow *caasv1.DecisionWorkflow) (*caasv1.DecisionWorkflow, error) {
	// Refresh workflow with all votes
	w, err := s.store.GetWorkflow(ctx, workflow.Id)
	if err != nil {
		return nil, err
	}

	approvals := 0
	rejections := 0
	escalations := 0
	for _, v := range w.Votes {
		switch v.Vote {
		case caasv1.VoteType_VOTE_TYPE_APPROVE:
			approvals++
		case caasv1.VoteType_VOTE_TYPE_REJECT:
			rejections++
		case caasv1.VoteType_VOTE_TYPE_ESCALATE:
			escalations++
		}
	}

	totalVotes := len(w.Votes)
	required := int(w.RequiredApprovals)
	totalReviewers := int(w.TotalReviewers)

	var newStatus caasv1.WorkflowStatus
	var outcome string

	if escalations > 0 {
		newStatus = caasv1.WorkflowStatus_WORKFLOW_STATUS_ESCALATED
		outcome = "escalated"
	} else if approvals >= required {
		newStatus = caasv1.WorkflowStatus_WORKFLOW_STATUS_APPROVED
		outcome = "approved"
	} else if rejections > totalReviewers-required {
		// Not enough remaining reviewers to reach approval threshold
		newStatus = caasv1.WorkflowStatus_WORKFLOW_STATUS_REJECTED
		outcome = "rejected"
	} else if totalVotes >= totalReviewers {
		// All votes are in but no consensus
		newStatus = caasv1.WorkflowStatus_WORKFLOW_STATUS_REJECTED
		outcome = "rejected_no_consensus"
	} else {
		// Still collecting votes
		if w.Status == caasv1.WorkflowStatus_WORKFLOW_STATUS_PENDING {
			newStatus = caasv1.WorkflowStatus_WORKFLOW_STATUS_IN_REVIEW
		} else {
			return w, nil // No change
		}
		outcome = ""
	}

	if err := s.store.UpdateWorkflowStatus(ctx, w.Id, newStatus, outcome); err != nil {
		return nil, err
	}

	if outcome != "" {
		log.Printf("WORKFLOW resolved: %s outcome=%s (approvals=%d rejections=%d)",
			w.Id, outcome, approvals, rejections)

		s.events.Emit("workflow_resolved", map[string]any{
			"workflow_id": w.Id,
			"outcome":     outcome,
			"approvals":   approvals,
			"rejections":  rejections,
		})
	}

	return s.store.GetWorkflow(ctx, w.Id)
}

// generateBlindCaseID creates a deterministic but opaque case ID.
func generateBlindCaseID(workflowID, reviewerID string) string {
	h := sha256.Sum256([]byte(workflowID + "::" + reviewerID))
	return "CASE-" + hex.EncodeToString(h[:6])
}

// redactCaseData strips PII fields from case data.
func redactCaseData(data *structpb.Struct) *structpb.Struct {
	if data == nil {
		return nil
	}

	redacted := &structpb.Struct{Fields: make(map[string]*structpb.Value)}

	piiFields := map[string]bool{
		"name": true, "email": true, "phone": true, "address": true,
		"display_name": true, "entity_id": true, "did": true,
		"ip_address": true, "user_agent": true,
	}

	for k, v := range data.Fields {
		if piiFields[k] {
			redacted.Fields[k] = structpb.NewStringValue("[REDACTED]")
		} else {
			redacted.Fields[k] = v
		}
	}

	return redacted
}
