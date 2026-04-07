package main

import (
	"context"
	"log"
	"time"

	pb "github.com/authzed/authzed-go/proto/authzed/api/v1"
	caasv1 "github.com/OpenAGX/caas/gen/go/caas/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AuthorizationServer implements caasv1.AuthorizationServiceServer.
type AuthorizationServer struct {
	caasv1.UnimplementedAuthorizationServiceServer
	spicedb *SpiceDBClient
	cache   *RedisCache
	events  *EventProducer
}

func NewAuthorizationServer(spicedb *SpiceDBClient, cache *RedisCache, events *EventProducer) *AuthorizationServer {
	return &AuthorizationServer{
		spicedb: spicedb,
		cache:   cache,
		events:  events,
	}
}

func (s *AuthorizationServer) Check(ctx context.Context, req *caasv1.CheckRequest) (*caasv1.CheckResponse, error) {
	if req.Subject == nil || req.Resource == nil || req.Permission == "" {
		return nil, status.Error(codes.InvalidArgument, "subject, permission, and resource are required")
	}

	start := time.Now()

	subType := req.Subject.EntityType
	subID := req.Subject.EntityId
	resType := req.Resource.ResourceType
	resID := req.Resource.ResourceId
	perm := req.Permission

	// Check cache
	if allowed, found := s.cache.GetCachedCheck(ctx, resType, resID, perm, subType, subID); found {
		latency := time.Since(start).Microseconds()
		log.Printf("CHECK [cached] %s:%s#%s@%s:%s → %v (%dμs)", resType, resID, perm, subType, subID, allowed, latency)
		return &caasv1.CheckResponse{
			Allowed:        allowed,
			CheckLatencyUs: latency,
		}, nil
	}

	// Query SpiceDB
	allowed, token, err := s.spicedb.Check(ctx, resType, resID, perm, subType, subID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "spicedb check: %v", err)
	}

	// Cache result
	s.cache.SetCachedCheck(ctx, resType, resID, perm, subType, subID, allowed)

	latency := time.Since(start).Microseconds()
	log.Printf("CHECK %s:%s#%s@%s:%s → %v (%dμs)", resType, resID, perm, subType, subID, allowed, latency)

	// Emit event
	s.events.Emit("check", map[string]any{
		"subject_type": subType, "subject_id": subID,
		"permission": perm,
		"resource_type": resType, "resource_id": resID,
		"allowed": allowed, "latency_us": latency,
	})

	return &caasv1.CheckResponse{
		Allowed:          allowed,
		ConsistencyToken: token,
		CheckLatencyUs:   latency,
	}, nil
}

func (s *AuthorizationServer) WriteRelationships(ctx context.Context, req *caasv1.WriteRelationshipsRequest) (*caasv1.WriteRelationshipsResponse, error) {
	if len(req.Updates) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one update required")
	}

	// Convert CAAS proto updates to SpiceDB updates
	var spiceUpdates []*pb.RelationshipUpdate
	for _, u := range req.Updates {
		if u.Relationship == nil || u.Relationship.Resource == nil || u.Relationship.Subject == nil {
			return nil, status.Error(codes.InvalidArgument, "relationship must have resource and subject")
		}

		op := pb.RelationshipUpdate_OPERATION_TOUCH
		switch u.Operation {
		case caasv1.RelationshipUpdate_OPERATION_CREATE:
			op = pb.RelationshipUpdate_OPERATION_TOUCH
		case caasv1.RelationshipUpdate_OPERATION_DELETE:
			op = pb.RelationshipUpdate_OPERATION_DELETE
		case caasv1.RelationshipUpdate_OPERATION_TOUCH:
			op = pb.RelationshipUpdate_OPERATION_TOUCH
		}

		spiceUpdates = append(spiceUpdates, &pb.RelationshipUpdate{
			Operation: op,
			Relationship: &pb.Relationship{
				Resource: &pb.ObjectReference{
					ObjectType: u.Relationship.Resource.ResourceType,
					ObjectId:   u.Relationship.Resource.ResourceId,
				},
				Relation: u.Relationship.Relation,
				Subject: &pb.SubjectReference{
					Object: &pb.ObjectReference{
						ObjectType: u.Relationship.Subject.EntityType,
						ObjectId:   u.Relationship.Subject.EntityId,
					},
				},
			},
		})

		// Invalidate cache for affected subject
		s.cache.InvalidateEntity(ctx, u.Relationship.Subject.EntityType, u.Relationship.Subject.EntityId)
	}

	token, err := s.spicedb.WriteRelationships(ctx, spiceUpdates)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "spicedb write: %v", err)
	}

	log.Printf("WRITE %d relationships", len(req.Updates))

	s.events.Emit("write_relationships", map[string]any{
		"count": len(req.Updates),
	})

	return &caasv1.WriteRelationshipsResponse{
		ConsistencyToken: token,
	}, nil
}

func (s *AuthorizationServer) ReadRelationships(ctx context.Context, req *caasv1.ReadRelationshipsRequest) (*caasv1.ReadRelationshipsResponse, error) {
	resType := ""
	resID := ""
	relation := ""

	if req.Resource != nil {
		resType = req.Resource.ResourceType
		resID = req.Resource.ResourceId
	}
	relation = req.Relation

	rels, err := s.spicedb.ReadRelationships(ctx, resType, resID, relation)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "spicedb read: %v", err)
	}

	var protoRels []*caasv1.Relationship
	for _, r := range rels {
		protoRels = append(protoRels, &caasv1.Relationship{
			Resource: &caasv1.Resource{
				ResourceType: r.Resource.ObjectType,
				ResourceId:   r.Resource.ObjectId,
			},
			Relation: r.Relation,
			Subject: &caasv1.Subject{
				EntityType: r.Subject.Object.ObjectType,
				EntityId:   r.Subject.Object.ObjectId,
			},
		})
	}

	return &caasv1.ReadRelationshipsResponse{
		Relationships: protoRels,
	}, nil
}

func (s *AuthorizationServer) LookupSubjects(req *caasv1.LookupSubjectsRequest, stream grpc.ServerStreamingServer[caasv1.LookupSubjectsResponse]) error {
	if req.Resource == nil {
		return status.Error(codes.InvalidArgument, "resource required")
	}

	subjects, err := s.spicedb.LookupSubjects(stream.Context(), req.Resource.ResourceType, req.Resource.ResourceId, req.Permission, req.SubjectType)
	if err != nil {
		return status.Errorf(codes.Internal, "spicedb lookup subjects: %v", err)
	}

	for _, subID := range subjects {
		if err := stream.Send(&caasv1.LookupSubjectsResponse{
			Subject: &caasv1.Subject{
				EntityType: req.SubjectType,
				EntityId:   subID,
			},
		}); err != nil {
			return err
		}
	}

	return nil
}

func (s *AuthorizationServer) LookupResources(req *caasv1.LookupResourcesRequest, stream grpc.ServerStreamingServer[caasv1.LookupResourcesResponse]) error {
	if req.Subject == nil {
		return status.Error(codes.InvalidArgument, "subject required")
	}

	resources, err := s.spicedb.LookupResources(stream.Context(), req.ResourceType, req.Permission, req.Subject.EntityType, req.Subject.EntityId)
	if err != nil {
		return status.Errorf(codes.Internal, "spicedb lookup resources: %v", err)
	}

	for _, resID := range resources {
		if err := stream.Send(&caasv1.LookupResourcesResponse{
			ResourceId: resID,
		}); err != nil {
			return err
		}
	}

	return nil
}

// WriteSchema loads the CAAS authorization schema into SpiceDB.
func (s *AuthorizationServer) WriteSchema(ctx context.Context, schema string) error {
	return s.spicedb.WriteSchema(ctx, schema)
}

// ValidateEntityType checks if an entity type is one of the 6 CAAS types.
func ValidateEntityType(t string) error {
	valid := map[string]bool{
		"user": true, "organization": true, "device": true,
		"service": true, "ai_agent": true, "autonomous_system": true,
	}
	if !valid[t] {
		return status.Errorf(codes.InvalidArgument, "invalid entity type %q", t)
	}
	return nil
}
