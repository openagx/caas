package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"time"

	caasv1 "github.com/OpenAGX/caas/gen/go/caas/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type DIDServer struct {
	caasv1.UnimplementedDIDServiceServer
	store         *PostgresStore
	events        *EventProducer
	encryptionKey []byte
	domain        string
}

func NewDIDServer(store *PostgresStore, events *EventProducer, encryptionKey []byte, domain string) *DIDServer {
	return &DIDServer{
		store:         store,
		events:        events,
		encryptionKey: encryptionKey,
		domain:        domain,
	}
}

// --- DID Operations ---

func (s *DIDServer) CreateDID(ctx context.Context, req *caasv1.CreateDIDRequest) (*caasv1.CreateDIDResponse, error) {
	if req.Method == caasv1.DIDMethod_DID_METHOD_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "method is required")
	}
	// entity_id is optional — standalone DIDs (e.g., a sovereign's federation issuer DID)
	// do not need to be tied to an entity row. did:web requires entity_id only if domain is unset.
	if req.Method == caasv1.DIDMethod_DID_METHOD_WEB && req.EntityId == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required for did:web")
	}

	kt := req.KeyType
	if kt == caasv1.KeyType_KEY_TYPE_UNSPECIFIED {
		kt = caasv1.KeyType_KEY_TYPE_ED25519
	}

	keyTypeStr := keyTypeToString(kt)

	// Generate keypair
	var pubBytes []byte
	var privBytes []byte
	var pubJWK json.RawMessage

	switch kt {
	case caasv1.KeyType_KEY_TYPE_ED25519:
		pub, priv, err := GenerateEd25519KeyPair()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "generate key: %v", err)
		}
		pubBytes = []byte(pub)
		privBytes = []byte(priv)
		pubJWK = Ed25519PublicKeyToJWK(pub)
	case caasv1.KeyType_KEY_TYPE_P256:
		pub, priv, err := GenerateP256KeyPair()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "generate key: %v", err)
		}
		pubBytes = elliptic.MarshalCompressed(pub.Curve, pub.X, pub.Y)
		privBytes, _ = x509.MarshalECPrivateKey(priv)
		pubJWK = P256PublicKeyToJWK(pub)
	default:
		return nil, status.Error(codes.InvalidArgument, "unsupported key type")
	}

	pubMultibase := PublicKeyToMultibase(pubBytes, keyTypeStr)

	// Construct DID string
	domain := req.Domain
	if domain == "" {
		domain = s.domain
	}

	var did string
	switch req.Method {
	case caasv1.DIDMethod_DID_METHOD_WEB:
		did = ConstructDIDWeb(domain, req.EntityId)
	case caasv1.DIDMethod_DID_METHOD_KEY:
		did = ConstructDIDKey(pubMultibase)
	}

	keyID := "key-1"

	// Build W3C DID Document
	document := BuildDIDDocument(did, req.EntityId, domain, keyID, keyTypeStr, pubMultibase)

	// Encrypt private key
	encPriv, nonce, err := EncryptPrivateKey(privBytes, s.encryptionKey)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "encrypt key: %v", err)
	}

	// Store DID Document
	doc, err := s.store.CreateDIDDocument(ctx, did, req.EntityId, didMethodToString(req.Method), document)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create did: %v", err)
	}

	// Store key
	fullKeyID := fmt.Sprintf("%s#%s", did, keyID)
	if err := s.store.StoreKey(ctx, fullKeyID, did, keyTypeStr, "assertion", pubMultibase, pubJWK, encPriv, nonce); err != nil {
		return nil, status.Errorf(codes.Internal, "store key: %v", err)
	}

	// Update entity's DID column (only if DID is tied to an entity)
	if req.EntityId != "" {
		if err := s.store.UpdateEntityDID(ctx, req.EntityId, did); err != nil {
			log.Printf("WARN: failed to update entity DID: %v", err)
		}
	}

	s.events.Emit("did.created", map[string]any{
		"did":       did,
		"entity_id": req.EntityId,
		"method":    didMethodToString(req.Method),
		"key_type":  keyTypeStr,
	})

	return &caasv1.CreateDIDResponse{
		Document: doc,
		Key: &caasv1.KeyPairMsg{
			KeyId:             fullKeyID,
			Did:               did,
			KeyType:           kt,
			Purpose:           caasv1.KeyPurpose_KEY_PURPOSE_ASSERTION,
			PublicKeyMultibase: pubMultibase,
			PublicKeyJwk:      string(pubJWK),
			CreatedAt:         timestamppb.Now(),
		},
	}, nil
}

func (s *DIDServer) ResolveDID(ctx context.Context, req *caasv1.ResolveDIDRequest) (*caasv1.ResolveDIDResponse, error) {
	if req.Did == "" {
		return nil, status.Error(codes.InvalidArgument, "did is required")
	}

	doc, err := s.store.GetDIDDocument(ctx, req.Did)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "did not found: %v", err)
	}

	if doc.Status == caasv1.DIDStatus_DID_STATUS_DEACTIVATED {
		return nil, status.Error(codes.FailedPrecondition, "DID is deactivated")
	}

	return &caasv1.ResolveDIDResponse{
		Document:     doc,
		DocumentJson: doc.DocumentJson,
	}, nil
}

func (s *DIDServer) UpdateDIDDocument(ctx context.Context, req *caasv1.UpdateDIDDocumentRequest) (*caasv1.UpdateDIDDocumentResponse, error) {
	if req.Did == "" {
		return nil, status.Error(codes.InvalidArgument, "did is required")
	}

	doc, err := s.store.GetDIDDocument(ctx, req.Did)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "did not found: %v", err)
	}

	// Parse existing document
	var document map[string]any
	if err := json.Unmarshal([]byte(doc.DocumentJson), &document); err != nil {
		return nil, status.Errorf(codes.Internal, "parse document: %v", err)
	}

	// Add verification methods
	for _, vm := range req.AddVerificationMethods {
		vmMap := BuildVerificationMethod(req.Did, vm.Id, vm.Type, vm.PublicKeyMultibase)
		AddVerificationMethodToDoc(document, vmMap, []string{"authentication", "assertionMethod"})
	}

	// Remove verification methods
	for _, vmID := range req.RemoveVerificationMethodIds {
		RemoveVerificationMethodFromDoc(document, vmID)
	}

	// Add services
	for _, svc := range req.AddServices {
		svcMap := map[string]any{
			"id":              svc.Id,
			"type":            svc.Type,
			"serviceEndpoint": svc.ServiceEndpoint,
		}
		AddServiceToDoc(document, svcMap)
	}

	// Remove services
	for _, svcID := range req.RemoveServiceIds {
		RemoveServiceFromDoc(document, svcID)
	}

	// Update controllers
	if len(req.AddControllers) > 0 || len(req.RemoveControllers) > 0 {
		controllers, _ := document["controller"].([]string)
		if controllers == nil {
			if c, ok := document["controller"].([]any); ok {
				for _, v := range c {
					if s, ok := v.(string); ok {
						controllers = append(controllers, s)
					}
				}
			}
		}
		for _, c := range req.AddControllers {
			controllers = append(controllers, c)
		}
		removeSet := make(map[string]bool)
		for _, c := range req.RemoveControllers {
			removeSet[c] = true
		}
		var filtered []string
		for _, c := range controllers {
			if !removeSet[c] {
				filtered = append(filtered, c)
			}
		}
		document["controller"] = filtered
	}

	if err := s.store.UpdateDIDDocumentJSON(ctx, req.Did, document); err != nil {
		return nil, status.Errorf(codes.Internal, "update document: %v", err)
	}

	s.events.Emit("did.updated", map[string]any{"did": req.Did})

	updated, _ := s.store.GetDIDDocument(ctx, req.Did)
	return &caasv1.UpdateDIDDocumentResponse{Document: updated}, nil
}

func (s *DIDServer) DeactivateDID(ctx context.Context, req *caasv1.DeactivateDIDRequest) (*caasv1.DeactivateDIDResponse, error) {
	if req.Did == "" {
		return nil, status.Error(codes.InvalidArgument, "did is required")
	}

	if err := s.store.DeactivateDID(ctx, req.Did); err != nil {
		return nil, status.Errorf(codes.Internal, "deactivate: %v", err)
	}

	s.events.Emit("did.deactivated", map[string]any{
		"did":    req.Did,
		"reason": req.Reason,
	})

	doc, _ := s.store.GetDIDDocument(ctx, req.Did)
	return &caasv1.DeactivateDIDResponse{Document: doc}, nil
}

func (s *DIDServer) ListDIDs(ctx context.Context, req *caasv1.ListDIDsRequest) (*caasv1.ListDIDsResponse, error) {
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 20
	}
	page := int(req.Page)
	if page <= 0 {
		page = 1
	}

	docs, total, err := s.store.ListDIDDocuments(ctx,
		req.EntityId,
		didMethodToString(req.MethodFilter),
		didStatusToString(req.StatusFilter),
		pageSize, (page-1)*pageSize,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list dids: %v", err)
	}

	return &caasv1.ListDIDsResponse{Documents: docs, Total: int32(total)}, nil
}

// --- Key Management ---

func (s *DIDServer) GenerateKeyPair(ctx context.Context, req *caasv1.GenerateKeyPairRequest) (*caasv1.GenerateKeyPairResponse, error) {
	if req.Did == "" {
		return nil, status.Error(codes.InvalidArgument, "did is required")
	}

	kt := req.KeyType
	if kt == caasv1.KeyType_KEY_TYPE_UNSPECIFIED {
		kt = caasv1.KeyType_KEY_TYPE_ED25519
	}
	purpose := req.Purpose
	if purpose == caasv1.KeyPurpose_KEY_PURPOSE_UNSPECIFIED {
		purpose = caasv1.KeyPurpose_KEY_PURPOSE_ASSERTION
	}

	keyTypeStr := keyTypeToString(kt)
	purposeStr := keyPurposeToString(purpose)

	var pubBytes, privBytes []byte
	var pubJWK json.RawMessage

	switch kt {
	case caasv1.KeyType_KEY_TYPE_ED25519:
		pub, priv, err := GenerateEd25519KeyPair()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "generate key: %v", err)
		}
		pubBytes = []byte(pub)
		privBytes = []byte(priv)
		pubJWK = Ed25519PublicKeyToJWK(pub)
	case caasv1.KeyType_KEY_TYPE_P256:
		pub, priv, err := GenerateP256KeyPair()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "generate key: %v", err)
		}
		pubBytes = elliptic.MarshalCompressed(pub.Curve, pub.X, pub.Y)
		privBytes, _ = x509.MarshalECPrivateKey(priv)
		pubJWK = P256PublicKeyToJWK(pub)
	}

	pubMultibase := PublicKeyToMultibase(pubBytes, keyTypeStr)

	// Count existing keys to determine key number
	existingKeys, _ := s.store.ListKeys(ctx, req.Did)
	keyNum := len(existingKeys) + 1
	keyID := fmt.Sprintf("%s#key-%d", req.Did, keyNum)

	encPriv, nonce, err := EncryptPrivateKey(privBytes, s.encryptionKey)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "encrypt key: %v", err)
	}

	if err := s.store.StoreKey(ctx, keyID, req.Did, keyTypeStr, purposeStr, pubMultibase, pubJWK, encPriv, nonce); err != nil {
		return nil, status.Errorf(codes.Internal, "store key: %v", err)
	}

	s.events.Emit("key.generated", map[string]any{
		"key_id":   keyID,
		"did":      req.Did,
		"key_type": keyTypeStr,
		"purpose":  purposeStr,
	})

	return &caasv1.GenerateKeyPairResponse{
		Key: &caasv1.KeyPairMsg{
			KeyId:             keyID,
			Did:               req.Did,
			KeyType:           kt,
			Purpose:           purpose,
			PublicKeyMultibase: pubMultibase,
			PublicKeyJwk:      string(pubJWK),
			CreatedAt:         timestamppb.Now(),
		},
	}, nil
}

func (s *DIDServer) RotateKey(ctx context.Context, req *caasv1.RotateKeyRequest) (*caasv1.RotateKeyResponse, error) {
	if req.KeyId == "" {
		return nil, status.Error(codes.InvalidArgument, "key_id is required")
	}

	// Get old key info
	oldKeyType, oldPubMB, _, _, err := s.store.GetKey(ctx, req.KeyId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "key not found: %v", err)
	}

	// Mark old key as rotated
	if err := s.store.RotateKey(ctx, req.KeyId); err != nil {
		return nil, status.Errorf(codes.Internal, "rotate key: %v", err)
	}

	oldKey := &caasv1.KeyPairMsg{
		KeyId:             req.KeyId,
		KeyType:           keyTypeFromString(oldKeyType),
		PublicKeyMultibase: oldPubMB,
		Rotated:           true,
	}

	s.events.Emit("key.rotated", map[string]any{"key_id": req.KeyId})

	return &caasv1.RotateKeyResponse{OldKey: oldKey}, nil
}

func (s *DIDServer) ListKeys(ctx context.Context, req *caasv1.ListKeysRequest) (*caasv1.ListKeysResponse, error) {
	if req.Did == "" {
		return nil, status.Error(codes.InvalidArgument, "did is required")
	}

	keys, err := s.store.ListKeys(ctx, req.Did)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list keys: %v", err)
	}

	return &caasv1.ListKeysResponse{Keys: keys}, nil
}

func (s *DIDServer) ExportPublicKey(ctx context.Context, req *caasv1.ExportPublicKeyRequest) (*caasv1.ExportPublicKeyResponse, error) {
	if req.KeyId == "" {
		return nil, status.Error(codes.InvalidArgument, "key_id is required")
	}

	keyType, pubMultibase, _, _, err := s.store.GetKey(ctx, req.KeyId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "key not found: %v", err)
	}

	format := req.Format
	if format == "" {
		format = "multibase"
	}

	switch format {
	case "multibase":
		return &caasv1.ExportPublicKeyResponse{PublicKey: pubMultibase, Format: "multibase"}, nil
	case "jwk":
		pubRaw, _, err := MultibaseToPublicKey(pubMultibase)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "decode key: %v", err)
		}
		var jwk json.RawMessage
		switch keyType {
		case "Ed25519":
			jwk = Ed25519PublicKeyToJWK(ed25519.PublicKey(pubRaw))
		case "P-256":
			x, y := elliptic.UnmarshalCompressed(elliptic.P256(), pubRaw)
			if x == nil {
				return nil, status.Error(codes.Internal, "invalid P-256 key")
			}
			jwk = P256PublicKeyToJWK(&ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y})
		}
		return &caasv1.ExportPublicKeyResponse{PublicKey: string(jwk), Format: "jwk"}, nil
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported format: %s", format)
	}
}

// --- Verifiable Credentials ---

func (s *DIDServer) IssueVerifiableCredential(ctx context.Context, req *caasv1.DIDServiceIssueVerifiableCredentialRequest) (*caasv1.DIDServiceIssueVerifiableCredentialResponse, error) {
	if req.IssuerDid == "" || req.CredentialType == "" {
		return nil, status.Error(codes.InvalidArgument, "issuer_did and credential_type are required")
	}

	// Resolve issuer DID
	issuerDoc, err := s.store.GetDIDDocument(ctx, req.IssuerDid)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "issuer DID not found: %v", err)
	}
	if issuerDoc.Status == caasv1.DIDStatus_DID_STATUS_DEACTIVATED {
		return nil, status.Error(codes.FailedPrecondition, "issuer DID is deactivated")
	}

	// Get signing key
	var signingKeyID, signingKeyType string
	var encPrivKey, keyNonce []byte

	if req.SigningKeyId != "" {
		signingKeyType, _, encPrivKey, keyNonce, err = s.store.GetKey(ctx, req.SigningKeyId)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "signing key not found: %v", err)
		}
		signingKeyID = req.SigningKeyId
	} else {
		signingKeyID, signingKeyType, _, encPrivKey, keyNonce, err = s.store.GetDefaultAssertionKey(ctx, req.IssuerDid)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "no assertion key for issuer: %v", err)
		}
	}

	// Decrypt private key
	privKeyBytes, err := DecryptPrivateKey(encPrivKey, keyNonce, s.encryptionKey)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decrypt key: %v", err)
	}

	// Build credential subject — inject subject DID as `id` if provided.
	// The exact same map is both signed and stored so verification is deterministic.
	credSubjectMap := map[string]any{}
	if req.CredentialSubject != nil {
		credSubjectMap = req.CredentialSubject.AsMap()
	}
	if req.SubjectDid != "" {
		credSubjectMap["id"] = req.SubjectDid
	}
	credSubjectJSON, _ := json.Marshal(credSubjectMap)

	// Build the credential document for signing
	now := time.Now().UTC()
	vcDoc := map[string]any{
		"@context":          []string{"https://www.w3.org/2018/credentials/v1"},
		"type":              []string{"VerifiableCredential", req.CredentialType},
		"issuer":            req.IssuerDid,
		"issuanceDate":      now.Format(time.RFC3339),
		"credentialSubject": credSubjectMap,
	}
	if req.ExpirationDate != nil {
		vcDoc["expirationDate"] = req.ExpirationDate.AsTime().Format(time.RFC3339)
	}

	// Canonicalize and sign
	canonical, err := CanonicalizeJSON(vcDoc)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "canonicalize: %v", err)
	}

	var signatureBytes []byte
	proofType := "Ed25519Signature2020"

	switch signingKeyType {
	case "Ed25519":
		privKey := ed25519.PrivateKey(privKeyBytes)
		signatureBytes = SignEd25519(privKey, canonical)
	case "P-256":
		privKey, parseErr := x509.ParseECPrivateKey(privKeyBytes)
		if parseErr != nil {
			return nil, status.Errorf(codes.Internal, "parse p256 key: %v", parseErr)
		}
		signatureBytes, err = SignP256(privKey, canonical)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "sign: %v", err)
		}
		proofType = "EcdsaSecp256r1Signature2019"
	}

	proof := map[string]any{
		"type":               proofType,
		"created":            now.Format(time.RFC3339),
		"verificationMethod": signingKeyID,
		"proofPurpose":       "assertionMethod",
		"proofValue":         base64.RawURLEncoding.EncodeToString(signatureBytes),
	}
	proofJSON, _ := json.Marshal(proof)

	contextJSON, _ := json.Marshal([]string{"https://www.w3.org/2018/credentials/v1"})
	typeJSON, _ := json.Marshal([]string{"VerifiableCredential", req.CredentialType})

	var expDate *time.Time
	if req.ExpirationDate != nil {
		t := req.ExpirationDate.AsTime()
		expDate = &t
	}

	vc, err := s.store.CreateCredentialV2(ctx, contextJSON, typeJSON, req.IssuerDid, req.SubjectDid, req.EntityId, credSubjectJSON, proofJSON, now, expDate)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "store credential: %v", err)
	}

	s.events.Emit("vc.issued", map[string]any{
		"credential_id":   vc.Id,
		"issuer_did":      req.IssuerDid,
		"credential_type": req.CredentialType,
	})

	return &caasv1.DIDServiceIssueVerifiableCredentialResponse{Credential: vc}, nil
}

func (s *DIDServer) VerifyCredential(ctx context.Context, req *caasv1.DIDServiceVerifyCredentialRequest) (*caasv1.DIDServiceVerifyCredentialResponse, error) {
	var checks []string

	var vc *caasv1.VerifiableCredentialV2
	var credSubjectJSON []byte
	var err error

	if req.CredentialId != "" {
		vc, credSubjectJSON, err = s.store.GetCredentialV2WithSubject(ctx, req.CredentialId)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "credential not found: %v", err)
		}
	} else {
		return nil, status.Error(codes.InvalidArgument, "credential_id is required")
	}

	// Check 1: Status
	if vc.Status == caasv1.CredentialStatus_CREDENTIAL_STATUS_REVOKED {
		checks = append(checks, "FAIL: credential is revoked")
		return &caasv1.DIDServiceVerifyCredentialResponse{Valid: false, Checks: checks, Credential: vc}, nil
	}
	checks = append(checks, "PASS: credential not revoked")

	// Check 2: Expiration
	if vc.ExpirationDate != nil && vc.ExpirationDate.AsTime().Before(time.Now()) {
		checks = append(checks, "FAIL: credential expired")
		return &caasv1.DIDServiceVerifyCredentialResponse{Valid: false, Checks: checks, Credential: vc}, nil
	}
	checks = append(checks, "PASS: credential not expired")

	// Check 3: Issuer DID is active
	issuerDoc, err := s.store.GetDIDDocument(ctx, vc.IssuerDid)
	if err != nil {
		checks = append(checks, fmt.Sprintf("FAIL: issuer DID not found: %v", err))
		return &caasv1.DIDServiceVerifyCredentialResponse{Valid: false, Checks: checks, Credential: vc}, nil
	}
	if issuerDoc.Status == caasv1.DIDStatus_DID_STATUS_DEACTIVATED {
		checks = append(checks, "FAIL: issuer DID is deactivated")
		return &caasv1.DIDServiceVerifyCredentialResponse{Valid: false, Checks: checks, Credential: vc}, nil
	}
	checks = append(checks, "PASS: issuer DID is active")

	// Check 4: Cryptographic signature verification
	var proof map[string]any
	if err := json.Unmarshal([]byte(vc.ProofJson), &proof); err != nil {
		checks = append(checks, "FAIL: invalid proof JSON")
		return &caasv1.DIDServiceVerifyCredentialResponse{Valid: false, Checks: checks, Credential: vc}, nil
	}

	vmID, _ := proof["verificationMethod"].(string)
	proofValue, _ := proof["proofValue"].(string)

	if vmID == "" || proofValue == "" {
		checks = append(checks, "FAIL: proof missing verificationMethod or proofValue")
		return &caasv1.DIDServiceVerifyCredentialResponse{Valid: false, Checks: checks, Credential: vc}, nil
	}

	// Get the signing key's public key
	keyType, pubMultibase, _, _, keyErr := s.store.GetKey(ctx, vmID)
	if keyErr != nil {
		checks = append(checks, fmt.Sprintf("FAIL: signing key not found: %v", keyErr))
		return &caasv1.DIDServiceVerifyCredentialResponse{Valid: false, Checks: checks, Credential: vc}, nil
	}

	pubRaw, _, mbErr := MultibaseToPublicKey(pubMultibase)
	if mbErr != nil {
		checks = append(checks, fmt.Sprintf("FAIL: decode public key: %v", mbErr))
		return &caasv1.DIDServiceVerifyCredentialResponse{Valid: false, Checks: checks, Credential: vc}, nil
	}

	sigBytes, decErr := base64.RawURLEncoding.DecodeString(proofValue)
	if decErr != nil {
		checks = append(checks, "FAIL: decode signature")
		return &caasv1.DIDServiceVerifyCredentialResponse{Valid: false, Checks: checks, Credential: vc}, nil
	}

	// Reconstruct the signed document (without proof) using the exact
	// credential_subject bytes that were stored at sign time.
	var ctxArr, typeArr []string
	json.Unmarshal([]byte(vc.ContextJson), &ctxArr)
	json.Unmarshal([]byte(vc.TypeJson), &typeArr)

	var credSubjectMap map[string]any
	if len(credSubjectJSON) > 0 {
		_ = json.Unmarshal(credSubjectJSON, &credSubjectMap)
	}
	if credSubjectMap == nil {
		credSubjectMap = map[string]any{}
	}

	vcDoc := map[string]any{
		"@context":          ctxArr,
		"type":              typeArr,
		"issuer":            vc.IssuerDid,
		"issuanceDate":      vc.IssuanceDate.AsTime().Format(time.RFC3339),
		"credentialSubject": credSubjectMap,
	}
	if vc.ExpirationDate != nil {
		vcDoc["expirationDate"] = vc.ExpirationDate.AsTime().Format(time.RFC3339)
	}

	canonical, _ := CanonicalizeJSON(vcDoc)

	var sigValid bool
	switch keyType {
	case "Ed25519":
		sigValid = VerifyEd25519Signature(ed25519.PublicKey(pubRaw), canonical, sigBytes)
	case "P-256":
		x, y := elliptic.UnmarshalCompressed(elliptic.P256(), pubRaw)
		if x != nil {
			pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
			sigValid = VerifyP256Signature(pub, canonical, sigBytes)
		}
	}

	if !sigValid {
		checks = append(checks, "FAIL: signature verification failed")
		return &caasv1.DIDServiceVerifyCredentialResponse{Valid: false, Checks: checks, Credential: vc}, nil
	}
	checks = append(checks, "PASS: signature verified")

	return &caasv1.DIDServiceVerifyCredentialResponse{Valid: true, Checks: checks, Credential: vc}, nil
}

func (s *DIDServer) RevokeCredential(ctx context.Context, req *caasv1.DIDServiceRevokeCredentialRequest) (*caasv1.DIDServiceRevokeCredentialResponse, error) {
	if req.CredentialId == "" {
		return nil, status.Error(codes.InvalidArgument, "credential_id is required")
	}

	if err := s.store.RevokeCredentialV2(ctx, req.CredentialId, req.Reason); err != nil {
		return nil, status.Errorf(codes.Internal, "revoke: %v", err)
	}

	s.events.Emit("vc.revoked", map[string]any{
		"credential_id": req.CredentialId,
		"reason":        req.Reason,
	})

	vc, _ := s.store.GetCredentialV2(ctx, req.CredentialId)
	return &caasv1.DIDServiceRevokeCredentialResponse{Credential: vc}, nil
}

func (s *DIDServer) GetCredential(ctx context.Context, req *caasv1.DIDServiceGetCredentialRequest) (*caasv1.DIDServiceGetCredentialResponse, error) {
	if req.CredentialId == "" {
		return nil, status.Error(codes.InvalidArgument, "credential_id is required")
	}

	vc, err := s.store.GetCredentialV2(ctx, req.CredentialId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "credential not found: %v", err)
	}

	return &caasv1.DIDServiceGetCredentialResponse{Credential: vc}, nil
}

func (s *DIDServer) ListCredentials(ctx context.Context, req *caasv1.DIDServiceListCredentialsRequest) (*caasv1.DIDServiceListCredentialsResponse, error) {
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 20
	}
	page := int(req.Page)
	if page <= 0 {
		page = 1
	}

	creds, total, err := s.store.ListCredentialsV2(ctx,
		req.EntityId, req.IssuerDid,
		credentialStatusToString(req.StatusFilter),
		pageSize, (page-1)*pageSize,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list credentials: %v", err)
	}

	return &caasv1.DIDServiceListCredentialsResponse{Credentials: creds, Total: int32(total)}, nil
}

// --- Verifiable Presentations ---

func (s *DIDServer) CreatePresentation(ctx context.Context, req *caasv1.DIDServiceCreatePresentationRequest) (*caasv1.DIDServiceCreatePresentationResponse, error) {
	if req.HolderDid == "" || len(req.CredentialIds) == 0 {
		return nil, status.Error(codes.InvalidArgument, "holder_did and credential_ids are required")
	}

	// Resolve holder DID
	holderDoc, err := s.store.GetDIDDocument(ctx, req.HolderDid)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "holder DID not found: %v", err)
	}
	if holderDoc.Status == caasv1.DIDStatus_DID_STATUS_DEACTIVATED {
		return nil, status.Error(codes.FailedPrecondition, "holder DID is deactivated")
	}

	// Get signing key
	var signingKeyID, signingKeyType string
	var encPrivKey, keyNonce []byte

	if req.SigningKeyId != "" {
		signingKeyType, _, encPrivKey, keyNonce, err = s.store.GetKey(ctx, req.SigningKeyId)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "signing key not found: %v", err)
		}
		signingKeyID = req.SigningKeyId
	} else {
		signingKeyID, signingKeyType, _, encPrivKey, keyNonce, err = s.store.GetDefaultAssertionKey(ctx, req.HolderDid)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "no assertion key for holder: %v", err)
		}
	}

	privKeyBytes, err := DecryptPrivateKey(encPrivKey, keyNonce, s.encryptionKey)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "decrypt key: %v", err)
	}

	// Build presentation document
	now := time.Now().UTC()
	vpDoc := map[string]any{
		"@context":              []string{"https://www.w3.org/2018/credentials/v1"},
		"type":                  []string{"VerifiablePresentation"},
		"holder":                req.HolderDid,
		"verifiableCredential":  req.CredentialIds,
	}

	canonical, _ := CanonicalizeJSON(vpDoc)

	var sigBytes []byte
	proofType := "Ed25519Signature2020"

	switch signingKeyType {
	case "Ed25519":
		privKey := ed25519.PrivateKey(privKeyBytes)
		sigBytes = SignEd25519(privKey, canonical)
	case "P-256":
		privKey, _ := x509.ParseECPrivateKey(privKeyBytes)
		sigBytes, _ = SignP256(privKey, canonical)
		proofType = "EcdsaSecp256r1Signature2019"
	}

	proof := map[string]any{
		"type":               proofType,
		"created":            now.Format(time.RFC3339),
		"verificationMethod": signingKeyID,
		"proofPurpose":       "authentication",
		"proofValue":         base64.RawURLEncoding.EncodeToString(sigBytes),
	}
	proofJSON, _ := json.Marshal(proof)

	vp, err := s.store.CreatePresentation(ctx, req.HolderDid, req.CredentialIds, proofJSON)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "store presentation: %v", err)
	}

	s.events.Emit("vp.created", map[string]any{
		"presentation_id": vp.Id,
		"holder_did":      req.HolderDid,
		"credential_count": len(req.CredentialIds),
	})

	return &caasv1.DIDServiceCreatePresentationResponse{Presentation: vp}, nil
}

func (s *DIDServer) VerifyPresentation(ctx context.Context, req *caasv1.DIDServiceVerifyPresentationRequest) (*caasv1.DIDServiceVerifyPresentationResponse, error) {
	if req.PresentationJson == "" {
		return nil, status.Error(codes.InvalidArgument, "presentation_json is required")
	}

	var vpDoc map[string]any
	if err := json.Unmarshal([]byte(req.PresentationJson), &vpDoc); err != nil {
		return &caasv1.DIDServiceVerifyPresentationResponse{Valid: false, Checks: []string{"FAIL: invalid JSON"}}, nil
	}

	var checks []string

	// Extract proof
	proofRaw, ok := vpDoc["proof"]
	if !ok {
		return &caasv1.DIDServiceVerifyPresentationResponse{Valid: false, Checks: []string{"FAIL: no proof in presentation"}}, nil
	}
	proofMap, _ := proofRaw.(map[string]any)
	vmID, _ := proofMap["verificationMethod"].(string)
	proofValue, _ := proofMap["proofValue"].(string)

	if vmID == "" || proofValue == "" {
		return &caasv1.DIDServiceVerifyPresentationResponse{Valid: false, Checks: []string{"FAIL: incomplete proof"}}, nil
	}

	// Get signing key
	keyType, pubMultibase, _, _, err := s.store.GetKey(ctx, vmID)
	if err != nil {
		checks = append(checks, fmt.Sprintf("FAIL: key not found: %v", err))
		return &caasv1.DIDServiceVerifyPresentationResponse{Valid: false, Checks: checks}, nil
	}
	checks = append(checks, "PASS: signing key found")

	pubRaw, _, _ := MultibaseToPublicKey(pubMultibase)
	sigBytes, _ := base64.RawURLEncoding.DecodeString(proofValue)

	// Remove proof from document, canonicalize, verify
	delete(vpDoc, "proof")
	canonical, _ := CanonicalizeJSON(vpDoc)

	var valid bool
	switch keyType {
	case "Ed25519":
		valid = VerifyEd25519Signature(ed25519.PublicKey(pubRaw), canonical, sigBytes)
	case "P-256":
		x, y := elliptic.UnmarshalCompressed(elliptic.P256(), pubRaw)
		if x != nil {
			pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
			valid = VerifyP256Signature(pub, canonical, sigBytes)
		}
	}

	if !valid {
		checks = append(checks, "FAIL: signature verification failed")
		return &caasv1.DIDServiceVerifyPresentationResponse{Valid: false, Checks: checks}, nil
	}
	checks = append(checks, "PASS: presentation signature verified")

	return &caasv1.DIDServiceVerifyPresentationResponse{Valid: true, Checks: checks}, nil
}

// --- Helpers ---

func intToAny(v any) []any {
	if arr, ok := v.([]any); ok {
		return arr
	}
	return nil
}

// ensureBigIntBytes pads big.Int bytes to the expected length.
func ensureBigIntBytes(b []byte, length int) []byte {
	if len(b) >= length {
		return b[:length]
	}
	padded := make([]byte, length)
	copy(padded[length-len(b):], b)
	return padded
}

// p256UnmarshalPublicKey reconstructs a P-256 public key from raw bytes.
func p256UnmarshalPublicKey(data []byte) *ecdsa.PublicKey {
	x, y := elliptic.UnmarshalCompressed(elliptic.P256(), data)
	if x == nil {
		// Try uncompressed
		x, y = elliptic.Unmarshal(elliptic.P256(), data)
	}
	if x == nil {
		return nil
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
}

// Ensure big.Int is imported and used
var _ = new(big.Int)
