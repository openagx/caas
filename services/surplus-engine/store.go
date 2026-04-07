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

// --- Listings ---

func (s *PostgresStore) CreateListing(ctx context.Context, req *caasv1.CreateListingRequest) (*caasv1.Listing, error) {
	var metaJSON []byte
	if req.Metadata != nil {
		metaJSON, _ = req.Metadata.MarshalJSON()
	} else {
		metaJSON = []byte("{}")
	}

	var lat, lon *float64
	var address, region, country *string
	if req.Location != nil {
		lat = &req.Location.Latitude
		lon = &req.Location.Longitude
		if req.Location.Address != "" {
			address = &req.Location.Address
		}
		if req.Location.Region != "" {
			region = &req.Location.Region
		}
		if req.Location.Country != "" {
			country = &req.Location.Country
		}
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t := req.ExpiresAt.AsTime()
		expiresAt = &t
	}

	row := s.pool.QueryRow(ctx,
		`INSERT INTO surplus_listings
		 (entity_id, listing_type, category, title, description, quantity, unit,
		  urgency, latitude, longitude, address, region, country, max_distance_km,
		  metadata, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 RETURNING id, created_at`,
		req.EntityId, listingTypeToString(req.Type), categoryToString(req.Category),
		req.Title, req.Description, req.Quantity, req.Unit,
		urgencyToString(req.Urgency), lat, lon, address, region, country,
		req.MaxDistanceKm, metaJSON, expiresAt,
	)

	var id string
	var createdAt time.Time
	if err := row.Scan(&id, &createdAt); err != nil {
		return nil, err
	}

	return &caasv1.Listing{
		Id:             id,
		EntityId:       req.EntityId,
		Type:           req.Type,
		Category:       req.Category,
		Title:          req.Title,
		Description:    req.Description,
		Quantity:       req.Quantity,
		Unit:           req.Unit,
		Urgency:        req.Urgency,
		Location:       req.Location,
		MaxDistanceKm:  req.MaxDistanceKm,
		Status:         caasv1.ListingStatus_LISTING_STATUS_ACTIVE,
		Metadata:       req.Metadata,
		CreatedAt:      timestamppb.New(createdAt),
	}, nil
}

func (s *PostgresStore) GetListing(ctx context.Context, id string) (*caasv1.Listing, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, entity_id, listing_type, category, title, description,
		        quantity, unit, urgency, latitude, longitude, address, region, country,
		        max_distance_km, status, metadata, expires_at, created_at, updated_at
		 FROM surplus_listings WHERE id = $1`, id,
	)

	var listing caasv1.Listing
	var typeStr, catStr, urgStr, statusStr string
	var lat, lon *float64
	var address, region, country *string
	var metaJSON []byte
	var expiresAt *time.Time
	var createdAt, updatedAt time.Time

	if err := row.Scan(
		&listing.Id, &listing.EntityId, &typeStr, &catStr,
		&listing.Title, &listing.Description, &listing.Quantity, &listing.Unit,
		&urgStr, &lat, &lon, &address, &region, &country,
		&listing.MaxDistanceKm, &statusStr, &metaJSON,
		&expiresAt, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}

	listing.Type = listingTypeFromString(typeStr)
	listing.Category = categoryFromString(catStr)
	listing.Urgency = urgencyFromString(urgStr)
	listing.Status = listingStatusFromString(statusStr)
	listing.CreatedAt = timestamppb.New(createdAt)
	listing.UpdatedAt = timestamppb.New(updatedAt)

	if lat != nil && lon != nil {
		listing.Location = &caasv1.GeoLocation{Latitude: *lat, Longitude: *lon}
		if address != nil {
			listing.Location.Address = *address
		}
		if region != nil {
			listing.Location.Region = *region
		}
		if country != nil {
			listing.Location.Country = *country
		}
	}

	if expiresAt != nil {
		listing.ExpiresAt = timestamppb.New(*expiresAt)
	}
	if len(metaJSON) > 0 {
		st := &structpb.Struct{}
		if st.UnmarshalJSON(metaJSON) == nil {
			listing.Metadata = st
		}
	}

	return &listing, nil
}

func (s *PostgresStore) ListListings(ctx context.Context, req *caasv1.ListListingsRequest, limit, offset int) ([]*caasv1.Listing, int, error) {
	query := `SELECT id FROM surplus_listings WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM surplus_listings WHERE 1=1`
	args := []any{}
	argIdx := 1

	if req.TypeFilter != caasv1.ListingType_LISTING_TYPE_UNSPECIFIED {
		query += fmt.Sprintf(` AND listing_type = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND listing_type = $%d`, argIdx)
		args = append(args, listingTypeToString(req.TypeFilter))
		argIdx++
	}
	if req.CategoryFilter != caasv1.ResourceCategory_RESOURCE_CATEGORY_UNSPECIFIED {
		query += fmt.Sprintf(` AND category = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND category = $%d`, argIdx)
		args = append(args, categoryToString(req.CategoryFilter))
		argIdx++
	}
	if req.StatusFilter != caasv1.ListingStatus_LISTING_STATUS_UNSPECIFIED {
		query += fmt.Sprintf(` AND status = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND status = $%d`, argIdx)
		args = append(args, listingStatusToString(req.StatusFilter))
		argIdx++
	}
	if req.EntityId != "" {
		query += fmt.Sprintf(` AND entity_id = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND entity_id = $%d`, argIdx)
		args = append(args, req.EntityId)
		argIdx++
	}

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

	var listings []*caasv1.Listing
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		l, err := s.GetListing(ctx, id)
		if err != nil {
			continue
		}
		listings = append(listings, l)
	}

	return listings, total, nil
}

func (s *PostgresStore) UpdateListing(ctx context.Context, req *caasv1.UpdateListingRequest) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE surplus_listings SET
		   quantity = COALESCE(NULLIF($2, 0), quantity),
		   urgency = COALESCE(NULLIF($3, ''), urgency),
		   status = COALESCE(NULLIF($4, ''), status),
		   updated_at = now()
		 WHERE id = $1`,
		req.ListingId,
		req.Quantity,
		urgencyToString(req.Urgency),
		listingStatusToString(req.Status),
	)
	return err
}

func (s *PostgresStore) CancelListing(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE surplus_listings SET status = 'cancelled', updated_at = now() WHERE id = $1`, id,
	)
	return err
}

// --- Matches ---

func (s *PostgresStore) CreateMatch(ctx context.Context,
	surplusListingID, needListingID, surplusEntityID, needEntityID string,
	distanceKm, matchScore float64, matchedQty int32, unit string,
) (*caasv1.Match, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO surplus_matches
		 (surplus_listing_id, need_listing_id, surplus_entity_id, need_entity_id,
		  distance_km, match_score, matched_quantity, unit)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, created_at`,
		surplusListingID, needListingID, surplusEntityID, needEntityID,
		distanceKm, matchScore, matchedQty, unit,
	)

	var id string
	var createdAt time.Time
	if err := row.Scan(&id, &createdAt); err != nil {
		return nil, err
	}

	return &caasv1.Match{
		Id:               id,
		SurplusListingId: surplusListingID,
		NeedListingId:    needListingID,
		SurplusEntityId:  surplusEntityID,
		NeedEntityId:     needEntityID,
		DistanceKm:       distanceKm,
		MatchScore:       matchScore,
		Status:           caasv1.MatchStatus_MATCH_STATUS_PROPOSED,
		MatchedQuantity:  matchedQty,
		Unit:             unit,
		CreatedAt:        timestamppb.New(createdAt),
	}, nil
}

func (s *PostgresStore) GetMatch(ctx context.Context, id string) (*caasv1.Match, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, surplus_listing_id, need_listing_id, surplus_entity_id, need_entity_id,
		        distance_km, match_score, status, matched_quantity, unit, logistics,
		        created_at, updated_at
		 FROM surplus_matches WHERE id = $1`, id,
	)

	var match caasv1.Match
	var statusStr string
	var logisticsJSON []byte
	var createdAt, updatedAt time.Time

	if err := row.Scan(
		&match.Id, &match.SurplusListingId, &match.NeedListingId,
		&match.SurplusEntityId, &match.NeedEntityId,
		&match.DistanceKm, &match.MatchScore, &statusStr,
		&match.MatchedQuantity, &match.Unit, &logisticsJSON,
		&createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}

	match.Status = matchStatusFromString(statusStr)
	match.CreatedAt = timestamppb.New(createdAt)
	match.UpdatedAt = timestamppb.New(updatedAt)

	if len(logisticsJSON) > 0 {
		st := &structpb.Struct{}
		if st.UnmarshalJSON(logisticsJSON) == nil {
			match.Logistics = st
		}
	}

	return &match, nil
}

func (s *PostgresStore) UpdateMatchStatus(ctx context.Context, id, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE surplus_matches SET status = $2, updated_at = now() WHERE id = $1`,
		id, status,
	)
	return err
}

func (s *PostgresStore) ListMatches(ctx context.Context, entityID string, statusFilter caasv1.MatchStatus, limit, offset int) ([]*caasv1.Match, int, error) {
	query := `SELECT id FROM surplus_matches WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM surplus_matches WHERE 1=1`
	args := []any{}
	argIdx := 1

	if entityID != "" {
		query += fmt.Sprintf(` AND (surplus_entity_id = $%d OR need_entity_id = $%d)`, argIdx, argIdx)
		countQuery += fmt.Sprintf(` AND (surplus_entity_id = $%d OR need_entity_id = $%d)`, argIdx, argIdx)
		args = append(args, entityID)
		argIdx++
	}
	if statusFilter != caasv1.MatchStatus_MATCH_STATUS_UNSPECIFIED {
		query += fmt.Sprintf(` AND status = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND status = $%d`, argIdx)
		args = append(args, matchStatusToString(statusFilter))
		argIdx++
	}

	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query += fmt.Sprintf(` ORDER BY match_score DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var matches []*caasv1.Match
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		m, err := s.GetMatch(ctx, id)
		if err != nil {
			continue
		}
		matches = append(matches, m)
	}

	return matches, total, nil
}

// --- Quality Verification ---

func (s *PostgresStore) CreateQualityVerification(ctx context.Context,
	matchID, verifierEntityID string, qualityScore int32, meetsRequirements bool, notes string,
) (*caasv1.QualityVerification, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO quality_verifications (match_id, verifier_entity_id, quality_score, meets_requirements, notes)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, verified_at`,
		matchID, verifierEntityID, qualityScore, meetsRequirements, notes,
	)

	var id string
	var verifiedAt time.Time
	if err := row.Scan(&id, &verifiedAt); err != nil {
		return nil, err
	}

	return &caasv1.QualityVerification{
		Id:                id,
		MatchId:           matchID,
		VerifierEntityId:  verifierEntityID,
		QualityScore:      qualityScore,
		MeetsRequirements: meetsRequirements,
		Notes:             notes,
		VerifiedAt:        timestamppb.New(verifiedAt),
	}, nil
}

// --- Metrics ---

func (s *PostgresStore) GetMetrics(ctx context.Context) (*caasv1.GetSurplusMetricsResponse, error) {
	metrics := &caasv1.GetSurplusMetricsResponse{
		ListingsByCategory: make(map[string]int32),
	}

	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM surplus_listings`).Scan(&metrics.TotalListings)
	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM surplus_listings WHERE listing_type = 'surplus' AND status = 'active'`).Scan(&metrics.ActiveSurplus)
	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM surplus_listings WHERE listing_type = 'need' AND status = 'active'`).Scan(&metrics.ActiveNeeds)
	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM surplus_matches`).Scan(&metrics.TotalMatches)
	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM surplus_matches WHERE status = 'verified'`).Scan(&metrics.FulfilledMatches)
	s.pool.QueryRow(ctx, `SELECT COALESCE(AVG(distance_km), 0) FROM surplus_matches`).Scan(&metrics.AvgMatchDistanceKm)
	s.pool.QueryRow(ctx, `SELECT COALESCE(AVG(quality_score), 0) FROM quality_verifications`).Scan(&metrics.AvgQualityScore)

	rows, err := s.pool.Query(ctx, `SELECT category, COUNT(*) FROM surplus_listings GROUP BY category`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var cat string
			var count int32
			if rows.Scan(&cat, &count) == nil {
				metrics.ListingsByCategory[cat] = count
			}
		}
	}

	return metrics, nil
}

// --- Converters ---

func listingTypeToString(t caasv1.ListingType) string {
	switch t {
	case caasv1.ListingType_LISTING_TYPE_SURPLUS:
		return "surplus"
	case caasv1.ListingType_LISTING_TYPE_NEED:
		return "need"
	default:
		return "surplus"
	}
}

func listingTypeFromString(s string) caasv1.ListingType {
	switch s {
	case "surplus":
		return caasv1.ListingType_LISTING_TYPE_SURPLUS
	case "need":
		return caasv1.ListingType_LISTING_TYPE_NEED
	default:
		return caasv1.ListingType_LISTING_TYPE_UNSPECIFIED
	}
}

func listingStatusToString(s caasv1.ListingStatus) string {
	switch s {
	case caasv1.ListingStatus_LISTING_STATUS_ACTIVE:
		return "active"
	case caasv1.ListingStatus_LISTING_STATUS_MATCHED:
		return "matched"
	case caasv1.ListingStatus_LISTING_STATUS_FULFILLED:
		return "fulfilled"
	case caasv1.ListingStatus_LISTING_STATUS_EXPIRED:
		return "expired"
	case caasv1.ListingStatus_LISTING_STATUS_CANCELLED:
		return "cancelled"
	default:
		return ""
	}
}

func listingStatusFromString(s string) caasv1.ListingStatus {
	switch s {
	case "active":
		return caasv1.ListingStatus_LISTING_STATUS_ACTIVE
	case "matched":
		return caasv1.ListingStatus_LISTING_STATUS_MATCHED
	case "fulfilled":
		return caasv1.ListingStatus_LISTING_STATUS_FULFILLED
	case "expired":
		return caasv1.ListingStatus_LISTING_STATUS_EXPIRED
	case "cancelled":
		return caasv1.ListingStatus_LISTING_STATUS_CANCELLED
	default:
		return caasv1.ListingStatus_LISTING_STATUS_UNSPECIFIED
	}
}

func categoryToString(c caasv1.ResourceCategory) string {
	switch c {
	case caasv1.ResourceCategory_RESOURCE_CATEGORY_FOOD:
		return "food"
	case caasv1.ResourceCategory_RESOURCE_CATEGORY_MEDICAL:
		return "medical"
	case caasv1.ResourceCategory_RESOURCE_CATEGORY_SHELTER:
		return "shelter"
	case caasv1.ResourceCategory_RESOURCE_CATEGORY_CLOTHING:
		return "clothing"
	case caasv1.ResourceCategory_RESOURCE_CATEGORY_EQUIPMENT:
		return "equipment"
	case caasv1.ResourceCategory_RESOURCE_CATEGORY_TRANSPORT:
		return "transport"
	case caasv1.ResourceCategory_RESOURCE_CATEGORY_COMPUTE:
		return "compute"
	case caasv1.ResourceCategory_RESOURCE_CATEGORY_ENERGY:
		return "energy"
	case caasv1.ResourceCategory_RESOURCE_CATEGORY_LABOR:
		return "labor"
	case caasv1.ResourceCategory_RESOURCE_CATEGORY_OTHER:
		return "other"
	default:
		return "other"
	}
}

func categoryFromString(s string) caasv1.ResourceCategory {
	switch s {
	case "food":
		return caasv1.ResourceCategory_RESOURCE_CATEGORY_FOOD
	case "medical":
		return caasv1.ResourceCategory_RESOURCE_CATEGORY_MEDICAL
	case "shelter":
		return caasv1.ResourceCategory_RESOURCE_CATEGORY_SHELTER
	case "clothing":
		return caasv1.ResourceCategory_RESOURCE_CATEGORY_CLOTHING
	case "equipment":
		return caasv1.ResourceCategory_RESOURCE_CATEGORY_EQUIPMENT
	case "transport":
		return caasv1.ResourceCategory_RESOURCE_CATEGORY_TRANSPORT
	case "compute":
		return caasv1.ResourceCategory_RESOURCE_CATEGORY_COMPUTE
	case "energy":
		return caasv1.ResourceCategory_RESOURCE_CATEGORY_ENERGY
	case "labor":
		return caasv1.ResourceCategory_RESOURCE_CATEGORY_LABOR
	case "other":
		return caasv1.ResourceCategory_RESOURCE_CATEGORY_OTHER
	default:
		return caasv1.ResourceCategory_RESOURCE_CATEGORY_UNSPECIFIED
	}
}

func urgencyToString(u caasv1.UrgencyLevel) string {
	switch u {
	case caasv1.UrgencyLevel_URGENCY_LEVEL_LOW:
		return "low"
	case caasv1.UrgencyLevel_URGENCY_LEVEL_MEDIUM:
		return "medium"
	case caasv1.UrgencyLevel_URGENCY_LEVEL_HIGH:
		return "high"
	case caasv1.UrgencyLevel_URGENCY_LEVEL_CRITICAL:
		return "critical"
	default:
		return ""
	}
}

func urgencyFromString(s string) caasv1.UrgencyLevel {
	switch s {
	case "low":
		return caasv1.UrgencyLevel_URGENCY_LEVEL_LOW
	case "medium":
		return caasv1.UrgencyLevel_URGENCY_LEVEL_MEDIUM
	case "high":
		return caasv1.UrgencyLevel_URGENCY_LEVEL_HIGH
	case "critical":
		return caasv1.UrgencyLevel_URGENCY_LEVEL_CRITICAL
	default:
		return caasv1.UrgencyLevel_URGENCY_LEVEL_UNSPECIFIED
	}
}

func matchStatusToString(s caasv1.MatchStatus) string {
	switch s {
	case caasv1.MatchStatus_MATCH_STATUS_PROPOSED:
		return "proposed"
	case caasv1.MatchStatus_MATCH_STATUS_ACCEPTED:
		return "accepted"
	case caasv1.MatchStatus_MATCH_STATUS_IN_TRANSIT:
		return "in_transit"
	case caasv1.MatchStatus_MATCH_STATUS_DELIVERED:
		return "delivered"
	case caasv1.MatchStatus_MATCH_STATUS_VERIFIED:
		return "verified"
	case caasv1.MatchStatus_MATCH_STATUS_REJECTED:
		return "rejected"
	case caasv1.MatchStatus_MATCH_STATUS_CANCELLED:
		return "cancelled"
	default:
		return "proposed"
	}
}

func matchStatusFromString(s string) caasv1.MatchStatus {
	switch s {
	case "proposed":
		return caasv1.MatchStatus_MATCH_STATUS_PROPOSED
	case "accepted":
		return caasv1.MatchStatus_MATCH_STATUS_ACCEPTED
	case "in_transit":
		return caasv1.MatchStatus_MATCH_STATUS_IN_TRANSIT
	case "delivered":
		return caasv1.MatchStatus_MATCH_STATUS_DELIVERED
	case "verified":
		return caasv1.MatchStatus_MATCH_STATUS_VERIFIED
	case "rejected":
		return caasv1.MatchStatus_MATCH_STATUS_REJECTED
	case "cancelled":
		return caasv1.MatchStatus_MATCH_STATUS_CANCELLED
	default:
		return caasv1.MatchStatus_MATCH_STATUS_UNSPECIFIED
	}
}

var _ = json.Marshal
