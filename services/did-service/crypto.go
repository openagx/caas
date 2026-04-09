package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
)

// --- Key Generation ---

func GenerateEd25519KeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate ed25519: %w", err)
	}
	return pub, priv, nil
}

func GenerateP256KeyPair() (*ecdsa.PublicKey, *ecdsa.PrivateKey, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate p256: %w", err)
	}
	return &priv.PublicKey, priv, nil
}

// --- AES-256-GCM Encryption ---

func EncryptPrivateKey(plaintext, encryptionKey []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("gcm: %w", err)
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("nonce: %w", err)
	}
	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func DecryptPrivateKey(ciphertext, nonce, encryptionKey []byte) ([]byte, error) {
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

// --- Signing ---

func SignEd25519(privateKey ed25519.PrivateKey, message []byte) []byte {
	return ed25519.Sign(privateKey, message)
}

func VerifyEd25519Signature(publicKey ed25519.PublicKey, message, signature []byte) bool {
	return ed25519.Verify(publicKey, message, signature)
}

func SignP256(privateKey *ecdsa.PrivateKey, message []byte) ([]byte, error) {
	hash := sha256.Sum256(message)
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	if err != nil {
		return nil, fmt.Errorf("p256 sign: %w", err)
	}
	// Encode as r || s (32 bytes each for P-256)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	sig := make([]byte, 64)
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):64], sBytes)
	return sig, nil
}

func VerifyP256Signature(publicKey *ecdsa.PublicKey, message, signature []byte) bool {
	if len(signature) != 64 {
		return false
	}
	hash := sha256.Sum256(message)
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	return ecdsa.Verify(publicKey, hash[:], r, s)
}

// --- Multibase Encoding ---
// Uses 'z' prefix (base58btc) for simplicity. For MVP, we use base64url with 'u' prefix.

func PublicKeyToMultibase(pub []byte, keyType string) string {
	// Multicodec prefixes
	var prefix []byte
	switch keyType {
	case "Ed25519":
		prefix = []byte{0xed, 0x01} // ed25519-pub multicodec
	case "P-256":
		prefix = []byte{0x80, 0x24} // p256-pub multicodec
	default:
		prefix = []byte{}
	}
	data := append(prefix, pub...)
	return "z" + base58Encode(data)
}

func MultibaseToPublicKey(multibase string) ([]byte, string, error) {
	if len(multibase) < 2 {
		return nil, "", errors.New("invalid multibase string")
	}
	if multibase[0] != 'z' {
		return nil, "", fmt.Errorf("unsupported multibase prefix: %c", multibase[0])
	}
	data, err := base58Decode(multibase[1:])
	if err != nil {
		return nil, "", fmt.Errorf("base58 decode: %w", err)
	}
	if len(data) < 2 {
		return nil, "", errors.New("data too short")
	}
	if data[0] == 0xed && data[1] == 0x01 {
		return data[2:], "Ed25519", nil
	}
	if data[0] == 0x80 && data[1] == 0x24 {
		return data[2:], "P-256", nil
	}
	return data, "unknown", nil
}

// --- JWK Export ---

func Ed25519PublicKeyToJWK(pub ed25519.PublicKey) json.RawMessage {
	jwk := map[string]string{
		"kty": "OKP",
		"crv": "Ed25519",
		"x":   base64.RawURLEncoding.EncodeToString(pub),
	}
	b, _ := json.Marshal(jwk)
	return b
}

func P256PublicKeyToJWK(pub *ecdsa.PublicKey) json.RawMessage {
	jwk := map[string]string{
		"kty": "EC",
		"crv": "P-256",
		"x":   base64.RawURLEncoding.EncodeToString(pub.X.Bytes()),
		"y":   base64.RawURLEncoding.EncodeToString(pub.Y.Bytes()),
	}
	b, _ := json.Marshal(jwk)
	return b
}

// --- Encryption Key Parsing ---

func ParseEncryptionKey(hexKey string) ([]byte, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid hex key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}
	return key, nil
}

// --- Base58 (Bitcoin alphabet) ---

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func base58Encode(data []byte) string {
	x := new(big.Int).SetBytes(data)
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)
	var result []byte
	for x.Cmp(zero) > 0 {
		x.DivMod(x, base, mod)
		result = append([]byte{base58Alphabet[mod.Int64()]}, result...)
	}
	for _, b := range data {
		if b != 0 {
			break
		}
		result = append([]byte{base58Alphabet[0]}, result...)
	}
	return string(result)
}

func base58Decode(s string) ([]byte, error) {
	x := big.NewInt(0)
	base := big.NewInt(58)
	for _, c := range s {
		idx := -1
		for i, a := range base58Alphabet {
			if a == c {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, fmt.Errorf("invalid base58 character: %c", c)
		}
		x.Mul(x, base)
		x.Add(x, big.NewInt(int64(idx)))
	}
	result := x.Bytes()
	for _, c := range s {
		if c != rune(base58Alphabet[0]) {
			break
		}
		result = append([]byte{0}, result...)
	}
	return result, nil
}
