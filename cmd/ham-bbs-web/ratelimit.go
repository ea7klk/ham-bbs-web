package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Login throttling (parity with the classic SSH BBS, simple-ham-bbs
// bbs/cmd/bbs/ratelimit.go).
//
// The SSH BBS stores its counters in SQLite because every session is its own
// process. The web app is a single process, so an in-memory table serves every
// request and survives across requests; it is rebuilt empty on restart, which
// is the documented trade-off.
//
// The key includes the callsign as well as the source address. The web app is
// published through a reverse proxy, so the proxy address is the peer for every
// caller; keying on the address alone would turn five wrong passwords anywhere
// into a site-wide lockout. Per account-and-address keeps the protection
// (guessing one account's password is capped) without that denial of service.

const (
	// authFailureWindow is the period over which failures accumulate.
	authFailureWindow = 15 * time.Minute
	// authFailureLimit is the number of failed attempts within that window
	// before the key is locked out.
	authFailureLimit = 5
	// authLockoutDuration is how long a locked key must wait.
	authLockoutDuration = 15 * time.Minute
	// authFailureRetention is how long an idle entry is kept.
	authFailureRetention = 24 * time.Hour
)

type authFailure struct {
	failures    int
	windowStart time.Time
	lockedUntil time.Time
}

type authThrottle struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]authFailure
}

func newAuthThrottle() *authThrottle {
	return &authThrottle{now: time.Now, entries: map[string]authFailure{}}
}

// clientAddress returns the peer address without its port. When the request has
// no usable peer, every such caller shares one bucket rather than escaping the
// limit.
func clientAddress(r *http.Request) string {
	raw := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	if raw == "" || len(raw) > 64 {
		return "unknown"
	}
	return raw
}

// authThrottleKey names one account as attempted from one source address.
func authThrottleKey(r *http.Request, callsign string) string {
	return clientAddress(r) + "|" + normalizeCallsign(callsign)
}

// lockedFor reports how much longer key is locked out, or zero.
func (t *authThrottle) lockedFor(key string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry, ok := t.entries[key]
	if !ok || entry.lockedUntil.IsZero() {
		return 0
	}
	remaining := entry.lockedUntil.Sub(t.now())
	if remaining <= 0 {
		delete(t.entries, key)
		return 0
	}
	return remaining
}

// recordFailure counts one failed attempt and locks the key once the window
// limit is reached. It returns whether the key is now locked.
func (t *authThrottle) recordFailure(key string) (bool, time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	at := t.now()
	entry := t.entries[key]
	if entry.windowStart.IsZero() || at.Sub(entry.windowStart) > authFailureWindow {
		entry.failures = 0
		entry.windowStart = at
	}
	entry.failures++
	if entry.failures >= authFailureLimit {
		entry.failures = 0
		entry.windowStart = at
		entry.lockedUntil = at.Add(authLockoutDuration)
	}
	t.entries[key] = entry
	t.prune(at)
	return !entry.lockedUntil.IsZero(), entry.lockedUntil.Sub(at)
}

// clear forgets a key's failures after a successful login.
func (t *authThrottle) clear(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, key)
}

// prune drops entries that have been idle past their retention, so the table
// cannot grow without bound. The caller holds t.mu.
func (t *authThrottle) prune(at time.Time) {
	if len(t.entries) < 64 {
		return
	}
	cutoff := at.Add(-authFailureRetention)
	for key, entry := range t.entries {
		if entry.windowStart.Before(cutoff) && entry.lockedUntil.Before(cutoff) {
			delete(t.entries, key)
		}
	}
}
