package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutesSecurityAndAuthorization(t *testing.T) {
	s := newTestServer(t)
	normal := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Operator", Email: "a@example.invalid"})
	sysop := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Sysop", Email: "sysop@example.invalid", IsSysop: true})
	handler := s.routes()

	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/login", nil))
	if login.Code != http.StatusOK || login.Header().Get("X-Content-Type-Options") != "nosniff" || login.Header().Get("Referrer-Policy") != "same-origin" {
		t.Fatalf("login route = %d, headers=%v", login.Code, login.Header())
	}
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/", nil))
	if unauthorized.Code != http.StatusSeeOther || unauthorized.Header().Get("Location") != "/login" {
		t.Fatalf("unauthorized home = %d, location=%q", unauthorized.Code, unauthorized.Header().Get("Location"))
	}

	s.sessions["normal-token"] = normal.Callsign
	authenticated := httptest.NewRequest(http.MethodGet, "/", nil)
	authenticated.AddCookie(&http.Cookie{Name: "bbs_session", Value: "normal-token"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticated)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Test BBS") {
		t.Fatalf("authenticated home = %d %q", response.Code, response.Body.String())
	}
	for _, path := range []string{"/admin/users", "/boards/new", "/bulletins/new"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: "bbs_session", Value: "normal-token"})
		got := httptest.NewRecorder()
		handler.ServeHTTP(got, req)
		if got.Code != http.StatusForbidden {
			t.Fatalf("non-sysop %s status = %d, want 403", path, got.Code)
		}
	}
	for _, path := range []string{"/aprs", "/aprs/received", "/aprs/sent", "/aprs/send"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: "bbs_session", Value: "normal-token"})
		got := httptest.NewRecorder()
		handler.ServeHTTP(got, req)
		if got.Code != http.StatusOK {
			t.Fatalf("normal %s status = %d, body=%q", path, got.Code, got.Body.String())
		}
	}
	s.sessions["sysop-token"] = sysop.Callsign
	for _, path := range []string{"/admin/users", "/boards/new", "/bulletins/new"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: "bbs_session", Value: "sysop-token"})
		got := httptest.NewRecorder()
		handler.ServeHTTP(got, req)
		if got.Code != http.StatusOK {
			t.Fatalf("sysop %s status = %d, body=%q", path, got.Code, got.Body.String())
		}
	}

	notFound := httptest.NewRecorder()
	handler.ServeHTTP(notFound, httptest.NewRequest(http.MethodGet, "/does-not-exist", nil))
	if notFound.Code != http.StatusSeeOther || notFound.Header().Get("Location") != "/login" {
		t.Fatalf("unknown unauthenticated route = %d, location=%q", notFound.Code, notFound.Header().Get("Location"))
	}
	authenticatedNotFound := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	authenticatedNotFound.AddCookie(&http.Cookie{Name: "bbs_session", Value: "normal-token"})
	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, authenticatedNotFound)
	if missing.Code != http.StatusOK {
		t.Fatalf("unknown authenticated route status = %d", missing.Code)
	}
}
