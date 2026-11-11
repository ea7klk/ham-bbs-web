package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The mockups are served to anyone, because the point is to look at them without
// an account, and they must not be indexed.
func TestMockupsAreServedAnonymously(t *testing.T) {
	s := newTestServer(t)
	handler := s.routes()

	pages := map[string]string{
		"/mockups/":                     "Four directions",
		"/mockups/1-front-panel.html":   "Front Panel",
		"/mockups/2-qsl-desk.html":      "QSL Desk",
		"/mockups/3-spectrum.html":      "Spectrum",
		"/mockups/4-packet-manual.html": "Packet Manual",
	}
	for path, marker := range pages {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, response.Code)
		}
		if !strings.Contains(response.Body.String(), marker) {
			t.Fatalf("%s does not look like the expected page (%q)", path, marker)
		}
		if got := response.Header().Get("X-Robots-Tag"); !strings.Contains(got, "noindex") {
			t.Fatalf("%s is indexable: X-Robots-Tag=%q", path, got)
		}
	}
}

// A bare /mockups lands on the index rather than 404ing.
func TestMockupsRootRedirects(t *testing.T) {
	s := newTestServer(t)
	response := httptest.NewRecorder()
	s.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/mockups", nil))
	if response.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", response.Code)
	}
	if got := response.Header().Get("Location"); got != "/mockups/" {
		t.Fatalf("Location = %q", got)
	}
}

// Every mockup must be self-contained: it is opened over HamNet, and a page that
// pulls a font from a CDN is a page that renders wrong on an isolated link.
func TestMockupsRequestNothingExternal(t *testing.T) {
	files, err := mockupFiles.ReadDir("mockups")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 5 {
		t.Fatalf("only %d mockup files embedded", len(files))
	}
	for _, file := range files {
		body, err := mockupFiles.ReadFile("mockups/" + file.Name())
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, scheme := range []string{`src="http://`, `src="https://`, `href="http://`, `href="https://`} {
			if strings.Contains(text, scheme) {
				t.Fatalf("%s references a remote asset (%s)", file.Name(), scheme)
			}
		}
		if !strings.Contains(text, `name="viewport"`) {
			t.Fatalf("%s has no viewport meta, so it will not reflow on a phone", file.Name())
		}
	}
}
