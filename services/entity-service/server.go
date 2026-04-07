package main

import (
	"context"
	"encoding/json"
	"log"

	caasv1 "github.com/OpenAGX/caas/gen/go/caas/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// EntityServer implements caasv1.EntityServiceServer.
type EntityServer struct {
	caasv1.UnimplementedEntityServiceServer
	store  *PostgresStore
	events *EventProducer
}

func NewEntityServer(store *PostgresStore, events *EventProducer) *EntityServer {
	return &EntityServer{store: store, events: events}
}

func RegisterEntityServer(s interface{ RegisterService(desc *interface{}, impl interface{}) }, srv *EntityServer) {
	// Placeholder — real registration is in main.go via caasv1.RegisterEntityServiceServer
}

func (s *EntityServer) CreateEntity(ctx context.Context, req *caasv1.CreateEntityRequest) (*caasv1.CreateEntityResponse, error) {
	if req.DisplayName == "" {
		return nil, status.Error(codes.InvalidArgument, "display_name is required")
	}
	if req.EntityType == caasv1.EntityType_ENTITY_TYPE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "entity_type is required")
	}

	var metadata json.RawMessage
	if req.Metadata != nil {
		b, _ := req.Metadata.MarshalJSON()
		metadata = b
	}

	entity, err := s.store.Create(ctx, EntityType(req.EntityType), req.DisplayName, metadata)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create entity: %v", err)
	}

	log.Printf("CREATE entity %s (type=%d, name=%s)", entity.ID, entity.EntityType, entity.DisplayName)

	s.events.Emit("entity_created", map[string]any{
		"entity_id":   entity.ID,
		"entity_type": entity.EntityType,
		"name":        entity.DisplayName,
	})

	return &caasv1.CreateEntityResponse{Entity: entityToProto(entity)}, nil
}

func (s *EntityServer) GetEntity(ctx context.Context, req *caasv1.GetEntityRequest) (*caasv1.GetEntityResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	entity, err := s.store.GetByID(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "entity %s not found: %v", req.Id, err)
	}

	return &caasv1.GetEntityResponse{Entity: entityToProto(entity)}, nil
}

func (s *EntityServer) UpdateEntity(ctx context.Context, req *caasv1.UpdateEntityRequest) (*caasv1.UpdateEntityResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	var metadata json.RawMessage
	if req.Metadata != nil {
		b, _ := req.Metadata.MarshalJSON()
		metadata = b
	}

	entity, err := s.store.Update(ctx, req.Id, req.DisplayName, metadata)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update entity: %v", err)
	}

	log.Printf("UPDATE entity %s", entity.ID)
	return &caasv1.UpdateEntityResponse{Entity: entityToProto(entity)}, nil
}

func (s *EntityServer) ListEntities(ctx context.Context, req *caasv1.ListEntitiesRequest) (*caasv1.ListEntitiesResponse, error) {
	var entityType *EntityType
	if req.EntityType != caasv1.EntityType_ENTITY_TYPE_UNSPECIFIED {
		et := EntityType(req.EntityType)
		entityType = &et
	}

	var state *LifecycleState
	if req.LifecycleState != caasv1.LifecycleState_LIFECYCLE_STATE_UNSPECIFIED {
		st := lifecycleStateFromProto(req.LifecycleState)
		state = &st
	}

	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}

	offset := 0
	// Simple page token: just the offset as string
	if req.PageToken != "" {
		var o int
		if err := json.Unmarshal([]byte(req.PageToken), &o); err == nil {
			offset = o
		}
	}

	entities, total, err := s.store.List(ctx, entityType, state, pageSize, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list entities: %v", err)
	}

	var protoEntities []*caasv1.Entity
	for _, e := range entities {
		protoEntities = append(protoEntities, entityToProto(e))
	}

	nextToken := ""
	if offset+pageSize < total {
		b, _ := json.Marshal(offset + pageSize)
		nextToken = string(b)
	}

	return &caasv1.ListEntitiesResponse{
		Entities:      protoEntities,
		NextPageToken: nextToken,
		TotalCount:    int32(total),
	}, nil
}

func (s *EntityServer) ActivateEntity(ctx context.Context, req *caasv1.ActivateEntityRequest) (*caasv1.ActivateEntityResponse, error) {
	entity, err := s.store.Activate(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "activate: %v", err)
	}
	log.Printf("ACTIVATE entity %s", entity.ID)
	s.events.Emit("entity_activated", map[string]any{"entity_id": entity.ID})
	return &caasv1.ActivateEntityResponse{Entity: entityToProto(entity)}, nil
}

func (s *EntityServer) SuspendEntity(ctx context.Context, req *caasv1.SuspendEntityRequest) (*caasv1.SuspendEntityResponse, error) {
	entity, err := s.store.Suspend(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "suspend: %v", err)
	}
	log.Printf("SUSPEND entity %s (reason: %s)", entity.ID, req.Reason)
	s.events.Emit("entity_suspended", map[string]any{"entity_id": entity.ID, "reason": req.Reason})
	return &caasv1.SuspendEntityResponse{Entity: entityToProto(entity)}, nil
}

func (s *EntityServer) RevokeEntity(ctx context.Context, req *caasv1.RevokeEntityRequest) (*caasv1.RevokeEntityResponse, error) {
	entity, err := s.store.Revoke(ctx, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "revoke: %v", err)
	}
	log.Printf("REVOKE entity %s (reason: %s)", entity.ID, req.Reason)
	s.events.Emit("entity_revoked", map[string]any{"entity_id": entity.ID, "reason": req.Reason})
	return &caasv1.RevokeEntityResponse{Entity: entityToProto(entity)}, nil
}

// --- Converters ---

func entityToProto(e *Entity) *caasv1.Entity {
	pe := &caasv1.Entity{
		Id:             e.ID,
		EntityType:     caasv1.EntityType(e.EntityType),
		DisplayName:    e.DisplayName,
		LifecycleState: lifecycleStateToProto(e.LifecycleState),
		CreatedAt:      timestamppb.New(e.CreatedAt),
		UpdatedAt:      timestamppb.New(e.UpdatedAt),
	}

	if e.DID != nil {
		pe.Did = *e.DID
	}

	if e.SuspendedAt != nil {
		pe.SuspendedAt = timestamppb.New(*e.SuspendedAt)
	}
	if e.RevokedAt != nil {
		pe.RevokedAt = timestamppb.New(*e.RevokedAt)
	}

	if e.Metadata != nil {
		var m map[string]any
		if json.Unmarshal(e.Metadata, &m) == nil {
			if s, err := structpb.NewStruct(m); err == nil {
				pe.Metadata = s
			}
		}
	}

	return pe
}

func lifecycleStateToProto(s LifecycleState) caasv1.LifecycleState {
	switch s {
	case StatePending:
		return caasv1.LifecycleState_LIFECYCLE_STATE_PENDING
	case StateActive:
		return caasv1.LifecycleState_LIFECYCLE_STATE_ACTIVE
	case StateSuspended:
		return caasv1.LifecycleState_LIFECYCLE_STATE_SUSPENDED
	case StateRevoked:
		return caasv1.LifecycleState_LIFECYCLE_STATE_REVOKED
	case StateArchived:
		return caasv1.LifecycleState_LIFECYCLE_STATE_ARCHIVED
	default:
		return caasv1.LifecycleState_LIFECYCLE_STATE_UNSPECIFIED
	}
}

func lifecycleStateFromProto(s caasv1.LifecycleState) LifecycleState {
	switch s {
	case caasv1.LifecycleState_LIFECYCLE_STATE_PENDING:
		return StatePending
	case caasv1.LifecycleState_LIFECYCLE_STATE_ACTIVE:
		return StateActive
	case caasv1.LifecycleState_LIFECYCLE_STATE_SUSPENDED:
		return StateSuspended
	case caasv1.LifecycleState_LIFECYCLE_STATE_REVOKED:
		return StateRevoked
	case caasv1.LifecycleState_LIFECYCLE_STATE_ARCHIVED:
		return StateArchived
	default:
		return StatePending
	}
}
