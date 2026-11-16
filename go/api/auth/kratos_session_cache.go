////////////////////////////////////////////////////////////
//
// Description:
// Short-lived in-memory cache of successful Kratos session lookups.
//
// Every /api request (and every protected frontend route) validates its
// session by calling Kratos /sessions/whoami. Pages that poll (e.g. the
// system monitor refreshes every second) would otherwise turn into one
// Kratos call per poll. Caching a *successful* lookup for a short TTL removes
// that load. Failed lookups are never cached.
//
// Trade-off: a session revoked elsewhere (logout on another device, admin
// disable) can keep working for up to sessionCacheTTL. Logout on this server
// evicts the entry immediately.
//
// Keys are SHA-256 digests of the request's Cookie and X-Session-Token
// headers, so raw credentials are not kept as map keys.
////////////////////////////////////////////////////////////

package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	ory "github.com/ory/client-go"
)

// sessionCacheTTL bounds how long a successful Kratos session lookup is reused.
var sessionCacheTTL = 30 * time.Second

// sessionCacheMaxEntries caps memory use. When reached, expired entries are
// purged; if the cache is still full, it is cleared.
const sessionCacheMaxEntries = 4096

type cachedKratosSession struct {
	session   *ory.Session
	expiresAt time.Time
}

var (
	sessionCacheMu sync.Mutex
	sessionCache   = map[string]cachedKratosSession{}
)

// sessionCacheKey returns "" (not cacheable) when no credentials are present.
func sessionCacheKey(cookie, sessionToken string) string {
	if cookie == "" && sessionToken == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(cookie + "\x00" + sessionToken))
	return hex.EncodeToString(sum[:])
}

// getCachedSession returns the cached session for key, or nil on miss/expiry.
func getCachedSession(key string) *ory.Session {
	if key == "" {
		return nil
	}
	sessionCacheMu.Lock()
	defer sessionCacheMu.Unlock()

	entry, ok := sessionCache[key]
	if !ok {
		return nil
	}
	if time.Now().After(entry.expiresAt) {
		delete(sessionCache, key)
		return nil
	}
	return entry.session
}

// putCachedSession stores a successful session lookup for sessionCacheTTL.
func putCachedSession(key string, session *ory.Session) {
	if key == "" || session == nil {
		return
	}
	sessionCacheMu.Lock()
	defer sessionCacheMu.Unlock()

	now := time.Now()
	if len(sessionCache) >= sessionCacheMaxEntries {
		for k, e := range sessionCache {
			if now.After(e.expiresAt) {
				delete(sessionCache, k)
			}
		}
		if len(sessionCache) >= sessionCacheMaxEntries {
			sessionCache = map[string]cachedKratosSession{}
		}
	}
	sessionCache[key] = cachedKratosSession{session: session, expiresAt: now.Add(sessionCacheTTL)}
}

// evictCachedSession removes the entry for key, if any.
func evictCachedSession(key string) {
	if key == "" {
		return
	}
	sessionCacheMu.Lock()
	defer sessionCacheMu.Unlock()
	delete(sessionCache, key)
}
