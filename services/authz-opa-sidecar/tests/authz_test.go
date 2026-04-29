package authz_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openagx/caas/services/authz-opa-sidecar/opa"
)

// AC-01: Spicedb allows + clean behavior = ALLOW
func TestAC01_SpiceDBAllow_CleanBehavior(t *testing.T) {
	input := authzInput{
		Spicedb: spicedbResult{Allowed: true},
		Subject: subjectInfo{TrustScore: 0.8, AuthStrength: "mfa"},
		Context: contextInfo{DeviceTrust: 0.9, IPReputation: 0.2},
		Behavior: behaviorInfo{AnomalyScore: 0.1, VelocityOps5m: 20, DriftScore: 0.1},
		Resource: resourceInfo{Sensitivity: "low"},
	}

	result := evaluatePolicy(input)
	
	if result.Allow != true {
		t.Errorf("AC-01 FAILED: expected allow=true, got %v", result.Allow)
	}
}

// AC-02: Spicedb denies = DENY regardless of behavior
func TestAC02_SpiceDBDeny(t *testing.T) {
	input := authzInput{
		Spicedb: spicedbResult{Allowed: false},
		Subject: subjectInfo{TrustScore: 0.8, AuthStrength: "mfa"},
		Context: contextInfo{DeviceTrust: 0.9, IPReputation: 0.2},
		Behavior: behaviorInfo{AnomalyScore: 0.1, VelocityOps5m: 20, DriftScore: 0.1},
		Resource: resourceInfo{Sensitivity: "low"},
	}

	result := evaluatePolicy(input)
	
	if result.Allow != false {
		t.Errorf("AC-02 FAILED: expected allow=false, got %v", result.Allow)
	}
}

// AC-03: High anomaly score = DENY (override)
func TestAC03_HighAnomaly_Deny(t *testing.T) {
	input := authzInput{
		Spicedb: spicedbResult{Allowed: true},
		Subject: subjectInfo{TrustScore: 0.8, AuthStrength: "mfa"},
		Context: contextInfo{DeviceTrust: 0.9, IPReputation: 0.2},
		Behavior: behaviorInfo{AnomalyScore: 0.95, VelocityOps5m: 20, DriftScore: 0.1},
		Resource: resourceInfo{Sensitivity: "low"},
	}

	result := evaluatePolicy(input)
	
	if result.Allow != false {
		t.Errorf("AC-03 FAILED: expected deny on high anomaly, got %v", result.Allow)
	}
	if !strings.Contains(result.Reason, "override_deny") {
		t.Errorf("AC-03 FAILED: expected override_deny reason")
	}
}

// AC-04: Low trust = DENY
func TestAC04_LowTrust_Deny(t *testing.T) {
	input := authzInput{
		Spicedb: spicedbResult{Allowed: true},
		Subject: subjectInfo{TrustScore: 0.2, AuthStrength: "mfa"},
		Context: contextInfo{DeviceTrust: 0.6, IPReputation: 0.3},
		Behavior: behaviorInfo{AnomalyScore: 0.1, VelocityOps5m: 20, DriftScore: 0.1},
		Resource: resourceInfo{Sensitivity: "low"},
	}

	result := evaluatePolicy(input)
	
	if result.Allow != false {
		t.Errorf("AC-04 FAILED: expected deny on low trust, got %v", result.Allow)
	}
}

// AC-05: MFA missing = DENY for high sensitivity
func TestAC05_NoMFA_HighSensitivity(t *testing.T) {
	input := authzInput{
		Spicedb: spicedbResult{Allowed: true},
		Subject: subjectInfo{TrustScore: 0.8, AuthStrength: "password"},
		Context: contextInfo{DeviceTrust: 0.6, IPReputation: 0.3},
		Behavior: behaviorInfo{AnomalyScore: 0.1, VelocityOps5m: 20, DriftScore: 0.1},
		Resource: resourceInfo{Sensitivity: "high"},
	}

	result := evaluatePolicy(input)
	
	if result.Allow != false {
		t.Errorf("AC-05 FAILED: expected deny without MFA on high sensitivity")
	}
}

// AC-06: Drift spike = DENY
func TestAC06_DriftSpike_Deny(t *testing.T) {
	input := authzInput{
		Spicedb: spicedbResult{Allowed: true},
		Subject: subjectInfo{TrustScore: 0.7, AuthStrength: "mfa"},
		Context: contextInfo{DeviceTrust: 0.9, IPReputation: 0.2},
		Behavior: behaviorInfo{AnomalyScore: 0.1, VelocityOps5m: 20, DriftScore: 0.8},
		Resource: resourceInfo{Sensitivity: "low"},
	}

	result := evaluatePolicy(input)
	
	if result.Allow != false {
		t.Errorf("AC-06 FAILED: expected deny on drift spike, got %v", result.Allow)
	}
}

// AC-07: Device trust low = DENY
func TestAC07_LowDeviceTrust_Deny(t *testing.T) {
	input := authzInput{
		Spicedb: spicedbResult{Allowed: true},
		Subject: subjectInfo{TrustScore: 0.8, AuthStrength: "mfa"},
		Context: contextInfo{DeviceTrust: 0.2, IPReputation: 0.3},
		Behavior: behaviorInfo{AnomalyScore: 0.1, VelocityOps5m: 20, DriftScore: 0.1},
		Resource: resourceInfo{Sensitivity: "low"},
	}

	result := evaluatePolicy(input)
	
	if result.Allow != false {
		t.Errorf("AC-07 FAILED: expected deny on low device trust")
	}
}

// AC-08: Velocity spike = DENY
func TestAC08_VelocitySpike_Deny(t *testing.T) {
	input := authzInput{
		Spicedb: spicedbResult{Allowed: true},
		Subject: subjectInfo{TrustScore: 0.8, AuthStrength: "mfa"},
		Context: contextInfo{DeviceTrust: 0.9, IPReputation: 0.2},
		Behavior: behaviorInfo{AnomalyScore: 0.1, VelocityOps5m: 200, DriftScore: 0.1},
		Resource: resourceInfo{Sensitivity: "low"},
	}

	result := evaluatePolicy(input)
	
	if result.Allow != false {
		t.Errorf("AC-08 FAILED: expected deny on velocity spike")
	}
}

// AC-09: Bad IP reputation = DENY
func TestAC09_BadIPReputation_Deny(t *testing.T) {
	input := authzInput{
		Spicedb: spicedbResult{Allowed: true},
		Subject: subjectInfo{TrustScore: 0.8, AuthStrength: "mfa"},
		Context: contextInfo{DeviceTrust: 0.9, IPReputation: 0.9},
		Behavior: behaviorInfo{AnomalyScore: 0.1, VelocityOps5m: 20, DriftScore: 0.1},
		Resource: resourceInfo{Sensitivity: "low"},
	}

	result := evaluatePolicy(input)
	
	if result.Allow != false {
		t.Errorf("AC-09 FAILED: expected deny on bad IP reputation")
	}
}

// AC-10: High sensitivity + elevated anomaly = DENY
func TestAC10_HighSensitivity_Anomaly(t *testing.T) {
	input := authzInput{
		Spicedb: spicedbResult{Allowed: true},
		Subject: subjectInfo{TrustScore: 0.8, AuthStrength: "mfa"},
		Context: contextInfo{DeviceTrust: 0.9, IPReputation: 0.2},
		Behavior: behaviorInfo{AnomalyScore: 0.6, VelocityOps5m: 20, DriftScore: 0.1},
		Resource: resourceInfo{Sensitivity: "high"},
	}

	result := evaluatePolicy(input)
	
	if result.Allow != false {
		t.Errorf("AC-10 FAILED: expected deny on high sensitivity + anomaly")
	}
}

// Helper structs
type authzInput struct {
	Spicedb   spicedbResult  `json:"spicedb"`
	Subject  subjectInfo   `json:"subject"`
	Context  contextInfo   `json:"context"`
	Behavior behaviorInfo `json:"behavior"`
	Resource resourceInfo `json:"resource"`
}

type spicedbResult struct {
	Allowed       bool     `json:"allowed"`
	Relationships []string `json:"relationships,omitempty"`
}

type subjectInfo struct {
	TrustScore   float64 `json:"trust_score"`
	AuthStrength string  `json:"auth_strength"`
}

type contextInfo struct {
	DeviceTrust   float64 `json:"device_trust"`
	IPReputation float64 `json:"ip_reputation"`
}

type behaviorInfo struct {
	AnomalyScore float64 `json:"anomaly_score"`
	VelocityOps5m int   `json:"velocity_ops_5m"`
	DriftScore   float64 `json:"drift_score"`
}

type resourceInfo struct {
	Sensitivity string `json:"sensitivity"`
}

type policyResult struct {
	Allow             bool     `json:"allow"`
	Reason            []string `json:"reason"`
	RiskScore         float64  `json:"risk_score"`
	RecommendedAction string   `json:"recommended_action"`
}

// evaluatePolicy simulates OPA evaluation
func evaluatePolicy(input authzInput) policyResult {
	allow := true
	var reason []string

	// Override checks
	if input.Behavior.AnomalyScore > 0.9 {
		allow = false
		reason = append(reason, "override_deny")
	}
	if input.Subject.TrustScore < 0.3 {
		allow = false
		reason = append(reason, "override_deny")
	}

	// SpiceDB check
	if !input.Spicedb.Allowed {
		allow = false
		reason = append(reason, "spicedb_failed")
	}

	// ABAC check
	if input.Subject.AuthStrength != "mfa" || input.Context.DeviceTrust < 0.5 || input.Context.IPReputation > 0.7 {
		if input.Resource.Sensitivity == "high" {
			allow = false
			reason = append(reason, "abac_failed")
		}
	}

	// Behavioral check
	if input.Behavior.AnomalyScore > 0.8 || input.Behavior.VelocityOps5m > 150 {
		allow = false
		reason = append(reason, "behavioral_failed")
	}

	// Drift check
	if input.Behavior.DriftScore > 0.65 || input.Subject.TrustScore < 0.6 {
		allow = false
		reason = append(reason, "drift_failed")
	}

	return policyResult{Allow: allow, Reason: reason}
}

// HTTP test server stub
func TestOPAEndpoint(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"allow":true,"reason":[],"risk_score":0.0,"recommended_action":"allow"}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/data/authz/decision")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}