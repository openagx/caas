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

// --- Workflows ---

func (s *PostgresStore) CreateWorkflow(
	ctx context.Context,
	workflowType, subjectEntityID string,
	caseData *structpb.Struct,
	requiredApprovals, totalReviewers int32,
	blindReview bool,
	coolOffHours int32,
) (*caasv1.DecisionWorkflow, error) {
	var caseJSON []byte
	if caseData != nil {
		caseJSON, _ = caseData.MarshalJSON()
	} else {
		caseJSON = []byte("{}")
	}

	row := s.pool.QueryRow(ctx,
		`INSERT INTO decision_workflows
		 (workflow_type, subject_entity_id, case_data, required_approvals,
		  total_reviewers, blind_review, cool_off_hours, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending')
		 RETURNING id, created_at`,
		workflowType, subjectEntityID, caseJSON,
		requiredApprovals, totalReviewers, blindReview, coolOffHours,
	)

	var id string
	var createdAt time.Time
	if err := row.Scan(&id, &createdAt); err != nil {
		return nil, err
	}

	return &caasv1.DecisionWorkflow{
		Id:                id,
		WorkflowType:      workflowType,
		RequiredApprovals: requiredApprovals,
		TotalReviewers:    totalReviewers,
		BlindReview:       blindReview,
		CoolOffHours:      coolOffHours,
		Status:            caasv1.WorkflowStatus_WORKFLOW_STATUS_PENDING,
		SubjectEntityId:   subjectEntityID,
		CaseData:          caseData,
		CreatedAt:         timestamppb.New(createdAt),
	}, nil
}

func (s *PostgresStore) GetWorkflow(ctx context.Context, id string) (*caasv1.DecisionWorkflow, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, workflow_type, required_approvals, total_reviewers,
		        blind_review, cool_off_hours, status, subject_entity_id,
		        case_data, outcome, created_at, completed_at
		 FROM decision_workflows WHERE id = $1`, id,
	)

	var wf caasv1.DecisionWorkflow
	var statusStr string
	var caseJSON []byte
	var outcome *string
	var createdAt time.Time
	var completedAt *time.Time

	if err := row.Scan(
		&wf.Id, &wf.WorkflowType, &wf.RequiredApprovals, &wf.TotalReviewers,
		&wf.BlindReview, &wf.CoolOffHours, &statusStr, &wf.SubjectEntityId,
		&caseJSON, &outcome, &createdAt, &completedAt,
	); err != nil {
		return nil, err
	}

	wf.Status = workflowStatusFromString(statusStr)
	wf.CreatedAt = timestamppb.New(createdAt)
	if completedAt != nil {
		wf.CompletedAt = timestamppb.New(*completedAt)
	}
	if outcome != nil {
		wf.Outcome = *outcome
	}
	if len(caseJSON) > 0 {
		s := &structpb.Struct{}
		if s.UnmarshalJSON(caseJSON) == nil {
			wf.CaseData = s
		}
	}

	// Load votes
	votes, err := s.GetVotesForWorkflow(ctx, id)
	if err == nil {
		wf.Votes = votes
	}

	return &wf, nil
}

func (s *PostgresStore) ListWorkflows(
	ctx context.Context,
	statusFilter *caasv1.WorkflowStatus,
	subjectEntityID string,
	limit, offset int,
) ([]*caasv1.DecisionWorkflow, int, error) {
	query := `SELECT id FROM decision_workflows WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM decision_workflows WHERE 1=1`
	args := []any{}
	argIdx := 1

	if statusFilter != nil {
		statusStr := workflowStatusToString(*statusFilter)
		query += fmt.Sprintf(` AND status = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND status = $%d`, argIdx)
		args = append(args, statusStr)
		argIdx++
	}
	if subjectEntityID != "" {
		query += fmt.Sprintf(` AND subject_entity_id = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND subject_entity_id = $%d`, argIdx)
		args = append(args, subjectEntityID)
		argIdx++
	}

	// Get total count
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

	var workflows []*caasv1.DecisionWorkflow
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		wf, err := s.GetWorkflow(ctx, id)
		if err != nil {
			continue
		}
		workflows = append(workflows, wf)
	}

	return workflows, total, nil
}

func (s *PostgresStore) UpdateWorkflowStatus(ctx context.Context, id string, status caasv1.WorkflowStatus, outcome string) error {
	statusStr := workflowStatusToString(status)
	if outcome != "" {
		_, err := s.pool.Exec(ctx,
			`UPDATE decision_workflows SET status = $2, outcome = $3, completed_at = now() WHERE id = $1`,
			id, statusStr, outcome,
		)
		return err
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE decision_workflows SET status = $2 WHERE id = $1`,
		id, statusStr,
	)
	return err
}

// --- Votes ---

func (s *PostgresStore) SubmitVote(
	ctx context.Context,
	workflowID, reviewerID string,
	vote caasv1.VoteType,
	reasoning string,
	confidence float32,
	timeSpentSeconds int32,
	blindCaseID string,
) (*caasv1.DecisionVote, error) {
	voteStr := voteTypeToString(vote)

	row := s.pool.QueryRow(ctx,
		`INSERT INTO decision_votes
		 (workflow_id, reviewer_id, vote, reasoning, confidence, time_spent_seconds, blind_case_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, created_at`,
		workflowID, reviewerID, voteStr, reasoning, confidence, timeSpentSeconds, blindCaseID,
	)

	var id string
	var createdAt time.Time
	if err := row.Scan(&id, &createdAt); err != nil {
		return nil, err
	}

	return &caasv1.DecisionVote{
		Id:               id,
		WorkflowId:       workflowID,
		ReviewerId:       reviewerID,
		Vote:             vote,
		Reasoning:        reasoning,
		Confidence:       confidence,
		TimeSpentSeconds: timeSpentSeconds,
		BlindCaseId:      blindCaseID,
		CreatedAt:        timestamppb.New(createdAt),
	}, nil
}

func (s *PostgresStore) GetVotesForWorkflow(ctx context.Context, workflowID string) ([]*caasv1.DecisionVote, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, workflow_id, reviewer_id, vote, reasoning, confidence,
		        time_spent_seconds, blind_case_id, created_at
		 FROM decision_votes WHERE workflow_id = $1 ORDER BY created_at`, workflowID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var votes []*caasv1.DecisionVote
	for rows.Next() {
		var v caasv1.DecisionVote
		var voteStr string
		var createdAt time.Time
		var confidence *float32
		var timeSpent *int32

		if err := rows.Scan(
			&v.Id, &v.WorkflowId, &v.ReviewerId, &voteStr, &v.Reasoning,
			&confidence, &timeSpent, &v.BlindCaseId, &createdAt,
		); err != nil {
			return nil, err
		}

		v.Vote = voteTypeFromString(voteStr)
		v.CreatedAt = timestamppb.New(createdAt)
		if confidence != nil {
			v.Confidence = *confidence
		}
		if timeSpent != nil {
			v.TimeSpentSeconds = *timeSpent
		}
		votes = append(votes, &v)
	}

	return votes, nil
}

func (s *PostgresStore) HasVoted(ctx context.Context, workflowID, reviewerID string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM decision_votes WHERE workflow_id = $1 AND reviewer_id = $2`,
		workflowID, reviewerID,
	).Scan(&count)
	return count > 0, err
}

func (s *PostgresStore) CheckCoolOff(ctx context.Context, reviewerID, subjectEntityID string, coolOffHours int32) (bool, error) {
	// Check if reviewer has voted on any workflow for this entity within cool-off period
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM decision_votes dv
		 JOIN decision_workflows dw ON dv.workflow_id = dw.id
		 WHERE dv.reviewer_id = $1
		   AND dw.subject_entity_id = $2
		   AND dv.created_at > now() - ($3 || ' hours')::interval`,
		reviewerID, subjectEntityID, fmt.Sprintf("%d", coolOffHours),
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count == 0, nil // true = cooled off (no recent votes)
}

// --- Reviewer Integrity ---

func (s *PostgresStore) UpdateReviewerStats(ctx context.Context, reviewerID string, vote caasv1.VoteType, timeSpentSeconds int32) error {
	// Upsert reviewer integrity record
	_, err := s.pool.Exec(ctx,
		`INSERT INTO reviewer_integrity (reviewer_id, total_reviews, integrity_score)
		 VALUES ($1, 1, 500)
		 ON CONFLICT (reviewer_id)
		 DO UPDATE SET
		   total_reviews = reviewer_integrity.total_reviews + 1,
		   approval_rate = (
		     SELECT COUNT(*) FILTER (WHERE vote = 'approve')::float / NULLIF(COUNT(*), 0)
		     FROM decision_votes WHERE reviewer_id = $1
		   ),
		   avg_time_per_review = (
		     SELECT AVG(time_spent_seconds)::float
		     FROM decision_votes WHERE reviewer_id = $1
		   ),
		   updated_at = now()`,
		reviewerID,
	)
	return err
}

func (s *PostgresStore) GetReviewerIntegrity(ctx context.Context, reviewerID string) (*caasv1.ReviewerIntegrity, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT reviewer_id, integrity_score, total_reviews, approval_rate,
		        avg_time_per_review, bias_flags, last_bias_check
		 FROM reviewer_integrity WHERE reviewer_id = $1`, reviewerID,
	)

	var ri caasv1.ReviewerIntegrity
	var approvalRate, avgTime *float32
	var lastBiasCheck *time.Time

	if err := row.Scan(
		&ri.ReviewerId, &ri.IntegrityScore, &ri.TotalReviews,
		&approvalRate, &avgTime, &ri.BiasFlags, &lastBiasCheck,
	); err != nil {
		return nil, err
	}

	if approvalRate != nil {
		ri.ApprovalRate = *approvalRate
	}
	if avgTime != nil {
		ri.AvgTimePerReview = *avgTime
	}
	if lastBiasCheck != nil {
		ri.LastBiasCheck = timestamppb.New(*lastBiasCheck)
	}

	return &ri, nil
}

// --- Converters ---

func workflowStatusToString(s caasv1.WorkflowStatus) string {
	switch s {
	case caasv1.WorkflowStatus_WORKFLOW_STATUS_PENDING:
		return "pending"
	case caasv1.WorkflowStatus_WORKFLOW_STATUS_IN_REVIEW:
		return "in_review"
	case caasv1.WorkflowStatus_WORKFLOW_STATUS_APPROVED:
		return "approved"
	case caasv1.WorkflowStatus_WORKFLOW_STATUS_REJECTED:
		return "rejected"
	case caasv1.WorkflowStatus_WORKFLOW_STATUS_ESCALATED:
		return "escalated"
	default:
		return "pending"
	}
}

func workflowStatusFromString(s string) caasv1.WorkflowStatus {
	switch s {
	case "pending":
		return caasv1.WorkflowStatus_WORKFLOW_STATUS_PENDING
	case "in_review":
		return caasv1.WorkflowStatus_WORKFLOW_STATUS_IN_REVIEW
	case "approved":
		return caasv1.WorkflowStatus_WORKFLOW_STATUS_APPROVED
	case "rejected":
		return caasv1.WorkflowStatus_WORKFLOW_STATUS_REJECTED
	case "escalated":
		return caasv1.WorkflowStatus_WORKFLOW_STATUS_ESCALATED
	default:
		return caasv1.WorkflowStatus_WORKFLOW_STATUS_UNSPECIFIED
	}
}

func voteTypeToString(v caasv1.VoteType) string {
	switch v {
	case caasv1.VoteType_VOTE_TYPE_APPROVE:
		return "approve"
	case caasv1.VoteType_VOTE_TYPE_REJECT:
		return "reject"
	case caasv1.VoteType_VOTE_TYPE_ESCALATE:
		return "escalate"
	case caasv1.VoteType_VOTE_TYPE_ABSTAIN:
		return "abstain"
	default:
		return "unspecified"
	}
}

func voteTypeFromString(s string) caasv1.VoteType {
	switch s {
	case "approve":
		return caasv1.VoteType_VOTE_TYPE_APPROVE
	case "reject":
		return caasv1.VoteType_VOTE_TYPE_REJECT
	case "escalate":
		return caasv1.VoteType_VOTE_TYPE_ESCALATE
	case "abstain":
		return caasv1.VoteType_VOTE_TYPE_ABSTAIN
	default:
		return caasv1.VoteType_VOTE_TYPE_UNSPECIFIED
	}
}

// --- unused but keeps import alive ---
var _ = json.Marshal
