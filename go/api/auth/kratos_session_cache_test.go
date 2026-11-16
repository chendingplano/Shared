package auth

import (
	"testing"
	"time"

	ory "github.com/ory/client-go"
)

func resetSessionCache(t *testing.T) {
	t.Helper()
	sessionCacheMu.Lock()
	sessionCache = map[string]cachedKratosSession{}
	sessionCacheMu.Unlock()
}

func TestSessionCacheKeyEmptyCredentialsNotCacheable(t *testing.T) {
	if k := sessionCacheKey("", ""); k != "" {
		t.Fatalf("expected empty key for no credentials, got %q", k)
	}
	if k := sessionCacheKey("a=1", ""); k == "" {
		t.Fatal("expected non-empty key for cookie credentials")
	}
}

func TestSessionCacheKeyDistinguishesCredentials(t *testing.T) {
	a := sessionCacheKey("ory_kratos_session=abc", "")
	b := sessionCacheKey("ory_kratos_session=abd", "")
	c := sessionCacheKey("", "token-1")
	if a == b || a == c {
		t.Fatalf("keys must differ for different credentials: %q %q %q", a, b, c)
	}
	if a == "ory_kratos_session=abc" {
		t.Fatal("key must not be the raw credential")
	}
}

func TestSessionCachePutGetEvict(t *testing.T) {
	resetSessionCache(t)
	key := sessionCacheKey("ory_kratos_session=abc", "")
	sess := &ory.Session{Id: "s1"}

	if got := getCachedSession(key); got != nil {
		t.Fatal("expected miss before put")
	}
	putCachedSession(key, sess)
	if got := getCachedSession(key); got != sess {
		t.Fatal("expected hit after put")
	}
	evictCachedSession(key)
	if got := getCachedSession(key); got != nil {
		t.Fatal("expected miss after evict")
	}
}

func TestSessionCacheExpires(t *testing.T) {
	resetSessionCache(t)
	old := sessionCacheTTL
	sessionCacheTTL = 10 * time.Millisecond
	defer func() { sessionCacheTTL = old }()

	key := sessionCacheKey("ory_kratos_session=exp", "")
	putCachedSession(key, &ory.Session{Id: "s2"})
	time.Sleep(30 * time.Millisecond)
	if got := getCachedSession(key); got != nil {
		t.Fatal("expected expired entry to miss")
	}
}

func TestSessionCacheIgnoresNilAndEmpty(t *testing.T) {
	resetSessionCache(t)
	putCachedSession("", &ory.Session{Id: "x"})
	putCachedSession("k", nil)
	if len(sessionCache) != 0 {
		t.Fatalf("expected no entries, got %d", len(sessionCache))
	}
}

func TestSessionCacheBoundedSize(t *testing.T) {
	resetSessionCache(t)
	for i := 0; i < sessionCacheMaxEntries+10; i++ {
		putCachedSession(sessionCacheKey("cookie", string(rune(i))+time.Now().String()), &ory.Session{Id: "s"})
	}
	sessionCacheMu.Lock()
	n := len(sessionCache)
	sessionCacheMu.Unlock()
	if n > sessionCacheMaxEntries {
		t.Fatalf("cache exceeded cap: %d", n)
	}
}
