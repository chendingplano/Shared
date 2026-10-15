package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
)

// AESGCMKeyLen is the required key length in bytes for EncryptString /
// DecryptString (AES-256-GCM).
const AESGCMKeyLen = 32

// Errors returned by this package.
var (
	ErrKeyLength        = errors.New("security: AES-GCM key must be 32 bytes")
	ErrCiphertextShort  = errors.New("security: ciphertext too short")
	ErrEnvVarMissing    = errors.New("security: environment variable is empty or unset")
	ErrEnvVarNotBase64  = errors.New("security: environment variable is not valid base64")
	ErrEnvVarKeyLength  = errors.New("security: decoded key has wrong length")
	ErrKeySourcesDiffer = errors.New("security: key in the environment differs from the one in the env file")
)

// EncryptString encrypts plaintext with AES-256-GCM using the given 32-byte
// key and returns base64-encoded (nonce || ciphertext || tag).
//
// A fresh 12-byte nonce is generated from crypto/rand for each call; callers
// must not reuse the returned ciphertext as a nonce source.
func EncryptString(plaintext string, key []byte) (string, error) {
	if len(key) != AESGCMKeyLen {
		return "", fmt.Errorf("%w: got %d", ErrKeyLength, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("security: aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("security: cipher.NewGCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("security: rand.Read nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// DecryptString reverses EncryptString. Returns an error if the ciphertext
// is malformed, truncated, or fails authentication under the given key.
func DecryptString(ciphertextB64 string, key []byte) (string, error) {
	if len(key) != AESGCMKeyLen {
		return "", fmt.Errorf("%w: got %d", ErrKeyLength, len(key))
	}
	if ciphertextB64 == "" {
		return "", ErrCiphertextShort
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", fmt.Errorf("security: base64 decode: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("security: aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("security: cipher.NewGCM: %w", err)
	}
	ns := gcm.NonceSize()
	if len(raw) < ns+gcm.Overhead() {
		return "", ErrCiphertextShort
	}
	nonce, sealed := raw[:ns], raw[ns:]
	pt, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("security: gcm.Open: %w", err)
	}
	return string(pt), nil
}

// LoadKeyFromEnv reads a base64-encoded 32-byte key from the given env var.
// The server should call this at startup and refuse to boot on error, so
// plaintext secrets are never silently accepted.
func LoadKeyFromEnv(varName string) ([]byte, error) {
	return decodeKey(varName, os.Getenv(varName))
}

// LoadKeyFromEnvOrFile is LoadKeyFromEnv for a project that also keeps the key in an env file
// such as .env. Loaders like godotenv never override a variable that is already set, so a stale
// copy in the shell or in a tool's config would silently win over the file, and data would be
// encrypted under a key nobody meant to use. fileValue is what the env file holds for varName
// ("" when the file or the line is missing); the caller reads the file, so this package needs no
// env-file parser. When both are set and differ it refuses, naming each key only by a short
// SHA-256 fingerprint of its base64 text.
func LoadKeyFromEnvOrFile(varName, fileValue string) ([]byte, error) {
	v := os.Getenv(varName)
	if v != "" && fileValue != "" && v != fileValue {
		return nil, fmt.Errorf("%w: %s (environment sha256 %s, env file sha256 %s)",
			ErrKeySourcesDiffer, varName, fingerprint(v), fingerprint(fileValue))
	}
	if v == "" {
		v = fileValue
	}
	return decodeKey(varName, v)
}

func fingerprint(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:6])
}

func decodeKey(varName, v string) ([]byte, error) {
	if v == "" {
		return nil, fmt.Errorf("%w: %s", ErrEnvVarMissing, varName)
	}
	raw, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrEnvVarNotBase64, varName, err)
	}
	if len(raw) != AESGCMKeyLen {
		return nil, fmt.Errorf("%w: %s decoded to %d bytes, want %d",
			ErrEnvVarKeyLength, varName, len(raw), AESGCMKeyLen)
	}
	return raw, nil
}
