package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *server {
	t.Helper()
	db, err := openDatabase(filepath.Join(t.TempDir(), "bbs.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	logDir := t.TempDir()
	text := map[string]map[string]any{"en": {}}
	return &server{
		cfg: config{
			bbsLog:     filepath.Join(logDir, "bbs.log"),
			aprsLog:    filepath.Join(logDir, "aprs.log"),
			sysops:     map[string]bool{},
			name:       "Test BBS",
			location:   "Test QTH",
			topic:      "Test topic",
			sysopName:  "Test Sysop",
			aprsServer: "127.0.0.1",
			aprsPort:   1,
		},
		db:       db,
		tpl:      parseTemplates(text),
		text:     text,
		sessions: map[string]string{},
	}
}

func testUser(t *testing.T, s *server, user dbUser) *dbUser {
	t.Helper()
	if user.Language == "" {
		user.Language = "en"
	}
	if err := s.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return &user
}

func formRequest(method, path string, values url.Values) *http.Request {
	var body *strings.Reader
	if values == nil {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(values.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if values != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req
}

func invokeHandler(handler func(http.ResponseWriter, *http.Request), req *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler(response, req)
	return response
}

func invokeUserHandler(handler func(http.ResponseWriter, *http.Request, *dbUser), req *http.Request, user *dbUser) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler(response, req, user)
	return response
}

func setPathValue(req *http.Request, name, value string) *http.Request {
	req.SetPathValue(name, value)
	return req
}

func assertRedirect(t *testing.T, response *httptest.ResponseRecorder, location string) {
	t.Helper()
	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body=%q", response.Code, http.StatusSeeOther, response.Body.String())
	}
	if got := response.Header().Get("Location"); got != location {
		t.Fatalf("Location = %q, want %q", got, location)
	}
}
