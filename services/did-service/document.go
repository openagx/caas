package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

// BuildDIDDocument constructs a W3C DID Document JSON-LD structure.
func BuildDIDDocument(did, entityID, domain, keyID, keyType, pubKeyMultibase string) map[string]any {
	vmType := "Ed25519VerificationKey2020"
	if keyType == "P-256" {
		vmType = "JsonWebKey2020"
	}

	vm := map[string]any{
		"id":                 fmt.Sprintf("%s#%s", did, keyID),
		"type":               vmType,
		"controller":         did,
		"publicKeyMultibase": pubKeyMultibase,
	}

	doc := map[string]any{
		"@context": []string{
			"https://www.w3.org/ns/did/v1",
			"https://w3id.org/security/suites/ed25519-2020/v1",
		},
		"id":                 did,
		"controller":         []string{did},
		"verificationMethod": []any{vm},
		"authentication":     []string{fmt.Sprintf("%s#%s", did, keyID)},
		"assertionMethod":    []string{fmt.Sprintf("%s#%s", did, keyID)},
	}

	if entityID != "" && domain != "" {
		doc["service"] = []any{
			map[string]any{
				"id":              fmt.Sprintf("%s#caas", did),
				"type":            "CAASEntity",
				"serviceEndpoint": fmt.Sprintf("https://%s/v1/entities/%s", domain, entityID),
			},
		}
	}

	return doc
}

// BuildVerificationMethod creates a verification method entry.
func BuildVerificationMethod(did, keyID, keyType, pubKeyMultibase string) map[string]any {
	vmType := "Ed25519VerificationKey2020"
	if keyType == "P-256" {
		vmType = "JsonWebKey2020"
	}
	return map[string]any{
		"id":                 fmt.Sprintf("%s#%s", did, keyID),
		"type":               vmType,
		"controller":         did,
		"publicKeyMultibase": pubKeyMultibase,
	}
}

// AddVerificationMethodToDoc adds a verification method to a DID Document and updates relationship arrays.
func AddVerificationMethodToDoc(doc map[string]any, vm map[string]any, purposes []string) {
	vms, _ := doc["verificationMethod"].([]any)
	doc["verificationMethod"] = append(vms, vm)

	vmID, _ := vm["id"].(string)
	for _, purpose := range purposes {
		refs, _ := doc[purpose].([]string)
		doc[purpose] = append(refs, vmID)
	}
}

// RemoveVerificationMethodFromDoc removes a verification method by ID.
func RemoveVerificationMethodFromDoc(doc map[string]any, vmID string) {
	if vms, ok := doc["verificationMethod"].([]any); ok {
		var filtered []any
		for _, vm := range vms {
			if m, ok := vm.(map[string]any); ok {
				if id, _ := m["id"].(string); id != vmID {
					filtered = append(filtered, vm)
				}
			}
		}
		doc["verificationMethod"] = filtered
	}

	// Remove from relationship arrays
	for _, rel := range []string{"authentication", "assertionMethod", "keyAgreement", "capabilityInvocation"} {
		if refs, ok := doc[rel].([]string); ok {
			var filtered []string
			for _, ref := range refs {
				if ref != vmID {
					filtered = append(filtered, ref)
				}
			}
			doc[rel] = filtered
		}
	}
}

// AddServiceToDoc adds a service endpoint to a DID Document.
func AddServiceToDoc(doc map[string]any, svc map[string]any) {
	svcs, _ := doc["service"].([]any)
	doc["service"] = append(svcs, svc)
}

// RemoveServiceFromDoc removes a service endpoint by ID.
func RemoveServiceFromDoc(doc map[string]any, svcID string) {
	if svcs, ok := doc["service"].([]any); ok {
		var filtered []any
		for _, svc := range svcs {
			if m, ok := svc.(map[string]any); ok {
				if id, _ := m["id"].(string); id != svcID {
					filtered = append(filtered, svc)
				}
			}
		}
		doc["service"] = filtered
	}
}

// CanonicalizeJSON produces deterministic JSON for signing (sorted keys).
func CanonicalizeJSON(doc map[string]any) ([]byte, error) {
	return marshalSorted(doc)
}

func marshalSorted(v any) ([]byte, error) {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf := []byte("{")
		for i, k := range keys {
			if i > 0 {
				buf = append(buf, ',')
			}
			keyJSON, _ := json.Marshal(k)
			buf = append(buf, keyJSON...)
			buf = append(buf, ':')
			valJSON, err := marshalSorted(val[k])
			if err != nil {
				return nil, err
			}
			buf = append(buf, valJSON...)
		}
		buf = append(buf, '}')
		return buf, nil
	case []any:
		buf := []byte("[")
		for i, item := range val {
			if i > 0 {
				buf = append(buf, ',')
			}
			itemJSON, err := marshalSorted(item)
			if err != nil {
				return nil, err
			}
			buf = append(buf, itemJSON...)
		}
		buf = append(buf, ']')
		return buf, nil
	default:
		return json.Marshal(v)
	}
}

// ConstructDIDWeb builds a did:web DID string.
func ConstructDIDWeb(domain, entityID string) string {
	return fmt.Sprintf("did:web:%s:entities:%s", domain, entityID)
}

// ConstructDIDKey builds a did:key DID string from a public key.
func ConstructDIDKey(pubKeyMultibase string) string {
	return fmt.Sprintf("did:key:%s", pubKeyMultibase)
}
