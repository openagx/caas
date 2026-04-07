package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	caasv1 "github.com/OpenAGX/caas/gen/go/caas/v1"
)

// MLClient communicates with the Python ML sidecar for trust scoring.
type MLClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewMLClient(baseURL string) *MLClient {
	return &MLClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// ScoreTrust calls the ML sidecar to get trust dimension scores.
func (c *MLClient) ScoreTrust(ctx context.Context, entityID string) (*caasv1.TrustDimensions, error) {
	body, _ := json.Marshal(map[string]string{"entity_id": entityID})

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/score", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ml sidecar unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ml sidecar returned %d", resp.StatusCode)
	}

	var result struct {
		IdentityVerification  int32 `json:"identity_verification"`
		BehavioralConsistency int32 `json:"behavioral_consistency"`
		NetworkReputation     int32 `json:"network_reputation"`
		TransactionHistory    int32 `json:"transaction_history"`
		ComplianceAdherence   int32 `json:"compliance_adherence"`
		TemporalStability     int32 `json:"temporal_stability"`
		PeerEndorsement       int32 `json:"peer_endorsement"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &caasv1.TrustDimensions{
		IdentityVerification:  result.IdentityVerification,
		BehavioralConsistency: result.BehavioralConsistency,
		NetworkReputation:     result.NetworkReputation,
		TransactionHistory:    result.TransactionHistory,
		ComplianceAdherence:   result.ComplianceAdherence,
		TemporalStability:     result.TemporalStability,
		PeerEndorsement:       result.PeerEndorsement,
	}, nil
}

// DetectSybil calls the ML sidecar to check for Sybil patterns.
func (c *MLClient) DetectSybil(ctx context.Context, entityID string) (float64, error) {
	body, _ := json.Marshal(map[string]string{"entity_id": entityID})

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/sybil-detect", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("ml sidecar unreachable: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		SybilProbability float64 `json:"sybil_probability"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	return result.SybilProbability, nil
}
