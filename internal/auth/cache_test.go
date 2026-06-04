package auth

import (
	"errors"
	"testing"
	"time"
)

type countingAuthenticator struct {
	calls  int
	result func(username, password string) Result
}

func (c *countingAuthenticator) Authenticate(username, password string) Result {
	c.calls++
	return c.result(username, password)
}

func (c *countingAuthenticator) Available() bool { return true }
func (c *countingAuthenticator) Type() string    { return "counting" }

func validIf(user, pass string) func(string, string) Result {
	return func(u, p string) Result {
		return Result{Valid: u == user && p == pass, Username: u}
	}
}

func newTestCache(inner Authenticator, ttl time.Duration, now func() time.Time) *cachingAuthenticator {
	c := WithCache(inner, ttl).(*cachingAuthenticator)
	c.now = now
	return c
}

func TestCache_HitSkipsInner(t *testing.T) {
	inner := &countingAuthenticator{result: validIf("admin", "secret")}
	c := newTestCache(inner, time.Minute, func() time.Time { return time.Unix(0, 0) })

	for range 5 {
		if !c.Authenticate("admin", "secret").Valid {
			t.Fatal("expected valid result")
		}
	}
	if inner.calls != 1 {
		t.Errorf("inner called %d times, want 1 (rest served from cache)", inner.calls)
	}
}

func TestCache_ExpiryReconsultsInner(t *testing.T) {
	inner := &countingAuthenticator{result: validIf("admin", "secret")}
	now := time.Unix(0, 0)
	c := newTestCache(inner, time.Minute, func() time.Time { return now })

	c.Authenticate("admin", "secret")
	now = now.Add(time.Minute + time.Second)
	c.Authenticate("admin", "secret")

	if inner.calls != 2 {
		t.Errorf("inner called %d times, want 2 (cache expired)", inner.calls)
	}
}

func TestCache_FailuresNotCached(t *testing.T) {
	inner := &countingAuthenticator{result: validIf("admin", "secret")}
	c := newTestCache(inner, time.Minute, func() time.Time { return time.Unix(0, 0) })

	for range 3 {
		if c.Authenticate("admin", "wrong").Valid {
			t.Fatal("expected invalid result")
		}
	}
	if inner.calls != 3 {
		t.Errorf("inner called %d times, want 3 (failures must not be cached)", inner.calls)
	}
}

func TestCache_SystemErrorNotCached(t *testing.T) {
	inner := &countingAuthenticator{result: func(u, _ string) Result {
		return Result{Valid: false, Username: u, Error: errors.New("shadow read denied")}
	}}
	c := newTestCache(inner, time.Minute, func() time.Time { return time.Unix(0, 0) })

	c.Authenticate("admin", "secret")
	c.Authenticate("admin", "secret")
	if inner.calls != 2 {
		t.Errorf("inner called %d times, want 2 (errors must not be cached)", inner.calls)
	}
}

func TestCache_DistinctCredentialsDistinctKeys(t *testing.T) {
	inner := &countingAuthenticator{result: func(u, _ string) Result {
		return Result{Valid: true, Username: u}
	}}
	c := newTestCache(inner, time.Minute, func() time.Time { return time.Unix(0, 0) })

	// Length-prefixing must keep ("ab","c") and ("a","bc") distinct.
	if cacheKey("ab", "c") == cacheKey("a", "bc") {
		t.Fatal("cache keys collided across the username/password boundary")
	}

	c.Authenticate("alice", "p1")
	c.Authenticate("bob", "p2")
	c.Authenticate("alice", "p1") // cached
	if inner.calls != 2 {
		t.Errorf("inner called %d times, want 2 (two distinct creds, one repeat)", inner.calls)
	}
}

func TestWithCache_ZeroTTLDisabled(t *testing.T) {
	inner := &countingAuthenticator{result: validIf("admin", "secret")}
	c := WithCache(inner, 0)
	if _, ok := c.(*cachingAuthenticator); ok {
		t.Fatal("zero TTL should return the authenticator unwrapped")
	}
	c.Authenticate("admin", "secret")
	c.Authenticate("admin", "secret")
	if inner.calls != 2 {
		t.Errorf("inner called %d times, want 2 (caching disabled)", inner.calls)
	}
}
