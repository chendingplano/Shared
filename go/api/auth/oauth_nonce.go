// Package auth provides OAuth nonce management for CSRF protection.
// This file implements per-request, time-limited, one-time-use nonces
// that replace the previous static nonce approach.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

// ErrNonceCacheFull is returned when the nonce cache reaches its maximum size.
// This indicates a potential DoS attack or system overload.
var ErrNonceCacheFull = errors.New("oauth nonce cache full - possible DoS attack")

// nonceEntry stores a nonce with its expiration time
type nonceEntry struct {
	expiresAt time.Time
}

// nonceCache provides thread-safe storage for OAuth state nonces
type nonceCache struct {
	mu       sync.RWMutex
	entries  map[string]nonceEntry
	ttl      time.Duration
	maxSize  int // Maximum number of nonces to store (DoS protection)
}

// Default nonce cache instance with 5-minute TTL and 100,000 max entries
// This is sufficient for OAuth flows while limiting attack windows
// At 100,000 entries with 44-byte keys, max memory usage is ~10MB
var oauthNonceCache = &nonceCache{
	entries: make(map[string]nonceEntry),
	ttl:     5 * time.Minute,
	maxSize: 100000,
}

// GenerateOAuthNonce creates a cryptographically secure, time-limited nonce.
// The nonce is 32 bytes (256 bits) of randomness, base64url-encoded.
// It automatically cleans up expired nonces on each call.
//
// SECURITY: This function will panic if crypto/rand fails. This is intentional -
// we must never fall back to predictable values for security-critical tokens.
// A panic is preferable to a security vulnerability.
//
// Returns empty string and logs error if cache is full (DoS protection).
func GenerateOAuthNonce() string {
	oauthNonceCache.cleanup()

	// Check cache size limit before generating (DoS protection)
	oauthNonceCache.mu.RLock()
	currentSize := len(oauthNonceCache.entries)
	oauthNonceCache.mu.RUnlock()

	if currentSize >= oauthNonceCache.maxSize {
		// Cache is full - this could indicate a DoS attack
		// Return empty string; caller should handle this gracefully
		return ""
	}

	// Generate 32 bytes of cryptographic randomness
	// SECURITY: We panic on failure rather than falling back to weak values.
	// crypto/rand.Read failing indicates a serious system problem (no entropy).
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		// FAIL SECURE: Panic rather than use predictable values
		// This should never happen on properly configured systems
		panic("crypto/rand.Read failed: " + err.Error())
	}

	nonce := base64.URLEncoding.EncodeToString(bytes)

	oauthNonceCache.mu.Lock()
	oauthNonceCache.entries[nonce] = nonceEntry{
		expiresAt: time.Now().Add(oauthNonceCache.ttl),
	}
	oauthNonceCache.mu.Unlock()

	return nonce
}

// ValidateOAuthNonce checks if a nonce is valid and consumes it.
// Returns true if the nonce exists and hasn't expired.
// The nonce is deleted after validation (one-time use).
func ValidateOAuthNonce(nonce string) bool {
	if nonce == "" {
		return false
	}

	oauthNonceCache.mu.Lock()
	defer oauthNonceCache.mu.Unlock()

	entry, exists := oauthNonceCache.entries[nonce]
	if !exists {
		return false
	}

	// Delete the nonce regardless of validity (one-time use)
	delete(oauthNonceCache.entries, nonce)

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		return false
	}

	return true
}

// cleanup removes expired nonces from the cache.
// Called lazily during nonce generation to avoid memory leaks.
func (nc *nonceCache) cleanup() {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	now := time.Now()
	for nonce, entry := range nc.entries {
		if now.After(entry.expiresAt) {
			delete(nc.entries, nonce)
		}
	}
}

// GetNonceCacheSize returns the current number of nonces in cache.
// Useful for monitoring and debugging.
func GetNonceCacheSize() int {
	oauthNonceCache.mu.RLock()
	defer oauthNonceCache.mu.RUnlock()
	return len(oauthNonceCache.entries)
}

// GetNonceCacheMaxSize returns the maximum cache size limit.
func GetNonceCacheMaxSize() int {
	return oauthNonceCache.maxSize
}

// IsNonceCacheFull returns true if the cache has reached its size limit.
func IsNonceCacheFull() bool {
	oauthNonceCache.mu.RLock()
	defer oauthNonceCache.mu.RUnlock()
	return len(oauthNonceCache.entries) >= oauthNonceCache.maxSize
}
