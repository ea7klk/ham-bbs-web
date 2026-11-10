package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testThrottle(now *time.Time) *authThrottle {
	throttle := newAuthThrottle()
	throttle.now = func() time.Time { return *now }
	return throttle
}

func TestAuthThrottleLocksAfterLimit(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	throttle := testThrottle(&now)
	key := "203.0.113.7|EA1ABC"

	for attempt := 1; attempt < authFailureLimit; attempt++ {
		if locked, _ := throttle.recordFailure(key); locked {
			t.Fatalf("locked at attempt %d, limit is %d", attempt, authFailureLimit)
		}
	}
	locked, wait := throttle.recordFailure(key)
	if !locked || wait != authLockoutDuration {
		t.Fatalf("attempt %d: locked=%t wait=%s", authFailureLimit, locked, wait)
	}
	if got := throttle.lockedFor(key); got != authLockoutDuration {
		t.Fatalf("lockedFor() = %s, want %s", got, authLockoutDuration)
	}

	now = now.Add(authLockoutDuration + time.Second)
	if got := throttle.lockedFor(key); got != 0 {
		t.Fatalf("lockout did not expire: %s", got)
	}
}

func TestAuthFailuresResetAfterWindow(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	throttle := testThrottle(&now)
	key := "203.0.113.7|EA1ABC"

	for attempt := 1; attempt < authFailureLimit; attempt++ {
		throttle.recordFailure(key)
	}
	now = now.Add(authFailureWindow + time.Minute)
	if locked, _ := throttle.recordFailure(key); locked {
		t.Fatal("failure after the window was counted toward the old window")
	}
}

func TestAuthThrottleIsPerAccountAndAddress(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	throttle := testThrottle(&now)

	for attempt := 0; attempt < authFailureLimit; attempt++ {
		throttle.recordFailure("203.0.113.7|EA1ABC")
	}
	if throttle.lockedFor("203.0.113.7|EA1ABC") == 0 {
		t.Fatal("locked key is not locked")
	}
	// Another caller must not inherit the lockout, and neither must another
	// account from the same proxy address.
	if got := throttle.lockedFor("198.51.100.9|EA1ABC"); got != 0 {
		t.Fatalf("account lockout leaked to another address: %s", got)
	}
	if got := throttle.lockedFor("203.0.113.7|EA7KLK"); got != 0 {
		t.Fatalf("account lockout leaked to another account: %s", got)
	}
}

func TestAuthThrottleClearResetsFailures(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	throttle := testThrottle(&now)
	key := "203.0.113.7|EA1ABC"

	for attempt := 0; attempt < authFailureLimit-1; attempt++ {
		throttle.recordFailure(key)
	}
	throttle.clear(key)
	if locked, _ := throttle.recordFailure(key); locked {
		t.Fatal("a successful login did not reset the failure count")
	}
}

func TestAuthThrottlePrunesIdleEntries(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	throttle := testThrottle(&now)
	old := now.Add(-authFailureRetention - time.Hour)
	for i := 0; i < 100; i++ {
		throttle.entries[fmt.Sprintf("203.0.113.%d|EA1ABC", i)] = authFailure{windowStart: old}
	}
	throttle.recordFailure("203.0.113.7|EA1ABC")
	if len(throttle.entries) != 1 {
		t.Fatalf("throttle kept %d idle entries", len(throttle.entries))
	}
}

func TestAuthThrottleKeyUsesThePeerAddress(t *testing.T) {
	cases := map[string]string{
		"203.0.113.7:5555": "203.0.113.7|EA1ABC",
		"203.0.113.7":      "203.0.113.7|EA1ABC",
		"":                 "unknown|EA1ABC",
	}
	for remoteAddr, want := range cases {
		request := formRequest(http.MethodPost, "/login", url.Values{})
		request.RemoteAddr = remoteAddr
		if got := authThrottleKey(request, " ea1abc "); got != want {
			t.Fatalf("authThrottleKey(%q) = %q, want %q", remoteAddr, got, want)
		}
	}
}

func TestLoginIsThrottledAndClearsOnSuccess(t *testing.T) {
	s := newTestServer(t)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	s.throttle.now = func() time.Time { return now }
	testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Operator", Email: "a@example.invalid", Language: "en", PasswordHash: hashPassword("secret")})

	login := func(password string) *httptest.ResponseRecorder {
		request := formRequest(http.MethodPost, "/login", url.Values{"callsign": {"EA1ABC"}, "password": {password}})
		request.RemoteAddr = "203.0.113.7:1234"
		return invokeHandler(s.loginPost, request)
	}

	for attempt := 0; attempt < authFailureLimit-1; attempt++ {
		if body := login("wrong").Body.String(); !strings.Contains(body, "web_wrong_login") {
			t.Fatalf("attempt %d did not report a wrong password: %q", attempt+1, body)
		}
	}
	if body := login("wrong").Body.String(); !strings.Contains(body, "web_login_locked") {
		t.Fatalf("the limit did not lock the account: %q", body)
	}
	// Even the right password is refused while the lockout stands.
	if body := login("secret").Body.String(); !strings.Contains(body, "web_login_locked") {
		t.Fatalf("correct password bypassed the lockout: %q", body)
	}

	now = now.Add(authLockoutDuration + time.Second)
	response := login("secret")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/" {
		t.Fatalf("login after the lockout = %d %q", response.Code, response.Header().Get("Location"))
	}
	if wait := s.throttle.lockedFor(authThrottleKey(formRequest(http.MethodPost, "/login", nil), "EA1ABC")); wait != 0 {
		t.Fatalf("successful login left a lockout behind: %s", wait)
	}
}
