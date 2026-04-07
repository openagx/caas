package main

import (
	"context"
	"log"
	"math"
	"sort"

	caasv1 "github.com/OpenAGX/caas/gen/go/caas/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type SurplusServer struct {
	caasv1.UnimplementedSurplusServiceServer
	store  *PostgresStore
	events *EventProducer
}

func NewSurplusServer(store *PostgresStore, events *EventProducer) *SurplusServer {
	return &SurplusServer{store: store, events: events}
}

// --- Listing Management ---

func (s *SurplusServer) CreateListing(ctx context.Context, req *caasv1.CreateListingRequest) (*caasv1.CreateListingResponse, error) {
	if req.EntityId == "" || req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id and title are required")
	}
	if req.Type == caasv1.ListingType_LISTING_TYPE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "listing type is required")
	}
	if req.Category == caasv1.ResourceCategory_RESOURCE_CATEGORY_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "category is required")
	}

	listing, err := s.store.CreateListing(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create listing: %v", err)
	}

	s.events.Emit("listing.created", map[string]any{
		"listing_id": listing.Id,
		"entity_id":  req.EntityId,
		"type":       listingTypeToString(req.Type),
		"category":   categoryToString(req.Category),
	})

	return &caasv1.CreateListingResponse{Listing: listing}, nil
}

func (s *SurplusServer) GetListing(ctx context.Context, req *caasv1.GetListingRequest) (*caasv1.GetListingResponse, error) {
	if req.ListingId == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}

	listing, err := s.store.GetListing(ctx, req.ListingId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "listing not found: %v", err)
	}

	return &caasv1.GetListingResponse{Listing: listing}, nil
}

func (s *SurplusServer) ListListings(ctx context.Context, req *caasv1.ListListingsRequest) (*caasv1.ListListingsResponse, error) {
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 20
	}
	page := int(req.Page)
	if page <= 0 {
		page = 1
	}

	listings, total, err := s.store.ListListings(ctx, req, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list listings: %v", err)
	}

	return &caasv1.ListListingsResponse{Listings: listings, Total: int32(total)}, nil
}

func (s *SurplusServer) UpdateListing(ctx context.Context, req *caasv1.UpdateListingRequest) (*caasv1.UpdateListingResponse, error) {
	if req.ListingId == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}

	if err := s.store.UpdateListing(ctx, req); err != nil {
		return nil, status.Errorf(codes.Internal, "update listing: %v", err)
	}

	listing, err := s.store.GetListing(ctx, req.ListingId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get listing: %v", err)
	}

	return &caasv1.UpdateListingResponse{Listing: listing}, nil
}

func (s *SurplusServer) CancelListing(ctx context.Context, req *caasv1.CancelListingRequest) (*caasv1.CancelListingResponse, error) {
	if req.ListingId == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}

	if err := s.store.CancelListing(ctx, req.ListingId); err != nil {
		return nil, status.Errorf(codes.Internal, "cancel listing: %v", err)
	}

	listing, err := s.store.GetListing(ctx, req.ListingId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get listing: %v", err)
	}

	s.events.Emit("listing.cancelled", map[string]any{
		"listing_id": listing.Id,
		"reason":     req.Reason,
	})

	return &caasv1.CancelListingResponse{Listing: listing}, nil
}

// --- Matching Engine ---

func (s *SurplusServer) FindMatches(ctx context.Context, req *caasv1.FindMatchesRequest) (*caasv1.FindMatchesResponse, error) {
	if req.ListingId == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}

	maxResults := int(req.MaxResults)
	if maxResults <= 0 {
		maxResults = 10
	}

	// Get the source listing
	source, err := s.store.GetListing(ctx, req.ListingId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "listing not found: %v", err)
	}

	// Find opposite type (surplus→need, need→surplus)
	oppositeType := caasv1.ListingType_LISTING_TYPE_NEED
	if source.Type == caasv1.ListingType_LISTING_TYPE_NEED {
		oppositeType = caasv1.ListingType_LISTING_TYPE_SURPLUS
	}

	// Get candidate listings
	candidates, _, err := s.store.ListListings(ctx, &caasv1.ListListingsRequest{
		TypeFilter:     oppositeType,
		CategoryFilter: source.Category,
		StatusFilter:   caasv1.ListingStatus_LISTING_STATUS_ACTIVE,
	}, 100, 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "find candidates: %v", err)
	}

	// Score and rank candidates
	type scoredMatch struct {
		listing  *caasv1.Listing
		score    float64
		distance float64
	}

	var scored []scoredMatch
	for _, candidate := range candidates {
		if candidate.Id == source.Id {
			continue
		}

		distance := haversineDistance(source.Location, candidate.Location)
		maxDist := source.MaxDistanceKm
		if maxDist <= 0 {
			maxDist = 100
		}

		// Skip if beyond max distance
		if distance > maxDist {
			continue
		}

		score := computeMatchScore(source, candidate, distance)
		scored = append(scored, scoredMatch{
			listing:  candidate,
			score:    score,
			distance: distance,
		})
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Create matches (cap at maxResults)
	if len(scored) > maxResults {
		scored = scored[:maxResults]
	}

	var matches []*caasv1.Match
	for _, sm := range scored {
		surplusID := source.Id
		needID := sm.listing.Id
		surplusEntityID := source.EntityId
		needEntityID := sm.listing.EntityId

		if source.Type == caasv1.ListingType_LISTING_TYPE_NEED {
			surplusID, needID = needID, surplusID
			surplusEntityID, needEntityID = needEntityID, surplusEntityID
		}

		matchedQty := min32(source.Quantity, sm.listing.Quantity)

		match, err := s.store.CreateMatch(ctx,
			surplusID, needID, surplusEntityID, needEntityID,
			sm.distance, sm.score, matchedQty, source.Unit,
		)
		if err != nil {
			log.Printf("WARN: failed to create match: %v", err)
			continue
		}
		matches = append(matches, match)

		s.events.Emit("match.proposed", map[string]any{
			"match_id":    match.Id,
			"distance_km": sm.distance,
			"score":       sm.score,
		})
	}

	return &caasv1.FindMatchesResponse{Matches: matches}, nil
}

func (s *SurplusServer) AcceptMatch(ctx context.Context, req *caasv1.AcceptMatchRequest) (*caasv1.AcceptMatchResponse, error) {
	if req.MatchId == "" {
		return nil, status.Error(codes.InvalidArgument, "match_id is required")
	}

	if err := s.store.UpdateMatchStatus(ctx, req.MatchId, "accepted"); err != nil {
		return nil, status.Errorf(codes.Internal, "accept match: %v", err)
	}

	match, err := s.store.GetMatch(ctx, req.MatchId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get match: %v", err)
	}

	s.events.Emit("match.accepted", map[string]any{"match_id": match.Id})

	return &caasv1.AcceptMatchResponse{Match: match}, nil
}

func (s *SurplusServer) UpdateMatchStatus(ctx context.Context, req *caasv1.UpdateMatchStatusRequest) (*caasv1.UpdateMatchStatusResponse, error) {
	if req.MatchId == "" {
		return nil, status.Error(codes.InvalidArgument, "match_id is required")
	}

	statusStr := matchStatusToString(req.Status)
	if err := s.store.UpdateMatchStatus(ctx, req.MatchId, statusStr); err != nil {
		return nil, status.Errorf(codes.Internal, "update match status: %v", err)
	}

	match, err := s.store.GetMatch(ctx, req.MatchId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get match: %v", err)
	}

	s.events.Emit("match.status_updated", map[string]any{
		"match_id": match.Id,
		"status":   statusStr,
	})

	return &caasv1.UpdateMatchStatusResponse{Match: match}, nil
}

func (s *SurplusServer) GetMatch(ctx context.Context, req *caasv1.GetMatchRequest) (*caasv1.GetMatchResponse, error) {
	if req.MatchId == "" {
		return nil, status.Error(codes.InvalidArgument, "match_id is required")
	}

	match, err := s.store.GetMatch(ctx, req.MatchId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "match not found: %v", err)
	}

	return &caasv1.GetMatchResponse{Match: match}, nil
}

func (s *SurplusServer) ListMatches(ctx context.Context, req *caasv1.ListMatchesRequest) (*caasv1.ListMatchesResponse, error) {
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 20
	}
	page := int(req.Page)
	if page <= 0 {
		page = 1
	}

	matches, total, err := s.store.ListMatches(ctx, req.EntityId, req.StatusFilter, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list matches: %v", err)
	}

	return &caasv1.ListMatchesResponse{Matches: matches, Total: int32(total)}, nil
}

// --- Quality Verification ---

func (s *SurplusServer) VerifyQuality(ctx context.Context, req *caasv1.VerifyQualityRequest) (*caasv1.VerifyQualityResponse, error) {
	if req.MatchId == "" || req.VerifierEntityId == "" {
		return nil, status.Error(codes.InvalidArgument, "match_id and verifier_entity_id are required")
	}
	if req.QualityScore < 0 || req.QualityScore > 100 {
		return nil, status.Error(codes.InvalidArgument, "quality_score must be between 0 and 100")
	}

	verification, err := s.store.CreateQualityVerification(ctx,
		req.MatchId, req.VerifierEntityId,
		req.QualityScore, req.MeetsRequirements, req.Notes,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "verify quality: %v", err)
	}

	// If quality verified, mark match as verified
	if req.MeetsRequirements {
		_ = s.store.UpdateMatchStatus(ctx, req.MatchId, "verified")
	}

	s.events.Emit("quality.verified", map[string]any{
		"match_id":           req.MatchId,
		"quality_score":      req.QualityScore,
		"meets_requirements": req.MeetsRequirements,
	})

	return &caasv1.VerifyQualityResponse{Verification: verification}, nil
}

// --- Metrics ---

func (s *SurplusServer) GetSurplusMetrics(ctx context.Context, req *caasv1.GetSurplusMetricsRequest) (*caasv1.GetSurplusMetricsResponse, error) {
	metrics, err := s.store.GetMetrics(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get metrics: %v", err)
	}
	return metrics, nil
}

// --- Matching Helpers ---

// haversineDistance calculates the great-circle distance between two points in km.
func haversineDistance(a, b *caasv1.GeoLocation) float64 {
	if a == nil || b == nil {
		return 0
	}
	const R = 6371.0 // Earth radius in km
	lat1 := a.Latitude * math.Pi / 180
	lat2 := b.Latitude * math.Pi / 180
	dLat := (b.Latitude - a.Latitude) * math.Pi / 180
	dLon := (b.Longitude - a.Longitude) * math.Pi / 180

	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
	return R * c
}

// computeMatchScore produces a 0-100 composite score from distance, urgency, and quantity.
func computeMatchScore(source, candidate *caasv1.Listing, distanceKm float64) float64 {
	// Distance score (closer = better): 40% weight
	maxDist := source.MaxDistanceKm
	if maxDist <= 0 {
		maxDist = 100
	}
	distanceScore := math.Max(0, 100*(1-distanceKm/maxDist)) * 0.4

	// Urgency score: 30% weight
	urgencyScore := float64(urgencyValue(candidate.Urgency)) * 25 * 0.3

	// Quantity compatibility: 30% weight
	qtyRatio := float64(min32(source.Quantity, candidate.Quantity)) / float64(max32(source.Quantity, 1))
	quantityScore := qtyRatio * 100 * 0.3

	return distanceScore + urgencyScore + quantityScore
}

func urgencyValue(u caasv1.UrgencyLevel) int {
	switch u {
	case caasv1.UrgencyLevel_URGENCY_LEVEL_CRITICAL:
		return 4
	case caasv1.UrgencyLevel_URGENCY_LEVEL_HIGH:
		return 3
	case caasv1.UrgencyLevel_URGENCY_LEVEL_MEDIUM:
		return 2
	case caasv1.UrgencyLevel_URGENCY_LEVEL_LOW:
		return 1
	default:
		return 1
	}
}

func min32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}
