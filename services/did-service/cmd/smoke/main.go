// Smoke test for did-service + federation-service integration.
//
// Requires:
//   - postgres running on localhost:15432 (via infra/smoke/docker-compose.smoke.yml)
//   - did-service running on :50058
//   - federation-service running on :50056
//
// Usage:
//   go run ./cmd/smoke
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	caasv1 "github.com/OpenAGX/caas/gen/go/caas/v1"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func main() {
	dbURL := envOr("SMOKE_DATABASE_URL", "postgres://caas:caas_dev@localhost:15432/caas")
	didEndpoint := envOr("SMOKE_DID_ENDPOINT", "localhost:50058")
	federationEndpoint := envOr("SMOKE_FEDERATION_ENDPOINT", "localhost:50056")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// --- Setup: create a test entity directly via SQL ---
	log.Println("[1/7] Connecting to Postgres and creating test entity...")
	pg, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		fatal("postgres connect: %v", err)
	}
	defer pg.Close(ctx)

	var entityID string
	if err := pg.QueryRow(ctx,
		`INSERT INTO entities (entity_type, display_name, lifecycle_state)
		 VALUES ($1, $2, $3) RETURNING id`,
		1, "smoke-test-entity-"+time.Now().Format("150405"), "active",
	).Scan(&entityID); err != nil {
		fatal("insert entity: %v", err)
	}
	log.Printf("  entity_id: %s", entityID)
	defer func() {
		cleanupCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_, _ = pg.Exec(cleanupCtx,
			`DELETE FROM verifiable_credentials WHERE entity_id = $1`, entityID)
		_, _ = pg.Exec(cleanupCtx,
			`DELETE FROM verifiable_credentials_v2 WHERE entity_id = $1`, entityID)
		_, _ = pg.Exec(cleanupCtx,
			`DELETE FROM did_keys WHERE did IN (SELECT did FROM did_documents WHERE entity_id = $1)`, entityID)
		_, _ = pg.Exec(cleanupCtx,
			`DELETE FROM did_verification_methods WHERE did IN (SELECT did FROM did_documents WHERE entity_id = $1)`, entityID)
		_, _ = pg.Exec(cleanupCtx,
			`DELETE FROM did_documents WHERE entity_id = $1`, entityID)
		_, _ = pg.Exec(cleanupCtx, `DELETE FROM entities WHERE id = $1`, entityID)
		log.Println("[cleanup] test entity + credentials + DIDs removed")
	}()

	// --- Dial did-service ---
	log.Println("[2/7] Dialing did-service at", didEndpoint)
	didConn, err := grpc.NewClient(didEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fatal("did-service dial: %v", err)
	}
	defer didConn.Close()
	didClient := caasv1.NewDIDServiceClient(didConn)

	// --- CreateDID ---
	log.Println("[3/7] Calling did.CreateDID...")
	createResp, err := didClient.CreateDID(ctx, &caasv1.CreateDIDRequest{
		EntityId: entityID,
		Method:   caasv1.DIDMethod_DID_METHOD_KEY,
		KeyType:  caasv1.KeyType_KEY_TYPE_ED25519,
	})
	if err != nil {
		fatal("did.CreateDID: %v", err)
	}
	if createResp.Document == nil || createResp.Document.Id == "" {
		fatal("did.CreateDID returned empty document")
	}
	subjectDID := createResp.Document.Id
	log.Printf("  DID: %s", subjectDID)
	if !strings.HasPrefix(subjectDID, "did:key:z") {
		fatal("expected did:key:z prefix, got %q", subjectDID)
	}
	if createResp.Key == nil || createResp.Key.PublicKeyMultibase == "" {
		fatal("did.CreateDID returned no key pair")
	}
	log.Printf("  key_id: %s  public_key(multibase): %s", createResp.Key.KeyId, truncate(createResp.Key.PublicKeyMultibase, 20))

	// --- IssueVerifiableCredential via did-service directly ---
	log.Println("[4/7] Calling did.IssueVerifiableCredential directly...")
	claims, _ := structpb.NewStruct(map[string]any{
		"role":           "smoke-tester",
		"clearanceLevel": 3,
	})
	issueResp, err := didClient.IssueVerifiableCredential(ctx, &caasv1.DIDServiceIssueVerifiableCredentialRequest{
		IssuerDid:         subjectDID, // self-issued for this test
		SubjectDid:        subjectDID,
		EntityId:          entityID,
		CredentialType:    "SmokeTestCredential",
		CredentialSubject: claims,
		ExpirationDate:    timestamppb.New(time.Now().Add(24 * time.Hour)),
	})
	if err != nil {
		fatal("did.IssueVerifiableCredential: %v", err)
	}
	v2 := issueResp.Credential
	if v2 == nil || v2.Id == "" {
		fatal("did.IssueVerifiableCredential returned empty credential")
	}
	log.Printf("  v2 credential id: %s", v2.Id)

	// Parse proof JSON and verify it has a proofValue
	var proof map[string]any
	if err := json.Unmarshal([]byte(v2.ProofJson), &proof); err != nil {
		fatal("parse proof JSON: %v (proof=%q)", err, v2.ProofJson)
	}
	proofValue, ok := proof["proofValue"].(string)
	if !ok || proofValue == "" {
		fatal("proof has no proofValue: %v", proof)
	}
	// did-service encodes Ed25519 signatures as base64url (RawURLEncoding, no padding).
	// Ed25519 signatures are 64 bytes → base64url len = ceil(64/3)*4 - padding = 86 chars.
	if len(proofValue) != 86 {
		fatal("expected base64url-encoded Ed25519 signature (86 chars), got %d chars: %q", len(proofValue), proofValue)
	}
	log.Printf("  proof.proofValue: %s (len=%d, base64url)", truncate(proofValue, 20), len(proofValue))

	// --- VerifyCredential via did-service ---
	log.Println("[5/7] Calling did.VerifyCredential...")
	verifyResp, err := didClient.VerifyCredential(ctx, &caasv1.DIDServiceVerifyCredentialRequest{
		CredentialId: v2.Id,
	})
	if err != nil {
		fatal("did.VerifyCredential: %v", err)
	}
	if !verifyResp.Valid {
		fatal("did.VerifyCredential returned valid=false: %v", verifyResp.Checks)
	}
	log.Printf("  valid=true checks=%v", verifyResp.Checks)

	// --- federation.IssueCredential (tests the new did-service delegation) ---
	log.Println("[6/7] Dialing federation-service and calling IssueCredential...")
	fedConn, err := grpc.NewClient(federationEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fatal("federation dial: %v", err)
	}
	defer fedConn.Close()
	fedClient := caasv1.NewFederationServiceClient(fedConn)

	fedClaims, _ := structpb.NewStruct(map[string]any{
		"citizenship": "smoke-jurisdiction",
		"issuedBy":    "smoke-test",
	})
	fedIssueResp, err := fedClient.IssueCredential(ctx, &caasv1.IssueCredentialRequest{
		EntityId:       entityID,
		CredentialType: "CitizenshipCredential",
		Claims:         fedClaims,
		ExpiresAt:      timestamppb.New(time.Now().Add(24 * time.Hour)),
	})
	if err != nil {
		fatal("federation.IssueCredential: %v", err)
	}
	fedCred := fedIssueResp.Credential
	if fedCred == nil || fedCred.Id == "" {
		fatal("federation.IssueCredential returned empty credential")
	}
	if fedCred.ProofSignature == "" {
		fatal("federation credential has empty proof_signature (expected base64url proofValue)")
	}
	if len(fedCred.ProofSignature) != 86 {
		fatal("expected federation proof_signature to be 86-char base64url Ed25519 signature, got %d chars", len(fedCred.ProofSignature))
	}
	log.Printf("  federation credential id: %s", fedCred.Id)
	log.Printf("  issuer_did: %s (note: federation issuer, not subject)", fedCred.IssuerDid)
	log.Printf("  proof_signature: %s (len=%d)", truncate(fedCred.ProofSignature, 20), len(fedCred.ProofSignature))

	// Verify the federation row is linked to a V2 credential
	var v2Ref *string
	if err := pg.QueryRow(ctx,
		`SELECT v2_credential_id::text FROM verifiable_credentials WHERE id = $1`, fedCred.Id,
	).Scan(&v2Ref); err != nil {
		fatal("lookup v2_credential_id: %v", err)
	}
	if v2Ref == nil || *v2Ref == "" {
		fatal("federation credential row has NULL v2_credential_id — delegation did not persist the link")
	}
	log.Printf("  linked v2_credential_id: %s", *v2Ref)

	// --- federation.VerifyCredential delegates to did-service ---
	log.Println("[7/7] Calling federation.VerifyCredential (delegates to did-service)...")
	fedVerifyResp, err := fedClient.VerifyCredential(ctx, &caasv1.VerifyCredentialRequest{
		CredentialId: fedCred.Id,
	})
	if err != nil {
		fatal("federation.VerifyCredential: %v", err)
	}
	if !fedVerifyResp.Valid {
		fatal("federation.VerifyCredential returned valid=false: %s", fedVerifyResp.Reason)
	}
	log.Printf("  valid=true reason=%q", fedVerifyResp.Reason)

	log.Println()
	log.Println("✅ SMOKE TEST PASSED")
	log.Println("   - did:key created with Ed25519 keypair")
	log.Println("   - V2 VC issued with real Ed25519 proofValue (base64url)")
	log.Println("   - did-service verified its own signature")
	log.Println("   - federation-service delegated issue to did-service")
	log.Println("   - federation row linked to V2 credential via FK")
	log.Println("   - federation-service delegated verify to did-service")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "❌ SMOKE FAIL: "+format+"\n", args...)
	os.Exit(1)
}
