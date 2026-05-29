package auth

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"time"
)

// DefaultCacheTTL is how long a successful authentication is trusted before
// the underlying authenticator is consulted again.
const DefaultCacheTTL = 60 * time.Second

// maxCacheEntries bounds the cache so a flood of distinct credentials cannot
// grow it without limit. When exceeded, expired entries are pruned; if still
// full, new successes are simply not cached (the underlying authenticator
// still runs, so behaviour stays correct — just uncached).
const maxCacheEntries = 1024

// cachingAuthenticator wraps an Authenticator and memoizes successful results
// for a short TTL, keyed by username plus a SHA-256 of the password (plaintext
// is never retained). Rejections and system errors are never cached, so a wrong
// password keeps paying full verification cost and a flipped account state is
// seen within one TTL. The point is the API polling firehose: the same valid
// credential is otherwise re-verified — running the memory-hard yescrypt KDF
// and emitting an "auth accepted" log — on every request.
type cachingAuthenticator struct {
	inner Authenticator
	ttl   time.Duration
	now   func() time.Time

	mu      sync.Mutex
	entries map[[32]byte]cacheEntry
}

type cacheEntry struct {
	result  Result
	expires time.Time
}

// WithCache wraps an authenticator so successful results are memoized for ttl.
// A non-positive ttl disables caching and returns the authenticator unchanged.
func WithCache(inner Authenticator, ttl time.Duration) Authenticator {
	if ttl <= 0 {
		return inner
	}
	return &cachingAuthenticator{
		inner:   inner,
		ttl:     ttl,
		now:     time.Now,
		entries: make(map[[32]byte]cacheEntry),
	}
}

// Authenticate returns a cached success when one is live, otherwise delegates
// to the wrapped authenticator and caches the result if it is a clean success.
func (c *cachingAuthenticator) Authenticate(username, password string) Result {
	key := cacheKey(username, password)
	now := c.now()

	c.mu.Lock()
	if e, ok := c.entries[key]; ok {
		if now.Before(e.expires) {
			c.mu.Unlock()
			return e.result
		}
		delete(c.entries, key)
	}
	c.mu.Unlock()

	result := c.inner.Authenticate(username, password)
	if !result.Valid || result.Error != nil {
		return result
	}

	c.mu.Lock()
	if len(c.entries) >= maxCacheEntries {
		c.pruneExpiredLocked(now)
	}
	if len(c.entries) < maxCacheEntries {
		c.entries[key] = cacheEntry{result: result, expires: now.Add(c.ttl)}
	}
	c.mu.Unlock()

	return result
}

func (c *cachingAuthenticator) pruneExpiredLocked(now time.Time) {
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
		}
	}
}

// Available reports whether the wrapped authenticator is usable.
func (c *cachingAuthenticator) Available() bool { return c.inner.Available() }

// Type returns the wrapped authenticator's type.
func (c *cachingAuthenticator) Type() string { return c.inner.Type() }

// cacheKey derives a collision-resistant key from the credentials without
// retaining the plaintext password. Length prefixes prevent ("ab","c") and
// ("a","bc") from colliding.
func cacheKey(username, password string) [32]byte {
	h := sha256.New()
	var n [8]byte
	binary.LittleEndian.PutUint64(n[:], uint64(len(username)))
	h.Write(n[:])
	h.Write([]byte(username))
	h.Write([]byte(password))
	var key [32]byte
	h.Sum(key[:0])
	return key
}
