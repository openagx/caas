package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	caasv1 "github.com/OpenAGX/caas/gen/go/caas/v1"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, connString string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

// --- DID Document Operations ---

func (s *PostgresStore) CreateDIDDocument(ctx context.Context, did, entityID, method string, document map[string]any) (*caasv1.DIDDocument, error) {
	docJSON, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("marshal document: %w", err)
	}

	var id string
	var createdAt, updatedAt time.Time
	err = s.pool.QueryRow(ctx,
		`INSERT INTO did_documents (did, entity_id, method, status, document)
		 VALUES ($1, $2, $3, 'active', $4)
		 RETURNING id, created_at, updated_at`,
		did, nilIfEmpty(entityID), method, docJSON,
	).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert did_document: %w", err)
	}

	return &caasv1.DIDDocument{
		Id:           did,
		Controller:   did,
		Status:       caasv1.DIDStatus_DID_STATUS_ACTIVE,
		DocumentJson: string(docJSON),
		EntityId:     entityID,
		Method:       didMethodFromString(method),
		CreatedAt:    timestamppb.New(createdAt),
		UpdatedAt:    timestamppb.New(updatedAt),
	}, nil
}

func (s *PostgresStore) GetDIDDocument(ctx context.Context, did string) (*caasv1.DIDDocument, error) {
	var entityID *string
	var method, status, docJSON string
	var createdAt, updatedAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT entity_id, method, status, document, created_at, updated_at
		 FROM did_documents WHERE did = $1`, did,
	).Scan(&entityID, &method, &status, &docJSON, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("get did_document: %w", err)
	}

	eid := ""
	if entityID != nil {
		eid = *entityID
	}

	return &caasv1.DIDDocument{
		Id:           did,
		Controller:   did,
		Status:       didStatusFromString(status),
		DocumentJson: docJSON,
		EntityId:     eid,
		Method:       didMethodFromString(method),
		CreatedAt:    timestamppb.New(createdAt),
		UpdatedAt:    timestamppb.New(updatedAt),
	}, nil
}

func (s *PostgresStore) ListDIDDocuments(ctx context.Context, entityID string, methodFilter, statusFilter string, limit, offset int) ([]*caasv1.DIDDocument, int, error) {
	query := `SELECT did, entity_id, method, status, document, created_at, updated_at FROM did_documents WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM did_documents WHERE 1=1`
	args := []any{}
	n := 0

	if entityID != "" {
		n++
		query += fmt.Sprintf(" AND entity_id = $%d", n)
		countQuery += fmt.Sprintf(" AND entity_id = $%d", n)
		args = append(args, entityID)
	}
	if methodFilter != "" {
		n++
		query += fmt.Sprintf(" AND method = $%d", n)
		countQuery += fmt.Sprintf(" AND method = $%d", n)
		args = append(args, methodFilter)
	}
	if statusFilter != "" {
		n++
		query += fmt.Sprintf(" AND status = $%d", n)
		countQuery += fmt.Sprintf(" AND status = $%d", n)
		args = append(args, statusFilter)
	}

	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count: %w", err)
	}

	n++
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", n)
	args = append(args, limit)
	n++
	query += fmt.Sprintf(" OFFSET $%d", n)
	args = append(args, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list did_documents: %w", err)
	}
	defer rows.Close()

	var docs []*caasv1.DIDDocument
	for rows.Next() {
		var did string
		var entityID *string
		var method, status, docJSON string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&did, &entityID, &method, &status, &docJSON, &createdAt, &updatedAt); err != nil {
			return nil, 0, err
		}
		eid := ""
		if entityID != nil {
			eid = *entityID
		}
		docs = append(docs, &caasv1.DIDDocument{
			Id:           did,
			Controller:   did,
			Status:       didStatusFromString(status),
			DocumentJson: docJSON,
			EntityId:     eid,
			Method:       didMethodFromString(method),
			CreatedAt:    timestamppb.New(createdAt),
			UpdatedAt:    timestamppb.New(updatedAt),
		})
	}

	return docs, total, nil
}

func (s *PostgresStore) UpdateDIDDocumentJSON(ctx context.Context, did string, document map[string]any) error {
	docJSON, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE did_documents SET document = $1, updated_at = now() WHERE did = $2`,
		docJSON, did,
	)
	return err
}

func (s *PostgresStore) DeactivateDID(ctx context.Context, did string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE did_documents SET status = 'deactivated', updated_at = now() WHERE did = $1`, did,
	)
	return err
}

func (s *PostgresStore) UpdateEntityDID(ctx context.Context, entityID, did string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE entities SET did = $1, updated_at = now() WHERE id = $2`, did, entityID,
	)
	return err
}

// --- Key Operations ---

func (s *PostgresStore) StoreKey(ctx context.Context, keyID, did, keyType, purpose, pubMultibase string, pubJWK json.RawMessage, encPrivKey, nonce []byte) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO did_keys (key_id, did, key_type, purpose, public_key_multibase, public_key_jwk, encrypted_private_key, encryption_nonce)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		keyID, did, keyType, purpose, pubMultibase, pubJWK, encPrivKey, nonce,
	)
	return err
}

func (s *PostgresStore) GetKey(ctx context.Context, keyID string) (keyType, pubMultibase string, encPrivKey, nonce []byte, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT key_type, public_key_multibase, encrypted_private_key, encryption_nonce
		 FROM did_keys WHERE key_id = $1 AND rotated = false`, keyID,
	).Scan(&keyType, &pubMultibase, &encPrivKey, &nonce)
	return
}

func (s *PostgresStore) GetDefaultAssertionKey(ctx context.Context, did string) (keyID, keyType, pubMultibase string, encPrivKey, nonce []byte, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT key_id, key_type, public_key_multibase, encrypted_private_key, encryption_nonce
		 FROM did_keys WHERE did = $1 AND purpose = 'assertion' AND rotated = false
		 ORDER BY created_at DESC LIMIT 1`, did,
	).Scan(&keyID, &keyType, &pubMultibase, &encPrivKey, &nonce)
	return
}

func (s *PostgresStore) ListKeys(ctx context.Context, did string) ([]*caasv1.KeyPairMsg, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT key_id, did, key_type, purpose, public_key_multibase, public_key_jwk, rotated, created_at
		 FROM did_keys WHERE did = $1 ORDER BY created_at DESC`, did,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*caasv1.KeyPairMsg
	for rows.Next() {
		var keyID, d, kt, purpose, pubMB string
		var pubJWK *string
		var rotated bool
		var createdAt time.Time
		if err := rows.Scan(&keyID, &d, &kt, &purpose, &pubMB, &pubJWK, &rotated, &createdAt); err != nil {
			return nil, err
		}
		jwk := ""
		if pubJWK != nil {
			jwk = *pubJWK
		}
		keys = append(keys, &caasv1.KeyPairMsg{
			KeyId:             keyID,
			Did:               d,
			KeyType:           keyTypeFromString(kt),
			Purpose:           keyPurposeFromString(purpose),
			PublicKeyMultibase: pubMB,
			PublicKeyJwk:      jwk,
			Rotated:           rotated,
			CreatedAt:         timestamppb.New(createdAt),
		})
	}
	return keys, nil
}

func (s *PostgresStore) RotateKey(ctx context.Context, keyID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE did_keys SET rotated = true, rotated_at = now() WHERE key_id = $1`, keyID,
	)
	return err
}

// --- Verifiable Credentials V2 ---

func (s *PostgresStore) CreateCredentialV2(ctx context.Context, contextJSON, typeJSON json.RawMessage, issuerDID, subjectDID, entityID string, credSubject json.RawMessage, proof json.RawMessage, issuanceDate time.Time, expirationDate *time.Time) (*caasv1.VerifiableCredentialV2, error) {
	var id string
	var storedIssuance, createdAt time.Time
	err := s.pool.QueryRow(ctx,
		`INSERT INTO verifiable_credentials_v2 (context, type, issuer_did, subject_did, entity_id, credential_subject, proof, issuance_date, expiration_date)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, issuance_date, created_at`,
		contextJSON, typeJSON, issuerDID, nilIfEmpty(subjectDID), nilIfEmpty(entityID), credSubject, proof, issuanceDate, expirationDate,
	).Scan(&id, &storedIssuance, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert vc_v2: %w", err)
	}

	vc := &caasv1.VerifiableCredentialV2{
		Id:           id,
		ContextJson:  string(contextJSON),
		TypeJson:     string(typeJSON),
		IssuerDid:    issuerDID,
		SubjectDid:   subjectDID,
		EntityId:     entityID,
		ProofJson:    string(proof),
		IssuanceDate: timestamppb.New(storedIssuance),
		Status:       caasv1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE,
	}
	if expirationDate != nil {
		vc.ExpirationDate = timestamppb.New(*expirationDate)
	}
	return vc, nil
}

func (s *PostgresStore) GetCredentialV2(ctx context.Context, id string) (*caasv1.VerifiableCredentialV2, error) {
	vc, _, err := s.GetCredentialV2WithSubject(ctx, id)
	return vc, err
}

// GetCredentialV2WithSubject returns the credential plus the raw credential_subject JSON bytes.
// The raw JSON is needed by the verify path so the signed document can be reconstructed byte-for-byte.
func (s *PostgresStore) GetCredentialV2WithSubject(ctx context.Context, id string) (*caasv1.VerifiableCredentialV2, []byte, error) {
	var contextJSON, typeJSON, issuerDID, proofJSON, status string
	var subjectDID, entityID *string
	var credSubject json.RawMessage
	var issuanceDate time.Time
	var expirationDate *time.Time

	err := s.pool.QueryRow(ctx,
		`SELECT context, type, issuer_did, subject_did, entity_id, credential_subject, proof, issuance_date, expiration_date, status
		 FROM verifiable_credentials_v2 WHERE id = $1`, id,
	).Scan(&contextJSON, &typeJSON, &issuerDID, &subjectDID, &entityID, &credSubject, &proofJSON, &issuanceDate, &expirationDate, &status)
	if err != nil {
		return nil, nil, fmt.Errorf("get vc_v2: %w", err)
	}

	vc := &caasv1.VerifiableCredentialV2{
		Id:           id,
		ContextJson:  contextJSON,
		TypeJson:     typeJSON,
		IssuerDid:    issuerDID,
		SubjectDid:   derefStr(subjectDID),
		EntityId:     derefStr(entityID),
		ProofJson:    proofJSON,
		IssuanceDate: timestamppb.New(issuanceDate),
		Status:       credentialStatusFromString(status),
	}
	if expirationDate != nil {
		vc.ExpirationDate = timestamppb.New(*expirationDate)
	}
	return vc, credSubject, nil
}

func (s *PostgresStore) RevokeCredentialV2(ctx context.Context, id, reason string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE verifiable_credentials_v2 SET status = 'revoked', revocation_reason = $1, revoked_at = now() WHERE id = $2`,
		reason, id,
	)
	return err
}

func (s *PostgresStore) ListCredentialsV2(ctx context.Context, entityID, issuerDID, statusFilter string, limit, offset int) ([]*caasv1.VerifiableCredentialV2, int, error) {
	query := `SELECT id, context, type, issuer_did, subject_did, entity_id, proof, issuance_date, expiration_date, status FROM verifiable_credentials_v2 WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM verifiable_credentials_v2 WHERE 1=1`
	args := []any{}
	n := 0

	if entityID != "" {
		n++
		f := fmt.Sprintf(" AND entity_id = $%d", n)
		query += f
		countQuery += f
		args = append(args, entityID)
	}
	if issuerDID != "" {
		n++
		f := fmt.Sprintf(" AND issuer_did = $%d", n)
		query += f
		countQuery += f
		args = append(args, issuerDID)
	}
	if statusFilter != "" {
		n++
		f := fmt.Sprintf(" AND status = $%d", n)
		query += f
		countQuery += f
		args = append(args, statusFilter)
	}

	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	n++
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", n)
	args = append(args, limit)
	n++
	query += fmt.Sprintf(" OFFSET $%d", n)
	args = append(args, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var creds []*caasv1.VerifiableCredentialV2
	for rows.Next() {
		var id, ctxJSON, tJSON, iDID, proofJSON, st string
		var sDID, eID *string
		var iDate time.Time
		var eDate *time.Time
		if err := rows.Scan(&id, &ctxJSON, &tJSON, &iDID, &sDID, &eID, &proofJSON, &iDate, &eDate, &st); err != nil {
			return nil, 0, err
		}
		vc := &caasv1.VerifiableCredentialV2{
			Id:           id,
			ContextJson:  ctxJSON,
			TypeJson:     tJSON,
			IssuerDid:    iDID,
			SubjectDid:   derefStr(sDID),
			EntityId:     derefStr(eID),
			ProofJson:    proofJSON,
			IssuanceDate: timestamppb.New(iDate),
			Status:       credentialStatusFromString(st),
		}
		if eDate != nil {
			vc.ExpirationDate = timestamppb.New(*eDate)
		}
		creds = append(creds, vc)
	}
	return creds, total, nil
}

// --- Verifiable Presentations ---

func (s *PostgresStore) CreatePresentation(ctx context.Context, holderDID string, credIDs []string, proof json.RawMessage) (*caasv1.VerifiablePresentationMsg, error) {
	var id string
	var createdAt time.Time
	err := s.pool.QueryRow(ctx,
		`INSERT INTO verifiable_presentations (holder_did, credential_ids, proof)
		 VALUES ($1, $2, $3)
		 RETURNING id, created_at`,
		holderDID, credIDs, proof,
	).Scan(&id, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert vp: %w", err)
	}

	return &caasv1.VerifiablePresentationMsg{
		Id:                      id,
		HolderDid:               holderDID,
		VerifiableCredentialIds: credIDs,
		ProofJson:               string(proof),
		CreatedAt:               timestamppb.New(createdAt),
	}, nil
}

// --- Helpers ---

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func didMethodFromString(s string) caasv1.DIDMethod {
	switch s {
	case "web":
		return caasv1.DIDMethod_DID_METHOD_WEB
	case "key":
		return caasv1.DIDMethod_DID_METHOD_KEY
	default:
		return caasv1.DIDMethod_DID_METHOD_UNSPECIFIED
	}
}

func didMethodToString(m caasv1.DIDMethod) string {
	switch m {
	case caasv1.DIDMethod_DID_METHOD_WEB:
		return "web"
	case caasv1.DIDMethod_DID_METHOD_KEY:
		return "key"
	default:
		return ""
	}
}

func didStatusFromString(s string) caasv1.DIDStatus {
	switch s {
	case "active":
		return caasv1.DIDStatus_DID_STATUS_ACTIVE
	case "deactivated":
		return caasv1.DIDStatus_DID_STATUS_DEACTIVATED
	default:
		return caasv1.DIDStatus_DID_STATUS_UNSPECIFIED
	}
}

func didStatusToString(s caasv1.DIDStatus) string {
	switch s {
	case caasv1.DIDStatus_DID_STATUS_ACTIVE:
		return "active"
	case caasv1.DIDStatus_DID_STATUS_DEACTIVATED:
		return "deactivated"
	default:
		return ""
	}
}

func keyTypeFromString(s string) caasv1.KeyType {
	switch s {
	case "Ed25519":
		return caasv1.KeyType_KEY_TYPE_ED25519
	case "P-256":
		return caasv1.KeyType_KEY_TYPE_P256
	default:
		return caasv1.KeyType_KEY_TYPE_UNSPECIFIED
	}
}

func keyTypeToString(kt caasv1.KeyType) string {
	switch kt {
	case caasv1.KeyType_KEY_TYPE_ED25519:
		return "Ed25519"
	case caasv1.KeyType_KEY_TYPE_P256:
		return "P-256"
	default:
		return "Ed25519"
	}
}

func keyPurposeFromString(s string) caasv1.KeyPurpose {
	switch s {
	case "authentication":
		return caasv1.KeyPurpose_KEY_PURPOSE_AUTHENTICATION
	case "assertion":
		return caasv1.KeyPurpose_KEY_PURPOSE_ASSERTION
	case "keyAgreement":
		return caasv1.KeyPurpose_KEY_PURPOSE_KEY_AGREEMENT
	case "capabilityInvocation":
		return caasv1.KeyPurpose_KEY_PURPOSE_CAPABILITY_INVOCATION
	default:
		return caasv1.KeyPurpose_KEY_PURPOSE_UNSPECIFIED
	}
}

func keyPurposeToString(kp caasv1.KeyPurpose) string {
	switch kp {
	case caasv1.KeyPurpose_KEY_PURPOSE_AUTHENTICATION:
		return "authentication"
	case caasv1.KeyPurpose_KEY_PURPOSE_ASSERTION:
		return "assertion"
	case caasv1.KeyPurpose_KEY_PURPOSE_KEY_AGREEMENT:
		return "keyAgreement"
	case caasv1.KeyPurpose_KEY_PURPOSE_CAPABILITY_INVOCATION:
		return "capabilityInvocation"
	default:
		return "assertion"
	}
}

func credentialStatusFromString(s string) caasv1.CredentialStatus {
	switch s {
	case "active":
		return caasv1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE
	case "revoked":
		return caasv1.CredentialStatus_CREDENTIAL_STATUS_REVOKED
	case "expired":
		return caasv1.CredentialStatus_CREDENTIAL_STATUS_EXPIRED
	default:
		return caasv1.CredentialStatus_CREDENTIAL_STATUS_UNSPECIFIED
	}
}

func credentialStatusToString(s caasv1.CredentialStatus) string {
	switch s {
	case caasv1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE:
		return "active"
	case caasv1.CredentialStatus_CREDENTIAL_STATUS_REVOKED:
		return "revoked"
	case caasv1.CredentialStatus_CREDENTIAL_STATUS_EXPIRED:
		return "expired"
	default:
		return ""
	}
}
