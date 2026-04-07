package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EntityType maps to the proto enum values.
type EntityType int

const (
	EntityTypeHuman            EntityType = 1
	EntityTypeOrganization     EntityType = 2
	EntityTypeDevice           EntityType = 3
	EntityTypeService          EntityType = 4
	EntityTypeAIAgent          EntityType = 5
	EntityTypeAutonomousSystem EntityType = 6
)

// LifecycleState tracks entity lifecycle.
type LifecycleState string

const (
	StatePending   LifecycleState = "pending"
	StateActive    LifecycleState = "active"
	StateSuspended LifecycleState = "suspended"
	StateRevoked   LifecycleState = "revoked"
	StateArchived  LifecycleState = "archived"
)

// Entity is the domain model.
type Entity struct {
	ID             string          `json:"id"`
	DID            *string         `json:"did,omitempty"`
	EntityType     EntityType      `json:"entity_type"`
	DisplayName    string          `json:"display_name"`
	LifecycleState LifecycleState  `json:"lifecycle_state"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	SuspendedAt    *time.Time      `json:"suspended_at,omitempty"`
	RevokedAt      *time.Time      `json:"revoked_at,omitempty"`
}

// PostgresStore handles entity persistence.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a connection pool.
func NewPostgresStore(ctx context.Context, connString string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("creating pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return &PostgresStore{pool: pool}, nil
}

// Close closes the connection pool.
func (s *PostgresStore) Close() {
	s.pool.Close()
}

// Create inserts a new entity.
func (s *PostgresStore) Create(ctx context.Context, entityType EntityType, displayName string, metadata json.RawMessage) (*Entity, error) {
	id := uuid.New().String()
	if metadata == nil {
		metadata = json.RawMessage("{}")
	}

	entity := &Entity{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO entities (id, entity_type, display_name, lifecycle_state, metadata)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, did, entity_type, display_name, lifecycle_state, metadata, created_at, updated_at, suspended_at, revoked_at`,
		id, entityType, displayName, StatePending, metadata,
	).Scan(
		&entity.ID, &entity.DID, &entity.EntityType, &entity.DisplayName,
		&entity.LifecycleState, &entity.Metadata, &entity.CreatedAt, &entity.UpdatedAt,
		&entity.SuspendedAt, &entity.RevokedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting entity: %w", err)
	}
	return entity, nil
}

// GetByID fetches an entity by ID.
func (s *PostgresStore) GetByID(ctx context.Context, id string) (*Entity, error) {
	entity := &Entity{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, did, entity_type, display_name, lifecycle_state, metadata, created_at, updated_at, suspended_at, revoked_at
		 FROM entities WHERE id = $1`, id,
	).Scan(
		&entity.ID, &entity.DID, &entity.EntityType, &entity.DisplayName,
		&entity.LifecycleState, &entity.Metadata, &entity.CreatedAt, &entity.UpdatedAt,
		&entity.SuspendedAt, &entity.RevokedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching entity %s: %w", id, err)
	}
	return entity, nil
}

// Update modifies an entity's display name and metadata.
func (s *PostgresStore) Update(ctx context.Context, id, displayName string, metadata json.RawMessage) (*Entity, error) {
	entity := &Entity{}
	err := s.pool.QueryRow(ctx,
		`UPDATE entities SET display_name = COALESCE(NULLIF($2, ''), display_name),
		 metadata = COALESCE($3, metadata), updated_at = now()
		 WHERE id = $1
		 RETURNING id, did, entity_type, display_name, lifecycle_state, metadata, created_at, updated_at, suspended_at, revoked_at`,
		id, displayName, metadata,
	).Scan(
		&entity.ID, &entity.DID, &entity.EntityType, &entity.DisplayName,
		&entity.LifecycleState, &entity.Metadata, &entity.CreatedAt, &entity.UpdatedAt,
		&entity.SuspendedAt, &entity.RevokedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("updating entity %s: %w", id, err)
	}
	return entity, nil
}

// List returns entities filtered by type and state.
func (s *PostgresStore) List(ctx context.Context, entityType *EntityType, state *LifecycleState, limit, offset int) ([]*Entity, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := `SELECT id, did, entity_type, display_name, lifecycle_state, metadata, created_at, updated_at, suspended_at, revoked_at
		FROM entities WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM entities WHERE 1=1`
	args := []any{}
	argIdx := 1

	if entityType != nil {
		query += fmt.Sprintf(" AND entity_type = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND entity_type = $%d", argIdx)
		args = append(args, *entityType)
		argIdx++
	}
	if state != nil {
		query += fmt.Sprintf(" AND lifecycle_state = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND lifecycle_state = $%d", argIdx)
		args = append(args, *state)
		argIdx++
	}

	// Get total count
	var total int
	err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting entities: %w", err)
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing entities: %w", err)
	}
	defer rows.Close()

	var entities []*Entity
	for rows.Next() {
		e := &Entity{}
		if err := rows.Scan(
			&e.ID, &e.DID, &e.EntityType, &e.DisplayName,
			&e.LifecycleState, &e.Metadata, &e.CreatedAt, &e.UpdatedAt,
			&e.SuspendedAt, &e.RevokedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning entity: %w", err)
		}
		entities = append(entities, e)
	}
	return entities, total, nil
}

// Lifecycle transitions

// Activate transitions an entity from pending to active.
func (s *PostgresStore) Activate(ctx context.Context, id string) (*Entity, error) {
	return s.transition(ctx, id, StateActive, []LifecycleState{StatePending, StateSuspended})
}

// Suspend transitions an entity to suspended.
func (s *PostgresStore) Suspend(ctx context.Context, id string) (*Entity, error) {
	return s.transition(ctx, id, StateSuspended, []LifecycleState{StateActive})
}

// Revoke transitions an entity to revoked.
func (s *PostgresStore) Revoke(ctx context.Context, id string) (*Entity, error) {
	return s.transition(ctx, id, StateRevoked, []LifecycleState{StateActive, StateSuspended})
}

func (s *PostgresStore) transition(ctx context.Context, id string, target LifecycleState, validFrom []LifecycleState) (*Entity, error) {
	// Build valid source states
	states := make([]string, len(validFrom))
	for i, st := range validFrom {
		states[i] = string(st)
	}

	entity := &Entity{}
	var extraCol string
	switch target {
	case StateSuspended:
		extraCol = ", suspended_at = now()"
	case StateRevoked:
		extraCol = ", revoked_at = now()"
	default:
		extraCol = ""
	}

	query := fmt.Sprintf(
		`UPDATE entities SET lifecycle_state = $1, updated_at = now() %s
		 WHERE id = $2 AND lifecycle_state = ANY($3)
		 RETURNING id, did, entity_type, display_name, lifecycle_state, metadata, created_at, updated_at, suspended_at, revoked_at`,
		extraCol,
	)

	err := s.pool.QueryRow(ctx, query, target, id, states).Scan(
		&entity.ID, &entity.DID, &entity.EntityType, &entity.DisplayName,
		&entity.LifecycleState, &entity.Metadata, &entity.CreatedAt, &entity.UpdatedAt,
		&entity.SuspendedAt, &entity.RevokedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("transitioning entity %s to %s: %w", id, target, err)
	}
	return entity, nil
}

// SpiceDB entity type name mapping
func EntityTypeToSpiceDB(t EntityType) string {
	switch t {
	case EntityTypeHuman:
		return "user"
	case EntityTypeOrganization:
		return "organization"
	case EntityTypeDevice:
		return "device"
	case EntityTypeService:
		return "service"
	case EntityTypeAIAgent:
		return "ai_agent"
	case EntityTypeAutonomousSystem:
		return "autonomous_system"
	default:
		return "user"
	}
}
