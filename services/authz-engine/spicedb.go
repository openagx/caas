package main

import (
	"context"
	"fmt"

	authzed "github.com/authzed/authzed-go/v1"
	pb "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/grpcutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// SpiceDBClient wraps the authzed-go client.
type SpiceDBClient struct {
	client *authzed.ClientWithExperimental
}

// NewSpiceDBClient creates a connection to SpiceDB.
func NewSpiceDBClient(endpoint, presharedKey string) (*SpiceDBClient, error) {
	client, err := authzed.NewClientWithExperimentalAPIs(
		endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpcutil.WithInsecureBearerToken(presharedKey),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to SpiceDB at %s: %w", endpoint, err)
	}
	return &SpiceDBClient{client: client}, nil
}

// Check verifies if a subject has a permission on a resource.
func (s *SpiceDBClient) Check(ctx context.Context, resourceType, resourceID, permission, subjectType, subjectID string) (bool, string, error) {
	resp, err := s.client.CheckPermission(ctx, &pb.CheckPermissionRequest{
		Resource:   &pb.ObjectReference{ObjectType: resourceType, ObjectId: resourceID},
		Permission: permission,
		Subject:    &pb.SubjectReference{Object: &pb.ObjectReference{ObjectType: subjectType, ObjectId: subjectID}},
		Consistency: &pb.Consistency{
			Requirement: &pb.Consistency_FullyConsistent{FullyConsistent: true},
		},
	})
	if err != nil {
		return false, "", fmt.Errorf("check permission: %w", err)
	}

	token := ""
	if resp.CheckedAt != nil {
		token = resp.CheckedAt.Token
	}

	return resp.Permissionship == pb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION, token, nil
}

// WriteRelationship creates or deletes a relationship tuple.
func (s *SpiceDBClient) WriteRelationship(ctx context.Context, op pb.RelationshipUpdate_Operation, resourceType, resourceID, relation, subjectType, subjectID string) (string, error) {
	resp, err := s.client.WriteRelationships(ctx, &pb.WriteRelationshipsRequest{
		Updates: []*pb.RelationshipUpdate{
			{
				Operation: op,
				Relationship: &pb.Relationship{
					Resource: &pb.ObjectReference{ObjectType: resourceType, ObjectId: resourceID},
					Relation: relation,
					Subject:  &pb.SubjectReference{Object: &pb.ObjectReference{ObjectType: subjectType, ObjectId: subjectID}},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("write relationship: %w", err)
	}
	return resp.WrittenAt.Token, nil
}

// WriteRelationships writes multiple relationship tuples atomically.
func (s *SpiceDBClient) WriteRelationships(ctx context.Context, updates []*pb.RelationshipUpdate) (string, error) {
	resp, err := s.client.WriteRelationships(ctx, &pb.WriteRelationshipsRequest{
		Updates: updates,
	})
	if err != nil {
		return "", fmt.Errorf("write relationships: %w", err)
	}
	return resp.WrittenAt.Token, nil
}

// ReadRelationships reads tuples matching a filter.
func (s *SpiceDBClient) ReadRelationships(ctx context.Context, resourceType, resourceID, relation string) ([]*pb.Relationship, error) {
	filter := &pb.RelationshipFilter{
		ResourceType: resourceType,
	}
	if resourceID != "" {
		filter.OptionalResourceId = resourceID
	}
	if relation != "" {
		filter.OptionalRelation = relation
	}

	stream, err := s.client.ReadRelationships(ctx, &pb.ReadRelationshipsRequest{
		RelationshipFilter: filter,
		Consistency: &pb.Consistency{
			Requirement: &pb.Consistency_FullyConsistent{FullyConsistent: true},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("read relationships: %w", err)
	}

	var rels []*pb.Relationship
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		rels = append(rels, resp.Relationship)
	}
	return rels, nil
}

// LookupSubjects finds all subjects with a permission on a resource.
func (s *SpiceDBClient) LookupSubjects(ctx context.Context, resourceType, resourceID, permission, subjectType string) ([]string, error) {
	stream, err := s.client.LookupSubjects(ctx, &pb.LookupSubjectsRequest{
		Resource:          &pb.ObjectReference{ObjectType: resourceType, ObjectId: resourceID},
		Permission:        permission,
		SubjectObjectType: subjectType,
		Consistency: &pb.Consistency{
			Requirement: &pb.Consistency_FullyConsistent{FullyConsistent: true},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("lookup subjects: %w", err)
	}

	var subjects []string
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		subjects = append(subjects, resp.Subject.SubjectObjectId)
	}
	return subjects, nil
}

// LookupResources finds all resources a subject has a permission on.
func (s *SpiceDBClient) LookupResources(ctx context.Context, resourceType, permission, subjectType, subjectID string) ([]string, error) {
	stream, err := s.client.LookupResources(ctx, &pb.LookupResourcesRequest{
		ResourceObjectType: resourceType,
		Permission:         permission,
		Subject:            &pb.SubjectReference{Object: &pb.ObjectReference{ObjectType: subjectType, ObjectId: subjectID}},
		Consistency: &pb.Consistency{
			Requirement: &pb.Consistency_FullyConsistent{FullyConsistent: true},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("lookup resources: %w", err)
	}

	var resources []string
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		resources = append(resources, resp.ResourceObjectId)
	}
	return resources, nil
}

// WriteSchema writes the authorization schema to SpiceDB.
func (s *SpiceDBClient) WriteSchema(ctx context.Context, schema string) error {
	_, err := s.client.WriteSchema(ctx, &pb.WriteSchemaRequest{
		Schema: schema,
	})
	return err
}
