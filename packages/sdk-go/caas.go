// Package caas provides a Go client SDK for the CAAS API Gateway.
//
// Usage:
//
//	client := caas.NewClient("http://localhost:3001")
//	entity, err := client.Entities.Create(ctx, caas.EntityTypeHuman, "Alice", nil)
//	result, err := client.Authz.Check(ctx, caas.Ref("user", entity.ID), "view", caas.Ref("document", "doc-1"))
package caas

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// EntityType represents the 6 CAAS entity types.
type EntityType int

const (
	EntityTypeUnspecified     EntityType = 0
	EntityTypeHuman          EntityType = 1
	EntityTypeOrganization   EntityType = 2
	EntityTypeDevice         EntityType = 3
	EntityTypeService        EntityType = 4
	EntityTypeAIAgent        EntityType = 5
	EntityTypeAutonomous     EntityType = 6
)

// AuthorityTier represents the 5-tier authority override hierarchy.
type AuthorityTier int

const (
	AuthorityTierIndividual        AuthorityTier = 1
	AuthorityTierInstitutional     AuthorityTier = 2
	AuthorityTierRegulatory        AuthorityTier = 3
	AuthorityTierJudicial          AuthorityTier = 4
	AuthorityTierSovereignEmergency AuthorityTier = 5
)

// ObjectRef is a typed object reference (e.g., user:alice).
type ObjectRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Ref creates an ObjectRef.
func Ref(objectType, id string) ObjectRef {
	return ObjectRef{Type: objectType, ID: id}
}

// Entity represents a CAAS entity.
type Entity struct {
	ID             string                 `json:"id"`
	DID            string                 `json:"did,omitempty"`
	EntityType     EntityType             `json:"entityType"`
	DisplayName    string                 `json:"displayName"`
	LifecycleState int                    `json:"lifecycleState"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      string                 `json:"createdAt"`
	UpdatedAt      string                 `json:"updatedAt"`
}

// CheckResult represents an authorization check result.
type CheckResult struct {
	Allowed   bool   `json:"allowed"`
	CheckedAt string `json:"checkedAt,omitempty"`
}

// TrustScore represents a trust score with 7 dimensions.
type TrustScore struct {
	EntityID     string                 `json:"entityId"`
	OverallScore int                    `json:"overallScore"`
	Dimensions   map[string]interface{} `json:"dimensions"`
	CalculatedAt string                 `json:"calculatedAt"`
}

// TrustAttestation is a privacy-preserving proof that trust meets a threshold.
type TrustAttestation struct {
	EntityID        string `json:"entityId"`
	MeetsThreshold  bool   `json:"meetsThreshold"`
	Threshold       int    `json:"threshold"`
	AttestationHash string `json:"attestationHash"`
	ExpiresAt       string `json:"expiresAt"`
	IssuedAt        string `json:"issuedAt"`
}

// FraudIncident represents a detected fraud incident.
type FraudIncident struct {
	ID                    string                 `json:"id"`
	IncidentType          string                 `json:"incident_type"`
	Severity              string                 `json:"severity"`
	Status                string                 `json:"status"`
	AffectedEntityID      string                 `json:"affected_entity_id"`
	DetectionLatencyMs    int                    `json:"detection_latency_ms"`
	Evidence              map[string]interface{} `json:"evidence"`
	Timeline              []map[string]interface{} `json:"timeline"`
	CreatedAt             string                 `json:"created_at"`
}

// Client is the main CAAS SDK client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client

	Entities   *EntityClient
	Authz      *AuthzClient
	Trust      *TrustClient
	Fraud      *FraudClient
	Decisions  *DecisionClient
	Federation *FederationClient
	Credentials *CredentialClient
}

// NewClient creates a new CAAS client.
func NewClient(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}

	c.Entities = &EntityClient{c}
	c.Authz = &AuthzClient{c}
	c.Trust = &TrustClient{c}
	c.Fraud = &FraudClient{c}
	c.Decisions = &DecisionClient{c}
	c.Federation = &FederationClient{c}
	c.Credentials = &CredentialClient{c}

	return c
}

// Option configures the client.
type Option func(*Client)

// WithAPIKey sets the API key for authentication.
func WithAPIKey(key string) Option {
	return func(c *Client) { c.apiKey = key }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithTimeout sets the request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient.Timeout = d }
}

func (c *Client) do(ctx context.Context, method, path string, body, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(errBody), Path: path}
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

// APIError represents an error from the CAAS API.
type APIError struct {
	StatusCode int
	Body       string
	Path       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("CAAS API error %d on %s: %s", e.StatusCode, e.Path, e.Body)
}

// Health checks the API gateway health.
func (c *Client) Health(ctx context.Context) error {
	var result map[string]string
	return c.do(ctx, "GET", "/health", nil, &result)
}

// --- Entity Client ---

type EntityClient struct{ c *Client }

func (e *EntityClient) Create(ctx context.Context, entityType EntityType, displayName string, metadata map[string]interface{}) (*Entity, error) {
	var result struct{ Entity *Entity `json:"entity"` }
	err := e.c.do(ctx, "POST", "/v1/entities", map[string]interface{}{
		"entity_type":  entityType,
		"display_name": displayName,
		"metadata":     metadata,
	}, &result)
	return result.Entity, err
}

func (e *EntityClient) Get(ctx context.Context, id string) (*Entity, error) {
	var result struct{ Entity *Entity `json:"entity"` }
	err := e.c.do(ctx, "GET", "/v1/entities/"+id, nil, &result)
	return result.Entity, err
}

func (e *EntityClient) List(ctx context.Context, pageSize int) ([]Entity, error) {
	var result struct{ Entities []Entity `json:"entities"` }
	err := e.c.do(ctx, "GET", fmt.Sprintf("/v1/entities?page_size=%d", pageSize), nil, &result)
	return result.Entities, err
}

func (e *EntityClient) Activate(ctx context.Context, id string) (*Entity, error) {
	var result struct{ Entity *Entity `json:"entity"` }
	err := e.c.do(ctx, "POST", "/v1/entities/"+id+"/activate", nil, &result)
	return result.Entity, err
}

func (e *EntityClient) Suspend(ctx context.Context, id, reason string) (*Entity, error) {
	var result struct{ Entity *Entity `json:"entity"` }
	err := e.c.do(ctx, "POST", "/v1/entities/"+id+"/suspend", map[string]string{"reason": reason}, &result)
	return result.Entity, err
}

func (e *EntityClient) Revoke(ctx context.Context, id, reason string) (*Entity, error) {
	var result struct{ Entity *Entity `json:"entity"` }
	err := e.c.do(ctx, "POST", "/v1/entities/"+id+"/revoke", map[string]string{"reason": reason}, &result)
	return result.Entity, err
}

// --- Authorization Client ---

type AuthzClient struct{ c *Client }

func (a *AuthzClient) Check(ctx context.Context, subject ObjectRef, permission string, resource ObjectRef) (*CheckResult, error) {
	var result CheckResult
	err := a.c.do(ctx, "POST", "/v1/authz/check", map[string]interface{}{
		"subject":    subject,
		"permission": permission,
		"resource":   resource,
	}, &result)
	return &result, err
}

func (a *AuthzClient) WriteRelationships(ctx context.Context, rels []map[string]interface{}) error {
	return a.c.do(ctx, "POST", "/v1/relationships", map[string]interface{}{"relationships": rels}, nil)
}

func (a *AuthzClient) ReadRelationships(ctx context.Context, resourceType, relation string) ([]map[string]interface{}, error) {
	params := url.Values{}
	if resourceType != "" {
		params.Set("resource_type", resourceType)
	}
	if relation != "" {
		params.Set("relation", relation)
	}
	qs := params.Encode()
	path := "/v1/relationships"
	if qs != "" {
		path += "?" + qs
	}
	var result struct{ Relationships []map[string]interface{} `json:"relationships"` }
	err := a.c.do(ctx, "GET", path, nil, &result)
	return result.Relationships, err
}

// --- Trust Client ---

type TrustClient struct{ c *Client }

func (t *TrustClient) GetScore(ctx context.Context, entityID string) (*TrustScore, error) {
	var result struct{ Score *TrustScore `json:"score"` }
	err := t.c.do(ctx, "GET", "/v1/trust/"+entityID, nil, &result)
	return result.Score, err
}

func (t *TrustClient) Verify(ctx context.Context, entityID string, minimumScore int) (bool, error) {
	var result struct{ Trusted bool `json:"trusted"` }
	err := t.c.do(ctx, "POST", "/v1/trust/verify", map[string]interface{}{
		"entity_id":     entityID,
		"minimum_score": minimumScore,
	}, &result)
	return result.Trusted, err
}

// Attest returns a privacy-preserving attestation that an entity meets a trust threshold.
func (t *TrustClient) Attest(ctx context.Context, entityID string, threshold int) (*TrustAttestation, error) {
	var result TrustAttestation
	err := t.c.do(ctx, "POST", "/v1/trust/attest", map[string]interface{}{
		"entity_id": entityID,
		"threshold": threshold,
	}, &result)
	return &result, err
}

// --- Fraud Client ---

type FraudClient struct{ c *Client }

func (f *FraudClient) ListIncidents(ctx context.Context, pageSize int) ([]FraudIncident, error) {
	var result struct{ Incidents []FraudIncident `json:"incidents"` }
	err := f.c.do(ctx, "GET", fmt.Sprintf("/v1/fraud/incidents?page_size=%d", pageSize), nil, &result)
	return result.Incidents, err
}

func (f *FraudClient) Simulate(ctx context.Context, attackType, targetEntityID string) (*FraudIncident, error) {
	var result struct{ Incident *FraudIncident `json:"incident"` }
	err := f.c.do(ctx, "POST", "/v1/fraud/simulate", map[string]interface{}{
		"attack_type":      attackType,
		"target_entity_id": targetEntityID,
	}, &result)
	return result.Incident, err
}

// --- Decision Client ---

type DecisionClient struct{ c *Client }

func (d *DecisionClient) CreateWorkflow(ctx context.Context, workflowType, subjectEntityID string, requiredApprovals, totalReviewers int) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := d.c.do(ctx, "POST", "/v1/decisions/workflows", map[string]interface{}{
		"workflow_type":      workflowType,
		"subject_entity_id":  subjectEntityID,
		"required_approvals": requiredApprovals,
		"total_reviewers":    totalReviewers,
		"blind_review":       true,
	}, &result)
	return result, err
}

func (d *DecisionClient) SubmitVote(ctx context.Context, workflowID, reviewerID string, vote int, reasoning string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := d.c.do(ctx, "POST", "/v1/decisions/votes", map[string]interface{}{
		"workflow_id": workflowID,
		"reviewer_id": reviewerID,
		"vote":        vote,
		"reasoning":   reasoning,
	}, &result)
	return result, err
}

// --- Federation Client ---

type FederationClient struct{ c *Client }

func (f *FederationClient) RegisterNode(ctx context.Context, name, jurisdiction, endpoint string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := f.c.do(ctx, "POST", "/v1/federation/nodes", map[string]interface{}{
		"name":         name,
		"jurisdiction": jurisdiction,
		"endpoint":     endpoint,
	}, &result)
	return result, err
}

func (f *FederationClient) ListNodes(ctx context.Context) ([]map[string]interface{}, error) {
	var result struct{ Nodes []map[string]interface{} `json:"nodes"` }
	err := f.c.do(ctx, "GET", "/v1/federation/nodes", nil, &result)
	return result.Nodes, err
}

func (f *FederationClient) CreateAuthorityOverride(ctx context.Context, workflowID, authorityEntityID string, tier AuthorityTier, justification, outcome string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := f.c.do(ctx, "POST", "/v1/authority/overrides", map[string]interface{}{
		"workflow_id":         workflowID,
		"authority_entity_id": authorityEntityID,
		"tier":                tier,
		"justification":       justification,
		"outcome":             outcome,
	}, &result)
	return result, err
}

// --- Credential Client ---

type CredentialClient struct{ c *Client }

func (cr *CredentialClient) Issue(ctx context.Context, entityID, credentialType string, claims map[string]interface{}) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := cr.c.do(ctx, "POST", "/v1/credentials", map[string]interface{}{
		"entity_id":       entityID,
		"credential_type": credentialType,
		"claims":          claims,
	}, &result)
	return result, err
}

func (cr *CredentialClient) Verify(ctx context.Context, credentialID string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := cr.c.do(ctx, "GET", "/v1/credentials/"+credentialID+"/verify", nil, &result)
	return result, err
}

func (cr *CredentialClient) List(ctx context.Context, entityID string) ([]map[string]interface{}, error) {
	var result struct{ Credentials []map[string]interface{} `json:"credentials"` }
	qs := ""
	if entityID != "" {
		qs = "?entity_id=" + entityID
	}
	err := cr.c.do(ctx, "GET", "/v1/credentials"+qs, nil, &result)
	return result.Credentials, err
}
